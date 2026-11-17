package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/netguard"
	"github.com/hikariming/dsh-ee/server/internal/openapi"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业连接：上游是 MCP 服务或 OpenAPI 描述的接口。凭据只存在 dsh-ee（主密钥加密），
// 员工的 DSH 通过 dsh-ee 的 /mcp 调用，dsh-ee 按人校验后带上凭据转发。

const (
	connResultLimit    = 64 << 10
	defaultConnTimeout = 30 * time.Second
)

// connKey 区分上游 MCP 客户端：只有共享凭据的连接所有人共用一个（user 为 0）；
// 带员工身份的连接每人一个，各自的会话，免得上游把一个人的会话认成另一个人。
type connKey struct {
	asset, user int64
}

type connClients struct {
	mu sync.Mutex
	m  map[connKey]*cachedConn
}

type cachedConn struct {
	key    string
	client *mcp.Client
}

// dropConnClient 丢掉一个连接的全部上游客户端（配置、共享凭据变了，或上游出错要重连）。
func (s *Server) dropConnClient(id int64) {
	s.conns.mu.Lock()
	for k := range s.conns.m {
		if k.asset == id {
			delete(s.conns.m, k)
		}
	}
	s.conns.mu.Unlock()
}

// dropUserConnClient 丢掉一位员工在一个连接上的上游客户端（他的凭据变了）。
func (s *Server) dropUserConnClient(id, userID int64) {
	s.conns.mu.Lock()
	delete(s.conns.m, connKey{id, userID})
	s.conns.mu.Unlock()
}

func (s *Server) saveConnSecret(r *http.Request, id int64, plain string) error {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return s.store.SetAssetSecret(r.Context(), id, nil, "")
	}
	sealed, err := s.box.Seal([]byte(plain))
	if err != nil {
		return err
	}
	s.dropConnClient(id)
	return s.store.SetAssetSecret(r.Context(), id, sealed, secret.Hint(plain))
}

// connHeader 按认证方式拼出访问上游要带的共享凭据请求头。
func (s *Server) connHeader(ctx context.Context, assetID int64, sp *assetSpec, override *string) (http.Header, error) {
	h := http.Header{}
	if sp.Auth == nil || sp.Auth.Type == "none" {
		return h, nil
	}
	var plain string
	if override != nil {
		plain = strings.TrimSpace(*override)
	} else if assetID > 0 {
		var enc []byte
		var err error
		if name := sharedNameOf(sp); name != "" {
			credential, loadErr := s.store.SharedCredentialSecret(ctx, assetID, name)
			if errors.Is(loadErr, store.ErrNotFound) {
				return nil, fmt.Errorf("命名共享凭据 %s 不存在，请先添加该凭据或改回默认", name)
			}
			if loadErr != nil {
				return nil, loadErr
			}
			enc = credential.SecretEnc
		} else {
			enc, err = s.store.AssetSecret(ctx, assetID)
		}
		if err == nil && len(enc) > 0 {
			b, openErr := s.box.Open(enc)
			if openErr != nil {
				return nil, fmt.Errorf("解密连接凭据（主密钥是否换过？）：%w", openErr)
			}
			plain = string(b)
		} else if err != nil {
			return nil, err
		}
	}
	if plain == "" {
		return nil, errors.New("这个连接需要凭据，但还没有填写")
	}
	switch sp.Auth.Type {
	case "bearer":
		h.Set("Authorization", "Bearer "+plain)
	case "header":
		h.Set(sp.Auth.Header, plain)
	}
	return h, nil
}

// sameOrigin 比较 scheme + host + 端口（默认端口按协议补齐）。
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		switch u.Scheme {
		case "https":
			return "443"
		default:
			return "80"
		}
	}
	return ua.Scheme == ub.Scheme && strings.EqualFold(ua.Hostname(), ub.Hostname()) && port(ua) == port(ub)
}

func (s *Server) connHTTP(sp *assetSpec) *http.Client {
	t := defaultConnTimeout
	if sp.TimeoutSec > 0 {
		t = time.Duration(sp.TimeoutSec) * time.Second
	}
	base := s.upstream
	return &http.Client{Transport: base.Transport, Timeout: t, CheckRedirect: sameHostRedirect}
}

