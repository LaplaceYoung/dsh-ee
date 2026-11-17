package api

import (
	"mime"
	"net/http"
	"strings"
)

// cspFrameAncestorsSelf 是只加 frame-ancestors 收口时的 Content-Security-Policy：
// DSH_EE_CSP=off 的旧行为（回滚用），也是完整策略里 frame-ancestors 的基线。
const cspFrameAncestorsSelf = "frame-ancestors 'self'"

// cspPolicyTemplate 是内置基线策略的固定指令；%s 处填入 index.html 内联脚本的 sha256 源。
// frame-ancestors 不写死在这里：它要和嵌入配置合并，见 cspPolicy。
//
// 默认（DSH_EE_CSP 未设置）就下发这份策略，所以每一项都要对得上前端的实际行为：
//   - default-src 'self'：兜底只允许同源；未单独列出的资源类型（media/worker/manifest 等）跟随它。
//   - script-src 'self' + 内联哈希：Vite 产物是 /assets/*.js；index.html 里唯一的经典内联脚本
//     （首屏主题 + 语言/标题）用启动时按实际下发字节算出的 sha256 放行。不用 'unsafe-inline'，
//     也不用 'unsafe-eval'（前端与依赖都不在运行时生成/执行代码）。
//   - style-src 'self' 'unsafe-inline'：Tailwind 产物是 /assets/*.css；React 的
//     style={{…}} 内联 style 属性只能由 'unsafe-inline' 放行（哈希和 nonce 管不到 style 属性）。
//   - img-src 'self' data: blob: https:：品牌 logo 走同源接口；插件市场图标可能是外部 https
//     图片（registry 只接受 https 或与索引同源的 http）；品牌设置里本地裁剪用 blob: 预览，
//     data: 留给内联图片（例如内联 SVG 数据）。
//   - font-src 'self' data:：前端不加载外部字体，data: 兜底内联字体。
//   - connect-src 'self'：所有 XHR/fetch/SSE 都打同源 /api、/v1、/mcp；没有跨站 API。
//   - base-uri 'self'、object-src 'none'、frame-src 'self'、form-action 'self'：
//     收紧 <base>、插件（object/embed）、子框架与表单目标；前端不用这些跨站能力。
const cspPolicyTemplate = "default-src 'self'; script-src 'self'%s; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:; " +
	"connect-src 'self'; base-uri 'self'; object-src 'none'; form-action 'self'; frame-src 'self'"

// cspMode 归一化 Options.CSP：只认 off / report / enforce（大小写、空白不敏感），
// 空值与其它值都按默认 enforce 处理。config 层对非法的 DSH_EE_CSP 已经 fail closed，
// 这里兜底内部调用方写错的场景：落到安全默认，而不是静默关掉策略。
func cspMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off":
		return "off"
	case "report":
		return "report"
	case "enforce":
		return "enforce"
	default:
		return "enforce"
	}
}

// cspPolicy 生成完整的策略字符串：固定指令 + 内联脚本哈希 + 合并进来的 frame-ancestors
// + 可选的 report-uri。frame-ancestors 只追加，不覆盖嵌入配置。
func cspPolicy(hashes []string, frameAncestors, reportURI string) string {
	policy := strings.Replace(cspPolicyTemplate, "%s", cspScriptSources(hashes), 1)
	policy += "; " + frameAncestors
	if directive := cspReportURI(reportURI); directive != "" {
		policy += "; " + directive
	}
	return policy
}

// cspScriptSources 把 sha256 源列表拼成 " 'sha256-…' 'sha256-…'"；空列表只留 'self'。
func cspScriptSources(hashes []string) string {
	var b strings.Builder
	for _, h := range hashes {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		b.WriteString(" '")
		b.WriteString(h)
		b.WriteString("'")
	}
	return b.String()
}

// cspReportURI 生成 report-uri 指令；地址里出现空白、控制字符或指令分隔符时直接丢弃：
// 这个值直接拼进响应头，不能让它破坏策略或造成头注入（config 层已 fail closed，这里兜底）。
func cspReportURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, r := range raw {
		if r < 0x21 || r == 0x7f || strings.ContainsRune(";,\"'<>", r) {
			return ""
		}
	}
	return "report-uri " + raw
}

// cspFor 算出这次请求要加的两个 CSP 头。
//
//   - enforce（默认，Options.CSP 未设置或无法识别）：完整策略（含合并后的 frame-ancestors）
//     放进 Content-Security-Policy，阻断违规。
//   - report：Content-Security-Policy 仍然保留 frame-ancestors（嵌入收口是强制的，不能因为
//     开报告模式就失效）；完整策略放进 Content-Security-Policy-Report-Only，只报告不阻断。
//   - off：显式关闭完整策略，只保留 frame-ancestors 收口（升级前的旧行为，回滚用）。
func (s *Server) cspFor(r *http.Request) cspHeaders {
	frameAncestors := s.frameAncestors(r)
	switch cspMode(s.opts.CSP) {
	case "off":
		return cspHeaders{enforce: frameAncestors}
	case "report":
		return cspHeaders{
			enforce:    frameAncestors,
			reportOnly: cspPolicy(s.opts.CSPInlineScriptHashes, frameAncestors, s.opts.CSPReportURI),
		}
	default:
		return cspHeaders{enforce: cspPolicy(s.opts.CSPInlineScriptHashes, frameAncestors, s.opts.CSPReportURI)}
	}
}

