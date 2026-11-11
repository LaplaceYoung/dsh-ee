package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// 员工侧 Agent 工具。键是路由表里的路由（"方法 路径"），一条路由最多对应一个 action。
// 要给某个工具加 action，在这里加一项，再确认 agentToolDocs 里有这个工具。

func dec(b []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func pick(m map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

func items(m map[string]any, key string) []map[string]any {
	arr, _ := m[key].([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if o, ok := v.(map[string]any); ok {
			out = append(out, o)
		}
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func argStr(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return strings.ToLower(strings.TrimSpace(s))
}

func argLimit(args map[string]any, def, max int) int {
	if f, ok := args["limit"].(float64); ok && f > 0 {
		return min(int(f), max)
	}
	return def
}

func contains(haystack string, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), needle)
}

var pLimit = agentParam{Name: "limit", Type: "integer", Desc: "最多返回几条", In: "local"}

// 技能、提示词、微应用的内容。服务端会按类型再校验一遍。
var specSchema = map[string]any{
	"type": "object",
	"description": "内容。skill/prompt：title、description、body（Markdown 正文）必填，skill 可加 when_to_use；" +
		"app：title、description、persona 必填，可加 icon（一个 emoji）、starters、skills、plugins、connections、knowledge、" +
		"tables（自带数据表）和 workbench（右侧工作台）。表：{name, title, visibility: own 各自填写 | shared 共同维护 | author 作者发布, " +
		"fields: [{name, title, kind: text|number|money|date|select|bool|user, options, required}]}。" +
		"工作台的数据源可以写 {\"table\": 表名}（统计写 {\"table\": 表名, \"args\": {\"group_by\": 字段}}，返回 items 里的 count），" +
		"表单的 submit 写 {\"label\": …, \"table\": 表名} 直接插入一行（不写 fields 就按表生成）；" +
		"表格的行按钮写 {\"label\": …, \"table\": 表名, \"set\": {字段: 值}} 改这一行，或 {\"label\": …, \"table\": 表名, \"delete\": true} 删除这一行。" +
		"每个组件写 width（full / wide / half / third / quarter）按仪表盘排；状态列用 format: status。版式与规范见技能 dsh-ee-app-design，做应用前先读。",
	"properties": map[string]any{
		"title":           map[string]any{"type": "string"},
		"description":     map[string]any{"type": "string", "description": "一两句话，DSH 靠它判断什么时候用"},
		"when_to_use":     map[string]any{"type": "string"},
		"body":            map[string]any{"type": "string"},
		"model_invocable": map[string]any{"type": "boolean"},
		"persona":         map[string]any{"type": "string"},
		"starters":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"skills":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"plugins":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"icon":            map[string]any{"type": "string"},
		"launch":          map[string]any{"type": "string", "enum": []string{"new", "resume"}},
		"connections":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"knowledge":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"tables":          map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"workbench":       map[string]any{"type": "object", "description": "{panes: [{id, type: panel, title, widgets: [...]}]}，组件类型 metric / table / chart / checklist / form / actions / markdown / heading / board，每个组件可写 width（full / wide / half / third / quarter）"},
		"web":             map[string]any{"type": "boolean", "description": "开放网页版：能用的人可以在浏览器（含手机）里打开 /a/<标识> 使用工作台，没有 AI"},
	},
}

var agentSpecs = map[string]*agentSpec{
	// ---- ee_me ----
	"GET /api/auth/me": {
		Tool: "ee_me", Action: "whoami", Summary: "我是谁：账号、部门、角色", Effect: effRead,
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			m := dec(body)
			out := pick(m, "username", "display_name", "department", "account_type", "is_super", "email")
			roles := []any{}
			for _, r := range items(m, "roles") {
				roles = append(roles, r["name"])
			}
			out["roles"] = roles
			return out
		},
	},
	"GET /api/me/usage": {
		Tool: "ee_me", Action: "usage", Summary: "每月额度、本月用量，以及近 30 天按天、按模型、按令牌的明细", Effect: effRead,
	},
	"GET /api/me/overview": {
		Tool: "ee_me", Action: "overview", Summary: "我装了哪些插件、哪些有更新、同部门在用什么、我发布的东西的采用情况", Effect: effRead,
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			m := dec(body)
			out := pick(m, "updates", "picks_scope", "reach")
			inst := []map[string]any{}
			for _, it := range items(m, "installed") {
				inst = append(inst, pick(it, "name", "title", "version", "latest", "has_update"))
			}
			picks := []map[string]any{}
			for _, it := range items(m, "picks") {
				p := pick(it, "name", "title", "kind", "users")
				p["description"] = clip(fmt.Sprint(it["description"]), 80)
				picks = append(picks, p)
			}
			mine := []map[string]any{}
			for _, it := range items(m, "contributions") {
				mine = append(mine, pick(it, "name", "title", "version", "status", "pending", "users"))
			}
			out["installed"], out["picks"], out["contributions"] = inst, picks, mine
			return out
		},
	},

	// ---- ee_tokens ----
	"GET /api/me/tokens": {
		Tool: "ee_tokens", Action: "list", Summary: "列出我的令牌（不含明文）", Effect: effRead,
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			out := []map[string]any{}
			for _, t := range items(dec(body), "tokens") {
				out = append(out, pick(t, "id", "name", "prefix", "source", "client_name", "scopes", "month_used", "monthly_token_limit", "expires_at", "last_used_at", "revoked_at"))
			}
			return map[string]any{"tokens": out}
		},
	},
	"DELETE /api/me/tokens/{id}": {
		Tool: "ee_tokens", Action: "revoke", Summary: "吊销我的一个令牌，用它的设备下次请求要重新登录", En: "Revoke a token", Effect: effDanger,
		Params: []agentParam{{Name: "id", Type: "string", Required: true, In: "path", Desc: "令牌 id，来自 list，形如 at:12"}},
		Preview: func(ctx context.Context, s *Server, p *Principal, args map[string]any) (string, error) {
			views, err := s.tokenViews(ctx, p.User.ID)
			if err != nil {
				return "", err
			}
			id, _ := args["id"].(string)
			for _, v := range views {
				if v.ID != id {
					continue
				}
				msg := fmt.Sprintf("将吊销令牌「%s」（%s…）。使用它的设备下次请求会要求重新登录。", v.Name, v.Prefix)
				if p.TokenID > 0 && id == fmt.Sprintf("at:%d", p.TokenID) {
					msg += "这正是当前这个 DSH 在用的令牌，吊销后需要在 DSH 里输入 /ee-login 重新登录。"
				}
				return msg, nil
			}
			return "", fmt.Errorf("没有找到令牌 %s，先用 list 查看", id)
		},
	},

	// ---- ee_hub ----
	"GET /api/hub": {
		Tool: "ee_hub", Action: "search", Summary: "在能力中心找技能、提示词、微应用、企业连接", Effect: effRead,
		Params: []agentParam{
			{Name: "q", Type: "string", In: "local", Desc: "关键词，匹配标题、说明、分类"},
			{Name: "kind", Type: "string", In: "local", Enum: []string{"skill", "prompt", "app", "connection"}},
			{Name: "only", Type: "string", In: "local", Enum: []string{"enabled", "available", "mine"}, Desc: "enabled 已在我的 DSH 生效；available 我能启用；mine 我负责的"},
			pLimit,
		},
		Shape: func(p *Principal, args map[string]any, body []byte) any {
			q, kind, only := argStr(args, "q"), argStr(args, "kind"), argStr(args, "only")
			limit := argLimit(args, 15, 40)
			out := []map[string]any{}
			total := 0
			for _, a := range items(dec(body), "assets") {
				if kind != "" && a["kind"] != kind {
					continue
				}
				enabled, _ := a["enabled"].(bool)
				avail, _ := a["available"].(bool)
				owned := a["owner"] == p.User.Username
				switch only {
				case "enabled":
					if !enabled {
						continue
					}
				case "available":
					if !avail {
						continue
					}
				case "mine":
					if !owned {
						continue
					}
				}
				if q != "" && !contains(fmt.Sprint(a["title"], " ", a["description"], " ", a["category"], " ", a["slug"]), q) {
					continue
				}
				total++
				if len(out) < limit {
					o := pick(a, "id", "slug", "kind", "title", "category", "status", "delivery", "available", "enabled", "uses", "users", "owner_name", "department")
					o["description"] = clip(fmt.Sprint(a["description"]), 120)
					out = append(out, o)
				}
			}
			return map[string]any{"total": total, "assets": out}
		},
	},
	"GET /api/hub/{id}": {
		Tool: "ee_hub", Action: "detail", Summary: "看一项能力的详情与正文", Effect: effRead,
		Params: []agentParam{{Name: "id", Type: "integer", Required: true, In: "path", Desc: "能力 id，来自 search"}},
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			m := dec(body)
			a, _ := m["asset"].(map[string]any)
			out := pick(a, "id", "slug", "kind", "title", "description", "category", "status", "delivery", "available", "enabled", "owner_name", "department", "can_manage", "live_version")
			if sp, ok := a["spec"].(map[string]any); ok {
				s := pick(sp, "when_to_use", "starters", "skills", "connections", "plugins", "instructions", "type")
				if b, ok := sp["body"].(string); ok {
					s["body"] = clip(b, 6000)
				}
				if b, ok := sp["persona"].(string); ok {
					s["persona"] = clip(b, 3000)
				}
				out["spec"] = s
			}
			tools := []map[string]any{}
			for _, t := range items(m, "tools") {
				tools = append(tools, pick(t, "name", "description", "read_only", "enabled"))
			}
			if len(tools) > 0 {
				out["tools"] = tools
			}
			vs := []map[string]any{}
			for _, v := range items(m, "versions") {
				vs = append(vs, pick(v, "version", "status", "created_at", "note"))
			}
			if len(vs) > 0 {
				out["versions"] = vs
			}
			// 企业连接的最近调用（只有能管理这个连接的人才会拿到）
			calls := []map[string]any{}
			for i, c := range items(m, "calls") {
				if i == 20 {
					break
				}
				calls = append(calls, pick(c, "tool", "username", "ok", "error", "latency_ms", "created_at"))
			}
			if len(calls) > 0 {
				out["calls"] = calls
			}
			return out
		},
	},
	"POST /api/hub/{id}/subscription": {
		Tool: "ee_hub", Action: "enable", Summary: "在我的 DSH 里启用一项能力", En: "Enable a capability", Effect: effWrite,
		Params: []agentParam{{Name: "id", Type: "integer", Required: true, In: "path"}},
	},
	"DELETE /api/hub/{id}/subscription": {
		Tool: "ee_hub", Action: "disable", Summary: "在我的 DSH 里停用一项能力", En: "Disable a capability", Effect: effWrite,
		Params: []agentParam{{Name: "id", Type: "integer", Required: true, In: "path"}},
	},
	"POST /api/hub": {
		Tool: "ee_hub", Action: "create", Summary: "把写好的技能、提示词或微应用提交到能力中心（按设置可能先审核）；for_me 做成「自己用」，不用审核、马上能用", En: "Create a capability", Effect: effWrite,
		Params: []agentParam{
			{Name: "kind", Type: "string", Required: true, Enum: []string{"skill", "prompt", "app"}},
			{Name: "slug", Type: "string", Required: true, Desc: "标识，小写字母数字和连字符，如 expense-policy，也是 /命令名"},
			{Name: "category", Type: "string"},
			{Name: "spec", Required: true, Schema: specSchema},
			{Name: "for_me", Type: "boolean", In: "local", Desc: "只给我自己用：不用审核，保存后马上出现在我的 DSH 里；想分享给同事再到网页上改可用范围"},
		},
		Body: func(_ context.Context, _ *Server, p *Principal, args map[string]any) (map[string]any, error) {
			out := map[string]any{}
			for _, k := range []string{"kind", "slug", "category", "spec"} {
				if v, ok := args[k]; ok {
					out[k] = v
				}
			}
			if mine, _ := args["for_me"].(bool); mine {
				out["scope"] = map[string]any{"all": false, "departments": []string{}, "roles": []int64{}, "users": []int64{p.User.ID}}
			}
			return out, nil
		},
	},
	"POST /api/hub/{id}/versions": {
		Tool: "ee_hub", Action: "new_version", Summary: "给我负责的能力提交新版本", En: "Submit a new version", Effect: effWrite,
		Params: []agentParam{
			{Name: "id", Type: "integer", Required: true, In: "path"},
			{Name: "spec", Required: true, Schema: specSchema},
		},
	},

	// ---- ee_plugins ----
	"GET /api/catalog": {
		Tool: "ee_plugins", Action: "catalog", Summary: "浏览插件市场里已上架的插件", Effect: effRead,
		Params: []agentParam{
			{Name: "q", Type: "string", In: "local", Desc: "关键词"},
			{Name: "category", Type: "string", In: "local"},
			pLimit,
		},
		Shape: func(p *Principal, args map[string]any, body []byte) any {
			q, cat := argStr(args, "q"), argStr(args, "category")
			limit := argLimit(args, 15, 40)
			out := []map[string]any{}
			total := 0
			for _, it := range items(dec(body), "plugins") {
				if cat != "" && strings.ToLower(fmt.Sprint(it["category"])) != cat {
					continue
				}
				if q != "" && !contains(fmt.Sprint(it["name"], " ", it["title"], " ", it["description"], " ", it["category"]), q) {
					continue
				}
				total++
				if len(out) < limit {
					o := pick(it, "name", "title", "kind", "category", "version", "installs", "origin", "status")
					o["description"] = clip(fmt.Sprint(it["description"]), 100)
					out = append(out, o)
				}
			}
			return map[string]any{"total": total, "plugins": out}
		},
	},
	"GET /api/catalog/{name...}": {
		Tool: "ee_plugins", Action: "detail", Summary: "看一个插件的详情", Effect: effRead,
		Params: []agentParam{{Name: "name", Type: "string", Required: true, In: "path", Desc: "包名，如 @acme/foo"}},
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			m := dec(body)
			out := pick(m, "name", "title", "description", "kind", "category", "version", "installs", "origin", "status", "score", "stars", "has_install_script", "dist_tags", "dependencies", "engines")
			out["readme"] = clip(fmt.Sprint(m["readme"]), 1500)
			vs := []map[string]any{}
			for _, v := range items(m, "versions") {
				vs = append(vs, pick(v, "version", "status", "created_at"))
			}
			out["versions"] = vs
			return out
		},
	},
	"GET /api/me/packages": {
		Tool: "ee_plugins", Action: "my_packages", Summary: "我发布的插件及每个版本的审核进度", Effect: effRead,
		Shape: func(_ *Principal, _ map[string]any, body []byte) any {
			out := []map[string]any{}
			for _, p := range items(dec(body), "packages") {
				o := pick(p, "name", "title", "version", "status", "pending")
				vs := []map[string]any{}
				for _, v := range items(p, "versions") {
					vs = append(vs, pick(v, "version", "status", "created_at"))
				}
				o["versions"] = vs
				out = append(out, o)
			}
			return map[string]any{"packages": out}
		},
	},
	"POST /api/agent/packages": {
		Tool: "ee_plugins", Action: "upload", Summary: "把本机 npm pack 出来的 .tgz 上传为插件（由接入插件在本机读取文件）", En: "Upload a plugin package", Effect: effWrite, Local: true,
		Params: []agentParam{{Name: "path", Type: "string", Required: true, In: "local", Desc: ".tgz 文件的本机路径"}},
	},
	"POST /api/me/import-requests": {
		Tool: "ee_plugins", Action: "import_request", Summary: "申请把社区里的 npm 插件引入企业源（进入待审核）", En: "Request importing a community plugin", Effect: effWrite,
		Params: []agentParam{
			{Name: "name", Type: "string", Required: true, Desc: "npm 包名"},
			{Name: "version", Type: "string"},
			{Name: "reason", Type: "string", Required: true, Desc: "用途，管理员据此审核"},
		},
	},

	// ---- ee_connections ----
	"GET /api/me/connections": {
		Tool: "ee_connections", Action: "mine", Summary: "我能用哪些内部系统及工具；哪些要我先在网页上填自己的凭据（credential.filled 为 false）；还没权限但可以申请的系统", Effect: effRead,
	},
	"POST /api/me/connection-requests": {
		Tool: "ee_connections", Action: "request_access", Summary: "申请使用某个内部系统（记入审计，管理员处理）", En: "Request access to a connection", Effect: effWrite,
		Params: []agentParam{
			{Name: "slug", Type: "string", Required: true, Desc: "连接标识，来自 mine 的 requestable"},
			{Name: "reason", Type: "string", Required: true, Desc: "用途"},
		},
	},
	"GET /api/me/access-requests": {
		Tool: "ee_connections", Action: "requests", Summary: "我提交的权限申请（连接、技能、知识库）和状态，以及待我审批的", Effect: effRead,
	},
	"POST /api/me/access-requests": {
		Tool: "ee_connections", Action: "request_for_app", Summary: "为了用某个应用，申请它缺的连接、技能或知识库（审批人是对应的维护者或管理员）", En: "Request access to a connection, skill or knowledge base", Effect: effWrite,
		Params: []agentParam{
			{Name: "kind", Type: "string", Required: true, Enum: []string{"connection", "skill", "knowledge"}},
			{Name: "slug", Type: "string", Required: true, Desc: "连接、技能或知识库的标识"},
			{Name: "app", Type: "string", Desc: "为了哪个应用（应用标识）；写了可以不填用途"},
			{Name: "reason", Type: "string", Desc: "用途"},
		},
	},

	// ---- ee_records ----
	"GET /api/me/calls/model": {
		Tool: "ee_records", Action: "model_calls", Summary: "我最近的模型调用", Effect: effRead,
		Params: []agentParam{{Name: "limit", Type: "integer", Desc: "最多几条，默认 20"}},
	},
	"GET /api/me/calls/connection": {
		Tool: "ee_records", Action: "connection_calls", Summary: "我最近的企业连接调用", Effect: effRead,
		Params: []agentParam{{Name: "limit", Type: "integer"}},
	},

	// ---- ee_doctor ----
	"GET /api/me/doctor": {
		Tool: "ee_doctor", Action: "check", Summary: "检查账号、额度、可用模型、能力目录，逐项给出 ok / warn / fail 与修复建议", Effect: effRead,
	},
}
