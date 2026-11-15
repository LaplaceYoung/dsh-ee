package api

// DeepSeek 工具代理（docs/dev/deepseek-tools.md）。
//
// DeepSeek 开放平台把 App 能力拆成了独立工具 API：Files（上传图片/文档后用 file_id 在
// 对话里引用）、ASR（整段音频转写）、TTS（文本合成音频）、Search / Fetch（搜索网页、
// 抓取网页正文）、OCR（按页识别文档或图片中的文字），以及 Anthropic 兼容的 Files。
// dsh-ee 把它们代理到「模型接入」里配置的一个上游，让管理员 / 机器人等受信客户端用同一个
// dsh-ee 令牌调用，不必各自持有上游 API key。
//
// 代理只做几件事：复用模型网关的鉴权（models scope + llm.use）并叠加访问边界（默认只有
// 管理员与机器人/服务账号可用）、按部署配置选上游、限制请求体大小与超时、把请求与响应
// 原样转发、给文件类操作写审计。它不解析 multipart/JSON、不落盘、不缓存、不记录文件内容、
// 音频、转写/识别文本；上游的状态码与错误体原样保留，dsh-ee 自己产生的错误用 OpenAI
// 错误壳。这些调用按上游工具计费，不消耗 dsh-ee 的 token 额度，也不写 token 用量统计
// （例外：在对话里引用 file_id 时，拼入上下文的输入 token 按普通对话记账）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

const (
	// deepseekFilesMaxBytes 是 POST /v1/files 与 Anthropic 兼容上传接受的请求体上限。
	// DeepSeek 按 purpose 限制单文件大小（图片 64 MiB、PDF/Office 100 MiB、纯文本/表格
	// 32 MiB），这里取最大值作为统一边界；更细的格式与大小由上游在解析 multipart 后校验。
	deepseekFilesMaxBytes int64 = 100 << 20
	// deepseekOCRMaxBytes 是 OCR 单文件上限（上游文档：100 MiB，超过返回 413）。
	deepseekOCRMaxBytes int64 = 100 << 20
	// deepseekASRMaxBytes 是 ASR 单次音频上限（上游文档：最大 8 MiB）。
	deepseekASRMaxBytes int64 = 8 << 20
	// deepseekTTSMaxBytes / deepseekSearchMaxBytes / deepseekFetchMaxBytes 是 JSON 请求体
	// 上限。上游对这几个接口的输入本来就很短（TTS 文档 64 KiB；Search 的 query 1–128 字符；
	// Fetch 的 url ≤8192 字节），统一取 64 KiB 挡住异常大包，又不影响正常字段。
	deepseekTTSMaxBytes    int64 = 64 << 10
	deepseekSearchMaxBytes int64 = 64 << 10
	deepseekFetchMaxBytes  int64 = 64 << 10

	// 超时。上传/流式合成给足时间（上游允许 100 MiB 文档在 120 秒内、其它文件在 10 分钟内
	// 传完；TTS 单次最多 30 分钟音频；OCR 从开始上传算起总时长 180 秒），只读元数据与
	// 普通 JSON 工具用较短超时兜底。
	deepseekFilesUploadTimeout = 10 * time.Minute
	deepseekFilesMetaTimeout   = 30 * time.Second
	deepseekASRTimeout         = 2 * time.Minute
	deepseekTTSTimeout         = 10 * time.Minute
	deepseekSearchTimeout      = 30 * time.Second
	deepseekFetchTimeout       = time.Minute
	deepseekOCRTimeout         = 5 * time.Minute
	// Anthropic 兼容 Files 与 OpenAI 兼容 Files 的上传/元数据超时保持一致。
	deepseekAnthropicFilesUploadTimeout = 10 * time.Minute
	deepseekAnthropicFilesMetaTimeout   = 30 * time.Second

	// deepseekAuditedResponseMaxBytes 是文件类接口为写审计而缓冲的上游响应上限。
	// 文件对象/列表/删除结果都是小 JSON，正常远小于这个值；超过时按已读部分回写并记日志。
	deepseekAuditedResponseMaxBytes int64 = 4 << 20

	// deepseekToolsVendor 是「模型接入」里 DeepSeek 官方上游的 vendor 标识；别与模型网关
	// 的 vendor 常量混用，工具代理只按它做默认上游选择。
	deepseekToolsVendor = "deepseek"

	// deepseekProviderUnavailable 是没有可用 DeepSeek 上游时的稳定错误码，客户端可以据此
	// 提示管理员配置，而不是当成临时网络错误盲目重试。
	deepseekProviderUnavailable = "deepseek_provider_unavailable"
	// deepseekToolsForbiddenCode 是普通员工调用文件/工具接口被访问边界拦下时的错误码。
	deepseekToolsForbiddenCode = "deepseek_tools_forbidden"
	// fileAccessDeniedCode 是普通员工在聊天里引用 file_id 被拦下时的错误码（gateway.go 共用）。
	fileAccessDeniedCode = "file_access_denied"

	// deepseekAnthropicBetaHeader 是 Anthropic 兼容 Files 端点必须携带的 beta 头。
	// dsh-ee 不校验也不补这个头（缺失由上游返回 400），只把它原样转发给上游。
	deepseekAnthropicBetaHeader = "anthropic-beta"
	deepseekAnthropicBetaValue  = "files-api-2025-04-14"
)

