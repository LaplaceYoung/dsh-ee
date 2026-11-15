package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 可配置的管理角色（docs/access-control.md §3）：可以关掉的超级管理员、互斥角色、不能给自己授权、双人批准、预设。
// 都是开关，默认关着，和原来的行为一样。

const (
	adminPolicySetting = "admin_policy"
	auditPolicy        = "admin_policy.update"

	// 双人批准可以管的操作
	DualRolesAssign = "roles.assign"
	DualConfig      = "config"
)

type adminPolicy struct {
	// 关掉超级管理员：所有地方只按具体权限点判断
	SuperDisabled bool `json:"super_disabled" yaml:"super_disabled"`
	// 管理员不能改自己的角色和属性
	NoSelfGrant bool `json:"no_self_grant" yaml:"no_self_grant"`
	// 几组角色（角色代码），同一组里的不能由同一个人同时持有
	Exclusive [][]string `json:"exclusive" yaml:"exclusive,omitempty"`
	// 要另一位管理员批准才生效的操作：roles.assign（给人授带管理权限的角色）、config（改字段、规则、角色和这些约束）
	DualApproval []string `json:"dual_approval" yaml:"dual_approval,omitempty"`
}

func (p adminPolicy) dual(action string) bool { return slices.Contains(p.DualApproval, action) }

func loadAdminPolicy(ctx context.Context, st *store.Store) adminPolicy {
	var p adminPolicy
	_, _ = st.GetSettingJSON(ctx, adminPolicySetting, &p)
	return p
}

func (s *Server) adminPolicy(ctx context.Context) adminPolicy {
	p := loadAdminPolicy(ctx, s.store)
	// 多副本时别的副本改了也要跟上
	s.store.SetSuperDisabled(p.SuperDisabled)
	return p
}

// selfGrantAllowed：管理员能不能改自己的角色和属性。
func (s *Server) selfGrantAllowed(ctx context.Context) bool { return !s.adminPolicy(ctx).NoSelfGrant }

// topAdmin 是导入示例内容这类「全局大操作」要的人：超级管理员；关掉超级管理员后，要同时有账号、连接、设置三类管理权限。
func (s *Server) topAdmin(a *rbac.Access) bool {
	if a.Super {
		return true
	}
	if !s.store.SuperDisabled() {
		return false
	}
	for _, p := range []string{rbac.UsersManage, rbac.ConnectionsManage, rbac.SettingsManage, rbac.PluginsManage} {
		if !a.Has(p) {
			return false
		}
	}
	return true
}

