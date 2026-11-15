package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 从 DSH 打开网页免登录（第一版 M12，docs/product-v1.md §10.6）。
//
// 接入插件凭设备令牌换一个一次性 code（60 秒有效，用一次即作废），浏览器打开
// /api/auth/handoff?code=… 时换成网页会话再跳到目标页。这样换来的会话标记为 limited：
// 只带员工权限，不能确认设备登录、不能生成接入码和令牌；管理功能要输入密码重新登录。
// 带 agent 范围的设备令牌本来就能以员工身份调接口，换成网页会话不会多出任何权限；
// 不带 agent 范围的令牌换不了。
//
// 多实例：一次性 code 放共享表 ephemeral_states（只存 sha256），一次性的 CAS 在
// store.ConsumeEphemeralState 里；A 实例生成、B 实例打开 handoff 也能成功。
// 数据库不可用时直接失败（返回 500），不回退到进程内存。

const (
	webLinkTTL  = 60 * time.Second
	webLinkKind = "web_link"
)

// webLinkPayload 是 handoff code 绑定的内容：谁、成功后跳到哪个站内路径。
type webLinkPayload struct {
	UserID int64  `json:"user_id"`
	Next   string `json:"next"`
}

// limitedAccess 是从 DSH 打开的网页会话的权限：员工权限，去掉「我的令牌」（令牌不能用来再造令牌）。
func limitedAccess(a *rbac.Access) *rbac.Access {
	out := rbac.ScopeFilter(a, []string{rbac.ScopeAgent})
	delete(out.Perms, rbac.KeysSelf)
	return out
}

// requireFullSession 拦住从 DSH 打开的网页会话做的敏感操作；已写好响应时返回 false。
func (s *Server) requireFullSession(w http.ResponseWriter, r *http.Request) bool {
	if sessionLimited(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "这个操作需要输入密码登录：你是从 DSH 直接打开的网页", "code": "reauth_required"})
		return false
	}
	return true
}

// webNext 只接受站内相对路径；不合法时返回空串。OIDC 登录入口的 safeNext 也复用这套判断
// （区别只是回退到 /go），两边不能各写一套：浏览器会吞掉制表符和换行，「/\t/evil」会变成
// 「//evil」。
func webNext(raw string) string {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\\") {
		return ""
	}
	for _, c := range raw {
		if c < 0x20 || c == 0x7f {
			return ""
		}
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/api/") {
		return ""
	}
	return raw
}

// POST /api/dsh/web-link {next}：接入插件凭设备令牌换一次性的网页登录链接。
func (s *Server) handleDshWebLink(w http.ResponseWriter, r *http.Request) {
	p, ok := s.dshPrincipal(w, r)
	if !ok {
		return
	}
	if p.User.AccountType != "human" {
		writeErr(w, http.StatusForbidden, "机器人账号不能登录网页")
		return
	}
	// 换来的会话带 agent 范围的整套员工权限（发布、写知识库、填应用数据）。只调模型、读目录的令牌
	// （API key、接入令牌、不带 agent 的设备令牌）本身做不了这些，不能靠换会话多出权限来。
	// 接入插件换不到时会打开原链接，员工在浏览器里登录一次即可。
	if !p.Scopes.Has(rbac.ScopeAgent) {
		writeErr(w, http.StatusForbidden, "这个令牌不能免登录打开网页，请在浏览器里登录")
		return
	}
	var req struct {
		Next string `json:"next"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	next := webNext(req.Next)
	if next == "" {
		writeErr(w, http.StatusBadRequest, "next 只能是 dsh-ee 网页上的相对路径")
		return
	}
	base, err := s.connectBase(r.Context(), r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	code := secret.Token()
	payload, err := json.Marshal(webLinkPayload{UserID: p.User.ID, Next: next})
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.PutEphemeralState(r.Context(), store.EphemeralState{
		Kind:      webLinkKind,
		StateHash: secret.Hash(code),
		Payload:   payload,
		ExpiresAt: time.Now().Add(webLinkTTL),
	}); err != nil {
		// 没有内存回退：多实例下回退会导致「A 实例生成的链接 B 实例用不了」。
		s.internalErr(w, r, err)
		return
	}
	q := url.Values{"code": {code}, "next": {next}}
	writeJSON(w, http.StatusOK, map[string]any{"url": base + "/api/auth/handoff?" + q.Encode(), "expires_in": int(webLinkTTL.Seconds())})
}

// GET /api/auth/handoff?code=…&next=…：换成网页会话并跳到目标页。
// next 只在 code 无效时用来回到登录页；成功时跳到 code 绑定的地址。
func (s *Server) handleAuthHandoff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	codeHash := secret.Hash(q.Get("code"))
	consumeFailed := func() {
		fallback := webNext(q.Get("next"))
		if fallback == "" {
			fallback = "/go"
		}
		http.Redirect(w, r, "/login?handoff=expired&next="+url.QueryEscape(fallback), http.StatusSeeOther)
	}
	stateFailed := func(err error) {
		switch {
		case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrEphemeralStateExpired):
			consumeFailed()
		default:
			// 数据库不可用：不能凭内存里的副本继续，否则多实例下行为不一致。
			s.internalErr(w, r, err)
		}
	}
	// 先读不消费：得先知道 code 属于谁，才能判断浏览器里是不是已经登录了另一个人。
	// 已有别人的会话时下面不消费 code —— 员工退出后，同一个链接在 60 秒内还能用。
	st, err := s.store.GetEphemeralState(r.Context(), webLinkKind, codeHash)
	if err != nil {
		stateFailed(err)
		return
	}
	var pending webLinkPayload
	if err := json.Unmarshal(st.Payload, &pending); err != nil {
		s.log.Warn("网页登录链接的 payload 无法解析", "err", err)
		consumeFailed()
		return
	}
	// 浏览器里已经有会话：是自己的就沿用（不把完整会话降级）；是别人的则绝不能替换。
	// handoff 是 GET、没有 CSRF 令牌，SameSite=Lax 也拦不住顶层导航：任何拿到 agent 令牌的
	// 员工都能把链接发给同事，静默把同事踢下线并登录成自己（登录 CSRF / 会话替换）。
	// 这时回登录页提示先退出，不自动登出当前用户（否则发个链接就能把人踢下线）。
	sameUser := false
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if u, _, err := s.store.SessionInfo(r.Context(), secret.Hash(c.Value)); err == nil {
			if u.ID != pending.UserID {
				http.Redirect(w, r, "/login?handoff=other-session&next="+url.QueryEscape(pending.Next), http.StatusSeeOther)
				return
			}
			sameUser = true
		}
	}
	// 真正「使用」这个链接：原子消费，保证一次性（多实例并发时也只有一个请求能建会话）。
	if _, err := s.store.ConsumeEphemeralState(r.Context(), webLinkKind, codeHash); err != nil {
		stateFailed(err)
		return
	}
	if sameUser {
		http.Redirect(w, r, pending.Next, http.StatusSeeOther)
		return
	}
	u, err := s.store.GetUser(r.Context(), pending.UserID)
	if notFound(err) || (err == nil && (u.Disabled || u.AccountType != "human")) {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(pending.Next), http.StatusSeeOther)
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	token := secret.Token()
	if err := s.store.CreateLimitedSession(r.Context(), secret.Hash(token), u.ID, s.opts.SessionTTL); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.auditAs(r, u, auditLogin, u.Username, map[string]any{"via": "dsh", "source": u.AuthSource, "limited": true})
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(s.opts.SessionTTL.Seconds()), HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, pending.Next, http.StatusSeeOther)
}