// deepseekFileIDRe 限定 file_id 的形状（DeepSeek 形如 file-api-xxxxxxxxxxxxxxxx）。
// 代理把它拼进上游 URL 的路径，必须挡住 "/"、"."、".." 与百分号编码等能把请求带离
// /files/{id} 的字符；file_id 不是调用方数据，只允许字母、数字、点、下划线和短横线。
var deepseekFileIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// errDeepSeekBodyTooLarge 是转发过程中发现请求体超过上限时，body reader 返回给
// http.Client 的错误；handler 通过 deepseekLimitedBody.exceeded 判断并回 413。
var errDeepSeekBodyTooLarge = errors.New("request body exceeds deepseek tools limit")

var errDeepSeekProviderUnavailable = errors.New("deepseek provider unavailable")

// ---- 上游选择 ----

// deepseekTarget 是工具代理这次请求要用的上游与解密后的 key。
type deepseekTarget struct {
	provider *store.Provider
	key      []byte
}

// deepseekProvider 选出工具代理使用的启用上游并解密它的 key。
//
// 选择规则（docs/dev/deepseek-tools.md）：
//   - 配置了 DSH_EE_DEEPSEEK_PROVIDER 时，按 provider 的 name 精确匹配，name 不中再按
//     数字 id 匹配；只在该上游启用时可用。显式配置由管理员负责，不额外要求
//     vendor=deepseek（有些企业用自建网关转 DeepSeek，vendor 会填 custom）。
//   - 没配置时取第一个启用且 vendor=deepseek 的上游（ListProviders 已按 priority、id
//     排序，进程内每次请求重新解析，管理端改完立即生效，不用重启）。
//   - 找不到启用上游或 key 解不开时返回 errDeepSeekProviderUnavailable。
//
// 这里只读取 provider 的 BaseURL 与密文 key：请求永远只发往管理员配置的上游地址，
// 不从客户端取 base_url，也不做模型网关的多上游故障切换（file_id 绑定在上传时的账号上，
// 换上游必然失效，切换没有意义）。
func (s *Server) deepseekProvider(ctx context.Context) (*store.Provider, []byte, error) {
	ps, err := s.store.ListProviders(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	var picked *store.Provider
	wanted := strings.TrimSpace(s.opts.DeepSeekProvider)
	if wanted == "" {
		for _, p := range ps {
			if p.Vendor == deepseekToolsVendor {
				picked = p
				break
			}
		}
	} else {
		for _, p := range ps {
			if p.Name == wanted {
				picked = p
				break
			}
		}
		if picked == nil {
			if id, perr := strconv.ParseInt(wanted, 10, 64); perr == nil && id > 0 {
				for _, p := range ps {
					if p.ID == id {
						picked = p
						break
					}
				}
			}
		}
	}
	if picked == nil {
		return nil, nil, errDeepSeekProviderUnavailable
	}
	key, err := s.box.Open(picked.APIKeyEnc)
	if err != nil {
		// key 解不开说明这条上游配置坏了；日志里只写 provider 名称，不带密文/明文。
		s.log.Error("deepseek tools provider key decrypt", "provider", picked.Name, "err", err)
		return nil, nil, errDeepSeekProviderUnavailable
	}
	return picked, key, nil
}

// resolveDeepSeekTarget 解析上游；失败时已写好响应（503 或 500）。
func (s *Server) resolveDeepSeekTarget(w http.ResponseWriter, r *http.Request) (*deepseekTarget, bool) {
	p, key, err := s.deepseekProvider(r.Context())
	if err != nil {
		if errors.Is(err, errDeepSeekProviderUnavailable) {
			writeOpenAIErr(w, http.StatusServiceUnavailable, "server_error", deepseekProviderUnavailable,
				"没有可用的 DeepSeek 上游：请管理员在「模型接入」添加 vendor=deepseek 的上游，或用 DSH_EE_DEEPSEEK_PROVIDER 指定")
			return nil, false
		}
		s.internalErr(w, r, err)
		return nil, false
	}
	return &deepseekTarget{provider: p, key: key}, true
}

// newRequest 构造发往选中上游的请求：只复用配置的 BaseURL 与解密出的 key。
// 不转发客户端的 Authorization / Cookie / Host / x-api-key 等头，避免把 dsh-ee 令牌带给上游。
func (t *deepseekTarget) newRequest(ctx context.Context, method, pathAndQuery string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, t.provider.BaseURL+pathAndQuery, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+string(t.key))
	return req, nil
}

