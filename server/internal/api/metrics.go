package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// metricsContentType 是 Prometheus 的文本格式；不引入客户端库，直接按格式输出。
const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// handleMetrics 是给监控抓取的最小指标页面。
//
// 默认关闭（DSH_EE_METRICS=true 才开），且只允许本机或内网来源抓取：指标里虽然没有
// 用户数据，但调用量、失败次数这类信息不适合暴露给公网。反向代理/负载均衡在另一台
// 内网机器上时仍算内网来源；公网直连因为对端地址是公网，会返回 404。
//
// 输出只有进程运行信息、登录计数与网关用量细项的累计值，没有按 IP、用户名、模型的高基数标签；
// 登录限流本身也只统计密码错误，不记录具体账号。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.opts.Metrics || !internalPeer(r) {
		writeErr(w, http.StatusNotFound, "接口不存在")
		return
	}
	version := strings.ReplaceAll(strings.ReplaceAll(s.opts.Version, `\`, `\\`), `"`, `\"`)
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP dsh_ee_uptime_seconds 服务进程已运行的秒数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "dsh_ee_uptime_seconds %.3f\n", time.Since(s.startedAt).Seconds())
	fmt.Fprintf(&b, "# HELP dsh_ee_goroutines 当前 goroutine 数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_goroutines gauge\n")
	fmt.Fprintf(&b, "dsh_ee_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&b, "# HELP dsh_ee_build_info 构建版本信息（值固定为 1）\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_build_info gauge\n")
	fmt.Fprintf(&b, "dsh_ee_build_info{version=\"%s\"} 1\n", version)
	fmt.Fprintf(&b, "# HELP dsh_ee_login_failures_total 用户名或密码错误的登录次数（含 npm login）\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_login_failures_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_login_failures_total %d\n", s.loginFailures.Load())
	fmt.Fprintf(&b, "# HELP dsh_ee_login_ip_limited_total 因同一 IP 失败次数过多被 429 拒绝的登录请求数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_login_ip_limited_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_login_ip_limited_total %d\n", s.loginIPLimited.Load())
	fmt.Fprintf(&b, "# HELP dsh_ee_login_username_limited_total 因用户名锁定被 429 拒绝的登录请求数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_login_username_limited_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_login_username_limited_total %d\n", s.loginUsernameLimited.Load())
	// 嵌入拒绝计数是进程内累计（与登录计数一样，多实例各自独立、重启清零）。
	fmt.Fprintf(&b, "# HELP dsh_ee_embed_denied_total 被拒绝的跨站嵌入请求数（与 embed.deny 审计同口径）\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_embed_denied_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_embed_denied_total %d\n", s.embedDenied.Load())
	// 活跃嵌入会话在数据库里（多实例共享同一份）；数据库没接线或不可用时省略这一行，不影响其它指标。
	if active, ok := s.activeEmbedSessions(r.Context()); ok {
		fmt.Fprintf(&b, "# HELP dsh_ee_embed_sessions_active 当前有效（未过期、未吊销）的嵌入会话数\n")
		fmt.Fprintf(&b, "# TYPE dsh_ee_embed_sessions_active gauge\n")
		fmt.Fprintf(&b, "dsh_ee_embed_sessions_active %d\n", active)
	}
	// 用量细项只在记账成功后累加，和 usage_logs 的累计口径一致；进程内累计，重启清零。
	fmt.Fprintf(&b, "# HELP dsh_ee_usage_reasoning_tokens_total 记账成功的推理（思考）token 累计数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_usage_reasoning_tokens_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_usage_reasoning_tokens_total %d\n", s.usageReasoningTokens.Load())
	fmt.Fprintf(&b, "# HELP dsh_ee_usage_cached_tokens_total 记账成功的命中缓存 prompt token 累计数\n")
	fmt.Fprintf(&b, "# TYPE dsh_ee_usage_cached_tokens_total counter\n")
	fmt.Fprintf(&b, "dsh_ee_usage_cached_tokens_total %d\n", s.usageCachedTokens.Load())

	w.Header().Set("Content-Type", metricsContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// activeEmbedSessions 查数据库里的活跃嵌入会话数（多实例共用一份计数）。
// 超时或查询失败时返回 false，指标行直接省略：抓取不能因为一个可选指标报错。
func (s *Server) activeEmbedSessions(ctx context.Context) (int, bool) {
	if s.store == nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	n, err := s.store.CountActiveEmbedSessionsGlobal(ctx)
	if err != nil {
		return 0, false
	}
	return n, true
}

// internalPeer 判断请求是否来自本机或内网地址。这里刻意不采信 X-Forwarded-For：
// 抓取端应该直接连服务，地址由 TCP 对端决定，客户端伪造不了。
func internalPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
