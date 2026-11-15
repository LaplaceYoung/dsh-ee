package api_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const embedHost = "https://harness.example.com"

type embedFixture struct {
	st    *store.Store
	box   *secret.Box
	opts  api.Options
	srv   *api.Server
	ts    *httptest.Server
	admin *client
	slug  string
}

// newEmbedFixture 起一个开启跨站嵌入的服务端，并建一个开放网页版的应用。
func newEmbedFixture(t *testing.T, mutate func(*api.Options)) *embedFixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	opts := api.Options{
		DataDir:         t.TempDir(),
		EmbedOrigins:    []string{embedHost},
		EmbedCrossSite:  true,
		EmbedSessionTTL: time.Hour,
	}
	if mutate != nil {
		mutate(&opts)
	}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	f := &embedFixture{st: st, box: box, opts: opts, srv: srv, ts: ts, admin: admin, slug: "embed-app"}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": f.slug, "delivery": "auto", "spec": embedWebSpec()}, nil)
	expect(t, "建应用", code, 201, body)
	return f
}

// embedWebSpec 是最小可用的开放网页版应用：一张「各自填写」的 notes 表 + 一个主页窗格。
func embedWebSpec() map[string]any {
	return map[string]any{
		"title": "嵌入应用", "description": "e2e", "persona": "你是嵌入助手。", "web": true,
		"tables": []any{map[string]any{
			"name": "notes", "title": "备注", "visibility": "own",
			"fields": []any{map[string]any{"name": "text", "title": "内容", "kind": "text"}},
		}},
		"workbench": map[string]any{"panes": []any{map[string]any{
			"id": "main", "type": "panel", "title": "主页",
			"widgets": []any{map[string]any{"type": "heading", "title": "内容"}},
		}}},
	}
}

// addApp 让管理员再建一个同结构的开放网页版应用，用来验证嵌入会话的应用边界。
func (f *embedFixture) addApp(t *testing.T, slug string) {
	t.Helper()
	code, body := f.admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": slug, "delivery": "auto", "spec": embedWebSpec()}, nil)
	expect(t, "建应用 "+slug, code, 201, body)
}

// user 建一个员工并返回；密码是 employee-password-1。
func (f *embedFixture) user(t *testing.T, username string) store.User {
	t.Helper()
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "password": "employee-password-1", "department": "研发部",
	}, nil)
	expect(t, "建员工 "+username, code, 201, body)
	return decode[store.User](t, body)
}

// token 直接往库里写一个 ee_at_ 令牌（明文用 plain）。
func (f *embedFixture) token(t *testing.T, userID int64, plain string, scopes []string) {
	t.Helper()
	tok := &store.AccessToken{
		UserID: userID, Name: plain, Prefix: plain, Source: "manual", Scopes: scopes,
		ClientName: "test-client", CreatedBy: 1,
	}
	if err := f.st.CreateAccessToken(context.Background(), tok, secret.Hash(plain)); err != nil {
		t.Fatal(err)
	}
}

