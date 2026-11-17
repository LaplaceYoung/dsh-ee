package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/ldapauth/ldaptest"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// ldapSavedView 是 GET /api/admin/auth/ldap 里本用例关心的部分。
type ldapSavedView struct {
	URL             string `json:"url"`
	BindDN          string `json:"bindDn"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	Enabled         bool   `json:"enabled"`
	BindPasswordSet bool   `json:"bindPasswordSet"`
}

func getLDAPView(t *testing.T, c *client) ldapSavedView {
	t.Helper()
	code, body := c.do("GET", "/api/admin/auth/ldap", nil, nil)
	expect(t, "读 LDAP 配置", code, 200, body)
	return decodeAs[ldapSavedView](t, body)
}

// expectLDAPConnectOK 用页面上「已保存配置（不带密码）」试连：能过 connect 一步说明
// 存储里的服务账号密码仍可解出，并且被目录接受。
func expectLDAPConnectOK(t *testing.T, c *client, input map[string]any) {
	t.Helper()
	code, body := c.do("POST", "/api/admin/auth/ldap/test", input, nil)
	expect(t, "用已存密码试连", code, 200, body)
	r := decodeAs[struct {
		OK    bool `json:"ok"`
		Steps []struct {
			Step string `json:"step"`
			OK   bool   `json:"ok"`
		} `json:"steps"`
	}](t, body)
	if !r.OK || len(r.Steps) != 1 || r.Steps[0].Step != "connect" || !r.Steps[0].OK {
		t.Fatalf("用已保存的服务账号密码试连失败：%s", body)
	}
}

func ldapConfigWithoutPassword(cfg map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cfg {
		if k != "bindPassword" {
			out[k] = v
		}
	}
	return out
}

func ldapConfigPatch(cfg map[string]any, kv map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cfg {
		out[k] = v
	}
	for k, v := range kv {
		out[k] = v
	}
	return out
}

// TestLDAPSaveDoesNotReuseStoredPasswordOnChangedTarget 覆盖「保存 LDAP 配置」的信任边界：
// 请求不带 bindPassword 时后端会沿用已存的服务账号密码；如果同一次保存还把连接目标换成
// 请求里指定的地址/账号/信道，旧密码就会被重新加密保存到新目标，之后 test/sync/login 都
// 会把它发给新目标（与 #210 的测试连接同源的 secret 外带，只是触发点在保存路径）。
func TestLDAPSaveDoesNotReuseStoredPasswordOnChangedTarget(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// 已保存的目录：服务账号密码 svc-secret。
	dir := ldaptest.Start(t)
	dir.Add(ldapSvc, "svc-secret", map[string][]string{"cn": {"svc"}})
	// 请求想换到的新目录：服务账号 DN 相同，但密码是 other-secret。
	otherDir := ldaptest.Start(t)
	otherDir.Add(ldapSvc, "other-secret", map[string][]string{"cn": {"svc"}})

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	cfg := map[string]any{
		"enabled": true, "url": dir.URL, "bindDn": ldapSvc, "bindPassword": "svc-secret",
		"baseDn": ldapBase, "userFilter": "(&(objectClass=person)(uid={username}))",
		"usernameAttr": "uid", "displayNameAttr": "cn", "departmentAttr": "ou", "emailAttr": "mail",
		"groupMode": "memberOf", "roleMappings": []map[string]any{},
		"autoCreate": true, "syncDisable": true, "timeoutSeconds": 2,
	}
	code, body = admin.do("PUT", "/api/admin/auth/ldap", cfg, nil)
	expect(t, "保存 LDAP 配置", code, 200, body)
	saved := ldapConfigWithoutPassword(cfg)
	if got := getLDAPView(t, admin); got.URL != dir.URL || !got.BindPasswordSet {
		t.Fatalf("初始配置没保存好：%+v", got)
	}

	t.Run("改地址且未重新输入密码时拒绝并保持原配置", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"url": otherDir.URL}), nil)
		if code == 200 {
			got := getLDAPView(t, admin)
			t.Fatalf("改地址且未重新输入密码时应拒绝保存；实际保存成功：存储中 url=%q、bindPasswordSet=%v"+
				"（已存服务账号密码被重新加密保存到了新地址，之后 test/sync/login 会把它发给新目标）",
				got.URL, got.BindPasswordSet)
		}
		expect(t, "改地址且未重新输入密码", code, 400, body)
		if !strings.Contains(string(body), "服务账号密码") {
			t.Fatalf("400 文案应提示重新输入服务账号密码：%s", body)
		}
		// 拒绝后已存配置必须原样：地址没变、密码还在且仍能解出并被原目录接受。
		got := getLDAPView(t, admin)
		if got.URL != dir.URL || !got.BindPasswordSet {
			t.Fatalf("被拒绝的保存改动了已存配置：%+v", got)
		}
		expectLDAPConnectOK(t, admin, saved)
	})

	t.Run("改 BindDN 且未重新输入密码时拒绝", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"bindDn": "cn=other,ou=system," + ldapBase}), nil)
		expect(t, "改 BindDN 且未重新输入密码", code, 400, body)
		if got := getLDAPView(t, admin); got.BindDN != ldapSvc || !got.BindPasswordSet {
			t.Fatalf("被拒绝的保存改动了已存配置：%+v", got)
		}
	})

	t.Run("改搜索起点且未重新输入密码时拒绝", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"baseDn": "dc=other,dc=local"}), nil)
		expect(t, "改搜索起点且未重新输入密码", code, 400, body)
	})

	t.Run("目标不变且未带密码时沿用已存密码", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap", saved, nil)
		expect(t, "目标不变、不带密码保存", code, 200, body)
		if !strings.Contains(string(body), `"bindPasswordSet":true`) {
			t.Fatalf("响应应保持 bindPasswordSet=true：%s", body)
		}
		expectLDAPConnectOK(t, admin, saved)
	})

	t.Run("只改超时与开关且未带密码时沿用已存密码", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"timeoutSeconds": 5}), nil)
		expect(t, "只改超时", code, 200, body)
		if got := getLDAPView(t, admin); got.TimeoutSeconds != 5 || !got.BindPasswordSet {
			t.Fatalf("超时没保存或密码丢了：%+v", got)
		}
		code, body = admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"enabled": false}), nil)
		expect(t, "关闭 LDAP", code, 200, body)
		code, body = admin.do("PUT", "/api/admin/auth/ldap", saved, nil)
		expect(t, "重新打开 LDAP", code, 200, body)
		expectLDAPConnectOK(t, admin, saved)
	})

	t.Run("换目标并显式输入密码时保存新密码", func(t *testing.T) {
		code, body := admin.do("PUT", "/api/admin/auth/ldap",
			ldapConfigPatch(saved, map[string]any{"url": otherDir.URL, "bindPassword": "other-secret"}), nil)
		expect(t, "换目标并显式输入密码", code, 200, body)
		got := getLDAPView(t, admin)
		if got.URL != otherDir.URL || !got.BindPasswordSet {
			t.Fatalf("新目标或新密码没保存：%+v", got)
		}
		moved := ldapConfigPatch(saved, map[string]any{"url": otherDir.URL})
		// 新密码必须生效：otherDir 的服务账号密码是 other-secret，旧密码 svc-secret 连不上。
		expectLDAPConnectOK(t, admin, moved)
	})
}