// ---- 访问边界 ----

// deepseekToolsAllowed 报告当前主体能不能使用 DeepSeek 文件/工具面以及引用 file_id：
//   - 部署开启 DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE 后对所有登录账号开放；
//   - 否则只允许管理员（账号带任一管理权限点）与机器人/服务账号（account_type=bot）。
//
// 文件属于上游共享账号、员工之间不隔离，所以默认不对普通员工开放；聊天里引用 file_id
// 也走同一个判断（gateway.go），避免「不给接口、但已知 id 仍能读」的绕过。
func (s *Server) deepseekToolsAllowed(p *Principal) bool {
	if s.opts.DeepSeekToolsAllowEmployee {
		return true
	}
	if p == nil {
		return false
	}
	if p.CanAdmin {
		return true
	}
	return p.User != nil && p.User.AccountType == store.AccountBot
}

// deepseekToolPrincipal 在模型网关鉴权之上再加文件/工具面的访问边界；拒绝时已写好 403。
func (s *Server) deepseekToolPrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	p, _, ok := s.authKey(w, r)
	if !ok {
		return nil, false
	}
	if !s.deepseekToolsAllowed(p) {
		writeOpenAIErr(w, http.StatusForbidden, "permission_error", deepseekToolsForbiddenCode,
			"DeepSeek 文件与工具接口默认只允许管理员和机器人/服务账号调用；普通员工需要管理员开启 DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE")
		return nil, false
	}
	return p, true
}

// ---- 文件操作审计 ----

// auditDeepSeekFiles 记一条文件代理的审计：actor / token 由 auditToken 填，target 是
// file_id（列表操作用 "files"），detail 只有接口类型与上游状态码。绝不记录文件名、
// 文件内容或识别文本。
func (s *Server) auditDeepSeekFiles(r *http.Request, p *Principal, action, target, api string, status int) {
	s.auditToken(r, p, action, target, map[string]any{"api": api, "status": status})
}

// deepseekFileIDFromJSON 取上游文件对象里的 id（OpenAI / Anthropic 兼容 Files 都用 id）。
// 解析失败返回空字符串，审计仍然记录 status。
func deepseekFileIDFromJSON(data []byte) string {
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &obj) != nil {
		return ""
	}
	return strings.TrimSpace(obj.ID)
}

// ---- 请求边界 ----

// deepseekLimitedBody 把客户端请求体边读边转给上游，最多转发 limit 字节。
// 到限后先探测是否还有数据（不转发探测到的字节）：还有就返回 errDeepSeekBodyTooLarge
// 并记下 exceeded，让 handler 回 413；恰好读满且在限处结束则正常返回 EOF。
type deepseekLimitedBody struct {
	r         io.Reader
	remaining int64
	exceeded  atomic.Bool
}

