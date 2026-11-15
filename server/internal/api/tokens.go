package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const accessTokenPrefix = "ee_at_"

// maxTokenExpiryDays 是手工 / 管理签发令牌的有效期上限，0 表示长期有效；
// 错误文案里的 3650 与它一致。
const maxTokenExpiryDays = 3650

type tokenView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	UserID      int64  `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Department  string `json:"department,omitempty"`
	AccountType string `json:"account_type,omitempty"`
	Name        string `json:"name"`
	Prefix      string `json:"prefix"`
	Source      string `json:"source"`
	// 令牌签发给哪类客户端：dsh / third_party
	ClientKind        string     `json:"client_kind"`
	Scopes            []string   `json:"scopes"`
	ClientName        string     `json:"client_name"`
	ClientOS          string     `json:"client_os"`
	MonthlyTokenLimit *int64     `json:"monthly_token_limit"`
	MonthUsed         int64      `json:"month_used"`
	AllowedModels     []string   `json:"allowed_models"`
	ExpiresAt         *time.Time `json:"expires_at"`
	LastUsedAt        *time.Time `json:"last_used_at"`
	LastIP            string     `json:"last_ip"`
	RevokedAt         *time.Time `json:"revoked_at"`
	CreatedAt         time.Time  `json:"created_at"`
	// 管理模式的到期时间；没开启或已到期时为空
	AdminUntil *time.Time `json:"admin_until"`
}

func accessView(t *store.AccessToken, used int64) tokenView {
	var adminUntil *time.Time
	if t.AdminUntil != nil && t.AdminUntil.After(time.Now()) {
		adminUntil = t.AdminUntil
	}
	return tokenView{AdminUntil: adminUntil,
		ID: "at:" + strconv.FormatInt(t.ID, 10), Kind: "access", Name: t.Name, Prefix: t.Prefix,
		UserID: t.UserID, Username: t.Username, Department: t.Department, AccountType: t.AccountType,
		Source: t.Source, ClientKind: store.NormalizeClientKind(t.ClientKind), Scopes: nonNil(t.Scopes), ClientName: t.ClientName, ClientOS: t.ClientOS,
		MonthlyTokenLimit: t.MonthlyTokenLimit, MonthUsed: used, AllowedModels: nonNil(t.AllowedModels),
		ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, LastIP: t.LastIP, RevokedAt: t.RevokedAt, CreatedAt: t.CreatedAt,
	}
}

func (s *Server) tokenViews(ctx context.Context, userID int64) ([]tokenView, error) {
	tokens, err := s.store.ListUserTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []tokenView{}
	for _, t := range tokens {
		used, err := s.store.TokenMonthUsage(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, accessView(t, used))
	}
	npm, err := s.store.ListNpmTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, t := range npm {
		scopes := []string{rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeHub, rbac.ScopeConnections}
		if t.Access == "publish" {
			scopes = append(scopes, rbac.ScopeRegistryPublish)
		}
		out = append(out, tokenView{ID: "npm:" + strconv.FormatInt(t.ID, 10), Kind: "npm", Name: t.Name, Prefix: t.Prefix,
			Source: "legacy", ClientKind: store.ClientKindDSH, Scopes: scopes, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt, CreatedAt: t.CreatedAt})
	}
	keys, err := s.store.ListAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		out = append(out, tokenView{ID: "key:" + strconv.FormatInt(k.ID, 10), Kind: "key", Name: k.Name, Prefix: k.Prefix,
			Source: "legacy", ClientKind: store.ClientKindDSH, Scopes: []string{rbac.ScopeModels, rbac.ScopeHub, rbac.ScopeConnections}, MonthlyTokenLimit: k.MonthlyTokenLimit,
			MonthUsed: k.MonthTokens, AllowedModels: nonNil(k.AllowedModels), ExpiresAt: k.ExpiresAt, LastUsedAt: k.LastUsedAt,
			RevokedAt: k.RevokedAt, CreatedAt: k.CreatedAt})
	}
	return out, nil
}

func (s *Server) handleMyTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.tokenViews(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNil(tokens)})
}

type tokenInput struct {
	Name              string   `json:"name"`
	Scopes            []string `json:"scopes"`
	MonthlyTokenLimit *int64   `json:"monthly_token_limit"`
	AllowedModels     []string `json:"allowed_models"`
	ExpiresInDays     int      `json:"expires_in_days"`
	UserID            int64    `json:"user_id"`
	// 令牌签发给哪类客户端；不填按签发入口的默认值：设备码是 dsh，手工/管理员签发是 third_party
	ClientKind string `json:"client_kind"`
	// 可选的客户端名字（复用令牌的设备名/client_name 列），第三方 Harness 可以填自己的名字
	ClientName string `json:"client_name"`
}

// scopeAllowed 判断这个 scope 能不能授给拥有权限 a 的账号。
// 手工令牌、API 客户端（client_credentials）共用同一套映射，避免两条签发路径的权限口径漂移。
func scopeAllowed(a *rbac.Access, scope string) bool {
	switch scope {
	case rbac.ScopeModels:
		return a.Has(rbac.LLMUse)
	case rbac.ScopeRegistryRead:
		return a.Has(rbac.RegistryUse)
	case rbac.ScopeRegistryPublish:
		return a.Has(rbac.RegistryPublish)
	case rbac.ScopeHub, rbac.ScopeConnections:
		return a.Has(rbac.PortalView)
	case rbac.ScopeAgent:
		return a.Has(rbac.PortalView)
	case rbac.ScopeConfig:
		return a.Has(rbac.FieldsManage, rbac.RolesManage)
	default:
		// admin 不能通过签发接口下发，未知 scope 也不认
		return false
	}
}

func validTokenScopes(a *rbac.Access, scopes []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, scope := range scopes {
		if seen[scope] || !scopeAllowed(a, scope) {
			return nil, false
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out, true
}

// allowedScopes 返回 scopes 里当前账号还能用的那部分（保序、去重）。
// client_credentials 用它把注册时申请的 scope 收敛到 owner 当前权限：管理员收回权限后立即生效。
func allowedScopes(a *rbac.Access, scopes []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, scope := range scopes {
		if seen[scope] || !scopeAllowed(a, scope) {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

// issueClientKind 决定新令牌的客户端类型。
// 默认值按签发入口区分：设备码是 DSH 的登录方式（dsh，见 store.ApproveDevice）；
// 网页手工创建与管理员签发主要给第三方 Harness / 机器人用（third_party）。
// 允许显式指定，例如给不用设备码登录的 DSH 手工建一个 dsh 令牌；不认别的值。
func issueClientKind(source, requested string) (string, bool) {
	if requested != "" {
		switch requested {
		case store.ClientKindDSH, store.ClientKindThirdParty:
			return requested, true
		default:
			return "", false
		}
	}
	if source == "device" {
		return store.ClientKindDSH, true
	}
	return store.ClientKindThirdParty, true
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, owner *store.User, source string, req tokenInput) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidName, nil, "令牌名称必填，且不能超过 128 个字符")
		return
	}
	clientKind, ok := issueClientKind(source, strings.TrimSpace(req.ClientKind))
	if !ok {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidClientKind, nil, "client_kind 只能是 dsh 或 third_party")
		return
	}
	if req.MonthlyTokenLimit != nil && *req.MonthlyTokenLimit < 0 {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidLimit, nil, "monthly_token_limit 不能为负数")
		return
	}
	if req.ExpiresInDays < 0 || req.ExpiresInDays > maxTokenExpiryDays {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidExpiry,
			map[string]any{"max": maxTokenExpiryDays}, "expires_in_days 应在 0–3650 之间，0 表示长期有效")
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeHub, rbac.ScopeConnections}
	}
	scopeOwner := currentAccess(r)
	if source == "issued" {
		var err error
		scopeOwner, err = s.store.UserAccess(r.Context(), owner.ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	scopes, ok := validTokenScopes(scopeOwner, req.Scopes)
	if !ok {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidScope, nil, "scope 超出账号权限，或包含暂不支持的 admin")
		return
	}
	plain := accessTokenPrefix + secret.Token()
	clientName := cleanClientField(req.ClientName, 128)
	t := &store.AccessToken{
		UserID: owner.ID, Name: req.Name, Prefix: plain[:12], Source: source, ClientKind: clientKind, Scopes: scopes,
		MonthlyTokenLimit: req.MonthlyTokenLimit, AllowedModels: req.AllowedModels, ClientName: clientName, CreatedBy: currentUser(r).ID,
	}
	if req.ExpiresInDays > 0 {
		expires := time.Now().UTC().AddDate(0, 0, req.ExpiresInDays).Truncate(time.Millisecond)
		t.ExpiresAt = &expires
	}
	var refreshID int64
	var refreshPlain string
	var refreshExpiresIn int64
	if s.opts.RefreshTokens {
		// 开启刷新令牌后，设备登录/手工签发/管理员签发都会多给一枚 refresh token；
		// 与 access token 在同一个事务里落库，避免只签出其中一半。
		rt, generated := newRefreshSeed(owner.ID, clientKind, s.clientIP(r), s.opts.RefreshFamilyTTL, s.opts.RefreshTokenTTL)
		if err := s.store.CreateAccessTokenWithRefresh(r.Context(), t, secret.Hash(plain), rt); err != nil {
			s.internalErr(w, r, err)
			return
		}
		refreshID, refreshPlain, refreshExpiresIn = rt.ID, generated, secondsUntil(rt.ExpiresAt)
	} else if err := s.store.CreateAccessToken(r.Context(), t, secret.Hash(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	// accessView 要用落库后的 t（必须已经拿到 id）
	resp := map[string]any{"token": plain, "info": accessView(t, 0)}
	if refreshPlain != "" {
		resp["refresh_token"] = refreshPlain
		resp["refresh_expires_in"] = refreshExpiresIn
	}
	action := "token.create"
	if source == "issued" {
		action = "token.issue"
	}
	detail := map[string]any{"id": t.ID, "user_id": owner.ID, "scopes": scopes, "client_kind": clientKind, "client_name": t.ClientName}
	if refreshID > 0 {
		detail["refresh"] = true
	}
	s.audit(r, action, plain[:12], detail)
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleCreateMyToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireFullSession(w, r) {
		return
	}
	settings, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !settings.Connect.AllowManualTokens {
		writeErrCode(w, http.StatusForbidden, codeTokenManualDisabled, nil, "管理员已关闭员工手动生成令牌")
		return
	}
	var req tokenInput
	if !readJSON(w, r, &req) {
		return
	}
	s.createToken(w, r, currentUser(r), "manual", req)
}

func (s *Server) handleRevokeMyToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	kind, raw, ok := strings.Cut(id, ":")
	if !ok {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidID, nil, "令牌 id 格式不正确")
		return
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidID, nil, "令牌 id 格式不正确")
		return
	}
	ctx := r.Context()
	switch kind {
	case "at":
		t, err := s.store.ListUserTokens(ctx, currentUser(r).ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		found := false
		for _, token := range t {
			if token.ID == n {
				found = true
			}
		}
		if !found {
			writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
			return
		}
		if err := s.store.RevokeToken(ctx, n); err != nil {
			s.internalErr(w, r, err)
			return
		}
	case "npm":
		// 只能吊销自己的：不是自己的一律按不存在处理，不暴露别人的令牌 id
		own, err := s.store.ListNpmTokens(ctx, currentUser(r).ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if !slices.ContainsFunc(own, func(token *store.NpmToken) bool { return token.ID == n }) {
			writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
			return
		}
		if err := s.store.RevokeNpmToken(ctx, n); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
			} else {
				s.internalErr(w, r, err)
			}
			return
		}
	case "key":
		if k, err := s.store.GetAPIKey(ctx, n); notFound(err) || (err == nil && k.UserID != currentUser(r).ID) {
			writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
			return
		} else if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if err := s.store.RevokeAPIKey(ctx, n); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
			} else {
				s.internalErr(w, r, err)
			}
			return
		}
	default:
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidKind, nil, "令牌类型不正确")
		return
	}
	s.audit(r, "token.revoke", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminTokenScope(r *http.Request) (string, bool) {
	if currentAccess(r).Has(rbac.KeysManage) {
		return "", true
	}
	if currentAccess(r).Has(rbac.TokensDeptManage) && currentUser(r).Department != "" {
		return currentUser(r).Department, true
	}
	return "", false
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request) {
	department, ok := s.adminTokenScope(r)
	if !ok {
		writeErrCode(w, http.StatusForbidden, codeTokenManageForbidden, nil, "没有令牌管理权限")
		return
	}
	q := r.URL.Query()
	f := store.TokenFilter{}
	if v := q.Get("user_id"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		f.UserID = &id
	}
	if v := q.Get("department"); v != "" {
		f.Department = &v
	}
	if v := q.Get("source"); v != "" {
		f.Source = &v
	}
	if v := q.Get("client_name"); v != "" {
		f.ClientName = &v
	}
	if v := q.Get("status"); v == "revoked" {
		revoked := true
		f.Revoked = &revoked
	} else if v == "active" {
		revoked := false
		f.Revoked = &revoked
	}
	tokens, err := s.store.ListTokens(r.Context(), f)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []tokenView{}
	for _, t := range tokens {
		if department != "" && (t.Department != department || t.AccountType != store.AccountBot) {
			continue
		}
		used, err := s.store.TokenMonthUsage(r.Context(), t.ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		out = append(out, accessView(t, used))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNil(out)})
}

// handleIssueToken 给员工 / 机器人签发 ee_at_ 令牌。
// 调用方式有两种（路由是 authUserToken）：
//   - 网页会话 + keys.manage / tokens.dept_manage：管理员在「令牌管理」页操作；
//   - Bearer 令牌（需 keys.manage，即开过管理模式的令牌）：给 CI / 机器人等无人值守场景续发。
//
// 两种方式走同一套校验与审计；Bearer 调用的审计由 audit() 自动记 via=token、token_id=调用方令牌。
func (s *Server) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	department, ok := s.adminTokenScope(r)
	if !ok {
		writeErrCode(w, http.StatusForbidden, codeTokenManageForbidden, nil, "没有令牌管理权限")
		return
	}
	var req tokenInput
	if !readJSON(w, r, &req) {
		return
	}
	owner, err := s.store.GetUser(r.Context(), req.UserID)
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, codeTokenUserNotFound, nil, "账号不存在")
		return
	}
	if department != "" && (owner.Department != department || owner.AccountType != store.AccountBot) {
		writeErrCode(w, http.StatusForbidden, codeTokenDepartmentScopeForbidden, nil, "只能管理本部门机器人账号的令牌")
		return
	}
	s.createToken(w, r, owner, "issued", req)
}

func (s *Server) handleRevokeTokens(w http.ResponseWriter, r *http.Request) {
	// department 非空表示调用者只有「本部门令牌管理」：只能动本部门机器人账号的令牌
	department, ok := s.adminTokenScope(r)
	if !ok {
		writeErrCode(w, http.StatusForbidden, codeTokenManageForbidden, nil, "没有令牌管理权限")
		return
	}
	var req struct {
		IDs        []string `json:"ids"`
		UserID     *int64   `json:"user_id"`
		ClientName *string  `json:"client_name"`
		Department *string  `json:"department"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	const deptOnly = "只能吊销本部门机器人账号的令牌"
	if len(req.IDs) > 0 {
		ids := make([]int64, 0, len(req.IDs))
		for _, raw := range req.IDs {
			kind, v, ok := strings.Cut(raw, ":")
			id, err := strconv.ParseInt(v, 10, 64)
			if !ok || kind != "at" || err != nil || id <= 0 {
				writeErrCode(w, http.StatusBadRequest, codeTokenBulkIDInvalid, nil, "批量吊销目前只支持 at:<id> 形式")
				return
			}
			t, err := s.store.AccessTokenByID(r.Context(), id)
			if notFound(err) {
				writeErrCode(w, http.StatusBadRequest, codeTokenUnknown, map[string]any{"id": raw}, "令牌不存在："+raw)
				return
			}
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			// 先全部校验再动手，避免越权请求吊销了一部分
			if department != "" && (t.Department != department || t.AccountType != store.AccountBot) {
				writeErrCode(w, http.StatusForbidden, codeTokenDepartmentRevokeForbidden, nil, deptOnly)
				return
			}
			ids = append(ids, id)
		}
		count := 0
		for _, id := range ids {
			err := s.store.RevokeToken(r.Context(), id)
			if notFound(err) {
				continue // 已经吊销过
			}
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			count++
		}
		s.audit(r, "token.revoke_bulk", "ids", map[string]any{"count": count, "ids": req.IDs})
		writeJSON(w, http.StatusOK, map[string]any{"revoked": count})
		return
	}
	f := store.TokenRevokeFilter{UserID: req.UserID, ClientName: req.ClientName, Department: req.Department}
	if f.ClientName != nil && *f.ClientName == "" && f.UserID == nil && f.Department == nil {
		f.ClientName = nil // 只给空设备名等于没有条件
	}
	if f.Empty() {
		writeErrCode(w, http.StatusBadRequest, codeTokenRevokeFilterRequired, nil, "请指定要吊销的令牌、账号、设备或部门")
		return
	}
	if department != "" {
		if req.Department != nil && *req.Department != department {
			writeErrCode(w, http.StatusForbidden, codeTokenDepartmentRevokeForbidden, nil, deptOnly)
			return
		}
		if req.UserID != nil {
			u, err := s.store.GetUser(r.Context(), *req.UserID)
			if err != nil || u.Department != department || u.AccountType != store.AccountBot {
				writeErrCode(w, http.StatusForbidden, codeTokenDepartmentRevokeForbidden, nil, deptOnly)
				return
			}
		}
		// 无论请求怎么写，都限定在本部门的机器人账号
		f.Department = &department
		f.BotOnly = true
	}
	n, err := s.store.RevokeTokensBy(r.Context(), f)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, "token.revoke_bulk", "filter", map[string]any{"count": n, "user_id": req.UserID, "client_name": req.ClientName, "department": f.Department})
	writeJSON(w, http.StatusOK, map[string]any{"revoked": n})
}