// instance 起第二个服务端，共用同一个数据库与配置（模拟多实例部署：不同端口/进程）。
func (f *embedFixture) instance(t *testing.T) *embedFixture {
	t.Helper()
	srv := api.New(f.st, f.box, f.opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &embedFixture{st: f.st, box: f.box, opts: f.opts, srv: srv, ts: ts, slug: f.slug}
}

func (f *embedFixture) mintAt(t *testing.T, base, token string, body map[string]any) (int, []byte) {
	t.Helper()
	return newClient(t, base).do("POST", "/api/embed/sessions", body, bearer(token))
}

func (f *embedFixture) mint(t *testing.T, token string, body map[string]any) (int, []byte) {
	t.Helper()
	return f.mintAt(t, f.ts.URL, token, body)
}

// exchange 用原始 HTTP 客户端，方便检查响应头（必须没有 Set-Cookie）。
func (f *embedFixture) exchangeAt(t *testing.T, base, code string) (*http.Response, []byte) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"code": code})
	req, _ := http.NewRequest("POST", base+"/api/auth/embed/exchange", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

func (f *embedFixture) exchange(t *testing.T, code string) (*http.Response, []byte) {
	t.Helper()
	return f.exchangeAt(t, f.ts.URL, code)
}

// exchangeStatus 给并发测试用：不在子 goroutine 里调 t.Fatal。
func exchangeStatus(base, code string) (int, error) {
	raw, _ := json.Marshal(map[string]string{"code": code})
	req, _ := http.NewRequest("POST", base+"/api/auth/embed/exchange", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// mintRaw 给需要检查响应头（Retry-After）的测试用。
func mintRaw(base, token string, body map[string]any) (*http.Response, []byte, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/api/embed/sessions", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out, nil
}

func (f *embedFixture) mintAndExchange(t *testing.T, token string) string {
	t.Helper()
	code, body := f.mint(t, token, map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "mint", code, 200, body)
	minted := decode[struct{ Code string }](t, body)
	resp, raw := f.exchange(t, minted.Code)
	if resp.StatusCode != 200 {
		t.Fatalf("exchange 失败：%d %s", resp.StatusCode, raw)
	}
	return decode[struct{ Token string }](t, raw).Token
}

func (f *embedFixture) meAt(t *testing.T, base, token string) (int, []byte) {
	t.Helper()
	return newClient(t, base).do("GET", "/api/auth/me", nil, bearer(token))
}

func (f *embedFixture) me(t *testing.T, token string) (int, []byte) {
	t.Helper()
	return f.meAt(t, f.ts.URL, token)
}

func TestEmbedMintAndExchange(t *testing.T) {
	f := newEmbedFixture(t, nil)
	emp := f.user(t, "embed-emp")
	f.token(t, emp.ID, "ee_at_embed_emp", []string{rbac.ScopeHub, rbac.ScopeAgent})

	// 没带令牌、令牌没有 agent 范围：都换不了 code
	if code, body := f.mint(t, "", map[string]any{"app": f.slug, "host_origin": embedHost}); code != 401 {
		t.Fatalf("无令牌 mint 应 401：%d %s", code, body)
	}
	f.token(t, emp.ID, "ee_at_embed_hubonly", []string{rbac.ScopeHub})
	if code, body := f.mint(t, "ee_at_embed_hubonly", map[string]any{"app": f.slug, "host_origin": embedHost}); code != 403 {
		t.Fatalf("没有 agent 范围的令牌应 403：%d %s", code, body)
	}

	// 来源不在允许列表、应用不存在：拒绝
	code, body := f.mint(t, "ee_at_embed_emp", map[string]any{"app": f.slug, "host_origin": "https://evil.example.com"})
	expect(t, "来源不在白名单", code, 403, body)
	if got := decode[struct{ Code string }](t, body).Code; got != "origin_not_allowed" {
		t.Fatalf("来源错误码 = %q", got)
	}
	code, body = f.mint(t, "ee_at_embed_emp", map[string]any{"app": "no-such-app", "host_origin": embedHost})
	expect(t, "应用不存在", code, 404, body)
	if got := decode[struct{ Code string }](t, body).Code; got != "app_not_found" {
		t.Fatalf("应用错误码 = %q", got)
	}

	// 正常 mint：60 秒、绑定应用与来源
	code, body = f.mint(t, "ee_at_embed_emp", map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "mint", code, 200, body)
	var minted struct {
		Code       string `json:"code"`
		ExpiresIn  int    `json:"expires_in"`
		SessionTTL int    `json:"session_ttl"`
		App        struct{ Slug, Title string }
	}
	if err := json.Unmarshal(body, &minted); err != nil {
		t.Fatal(err)
	}
	if minted.Code == "" || minted.ExpiresIn != 60 || minted.SessionTTL != 3600 || minted.App.Slug != f.slug {
		t.Fatalf("mint 响应不对：%s", body)
	}

	// 先确认审计记了 mint
	logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "embed.session_create"})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("mint 应该记 embed.session_create 审计")
	}

	// exchange：不带 cookie、不 Set-Cookie，换到 ee_emb_ limited 令牌
	resp, raw := f.exchange(t, minted.Code)
	if resp.StatusCode != 200 {
		t.Fatalf("exchange 失败：%d %s", resp.StatusCode, raw)
	}
	if cookies := resp.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("exchange 不能 Set-Cookie：%v", cookies)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("exchange 应 no-store，实际 %q", resp.Header.Get("Cache-Control"))
	}
	exchanged := decode[struct {
		Token   string `json:"token"`
		Type    string `json:"token_type"`
		Limited bool   `json:"limited"`
	}](t, raw)
	if !strings.HasPrefix(exchanged.Token, "ee_emb_") || exchanged.Type != "Bearer" || !exchanged.Limited {
		t.Fatalf("exchange 响应不对：%s", raw)
	}

	// code 一次性：同一个 code 再兑一次失败
	resp, raw = f.exchange(t, minted.Code)
	if resp.StatusCode != 400 || decode[struct{ Code string }](t, raw).Code != "embed_code_invalid" {
		t.Fatalf("code 应该一次性：%d %s", resp.StatusCode, raw)
	}
	resp, raw = f.exchange(t, "not-a-code")
	if resp.StatusCode != 400 || decode[struct{ Code string }](t, raw).Code != "embed_code_invalid" {
		t.Fatalf("无效 code 应 400 embed_code_invalid：%d %s", resp.StatusCode, raw)
	}

	// me：limited + embed 信息
	code, body = f.me(t, exchanged.Token)
	expect(t, "嵌入会话读 me", code, 200, body)
	var me struct {
		Limited bool `json:"limited"`
		Embed   *struct{ App, Host string }
	}
	_ = json.Unmarshal(body, &me)
	if !me.Limited || me.Embed == nil || me.Embed.App != f.slug || me.Embed.Host != embedHost {
		t.Fatalf("me 应该返回 limited + embed：%s", body)
	}

	// 路由白名单：页面需要的接口放行，其它一律 403 embed_scope
	c := newClient(t, f.ts.URL)
	for _, ok := range []struct{ method, path string }{
		{"GET", "/api/settings"},
		{"GET", "/api/me/menus"},
		{"GET", "/api/me/apps/" + f.slug + "/web"},
		{"GET", "/api/apps/" + f.slug + "/data"},
		{"GET", "/api/auth/methods"},
	} {
		code, body := c.do(ok.method, ok.path, nil, bearer(exchanged.Token))
		expect(t, "白名单内 "+ok.path, code, 200, body)
	}
	for _, blocked := range []struct{ method, path string }{
		{"GET", "/api/admin/users"},
		{"GET", "/api/me/tokens"},
		{"POST", "/api/me/tokens"},
		{"PUT", "/api/admin/settings"},
		{"GET", "/api/hub"},
	} {
		code, body := c.do(blocked.method, blocked.path, map[string]any{}, bearer(exchanged.Token))
		expect(t, "白名单外 "+blocked.path, code, 403, body)
		if got := decode[struct{ Code string }](t, body).Code; got != "embed_scope" {
			t.Fatalf("%s 的错误码 = %q", blocked.path, got)
		}
	}

	// logout：撤销会话，me 立刻 401
	if code, body := c.do("POST", "/api/auth/embed/logout", map[string]any{}, bearer(exchanged.Token)); code != 204 {
		t.Fatalf("logout 应 204：%d %s", code, body)
	}
	if code, _ := f.me(t, exchanged.Token); code != 401 {
		t.Fatalf("logout 后应 401：%d", code)
	}
	if logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "embed.revoke"}); err != nil || len(logs) == 0 {
		t.Fatalf("logout 应该记 embed.revoke 审计：%v %d", err, len(logs))
	}
}

