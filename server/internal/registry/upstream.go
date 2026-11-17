package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/netguard"
)

// 已知的公共上游。
var UpstreamRegistries = map[string]string{
	"npmjs":     "https://registry.npmjs.org",
	"npmmirror": "https://registry.npmmirror.com",
}

const maxPackumentSize = 256 << 20

var ErrUpstreamNotFound = errors.New("Upstream has no such package")

// Upstream 访问上游 npm registry，packument 缓存在磁盘上：TTL 内直接用，过期后带 ETag 重新验证，
// 上游不可用时退回旧缓存，保证内网安装不因外网抖动失败。
type Upstream struct {
	hc  *http.Client
	dir string
	ttl time.Duration
	// 允许 tarball 地址解析到内网（内部 npm 镜像需要）；默认只允许公网地址，
	// 防止上游 packument 把服务端当跳板访问内网/云元数据。
	allowPrivate bool
	// 管理端配置的上游 registry origin（scheme://host[:port]）。上游返回的 dist.tarball
	// 与它同源时放行内网地址：内部镜像/代理通常把 packument 和 tarball 放在同一服务，
	// 这是管理员显式配置过的信任来源。
	trustedOrigins map[string]struct{}

	mu       sync.Mutex
	inflight map[string]*fetchCall
	parsed   map[string]parsedEntry
}

type fetchCall struct {
	done chan struct{}
	doc  *UpstreamDoc
	err  error
}

type parsedEntry struct {
	fetchedAt time.Time
	doc       *UpstreamDoc
}

type cacheMeta struct {
	URL       string    `json:"url"`
	ETag      string    `json:"etag"`
	Status    int       `json:"status"`
	FetchedAt time.Time `json:"fetched_at"`
}

