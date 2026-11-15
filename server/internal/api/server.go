// Package api 是 dsh-ee 的 HTTP 层：登录与会话、管理接口、以及 OpenAI 兼容的模型网关。
package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"github.com/hikariming/dsh-ee/server/internal/datahub"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/kb"
	"github.com/hikariming/dsh-ee/server/internal/lease"
	"github.com/hikariming/dsh-ee/server/internal/oidc"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

type Options struct {
	CookieSecure bool
	SessionTTL   time.Duration
	// 访问上游模型服务的客户端；测试里可替换。
	Upstream *http.Client
	// 构建版本，/healthz 返回。
	Version string
	// 嵌入的前端；为空时只提供接口（测试、或前端单独部署）。
	Web http.Handler
	// npm 源的数据目录（tarball、上游缓存）；为空时用临时目录（测试）。
	DataDir string
	// 对外地址，用于生成 tarball 下载地址；为空时按请求推断。
	PublicURL string
	// 浏览器内第三方 Harness 直连 /v1 与 /mcp 的跨域白名单，逗号分隔的完整 origin
	// 已由 config 解析、校验；为空时关闭 CORS，见 cors.go。
	CORSOrigins []string
	// 可信反向代理的地址或网段（DSH_EE_TRUSTED_PROXIES）。只有来自这些对端的
	// X-Forwarded-* 才会被采信；为空时只信任本机回环地址。
	TrustedProxies []netip.Prefix
	// 监听地址；示例里的演示差旅系统连接从这里访问 dsh-ee 自己。为空时用请求的 Host。
	Addr string
	// 管理模式里「小时」的长度，默认 1 小时；只给自动化测试缩短用。
	AdminModeUnit time.Duration
	// 访问上游 npm registry 与 dshfind 目录的客户端；测试里可替换。
	NpmUpstream *http.Client
	// 上游 packument 缓存时间，默认 5 分钟。
	UpstreamTTL time.Duration
	// dshfind 插件目录地址；为空时用默认的 api.dshfind.com。
	FeedURL string
	// 访问 OIDC 身份提供方的客户端；测试里可替换。
	OIDCClient *http.Client
	// 数据中台（datahub.go）；为空表示没开，DataHubErr 说明原因
	DataHub    *datahub.Hub
	DataHubErr string
	// 公开演示站；nil 表示正常部署。见 demo_site.go
	DemoSite *DemoSite
	// 轮换 ee_jwt 密钥时，新钥匙过多久开始签名；0 用默认的 10 分钟（测试里调短）
	EEKeyDelay time.Duration
	// 登录失败按客户端 IP 限流：窗口内失败多少次后返回 429。0 用默认值（15 分钟 50 次），
	// 小于 0 关闭；配置里 DSH_EE_LOGIN_IP_LIMIT=0/off 会传 -1。
	LoginIPLimit int
	// 上面限流窗口的长度；0 用默认的 15 分钟。
	LoginIPWindow time.Duration
	// 是否开放 GET /metrics（给监控抓取）。默认关闭。见 metrics.go。
	Metrics bool
	// 协议面（/v1/* 与 /mcp）每令牌每分钟的请求数；0 表示关闭。
	// 线上默认由 config 的 DSH_EE_RATE_LIMIT 给 60，测试里按需设置。
	RateLimitPerMinute int
	// 限流窗口，默认 1 分钟；只给自动化测试缩短用。
	RateLimitWindow time.Duration
	// 允许用 iframe 嵌入网页的宿主 origin（DSH_EE_EMBED_ORIGINS）；为空时只允许同站。
	// 只由 config 解析、校验后传进来，见 security_headers.go。
	EmbedOrigins []string
	// 跨站 iframe 嵌入 v1 总开关（DSH_EE_EMBED_CROSS_SITE）；默认关闭。
	EmbedCrossSite bool
	// ee_emb_ 嵌入会话有效期（DSH_EE_EMBED_SESSION_TTL）；0 用默认 2h。
	EmbedSessionTTL time.Duration
	// 一次性 embed code 有效期；0 用默认 60s。只给自动化测试缩短用。
	EmbedCodeTTL time.Duration
	// 扩展签名（docs/plugins.md §9.3）：受信任的签名公钥，安装计划里据此核对签名。
	ExtensionTrustedKeys []ed25519.PublicKey
	// 强制验证签名（DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1）：没签名或验不过的扩展不让装。
	ExtensionRequireSignature bool
	// 旧版 npm 令牌 / API key 的 scope 收敛（docs/token-lifecycle.md）；默认 false 保持旧行为。
	RestrictLegacyTokenScopes bool
	// mint 的共享（数据库）固定窗口限流：每 IP 每分钟次数；0 用默认 30（DSH_EE_EMBED_MINT_PER_MINUTE）。
	// 多实例部署时是合计额度，不是每实例一份。
	EmbedMintPerMinute int
	// exchange 的共享固定窗口限流：每 IP 每分钟次数；0 用默认 60（DSH_EE_EMBED_EXCHANGE_PER_MINUTE）。
	EmbedExchangePerMinute int
	// 每用户同时有效的嵌入会话上限；0 用默认 5，超过时撤销最旧的会话。
	EmbedMaxSessionsPerUser int
	// 内容安全策略（DSH_EE_CSP）：off / report / enforce；空或其它值按 off。
	// report 用 Content-Security-Policy-Report-Only，enforce 用 Content-Security-Policy；
	// 两种模式都保留 #118 的 frame-ancestors 嵌入收口。见 security_headers.go。
	CSP string
	// CSP 违规报告地址（DSH_EE_CSP_REPORT_URI）；为空时策略里不加 report-uri。
	CSPReportURI string
	// index.html 里内联脚本的 CSP sha256 源（形如 "sha256-…"）。由 webui 在启动时从
	// 嵌入的构建产物算出来，见 webui.InlineScriptHashes；为空时 script-src 只有 'self'。
	CSPInlineScriptHashes []string
	// 企业目录（directory.go）：tarball 来源的主机白名单；空表示只接受公网 https 地址。
	DirectoryTarballHosts []string
	// API 客户端 client_credentials（clients.go）：每 IP 每分钟最多签发几次；0 表示关闭限流，
	// 配置默认 120（DSH_EE_CLIENT_TOKEN_PER_MINUTE）。只防撞库，正常集成换 token 远低于这个量。
	ClientTokenPerMinute int
	// 多实例后台任务租约（internal/lease），由 cmd/dsh-ee 创建后传入，后台循环与清理任务共用。
	// nil 表示不启用：每个实例都会执行周期任务（单实例与测试的原有行为）。
	BackgroundLeases *lease.Runner
	// 刷新令牌（docs/token-lifecycle.md §3）：默认关闭；关闭时 POST /api/auth/refresh 返回 404，
	// 设备登录 / 手工签发也不会额外返回 refresh token。
	RefreshTokens bool
	// refresh 换出来的 access token 有效期；0 用默认 1h。
	RefreshAccessTTL time.Duration
	// 单枚 refresh token 的空闲有效期；0 用默认 30 天。
	RefreshTokenTTL time.Duration
	// 令牌族绝对有效期；0 用默认 180 天。
	RefreshFamilyTTL time.Duration
	// 轮换宽限：>0 时旧 refresh 在轮换后这段时间内重试会原样返回上次响应；0 严格。
	RefreshGrace time.Duration
}

