package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

type extensionFixture struct {
	st    *store.Store
	ts    *httptest.Server
	admin *client
	users map[string]*client
	dsh   map[string]dshClient
}

func newExtensionFixture(t *testing.T) *extensionFixture {
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
	f := &extensionFixture{st: st, ts: ts, admin: newClient(t, ts.URL), users: map[string]*client{}, dsh: map[string]dshClient{}}
	code, body := f.admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	for _, name := range []string{"alice", "bob"} {
		code, body = f.admin.do("POST", "/api/admin/users", map[string]any{"username": name, "password": name + "-password-1", "display_name": name, "department": "研发部"}, nil)
		expect(t, "建员工", code, 201, body)
		u := newClient(t, ts.URL)
		code, body = u.do("POST", "/api/auth/login", map[string]string{"username": name, "password": name + "-password-1"}, nil)
		expect(t, "员工登录", code, 200, body)
		code, body = u.do("POST", "/api/me/tokens", map[string]any{"name": "extension-test", "scopes": []string{"hub", "connections"}}, nil)
		expect(t, "员工令牌", code, 201, body)
		f.users[name] = u
		f.dsh[name] = dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	return f
}

func (f *extensionFixture) checkArchive(t *testing.T, archive []byte) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", f.ts.URL+"/api/admin/extensions/check", bytes.NewReader(archive))
	req.Header.Set("Content-Type", "application/zip")
	resp, err := f.admin.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func makeDeclarativeZip(t *testing.T, manifest, document string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{"dsh-ee.extension.json": manifest, "openapi.yaml": document}
	for name, text := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func extensionDocument(up string) string {
	return `openapi: 3.0.3
info: {title: tasks, version: 1.0.0}
servers:
  - url: ` + up + `
paths:
  /items:
    get:
      operationId: list_items
      parameters:
        - {name: region, in: query, schema: {type: string}}
    post:
      operationId: create_item
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [title]
              properties:
                title: {type: string}
                priority: {type: string}
                tenant: {type: string}
                count: {type: integer}
`
}

func extensionManifest(up string) string {
	return `{
		"id":"acme/tasks","version":"1.0.0","name":"任务助手","description":"declarative install integration test",
		"kind":"declarative","openapi":"openapi.yaml","servers":["` + up + `"],
		"credential":{"mode":"user_secret","header":"X-User","hint":"在测试系统里生成自己的 token","shared":{"type":"header","header":"X-Service"}},
		"config":[{"key":"tenant","type":"secret","label":"租户密钥","required":true}],
		"tools":[
			{"name":"list_items","readOnly":true,"params":[{"name":"region","label":"区域","fill":"app","default":"cn"}],"keep":["id","title","region","owner"]},
			{"name":"create_item","params":[
				{"name":"body.priority","label":"优先级","fill":"admin","default":"中","options":["中","高"]},
				{"name":"body.tenant","label":"租户标识","fill":"admin","required":true},
				{"name":"body.count","label":"数量","type":"integer","fill":"app","default":2,"enum":[1,2]}
			]}
		]
	}`
}

func installExtension(f *extensionFixture, archive []byte, server, slug string) (int, []byte) {
	return f.admin.do("POST", "/api/admin/extensions/install", map[string]any{
		"archive": base64.StdEncoding.EncodeToString(archive), "server": server, "slug": slug,
		"config": map[string]any{"tenant": "secret-tenant", "body.tenant": "tenant-A", "body.priority": "高"}, "shared_secret": "service-secret",
	}, nil)
}

type extensionUpstream struct {
	mu       sync.Mutex
	users    []string
	shared   []string
	regions  []string
	createBo map[string]any
}

func (u *extensionUpstream) handler(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.users = append(u.users, r.Header.Get("X-User"))
	u.shared = append(u.shared, r.Header.Get("X-Service"))
	u.regions = append(u.regions, r.URL.Query().Get("region"))
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&u.createBo); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"id": 1, "title": "ticket", "region": r.URL.Query().Get("region"), "owner": r.Header.Get("X-User"), "secret": "must-not-reach-model",
	}})
}

