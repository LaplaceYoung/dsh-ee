package api_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/eejwt"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// fakeServiceExt 是照协议手写的一个服务扩展（docs/extension-protocol.md），用 dsh-ee 的 JWKS 验每个请求。
// SDK 自己的测试在 extensions/sdk-go；这里验的是 dsh-ee 这一侧有没有照协议做。
type fakeServiceExt struct {
	t        *testing.T
	ts       *httptest.Server
	jwks     string
	manifest func(self string) string

	mu sync.Mutex
	// 最近一次工具调用看到的
	sub, username, cred, shared string
	sys                         bool
	config                      map[string]any
	args                        map[string]any
	// 知识源：id → 标题、正文；changes 是待返回的增量
	docs     map[string][2]string
	changes  []map[string]any
	cursorN  int
	validate func(cfg map[string]any) string
	hits     int
	down     bool
	// oauth 钩子
	issued int
}

func (f *fakeServiceExt) claims(r *http.Request) (map[string]any, bool) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	resp, err := http.Get(f.jwks)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	var set eejwt.JWKS
	_ = json.NewDecoder(resp.Body).Decode(&set)
	c, err := eejwt.Verify(tok, set, eejwt.Expect{Audience: "acme/wiki"})
	return c, err == nil
}

func newFakeServiceExt(t *testing.T, eeBase string, manifest func(self string) string) *fakeServiceExt {
	f := &fakeServiceExt{t: t, jwks: eeBase + "/.well-known/jwks.json", manifest: manifest, docs: map[string][2]string{}}
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(w http.ResponseWriter, status int, code, msg string) {
		writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
	}
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/dsh-ee-extension.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, f.manifest(f.ts.URL))
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		if f.down {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		claims, ok := f.claims(r)
		if !ok {
			fail(w, 401, "unauthenticated", "bad token")
			return
		}
		sys, _ := claims["sys"].(bool)
		cfg := map[string]any{}
		if raw := r.Header.Get("X-DSH-EE-Config"); raw != "" {
			b, _ := base64.StdEncoding.DecodeString(raw)
			_ = json.Unmarshal(b, &cfg)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/mcp":
			id := in["id"]
			reply := func(result any) { writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "result": result}) }
			if id == nil {
				w.WriteHeader(202)
				return
			}
			switch in["method"] {
			case "initialize":
				reply(map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "wiki", "version": "1"}})
			case "tools/list":
				reply(map[string]any{"tools": []map[string]any{
					{"name": "whoami", "description": "x", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}, "annotations": map[string]any{"readOnlyHint": true}},
					// MCP 里说只读、清单里没标：以清单为准，当写操作
					{"name": "create_page", "description": "x", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}, "space": map[string]any{"type": "string"}}}, "annotations": map[string]any{"readOnlyHint": true}},
					{"name": "hidden", "description": "清单里没有", "inputSchema": map[string]any{"type": "object"}},
				}})
			case "tools/call":
				p, _ := in["params"].(map[string]any)
				f.args, _ = p["arguments"].(map[string]any)
				f.sub, _ = claims["sub"].(string)
				f.username, _ = claims["preferred_username"].(string)
				f.sys, f.config = sys, cfg
				f.cred, f.shared = r.Header.Get("X-DSH-EE-Credential"), r.Header.Get("X-DSH-EE-Shared-Credential")
				if q, _ := f.args["q"].(string); q == "deny" {
					reply(map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "只有 HR 能看"}}, "_meta": map[string]any{"dsh-ee/error": map[string]string{"code": "forbidden"}}})
					return
				}
				reply(map[string]any{"content": []map[string]any{{"type": "text", "text": "hello " + f.username}}})
			}
		case "/ee/health":
			writeJSON(w, 200, map[string]any{"ok": true, "version": "1.0.0"})
		case "/ee/validate":
			c, _ := in["config"].(map[string]any)
			if f.validate != nil {
				if msg := f.validate(c); msg != "" {
					writeJSON(w, 200, map[string]any{"ok": false, "message": msg})
					return
				}
			}
			writeJSON(w, 200, map[string]any{"ok": true})
		case "/ee/kb/browse":
			writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "space-hr", "title": "人事", "kind": "folder", "hasChildren": true}}})
		case "/ee/kb/changes":
			if !sys {
				fail(w, 403, "forbidden", "同步应该用系统身份")
				return
			}
			items := f.changes
			f.changes = nil
			f.cursorN++
			writeJSON(w, 200, map[string]any{"items": items, "cursor": fmt.Sprintf("c%d", f.cursorN)})
		case "/ee/kb/document":
			id, _ := in["id"].(string)
			d, ok := f.docs[id]
			if !ok {
				fail(w, 404, "not_found", "没有这篇")
				return
			}
			writeJSON(w, 200, map[string]any{"id": id, "title": d[0], "content": d[1], "url": "https://wiki.example.com/" + id})
		case "/ee/kb/search":
			if sys {
				fail(w, 403, "forbidden", "联邦检索应该带员工身份")
				return
			}
			who, _ := claims["preferred_username"].(string)
			if who == "bob" {
				fail(w, 403, "forbidden", "没有权限")
				return
			}
			writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "p9", "title": "薪酬带宽（" + who + " 可见）", "snippet": "P6 带宽…"}}})
		case "/ee/endpoint":
			body, _ := base64.StdEncoding.DecodeString(fmt.Sprint(in["body"]))
			writeJSON(w, 200, map[string]any{"status": 200, "headers": map[string][]string{"Content-Type": {"text/plain"}, "Set-Cookie": {"evil=1"}},
				"body": base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v %v %s", in["method"], in["path"], body)))})
		case "/ee/oauth/authorize-url":
			writeJSON(w, 200, map[string]any{"url": "https://sso.example.com/authorize?state=" + url.QueryEscape(fmt.Sprint(in["state"]))})
		case "/ee/oauth/exchange":
			p, _ := in["params"].(map[string]any)
			if p["code"] != "good" {
				fail(w, 403, "authorize", "授权码不对")
				return
			}
			f.issued++
			writeJSON(w, 200, map[string]any{"accessToken": fmt.Sprintf("at-%d", f.issued), "refreshToken": "rt-1", "account": "alice@wiki"})
		default:
			fail(w, 404, "not_implemented", r.URL.Path)
		}
	}))
	t.Cleanup(f.ts.Close)
	return f
}

