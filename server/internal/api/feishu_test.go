package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/feishu"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

type fakeFeishuContacts struct {
	base       http.Handler
	department feishu.Department
	users      []feishu.User
	fail       bool
}

func (f *fakeFeishuContacts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/open-apis/auth/") {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-feishu"})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/open-apis/contact/") {
		if r.Header.Get("Authorization") != "Bearer tenant-feishu" {
			w.WriteHeader(401)
			return
		}
		if f.fail {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 40001, "msg": "missing scope"})
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/departments/parentchildren"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []feishu.Department{f.department}, "has_more": false}})
		case strings.Contains(r.URL.Path, "/users/find_by_department"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": f.users, "has_more": false}})
		default:
			http.NotFound(w, r)
		}
		return
	}
	f.base.ServeHTTP(w, r)
}

func TestFeishuLoginAndOrganizationSync(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: idp.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	contacts := &fakeFeishuContacts{base: idp.Config.Handler, department: feishu.Department{ID: "dep-rd", Name: "研发部"}, users: []feishu.User{
		{OpenID: "ou-1", Name: "吴安", Email: "alice@example.com", DepartmentIDs: []string{"dep-rd"}},
		{OpenID: "ou-2", Name: "柏博", Email: "bob@example.com", DepartmentIDs: []string{"dep-rd"}},
		{OpenID: "ou-3", Name: "柯若", Email: "carol@example.com", DepartmentIDs: []string{"dep-rd"}},
	}}
	contacts.users = append([]feishu.User(nil), contacts.users...)
	idp.Config.Handler = contacts

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "sales-lead", "name": "销售主管", "permissions": []string{"dashboard.view"}}, nil)
	expect(t, "创建部门映射角色", code, 201, body)
	roleID := decodeAs[struct{ ID int64 }](t, body).ID
	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "name": "飞书", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"scopes": "profile email", "usernameClaim": "preferred_username", "nameClaim": "name", "emailClaim": "email",
		"departmentClaim": "department", "groupsClaim": "groups", "autoCreate": true, "feishuApiBase": idp.URL,
		"roleMappings": []map[string]any{{"group": "dep-sales", "roleId": roleID}},
	}, nil)
	expect(t, "保存飞书统一登录", code, 200, body)
	if strings.Contains(string(body), "idp-secret") {
		t.Fatalf("不能回显 app secret：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
	expect(t, "首次同步", code, 200, body)
	var sync struct {
		OK         bool     `json:"ok"`
		Remote     int      `json:"remote"`
		Created    int      `json:"created"`
		Reconciled int      `json:"reconciled"`
		Disabled   []string `json:"disabled"`
	}
	if err := json.Unmarshal(body, &sync); err != nil || !sync.OK || sync.Remote != 3 || sync.Created != 3 || sync.Reconciled != 0 {
		t.Fatalf("首次组织同步结果不对：%s err=%v", body, err)
	}
	alice, err := st.GetUserByUsername(ctx, "alice")
	if err != nil || alice.AuthSource != store.AuthOIDC || alice.ExternalDN != "feishu:ou-1" || alice.Department != "研发部" {
		t.Fatalf("同步后的员工来源、open_id 或部门不对：%+v err=%v", alice, err)
	}

	// 真的走一遍 OIDC 回调，同一个 open_id 登录复用同步时已建账号。
	emp := newClient(t, ts.URL)
	loc := idp.login(t, emp, map[string]any{"sub": "ou-1", "preferred_username": "alice@example.com", "name": "吴安", "email": "alice@example.com"}, nil)
	if loc != "/build" {
		t.Fatalf("飞书身份回调失败：%s", loc)
	}
	code, body = emp.do("GET", "/api/auth/me", nil, nil)
	expect(t, "飞书登录后读取当前员工", code, 200, body)
	if !strings.Contains(string(body), `"username":"alice"`) || !strings.Contains(string(body), `"external_dn":"feishu:ou-1"`) {
		t.Fatalf("登录没有复用同步账号：%s", body)
	}

	// 组织变化只改资料与映射角色，不新增另一个账号。
	contacts.department = feishu.Department{ID: "dep-sales", Name: "销售部"}
	for i := range contacts.users {
		contacts.users[i].DepartmentIDs = []string{contacts.department.ID}
	}
	code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
	expect(t, "部门变化同步", code, 200, body)
	alice, err = st.GetUserByUsername(ctx, "alice")
	if err != nil || alice.Department != "销售部" {
		t.Fatalf("部门没有随目录更新：%+v err=%v", alice, err)
	}
	access, err := st.UserAccess(ctx, alice.ID)
	if err != nil || len(access.RoleIDs) != 2 {
		t.Fatalf("应有员工与映射角色两个角色：%+v err=%v", access, err)
	}

	// bob 从目录消失、carol 明确冻结，都应按同一开关禁用；alice 不受影响。
	contacts.users = append(contacts.users[:1], feishu.User{OpenID: "ou-3", Name: "柯若", Email: "carol@example.com", DepartmentIDs: []string{"dep-sales"}, Membership: struct {
		IsDeleted bool `json:"is_deleted"`
		IsFrozen  bool `json:"is_frozen"`
	}{IsDeleted: true}})
	code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
	expect(t, "目录缺席但尚未打开禁用", code, 200, body)
	if _, err := st.GetUserByUsername(ctx, "bob"); err != nil {
		t.Fatal(err)
	}
	settings := decodeAs[map[string]any](t, mustGet(t, admin, "/api/admin/auth/oidc"))
	settings["feishuSync"] = true
	settings["feishuSyncDisable"] = true
	delete(settings, "clientSecret")
	code, body = admin.do("PUT", "/api/admin/auth/oidc", settings, nil)
	expect(t, "打开目录同步与缺席禁用", code, 200, body)
	code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
	expect(t, "目录缺席禁用", code, 200, body)
	if err := json.Unmarshal(body, &sync); err != nil || len(sync.Disabled) != 2 {
		t.Fatalf("目录缺席和冻结的员工都应禁用：%s err=%v", body, err)
	}
	if slices.Sort(sync.Disabled); sync.Disabled[0] != "bob" || sync.Disabled[1] != "carol" {
		t.Fatalf("目录缺席的 bob 应被禁用：%s err=%v", body, err)
	}
	alice, err = st.GetUserByUsername(ctx, "alice")
	if err != nil || alice.Disabled {
		t.Fatalf("不应误禁用仍存在的 alice：%+v err=%v", alice, err)
	}

	// 目录响应坚决不用来代替禁用：整轮失败不应禁用 alice，错误不泄露 client secret。
	contacts.fail = true
	code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
	expect(t, "目录错误明确报告", code, 200, body)
	if !strings.Contains(string(body), "missing scope") || strings.Contains(string(body), "idp-secret") {
		t.Fatalf("目录错误处理不对：%s", body)
	}
	alice, err = st.GetUserByUsername(ctx, "alice")
	if err != nil || alice.Disabled {
		t.Fatalf("目录失败不应禁用 alice：%+v err=%v", alice, err)
	}
}