// TestEmbedSessionAppScope：为应用 A mint 的 ee_emb_ 会话只能访问 A；同一员工本人对 B 有权限
// （B 的会话能正常用），但 A 的会话读写 B 必须 403。无 slug 的公共接口仍放行。
func TestEmbedSessionAppScope(t *testing.T) {
	f := newEmbedFixture(t, nil)
	other := "embed-other"
	f.addApp(t, other)
	emp := f.user(t, "embed-scope-emp")
	f.token(t, emp.ID, "ee_at_embed_scope", []string{rbac.ScopeHub, rbac.ScopeAgent})
	tokenA := f.mintAndExchange(t, "ee_at_embed_scope")

	// 同一员工为 B 也能换到会话：说明员工本人对 B 有权限，下面 403 不是 RBAC 拦出来的。
	code, body := f.mint(t, "ee_at_embed_scope", map[string]any{"app": other, "host_origin": embedHost})
	expect(t, "为另一个应用 mint", code, 200, body)
	resp, raw := f.exchange(t, decode[struct{ Code string }](t, body).Code)
	if resp.StatusCode != 200 {
		t.Fatalf("exchange B 失败：%d %s", resp.StatusCode, raw)
	}
	tokenB := decode[struct{ Token string }](t, raw).Token

	c := newClient(t, f.ts.URL)

	// 原行为不变：A 会话能读、写、改、删 A 自己的表。
	code, body = c.do("GET", "/api/apps/"+f.slug+"/data/notes", nil, bearer(tokenA))
	expect(t, "A 会话读 A 的数据表", code, 200, body)
	code, body = c.do("POST", "/api/apps/"+f.slug+"/data/notes", map[string]any{"row": map[string]any{"text": "自己的行"}}, bearer(tokenA))
	expect(t, "A 会话写 A 的数据表", code, 201, body)
	type rowItem struct {
		ID int64 `json:"id"`
	}
	rowID := decode[struct {
		Items []rowItem `json:"items"`
	}](t, body).Items[0].ID
	itemPath := "/api/apps/" + f.slug + "/data/notes/" + strconv.FormatInt(rowID, 10)
	code, body = c.do("PATCH", itemPath, map[string]any{"row": map[string]any{"text": "改一下"}}, bearer(tokenA))
	expect(t, "A 会话改 A 的行", code, 200, body)
	code, body = c.do("DELETE", itemPath, nil, bearer(tokenA))
	expect(t, "A 会话删 A 的行", code, 204, body)
	code, body = c.do("GET", "/api/me/apps/"+f.slug+"/web", nil, bearer(tokenA))
	expect(t, "A 会话读 A 的网页版", code, 200, body)

	// 跨应用：A 会话访问 B 的每个应用级接口都 403 embed_scope，处理函数不执行。
	cross := []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/apps/" + other + "/data", nil},
		{"GET", "/api/apps/" + other + "/data/notes", nil},
		{"POST", "/api/apps/" + other + "/data/notes", map[string]any{"row": map[string]any{"text": "越权"}}},
		{"PATCH", "/api/apps/" + other + "/data/notes/1", map[string]any{"row": map[string]any{"text": "越权"}}},
		{"DELETE", "/api/apps/" + other + "/data/notes/1", nil},
		{"GET", "/api/apps/" + other + "/data/notes/export", nil},
		{"GET", "/api/me/apps/" + other + "/web", nil},
		{"POST", "/api/me/apps/" + other + "/source", map[string]any{"pane": "main", "widget": 0}},
	}
	for _, req := range cross {
		code, body := c.do(req.method, req.path, req.body, bearer(tokenA))
		expect(t, "越界 "+req.method+" "+req.path, code, 403, body)
		got := decode[struct {
			Code  string `json:"code"`
			Stage string `json:"stage"`
			Error string `json:"error"`
		}](t, body)
		if got.Code != "embed_scope" || got.Stage != "session" || !strings.Contains(got.Error, "只能访问被嵌入的应用") {
			t.Fatalf("越界 %s %s 的错误体不对：%s", req.method, req.path, body)
		}
	}

	// 反向同样成立：B 会话能正常用 B，访问 A 同样 403。
	if code, body := c.do("GET", "/api/apps/"+other+"/data/notes", nil, bearer(tokenB)); code != 200 {
		t.Fatalf("B 会话读 B 应 200：%d %s", code, body)
	}
	if code, body := c.do("GET", "/api/me/apps/"+other+"/web", nil, bearer(tokenB)); code != 200 {
		t.Fatalf("B 会话读 B 网页版应 200：%d %s", code, body)
	}
	if code, body := c.do("GET", "/api/apps/"+f.slug+"/data/notes", nil, bearer(tokenB)); code != 403 {
		t.Fatalf("B 会话读 A 应 403：%d %s", code, body)
	}

	// 无应用 slug 的公共接口不受影响；me 里的 embed.app 仍是会话绑定的应用。
	for _, path := range []string{"/api/auth/me", "/api/auth/methods", "/api/settings", "/api/me/menus"} {
		code, body := c.do("GET", path, nil, bearer(tokenA))
		expect(t, "无 slug 接口 "+path, code, 200, body)
	}
	code, body = f.me(t, tokenA)
	expect(t, "嵌入会话读 me", code, 200, body)
	if info := decode[struct {
		Embed *struct{ App string } `json:"embed"`
	}](t, body).Embed; info == nil || info.App != f.slug {
		t.Fatalf("me.embed.app 应为 %s：%s", f.slug, body)
	}
}

