package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 目录扩展包下载的拨号级 IP 校验（DNS rebinding TOCTOU，与 registry 上游 #219 同一思路）：
// 预检解析到公网地址、真正拨号时再解析到内网地址的域名，不能把请求带到内网端口。
//
// 这里不连数据库：fetchDirectoryExtension 只用到 opts.DirectoryTarballHosts 和 s.upstream，
// 直接构造 Server 即可，测试和 DSH_EE_TEST_DSN 解耦。

// directoryStubResolver 替换 lookupDirectoryIP：指定主机按预设序列返回解析结果（用完后重复最后
// 一项），其它主机仍走系统解析器。用来模拟 TTL=0 的攻击者域名——预检时解析到公网地址，拨号时
// 解析到内网地址；本机无外网/DNS 也能跑。
type directoryStubResolver struct {
	mu    sync.Mutex
	calls map[string]int
}

func stubDirectoryResolver(t *testing.T, answers map[string][][]net.IPAddr) *directoryStubResolver {
	t.Helper()
	old := lookupDirectoryIP
	stub := &directoryStubResolver{calls: map[string]int{}}
	lookupDirectoryIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		seq, ok := answers[host]
		if !ok {
			return old(ctx, host)
		}
		stub.mu.Lock()
		i := stub.calls[host]
		stub.calls[host]++
		stub.mu.Unlock()
		if i >= len(seq) {
			i = len(seq) - 1
		}
		return seq[i], nil
	}
	t.Cleanup(func() { lookupDirectoryIP = old })
	return stub
}

func (s *directoryStubResolver) callsFor(host string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[host]
}

func directoryIPAddrs(ips ...string) []net.IPAddr {
	addrs := make([]net.IPAddr, 0, len(ips))
	for _, s := range ips {
		addrs = append(addrs, net.IPAddr{IP: net.ParseIP(s)})
	}
	return addrs
}

// directoryTestLogger 是内部测试用的静默 logger。
func directoryTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// directoryPackServer 是假的扩展下载站（https + 自签证书，证书 SAN 含 example.com/127.0.0.1），
// 记录每个路径被请求的次数；handler 可按路径定制（重定向测试用）。
type directoryPackServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newDirectoryPackServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *directoryPackServer {
	t.Helper()
	s := &directoryPackServer{hits: map[string]int{}}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *directoryPackServer) port(t *testing.T) string {
	t.Helper()
	_, port, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func (s *directoryPackServer) hitsFor(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// plainDirectoryUpstream 返回信任测试站证书、但拨号行为与生产默认一致的传输层（没有自定义
// DialContext / Proxy）。httptest.Server.Client() 自带的 DialContext 只给固定端口做安全映射，
// 这里用它会让「拨号层解析」测不到，所以清掉；TLS 信任根保留。
func plainDirectoryUpstream(t *testing.T, packs *directoryPackServer) *http.Client {
	t.Helper()
	tr := packs.Client().Transport.(*http.Transport).Clone()
	tr.DialContext = nil
	tr.Proxy = nil
	return &http.Client{Transport: tr}
}

// 修前红：攻击者域名第一次解析到公网地址（下载前预检通过），第二次解析到 127.0.0.1。修前下载
// client 直接复用共享传输层，传输层在拨号时自己再解析一次并连上内网端口（命中 1 次）；修后拨号
// 层用同一份解析结果校验并直接拒绝，内网端口一次都不会被访问。
//
// 测试里的 example.com 扮演攻击者域名（httptest 证书 SAN 含 example.com，红方案例的 TLS 也能
// 通过）；共享传输层的 DialContext 用同一个 stub 的第二次结果拨到内网端口，等价于传输层拨号时
// 的第二次解析。
func TestDirectoryExtensionDNSRebindingBlocked(t *testing.T) {
	var hits atomic.Int32
	internal := newDirectoryPackServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("internal-secret"))
	})
	// 首次解析给公网地址，之后都给内网地址：预检看到 #1（通过），拨号看到 #2（127.0.0.1）。
	resolver := stubDirectoryResolver(t, map[string][][]net.IPAddr{
		"example.com": {directoryIPAddrs("93.184.216.34"), directoryIPAddrs("127.0.0.1")},
	})

	// 共享上游 client：传输层拨号时按同一个 stub 再解析一次，连到内网端口（修前就会命中）。
	upTransport := internal.Client().Transport.(*http.Transport).Clone()
	upTransport.Proxy = nil
	upTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := lookupDirectoryIP(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("解析 %s 失败：%v", host, err)
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	upstream := &http.Client{Transport: upTransport}

	s := New(nil, nil, Options{DataDir: t.TempDir(), Upstream: upstream}, directoryTestLogger())
	entry := &store.DirectoryEntry{
		Kind: directory.KindExtension, Name: "acme/itsm", Version: "1.0.0",
		Digest: strings.Repeat("a", 64), SourceType: directory.SourceTarball,
		SourceURL: "https://example.com:" + internal.port(t) + "/v1.zip",
	}
	_, err := s.fetchDirectoryExtension(context.Background(), entry)
	if n := hits.Load(); n != 0 {
		t.Fatalf("内网端口被请求了 %d 次：DNS rebinding TOCTOU 仍然可用（盲 SSRF）", n)
	}
	if err == nil {
		t.Fatal("rebinding 到内网的扩展包应被拒绝")
	}
	if !strings.Contains(err.Error(), "不允许的地址") {
		t.Fatalf("错误应说明解析到不允许的地址，得到：%v", err)
	}
	// 预检 + 拨号各解析一次，两次都走同一个注入 resolver：证明防的是「两次解析结果不一致」。
	if n := resolver.callsFor("example.com"); n < 2 {
		t.Fatalf("应至少解析两次（预检 + 拨号），实际 %d 次", n)
	}
}

