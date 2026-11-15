package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 令牌自省（docs/token-lifecycle.md）：第三方 Harness / SDK 在发请求前确认令牌还有效、
// 有哪些 scope、属于谁；管理页也能按令牌查一次。只返回元数据，永远不返回令牌哈希或明文。

type introspectUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Department  string `json:"department"`
	AccountType string `json:"account_type"`
}

type introspectResponse struct {
	// 令牌现在能不能用；不能用时其余字段尽量给出可解释的原因（管理员查询时）
	Active bool   `json:"active"`
	Reason string `json:"reason,omitempty"` // revoked / expired / user_disabled / not_found / inactive
	// access（ee_at_）/ npm（旧 ee_npm_）/ key（旧 sk-ee-）
	Kind string `json:"kind,omitempty"`
	// at:<id> 或 key:<id>；旧版 npm 令牌没有可展示的 id
	TokenID    string          `json:"token_id,omitempty"`
	Legacy     bool            `json:"legacy,omitempty"`
	ClientKind string          `json:"client_kind,omitempty"` // dsh / third_party（旧令牌按 dsh 兼容）
	ClientName string          `json:"client_name,omitempty"`
	Prefix     string          `json:"prefix,omitempty"`
	Scopes     []string        `json:"scopes,omitempty"`
	ExpiresAt  *time.Time      `json:"expires_at"`
	User       *introspectUser `json:"user,omitempty"`
}

func introspectUserOf(u *store.User) *introspectUser {
	if u == nil {
		return nil
	}
	return &introspectUser{ID: u.ID, Username: u.Username, Department: u.Department, AccountType: u.AccountType}
}

