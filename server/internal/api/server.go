// Package api 是 dsh-ee 的 HTTP 层：登录与会话、管理接口、以及 OpenAI 兼容的模型网关。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hikariming/dsh-ee/server/internal/datahub"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/kb"
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
}

type Server struct {
	store           *store.Store
	box             *secret.Box
	opts            Options
	log             *slog.Logger
	limiter         *loginLimiter
	deviceCodeRate  *ipRate
	deviceTokenRate *ipRate
	upstream        *http.Client
	registry        *registry.Service
	settings        settingsCache
	// 能力目录变化的通知（DSH 的 MCP 长连接据此刷新工具）
	hub hubBroker
	// 停机时关闭，让 /mcp 事件流自己结束
	streamsDone chan struct{}
	streamsOnce sync.Once
	// 上游 MCP 连接的缓存，按资产 id
	conns connClients
	// ee_jwt 的签名密钥（内存里的一份，定期重读库，见 user_identity.go）
	eeKeyring eeKeyring
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
	// 「开始使用」页的接入码
	setupCodes setupCodes
	// 全部路由；Agent 工具也从这里取处理函数
	routeTable []route
	// 由路由表生成的 Agent 工具
	agentTools []*agentTool
	// 知识库的内存关键词索引
	kb kbIndex
	// OIDC 登录：发现配置与公钥缓存、进行中的登录请求
	oidcOnce   sync.Once
	oidcClient *oidc.Client
	oidcStates oidcStates
	webLinks   webLinks
}

func New(st *store.Store, box *secret.Box, opts Options, log *slog.Logger) *Server {
	if opts.SessionTTL == 0 {
		opts.SessionTTL = 7 * 24 * time.Hour
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
	s := &Server{store: st, box: box, opts: opts, log: log, limiter: newLoginLimiter(), deviceCodeRate: newIPRate(deviceCodePerMinute), deviceTokenRate: newIPRate(deviceTokenPerMinute), upstream: up, connectKick: make(chan struct{}, 1), elevationKick: make(chan struct{}, 1), streamsDone: make(chan struct{})}
	s.kb.ix = kb.NewIndex()
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
		writeOpenAIErr(w, http.StatusNotFound, "invalid_request_error", "not_found", "接口不存在")
	})
	if s.opts.Web != nil {
		mux.Handle("/", s.opts.Web)
	}

	return s.logRequests(mux)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
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
	// 会话是从 DSH 凭设备令牌换来的，只带员工权限（见 web_link.go）
	limited bool
}

func withUser(ctx context.Context, u *store.User, a *rbac.Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: a})
}

func withLimitedUser(ctx context.Context, u *store.User, a *rbac.Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, principal{user: u, access: limitedAccess(a), limited: true})
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

func notFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

// RunBackground 跑定时任务：LDAP、插件更新、接入插件与 Desktop 版本同步。
func (s *Server) RunBackground(ctx context.Context) {
	go s.runLDAPSync(ctx)
	go s.runElevationExpiry(ctx)
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
			j, err := s.registry.CheckUpdates(ctx, 0)
			if err != nil {
				s.log.Warn("检查上游新版本失败", "err", err)
			} else if j != nil {
				s.log.Info("发现上游新版本，已开始同步", "job", j.ID, "count", j.Total)
			}
			t.Reset(6 * time.Hour)
		}
	}
}