type Server struct {
	store           *store.Store
	box             *secret.Box
	opts            Options
	log             *slog.Logger
	limiter         *loginLimiter
	loginIPLimiter  *loginIPLimiter
	deviceCodeRate  *ipRate
	deviceTokenRate *ipRate
	// client_credentials 的每 IP 限流；nil 表示关闭（见 clients.go）
	clientTokenRate *ipRate
	// 启动时间、登录计数与嵌入拒绝计数，只给 /metrics 用
	startedAt            time.Time
	loginFailures        atomic.Uint64
	loginIPLimited       atomic.Uint64
	loginUsernameLimited atomic.Uint64
	// 被拒绝的嵌入请求数（embed.deny 审计）：进程内累计，重启清零、多实例各自独立
	embedDenied atomic.Uint64
	// 可信反向代理网段；只有这些对端的 X-Forwarded-* 会被采信（见 proxy.go）
	trustedProxies []netip.Prefix
	upstream       *http.Client
	registry       *registry.Service
	settings       settingsCache
	// 能力目录变化的通知（DSH 的 MCP 长连接据此刷新工具）
	hub hubBroker
	// 停机时关闭，让 /mcp 事件流自己结束
	streamsDone chan struct{}
	streamsOnce sync.Once
	// 上游 MCP 连接的缓存，按资产 id
	conns connClients
	// 网关的上游连接与模型冷却状态
	providerCD providerCooldowns
	// ee_jwt 的签名密钥（内存里的一份，定期重读库，见 user_identity.go）
	eeKeyring eeKeyring
	// 服务扩展：连续出错时的熔断、回调地址的限流（service_ext.go）
	extBreak        extBreaker
	extEndpointRate keyRate
	// /v1/* 与 /mcp 的每令牌限流；nil 表示关闭（见 rate_limit.go）。
	// 计数优先走数据库里的共享固定窗口，tokenRate 只在数据库不可用或没有数据库
	// （单元测试直接构造 Server）时兜底。
	tokenRate *tokenRate
	// 共享限流每个窗口已建行的 key 集合：key 来自令牌（攻击者可以随便造无效令牌），
	// 超过上限后的陌生 key 共用一个溢出 key，数据库行数不会跟着请求量无限涨（见 rate_limit.go）。
	rateKeys rateKeyAdmission
	// 「退回内存限流」告警的限频时间（UnixNano），避免数据库持续故障时每个请求刷一条日志。
	rateDBWarnedAt atomic.Int64
	// 示例内容里演示差旅系统的内存数据
	demo demoTravel
	crm  demoCRM
	itsm demoITSM
	// 设置或模型变化后触发接入插件检查
	connectKick chan struct{}
	connectMu   sync.Mutex
	// 有令牌开启 / 结束管理模式后触发到期监控重算
	elevationKick chan struct{}
	releaseFeeds  map[string]string
	releaseDMGs   map[string]string
	releaseHTTP   *http.Client
	releaseKick   chan struct{}
	releaseMu     sync.Mutex
	releaseJobs   map[string]bool
	// 全部路由；Agent 工具也从这里取处理函数
	routeTable []route
	// 由路由表生成的 Agent 工具
	agentTools []*agentTool
	// 后台周期任务的租约（多实例选主）；nil 时直接执行
	leases *lease.Runner
	// 知识库的内存关键词索引
	kb kbIndex
	// OIDC 登录：发现配置与公钥缓存（一次性 state/PKCE 在共享表 ephemeral_states 里）
	oidcOnce   sync.Once
	oidcClient *oidc.Client
}

