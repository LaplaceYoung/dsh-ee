package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 管理侧 Agent 工具：只有令牌开着管理模式（admin 范围且没到期）时才出现，
// 处理函数与网页管理页是同一份，权限点检查、防提权、审计都照旧。
//
// 约定：
//   - 会改变别人权限、费用、可见性、系统行为的操作一律 danger（先预览后确认）。
//   - 机密（上游 API key、LDAP 绑定密码、连接凭据、令牌明文、用户密码）不进入任何入参，也不会出现在返回里。
//   - 返回都经 Shape 白名单 / 截断，避免把整张表倒进模型上下文。

var (
	pID      = agentParam{Name: "id", Type: "integer", Required: true, In: "path"}
	pQ       = agentParam{Name: "q", Type: "string", In: "local", Desc: "关键词"}
	pLimit30 = agentParam{Name: "limit", Type: "integer", In: "local", Desc: "最多返回几条，默认 30"}
)

func init() {
	for k, v := range adminSpecs() {
		if _, dup := agentSpecs[k]; dup {
			panic("agentSpecs 重复：" + k)
		}
		v.Admin = true
		agentSpecs[k] = v
		delete(agentPending, k)
	}
	delete(agentExcluded, "GET /api/settings")
	// 员工侧的两个管理模式入口
	agentSpecs["GET /api/me/admin-link"] = &agentSpec{
		Tool: "ee_tokens", Action: "request_admin", Summary: "要用管理工具时，给出网页链接，管理员在浏览器输入密码开启管理模式（Agent 无法替人开启）", Effect: effRead,
		Show: func(p *Principal) bool { return p.CanAdmin && !p.AdminLive },
	}
	agentSpecs["DELETE /api/me/tokens/{id}/elevate"] = &agentSpec{
		Tool: "ee_tokens", Action: "end_admin", Summary: "提前结束当前令牌的管理模式", En: "End admin mode", Effect: effWrite,
		Params: []agentParam{{Name: "id", Type: "string", In: "path", Default: "current", Desc: "默认当前令牌"}},
		Show:   func(p *Principal) bool { return p.AdminLive },
	}
	agentExcluded["POST /api/me/tokens/{id}/elevate"] = "管理模式只能由本人在网页上输入密码开启，Agent 不能替人输入密码"
	for _, p := range []string{
		"POST /api/admin/permissions", "DELETE /api/admin/permissions/{code}",
	} {
		delete(agentPending, p)
		agentExcluded[p] = "权限点目录以代码为准，不让 Agent 增删；角色里勾选权限点用 ee_roles"
	}
	for p, why := range map[string]string{
		"POST /api/admin/providers":         "接入新供应商需要上游 API key，机密不经过模型上下文，请在网页上接入",
		"GET /api/admin/keys":               "旧版 API key 在网页管理",
		"POST /api/admin/keys":              "密钥明文不进模型上下文",
		"DELETE /api/admin/keys/{id}":       "旧版 API key 在网页管理",
		"POST /api/admin/tokens":            "下发令牌的明文不进模型上下文，请在网页「令牌管理」下发",
		"GET /api/admin/npm-tokens":         "旧版 npm 令牌在网页管理",
		"DELETE /api/admin/npm-tokens/{id}": "旧版 npm 令牌在网页管理",
	} {
		delete(agentPending, p)
		agentExcluded[p] = why
	}
	delete(agentPending, "GET /api/admin/tarball")
	agentExcluded["GET /api/admin/tarball"] = "二进制下载，不适合给模型；审核时读内容用 ee_review.inspect_package"
}

// ---- 通用小工具 ----

// changes 把入参里（除 skip 外）的键值排成一行，给预览用。
func changes(args map[string]any, skip ...string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		if k == "action" || k == "confirm" || slices.Contains(skip, k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		b, _ := json.Marshal(args[k])
		parts = append(parts, k+"="+clip(string(b), 80))
	}
	if len(parts) == 0 {
		return "（无参数）"
	}
	return strings.Join(parts, "，")
}

func argID(args map[string]any) int64 {
	f, _ := args["id"].(float64)
	return int64(f)
}

func userLabel(ctx context.Context, s *Server, id int64) (string, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		if notFound(err) {
			return "", fmt.Errorf("没有 id 为 %d 的用户", id)
		}
		return "", err
	}
	label := u.Username
	if u.Department != "" {
		label += "（" + u.Department + "）"
	}
	if u.AccountType == store.AccountBot {
		label += "【机器人】"
	}
	return label, nil
}

// roleWithCount 取角色及使用人数（GetRole 不带人数，列表才有）。
func roleWithCount(ctx context.Context, s *Server, id int64) (*store.Role, error) {
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, fmt.Errorf("没有 id 为 %d 的角色", id)
}

func roleLabels(ctx context.Context, s *Server, ids []any) string {
	var names []string
	for _, v := range ids {
		f, _ := v.(float64)
		if r, err := s.store.GetRole(ctx, int64(f)); err == nil {
			names = append(names, r.Name)
		} else {
			names = append(names, fmt.Sprintf("#%d(不存在)", int64(f)))
		}
	}
	if len(names) == 0 {
		return "（无角色）"
	}
	return strings.Join(names, "、")
}

func assetLabel(ctx context.Context, s *Server, id int64) (string, error) {
	a, err := s.store.GetAsset(ctx, id)
	if err != nil {
		if notFound(err) {
			return "", fmt.Errorf("没有 id 为 %d 的能力", id)
		}
		return "", err
	}
	return fmt.Sprintf("%s「%s」(%s)", a.Kind, a.Title, a.Slug), nil
}

// generic 是没有专门预览的危险操作的预览：写明动作与参数。
func generic(what string, skip ...string) func(context.Context, *Server, *Principal, map[string]any) (string, error) {
	return func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (string, error) {
		return fmt.Sprintf("将%s：%s。", what, changes(args, skip...)), nil
	}
}

// listShape 把 {key: [...]} 里的每项只留白名单字段，并按 limit 截断。
func listShape(key string, fields ...string) func(*Principal, map[string]any, []byte) any {
	return func(_ *Principal, args map[string]any, body []byte) any {
		limit := argLimit(args, 30, 100)
		m := dec(body)
		all := items(m, key)
		out := make([]map[string]any, 0, min(len(all), limit))
		for _, it := range all {
			if len(out) == limit {
				break
			}
			out = append(out, pick(it, fields...))
		}
		return map[string]any{"total": len(all), key: out}
	}
}

// 一个不认识的响应也不要原样倒给模型：深度截断长字符串，列表最多保留 limit 项。
func trimmed(body []byte) any {
	var v any
	_ = json.Unmarshal(body, &v)
	return trimValue(v, 0)
}

