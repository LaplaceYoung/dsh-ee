package api_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 「用 / 造 / 管」：自己用免审核、扩大范围走审核、员工菜单里没有管理入口。
func TestBuildSelfUse(t *testing.T) {
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

	users := map[string]*client{}
	ids := map[string]int64{}
	dsh := map[string]dshClient{}
	for _, u := range []struct{ name, dept string }{{"alice", "销售部"}, {"bob", "销售部"}, {"carol", "财务部"}} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": u.name, "password": u.name + "-password-1", "department": u.dept}, nil)
		expect(t, "建用户", code, 201, body)
		ids[u.name] = decodeAs[struct{ ID int64 }](t, body).ID
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": u.name, "password": u.name + "-password-1"}, nil)
		expect(t, "登录", code, 200, body)
		code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
		expect(t, "接入令牌", code, 201, body)
		users[u.name] = c
		dsh[u.name] = dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	alice, bob := users["alice"], users["bob"]
	appNames := func(c catalog) []string {
		var out []string
		for _, a := range c.Apps {
			out = append(out, a.Name)
		}
		return out
	}
	app := func(slug string, scope map[string]any) map[string]any {
		return map[string]any{"kind": "app", "slug": slug, "scope": scope, "spec": map[string]any{
			"title": "拜访助手 " + slug, "description": "整理客户拜访要点", "persona": "你是销售助理。"}}
	}
	type created struct {
		Asset  store.Asset
		Status string
	}

	// ---- 范围只有本人 → 直接发布，下发方式固定自选 ----
	code, body = alice.do("POST", "/api/hub", app("visit-brief", map[string]any{"users": []int64{ids["alice"]}}), nil)
	expect(t, "自己用的应用", code, 201, body)
	mine := decodeAs[created](t, body)
	if mine.Status != store.StatusPublished || mine.Asset.Delivery != store.DeliveryOptional {
		t.Fatalf("自己用应免审核、自选：%s", body)
	}
	if got := appNames(dsh["alice"].catalog()); !slices.Contains(got, "visit-brief") {
		t.Fatalf("作者的 DSH 里应能打开：%v", got)
	}
	for _, other := range []string{"bob", "carol"} {
		if got := appNames(dsh[other].catalog()); slices.Contains(got, "visit-brief") {
			t.Fatalf("%s 不应看到别人自己用的应用：%v", other, got)
		}
	}
	code, body = bob.do("GET", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), nil, nil)
	expect(t, "别人看不到详情", code, 404, body)

	// 「自己用」的新版本同样免审核
	code, body = alice.do("POST", fmt.Sprintf("/api/hub/%d/versions", mine.Asset.ID), map[string]any{"spec": map[string]any{
		"title": "拜访助手", "description": "整理客户拜访要点（改）", "persona": "你是销售助理。"}}, nil)
	expect(t, "自己用的新版本", code, 201, body)
	if decodeAs[struct{ Status string }](t, body).Status != store.StatusPublished {
		t.Fatalf("自己用的新版本应直接生效：%s", body)
	}

	// ---- 范围里有任何部门、角色或别人 → 待审核 ----
	for i, sc := range []map[string]any{
		{"departments": []string{"销售部"}},
		{"users": []int64{ids["alice"]}, "departments": []string{"销售部"}},
		{"users": []int64{ids["alice"]}, "roles": []int64{2}},
		{"users": []int64{ids["alice"], ids["bob"]}},
		{"all": true},
	} {
		code, body = alice.do("POST", "/api/hub", app(fmt.Sprintf("shared-%d", i), sc), nil)
		expect(t, "分享的应用", code, 201, body)
		if decodeAs[created](t, body).Status != store.StatusPending {
			t.Fatalf("范围 %v 应待审核：%s", sc, body)
		}
	}

	// ---- 改范围即提交审核：通过前作者照常能用，别人看不到 ----
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), map[string]any{"scope": map[string]any{"departments": []string{"销售部"}},
		"updated_at": scopeToken(t, alice, fmt.Sprintf("/api/hub/%d", mine.Asset.ID))}, nil)
	expect(t, "申请扩大范围", code, 200, body)
	patched := decodeAs[store.Asset](t, body)
	if patched.PendingScope == nil || !patched.Scope.OnlyUser(ids["alice"]) {
		t.Fatalf("扩大的范围应进审核、现有范围不变：%s", body)
	}
	if got := appNames(dsh["bob"].catalog()); slices.Contains(got, "visit-brief") {
		t.Fatalf("审核通过前 bob 不应看到：%v", got)
	}
	if got := appNames(dsh["alice"].catalog()); !slices.Contains(got, "visit-brief") {
		t.Fatalf("审核期间作者照常能用：%v", got)
	}
	code, body = alice.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": mine.Asset.ID, "action": "approve"}, nil)
	expect(t, "员工不能审核范围", code, 403, body)
	code, body = admin.do("GET", "/api/admin/hub/review", nil, nil)
	expect(t, "审核队列", code, 200, body)
	queue := decodeAs[struct {
		Scopes []struct {
			Asset     store.Asset
			Requested store.Scope
			Publisher string
		}
	}](t, body)
	if len(queue.Scopes) != 1 || queue.Scopes[0].Asset.ID != mine.Asset.ID || queue.Scopes[0].Publisher != "alice" || !slices.Equal(queue.Scopes[0].Requested.Departments, []string{"销售部"}) {
		t.Fatalf("审核队列里应有范围申请：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": mine.Asset.ID, "action": "reject"}, nil)
	expect(t, "驳回要写原因", code, 400, body)
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": mine.Asset.ID, "action": "approve"}, nil)
	expect(t, "通过范围申请", code, 204, body)
	code, body = bob.do("POST", fmt.Sprintf("/api/hub/%d/subscription", mine.Asset.ID), nil, nil)
	expect(t, "bob 启用", code, 204, body)
	if got := appNames(dsh["bob"].catalog()); !slices.Contains(got, "visit-brief") {
		t.Fatalf("通过后同部门的 bob 能用：%v", got)
	}
	if got := appNames(dsh["carol"].catalog()); slices.Contains(got, "visit-brief") {
		t.Fatalf("范围外的 carol 仍看不到：%v", got)
	}
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": mine.Asset.ID, "action": "approve"}, nil)
	expect(t, "没有在审的申请", code, 409, body)

	// 再扩大到全员 → 驳回：范围不变，作者看到原因
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), map[string]any{"scope": map[string]any{"all": true},
		"updated_at": scopeToken(t, alice, fmt.Sprintf("/api/hub/%d", mine.Asset.ID))}, nil)
	expect(t, "申请全员", code, 200, body)
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": mine.Asset.ID, "action": "reject", "note": "先在部门里试一个月"}, nil)
	expect(t, "驳回范围申请", code, 204, body)
	code, body = alice.do("GET", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), nil, nil)
	expect(t, "作者看详情", code, 200, body)
	det := decodeAs[struct{ Asset store.Asset }](t, body)
	if det.Asset.PendingScope != nil || det.Asset.ScopeNote != "先在部门里试一个月" || det.Asset.Scope.All {
		t.Fatalf("驳回后范围不变并带原因：%s", body)
	}

	// 收窄范围直接生效，并作废在审的申请
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), map[string]any{"scope": map[string]any{"all": true},
		"updated_at": scopeToken(t, alice, fmt.Sprintf("/api/hub/%d", mine.Asset.ID))}, nil)
	expect(t, "又申请全员", code, 200, body)
	code, body = alice.do("PATCH", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), map[string]any{"scope": map[string]any{"users": []int64{ids["alice"]}},
		"updated_at": scopeToken(t, alice, fmt.Sprintf("/api/hub/%d", mine.Asset.ID))}, nil)
	expect(t, "收回到自己用", code, 200, body)
	back := decodeAs[store.Asset](t, body)
	if back.PendingScope != nil || !back.Scope.OnlyUser(ids["alice"]) {
		t.Fatalf("收窄应直接生效并撤销申请：%s", body)
	}
	if got := appNames(dsh["bob"].catalog()); slices.Contains(got, "visit-brief") {
		t.Fatalf("收回后 bob 不应再有：%v", got)
	}

	// 管理员改范围直接生效
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", mine.Asset.ID), map[string]any{"scope": map[string]any{"all": true},
		"updated_at": scopeToken(t, admin, fmt.Sprintf("/api/hub/%d", mine.Asset.ID))}, nil)
	expect(t, "管理员改范围", code, 200, body)
	if a := decodeAs[store.Asset](t, body); !a.Scope.All || a.PendingScope != nil {
		t.Fatalf("管理员改范围不用审核：%s", body)
	}

	// 只有管理员能把应用设为样板
	code, body = alice.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "tpl-try", "scope": map[string]any{"users": []int64{ids["alice"]}},
		"spec": map[string]any{"title": "样板", "description": "试试", "persona": "你好", "template": true}}, nil)
	expect(t, "员工带样板标记", code, 201, body)
	code, body = alice.do("GET", fmt.Sprintf("/api/hub/%d", decodeAs[created](t, body).Asset.ID), nil, nil)
	if strings.Contains(string(body), `"template":true`) {
		t.Fatalf("员工不能设样板：%s", body)
	}

	// ---- 菜单：员工只看到「用 / 造 / 接入 / 开发」，没有管理入口，也没有「插件」 ----
	code, body = alice.do("GET", "/api/me/menus", nil, nil)
	expect(t, "员工菜单", code, 200, body)
	menus := decodeAs[struct{ Menus []store.Menu }](t, body).Menus
	var groups, visible []string
	byID := map[int64]store.Menu{}
	for _, m := range menus {
		byID[m.ID] = m
		if m.Kind == "group" {
			groups = append(groups, m.Title)
		}
	}
	for _, m := range menus {
		if m.Kind == "group" || !m.Visible || (m.ParentID != nil && !byID[*m.ParentID].Visible) {
			continue
		}
		visible = append(visible, m.Title)
		if strings.HasPrefix(m.Path, "/admin") {
			t.Fatalf("员工不应看到管理页面：%+v", m)
		}
		if strings.Contains(m.Title, "插件") {
			t.Fatalf("员工侧边栏不应出现「插件」：%+v", m)
		}
	}
	if !slices.Equal(groups, []string{"用", "造", "接入", "开发"}) {
		t.Fatalf("员工的分组：%v", groups)
	}
	if len(visible) == 0 || visible[0] != "企业应用" {
		t.Fatalf("默认首页应是企业应用：%v", visible)
	}
	code, body = admin.do("GET", "/api/me/menus", nil, nil)
	expect(t, "管理员菜单", code, 200, body)
	if !strings.Contains(string(body), `"title":"运营"`) {
		t.Fatalf("管理员应看到「运营」：%s", body)
	}
}
