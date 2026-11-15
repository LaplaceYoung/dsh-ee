package api

// Anthropic Messages API 兼容端点：POST /v1/messages（v0）。
//
// 目标：让 Claude Code / Anthropic SDK 用 dsh-ee 的企业令牌与额度直接调模型。
// 请求体转成 OpenAI chat completions 交给现有网关（鉴权、模型权限、额度、上游故障转移、
// 用量记账全部复用），响应再转回 Anthropic 的 message / SSE 事件格式。
//
// 明确不做：count_tokens / batches / files / 完整 beta 头；服务端工具（web_search 等）不执行。
// 文档：docs/dev/anthropic-api.md。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// writeAnthropicErr 写 Anthropic 形状的错误：
// {"type":"error","error":{"type","message"}}。typ 为空时按 HTTP 状态码推导。
func writeAnthropicErr(w http.ResponseWriter, status int, typ, msg string) {
	if typ == "" {
		typ = anthropicErrorType(status)
	}
	writeJSON(w, status, map[string]any{
		"type":  "error",
		"error": map[string]string{"type": typ, "message": msg},
	})
}

// anthropicErrorType 按 HTTP 状态码给出 Anthropic 的错误类型。
func anthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "invalid_request_error"
	}
}

// anthropicErrorWriter 把网关公共校验/故障转移产生的错误写成 Anthropic 错误壳。
// 第二个参数（OpenAI code）只用来记日志，类型由状态码决定。
func (s *Server) anthropicErrorWriter(w http.ResponseWriter) llmErrWriter {
	return func(status int, typ, code, msg string) {
		writeAnthropicErr(w, status, "", msg)
	}
}

// authAnthropicKey 与 authKey 等价，但接受 x-api-key 或 Authorization: Bearer，
// 并且错误按 Anthropic 形状返回（401 authentication_error / 403 permission_error）。
func (s *Server) authAnthropicKey(w http.ResponseWriter, r *http.Request) (*Principal, *store.UserQuota, bool) {
	token := strings.TrimSpace(r.Header.Get("X-Api-Key"))
	if token == "" {
		if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			token = strings.TrimSpace(t)
		}
	}
	if token == "" {
		writeAnthropicErr(w, http.StatusUnauthorized, "", "缺少或无效的 API key：请用 x-api-key 或 Authorization: Bearer 传入")
		return nil, nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		var be bearerError
		if errors.As(err, &be) {
			writeAnthropicErr(w, http.StatusUnauthorized, "", "API key 无效、已吊销或已过期（"+string(be)+"）")
		} else {
			s.log.Error("anthropic gateway auth", "err", err)
			writeAnthropicErr(w, http.StatusInternalServerError, "", "服务器内部错误")
		}
		return nil, nil, false
	}
	if !p.Scopes.Has(rbac.ScopeModels) || !p.Access.Has(rbac.LLMUse) {
		writeAnthropicErr(w, http.StatusForbidden, "permission_error", "你的账号没有调用大模型的权限，请联系管理员")
		return nil, nil, false
	}
	quota := p.quota()
	if quota.QuotaExpiresAt != nil && !quota.QuotaExpiresAt.After(time.Now()) {
		writeAnthropicErr(w, http.StatusForbidden, "permission_error", "额度已过期，请联系管理员")
		return nil, nil, false
	}
	return p, quota, true
}

// upstreamErrorMessage 从上游的 OpenAI 错误体里取 message，取不到就返回空串。
func upstreamErrorMessage(data []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &parsed) == nil {
		return strings.TrimSpace(parsed.Error.Message)
	}
	return ""
}