func TestEmbedCodeExpiry(t *testing.T) {
	f := newEmbedFixture(t, func(o *api.Options) { o.EmbedCodeTTL = 20 * time.Millisecond })
	other := f.instance(t)
	emp := f.user(t, "embed-exp")
	f.token(t, emp.ID, "ee_at_embed_exp", []string{rbac.ScopeHub, rbac.ScopeAgent})
	code, body := f.mint(t, "ee_at_embed_exp", map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "mint", code, 200, body)
	minted := decode[struct{ Code string }](t, body)
	time.Sleep(50 * time.Millisecond)
	// 换另一个实例也一样：过期判断在共享表里。
	resp, raw := other.exchange(t, minted.Code)
	if resp.StatusCode != 400 || decode[struct{ Code string }](t, raw).Code != "embed_code_expired" {
		t.Fatalf("过期 code 应 400 embed_code_expired：%d %s", resp.StatusCode, raw)
	}
	// 后台清理把过期 code 物理删掉；之后再兑就是 invalid（测试里直接调清理，不等 ticker）。
	if n, err := f.st.DeleteExpiredEmbedCodes(context.Background()); err != nil || n < 1 {
		t.Fatalf("清理过期 code：n=%d err=%v", n, err)
	}
	// 多实例都会跑清理：重复执行幂等，第二次没有东西可删。
	if n, err := f.st.DeleteExpiredEmbedCodes(context.Background()); err != nil || n != 0 {
		t.Fatalf("清理应该幂等：n=%d err=%v", n, err)
	}
	resp, raw = other.exchange(t, minted.Code)
	if resp.StatusCode != 400 || decode[struct{ Code string }](t, raw).Code != "embed_code_invalid" {
		t.Fatalf("清理后 code 应 400 embed_code_invalid：%d %s", resp.StatusCode, raw)
	}
}