func wikiManifest(version, credential string, extraTools string) func(self string) string {
	return func(self string) string {
		return fmt.Sprintf(`{
  "id": "acme/wiki", "version": %q, "kind": "service", "protocol": "1",
  "name": "内部 Wiki", "description": "查和写 Wiki",
  "service": { "url": %q },
  "capabilities": ["tools", "validate", "health", "knowledge.documents", "knowledge.search", "endpoint", "oauth"],
  "permissions": { "endpoint": true },
  "config": [
    { "key": "tenant", "type": "text", "label": "租户", "required": true },
    { "key": "api_key", "type": "secret", "label": "接口密钥" }
  ],
  "credential": %s,
  "tools": [
    { "name": "whoami", "readOnly": true, "description": "看看我是谁", "params": [{ "name": "q", "type": "string" }] },
    { "name": "create_page", "description": "新建页面", "params": [
      { "name": "title", "type": "string", "required": true },
      { "name": "space", "type": "string", "fill": "admin", "label": "写到哪个空间", "default": "公共" }
    ] }%s
  ]
}`, version, self, credential, extraTools)
	}
}

type svcFixture struct {
	t     *testing.T
	ts    *httptest.Server
	st    *store.Store
	admin *client
}

func newSvcFixture(t *testing.T) *svcFixture {
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	return &svcFixture{t: t, ts: ts, st: st, admin: admin}
}

// user 建一个员工并登录，返回网页会话和 DSH 令牌。
func (f *svcFixture) user(name string) (*client, dshClient) {
	f.t.Helper()
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{"username": name, "password": name + "-password-123", "display_name": name}, nil)
	expect(f.t, "建用户 "+name, code, 201, body)
	c := newClient(f.t, f.ts.URL)
	code, body = c.do("POST", "/api/auth/login", map[string]string{"username": name, "password": name + "-password-123"}, nil)
	expect(f.t, name+" 登录", code, 200, body)
	return c, f.token(c)
}

func (f *svcFixture) token(c *client) dshClient {
	f.t.Helper()
	code, body := c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(f.t, "接入令牌", code, 201, body)
	return dshClient{t: f.t, base: f.ts.URL, token: decodeAs[struct{ Token string }](f.t, body).Token}
}

func toolCall(t *testing.T, d dshClient, header map[string]string, tool string, args map[string]any) (string, bool) {
	t.Helper()
	// header 为 nil 时演的是别的 Agent：不带接入插件的标识
	h := map[string]string{"Accept": "application/json, text/event-stream", "X-DshEE-Client": ""}
	for k, v := range header {
		h[k] = v
	}
	code, body, _ := d.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}, h)
	if code != 200 {
		t.Fatalf("调 %s：HTTP %d %s", tool, code, body)
	}
	var res map[string]any
	_ = json.Unmarshal(body, &res)
	r, _ := res["result"].(map[string]any)
	if r == nil {
		t.Fatalf("调 %s：%s", tool, body)
	}
	text := ""
	for _, c := range r["content"].([]any) {
		text += fmt.Sprint(c.(map[string]any)["text"])
	}
	isErr, _ := r["isError"].(bool)
	return text, isErr
}

// toolCallResult 同 toolCall，但返回原始 result：确认结果的结构化 _meta 用它来断言。
func toolCallResult(t *testing.T, d dshClient, header map[string]string, tool string, args map[string]any) map[string]any {
	t.Helper()
	h := map[string]string{"Accept": "application/json, text/event-stream", "X-DshEE-Client": ""}
	for k, v := range header {
		h[k] = v
	}
	code, body, _ := d.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}, h)
	if code != 200 {
		t.Fatalf("调 %s：HTTP %d %s", tool, code, body)
	}
	var res map[string]any
	_ = json.Unmarshal(body, &res)
	r, _ := res["result"].(map[string]any)
	if r == nil {
		t.Fatalf("调 %s：%s", tool, body)
	}
	return r
}

// resultText / resultIsError 取工具结果的文本与错误标记。
func resultText(r map[string]any) string {
	text := ""
	for _, c := range r["content"].([]any) {
		text += fmt.Sprint(c.(map[string]any)["text"])
	}
	return text
}

func resultIsError(r map[string]any) bool {
	isErr, _ := r["isError"].(bool)
	return isErr
}

var asDSH = map[string]string{"X-DshEE-Client": "dsh-connect/test"}

