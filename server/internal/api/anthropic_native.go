package api

// DeepSeek 原生 Anthropic 透传。
//
// 当对外模型候选线路里第一条（最高优先级）的 provider vendor 是 deepseek 时，/v1/messages
// 直接把请求转发到 <provider.base>/anthropic/v1/messages：dsh-ee 只做鉴权、模型权限、额度、
// 限流、model 改写、响应逐字节转发与 usage 旁路记账，不解析也不改写 server tools / citations /
// safety / title / latest_reminder / file / vision 等 DeepSeek 扩展块。
//
// 其它 vendor 仍走 anthropic_convert.go 的「Anthropic → OpenAI chat」降级转换层。
// 故障转移只在同协议内进行：第一条线路是 deepseek 就只在 deepseek 线路之间切换，反之亦然，
// 不跨协议（见 docs/dev/anthropic-api.md）。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// anthropicNativeVendor 是支持原生 Anthropic 格式的 provider vendor。
const anthropicNativeVendor = "deepseek"

// anthropicNativeVendorAllowed 判断 provider 是否支持原生透传（vendor 大小写不敏感）。
func anthropicNativeVendorAllowed(p *store.Provider) bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.Vendor), anthropicNativeVendor)
}

// anthropicNativeURL 拼上游原生 Messages 地址：<base>/anthropic/v1/messages。
// 管理端按 DeepSeek 官方文档把 base_url 配成 https://api.deepseek.com 即可；
// 已经带 /anthropic 后缀的配置也兼容，避免拼成 /anthropic/anthropic/...
func anthropicNativeURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/anthropic") {
		return base + "/v1/messages"
	}
	return base + "/anthropic/v1/messages"
}

// splitAnthropicCandidates 按 vendor 把候选线路分成「原生透传」与「降级转换」两组；
// 两组各自保持原有优先级顺序，方便调用方按第一条线路决定协议后只在同协议内故障转移。
func splitAnthropicCandidates(cands []store.Candidate) (native, converted []store.Candidate) {
	for _, c := range cands {
		if anthropicNativeVendorAllowed(c.Provider) {
			native = append(native, c)
		} else {
			converted = append(converted, c)
		}
	}
	return native, converted
}