// sameHostRedirect 不跟随跳到别的主机的重定向。Go 只会在跨域时去掉 Authorization，
// 自定义请求头（X-API-Key、放员工身份的头）会原样带过去；上游一个重定向就能把凭据送到别处。
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("重定向次数太多")
	}
	if !sameOrigin(via[0].URL.String(), req.URL.String()) {
		return fmt.Errorf("上游把请求重定向到了另一个地址（%s），为了不把凭据带过去，已停止", req.URL.Host)
	}
	return nil
}

// ---- 连接测试的地址校验 ----
//
// 试连是管理员让服务端主动发请求，所以目标不能由请求方随便定：云主机元数据
// （169.254.169.254、fd00:ec2::254、100.100.100.200 等）、链路本地、组播这些地址
// 一旦能连，就会被当成读云凭据或探测内网的跳板。内网 MCP 是合法需求，所以
// RFC1918 与 loopback（本机上的 MCP 服务）明确放行。

// connTestMetadataIPs 是不在链路本地段里的已知云主机元数据地址（段内的由 IsLinkLocalUnicast 拦）。
// 清单来自 internal/netguard，与 registry / directory 的 blocked 判定共用，避免三处各写一份漂移。
var connTestMetadataIPs = netguard.MetadataIPs()

// ValidateConnTestURL 检查试连要访问的地址：只允许 http(s)，且目标解析出的每个 IP
// 都不能是云元数据、链路本地、组播、未指定之类的地址。内网 MCP 是合法用法：
// RFC1918 与 loopback 放行，所以管理员照常能测内网/本机的 MCP 服务。
func ValidateConnTestURL(ctx context.Context, rawURL string) error {
	return validateConnTestURL(ctx, rawURL, net.DefaultResolver.LookupIPAddr)
}

// validateConnTestURL 的 lookup 可替换，单测里用来模拟任意 DNS 结果。
func validateConnTestURL(ctx context.Context, rawURL string, lookup func(context.Context, string) ([]net.IPAddr, error)) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return errors.New("地址必须是 http(s):// 开头的完整地址")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("连接测试只支持 http(s):// 地址")
	}
	if u.User != nil {
		return errors.New("地址里不能带账号密码")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("地址必须是 http(s):// 开头的完整地址")
	}
	if ip := net.ParseIP(host); ip != nil {
		if reason := connTestBlockedIP(ip); reason != "" {
			return fmt.Errorf("连接地址 %s 属于%s，连接测试不允许访问", host, reason)
		}
		return nil
	}
	ips, err := lookup(ctx, host)
	if err != nil {
		return fmt.Errorf("解析连接地址 %s 失败：%v", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("连接地址 %s 没有解析到任何 IP", host)
	}
	for _, addr := range ips {
		if reason := connTestBlockedIP(addr.IP); reason != "" {
			return fmt.Errorf("连接地址 %s 解析到%s %s，连接测试不允许访问", host, reason, addr.IP)
		}
	}
	return nil
}

// connTestBlockedIP 返回不允许试连的原因；空字符串表示可以。
// 只拦云元数据与不该由服务端主动访问的地址：loopback、RFC1918 / ULA 等内网地址放行。
func connTestBlockedIP(ip net.IP) string {
	if ip == nil {
		return "无法识别的地址"
	}
	if ip.IsLoopback() || ip.Equal(net.IPv6loopback) {
		return ""
	}
	ip = canonicalConnTestIP(ip)
	if ip == nil {
		return "无法识别的地址"
	}
	if ip.IsLoopback() {
		return "" // ::127.0.0.1 这类 IPv4-compatible 写法归一化后也是本机
	}
	for _, m := range connTestMetadataIPs {
		if ip.Equal(m) {
			return "云主机元数据地址"
		}
	}
	switch {
	case ip.IsUnspecified():
		return "未指定地址"
	case netguard.IsZeroNetwork(ip):
		// 0.0.0.0/8 除 0.0.0.0 本身外的地址在部分系统会路由到本机，不是可访问的目标。
		return "保留地址"
	case ip.IsLinkLocalUnicast():
		return "链路本地地址"
	case ip.IsMulticast():
		return "组播地址"
	case !ip.IsGlobalUnicast():
		return "非公网单播地址"
	}
	return ""
}