// policyProblems 检查一份约束能不能生效：关超级管理员前每个管理权限点都要有人持有；互斥的角色要存在、现在没人同时持有。
func (s *Server) policyProblems(ctx context.Context, p *adminPolicy) ([]string, error) {
	var out []string
	for _, a := range p.DualApproval {
		if a != DualRolesAssign && a != DualConfig {
			out = append(out, "双人批准只能管 "+DualRolesAssign+" 和 "+DualConfig)
		}
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	codeOf := map[int64]string{}
	known := map[string]bool{}
	for _, r := range roles {
		codeOf[r.ID] = r.Code
		known[r.Code] = true
	}
	var groups [][]string
	for _, g := range p.Exclusive {
		g = dedupeStrings(g)
		if len(g) < 2 {
			continue
		}
		for _, c := range g {
			if !known[c] {
				out = append(out, "互斥角色里的 "+c+" 不存在")
			}
			if c == rbac.RoleSuper || c == rbac.RoleMember {
				out = append(out, "内置的 "+c+" 角色不能放进互斥组")
			}
		}
		groups = append(groups, g)
	}
	p.Exclusive = groups
	if len(groups) > 0 {
		refs, err := s.store.UserRoleRefs(ctx)
		if err != nil {
			return nil, err
		}
		for uid, rs := range refs {
			var codes []string
			for _, r := range rs {
				codes = append(codes, codeOf[r.ID])
			}
			if g := exclusiveClash(groups, codes); g != "" {
				out = append(out, fmt.Sprintf("用户 %s 现在同时有互斥的角色 %s，先拿掉其中一个", s.usernameByID(ctx, uid), g))
			}
		}
	}
	if p.SuperDisabled {
		held, err := s.store.PermissionHolders(ctx)
		if err != nil {
			return nil, err
		}
		var missing []string
		for _, perm := range rbac.Catalog {
			if rbac.IsAdminPerm(perm.Code) && held[perm.Code] == 0 {
				missing = append(missing, perm.Name+"（"+perm.Code+"）")
			}
		}
		if len(missing) > 0 {
			out = append(out, "关掉超级管理员之前，每个管理权限点都要有一个启用中的人经由普通角色持有，不然会把自己锁在外面。现在没人持有："+strings.Join(missing, "、"))
		}
	}
	return out, nil
}

func (s *Server) usernameByID(ctx context.Context, id int64) string {
	if u, err := s.store.GetUser(ctx, id); err == nil {
		return u.Username
	}
	return fmt.Sprint(id)
}

// exclusiveClash 返回 codes 里同时出现的一组互斥角色（写成「a 和 b」），没有就是空。
func exclusiveClash(groups [][]string, codes []string) string {
	for _, g := range groups {
		var hit []string
		for _, c := range g {
			if slices.Contains(codes, c) {
				hit = append(hit, c)
			}
		}
		if len(hit) >= 2 {
			return strings.Join(hit, " 和 ")
		}
	}
	return ""
}

// GET /api/admin/policy
func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	p := s.adminPolicy(r.Context())
	if p.Exclusive == nil {
		p.Exclusive = [][]string{}
	}
	if p.DualApproval == nil {
		p.DualApproval = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": p, "presets": policyPresets})
}

// PUT /api/admin/policy：改管理约束。开了双人批准（config）时要走变更请求。
func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	var req adminPolicy
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	cur := s.adminPolicy(ctx)
	if cur.dual(DualConfig) {
		writeErr(w, http.StatusConflict, "开了双人批准：管理约束要通过配置变更提交（dsh-ee config apply，或「字段与访问规则 → 变更请求」）")
		return
	}
	if msg := s.setPolicy(r, cur, req); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	s.handleGetPolicy(w, r)
}

// setPolicy 检查并保存一份管理约束。
func (s *Server) setPolicy(r *http.Request, cur, req adminPolicy) string {
	ctx := r.Context()
	// 关掉超级管理员要超级管理员本人来做；重新打开要能管角色的人（关掉之后没有超级管理员了）
	a := currentAccess(r)
	if req.SuperDisabled && !cur.SuperDisabled && !a.Super {
		return "只有超级管理员能关掉超级管理员"
	}
	problems, err := s.policyProblems(ctx, &req)
	if err != nil {
		return err.Error()
	}
	if len(problems) > 0 {
		return strings.Join(problems, "；")
	}
	if err := s.store.PutSettingJSON(ctx, adminPolicySetting, req, currentUser(r).ID); err != nil {
		return err.Error()
	}
	s.store.SetSuperDisabled(req.SuperDisabled)
	s.audit(r, auditPolicy, adminPolicySetting, map[string]any{"before": cur, "after": req})
	s.hub.notify()
	return ""
}

// ---- 预设（docs/access-control.md §3.3）----

type presetRole struct {
	Code, Name string
	Perms      []string
}

var separatedRoles = []presetRole{
	{"sys-admin", "系统管理员", []string{rbac.UsersView, rbac.UsersManage, rbac.MenusManage, rbac.ProvidersManage, rbac.QuotasManage, rbac.KeysManage,
		rbac.DashboardView, rbac.PluginsManage, rbac.ConnectionsManage, rbac.KnowledgeManage, rbac.RegistryManage, rbac.UpstreamManage,
		rbac.DistributionManage, rbac.SettingsManage, rbac.TokensDeptManage}},
	{"access-admin", "权限管理员", []string{rbac.UsersView, rbac.RolesManage, rbac.FieldsManage}},
	{"auditor", "审计员", []string{rbac.AuditView, rbac.DashboardView}},
}