func trimValue(v any, depth int) any {
	switch x := v.(type) {
	case string:
		return clip(x, 300)
	case []any:
		out := make([]any, 0, min(len(x), 50))
		for i, e := range x {
			if i == 50 {
				break
			}
			out = append(out, trimValue(e, depth+1))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = trimValue(e, depth+1)
		}
		return out
	}
	return v
}

func trimShape(_ *Principal, _ map[string]any, body []byte) any { return trimmed(body) }

// mergeInto 把 changes 合并进 base：只允许 base 里已有的键（防止拼错字段被悄悄丢弃），对象递归合并，
// banned 里的键（点号路径）一律拒绝。
func mergeInto(base, changes map[string]any, banned []string, path string) error {
	for k, v := range changes {
		full := strings.TrimPrefix(path+"."+k, ".")
		if slices.Contains(banned, full) {
			return fmt.Errorf("不能通过 Agent 修改 %s", full)
		}
		cur, ok := base[k]
		if !ok && !slices.Contains(settingsFreeMaps, path) {
			return fmt.Errorf("没有这个设置项：%s", full)
		}
		cm, curIsMap := cur.(map[string]any)
		vm, newIsMap := v.(map[string]any)
		if curIsMap && newIsMap {
			if err := mergeInto(cm, vm, banned, full); err != nil {
				return err
			}
			continue
		}
		base[k] = v
	}
	return nil
}

func diffLines(before, after map[string]any, path string) []string {
	var out []string
	keys := make([]string, 0, len(after))
	for k := range after {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		full := strings.TrimPrefix(path+"."+k, ".")
		bm, b1 := before[k].(map[string]any)
		am, a1 := after[k].(map[string]any)
		if b1 && a1 {
			out = append(out, diffLines(bm, am, full)...)
			continue
		}
		bj, _ := json.Marshal(before[k])
		aj, _ := json.Marshal(after[k])
		if string(bj) != string(aj) {
			out = append(out, fmt.Sprintf("%s：%s → %s", full, clip(string(bj), 60), clip(string(aj), 60)))
		}
	}
	return out
}

// 这些设置项牵连登录方式与企业作用域，改错会把人锁在外面或让 npm 源整体失效，只在网页上改。
var settingsBanned = []string{"auth", "scope", "brand.logoRev"}

// settingsFreeMaps 下面的键由使用者自定（颜色名、设计变量名），可以新增；值为 null 表示删掉
var settingsFreeMaps = []string{"brand.theme.colors", "brand.theme.tokens"}

func settingsMap(ctx context.Context, s *Server) (map[string]any, error) {
	set, err := s.appSettings(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(set)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m, nil
}

func settingsBody(ctx context.Context, s *Server, _ *Principal, args map[string]any) (map[string]any, error) {
	base, err := settingsMap(ctx, s)
	if err != nil {
		return nil, err
	}
	ch, _ := args["changes"].(map[string]any)
	if err := mergeInto(base, ch, settingsBanned, ""); err != nil {
		return nil, err
	}
	// 预览阶段就把取值不对的改动拦下来，不要等用户同意了才报错
	raw, _ := json.Marshal(base)
	var next store.AppSettings
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&next); err != nil {
		return nil, fmt.Errorf("设置格式不对：%v", err)
	}
	if msg := validateSettings(&next); msg != "" {
		return nil, errors.New(msg)
	}
	return base, nil
}