func sortedScopes(set ScopeSet) []string {
	out := make([]string, 0, len(set))
	for scope := range set {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func sortedScopeList(scopes []string) []string {
	out := append([]string(nil), scopes...)
	sort.Strings(out)
	return out
}

// introspectPrincipal 是「用这个令牌自己查自己」时的结果，直接用 resolveBearer 已经解析好的主体。
func introspectPrincipal(p *Principal) *introspectResponse {
	resp := &introspectResponse{
		Active: true, Kind: "access", Legacy: p.Legacy != "", ClientKind: p.ClientKind,
		ClientName: p.Client, Prefix: p.Prefix, Scopes: sortedScopes(p.Scopes), ExpiresAt: p.ExpiresAt,
		User: introspectUserOf(p.User),
	}
	switch {
	case p.TokenID > 0:
		resp.Kind = "access"
		resp.TokenID = "at:" + strconv.FormatInt(p.TokenID, 10)
	case p.Legacy == "npm":
		resp.Kind = "npm"
	case p.Legacy == "key":
		resp.Kind = "key"
		if p.LegacyID > 0 {
			resp.TokenID = "key:" + strconv.FormatInt(p.LegacyID, 10)
		}
	}
	return resp
}

// introspectRawToken 按明文令牌查状态，不更新 last_used / 滑动续期：自省不是「使用」。
func (s *Server) introspectRawToken(ctx context.Context, raw string) (*introspectResponse, error) {
	raw = strings.TrimSpace(raw)
	hash := secret.Hash(raw)
	now := time.Now()
	switch {
	case strings.HasPrefix(raw, accessTokenPrefix):
		t, err := s.store.ActiveAccessToken(ctx, hash)
		active := err == nil
		if !active {
			if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
			// 不是「能用」的令牌：仍然按哈希找到记录，给出吊销/过期/停用之类的解释
			inactive, lookupErr := s.store.AccessTokenByHash(ctx, hash)
			if errors.Is(lookupErr, store.ErrNotFound) {
				return &introspectResponse{Active: false, Reason: "not_found", Kind: "access"}, nil
			}
			if lookupErr != nil {
				return nil, lookupErr
			}
			t = inactive
		}
		u, uerr := s.store.GetUser(ctx, t.UserID)
		if uerr != nil && !errors.Is(uerr, store.ErrNotFound) {
			return nil, uerr
		}
		var account *rbac.Access
		if u != nil {
			if account, err = s.store.UserAccess(ctx, u.ID); err != nil {
				return nil, err
			}
		}
		_, effective, _ := effectiveAccessTokenScopes(account, t.Scopes, t.AdminUntil, now)
		resp := &introspectResponse{
			Active: active, Kind: "access", TokenID: "at:" + strconv.FormatInt(t.ID, 10),
			ClientKind: store.NormalizeClientKind(t.ClientKind), ClientName: t.ClientName, Prefix: t.Prefix,
			Scopes: sortedScopeList(effective), ExpiresAt: t.ExpiresAt, User: introspectUserOf(u),
		}
		if !active {
			switch {
			case t.RevokedAt != nil:
				resp.Reason = "revoked"
			case t.ExpiresAt != nil && !t.ExpiresAt.After(now):
				resp.Reason = "expired"
			case u != nil && u.Disabled:
				resp.Reason = "user_disabled"
			default:
				resp.Reason = "inactive"
			}
		}
		return resp, nil
	case strings.HasPrefix(raw, npmTokenPrefix):
		t, u, err := s.store.ActiveNpmToken(ctx, hash)
		if errors.Is(err, store.ErrNotFound) {
			return &introspectResponse{Active: false, Reason: "not_found", Kind: "npm", Legacy: true, ClientKind: store.ClientKindDSH}, nil
		}
		if err != nil {
			return nil, err
		}
		return &introspectResponse{
			Active: true, Kind: "npm", Legacy: true, ClientKind: store.ClientKindDSH, Prefix: t.Prefix,
			Scopes: sortedScopeList(legacyNpmScopes(t.Access, s.opts.RestrictLegacyTokenScopes)),
			User:   introspectUserOf(u),
		}, nil
	case strings.HasPrefix(raw, keyPrefix):
		k, _, err := s.store.ActiveKeyByHash(ctx, hash)
		if errors.Is(err, store.ErrNotFound) {
			return &introspectResponse{Active: false, Reason: "not_found", Kind: "key", Legacy: true, ClientKind: store.ClientKindDSH}, nil
		}
		if err != nil {
			return nil, err
		}
		u, err := s.store.GetUser(ctx, k.UserID)
		if errors.Is(err, store.ErrNotFound) {
			return &introspectResponse{Active: false, Reason: "not_found", Kind: "key", Legacy: true, ClientKind: store.ClientKindDSH}, nil
		}
		if err != nil {
			return nil, err
		}
		return &introspectResponse{
			Active: true, Kind: "key", Legacy: true, TokenID: "key:" + strconv.FormatInt(k.ID, 10),
			ClientKind: store.ClientKindDSH, Prefix: k.Prefix, Scopes: sortedScopeList(legacyKeyScopes(s.opts.RestrictLegacyTokenScopes)),
			ExpiresAt: k.ExpiresAt, User: introspectUserOf(u),
		}, nil
	default:
		return &introspectResponse{Active: false, Reason: "not_found"}, nil
	}
}

type introspectRequest struct {
	// 要查的令牌；不填时用自己的 Bearer 令牌（self-introspection）
	Token string `json:"token"`
}

// POST /api/auth/introspect：Bearer 自身或管理员查询令牌状态。
//
// self：Authorization: Bearer <要查的令牌>，body 留空。
// admin：用网页会话或带 keys.manage 的令牌调，body 里传 {"token":"ee_at_…"}；
// 返回 active:false 时也带原因（吊销/过期/账号停用），方便客户端决定重新登录。
func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	var req introspectRequest
	// body 可省略：self-introspection 时直接看 Authorization
	if r.ContentLength != 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}
	bearer, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	bearer = strings.TrimSpace(bearer)

	var caller *Principal
	var sessionAccess *rbac.Access
	if hasBearer && bearer != "" {
		p, err := s.resolveBearer(r.Context(), bearer, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
		if err != nil {
			var be bearerError
			if errors.As(err, &be) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "令牌无效、已吊销或已过期", "code": string(be)})
			} else {
				s.internalErr(w, r, err)
			}
			return
		}
		caller = p
	} else {
		// 没有 Bearer 时只接受网页会话；管理员用它查任意令牌
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "请先登录，或用 Bearer 令牌调这个接口")
			return
		}
		u, limited, err := s.store.SessionInfo(r.Context(), secret.Hash(c.Value))
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if sessionAccess, err = s.store.UserAccess(r.Context(), u.ID); err != nil {
			s.internalErr(w, r, err)
			return
		}
		// 从 DSH 打开的网页会话只带员工权限（与 handleMe / npmAuth 同口径）：
		// 账号本身有 keys.manage（含超管）也不能借它查别人的令牌，否则会泄露属主与令牌元数据。
		if limited {
			sessionAccess = limitedAccess(sessionAccess)
		}
	}

	target := strings.TrimSpace(req.Token)
	if target == "" {
		if caller == nil {
			writeErr(w, http.StatusBadRequest, "要查询的令牌必填：自己的令牌用 Bearer 查自己；管理员在 body 里传 token")
			return
		}
		writeJSON(w, http.StatusOK, introspectPrincipal(caller))
		return
	}
	// 同一枚令牌同时出现在 Bearer 和 body 里：按 self 处理
	if caller != nil && target == bearer {
		writeJSON(w, http.StatusOK, introspectPrincipal(caller))
		return
	}
	if caller == nil && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		// 有 Authorization 头但令牌为空（如 "Bearer "）：按未认证处理
		writeErr(w, http.StatusUnauthorized, "请先登录，或用 Bearer 令牌调这个接口")
		return
	}
	// 查别的令牌要求 keys.manage（管理员）；普通令牌只能查自己
	admin := false
	if caller != nil {
		admin = caller.Access.Has(rbac.KeysManage)
	} else if sessionAccess != nil {
		admin = sessionAccess.Has(rbac.KeysManage)
	}
	if !admin {
		writeErr(w, http.StatusForbidden, "只有管理员能查询别的令牌；自己的令牌直接用 Bearer 调这个接口")
		return
	}
	resp, err := s.introspectRawToken(r.Context(), target)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