// 服务扩展：从服务地址安装 → 工具以清单为准 → 调用时带员工身份、配置和凭据 → 错误码 → 回调地址 → 健康检查与熔断。
func TestServiceExtension(t *testing.T) {
	f := newSvcFixture(t)
	ext := newFakeServiceExt(t, f.ts.URL, wikiManifest("1.0.0", `{ "mode": "user_secret", "hint": "在 Wiki 的个人设置里生成" }`, ""))
	ext.validate = func(cfg map[string]any) string {
		if cfg["tenant"] == "bad" {
			return "没有这个租户"
		}
		return ""
	}
	admin := f.admin

	// 上传清单装不了服务扩展：要填服务地址
	code, body := admin.do("POST", "/api/admin/extensions/check", map[string]any{"files": map[string]string{"dsh-ee.extension.json": ext.manifest(ext.ts.URL)}}, nil)
	expect(t, "上传服务扩展的清单", code, 200, body)
	if decodeAs[struct{ Check struct{ OK bool } }](t, body).Check.OK {
		t.Fatalf("服务扩展不能用上传的方式装：%s", body)
	}

	// 1. 检查：从服务地址取清单，给出安装计划
	code, body = admin.do("POST", "/api/admin/extensions/check", map[string]any{"service_url": ext.ts.URL + "/"}, nil)
	expect(t, "检查服务扩展", code, 200, body)
	plan := decodeAs[struct {
		Check    struct{ OK bool }
		Kind     string
		Slug     string
		Config   []struct{ Key, Type string }
		Handover []string
		Check2   any
	}](t, body)
	if !plan.Check.OK || plan.Kind != "service" || plan.Slug != "wiki" || len(plan.Config) != 2 || len(plan.Handover) < 2 {
		t.Fatalf("安装计划不对：%s", body)
	}

	// 2. 必填的配置没填、扩展说配置不对，都装不上
	code, body = admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL}, nil)
	expect(t, "没填必填配置", code, 400, body)
	code, body = admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL, "config": map[string]any{"tenant": "bad"}}, nil)
	expect(t, "扩展说配置不对", code, 400, body)
	if !strings.Contains(string(body), "没有这个租户") {
		t.Fatalf("应带上扩展给的原因：%s", body)
	}

	// 3. 安装
	code, body = admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL, "config": map[string]any{"tenant": "acme", "api_key": "k-secret-123"}}, nil)
	expect(t, "安装服务扩展", code, 201, body)
	inst := decodeAs[struct {
		Tools       int
		SyncError   string   `json:"sync_error"`
		Warnings    []string `json:"warnings"`
		EndpointURL string   `json:"endpoint_url"`
		Asset       struct {
			ID   int64
			Slug string
		}
	}](t, body)
	if inst.Tools != 2 || inst.SyncError != "" || inst.Asset.Slug != "wiki" || !strings.Contains(inst.EndpointURL, "/x/") {
		t.Fatalf("安装结果不对：%s", body)
	}
	if len(inst.Warnings) != 2 {
		t.Fatalf("应提醒清单和 MCP 对不上的两处（hidden 没收、create_page 只读不一致）：%v", inst.Warnings)
	}
	if strings.Contains(string(body), "k-secret-123") {
		t.Fatalf("响应里不该有配置里的密钥：%s", body)
	}

	// 扩展信息：密钥不回显
	code, body = admin.do("GET", fmt.Sprintf("/api/admin/connections/%d/extension", inst.Asset.ID), nil, nil)
	expect(t, "扩展信息", code, 200, body)
	if strings.Contains(string(body), "k-secret-123") || !strings.Contains(string(body), `"tenant":"acme"`) {
		t.Fatalf("扩展信息不对：%s", body)
	}

	alice, aliceDSH := f.user("alice")
	code, body = alice.do("POST", fmt.Sprintf("/api/hub/%d/subscription", inst.Asset.ID), nil, nil)
	if code >= 300 {
		t.Fatalf("启用连接：%d %s", code, body)
	}

	// 4. 只读工具默认启用，写操作要管理员打开；清单里没有的工具不收
	names := map[string]map[string]any{}
	for _, x := range aliceDSH.rpc("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
		m := x.(map[string]any)
		if n := m["name"].(string); strings.HasPrefix(n, "wiki__") {
			names[n] = m
		}
	}
	if len(names) != 1 || names["wiki__whoami"] == nil {
		t.Fatalf("应只有 whoami：%v", names)
	}

	// 5. 员工没填自己的凭据：不发请求，告诉他去哪填
	before := ext.hits
	text, isErr := toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil)
	if !isErr || !strings.Contains(text, "你本人的凭据") || ext.hits != before {
		t.Fatalf("没填凭据应拦下：%v %s", isErr, text)
	}
	// 普通员工看连接详情，看不到扩展的配置和回调地址
	code, body = alice.do("GET", fmt.Sprintf("/api/hub/%d", inst.Asset.ID), nil, nil)
	expect(t, "员工看连接", code, 200, body)
	if strings.Contains(string(body), "$enc") || strings.Contains(string(body), "endpoint") || strings.Contains(string(body), "acme\"") && strings.Contains(string(body), "tenant") {
		t.Fatalf("员工不该看到扩展的配置：%s", body)
	}
	code, body = alice.do("PUT", fmt.Sprintf("/api/me/connections/%d/credential", inst.Asset.ID), map[string]string{"secret": "alice-wiki-token"}, nil)
	expect(t, "填自己的凭据", code, 204, body)

	// 6. 调用：扩展拿到验过签的员工身份、员工的凭据、管理员的配置（密钥已解密）
	text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", map[string]any{"q": "x"})
	if isErr || text != "hello alice" {
		t.Fatalf("whoami：%v %s", isErr, text)
	}
	ext.mu.Lock()
	if ext.username != "alice" || ext.sys || ext.cred != "alice-wiki-token" || ext.config["tenant"] != "acme" || ext.config["api_key"] != "k-secret-123" {
		t.Fatalf("扩展收到的不对：user=%q sys=%v cred=%q config=%v", ext.username, ext.sys, ext.cred, ext.config)
	}
	ext.mu.Unlock()

	// 7. 带错误码的结果换成模型能看懂的话
	text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", map[string]any{"q": "deny"})
	if !isErr || !strings.Contains(text, "没有权限") || !strings.Contains(text, "只有 HR 能看") || !strings.Contains(text, "不要重试") {
		t.Fatalf("forbidden 的说明不对：%s", text)
	}

	// 8. 写操作：打开后，管理员固定的参数由 dsh-ee 补上，模型改不了
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/create_page", inst.Asset.ID), map[string]any{"enabled": true}, nil)
	expect(t, "启用写操作", code, 204, body)
	schema, _ := json.Marshal(aliceDSH.rpc("tools/list", nil))
	if strings.Contains(string(schema), `"space"`) {
		t.Fatalf("管理员固定的参数不该出现在工具定义里")
	}
	if text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__create_page", map[string]any{"title": "周报", "space": "机密"}); isErr {
		t.Fatalf("create_page：%s", text)
	}
	ext.mu.Lock()
	if ext.args["space"] != "公共" || ext.args["title"] != "周报" {
		t.Fatalf("space 应是管理员定的值：%v", ext.args)
	}
	ext.mu.Unlock()

	// 9. 写确认只认令牌的 client_kind 属性，不认可伪造的 X-DshEE-Client 头：
	//    dsh 令牌（设备码签发的 DSH 登录）沿用「DSH 自己有审批卡」的快捷语义；
	//    third_party 令牌始终人工确认。先给 alice 建一个 third_party 令牌，演第三方 Harness。
	code, body = alice.do("POST", "/api/me/tokens", map[string]any{
		"name": "claude-code", "scopes": []string{"hub", "connections"},
		"client_kind": "third_party", "client_name": "claude-code",
	}, nil)
	expect(t, "建 third_party 令牌", code, 201, body)
	other := decodeAs[struct {
		Token string `json:"token"`
		Info  struct {
			ID         string `json:"id"`
			ClientKind string `json:"client_kind"`
			ClientName string `json:"client_name"`
		} `json:"info"`
	}](t, body)
	if other.Info.ClientKind != "third_party" || other.Info.ClientName != "claude-code" {
		t.Fatalf("third_party 令牌响应不对：%s", body)
	}
	var thirdPartyID int64
	fmt.Sscanf(other.Info.ID, "at:%d", &thirdPartyID)
	thirdParty := dshClient{t: t, base: f.ts.URL, token: other.Token}

	// dsh 令牌即使请求头不是 dsh-connect，也按 DSH 快捷语义走
	if text, isErr = toolCall(t, aliceDSH, nil, "wiki__create_page", map[string]any{"title": "令牌是 DSH 的"}); isErr {
		t.Fatalf("dsh 令牌不该要确认：%s", text)
	}

	// third_party 令牌：写操作先停下，员工在网页上同意后，同样的参数再调一次才执行，且只执行一次。
	// 这里故意带伪造的 X-DshEE-Client: dsh-connect，确认不能被它绕过。
	args := map[string]any{"title": "年度总结"}
	before = ext.hits
	first := toolCallResult(t, thirdParty, asDSH, "wiki__create_page", args)
	text, isErr = resultText(first), resultIsError(first)
	if !isErr || !strings.Contains(text, "/confirm/") || ext.hits != before {
		t.Fatalf("third_party 令牌伪造请求头也应要求确认且不执行：%v %s", isErr, text)
	}
	// 需要确认时结构化给出确认页；旧客户端只读 content/isError，会忽略 _meta。
	meta, _ := first["_meta"].(map[string]any)
	confirmation, _ := meta["dsh-ee/confirmation"].(map[string]any)
	if confirmation == nil || len(confirmation) != 3 {
		t.Fatalf("确认结果应带 _meta['dsh-ee/confirmation']{id,url,expires_at}：%v", first)
	}
	rawFirst, _ := json.Marshal(first)
	for _, leak := range []string{"年度总结", "args", "sha256", "token"} {
		if strings.Contains(string(rawFirst), leak) {
			t.Fatalf("确认结果不该带回参数/摘要/凭据（%q）：%s", leak, rawFirst)
		}
	}
	if expiresAt, _ := confirmation["expires_at"].(string); expiresAt == "" {
		t.Fatalf("expires_at 不能为空：%v", confirmation)
	} else if ts, err := time.Parse(time.RFC3339, expiresAt); err != nil || !ts.After(time.Now()) {
		t.Fatalf("expires_at 应是未来的 RFC3339 时间：%q %v", expiresAt, err)
	}
	code, body = alice.do("GET", "/api/me/confirmations", nil, nil)
	expect(t, "待确认列表", code, 200, body)
	pending := decodeAs[struct {
		Items []struct {
			ID           int64
			Tool, System string
			Args         string
		} `json:"items"`
	}](t, body)
	if len(pending.Items) != 1 || pending.Items[0].Tool != "create_page" || !strings.Contains(pending.Items[0].Args, "年度总结") {
		t.Fatalf("待确认列表不对：%s", body)
	}
	cid := pending.Items[0].ID
	// 结构化字段指向同一条待确认记录；文本保持旧格式（旧客户端照旧展示）。
	confirmURL, _ := confirmation["url"].(string)
	if id, _ := confirmation["id"].(float64); id != float64(cid) || !strings.HasSuffix(confirmURL, fmt.Sprintf("/confirm/%d", cid)) {
		t.Fatalf("确认 _meta 与待确认记录不一致：%v，cid=%d", confirmation, cid)
	}
	wantText := "这个操作会修改「" + pending.Items[0].System + "」里的数据，要先由你本人在网页上确认，现在还没有执行。\n" +
		"请打开 " + confirmURL + " 核对参数并点「同意」，然后让我用完全相同的参数再调用一次。\n" +
		"同意后 10 分钟内有效，只能执行一次；参数变了要重新确认。不要改用别的工具绕过。"
	if text != wantText {
		t.Fatalf("确认文本应保持旧格式：\n got %q\nwant %q", text, wantText)
	}
	// 确认记录里带发起确认的令牌
	var confirmToken sql.NullInt64
	if err := f.st.DB().QueryRowContext(context.Background(), "SELECT token_id FROM write_confirmations WHERE id = ?", cid).Scan(&confirmToken); err != nil {
		t.Fatalf("读确认记录：%v", err)
	}
	if !confirmToken.Valid || confirmToken.Int64 != thirdPartyID {
		t.Fatalf("确认记录应记下发起确认的令牌：%v，期望 %d", confirmToken, thirdPartyID)
	}
	// 别人看不到、也替他点不了
	_, bobDSH := f.user("bob")
	bob := newClient(t, f.ts.URL)
	code, body = bob.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-123"}, nil)
	expect(t, "bob 登录", code, 200, body)
	code, body = bob.do("POST", fmt.Sprintf("/api/me/confirmations/%d", cid), map[string]any{"approve": true}, nil)
	expect(t, "别人替他同意", code, 404, body)
	// 再调一次（还没同意）：还是那一条，不新建
	toolCall(t, thirdParty, nil, "wiki__create_page", args)
	code, body = alice.do("GET", "/api/me/confirmations", nil, nil)
	if n := len(decodeAs[struct {
		Items []any `json:"items"`
	}](t, body).Items); n != 1 {
		t.Fatalf("同样的参数不该重复发起确认：%d", n)
	}
	code, body = alice.do("POST", fmt.Sprintf("/api/me/confirmations/%d", cid), map[string]any{"approve": true}, nil)
	expect(t, "同意", code, 200, body)
	// 审批审计里带发起确认的令牌，能按令牌追溯
	code, body = admin.do("GET", "/api/admin/audit?action=write.confirm", nil, nil)
	expect(t, "写确认审计", code, 200, body)
	if !strings.Contains(string(body), fmt.Sprintf(`"token_id":%d`, thirdPartyID)) {
		t.Fatalf("写确认审计应带令牌归属：%s", body)
	}
	// 参数变了：要重新确认
	if text, isErr = toolCall(t, thirdParty, nil, "wiki__create_page", map[string]any{"title": "别的"}); !isErr || !strings.Contains(text, "/confirm/") {
		t.Fatalf("参数变了应重新确认：%s", text)
	}
	if text, isErr = toolCall(t, thirdParty, nil, "wiki__create_page", args); isErr {
		t.Fatalf("同意后应执行：%s", text)
	}
	// 执行成功的调用记录里带这次令牌的 id
	var callToken sql.NullInt64
	if err := f.st.DB().QueryRowContext(context.Background(), "SELECT token_id FROM connection_calls ORDER BY id DESC LIMIT 1").Scan(&callToken); err != nil {
		t.Fatalf("读连接调用记录：%v", err)
	}
	if !callToken.Valid || callToken.Int64 != thirdPartyID {
		t.Fatalf("连接调用应记下令牌归属：%v，期望 %d", callToken, thirdPartyID)
	}
	if text, isErr = toolCall(t, thirdParty, nil, "wiki__create_page", args); !isErr || !strings.Contains(text, "/confirm/") {
		t.Fatalf("一次同意只能执行一次：%v %s", isErr, text)
	}
	// 只读工具不用确认
	if text, isErr = toolCall(t, thirdParty, nil, "wiki__whoami", nil); isErr {
		t.Fatalf("只读工具不该要确认：%s", text)
	}
	// 管理员可以关掉
	code, body = admin.do("PUT", "/api/admin/write-confirm", map[string]string{"mode": "off"}, nil)
	expect(t, "关掉服务端确认", code, 200, body)
	if text, isErr = toolCall(t, thirdParty, nil, "wiki__create_page", map[string]any{"title": "关掉之后"}); isErr {
		t.Fatalf("关掉后不该要确认：%s", text)
	}
	_ = bobDSH

	// 10. 回调地址：第三方不用登录就能打进来，原样转给扩展；扩展不能借它种 cookie
	resp, err := http.Post(inst.EndpointURL+"events/approve?x=1", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(cb) != `POST /events/approve {"a":1}` || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("回调转发不对：%d %s cookie=%q", resp.StatusCode, cb, resp.Header.Get("Set-Cookie"))
	}
	resp, _ = http.Get(f.ts.URL + "/x/" + strings.Repeat("a", 32) + "/x")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("不存在的回调地址应 404：%d", resp.StatusCode)
	}

	// 11. 健康检查；扩展换了清单只记下来，不自动生效
	code, body = admin.do("POST", fmt.Sprintf("/api/admin/connections/%d/extension/health", inst.Asset.ID), nil, nil)
	expect(t, "健康检查", code, 200, body)
	if h := decodeAs[struct{ OK, Changed bool }](t, body); !h.OK || h.Changed {
		t.Fatalf("健康检查不对：%s", body)
	}
	ext.manifest = wikiManifest("1.1.0", `{ "mode": "user_secret", "hint": "在 Wiki 的个人设置里生成" }`, `,
    { "name": "delete_page", "description": "删页面" }`)
	code, body = admin.do("POST", fmt.Sprintf("/api/admin/connections/%d/extension/health", inst.Asset.ID), nil, nil)
	if h := decodeAs[struct {
		OK, Changed bool
		NewVersion  string `json:"new_version"`
	}](t, body); !h.OK || !h.Changed || h.NewVersion != "1.1.0" {
		t.Fatalf("应发现清单变了：%s", body)
	}
	// 升级要管理员确认：计划里写明新增了写操作
	code, body = admin.do("POST", "/api/admin/extensions/check", map[string]any{"service_url": ext.ts.URL}, nil)
	expect(t, "升级计划", code, 200, body)
	if !strings.Contains(string(body), "新增写操作工具 delete_page") || !strings.Contains(string(body), `"set":true`) {
		t.Fatalf("升级计划不对：%s", body)
	}
	// 升级时密钥留空 = 沿用原来的
	code, body = admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL, "config": map[string]any{"tenant": "acme2"}}, nil)
	expect(t, "升级", code, 201, body)
	if text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil); isErr {
		t.Fatalf("升级后调用：%s", text)
	}
	ext.mu.Lock()
	if ext.config["tenant"] != "acme2" || ext.config["api_key"] != "k-secret-123" {
		t.Fatalf("升级后密钥应沿用：%v", ext.config)
	}
	ext.down = true
	ext.mu.Unlock()

	// 12. 扩展连续出错：熔断，之后不再去碰它
	for i := 0; i < 5; i++ {
		toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil)
	}
	before = ext.hits
	text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil)
	if !isErr || !strings.Contains(text, "暂时停用") || ext.hits != before {
		t.Fatalf("应已熔断：%v %s（多打了 %d 次）", isErr, text, ext.hits-before)
	}
}