// canonicalConnTestIP 把 IPv4 / IPv4-mapped / IPv4-compatible 的写法都归一到 4 字节或 16 字节，
// 免得换成 ::ffff:169.254.169.254 这种写法绕过按段判断。
func canonicalConnTestIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	v16 := ip.To16()
	if v16 == nil {
		return nil
	}
	zeros := true
	for _, b := range v16[:10] {
		if b != 0 {
			zeros = false
			break
		}
	}
	// IPv4-compatible：::a.b.c.d，前 10 字节为 0，第 11、12 字节也为 0。
	if zeros && v16[10] == 0 && v16[11] == 0 {
		return v16[12:]
	}
	return v16
}

// connTestDialControl 在真正拨号时按连接到的 IP 再校验一次：URL 校验时解析到公网、
// 拨号时改解析到元数据（DNS 重绑定）的窗口也被堵上。
func connTestDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("连接测试的地址不合法：%s", address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("连接测试的地址不合法：%s", address)
	}
	if reason := connTestBlockedIP(ip); reason != "" {
		return fmt.Errorf("连接测试不允许访问%s %s", reason, ip)
	}
	return nil
}

// connTestHTTP 是试连专用的 HTTP 客户端：不跟随跨主机重定向（sameHostRedirect），
// 每一跳都重新做地址校验，并在可能时加拨号级 IP 校验。正常连接仍走 connHTTP，行为不变。
func (s *Server) connTestHTTP(sp *assetSpec) *http.Client {
	hc := s.connHTTP(sp)
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := sameHostRedirect(req, via); err != nil {
			return err
		}
		// 同源主机名也可能在解析后换成元数据地址，所以每一跳都按当前 URL 重新判定。
		return ValidateConnTestURL(req.Context(), req.URL.String())
	}
	var bt *http.Transport
	switch t := s.upstream.Transport.(type) {
	case nil:
		bt = http.DefaultTransport.(*http.Transport).Clone()
		// DefaultTransport 自带的拨号没有额外的安全限制，换成本函数的 guard。
		bt.DialContext = nil
	case *http.Transport:
		bt = t.Clone()
	}
	if bt == nil {
		return hc
	}
	// 有自定义拨号的 transport（例如演示站的 guard 把自己限制在本机）不能被覆盖，
	// 这些情况仍有 URL 校验与每一跳的重定向校验兜底。
	if bt.DialContext == nil {
		d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: connTestDialControl}
		bt.DialContext = d.DialContext
	}
	hc.Transport = bt
	return hc
}

// connCaller 是这一次调用的发起人。放在 context 里交给上游客户端：带员工身份的连接每人缓存一个客户端，
// 但员工的部门、角色会变，所以请求头每次按当前的身份现取，不用建客户端时的那一份。
type connCaller struct {
	v    viewer
	base string
}

type connCallerKey struct{}

func withCaller(ctx context.Context, v viewer, base string) context.Context {
	return context.WithValue(ctx, connCallerKey{}, connCaller{v, base})
}