// 白名单（企业内网对象存储）是显式放行：内网地址不拒绝，但仍然只拨这次解析出的 IP，不再交给
// transport 二次解析。正常目录下载不回归。
func TestDirectoryExtensionTrustedHostDialsResolvedIP(t *testing.T) {
	body := []byte("extension-zip-bytes")
	// 拨号层只换 TCP 连接，TLS 的 SNI / 证书校验仍按 URL 里的主机名做。
	var gotSNI atomic.Value
	packs := newDirectoryPackServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			gotSNI.Store(r.TLS.ServerName)
		}
		_, _ = w.Write(body)
	})
	stubDirectoryResolver(t, map[string][][]net.IPAddr{
		"example.com": {directoryIPAddrs("127.0.0.1")},
	})
	upstream := plainDirectoryUpstream(t, packs)
	s := New(nil, nil, Options{
		DataDir:               t.TempDir(),
		Upstream:              upstream,
		DirectoryTarballHosts: []string{"example.com"},
	}, directoryTestLogger())
	entry := &store.DirectoryEntry{
		Kind: directory.KindExtension, Name: "acme/itsm", Version: "1.0.0",
		Digest: strings.Repeat("a", 64), SourceType: directory.SourceTarball,
		SourceURL: "https://example.com:" + packs.port(t) + "/v1.zip", Size: int64(len(body)),
	}
	raw, err := s.fetchDirectoryExtension(context.Background(), entry)
	if err != nil {
		t.Fatalf("白名单内网存储应能正常下载：%v", err)
	}
	if string(raw) != string(body) {
		t.Fatalf("下载内容不对：%q", raw)
	}
	if n := packs.hitsFor("/v1.zip"); n != 1 {
		t.Fatalf("下载次数不对：%d", n)
	}
	if sni, _ := gotSNI.Load().(string); sni != "example.com" {
		t.Fatalf("TLS SNI 应取 URL 里的主机名，得到 %q", sni)
	}
	// 共享 client 不能被改：下载用的是克隆出来的传输层（带拨号校验），原 client 保持原样。
	orig := upstream.Transport.(*http.Transport)
	if orig.DialContext != nil || orig.Proxy != nil {
		t.Fatalf("共享上游 client 的传输层被改了：DialContext=%v Proxy=%v", orig.DialContext != nil, orig.Proxy != nil)
	}
}

// 重定向策略保持不变：同主机（同 origin）重定向照常跟随；跨主机重定向仍然拒绝、目标一次都不访问。
func TestDirectoryExtensionRedirectPolicy(t *testing.T) {
	body := []byte("redirected-zip")
	var finalHits atomic.Int32
	packs := newDirectoryPackServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old.zip":
			http.Redirect(w, r, "https://"+r.Host+"/new.zip", http.StatusFound)
		case "/new.zip":
			finalHits.Add(1)
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	})
	stubDirectoryResolver(t, map[string][][]net.IPAddr{
		"example.com": {directoryIPAddrs("127.0.0.1")},
	})
	s := New(nil, nil, Options{
		DataDir:               t.TempDir(),
		Upstream:              plainDirectoryUpstream(t, packs),
		DirectoryTarballHosts: []string{"example.com"},
	}, directoryTestLogger())
	entry := &store.DirectoryEntry{
		Kind: directory.KindExtension, Name: "acme/itsm", Version: "1.0.0",
		Digest: strings.Repeat("a", 64), SourceType: directory.SourceTarball,
		SourceURL: "https://example.com:" + packs.port(t) + "/old.zip", Size: int64(len(body)),
	}
	raw, err := s.fetchDirectoryExtension(context.Background(), entry)
	if err != nil {
		t.Fatalf("同主机重定向应正常跟随：%v", err)
	}
	if string(raw) != string(body) || finalHits.Load() != 1 {
		t.Fatalf("重定向后的内容不对：%q hits=%d", raw, finalHits.Load())
	}

	// 跨主机重定向：sameHostRedirect 继续拒绝，目标端口一次都不访问。
	cross := newDirectoryPackServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old.zip":
			http.Redirect(w, r, "https://other.example.com:"+packs.port(t)+"/new.zip", http.StatusFound)
		case "/new.zip":
			finalHits.Add(1)
			_, _ = w.Write(body)
		}
	})
	crossEntry := *entry
	crossEntry.SourceURL = "https://example.com:" + cross.port(t) + "/old.zip"
	before := finalHits.Load()
	if _, err := s.fetchDirectoryExtension(context.Background(), &crossEntry); err == nil {
		t.Fatal("跨主机重定向应被拒绝")
	} else if !strings.Contains(err.Error(), "另一个地址") {
		t.Fatalf("错误应说明重定向到了另一个地址，得到：%v", err)
	}
	if after := finalHits.Load(); after != before {
		t.Fatalf("跨主机重定向的目标不应被访问：%d → %d", before, after)
	}
}