func kbText(t *testing.T, d dshClient, tool string, args map[string]any) map[string]any {
	t.Helper()
	text, isErr := toolCall(t, d, asDSH, tool, args)
	if isErr {
		t.Fatalf("%s：%s", tool, text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s 的结果不是 JSON：%s", tool, text)
	}
	return out
}

// 知识源：同步（增、改、删，用系统身份）和联邦检索（用员工自己的身份，各人看到的不一样）。
func TestKnowledgeSources(t *testing.T) {
	f := newSvcFixture(t)
	ext := newFakeServiceExt(t, f.ts.URL, wikiManifest("1.0.0", `{ "mode": "ee_jwt" }`, ""))
	admin := f.admin
	code, body := admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL, "config": map[string]any{"tenant": "acme"}}, nil)
	expect(t, "安装服务扩展", code, 201, body)
	assetID := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	code, body = admin.do("POST", "/api/kb", map[string]any{"slug": "hr", "title": "人事制度"}, nil)
	expect(t, "建库", code, 201, body)
	kbID := decodeAs[struct{ ID int64 }](t, body).ID
	_, aliceDSH := f.user("alice")
	_, bobDSH := f.user("bob")

	code, body = admin.do("GET", fmt.Sprintf("/api/kb/%d/sources", kbID), nil, nil)
	expect(t, "知识源列表", code, 200, body)
	if !strings.Contains(string(body), `"modes":["sync","federated"]`) {
		t.Fatalf("应列出能挂的扩展：%s", body)
	}
	code, body = admin.do("POST", fmt.Sprintf("/api/kb/%d/sources/browse", kbID), map[string]any{"asset_id": assetID}, nil)
	expect(t, "浏览知识源", code, 200, body)
	if !strings.Contains(string(body), "space-hr") {
		t.Fatalf("浏览结果不对：%s", body)
	}

	// 同步：新增
	ext.mu.Lock()
	ext.docs["p1"] = [2]string{"请假制度", "# 请假制度\n\n入职满一年有 5 天年假。"}
	ext.changes = []map[string]any{{"id": "p1", "title": "请假制度", "folder": "考勤"}, {"id": "gone", "title": "源系统里已经没有的"}}
	ext.mu.Unlock()
	code, body = admin.do("POST", fmt.Sprintf("/api/kb/%d/sources", kbID), map[string]any{
		"asset_id": assetID, "mode": "sync", "folder": "wiki", "scope": []map[string]string{{"id": "space-hr", "title": "人事"}}}, nil)
	expect(t, "挂同步知识源", code, 201, body)
	created := decodeAs[struct {
		Source struct {
			ID       int64
			DocCount int `json:"doc_count"`
		}
		Sync struct {
			Added, Skipped int
			Error          string
		}
	}](t, body)
	if created.Sync.Added != 1 || created.Sync.Skipped != 1 || created.Sync.Error != "" || created.Source.DocCount != 1 {
		t.Fatalf("第一轮同步不对：%s", body)
	}
	res := kbText(t, aliceDSH, "kb__search", map[string]any{"query": "年假"})
	hits, _ := res["hits"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["title"] != "请假制度" || hits[0].(map[string]any)["folder"] != "wiki/考勤" {
		t.Fatalf("同步进来的文档应能检索到：%v", res)
	}

	// 改：正文变了出新版本；没变的不动
	ext.mu.Lock()
	ext.docs["p1"] = [2]string{"请假制度", "# 请假制度\n\n入职满一年有 7 天年假。"}
	ext.changes = []map[string]any{{"id": "p1", "folder": "考勤"}}
	ext.mu.Unlock()
	sync := func() map[string]any {
		code, body := admin.do("POST", fmt.Sprintf("/api/kb/%d/sources/%d/sync", kbID, created.Source.ID), nil, nil)
		expect(t, "同步", code, 200, body)
		return decodeAs[map[string]any](t, body)
	}
	if r := sync(); r["updated"] != float64(1) || r["added"] != float64(0) {
		t.Fatalf("应更新 1 篇：%v", r)
	}
	if res = kbText(t, aliceDSH, "kb__search", map[string]any{"query": "7 天"}); len(res["hits"].([]any)) != 1 {
		t.Fatalf("应检索到新正文：%v", res)
	}
	ext.mu.Lock()
	ext.changes = []map[string]any{{"id": "p1", "folder": "考勤"}}
	ext.mu.Unlock()
	if r := sync(); r["updated"] != float64(0) {
		t.Fatalf("内容没变不该出新版本：%v", r)
	}
	// 删
	ext.mu.Lock()
	ext.changes = []map[string]any{{"id": "p1", "deleted": true}}
	ext.mu.Unlock()
	if r := sync(); r["deleted"] != float64(1) || r["docs"] != float64(0) {
		t.Fatalf("应删掉 1 篇：%v", r)
	}
	if res = kbText(t, aliceDSH, "kb__search", map[string]any{"query": "年假"}); len(res["hits"].([]any)) != 0 {
		t.Fatalf("删掉后不该还能检索到：%v", res)
	}

	// 普通员工不能挂知识源
	alice := newClient(t, f.ts.URL)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-password-123"}, nil)
	expect(t, "alice 登录", code, 200, body)
	code, body = alice.do("POST", fmt.Sprintf("/api/kb/%d/sources", kbID), map[string]any{"asset_id": assetID, "mode": "federated"}, nil)
	expect(t, "员工挂知识源", code, 403, body)

	// 库的维护者、但不能管理企业连接：同步和浏览用的是扩展的系统身份，读到的内容会给库里的人看，
	// 和内置的飞书文档一样只有能管理连接的人能挂；联邦检索用员工自己的身份，维护者可以挂
	_, body = alice.do("GET", "/api/auth/me", nil, nil)
	aliceID := decodeAs[store.User](t, body).ID
	code, body = admin.do("PATCH", fmt.Sprintf("/api/kb/%d", kbID), map[string]any{"maintainers": map[string]any{"users": []int64{aliceID}},
		"updated_at": scopeToken(t, admin, fmt.Sprintf("/api/kb/%d", kbID))}, nil)
	expect(t, "alice 当维护者", code, 200, body)
	code, body = alice.do("GET", fmt.Sprintf("/api/kb/%d/sources", kbID), nil, nil)
	expect(t, "维护者看知识源", code, 200, body)
	offered := decodeAs[struct {
		Extensions []struct{ Modes []string }
	}](t, body).Extensions
	if len(offered) != 1 || strings.Join(offered[0].Modes, ",") != "federated" {
		t.Fatalf("不能管理连接的维护者只该看到联邦检索：%s", body)
	}
	ext.mu.Lock()
	browsed := ext.hits
	ext.mu.Unlock()
	code, body = alice.do("POST", fmt.Sprintf("/api/kb/%d/sources/browse", kbID), map[string]any{"asset_id": assetID}, nil)
	if code == 200 || strings.Contains(string(body), "space-hr") {
		t.Fatalf("不能管理连接的维护者不能用扩展的系统身份浏览：%d %s", code, body)
	}
	code, body = alice.do("POST", fmt.Sprintf("/api/kb/%d/sources", kbID), map[string]any{"asset_id": assetID, "mode": "sync", "title": "偷偷同步"}, nil)
	if code == 201 {
		t.Fatalf("不能管理连接的维护者不能挂同步：%s", body)
	}
	ext.mu.Lock()
	if ext.hits != browsed {
		t.Fatalf("被拒绝的请求不该碰到扩展（多打了 %d 次）", ext.hits-browsed)
	}
	ext.mu.Unlock()
	code, body = alice.do("POST", fmt.Sprintf("/api/kb/%d/sources", kbID), map[string]any{"asset_id": assetID, "mode": "federated", "title": "维护者挂的实时检索"}, nil)
	expect(t, "维护者挂联邦检索", code, 201, body)
	code, body = admin.do("DELETE", fmt.Sprintf("/api/kb/%d/sources/%d", kbID, decodeAs[struct{ Source struct{ ID int64 } }](t, body).Source.ID), nil, nil)
	expect(t, "删掉维护者挂的", code, 200, body)

	// 联邦检索：用员工自己的身份查，源系统按它的权限返回
	code, body = admin.do("POST", fmt.Sprintf("/api/kb/%d/sources", kbID), map[string]any{"asset_id": assetID, "mode": "federated", "title": "Wiki 实时"}, nil)
	expect(t, "挂联邦检索", code, 201, body)
	fedID := decodeAs[struct{ Source struct{ ID int64 } }](t, body).Source.ID
	res = kbText(t, aliceDSH, "kb__search", map[string]any{"query": "薪酬"})
	external, _ := res["external"].([]any)
	if len(external) != 1 {
		t.Fatalf("应有一个外部来源：%v", res)
	}
	src := external[0].(map[string]any)
	extHits, _ := src["hits"].([]any)
	if src["source"] != "Wiki 实时" || len(extHits) != 1 || !strings.Contains(fmt.Sprint(extHits[0].(map[string]any)["title"]), "alice 可见") {
		t.Fatalf("联邦检索结果不对：%v", src)
	}
	ref := fmt.Sprint(extHits[0].(map[string]any)["doc"])
	if ref != fmt.Sprintf("ext:%d:p9", fedID) {
		t.Fatalf("外部文档编号不对：%s", ref)
	}
	// 源系统说 bob 没权限：只注明这个来源没查成，本地检索不受影响
	res = kbText(t, bobDSH, "kb__search", map[string]any{"query": "薪酬"})
	src = res["external"].([]any)[0].(map[string]any)
	if len(src["hits"].([]any)) != 0 || !strings.Contains(fmt.Sprint(src["error"]), "没有权限") {
		t.Fatalf("bob 不该看到结果：%v", src)
	}
	// 读外部文档：实时取，不落库
	ext.mu.Lock()
	ext.docs["p9"] = [2]string{"薪酬带宽", "第一行\n第二行"}
	ext.mu.Unlock()
	doc := kbText(t, aliceDSH, "kb__read", map[string]any{"external": ref})
	if doc["title"] != "薪酬带宽" || !strings.Contains(fmt.Sprint(doc["text"]), "2| 第二行") || doc["external"] != true {
		t.Fatalf("读外部文档不对：%v", doc)
	}
	if text, isErr := toolCall(t, aliceDSH, asDSH, "kb__read", map[string]any{"external": "ext:99999:p9"}); !isErr {
		t.Fatalf("不存在的知识源应报错：%s", text)
	}

	// 摘掉知识源
	code, body = admin.do("DELETE", fmt.Sprintf("/api/kb/%d/sources/%d", kbID, fedID), nil, nil)
	expect(t, "摘掉联邦检索", code, 200, body)
	if res = kbText(t, aliceDSH, "kb__search", map[string]any{"query": "薪酬"}); res["external"] != nil {
		t.Fatalf("摘掉后不该再查外部来源：%v", res)
	}
}

