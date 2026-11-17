package registry

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateUpstreamURLBlocksPrivateTargets(t *testing.T) {
	u := &Upstream{}
	for _, raw := range []string{
		"http://127.0.0.1/x.tgz",
		"http://10.1.2.3/x.tgz",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/x.tgz",
		"ftp://example.com/x.tgz",
	} {
		if err := u.ValidateUpstreamURL(context.Background(), raw); err == nil {
			t.Errorf("%s 应被拒绝", raw)
		}
	}
	for _, raw := range []string{"https://8.8.8.8/x.tgz", "http://1.1.1.1/x.tgz"} {
		if err := u.ValidateUpstreamURL(context.Background(), raw); err != nil {
			t.Errorf("%s 是公网地址，应放行：%v", raw, err)
		}
	}
	u.allowPrivate = true
	if err := u.ValidateUpstreamURL(context.Background(), "http://127.0.0.1/x.tgz"); err != nil {
		t.Fatalf("显式允许内网时不该拦：%v", err)
	}
}

// 管理端配置的上游 registry 返回同源的 tarball 时（内部镜像的常见形态），内网地址放行；
// 换个端口（不同 origin）或者换个主机名，仍按内网目标拒绝。
func TestValidateUpstreamURLAllowsTrustedUpstreamOrigin(t *testing.T) {
	u := &Upstream{}
	u.trustUpstream("http://127.0.0.1:4321/")
	if err := u.ValidateUpstreamURL(context.Background(), "http://127.0.0.1:4321/x/-/x-1.0.0.tgz"); err != nil {
		t.Fatalf("与配置的上游同源的内网 tarball 应放行：%v", err)
	}
	for _, raw := range []string{
		"http://127.0.0.1:4322/x/-/x-1.0.0.tgz",
		"http://localhost:4321/x/-/x-1.0.0.tgz",
	} {
		if err := u.ValidateUpstreamURL(context.Background(), raw); err == nil {
			t.Errorf("%s 与配置的上游不同源，应拒绝", raw)
		}
	}
}

// 端到端：从本地假上游取 packument，再下载它同源返回的 tarball；内部镜像（packument 与
// tarball 同服务）在默认配置下必须可用，而其他内网地址仍然被拒绝。
func TestFetchTarballFromTrustedUpstreamOrigin(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "")
	files := map[string]string{"package/package.json": `{"name":"x","version":"1.0.0"}`}
	body := tgz(t, files)
	s512 := sha512.Sum512(body)
	s1 := sha1.Sum(body)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(s512[:])
	shasum := hex.EncodeToString(s1[:])

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			_, _ = w.Write(body)
			return
		}
		fmt.Fprintf(w, `{"name":"x","dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"name":"x","version":"1.0.0","dist":{"integrity":%q,"shasum":%q,"tarball":%q}}}}`,
			integrity, shasum, srv.URL+"/x/-/x-1.0.0.tgz")
	}))
	defer srv.Close()

	u, err := NewUpstream(srv.Client(), t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := u.Packument(context.Background(), srv.URL, "x", 0)
	if err != nil {
		t.Fatalf("取 packument：%v", err)
	}
	if doc.Version("1.0.0") == nil {
		t.Fatal("packument 里没有 1.0.0")
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, err := u.FetchTarball(context.Background(), blobs, srv.URL+"/x/-/x-1.0.0.tgz", integrity, shasum, 10<<20)
	if err != nil {
		t.Fatalf("同源 tarball 应该能下载：%v", err)
	}
	if d.Size != int64(len(body)) {
		t.Fatalf("大小不对：%d != %d", d.Size, len(body))
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer other.Close()
	if _, err := u.FetchTarball(context.Background(), blobs, other.URL+"/x.tgz", "", "", 10<<20); err == nil {
		t.Fatal("不同 origin 的内网 tarball 应拒绝")
	}
}

// 0.0.0.0/8 与云主机元数据地址（阿里云 100.100.100.200）不在 IsPrivate/链路本地段里，
// registry 也必须按「不能访问的目标」拒绝：三处清单（registry / directory / 连接试连）
// 要一致，否则同一类 SSRF 只在某一条入口被拦。
func TestBlockedUpstreamIPRestrictedRanges(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"0.0.0.0", true},
		{"0.1.2.3", true},
		{"100.100.100.200", true},
		{"::ffff:100.100.100.200", true},
		{"169.254.169.254", true}, // 链路本地，原本就拦
		{"10.0.0.5", true},        // RFC1918，原本就拦（allowPrivate 才放行）
		{"127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false},
	}
	for _, c := range cases {
		if got := blockedUpstreamIP(net.ParseIP(c.ip)); got != c.blocked {
			t.Errorf("blockedUpstreamIP(%s) = %v，期望 %v", c.ip, got, c.blocked)
		}
	}
}

// allowPrivate（DSH_EE_UPSTREAM_ALLOW_PRIVATE）仍然放行这些目标：只补黑名单，
// 不改变显式放行内网镜像开关的语义。
func TestValidateUpstreamURLRestrictedRangesAllowPrivate(t *testing.T) {
	u := &Upstream{}
	for _, raw := range []string{
		"http://0.0.0.0/x.tgz",
		"http://0.1.2.3/x.tgz",
		"http://100.100.100.200/latest/meta-data/",
	} {
		if err := u.ValidateUpstreamURL(context.Background(), raw); err == nil {
			t.Errorf("%s 应被拒绝", raw)
		}
	}
	u.allowPrivate = true
	for _, raw := range []string{"http://0.1.2.3/x.tgz", "http://100.100.100.200/x.tgz"} {
		if err := u.ValidateUpstreamURL(context.Background(), raw); err != nil {
			t.Errorf("显式允许内网时 %s 不该被拦：%v", raw, err)
		}
	}
}