func (b *deepseekLimitedBody) Read(p []byte) (int, error) {
	if b.remaining > 0 {
		if int64(len(p)) > b.remaining {
			p = p[:b.remaining]
		}
		n, err := b.r.Read(p)
		b.remaining -= int64(n)
		return n, err
	}
	var probe [1]byte
	n, err := b.r.Read(probe[:])
	if n > 0 {
		b.exceeded.Store(true)
		return 0, errDeepSeekBodyTooLarge
	}
	return 0, err
}

// writeDeepSeekTooLarge 回 413，错误体沿用网关的 OpenAI 错误壳。
func writeDeepSeekTooLarge(w http.ResponseWriter, limit string) {
	writeOpenAIErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large",
		"请求体过大：这个接口最多接受 "+limit)
}

// readDeepSeekBody 读完一个受大小限制的请求体（ASR/TTS/Search/Fetch 这类本来就小的请求）。
// 超限时回 413，读取失败回 400。
func readDeepSeekBody(w http.ResponseWriter, r *http.Request, limit int64, human string) ([]byte, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", "请求体读取失败")
		return nil, false
	}
	if int64(len(raw)) > limit {
		writeDeepSeekTooLarge(w, human)
		return nil, false
	}
	return raw, true
}

// ---- 上游响应 ----

// deepseekUpstreamError 写上游网络错误对应的响应。客户端已断开时不写。
// 日志只带 op / provider / error（不含请求体、文件内容或令牌）。
func (s *Server) deepseekUpstreamError(w http.ResponseWriter, r *http.Request, tgt *deepseekTarget, op string, err error) {
	if r.Context().Err() != nil {
		s.log.Debug("deepseek tools client gone", "op", op)
		return
	}
	s.log.Warn("deepseek tools upstream failed", "op", op, "provider", tgt.provider.Name, "err", err)
	if errors.Is(err, context.DeadlineExceeded) {
		writeOpenAIErr(w, http.StatusGatewayTimeout, "server_error", "upstream_timeout", "上游服务超时")
		return
	}
	writeOpenAIErr(w, http.StatusBadGateway, "server_error", "upstream_unreachable", "上游服务不可达")
}

// doDeepSeek 发上游请求；失败时已写好错误响应。
func (s *Server) doDeepSeek(w http.ResponseWriter, r *http.Request, tgt *deepseekTarget, req *http.Request, op string) (*http.Response, bool) {
	resp, err := s.upstream.Do(req)
	if err != nil {
		s.deepseekUpstreamError(w, r, tgt, op, err)
		return nil, false
	}
	return resp, true
}

// relayDeepSeekResponse 把上游响应回写给客户端：状态码、Content-Type、Retry-After 原样，
// 响应体流式转发（TTS 音频、OCR 的换行保活边收边回传）。不落盘、不缓存、不解析内容。
// 统一加 Cache-Control: no-store，避免反向代理/中间层缓存文件元数据、转写/识别文本或音频。
func (s *Server) relayDeepSeekResponse(w http.ResponseWriter, resp *http.Response, flush bool, op string) {
	defer resp.Body.Close()
	writeDeepSeekResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // 客户端断开
			}
			if flush && flusher != nil {
				flusher.Flush()
			}
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			// 状态码已经写出，无法再换错误体；只记一条不含内容的日志。
			s.log.Warn("deepseek tools upstream body interrupted", "op", op)
		}
		if flusher != nil && flush {
			flusher.Flush()
		}
		return
	}
}

// relayDeepSeekResponseBuffered 用于要写审计的文件接口：上游响应是小 JSON（文件对象 /
// 列表 / 删除结果），先读入内存（上限 4 MiB）拿到 file_id 并写审计，再原样回写。
// 不落盘；超过上限时只保留已读部分并记日志。
func (s *Server) relayDeepSeekResponseBuffered(w http.ResponseWriter, resp *http.Response, op string, onResult func(status int, body []byte)) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, deepseekAuditedResponseMaxBytes+1))
	if int64(len(data)) > deepseekAuditedResponseMaxBytes {
		data = data[:deepseekAuditedResponseMaxBytes]
		s.log.Warn("deepseek tools audited response truncated", "op", op)
	}
	if err != nil && resp.StatusCode < http.StatusBadRequest {
		s.log.Warn("deepseek tools read audited response", "op", op)
	}
	if onResult != nil {
		onResult(resp.StatusCode, data)
	}
	writeDeepSeekResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(data)
}