// handleAnthropicMessages 是 POST /v1/messages 的处理函数。
func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	k, quota, ok := s.authAnthropicKey(w, r)
	if !ok {
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil || len(raw) > maxRequestBody {
		writeAnthropicErr(w, http.StatusRequestEntityTooLarge, "", "请求体过大")
		return
	}
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // 保留数字原样；max_tokens 等由转换层统一校验
	if err := dec.Decode(&body); err != nil {
		writeAnthropicErr(w, http.StatusBadRequest, "", "请求体不是合法 JSON")
		return
	}
	model := jsonStr(body["model"])
	if model == "" {
		writeAnthropicErr(w, http.StatusBadRequest, "", "缺少 model")
		return
	}
	if !modelAllowed(k, quota, model) {
		writeAnthropicErr(w, http.StatusForbidden, "permission_error", "你的账号不能使用模型 "+model+"，可在 /v1/models 查看可用模型")
		return
	}

	// 先取候选线路：第一条（最高优先级）线路的 vendor 决定本次请求走 DeepSeek 原生透传，
	// 还是降级到 OpenAI chat 的转换层。故障转移只在同协议内进行（不跨协议），见 anthropic_native.go。
	cands, ok := s.loadModelCandidates(w, r, model, s.anthropicErrorWriter(w))
	if !ok {
		return
	}
	nativeCands, convertedCands := splitAnthropicCandidates(cands)
	if anthropicNativeVendorAllowed(cands[0].Provider) {
		// 原生路径：不解析/不改写请求内容块，只改 model；响应逐字节转发 + usage 旁路。
		if !s.checkLLMQuota(w, r, k, quota, s.anthropicErrorWriter(w)) {
			return
		}
		s.handleAnthropicNative(w, r, k, model, raw, body, start, nativeCands)
		return
	}

	chatBody, err := anthropicToChat(body)
	if err != nil {
		var br anthropicBadRequest
		if errors.As(err, &br) {
			writeAnthropicErr(w, http.StatusBadRequest, "", br.msg)
		} else {
			s.log.Error("convert anthropic request", "err", err)
			writeAnthropicErr(w, http.StatusInternalServerError, "", "服务器内部错误")
		}
		return
	}
	if !s.checkLLMQuota(w, r, k, quota, s.anthropicErrorWriter(w)) {
		return
	}

	stream, _ := chatBody["stream"].(bool)
	if stream {
		// 与 OpenAI 网关一致：流式要求上游在最后一块里带 usage，否则统计不到 token。
		chatBody["stream_options"] = map[string]any{"include_usage": true}
	}
	// Anthropic 转换层不注入 Chatbot 选项：DSH_EE_SAFETY_CHECK / DSH_EE_CHAT_TITLE 只作用于
	// Chat Completions 端点，这里传 nil 保持请求体原样。
	resp, p, ok := s.openChatUpstreamCandidates(w, r, k, model, chatBody, stream, start, convertedCands, s.anthropicErrorWriter(w), nil)
	if !ok {
		return
	}
	defer resp.Body.Close()

	if stream && resp.StatusCode == http.StatusOK {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		promptEstimate := estimateTokens(raw)
		usage, streamed := relayAnthropicSSE(w, resp.Body, model, promptEstimate)
		if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
			usage = tokenUsage{PromptTokens: promptEstimate, CompletionTokens: streamed}
			s.log.Warn("anthropic stream ended without usage, recorded estimate", "provider", p.Name, "model", model,
				"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
		}
		s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		s.log.Warn("read upstream body", "err", err)
	}
	if resp.StatusCode != http.StatusOK {
		// 上游错误统一换成 Anthropic 错误壳；状态码与上游一致（429/5xx 由故障转移先处理）。
		msg := upstreamErrorMessage(data)
		if msg == "" {
			msg = "上游模型服务返回错误"
		}
		writeAnthropicErr(w, resp.StatusCode, "", msg)
		s.recordUsage(r.Context(), k, p, model, tokenUsage{}, resp.StatusCode, start)
		return
	}
	message, usage, err := chatToAnthropicMessage(data, model)
	if err != nil {
		s.log.Warn("convert anthropic response", "err", err, "provider", p.Name)
		writeAnthropicErr(w, http.StatusBadGateway, "", "上游返回无法解析的响应")
		s.recordUsage(r.Context(), k, p, model, tokenUsage{}, http.StatusBadGateway, start)
		return
	}
	writeJSON(w, http.StatusOK, message)
	s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
}

