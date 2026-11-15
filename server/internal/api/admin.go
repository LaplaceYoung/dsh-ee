package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// ---- 用户 ----

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	refs, err := s.store.UserRoleRefs(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	for _, u := range users {
		u.Roles = nonNil(refs[u.ID])
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": nonNil(users)})
}

type createUserReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Department  string `json:"department"`
	// 不传时给默认的「员工」角色；传 [] 表示不分配任何角色。
	RoleIDs           *[]int64      `json:"role_ids"`
	MonthlyTokenLimit nullableInt64 `json:"monthly_token_limit"`
	// local（默认）/ ldap：预先建好的目录账号，不设密码，首次登录时由目录验证并补全资料
	AuthSource  string `json:"auth_source"`
	Email       string `json:"email"`
	AccountType string `json:"account_type"`
	OwnerID     *int64 `json:"owner_id"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if !usernameRe.MatchString(req.Username) {
		writeErrCode(w, http.StatusBadRequest, codeUserInvalidUsername, nil, "用户名只能包含小写字母、数字和 . _ -，2 到 64 个字符")
		return
	}
	var roleIDs []int64
	if req.RoleIDs != nil {
		roleIDs = *req.RoleIDs
	} else {
		member, err := s.store.GetRoleByCode(r.Context(), rbac.RoleMember)
		if err != nil && !notFound(err) {
			s.internalErr(w, r, err)
			return
		}
		if member != nil {
			roleIDs = []int64{member.ID}
		}
	}
	// 分配角色属于「管理角色」的权限；只有用户管理权限时只能用默认角色
	if req.RoleIDs != nil && !currentAccess(r).Has(rbac.RolesManage) {
		writeErrCode(w, http.StatusForbidden, codeUserRolesAssignForbidden, nil, "分配角色需要「管理角色与权限」权限")
		return
	}
	// 默认角色是系统约定，不算「授予」；显式指定的角色要过防提权检查
	if req.RoleIDs != nil {
		if e, ok := s.checkGrantable(r, roleIDs); !ok {
			writeErrCode(w, http.StatusForbidden, e.code, e.params, e.msg)
			return
		}
	}
	// 额度属于「分配额度」权限：只有「管理用户」权限时，不能借建号显式设置或清空额度
	if !canSetQuotaOnCreate(currentAccess(r), req.MonthlyTokenLimit) {
		writeErrCode(w, http.StatusForbidden, codeUserQuotaAssignForbidden, nil, "设置月度额度需要「分配额度」权限")
		return
	}
	if req.AccountType == "" {
		req.AccountType = store.AccountHuman
	}
	if req.AccountType != store.AccountHuman && req.AccountType != store.AccountBot {
		writeErrCode(w, http.StatusBadRequest, codeUserInvalidAccountType, nil, "account_type 只能是 human 或 bot")
		return
	}
	if req.AccountType == store.AccountBot && (req.AuthSource != "" && req.AuthSource != store.AuthLocal || req.Password != "") {
		writeErrCode(w, http.StatusBadRequest, codeUserBotCredentialsForbidden, nil, "机器人账号不能设置密码或 LDAP 登录")
		return
	}
	var hash string
	if req.AccountType == store.AccountHuman {
		switch req.AuthSource {
		case "", store.AuthLocal:
			req.AuthSource = store.AuthLocal
			h, err := HashPassword(req.Password)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			hash = h
		case store.AuthLDAP, store.AuthOIDC:
			if req.Password != "" {
				writeErrCode(w, http.StatusBadRequest, codeUserExternalPasswordForbidden, nil, "LDAP / OIDC 账号不设本地密码")
				return
			}
		default:
			writeErrCode(w, http.StatusBadRequest, codeUserInvalidAuthSource, nil, "auth_source 只能是 local、ldap 或 oidc")
			return
		}
	}
	// 新用户默认 0：管理员分配额度后才能调用模型；显式传 null 表示不限额（要「分配额度」权限）。
	limit := int64(0)
	if req.MonthlyTokenLimit.Set {
		if req.MonthlyTokenLimit.Value != nil && *req.MonthlyTokenLimit.Value < 0 {
			writeErrCode(w, http.StatusBadRequest, codeUserInvalidQuota, nil, "monthly_token_limit 不能为负数")
			return
		}
		if req.MonthlyTokenLimit.Value != nil {
			limit = *req.MonthlyTokenLimit.Value
		}
	}
	u := &store.User{
		Username:          req.Username,
		DisplayName:       strings.TrimSpace(req.DisplayName),
		Department:        strings.TrimSpace(req.Department),
		Email:             strings.TrimSpace(req.Email),
		PasswordHash:      hash,
		MonthlyTokenLimit: &limit,
		AuthSource:        req.AuthSource,
		AccountType:       req.AccountType,
		OwnerID:           req.OwnerID,
	}
	if req.MonthlyTokenLimit.Set && req.MonthlyTokenLimit.Value == nil {
		u.MonthlyTokenLimit = nil
	}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErrCode(w, http.StatusConflict, codeUserExists, nil, "用户名已存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.SetUserRoles(r.Context(), u.ID, dedupe(roleIDs)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	action := auditUserCreate
	if u.AccountType == store.AccountBot {
		action = "user.create_bot"
	}
	s.audit(r, action, u.Username, map[string]any{"id": u.ID, "source": u.AuthSource, "roles": dedupe(roleIDs), "owner_id": u.OwnerID, "monthly_token_limit": u.MonthlyTokenLimit})
	writeJSON(w, http.StatusCreated, u)
}

// canSetQuotaOnCreate：建账号时能不能显式设置 / 清空月度额度。额度属于「分配额度」
// 权限，只有「管理用户」时不能借建号开出不限额账号；不传这个字段按默认额度处理。
func canSetQuotaOnCreate(a *rbac.Access, limit nullableInt64) bool {
	return !limit.Set || a.Has(rbac.QuotasManage)
}

type updateUserReq struct {
	DisplayName *string `json:"display_name"`
	Department  *string `json:"department"`
	Disabled    *bool   `json:"disabled"`
	Password    *string `json:"password"`
	// 解除 OIDC 账号绑定的 sub：统一登录里重建了账号时用，下次登录重新绑定
	UnbindIdentity bool `json:"unbind_identity"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateUserReq
	if !readJSON(w, r, &req) {
		return
	}
	me := currentUser(r)
	if id == me.ID && req.Disabled != nil && *req.Disabled {
		writeErrCode(w, http.StatusBadRequest, codeUserCannotDisableSelf, nil, "不能禁用自己")
		return
	}
	if id == me.ID && req.UnbindIdentity {
		writeErrCode(w, http.StatusBadRequest, codeUserCannotUnbindSelf, nil, "不能解除自己的身份绑定")
		return
	}
	target, err := s.store.UserAccess(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 不能动权限比自己大的人，否则禁用、重置密码或解除身份绑定就能变相夺权
	if (req.Disabled != nil || req.Password != nil || req.UnbindIdentity) && !outranks(currentAccess(r), target) {
		writeErrCode(w, http.StatusForbidden, codeUserOutrankedTarget, nil, "不能禁用、重置密码或解除权限比自己大的用户的身份绑定")
		return
	}
	// 部门是资产 / 知识库的可见范围，也是「本部门令牌管理」的作用域：改部门等于替
	// 目标调整可访问的范围，所以只能由权限严格大于目标的管理员调整下级部门，
	// 改自己、同级或上级都不行（超级管理员本来就绕开一切检查，不受此限）。
	if req.Department != nil && !canChangeDepartment(currentAccess(r), target) {
		writeErrCode(w, http.StatusForbidden, codeUserDepartmentChangeForbidden, nil, "不能修改自己、同级或权限比自己大的用户的部门")
		return
	}
	// 部门变更要记前后值，先读一次变更前的用户
	var before *store.User
	if req.Department != nil {
		before, err = s.store.GetUser(r.Context(), id)
		if err != nil {
			if notFound(err) {
				writeErrCode(w, http.StatusNotFound, codeUserNotFound, nil, "用户不存在")
				return
			}
			s.internalErr(w, r, err)
			return
		}
	}
	if target.Super && req.Disabled != nil && *req.Disabled {
		if n, err := s.store.CountSuperUsers(r.Context(), id); err != nil {
			s.internalErr(w, r, err)
			return
		} else if n == 0 {
			writeErrCode(w, http.StatusBadRequest, codeUserLastSuperAdminDisable, nil, "这是最后一个启用中的超级管理员，不能禁用")
			return
		}
	}
	p := store.UserPatch{DisplayName: req.DisplayName, Department: req.Department, Disabled: req.Disabled}
	if req.Password != nil {
		if u, err := s.store.GetUser(r.Context(), id); err == nil && (u.AuthSource == store.AuthLDAP || u.AuthSource == store.AuthOIDC) {
			writeErrCode(w, http.StatusBadRequest, codeUserPasswordManagedExternally, nil, "LDAP / OIDC 账号的密码由企业目录或统一登录管理，不能在这里重置")
			return
		}
		hash, err := HashPassword(*req.Password)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		p.PasswordHash = &hash
	}
	if req.UnbindIdentity {
		if u, err := s.store.GetUser(r.Context(), id); err != nil || u.AuthSource != store.AuthOIDC {
			writeErrCode(w, http.StatusBadRequest, codeUserNoIdentityBinding, nil, "只有企业统一登录（OIDC）的账号有身份绑定可以解除")
			return
		}
		none := ""
		p.ExternalDN = &none
	}
	if err := s.store.UpdateUser(r.Context(), id, p); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeUserNotFound, nil, "用户不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if req.Disabled != nil && *req.Disabled {
		// 停用的员工按人存的连接凭据一起删掉（docs/plugins.md §7.6）
		s.purgeUserCredentials(r.Context(), id)
	}
	// 禁用、重置密码或解除身份绑定后，踢掉该用户所有会话
	if (req.Disabled != nil && *req.Disabled) || req.Password != nil || req.UnbindIdentity {
		if err := s.store.DeleteUserSessions(r.Context(), id); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	u, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	detail := map[string]any{"id": id}
	if req.Disabled != nil {
		detail["disabled"] = *req.Disabled
	}
	if req.Password != nil {
		detail["password_reset"] = true
	}
	if req.UnbindIdentity {
		detail["identity_unbound"] = true
	}
	if req.DisplayName != nil || req.Department != nil {
		detail["profile"] = true
	}
	if req.Department != nil {
		detail["old_department"] = before.Department
		detail["new_department"] = u.Department
	}
	s.audit(r, auditUserUpdate, u.Username, detail)
	s.hub.notify()
	writeJSON(w, http.StatusOK, u)
}

// canChangeDepartment：actor 能不能改目标的部门。部门决定资产 / 知识库的可见范围，
// 也是「本部门令牌管理」的作用域，所以只允许调整权限严格小于自己的下级：自己、
// 同级（权限集合相同）和权限更大的用户都不行。超级管理员不受限。
func canChangeDepartment(actor, target *rbac.Access) bool {
	if actor.Super {
		return true
	}
	if target.Super || len(target.Perms) >= len(actor.Perms) {
		return false
	}
	for p := range target.Perms {
		if !actor.Perms[p] {
			return false
		}
	}
	return true
}

// ---- 代发 key（员工也可以在自己的「我的 API key」里自助创建）----

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys(r.Context(), 0)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": nonNil(keys)})
}

