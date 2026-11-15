package api_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 员工和管理员都能让 DSH 里的 Agent 做专家、改专家；管理员在管理模式里用 Agent 管分类、禁用策略、查权限。
func TestExpertAgentTools(t *testing.T) {
	f := newPrincipalFixture(t)
	limit := int64(100000)
	emp := f.user(t, "expert-author", &limit)
	f.token(t, emp.ID, "ee_at_expert_author", agentScopes, nil, nil)
	e := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_expert_author"}}
	call := func(a agent, tool string, args map[string]any) map[string]any {
		t.Helper()
		text, isErr := a.call(tool, args)
		if isErr {
			t.Fatalf("%s.%v 失败：%s", tool, args["action"], text)
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(text), &m)
		return m
	}

	// 设计规范技能在目录里
	code, body, _ := e.do("GET", "/api/dsh/skills/dsh-ee-expert-design", nil, nil)
	if code != 200 || !strings.Contains(string(body), "form") || !strings.Contains(string(body), "new_version") {
		t.Fatalf("应下发专家设计规范：%d %s", code, body)
	}

	// 员工让 Agent 做一个「自己用」的专家：不用审核，马上能召唤
	created := call(e, "ee_hub", map[string]any{"action": "create", "kind": "app", "slug": "my-contract-expert", "for_me": true,
		"spec": map[string]any{"title": "合同专家", "description": "审合同", "persona": "你是合同专家。", "form": "expert",
			"expertise": []any{"合同"}, "method": "先看付款条款。", "starters": []any{"审一下这份合同"}}})
	asset, _ := created["asset"].(map[string]any)
	id := int64(asset["id"].(float64))
	code, body, _ = e.do("GET", "/api/dsh/skills/my-contract-expert", nil, nil)
	if code != 200 || !strings.Contains(string(body), "先看付款条款") {
		t.Fatalf("自己用的专家应马上能召唤：%d %s", code, body)
	}

	// Agent 读详情能看到专家字段，在此基础上改版本，没改的字段保留
	detail := call(e, "ee_hub", map[string]any{"action": "detail", "id": id})
	spec, _ := detail["spec"].(map[string]any)
	if spec["form"] != "expert" || spec["method"] != "先看付款条款。" || fmt.Sprint(spec["expertise"]) != "[合同]" {
		t.Fatalf("detail 应带专家字段：%v", detail)
	}
	spec["method"] = "先看付款条款，再看违约责任。"
	call(e, "ee_hub", map[string]any{"action": "new_version", "id": id, "spec": spec})
	code, body, _ = e.do("GET", "/api/dsh/skills/my-contract-expert", nil, nil)
	if code != 200 || !strings.Contains(string(body), "再看违约责任") || !strings.Contains(string(body), "合同") {
		t.Fatalf("改版本后应生效：%d %s", code, body)
	}

	// 员工没有管理模式：看不到 ee_expert_admin
	if hasTool(e, "ee_expert_admin") {
		t.Fatal("员工不该有 ee_expert_admin")
	}

	// 管理员开管理模式后，用 Agent 管分类、禁用策略、查权限
	code, raw := f.admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "管理员", code, 200, raw)
	adminID := int64(decodeAs[struct{ ID int64 }](t, raw).ID)
	tk := f.token(t, adminID, "ee_at_expert_admin", agentScopes, nil, nil)
	code, raw = f.elevate(t, f.admin, tk.ID, adminPassword, 1)
	expect(t, "开启管理模式", code, 200, raw)
	a := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_expert_admin"}}
	cat := call(a, "ee_expert_admin", map[string]any{"action": "create_category", "group": "app", "name": "法务"})
	catID := int64(cat["id"].(float64))
	preview, _ := a.confirmed("ee_hub_admin", map[string]any{"action": "patch", "id": id, "category_id": catID, "featured": true})
	if !strings.Contains(preview, "合同专家") {
		t.Fatalf("预览：%s", preview)
	}
	preview, _ = a.confirmed("ee_expert_admin", map[string]any{"action": "create_block", "scope": map[string]any{"users": []any{emp.ID}}, "asset_ids": []any{id}, "note": "交接中"})
	if !strings.Contains(preview, "交接中") || !strings.Contains(preview, fmt.Sprintf("#%d", id)) {
		t.Fatalf("禁用预览：%s", preview)
	}
	eff := call(a, "ee_expert_admin", map[string]any{"action": "effective", "user": emp.ID})
	found := false
	for _, it := range eff["items"].([]any) {
		m := it.(map[string]any)
		if m["slug"] == "my-contract-expert" {
			found = m["reason"] == "blocked"
		}
	}
	if !found {
		t.Fatalf("权限查询应指出被禁用：%v", eff)
	}
	blocks := call(a, "ee_expert_admin", map[string]any{"action": "blocks"})
	b0 := blocks["blocks"].([]any)[0].(map[string]any)
	a.confirmed("ee_expert_admin", map[string]any{"action": "delete_block", "id": int64(b0["id"].(float64))})
	preview, _ = a.confirmed("ee_expert_admin", map[string]any{"action": "update_category", "id": catID, "scope": map[string]any{"departments": []any{"法务部"}}})
	if !strings.Contains(preview, "法务") {
		t.Fatalf("改分类预览：%s", preview)
	}
}