// handleAnthropicCountTokens 明确告知 count_tokens 不在兼容范围（v0）。
// 不实现，但也不要让 Anthropic 客户端拿到 /v1/ 兜底的 OpenAI 错误壳。
func (s *Server) handleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.authAnthropicKey(w, r); !ok {
		return
	}
	writeAnthropicErr(w, http.StatusNotFound, "not_found_error", "count_tokens 暂不支持：dsh-ee 目前只兼容 POST /v1/messages")
}

// ---- 流式：OpenAI SSE → Anthropic SSE ----

// anthropicSSE 把 OpenAI 的流式增量写成 Anthropic 的事件序列：
// message_start → (content_block_start/delta/stop)* → message_delta → message_stop。
// 思考、正文、工具参数分别对应 thinking / text / tool_use 三类内容块。
type anthropicSSE struct {
	w       http.ResponseWriter
	flusher http.Flusher
	model   string
	// 请求体估算的输入 token，上游最后一块没带 usage 时兜底。
	promptEstimate int

	nextIndex int
	// 当前未结束的内容块；openKind 为空表示没有打开的块。
	openIndex   int
	openKind    string // thinking / text / tool
	openToolKey int    // 当前打开的工具块对应的 OpenAI tool_calls[].index

	thinkingText strings.Builder
	toolSeen     bool
	streamed     int // 已转发增量的 token 估算
	usage        tokenUsage
	stopReason   string
}

func newAnthropicSSE(w http.ResponseWriter, model string, promptEstimate int) *anthropicSSE {
	flusher, _ := w.(http.Flusher)
	return &anthropicSSE{
		w: w, flusher: flusher, model: model, promptEstimate: promptEstimate,
		openIndex: -1, openToolKey: -1,
	}
}

func (a *anthropicSSE) writeEvent(name string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return err // 客户端断开
	}
	if a.flusher != nil {
		a.flusher.Flush()
	}
	return nil
}

func (a *anthropicSSE) startBlock(kind string, block map[string]any) error {
	index := a.nextIndex
	a.nextIndex++
	a.openIndex = index
	a.openKind = kind
	return a.writeEvent("content_block_start", map[string]any{
		"type": "content_block_start", "index": index, "content_block": block,
	})
}

func (a *anthropicSSE) blockDelta(index int, delta map[string]any) error {
	return a.writeEvent("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index, "delta": delta,
	})
}

func (a *anthropicSSE) stopOpen() error {
	if a.openIndex < 0 {
		return nil
	}
	err := a.writeEvent("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": a.openIndex,
	})
	a.openIndex = -1
	a.openKind = ""
	a.openToolKey = -1
	return err
}

// closeThinking 在思考块结束时补一个 signature_delta（Anthropic 官方会发），再关块。
func (a *anthropicSSE) closeThinking() error {
	if a.openKind != "thinking" {
		return a.stopOpen()
	}
	if err := a.blockDelta(a.openIndex, map[string]any{
		"type": "signature_delta", "signature": anthropicThinkingSignature(a.thinkingText.String()),
	}); err != nil {
		return err
	}
	return a.stopOpen()
}

