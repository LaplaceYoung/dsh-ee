package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 员工在 DSH 里启用、停用「自选」微应用（/api/dsh/apps/{name}/subscription）。
func TestDshAppSubscription(t *testing.T) {
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

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "bob-password-1", "department": "财务部"}, nil)
	expect(t, "建用户", code, 201, body)
	bobWeb := newClient(t, ts.URL)
	code, body = bobWeb.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-1"}, nil)
	expect(t, "登录", code, 200, body)
	code, body = bobWeb.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "接入令牌", code, 201, body)
	bob := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}

	app := func(slug, delivery string, scope map[string]any) {
		t.Helper()
		req := map[string]any{"kind": "app", "slug": slug, "delivery": delivery, "spec": map[string]any{"title": slug, "description": "d", "persona": "你是 " + slug + "。", "icon": "🧾"}}
		if scope != nil {
			req["scope"] = scope
		}
		code, body := admin.do("POST", "/api/hub", req, nil)
		expect(t, "建微应用 "+slug, code, 201, body)
	}
	app("expense-pick", "optional", nil)
	app("always-on", "auto", nil)
	app("rd-only", "optional", map[string]any{"departments": []string{"研发效能部"}})
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "skill", "slug": "a-skill", "delivery": "optional", "spec": map[string]any{"title": "技能", "description": "d", "body": "b"}}, nil)
	expect(t, "建技能", code, 201, body)

	type entry struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Delivery string `json:"delivery"`
		Launch   string `json:"launch"`
	}
	read := func() (apps, available, skills []string, etag string) {
		t.Helper()
		code, raw, header := bob.do("GET", "/api/dsh/catalog", nil, nil)
		expect(t, "目录", code, 200, raw)
		var c struct {
			Apps      []entry `json:"apps"`
			Available []entry `json:"available_apps"`
			Skills    []entry `json:"skills"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		if c.Available == nil {
			t.Fatalf("available_apps 应是数组：%s", raw)
		}
		for _, e := range c.Apps {
			if e.Name == systemHomeApp { // 系统应用，home_test 单独测
				// 工作台是员工的「总助理」：再点一次回到原来的会话，不是每次新开一个（#252）
				if e.Launch != "resume" {
					t.Fatalf("工作台应当回到上次的会话（launch: resume）：%s", raw)
				}
				continue
			}
			apps = append(apps, e.Name)
			if want := map[string]string{"always-on": "auto", "expense-pick": "optional"}[e.Name]; e.Delivery != want {
				t.Fatalf("%s 的 delivery 应是 %s：%s", e.Name, want, raw)
			}
		}
		for _, e := range c.Available {
			available = append(available, e.Name)
			if e.Icon != "🧾" {
				t.Fatalf("可启用的应用应带展示信息：%s", raw)
			}
		}
		for _, e := range c.Skills {
			if e.Name == systemHomeApp {
				continue
			}
			skills = append(skills, e.Name)
		}
		return apps, available, skills, header.Get("ETag")
	}

	apps, available, skills, etag := read()
	if !slices.Equal(apps, []string{"always-on"}) || !slices.Equal(available, []string{"expense-pick"}) || slices.Contains(skills, "expense-pick") {
		t.Fatalf("启用前：apps=%v available=%v skills=%v", apps, available, skills)
	}
	if code, raw, _ := bob.do("GET", "/api/dsh/skills/expense-pick", nil, nil); code != 404 {
		t.Fatalf("没启用的微应用 /名字 应加载不到：%d %s", code, raw)
	}

	code, raw, _ := bob.do("POST", "/api/dsh/apps/expense-pick/subscription", nil, nil)
	expect(t, "启用", code, 200, raw)
	apps, available, skills, next := read()
	if !slices.Contains(apps, "expense-pick") || len(available) != 0 || !slices.Contains(skills, "expense-pick") || next == etag {
		t.Fatalf("启用后：apps=%v available=%v skills=%v etag 变了=%v", apps, available, skills, next != etag)
	}
	if code, raw, _ := bob.do("GET", "/api/dsh/skills/expense-pick", nil, nil); code != 200 {
		t.Fatalf("启用后 /名字 应能加载：%d %s", code, raw)
	}
	// 网页上同样显示为已启用（同一张订阅表）
	code, body = bobWeb.do("GET", "/api/hub", nil, nil)
	expect(t, "能力中心", code, 200, body)
	var hub struct {
		Assets []struct {
			Slug       string `json:"slug"`
			Subscribed bool   `json:"subscribed"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(body, &hub)
	if i := slices.IndexFunc(hub.Assets, func(a struct {
		Slug       string `json:"slug"`
		Subscribed bool   `json:"subscribed"`
	}) bool {
		return a.Slug == "expense-pick"
	}); i < 0 || !hub.Assets[i].Subscribed {
		t.Fatalf("网页上应显示已启用：%s", body)
	}

	code, raw, _ = bob.do("DELETE", "/api/dsh/apps/expense-pick/subscription", nil, nil)
	expect(t, "停用", code, 200, raw)
	apps, available, _, _ = read()
	if slices.Contains(apps, "expense-pick") || !slices.Equal(available, []string{"expense-pick"}) {
		t.Fatalf("停用后：apps=%v available=%v", apps, available)
	}

	for _, c := range []struct {
		what, method, name string
		want               int
	}{
		{"默认生效的不能停用", "DELETE", "always-on", 400},
		{"默认生效的不需要启用", "POST", "always-on", 400},
		{"范围外的微应用", "POST", "rd-only", 404},
		{"不是微应用", "POST", "a-skill", 404},
		{"不存在", "POST", "nope", 404},
	} {
		if code, raw, _ := bob.do(c.method, "/api/dsh/apps/"+c.name+"/subscription", nil, nil); code != c.want {
			t.Fatalf("%s：期望 %d，实际 %d %s", c.what, c.want, code, raw)
		}
	}
	if _, available, _, _ := read(); slices.Contains(available, "rd-only") {
		t.Fatal("范围外的微应用不应出现在可启用列表里")
	}

	// 没有 hub 范围的令牌不能启用
	code, body = bobWeb.do("POST", "/api/me/tokens", map[string]any{"name": "仅连接", "scopes": []string{"connections"}}, nil)
	expect(t, "建只有连接范围的令牌", code, 201, body)
	narrow := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	if code, raw, _ := narrow.do("POST", "/api/dsh/apps/expense-pick/subscription", nil, nil); code != 403 {
		t.Fatalf("没有 hub 范围的令牌应被拒绝：%d %s", code, raw)
	}
	if code, raw, _ := bob.do("POST", "/api/dsh/apps/expense-pick/subscription", nil, map[string]string{"Authorization": "Bearer ee_at_bogus"}); code != 401 {
		t.Fatalf("无效令牌应返回 401：%d %s", code, raw)
	}
}
