package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 管理模式：有管理权限的人在网页上输入密码，给自己的某个令牌开启 1 或 8 小时的管理权限。
// 令牌平时只有员工权限；Agent 自己无法开启，也无法替人输入密码。

const (
	auditElevate       = "token.elevate"
	auditElevateEnd    = "token.elevate_end"
	auditElevateExpire = "token.elevate_expire"
)

// ownAccessToken 按 "at:12" 或 "12" 取当前用户自己的、仍有效的令牌；"current" 指发起这次调用的令牌（只有 Agent 调用才有）。
func (s *Server) ownAccessToken(w http.ResponseWriter, r *http.Request) (*store.AccessToken, bool) {
	raw := strings.TrimPrefix(r.PathValue("id"), "at:")
	var id int64
	if raw == "current" {
		if info, ok := agentFrom(r.Context()); ok {
			id = info.Principal.TokenID
		}
	} else {
		id, _ = strconv.ParseInt(raw, 10, 64)
	}
	if id <= 0 {
		writeErrCode(w, http.StatusBadRequest, codeTokenInvalidID, nil, "令牌 id 格式不正确")
		return nil, false
	}
	tokens, err := s.store.ListUserTokens(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	for _, t := range tokens {
		if t.ID == id && t.RevokedAt == nil {
			return t, true
		}
	}
	// 不是自己的一律按不存在处理
	writeErrCode(w, http.StatusNotFound, codeTokenNotFound, nil, "令牌不存在")
	return nil, false
}

// POST /api/me/tokens/{id}/elevate：{password, hours: 1|8}。只能由网页会话发起。
func (s *Server) handleElevate(w http.ResponseWriter, r *http.Request) {
	if _, ok := agentFrom(r.Context()); ok {
		writeErrCode(w, http.StatusForbidden, codeAdminModeWebOnly, nil, "管理模式只能由本人在网页上输入密码开启")
		return
	}
	var req struct {
		Password string `json:"password"`
		Hours    int    `json:"hours"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Hours != 1 && req.Hours != 8 {
		writeErrCode(w, http.StatusBadRequest, codeAdminModeInvalidDuration, nil, "管理模式只能开 1 小时或 8 小时")
		return
	}
	me := currentUser(r)
	if !rbac.HasAdminPerms(currentAccess(r)) {
		writeErrCode(w, http.StatusForbidden, codeAdminModePermissionRequired, nil, "你的账号没有管理权限，不能开启管理模式")
		return
	}
	t, ok := s.ownAccessToken(w, r)
	if !ok {
		return
	}
	// 密码再验一次，与登录共用失败限流（LDAP 账号走一次目录验证）
	key := "elevate:" + me.Username
	if s.limiter.locked(key) {
		writeErrCode(w, http.StatusTooManyRequests, codeAdminModeTooManyAttempts,
			map[string]any{"minutes": int(lockFor / time.Minute)}, "失败次数过多，请 15 分钟后再试")
		return
	}
	if _, err := s.authenticate(r.Context(), me.Username, req.Password); err != nil {
		if err == errBadCredentials {
			s.limiter.fail(key)
			s.auditAs(r, me, auditLoginFailed, me.Username, map[string]any{"via": "elevate"})
			// 不能用 401：网页把任何 401 都当成会话过期而退出登录
			writeErrCode(w, http.StatusForbidden, codeAdminModeBadPassword, nil, "密码不对")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.limiter.reset(key)
	unit := s.opts.AdminModeUnit
	if unit <= 0 {
		unit = time.Hour
	}
	until := time.Now().Add(time.Duration(req.Hours) * unit)
	if err := s.store.ElevateToken(r.Context(), t.ID, until); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditElevate, "at:"+strconv.FormatInt(t.ID, 10), map[string]any{"hours": req.Hours, "until": until.UTC(), "client": t.ClientName})
	s.hub.notify()
	s.kickElevation()
	writeJSON(w, http.StatusOK, map[string]any{"admin_until": until.UTC()})
}

// DELETE /api/me/tokens/{id}/elevate：提前结束管理模式。网页和 Agent 都可以（降权永远安全）；id 可以写 current。
func (s *Server) handleEndElevate(w http.ResponseWriter, r *http.Request) {
	t, ok := s.ownAccessToken(w, r)
	if !ok {
		return
	}
	if err := s.store.EndElevation(r.Context(), t.ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditElevateEnd, "at:"+strconv.FormatInt(t.ID, 10), map[string]any{"client": t.ClientName})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/me/admin-link：Agent 让管理员去网页开启管理模式时给的链接。
func (s *Server) handleAdminLink(w http.ResponseWriter, r *http.Request) {
	// 令牌的权限是缩小过的，要看账号本身有没有管理权限
	can := rbac.HasAdminPerms(currentAccess(r))
	if info, ok := agentFrom(r.Context()); ok {
		can = info.Principal.CanAdmin
	}
	if !can {
		writeErrCode(w, http.StatusForbidden, codeAdminModeForbidden, nil, "你的账号没有管理权限")
		return
	}
	base, err := s.connectBase(r.Context(), r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	info, _ := agentFrom(r.Context())
	link := base + "/tokens"
	resp := map[string]any{}
	if info != nil && info.Principal.TokenID > 0 {
		link += "?elevate=at:" + strconv.FormatInt(info.Principal.TokenID, 10)
		resp["admin_live"] = info.Principal.AdminLive
		resp["admin_until"] = info.Principal.AdminUntil
	}
	resp["url"] = link
	resp["note"] = "在浏览器打开链接，输入登录密码并选择 1 或 8 小时即可开启；Agent 无法替你开启。开启后几秒内 DSH 里会出现管理工具。"
	writeJSON(w, http.StatusOK, resp)
}

// kickElevation 通知到期监控重新计算下一次到期时间。
func (s *Server) kickElevation() {
	select {
	case s.elevationKick <- struct{}{}:
	default:
	}
}

// runElevationExpiry 在管理模式到期的那一刻写审计并通知 DSH 刷新工具（令牌的权限本身不依赖它，到期即失效）。
func (s *Server) runElevationExpiry(ctx context.Context) {
	for {
		wait := time.Minute
		if next, err := s.store.NextElevationExpiry(ctx); err != nil {
			s.log.Warn("读取管理模式到期时间失败", "err", err)
		} else if next != nil {
			wait = min(max(time.Until(*next), 0)+50*time.Millisecond, time.Hour)
		} else {
			wait = time.Hour
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.elevationKick:
			timer.Stop()
			continue
		case <-timer.C:
		}
		ran := s.runLeased(ctx, leaseElevationExpiry, func(ctx context.Context) {
			expired, err := s.store.ExpireElevations(ctx)
			if err != nil {
				s.log.Warn("清理到期的管理模式失败", "err", err)
				return
			}
			for _, t := range expired {
				s.auditSystem(ctx, auditElevateExpire, "at:"+strconv.FormatInt(t.ID, 10), map[string]any{"user_id": t.UserID, "client": t.ClientName})
			}
			// 本实例上的 DSH 长连接要刷新工具列表；hub 是进程内的，多实例时别的实例的
			// 连接由它们自己在同一个到期时刻通知（见 !ran 分支）。
			if len(expired) > 0 {
				s.hub.notify()
			}
		})
		if !ran && ctx.Err() == nil {
			// 写库/审计由 leader 负责；本实例只通知自己的 DSH 连接刷新工具列表。
			// 下一轮会重新读到期时间：leader 清掉后这里不再触发，最多多刷几次。
			s.hub.notify()
		}
	}
}