// consume 处理一个 OpenAI 流式分块。返回值非 nil 表示客户端已断开，调用方停止转发。
func (a *anthropicSSE) consume(chunk *openAIStreamChunk) error {
	for _, choice := range chunk.Choices {
		delta := choice.Delta
		reasoning := delta.ReasoningContent
		if reasoning == "" {
			reasoning = delta.Reasoning
		}
		if reasoning != "" {
			if a.openKind != "thinking" {
				if err := a.stopOpen(); err != nil {
					return err
				}
				if err := a.startBlock("thinking", map[string]any{
					"type": "thinking", "thinking": "", "signature": "",
				}); err != nil {
					return err
				}
			}
			a.thinkingText.WriteString(reasoning)
			a.streamed += estimateTokens([]byte(reasoning))
			if err := a.blockDelta(a.openIndex, map[string]any{"type": "thinking_delta", "thinking": reasoning}); err != nil {
				return err
			}
		}
		if delta.Content != "" {
			if a.openKind == "thinking" {
				if err := a.closeThinking(); err != nil {
					return err
				}
			} else if a.openKind != "text" {
				if err := a.stopOpen(); err != nil {
					return err
				}
			}
			if a.openKind != "text" {
				if err := a.startBlock("text", map[string]any{"type": "text", "text": ""}); err != nil {
					return err
				}
			}
			a.streamed += estimateTokens([]byte(delta.Content))
			if err := a.blockDelta(a.openIndex, map[string]any{"type": "text_delta", "text": delta.Content}); err != nil {
				return err
			}
		}
		for _, tc := range delta.ToolCalls {
			// OpenAI 兼容上游的并行工具调用按 index 顺序出增量；换 index 就关掉上一个工具块。
			if a.openKind != "tool" || a.openToolKey != tc.Index {
				if a.openKind == "thinking" {
					if err := a.closeThinking(); err != nil {
						return err
					}
				} else if err := a.stopOpen(); err != nil {
					return err
				}
				if err := a.startBlock("tool", map[string]any{
					"type": "tool_use", "id": anthropicToolUseID(tc.ID), "name": tc.Function.Name, "input": map[string]any{},
				}); err != nil {
					return err
				}
				a.openToolKey = tc.Index
				a.toolSeen = true
				a.streamed += estimateTokens([]byte(tc.Function.Name))
			}
			if tc.Function.Arguments != "" {
				a.streamed += estimateTokens([]byte(tc.Function.Arguments))
				if err := a.blockDelta(a.openIndex, map[string]any{
					"type": "input_json_delta", "partial_json": tc.Function.Arguments,
				}); err != nil {
					return err
				}
			}
		}
		if choice.FinishReason != "" {
			a.stopReason = choice.FinishReason
		}
	}
	if chunk.Usage != nil {
		a.usage = tokenUsage{
			PromptTokens:     chunk.Usage.PromptTokens,
			CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens:      chunk.Usage.TotalTokens,
		}
	}
	return nil
}

// resultUsage 汇总本次流式调用的记账用量：优先上游给的 usage，没有就按请求和已转发增量估算。
func (a *anthropicSSE) resultUsage() tokenUsage {
	usage := a.usage
	if usage.PromptTokens == 0 {
		usage.PromptTokens = a.promptEstimate
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = a.streamed
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

// finishEvents 补上 message_delta 与 message_stop。
func (a *anthropicSSE) finishEvents() error {
	if a.openKind == "thinking" {
		if err := a.closeThinking(); err != nil {
			return err
		}
	} else if err := a.stopOpen(); err != nil {
		return err
	}
	usage := a.resultUsage()
	stopReason := anthropicStopReason(a.stopReason, a.toolSeen)
	if err := a.writeEvent("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{
			"input_tokens":                usage.PromptTokens,
			"output_tokens":               usage.CompletionTokens,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
		},
	}); err != nil {
		return err
	}
	return a.writeEvent("message_stop", map[string]any{"type": "message_stop"})
}

// relayAnthropicSSE 逐行读取上游 OpenAI SSE 并写出 Anthropic SSE，同时收集 token 用量。
// 第二个返回值是按已转发增量估出的输出 token，上游没给 usage 时记账兜底用。
func relayAnthropicSSE(w http.ResponseWriter, body io.Reader, model string, promptEstimate int) (tokenUsage, int) {
	stream := newAnthropicSSE(w, model, promptEstimate)
	if err := stream.writeEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            newAnthropicID("msg_"),
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": promptEstimate, "output_tokens": 0},
		},
	}); err != nil {
		return tokenUsage{}, 0
	}

	br := bufio.NewReaderSize(body, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if payload, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:")); ok {
				payload = bytes.TrimSpace(payload)
				if len(payload) > 0 && !bytes.Equal(payload, []byte("[DONE]")) {
					var chunk openAIStreamChunk
					if json.Unmarshal(payload, &chunk) == nil {
						if werr := stream.consume(&chunk); werr != nil {
							return stream.resultUsage(), stream.streamed // 客户端断开：不再写事件
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	_ = stream.finishEvents() // 写失败说明客户端已断开，用量仍按已转发内容记账
	return stream.resultUsage(), stream.streamed
}
