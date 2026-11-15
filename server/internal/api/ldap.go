package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/ldapauth"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// LDAP 配置单独存一行设置：里面有服务账号，/api/settings 对所有登录用户可读，不能放在那里。
const (
	ldapSettingsKey = "ldap"
	ldapSyncKey     = "ldap_sync"
)

// ldapStored 是落库的形式：服务账号密码用主密钥加密。
type ldapStored struct {
	ldapauth.Config
	BindPasswordEnc string `json:"bindPasswordEnc,omitempty"`
}

func (s *Server) loadLDAP(ctx context.Context) (*ldapauth.Config, error) {
	st := ldapStored{Config: *ldapauth.Defaults()}
	if _, err := s.store.GetSettingJSON(ctx, ldapSettingsKey, &st); err != nil {
		return nil, err
	}
	if st.RoleMappings == nil {
		st.RoleMappings = []ldapauth.RoleMapping{}
	}
	cfg := st.Config
	if st.BindPasswordEnc != "" {
		raw, err := base64.StdEncoding.DecodeString(st.BindPasswordEnc)
		if err != nil {
			return nil, err
		}
		pw, err := s.box.Open(raw)
		if err != nil {
			return nil, fmt.Errorf("解密 LDAP 服务账号密码（主密钥是否换过？）: %w", err)
		}
		cfg.BindPassword = string(pw)
	}
	cfg.ExtraAttrs = s.syncAttrs(ctx, "ldap")
	return &cfg, nil
}

func (s *Server) saveLDAP(ctx context.Context, cfg *ldapauth.Config, userID int64) error {
	st := ldapStored{Config: *cfg}
	if cfg.BindPassword != "" {
		sealed, err := s.box.Seal([]byte(cfg.BindPassword))
		if err != nil {
			return err
		}
		st.BindPasswordEnc = base64.StdEncoding.EncodeToString(sealed)
	}
	return s.store.PutSettingJSON(ctx, ldapSettingsKey, st, userID)
}

// ldapEnabled 用于登录页提示与系统设置里的 auth 字段。读不到配置时当作没开。
func (s *Server) ldapEnabled(ctx context.Context) bool {
	cfg, err := s.loadLDAP(ctx)
	return err == nil && cfg.Enabled && cfg.URL != ""
}

// ---- 管理接口 ----

type ldapView struct {
	ldapauth.Config
	BindPasswordSet bool            `json:"bindPasswordSet"`
	LastSync        *ldapSyncResult `json:"lastSync"`
}

