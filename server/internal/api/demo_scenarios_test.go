package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 示例场景：先只导入小团队「山海工作室」，再导入示例公司；两边互相看不到对方的应用，清除时全删。
func TestDemoScenarios(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	ts := httptest.NewUnstartedServer(nil)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), Addr: ts.Listener.Addr().String()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts.Config.Handler = srv.Handler()
	ts.Start()
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	type result struct {
		Scenario string
		Items    []struct {
			Kind, Slug, Result, Error string
		}
		Accounts []struct {
			Username, Password string
		}
		Guide []struct {
			Scenario, Username, App string
		}
	}
	type status struct {
		Imported, Present bool
		Scenarios         []struct {
			ID, Kind, Title   string
			Imported, Present bool
			People            int
		}
	}

	code, body = admin.do("POST", "/api/admin/demo?scenario=nope", nil, nil)
	expect(t, "没有的场景", code, 400, body)

	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "示例状态", code, 200, body)
	if s := decodeAs[status](t, body); len(s.Scenarios) != 2 || s.Scenarios[0].ID != "xinghe" || s.Scenarios[1].ID != "novel" || s.Scenarios[1].Kind != "team" || s.Imported {
		t.Fatalf("应列出两个还没导入的场景：%s", body)
	}

	// 只导入小团队
	code, body = admin.do("POST", "/api/admin/demo?scenario=novel", nil, nil)
	expect(t, "导入山海工作室", code, 200, body)
	novel := decodeAs[result](t, body)
	for _, it := range novel.Items {
		if it.Result != "created" {
			t.Fatalf("%s %s 没建成：%s %s", it.Kind, it.Slug, it.Result, it.Error)
		}
	}
	// 2 个库 + 5 项能力 + 4 个成员 + 1 个成员自己做的应用
	if novel.Scenario != "novel" || len(novel.Items) != 12 || len(novel.Accounts) != 4 || len(novel.Guide) != 2 || novel.Guide[0].Scenario != "novel" {
		t.Fatalf("山海工作室应建 12 项、4 个账号：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "示例状态", code, 200, body)
	if s := decodeAs[status](t, body); !s.Imported || s.Scenarios[0].Imported || !s.Scenarios[1].Imported {
		t.Fatalf("只有山海工作室算已导入：%s", body)
	}
	// 只有一个场景时不加隔离策略
	code, body = admin.do("GET", "/api/admin/hub/blocks", nil, nil)
	expect(t, "禁用策略", code, 200, body)
	if n := len(decodeAs[struct{ Blocks []any }](t, body).Blocks); n != 0 {
		t.Fatalf("只导入一个场景时不应有禁用策略：%s", body)
	}

	connect := func(username, password string) dshClient {
		c := newClient(t, ts.URL)
		code, body := c.do("POST", "/api/auth/login", map[string]string{"username": username, "password": password}, nil)
		expect(t, username+" 登录", code, 200, body)
		code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
		expect(t, username+" 的接入令牌", code, 201, body)
		return dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	appsOf := func(d dshClient) []string {
		var out []string
		for _, a := range d.catalog().Apps {
			out = append(out, a.Name)
			if len(a.Missing) > 0 {
				t.Fatalf("%s 缺能力：%v", a.Name, a.Missing)
			}
		}
		return out
	}

	// 主笔沈墨：设定问答、两位审稿专家、章节进度；审稿小组是自选的
	if novel.Accounts[0].Username != "demo.shen.mo" {
		t.Fatalf("账号顺序变了：%s", body)
	}
	shen := connect(novel.Accounts[0].Username, novel.Accounts[0].Password)
	apps := appsOf(shen)
	for _, want := range []string{"story-bible", "continuity-editor", "line-editor", "chapter-board"} {
		if !slices.Contains(apps, want) {
			t.Fatalf("沈墨的 DSH 里应有 %s：%v", want, apps)
		}
	}
	if slices.Contains(apps, "chapter-review") {
		t.Fatalf("自选的审稿小组没启用前不应下发：%v", apps)
	}
	if !slices.Contains(shen.catalog().skillNames(), "chapter-recap") {
		t.Fatalf("章节梗概技能应下发：%v", shen.catalog().skillNames())
	}
	// 设定集查得到引雾的规则；章节进度全员可见
	if _, isErr, text := shen.kbCall("search", map[string]any{"query": "引雾 三次"}); isErr || !strings.Contains(text, "世界观与引雾") {
		t.Fatalf("设定集里应查得到引雾的规则：%s", text)
	}
	if out, isErr, text := shen.appCall("rows", map[string]any{"app": "chapter-board", "table": "chapters"}); isErr || out["total"] != float64(8) || !strings.Contains(text, "返稿修改") {
		t.Fatalf("沈墨应看到全部 8 章的进度：%s", text)
	}
	if out, isErr, text := shen.appCall("rows", map[string]any{"app": "chapter-board", "table": "threads"}); isErr || out["total"] != float64(4) {
		t.Fatalf("伏笔清单全员可见：%s", text)
	}

	// 再导入示例公司：两个场景并存，互相看不到对方的应用
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "导入示例公司", code, 200, body)
	company := decodeAs[result](t, body)
	for _, it := range company.Items {
		if it.Result != "created" {
			t.Fatalf("%s %s 没建成：%s %s", it.Kind, it.Slug, it.Result, it.Error)
		}
	}
	if company.Scenario != "xinghe" || len(company.Accounts) != 10 {
		t.Fatalf("示例公司应建 10 个账号：%s", body)
	}
	apps = appsOf(shen)
	for _, hidden := range []string{"travel-expense", "onboarding-buddy", "team-outing", "contract-reviewer"} {
		if slices.Contains(apps, hidden) {
			t.Fatalf("写小说的沈墨不应看到示例公司的 %s：%v", hidden, apps)
		}
	}
	if !slices.Contains(apps, "story-bible") || slices.Contains(shen.catalog().skillNames(), "meeting-minutes") {
		t.Fatalf("沈墨应仍有自己的应用，没有示例公司的技能：%v %v", apps, shen.catalog().skillNames())
	}
	// 销售部的周航看不到工作室的应用，自己的照常
	zhou := connect(company.Accounts[4].Username, company.Accounts[4].Password)
	apps = appsOf(zhou)
	for _, hidden := range []string{"story-bible", "continuity-editor", "chapter-board"} {
		if slices.Contains(apps, hidden) {
			t.Fatalf("示例公司的周航不应看到山海工作室的 %s：%v", hidden, apps)
		}
	}
	if !slices.Contains(apps, "customer-brief") || slices.Contains(zhou.catalog().skillNames(), "chapter-recap") {
		t.Fatalf("周航应仍有客户拜访简报，没有工作室的技能：%v", apps)
	}
	// 重复导入不重复建隔离策略：研发部那条 + 两个场景各一条
	code, body = admin.do("POST", "/api/admin/demo?scenario=novel", nil, nil)
	expect(t, "重复导入山海工作室", code, 200, body)
	for _, it := range decodeAs[result](t, body).Items {
		if it.Result != "exists" {
			t.Fatalf("重复导入时 %s 应跳过：%s", it.Slug, it.Result)
		}
	}
	code, body = admin.do("GET", "/api/admin/hub/blocks", nil, nil)
	expect(t, "禁用策略", code, 200, body)
	if n := len(decodeAs[struct{ Blocks []any }](t, body).Blocks); n != 3 {
		t.Fatalf("应有 3 条示例禁用策略：%s", body)
	}

	// 清除：两个场景一起删
	code, body = admin.do("DELETE", "/api/admin/demo", nil, nil)
	expect(t, "清除示例", code, 200, body)
	if c := decodeAs[struct{ Assets, KBases, Users int }](t, body); c.Assets != 24 || c.KBases != 8 || c.Users != 14 {
		t.Fatalf("应删 24 项能力、8 个库、14 个账号：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "清除后的状态", code, 200, body)
	if s := decodeAs[status](t, body); s.Present || s.Imported || s.Scenarios[1].Present {
		t.Fatalf("清除后不应还有示例：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/hub/blocks", nil, nil)
	expect(t, "清除后的禁用策略", code, 200, body)
	if n := len(decodeAs[struct{ Blocks []any }](t, body).Blocks); n != 0 {
		t.Fatalf("清除后不应还有示例禁用策略：%s", body)
	}
}
