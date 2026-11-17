package registry

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubUpstreamResolver 替换 lookupUpstreamIP：指定主机按预设序列返回解析结果（用完后重复最后
// 一项），其它主机仍走系统解析器。用来模拟 TTL=0 的攻击者域名——预检时解析到公网地址，真正
// 拨号时解析到内网地址；本机无外网/DNS 也能跑。
func stubUpstreamResolver(t *testing.T, answers map[string][][]net.IPAddr) {
	t.Helper()
	old := lookupUpstreamIP
	var mu sync.Mutex
	calls := map[string]int{}
	lookupUpstreamIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		seq, ok := answers[host]
		if !ok {
			return old(ctx, host)
		}
		mu.Lock()
		i := calls[host]
		calls[host]++
		mu.Unlock()
		if i >= len(seq) {
			i = len(seq) - 1
		}
		return seq[i], nil
	}
	t.Cleanup(func() { lookupUpstreamIP = old })
}

func ipAddrs(ips ...string) []net.IPAddr {
	addrs := make([]net.IPAddr, 0, len(ips))
	for _, s := range ips {
		addrs = append(addrs, net.IPAddr{IP: net.ParseIP(s)})
	}
	return addrs
}

// 修前复现（盲 SSRF）：攻击者域名第一次解析到公网地址（ValidateUpstreamURL 通过），第二次解析
// 到 127.0.0.1。修前 transport 在拨号时自己再解析一次并连上内网端口；修后拨号层用同一份解析
// 结果校验并拒绝，内网端口一次都不会被访问。
//
// 测试里的 localhost 扮演攻击者域名：修前由 transport 用真实 DNS 解析到 127.0.0.1（等价于第二次
// 解析返回内网），修后拨号层使用注入 resolver 的第二次结果。
func TestFetchTarballDNSRebindingBlocked(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "")
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("internal-secret"))
	}))
	defer internal.Close()
	_, port, err := net.SplitHostPort(internal.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	// 首次解析给公网地址，之后都给内网地址。
	stubUpstreamResolver(t, map[string][][]net.IPAddr{
		"localhost": {ipAddrs("93.184.216.34"), ipAddrs("127.0.0.1")},
	})

	u, err := NewUpstream(&http.Client{Transport: &http.Transport{
		// 与默认部署一致：Proxy 函数存在但在本机没有配置代理时返回 nil。
		Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
	}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = u.FetchTarball(context.Background(), blobs, "http://localhost:"+port+"/x.tgz", "", "", 1<<20)
	if n := hits.Load(); n != 0 {
		t.Fatalf("内网端口被请求了 %d 次：DNS rebinding 仍然可用（盲 SSRF）", n)
	}
	if err == nil {
		t.Fatal("rebinding 到内网的 tarball 应被拒绝")
	}
	if !strings.Contains(err.Error(), "internal address") {
		t.Fatalf("错误应说明解析到内网地址，得到：%v", err)
	}
}

// sriAndShasum 生成 tarball 的 integrity（sha512）与 shasum（sha1），和 npm 元数据口径一致。
func sriAndShasum(body []byte) (string, string) {
	s512 := sha512.Sum512(body)
	s1 := sha1.Sum(body)
	return "sha512-" + base64.StdEncoding.EncodeToString(s512[:]), hex.EncodeToString(s1[:])
}

// 放行开关（DSH_EE_UPSTREAM_ALLOW_PRIVATE）命中时，拨号层仍然只拨这次解析出的 IP：
// 内网主机名解析到 127.0.0.1，应该正常下载，而不是交给 transport 再解析一次。
func TestFetchTarballAllowPrivateDialsResolvedIP(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "1")
	body := tgz(t, map[string]string{"package/package.json": `{"name":"x","version":"1.0.0"}`})
	integrity, shasum := sriAndShasum(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	stubUpstreamResolver(t, map[string][][]net.IPAddr{
		"internal.mirror.test": {ipAddrs("127.0.0.1")},
	})
	u, err := NewUpstream(&http.Client{Transport: &http.Transport{}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, err := u.FetchTarball(context.Background(), blobs, "http://internal.mirror.test:"+port+"/x.tgz", integrity, shasum, 10<<20)
	if err != nil {
		t.Fatalf("显式放行内网后应拨解析出的 IP 下载成功：%v", err)
	}
	if d.Size != int64(len(body)) {
		t.Fatalf("大小不对：%d != %d", d.Size, len(body))
	}
}

// 生产部署形态（默认 *http.Transport、Proxy 为空）：管理端配置的上游（trustedOrigins）解析到
// 内网时，packument 与同源 tarball 都应正常取到——放行不等于跳过拨号层，只是不拒绝内网 IP。
func TestFetchTarballTrustedOriginDialsResolvedIP(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "")
	const host = "mirror.corp.test"
	body := tgz(t, map[string]string{"package/package.json": `{"name":"x","version":"1.0.0"}`})
	integrity, shasum := sriAndShasum(body)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			_, _ = w.Write(body)
			return
		}
		fmt.Fprintf(w, `{"name":"x","dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"name":"x","version":"1.0.0","dist":{"integrity":%q,"shasum":%q,"tarball":%q}}}}`,
			integrity, shasum, "http://"+host+r.URL.Path+"/-/x-1.0.0.tgz")
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + host + ":" + port
	stubUpstreamResolver(t, map[string][][]net.IPAddr{host: {ipAddrs("127.0.0.1")}})

	u, err := NewUpstream(&http.Client{Transport: &http.Transport{}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Packument(context.Background(), base, "x", 0); err != nil {
		t.Fatalf("内部镜像的 packument 应能取到：%v", err)
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, err := u.FetchTarball(context.Background(), blobs, base+"/x/-/x-1.0.0.tgz", integrity, shasum, 10<<20)
	if err != nil {
		t.Fatalf("与配置上游同源的 tarball 应能下载：%v", err)
	}
	if d.Size != int64(len(body)) {
		t.Fatalf("大小不对：%d != %d", d.Size, len(body))
	}
}

// 重定向逐跳校验保持不变：受信任上游 302 到另一个内网地址（不同 origin）仍然拒绝，
// 目标端口一次都不被访问。
func TestFetchTarballRedirectToInternalRejected(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "")
	var targetHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		_, _ = w.Write([]byte("internal"))
	}))
	defer internal.Close()
	_, internalPort, err := net.SplitHostPort(internal.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:"+internalPort+"/x.tgz", http.StatusFound)
	}))
	defer upstream.Close()

	u, err := NewUpstream(&http.Client{Transport: &http.Transport{}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u.trustUpstream(upstream.URL) // 预检对配置的上游同源放行，重定向目标不同源
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.FetchTarball(context.Background(), blobs, upstream.URL+"/x.tgz", "", "", 1<<20)
	if err == nil {
		t.Fatal("重定向到内网地址应被拒绝")
	}
	if n := targetHits.Load(); n != 0 {
		t.Fatalf("重定向目标端口被请求了 %d 次", n)
	}
}

// 配置了代理时不破坏代理：目标主机由代理解析，请求照常走代理链路（拨号级校验不适用），
// 不会被错误地拦在代理地址上。目标地址仍由 ValidateUpstreamURL 预检。
func TestFetchTarballThroughProxyStillWorks(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "")
	body := tgz(t, map[string]string{"package/package.json": `{"name":"x","version":"1.0.0"}`})
	integrity, shasum := sriAndShasum(body)
	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		_, _ = w.Write(body)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	stubUpstreamResolver(t, map[string][][]net.IPAddr{
		"cdn.attacker.test": {ipAddrs("93.184.216.34")},
	})
	u, err := NewUpstream(&http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.FetchTarball(context.Background(), blobs, "http://cdn.attacker.test/x.tgz", integrity, shasum, 10<<20); err != nil {
		t.Fatalf("配置代理时应仍能下载：%v", err)
	}
	if proxyHits.Load() == 0 {
		t.Fatal("请求没有走代理")
	}
}

// TLS 不受拨号层影响：TCP 拨到解析出的 IP，TLS 的 ServerName/SNI 仍取 URL 里的主机名。
// 这里用 allowPrivate 放行 127.0.0.1，并让假 TLS 服务断言 SNI 是 URL 主机名。
func TestFetchTarballTLSKeepsURLServerName(t *testing.T) {
	t.Setenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE", "1")
	const host = "tls.mirror.test"
	body := tgz(t, map[string]string{"package/package.json": `{"name":"x","version":"1.0.0"}`})
	integrity, shasum := sriAndShasum(body)
	var sni atomic.Value
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			sni.Store(r.TLS.ServerName)
		}
		_, _ = w.Write(body)
	}))
	srv.StartTLS()
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	stubUpstreamResolver(t, map[string][][]net.IPAddr{host: {ipAddrs("127.0.0.1")}})
	client := &http.Client{Transport: &http.Transport{
		// 只为测试里的自签证书；SNI 仍由 URL 主机名决定，断言在服务端做。
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	u, err := NewUpstream(client, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.FetchTarball(context.Background(), blobs, "https://"+host+":"+port+"/x.tgz", integrity, shasum, 10<<20); err != nil {
		t.Fatalf("放行内网后应能通过 TLS 下载：%v", err)
	}
	if got, _ := sni.Load().(string); got != host {
		t.Fatalf("TLS ServerName（SNI）应为 %q，实际 %q", host, got)
	}
}
