package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 公开协议面（/v1/*、Anthropic Messages 兼容面与 /mcp）的每令牌限流与请求 ID。
//
// 限流计数在数据库的 shared_rate_limits 表里（见 store.TakeSharedRateLimit）：多实例
// 连同一个库时所有实例共用同一份额度，不会因为多实例部署把额度放大 N 倍。固定窗口按
// 窗口长度对齐，依赖各实例时钟同步（部署时用 NTP）；窗口边界有几秒偏差时额度最多多
// 放出一小截，不会成倍放大。key 是令牌的 sha256（Anthropic 兼容端点支持 x-api-key；
// 没有令牌时退回客户端 IP 的摘要），进程与数据库里都不留令牌明文；同一个令牌的模型调用与 MCP 调用共用一份
// 额度，不同令牌互不影响。
//
// tokenRate 是进程内的固定窗口限流，现在只在两种情况下使用：数据库不可用时兜底
// （takeRateLimit），以及单元测试直接构造 Server、没有数据库的时候。退化成每实例
// 一份额度，仍然有限流（不会 fail open），只是多实例下额度会放大 N 倍。

// tokenRate 是按令牌的固定窗口限流（window 内最多 limit 次），作为共享限流的兜底。
type tokenRate struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	items     map[string]*rateWindow
	lastPurge time.Time
}

// 一个限流窗口内最多记多少个 key。key 来自令牌（攻击者可以随便造无效令牌），
// 所以要有上限；超限时新 key 共用一个溢出桶，内存不会跟着请求量无限涨。
const (
	maxRateKeys    = 10000
	rateOverflowID = "\x00overflow"

	// 共享限流里超过 maxRateKeys 后的陌生 key 统一用这个 key（跨实例也都是同一行，
	// 相当于所有「不认识的令牌」共用一个很窄的额度，防止拿随机令牌把表刷爆）。
	sharedRateOverflowKey = "__overflow__"

	// 共享限流单次查库的超时。库慢/挂时尽快退回内存限流，不把请求一起拖死。
	sharedRateDBTimeout = 2 * time.Second
)

// rateKeyAdmission 记录当前窗口里已经为哪些 key 建过数据库行，把每个实例每窗口的
// 行数限制在 maxRateKeys。只存内存、每个实例一份：总行数上限约「实例数 × maxRateKeys」，
// 窗口翻页后重置。key 是哈希后的限流 key，不是令牌明文。
type rateKeyAdmission struct {
	mu          sync.Mutex
	windowStart time.Time
	keys        map[string]struct{}
}

// admit 返回这个 key 在共享限流里该用的 key：窗口内的老 key 原样返回，超过上限后的
// 陌生 key 落到 sharedRateOverflowKey。窗口起点变化时清空上一窗口的集合。
func (a *rateKeyAdmission) admit(key string, windowStart time.Time) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.keys == nil || !a.windowStart.Equal(windowStart) {
		a.windowStart = windowStart
		a.keys = map[string]struct{}{}
	}
	if _, ok := a.keys[key]; ok {
		return key
	}
	if len(a.keys) >= maxRateKeys {
		return sharedRateOverflowKey
	}
	a.keys[key] = struct{}{}
	return key
}

func newTokenRate(limit int, window time.Duration) *tokenRate {
	if window <= 0 {
		window = time.Minute
	}
	return &tokenRate{limit: limit, window: window, items: map[string]*rateWindow{}}
}

// rateInfo 是一次限流判定的结果，用来写 X-RateLimit-* 与 Retry-After。
type rateInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// take 记一次请求并返回当前窗口状态；窗口内的第 limit+1 次起返回 false。
func (l *tokenRate) take(key string) (rateInfo, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.items) >= maxRateKeys && now.Sub(l.lastPurge) >= time.Minute {
		// 顺手清掉过期窗口；限频，避免大量新 key 时每个请求都扫一遍 map
		for k, v := range l.items {
			if now.Sub(v.start) >= l.window {
				delete(l.items, k)
			}
		}
		l.lastPurge = now
	}
	if len(l.items) >= maxRateKeys {
		// 已经不认识的 key 一律进溢出桶，已有的 key 继续用自己的窗口
		if _, ok := l.items[key]; !ok {
			key = rateOverflowID
		}
	}
	w := l.items[key]
	if w == nil || now.Sub(w.start) >= l.window {
		w = &rateWindow{start: now}
		l.items[key] = w
	}
	w.count++
	info := rateInfo{Limit: l.limit, Remaining: l.limit - w.count, Reset: w.start.Add(l.window)}
	if info.Remaining < 0 {
		info.Remaining = 0
	}
	return info, w.count <= l.limit
}

// rateLimit 给 /v1/* 与 /mcp 加上每令牌限流。成功与 429 响应都带 X-RateLimit-*，
// 429 另带 Retry-After；错误体维持各端点原来的形状（见 writeRateLimited）。
// 其它路径完全不受影响。
func (s *Server) rateLimit(next http.Handler) http.Handler {
	if s.tokenRate == nil {
		return next
	}
	window := s.rateWindow()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rateLimitedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// CORS 预检由浏览器自动发出、每个跨域请求前都可能来一次，不是业务调用，
		// 不占令牌额度，也不返回限流头（否则浏览器里的第三方 Harness 会被预检拖累）。
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		info, ok := s.takeRateLimit(r.Context(), s.rateLimitKey(r), window)
		setRateLimitHeaders(w.Header(), info)
		if !ok {
			// Retry-After 按秒向上取整，至少 1 秒；客户端照它退避。
			retry := int((time.Until(info.Reset) + time.Second - 1) / time.Second)
			if retry < 1 {
				retry = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rateWindow 返回限流窗口长度：线上固定 1 分钟；Options.RateLimitWindow 只给自动化测试缩短用。
func (s *Server) rateWindow() time.Duration {
	if s.opts.RateLimitWindow > 0 {
		return s.opts.RateLimitWindow
	}
	return time.Minute
}

// takeRateLimit 记一次请求，返回当前窗口状态与是否放行。
//
// 优先用数据库里的共享计数（多实例合计一份额度）。数据库不可用时退回进程内固定窗口：
// 限流是保护性措施，不能因为限流存储故障把 /v1 与 /mcp 整体打成 5xx 或 429；退化成
// 每实例一份额度仍然有限流，不会 fail open，只是多实例下额度会放大 N 倍，并限频告警。
func (s *Server) takeRateLimit(ctx context.Context, key string, window time.Duration) (rateInfo, bool) {
	if s.store == nil {
		// 单元测试会直接构造 &Server{}（没有数据库）：保持进程内限流。
		return s.tokenRate.take(key)
	}
	windowStart := time.Now().UTC().Truncate(window)
	sharedKey := s.rateKeys.admit(key, windowStart)
	// 一次 upsert + 一次读回，正常在毫秒级；库慢/挂时由超时兜底退回内存限流。
	dbCtx, cancel := context.WithTimeout(ctx, sharedRateDBTimeout)
	state, err := s.store.TakeSharedRateLimit(dbCtx, sharedKey, windowStart, window, s.tokenRate.limit)
	cancel()
	if err != nil {
		// 客户端自己断开导致的取消是正常现象，不算数据库故障，不写告警。
		if ctx.Err() == nil {
			s.warnRateLimitFallback(err)
		}
		return s.tokenRate.take(key)
	}
	return rateInfo{Limit: state.Limit, Remaining: state.Remaining, Reset: state.Reset}, state.Allowed
}

// warnRateLimitFallback 数据库不可用时退回内存限流，并限频告警（每分钟最多一条），
// 避免数据库持续不可用时每个请求都刷一条日志。
func (s *Server) warnRateLimitFallback(err error) {
	const interval = time.Minute
	now := time.Now().UnixNano()
	last := s.rateDBWarnedAt.Load()
	if last != 0 && now-last < int64(interval) {
		return
	}
	if !s.rateDBWarnedAt.CompareAndSwap(last, now) {
		return
	}
	s.log.Warn("共享限流数据库不可用，已退回单实例内存限流", "err", err)
}

// rateLimitedPath 只覆盖公开协议面：OpenAI 兼容网关、Anthropic 兼容端点（含别名）与 MCP。
func rateLimitedPath(path string) bool {
	return isV1Path(path) || isMCPPath(path) || isAnthropicMessagesPath(path)
}

func isV1Path(path string) bool { return path == "/v1" || strings.HasPrefix(path, "/v1/") }

func isMCPPath(path string) bool { return path == "/mcp" || strings.HasPrefix(path, "/mcp/") }

// rateLimitKey 用令牌决定额度；没有 Authorization 时退回客户端 IP。
// Anthropic 兼容端点允许只带 x-api-key：该端点优先按 x-api-key 取令牌（与 auth 一致），
// 其它端点仍以 Bearer 为准。只放哈希，进程里的 map 不留令牌明文。
// 只放令牌的 sha256，进程内存与数据库里都不留令牌明文。
func (s *Server) rateLimitKey(r *http.Request) string {
	token := ""
	if isAnthropicMessagesPath(r.URL.Path) {
		token = strings.TrimSpace(r.Header.Get("X-Api-Key"))
	}
	if token == "" {
		if bearerToken, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			token = strings.TrimSpace(bearerToken)
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Api-Key"))
	}
	if token != "" {
		return "token:" + secret.Hash(token)
	}
	return "ip:" + s.clientIP(r)
}

func setRateLimitHeaders(h http.Header, info rateInfo) {
	h.Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(info.Reset.Unix(), 10))
}

// writeRateLimited 超限时保留原来的错误体形状：/v1 用 OpenAI 错误壳（Anthropic
// 兼容端点用 Anthropic 错误壳），/mcp 用 JSON-RPC 错误对象（code -32000）。
func writeRateLimited(w http.ResponseWriter, r *http.Request) {
	const msg = "请求过于频繁，请稍后重试"
	switch {
	case isMCPPath(r.URL.Path):
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error":   &mcp.Error{Code: -32000, Message: msg},
		})
	case isAnthropicMessagesPath(r.URL.Path):
		writeAnthropicErr(w, http.StatusTooManyRequests, "rate_limit_error", msg)
	case isV1Path(r.URL.Path):
		writeOpenAIErr(w, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", msg)
	default:
		writeErr(w, http.StatusTooManyRequests, msg)
	}
}

// isAnthropicMessagesPath 是 Anthropic Messages 兼容面（含 /anthropic/v1/messages 别名）；
// 限流、鉴权 token 口径与 404 兜底都按它区分错误壳。
func isAnthropicMessagesPath(path string) bool {
	return path == "/v1/messages" || strings.HasPrefix(path, "/v1/messages/") ||
		path == "/anthropic/v1/messages" || strings.HasPrefix(path, "/anthropic/v1/messages/")
}

// ---- X-Request-Id ----

type requestIDCtxKey struct{}

// requestID 生成或透传 X-Request-Id：请求头里带合法值就原样沿用（不覆盖），
// 否则生成一个随机 id。响应头回写同一个值，日志里也带上，便于和客户端/代理对账。
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := validRequestID(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDCtxKey{}, id)))
	})
}

// requestIDFrom 取中间件放进上下文的请求 id（日志用）。
func requestIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(requestIDCtxKey{}).(string)
	return id
}

// validRequestID 只接受能安全回写的请求 id：可见 ASCII、长度有上限。
// 不合法时返回空串，交给中间件重新生成，避免响应头被注入或日志被撑爆。
func validRequestID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 128 {
		return ""
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x21 || v[i] > 0x7e {
			return ""
		}
	}
	return v
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 基本不会失败；退回时间戳，至少保证每个请求有 id
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