// oauth：服务扩展的钩子流程，和声明式扩展的标准授权码流程（含到期刷新）。
func TestExtensionOAuth(t *testing.T) {
	f := newSvcFixture(t)
	admin := f.admin
	noRedirect := func(c *client, path string) (int, string) {
		t.Helper()
		hc := *c.hc
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		resp, err := hc.Get(c.base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location")
	}
	stateOf := func(c *client, assetID int64) (string, string) {
		t.Helper()
		code, body := c.do("POST", fmt.Sprintf("/api/me/connections/%d/oauth/start", assetID), nil, nil)
		expect(t, "发起授权", code, 200, body)
		target := decodeAs[struct{ URL string }](t, body).URL
		u, err := url.Parse(target)
		if err != nil || u.Query().Get("state") == "" {
			t.Fatalf("授权地址不对：%s", target)
		}
		return u.Query().Get("state"), target
	}

	// ---- 钩子流程 ----
	ext := newFakeServiceExt(t, f.ts.URL, wikiManifest("1.0.0", `{ "mode": "oauth" }`, ""))
	code, body := admin.do("POST", "/api/admin/extensions/install", map[string]any{"service_url": ext.ts.URL, "config": map[string]any{"tenant": "acme"}}, nil)
	expect(t, "安装服务扩展", code, 201, body)
	wiki := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	alice, aliceDSH := f.user("alice")
	bob, _ := f.user("bob")
	for _, c := range []*client{alice, bob} {
		c.do("POST", fmt.Sprintf("/api/hub/%d/subscription", wiki), nil, nil)
	}
	before := ext.hits
	text, isErr := toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil)
	if !isErr || !strings.Contains(text, "授权") || ext.hits != before {
		t.Fatalf("没授权应拦下、不去碰扩展：%v %s", isErr, text)
	}
	state, _ := stateOf(alice, wiki)
	// 别人拿着 alice 的 state 回调：不认
	if status, loc := noRedirect(bob, "/api/oauth/callback?code=good&state="+url.QueryEscape(state)); status != 302 || !strings.Contains(loc, "oauth_error") {
		t.Fatalf("别人的 state 不该通过：%d %s", status, loc)
	}
	if status, loc := noRedirect(alice, "/api/oauth/callback?code=bad&state="+url.QueryEscape(state)); status != 302 || !strings.Contains(loc, "oauth_error") {
		t.Fatalf("授权码不对应报错：%d %s", status, loc)
	}
	if status, loc := noRedirect(alice, "/api/oauth/callback?code=good&state="+url.QueryEscape(state)); status != 302 || !strings.HasSuffix(loc, fmt.Sprintf("/assets/%d?oauth=ok", wiki)) {
		t.Fatalf("授权应成功：%d %s", status, loc)
	}
	if text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil); isErr {
		t.Fatalf("授权后调用：%s", text)
	}
	ext.mu.Lock()
	if ext.cred != "at-1" || ext.username != "alice" {
		t.Fatalf("扩展应拿到 access token，拿不到 refresh token：cred=%q", ext.cred)
	}
	ext.mu.Unlock()
	code, body = alice.do("GET", "/api/me/connections", nil, nil)
	expect(t, "我的连接", code, 200, body)
	if !strings.Contains(string(body), `"mode":"oauth"`) || !strings.Contains(string(body), "alice@wiki") || strings.Contains(string(body), "rt-1") {
		t.Fatalf("我的连接应显示已授权的账号，不带令牌：%s", body)
	}
	// 取消授权
	code, body = alice.do("DELETE", fmt.Sprintf("/api/me/connections/%d/credential", wiki), nil, nil)
	expect(t, "取消授权", code, 204, body)
	if text, isErr = toolCall(t, aliceDSH, asDSH, "wiki__whoami", nil); !isErr || !strings.Contains(text, "授权") {
		t.Fatalf("取消授权后应要重新授权：%s", text)
	}

	// ---- 标准流程（声明式扩展）----
	var mu sync.Mutex
	var gotAuth string
	var forms []url.Values
	n := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/oauth/token":
			_ = r.ParseForm()
			forms = append(forms, r.PostForm)
			if r.PostForm.Get("client_id") != "cid" || r.PostForm.Get("client_secret") != "csecret" {
				w.WriteHeader(401)
				_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
				return
			}
			n++
			// 30 秒就到期：下一次调用前要先刷新
			_, _ = fmt.Fprintf(w, `{"access_token":"std-%d","refresh_token":"r-%d","expires_in":30}`, n, n)
		case "/api/tickets":
			gotAuth = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `[{"id":1,"title":"VPN"}]`)
		}
	}))
	defer up.Close()
	manifest := fmt.Sprintf(`{
  "id": "acme/itsm", "version": "1.0.0", "name": "工单", "description": "查工单", "kind": "declarative",
  "openapi": "openapi.yaml", "servers": [%q],
  "credential": { "mode": "oauth" },
  "oauth": { "authorizeUrl": "%s/oauth/authorize", "tokenUrl": "%s/oauth/token", "scopes": ["tickets.read"] },
  "tools": [{ "name": "list_tickets", "readOnly": true }]
}`, up.URL, up.URL, up.URL)
	pkg := map[string]any{"files": map[string]string{"dsh-ee.extension.json": manifest, "openapi.yaml": extOpenAPI}}
	code, body = admin.do("POST", "/api/admin/extensions/install", pkg, nil)
	expect(t, "没填 OAuth 客户端", code, 400, body)
	pkg["oauth_client_id"], pkg["oauth_client_secret"] = "cid", "csecret"
	code, body = admin.do("POST", "/api/admin/extensions/install", pkg, nil)
	expect(t, "安装 oauth 的声明式扩展", code, 201, body)
	if strings.Contains(string(body), "csecret") || !strings.Contains(string(body), "/api/oauth/callback") {
		t.Fatalf("应给出要登记的回调地址，不带 client_secret：%s", body)
	}
	itsm := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	alice.do("POST", fmt.Sprintf("/api/hub/%d/subscription", itsm), nil, nil)
	if text, isErr = toolCall(t, aliceDSH, asDSH, "itsm__list_tickets", nil); !isErr || !strings.Contains(text, "授权") {
		t.Fatalf("没授权应拦下：%s", text)
	}
	state, target := stateOf(alice, itsm)
	tu, _ := url.Parse(target)
	if q := tu.Query(); !strings.HasPrefix(target, up.URL+"/oauth/authorize") || q.Get("client_id") != "cid" || q.Get("scope") != "tickets.read" || !strings.HasSuffix(q.Get("redirect_uri"), "/api/oauth/callback") || q.Get("response_type") != "code" {
		t.Fatalf("标准授权地址不对：%s", target)
	}
	if status, loc := noRedirect(alice, "/api/oauth/callback?code=abc&state="+url.QueryEscape(state)); status != 302 || !strings.Contains(loc, "oauth=ok") {
		t.Fatalf("授权应成功：%d %s", status, loc)
	}
	if text, isErr = toolCall(t, aliceDSH, asDSH, "itsm__list_tickets", nil); isErr || !strings.Contains(text, "VPN") {
		t.Fatalf("授权后调用：%s", text)
	}
	mu.Lock()
	defer mu.Unlock()
	// 换到的令牌 30 秒就到期，调用前先刷新了一次：带出去的是刷新后的
	if len(forms) != 2 || forms[0].Get("grant_type") != "authorization_code" || forms[0].Get("code") != "abc" ||
		forms[1].Get("grant_type") != "refresh_token" || forms[1].Get("refresh_token") != "r-1" || gotAuth != "Bearer std-2" {
		t.Fatalf("换令牌和刷新不对：forms=%v auth=%q", forms, gotAuth)
	}
}