// connClient 取一个连接的上游 MCP 客户端；配置或凭据变了就新建。
// 连接带员工身份时，v 是发起调用的员工、base 是他访问 dsh-ee 的地址；员工身份的请求头每次请求现取
// （ee_jwt 只有几分钟有效）。
func (s *Server) connClient(ctx context.Context, a *store.Asset, version int, sp *assetSpec, v viewer, base string) (*mcp.Client, error) {
	key := fmt.Sprintf("%d|%s|%s|%s", version, a.SecretHint, sp.URL, sharedNameOf(sp))
	ck := connKey{asset: a.ID}
	if sp.UserAuth != nil {
		if v.user == nil {
			return nil, errors.New("这个连接按员工本人的身份调用，需要知道是谁在调")
		}
		ck.user = v.user.ID
	}
	s.conns.mu.Lock()
	if s.conns.m == nil {
		s.conns.m = map[connKey]*cachedConn{}
	}
	if c := s.conns.m[ck]; c != nil && c.key == key {
		s.conns.mu.Unlock()
		return c.client, nil
	}
	s.conns.mu.Unlock()
	h, err := s.connHeader(ctx, a.ID, sp, nil)
	if err != nil {
		return nil, err
	}
	c := &mcp.Client{URL: sp.URL, Header: h, HC: s.connHTTP(sp)}
	if sp.UserAuth != nil {
		// 先取一次：没填凭据、签不了令牌这类问题在这里就报出来，不留下一个建不起会话的客户端
		if _, err := s.userAuthHeader(ctx, a, sp, v, base); err != nil {
			return nil, err
		}
		uid := v.user.ID
		c.Dynamic = func(ctx context.Context) (http.Header, error) {
			who, ok := ctx.Value(connCallerKey{}).(connCaller)
			if !ok || who.v.user == nil || who.v.user.ID != uid {
				return nil, errors.New("这个连接按员工本人的身份调用，调用时没有带上是谁")
			}
			return s.userAuthHeader(ctx, a, sp, who.v, who.base)
		}
	}
	s.conns.mu.Lock()
	s.conns.m[ck] = &cachedConn{key: key, client: c}
	s.conns.mu.Unlock()
	return c, nil
}

// discoverTools 向上游取工具清单（MCP）或解析接口文档（OpenAPI）。
func (s *Server) discoverTools(ctx context.Context, sp *assetSpec, client *mcp.Client) ([]*store.ConnectionTool, string, error) {
	var out []*store.ConnectionTool
	switch sp.Type {
	case "mcp":
		tools, err := client.ListTools(ctx)
		if err != nil {
			return nil, "", err
		}
		usedNames := map[string]bool{}
		for _, t := range tools {
			name := openapi.UniqueToolName(openapi.ToolName(t.Name), usedNames)
			schema := t.InputSchema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			desc := t.Description
			if desc == "" {
				desc = t.Title
			}
			// MCP 工具默认启用；管理员可以逐个关掉或限定角色
			out = append(out, &store.ConnectionTool{Name: name, Description: desc, InputSchema: schema, ReadOnly: t.ReadOnly(), Enabled: true})
		}
		if sp.Extension.service() {
			m, err := sp.Extension.manifest()
			if err != nil {
				return nil, "", err
			}
			out, _ = applyServiceExtension(m, out)
		}
		return out, client.Instructions, nil
	case "openapi":
		spec, err := openapi.Parse([]byte(sp.OpenAPI))
		if err != nil {
			return nil, "", fmt.Errorf("解析接口文档：%w", err)
		}
		for _, t := range spec.Tools {
			op, _ := json.Marshal(t.Op)
			// 只有只读（GET）的接口默认启用，写操作要管理员确认后打开
			out = append(out, &store.ConnectionTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema, Operation: op, ReadOnly: t.ReadOnly, Enabled: t.ReadOnly})
		}
		if sp.Extension != nil {
			if err := applyExtension(sp, out); err != nil {
				return nil, "", err
			}
		}
		return out, "", nil
	}
	return nil, "", errors.New("未知的连接类型")
}

// openAPIBase 是 OpenAPI 连接的请求根地址：填了就用填的，否则用文档里 servers 的第一项。
func openAPIBase(sp *assetSpec) (string, error) {
	if sp.URL != "" {
		return sp.URL, nil
	}
	spec, err := openapi.Parse([]byte(sp.OpenAPI))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(spec.BaseURL, "http://") && !strings.HasPrefix(spec.BaseURL, "https://") {
		return "", errors.New("接口文档里没有完整的 servers 地址，请在连接里填写接口根地址")
	}
	return spec.BaseURL, nil
}

