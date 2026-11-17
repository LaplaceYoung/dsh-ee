package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/ldapauth/ldaptest"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// captureDirectory 是一个只记录 TCP 连接与首包的假目录：不说 LDAP 协议，收到数据就断开。
// 用来证明「测试连接」没有把已存的服务账号密码 bind 到请求里指定的新地址上。
type captureDirectory struct {
	ln net.Listener

	mu    sync.Mutex
	conns int
	data  []byte
}

func newCaptureDirectory(t *testing.T) *captureDirectory {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听假目录: %v", err)
	}
	d := &captureDirectory{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go d.serve()
	return d
}

func (d *captureDirectory) serve() {
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			return
		}
		d.mu.Lock()
		d.conns++
		d.mu.Unlock()
		_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		d.mu.Lock()
		d.data = append(d.data, buf[:n]...)
		d.mu.Unlock()
		_ = conn.Close()
	}
}

func (d *captureDirectory) url() string { return "ldap://" + d.ln.Addr().String() }

func (d *captureDirectory) stats() (int, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conns, string(d.data)
}

// waitConns 等假目录收到 count 条连接（显式密码那条用例只要求「确实尝试连接了」）。
func (d *captureDirectory) waitConns(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := d.stats(); n >= count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	n, data := d.stats()
	t.Fatalf("假目录只收到 %d 条连接（期望 %d），首包 %q", n, count, data)
}

type ldapTestResponse struct {
	OK    bool `json:"ok"`
	Steps []struct {
		Step  string `json:"step"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	} `json:"steps"`
}

// TestLDAPTestDoesNotReuseStoredPasswordOnChangedTarget 覆盖「测试连接」的可信边界：
// 已存的服务账号密码只能用于已保存的目录目标；请求把连接目标换成另一个地址时不带密码
// 必须 400，绝不能回填已存密码去 bind 新地址（secret 外带 + SSRF 的利用链）。
func TestLDAPTestDoesNotReuseStoredPasswordOnChangedTarget(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	dir := ldaptest.Start(t)
	dir.Add(ldapSvc, "svc-secret", map[string][]string{"cn": {"svc"}})

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

	// 页面测试时提交的是当前表单：已保存配置不含密码，只有管理员重新输入时才带 bindPassword。
	saved := map[string]any{}
	for k, v := range cfg {
		if k != "bindPassword" {
			saved[k] = v
		}
	}
	req := func(patch map[string]any) map[string]any {
		m := map[string]any{}
		for k, v := range saved {
			m[k] = v
		}
		for k, v := range patch {
			m[k] = v
		}
		return m
	}

	attacker := newCaptureDirectory(t)

	t.Run("目标与已存配置一致时沿用已存密码", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test", req(nil), nil)
		expect(t, "测试已保存配置", code, 200, body)
		r := decodeAs[ldapTestResponse](t, body)
		if !r.OK || len(r.Steps) != 1 || r.Steps[0].Step != "connect" || !r.Steps[0].OK {
			t.Fatalf("应能用已存服务账号连上目录：%s", body)
		}
		if n, _ := attacker.stats(); n != 0 {
			t.Fatalf("测试已保存配置不应连到其它地址：连接数 %d", n)
		}
	})

	t.Run("改地址且不带密码时拒绝并不得外带已存密码", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test", req(map[string]any{"url": attacker.url()}), nil)
		// 先看秘密有没有真的被送出去：这是漏洞本体。连接数或首包里出现已存密码都算失败。
		if n, data := attacker.stats(); n != 0 || strings.Contains(data, "svc-secret") {
			t.Fatalf("换地址且未重新输入密码时，已存服务账号密码被发到请求指定的地址：连接数=%d 首包=%q", n, data)
		}
		expect(t, "改地址且不带密码", code, 400, body)
		if !strings.Contains(string(body), "服务账号密码") {
			t.Fatalf("400 文案应提示重新输入服务账号密码：%s", body)
		}
	})

	t.Run("改 BindDN 且不带密码时拒绝", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test",
			req(map[string]any{"bindDn": "cn=other,ou=system," + ldapBase}), nil)
		expect(t, "改 BindDN 且不带密码", code, 400, body)
	})

	t.Run("改 StartTLS 且不带密码时拒绝", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test", req(map[string]any{"startTls": true}), nil)
		expect(t, "改 StartTLS 且不带密码", code, 400, body)
	})

	t.Run("改搜索起点且不带密码时拒绝", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test", req(map[string]any{"baseDn": "dc=other,dc=local"}), nil)
		expect(t, "改搜索起点且不带密码", code, 400, body)
	})

	t.Run("只改与连接目标无关的字段仍沿用已存密码", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test", req(map[string]any{"timeoutSeconds": 3}), nil)
		expect(t, "改超时仍可测试", code, 200, body)
		if r := decodeAs[ldapTestResponse](t, body); !r.OK {
			t.Fatalf("只改超时不应要求重新输入密码：%s", body)
		}
	})

	t.Run("改地址但显式给了密码时允许测试", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test",
			req(map[string]any{"url": attacker.url(), "bindPassword": "typed-password"}), nil)
		expect(t, "改地址并显式给密码", code, 200, body)
		r := decodeAs[ldapTestResponse](t, body)
		if len(r.Steps) != 1 || r.Steps[0].Step != "connect" {
			t.Fatalf("显式密码时应真的去连请求里的地址：%s", body)
		}
		attacker.waitConns(t, 1)
		if _, data := attacker.stats(); strings.Contains(data, "svc-secret") {
			t.Fatalf("显式给了密码时不应再使用已存密码：首包=%q", data)
		}
	})

	t.Run("非法地址仍被现有校验拒绝", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/auth/ldap/test",
			req(map[string]any{"url": "http://127.0.0.1:389"}), nil)
		expect(t, "非 ldap(s) 地址", code, 400, body)
		if !strings.Contains(string(body), "目录地址") {
			t.Fatalf("应返回地址格式错误：%s", body)
		}
	})

	if strings.Contains(string(body), "svc-secret") {
		t.Fatalf("测试连接响应不应包含服务账号密码：%s", body)
	}
}
