package api_test

import (
	"context"
	"crypto/rand"
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
)

const extOpenAPI = `openapi: 3.0.3
info: { title: Tickets, version: "1" }
paths:
  /api/tickets:
    get:
      operationId: list_tickets
      summary: List tickets
      parameters:
        - { name: status, in: query, schema: { type: string } }
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id: { type: integer }
                    title: { type: string }
                    internal_note: { type: string }
    post:
      operationId: create_ticket
      summary: Open a ticket
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [title, priority]
              properties:
                title: { type: string }
                priority: { type: string }
      responses: { "201": { description: created } }
`

// extManifest 是测试用的声明式扩展清单；server 是假的工单系统的地址。
func extManifest(version, server string, listReadOnly bool, extraServers ...string) string {
	servers, _ := json.Marshal(append([]string{server}, extraServers...))
	return fmt.Sprintf(`{
  "id": "acme/itsm",
  "version": %q,
  "name": { "zh": "工单", "en": "Tickets" },
  "description": "查工单、建工单",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": %s,
  "credential": { "mode": "shared" },
  "tools": [
    { "name": "list_tickets", "readOnly": %t, "description": "按状态列出工单", "keep": ["id", "title"] },
    { "name": "create_ticket", "params": [
      { "name": "body.priority", "fill": "admin", "type": "string", "label": "Agent 建单的优先级", "default": "中" }
    ] }
  ]
}`, version, servers, listReadOnly)
}