func TestEmbedSessionExpiry(t *testing.T) {
	f := newEmbedFixture(t, func(o *api.Options) { o.EmbedSessionTTL = 30 * time.Millisecond })
	emp := f.user(t, "embed-ttl")
	f.token(t, emp.ID, "ee_at_embed_ttl", []string{rbac.ScopeHub, rbac.ScopeAgent})
	tok := f.mintAndExchange(t, "ee_at_embed_ttl")
	if code, body := f.me(t, tok); code != 200 {
		t.Fatalf("会话应可用：%d %s", code, body)
	}
	time.Sleep(60 * time.Millisecond)
	if code, _ := f.me(t, tok); code != 401 {
		t.Fatalf("过期会话应 401：%d", code)
	}
}

func TestEmbedSessionInvalidatedByPasswordChange(t *testing.T) {
	f := newEmbedFixture(t, nil)
	emp := f.user(t, "embed-pw")
	f.token(t, emp.ID, "ee_at_embed_pw", []string{rbac.ScopeHub, rbac.ScopeAgent})
	tok := f.mintAndExchange(t, "ee_at_embed_pw")

	code, body := f.admin.do("PATCH", "/api/admin/users/"+itoa(emp.ID), map[string]any{"password": "new-employee-password-1"}, nil)
	expect(t, "管理员重置密码", code, 200, body)
	if code, _ := f.me(t, tok); code != 401 {
		t.Fatalf("改密后嵌入会话应失效：%d", code)
	}

	// 重新登录拿新 code 后禁用账号，同样失效
	f.token(t, emp.ID, "ee_at_embed_pw2", []string{rbac.ScopeHub, rbac.ScopeAgent})
	tok = f.mintAndExchange(t, "ee_at_embed_pw2")
	code, body = f.admin.do("PATCH", "/api/admin/users/"+itoa(emp.ID), map[string]any{"disabled": true}, nil)
	expect(t, "禁用账号", code, 200, body)
	if code, _ := f.me(t, tok); code != 401 {
		t.Fatalf("禁用后嵌入会话应失效：%d", code)
	}
}

func TestEmbedMintRequiresHumanUser(t *testing.T) {
	// 机器人令牌不能嵌入网页（与网页登录的现有约束一致）。
	f := newEmbedFixture(t, nil)
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": "embed-bot", "account_type": "bot", "owner_id": 1,
	}, nil)
	expect(t, "建机器人", code, 201, body)
	botUser := decode[store.User](t, body)
	f.token(t, botUser.ID, "ee_at_embed_bot", []string{rbac.ScopeHub, rbac.ScopeAgent})
	code, body = f.mint(t, "ee_at_embed_bot", map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "机器人不能 mint", code, 403, body)
}

// E5：错误体带 stage/request_id；embed.deny 审计带 app/host（不记 code/令牌明文）。
func TestEmbedErrorObservability(t *testing.T) {
	f := newEmbedFixture(t, nil)
	emp := f.user(t, "embed-obs")
	f.token(t, emp.ID, "ee_at_embed_obs", []string{rbac.ScopeHub, rbac.ScopeAgent})

	raw := func(path string, body map[string]any, bearerToken string) (*http.Response, []byte) {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", f.ts.URL+path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp, out
	}

	// mint：来源不在白名单 → stage=mint + request_id，且与 X-Request-Id 一致
	resp, body := raw("/api/embed/sessions", map[string]any{"app": f.slug, "host_origin": "https://evil.example.com"}, "ee_at_embed_obs")
	if resp.StatusCode != 403 {
		t.Fatalf("mint 应 403：%d %s", resp.StatusCode, body)
	}
	var mintErr struct {
		Code      string `json:"code"`
		Stage     string `json:"stage"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &mintErr); err != nil {
		t.Fatal(err)
	}
	if mintErr.Code != "origin_not_allowed" || mintErr.Stage != "mint" || mintErr.RequestID == "" {
		t.Fatalf("mint 错误体不对：%s", body)
	}
	if got := resp.Header.Get("X-Request-Id"); got != mintErr.RequestID {
		t.Fatalf("X-Request-Id=%q 与错误体 request_id=%q 不一致", got, mintErr.RequestID)
	}

	// exchange：无效 code → stage=exchange
	resp, body = raw("/api/auth/embed/exchange", map[string]any{"code": "nope"}, "")
	if resp.StatusCode != 400 {
		t.Fatalf("exchange 应 400：%d %s", resp.StatusCode, body)
	}
	var exErr struct {
		Code      string `json:"code"`
		Stage     string `json:"stage"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &exErr); err != nil {
		t.Fatal(err)
	}
	if exErr.Code != "embed_code_invalid" || exErr.Stage != "exchange" || exErr.RequestID == "" {
		t.Fatalf("exchange 错误体不对：%s", body)
	}

	// embed.deny 审计：带 reason + app + host_origin
	logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "embed.deny"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Detail["reason"] == "origin_not_allowed" && l.Detail["app"] == f.slug && l.Detail["host_origin"] == "https://evil.example.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("embed.deny 审计应带 reason/app/host_origin：%+v", logs)
	}
}

