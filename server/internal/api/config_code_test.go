package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// 配置即代码（config_code.go）不能绕过管理约束的权限：
// 直接 PUT /api/admin/policy 要 roles.manage，关掉超级管理员还要超级管理员本人；
// 走配置变更（提交 / 批准 / 生效）必须用同一套判定，不能靠 fields.manage 自批绕过。

// testConfigJSON 拼一份整份权限配置（JSON 也是 YAML 的子集）。
func testConfigJSON(policy map[string]any, fields []any) string {
	if fields == nil {
		fields = []any{}
	}
	b, err := json.Marshal(map[string]any{
		"version": 1,
		"policy":  policy,
		"roles":   []any{},
		"fields":  fields,
		"rules":   []any{},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// testAddRoleUser 建一个角色、一个带该角色的管理员并登录。
func testAddRoleUser(t *testing.T, f *principalFixture, username, code string, perms []string) *client {
	t.Helper()
	status, body := f.admin.do("POST", "/api/admin/roles", map[string]any{"code": code, "name": code, "permissions": perms}, nil)
	expect(t, "建角色 "+code, status, http.StatusCreated, body)
	roleID := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	password := username + "-password-1"
	status, body = f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "password": password, "role_ids": []int64{roleID},
	}, nil)
	expect(t, "建管理员 "+username, status, http.StatusCreated, body)
	c := newClient(t, f.ts.URL)
	status, body = c.do("POST", "/api/auth/login", map[string]string{"username": username, "password": password}, nil)
	expect(t, username+" 登录", status, http.StatusOK, body)
	return c
}

// testAllAdminPerms 是全部管理类权限点：普通角色持有它们之后，「关掉超级管理员」的检查才过得去。
func testAllAdminPerms() []string {
	var out []string
	for _, p := range rbac.Catalog {
		if rbac.IsAdminPerm(p.Code) {
			out = append(out, p.Code)
		}
	}
	return out
}

func testSubmitConfig(t *testing.T, c *client, note, doc string) (int, []byte) {
	t.Helper()
	return c.do("POST", "/api/admin/config/changes", map[string]any{"config": doc, "note": note}, nil)
}

func testApproveChange(t *testing.T, c *client, id int64) (int, []byte) {
	t.Helper()
	return c.do("POST", "/api/admin/config/changes/"+itoa(id), map[string]any{"approve": true}, nil)
}

func testChangeID(t *testing.T, body []byte) int64 {
	t.Helper()
	return decode[struct {
		ChangeRequest struct {
			ID int64 `json:"id"`
		} `json:"change_request"`
	}](t, body).ChangeRequest.ID
}

// testStoredPolicy 直接读库里的管理约束。关掉超级管理员之后管理接口就进不去了，不能用接口读。
func testStoredPolicy(t *testing.T, f *principalFixture) map[string]any {
	t.Helper()
	var out map[string]any
	ok, err := f.st.GetSettingJSON(context.Background(), "admin_policy", &out)
	if err != nil {
		t.Fatalf("读管理约束: %v", err)
	}
	if !ok {
		return map[string]any{}
	}
	return out
}

func testStoredBool(t *testing.T, f *principalFixture, key string) bool {
	t.Helper()
	v, _ := testStoredPolicy(t, f)[key].(bool)
	return v
}

func testWantErrContains(t *testing.T, what string, body []byte, want string) {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("%s：解析错误响应失败：%v；响应 %s", what, err, body)
	}
	if !strings.Contains(e.Error, want) {
		t.Fatalf("%s：错误文案应包含 %q，得到 %q；响应 %s", what, want, e.Error, body)
	}
}

