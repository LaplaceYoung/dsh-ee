package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/ldapauth/ldaptest"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const (
	ldapBase = "dc=acme,dc=local"
	ldapSvc  = "cn=svc,ou=system,dc=acme,dc=local"
	devsDN   = "cn=developers,ou=groups,dc=acme,dc=local"
)

func person(uid, cn, ou string, groups ...string) map[string][]string {
	return map[string][]string{"objectClass": {"person"}, "uid": {uid}, "cn": {cn}, "ou": {ou}, "mail": {strings.ToLower(uid) + "@acme.local"}, "memberOf": groups}
}

type meUser struct {
	store.User
	Permissions []string `json:"permissions"`
}

func roleIDs(rs []store.RoleRef) []int64 {
	out := []int64{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	slices.Sort(out)
	return out
}

func TestLDAPLogin(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	dir := ldaptest.Start(t)
	dir.Add(ldapSvc, "svc-secret", map[string][]string{"cn": {"svc"}})
	dir.Add("uid=alice,ou=people,"+ldapBase, "alice-dir-pw", person("Alice", "Alice Liu", "研发效能部", devsDN))
	dir.Add("uid=bob,ou=people,"+ldapBase, "bob-dir-pw", person("bob", "Bob", "财务部"))
	// 目录里有一个和本地管理员同名的人：不能借此登录本地管理员
	dir.Add("uid=admin,ou=people,"+ldapBase, "dir-admin-pw", person("admin", "Imposter", "外包"))

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("GET", "/api/auth/methods", nil, nil)
	expect(t, "登录方式", code, 200, body)
	if decodeAs[map[string]bool](t, body)["ldap"] {
		t.Fatal("还没配置 LDAP")
	}

	// 一个由组映射的角色：开发者 → 审核员（管理能力与审核）
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "reviewer", "name": "审核员", "permissions": []string{"plugins.manage"}}, nil)
	expect(t, "建角色", code, 201, body)
	reviewer := decodeAs[store.Role](t, body)
	code, body = admin.do("GET", "/api/admin/roles", nil, nil)
	expect(t, "角色列表", code, 200, body)
	var member store.Role
	for _, r := range decodeAs[struct{ Roles []store.Role }](t, body).Roles {
		if r.Code == "member" {
			member = r
		}
	}

	cfg := map[string]any{
		"enabled": true, "url": dir.URL, "bindDn": ldapSvc, "bindPassword": "svc-secret", "baseDn": ldapBase,
		"userFilter": "(&(objectClass=person)(uid={username}))", "usernameAttr": "uid", "displayNameAttr": "cn",
		"departmentAttr": "ou", "emailAttr": "mail", "groupMode": "memberOf",
		"roleMappings": []map[string]any{{"group": "developers", "roleId": reviewer.ID}},
		"autoCreate":   true, "syncDisable": true,
	}
	bad := map[string]any{}
	for k, v := range cfg {
		bad[k] = v
	}
	bad["userFilter"] = "(uid=alice)"
	code, body = admin.do("PUT", "/api/admin/auth/ldap", bad, nil)
	expect(t, "过滤条件没有 {username}", code, 400, body)

	// 保存前先试连，带一个账号
	test := map[string]any{"testUsername": "alice", "testPassword": "alice-dir-pw"}
	for k, v := range cfg {
		test[k] = v
	}
	code, body = admin.do("POST", "/api/admin/auth/ldap/test", test, nil)
	expect(t, "试连", code, 200, body)
	tr := decodeAs[struct {
		OK    bool
		Steps []struct {
			Step string
			OK   bool
		}
		Roles    []store.RoleRef
		Username string
	}](t, body)
	if !tr.OK || len(tr.Steps) != 3 || tr.Username != "alice" || len(tr.Roles) != 1 || tr.Roles[0].ID != reviewer.ID {
		t.Fatalf("试连结果不对：%s", body)
	}
	test["testPassword"] = "wrong"
	code, body = admin.do("POST", "/api/admin/auth/ldap/test", test, nil)
	expect(t, "试连错误密码", code, 200, body)
	if decodeAs[struct{ OK bool }](t, body).OK {
		t.Fatal("错误密码的试连应失败")
	}

	code, body = admin.do("PUT", "/api/admin/auth/ldap", cfg, nil)
	expect(t, "保存 LDAP 配置", code, 200, body)
	if strings.Contains(string(body), "svc-secret") || !strings.Contains(string(body), `"bindPasswordSet":true`) {
		t.Fatalf("服务账号密码不应返回：%s", body)
	}
	code, body = admin.do("GET", "/api/auth/methods", nil, nil)
	expect(t, "登录方式", code, 200, body)
	if !decodeAs[map[string]bool](t, body)["ldap"] {
		t.Fatal("开启后登录页应显示 LDAP")
	}

	// 目录用户首次登录：自动建号，资料与角色来自目录（用户名统一小写）
	alice := newClient(t, ts.URL)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "wrong"}, nil)
	expect(t, "目录密码错误", code, 401, body)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": ""}, nil)
	expect(t, "空密码", code, 401, body)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "Alice", "password": "alice-dir-pw"}, nil)
	expect(t, "目录用户登录", code, 200, body)
	code, body = alice.do("GET", "/api/auth/me", nil, nil)
	expect(t, "me", code, 200, body)
	me := decodeAs[meUser](t, body)
	if me.Username != "alice" || me.AuthSource != "ldap" || me.Department != "研发效能部" || me.DisplayName != "Alice Liu" || me.Email != "alice@acme.local" {
		t.Fatalf("目录资料没同步：%s", body)
	}
	if got, want := roleIDs(me.Roles), []int64{member.ID, reviewer.ID}; !slices.Equal(got, sortedIDs(want)) {
		t.Fatalf("角色应为员工 + 审核员：%v", got)
	}
	if strings.Contains(string(body), "password") {
		t.Fatalf("不应返回密码相关字段：%s", body)
	}
	code, body = alice.do("POST", "/api/auth/password", map[string]string{"old_password": "alice-dir-pw", "new_password": "something-new-1"}, nil)
	expect(t, "LDAP 账号不能在本地改密码", code, 400, body)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(me.ID), map[string]any{"password": "reset-password-1"}, nil)
	expect(t, "LDAP 账号不能重置密码", code, 400, body)

	// npm login 也走目录
	npm := newClient(t, ts.URL)
	code, body = npm.do("PUT", "/npm/-/user/org.couchdb.user:alice", map[string]string{"name": "alice", "password": "alice-dir-pw"}, nil)
	expect(t, "npm login（LDAP）", code, 201, body)
	code, body = npm.do("PUT", "/npm/-/user/org.couchdb.user:alice", map[string]string{"name": "alice", "password": "nope"}, nil)
	expect(t, "npm login 错误密码", code, 401, body)

	// 本地管理员仍用本地密码；目录里同名的人用目录密码登不上
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "dir-admin-pw"}, nil)
	expect(t, "目录同名用户冒用本地管理员", code, 401, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "LDAP 开启后本地管理员仍能登录", code, 200, body)

	// 移出 developers 组：再登录时失去映射的角色，管理员手动给的角色保留
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "keys", "name": "key 管理员", "permissions": []string{"keys.manage"}}, nil)
	expect(t, "建角色", code, 201, body)
	manual := decodeAs[store.Role](t, body)
	code, body = admin.do("PUT", "/api/admin/users/"+itoa(me.ID)+"/roles", map[string]any{"role_ids": []int64{member.ID, reviewer.ID, manual.ID}}, nil)
	expect(t, "手动加角色", code, 200, body)
	dir.Set("uid=alice,ou=people,"+ldapBase, "memberOf")
	dir.Set("uid=alice,ou=people,"+ldapBase, "ou", "平台部")
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-dir-pw"}, nil)
	expect(t, "再次登录", code, 200, body)
	code, body = alice.do("GET", "/api/auth/me", nil, nil)
	expect(t, "me", code, 200, body)
	me = decodeAs[meUser](t, body)
	if got := roleIDs(me.Roles); !slices.Equal(got, sortedIDs([]int64{member.ID, manual.ID})) {
		t.Fatalf("离开组后应失去审核员、保留手动角色：%v", got)
	}
	if me.Department != "平台部" {
		t.Fatalf("部门应同步为平台部：%s", me.Department)
	}

	// 只有系统设置权限、没有角色管理权限的人：能改连接，不能改组映射
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "ops", "name": "运维", "permissions": []string{"settings.manage"}}, nil)
	expect(t, "建角色", code, 201, body)
	ops := decodeAs[store.Role](t, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "ops", "password": "ops-password-1", "role_ids": []int64{ops.ID}}, nil)
	expect(t, "建运维", code, 201, body)
	opsC := newClient(t, ts.URL)
	code, body = opsC.do("POST", "/api/auth/login", map[string]string{"username": "ops", "password": "ops-password-1"}, nil)
	expect(t, "运维登录", code, 200, body)
	escalate := map[string]any{}
	for k, v := range cfg {
		escalate[k] = v
	}
	delete(escalate, "bindPassword")
	escalate["roleMappings"] = []map[string]any{{"group": "developers", "roleId": reviewer.ID}, {"group": "ops", "roleId": 1}}
	code, body = opsC.do("PUT", "/api/admin/auth/ldap", escalate, nil)
	expect(t, "无角色权限时改映射", code, 403, body)
	escalate["roleMappings"] = cfg["roleMappings"]
	escalate["timeoutSeconds"] = 5
	code, body = opsC.do("PUT", "/api/admin/auth/ldap", escalate, nil)
	expect(t, "不改映射时可以保存（沿用已存的服务账号密码）", code, 200, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-dir-pw"}, nil)
	expect(t, "沿用服务账号密码后仍能登录", code, 200, body)

	// 员工不能读 LDAP 配置
	code, body = alice.do("GET", "/api/admin/auth/ldap", nil, nil)
	expect(t, "员工读 LDAP 配置", code, 403, body)

	// 目录同步：bob 离职后被禁用；搜索起点写错时整次放弃
	dir.Remove("uid=bob,ou=people," + ldapBase)
	typo := map[string]any{}
	for k, v := range escalate {
		typo[k] = v
	}
	typo["baseDn"] = "dc=typo,dc=local"
	// 改搜索起点属于换连接目标：按保存路径的信任边界要重新输入服务账号密码
	typo["bindPassword"] = "svc-secret"
	code, body = admin.do("PUT", "/api/admin/auth/ldap", typo, nil)
	expect(t, "写错搜索起点", code, 200, body)
	code, body = admin.do("POST", "/api/admin/auth/ldap/sync", nil, nil)
	expect(t, "同步（配置错）", code, 200, body)
	if r := decodeAs[struct {
		OK       bool
		Disabled []string
	}](t, body); r.OK || len(r.Disabled) != 0 {
		t.Fatalf("搜索起点错时不应禁用任何人：%s", body)
	}
	revert := map[string]any{}
	for k, v := range escalate {
		revert[k] = v
	}
	revert["bindPassword"] = "svc-secret"
	code, body = admin.do("PUT", "/api/admin/auth/ldap", revert, nil)
	expect(t, "改回", code, 200, body)
	code, body = admin.do("POST", "/api/admin/auth/ldap/sync", nil, nil)
	expect(t, "同步", code, 200, body)
	if r := decodeAs[struct {
		OK       bool
		Disabled []string
	}](t, body); !r.OK || !slices.Equal(r.Disabled, []string{"bob"}) {
		t.Fatalf("应禁用 bob：%s", body)
	}
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-dir-pw"}, nil)
	expect(t, "被禁用后不能登录", code, 401, body)

	// 关闭自动建号：目录里有、本地没有的人得到明确的提示
	dir.Add("uid=carol,ou=people,"+ldapBase, "carol-pw", person("carol", "Carol", "法务部"))
	escalate["autoCreate"] = false
	code, body = admin.do("PUT", "/api/admin/auth/ldap", escalate, nil)
	expect(t, "关闭自动建号", code, 200, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": "carol-pw"}, nil)
	expect(t, "未开通", code, 403, body)
	// 管理员预先建 LDAP 账号（不设密码）后就能登录
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "carol", "auth_source": "ldap"}, nil)
	expect(t, "预建 LDAP 账号", code, 201, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "carol", "password": "carol-pw"}, nil)
	expect(t, "预建后登录", code, 200, body)

	// 关闭 LDAP：目录账号不能再登录，本地账号不受影响
	escalate["enabled"] = false
	code, body = admin.do("PUT", "/api/admin/auth/ldap", escalate, nil)
	expect(t, "关闭 LDAP", code, 200, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-dir-pw"}, nil)
	expect(t, "关闭后目录账号登录", code, 401, body)
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	if decodeAs[map[string]any](t, body)["auth"] != "local" {
		t.Fatalf("关闭 LDAP 后认证方式应回到 local：%s", body)
	}

	// 审计日志里有登录成功、失败与配置变更
	code, body = admin.do("GET", "/api/admin/audit?limit=200", nil, nil)
	expect(t, "审计日志", code, 200, body)
	seen := map[string]int{}
	for _, l := range decodeAs[struct{ Logs []store.AuditLog }](t, body).Logs {
		seen[l.Action]++
		if strings.Contains(string(mustJSON(l.Detail)), "secret") {
			t.Fatalf("审计日志里不应有密码：%+v", l)
		}
	}
	for _, a := range []string{"auth.login", "auth.login_failed", "ldap.update", "ldap.sync", "user.update", "user.roles"} {
		if seen[a] == 0 {
			t.Errorf("审计日志里缺少 %s：%v", a, seen)
		}
	}
	code, body = alice.do("GET", "/api/admin/audit", nil, nil)
	expect(t, "员工读审计日志", code, 403, body)
}

func sortedIDs(xs []int64) []int64 {
	out := append([]int64{}, xs...)
	slices.Sort(out)
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