func New(st *store.Store, box *secret.Box, opts Options, log *slog.Logger) *Server {
	if opts.SessionTTL == 0 {
		opts.SessionTTL = 7 * 24 * time.Hour
	}
	// 刷新令牌的默认 TTL；只在开启 refresh 时用得到，但先归一到合法值，避免响应里算出空时间。
	if opts.RefreshAccessTTL <= 0 {
		opts.RefreshAccessTTL = time.Hour
	}
	if opts.RefreshTokenTTL <= 0 {
		opts.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if opts.RefreshFamilyTTL <= 0 {
		opts.RefreshFamilyTTL = 180 * 24 * time.Hour
	}
	if opts.RefreshFamilyTTL < opts.RefreshTokenTTL {
		opts.RefreshFamilyTTL = opts.RefreshTokenTTL
	}
	if len(opts.TrustedProxies) == 0 {
		// 没接线时也保持安全默认：只信任本机回环地址上的代理。
		opts.TrustedProxies = defaultTrustedProxies()
	}
	if opts.DemoSite != nil {
		d := *opts.DemoSite
		if _, port, err := net.SplitHostPort(opts.Addr); err == nil {
			d.selfPort = port
		}
		opts.DemoSite = &d
	}
	up := opts.Upstream
	if up == nil {
		up = &http.Client{Transport: opts.DemoSite.guard(&http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 2 * time.Minute,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   32,
		})}
	}
	s := &Server{store: st, box: box, opts: opts, log: log, leases: opts.BackgroundLeases, limiter: newLoginLimiter(), deviceCodeRate: newIPRate(deviceCodePerMinute), deviceTokenRate: newIPRate(deviceTokenPerMinute), trustedProxies: opts.TrustedProxies, upstream: up, connectKick: make(chan struct{}, 1), elevationKick: make(chan struct{}, 1), streamsDone: make(chan struct{})}
	if opts.RateLimitPerMinute > 0 {
		s.tokenRate = newTokenRate(opts.RateLimitPerMinute, opts.RateLimitWindow)
	}
	s.loginIPLimiter = newLoginIPLimiter(opts.LoginIPLimit, opts.LoginIPWindow)
	if opts.ClientTokenPerMinute > 0 {
		s.clientTokenRate = newIPRate(opts.ClientTokenPerMinute)
	}
	s.startedAt = time.Now()
	s.kb.ix = kb.NewIndex()
	s.extEndpointRate.limit = extEndpointPerMinute
	s.initDshReleases()
	s.registry = s.newRegistry()
	s.routeTable = s.routes()
	s.agentTools = buildAgentTools(s.routeTable)
	return s
}

