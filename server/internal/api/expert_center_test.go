package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 专家 · 技能 · 连接器（docs/expert-center.md）第一批：专家形态、分类与禁用策略对可用性的影响、
// DSH 目录的新字段、技能与连接的自选启用、权限查询。
func TestExpertCenter(t *testing.T) {
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
	bobID := decodeAs[struct{ ID int64 }](t, body).ID
	bobWeb := newClient(t, ts.URL)
	code, body = bobWeb.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-1"}, nil)
	expect(t, "登录", code, 200, body)
	code, body = bobWeb.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "接入令牌", code, 201, body)
	bob := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}

	create := func(what string, req map[string]any, want int) int64 {
		t.Helper()
		code, body := admin.do("POST", "/api/hub", req, nil)
		expect(t, what, code, want, body)
		if want != 201 {
			return 0
		}
		return decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	}
	expertSpec := func(extra map[string]any) map[string]any {
		sp := map[string]any{"title": "报销专家", "description": "帮你核对报销单", "persona": "你是财务报销专家。", "form": "expert",
			"expertise": []string{"报销", "差旅", "报销"}, "method": "先核对发票，再对照差旅标准。", "starters": []string{"帮我看看这张发票能不能报"}}
		for k, v := range extra {
			sp[k] = v
		}
		return sp
	}

	// ---- 专家形态的校验 ----
	create("专家不能带网页版", map[string]any{"kind": "app", "slug": "bad-expert", "spec": expertSpec(map[string]any{"web": true})}, 400)
	create("形态只能是 app / expert", map[string]any{"kind": "app", "slug": "bad-form", "spec": expertSpec(map[string]any{"form": "team"})}, 400)
	expertID := create("建专家", map[string]any{"kind": "app", "slug": "expense-expert", "delivery": "auto", "spec": expertSpec(nil)}, 201)
	skillID := create("建自选技能", map[string]any{"kind": "skill", "slug": "invoice-check", "delivery": "optional", "spec": map[string]any{"title": "发票核对", "description": "核对发票", "body": "步骤……"}}, 201)
	up := newFakeMCP(t)
	connID := create("建自选连接", map[string]any{"kind": "connection", "slug": "erp", "delivery": "optional",
		"spec": map[string]any{"title": "ERP", "type": "mcp", "url": up.URL, "auth": map[string]any{"type": "bearer"}}, "secret": "up-secret"}, 201)
	create("建要员工凭据的连接", map[string]any{"kind": "connection", "slug": "oa", "delivery": "auto",
		"spec": map[string]any{"title": "OA", "type": "mcp", "url": up.URL, "auth": map[string]any{"type": "bearer"},
			"user_auth": map[string]any{"mode": "user_secret", "header": "X-User-Token", "hint": "填你的 OA 令牌"}}, "secret": "up-secret"}, 201)

	type catalog struct {
		Revision string `json:"revision"`
		Apps     []struct {
			Name       string   `json:"name"`
			Form       string   `json:"form"`
			Expertise  []string `json:"expertise"`
			CategoryID int64    `json:"category_id"`
			Featured   bool     `json:"featured"`
		} `json:"apps"`
		Skills []struct {
			Name     string `json:"name"`
			Delivery string `json:"delivery"`
		} `json:"skills"`
		AvailableSkills []struct {
			Name    string `json:"name"`
			AssetID int64  `json:"asset_id"`
		} `json:"available_skills"`
		Connections []struct {
			Name     string `json:"name"`
			Enabled  bool   `json:"enabled"`
			Tools    int    `json:"tools"`
			Auth     string `json:"auth"`
			AuthMode string `json:"auth_mode"`
		} `json:"connections"`
		Categories []struct {
			ID    int64  `json:"id"`
			Group string `json:"group"`
			Name  string `json:"name"`
		} `json:"categories"`
	}
	read := func() catalog {
		t.Helper()
		code, raw, _ := bob.do("GET", "/api/dsh/catalog", nil, nil)
		expect(t, "目录", code, 200, raw)
		var c catalog
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		if c.AvailableSkills == nil || c.Connections == nil || c.Categories == nil {
			t.Fatalf("新字段应是数组：%s", raw)
		}
		return c
	}
	appNames := func(c catalog) []string {
		var out []string
		for _, a := range c.Apps {
			out = append(out, a.Name)
		}
		return out
	}
	skillNames := func(c catalog) []string {
		var out []string
		for _, s := range c.Skills {
			out = append(out, s.Name)
		}
		return out
	}

	// ---- 目录：专家、自选技能、连接器卡片 ----
	c := read()
	i := slices.IndexFunc(c.Apps, func(a struct {
		Name       string   `json:"name"`
		Form       string   `json:"form"`
		Expertise  []string `json:"expertise"`
		CategoryID int64    `json:"category_id"`
		Featured   bool     `json:"featured"`
	}) bool {
		return a.Name == "expense-expert"
	})
	if i < 0 || c.Apps[i].Form != "expert" || !slices.Equal(c.Apps[i].Expertise, []string{"报销", "差旅"}) {
		t.Fatalf("专家应在 apps 里，带 form 与去重后的擅长领域：%+v", c.Apps)
	}
	code, raw, _ := bob.do("GET", "/api/dsh/skills/expense-expert", nil, nil)
	expect(t, "召唤专家", code, 200, raw)
	if !strings.Contains(string(raw), "工作方法") || !strings.Contains(string(raw), "先核对发票") {
		t.Fatalf("专家的指令里应带工作方法：%s", raw)
	}
	if slices.Contains(skillNames(c), "invoice-check") || len(c.AvailableSkills) != 1 || c.AvailableSkills[0].Name != "invoice-check" || c.AvailableSkills[0].AssetID != skillID {
		t.Fatalf("自选技能启用前应只在 available_skills：skills=%v available=%+v", skillNames(c), c.AvailableSkills)
	}
	conns := map[string]int{}
	for i, cn := range c.Connections {
		conns[cn.Name] = i
	}
	if len(c.Connections) != 2 {
		t.Fatalf("应有两张连接器卡片：%+v", c.Connections)
	}
	if erp := c.Connections[conns["erp"]]; erp.Enabled || erp.Tools != 3 || erp.Auth != "ready" {
		t.Fatalf("自选连接启用前：%+v", erp)
	}
	if oa := c.Connections[conns["oa"]]; !oa.Enabled || oa.Auth != "needs_auth" || oa.AuthMode != "user_secret" {
		t.Fatalf("要员工凭据的连接应显示待授权：%+v", oa)
	}

	// ---- 技能与连接的自选启用 ----
	code, raw, _ = bob.do("POST", "/api/dsh/skills/invoice-check/subscription", nil, nil)
	expect(t, "启用技能", code, 200, raw)
	c = read()
	if !slices.Contains(skillNames(c), "invoice-check") || len(c.AvailableSkills) != 0 {
		t.Fatalf("启用技能后：skills=%v available=%+v", skillNames(c), c.AvailableSkills)
	}
	code, raw, _ = bob.do("POST", "/api/dsh/skills/expense-expert/subscription", nil, nil)
	expect(t, "技能接口不收专家", code, 404, raw)
	code, raw, _ = bob.do("DELETE", "/api/dsh/connections/oa/subscription", nil, nil)
	expect(t, "默认生效的连接不能停用", code, 400, raw)
	code, raw, _ = bob.do("POST", "/api/dsh/connections/erp/subscription", nil, nil)
	expect(t, "启用连接", code, 200, raw)
	if c = read(); !c.Connections[conns["erp"]].Enabled {
		t.Fatalf("启用连接后应生效：%+v", c.Connections)
	}
	code, raw, _ = bob.do("DELETE", "/api/dsh/skills/invoice-check/subscription", nil, nil)
	expect(t, "停用技能", code, 200, raw)

	// ---- 分类：范围收窄后，范围外的人看不到这个分类下的能力 ----
	code, body = admin.do("POST", "/api/admin/hub/categories", map[string]any{"group": "app", "name": "财务", "scope": map[string]any{"all": true}}, nil)
	expect(t, "建分类", code, 201, body)
	catID := decodeAs[struct{ ID int64 }](t, body).ID
	code, body = admin.do("POST", "/api/admin/hub/categories", map[string]any{"group": "app", "name": "财务"}, nil)
	expect(t, "同组重名", code, 409, body)
	code, body = admin.do("POST", "/api/admin/hub/categories", map[string]any{"group": "skill", "name": "财务"}, nil)
	expect(t, "不同组可以同名", code, 201, body)
	skillCatID := decodeAs[struct{ ID int64 }](t, body).ID
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", expertID), map[string]any{"category_id": skillCatID}, nil)
	expect(t, "分类分组要和能力对得上", code, 400, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", expertID), map[string]any{"category_id": catID, "featured": true}, nil)
	expect(t, "归类并设精选", code, 200, body)
	if got := decodeAs[struct {
		CategoryID int64  `json:"category_id"`
		Category   string `json:"category"`
		Featured   bool   `json:"featured"`
	}](t, body); got.CategoryID != catID || got.Category != "财务" || !got.Featured {
		t.Fatalf("归类后分类文本应同步：%s", body)
	}
	c = read()
	if i := slices.Index(appNames(c), "expense-expert"); i < 0 || c.Apps[i].CategoryID != catID || !c.Apps[i].Featured {
		t.Fatalf("目录里的专家应带分类与精选：%+v", c.Apps)
	}
	if !slices.ContainsFunc(c.Categories, func(x struct {
		ID    int64  `json:"id"`
		Group string `json:"group"`
		Name  string `json:"name"`
	}) bool {
		return x.ID == catID && x.Group == "app"
	}) {
		t.Fatalf("目录应列出可见的分类：%+v", c.Categories)
	}
	before := c.Revision
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/hub/categories/%d", catID), map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}}}, nil)
	expect(t, "收窄分类范围", code, 200, body)
	c = read()
	if slices.Contains(appNames(c), "expense-expert") || c.Revision == before {
		t.Fatalf("分类范围外的人不该再看到专家，修订号也应变：apps=%v", appNames(c))
	}
	code, raw, _ = bob.do("GET", "/api/dsh/skills/expense-expert", nil, nil)
	expect(t, "分类范围外召唤不到", code, 404, raw)

	// ---- 权限查询 ----
	type eff struct {
		Items []struct {
			Slug     string  `json:"slug"`
			Form     string  `json:"form"`
			Result   string  `json:"result"`
			Reason   string  `json:"reason"`
			BlockIDs []int64 `json:"block_ids"`
		} `json:"items"`
	}
	effective := func() map[string]string {
		t.Helper()
		code, body := admin.do("GET", fmt.Sprintf("/api/admin/hub/effective?user=%d", bobID), nil, nil)
		expect(t, "权限查询", code, 200, body)
		out := map[string]string{}
		for _, it := range decodeAs[eff](t, body).Items {
			out[it.Slug] = it.Result + "/" + it.Reason
		}
		return out
	}
	if e := effective(); e["expense-expert"] != "hidden/category" || e["invoice-check"] != "available/" || e["erp"] != "enabled/" || e["oa"] != "enabled/" {
		t.Fatalf("权限查询结果不对：%v", e)
	}

	// ---- 禁用策略 ----
	code, body = admin.do("POST", "/api/admin/hub/blocks", map[string]any{"scope": map[string]any{"users": []int64{bobID}}, "asset_ids": []int64{connID}}, nil)
	expect(t, "禁用策略要写原因", code, 400, body)
	code, body = admin.do("POST", "/api/admin/hub/blocks", map[string]any{"scope": map[string]any{"users": []int64{bobID}}, "asset_ids": []int64{connID}, "note": "离职交接中"}, nil)
	expect(t, "建禁用策略", code, 201, body)
	blockID := decodeAs[struct{ ID int64 }](t, body).ID
	c = read()
	if slices.ContainsFunc(c.Connections, func(x struct {
		Name     string `json:"name"`
		Enabled  bool   `json:"enabled"`
		Tools    int    `json:"tools"`
		Auth     string `json:"auth"`
		AuthMode string `json:"auth_mode"`
	}) bool {
		return x.Name == "erp"
	}) {
		t.Fatalf("被禁用的连接不该再出现：%+v", c.Connections)
	}
	code, raw, _ = bob.do("POST", "/api/dsh/connections/erp/subscription", nil, nil)
	expect(t, "被禁用的连接不能启用", code, 404, raw)
	code, body = admin.do("GET", fmt.Sprintf("/api/admin/hub/effective?user=%d", bobID), nil, nil)
	expect(t, "权限查询", code, 200, body)
	for _, it := range decodeAs[eff](t, body).Items {
		if it.Slug == "erp" && (it.Reason != "blocked" || !slices.Equal(it.BlockIDs, []int64{blockID})) {
			t.Fatalf("应指出是哪条禁用策略挡住的：%+v", it)
		}
		if it.Slug == "expense-expert" && it.Form != "expert" {
			t.Fatalf("权限查询应带专家形态：%+v", it)
		}
	}
	code, body = admin.do("DELETE", fmt.Sprintf("/api/admin/hub/blocks/%d", blockID), nil, nil)
	expect(t, "删禁用策略", code, 204, body)
	if e := effective(); e["erp"] != "enabled/" {
		t.Fatalf("删掉禁用策略后应恢复（订阅还在）：%v", e)
	}

	// ---- 删分类：能力变成没归类，又能看到了 ----
	code, body = admin.do("DELETE", fmt.Sprintf("/api/admin/hub/categories/%d", catID), nil, nil)
	expect(t, "删分类", code, 204, body)
	if !slices.Contains(appNames(read()), "expense-expert") {
		t.Fatal("删掉分类后专家应重新可见")
	}

	// ---- 专家团队：组长的指令里带上成员的角色说明，没权限的成员算缺少的能力 ----
	create("成员专家", map[string]any{"kind": "app", "slug": "tax-expert", "delivery": "auto", "spec": map[string]any{"title": "税务专家", "description": "税", "persona": "你是税务专家。", "form": "expert", "method": "先查税率表。"}}, 201)
	create("普通应用不能有团队成员", map[string]any{"kind": "app", "slug": "team-app", "delivery": "auto", "spec": map[string]any{"title": "x", "description": "x", "persona": "x", "members": []string{"tax-expert"}}}, 201)
	create("组长专家", map[string]any{"kind": "app", "slug": "finance-team", "delivery": "auto", "spec": map[string]any{"title": "财务团队", "description": "财务问题", "persona": "你是财务组长。", "form": "expert",
		"members": []string{"expense-expert", "tax-expert", "not-there", "finance-team"}}}, 201)
	code, raw, _ = bob.do("GET", "/api/dsh/skills/finance-team", nil, nil)
	expect(t, "召唤组长", code, 200, raw)
	team := string(raw)
	for _, want := range []string{"团队协作", "成员：报销专家", "成员：税务专家", "先查税率表", "not-there"} {
		if !strings.Contains(team, want) {
			t.Fatalf("组长指令里应有「%s」：%s", want, raw)
		}
	}
	if strings.Contains(team, "成员：财务团队") {
		t.Fatalf("组长不该把自己当成员：%s", raw)
	}
	code, raw, _ = bob.do("GET", "/api/dsh/skills/team-app", nil, nil)
	expect(t, "普通应用", code, 200, raw)
	if strings.Contains(string(raw), "团队协作") {
		t.Fatalf("普通应用的团队成员应被清掉：%s", raw)
	}

	// ---- 普通员工不能设精选，也进不了管理接口 ----
	code, body = bobWeb.do("PATCH", fmt.Sprintf("/api/hub/%d", expertID), map[string]any{"featured": false}, nil)
	if code != 403 {
		t.Fatalf("员工设精选应被拒：%d %s", code, body)
	}
	code, body = bobWeb.do("GET", "/api/admin/hub/categories", nil, nil)
	expect(t, "员工不能管分类", code, 403, body)
}