// writeDeepSeekResponseHeaders 原样带上上游的 Content-Type / Retry-After，并统一禁缓存。
func writeDeepSeekResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		w.Header().Set("Retry-After", ra)
	}
	w.Header().Set("Cache-Control", "no-store")
}

// ---- 通用代理流程 ----

// proxyDeepSeekJSON 代理「JSON 请求体 → 上游响应」的端点（TTS / Search / Fetch）。
// flush=true 用于音频等流式响应（TTS）。
func (s *Server) proxyDeepSeekJSON(w http.ResponseWriter, r *http.Request, upstreamPath, op, limitHuman string, maxBytes int64, timeout time.Duration, flush bool) {
	if r.ContentLength > maxBytes {
		writeDeepSeekTooLarge(w, limitHuman)
		return
	}
	tgt, ok := s.resolveDeepSeekTarget(w, r)
	if !ok {
		return
	}
	raw, ok := readDeepSeekBody(w, r, maxBytes, limitHuman)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	req, err := tgt.newRequest(ctx, http.MethodPost, upstreamPath, bytes.NewReader(raw))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, ok := s.doDeepSeek(w, r, tgt, req, op)
	if !ok {
		return
	}
	s.relayDeepSeekResponse(w, resp, flush, op)
}

// proxyDeepSeekUpload 代理「multipart/二进制请求体流式转发」的上传类端点
// （Files 上传、ASR、OCR、Anthropic 兼容 Files 上传）。请求体不落盘、不解析，
// 只按 Content-Length/流式计数限制大小；prep 可以再补上游要求的头（如 anthropic-beta）；
// onResult 非 nil 时缓冲小 JSON 响应用于写审计（文件类接口用）。
func (s *Server) proxyDeepSeekUpload(w http.ResponseWriter, r *http.Request, upstreamPath, op, limitHuman string, maxBytes int64, timeout time.Duration, flush bool, prep func(req *http.Request), onResult func(status int, body []byte)) {
	if r.ContentLength > maxBytes {
		writeDeepSeekTooLarge(w, limitHuman)
		return
	}
	tgt, ok := s.resolveDeepSeekTarget(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	body := &deepseekLimitedBody{r: r.Body, remaining: maxBytes}
	req, err := tgt.newRequest(ctx, http.MethodPost, upstreamPath, body)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if prep != nil {
		prep(req)
	}
	req.ContentLength = r.ContentLength

	resp, err := s.upstream.Do(req)
	if err != nil {
		if body.exceeded.Load() {
			writeDeepSeekTooLarge(w, limitHuman)
			return
		}
		s.deepseekUpstreamError(w, r, tgt, op, err)
		return
	}
	if onResult != nil {
		s.relayDeepSeekResponseBuffered(w, resp, op, onResult)
		return
	}
	s.relayDeepSeekResponse(w, resp, flush, op)
}

// proxyDeepSeekPassThrough 代理 GET/DELETE 这类没有请求体的端点：查询串原样转发，
// 回写上游响应。prep 可以再补上游要求的头（如 anthropic-beta）；onResult 非 nil 时
// 缓冲小 JSON 响应用于写审计（文件列表/删除用）。
func (s *Server) proxyDeepSeekPassThrough(w http.ResponseWriter, r *http.Request, method, upstreamPath, op string, timeout time.Duration, prep func(req *http.Request), onResult func(status int, body []byte)) {
	tgt, ok := s.resolveDeepSeekTarget(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	path := upstreamPath
	if q := r.URL.RawQuery; q != "" {
		path += "?" + q
	}
	req, err := tgt.newRequest(ctx, method, path, nil)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if prep != nil {
		prep(req)
	}
	resp, ok := s.doDeepSeek(w, r, tgt, req, op)
	if !ok {
		return
	}
	if onResult != nil {
		s.relayDeepSeekResponseBuffered(w, resp, op, onResult)
		return
	}
	s.relayDeepSeekResponse(w, resp, false, op)
}

// deepseekFileIDFromPath 取并校验路径里的 file_id。file_id 会拼进上游 URL 路径，
// 先按允许的字符集校验，挡住路径注入（编码后的 "/"、"."、".." 等）。
func deepseekFileIDFromPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !deepseekFileIDRe.MatchString(id) {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", "invalid_file_id",
			"file_id 不合法：只允许字母、数字、点、下划线和短横线，且不能以点开头")
		return "", false
	}
	return id, true
}

