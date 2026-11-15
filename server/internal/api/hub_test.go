package api_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// fakeMCP 是一个要求 Bearer 凭据的上游 MCP 服务，记下收到的调用。
type fakeMCP struct {
	*httptest.Server
	calls atomic.Int32
	last  atomic.Value
}

func newFakeMCP(t *testing.T) *fakeMCP {
	f := &fakeMCP{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer up-secret" {
			w.WriteHeader(401)
			return
		}
		var m struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any
		switch m.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "s1")
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "crm"}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{
				{"name": "lookup", "description": "Look up a customer", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}, "annotations": map[string]any{"readOnlyHint": true}},
				{"name": "update", "description": "Update a customer", "inputSchema": map[string]any{"type": "object"}},
				{"name": "export", "description": "Export all customers", "inputSchema": map[string]any{"type": "object"}},
			}}
		case "tools/call":
			f.calls.Add(1)
			f.last.Store(string(m.Params))
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "CRM says hi " + string(m.Params)}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": result})
	}))
	t.Cleanup(f.Close)
	return f
}

// dsh 是员工本机 DSH 的视角：用接入令牌调 /api/dsh 与 /mcp。
type dshClient struct {
	t     *testing.T
	base  string
	token string
}

func (d dshClient) do(method, path string, body any, header map[string]string) (int, []byte, http.Header) {
	d.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, d.base+path, rd)
	req.Header.Set("Authorization", "Bearer "+d.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// 这个客户端演的是 DSH 里的接入插件；要演别的 Agent（Claude Code 之类）时把这个头传成空
	req.Header.Set("X-DshEE-Client", "dsh-connect/test")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

func (d dshClient) rpc(method string, params any) map[string]any {
	d.t.Helper()
	code, body, _ := d.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, map[string]string{"Accept": "application/json, text/event-stream"})
	if code != 200 {
		d.t.Fatalf("%s：HTTP %d %s", method, code, body)
	}
	var r map[string]any
	_ = json.Unmarshal(body, &r)
	return r
}

