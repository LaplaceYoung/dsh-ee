package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 网页版应用（M14）：开放网页版才能打开；按组件位置取数，连接、知识库、自带的表都按员工权限执行。
func TestWebApp(t *testing.T) {
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

	rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/summary":
			_, _ = w.Write([]byte(`{"total":128400}`))
		default:
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}
	}))
	defer rest.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "bob-password-1", "department": "研发部"}, nil)
	expect(t, "建用户", code, 201, body)
	bob := newClient(t, ts.URL)
	code, body = bob.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-1"}, nil)
	expect(t, "bob 登录", code, 200, body)

	doc := `{"openapi":"3.0.0","info":{"title":"Rows"},"paths":{"/summary":{"get":{"operationId":"summary"}},"/flaky":{"get":{"operationId":"flaky"}}}}`
	for _, c := range []struct {
		slug  string
		scope map[string]any
	}{{"rows", map[string]any{"all": true}}, {"secret", map[string]any{"departments": []string{"财务部"}}}} {
		code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": c.slug, "delivery": "auto", "scope": c.scope,
			"spec": map[string]any{"title": c.slug, "type": "openapi", "url": rest.URL, "openapi": doc, "auth": map[string]any{"type": "none"}}}, nil)
		expect(t, "接入连接 "+c.slug, code, 201, body)
	}

	widgets := []map[string]any{
		{"type": "metric", "title": "总额", "source": map[string]any{"connection": "rows", "tool": "summary"}, "field": "total"},
		{"type": "metric", "title": "机密", "source": map[string]any{"connection": "secret", "tool": "summary"}, "field": "total"},
		{"type": "metric", "title": "坏的", "source": map[string]any{"connection": "rows", "tool": "flaky"}, "field": "total"},
		{"type": "table", "title": "我的报名", "source": map[string]any{"table": "signups"}, "rows": "items", "columns": []map[string]any{{"field": "session", "title": "场次"}}},
		{"type": "table", "title": "知识库", "source": map[string]any{"connection": "kb", "tool": "list"}, "rows": "items", "columns": []map[string]any{{"field": "title", "title": "名称"}}},
		{"type": "form", "title": "报名", "submit": map[string]any{"label": "报名", "table": "signups"}},
		{"type": "board", "title": "AI 看板"},
	}
	spec := func(web bool) map[string]any {
		return map[string]any{
			"title": "网页版测试", "description": "x", "persona": "x", "connections": []string{"rows", "secret"}, "web": web,
			"tables":    []map[string]any{{"name": "signups", "title": "报名", "fields": []map[string]any{{"name": "session", "title": "场次", "kind": "text", "required": true}}}},
			"workbench": map[string]any{"panes": []map[string]any{{"id": "main", "type": "panel", "title": "面板", "widgets": widgets}}},
		}
	}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "no-wb", "spec": map[string]any{"title": "x", "description": "x", "persona": "x", "web": true}}, nil)
	expect(t, "没有工作台不能开网页版", code, 400, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "board-app", "delivery": "auto", "spec": spec(false)}, nil)
	expect(t, "建没开网页版的应用", code, 201, body)
	code, body = bob.do("GET", "/api/me/apps/board-app/web", nil, nil)
	expect(t, "没开网页版：员工打不开", code, 404, body)
	code, body = admin.do("GET", "/api/me/apps/board-app/web", nil, nil)
	expect(t, "没开网页版：作者能预览", code, 200, body)

	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "web-app", "delivery": "auto", "spec": spec(true)}, nil)
	expect(t, "建网页版应用", code, 201, body)
	code, body = bob.do("GET", "/api/me/apps/web-app/web", nil, nil)
	expect(t, "员工打开网页版", code, 200, body)
	var info struct {
		Title     string `json:"title"`
		Tables    bool   `json:"tables"`
		Admin     bool   `json:"admin"`
		Workbench struct {
			Panes []struct {
				Widgets []map[string]any `json:"widgets"`
			} `json:"panes"`
		} `json:"workbench"`
	}
	_ = json.Unmarshal(body, &info)
	if info.Title != "网页版测试" || !info.Tables || info.Admin || len(info.Workbench.Panes[0].Widgets) != 7 {
		t.Fatalf("网页版信息不对：%s", body)
	}

	source := func(c *client, widget int) map[string]any {
		t.Helper()
		code, body := c.do("POST", "/api/me/apps/web-app/source", map[string]any{"pane": "main", "widget": widget}, nil)
		expect(t, "取数", code, 200, body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		return m
	}
	if m := source(bob, 0); m["value"].(map[string]any)["total"] != float64(128400) {
		t.Fatalf("连接的只读工具：%v", m)
	}
	if m := source(bob, 1); m["forbidden"] != true {
		t.Fatalf("员工没权限的连接应是无权限：%v", m)
	}
	if m := source(bob, 2); m["error"] == nil || !strings.Contains(m["error"].(string), "500") && !strings.Contains(m["error"].(string), "boom") {
		t.Fatalf("上游出错应给原因：%v", m)
	}
	if m := source(bob, 4); m["forbidden"] != true {
		t.Fatalf("没有能查的知识库应是无权限：%v", m)
	}
	// 表单写进表，表格读出来（只看到自己的）
	code, body = bob.do("POST", "/api/apps/web-app/data/signups", map[string]any{"row": map[string]any{"session": "上午"}}, nil)
	expect(t, "网页版表单写入", code, 201, body)
	code, body = admin.do("POST", "/api/apps/web-app/data/signups", map[string]any{"row": map[string]any{"session": "下午"}}, nil)
	expect(t, "作者也报一条", code, 201, body)
	if m := source(bob, 3); m["value"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("自带的表：员工只看到自己的：%v", m)
	}
	if m := source(admin, 3); m["value"].(map[string]any)["total"] != float64(2) {
		t.Fatalf("自带的表：作者看全部：%v", m)
	}
	// 没有数据源的组件、不存在的组件
	code, body = bob.do("POST", "/api/me/apps/web-app/source", map[string]any{"pane": "main", "widget": 6}, nil)
	expect(t, "看板没有数据源", code, 404, body)
	code, body = bob.do("POST", "/api/me/apps/web-app/source", map[string]any{"pane": "main", "widget": 99}, nil)
	expect(t, "不存在的组件", code, 404, body)

	// DSH 目录里标出开了网页版
	code, body = bob.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "接入令牌", code, 201, body)
	d := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	code, body, _ = d.do("GET", "/api/dsh/catalog", nil, nil)
	expect(t, "目录", code, 200, body)
	if !strings.Contains(string(body), `"name":"web-app"`) || !strings.Contains(string(body), `"web":true`) {
		t.Fatalf("目录里应标出网页版：%s", body)
	}

	// 不在范围内的人打不开
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "eve", "password": "eve-password-1", "department": "财务部"}, nil)
	expect(t, "建 eve", code, 201, body)
	webAppID := assetIDBySlug(t, admin, "web-app")
	code, body = admin.do("PATCH", "/api/hub/"+itoa(webAppID), map[string]any{"scope": map[string]any{"departments": []string{"研发部"}},
		"updated_at": scopeToken(t, admin, "/api/hub/"+itoa(webAppID))}, nil)
	expect(t, "缩小范围", code, 200, body)
	eve := newClient(t, ts.URL)
	code, body = eve.do("POST", "/api/auth/login", map[string]string{"username": "eve", "password": "eve-password-1"}, nil)
	expect(t, "eve 登录", code, 200, body)
	code, body = eve.do("GET", "/api/me/apps/web-app/web", nil, nil)
	expect(t, "范围外打不开", code, 404, body)
	code, body = eve.do("POST", "/api/me/apps/web-app/source", map[string]any{"pane": "main", "widget": 0}, nil)
	expect(t, "范围外不能取数", code, 404, body)
}

func assetIDBySlug(t *testing.T, c *client, slug string) int64 {
	t.Helper()
	code, body := c.do("GET", "/api/hub", nil, nil)
	expect(t, "能力列表", code, 200, body)
	var out struct {
		Assets []struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(body, &out)
	for _, a := range out.Assets {
		if a.Slug == slug {
			return a.ID
		}
	}
	t.Fatalf("没有 %s", slug)
	return 0
}