// ---- OpenAI 兼容 Files API ----

// handleDeepSeekFilesUpload 代理 POST /v1/files：multipart 原样流式转发到 <base>/files
// （不解析字段，purpose 排在 file 前等上游要求因此不受影响），上限 100 MiB；写审计。
func (s *Server) handleDeepSeekFilesUpload(w http.ResponseWriter, r *http.Request) {
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekUpload(w, r, "/files", "files.upload", "100 MiB", deepseekFilesMaxBytes, deepseekFilesUploadTimeout, false, nil, func(status int, body []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesUpload, deepseekFileIDFromJSON(body), "openai", status)
	})
}

// handleDeepSeekFilesList 代理 GET /v1/files：查询串（after/limit/order/purpose）原样转发；写审计。
func (s *Server) handleDeepSeekFilesList(w http.ResponseWriter, r *http.Request) {
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodGet, "/files", "files.list", deepseekFilesMetaTimeout, nil, func(status int, _ []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesList, "files", "openai", status)
	})
}

// handleDeepSeekFilesGet 代理 GET /v1/files/{id}。
func (s *Server) handleDeepSeekFilesGet(w http.ResponseWriter, r *http.Request) {
	id, ok := deepseekFileIDFromPath(w, r)
	if !ok {
		return
	}
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodGet, "/files/"+id, "files.get", deepseekFilesMetaTimeout, nil, nil)
}

// handleDeepSeekFilesDelete 代理 DELETE /v1/files/{id}；写审计（只记 file_id，不记文件名）。
func (s *Server) handleDeepSeekFilesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := deepseekFileIDFromPath(w, r)
	if !ok {
		return
	}
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodDelete, "/files/"+id, "files.delete", deepseekFilesMetaTimeout, nil, func(status int, _ []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesDelete, id, "openai", status)
	})
}

// ---- ASR / TTS / Search / Fetch / OCR ----

// handleDeepSeekASR 代理 POST /v1/tools/asr：multipart 音频原样转发，上限 8 MiB。
func (s *Server) handleDeepSeekASR(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekUpload(w, r, "/v1/tools/asr", "tools.asr", "8 MiB", deepseekASRMaxBytes, deepseekASRTimeout, false, nil, nil)
}

// handleDeepSeekTTS 代理 POST /v1/tools/tts：JSON 转发，音频在响应体里边合成边回传，
// 逐块转发并 flush，不在 dsh-ee 缓冲完整音频。
func (s *Server) handleDeepSeekTTS(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekJSON(w, r, "/v1/tools/tts", "tools.tts", "64 KiB", deepseekTTSMaxBytes, deepseekTTSTimeout, true)
}

// handleDeepSeekSearch 代理 POST /v1/tools/search：JSON 原样转发，返回上游结果数组。
func (s *Server) handleDeepSeekSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekJSON(w, r, "/v1/tools/search", "tools.search", "64 KiB", deepseekSearchMaxBytes, deepseekSearchTimeout, false)
}

// handleDeepSeekFetch 代理 POST /v1/tools/fetch：JSON 原样转发，返回网页 Markdown。
func (s *Server) handleDeepSeekFetch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekJSON(w, r, "/v1/tools/fetch", "tools.fetch", "64 KiB", deepseekFetchMaxBytes, deepseekFetchTimeout, false)
}

// handleDeepSeekOCR 代理 POST /v1/tools/ocr：multipart 原样流式转发，上限 100 MiB。
// 上游在识别期间持续发送换行符保活，完成后才返回 JSON；这里逐块 flush 原样透传字节与
// Content-Type，客户端收到的整个响应体仍是「前导换行 + JSON」。
func (s *Server) handleDeepSeekOCR(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekUpload(w, r, "/v1/tools/ocr", "tools.ocr", "100 MiB", deepseekOCRMaxBytes, deepseekOCRTimeout, true, nil, nil)
}