// anthropicUsage 是 Anthropic 原生响应/SSE 里的 token 用量。
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// promptTokens 把 Anthropic 的输入拆项合并成记账口径（与 OpenAI 网关的 prompt_tokens 一致：
// 含缓存创建与缓存命中，因为额度按 token 总量统计）。
func (u *anthropicUsage) promptTokens() int {
	if u == nil {
		return 0
	}
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// mergeAnthropicUsage 合并流式里多次出现的 usage（message_start 给 input，message_delta 给
// output）；同一个字段取最大值，避免后到的部分字段把已拿到的完整值覆盖成 0。
func mergeAnthropicUsage(dst *tokenUsage, u *anthropicUsage) {
	if n := u.promptTokens(); n > dst.PromptTokens {
		dst.PromptTokens = n
	}
	if u.OutputTokens > dst.CompletionTokens {
		dst.CompletionTokens = u.OutputTokens
	}
	dst.TotalTokens = dst.PromptTokens + dst.CompletionTokens
}

// openAnthropicNative 按候选线路依次尝试 DeepSeek 原生端点。策略与 openChatUpstream 一致：
// 连不上、429、5xx 换下一条；开始向客户端写响应后不再切换；失败时已经按 writeErr 写好响应。
func (s *Server) openAnthropicNative(w http.ResponseWriter, r *http.Request, k *Principal, model string, body map[string]any, stream bool, start time.Time, cands []store.Candidate, writeErr llmErrWriter) (*http.Response, *store.Provider, bool) {
	var resp *http.Response
	var p *store.Provider
	for i, c := range cands {
		last := i == len(cands)-1
		p = c.Provider
		cooldownKey := providerCooldownKey{providerID: p.ID, model: c.UpstreamModel}
		cooldown := s.providerCD.snapshot(cooldownKey, time.Now())
		// 冷却中的线路直接跳过；全部冷却时仍会尝试最后一条兜底。
		if !last && cooldown.cooling() {
			continue
		}
		upstreamKey, err := s.openProviderKey(p)
		if err != nil {
			s.log.Error("provider key unavailable", "provider", p.Name, "err", err)
			if last {
				writeErr(http.StatusInternalServerError, "server_error", "provider_misconfigured", "上游配置有误，请联系管理员")
				return nil, nil, false
			}
			continue
		}
		// 只改写 model（对外名 → 上游名），其余字段按请求原样转发。
		body["model"] = c.UpstreamModel
		out, err := json.Marshal(body)
		if err != nil {
			writeErr(http.StatusBadRequest, "invalid_request_error", "invalid_json", "请求体无法转发")
			return nil, nil, false
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, anthropicNativeURL(p.BaseURL), bytes.NewReader(out))
		if err != nil {
			s.log.Error("build native anthropic request", "provider", p.Name, "err", err)
			writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
			return nil, nil, false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+string(upstreamKey))
		// anthropic-version / anthropic-beta 原样透传（含 files-api 等 beta 头）；
		// dsh-ee 不解析这些头，是否支持由 DeepSeek 上游决定。
		for _, name := range []string{"Anthropic-Version", "Anthropic-Beta"} {
			for _, v := range r.Header.Values(name) {
				req.Header.Add(name, v)
			}
		}
		if stream {
			req.Header.Set("Accept", "text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
		resp, err = s.upstream.Do(req)
		if err != nil {
			if r.Context().Err() == nil {
				s.providerCD.mark(providerCooldownKey{providerID: p.ID}, providerCooldownDefault, time.Now())
			}
			s.log.Warn("native anthropic upstream failed", "provider", p.Name, "err", err)
			s.recordUsage(r.Context(), k, p, model, tokenUsage{}, http.StatusBadGateway, start)
			if r.Context().Err() != nil {
				return nil, nil, false // 客户端已断开
			}
			if last {
				writeErr(http.StatusBadGateway, "server_error", "upstream_unreachable", "上游服务不可达")
				return nil, nil, false
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			s.providerCD.mark(cooldownKey, cooldownFor(resp.Header.Get("Retry-After")), time.Now())
			if !last {
				s.log.Warn("native anthropic failover", "provider", p.Name, "status", resp.StatusCode)
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
				s.recordUsage(r.Context(), k, p, model, tokenUsage{}, resp.StatusCode, start)
				continue
			}
		} else if resp.StatusCode < http.StatusBadRequest {
			s.providerCD.clear(cooldownKey, cooldown)
		}
		return resp, p, true
	}
	return nil, nil, false
}

// handleAnthropicNative 转发一次 DeepSeek 原生 Messages 请求：成功响应逐字节转发，
// 错误响应也按上游的 Anthropic 错误壳原样转发；只旁路读 usage 记账。
func (s *Server) handleAnthropicNative(w http.ResponseWriter, r *http.Request, k *Principal, model string, raw []byte, body map[string]any, start time.Time, cands []store.Candidate) {
	stream, _ := body["stream"].(bool)
	resp, p, ok := s.openAnthropicNative(w, r, k, model, body, stream, start, cands, s.anthropicErrorWriter(w))
	if !ok {
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	promptEstimate := estimateTokens(raw)

	if resp.StatusCode >= http.StatusBadRequest {
		// 上游错误按 Anthropic 原生壳原样转发（DeepSeek 的错误体本身就是 Anthropic 形状）。
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		if err != nil {
			s.log.Warn("read native anthropic error body", "err", err)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(data)
		s.recordUsage(r.Context(), k, p, model, tokenUsage{}, resp.StatusCode, start)
		return
	}

	if strings.Contains(strings.ToLower(ct), "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(resp.StatusCode)
		usage, _ := relayAnthropicNativeSSE(w, resp.Body, promptEstimate)
		s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		s.log.Warn("read native anthropic body", "err", err)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(data)
	usage := nativeResponseUsage(data)
	if usage.TotalTokens == 0 {
		// 上游没给 usage：按请求体与响应正文估算，避免这次调用记成 0 让额度形同虚设。
		usage = tokenUsage{
			PromptTokens:     promptEstimate,
			CompletionTokens: estimateAnthropicNativeContent(data),
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		s.log.Warn("native anthropic response without usage, recorded estimate", "provider", p.Name, "model", model,
			"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
	}
	s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
}

// nativeResponseUsage 从原生（非流式）Anthropic message 里取 usage，取不到返回零值。
func nativeResponseUsage(data []byte) tokenUsage {
	var parsed struct {
		Usage *anthropicUsage `json:"usage"`
	}
	if json.Unmarshal(data, &parsed) != nil || parsed.Usage == nil {
		return tokenUsage{}
	}
	return tokenUsage{
		PromptTokens:     parsed.Usage.promptTokens(),
		CompletionTokens: parsed.Usage.OutputTokens,
		TotalTokens:      parsed.Usage.promptTokens() + parsed.Usage.OutputTokens,
	}
}

// estimateAnthropicNativeContent 在没有 usage 时按响应正文的 text/thinking/tool input 估算输出 token。
func estimateAnthropicNativeContent(data []byte) int {
	var parsed struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Thinking string          `json:"thinking"`
			Input    json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return estimateTokens(data)
	}
	n := 0
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			n += estimateTokens([]byte(block.Text))
		case "thinking":
			n += estimateTokens([]byte(block.Thinking))
		case "tool_use", "server_tool_use":
			n += estimateTokens(block.Input)
		}
	}
	if n == 0 {
		n = estimateTokens(data)
	}
	return n
}

// anthropicStreamEvent 只取旁路记账需要的字段，其余字节原样转发。
type anthropicStreamEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
	Delta *struct {
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

// relayAnthropicNativeSSE 逐行转发上游原生 Anthropic SSE（字节不改），同时旁路收集 usage；
// 上游没给 usage 时按请求估算输入、按已转发的 text/thinking/tool 参数估算输出。
func relayAnthropicNativeSSE(w http.ResponseWriter, body io.Reader, promptEstimate int) (tokenUsage, int) {
	flusher, _ := w.(http.Flusher)
	var usage tokenUsage
	streamed := 0
	br := bufio.NewReaderSize(body, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				break // 客户端断开：字节没转发完，但用量仍按已读到的部分记
			}
			trimmed := bytes.TrimSpace(line)
			if payload, ok := bytes.CutPrefix(trimmed, []byte("data:")); ok {
				payload = bytes.TrimSpace(payload)
				if len(payload) > 0 && !bytes.Equal(payload, []byte("[DONE]")) {
					var ev anthropicStreamEvent
					if json.Unmarshal(payload, &ev) == nil {
						if ev.Message != nil && ev.Message.Usage != nil {
							mergeAnthropicUsage(&usage, ev.Message.Usage)
						}
						if ev.Usage != nil {
							mergeAnthropicUsage(&usage, ev.Usage)
						}
						if ev.Delta != nil {
							streamed += estimateTokens([]byte(ev.Delta.Text))
							streamed += estimateTokens([]byte(ev.Delta.Thinking))
							streamed += estimateTokens([]byte(ev.Delta.PartialJSON))
						}
					}
				}
			}
			if len(trimmed) == 0 && flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if flusher != nil {
				flusher.Flush()
			}
			break
		}
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = promptEstimate
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = streamed
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage, streamed
}