// TestConfigChangePolicyNeedsRolesManage：只有 fields.manage 的管理员不能借配置变更改管理约束；
// 有 roles.manage 的可以；不涉及约束变化的正常配置变更不受影响。
func TestConfigChangePolicyNeedsRolesManage(t *testing.T) {
	f := newPrincipalFixture(t)
	fieldAdmin := testAddRoleUser(t, f, "cfg-fields-admin", "cfg-fields-only", []string{rbac.FieldsManage})
	policyAdmin := testAddRoleUser(t, f, "cfg-policy-admin", "cfg-policy-mgr", []string{rbac.FieldsManage, rbac.RolesManage})

	// 改约束（不改超级管理员开关）：只有 fields.manage 的管理员提交就 403。
	policyDoc := testConfigJSON(map[string]any{"super_disabled": false, "no_self_grant": true}, nil)
	status, body := testSubmitConfig(t, fieldAdmin, "改管理约束", policyDoc)
	expect(t, "fields.manage 提交改管理约束的配置", status, http.StatusForbidden, body)
	testWantErrContains(t, "fields.manage 提交改管理约束的配置", body, rbac.RolesManage)
	if testStoredBool(t, f, "no_self_grant") {
		t.Fatalf("被拒绝的配置不能把 no_self_grant 写进库")
	}

	// 同时有 roles.manage 的管理员提交、自己批准（没开双人批准，允许自批）→ 生效。
	status, body = testSubmitConfig(t, policyAdmin, "改管理约束", policyDoc)
	expect(t, "roles.manage 提交改管理约束的配置", status, http.StatusCreated, body)
	id := testChangeID(t, body)
	status, body = testApproveChange(t, policyAdmin, id)
	expect(t, "roles.manage 批准改管理约束的配置", status, http.StatusOK, body)
	if !testStoredBool(t, f, "no_self_grant") {
		t.Fatalf("批准后 no_self_grant 应该生效")
	}

	// 回滚也是一份整份配置：把约束改回去同样要 roles.manage，fields.manage 不能借回滚绕过去。
	status, body = fieldAdmin.do("POST", "/api/admin/config/changes/"+itoa(id)+"/rollback", nil, nil)
	expect(t, "fields.manage 回滚改约束的变更", status, http.StatusForbidden, body)
	testWantErrContains(t, "fields.manage 回滚改约束的变更", body, rbac.RolesManage)
	if !testStoredBool(t, f, "no_self_grant") {
		t.Fatalf("被拒绝的回滚不能改管理约束")
	}

	// 正常配置变更（只加字段，约束不变）：fields.manage 照原流程提交、批准、生效。
	fieldDoc := testConfigJSON(map[string]any{"super_disabled": false, "no_self_grant": true}, []any{
		map[string]any{"object": "document", "key": "cfg-tag", "label": "配置标签", "type": "text"},
	})
	status, body = testSubmitConfig(t, fieldAdmin, "加字段", fieldDoc)
	expect(t, "普通配置变更提交", status, http.StatusCreated, body)
	id = testChangeID(t, body)
	status, body = testApproveChange(t, fieldAdmin, id)
	expect(t, "普通配置变更批准", status, http.StatusOK, body)
	status, body = fieldAdmin.do("GET", "/api/admin/fields", nil, nil)
	expect(t, "读字段", status, http.StatusOK, body)
	if !strings.Contains(string(body), "cfg-tag") {
		t.Fatalf("普通配置变更应该生效（有 cfg-tag 字段）；响应 %s", body)
	}
}

// TestConfigChangePolicySuperGate：关掉超级管理员只能超级管理员本人做，提交和批准两道都要拦；
// 超级管理员自己做不会被新检查误伤。
func TestConfigChangePolicySuperGate(t *testing.T) {
	f := newPrincipalFixture(t)
	// 关超管之前要求每个管理权限点都有普通角色的人持有，先备好一个。
	testAddRoleUser(t, f, "cfg-holder", "cfg-holder-role", testAllAdminPerms())
	fieldAdmin := testAddRoleUser(t, f, "cfg-super-fields", "cfg-super-fields-only", []string{rbac.FieldsManage})

	superDoc := testConfigJSON(map[string]any{"super_disabled": true, "no_self_grant": false}, nil)

	// 只有 fields.manage 的管理员连提交都过不了。
	status, body := testSubmitConfig(t, fieldAdmin, "关超管", superDoc)
	expect(t, "fields.manage 提交关超管配置", status, http.StatusForbidden, body)
	testWantErrContains(t, "fields.manage 提交关超管配置", body, "只有超级管理员能关掉超级管理员")
	if testStoredBool(t, f, "super_disabled") {
		t.Fatalf("被拒绝的配置不能关掉超级管理员")
	}

	// 超级管理员提交的待批请求，fields.manage 管理员也不能批准（批准时才真正生效）。
	status, body = testSubmitConfig(t, f.admin, "关超管", superDoc)
	expect(t, "超级管理员提交关超管配置", status, http.StatusCreated, body)
	id := testChangeID(t, body)
	status, body = testApproveChange(t, fieldAdmin, id)
	expect(t, "fields.manage 批准关超管配置", status, http.StatusForbidden, body)
	testWantErrContains(t, "fields.manage 批准关超管配置", body, "只有超级管理员能关掉超级管理员")
	if testStoredBool(t, f, "super_disabled") {
		t.Fatalf("批准被拒的变更不能生效")
	}

	// 超级管理员自己批准：成功，约束真的生效。
	status, body = testApproveChange(t, f.admin, id)
	expect(t, "超级管理员批准关超管配置", status, http.StatusOK, body)
	if !testStoredBool(t, f, "super_disabled") {
		t.Fatalf("超级管理员批准后 super_disabled 应该打开；响应 %s", body)
	}
}
