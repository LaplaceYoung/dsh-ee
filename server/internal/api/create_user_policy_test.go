package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// 建账号时直接带上角色（role_ids）也是授角色：互斥角色、双人批准要和「改角色」一样生效，
// 否则开了双人批准的管理员可以自己建一个带管理角色的新账号（密码是他设的），绕过第二个人。
func TestCreateUserRolesFollowAdminPolicy(t *testing.T) {
	f := newPrincipalFixture(t)
	mkRole := func(code string, perms []string) int64 {
		status, body := f.admin.do("POST", "/api/admin/roles", map[string]any{"code": code, "name": code, "permissions": perms}, nil)
		expect(t, "建角色 "+code, status, http.StatusCreated, body)
		return decode[struct {
			ID int64 `json:"id"`
		}](t, body).ID
	}
	sysAdmin := mkRole("sys-admin", []string{"users.manage"})
	accessAdmin := mkRole("access-admin", []string{"fields.manage"})
	plain := mkRole("plain", []string{"portal.view"})

	policy := map[string]any{"super_disabled": false, "no_self_grant": false,
		"exclusive": [][]string{{"sys-admin", "access-admin"}}, "dual_approval": []string{"roles.assign"}}
	status, body := f.admin.do("PUT", "/api/admin/policy", policy, nil)
	expect(t, "开双人批准和互斥角色", status, http.StatusOK, body)

	create := func(name string, roles []int64) (int, string) {
		status, body := f.admin.do("POST", "/api/admin/users", map[string]any{"username": name, "password": name + "-password-1", "role_ids": roles}, nil)
		return status, string(body)
	}
	exists := func(name string) bool {
		_, body := f.admin.do("GET", "/api/admin/users", nil, nil)
		return strings.Contains(string(body), `"username":"`+name+`"`)
	}

	if status, body := create("shadow-admin", []int64{sysAdmin}); status == http.StatusCreated || exists("shadow-admin") {
		t.Fatalf("开了双人批准，建账号时不能直接带管理角色：%d %s", status, body)
	}
	if status, body := create("both-admin", []int64{sysAdmin, accessAdmin}); status != http.StatusForbidden || !strings.Contains(body, "互斥") || exists("both-admin") {
		t.Fatalf("互斥的两个角色不能一起给：%d %s", status, body)
	}
	if status, body := create("plain-user", []int64{plain}); status != http.StatusCreated {
		t.Fatalf("不含管理权限的角色照常可以在建账号时给：%d %s", status, body)
	}
}