var policyPresets = []map[string]any{
	{"name": "standard", "title": "标准", "description": "一个超级管理员，什么都能做（默认）"},
	{"name": "separated", "title": "分权管理", "description": "建系统管理员、权限管理员、审计员三个角色并设为互斥。给人分好角色后，再在这里关掉超级管理员"},
}

// POST /api/admin/policy/preset {name}
func (s *Server) handleApplyPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	cur := s.adminPolicy(ctx)
	if cur.dual(DualConfig) {
		writeErr(w, http.StatusConflict, "开了双人批准：预设要通过配置变更提交")
		return
	}
	switch req.Name {
	case "standard":
		next := adminPolicy{}
		if msg := s.setPolicy(r, cur, next); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	case "separated":
		if !currentAccess(r).Super && !currentAccess(r).Has(rbac.RolesManage) {
			writeErr(w, http.StatusForbidden, "没有权限")
			return
		}
		var codes []string
		for _, pr := range separatedRoles {
			codes = append(codes, pr.Code)
			if _, err := s.store.GetRoleByCode(ctx, pr.Code); err == nil {
				continue
			}
			if !subset(currentAccess(r), pr.Perms) {
				writeErr(w, http.StatusForbidden, "建「"+pr.Name+"」要你自己有它的全部权限")
				return
			}
			role := &store.Role{Code: pr.Code, Name: pr.Name, Description: "分权管理预设", Permissions: pr.Perms}
			if err := s.store.CreateRole(ctx, role); err != nil {
				s.internalErr(w, r, err)
				return
			}
		}
		next := cur
		next.Exclusive = append(slices.Clone(cur.Exclusive), codes)
		if msg := s.setPolicy(r, cur, next); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "没有这个预设：standard / separated")
		return
	}
	s.handleGetPolicy(w, r)
}

// ---- 给人改角色：互斥、不能给自己、双人批准 ----

// roleChangeProblem 检查给 target 换成 ids 这几个角色是否违反管理约束。返回问题和这次是不是授出了管理权限。
func (s *Server) roleChangeProblem(ctx context.Context, p adminPolicy, actorID, targetID int64, before *rbac.Access, ids []int64) (string, bool, error) {
	if p.NoSelfGrant && actorID == targetID {
		return "不能改自己的角色：请另一位管理员来改", false, nil
	}
	var codes []string
	grantsAdmin := false
	for _, id := range ids {
		role, err := s.store.GetRole(ctx, id)
		if err != nil {
			return "角色不存在", false, nil
		}
		codes = append(codes, role.Code)
		if slices.Contains(before.RoleIDs, id) {
			continue
		}
		if role.IsSuper {
			grantsAdmin = true
		}
		for _, perm := range role.Permissions {
			if rbac.IsAdminPerm(perm) {
				grantsAdmin = true
			}
		}
	}
	if g := exclusiveClash(p.Exclusive, codes); g != "" {
		return "角色 " + g + " 是互斥的，不能由同一个人同时持有", grantsAdmin, nil
	}
	return "", grantsAdmin, nil
}

// submitRolesChange 在开了双人批准时把授角色变成一条待批的变更请求。
func (s *Server) submitRolesChange(r *http.Request, targetID int64, ids []int64) (*store.ChangeRequest, error) {
	payload, _ := json.Marshal(map[string]any{"user_id": targetID, "role_ids": ids})
	c := &store.ChangeRequest{Kind: store.ChangeUserRoles, Title: "给 " + s.usernameByID(r.Context(), targetID) + " 改角色", Payload: string(payload), CreatedBy: currentUser(r).ID}
	if err := s.store.CreateChangeRequest(r.Context(), c); err != nil {
		return nil, err
	}
	s.audit(r, "change_request.create", fmt.Sprint(c.ID), map[string]any{"kind": c.Kind, "title": c.Title})
	return c, nil
}

// ---- 恢复账号（docs/access-control.md §3.4）----

