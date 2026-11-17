package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// 假 DNS：按主机名给出固定结果，单测不需要真的解析，也不再依赖外部域名。
func stubConnTestLookup(t *testing.T, records map[string][]string, lookuperr error) func(context.Context, string) ([]net.IPAddr, error) {
	t.Helper()
	return func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if lookuperr != nil {
			return nil, lookuperr
		}
		raw, ok := records[host]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		out := make([]net.IPAddr, 0, len(raw))
		for _, s := range raw {
			out = append(out, net.IPAddr{IP: net.ParseIP(s)})
		}
		return out, nil
	}
}

// 字面量 IP 与 scheme 的判定。内网 MCP 是合法需求，RFC1918 / loopback 必须放行。
func TestConnTestValidateURL(t *testing.T) {
	ctx := context.Background()
	allowed := []string{
		"http://10.0.0.5:8080/mcp",       // RFC1918
		"http://172.16.3.9/mcp",          // RFC1918
		"http://192.168.31.10:9000",      // RFC1918
		"http://127.0.0.1:8080/mcp",      // 本机 MCP
		"https://127.0.0.1/mcp",          // 本机 MCP（https）
		"http://[::1]:8080/mcp",          // IPv6 本机
		"http://[fd12:3456::1]:8080/mcp", // 内网 IPv6（ULA）
		"http://[::ffff:10.0.0.5]/mcp",   // IPv4-mapped 的内网地址
		"http://[::ffff:127.0.0.1]/mcp",  // IPv4-mapped 的本机地址
		"https://8.8.8.8/mcp",            // 公网
	}
	for _, raw := range allowed {
		if err := ValidateConnTestURL(ctx, raw); err != nil {
			t.Errorf("%s 应允许试连，却拒绝：%v", raw, err)
		}
	}

	blocked := []struct {
		url  string
		want string
	}{
		{"http://169.254.169.254/latest/meta-data/", "云主机元数据地址"},
		{"http://169.254.170.2/v2/credentials", "云主机元数据地址"},
		{"http://169.254.20.10/mcp", "链路本地地址"},
		{"http://100.100.100.200/latest/meta-data/", "云主机元数据地址"},
		{"http://[fd00:ec2::254]/latest/meta-data/", "云主机元数据地址"},
		{"http://[fe80::1]/mcp", "链路本地地址"},
		{"http://[::ffff:169.254.169.254]/", "云主机元数据地址"},
		{"http://0.0.0.0:8080/mcp", "未指定地址"},
		{"http://0.1.2.3/mcp", "保留地址"}, // 0.0.0.0/8：部分系统会路由到本机，三处清单一致拦
		{"http://224.0.0.1/mcp", "组播地址"},
		{"http://255.255.255.255/mcp", "非公网单播地址"},
		{"file:///etc/passwd", "http(s)"},
		{"gopher://127.0.0.1:70/_", "http(s)"},
		{"ftp://10.0.0.5/mcp", "http(s)"},
		{"//127.0.0.1/mcp", "http(s)"},
		{"http:///mcp", "http(s)"},
	}
	for _, c := range blocked {
		err := ValidateConnTestURL(ctx, c.url)
		if err == nil {
			t.Errorf("%s 应被拒绝", c.url)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s 的拒绝原因应包含 %q，得到：%v", c.url, c.want, err)
		}
	}
}