// syncConnectionTools 重新发现工具并替换清单，返回工具数。
// 连接带员工身份时，用 v（点同步的管理员）自己的身份去取工具清单；base 是他访问 dsh-ee 的地址。
func (s *Server) syncConnectionTools(ctx context.Context, id int64, v viewer, base string) (int, error) {
	a, err := s.store.GetAsset(ctx, id)
	if err != nil {
		return 0, err
	}
	lv, err := s.store.LiveAssetVersion(ctx, id)
	if err != nil {
		return 0, err
	}
	sp := parseSpec(lv.Spec)
	var client *mcp.Client
	if sp.Extension.service() {
		// 服务扩展的工具清单用系统身份取：不该要求点同步的管理员先有自己的凭据
		if client, err = s.serviceClient(ctx, a, sp, base); err != nil {
			return 0, err
		}
	} else if sp.Type == "mcp" {
		if client, err = s.connClient(ctx, a, lv.Version, sp, v, base); err != nil {
			if errors.Is(err, errNeedCredential) {
				return 0, errors.New("这个连接按员工本人的凭据调用：请先在连接页面填上你自己的凭据，再同步工具")
			}
			return 0, err
		}
	}
	ctx, cancel := context.WithTimeout(withCaller(ctx, v, base), 60*time.Second)
	defer cancel()
	tools, _, err := s.discoverTools(ctx, sp, client)
	if err != nil {
		s.dropConnClient(id)
		return 0, err
	}
	if err := s.store.ReplaceConnectionTools(ctx, id, tools); err != nil {
		return 0, err
	}
	s.hub.notify()
	return len(tools), nil
}

// callConnectionTool 以员工身份调用连接里的一个工具，返回 MCP 工具结果。
// v 是发起调用的员工，base 是他访问 dsh-ee 的地址（签 ee_jwt、给授权链接时用）。
func (s *Server) callConnectionTool(ctx context.Context, a *store.Asset, t *store.ConnectionTool, v viewer, base string, args json.RawMessage) json.RawMessage {
	start := time.Now()
	result, mode, ext, err := s.doConnectionCall(ctx, a, t, v, base, args)
	call := store.ConnectionCall{AssetID: a.ID, Tool: t.Name, UserID: v.user.ID, OK: err == nil, LatencyMS: int(time.Since(start).Milliseconds()), TokenID: v.tokenID, AuthMode: mode, Extension: ext}
	// 记下这次用的是哪一份命名共享凭据（空是原连接默认的那一份）
	if lv, metaErr := s.store.LiveAssetVersion(ctx, a.ID); metaErr == nil {
		call.SharedName = sharedNameOf(parseSpec(lv.Spec))
	}
	if err != nil {
		call.Error = err.Error()
		result = mcp.TextResult("调用 "+a.Title+" 失败："+err.Error(), true)
		if errors.Is(err, errNeedAuthorize) {
			call.Error = "员工还没授权"
			result = mcp.TextResult(extErrorText(a, base, "authorize", ""), true)
		} else if errors.Is(err, errNeedCredential) {
			// 还没发请求：告诉模型该让员工去哪里填，别当成对方系统的故障去重试
			call.Error = "员工还没填自己的凭据"
			result = mcp.TextResult("调用 "+a.Title+" 需要你本人的凭据，你还没有填。请先打开 "+base+"/assets/"+strconv.FormatInt(a.ID, 10)+
				" ，在「我的凭据」里填好，再重试。不要用别人的凭据，也不要改用其他工具绕过。", true)
		} else if !t.ReadOnly && outcomeUnknown(err) {
			// 请求已经发出去了，只是没等到回复：对方可能已经执行。
			// 若照常说"失败"，模型会很自然地重试，写操作就可能做两遍（重复建单、重复发消息）。
			call.Error = "结果不确定：" + err.Error()
			result = mcp.TextResult("调用 "+a.Title+" 的结果不确定：请求已经发出，但没有等到回复（"+err.Error()+"）。"+
				"对方系统可能已经执行了这次操作。请先用查询类工具确认是否已经生效，不要直接重试；确认不了就把情况告诉用户。", true)
		}
	} else if isErrorResult(result) {
		call.OK = false
		call.Error = "上游返回错误"
	}
	if rerr := s.store.RecordConnectionCall(context.WithoutCancel(ctx), call); rerr != nil {
		s.log.Warn("记录连接调用失败", "err", rerr)
	}
	return result
}

