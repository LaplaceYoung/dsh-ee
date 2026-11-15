package api

import (
	"net/http"
	"strings"
)

// 浏览器内第三方 Harness 直连 /v1 与 /mcp 的跨域白名单（DSH_EE_CORS_ORIGINS）。
//
// 默认关闭：白名单为空时这层中间件原样透传，不加任何 CORS 头。只对第三方 Harness 的
// 入口 /v1/*、/responses（Responses API 别名）与 /mcp 生效；网页接口（/api/*）依赖
// 同源会话 cookie，不开放跨域。
//
// 不使用 cookie：Harness 用 Authorization: Bearer 令牌，JS 自己带在请求头上，因此不发
// Access-Control-Allow-Credentials，Access-Control-Allow-Origin 也永远不是 *，
// 而是按白名单精确回显请求的 Origin。
const (
	corsAllowMethods = "GET, POST, DELETE, OPTIONS"
	corsAllowHeaders = "Authorization, Content-Type, X-Api-Key, Mcp-Protocol-Version, Mcp-Session-Id, X-DshEE-Client, Dsh-EE-API-Version"
	corsMaxAge       = "600"
	// 浏览器里的 JS 只能读 CORS 安全列表内的响应头，其余（大小写不敏感的）非简单响应头
	// 必须在实际响应里用 Access-Control-Expose-Headers 放出来。这里放的是第三方 Harness
	// 会用到的协议头：/mcp 的会话号、请求号、限流/重试提示与 API 版本。头名与 #123
	// （X-Request-Id、Dsh-EE-API-Version）以及后续限流 PR 的约定保持一致；即使某个头当前
	// 还没有实际发出，出现在列表里也无副作用（浏览器只对实际收到的头生效）。
	corsExposeHeaders = "Mcp-Session-Id, X-Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After, Dsh-EE-API-Version"
)

// cors 给白名单里的来源加 CORS 响应头。未列出的来源不加任何头（浏览器会按同源策略
// 拦截，响应本身与关闭 CORS 时一致）。预检请求统一回 204，不进入业务路由。
func (s *Server) cors(next http.Handler) http.Handler {
	if len(s.opts.CORSOrigins) == 0 {
		return next
	}
	allowed := make(map[string]bool, len(s.opts.CORSOrigins))
	for _, origin := range s.opts.CORSOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isCORSRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		// 只有浏览器会带 Origin 发预检；没有 Origin 的 OPTIONS 保持原有行为（落到路由，
		// /v1 与 /mcp 自己的 405 处理）。
		if r.Method == http.MethodOptions && origin != "" {
			if allowed[origin] {
				setCORSGrant(w, origin)
				w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if allowed[origin] {
			setCORSGrant(w, origin)
			// Expose-Headers 只对实际响应有意义（预检响应不会被 JS 读取）。
			w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
		}
		next.ServeHTTP(w, r)
	})
}

// isCORSRoute 是 CORS 白名单的作用范围：第三方 Harness 的协议入口 /v1/*、Anthropic 别名
// /anthropic/v1/*、Responses 别名 /responses 与 /mcp。
func isCORSRoute(path string) bool {
	return path == "/mcp" || strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/anthropic/v1/") || isResponsesAPIPath(path)
}

// setCORSGrant 精确回显请求的 Origin，并声明响应随 Origin 变化。
// 用 Add 而不是 Set：上游处理函数可能已经写了其它 Vary 值（如 Accept-Encoding）。
func setCORSGrant(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
}