// CloseStreams 结束所有 /mcp 事件流，交给 http.Server.RegisterOnShutdown。
// Shutdown 只等连接空闲、不取消请求，而事件流不会自己结束：只要有一台 DSH 在线，
// 每次重启都要等满超时，最后还以超时错误退出。DSH 断线后会自己重连。
func (s *Server) CloseStreams() {
	s.streamsOnce.Do(func() { close(s.streamsDone) })
}

// newRegistry 初始化 npm 源。数据目录建不了时 npm 相关接口返回 503，其余功能不受影响。
func (s *Server) newRegistry() *registry.Service {
	dir := s.opts.DataDir
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "dsh-ee-data-"); err != nil {
			s.log.Error("创建临时数据目录失败", "err", err)
			return nil
		}
	}
	s.opts.DataDir = dir
	hc := s.opts.NpmUpstream
	if hc == nil {
		hc = &http.Client{Transport: s.opts.DemoSite.guardNpm(&http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: time.Minute,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   16,
		})}
	}
	ttl := s.opts.UpstreamTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	blobs, err := registry.NewBlobs(dir)
	if err != nil {
		s.log.Error("npm 源不可用", "err", err)
		return nil
	}
	up, err := registry.NewUpstream(hc, dir, ttl)
	if err != nil {
		s.log.Error("npm 源不可用", "err", err)
		return nil
	}
	svc := registry.NewService(s.store, blobs, up, s.appSettings, s.log)
	if s.opts.FeedURL != "" {
		svc.FeedURL = s.opts.FeedURL
	}
	return svc
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routeTable {
		mux.Handle(rt.pattern, rt.handler(s))
	}

	// 没匹配上的接口返回 JSON 404，不要落到前端的 index.html
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusNotFound, "接口不存在") })
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		// Anthropic 兼容面下的其它路径（batches/files/count_tokens 的子路径等）用 Anthropic 错误壳，
		// 避免 Claude Code / Anthropic SDK 拿到 OpenAI 形状的 404。
		if isAnthropicMessagesPath(r.URL.Path) {
			writeAnthropicErr(w, http.StatusNotFound, "not_found_error", "接口不存在：Anthropic 兼容目前只实现 POST /v1/messages")
			return
		}
		writeOpenAIErr(w, http.StatusNotFound, "invalid_request_error", "not_found", "接口不存在")
	})
	// Anthropic 兼容面的别名前缀（DeepSeek 的 base URL 习惯）：未匹配路径用 Anthropic 错误壳，
	// 不要落到前端 SPA 兜底。
	mux.HandleFunc("/anthropic/", func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicErr(w, http.StatusNotFound, "not_found_error", "接口不存在：Anthropic 兼容入口是 POST /v1/messages（别名 /anthropic/v1/messages）")
	})
	if s.opts.Web != nil {
		mux.Handle("/", s.opts.Web)
	}

	// 中立 Harness 契约的版本响应头/请求号只加在 /v1 与发现文档上，既有 API 语义不变。
	// 浏览器内第三方 Harness 的跨域白名单（DSH_EE_CORS_ORIGINS，默认关闭），只作用于 /v1/* 与 /mcp，见 cors.go；
	// 放在 harnessAPI 外面，预检先于方法检查处理，harness 的错误响应也带 CORS 头。
	// securityHeaders 在最外层，harnessAPI 直接写出的响应同样带文档安全头。
	// 每令牌限流（DSH_EE_RATE_LIMIT）放在 cors 里面，429 也带 CORS 头，浏览器能读到 Retry-After；
	// requestID 在最外层，日志与响应用同一个请求号。
	return s.requestID(s.logRequests(s.securityHeaders(s.cors(s.rateLimit(harnessAPI(mux))))))
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds(), "request_id", requestIDFrom(r))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 透传给底层，网关的流式响应依赖它。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---- JSON 工具 ----

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// httpProblem 是带状态码和给人看的信息的失败：处理函数内部把校验失败返回给外层统一写回，
// 免得每个步骤都自己 writeErr。实现函数见 writeProblem。
type httpProblem struct {
	status int
	msg    string
}

func (e *httpProblem) Error() string { return e.msg }