// GET /api/admin/auth/ldap
func (s *Server) handleGetLDAP(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadLDAP(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	v := ldapView{Config: *cfg, BindPasswordSet: cfg.BindPassword != ""}
	var last ldapSyncResult
	if ok, err := s.store.GetSettingJSON(r.Context(), ldapSyncKey, &last); err != nil {
		s.internalErr(w, r, err)
		return
	} else if ok {
		v.LastSync = &last
	}
	writeJSON(w, http.StatusOK, v)
}

// ldapInput 是页面提交的配置。bindPassword 不传表示沿用已存的，传空串表示清除。
type ldapInput struct {
	ldapauth.Config
	BindPassword *string `json:"bindPassword"`
	// 只读字段，页面会原样带回来，忽略即可
	BindPasswordSet *bool           `json:"bindPasswordSet"`
	LastSync        *ldapSyncResult `json:"lastSync"`
}

// resolveInput 把页面提交的配置补上已存的服务账号密码并校验。
func (s *Server) resolveInput(ctx context.Context, in *ldapInput) (*ldapauth.Config, string, error) {
	cfg := in.Config
	if in.BindPassword != nil {
		cfg.BindPassword = *in.BindPassword
	} else {
		cur, err := s.loadLDAP(ctx)
		if err != nil {
			return nil, "", err
		}
		cfg.BindPassword = cur.BindPassword
	}
	if cfg.BindDN == "" {
		cfg.BindPassword = ""
	}
	if msg := cfg.Validate(); msg != "" {
		return nil, msg, nil
	}
	if cfg.Enabled && cfg.BindDN != "" && cfg.BindPassword == "" {
		return nil, "填了服务账号就要填它的密码", nil
	}
	return &cfg, "", nil
}

// PUT /api/admin/auth/ldap
func (s *Server) handlePutLDAP(w http.ResponseWriter, r *http.Request) {
	var in ldapInput
	if !readJSON(w, r, &in) {
		return
	}
	cfg, msg, err := s.resolveInput(r.Context(), &in)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	cur, err := s.loadLDAP(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 组 → 角色映射等于给目录里的一批人授予角色：要有「管理角色」权限，且只能映射自己能授予的角色
	if !slices.Equal(cur.RoleMappings, cfg.RoleMappings) {
		if !currentAccess(r).Has(rbac.RolesManage) {
			writeErr(w, http.StatusForbidden, "修改组与角色的映射需要「管理角色与权限」权限")
			return
		}
		if msg, ok := s.checkGrantable(r, cfg.ManagedRoles()); !ok {
			writeErr(w, http.StatusForbidden, msg)
			return
		}
	}
	if err := s.saveLDAP(r.Context(), cfg, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.syncAuthSetting(r.Context(), cfg.Enabled, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditLDAPUpdate, cfg.URL, map[string]any{"enabled": cfg.Enabled, "mappings": len(cfg.RoleMappings), "password_changed": in.BindPassword != nil})
	s.handleGetLDAP(w, r)
}

// syncAuthSetting 让系统设置里的 auth 字段跟着 LDAP 开关走（前端据此显示当前认证方式）。
func (s *Server) syncAuthSetting(ctx context.Context, ldapOn bool, userID int64) error {
	v, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	want := "local"
	if ldapOn {
		want = "ldap"
	}
	if v.Auth == want {
		return nil
	}
	v.Auth = want
	if err := s.store.SaveSettings(ctx, v, userID); err != nil {
		return err
	}
	s.settings.mu.Lock()
	s.settings.v = nil
	s.settings.mu.Unlock()
	return nil
}

type ldapTestReq struct {
	ldapInput
	TestUsername string `json:"testUsername"`
	TestPassword string `json:"testPassword"`
}

type ldapTestResult struct {
	OK bool `json:"ok"`
	// connect：连接与服务账号绑定 / search：找用户 / verify：验证密码
	Steps []ldapTestStep  `json:"steps"`
	Entry *ldapauth.Entry `json:"entry"`
	// 按映射会得到的角色
	Roles []store.RoleRef `json:"roles"`
	// 登录后在 dsh-ee 里的用户名；与已有本地账号重名时不能登录
	Username string `json:"username"`
	Conflict string `json:"conflict"`
}

type ldapTestStep struct {
	Step  string `json:"step"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// POST /api/admin/auth/ldap/test：用页面上还没保存的配置试连，可选地试登录一个账号。
func (s *Server) handleTestLDAP(w http.ResponseWriter, r *http.Request) {
	var req ldapTestReq
	if !readJSON(w, r, &req) {
		return
	}
	cfg, msg, err := s.resolveInput(r.Context(), &req.ldapInput)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if cfg.URL == "" {
		writeErr(w, http.StatusBadRequest, "请先填目录地址")
		return
	}
	res := ldapTestResult{Steps: []ldapTestStep{}, Roles: []store.RoleRef{}}
	step := func(name string, err error) bool {
		st := ldapTestStep{Step: name, OK: err == nil}
		if err != nil {
			st.Error = err.Error()
		}
		res.Steps = append(res.Steps, st)
		return err == nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cn, err := cfg.Dial(ctx)
	if !step("connect", err) {
		writeJSON(w, http.StatusOK, res)
		return
	}
	defer cn.Close()
	username := strings.TrimSpace(req.TestUsername)
	if username == "" {
		res.OK = true
		writeJSON(w, http.StatusOK, res)
		return
	}
	e, err := cn.Lookup(username)
	if !step("search", err) {
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.Entry = e
	res.Username = strings.ToLower(e.Username)
	if res.Username == "" {
		res.Username = strings.ToLower(username)
	}
	if !usernameRe.MatchString(res.Username) {
		res.Conflict = "invalid"
	} else if u, err := s.store.GetUserByUsername(r.Context(), res.Username); err == nil && u.AuthSource != store.AuthLDAP {
		res.Conflict = "local"
	}
	roles, err := s.store.ListRoles(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	matched := cfg.MatchRoles(e.Groups)
	for _, role := range roles {
		if slices.Contains(matched, role.ID) {
			res.Roles = append(res.Roles, store.RoleRef{ID: role.ID, Code: role.Code, Name: role.Name, IsSuper: role.IsSuper})
		}
	}
	if req.TestPassword != "" {
		if !step("verify", cn.Verify(e.DN, req.TestPassword)) {
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	res.OK = true
	writeJSON(w, http.StatusOK, res)
}

// POST /api/admin/auth/ldap/sync：立即核对一次目录。
func (s *Server) handleSyncLDAP(w http.ResponseWriter, r *http.Request) {
	res, err := s.syncLDAP(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /api/auth/methods：登录页用，不需要登录。oidc 为 null 表示没开企业统一登录。
func (s *Server) handleAuthMethods(w http.ResponseWriter, r *http.Request) {
	var sso any
	if cfg, ok := s.oidcEnabled(r.Context()); ok {
		sso = map[string]string{"name": cfg.Name}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ldap": s.ldapEnabled(r.Context()), "oidc": sso, "demo": s.demoSiteInfo()})
}

// ---- 登录 ----

var (
	errBadCredentials = errors.New("用户名或密码错误")
	errNotProvisioned = errors.New("你的企业目录账号还没有开通 dsh-ee，请联系管理员")
	errDirectory      = errors.New("企业目录暂时无法验证，请稍后再试或联系管理员")
)

// authenticate 验证用户名和密码，网页登录与 npm login 共用。
//
// 本地账号始终用本地密码验证，即使开了 LDAP：管理员在目录故障时仍能登录；
// 目录里与本地账号同名的用户也不能借此冒用本地账号。其余用户在开启 LDAP 时交给目录验证，
// 首次登录按配置自动建号，每次登录同步资料与组映射的角色。
//
// 拒绝分支的等时化（避免靠响应时间枚举账号）：
//   - 可本地判定的（bot 账号、LDAP 未启用）先跑一次 dummyHash 的完整 bcrypt 再拒绝；
//   - 本地人类账号走 verifyLocalPassword，密码错误或被禁用都恰好一次 bcrypt；
//   - 目录返回凭据错误、目录同名本地账号冲突、目录用户被禁用都在一次真实目录往返之后，
//     耗时由网络与目录决定，本地无法也不需补齐。
func (s *Server) authenticate(ctx context.Context, username, password string) (*store.User, error) {
	name := strings.ToLower(strings.TrimSpace(username))
	u, err := s.store.GetUserByUsername(ctx, name)
	if err != nil && !notFound(err) {
		return nil, err
	}
	if u != nil && u.AccountType != store.AccountHuman {
		// bot 账号不能登录，但「账号存在」不能比「账号不存在」快：后者会跑一次 dummyHash
		// 比对，这里同样跑一次再拒绝（bot 账号按建号约定没有本地密码）。
		equalizePasswordCompare(password)
		return nil, errBadCredentials
	}
	if u != nil && u.AuthSource != store.AuthLDAP {
		// 没有本地密码的账号（OIDC / 设备码建号）也走一次完整 bcrypt 再拒绝，见 verifyLocalPassword。
		if !verifyLocalPassword(u, password) || u.Disabled {
			return nil, errBadCredentials
		}
		return u, nil
	}
	cfg, err := s.loadLDAP(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.URL == "" {
		// 和存在的用户比对一样耗时，免得靠响应时间判断用户名是否存在
		equalizePasswordCompare(password)
		return nil, errBadCredentials
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, err := cfg.Authenticate(ctx, strings.TrimSpace(username), password)
	if errors.Is(err, ldapauth.ErrInvalidCredentials) {
		return nil, errBadCredentials
	}
	if err != nil {
		s.log.Warn("LDAP 认证出错", "user", name, "err", err)
		return nil, errDirectory
	}
	canonical := strings.ToLower(strings.TrimSpace(e.Username))
	if canonical == "" {
		canonical = name
	}
	if canonical != name {
		u, err = s.store.GetUserByUsername(ctx, canonical)
		if err != nil && !notFound(err) {
			return nil, err
		}
		if u != nil && u.AuthSource != store.AuthLDAP {
			s.log.Warn("目录用户与本地账号重名，拒绝登录", "user", canonical)
			return nil, errBadCredentials
		}
	}
	if u == nil {
		if !cfg.AutoCreate {
			return nil, errNotProvisioned
		}
		if !usernameRe.MatchString(canonical) {
			s.log.Warn("目录用户名不符合 dsh-ee 规则，无法自动建号", "user", canonical)
			return nil, errNotProvisioned
		}
		return s.createLDAPUser(ctx, cfg, canonical, e)
	}
	if u.Disabled {
		return nil, errBadCredentials
	}
	if err := s.applyLDAPEntry(ctx, cfg, u, e); err != nil {
		return nil, err
	}
	return s.store.GetUser(ctx, u.ID)
}

func (s *Server) createLDAPUser(ctx context.Context, cfg *ldapauth.Config, username string, e *ldapauth.Entry) (*store.User, error) {
	zero := int64(0)
	u := &store.User{
		Username: username, DisplayName: e.DisplayName, Department: e.Department, Email: e.Email,
		AuthSource: store.AuthLDAP, ExternalDN: e.DN, MonthlyTokenLimit: &zero,
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			// 同一个人在两个标签页同时首次登录
			return s.store.GetUserByUsername(ctx, username)
		}
		return nil, err
	}
	roles, err := s.ldapRoles(ctx, cfg, nil, e.Groups)
	if err != nil {
		return nil, err
	}
	// 新账号默认是「员工」，除非「员工」角色本身由组映射决定
	if member, err := s.store.GetRoleByCode(ctx, rbac.RoleMember); err == nil && !slices.Contains(cfg.ManagedRoles(), member.ID) {
		roles = dedupe(append(roles, member.ID))
	}
	if err := s.store.SetUserRoles(ctx, u.ID, roles); err != nil {
		return nil, err
	}
	s.applyDirectoryFields(ctx, "ldap", u.ID, e.Attrs)
	s.log.Info("LDAP 用户首次登录，已创建账号", "user", username, "roles", roles)
	return s.store.GetUser(ctx, u.ID)
}

// applyLDAPEntry 把目录里的资料与组映射的角色同步到本地账号。资料为空的字段不覆盖。
func (s *Server) applyLDAPEntry(ctx context.Context, cfg *ldapauth.Config, u *store.User, e *ldapauth.Entry) error {
	var p store.UserPatch
	changed := false
	set := func(dst **string, cur, next string) {
		if next != "" && next != cur {
			*dst = &next
			changed = true
		}
	}
	set(&p.DisplayName, u.DisplayName, e.DisplayName)
	set(&p.Department, u.Department, e.Department)
	set(&p.Email, u.Email, e.Email)
	set(&p.ExternalDN, u.ExternalDN, e.DN)
	if changed {
		if err := s.store.UpdateUser(ctx, u.ID, p); err != nil {
			return err
		}
	}
	s.applyDirectoryFields(ctx, "ldap", u.ID, e.Attrs)
	if len(cfg.RoleMappings) == 0 {
		return nil
	}
	a, err := s.store.UserAccess(ctx, u.ID)
	if err != nil {
		return err
	}
	next, err := s.ldapRoles(ctx, cfg, a.RoleIDs, e.Groups)
	if err != nil {
		return err
	}
	cur := dedupe(append([]int64{}, a.RoleIDs...))
	slices.Sort(cur)
	slices.Sort(next)
	if slices.Equal(cur, next) {
		return nil
	}
	return s.store.SetUserRoles(ctx, u.ID, next)
}

// ldapRoles 计算目录同步后的角色：保留不由映射管理的角色，映射管理的按当前所属组重新决定。
// 映射里引用了已删除的角色时跳过。
func (s *Server) ldapRoles(ctx context.Context, cfg *ldapauth.Config, current []int64, groups []string) ([]int64, error) {
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	exists := map[int64]bool{}
	for _, r := range roles {
		exists[r.ID] = true
	}
	managed := cfg.ManagedRoles()
	out := []int64{}
	for _, id := range current {
		if !slices.Contains(managed, id) {
			out = append(out, id)
		}
	}
	for _, id := range cfg.MatchRoles(groups) {
		if exists[id] {
			out = append(out, id)
		}
	}
	return dedupe(out), nil
}

// ---- 目录同步 ----

type ldapSyncResult struct {
	At       time.Time `json:"at"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Checked  int       `json:"checked"`
	Updated  int       `json:"updated"`
	Disabled []string  `json:"disabled"`
	// 开了「同步禁用」以外的情况下，找不到的账号只报告不处理
	Missing []string `json:"missing"`
}

// syncLDAP 逐个核对 LDAP 账号：更新资料与角色；目录里已删除或在 AD 里被禁用的，按配置在本地禁用。
// 目录连不上、或超过一半的账号「消失」（多半是配置改错了）时整次放弃，不禁用任何人。
func (s *Server) syncLDAP(ctx context.Context) (*ldapSyncResult, error) {
	res := &ldapSyncResult{At: time.Now().UTC(), Disabled: []string{}, Missing: []string{}}
	finish := func(err error) (*ldapSyncResult, error) {
		if err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
		}
		if perr := s.store.PutSettingJSON(ctx, ldapSyncKey, res, 0); perr != nil {
			return nil, perr
		}
		return res, nil
	}
	cfg, err := s.loadLDAP(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.URL == "" {
		return finish(errors.New("没有开启 LDAP 登录"))
	}
	users, err := s.store.ListUsersBySource(ctx, store.AuthLDAP)
	if err != nil {
		return nil, err
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cn, err := cfg.Dial(ctx2)
	if err != nil {
		return finish(err)
	}
	defer cn.Close()

	type found struct {
		u *store.User
		e *ldapauth.Entry
	}
	var present []found
	var gone []*store.User
	for _, u := range users {
		e, err := cn.Lookup(u.Username)
		if errors.Is(err, ldapauth.ErrUserNotFound) || (err == nil && e.Disabled) {
			gone = append(gone, u)
			continue
		}
		if err != nil {
			return finish(fmt.Errorf("核对 %s 时出错，已停止：%w", u.Username, err))
		}
		present = append(present, found{u, e})
	}
	res.Checked = len(users)
	if len(gone) > 3 && len(gone)*2 > len(users) {
		return finish(fmt.Errorf("有 %d / %d 个账号在目录里找不到，可能是搜索起点或过滤条件改错了；为安全起见没有禁用任何账号", len(gone), len(users)))
	}
	for _, f := range present {
		before := *f.u
		if err := s.applyLDAPEntry(ctx, cfg, f.u, f.e); err != nil {
			return finish(err)
		}
		if f.e.DisplayName != before.DisplayName || f.e.Department != before.Department {
			res.Updated++
		}
	}
	for _, u := range gone {
		if !cfg.SyncDisable {
			res.Missing = append(res.Missing, u.Username)
			continue
		}
		yes := true
		if err := s.store.UpdateUser(ctx, u.ID, store.UserPatch{Disabled: &yes}); err != nil {
			return finish(err)
		}
		if err := s.store.DeleteUserSessions(ctx, u.ID); err != nil {
			return finish(err)
		}
		s.purgeUserCredentials(ctx, u.ID)
		res.Disabled = append(res.Disabled, u.Username)
		s.auditSystem(ctx, auditUserUpdate, u.Username, map[string]any{"disabled": true, "reason": "ldap_missing"})
	}
	if len(res.Disabled) > 0 {
		s.log.Info("LDAP 同步：已禁用目录里不存在或已禁用的账号", "users", res.Disabled)
	}
	s.auditSystem(ctx, auditLDAPSync, "", map[string]any{"checked": res.Checked, "updated": res.Updated, "disabled": len(res.Disabled), "missing": len(res.Missing)})
	return finish(nil)
}

// runLDAPSync 每小时核对一次目录（开启了 LDAP 与同步禁用时）。
func (s *Server) runLDAPSync(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runLeased(ctx, leaseLDAPSync, func(ctx context.Context) {
				cfg, err := s.loadLDAP(ctx)
				if err != nil || !cfg.Enabled || !cfg.SyncDisable {
					return
				}
				if res, err := s.syncLDAP(ctx); err != nil {
					s.log.Warn("LDAP 同步失败", "err", err)
				} else if !res.OK {
					s.log.Warn("LDAP 同步未完成", "err", res.Error)
				}
			})
		}
	}
}