// 声明式扩展：检查 → 安装成连接 → 调用时清单生效（只读、管理员参数、只留字段、地址白名单）→ 升级。
func TestDeclarativeExtension(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 假的工单系统：记下收到的请求
	var mu sync.Mutex
	var gotAuth, gotBody, gotKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotAuth, gotKey = r.Header.Get("Authorization"), r.Header.Get("X-API-Key")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/assets":
			w.Write([]byte(`{"records":[{"assetNo":"A-001","owner":"王芳","purchase_price":8999}],"total":1,"debug":"x"}`))
		case "GET /api/tickets":
			w.Write([]byte(`[{"id":1,"title":"VPN 连不上","internal_note":"只给 IT 看"}]`))
		case "POST /api/tickets":
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(201)
			w.Write([]byte(`{"id":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "接入令牌", code, 201, body)
	dsh := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	call := func(tool string, args map[string]any) (string, bool) {
		t.Helper()
		res := dsh.rpc("tools/call", map[string]any{"name": tool, "arguments": args})
		r, _ := res["result"].(map[string]any)
		if r == nil {
			t.Fatalf("调 %s：%v", tool, res)
		}
		text := ""
		for _, c := range r["content"].([]any) {
			text += fmt.Sprint(c.(map[string]any)["text"])
		}
		isErr, _ := r["isError"].(bool)
		return text, isErr
	}
	tools := func() map[string]map[string]any {
		t.Helper()
		out := map[string]map[string]any{}
		for _, x := range dsh.rpc("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
			m := x.(map[string]any)
			if name := m["name"].(string); strings.HasPrefix(name, "itsm__") {
				out[name] = m
			}
		}
		return out
	}
	pkg := func(manifest string) map[string]any {
		return map[string]any{"files": map[string]string{"dsh-ee.extension.json": manifest, "./openapi.yaml": extOpenAPI}}
	}

	// 1. 检查：不改任何东西，给出安装计划
	code, body = admin.do("POST", "/api/admin/extensions/check", pkg(extManifest("1.0.0", up.URL, true)), nil)
	expect(t, "检查扩展", code, 200, body)
	plan := decodeAs[struct {
		Check struct {
			OK       bool
			Identity string
			Tools    []struct {
				Name     string
				ReadOnly bool
			}
		}
		Slug        string
		AdminParams []struct{ Key string } `json:"admin_params"`
		Existing    any
	}](t, body)
	if !plan.Check.OK || plan.Check.Identity != "acme/itsm@1.0.0" || plan.Slug != "itsm" || len(plan.AdminParams) != 1 || plan.AdminParams[0].Key != "priority" || plan.Existing != nil {
		t.Fatalf("安装计划不对：%s", body)
	}

	// 检查不过的不能装
	bad := pkg(strings.Replace(extManifest("1.0.0", up.URL, true), `"kind": "declarative"`, `"kind": "declarative", "oops": 1`, 1))
	bad["files"].(map[string]string)["dsh-ee.extension.json"] = `{"id": "acme/itsm"}`
	code, body = admin.do("POST", "/api/admin/extensions/install", bad, nil)
	expect(t, "检查不过的扩展", code, 400, body)

	// 管理员填的值类型要和清单写的一致
	wrong := pkg(extManifest("1.0.0", up.URL, true))
	wrong["admin"] = map[string]any{"priority": 5}
	code, body = admin.do("POST", "/api/admin/extensions/install", wrong, nil)
	expect(t, "参数类型不对", code, 400, body)

	// 2. 安装
	req := pkg(extManifest("1.0.0", up.URL, true))
	req["admin"] = map[string]any{"priority": "低"}
	req["secret"] = "svc-token"
	code, body = admin.do("POST", "/api/admin/extensions/install", req, nil)
	expect(t, "安装扩展", code, 201, body)
	inst := decodeAs[struct {
		Identity  string
		Tools     int
		SyncError string `json:"sync_error"`
		Asset     struct {
			ID   int64
			Slug string
		}
	}](t, body)
	if inst.Identity != "acme/itsm@1.0.0" || inst.Tools != 2 || inst.SyncError != "" || inst.Asset.Slug != "itsm" {
		t.Fatalf("安装结果不对：%s", body)
	}

	// 3. 写操作默认不启用；只读工具换上清单里的说明
	got := tools()
	if _, ok := got["itsm__create_ticket"]; ok || got["itsm__list_tickets"] == nil {
		t.Fatalf("应只有只读工具：%v", got)
	}
	if d := got["itsm__list_tickets"]["description"].(string); !strings.Contains(d, "按状态列出工单") {
		t.Fatalf("应用清单里的说明：%q", d)
	}

	// 4. 返回只留清单要的字段；带上共享凭据
	text, isErr := call("itsm__list_tickets", nil)
	if isErr || !strings.Contains(text, "VPN") || strings.Contains(text, "internal_note") || gotAuth != "Bearer svc-token" {
		t.Fatalf("list_tickets：%v %s（auth %q）", isErr, text, gotAuth)
	}

	// 5. 管理员打开写操作：优先级由管理员定，模型看不到也改不了
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/create_ticket", inst.Asset.ID), map[string]any{"enabled": true}, nil)
	expect(t, "启用写操作", code, 204, body)
	schema, _ := json.Marshal(tools()["itsm__create_ticket"]["inputSchema"])
	if strings.Contains(string(schema), "priority") || !strings.Contains(string(schema), "title") {
		t.Fatalf("priority 不该出现在工具定义里：%s", schema)
	}
	if text, isErr = call("itsm__create_ticket", map[string]any{"body": map[string]any{"title": "打印机卡纸", "priority": "高"}}); isErr {
		t.Fatalf("create_ticket：%s", text)
	}
	if !strings.Contains(gotBody, `"priority":"低"`) || !strings.Contains(gotBody, "打印机卡纸") {
		t.Fatalf("应带管理员设置的优先级：%s", gotBody)
	}

	// 模型把整个 body 传成字符串想绕过：不发请求
	gotBody = ""
	if text, isErr = call("itsm__create_ticket", map[string]any{"body": `{"title":"x","priority":"高"}`}); !isErr || gotBody != "" {
		t.Fatalf("body 不是对象时不该发出去：%v %s（body %s）", isErr, text, gotBody)
	}

	// 6. 调用记录写明扩展的版本和摘要
	code, body = admin.do("GET", fmt.Sprintf("/api/hub/%d", inst.Asset.ID), nil, nil)
	expect(t, "连接详情", code, 200, body)
	detail := decodeAs[struct {
		Calls []struct{ Tool, Extension string }
		Asset struct{ Spec map[string]any }
	}](t, body)
	if len(detail.Calls) != 3 || !strings.HasPrefix(detail.Calls[0].Extension, "acme/itsm@1.0.0#") || len(detail.Calls[0].Extension) != len("acme/itsm@1.0.0#")+12 {
		t.Fatalf("调用记录应带扩展版本：%s", body)
	}

	// 7. 清单和接口文档不能在连接页面上单独改；根地址不能改到清单之外
	sp := detail.Asset.Spec
	if sp["extension"] == nil {
		t.Fatalf("连接详情里应有扩展信息：%s", body)
	}
	sp["openapi"] = strings.Replace(extOpenAPI, "List tickets", "List all tickets", 1)
	code, body = admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", inst.Asset.ID), map[string]any{"spec": sp}, nil)
	expect(t, "单独改接口文档", code, 400, body)
	sp["openapi"] = extOpenAPI
	sp["url"] = "http://evil.example.com"
	code, body = admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", inst.Asset.ID), map[string]any{"spec": sp}, nil)
	expect(t, "根地址改到清单之外", code, 400, body)
	sp["url"] = up.URL
	// 不能悄悄变回手工连接：清单规定的地址、固定参数、返回筛选就都不管用了
	ext := sp["extension"]
	delete(sp, "extension")
	code, body = admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", inst.Asset.ID), map[string]any{"spec": sp}, nil)
	expect(t, "变回手工连接", code, 400, body)
	if !strings.Contains(string(body), "不能改成手工连接") {
		t.Fatalf("应说明原因：%s", body)
	}
	sp["extension"] = ext
	sp["timeout_seconds"] = 20
	code, body = admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", inst.Asset.ID), map[string]any{"spec": sp}, nil)
	expect(t, "改超时这类连接设置", code, 201, body)

	// 8. 升级：只读变成写操作、多了一个地址 → 要确认；确认后这个工具先停用
	up2 := pkg(extManifest("1.1.0", up.URL, false, "https://backup.example.com"))
	code, body = admin.do("POST", "/api/admin/extensions/check", up2, nil)
	expect(t, "检查新版本", code, 200, body)
	if !strings.Contains(string(body), "从只读变成了写操作") || !strings.Contains(string(body), "backup.example.com") || !strings.Contains(string(body), `"version":"1.0.0"`) {
		t.Fatalf("升级计划应列出变化：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/extensions/install", up2, nil)
	expect(t, "没确认就升级", code, 409, body)
	up2["confirm"] = true
	code, body = admin.do("POST", "/api/admin/extensions/install", up2, nil)
	expect(t, "确认后升级", code, 201, body)
	if !strings.Contains(string(body), `"upgraded_from":"1.0.0"`) {
		t.Fatalf("应是升级：%s", body)
	}
	got = tools()
	if _, ok := got["itsm__list_tickets"]; ok {
		t.Fatalf("变成写操作的工具应先停用：%v", got)
	}
	if got["itsm__create_ticket"] == nil {
		t.Fatalf("原来就是写操作、管理员打开过的工具保持启用：%v", got)
	}
	// 升级沿用了管理员设置的值和共享凭据
	if text, isErr = call("itsm__create_ticket", map[string]any{"body": map[string]any{"title": "换鼠标"}}); isErr || gotAuth != "Bearer svc-token" || !strings.Contains(gotBody, `"priority":"低"`) {
		t.Fatalf("升级后：%v %s（auth %q，body %s）", isErr, text, gotAuth, gotBody)
	}

	// 9. 清单指定共享凭据放在哪个请求头；列表包了一层时 keep 写到第二层
	assets := map[string]any{"files": map[string]string{"openapi.yaml": extAssetsOpenAPI, "dsh-ee.extension.json": fmt.Sprintf(`{
  "id": "acme/assets", "version": "1.0.0", "name": "固定资产", "description": "查固定资产", "kind": "declarative",
  "openapi": "openapi.yaml", "servers": [%q],
  "credential": { "mode": "shared", "header": "X-API-Key" },
  "tools": [ { "name": "list_assets", "readOnly": true, "keep": ["records.assetNo", "records.owner", "total"] } ]
}`, up.URL)}}
	// 提交上来的放法不算数，以清单为准
	assets["auth"] = map[string]any{"type": "bearer"}
	assets["secret"] = "key-123"
	code, body = admin.do("POST", "/api/admin/extensions/install", assets, nil)
	expect(t, "安装第二个扩展", code, 201, body)
	res := dsh.rpc("tools/call", map[string]any{"name": "assets__list_assets", "arguments": map[string]any{}})
	out, _ := json.Marshal(res["result"])
	if !strings.Contains(string(out), "A-001") || !strings.Contains(string(out), "total") || strings.Contains(string(out), "purchase_price") || strings.Contains(string(out), "debug") {
		t.Fatalf("包了一层的列表应按 records.<字段> 筛：%s", out)
	}
	if gotKey != "key-123" || gotAuth != "" {
		t.Fatalf("共享凭据应放在清单指定的 X-API-Key 里：key %q auth %q", gotKey, gotAuth)
	}
}

const extAssetsOpenAPI = `openapi: 3.0.3
info: { title: Assets, version: "1" }
paths:
  /api/assets:
    get:
      operationId: list_assets
      summary: List assets
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  total: { type: integer }
                  debug: { type: string }
                  records:
                    type: array
                    items:
                      type: object
                      properties:
                        assetNo: { type: string }
                        owner: { type: string }
                        purchase_price: { type: number }
`