func NewUpstream(hc *http.Client, dataDir string, ttl time.Duration) (*Upstream, error) {
	dir := filepath.Join(dataDir, "upstream")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	allowPrivate := strings.EqualFold(os.Getenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE"), "true") || os.Getenv("DSH_EE_UPSTREAM_ALLOW_PRIVATE") == "1"
	u := &Upstream{dir: dir, ttl: ttl, allowPrivate: allowPrivate, trustedOrigins: map[string]struct{}{}, inflight: map[string]*fetchCall{}, parsed: map[string]parsedEntry{}}
	u.hc = u.guardClient(hc)
	return u, nil
}

// PackumentURL 拼上游的包地址：作用域包写成 @scope%2fname，所有 registry 都认。
func PackumentURL(base, name string) string {
	return strings.TrimRight(base, "/") + "/" + strings.Replace(url.PathEscape(name), "/", "%2F", 1)
}

// Packument 取上游文档。maxAge 为 0 时用默认 TTL；负数表示必须重新验证。
func (u *Upstream) Packument(ctx context.Context, base, name string, maxAge time.Duration) (*UpstreamDoc, error) {
	if maxAge == 0 {
		maxAge = u.ttl
	}
	u.trustUpstream(base)
	key := cacheKey(base, name)
	u.mu.Lock()
	if c, ok := u.inflight[key]; ok {
		u.mu.Unlock()
		select {
		case <-c.done:
			return c.doc, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &fetchCall{done: make(chan struct{})}
	u.inflight[key] = c
	u.mu.Unlock()

	// 不跟随调用方取消：同一个包可能有别的请求在等
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	c.doc, c.err = u.fetch(fetchCtx, base, name, key, maxAge)
	cancel()

	u.mu.Lock()
	delete(u.inflight, key)
	u.mu.Unlock()
	close(c.done)
	return c.doc, c.err
}

func (u *Upstream) fetch(ctx context.Context, base, name, key string, maxAge time.Duration) (*UpstreamDoc, error) {
	metaPath := filepath.Join(u.dir, key+".meta")
	bodyPath := filepath.Join(u.dir, key+".json")
	var meta cacheMeta
	if b, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	fresh := meta.Status != 0 && time.Since(meta.FetchedAt) < maxAge
	if fresh {
		if meta.Status == http.StatusNotFound {
			return nil, ErrUpstreamNotFound
		}
		if doc, err := u.load(key, bodyPath, meta.FetchedAt); err == nil {
			return doc, nil
		}
	}

	pkgURL := PackumentURL(base, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dsh-ee")
	if meta.ETag != "" && meta.Status == http.StatusOK && meta.URL == pkgURL {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	resp, err := u.hc.Do(req)
	if err != nil {
		return u.stale(key, bodyPath, meta, fmt.Errorf("Accessing upstream %s: %w", pkgURL, err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		meta.FetchedAt = time.Now()
		u.writeMeta(metaPath, meta)
		return u.load(key, bodyPath, meta.FetchedAt)
	case resp.StatusCode == http.StatusNotFound:
		u.writeMeta(metaPath, cacheMeta{URL: pkgURL, Status: http.StatusNotFound, FetchedAt: time.Now()})
		return nil, ErrUpstreamNotFound
	case resp.StatusCode != http.StatusOK:
		return u.stale(key, bodyPath, meta, fmt.Errorf("Upstream %s returned %d", pkgURL, resp.StatusCode))
	}

	tmp, err := os.CreateTemp(u.dir, "fetch-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxPackumentSize+1))
	tmp.Close()
	if err != nil {
		return u.stale(key, bodyPath, meta, fmt.Errorf("Reading upstream %s: %w", pkgURL, err))
	}
	if n > maxPackumentSize {
		return nil, fmt.Errorf("Packument from upstream %s exceeds %d MB", name, maxPackumentSize>>20)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	doc, err := ParseUpstreamDoc(b)
	if err != nil {
		return u.stale(key, bodyPath, meta, err)
	}
	if doc.Name != "" && doc.Name != name {
		return nil, fmt.Errorf("Package name %q returned by upstream does not match the requested %q", doc.Name, name)
	}
	doc.Name = name
	if err := os.Rename(tmp.Name(), bodyPath); err != nil {
		return nil, err
	}
	meta = cacheMeta{URL: pkgURL, ETag: resp.Header.Get("ETag"), Status: http.StatusOK, FetchedAt: time.Now()}
	u.writeMeta(metaPath, meta)
	u.remember(key, meta.FetchedAt, doc)
	return doc, nil
}

// stale 在上游出错时退回磁盘上的旧缓存。
func (u *Upstream) stale(key, bodyPath string, meta cacheMeta, cause error) (*UpstreamDoc, error) {
	if meta.Status == http.StatusOK {
		if doc, err := u.load(key, bodyPath, meta.FetchedAt); err == nil {
			return doc, nil
		}
	}
	return nil, cause
}

func (u *Upstream) load(key, bodyPath string, fetchedAt time.Time) (*UpstreamDoc, error) {
	u.mu.Lock()
	if e, ok := u.parsed[key]; ok && e.fetchedAt.Equal(fetchedAt) {
		u.mu.Unlock()
		return e.doc, nil
	}
	u.mu.Unlock()
	b, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, err
	}
	doc, err := ParseUpstreamDoc(b)
	if err != nil {
		return nil, err
	}
	u.remember(key, fetchedAt, doc)
	return doc, nil
}

// remember 在内存里留最近解析过的文档；大包的 packument 解析一次要几十毫秒。
func (u *Upstream) remember(key string, fetchedAt time.Time, doc *UpstreamDoc) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.parsed) >= 256 {
		for k := range u.parsed {
			delete(u.parsed, k)
			if len(u.parsed) < 192 {
				break
			}
		}
	}
	u.parsed[key] = parsedEntry{fetchedAt: fetchedAt, doc: doc}
}

func (u *Upstream) writeMeta(path string, m cacheMeta) {
	b, _ := json.Marshal(m)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o640) == nil {
		_ = os.Rename(tmp, path)
	}
}

// Forget 删除某个包的缓存（本地状态变化后不必等 TTL）。
func (u *Upstream) Forget(base, name string) {
	key := cacheKey(base, name)
	u.mu.Lock()
	delete(u.parsed, key)
	u.mu.Unlock()
	_ = os.Remove(filepath.Join(u.dir, key+".meta"))
}

func cacheKey(base, name string) string {
	h := sha256.Sum256([]byte(strings.TrimRight(base, "/") + "\n" + name))
	return hex.EncodeToString(h[:])
}

// lookupUpstreamIP 解析上游主机名。抽成包级变量是为了在测试里替换：DNS rebinding 的复现需要
// 让同一主机在「预检」和「拨号」两次解析里返回不同地址，而本机测试不依赖真实 DNS/外网。
var lookupUpstreamIP = net.DefaultResolver.LookupIPAddr

// upstreamDialTimeout 与 net/http 默认 Dialer 的拨号超时一致，不改变现有超时行为。
const upstreamDialTimeout = 30 * time.Second

// upstreamTrustedDial 是请求上下文标记：该请求的目标属于 allowPrivate/trustedOrigins 放行范围，
// 拨号层不再校验内网地址（放行时仍然只拨校验时解析出的 IP）。
type upstreamTrustedDial struct{}

func isTrustedDial(ctx context.Context) bool {
	return ctx.Value(upstreamTrustedDial{}) != nil
}

// guardClient 给上游 HTTP client 加拨号级 IP 校验（见 guardedDialContext）：直连时先解析一次、
// 逐个 IP 校验、再拨解析出的 IP，transport 不再自己二次解析，DNS rebinding 无法把预检过的
// 公网地址换成内网地址。默认部署的 client 是 *http.Transport（Proxy=ProxyFromEnvironment、
// 没有 DialContext），这里克隆一份再包装，不改调用方传入的 client/transport。
//
// 走代理时目标主机由代理侧解析，拨号级校验不适用：这类请求保持原来的代理链路（代理是部署方
// 配置的信任组件）；目标地址仍由 FetchTarball 逐跳预检。
//
// 调用方通过 NpmUpstream 显式注入的 client 归调用方管：非 *http.Transport、自带 DialTLSContext/
// DialTLS（TLS 请求会绕过 DialContext），或自带 DialContext 的传输层没有可插的钩子——前者原样
// 返回，后者在校验后仍交给它拨（最终拨号/解析归它负责，例如演示站 guard 的 Control 会按真正连到
// 的 IP 再拦一次）。默认部署（main.go）两种都没设，走完整的“同一份解析结果校验+拨号”。
func (u *Upstream) guardClient(hc *http.Client) *http.Client {
	if hc == nil {
		hc = &http.Client{}
	}
	c := *hc
	var tr *http.Transport
	var baseDial func(ctx context.Context, network, addr string) (net.Conn, error)
	if c.Transport == nil {
		// nil Transport 表示 http.DefaultTransport：只借它的 Proxy/TLS 等配置，拨号用我们自己的
		// dialer（DefaultTransport 自带的 dialer 没有安全策略，交给它会二次解析、失去 TOCTOU 保证）。
		var ok bool
		tr, ok = http.DefaultTransport.(*http.Transport)
		if !ok || tr == nil {
			return &c
		}
	} else {
		var ok bool
		tr, ok = c.Transport.(*http.Transport)
		if !ok || tr == nil || tr.DialTLSContext != nil || tr.DialTLS != nil {
			return &c
		}
		baseDial = tr.DialContext
	}
	direct := tr.Clone()
	direct.Proxy = nil
	direct.DialContext = u.guardedDialContext(baseDial)
	c.Transport = &upstreamTransport{u: u, direct: direct, proxied: tr, proxy: tr.Proxy}
	return &c
}

// upstreamTransport 按请求决定直连还是走代理：直连用带拨号级 IP 校验的传输层，走代理时用原
// 传输层（transport 的 Proxy 函数返回非 nil 即为走代理）。两条链路各用独立的连接池。
type upstreamTransport struct {
	u       *Upstream
	direct  *http.Transport // Proxy 置空 + 拨号级 IP 校验
	proxied *http.Transport // 原传输层，保留代理配置
	proxy   func(*http.Request) (*url.URL, error)
}

func (t *upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.proxy != nil {
		pu, err := t.proxy(req)
		if err != nil {
			return nil, err
		}
		if pu != nil {
			return t.proxied.RoundTrip(req)
		}
	}
	if req.URL != nil && (t.u.allowPrivate || t.u.isTrustedOrigin(req.URL.String())) {
		req = req.WithContext(context.WithValue(req.Context(), upstreamTrustedDial{}, true))
	}
	return t.direct.RoundTrip(req)
}

// guardedDialContext 返回带 IP 校验的拨号函数：解析主机 → 逐个 IP 过 blockedUpstreamIP →
// 用同一次解析结果直接拨号。校验和拨号绑定在同一份解析结果上，transport 不会在拨号时再解析
// 一次，因此 TTL=0 的攻击者域名无法在校验后把目标换成内网/云元数据地址。
// 调用方原有的 DialContext（如演示站 guard）非 nil 时，校验通过后仍交给它拨（放行目标直接
// 交给它），由它自己再做连接期检查（演示站的 Control 回调就是按真正连到的 IP 拦的）。
func (u *Upstream) guardedDialContext(base func(ctx context.Context, network, addr string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: upstreamDialTimeout, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("Dialing upstream %s: %w", addr, err)
		}
		trusted := u.allowPrivate || isTrustedDial(ctx)
		if trusted && base != nil {
			// 放行目标 + 调用方自定义 dialer：拨号/解析归调用方（如演示站 guard 的 Control 复核）。
			return base(ctx, network, addr)
		}
		ips, err := lookupUpstreamIP(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("Resolving upstream host %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("Resolving upstream host %s: no addresses", host)
		}
		if !trusted {
			for _, a := range ips {
				if blockedUpstreamIP(a.IP) {
					return nil, fmt.Errorf("Upstream host %s resolves to internal address %s; rejected. For an internal mirror, set DSH_EE_UPSTREAM_ALLOW_PRIVATE=1", host, a.IP)
				}
			}
		}
		if base != nil {
			return base(ctx, network, addr)
		}
		var lastErr error
		for _, a := range ips {
			if network == "tcp4" && a.IP.To4() == nil {
				continue
			}
			if network == "tcp6" && a.IP.To4() != nil {
				continue
			}
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
			if derr == nil {
				return conn, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = derr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no address of the requested network family")
		}
		return nil, fmt.Errorf("Dialing upstream host %s: %w", host, lastErr)
	}
}

// blockedUpstreamIP 判断地址是否属于本机/内网/链路本地等不应由服务端主动访问的范围。
// 0.0.0.0/8 与云主机元数据地址（含阿里云 100.100.100.200）不在 IsPrivate/链路本地
// 段里，用 netguard 的共享清单补上，和企业目录、连接试连三处保持一致。
func blockedUpstreamIP(ip net.IP) bool {
	return netguard.IsZeroNetwork(ip) || netguard.IsMetadataIP(ip) ||
		ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast()
}

// upstreamOrigin 归一 scheme+host 作为 origin 键（host 含端口）。
func upstreamOrigin(scheme, host string) string {
	return strings.ToLower(scheme) + "://" + strings.ToLower(host)
}

// trustUpstream 记录管理端配置的上游 registry origin。上游返回的 dist.tarball 与它同源时
// 按管理员信任处理（内部镜像/代理的 tarball 通常与 packument 同源），其余目标仍按公网校验。
func (u *Upstream) trustUpstream(base string) {
	pu, err := url.Parse(base)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return
	}
	u.mu.Lock()
	if u.trustedOrigins == nil {
		u.trustedOrigins = map[string]struct{}{}
	}
	u.trustedOrigins[upstreamOrigin(pu.Scheme, pu.Host)] = struct{}{}
	u.mu.Unlock()
}

func (u *Upstream) isTrustedOrigin(rawURL string) bool {
	pu, err := url.Parse(rawURL)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return false
	}
	u.mu.Lock()
	_, ok := u.trustedOrigins[upstreamOrigin(pu.Scheme, pu.Host)]
	u.mu.Unlock()
	return ok
}

// ValidateUpstreamURL 解析 tarball 地址并检查它会连到的每个 IP；上游 packument 是第三方内容，
// 不能让它的 dist.tarball 指向内网或云主机元数据。与管理员配置的上游 registry 同源的地址按
// 内部镜像处理（放行），其他的内网目标需要显式设置 DSH_EE_UPSTREAM_ALLOW_PRIVATE=1。
//
// 这里只是尽早失败（并给重定向逐跳预检用）；真正消除 DNS rebinding TOCTOU 的是 client 的拨号层
// （guardedDialContext）：它复验同一份解析结果并直接拨解析出的 IP，不再交给 transport 二次解析。
func (u *Upstream) ValidateUpstreamURL(ctx context.Context, rawURL string) error {
	pu, err := url.Parse(rawURL)
	if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
		return fmt.Errorf("Invalid upstream tarball URL %q", rawURL)
	}
	host := pu.Hostname()
	if host == "" {
		return fmt.Errorf("Invalid upstream tarball URL %q", rawURL)
	}
	if u.allowPrivate || u.isTrustedOrigin(rawURL) {
		return nil
	}
	ips, err := lookupUpstreamIP(ctx, host)
	if err != nil {
		return fmt.Errorf("Resolving upstream tarball URL %s: %w", host, err)
	}
	for _, addr := range ips {
		if !blockedUpstreamIP(addr.IP) {
			continue
		}
		return fmt.Errorf("Upstream tarball URL %s resolves to internal address %s; rejected. For an internal mirror, set DSH_EE_UPSTREAM_ALLOW_PRIVATE=1", host, addr.IP)
	}
	return nil
}

// FetchTarball 下载上游 tarball 写入 blobs，并按元数据校验完整性。u.hc 的传输层带拨号级 IP
// 校验（见 guardClient/guardedDialContext），dist.tarball 的主机换成 DNS rebinding 域名也连不到内网。
func (u *Upstream) FetchTarball(ctx context.Context, blobs *Blobs, rawURL, integrity, shasum string, limit int64) (Digest, error) {
	if err := u.ValidateUpstreamURL(ctx, rawURL); err != nil {
		return Digest{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Digest{}, err
	}
	req.Header.Set("User-Agent", "dsh-ee")
	// 每一跳重定向都重新校验；默认不跟随到内网。
	client := *u.hc
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("Too many redirects")
		}
		return u.ValidateUpstreamURL(req.Context(), req.URL.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		return Digest{}, fmt.Errorf("Downloading %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Digest{}, fmt.Errorf("Downloading %s: upstream returned %d", rawURL, resp.StatusCode)
	}
	d, err := blobs.Put(resp.Body, limit)
	if err != nil {
		return Digest{}, fmt.Errorf("Downloading %s: %w", rawURL, err)
	}
	if err := CheckIntegrity(d, integrity, shasum); err != nil {
		return Digest{}, fmt.Errorf("%s checksum verification failed: %w", rawURL, err)
	}
	return d, nil
}
