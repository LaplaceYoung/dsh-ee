package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// deviceTokenPayload 是批准设备时密封在 device_codes.token_sealed 里的内容。
// 开启刷新令牌（DSH_EE_REFRESH_TOKENS）后会额外带第一枚 refresh token；
// 关闭时仍然按旧格式只密封 access token 明文，新旧格式靠首字符区分（'{' 表示 JSON）。
type deviceTokenPayload struct {
	AccessToken      string     `json:"access_token"`
	RefreshToken     string     `json:"refresh_token,omitempty"`
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
}

type rateWindow struct {
	start time.Time
	count int
}

// ipRate 是按 IP 的固定窗口限流（每分钟 limit 次）。
type ipRate struct {
	mu    sync.Mutex
	limit int
	items map[string]*rateWindow
}

func newIPRate(limit int) *ipRate { return &ipRate{limit: limit, items: map[string]*rateWindow{}} }

// 发起登录与轮询分开限流。轮询的单设备速度由 slow_down 约束，这里只防滥用：
// 公司内网常常多人共用一个出口 IP，额度要够几十台设备同时按 5 秒间隔轮询。
const (
	deviceCodePerMinute  = 30
	deviceTokenPerMinute = 600
)

func (l *ipRate) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w := l.items[ip]
	if w == nil || now.Sub(w.start) >= time.Minute {
		if len(l.items) > 10000 {
			// 顺手清掉过期窗口，防止 map 无限增长
			for k, v := range l.items {
				if now.Sub(v.start) >= time.Minute {
					delete(l.items, k)
				}
			}
		}
		l.items[ip] = &rateWindow{start: now, count: 1}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// writeDeviceErr 写设备码流程（RFC 8628）的错误。error 字段是协议机器码，DSH 客户端按它
// 分支，必须保持原样；另加前端用的 code（见 errcodes.go 的 deviceErrCode）。
func writeDeviceErr(w http.ResponseWriter, status int, oauthCode string) {
	writeErrCode(w, status, deviceErrCode[oauthCode], nil, oauthCode)
}

type deviceCodeReq struct {
	ClientName    string   `json:"client_name"`
	ClientOS      string   `json:"client_os"`
	ClientVersion string   `json:"client_version"`
	Scopes        []string `json:"scopes"`
	// 「开始使用」页的接入码：带上后这次登录会出现在那个页面上
	SetupCode string `json:"setup_code"`
}

func cleanClientField(v string, max int) string {
	v = strings.TrimSpace(v)
	v = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, v)
	r := []rune(v)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

func validDeviceScopes(scopes []string) bool {
	seen := map[string]bool{}
	for _, scope := range scopes {
		if seen[scope] {
			return false
		}
		seen[scope] = true
		switch scope {
		case rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeRegistryPublish, rbac.ScopeHub, rbac.ScopeConnections, rbac.ScopeAgent:
		default:
			return false
		}
	}
	return true
}

func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if !s.deviceCodeRate.allow(s.clientIP(r)) {
		writeDeviceErr(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	var req deviceCodeReq
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeHub, rbac.ScopeConnections, rbac.ScopeAgent}
	}
	if !validDeviceScopes(req.Scopes) {
		writeErrCode(w, http.StatusBadRequest, codeDeviceScopeUnsupported, nil, "scope 不受支持")
		return
	}
	setupCodeOK, err := s.validSetupCode(r.Context(), req.SetupCode)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !setupCodeOK {
		writeErrCode(w, http.StatusBadRequest, codeDeviceSetupCodeExpired, nil, setupCodeExpired)
		return
	}
	plain := secret.Token()
	d := &store.DeviceCode{
		UserCode: store.NewUserCode(), ClientName: cleanClientField(req.ClientName, 128),
		ClientOS: cleanClientField(req.ClientOS, 64), ClientVersion: cleanClientField(req.ClientVersion, 64),
		ClientIP: s.clientIP(r), Scopes: strings.Join(req.Scopes, ","), Status: "pending", ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Truncate(time.Millisecond),
	}
	if err := s.store.CreateDeviceCode(r.Context(), d, secret.Hash(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if req.SetupCode != "" {
		if _, err := s.attachSetupCode(r.Context(), req.SetupCode, d.UserCode); err != nil && !notFound(err) {
			// 接入码可能在两步之间被重新生成；设备登录本身已建好，不因此失败。
			s.log.Warn("记录接入码的设备登录失败", "err", err)
		}
	}
	uri := s.publicBase(r) + "/device"
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code": plain, "user_code": store.FormatUserCode(d.UserCode), "verification_uri": uri,
		"verification_uri_complete": uri + "?code=" + store.FormatUserCode(d.UserCode),
		"interval":                  d.IntervalSeconds, "expires_in": 600,
	})
}

