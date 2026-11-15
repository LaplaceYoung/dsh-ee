package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// agent 是员工本机 DSH 的视角：用带 agent 范围的令牌调 /mcp 里的 ee_* 工具。
type agent struct {
	dshClient
}

var agentScopes = []string{rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeRegistryPublish, rbac.ScopeHub, rbac.ScopeConnections, rbac.ScopeAgent}

func (a agent) call(tool string, args map[string]any) (text string, isErr bool) {
	a.t.Helper()
	r := a.rpc("tools/call", map[string]any{"name": tool, "arguments": args})
	if r["error"] != nil {
		a.t.Fatalf("%s 返回 JSON-RPC 错误：%v", tool, r["error"])
	}
	res := r["result"].(map[string]any)
	content := res["content"].([]any)
	isErr, _ = res["isError"].(bool)
	return content[0].(map[string]any)["text"].(string), isErr
}

func (a agent) ok(tool string, args map[string]any) map[string]any {
	a.t.Helper()
	text, isErr := a.call(tool, args)
	if isErr {
		a.t.Fatalf("%s %v 失败：%s", tool, args["action"], text)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(text), &m)
	return m
}

func (a agent) fail(tool string, args map[string]any) string {
	a.t.Helper()
	text, isErr := a.call(tool, args)
	if !isErr {
		a.t.Fatalf("%s %v 应该失败，却成功了：%s", tool, args["action"], text)
	}
	return text
}

func (a agent) tools() map[string]map[string]any {
	a.t.Helper()
	out := map[string]map[string]any{}
	for _, t := range a.rpc("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
		m := t.(map[string]any)
		out[m["name"].(string)] = m
	}
	return out
}

// actions 是某个 ee_* 工具此刻向这个人开放的 action。
func (a agent) actions(tool string) []string {
	a.t.Helper()
	def := a.tools()[tool]
	if def == nil {
		return nil
	}
	enum := def["inputSchema"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)["enum"].([]any)
	var out []string
	for _, e := range enum {
		out = append(out, e.(string))
	}
	slices.Sort(out)
	return out
}