type issueKeyReq struct {
	UserID    int64      `json:"user_id"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

const keyPrefix = "sk-ee-"

func (s *Server) handleIssueKey(w http.ResponseWriter, r *http.Request) {
	var req issueKeyReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErrCode(w, http.StatusBadRequest, codeKeyInvalidName, nil, "name 不能为空")
		return
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		writeErrCode(w, http.StatusBadRequest, codeKeyInvalidExpiry, nil, "expires_at 必须是将来的时间")
		return
	}
	owner, err := s.store.GetUser(r.Context(), req.UserID)
	if notFound(err) {
		writeErrCode(w, http.StatusBadRequest, codeKeyUserNotFound, nil, "user_id 对应的用户不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if owner.Disabled {
		writeErrCode(w, http.StatusBadRequest, codeKeyUserDisabled, nil, "该用户已被禁用")
		return
	}

	plain := keyPrefix + secret.Token()
	k := &store.APIKey{
		UserID:    owner.ID,
		Username:  owner.Username,
		Name:      req.Name,
		Prefix:    plain[:len(keyPrefix)+6],
		ExpiresAt: req.ExpiresAt,
		CreatedBy: currentUser(r).ID,
	}
	if k.ExpiresAt != nil {
		t := k.ExpiresAt.UTC()
		k.ExpiresAt = &t
	}
	if err := s.store.CreateAPIKey(r.Context(), k, secret.Hash(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKeyCreate, owner.Username, map[string]any{"id": k.ID, "name": k.Name, "prefix": k.Prefix, "self": false})
	// 明文只在这里返回一次
	writeJSON(w, http.StatusCreated, map[string]any{"key": plain, "api_key": k})
}

func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	k, _ := s.store.GetAPIKey(r.Context(), id)
	if err := s.store.RevokeAPIKey(r.Context(), id); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeKeyNotFoundOrRevoked, nil, "key 不存在或已吊销")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if k != nil {
		s.audit(r, auditKeyRevoke, k.Username, map[string]any{"id": id, "name": k.Name, "prefix": k.Prefix, "self": false})
	}
	w.WriteHeader(http.StatusNoContent)
}

// nonNil 让空列表序列化成 [] 而不是 null。
func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}