// 主机名要先解析再判定：解析到元数据、或者有多个 IP 里有一个被拦，都要拒绝。
func TestConnTestValidateURLResolvedHosts(t *testing.T) {
	ctx := context.Background()
	lookup := stubConnTestLookup(t, map[string][]string{
		"crm.internal":      {"10.1.2.3"},
		"dual.internal":     {"192.168.1.5", "fd12:3456::1"},
		"localhost":         {"127.0.0.1", "::1"},
		"metadata.internal": {"169.254.169.254"},
		"mixed.internal":    {"203.0.113.7", "fe80::1"},
		"aws6.internal":     {"fd00:ec2::254"},
		"empty.internal":    {},
	}, nil)

	for _, host := range []string{"http://crm.internal:8080/mcp", "http://dual.internal/mcp", "http://localhost:3000/mcp"} {
		if err := validateConnTestURL(ctx, host, lookup); err != nil {
			t.Errorf("%s 解析到内网/本机地址，应允许试连：%v", host, err)
		}
	}

	blocked := []struct {
		url  string
		want string
	}{
		{"http://metadata.internal/latest/meta-data/", "云主机元数据地址"},
		{"http://mixed.internal/mcp", "链路本地地址"},
		{"http://aws6.internal/latest/meta-data/", "云主机元数据地址"},
		{"http://empty.internal/mcp", "没有解析到"},
		{"http://gone.internal/mcp", "解析连接地址 gone.internal 失败"},
	}
	for _, c := range blocked {
		err := validateConnTestURL(ctx, c.url, lookup)
		if err == nil {
			t.Errorf("%s 应被拒绝", c.url)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s 的拒绝原因应包含 %q，得到：%v", c.url, c.want, err)
		}
	}

	// 解析本身失败（DNS 出问题）要拒绝而不是放行：安全判定失败时宁可测不了。
	if err := validateConnTestURL(ctx, "http://gone.internal/mcp", stubConnTestLookup(t, nil, errors.New("dns down"))); err == nil {
		t.Error("DNS 解析失败时应拒绝")
	}
}

// 拨号级的第二道校验：真正要连的 IP 也要过一遍，防 DNS 重绑定。
func TestConnTestDialControl(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8080":     true,
		"[::1]:8080":         true,
		"10.0.0.5:8080":      true,
		"192.168.1.10:443":   true,
		"172.16.5.5:80":      true,
		"8.8.8.8:443":        true,
		"169.254.169.254:80": false,
		"[fe80::1]:80":       false,
		"[fd00:ec2::254]:80": false,
		"100.100.100.200:80": false,
		"0.1.2.3:80":         false,
		"not-an-ip:80":       false,
		"":                   false,
	}
	for addr, allow := range cases {
		err := connTestDialControl("tcp", addr, nil)
		if allow && err != nil {
			t.Errorf("%s 应允许拨号，却被拒绝：%v", addr, err)
		}
		if !allow && err == nil {
			t.Errorf("%s 应被拒绝拨号", addr)
		}
	}
}

// 试连客户端：跨主机重定向必须停（不能把凭据/请求带到别处），同源重定向照常跟随。
func TestConnTestHTTPRedirectPolicy(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	jump := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/mcp", http.StatusTemporaryRedirect)
	}))
	defer jump.Close()

	s := &Server{upstream: &http.Client{Transport: &http.Transport{}}}
	hc := s.connTestHTTP(&assetSpec{URL: jump.URL})
	if _, err := hc.Get(jump.URL); err == nil || !strings.Contains(err.Error(), "重定向") {
		t.Fatalf("跨主机重定向应被拒绝，得到：%v", err)
	}
	if n := targetHits.Load(); n != 0 {
		t.Fatalf("不应跟随跨主机重定向，目标却被请求了 %d 次", n)
	}

	same := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mcp" {
			http.Redirect(w, r, "/mcp/", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer same.Close()
	hc = s.connTestHTTP(&assetSpec{URL: same.URL + "/mcp"})
	resp, err := hc.Get(same.URL + "/mcp")
	if err != nil {
		t.Fatalf("同源重定向应允许：%v", err)
	}
	resp.Body.Close()
}

// 演示站的 guard 等自定义拨号不能被试连客户端覆盖掉（否则演示站会失去「只能连自己」的限制）。
func TestConnTestHTTPKeepsCustomDialer(t *testing.T) {
	var dialed atomic.Int32
	upstream := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s := &Server{upstream: upstream}
	hc := s.connTestHTTP(&assetSpec{URL: ts.URL})
	resp, err := hc.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if dialed.Load() == 0 {
		t.Fatal("自定义拨号（如演示站的 guard）应保留，不能被普通拨号替换")
	}
}

// 没有自定义 transport 时（生产默认），要给试连客户端装上拨号级 IP 校验。
func TestConnTestHTTPDefaultTransport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s := &Server{upstream: &http.Client{}}
	hc := s.connTestHTTP(&assetSpec{URL: ts.URL})
	tr, ok := hc.Transport.(*http.Transport)
	if !ok || tr.DialContext == nil {
		t.Fatal("默认 transport 应装上拨号级 IP 校验")
	}
	resp, err := hc.Get(ts.URL)
	if err != nil {
		t.Fatalf("本机地址应能连：%v", err)
	}
	resp.Body.Close()
}