func TestDeclarativeExtensionInstallAndCall(t *testing.T) {
	upstream := &extensionUpstream{}
	up := httptest.NewServer(http.HandlerFunc(upstream.handler))
	defer up.Close()
	f := newExtensionFixture(t)
	archive := makeDeclarativeZip(t, extensionManifest(up.URL), extensionDocument(up.URL))

	code, preview := f.checkArchive(t, archive)
	expect(t, "安装前预检", code, 200, preview)
	var check map[string]any
	if err := json.Unmarshal(preview, &check); err != nil || check["ok"] != true || check["identity"] != "acme/tasks@1.0.0" {
		t.Fatalf("预检信息不对：%s err=%v", preview, err)
	}
	if fields, _ := check["install_fields"].([]any); len(fields) < 3 {
		t.Fatalf("secret 配置与 fill:admin 应进入安装表单：%s", preview)
	}

	code, preview = installExtension(f, archive, up.URL, "tasks")
	expect(t, "安装声明式扩展", code, 201, preview)
	var created struct {
		Asset store.Asset `json:"asset"`
		Tools int         `json:"tools"`
	}
	if err := json.Unmarshal(preview, &created); err != nil || created.Asset.ID == 0 || created.Tools != 2 {
		t.Fatalf("安装没有生成完整连接：%s", preview)
	}
	for _, value := range []string{"secret-tenant", "service-secret"} {
		if strings.Contains(string(preview), value) {
			t.Fatalf("安装响应不能回显凭据：%s", preview)
		}
	}
	in, err := f.st.ExtensionInstallationOf(context.Background(), created.Asset.ID)
	if err != nil || in.ID != "acme/tasks" || in.Version != "1.0.0" || in.Digest == "" || strings.Contains(in.Config, "secret-tenant") {
		t.Fatalf("安装元数据/公开配置不对：%+v err=%v", in, err)
	}
	enc, err := f.st.ExtensionSecretConfig(context.Background(), created.Asset.ID)
	if err != nil || len(enc) == 0 || bytes.Contains(enc, []byte("secret-tenant")) {
		t.Fatalf("secret 配置应加密保存：%x err=%v", enc, err)
	}

	// 换 slug 重复安装时，元数据冲突发生后必须回滚刚创建的 asset。
	code, preview = installExtension(f, archive, up.URL, "tasks-copy")
	expect(t, "重复扩展拒绝安装", code, 409, preview)
	if _, err := f.st.GetAssetBySlug(context.Background(), "tasks-copy"); err != store.ErrNotFound {
		t.Fatalf("重复安装失败后应回滚资产，err=%v", err)
	}

	// 没有填个人凭据时不发请求。
	if r := f.dsh["alice"].rpc("tools/call", map[string]any{"name": "tasks__list_items", "arguments": map[string]any{}}); !strings.Contains(fmt.Sprint(r["result"]), "凭据") {
		t.Fatalf("没有个人凭据时应该提示先填：%v", r)
	}
	for _, name := range []string{"alice", "bob"} {
		code, preview = f.users[name].do("PUT", fmt.Sprintf("/api/me/connections/%d/credential", created.Asset.ID), map[string]string{"secret": name + "-connection-secret"}, nil)
		expect(t, "保存员工凭据", code, 204, preview)
	}
	for _, name := range []string{"alice", "bob"} {
		r := f.dsh[name].rpc("tools/call", map[string]any{"name": "tasks__list_items", "arguments": map[string]any{"region": "malicious"}})
		if strings.Contains(fmt.Sprint(r), "must-not-reach-model") || !strings.Contains(fmt.Sprint(r), `"owner":"`+name+"-connection-secret") {
			t.Fatalf("返回应按员工与 keep 过滤：%v", r)
		}
	}
	for i, name := range []string{"alice", "bob"} {
		if upstream.users[i] != name+"-connection-secret" || upstream.shared[i] != "service-secret" {
			t.Fatalf("共享与员工身份混用了：users=%v shared=%v", upstream.users, upstream.shared)
		}
		if upstream.regions[i] != "cn" {
			t.Fatalf("fill:app 被模型伪造的参数覆盖了：%v", upstream.regions)
		}
	}

	// 写工具默认停用，先由管理员显式启用，再检查它对模型暴露的 schema。
	code, preview = f.admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/create_item", created.Asset.ID), map[string]any{"enabled": true}, nil)
	expect(t, "管理员显式启用写工具", code, 204, preview)
	storedTools, err := f.st.ConnectionTools(context.Background(), created.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range storedTools {
		if tool.Name == "create_item" && !tool.Enabled {
			t.Fatalf("写工具在数据库里仍是停用：%s", preview)
		}
	}
	list := f.dsh["alice"].rpc("tools/list", nil)
	result := list["result"].(map[string]any)
	var createSchema map[string]any
	for _, raw := range result["tools"].([]any) {
		tool := raw.(map[string]any)
		if tool["name"] == "tasks__create_item" {
			createSchema = tool["inputSchema"].(map[string]any)
		}
	}
	if createSchema == nil {
		t.Fatalf("找不到声明式写工具：%v", list)
	}
	body := createSchema["properties"].(map[string]any)["body"].(map[string]any)
	props := body["properties"].(map[string]any)
	if len(props) != 1 || props["title"] == nil {
		t.Fatalf("模型只能填 title，实际 schema：%v", createSchema)
	}
	r := f.dsh["alice"].rpc("tools/call", map[string]any{"name": "tasks__create_item", "arguments": map[string]any{
		"body": map[string]any{"title": "new", "priority": "伪造", "tenant": "伪造", "count": 999},
	}})
	if !strings.Contains(fmt.Sprint(r["result"]), "ok") {
		t.Fatalf("启用后应能创建：%v", r)
	}
	if upstream.createBo["priority"] != "高" || upstream.createBo["tenant"] != "tenant-A" || upstream.createBo["count"] != float64(2) {
		t.Fatalf("管理员/应用参数没有按清单覆盖：%v", upstream.createBo)
	}
	calls, err := f.st.RecentConnectionCalls(context.Background(), created.Asset.ID, 1)
	if err != nil || len(calls) != 1 || calls[0].ExtensionID != "acme/tasks" || calls[0].ExtensionVersion != "1.0.0" || calls[0].ExtensionDigest == "" {
		t.Fatalf("调用应记录清单版本和 digest：%+v err=%v", calls, err)
	}

	code, preview = f.admin.do("DELETE", fmt.Sprintf("/api/hub/%d", created.Asset.ID), nil, nil)
	expect(t, "删除声明式连接", code, 204, preview)
	if _, err := f.st.ExtensionInstallationOf(context.Background(), created.Asset.ID); err != store.ErrNotFound {
		t.Fatalf("删除连接后安装元数据也应清理，err=%v", err)
	}
}