// ---- Anthropic 兼容 Files API ----

// normalizeAnthropicAuth 让 Anthropic 兼容端点既接受 Authorization: Bearer，也接受
// Anthropic SDK 默认的 x-api-key（等价 Bearer）。只改本次请求的内存头，不转发给上游：
// 上游拿到的始终是企业配置的 key。
func normalizeAnthropicAuth(r *http.Request) {
	if r.Header.Get("Authorization") != "" {
		return
	}
	if key := strings.TrimSpace(r.Header.Get("x-api-key")); key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
}

// copyAnthropicBetaHeaders 把客户端带来的 anthropic-beta / anthropic-version 头原样转发。
// Anthropic 兼容 Files 端点必须携带 anthropic-beta: files-api-2025-04-14；dsh-ee 不校验、
// 不补默认值（缺失时由上游返回 400），只保证这个头能到达上游。
func copyAnthropicBetaHeaders(r *http.Request, req *http.Request) {
	for _, name := range []string{deepseekAnthropicBetaHeader, "anthropic-version"} {
		for _, v := range r.Header.Values(name) {
			req.Header.Add(name, v)
		}
	}
}

// handleDeepSeekAnthropicFilesUpload 代理 POST /anthropic/v1/files（multipart 原样转发，
// 100 MiB，保留 anthropic-beta 头）；写审计。
func (s *Server) handleDeepSeekAnthropicFilesUpload(w http.ResponseWriter, r *http.Request) {
	normalizeAnthropicAuth(r)
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekUpload(w, r, "/anthropic/v1/files", "anthropic.files.upload", "100 MiB", deepseekFilesMaxBytes, deepseekAnthropicFilesUploadTimeout, false, func(req *http.Request) {
		copyAnthropicBetaHeaders(r, req)
	}, func(status int, body []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesUpload, deepseekFileIDFromJSON(body), "anthropic", status)
	})
}

// handleDeepSeekAnthropicFilesList 代理 GET /anthropic/v1/files（查询串 after_id/before_id/
// limit 原样转发，保留 anthropic-beta 头）；写审计。
func (s *Server) handleDeepSeekAnthropicFilesList(w http.ResponseWriter, r *http.Request) {
	normalizeAnthropicAuth(r)
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodGet, "/anthropic/v1/files", "anthropic.files.list", deepseekAnthropicFilesMetaTimeout, func(req *http.Request) {
		copyAnthropicBetaHeaders(r, req)
	}, func(status int, _ []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesList, "files", "anthropic", status)
	})
}

// handleDeepSeekAnthropicFilesGet 代理 GET /anthropic/v1/files/{id}。
func (s *Server) handleDeepSeekAnthropicFilesGet(w http.ResponseWriter, r *http.Request) {
	id, ok := deepseekFileIDFromPath(w, r)
	if !ok {
		return
	}
	normalizeAnthropicAuth(r)
	if _, ok := s.deepseekToolPrincipal(w, r); !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodGet, "/anthropic/v1/files/"+id, "anthropic.files.get", deepseekAnthropicFilesMetaTimeout, func(req *http.Request) {
		copyAnthropicBetaHeaders(r, req)
	}, nil)
}

// handleDeepSeekAnthropicFilesDelete 代理 DELETE /anthropic/v1/files/{id}；写审计。
func (s *Server) handleDeepSeekAnthropicFilesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := deepseekFileIDFromPath(w, r)
	if !ok {
		return
	}
	normalizeAnthropicAuth(r)
	p, ok := s.deepseekToolPrincipal(w, r)
	if !ok {
		return
	}
	s.proxyDeepSeekPassThrough(w, r, http.MethodDelete, "/anthropic/v1/files/"+id, "anthropic.files.delete", deepseekAnthropicFilesMetaTimeout, func(req *http.Request) {
		copyAnthropicBetaHeaders(r, req)
	}, func(status int, _ []byte) {
		s.auditDeepSeekFiles(r, p, auditFilesDelete, id, "anthropic", status)
	})
}
