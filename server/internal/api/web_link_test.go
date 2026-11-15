package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// 从 DSH 打开网页免登录（M12）：设备令牌换一次性链接 → 浏览器换成只带员工权限的网页会话。
func TestWebLinkHandoff(t *testing.T) {
	f := newPrincipalFixture(t)
	emp := f.user(t, "handoff-emp", nil)
	f.token(t, emp.ID, "ee_at_hoemp", []string{rbac.ScopeHub, rbac.ScopeConnections, rbac.ScopeAgent}, nil, nil)
	// 管理员自己的设备令牌：换来的网页会话也不能有管理权限
	f.token(t, 1, "ee_at_hoadm", []string{rbac.ScopeHub, rbac.ScopeConnections, rbac.ScopeAgent}, nil, nil)
	dsh := newClient(t, f.ts.URL)

	link := func(token, next string) (int, string) {
		t.Helper()
		code, body := dsh.do("POST", "/api/dsh/web-link", map[string]string{"next": next}, bearer(token))
		var v struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(body, &v)
		return code, v.URL
	}
	// 浏览器：带 cookie、不自动跟随跳转，看清每一步
	type browser struct {
		hc *http.Client
	}
	newBrowser := func() *browser {
		jar, _ := cookiejar.New(nil)
		return &browser{hc: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	}
	open := func(b *browser, raw string) (int, string, http.Header) {
		t.Helper()
		resp, err := b.hc.Get(raw)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location"), resp.Header
	}
	api := func(b *browser, method, path string, body string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, f.ts.URL+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := b.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}

	// 参数校验
	for _, bad := range []string{"https://evil.example/x", "//evil.example", "/\\evil", "/\t/evil", "/api/admin/users", ""} {
		if code, _ := link("ee_at_hoemp", bad); code != 400 {
			t.Fatalf("next=%q 应被拒绝，得到 %d", bad, code)
		}
	}
	if code, _ := link("ee_at_nope", "/knowledge"); code != 401 {
		t.Fatalf("无效令牌应 401，得到 %d", code)
	}
	// 没有 agent 范围的令牌（只调模型、读目录用的）换不了网页会话：会话带整套员工权限，比令牌本身大
	f.token(t, emp.ID, "ee_at_homodel", []string{rbac.ScopeModels, rbac.ScopeHub, rbac.ScopeConnections}, nil, nil)
	if code, raw := link("ee_at_homodel", "/knowledge"); code != 403 {
		t.Fatalf("没有 agent 范围的令牌不应换到网页登录链接：%d %s", code, raw)
	}

	// 正常流程：跳到目标页，会话只带员工权限
	code, raw := link("ee_at_hoemp", "/knowledge/docs/1?v=2#L3")
	if code != 200 || !strings.Contains(raw, "/api/auth/handoff?code=") {
		t.Fatalf("换链接失败：%d %s", code, raw)
	}
	b := newBrowser()
	st, loc, hdr := open(b, raw)
	if st != http.StatusSeeOther || loc != "/knowledge/docs/1?v=2#L3" {
		t.Fatalf("应跳到目标页：%d %q", st, loc)
	}
	if hdr.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("handoff 应禁止 Referer：%v", hdr)
	}
	st, body := api(b, "GET", "/api/auth/me", "")
	expect(t, "handoff 会话读 me", st, 200, body)
	var me struct {
		Username    string   `json:"username"`
		Limited     bool     `json:"limited"`
		Reauth      bool     `json:"reauth"`
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(body, &me)
	if me.Username != emp.Username || !me.Limited || me.Reauth {
		t.Fatalf("员工的 handoff 会话不对：%s", body)
	}
	for _, p := range me.Permissions {
		if p == rbac.KeysSelf {
			t.Fatalf("handoff 会话不应能管理令牌：%v", me.Permissions)
		}
	}
	st, body = api(b, "GET", "/api/me/connections", "")
	expect(t, "handoff 会话能用员工功能", st, 200, body)
	st, body = api(b, "POST", "/api/me/tokens", `{"name":"x"}`)
	expect(t, "handoff 会话不能生成令牌", st, 403, body)
	st, body = api(b, "POST", "/api/me/npm-tokens", `{"name":"x"}`)
	expect(t, "handoff 会话不能生成 npm 令牌", st, 403, body)
	st, body = api(b, "POST", "/api/me/keys", `{"name":"x"}`)
	expect(t, "handoff 会话不能生成 API key", st, 403, body)
	st, body = api(b, "POST", "/api/me/get-started", `{}`)
	expect(t, "handoff 会话不能生成接入码", st, 403, body)
	st, body = api(b, "POST", "/api/device/ABCDEFGH", `{"approve":true}`)
	expect(t, "handoff 会话不能确认设备登录", st, 403, body)

	// 一次性：再打开同一个链接回到登录页
	b2 := newBrowser()
	st, loc, _ = open(b2, raw)
	if st != http.StatusSeeOther || !strings.HasPrefix(loc, "/login?handoff=expired") || !strings.Contains(loc, url.QueryEscape("/knowledge/docs/1?v=2#L3")) {
		t.Fatalf("重放应回到登录页：%d %q", st, loc)
	}
	// 篡改的 next 只用于失败时回登录页，而且照样校验
	st, loc, _ = open(b2, f.ts.URL+"/api/auth/handoff?code=bogus&next="+url.QueryEscape("//evil.example"))
	if st != http.StatusSeeOther || loc != "/login?handoff=expired&next="+url.QueryEscape("/go") {
		t.Fatalf("坏 next 应退回 /go：%q", loc)
	}

	// 管理员：handoff 会话没有管理权限，提示输入密码登录
	_, raw = link("ee_at_hoadm", "/admin/users")
	ab := newBrowser()
	open(ab, raw)
	st, body = api(ab, "GET", "/api/auth/me", "")
	expect(t, "管理员 handoff 会话读 me", st, 200, body)
	var adminMe struct {
		IsSuper bool `json:"is_super"`
		Limited bool `json:"limited"`
		Reauth  bool `json:"reauth"`
	}
	_ = json.Unmarshal(body, &adminMe)
	if adminMe.IsSuper || !adminMe.Limited || !adminMe.Reauth {
		t.Fatalf("管理员的 handoff 会话应受限并提示重新登录：%s", body)
	}
	st, body = api(ab, "GET", "/api/admin/users", "")
	expect(t, "handoff 会话不能进管理接口", st, 403, body)
	// 输入密码登录后是完整会话
	st, body = api(ab, "POST", "/api/auth/login", `{"username":"admin","password":"admin-password-123"}`)
	expect(t, "重新登录", st, 200, body)
	st, body = api(ab, "GET", "/api/admin/users", "")
	expect(t, "重新登录后能进管理接口", st, 200, body)
	// 已有完整会话时再走 handoff：直接用，不降级，也不重新发 cookie
	_, raw = link("ee_at_hoadm", "/admin/users")
	st, loc, hdr = open(ab, raw)
	if st != http.StatusSeeOther || loc != "/admin/users" || len(hdr.Values("Set-Cookie")) != 0 {
		t.Fatalf("同一个人已有完整会话时应直接跳转、不重建会话：%d %q %v", st, loc, hdr.Values("Set-Cookie"))
	}
	st, body = api(ab, "GET", "/api/admin/users", "")
	expect(t, "handoff 不把完整会话降级", st, 200, body)

	// 浏览器里已经有别人的完整会话（这里是管理员，code 属于员工）：不能替换会话。
	// handoff 是 GET、没有 CSRF 令牌，链接谁都能发给谁；替换会话等于把当前用户静默踢下线、
	// 变成链接主人的身份（登录 CSRF / 会话替换）。应回登录页提示先退出，且不自动登出。
	_, raw = link("ee_at_hoemp", "/knowledge")
	st, loc, hdr = open(ab, raw)
	if st != http.StatusSeeOther || !strings.HasPrefix(loc, "/login?handoff=other-session") {
		t.Fatalf("已有别人的会话时应回登录页提示：%d %q", st, loc)
	}
	if set := hdr.Values("Set-Cookie"); len(set) != 0 {
		t.Fatalf("不能覆盖已有会话的 cookie：%v", set)
	}
	st, body = api(ab, "GET", "/api/auth/me", "")
	expect(t, "他人的 handoff 不应影响原会话", st, 200, body)
	var kept struct {
		Username string `json:"username"`
		Limited  bool   `json:"limited"`
	}
	_ = json.Unmarshal(body, &kept)
	if kept.Username != "admin" || kept.Limited {
		t.Fatalf("原会话应原样保留：%s", body)
	}
	st, body = api(ab, "GET", "/api/admin/users", "")
	expect(t, "原会话仍是完整会话", st, 200, body)

	// 被拒时没有消费 code：退出后同一链接（60 秒内）在没有会话的浏览器里照常可用。
	fb := newBrowser()
	st, loc, hdr = open(fb, raw)
	if st != http.StatusSeeOther || loc != "/knowledge" {
		t.Fatalf("退出后同一链接应正常跳到目标页：%d %q", st, loc)
	}
	if len(hdr.Values("Set-Cookie")) == 0 {
		t.Fatal("无会话时 handoff 应发会话 cookie")
	}
	st, body = api(fb, "GET", "/api/auth/me", "")
	expect(t, "无会话时仍能建 limited 会话", st, 200, body)
	var fresh struct {
		Username string `json:"username"`
		Limited  bool   `json:"limited"`
	}
	_ = json.Unmarshal(body, &fresh)
	if fresh.Username != emp.Username || !fresh.Limited {
		t.Fatalf("无会话时应建 limited 会话：%s", body)
	}
}