// ---- 多实例共享（stacked #118）：code / 会话 / 限流都在数据库里 ----

// TestEmbedCodeSharedAcrossInstances：A 实例 mint、B 实例 exchange 成功；
// code 只存哈希、绑定信息完整、跨实例一次性；会话共享可读。
func TestEmbedCodeSharedAcrossInstances(t *testing.T) {
	f := newEmbedFixture(t, nil)
	other := f.instance(t)
	emp := f.user(t, "embed-multi")
	f.token(t, emp.ID, "ee_at_embed_multi", []string{rbac.ScopeHub, rbac.ScopeAgent})
	ctx := context.Background()

	// next 只能绑定到本应用的站内路径：跨应用/管理页在 mint 时拒绝。
	code, body := f.mint(t, "ee_at_embed_multi", map[string]any{"app": f.slug, "host_origin": embedHost, "next": "/admin/users"})
	expect(t, "非法 next", code, 400, body)
	if got := decode[struct{ Code string }](t, body).Code; got != "next_invalid" {
		t.Fatalf("next 错误码 = %q", got)
	}

	next := "/a/" + f.slug + "?tab=main"
	code, body = f.mint(t, "ee_at_embed_multi", map[string]any{"app": f.slug, "host_origin": embedHost, "next": next})
	expect(t, "A 实例 mint", code, 200, body)
	minted := decode[struct{ Code string }](t, body)

	// 库里只存 sha256、绑定用户/应用/来源/next，且还未消费。
	var (
		hash, host, nextPath, appSlug string
		userID                        int64
		consumed                      sql.NullTime
	)
	if err := f.st.DB().QueryRowContext(ctx,
		`SELECT code_hash, host_origin, next_path, user_id, app_slug, consumed_at FROM embed_codes WHERE code_hash = ?`,
		secret.Hash(minted.Code)).
		Scan(&hash, &host, &nextPath, &userID, &appSlug, &consumed); err != nil {
		t.Fatalf("读 embed_codes：%v", err)
	}
	if hash != secret.Hash(minted.Code) || hash == minted.Code {
		t.Fatalf("库里应该只存 code 的 sha256：hash=%q", hash)
	}
	if host != embedHost || nextPath != next || userID != emp.ID || appSlug != f.slug {
		t.Fatalf("code 绑定信息不对：host=%q next=%q user=%d app=%q", host, nextPath, userID, appSlug)
	}
	if consumed.Valid {
		t.Fatal("刚 mint 的 code 不应该已消费")
	}

	// A 实例 mint、B 实例 exchange：跨实例可用，并带回 mint 时绑定的 app/next。
	resp, raw := other.exchange(t, minted.Code)
	if resp.StatusCode != 200 {
		t.Fatalf("B 实例 exchange 失败：%d %s", resp.StatusCode, raw)
	}
	var exchanged struct {
		Token string `json:"token"`
		Next  string `json:"next"`
		App   struct{ Slug, Title string }
	}
	if err := json.Unmarshal(raw, &exchanged); err != nil {
		t.Fatal(err)
	}
	if exchanged.Token == "" || exchanged.Next != next || exchanged.App.Slug != f.slug || exchanged.App.Title == "" {
		t.Fatalf("exchange 响应没有带回 mint 的绑定：%s", raw)
	}

	// 会话也在共享表里：A 实例认 B 实例 exchange 出来的 token，me.embed 绑定不变。
	code, body = f.me(t, exchanged.Token)
	expect(t, "A 实例读 B 实例的嵌入会话", code, 200, body)
	me := decode[struct {
		Limited bool `json:"limited"`
		Embed   *struct{ App, Host string }
	}](t, body)
	if !me.Limited || me.Embed == nil || me.Embed.App != f.slug || me.Embed.Host != embedHost {
		t.Fatalf("共享会话的 me 不对：%s", body)
	}

	// 已消费：consumed_at 落库，两个实例都不能再兑。
	if err := f.st.DB().QueryRowContext(ctx,
		`SELECT consumed_at FROM embed_codes WHERE code_hash = ?`, secret.Hash(minted.Code)).Scan(&consumed); err != nil {
		t.Fatalf("读 consumed_at：%v", err)
	}
	if !consumed.Valid {
		t.Fatal("exchange 后 consumed_at 应该落库")
	}
	for name, inst := range map[string]*embedFixture{"A": f, "B": other} {
		resp, raw := inst.exchange(t, minted.Code)
		if resp.StatusCode != 400 || decode[struct{ Code string }](t, raw).Code != "embed_code_invalid" {
			t.Fatalf("%s 实例重复 exchange 应 400 embed_code_invalid：%d %s", name, resp.StatusCode, raw)
		}
	}
}