// Recover 在服务器上生成一个一次性的恢复账号：有全部管理权限点（不靠超级管理员），1 小时后自动停用。
// 把自己锁在外面（比如关掉超级管理员后唯一的权限管理员离职了）时用。审计里留记录。
//
// 审计写入失败时账号已经建好了：此时仍会填好 username/password/expires 并返回非 nil error。
// 调用方必须把凭据交给操作人，同时明确告警「账号已创建但审计没写成」，让人工补记并排查 audit_logs 表；
// 不能因为 err 非 nil 就当没创建、把凭据丢掉 —— 那会让管理员以为没拿到入口，而库里躺着一个无审计的全权账号。
func Recover(ctx context.Context, st *store.Store) (username, password string, expires time.Time, err error) {
	if err := st.SyncPermissions(ctx, rbac.Catalog); err != nil {
		return "", "", time.Time{}, err
	}
	role, err := st.GetRoleByCode(ctx, "recovery")
	var perms []string
	for _, p := range rbac.Catalog {
		perms = append(perms, p.Code)
	}
	if err != nil {
		role = &store.Role{Code: "recovery", Name: "应急恢复", Description: "dsh-ee admin recover 生成的恢复账号用，1 小时后账号自动停用", Permissions: perms}
		if err := st.CreateRole(ctx, role); err != nil {
			return "", "", time.Time{}, err
		}
	} else if err := st.UpdateRole(ctx, role.ID, store.RolePatch{Permissions: perms}); err != nil {
		return "", "", time.Time{}, err
	}
	username = "recover-" + secret.Token()[:8]
	password = secret.Token()[:24]
	hash, err := HashPassword(password)
	if err != nil {
		return "", "", time.Time{}, err
	}
	u := &store.User{Username: strings.ToLower(username), DisplayName: "应急恢复", PasswordHash: hash}
	if err := st.CreateUser(ctx, u); err != nil {
		return "", "", time.Time{}, err
	}
	if err := st.SetUserRoles(ctx, u.ID, []int64{role.ID}); err != nil {
		return "", "", time.Time{}, err
	}
	expires = time.Now().Add(time.Hour)
	if err := st.AddRecoveryAccount(ctx, u.ID, expires); err != nil {
		return "", "", time.Time{}, err
	}
	if err := st.InsertAudit(ctx, &store.AuditLog{ActorID: &u.ID, ActorName: u.Username, Action: "admin.recover", Target: u.Username, Via: "cli",
		Detail: map[string]any{"expires_at": expires}}); err != nil {
		// 账号此时已完整创建（角色/密码/到期停用都生效），只是审计没写成。
		// 照常把凭据带回去，让调用方告警并让人工补记；绝不能静默吞掉审计错误。
		return u.Username, password, expires, fmt.Errorf("应急恢复账号 %q 已创建（%s 到期自动停用），但审计记录写入 audit_logs 失败；请人工记录该账号与到期时间并排查审计表：%w", u.Username, expires.Format(time.RFC3339), err)
	}
	return u.Username, password, expires, nil
}

// runRecoveryExpiry 每分钟停用过期的恢复账号。
func (s *Server) runRecoveryExpiry(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runLeased(ctx, leaseRecoveryExpiry, func(ctx context.Context) {
				s.expireRecovery(ctx)
				s.adminPolicy(ctx)
			})
		}
	}
}

func (s *Server) expireRecovery(ctx context.Context) {
	ids, err := s.store.ExpiredRecoveryAccounts(ctx)
	if err != nil {
		s.log.Warn("查过期的恢复账号失败", "err", err)
		return
	}
	for _, id := range ids {
		on := true
		if err := s.store.UpdateUser(ctx, id, store.UserPatch{Disabled: &on}); err != nil {
			s.log.Warn("停用恢复账号失败", "user", id, "err", err)
			continue
		}
		_ = s.store.DeleteUserSessions(ctx, id)
		s.log.Info("恢复账号已到期停用", "user", id)
	}
}
