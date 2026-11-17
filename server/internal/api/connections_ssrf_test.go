package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 连接测试不能被当成读云主机元数据、探链路本地地址的跳板：
// 这些都应在本地被拒绝（不是等拨号超时），且错误里说清原因。
func TestConnTestRejectsMetadataAndLinkLocal(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"云主机元数据 IPv4", "http://169.254.169.254/latest/meta-data/", "云主机元数据地址"},
		{"链路本地 IPv6", "http://[fe80::1]/mcp", "链路本地地址"},
		{"AWS IPv6 元数据", "http://[fd00:ec2::254]/latest/meta-data/", "云主机元数据地址"},
		{"阿里云元数据", "http://100.100.100.200/latest/meta-data/", "云主机元数据地址"},
	}
	for _, c := range cases {
		spec := map[string]any{"title": "不该连上的地址", "type": "mcp", "url": c.url, "auth": map[string]any{"type": "none"}}
		start := time.Now()
		code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"spec": spec, "slug": "ssrf-probe"}, nil)
		expect(t, c.name, code, 200, body)
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("%s：响应不是 JSON：%s", c.name, body)
		}
		if resp.OK || !strings.Contains(resp.Error, c.want) {
			t.Fatalf("%s：应在本地拒绝并说明原因，得到 %s", c.name, body)
		}
		if spent := time.Since(start); spent > 5*time.Second {
			t.Fatalf("%s：应在本地拒绝，不该真的去连（耗时 %s）", c.name, spent)
		}
	}

	// 内网 MCP 是合法需求：回环上的 MCP 服务仍然能试连。
	// （10/8、172.16/12、192.168/16 这些 RFC1918 地址本机没法监听，由 ValidateConnTestURL 的单测覆盖。）
	up := newFakeMCP(t)
	spec := map[string]any{"title": "内网 CRM", "type": "mcp", "url": up.URL, "auth": map[string]any{"type": "bearer"}}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"spec": spec, "secret": "up-secret"}, nil)
	expect(t, "内网 MCP 试连", code, 200, body)
	if !decodeAs[struct {
		OK bool `json:"ok"`
	}](t, body).OK {
		t.Fatalf("内网 MCP 仍应可以试连：%s", body)
	}
}