// TestEmbedCodeConcurrentExchangeOnce：同一个 code 被多个实例并发 exchange，只有一个成功。
func TestEmbedCodeConcurrentExchangeOnce(t *testing.T) {
	f := newEmbedFixture(t, nil)
	other := f.instance(t)
	emp := f.user(t, "embed-race")
	f.token(t, emp.ID, "ee_at_embed_race", []string{rbac.ScopeHub, rbac.ScopeAgent})
	code, body := f.mint(t, "ee_at_embed_race", map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "mint", code, 200, body)
	minted := decode[struct{ Code string }](t, body)

	const n = 8
	bases := []string{f.ts.URL, other.ts.URL}
	statuses := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], errs[i] = exchangeStatus(bases[i%2], minted.Code)
		}(i)
	}
	wg.Wait()

	success := 0
	for i, status := range statuses {
		if errs[i] != nil {
			t.Fatalf("并发 exchange 请求失败：%v", errs[i])
		}
		switch status {
		case 200:
			success++
		case 400:
			// 其余请求必须是 invalid；不能是 500/429。
		default:
			t.Fatalf("并发 exchange 返回了意外状态：%d", status)
		}
	}
	if success != 1 {
		t.Fatalf("同一个 code 并发 exchange 只能成功一次，实际成功 %d 次", success)
	}
	// 只消费一次、只建一条会话。
	var consumed int
	if err := f.st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM embed_codes WHERE code_hash = ? AND consumed_at IS NOT NULL`,
		secret.Hash(minted.Code)).Scan(&consumed); err != nil || consumed != 1 {
		t.Fatalf("consumed_at 应该只写一次：%d %v", consumed, err)
	}
	if n, err := f.st.CountActiveEmbedSessions(context.Background(), emp.ID); err != nil || n != 1 {
		t.Fatalf("并发 exchange 只应该建一条会话：%d %v", n, err)
	}
}

// TestEmbedSessionPerUserLimitAndRevokeShared：每用户会话上限、吊销在跨实例时同样生效。
func TestEmbedSessionPerUserLimitAndRevokeShared(t *testing.T) {
	f := newEmbedFixture(t, func(o *api.Options) { o.EmbedMaxSessionsPerUser = 2 })
	other := f.instance(t)
	emp := f.user(t, "embed-cap")
	f.token(t, emp.ID, "ee_at_embed_cap", []string{rbac.ScopeHub, rbac.ScopeAgent})
	tokens := []string{
		f.mintAndExchange(t, "ee_at_embed_cap"),
		f.mintAndExchange(t, "ee_at_embed_cap"),
		f.mintAndExchange(t, "ee_at_embed_cap"),
	}

	// 3 次握手只留 2 个活跃会话：超上限时最旧的被撤销（created_at 同毫秒时顺序不保证，
	// 所以只断言「恰好 2 个还能用、且 B 实例看到的结果与 A 一致」）。
	active := 0
	activeToken := ""
	for _, tok := range tokens {
		code, _ := f.me(t, tok)
		codeB, _ := other.me(t, tok)
		if code != codeB {
			t.Fatalf("两个实例看到的会话状态不一致：A=%d B=%d", code, codeB)
		}
		switch code {
		case 200:
			active++
			activeToken = tok
		case 401:
		default:
			t.Fatalf("me 返回了意外状态：%d", code)
		}
	}
	if active != 2 {
		t.Fatalf("每用户会话上限 2，实际还能用 %d 个", active)
	}
	if n, err := f.st.CountActiveEmbedSessions(context.Background(), emp.ID); err != nil || n != 2 {
		t.Fatalf("活跃会话数 = %d %v，期望 2", n, err)
	}

	// 吊销跨实例：A 实例 logout，B 实例立刻 401（revoked_at 在共享表里）。
	c := newClient(t, f.ts.URL)
	if code, body := c.do("POST", "/api/auth/embed/logout", map[string]any{}, bearer(activeToken)); code != 204 {
		t.Fatalf("logout 应 204：%d %s", code, body)
	}
	if code, _ := other.me(t, activeToken); code != 401 {
		t.Fatalf("B 实例应立刻看到会话被吊销：%d", code)
	}
	var revoked int
	if err := f.st.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM embed_sessions WHERE revoked_at IS NOT NULL").Scan(&revoked); err != nil || revoked != 1 {
		t.Fatalf("revoked_at 应该落库：%d %v", revoked, err)
	}
	// 保留期内的终态会话先留着（管理端「运行概览」按天统计要用），超过保留期才物理删除。
	if n, err := f.st.DeleteExpiredEmbedSessions(context.Background()); err != nil || n != 0 {
		t.Fatalf("保留期内的已吊销会话不应被立即清理：%d %v", n, err)
	}
	if _, err := f.st.DB().ExecContext(context.Background(),
		"UPDATE embed_sessions SET revoked_at = ? WHERE revoked_at IS NOT NULL", time.Now().UTC().Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := f.st.DeleteExpiredEmbedSessions(context.Background()); err != nil || n < 1 {
		t.Fatalf("超过保留期的已吊销会话应被清理：%d %v", n, err)
	}
}

// TestEmbedRateLimitSharedAcrossInstances：mint/exchange 限流是数据库里的合计额度，
// 不会因为多实例部署而成倍放水。
func TestEmbedRateLimitSharedAcrossInstances(t *testing.T) {
	// mint：两个实例合计每分钟 2 次（第三个请求在另一个实例上也应该 429）。
	fm := newEmbedFixture(t, func(o *api.Options) { o.EmbedMintPerMinute = 2 })
	otherMint := fm.instance(t)
	emp := fm.user(t, "embed-mint-rate")
	fm.token(t, emp.ID, "ee_at_embed_mint_rate", []string{rbac.ScopeHub, rbac.ScopeAgent})
	mintBody := map[string]any{"app": fm.slug, "host_origin": embedHost}
	for i := 0; i < 2; i++ {
		code, body := fm.mint(t, "ee_at_embed_mint_rate", mintBody)
		expect(t, "共享额度内 mint", code, 200, body)
	}
	resp, body, err := mintRaw(otherMint.ts.URL, "ee_at_embed_mint_rate", mintBody)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 429 {
		t.Fatalf("跨实例的第三次 mint 应 429：%d %s", resp.StatusCode, body)
	}
	if got := decode[struct{ Code string }](t, body).Code; got != "rate_limited" {
		t.Fatalf("限流错误码 = %q", got)
	}
	if got := resp.Header.Get("Retry-After"); got != "60" {
		t.Fatalf("429 应带 Retry-After=60，实际 %q", got)
	}

	// exchange：另一个库，合计每分钟 2 次（限流键按 IP，两个实例共用一个客户端 IP）。
	fe := newEmbedFixture(t, func(o *api.Options) { o.EmbedMintPerMinute = 10; o.EmbedExchangePerMinute = 2 })
	otherExchange := fe.instance(t)
	empE := fe.user(t, "embed-ex-rate")
	fe.token(t, empE.ID, "ee_at_embed_ex_rate", []string{rbac.ScopeHub, rbac.ScopeAgent})
	var codes []string
	for i := 0; i < 3; i++ {
		code, body := fe.mint(t, "ee_at_embed_ex_rate", map[string]any{"app": fe.slug, "host_origin": embedHost})
		expect(t, "mint", code, 200, body)
		codes = append(codes, decode[struct{ Code string }](t, body).Code)
	}
	for i, code := range codes[:2] {
		inst := fe
		if i == 1 {
			inst = otherExchange
		}
		resp, body := inst.exchange(t, code)
		if resp.StatusCode != 200 {
			t.Fatalf("共享额度内 exchange 应成功：%d %s", resp.StatusCode, body)
		}
	}
	resp, body = otherExchange.exchange(t, codes[2])
	if resp.StatusCode != 429 {
		t.Fatalf("跨实例的第三次 exchange 应 429：%d %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Retry-After"); got != "60" {
		t.Fatalf("429 应带 Retry-After=60，实际 %q", got)
	}
}