func (d dshClient) toolNames() []string {
	r := d.rpc("tools/list", nil)
	var out []string
	for _, t := range r["result"].(map[string]any)["tools"].([]any) {
		name := t.(map[string]any)["name"].(string)
		if strings.HasPrefix(name, "home__") { // 系统应用「工作台」的只读工具，home_test 单独测
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// systemHomeApp 是系统应用「工作台」的标识（api.workbenchHomeName），它对所有人都会下发。
const systemHomeApp = "workbench"

type catalogSkill struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	ModelInvocable bool   `json:"model_invocable"`
	UserInvocable  bool   `json:"user_invocable"`
	Version        int    `json:"version"`
}

type catalogApp struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Starters    []string `json:"starters"`
	Launch      string   `json:"launch"`
	Version     int      `json:"version"`
	Missing     []string `json:"missing"`
}

type catalog struct {
	Skills   []catalogSkill `json:"skills"`
	Apps     []catalogApp   `json:"apps"`
	Required []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"required_plugins"`
	Revision   string `json:"revision"`
	ServerName string `json:"server_name"`
}

func (c catalog) skillNames() []string {
	var out []string
	for _, s := range c.Skills {
		out = append(out, s.Name)
	}
	return out
}

func (d dshClient) catalog() catalog {
	d.t.Helper()
	code, body, _ := d.do("GET", "/api/dsh/catalog", nil, nil)
	if code != 200 {
		d.t.Fatalf("catalog：%d %s", code, body)
	}
	var c catalog
	_ = json.Unmarshal(body, &c)
	// 系统应用「工作台」对所有人都会下发（home_test 单独测它），这里去掉，断言只看测试自己建的。
	c.Skills = slices.DeleteFunc(c.Skills, func(e catalogSkill) bool { return e.Name == systemHomeApp })
	c.Apps = slices.DeleteFunc(c.Apps, func(e catalogApp) bool { return e.Name == systemHomeApp })
	return c
}

func TestHub(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	var sawUsage atomic.Bool
	llm := fakeUpstream(t, &sawUsage)
	defer llm.Close()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), Upstream: llm.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"], set["orgName"], set["baseUrl"] = "@acme", "ACME", ts.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "设置作用域", code, 200, body)

	users := map[string]*client{}
	dsh := map[string]dshClient{}
	for _, u := range []struct{ name, dept string }{{"alice", "研发效能部"}, {"bob", "财务部"}} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": u.name, "password": u.name + "-password-1", "department": u.dept}, nil)
		expect(t, "建用户", code, 201, body)
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": u.name, "password": u.name + "-password-1"}, nil)
		expect(t, "登录", code, 200, body)
		code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
		expect(t, "接入令牌", code, 201, body)
		users[u.name] = c
		dsh[u.name] = dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	alice, bob := users["alice"], users["bob"]

	// ---- 技能：员工提交 → 待审核 → 管理员通过 ----
	skill := map[string]any{"kind": "skill", "slug": "expense-policy", "spec": map[string]any{
		"title": "报销制度", "description": "差旅与报销标准", "when_to_use": "员工问能不能报销时", "body": "住宿每晚 1300 元以内。", "icon": "🧾", "launch": "resume"}}
	code, body = alice.do("POST", "/api/hub", skill, nil)
	expect(t, "提交技能", code, 201, body)
	expenseID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID
	if decodeAs[struct{ Status string }](t, body).Status != "pending" {
		t.Fatalf("白名单模式下员工提交应待审核：%s", body)
	}
	code, body = alice.do("GET", fmt.Sprintf("/api/hub/%d", expenseID), nil, nil)
	expect(t, "读取技能详情", code, 200, body)
	cleared := decodeAs[struct {
		Asset struct {
			Spec struct {
				Icon   string `json:"icon"`
				Launch string `json:"launch"`
			}
		}
	}](t, body)
	if cleared.Asset.Spec.Icon != "" || cleared.Asset.Spec.Launch != "" {
		t.Fatalf("非微应用不应保留图标与打开方式：%s", body)
	}
	code, body = alice.do("POST", "/api/hub", skill, nil)
	expect(t, "标识重复", code, 409, body)
	bad := map[string]any{"kind": "skill", "slug": "Bad_Name", "spec": skill["spec"]}
	code, body = alice.do("POST", "/api/hub", bad, nil)
	expect(t, "标识不合法", code, 400, body)
	if initial := dsh["alice"].catalog(); len(initial.Skills) != 0 || initial.Apps == nil {
		t.Fatalf("待审核的技能不应下发，空微应用目录应为 []：%+v", initial)
	}
	code, body = admin.do("GET", "/api/admin/hub/review", nil, nil)
	expect(t, "审核队列", code, 200, body)
	if !strings.Contains(string(body), "expense-policy") {
		t.Fatalf("审核队列里应有技能：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/hub/review", map[string]any{"asset_id": expenseID, "version": 1, "action": "approve"}, nil)
	expect(t, "通过", code, 204, body)
	// alice 是作者，默认启用；bob 没启用（自选），也在范围内
	if c := dsh["alice"].catalog(); !slices.Equal(c.skillNames(), []string{"expense-policy"}) || !c.Skills[0].ModelInvocable {
		t.Fatalf("alice 应有技能：%+v", c)
	}
	if len(dsh["bob"].catalog().Skills) != 0 {
		t.Fatal("bob 没启用，不应下发")
	}
	code, body = bob.do("POST", fmt.Sprintf("/api/hub/%d/subscription", expenseID), nil, nil)
	expect(t, "bob 启用", code, 204, body)
	c := dsh["bob"].catalog()
	if !slices.Equal(c.skillNames(), []string{"expense-policy"}) {
		t.Fatalf("bob 启用后应有技能：%+v", c)
	}
	// 目录没变 → 304
	code, _, _ = dsh["bob"].do("GET", "/api/dsh/catalog", nil, map[string]string{"If-None-Match": `"` + c.Revision + `"`})
	if code != 304 {
		t.Fatalf("目录没变应 304，得到 %d", code)
	}
	code, body, _ = dsh["bob"].do("GET", "/api/dsh/skills/expense-policy", nil, nil)
	if code != 200 || !strings.Contains(string(body), "1300") {
		t.Fatalf("技能正文：%d %s", code, body)
	}
	// 范围收窄到研发效能部：bob 立即失去，alice 保留
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", expenseID), map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}},
		"updated_at": scopeToken(t, alice, fmt.Sprintf("/api/hub/%d", expenseID))}, nil)
	expect(t, "作者改范围", code, 200, body)
	if len(dsh["bob"].catalog().Skills) != 0 {
		t.Fatal("范围外的人不应再有")
	}
	code, body, _ = dsh["bob"].do("GET", "/api/dsh/skills/expense-policy", nil, nil)
	if code != 404 {
		t.Fatalf("范围外读正文应 404：%d", code)
	}
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", expenseID), map[string]any{"delivery": "auto"}, nil)
	expect(t, "作者不能设自动生效", code, 403, body)
	code, body = bob.do("GET", "/api/hub", nil, nil)
	expect(t, "bob 看能力中心", code, 200, body)
	if strings.Contains(string(body), "expense-policy") {
		t.Fatal("范围外的能力不应出现在 bob 的能力中心")
	}

	// 新版本：通过后线上内容更新；使用次数记录了
	code, body = alice.do("POST", fmt.Sprintf("/api/hub/%d/versions", expenseID), map[string]any{"spec": map[string]any{
		"title": "报销制度", "description": "差旅与报销标准", "body": "住宿每晚 1500 元以内。"}}, nil)
	expect(t, "新版本", code, 201, body)
	code, body = admin.do("POST", "/api/admin/hub/review", map[string]any{"asset_id": expenseID, "version": 2, "action": "reject"}, nil)
	expect(t, "驳回要写原因", code, 400, body)
	code, body = admin.do("POST", "/api/admin/hub/review", map[string]any{"asset_id": expenseID, "version": 2, "action": "approve"}, nil)
	expect(t, "通过新版本", code, 204, body)
	_, body, _ = dsh["alice"].do("GET", "/api/dsh/skills/expense-policy", nil, nil)
	if !strings.Contains(string(body), "1500") {
		t.Fatalf("应下发新版本：%s", body)
	}
	code, body = alice.do("GET", fmt.Sprintf("/api/hub/%d", expenseID), nil, nil)
	expect(t, "详情", code, 200, body)
	det := decodeAs[struct {
		Versions []any
	}](t, body)
	if len(det.Versions) != 2 {
		t.Fatalf("作者应看到两个版本：%s", body)
	}

	// ---- 提示词：管理员直接上架并设为全员自动生效 ----
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "prompt", "slug": "weekly-report", "delivery": "auto",
		"spec": map[string]any{"title": "周报", "description": "把本周工作整理成周报", "body": "按项目整理本周进展。"}}, nil)
	expect(t, "提示词", code, 201, body)
	c = dsh["bob"].catalog()
	if !slices.Equal(c.skillNames(), []string{"weekly-report"}) || c.Skills[0].ModelInvocable || !c.Skills[0].UserInvocable {
		t.Fatalf("提示词应自动下发且只给人调用：%+v", c)
	}

	// ---- 企业连接：MCP ----
	up := newFakeMCP(t)
	code, body = alice.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": "crm", "spec": map[string]any{"title": "CRM", "type": "mcp", "url": up.URL}}, nil)
	expect(t, "员工不能接入连接", code, 403, body)
	connSpec := map[string]any{"title": "CRM", "description": "客户与合同", "type": "mcp", "url": up.URL, "auth": map[string]any{"type": "bearer"}}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"spec": connSpec, "secret": "wrong"}, nil)
	expect(t, "试连", code, 200, body)
	if decodeAs[struct{ OK bool }](t, body).OK {
		t.Fatal("错误凭据的试连应失败")
	}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"spec": connSpec, "secret": "up-secret"}, nil)
	expect(t, "试连", code, 200, body)
	if !decodeAs[struct{ OK bool }](t, body).OK {
		t.Fatalf("试连应成功：%s", body)
	}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": "crm", "delivery": "auto", "spec": connSpec, "secret": "up-secret"}, nil)
	expect(t, "接入 CRM", code, 201, body)
	if strings.Contains(string(body), "up-secret") || decodeAs[struct{ Tools int }](t, body).Tools != 3 {
		t.Fatalf("应同步到 3 个工具且不回显凭据：%s", body)
	}
	crmID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID

	// 复用已存凭据试连时，只能测这个连接自己：不能把企业凭据发到请求里换的地址
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("不应把已保存的连接凭据发到别的地址")
	}))
	defer attacker.Close()
	badSpec := map[string]any{"title": "CRM", "type": "mcp", "url": attacker.URL, "auth": map[string]any{"type": "bearer"}}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"id": crmID, "spec": badSpec}, nil)
	expect(t, "复用已存凭据试连别的地址", code, 200, body)
	if decodeAs[struct{ OK bool }](t, body).OK || !strings.Contains(string(body), "已保存的凭据只能用于原来的地址") {
		t.Fatalf("已存凭据不应能发到别的地址：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"id": crmID, "spec": connSpec}, nil)
	expect(t, "复用已存凭据试连原地址", code, 200, body)
	if !decodeAs[struct{ OK bool }](t, body).OK {
		t.Fatalf("原地址应能复用已存凭据：%s", body)
	}

	init := dsh["bob"].rpc("initialize", map[string]any{"protocolVersion": "2025-11-25"})
	if init["result"].(map[string]any)["protocolVersion"] != "2025-11-25" || !strings.Contains(fmt.Sprint(init), "CRM") {
		t.Fatalf("initialize：%v", init)
	}
	if got := dsh["bob"].toolNames(); !slices.Equal(got, []string{"crm__export", "crm__lookup", "crm__update"}) {
		t.Fatalf("bob 的工具：%v", got)
	}
	// 关掉 export；update 只给审核员角色
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/export", crmID), map[string]any{"enabled": false}, nil)
	expect(t, "关工具", code, 204, body)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "sales", "name": "销售", "permissions": []string{"portal.view"}}, nil)
	expect(t, "建角色", code, 201, body)
	sales := decodeAs[store.Role](t, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/update", crmID), map[string]any{"enabled": true, "roles": []int64{sales.ID}}, nil)
	expect(t, "限定角色", code, 204, body)
	if got := dsh["bob"].toolNames(); !slices.Equal(got, []string{"crm__lookup"}) {
		t.Fatalf("bob 只应剩 lookup：%v", got)
	}
	res := dsh["bob"].rpc("tools/call", map[string]any{"name": "crm__lookup", "arguments": map[string]any{"q": "ACME"}})
	if !strings.Contains(fmt.Sprint(res["result"]), "CRM says hi") || up.calls.Load() != 1 {
		t.Fatalf("调用应转发到上游：%v", res)
	}
	res = dsh["bob"].rpc("tools/call", map[string]any{"name": "crm__update", "arguments": map[string]any{}})
	if res["error"] == nil || up.calls.Load() != 1 {
		t.Fatalf("没权限的工具不能调用：%v", res)
	}
	// 重新同步不丢管理员的设置
	code, body = admin.do("POST", fmt.Sprintf("/api/admin/connections/%d/sync", crmID), nil, nil)
	expect(t, "重新同步", code, 200, body)
	if got := dsh["bob"].toolNames(); !slices.Equal(got, []string{"crm__lookup"}) {
		t.Fatalf("同步后应保留工具设置：%v", got)
	}
	code, body = admin.do("GET", fmt.Sprintf("/api/hub/%d", crmID), nil, nil)
	expect(t, "连接详情", code, 200, body)
	if !strings.Contains(string(body), `"calls":[{"tool":"lookup","username":"bob","ok":true`) {
		t.Fatalf("应记录调用：%s", body)
	}

	// 事件流：改设置后推 tools/list_changed
	req, _ := http.NewRequest("GET", ts.URL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+dsh["bob"].token)
	req.Header.Set("Accept", "text/event-stream")
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(sctx))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("打开事件流：%v %v", err, resp)
	}
	got := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "notifications/tools/list_changed") {
				got <- true
				return
			}
		}
		got <- false
	}()
	time.Sleep(100 * time.Millisecond)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/connections/%d/tools/export", crmID), map[string]any{"enabled": true}, nil)
	expect(t, "开工具", code, 204, body)
	select {
	case ok := <-got:
		if !ok {
			t.Fatal("事件流结束了也没收到 list_changed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到 tools/list_changed")
	}
	resp.Body.Close()

	// ---- OpenAPI 连接：只读接口默认启用，写操作默认关闭 ----
	var gotKey atomic.Value
	rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey.Store(r.Header.Get("X-API-Key"))
		_, _ = fmt.Fprintf(w, `{"path":%q,"q":%q}`, r.URL.Path, r.URL.RawQuery)
	}))
	defer rest.Close()
	doc := `{"openapi":"3.0.0","info":{"title":"OA"},"paths":{"/leave/{id}":{"get":{"operationId":"getLeave","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}]},"delete":{"operationId":"cancelLeave","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}]}}}}`
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": "oa", "delivery": "auto", "scope": map[string]any{"departments": []string{"财务部"}},
		"spec": map[string]any{"title": "OA", "type": "openapi", "url": rest.URL + "/v1", "openapi": doc, "auth": map[string]any{"type": "header", "header": "X-API-Key"}}, "secret": "oa-key"}, nil)
	expect(t, "接入 OA", code, 201, body)
	if got := dsh["bob"].toolNames(); !slices.Equal(got, []string{"crm__export", "crm__lookup", "oa__getLeave"}) {
		t.Fatalf("bob 的工具（含 OA 的只读接口）：%v", got)
	}
	if got := dsh["alice"].toolNames(); slices.Contains(got, "oa__getLeave") {
		t.Fatal("OA 只给财务部")
	}
	res = dsh["bob"].rpc("tools/call", map[string]any{"name": "oa__getLeave", "arguments": map[string]any{"id": "42"}})
	if !strings.Contains(fmt.Sprint(res["result"]), `/v1/leave/42`) || gotKey.Load() != "oa-key" {
		t.Fatalf("OpenAPI 调用：%v key=%v", res, gotKey.Load())
	}

	// ---- 微应用 + 插件必装 ----
	if err := st.EnsureUpstreamPackage(ctx, &store.NpmPackage{Name: "dsh-ledger", IsPlugin: true, Title: "Ledger", DistTags: map[string]string{"latest": "1.0.0"}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := st.InsertMirroredVersion(ctx, &store.NpmVersion{Name: "dsh-ledger", Version: "1.0.0", Manifest: json.RawMessage(`{}`), Integrity: "sha512-x", Shasum: strings.Repeat("0", 40), Size: 1, BlobKey: "k", IsPlugin: true, Status: store.StatusPublished, PublishedAt: &now}); err != nil {
		t.Fatal(err)
	}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "expense-assistant", "delivery": "auto",
		"spec": map[string]any{"title": "报销助手", "description": "帮你判断能不能报", "persona": "你是报销助手。", "icon": " 🧾 ", "launch": " resume ",
			"starters": []string{"这张发票能报吗？"}, "skills": []string{"expense-policy"}, "connections": []string{"crm"}, "plugins": []string{"dsh-ledger"}}}, nil)
	expect(t, "微应用", code, 201, body)
	appID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID
	code, body = admin.do("GET", fmt.Sprintf("/api/hub/%d", appID), nil, nil)
	expect(t, "读取微应用详情", code, 200, body)
	savedApp := decodeAs[struct {
		Asset struct {
			Spec struct {
				Icon   string `json:"icon"`
				Launch string `json:"launch"`
			}
		}
	}](t, body)
	if savedApp.Asset.Spec.Icon != "🧾" || savedApp.Asset.Spec.Launch != "resume" {
		t.Fatalf("微应用图标与打开方式应去空格并保存：%s", body)
	}
	for _, tc := range []struct{ name, icon, launch string }{
		{"图标过长", strings.Repeat("🧾", 9), "new"},
		{"打开方式非法", "🧾", "later"},
	} {
		code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "invalid-app", "spec": map[string]any{
			"title": "非法微应用", "description": "校验", "persona": "校验", "icon": tc.icon, "launch": tc.launch}}, nil)
		expect(t, tc.name, code, 400, body)
	}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "alpha-app", "delivery": "auto", "spec": map[string]any{
		"title": "A 先排", "description": "验证默认打开方式", "persona": "你是测试助手。"}}, nil)
	expect(t, "默认打开方式的微应用", code, 201, body)
	c = dsh["alice"].catalog()
	if !slices.Contains(c.skillNames(), "expense-assistant") || len(c.Required) != 1 || c.Required[0].Name != "dsh-ledger" || c.Required[0].Reason != "app:expense-assistant" {
		t.Fatalf("微应用与其依赖的插件：%+v", c)
	}
	if len(c.Apps) != 2 || c.Apps[0].Name != "alpha-app" || c.Apps[1].Name != "expense-assistant" {
		t.Fatalf("微应用目录应按标题排序：%+v", c.Apps)
	}
	if c.Apps[0].Launch != "new" || c.Apps[0].Starters == nil {
		t.Fatalf("默认打开方式应为 new，空示例问题应为 []：%+v", c.Apps[0])
	}
	app := c.Apps[1]
	if app.Title != "报销助手" || app.Description != "帮你判断能不能报" || app.Icon != "🧾" || app.Launch != "resume" || app.Version != 1 || len(app.Missing) != 0 || !slices.Equal(app.Starters, []string{"这张发票能报吗？"}) {
		t.Fatalf("微应用目录内容不对：%+v", app)
	}
	legacySkill := false
	for _, sk := range c.Skills {
		if sk.Name == "expense-assistant" {
			legacySkill = sk.Title == app.Title && sk.Description == app.Description && sk.Kind == "app" && !sk.ModelInvocable && sk.UserInvocable && sk.Version == 1
		}
	}
	if !legacySkill {
		t.Fatalf("旧版插件使用的 skills 条目不应改变：%+v", c.Skills)
	}
	oldRevision := c.Revision
	code, body = admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", appID), map[string]any{"spec": map[string]any{
		"title": "报销助手", "description": "帮你判断能不能报", "persona": "你是报销助手。", "icon": "💰", "launch": "resume",
		"starters": []string{"这张发票能报吗？"}, "skills": []string{"expense-policy"}, "connections": []string{"crm"}, "plugins": []string{"dsh-ledger"}}}, nil)
	expect(t, "只改图标的新版本", code, 201, body)
	updated := dsh["alice"].catalog()
	if updated.Revision == oldRevision || len(updated.Apps) != 2 || updated.Apps[1].Icon != "💰" || updated.Apps[1].Version != 2 {
		t.Fatalf("改图标后目录与 ETag 应变化：old=%s new=%+v", oldRevision, updated)
	}
	code, body, header := dsh["alice"].do("GET", "/api/dsh/catalog", nil, map[string]string{"If-None-Match": `"` + oldRevision + `"`})
	expect(t, "旧 ETag 失效", code, 200, body)
	if header.Get("ETag") != `"`+updated.Revision+`"` {
		t.Fatalf("ETag 应使用新目录修订号：%q", header.Get("ETag"))
	}
	bobCatalog := dsh["bob"].catalog()
	if len(bobCatalog.Apps) != 2 || !slices.Equal(bobCatalog.Apps[1].Missing, []string{"expense-policy"}) {
		t.Fatalf("bob 的微应用目录应标记缺失依赖：%+v", bobCatalog.Apps)
	}
	_, body, _ = dsh["alice"].do("GET", "/api/dsh/skills/expense-assistant", nil, nil)
	content := decodeAs[struct{ Content string }](t, body).Content
	for _, want := range []string{"你是报销助手", "`expense-policy`", "mcp__acme__crm__", "这张发票能报吗"} {
		if !strings.Contains(content, want) {
			t.Fatalf("微应用正文缺少 %q：%s", want, content)
		}
	}
	// bob 不在技能的范围内：正文里要提醒不可用
	_, body, _ = dsh["bob"].do("GET", "/api/dsh/skills/expense-assistant", nil, nil)
	if !strings.Contains(decodeAs[struct{ Content string }](t, body).Content, "不要尝试调用：expense-policy") {
		t.Fatalf("bob 的微应用正文应提示 expense-policy 不可用：%s", body)
	}
	code, body = admin.do("PUT", "/api/admin/plugin-policies/dsh-ledger", map[string]any{"scope": map[string]any{"all": true}, "delivery": "auto"}, nil)
	expect(t, "插件设为自动安装", code, 200, body)
	if c := dsh["bob"].catalog(); len(c.Required) != 1 || c.Required[0].Reason != "policy" {
		t.Fatalf("插件策略：%+v", c)
	}
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", appID), map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}},
		"updated_at": scopeToken(t, admin, fmt.Sprintf("/api/hub/%d", appID))}, nil)
	expect(t, "收窄微应用范围", code, 200, body)
	outside := dsh["bob"].catalog()
	if slices.Contains(outside.skillNames(), "expense-assistant") || len(outside.Apps) != 1 || outside.Apps[0].Name != "alpha-app" {
		t.Fatalf("范围外的员工不应看到微应用：%+v", outside)
	}
	if inside := dsh["alice"].catalog(); len(inside.Apps) != 2 {
		t.Fatalf("范围内的员工仍应看到微应用：%+v", inside.Apps)
	}

	// ---- 接入令牌也能调模型网关 ----
	code, body = admin.do("POST", "/api/admin/providers", map[string]any{"name": "up", "base_url": llm.URL, "api_key": upstreamSecret, "priority": 1, "models": []string{"deepseek-chat"}}, nil)
	expect(t, "上游", code, 201, body)
	bobUser := decodeAs[struct{ Users []store.User }](t, mustGet(t, admin, "/api/admin/users")).Users
	for _, u := range bobUser {
		if u.Username == "bob" {
			code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d/quota", u.ID), map[string]any{"monthly_token_limit": 100000}, nil)
			expect(t, "额度", code, 200, body)
		}
	}
	code, body, _ = dsh["bob"].do("POST", "/v1/chat/completions", map[string]any{"model": "deepseek-chat", "messages": []map[string]string{{"role": "user", "content": "hi"}}}, nil)
	if code != 200 {
		t.Fatalf("接入令牌调模型：%d %s", code, body)
	}

	// ---- 接入插件 ----
	code, body = admin.do("POST", "/api/admin/connect/publish", map[string]any{}, nil)
	expect(t, "发布接入插件", code, 200, body)
	cs := decodeAs[struct{ Package, Version, Error string }](t, body)
	if cs.Package != "@acme/dsh-connect" || cs.Version != "1.0.1" || cs.Error != "" {
		t.Fatalf("接入插件：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/connect/publish", map[string]any{}, nil)
	expect(t, "内容没变不发新版本", code, 200, body)
	if decodeAs[struct{ Version string }](t, body).Version != "1.0.1" {
		t.Fatalf("内容没变不应发新版本：%s", body)
	}
	code, body, _ = dsh["bob"].do("GET", "/npm/@acme%2fdsh-connect", nil, nil)
	if code != 200 {
		t.Fatalf("接入插件的 packument：%d %s", code, body)
	}
	tarURL := decodeAs[packument](t, body).Versions["1.0.1"].Dist.Tarball
	code, body, _ = dsh["bob"].do("GET", strings.TrimPrefix(tarURL, ts.URL), nil, nil)
	if code != 200 {
		t.Fatalf("下载接入插件：%d", code)
	}
	files := untar(t, body)
	patch := files["package/cordis.patch.yml"]
	for _, want := range []string{"name: '@acme/dsh-connect'", "base: '" + ts.URL + "'", "serverName: 'acme'", "tokenRef: DSHEE_TOKEN",
		"X-DshEE-Client: 'dsh-connect/1.0.1'", "indexes:", "baseURL: '" + ts.URL + "/v1'", "model: 'deepseek-chat'"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("接入组合包缺少 %q：\n%s", want, patch)
		}
	}
	if strings.Contains(patch, "dsh-mcp-client") {
		t.Fatal("接入组合包不应再包含 dsh-mcp-client")
	}
	if !strings.Contains(files["package/package.json"], `"patch": "./cordis.patch.yml"`) || !strings.Contains(files["package/index.js"], "registerProvider") {
		t.Fatalf("package.json / index.js 不对：%v", files["package/package.json"])
	}
	// 改设置 → 自动发新版本（后台合并触发，这里直接调发布）
	set["connect"].(map[string]any)["market"] = false
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "关掉插件索引", code, 200, body)
	code, body = admin.do("POST", "/api/admin/connect/publish", map[string]any{}, nil)
	expect(t, "重新发布", code, 200, body)
	if decodeAs[struct{ Version string }](t, body).Version != "1.0.2" {
		t.Fatalf("内容变了应发 1.0.2：%s", body)
	}
	code, body = bob.do("GET", "/api/connect", nil, nil)
	expect(t, "接入信息", code, 200, body)
	if ci := decodeAs[struct {
		Ready   bool
		Package string
	}](t, body); !ci.Ready || ci.Package != "@acme/dsh-connect" {
		t.Fatalf("接入信息：%s", body)
	}
	resp, err = http.Get(ts.URL + "/setup.sh")
	if err != nil {
		t.Fatal(err)
	}
	sh, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{"CONNECT_PKG='@acme/dsh-connect'", "DSHEE_TOKEN=$TOKEN", "plugin --profile \"$profile\" add"} {
		if !strings.Contains(string(sh), want) {
			t.Fatalf("setup.sh 缺少 %q", want)
		}
	}

	// ---- 令牌与权限 ----
	code, _, _ = dshClient{t: t, base: ts.URL, token: "ee_npm_nope"}.do("GET", "/api/dsh/catalog", nil, nil)
	if code != 401 {
		t.Fatalf("无效令牌应 401：%d", code)
	}
	code, body = alice.do("DELETE", fmt.Sprintf("/api/hub/%d", crmID), nil, nil)
	expect(t, "员工不能删连接", code, 403, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", crmID), map[string]any{"status": "unpublished", "note": "停用"}, nil)
	expect(t, "下架连接", code, 200, body)
	if got := dsh["bob"].toolNames(); slices.Contains(got, "crm__lookup") {
		t.Fatalf("下架后不应再有 CRM 工具：%v", got)
	}
	code, body = admin.do("GET", "/api/admin/audit?action=asset.", nil, nil)
	expect(t, "审计", code, 200, body)
	if !strings.Contains(string(body), "connection:crm") {
		t.Fatalf("审计日志应记录能力操作：%s", body)
	}
}

func TestDshCatalogAppMissingAfterDependency(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "app-dependency-order", nil)
	f.token(t, u.ID, "ee_at_dep_order", []string{rbac.ScopeHub}, nil, nil)
	code, body := f.admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "app-before-skill", "delivery": "auto", "spec": map[string]any{
		"title": "先建应用", "description": "验证依赖顺序", "persona": "你是测试助手。", "skills": []string{"later-skill"}}}, nil)
	expect(t, "先建依赖技能的微应用", code, 201, body)
	code, body = f.admin.do("POST", "/api/hub", map[string]any{"kind": "skill", "slug": "later-skill", "delivery": "auto", "spec": map[string]any{
		"title": "后建技能", "description": "验证依赖顺序", "body": "后上线的技能正文。"}}, nil)
	expect(t, "后建并上线技能", code, 201, body)
	c := (dshClient{t: t, base: f.ts.URL, token: "ee_at_dep_order"}).catalog()
	if len(c.Apps) != 1 || len(c.Apps[0].Missing) != 0 {
		t.Fatalf("目录建完后依赖技能已可用，不应误报缺失：%+v", c.Apps)
	}
}

func mustGet(t *testing.T, c *client, path string) []byte {
	t.Helper()
	code, body := c.do("GET", path, nil, nil)
	expect(t, path, code, 200, body)
	return body
}

func untar(t *testing.T, tgz []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}