func eeTools(m map[string]map[string]any) []string {
	var out []string
	for n := range m {
		if strings.HasPrefix(n, "ee_") {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

func TestAgentTools(t *testing.T) {
	f := newPrincipalFixture(t)
	limit := int64(100000)
	emp := f.user(t, "agent-emp", &limit)
	f.token(t, emp.ID, "ee_at_agent_emp", agentScopes, nil, nil)
	a := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_agent_emp"}}
	web := newClient(t, f.ts.URL)
	code, body := web.do("POST", "/api/auth/login", map[string]string{"username": emp.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录网页", code, 200, body)

	t.Run("工具清单按令牌范围与账号权限过滤", func(t *testing.T) {
		want := []string{"ee_connections", "ee_doctor", "ee_hub", "ee_me", "ee_plugins", "ee_records", "ee_tokens"}
		if got := eeTools(a.tools()); !slices.Equal(got, want) {
			t.Fatalf("员工的 ee_* 工具：%v，期望 %v", got, want)
		}
		// 员工永远看不到管理类 action，也看不到还没开放的管理工具
		for _, name := range []string{"ee_users", "ee_roles", "ee_settings", "ee_admin_tokens"} {
			if a.tools()[name] != nil {
				t.Fatalf("员工不该看到 %s", name)
			}
		}
		// 没有 agent 范围的令牌：一个 ee_* 都没有
		f.token(t, emp.ID, "ee_at_no_agent", []string{rbac.ScopeModels, rbac.ScopeHub, rbac.ScopeConnections}, nil, nil)
		plain := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_no_agent"}}
		if got := eeTools(plain.tools()); len(got) != 0 {
			t.Fatalf("没有 agent 范围不该有工具：%v", got)
		}
		if text := plain.fail("ee_me", map[string]any{"action": "whoami"}); !strings.Contains(text, "Agent") {
			t.Fatalf("没有 agent 范围调用应提示：%s", text)
		}
		// 旧版 npm 接入令牌同样没有
		code, body := web.do("POST", "/api/me/npm-tokens", map[string]any{"name": "legacy", "access": "read"}, nil)
		expect(t, "旧令牌", code, 201, body)
		legacy := agent{dshClient{t: t, base: f.ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}}
		if got := eeTools(legacy.tools()); len(got) != 0 {
			t.Fatalf("旧令牌不该有 ee_* 工具：%v", got)
		}
	})

	t.Run("体积预算", func(t *testing.T) {
		raw, _ := json.Marshal(a.rpc("tools/list", nil)["result"])
		var tools []map[string]any
		var res struct{ Tools []map[string]any }
		_ = json.Unmarshal(raw, &res)
		for _, tl := range res.Tools {
			if strings.HasPrefix(tl["name"].(string), "ee_") {
				tools = append(tools, tl)
			}
		}
		b, _ := json.Marshal(tools)
		// 近似 token 数：字符数 / 2.5，员工侧预算 6k
		approx := float64(len([]rune(string(b)))) / 2.5
		t.Logf("员工侧 tools/list：%d 字符，约 %.0f token", len([]rune(string(b))), approx)
		if approx > 6000 {
			t.Fatalf("员工侧 tools/list 约 %.0f token，超过 6000 预算", approx)
		}
	})

	t.Run("账号与额度", func(t *testing.T) {
		me := a.ok("ee_me", map[string]any{"action": "whoami"})
		if me["username"] != "agent-emp" || me["department"] != "研发效能部" {
			t.Fatalf("whoami：%v", me)
		}
		if _, leaked := me["permissions"]; leaked {
			t.Fatal("whoami 不该带权限点清单")
		}
		code, body, _ := a.do("POST", "/v1/chat/completions", map[string]any{"model": "chat", "messages": []map[string]string{{"role": "user", "content": "hi"}}}, nil)
		expect(t, "调一次模型", code, 200, body)
		u := a.ok("ee_me", map[string]any{"action": "usage"})
		q := u["quota"].(map[string]any)
		if q["monthly_token_limit"].(float64) != 100000 || q["month_tokens"].(float64) <= 0 {
			t.Fatalf("额度与用量：%v", u)
		}
		if len(u["by_model"].([]any)) == 0 || len(u["by_token"].([]any)) == 0 || len(u["by_day"].([]any)) == 0 {
			t.Fatalf("用量明细应非空：%v", u)
		}
		if u["current_token"] == nil {
			t.Fatalf("应带当前令牌的用量：%v", u)
		}
		calls := a.ok("ee_records", map[string]any{"action": "model_calls"})
		if cs := calls["calls"].([]any); len(cs) != 1 || cs[0].(map[string]any)["model"] != "chat" {
			t.Fatalf("模型调用记录：%v", calls)
		}
		a.ok("ee_me", map[string]any{"action": "overview"})
	})

	t.Run("参数校验", func(t *testing.T) {
		if text := a.fail("ee_hub", map[string]any{"action": "nope"}); !strings.Contains(text, "action 必须是") {
			t.Fatal(text)
		}
		if text := a.fail("ee_hub", map[string]any{"action": "detail"}); !strings.Contains(text, "缺少必填参数 id") {
			t.Fatal(text)
		}
		if text := a.fail("ee_hub", map[string]any{"action": "search", "keyword": "x"}); !strings.Contains(text, "没有参数") {
			t.Fatal(text)
		}
		if text := a.fail("ee_hub", map[string]any{"action": "create", "kind": "connection", "slug": "x", "spec": map[string]any{}}); !strings.Contains(text, "kind 必须是") {
			t.Fatalf("员工不能经 Agent 建连接：%s", text)
		}
		if text := a.fail("ee_nope", map[string]any{"action": "x"}); !strings.Contains(text, "没有这个工具") {
			t.Fatal(text)
		}
	})

	t.Run("能力中心：搜索、启用、分享、审计来源", func(t *testing.T) {
		code, body := f.admin.do("POST", "/api/hub", map[string]any{"kind": "skill", "slug": "expense-policy", "spec": map[string]any{
			"title": "报销制度", "description": "差旅与报销标准", "body": "住宿每晚 1300 元以内。"}}, nil)
		expect(t, "管理员建技能", code, 201, body)
		id := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID

		res := a.ok("ee_hub", map[string]any{"action": "search", "q": "报销"})
		assets := res["assets"].([]any)
		if len(assets) != 1 || assets[0].(map[string]any)["slug"] != "expense-policy" || assets[0].(map[string]any)["enabled"] == true {
			t.Fatalf("搜索：%v", res)
		}
		if got := a.ok("ee_hub", map[string]any{"action": "search", "q": "不存在的东西"}); len(got["assets"].([]any)) != 0 {
			t.Fatalf("搜不到应为空：%v", got)
		}
		det := a.ok("ee_hub", map[string]any{"action": "detail", "id": id})
		if !strings.Contains(fmt.Sprint(det["spec"]), "1300") {
			t.Fatalf("详情应带正文：%v", det)
		}
		before := a.catalog()
		if slices.Contains(before.skillNames(), "expense-policy") {
			t.Fatal("启用前不该有这项技能")
		}
		text, isErr := a.call("ee_hub", map[string]any{"action": "enable", "id": id})
		if isErr || text != "已完成" {
			t.Fatalf("启用：%s", text)
		}
		after := a.catalog()
		if !slices.Contains(after.skillNames(), "expense-policy") || after.Revision == before.Revision {
			t.Fatalf("启用后目录应变化：%v", after.skillNames())
		}
		a.ok("ee_hub", map[string]any{"action": "disable", "id": id})
		if slices.Contains(a.catalog().skillNames(), "expense-policy") {
			t.Fatal("停用后不该有这项技能")
		}

		// 分享一个提示词：白名单模式下进入待审核
		res = a.ok("ee_hub", map[string]any{"action": "create", "kind": "prompt", "slug": "weekly-brief", "spec": map[string]any{
			"title": "周报提示词", "description": "整理周报", "body": "请把本周工作整理成三段。"}})
		if res["status"] != "pending" {
			t.Fatalf("员工经 Agent 分享应待审核：%v", res)
		}
		code, body = f.admin.do("GET", "/api/admin/hub/review", nil, nil)
		expect(t, "审核队列", code, 200, body)
		if !strings.Contains(string(body), "weekly-brief") {
			t.Fatalf("审核队列里应有：%s", body)
		}
		// for_me：一句话做一个「自己用」的应用，带数据表，不用审核、马上能写数据
		res = a.ok("ee_hub", map[string]any{"action": "create", "kind": "app", "slug": "my-outing", "for_me": true, "spec": map[string]any{
			"title": "我的团建报名", "description": "收集报名", "persona": "你是报名助手。", "icon": "🏕",
			"tables": []map[string]any{{"name": "signups", "title": "报名", "fields": []map[string]any{{"name": "session", "title": "场次", "kind": "select", "options": []string{"上午", "下午"}, "required": true}}}},
			"workbench": map[string]any{"panes": []map[string]any{{"id": "main", "type": "panel", "title": "报名", "widgets": []map[string]any{
				{"type": "form", "title": "报名", "submit": map[string]any{"label": "提交", "table": "signups"}},
			}}}},
		}})
		if res["status"] != "published" {
			t.Fatalf("for_me 应免审核直接上线：%v", res)
		}
		if out := a.ok("app__write", map[string]any{"action": "insert", "app": "my-outing", "table": "signups", "rows": []map[string]any{{"session": "上午"}}}); out["inserted"] != float64(1) {
			t.Fatalf("自己用的应用马上能写数据：%v", out)
		}
		if text := a.fail("ee_hub", map[string]any{"action": "create", "kind": "skill", "slug": "dsh-ee-guide", "spec": map[string]any{"title": "x", "description": "x", "body": "x"}}); !strings.Contains(text, "dsh-ee-") {
			t.Fatalf("系统技能前缀应被保留：%s", text)
		}
		// 网页校验的错误原样回给模型
		if text := a.fail("ee_hub", map[string]any{"action": "create", "kind": "prompt", "slug": "Bad Name", "spec": map[string]any{"title": "x", "description": "x", "body": "x"}}); !strings.Contains(text, "标识只能用小写字母") {
			t.Fatalf("应回传网页上的那句错误：%s", text)
		}
		// 员工不能直接给自己提交别人的能力的新版本
		if text := a.fail("ee_hub", map[string]any{"action": "new_version", "id": id, "spec": map[string]any{"title": "x", "description": "x", "body": "x"}}); !strings.Contains(text, "负责人") {
			t.Fatalf("非负责人提交新版本应被拒绝：%s", text)
		}

		// 审计：来源是 agent，带令牌与工具名；网页操作仍是 web
		code, body = f.admin.do("GET", "/api/admin/audit?via=agent", nil, nil)
		expect(t, "按来源筛审计", code, 200, body)
		logs := decodeAs[struct {
			Logs []struct {
				Action  string         `json:"action"`
				Via     string         `json:"via"`
				TokenID *int64         `json:"token_id"`
				Client  string         `json:"client"`
				Actor   string         `json:"actor_name"`
				Detail  map[string]any `json:"detail"`
			} `json:"logs"`
		}](t, body).Logs
		var sawCreate bool
		for _, l := range logs {
			if l.Via != "agent" || l.TokenID == nil || l.Actor != "agent-emp" || l.Client != "test-client" {
				t.Fatalf("经 Agent 的审计行不对：%+v", l)
			}
			if l.Action == "asset.create" && l.Detail["tool"] == "ee_hub.create" {
				sawCreate = true
			}
		}
		if !sawCreate {
			t.Fatalf("应有 ee_hub.create 的审计：%+v", logs)
		}
		code, body = f.admin.do("GET", "/api/admin/audit?via=web", nil, nil)
		expect(t, "网页来源", code, 200, body)
		if strings.Contains(string(body), "ee_hub.create") {
			t.Fatal("网页来源里不该有 Agent 的操作")
		}
	})

	t.Run("令牌：吊销要两步确认", func(t *testing.T) {
		victim := f.token(t, emp.ID, "ee_at_victim", agentScopes, nil, nil)
		other := f.user(t, "agent-other", nil)
		f.token(t, other.ID, "ee_at_other_agent", agentScopes, nil, nil)
		o := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_other_agent"}}
		id := fmt.Sprintf("at:%d", victim.ID)
		alive := func() bool {
			code, _, _ := a.do("GET", "/api/dsh/whoami", nil, map[string]string{"Authorization": "Bearer ee_at_victim"})
			return code == 200
		}

		list := a.ok("ee_tokens", map[string]any{"action": "list"})
		if !strings.Contains(fmt.Sprint(list), "ee_at_victim") || strings.Contains(fmt.Sprint(list), "token_hash") {
			t.Fatalf("令牌列表：%v", list)
		}
		// 第一步：只给预览，没有任何副作用
		first := a.ok("ee_tokens", map[string]any{"action": "revoke", "id": id})
		nonce, _ := first["confirm"].(string)
		if first["confirm_required"] != true || nonce == "" || !strings.Contains(fmt.Sprint(first["preview"]), "将吊销令牌") {
			t.Fatalf("第一步应返回预览与确认码：%v", first)
		}
		if !alive() {
			t.Fatal("预览不该吊销令牌")
		}
		// 换参数、换人、换令牌、瞎编的确认码：全部拒绝，且不消耗真确认码
		bad := []struct {
			name string
			who  agent
			args map[string]any
		}{
			{"换了参数", a, map[string]any{"action": "revoke", "id": "at:999999", "confirm": nonce}},
			{"别人的令牌", o, map[string]any{"action": "revoke", "id": id, "confirm": nonce}},
			{"瞎编的确认码", a, map[string]any{"action": "revoke", "id": id, "confirm": "not-a-real-nonce"}},
		}
		for _, c := range bad {
			if text := c.who.fail("ee_tokens", c.args); !strings.Contains(text, "确认码") && !strings.Contains(text, "没有找到令牌") {
				t.Fatalf("%s 应被拒绝：%s", c.name, text)
			}
		}
		// 同一用户的另一个令牌也不能用这个确认码
		f.token(t, emp.ID, "ee_at_agent_emp2", agentScopes, nil, nil)
		a2 := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_agent_emp2"}}
		if text := a2.fail("ee_tokens", map[string]any{"action": "revoke", "id": id, "confirm": nonce}); !strings.Contains(text, "确认码") {
			t.Fatalf("换令牌应被拒绝：%s", text)
		}
		if !alive() {
			t.Fatal("被拒绝的确认不该吊销令牌")
		}
		// 第二步
		text, isErr := a.call("ee_tokens", map[string]any{"action": "revoke", "id": id, "confirm": nonce})
		if isErr || text != "已完成" {
			t.Fatalf("带正确确认码应执行：%s", text)
		}
		if alive() {
			t.Fatal("确认后令牌应已吊销")
		}
		// 重放
		if text := a.fail("ee_tokens", map[string]any{"action": "revoke", "id": id, "confirm": nonce}); !strings.Contains(text, "确认码") {
			t.Fatalf("确认码只能用一次：%s", text)
		}
		// 过期：直接把库里的过期时间拨到过去
		f.token(t, emp.ID, "ee_at_victim2", agentScopes, nil, nil)
		var v2 int64
		for _, tk := range decodeAs[struct {
			Tokens []struct {
				ID     string
				Prefix string
			}
		}](t, mustGet(t, web, "/api/me/tokens")).Tokens {
			if tk.Prefix == "ee_at_victim2" {
				fmt.Sscanf(tk.ID, "at:%d", &v2)
			}
		}
		first = a.ok("ee_tokens", map[string]any{"action": "revoke", "id": fmt.Sprintf("at:%d", v2)})
		if _, err := f.st.DB().ExecContext(context.Background(), "UPDATE agent_confirms SET expires_at = ?", time.Now().Add(-time.Minute).UTC()); err != nil {
			t.Fatal(err)
		}
		if text := a.fail("ee_tokens", map[string]any{"action": "revoke", "id": fmt.Sprintf("at:%d", v2), "confirm": first["confirm"]}); !strings.Contains(text, "确认码") {
			t.Fatalf("过期的确认码应被拒绝：%s", text)
		}
		// 别人的令牌 id：预览阶段就找不到
		if text := o.fail("ee_tokens", map[string]any{"action": "revoke", "id": fmt.Sprintf("at:%d", v2)}); !strings.Contains(text, "没有找到令牌") {
			t.Fatalf("不能预览别人的令牌：%s", text)
		}
	})

	t.Run("企业连接：查看与申请", func(t *testing.T) {
		up := newFakeMCP(t)
		code, body := f.admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": "crm", "delivery": "auto",
			"scope": map[string]any{"departments": []string{"财务部"}},
			"spec":  map[string]any{"title": "CRM", "description": "客户与合同", "type": "mcp", "url": up.URL, "auth": map[string]any{"type": "bearer"}}, "secret": "up-secret"}, nil)
		expect(t, "接入 CRM（只给财务部）", code, 201, body)

		mine := a.ok("ee_connections", map[string]any{"action": "mine"})
		if len(mine["connections"].([]any)) != 0 {
			t.Fatalf("研发部员工不该有 CRM：%v", mine)
		}
		req := mine["requestable"].([]any)
		if len(req) != 1 || req[0].(map[string]any)["slug"] != "crm" {
			t.Fatalf("应列出可申请的 CRM：%v", mine)
		}
		if strings.Contains(fmt.Sprint(mine), up.URL) || strings.Contains(fmt.Sprint(mine), "up-secret") {
			t.Fatalf("不该泄露连接地址或凭据：%v", mine)
		}
		if text := a.fail("ee_connections", map[string]any{"action": "request_access", "slug": "nope", "reason": "x"}); !strings.Contains(text, "没有找到") {
			t.Fatal(text)
		}
		res := a.ok("ee_connections", map[string]any{"action": "request_access", "slug": "crm", "reason": "要查客户合同"})
		if res["status"] != "pending" || res["already_pending"] == true {
			t.Fatalf("申请：%v", res)
		}
		res = a.ok("ee_connections", map[string]any{"action": "request_access", "slug": "crm", "reason": "要查客户合同"})
		if res["already_pending"] != true {
			t.Fatalf("重复申请应合并：%v", res)
		}
		mine = a.ok("ee_connections", map[string]any{"action": "mine"})
		if len(mine["requests"].([]any)) != 1 {
			t.Fatalf("应只有一条申请：%v", mine)
		}
		code, body = f.admin.do("GET", "/api/admin/audit?action=connection.request", nil, nil)
		expect(t, "审计里有申请", code, 200, body)
		if strings.Count(string(body), `"action":"connection.request"`) != 1 || !strings.Contains(string(body), `"via":"agent"`) {
			t.Fatalf("申请应记一条来源为 agent 的审计：%s", body)
		}
		// 管理员放开范围后，同一个人就能直接用了
		var crmID int64
		for _, as := range decodeAs[struct {
			Assets []struct {
				ID   int64
				Slug string
			}
		}](t, mustGet(t, f.admin, "/api/hub")).Assets {
			if as.Slug == "crm" {
				crmID = as.ID
			}
		}
		code, body = f.admin.do("PATCH", fmt.Sprintf("/api/hub/%d", crmID), map[string]any{"scope": map[string]any{"all": true},
			"updated_at": scopeToken(t, f.admin, fmt.Sprintf("/api/hub/%d", crmID))}, nil)
		expect(t, "开放范围", code, 200, body)
		mine = a.ok("ee_connections", map[string]any{"action": "mine"})
		if len(mine["connections"].([]any)) != 1 || len(mine["requestable"].([]any)) != 0 {
			t.Fatalf("开放后应能用：%v", mine)
		}
		if text := a.fail("ee_connections", map[string]any{"action": "request_access", "slug": "crm", "reason": "再申请一次"}); !strings.Contains(text, "已经可以使用") {
			t.Fatal(text)
		}
		a.ok("ee_records", map[string]any{"action": "connection_calls"})
	})

	t.Run("插件", func(t *testing.T) {
		a.ok("ee_plugins", map[string]any{"action": "catalog"})
		a.ok("ee_plugins", map[string]any{"action": "my_packages"})
		if text := a.fail("ee_plugins", map[string]any{"action": "detail", "name": "not a valid name"}); text == "" {
			t.Fatal("非法包名应有提示")
		}
		// upload 要读本机文件，服务端只声明；直接调用要提示升级插件
		if text := a.fail("ee_plugins", map[string]any{"action": "upload", "path": "/tmp/x.tgz"}); !strings.Contains(text, "接入插件") {
			t.Fatalf("服务端不执行本地 action：%s", text)
		}
		if text := a.fail("ee_plugins", map[string]any{"action": "import_request", "name": "left-pad", "reason": ""}); text == "" {
			t.Fatal("缺原因应被拒绝")
		}
	})

	t.Run("上传：接入插件用令牌直传", func(t *testing.T) {
		pkg := pluginPkg("@company/agent-upload", "1.0.0", nil)
		tgz := makeTgz(t, pkg, bundleFile)
		req, _ := http.NewRequest("POST", f.ts.URL+"/api/agent/packages", strings.NewReader(string(tgz)))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Authorization", "Bearer ee_at_agent_emp")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("带 agent 令牌上传应 201，得到 %d", resp.StatusCode)
		}
		// 没有 agent 范围、没有令牌、旧令牌：都不行
		for name, token := range map[string]string{"无 agent 范围": "ee_at_no_agent", "无令牌": ""} {
			req, _ := http.NewRequest("POST", f.ts.URL+"/api/agent/packages", strings.NewReader(string(tgz)))
			req.Header.Set("Content-Type", "application/octet-stream")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 401 && resp.StatusCode != 403 {
				t.Fatalf("%s 不该能上传：%d", name, resp.StatusCode)
			}
		}
		code, body := f.admin.do("GET", "/api/admin/audit?action=package.publish", nil, nil)
		expect(t, "审计", code, 200, body)
		if !strings.Contains(string(body), `"via":"agent"`) {
			t.Fatalf("上传应记为经 Agent：%s", body)
		}
	})

	t.Run("诊断", func(t *testing.T) {
		d := a.ok("ee_doctor", map[string]any{"action": "check"})
		checks := d["checks"].([]any)
		status := map[string]string{}
		for _, c := range checks {
			m := c.(map[string]any)
			status[m["id"].(string)] = m["status"].(string)
		}
		if status["account"] != "ok" || status["quota"] != "ok" || status["models"] != "ok" {
			t.Fatalf("健康账号应全部 ok：%v", status)
		}
		// 没有额度的员工：quota 一项失败并给出修复建议
		broke := f.user(t, "agent-broke", nil)
		code, body := f.admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d/quota", broke.ID), map[string]any{"monthly_token_limit": 0}, nil)
		expect(t, "清空额度", code, 200, body)
		f.token(t, broke.ID, "ee_at_broke", agentScopes, nil, nil)
		b := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_broke"}}
		d = b.ok("ee_doctor", map[string]any{"action": "check"})
		for _, c := range d["checks"].([]any) {
			m := c.(map[string]any)
			if m["id"] == "quota" && (m["status"] != "fail" || m["fix"] == "") {
				t.Fatalf("没额度应 fail 且有建议：%v", m)
			}
		}
	})

	t.Run("系统技能", func(t *testing.T) {
		names := a.catalog().skillNames()
		for _, want := range []string{"dsh-ee-guide", "dsh-ee-doctor", "dsh-ee-share", "dsh-ee-app-design"} {
			if !slices.Contains(names, want) {
				t.Fatalf("目录里应有系统技能 %s：%v", want, names)
			}
		}
		code, body, _ := a.do("GET", "/api/dsh/skills/dsh-ee-guide", nil, nil)
		if code != 200 || !strings.Contains(string(body), "mcp__ee__ee_hub") || !strings.Contains(string(body), "confirm") {
			t.Fatalf("指南正文应写明工具名与两步确认：%d %s", code, body)
		}
		// 没有发布权限的人没有 share
		code, body = f.admin.do("POST", "/api/admin/roles", map[string]any{"code": "viewer-only", "name": "只读", "permissions": []string{rbac.PortalView, rbac.LLMUse}}, nil)
		expect(t, "建只读角色", code, 201, body)
		role := decodeAs[struct{ ID int64 }](t, body)
		ro := f.user(t, "agent-ro", &limit)
		code, body = f.admin.do("PUT", fmt.Sprintf("/api/admin/users/%d/roles", ro.ID), map[string]any{"role_ids": []int64{role.ID}}, nil)
		expect(t, "只给只读角色", code, 200, body)
		f.token(t, ro.ID, "ee_at_ro", agentScopes, nil, nil)
		r := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_ro"}}
		if names := r.catalog().skillNames(); slices.Contains(names, "dsh-ee-share") || !slices.Contains(names, "dsh-ee-guide") ||
			slices.Contains(names, "dsh-ee-extension-dev") || slices.Contains(names, "dsh-ee-access-config") {
			t.Fatalf("只读角色的系统技能：%v", names)
		}
		// 写扩展、配权限的技能只给有对应权限的人（docs/agent-dev.md §2.2）
		code, body = f.admin.do("POST", "/api/admin/roles", map[string]any{"code": "ext-dev", "name": "扩展开发", "permissions": []string{rbac.PortalView, rbac.LLMUse, rbac.ExtensionsDevelop, rbac.FieldsManage}}, nil)
		expect(t, "建扩展开发角色", code, 201, body)
		devRole := decodeAs[struct{ ID int64 }](t, body)
		dev := f.user(t, "agent-dev", &limit)
		code, body = f.admin.do("PUT", fmt.Sprintf("/api/admin/users/%d/roles", dev.ID), map[string]any{"role_ids": []int64{devRole.ID}}, nil)
		expect(t, "给扩展开发角色", code, 200, body)
		f.token(t, dev.ID, "ee_at_dev", agentScopes, nil, nil)
		dv := agent{dshClient{t: t, base: f.ts.URL, token: "ee_at_dev"}}
		if names := dv.catalog().skillNames(); !slices.Contains(names, "dsh-ee-extension-dev") || !slices.Contains(names, "dsh-ee-access-config") {
			t.Fatalf("有 extensions.develop 和 fields.manage 的人应有两个技能：%v", names)
		}
		if code, body, _ := dv.do("GET", "/api/dsh/skills/dsh-ee-access-config", nil, nil); code != 200 || !strings.Contains(string(body), "dsh-ee config plan") {
			t.Fatalf("配权限技能的正文：%d %s", code, body)
		}
		// 权限矩阵：只读角色的 action 被收窄
		if got := r.actions("ee_hub"); !slices.Equal(got, []string{"detail", "disable", "enable", "search"}) {
			t.Fatalf("只读角色的 ee_hub：%v", got)
		}
		if got := r.actions("ee_plugins"); !slices.Equal(got, []string{"catalog", "detail"}) {
			t.Fatalf("只读角色的 ee_plugins：%v", got)
		}
		if got := r.tools()["ee_tokens"]; got != nil {
			t.Fatalf("只读角色没有令牌权限，不该看到 ee_tokens：%v", got)
		}
		if text := r.fail("ee_hub", map[string]any{"action": "create", "kind": "prompt", "slug": "x-y", "spec": map[string]any{"title": "x", "description": "x", "body": "x"}}); !strings.Contains(text, "没有权限") {
			t.Fatalf("没有权限的 action 要拒绝：%s", text)
		}
		// 系统技能名不被员工占用
		if code, body, _ := a.do("GET", "/api/dsh/skills/dsh-ee-guide", nil, nil); code != 200 {
			t.Fatalf("%d %s", code, body)
		}
	})

	t.Run("总开关与 list_changed", func(t *testing.T) {
		stream := func(token string) (*http.Response, chan bool) {
			req, _ := http.NewRequest("GET", f.ts.URL+"/mcp", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "text/event-stream")
			sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
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
			return resp, got
		}
		waitChanged := func(what string, got chan bool) {
			t.Helper()
			select {
			case ok := <-got:
				if !ok {
					t.Fatalf("%s：事件流结束也没收到 list_changed", what)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s：没有收到 tools/list_changed", what)
			}
		}
		// 改角色
		resp, got := stream("ee_at_agent_emp")
		code, body := f.admin.do("POST", "/api/admin/roles", map[string]any{"code": "tmp-role", "name": "临时", "permissions": []string{rbac.PortalView}}, nil)
		expect(t, "建角色", code, 201, body)
		role := decodeAs[struct{ ID int64 }](t, body)
		code, body = f.admin.do("PUT", fmt.Sprintf("/api/admin/users/%d/roles", emp.ID), map[string]any{"role_ids": []int64{role.ID}}, nil)
		expect(t, "改角色", code, 200, body)
		waitChanged("改角色", got)
		resp.Body.Close()
		// 角色降到只读后，员工的写 action 立即消失
		if acts := a.actions("ee_hub"); slices.Contains(acts, "create") {
			t.Fatalf("改角色后 create 应消失：%v", acts)
		}

		// 关总开关：ee_* 工具与系统技能一起消失
		code, body = f.admin.do("GET", "/api/settings", nil, nil)
		expect(t, "读设置", code, 200, body)
		set := decodeAs[map[string]any](t, body)
		set["connect"].(map[string]any)["agentEnabled"] = false
		resp, got = stream("ee_at_agent_emp")
		code, body = f.admin.do("PUT", "/api/admin/settings", set, nil)
		expect(t, "关闭 Agent", code, 200, body)
		waitChanged("关总开关", got)
		resp.Body.Close()
		if got := eeTools(a.tools()); len(got) != 0 {
			t.Fatalf("关闭后不该有 ee_* 工具：%v", got)
		}
		if names := a.catalog().skillNames(); slices.Contains(names, "dsh-ee-guide") {
			t.Fatalf("关闭后不该下发系统技能：%v", names)
		}
		if text := a.fail("ee_me", map[string]any{"action": "whoami"}); !strings.Contains(text, "未开启") {
			t.Fatalf("关闭后调用应提示：%s", text)
		}
		code, _, _ = a.do("POST", "/api/agent/packages", nil, nil)
		if code != 403 {
			t.Fatalf("关闭后上传入口应 403：%d", code)
		}
	})
}