// statusErr 造一个带状态码的失败。
func statusErr(status int, msg string) error { return &httpProblem{status: status, msg: msg} }

// writeProblem 把处理函数返回的失败写回：带状态码的按状态码，其余按 500。
func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	var p *httpProblem
	if errors.As(err, &p) {
		writeErr(w, p.status, p.msg)
		return
	}
	s.internalErr(w, r, err)
}

func (s *Server) internalErr(w http.ResponseWriter, r *http.Request, err error) {
	// 客户端刷新或切页会取消请求上下文，这时写库/读库报 context canceled 是正常现象，
	// 既不是服务端故障，也没有可写的响应，别在日志里刷 500。
	if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
		s.log.Debug("请求已取消", "path", r.URL.Path)
		return
	}
	s.log.Error("internal error", "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "服务器内部错误")
}

// readJSON 要求 Content-Type 为 application/json：浏览器跨站表单发不出这种请求，
// 配合 SameSite=Lax 的会话 cookie 挡住 CSRF。
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "请求体必须是 application/json")
		return false
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体格式错误："+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "非法的 id")
		return 0, false
	}
	return id, true
}

// ---- 当前用户 ----

type ctxKey struct{}

type principal struct {
	user   *store.User
	access *rbac.Access
	// 会话是从 DSH 凭设备令牌换来的，或者来自跨站嵌入的 ee_emb_ 会话：只带员工权限
	limited bool
	// embed 非 nil 表示这是一条 ee_emb_ 嵌入会话（见 embed.go）
	embed *embedPrincipal
	// 经由接入令牌认证时的令牌 id 与客户端类型；网页会话为零值
	tokenID    int64
	clientKind string
}

func withUser(ctx context.Context, u *store.User, a *rbac.Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: a})
}

// withTokenUser 与 withUser 相同，并带上令牌属性（id / 客户端类型），供写确认与调用记录用。
func withTokenUser(ctx context.Context, u *store.User, a *rbac.Access, tokenID int64, clientKind string) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: a, tokenID: tokenID, clientKind: clientKind})
}

func withLimitedUser(ctx context.Context, u *store.User, a *rbac.Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: limitedAccess(a), limited: true})
}

// withEmbedUser 把 ee_emb_ 会话放进请求上下文：权限按 limited 收窄，并带上应用与宿主 origin。
func withEmbedUser(ctx context.Context, u *store.User, a *rbac.Access, info *embedPrincipal) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: limitedAccess(a), limited: true, embed: info})
}

// sessionEmbed 返回当前请求的嵌入会话信息；不是嵌入会话时 nil。
func sessionEmbed(r *http.Request) *embedPrincipal {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.embed
}

// sessionLimited 表示当前网页会话是从 DSH 打开的，只带员工权限。
func sessionLimited(r *http.Request) bool {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.limited
}

func currentUser(r *http.Request) *store.User {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.user
}

// currentAccess 在未登录时返回 nil，Has 对 nil 一律为 false。
func currentAccess(r *http.Request) *rbac.Access {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.access
}

// currentTokenID 返回这次请求所用接入令牌的 id；网页会话或旧令牌为 0。
func currentTokenID(r *http.Request) int64 {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.tokenID
}

// currentClientKind 返回这次请求所用令牌的客户端类型；网页会话为空。
func currentClientKind(r *http.Request) string {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p.clientKind
}

func notFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

// RunBackground 跑定时任务：LDAP、飞书组织同步、插件更新、接入插件与 Desktop 版本同步。
func (s *Server) RunBackground(ctx context.Context) {
	go s.runLDAPSync(ctx)
	go s.runFeishuSync(ctx)
	go s.runExtHealth(ctx)
	go s.runKBSourceSync(ctx)
	go s.runElevationExpiry(ctx)
	go s.runRecoveryExpiry(ctx)
	go s.runAppRowPurge(ctx)
	go s.runDshReleases(ctx)
	if s.registry == nil {
		return
	}
	go s.runConnect(ctx)
	// 启动一分钟后先查一次：只靠 6 小时的定时器的话，重启得勤就一直轮不到
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runLeased(ctx, leaseRegistryCheckUpdate, func(ctx context.Context) {
				j, err := s.registry.CheckUpdates(ctx, 0)
				if err != nil {
					s.log.Warn("检查上游新版本失败", "err", err)
				} else if j != nil {
					s.log.Info("发现上游新版本，已开始同步", "job", j.ID, "count", j.Total)
				}
			})
			t.Reset(6 * time.Hour)
		}
	}
}