func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	if !s.deviceTokenRate.allow(s.clientIP(r)) {
		writeDeviceErr(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	d, err := s.store.DeviceByHash(r.Context(), secret.Hash(req.DeviceCode))
	if err != nil {
		writeDeviceErr(w, http.StatusBadRequest, "expired_token")
		return
	}
	if d.ExpiresAt.Before(time.Now()) {
		writeDeviceErr(w, http.StatusBadRequest, "expired_token")
		return
	}
	switch d.Status {
	case "denied":
		writeDeviceErr(w, http.StatusBadRequest, "access_denied")
		return
	case "consumed":
		writeDeviceErr(w, http.StatusBadRequest, "expired_token")
		return
	case "pending":
		slow, err := s.store.PollDevice(r.Context(), d.UserCode, time.Duration(d.IntervalSeconds)*time.Second)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if slow {
			writeDeviceErr(w, http.StatusBadRequest, "slow_down")
		} else {
			writeDeviceErr(w, http.StatusBadRequest, "authorization_pending")
		}
		return
	}
	sealed, err := s.store.ConsumeDevice(r.Context(), d.UserCode)
	if err != nil {
		writeDeviceErr(w, http.StatusBadRequest, "expired_token")
		return
	}
	raw, err := s.box.Open(sealed)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 关闭 refresh 时是旧的「只密封 access token」格式；开启后是 JSON
	accessPlain := raw
	var refreshPlain string
	var refreshExpiresAt *time.Time
	if len(raw) > 0 && raw[0] == '{' {
		var payload deviceTokenPayload
		if err := json.Unmarshal(raw, &payload); err != nil || payload.AccessToken == "" {
			s.internalErr(w, r, errors.New("设备令牌密封内容损坏"))
			return
		}
		accessPlain = []byte(payload.AccessToken)
		refreshPlain = payload.RefreshToken
		refreshExpiresAt = payload.RefreshExpiresAt
	}
	t, err := s.store.AccessTokenByHash(r.Context(), secret.Hash(string(accessPlain)))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.demoStat("dsh_connect", t.Username)
	resp := map[string]any{
		"access_token": string(accessPlain), "scopes": strings.Split(d.Scopes, ","),
		"user": map[string]string{"username": t.Username, "department": t.Department}, "expires_at": t.ExpiresAt,
	}
	if refreshPlain != "" {
		resp["refresh_token"] = refreshPlain
		if refreshExpiresAt != nil {
			resp["refresh_expires_in"] = secondsUntil(*refreshExpiresAt)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeviceInfo(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.DeviceByUserCode(r.Context(), r.PathValue("user_code"))
	if err != nil || d.ExpiresAt.Before(time.Now()) {
		writeErrCode(w, http.StatusNotFound, codeDeviceCodeNotFound, nil, "确认码不存在或已过期")
		return
	}
	replace := false
	if d.ClientName != "" {
		tokens, err := s.store.ListUserTokens(r.Context(), currentUser(r).ID)
		if err == nil {
			for _, t := range tokens {
				if t.Source == "device" && t.ClientName == d.ClientName && t.RevokedAt == nil {
					replace = true
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_code": store.FormatUserCode(d.UserCode), "client_name": d.ClientName, "client_os": d.ClientOS,
		"client_version": d.ClientVersion, "client_ip": d.ClientIP, "scopes": strings.Split(d.Scopes, ","),
		"expires_at": d.ExpiresAt, "replace_existing": replace,
	})
}

func (s *Server) handleDeviceApprove(w http.ResponseWriter, r *http.Request) {
	if !s.requireFullSession(w, r) {
		return
	}
	d, err := s.store.DeviceByUserCode(r.Context(), r.PathValue("user_code"))
	if err != nil || d.Status != "pending" || d.ExpiresAt.Before(time.Now()) {
		writeErrCode(w, http.StatusBadRequest, codeDeviceCodeNotPending, nil, "确认码不存在、已处理或已过期")
		return
	}
	var req struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !req.Approve {
		if err := s.store.DenyDevice(r.Context(), d.UserCode); err != nil {
			s.internalErr(w, r, err)
			return
		}
		s.audit(r, "device.deny", store.FormatUserCode(d.UserCode), map[string]any{"client_name": d.ClientName})
		writeJSON(w, http.StatusOK, map[string]any{"status": "denied"})
		return
	}
	plain := accessTokenPrefix + secret.Token()
	settings, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	days := settings.Connect.DeviceTokenDays
	expires := time.Now().UTC().AddDate(0, 0, days).Truncate(time.Millisecond)
	t := &store.AccessToken{Name: d.ClientName, Prefix: plain[:12], MonthlyTokenLimit: nil, ExpiresAt: &expires, SlidingDays: &days}
	// 开启刷新令牌时，批准设备这一步就把第一枚 refresh token 一起签出来，
	// 和 access token 在同一个事务里落库；客户端轮询 /api/device/token 时一次性拿到两枚。
	var rt *store.RefreshToken
	var sealed []byte
	if s.opts.RefreshTokens {
		seed, refreshPlain := newRefreshSeed(currentUser(r).ID, store.ClientKindDSH, s.clientIP(r), s.opts.RefreshFamilyTTL, s.opts.RefreshTokenTTL)
		rt = seed
		payload, err := json.Marshal(deviceTokenPayload{AccessToken: plain, RefreshToken: refreshPlain, RefreshExpiresAt: &rt.ExpiresAt})
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if sealed, err = s.box.Seal(payload); err != nil {
			s.internalErr(w, r, err)
			return
		}
	} else if sealed, err = s.box.Seal([]byte(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.ApproveDevice(r.Context(), d.UserCode, currentUser(r).ID, t, secret.Hash(plain), sealed, rt); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, "device.approve", store.FormatUserCode(d.UserCode), map[string]any{"client_name": d.ClientName, "token_id": t.ID})
	writeJSON(w, http.StatusOK, map[string]any{"status": "approved"})
}

func (s *Server) handleDshLogout(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "缺少接入令牌", "code": tokenInvalid})
		return
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "接入令牌无效", "code": tokenInvalid})
		return
	}
	if p.TokenID > 0 {
		if err := s.store.RevokeToken(r.Context(), p.TokenID); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