// outcomeUnknown 判断错误是否发生在请求发出之后、拿到回复之前（超时、连接被断开）。
// 连不上（拨号失败）说明请求根本没发出去，不算。
func outcomeUnknown(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isErrorResult(raw json.RawMessage) bool {
	var r struct {
		IsError bool `json:"isError"`
	}
	return json.Unmarshal(raw, &r) == nil && r.IsError
}

// doConnectionCall 发出一次调用。另外返回这次带的员工身份（空 / ee_jwt / user_secret）和
// 扩展的版本（`作者/名字@版本#摘要前 12 位`，不是扩展装的就是空），都记进调用记录。
func (s *Server) doConnectionCall(ctx context.Context, a *store.Asset, t *store.ConnectionTool, v viewer, base string, args json.RawMessage) (json.RawMessage, string, string, error) {
	lv, err := s.store.LiveAssetVersion(ctx, a.ID)
	if err != nil {
		return nil, "", "", err
	}
	sp := parseSpec(lv.Spec)
	mode := ""
	if sp.UserAuth != nil {
		mode = sp.UserAuth.Mode
	}
	ext := sp.Extension.label()
	switch sp.Type {
	case "mcp":
		service := sp.Extension.service()
		if service {
			if p := sp.UserAuth.personal(); p != "" {
				mode = p
			}
			if !s.extBreak.allow(a.ID) {
				return nil, mode, ext, errExtBroken
			}
			if args, err = sp.Extension.prepareService(t.Name, args); err != nil {
				return nil, mode, ext, err
			}
		}
		client, err := s.connClient(ctx, a, lv.Version, sp, v, base)
		if err != nil {
			return nil, mode, ext, err
		}
		res, err := client.CallTool(withCaller(ctx, v, base), t.Name, args)
		if service {
			// 凭据没填、还没授权这类问题不是扩展的故障，不计入熔断
			s.extBreak.report(a.ID, err != nil && !errors.Is(err, errNeedCredential) && !errors.Is(err, errNeedAuthorize))
		}
		if err != nil {
			if sp.UserAuth != nil {
				s.dropUserConnClient(a.ID, v.user.ID)
			} else {
				s.dropConnClient(a.ID)
			}
			return nil, mode, ext, err
		}
		if service {
			if mapped, code := extToolResult(a, base, res); code != "" {
				return mapped, mode, ext, nil
			}
		}
		return limitMCPResult(res, connResultLimit), mode, ext, nil
	case "openapi":
		var op openapi.Operation
		if err := json.Unmarshal(t.Operation, &op); err != nil {
			return nil, mode, ext, errors.New("工具缺少接口信息，请重新同步")
		}
		root, err := openAPIBase(sp)
		if err != nil {
			return nil, mode, ext, err
		}
		var m map[string]any
		dec := json.NewDecoder(strings.NewReader(string(args)))
		dec.UseNumber()
		if len(args) > 0 {
			if err := dec.Decode(&m); err != nil {
				return nil, mode, ext, errors.New("参数不是 JSON 对象")
			}
		}
		if m == nil {
			m = map[string]any{}
		}
		// 扩展装的连接：只往清单列出的地址发，补上不由模型填的参数；返回只留清单要的字段
		var keep []string
		if sp.Extension != nil {
			if keep, err = sp.Extension.prepare(root, t.Name, m); err != nil {
				return nil, mode, ext, err
			}
		}
		h, err := s.connHeader(ctx, a.ID, sp, nil)
		if err != nil {
			return nil, mode, ext, err
		}
		uh, err := s.userAuthHeader(ctx, a, sp, v, base)
		if err != nil {
			return nil, mode, ext, err
		}
		for k, vs := range uh {
			h[k] = vs
		}
		// 有 keep 时先多读一些，否则 >64KiB 的合法 JSON 会绕过字段筛选，
		// 把清单要求隐藏的字段原样带回模型。读到的内容仍受 4MiB 上限约束。
		readLimit := int64(connResultLimit + 1)
		if len(keep) > 0 {
			readLimit = 4<<20 + 1
		}
		status, body, err := openapi.Call(ctx, s.connHTTP(sp), root, op, m, h, readLimit)
		if err != nil {
			return nil, mode, ext, err
		}
		if status < 400 && len(keep) > 0 {
			body = extension.Keep(body, keep)
		}
		text := string(body)
		if len(body) > connResultLimit {
			text = strings.ToValidUTF8(string(body[:connResultLimit]), "") + "\n…（结果过长，已截断）"
		}
		if status >= 400 {
			return mcp.TextResult(fmt.Sprintf("HTTP %d\n%s", status, text), true), mode, ext, nil
		}
		return mcp.TextResult(text, false), mode, ext, nil
	}
	return nil, mode, ext, errors.New("未知的连接类型")
}

// limitMCPResult 让 MCP 连接的结果与 OpenAPI 连接一样受 connResultLimit 约束。
// 上游一次返回几 MB（整张表、整份日志）会直接撑爆模型上下文或白烧 token，
// 所以超长时文字块按剩余额度截断，非文字块（如 base64 图片）和 structuredContent 丢弃，
// 末尾提示模型缩小查询范围；isError 原样保留。
func limitMCPResult(raw json.RawMessage, limit int) json.RawMessage {
	if len(raw) <= limit {
		return raw
	}
	var r struct {
		Content []map[string]any `json:"content"`
		IsError bool             `json:"isError"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return mcp.TextResult("结果过长且无法解析，已丢弃；请缩小查询范围后重试", true)
	}
	var b strings.Builder
	left := limit
	dropped := 0
	for _, c := range r.Content {
		text, ok := c["text"].(string)
		if c["type"] != "text" || !ok {
			dropped++
			continue
		}
		if left <= 0 {
			break
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if len(text) > left {
			text = cutUTF8(text, left)
		}
		b.WriteString(text)
		left -= len(text)
	}
	note := fmt.Sprintf("\n…（结果过长，只保留前 %d KB", limit>>10)
	if dropped > 0 {
		note += fmt.Sprintf("，另有 %d 个非文字内容未返回", dropped)
	}
	note += "；请缩小查询范围或分页后再查）"
	return mcp.TextResult(b.String()+note, r.IsError)
}

// cutUTF8 截到不超过 n 字节，且不切断多字节字符。
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// ---- 管理接口 ----

func (s *Server) loadConnection(w http.ResponseWriter, r *http.Request) (*store.Asset, bool) {
	a, ok := s.loadAsset(w, r)
	if !ok {
		return nil, false
	}
	if a.Kind != store.KindConnection {
		writeErr(w, http.StatusNotFound, "连接不存在")
		return nil, false
	}
	return a, true
}

// POST /api/admin/connections/{id}/sync：重新发现工具。
func (s *Server) handleConnSync(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	s.dropConnClient(a.ID)
	n, err := s.syncConnectionTools(r.Context(), a.ID, viewerOf(r), s.callerBase(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tools": n})
}

type connToolReq struct {
	Enabled bool    `json:"enabled"`
	Roles   []int64 `json:"roles"`
}

// PATCH /api/admin/connections/{id}/tools/{name}：启用 / 停用工具，限定角色。
func (s *Server) handleConnTool(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	var req connToolReq
	if !readJSON(w, r, &req) {
		return
	}
	name := r.PathValue("name")
	if err := s.store.SetConnectionTool(r.Context(), a.ID, name, req.Enabled, dedupe(req.Roles)); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "工具不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditConnTool, a.Slug+"__"+name, map[string]any{"enabled": req.Enabled, "roles": req.Roles})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

type connTestReq struct {
	Spec   json.RawMessage `json:"spec"`
	Secret *string         `json:"secret"`
	// 编辑已有连接时传 id：没填凭据就用已存的
	ID int64 `json:"id"`
	// 新建连接保存前试连时传页面上的 slug：ee_jwt 默认 aud 用它，不能拿占位 "test" 去签
	Slug string `json:"slug"`
}

// POST /api/admin/connections/test：用还没保存的配置试连，返回能发现的工具。
func (s *Server) handleConnTest(w http.ResponseWriter, r *http.Request) {
	var req connTestReq
	if !readJSONLimit(w, r, &req, 6<<20) {
		return
	}
	sp, _, ok := s.decodeSpec(w, r, store.KindConnection, req.Spec)
	if !ok {
		return
	}
	if req.Slug != "" && (!slugRe.MatchString(req.Slug) || len(req.Slug) > maxSlug) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "连接的标识不合法，应是小写字母、数字和连字符"})
		return
	}
	// 用已保存的凭据试连时，只允许测试这个连接自己：不读请求里的新 URL，
	// 否则任何有 connections.manage 的人都能把已存的企业凭据发到自己的服务器。
	if req.ID > 0 && req.Secret == nil {
		a, err := s.store.GetAsset(r.Context(), req.ID)
		if err != nil || a.Kind != store.KindConnection {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "连接不存在"})
			return
		}
		v, err := s.store.LiveAssetVersion(r.Context(), req.ID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "这个连接还没有已上架的版本，不能复用已保存的凭据；请重新填写凭据或在编辑页保存后再试"})
			return
		}
		var saved assetSpec
		if err := json.Unmarshal(v.Spec, &saved); err != nil || saved.URL == "" {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "已保存的连接配置无法读取，请重新填写凭据"})
			return
		}
		if !sameOrigin(saved.URL, sp.URL) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "已保存的凭据只能用于原来的地址；换地址请重新填写凭据"})
			return
		}
		sp = &saved
		s.audit(r, "connection.test", a.Slug, map[string]any{"url": sp.URL, "reused_secret": true})
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// 试连是让服务端替人发请求：目标地址先判定，云元数据、链路本地这类一律不连。
	// 内网 MCP 是合法用法（RFC1918 与 loopback 放行），不在这里禁私网。
	if sp.URL != "" {
		if err := ValidateConnTestURL(ctx, sp.URL); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	h, err := s.connHeader(ctx, req.ID, sp, req.Secret)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var client *mcp.Client
	if sp.Type == "mcp" {
		client = &mcp.Client{URL: sp.URL, Header: h, HC: s.connTestHTTP(sp)}
		defer client.HC.CloseIdleConnections()
		if sp.UserAuth != nil {
			// 试连时用点「测试」的管理员自己的身份；user_secret 的连接还没保存时没法填凭据，只带共享凭据去试
			var probe *store.Asset
			if req.ID > 0 {
				probe, _ = s.store.GetAsset(ctx, req.ID)
			}
			if probe == nil {
				slug := req.Slug
				if slug == "" {
					slug = "test"
				}
				probe = &store.Asset{Slug: slug}
			}
			v, base := viewerOf(r), s.callerBase(r)
			if _, err := s.userAuthHeader(ctx, probe, sp, v, base); err == nil {
				client.Dynamic = func(ctx context.Context) (http.Header, error) { return s.userAuthHeader(ctx, probe, sp, v, base) }
			} else if !errors.Is(err, errNeedCredential) {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	start := time.Now()
	tools, instructions, err := s.discoverTools(ctx, sp, client)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency_ms": time.Since(start).Milliseconds()})
		return
	}
	resp := map[string]any{"ok": true, "tools": tools, "instructions": instructions, "latency_ms": time.Since(start).Milliseconds()}
	if sp.Type == "openapi" {
		if base, err := openAPIBase(sp); err == nil {
			resp["base_url"] = base
		} else {
			resp["ok"], resp["error"] = false, err.Error()
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// callerBase 是这次请求的人访问 dsh-ee 用的地址：系统设置里填了「访问地址」就用它，否则按请求推断。
// 签 ee_jwt 时作签发方，所以要稳定：生产环境应当在系统设置里填好。
func (s *Server) callerBase(r *http.Request) string {
	base, err := s.connectBase(r.Context(), r)
	if err != nil {
		return ""
	}
	return base
}