// frameAncestors 决定哪些页面可以用 iframe 嵌入 dsh-ee 的网页。只有微应用网页版
// （/a/<标识>）放行有效配置里的宿主 origin；其它页面（尤其是 /admin 管理页）一律 'self'，
// 免得把管理后台也框进配置过的宿主里。
func (s *Server) frameAncestors(r *http.Request) string {
	csp := cspFrameAncestorsSelf
	if isEmbedPage(r.URL.Path) {
		if cfg, err := s.embedConfig(r.Context()); err == nil {
			for _, origin := range cfg.origins {
				csp += " " + origin
			}
		}
	}
	return csp
}

// securityHeaders 给 HTML 文档加安全头。
//
// CSP 的 frame-ancestors 决定哪些页面可以用 iframe 嵌入 dsh-ee 的网页（#118）；在其之上，
// 默认会再给 HTML 加一份内置的完整内容安全策略（DSH_EE_CSP=enforce，未设置时的默认值）。
// DSH_EE_CSP=report 时完整策略只报告不阻断，DSH_EE_CSP=off 时回到只加 frame-ancestors 的旧行为。
// 其余三个头是通用的低风险加固。只加在 HTML 出口上：静态资源、JSON、3xx 重定向、SSE 流都不受影响。
//
// 不设 X-Frame-Options：ALLOW-FROM 已被废弃，现代浏览器看 frame-ancestors；两边各写一份
// （反代一份、Go 一份）反而容易出现「改了其中一边以为放行了」的歧义，CSP 是唯一事实源。
//
// 来源每请求从「env ∪ settings.embed」解析（appSettings 有 5 秒缓存），管理端保存后立即生效。
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&securityHeadersWriter{ResponseWriter: w, csp: s.cspFor(r)}, r)
	})
}

// isEmbedPage 只有微应用网页版的路由可以配置宿主 origin；/admin、/login、/go 等页面
// 保持只允许同站，避免 DSH_EE_EMBED_ORIGINS 顺带把管理页面也放出去。
func isEmbedPage(path string) bool {
	return strings.HasPrefix(path, "/a/")
}

// cspHeaders 是这次请求要写的 CSP 头；空字符串表示不加对应的头。
type cspHeaders struct {
	// enforce 写 Content-Security-Policy（会执行）。
	enforce string
	// reportOnly 写 Content-Security-Policy-Report-Only（只报告）。
	reportOnly string
}

// securityHeadersWriter 在第一次写响应前检查 Content-Type，只给 HTML 加头。
// 响应头必须先于 WriteHeader 落到 header map 里，所以 WriteHeader 和 Write 两个入口都要判断
// （handler 不调 WriteHeader 时由第一次 Write 隐式写 200）。
type securityHeadersWriter struct {
	http.ResponseWriter
	csp   cspHeaders
	wrote bool
}

func (w *securityHeadersWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	// 1xx 只是预告，真正的响应头后面还会写。
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.wrote = true
	// 3xx 不是可渲染的文档：http.Redirect 会给 GET 加一个 text/html 的跳转体，
	// 但这层加的是文档安全头，不加在重定向上，免得覆盖 handoff 这类接口自己设的 no-referrer。
	if status < 300 || status >= 400 {
		w.setSecurityHeaders()
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *securityHeadersWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		// handler 没写 WriteHeader，第一次 Write 隐式写 200。
		w.wrote = true
		w.setSecurityHeaders()
	}
	return w.ResponseWriter.Write(p)
}

// setSecurityHeaders 只给 HTML 出口加头；非 HTML（JSON、静态资源、SSE）保持原样。
func (w *securityHeadersWriter) setSecurityHeaders() {
	if !isHTMLContentType(w.Header().Get("Content-Type")) {
		return
	}
	h := w.Header()
	if w.csp.enforce != "" {
		h.Set("Content-Security-Policy", w.csp.enforce)
	}
	if w.csp.reportOnly != "" {
		h.Set("Content-Security-Policy-Report-Only", w.csp.reportOnly)
	}
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

// Flush 让 SSE（网关流式回答、/mcp 事件流）穿过这层中间件。
func (w *securityHeadersWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap 供 http.ResponseController 取回底层 ResponseWriter。
func (w *securityHeadersWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func isHTMLContentType(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "text/html"
}