func ldapBase(ctx context.Context, s *Server) (map[string]any, error) {
	cfg, err := s.loadLDAP(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(cfg)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m, nil
}

func ldapBody(ctx context.Context, s *Server, _ *Principal, args map[string]any) (map[string]any, error) {
	base, err := ldapBase(ctx, s)
	if err != nil {
		return nil, err
	}
	ch, _ := args["changes"].(map[string]any)
	for _, k := range []string{"bindPassword", "bindPasswordSet", "lastSync", "testPassword"} {
		if _, ok := ch[k]; ok {
			return nil, fmt.Errorf("%s 不能通过 Agent 设置，绑定密码请在网页上填写", k)
		}
	}
	if err := mergeInto(base, ch, nil, ""); err != nil {
		return nil, err
	}
	return base, nil
}

var ldapSecretKeys = []string{"bindPassword", "testPassword"}

func adminSpecs() map[string]*agentSpec {
	rd, wr, dg := effRead, effWrite, effDanger
	return map[string]*agentSpec{
		// ---- ee_users ----
		"GET /api/admin/users": {
			Tool: "ee_users", Action: "list", Summary: "查用户与机器人账号（可按关键词、部门筛）", Effect: rd,
			Params: []agentParam{pQ, {Name: "department", Type: "string", In: "local"}, {Name: "type", Type: "string", In: "local", Enum: []string{"human", "bot"}}, pLimit30},
			Shape: func(_ *Principal, args map[string]any, body []byte) any {
				q, dept, typ := argStr(args, "q"), argStr(args, "department"), argStr(args, "type")
				limit := argLimit(args, 30, 100)
				out := []map[string]any{}
				total := 0
				for _, u := range items(dec(body), "users") {
					if dept != "" && strings.ToLower(fmt.Sprint(u["department"])) != dept {
						continue
					}
					if typ != "" && u["account_type"] != typ {
						continue
					}
					if q != "" && !contains(fmt.Sprint(u["username"], " ", u["display_name"], " ", u["email"]), q) {
						continue
					}
					total++
					if len(out) < limit {
						o := pick(u, "id", "username", "display_name", "department", "disabled", "account_type", "auth_source", "monthly_token_limit", "month_tokens", "last_login_at", "owner_id")
						roles := []any{}
						for _, r := range items(u, "roles") {
							roles = append(roles, r["name"])
						}
						o["roles"] = roles
						out = append(out, o)
					}
				}
				return map[string]any{"total": total, "users": out}
			},
		},
		"POST /api/admin/users": {
			Tool: "ee_users", Action: "create", Summary: "新建用户。人类账号用 auth_source=ldap（首次登录由目录验证，不设密码）；机器人账号用 account_type=bot。新用户额度默认 0", En: "Create a user", Effect: wr,
			Params: []agentParam{
				{Name: "username", Type: "string", Required: true},
				{Name: "display_name", Type: "string"}, {Name: "department", Type: "string"}, {Name: "email", Type: "string"},
				{Name: "auth_source", Type: "string", Enum: []string{"ldap"}, Desc: "人类账号填 ldap；机器人不填"},
				{Name: "account_type", Type: "string", Enum: []string{"human", "bot"}},
				{Name: "owner_id", Type: "integer", Desc: "机器人账号的负责人用户 id"},
				{Name: "role_ids", Type: "array", Items: "integer", Desc: "不填给默认员工角色；[] 不给角色"},
				{Name: "monthly_token_limit", Type: "integer", Desc: "每月 token 额度"},
			},
		},
		"PATCH /api/admin/users/{id}": {
			Tool: "ee_users", Action: "update", Summary: "改用户的显示名、部门，或停用 / 启用", En: "Update a user", Effect: dg,
			Params: []agentParam{pID, {Name: "display_name", Type: "string"}, {Name: "department", Type: "string"}, {Name: "disabled", Type: "boolean"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				l, err := userLabel(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				msg := fmt.Sprintf("将修改用户 %s：%s。", l, changes(args, "id"))
				if d, ok := args["disabled"].(bool); ok && d {
					msg += "停用后他的令牌与会话都不能再用。"
				}
				return msg, nil
			},
		},
		"PUT /api/admin/users/{id}/roles": {
			Tool: "ee_users", Action: "set_roles", Summary: "整体设置用户的角色（覆盖原有角色）", En: "Set user roles", Effect: dg,
			Params: []agentParam{pID, {Name: "role_ids", Type: "array", Items: "integer", Required: true}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				l, err := userLabel(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				cur := []any{}
				if refs, err := s.store.UserRoleRefs(ctx); err == nil {
					for _, r := range refs[argID(args)] {
						cur = append(cur, float64(r.ID))
					}
				}
				next, _ := args["role_ids"].([]any)
				return fmt.Sprintf("将把用户 %s 的角色从【%s】改为【%s】，权限随之立即变化。", l, roleLabels(ctx, s, cur), roleLabels(ctx, s, next)), nil
			},
		},
		"PATCH /api/admin/users/{id}/quota": {
			Tool: "ee_users", Action: "set_quota", Summary: "设置用户的每月额度、可用模型、额度到期时间（没传的项不变）", En: "Set user quota", Effect: wr,
			Params: []agentParam{pID,
				{Name: "monthly_token_limit", Type: "integer", In: "local", Desc: "每月 token 上限；0 表示不能调用"},
				{Name: "unlimited", Type: "boolean", In: "local", Desc: "true 表示不限额度"},
				{Name: "allowed_models", Type: "array", In: "local", Desc: "可用的对外模型名"},
				{Name: "all_models", Type: "boolean", In: "local", Desc: "true 表示允许全部模型"},
				{Name: "expires_at", Type: "string", In: "local", Desc: "额度到期时间，RFC 3339"},
				{Name: "never_expires", Type: "boolean", In: "local", Desc: "true 表示额度永不过期"},
			},
			Body: func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (map[string]any, error) {
				out := map[string]any{}
				if u, _ := args["unlimited"].(bool); u {
					out["monthly_token_limit"] = nil
				} else if v, ok := args["monthly_token_limit"]; ok {
					out["monthly_token_limit"] = v
				}
				if a, _ := args["all_models"].(bool); a {
					out["allowed_models"] = nil
				} else if v, ok := args["allowed_models"]; ok {
					out["allowed_models"] = v
				}
				if n, _ := args["never_expires"].(bool); n {
					out["quota_expires_at"] = nil
				} else if v, ok := args["expires_at"]; ok {
					out["quota_expires_at"] = v
				}
				if len(out) == 0 {
					return nil, fmt.Errorf("至少要给一项：monthly_token_limit / unlimited / allowed_models / all_models / expires_at / never_expires")
				}
				return out, nil
			},
		},

		// ---- ee_roles ----
		"GET /api/admin/roles": {
			Tool: "ee_roles", Action: "list", Summary: "列出角色及其权限点", Effect: rd,
			Shape: listShape("roles", "id", "code", "name", "description", "is_super", "builtin", "permissions", "user_count"),
		},
		"GET /api/admin/permissions": {
			Tool: "ee_roles", Action: "permissions", Summary: "列出全部权限点（编码、名称、分组）", Effect: rd,
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				out := []map[string]any{}
				for _, p := range items(dec(body), "permissions") {
					out = append(out, pick(p, "code", "name", "group"))
				}
				return map[string]any{"permissions": out}
			},
		},
		"POST /api/admin/roles": {
			Tool: "ee_roles", Action: "create", Summary: "新建角色并勾选权限点", En: "Create a role", Effect: wr,
			Params: []agentParam{{Name: "code", Type: "string", Required: true}, {Name: "name", Type: "string", Required: true}, {Name: "description", Type: "string"}, {Name: "permissions", Type: "array", Desc: "权限点编码"}},
		},
		"PATCH /api/admin/roles/{id}": {
			Tool: "ee_roles", Action: "update", Summary: "改角色的名称、说明或权限点（整体覆盖 permissions）", En: "Update a role", Effect: dg,
			Params: []agentParam{pID, {Name: "name", Type: "string"}, {Name: "description", Type: "string"}, {Name: "permissions", Type: "array"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				r, err := roleWithCount(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				msg := fmt.Sprintf("将修改角色「%s」（%d 人在用）：%s。", r.Name, r.UserCount, changes(args, "id"))
				if next, ok := args["permissions"].([]any); ok {
					var add, del []string
					have := map[string]bool{}
					for _, p := range r.Permissions {
						have[p] = true
					}
					want := map[string]bool{}
					for _, v := range next {
						c, _ := v.(string)
						want[c] = true
						if !have[c] {
							add = append(add, c)
						}
					}
					for _, p := range r.Permissions {
						if !want[p] {
							del = append(del, p)
						}
					}
					msg += fmt.Sprintf("权限点新增 %v，移除 %v。", add, del)
				}
				return msg, nil
			},
		},
		"DELETE /api/admin/roles/{id}": {
			Tool: "ee_roles", Action: "delete", Summary: "删除角色", En: "Delete a role", Effect: dg,
			Params: []agentParam{pID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				r, err := roleWithCount(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("将删除角色「%s」，当前有 %d 人拥有它，他们会失去它带来的权限。", r.Name, r.UserCount), nil
			},
		},

		// ---- ee_menus ----
		"GET /api/admin/menus": {
			Tool: "ee_menus", Action: "list", Summary: "列出菜单树", Effect: rd,
			Shape: listShape("menus", "id", "parent_id", "kind", "title", "path", "page_key", "permission", "sort", "visible"),
		},
		"POST /api/admin/menus": {
			Tool: "ee_menus", Action: "create", Summary: "新建菜单项", En: "Create a menu item", Effect: wr,
			Params: []agentParam{{Name: "title", Type: "string", Required: true}, {Name: "kind", Type: "string", Desc: "page / group"}, {Name: "path", Type: "string"}, {Name: "page_key", Type: "string"}, {Name: "icon", Type: "string"}, {Name: "permission", Type: "string"}, {Name: "parent_id", Type: "integer"}, {Name: "sort", Type: "integer"}, {Name: "visible", Type: "boolean"}},
		},
		"PATCH /api/admin/menus/{id}": {
			Tool: "ee_menus", Action: "update", Summary: "改菜单的名称、路径、权限、显隐", En: "Update a menu item", Effect: wr,
			Params: []agentParam{pID, {Name: "title", Type: "string", In: "local"}, {Name: "path", Type: "string", In: "local"}, {Name: "page_key", Type: "string", In: "local"}, {Name: "icon", Type: "string", In: "local"}, {Name: "permission", Type: "string", In: "local"}, {Name: "parent_id", Type: "integer", In: "local"}, {Name: "sort", Type: "integer", In: "local"}, {Name: "visible", Type: "boolean", In: "local"}, {Name: "kind", Type: "string", In: "local"}},
			// 网页的菜单保存是整体提交，这里把没传的项补成现有的值
			Body: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (map[string]any, error) {
				m, err := s.store.GetMenu(ctx, argID(args))
				if err != nil {
					return nil, fmt.Errorf("没有 id 为 %d 的菜单", argID(args))
				}
				out := map[string]any{"parent_id": m.ParentID, "kind": m.Kind, "title": m.Title, "path": m.Path, "page_key": m.PageKey, "icon": m.Icon, "permission": m.Permission, "sort": m.Sort, "visible": m.Visible}
				for _, k := range []string{"title", "path", "page_key", "icon", "permission", "parent_id", "sort", "visible", "kind"} {
					if v, ok := args[k]; ok {
						out[k] = v
					}
				}
				return out, nil
			},
		},
		"PUT /api/admin/menus/order": {
			Tool: "ee_menus", Action: "reorder", Summary: "调整菜单顺序与层级", En: "Reorder menus", Effect: wr,
			Params: []agentParam{{Name: "items", Required: true, Schema: map[string]any{"type": "array", "description": "每项 {id, parent_id, sort}", "items": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}, "parent_id": map[string]any{"type": "integer"}, "sort": map[string]any{"type": "integer"}}}}}},
		},
		"DELETE /api/admin/menus/{id}": {
			Tool: "ee_menus", Action: "delete", Summary: "删除菜单项", En: "Delete a menu item", Effect: dg,
			Params: []agentParam{pID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				m, err := s.store.GetMenu(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的菜单", argID(args))
				}
				return fmt.Sprintf("将删除菜单「%s」（%s），所有人的导航里都会少这一项。", m.Title, m.Path), nil
			},
		},

		// ---- ee_models ----
		"GET /api/admin/providers": {
			Tool: "ee_models", Action: "providers", Summary: "列出上游供应商与连通状态（不含 API key）", Effect: rd,
			Shape: listShape("providers", "id", "name", "vendor", "base_url", "api_key_hint", "priority", "enabled", "last_check_at", "last_check_ok", "last_check_msg", "models"),
		},
		"PATCH /api/admin/providers/{id}": {
			Tool: "ee_models", Action: "update_provider", Summary: "改供应商的类型、地址、优先级，或启停（不能改 API key）", En: "Update a provider", Effect: dg,
			Params: []agentParam{pID, {Name: "vendor", Type: "string"}, {Name: "base_url", Type: "string"}, {Name: "priority", Type: "integer"}, {Name: "enabled", Type: "boolean"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				p, err := s.store.GetProvider(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的供应商", argID(args))
				}
				return fmt.Sprintf("将修改供应商「%s」（上游模型 %v）：%s。改动立即影响所有走它的调用。", p.Name, p.Models, changes(args, "id")), nil
			},
		},
		"DELETE /api/admin/providers/{id}": {
			Tool: "ee_models", Action: "delete_provider", Summary: "删除供应商及其下的模型", En: "Delete a provider", Effect: dg,
			Params: []agentParam{pID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				p, err := s.store.GetProvider(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的供应商", argID(args))
				}
				return fmt.Sprintf("将删除供应商「%s」及其模型 %v，调用这些模型的人会立刻失败。", p.Name, p.Models), nil
			},
		},
		"POST /api/admin/providers/{id}/check": {
			Tool: "ee_models", Action: "check_provider", Summary: "检测供应商连通性", En: "Check a provider", Effect: wr,
			Params: []agentParam{pID},
		},
		"GET /api/admin/models": {
			Tool: "ee_models", Action: "models", Summary: "列出对外模型及其上游映射", Effect: rd,
			Shape: listShape("models", "id", "provider_id", "provider_name", "name", "upstream_model", "display_name", "enabled"),
		},
		"POST /api/admin/models": {
			Tool: "ee_models", Action: "create_models", Summary: "在某个供应商下上架模型", En: "Add models", Effect: wr,
			Params: []agentParam{{Name: "provider_id", Type: "integer", Required: true}, {Name: "items", Required: true, Schema: map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"upstream_model": map[string]any{"type": "string"}, "name": map[string]any{"type": "string", "description": "对外名，不填同上游名"}, "display_name": map[string]any{"type": "string"}}}}}},
		},
		"PATCH /api/admin/models/{id}": {
			Tool: "ee_models", Action: "update_model", Summary: "改模型的对外名、上游名、显示名，或启停", En: "Update a model", Effect: wr,
			Params: []agentParam{pID, {Name: "name", Type: "string"}, {Name: "upstream_model", Type: "string"}, {Name: "display_name", Type: "string"}, {Name: "enabled", Type: "boolean"}},
		},
		"DELETE /api/admin/models/{id}": {
			Tool: "ee_models", Action: "delete_model", Summary: "删除对外模型", En: "Delete a model", Effect: dg,
			Params: []agentParam{pID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				m, err := s.store.GetModel(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的模型", argID(args))
				}
				return fmt.Sprintf("将删除对外模型「%s」（上游 %s / %s），调用它的人会立刻失败。", m.Name, m.ProviderName, m.UpstreamModel), nil
			},
		},

		// ---- ee_admin_tokens ----
		"GET /api/admin/tokens": {
			Tool: "ee_admin_tokens", Action: "list", Summary: "查全员令牌（不含明文）", Effect: rd,
			Params: []agentParam{
				{Name: "user_id", Type: "integer", In: "query"}, {Name: "department", Type: "string", In: "query"},
				{Name: "source", Type: "string", In: "query", Desc: "device / manual / admin / legacy"},
				{Name: "client_name", Type: "string", In: "query"}, {Name: "status", Type: "string", In: "query", Enum: []string{"active", "revoked"}}, pLimit30,
			},
			Shape: listShape("tokens", "id", "kind", "username", "department", "account_type", "name", "prefix", "source", "client_name", "scopes", "month_used", "monthly_token_limit", "expires_at", "last_used_at", "revoked_at", "admin_until"),
		},
		"POST /api/admin/tokens/revoke": {
			Tool: "ee_admin_tokens", Action: "revoke", Summary: "吊销令牌：按 ids，或按账号 / 设备名 / 部门批量（至少给一个条件）", En: "Revoke tokens", Effect: dg,
			Params: []agentParam{
				{Name: "ids", Type: "array", Desc: "令牌 id，形如 at:12"},
				{Name: "user_id", Type: "integer"}, {Name: "client_name", Type: "string"}, {Name: "department", Type: "string"},
			},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				if ids, ok := args["ids"].([]any); ok && len(ids) > 0 {
					var names []string
					for _, raw := range ids {
						id, _ := raw.(string)
						var n int64
						if _, err := fmt.Sscanf(id, "at:%d", &n); err != nil {
							return "", fmt.Errorf("令牌 id 要写成 at:<数字>：%q", id)
						}
						t, err := s.store.AccessTokenByID(ctx, n)
						if err != nil {
							return "", fmt.Errorf("没有令牌 %s", id)
						}
						names = append(names, fmt.Sprintf("%s 的「%s」(%s…)", t.Username, t.Name, t.Prefix))
					}
					return fmt.Sprintf("将吊销 %d 个令牌：%s。用它们的 DSH 下次请求会要求重新登录。", len(names), strings.Join(names, "；")), nil
				}
				f := store.TokenRevokeFilter{}
				if v, ok := args["user_id"].(float64); ok {
					id := int64(v)
					f.UserID = &id
				}
				if v, ok := args["client_name"].(string); ok && v != "" {
					f.ClientName = &v
				}
				if v, ok := args["department"].(string); ok && v != "" {
					f.Department = &v
				}
				if f.Empty() {
					return "", fmt.Errorf("请指定要吊销的令牌 id、账号、设备名或部门")
				}
				n, err := s.store.CountTokensBy(ctx, f)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("按条件（%s）将吊销 %d 个有效令牌。", changes(args), n), nil
			},
		},

		// ---- ee_review ----
		"GET /api/admin/hub/review": {
			Tool: "ee_review", Action: "queue_assets", Summary: "待审核的技能、提示词、微应用版本", Effect: rd,
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				out := []map[string]any{}
				for _, it := range items(dec(body), "items") {
					o := map[string]any{"asset": pick(asMap(it["asset"]), "id", "kind", "slug", "title")}
					for _, k := range []string{"version", "publisher", "department", "created_at"} {
						o[k] = it[k]
					}
					if sp, ok := it["spec"].(map[string]any); ok {
						o["spec"] = map[string]any{"title": sp["title"], "description": sp["description"], "when_to_use": sp["when_to_use"], "body": clip(fmt.Sprint(sp["body"]), 1500), "persona": clip(fmt.Sprint(sp["persona"]), 1000)}
					}
					out = append(out, o)
				}
				scopes := []map[string]any{}
				for _, it := range items(dec(body), "scopes") {
					o := map[string]any{"asset": pick(asMap(it["asset"]), "id", "kind", "slug", "title")}
					for _, k := range []string{"scope", "requested", "publisher", "department", "created_at"} {
						o[k] = it[k]
					}
					scopes = append(scopes, o)
				}
				return map[string]any{"items": out, "scopes": scopes}
			},
		},
		"POST /api/admin/hub/scope-review": {
			Tool: "ee_review", Action: "decide_scope", Summary: "通过或驳回负责人扩大可用范围的申请（「自己用」的应用要分享给同事时）", En: "Review a scope change", Effect: dg,
			Params: []agentParam{{Name: "asset_id", Type: "integer", Required: true}, {Name: "decision", Type: "string", Required: true, Enum: []string{"approve", "reject"}, As: "action"}, {Name: "note", Type: "string", Desc: "驳回时写原因"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				f, _ := args["asset_id"].(float64)
				l, err := assetLabel(ctx, s, int64(f))
				if err != nil {
					return "", err
				}
				verb := map[string]string{"approve": "通过", "reject": "驳回"}[fmt.Sprint(args["decision"])]
				return fmt.Sprintf("将%s %s 扩大可用范围的申请。%s", verb, l, map[string]string{"approve": "通过后新范围内的人立即可见。", "reject": ""}[fmt.Sprint(args["decision"])]), nil
			},
		},
		"GET /api/admin/review": {
			Tool: "ee_review", Action: "queue_packages", Summary: "待审核的插件版本（含自动检查结果）", Effect: rd,
			Shape: func(_ *Principal, _ map[string]any, body []byte) any { return trimmed(body) },
		},
		"POST /api/admin/hub/review": {
			Tool: "ee_review", Action: "decide_asset", Summary: "通过或驳回一个技能 / 提示词 / 微应用版本", En: "Review a capability", Effect: dg,
			Params: []agentParam{{Name: "asset_id", Type: "integer", Required: true}, {Name: "asset_version", Type: "integer", Required: true, As: "version"}, {Name: "decision", Type: "string", Required: true, Enum: []string{"approve", "reject"}, As: "action"}, {Name: "note", Type: "string", Desc: "驳回时写原因"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				f, _ := args["asset_id"].(float64)
				l, err := assetLabel(ctx, s, int64(f))
				if err != nil {
					return "", err
				}
				verb := map[string]string{"approve": "通过并上架", "reject": "驳回"}[fmt.Sprint(args["decision"])]
				return fmt.Sprintf("将%s %s 的第 %v 版。%s", verb, l, args["asset_version"], map[string]string{"approve": "上架后范围内的人立即可见。", "reject": ""}[fmt.Sprint(args["decision"])]), nil
			},
		},
		"POST /api/admin/review": {
			Tool: "ee_review", Action: "decide_package", Summary: "通过 / 驳回 / 下架 / 恢复一个插件（version 为空作用于整个包；整包恢复只恢复上过架的版本）", En: "Review a plugin", Effect: dg,
			Params: []agentParam{{Name: "name", Type: "string", Required: true}, {Name: "version", Type: "string"}, {Name: "decision", Type: "string", Required: true, Enum: []string{"approve", "reject", "unpublish", "restore"}, As: "action"}, {Name: "note", Type: "string"}},
			Preview: func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (string, error) {
				v := fmt.Sprint(args["version"])
				if v == "<nil>" || v == "" {
					v = "（整个包）"
				}
				return fmt.Sprintf("将对插件 %v %s 执行「%v」。通过 / 恢复后员工可以安装，下架后装不了。", args["name"], v, args["decision"]), nil
			},
		},

		// ---- ee_hub_admin ----
		"PATCH /api/hub/{id}": {
			Tool: "ee_hub_admin", Action: "patch", Summary: "改能力的可用范围、下发方式（auto 自动生效 / optional 自选）、分类、精选，或上下架（下架要写 note）", En: "Change a capability", Effect: dg,
			Params: []agentParam{pID, {Name: "category", Type: "string"}, {Name: "category_id", Type: "integer", Desc: "受管分类 id，0 表示取消归类"}, {Name: "featured", Type: "boolean", Desc: "设为精选"}, {Name: "scope", Schema: scopeSchema}, {Name: "delivery", Type: "string", Enum: []string{"auto", "optional"}}, {Name: "status", Type: "string", Enum: []string{"active", "unpublished"}}, {Name: "note", Type: "string"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				l, err := assetLabel(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				msg := fmt.Sprintf("将修改 %s：%s。", l, changes(args, "id"))
				if args["delivery"] == "auto" {
					msg += "设为自动生效会把它推到范围内所有人的 DSH 里。"
				}
				if args["status"] == "unpublished" {
					msg += "下架后所有人的 DSH 立即失去它。"
				}
				return msg, nil
			},
		},
		"DELETE /api/hub/{id}": {
			Tool: "ee_hub_admin", Action: "delete", Summary: "删除一项能力（连同版本与订阅）", En: "Delete a capability", Effect: dg,
			Params: []agentParam{pID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				l, err := assetLabel(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("将永久删除 %s 及其全部版本，所有人的 DSH 立即失去它。", l), nil
			},
		},
		"PUT /api/admin/plugin-policies/{name...}": {
			Tool: "ee_hub_admin", Action: "plugin_policy", Summary: "设置某个插件的可用范围与下发方式（auto 会在范围内员工的 DSH 里自动安装）", En: "Set plugin policy", Effect: dg,
			Params:  []agentParam{{Name: "name", Type: "string", Required: true, In: "path", Desc: "包名"}, {Name: "scope", Schema: scopeSchema, Required: true}, {Name: "delivery", Type: "string", Required: true, Enum: []string{"auto", "optional"}}},
			Preview: generic("设置插件策略", "name"),
		},

		// ---- ee_connections_admin ----
		"POST /api/admin/connections/test": {
			Tool: "ee_connections_admin", Action: "test", Summary: "用已存的配置与凭据试连一个企业连接，返回能发现的工具", Effect: rd,
			Params: []agentParam{{Name: "id", Type: "integer", Required: true, In: "local", Desc: "连接的能力 id"}},
			Body: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (map[string]any, error) {
				lv, err := s.store.LiveAssetVersion(ctx, argID(args))
				if err != nil {
					return nil, fmt.Errorf("没有 id 为 %d 的上架连接", argID(args))
				}
				return map[string]any{"id": argID(args), "spec": json.RawMessage(lv.Spec)}, nil
			},
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				m := dec(body)
				out := pick(m, "ok", "message", "latency_ms", "error")
				tools := []map[string]any{}
				for _, t := range items(m, "tools") {
					tools = append(tools, pick(t, "name", "description", "read_only"))
				}
				out["tools"] = tools
				return out
			},
		},
		"POST /api/admin/connections/{id}/sync": {
			Tool: "ee_connections_admin", Action: "sync", Summary: "重新从上游同步连接的工具清单", En: "Resync connection tools", Effect: wr,
			Params: []agentParam{pID},
		},
		"PATCH /api/admin/connections/{id}/tools/{name}": {
			Tool: "ee_connections_admin", Action: "tool", Summary: "开关某个工具，并可限定只有哪些角色能用（roles 为空表示范围内的人都能用）", En: "Change a connection tool", Effect: dg,
			Params: []agentParam{pID, {Name: "name", Type: "string", Required: true, In: "path", Desc: "工具名"}, {Name: "enabled", Type: "boolean", Required: true}, {Name: "roles", Type: "array", Items: "integer"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				l, err := assetLabel(ctx, s, argID(args))
				if err != nil {
					return "", err
				}
				msg := fmt.Sprintf("将%s %s 的工具 %v", map[bool]string{true: "启用", false: "停用"}[args["enabled"] == true], l, args["name"])
				if roles, ok := args["roles"].([]any); ok && len(roles) > 0 {
					msg += "，限定角色：" + roleLabels(ctx, s, roles)
				}
				msg += "。启用会修改数据的工具意味着员工的 Agent 可以对企业系统做写操作。"
				return msg, nil
			},
		},

		// ---- ee_registry ----
		"GET /api/admin/registry": {
			Tool: "ee_registry", Action: "overview", Summary: "npm 源总览：包数、下载、路由与上游状态", Effect: rd, Shape: trimShape,
		},
		"POST /api/admin/registry/check": {
			Tool: "ee_registry", Action: "check", Summary: "检测上游 npm 源是否可达", En: "Check the upstream registry", Effect: wr,
		},
		"GET /api/admin/packages": {
			Tool: "ee_registry", Action: "packages", Summary: "插件库：全部状态的包（可按关键词、状态筛）", Effect: rd,
			Params: []agentParam{pQ, {Name: "status", Type: "string", In: "local", Enum: []string{"published", "pending", "unpublished"}}, pLimit30},
			Shape: func(_ *Principal, args map[string]any, body []byte) any {
				q, st := argStr(args, "q"), argStr(args, "status")
				limit := argLimit(args, 30, 100)
				out := []map[string]any{}
				total := 0
				for _, it := range items(dec(body), "plugins") {
					if st != "" && it["status"] != st {
						continue
					}
					if q != "" && !contains(fmt.Sprint(it["name"], " ", it["title"], " ", it["description"]), q) {
						continue
					}
					total++
					if len(out) < limit {
						out = append(out, pick(it, "name", "title", "kind", "category", "version", "status", "pending", "installs", "origin", "owner", "has_install_script"))
					}
				}
				return map[string]any{"total": total, "plugins": out}
			},
		},
		"GET /api/admin/packages/{name...}": {
			Tool: "ee_registry", Action: "package", Summary: "看一个插件的完整信息（版本、自动检查结果、依赖）", Effect: rd,
			Params: []agentParam{{Name: "name", Type: "string", Required: true, In: "path"}},
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				m := dec(body)
				out := pick(m, "name", "title", "description", "kind", "category", "version", "status", "origin", "owner", "has_install_script", "dist_tags", "dependencies", "engines", "scan")
				out["readme"] = clip(fmt.Sprint(m["readme"]), 1500)
				vs := []map[string]any{}
				for _, v := range items(m, "versions") {
					vs = append(vs, pick(v, "version", "status", "publisher", "reviewer", "created_at"))
				}
				out["versions"] = vs
				return trimValue(out, 0)
			},
		},
		"PATCH /api/admin/packages/{name...}": {
			Tool: "ee_registry", Action: "edit_package", Summary: "改插件在插件市场的展示信息", En: "Edit plugin display info", Effect: wr,
			Params: []agentParam{{Name: "name", Type: "string", Required: true, In: "path"}, {Name: "title", Type: "string"}, {Name: "description", Type: "string"}, {Name: "category", Type: "string"}, {Name: "kind", Type: "string"}, {Name: "icon", Type: "string"}, {Name: "featured", Type: "boolean"}},
		},
		"GET /api/admin/upstream/feed": {
			Tool: "ee_registry", Action: "upstream_feed", Summary: "上游插件目录（dshfind），可按关键词筛", Effect: rd,
			Params: []agentParam{pQ, pLimit30},
			Shape: func(_ *Principal, args map[string]any, body []byte) any {
				q := argStr(args, "q")
				limit := argLimit(args, 30, 100)
				m := dec(body)
				out := []map[string]any{}
				total := 0
				for _, it := range items(m, "plugins") {
					if q != "" && !contains(fmt.Sprint(it["name"], " ", it["title"], " ", it["description"]), q) {
						continue
					}
					total++
					if len(out) < limit {
						o := pick(it, "name", "title", "version", "score", "stars", "preloaded", "status")
						o["description"] = clip(fmt.Sprint(it["description"]), 100)
						out = append(out, o)
					}
				}
				return map[string]any{"total": total, "plugins": out, "error": m["error"]}
			},
		},
		"POST /api/admin/upstream/sync": {
			Tool: "ee_registry", Action: "sync", Summary: "预载上游插件及其依赖到企业源；publish=true 需要 plugins.manage，且自动检查 fail 的包仍进待审核", En: "Sync upstream plugins", Effect: dg,
			Params: []agentParam{{Name: "names", Type: "array", Required: true, Desc: "包名"}, {Name: "publish", Type: "boolean"}},
			Preview: func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (string, error) {
				names, _ := args["names"].([]any)
				msg := fmt.Sprintf("将从上游预载 %d 个插件及其依赖：%s。", len(names), clip(fmt.Sprint(names), 200))
				if args["publish"] == true {
					msg += "会直接上架，员工立刻能装；需要 plugins.manage 权限，自动检查 fail 的包仍会进入待审核。"
				} else {
					msg += "预载后进入待审核。"
				}
				return msg, nil
			},
		},
		"GET /api/admin/upstream/jobs": {
			Tool: "ee_registry", Action: "jobs", Summary: "同步任务列表", Effect: rd, Shape: trimShape,
		},
		"GET /api/admin/upstream/jobs/{id}": {
			Tool: "ee_registry", Action: "job", Summary: "看一个同步任务的进度与结果", Effect: rd,
			Params: []agentParam{pID}, Shape: trimShape,
		},
		"POST /api/admin/upstream/check-updates": {
			Tool: "ee_registry", Action: "check_updates", Summary: "检查已预载插件在上游有没有新版本", En: "Check upstream updates", Effect: wr, Shape: trimShape,
		},

		// ---- ee_settings ----
		"GET /api/settings": {
			Tool: "ee_settings", Action: "get", Summary: "读系统设置（站点、npm 源、上游、DSH 接入、Agent 开关）", Effect: rd, Shape: trimShape,
		},
		"PUT /api/admin/settings": {
			Tool: "ee_settings", Action: "put", Summary: "改系统设置：changes 是要改的项，按设置的层级写，如 {\"connect\":{\"defaultModel\":\"chat\"}}；auth、scope 只能在网页改。" +
				"换 DSH 品牌外观写 brand：enabled、name（侧栏名称）、title（首屏标题）、logoShape（circle/rounded/plain）、" +
				"theme.colors.<accent|accentText|accentSoft|sidebar|sidebarActive|background|input>={light,dark}（CSS 颜色，dark 留空则深色模式用默认），" +
				"theme.tokens.<--dsw-变量名>={light,dark} 直接覆盖 DSH 设计变量；某个颜色设为 null 即删除。logo 只能在网页上传", En: "Change system settings", Effect: dg,
			Params: []agentParam{{Name: "changes", Required: true, Schema: map[string]any{"type": "object", "description": "按 ee_settings.get 返回的层级写要改的项"}}},
			Body:   settingsBody,
			Preview: func(ctx context.Context, s *Server, p *Principal, args map[string]any) (string, error) {
				cur, err := settingsMap(ctx, s)
				if err != nil {
					return "", err
				}
				after, err := settingsBody(ctx, s, p, args)
				if err != nil {
					return "", err
				}
				lines := diffLines(cur, after, "")
				if len(lines) == 0 {
					return "这些改动与当前设置一致，不会有变化。", nil
				}
				return "将修改系统设置：" + strings.Join(lines, "；") + "。", nil
			},
		},
		"GET /api/admin/connect": {
			Tool: "ee_settings", Action: "connect_status", Summary: "DSH 接入插件的发布状态", Effect: rd,
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				m := dec(body)
				return map[string]any{"state": m["state"], "package": m["package"], "stale": m["stale"]}
			},
		},
		"POST /api/admin/connect/publish": {
			Tool: "ee_settings", Action: "connect_publish", Summary: "立即生成并发布 DSH 接入插件（内容没变时不发，force=true 强制）", En: "Publish the DSH connect plugin", Effect: dg,
			Params:  []agentParam{{Name: "force", Type: "boolean"}},
			Preview: generic("发布新版本的 DSH 接入插件，员工的 DSH 下次检查更新时会升级"),
		},
		"GET /api/admin/auth/ldap": {
			Tool: "ee_settings", Action: "ldap_get", Summary: "读 LDAP / AD 配置（不含绑定密码）与最近一次同步结果", Effect: rd,
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				m := dec(body)
				for _, k := range ldapSecretKeys {
					delete(m, k)
				}
				return trimValue(m, 0)
			},
		},
		"PUT /api/admin/auth/ldap": {
			Tool: "ee_settings", Action: "ldap_put", Summary: "改 LDAP 配置：changes 是要改的项（键名同 ldap_get）；绑定密码只能在网页填", En: "Change the LDAP config", Effect: dg,
			Params: []agentParam{{Name: "changes", Required: true, Schema: map[string]any{"type": "object"}}},
			Body:   ldapBody,
			Preview: func(ctx context.Context, s *Server, p *Principal, args map[string]any) (string, error) {
				cur, err := ldapBase(ctx, s)
				if err != nil {
					return "", err
				}
				after, err := ldapBody(ctx, s, p, args)
				if err != nil {
					return "", err
				}
				lines := diffLines(cur, after, "")
				if len(lines) == 0 {
					return "这些改动与当前 LDAP 配置一致。", nil
				}
				return "将修改 LDAP 配置：" + strings.Join(lines, "；") + "。登录方式与组到角色的映射会随之变化。", nil
			},
		},
		"POST /api/admin/auth/ldap/test": {
			Tool: "ee_settings", Action: "ldap_test", Summary: "用已存的 LDAP 配置试连与绑定（不验证具体用户密码）", Effect: rd,
			Body: func(ctx context.Context, s *Server, _ *Principal, _ map[string]any) (map[string]any, error) {
				return ldapBase(ctx, s)
			},
			Shape: trimShape,
		},
		"POST /api/admin/auth/ldap/sync": {
			Tool: "ee_settings", Action: "ldap_sync", Summary: "立即按目录同步用户与角色", En: "Sync users from LDAP", Effect: dg,
			Preview: generic("按 LDAP 目录同步用户资料与角色映射，可能新建、停用用户或改动角色"),
			Shape:   trimShape,
		},

		// ---- 管理员向导（阶段 D） ----
		"GET /api/admin/setup-status": {
			Tool: "ee_settings", Action: "setup_status", Summary: "首次部署清单：访问地址、供应商、模型、默认模型、额度、LDAP、角色、连接、接入插件，每步是否完成与下一步怎么做；附员工接入命令与通知文案", Effect: rd,
		},
		"GET /api/admin/attention": {
			Tool: "ee_stats", Action: "attention", Summary: "日常巡检：今天有什么要处理的（待审核、连接申请、额度将耗尽、同步失败、连接 / 供应商异常）", Effect: rd,
		},
		"GET /api/admin/review/inspect": {
			Tool: "ee_review", Action: "inspect_package", Summary: "审核插件时看内容：文件清单、安装脚本、依赖、疑似密钥、可疑点；给 file 读单个文本文件", Effect: rd,
			Params: []agentParam{{Name: "name", Type: "string", Required: true, In: "query"}, {Name: "version", Type: "string", Required: true, In: "query"}, {Name: "file", Type: "string", In: "query", Desc: "包内相对路径，如 dist/index.js"}},
			Shape:  func(_ *Principal, _ map[string]any, body []byte) any { return trimValue(dec(body), 0) },
		},
		"GET /api/admin/hub/inspect": {
			Tool: "ee_review", Action: "inspect_asset", Summary: "审核技能 / 提示词 / 微应用时看全文与可疑点，并附线上版本对照", Effect: rd,
			Params: []agentParam{{Name: "asset_id", Type: "integer", Required: true, In: "query"}, {Name: "asset_version", Type: "integer", Required: true, In: "query", As: "version"}},
			Shape:  func(_ *Principal, _ map[string]any, body []byte) any { return dec(body) },
		},
		"GET /api/admin/connection-requests": {
			Tool: "ee_connections_admin", Action: "requests", Summary: "员工提交的待处理连接申请（谁、要哪个连接、理由）", Effect: rd,
		},
		"POST /api/admin/connection-requests/{id}": {
			Tool: "ee_connections_admin", Action: "resolve", Summary: "通过或驳回一条连接申请；通过会把申请人加进该连接的可用范围", En: "Resolve a connection request", Effect: dg,
			Params: []agentParam{pID, {Name: "decision", Type: "string", Required: true, Enum: []string{"approve", "reject"}}, {Name: "note", Type: "string"}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				pr, err := s.store.GetPendingAccessRequest(ctx, argID(args))
				if err != nil || pr.Kind != store.RequestConnection {
					return "", fmt.Errorf("没有待处理的申请 #%d，先用 requests 查看", argID(args))
				}
				if args["decision"] == "approve" {
					return fmt.Sprintf("将通过 %s（%s）对连接「%s」的申请（理由：%s）：申请人会被加进该连接的可用范围，之后能用其中其角色允许的工具。", pr.Username, pr.Department, pr.Title, pr.Reason), nil
				}
				return fmt.Sprintf("将驳回 %s 对连接「%s」的申请（理由：%s）。", pr.Username, pr.Title, pr.Reason), nil
			},
		},

		// ---- ee_audit / ee_stats ----
		"GET /api/admin/audit": {
			Tool: "ee_audit", Action: "query", Summary: "查审计日志（新的在前；用 before 翻页，值取上一页最小的 id）", Effect: rd,
			Params: []agentParam{
				{Name: "event", Type: "string", In: "query", As: "action", Desc: "审计动作，或以 . 结尾的前缀如 user."},
				{Name: "actor", Type: "integer", In: "query", Desc: "操作人用户 id"}, {Name: "q", Type: "string", In: "query", Desc: "在操作人、对象里搜"},
				{Name: "via", Type: "string", In: "query", Enum: []string{"web", "token", "agent", "system"}},
				{Name: "since", Type: "string", In: "query", Desc: "RFC 3339"}, {Name: "until", Type: "string", In: "query", Desc: "RFC 3339"},
				{Name: "before", Type: "integer", In: "query"}, {Name: "limit", Type: "integer", In: "query", Desc: "默认 30"},
			},
			Shape: func(_ *Principal, _ map[string]any, body []byte) any {
				out := []map[string]any{}
				for _, l := range items(dec(body), "logs") {
					o := pick(l, "id", "created_at", "actor_name", "action", "target", "via", "client", "ip")
					if d, ok := l["detail"].(map[string]any); ok {
						o["detail"] = trimValue(d, 0)
					}
					out = append(out, o)
				}
				return map[string]any{"logs": out}
			},
		},
		"GET /api/admin/stats": {
			Tool: "ee_stats", Action: "query", Summary: "运营统计：每日用量、活跃用户、安装、部门分布；group_by=token 按令牌", Effect: rd,
			Params: []agentParam{{Name: "days", Type: "integer", In: "query", Desc: "1–90，默认 14"}, {Name: "group_by", Type: "string", In: "query", Enum: []string{"token"}}},
			Shape:  trimShape,
		},
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

var scopeSchema = map[string]any{
	"type":        "object",
	"description": "可用范围：all=true 全员；否则 departments（部门名）、roles（角色 id）、users（用户 id）取并集",
	"properties": map[string]any{
		"all":         map[string]any{"type": "boolean"},
		"departments": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"roles":       map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"users":       map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
	},
}
