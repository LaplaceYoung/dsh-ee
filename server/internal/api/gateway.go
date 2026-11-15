package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const (
	maxRequestBody  = 16 << 20
	maxResponseBody = 32 << 20
)

// 网关的错误沿用 OpenAI 的格式，客户端 SDK 能直接解析。
func writeOpenAIErr(w http.ResponseWriter, status int, typ, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": msg, "type": typ, "code": code}})
}

func dshConnect(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("X-DshEE-Client"), "dsh-connect/")
}

// writeGatewayError 给普通客户端保留 OpenAI 状态码；接入插件需要能直接展示的 400 中文文案。
func writeGatewayError(w http.ResponseWriter, r *http.Request, status int, typ, code, msg string) {
	if !dshConnect(r) {
		writeOpenAIErr(w, status, typ, code, msg)
		return
	}
	mapped := code
	switch code {
	case "invalid_api_key", tokenRevoked, tokenExpired, tokenInvalid:
		mapped, msg = "dshee_login_required", "企业账号登录已失效，在对话框输入 /ee-login 重新登录"
	case "permission_denied":
		mapped, msg = "dshee_forbidden", "你的账号没有调用大模型的权限，请联系管理员"
	case "model_not_allowed":
		mapped = "dshee_model_denied"
	case "monthly_quota_exceeded":
		if strings.Contains(msg, "还没有") {
			mapped = "dshee_no_allowance"
		} else {
			mapped = "dshee_monthly_limit_reached"
		}
	case "quota_expired":
		mapped, msg = "dshee_allowance_expired", "额度已过期，请联系管理员"
	}
	writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", mapped, msg)
}

func (s *Server) authKey(w http.ResponseWriter, r *http.Request) (*Principal, *store.UserQuota, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeGatewayError(w, r, http.StatusUnauthorized, "invalid_request_error", tokenInvalid, "缺少或无效的 API key")
		return nil, nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		var be bearerError
		if errors.As(err, &be) {
			writeGatewayError(w, r, http.StatusUnauthorized, "invalid_request_error", string(be), "API key 无效、已吊销或已过期")
		} else {
			s.log.Error("gateway auth", "err", err)
			writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		}
		return nil, nil, false
	}
	if !p.Scopes.Has(rbac.ScopeModels) || !p.Access.Has(rbac.LLMUse) {
		writeGatewayError(w, r, http.StatusForbidden, "permission_error", "permission_denied", "你的账号没有调用大模型的权限，请联系管理员")
		return nil, nil, false
	}
	quota := p.quota()
	if quota.QuotaExpiresAt != nil && !quota.QuotaExpiresAt.After(time.Now()) {
		writeGatewayError(w, r, http.StatusForbidden, "permission_error", "quota_expired", "额度已过期，请联系管理员")
		return nil, nil, false
	}
	return p, quota, true
}

// modelAllowed 同时校验账号额度与令牌上的模型限制。
func modelAllowed(p *Principal, quota *store.UserQuota, model string) bool {
	if len(quota.AllowedModels) > 0 && !slices.Contains(quota.AllowedModels, model) {
		return false
	}
	return len(p.Models) == 0 || slices.Contains(p.Models, model)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	k, quota, ok := s.authKey(w, r)
	if !ok {
		return
	}
	ms, err := s.store.PublicModels(r.Context())
	if err != nil {
		s.log.Error("list models", "err", err)
		writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		return
	}
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	data := []model{}
	for _, m := range ms {
		if modelAllowed(k, quota, m.Name) {
			data = append(data, model{ID: m.Name, Object: "model", OwnedBy: m.OwnedBy})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// llmErrWriter 写一条模型网关错误。同一套校验/故障转移逻辑供 OpenAI 与 Anthropic
// 两个端点复用，差别只在错误壳与状态码文案，由各端点传入自己的写法。
type llmErrWriter func(status int, typ, code, msg string)

// upstreamBodyFn 在每条候选线路上生成实际发给上游的请求体，参数是客户端请求体、
// 该线路的上游模型名与 provider.vendor。Chat Completions 用它按 vendor 注入 DeepSeek
// Chatbot 的 safety_check / title（docs/dev/chatbot.md）；nil 时保持「只改写 model、
// 其余字段原样」的默认行为（Anthropic 端点的转换层走这条路径）。
type upstreamBodyFn func(clientBody map[string]any, upstreamModel, vendor string) map[string]any

// openaiErrorWriter 是 OpenAI 兼容网关错误的写法：普通客户端拿 OpenAI 壳，
// DSH 接入插件额外映射成 400 的可展示文案（见 writeGatewayError）。
func (s *Server) openaiErrorWriter(w http.ResponseWriter, r *http.Request) llmErrWriter {
	return func(status int, typ, code, msg string) { writeGatewayError(w, r, status, typ, code, msg) }
}

// openaiUpstreamErrorWriter 是故障转移过程里的上游错误写法：保持 OpenAI 壳，
// 不做 DSH 文案映射（与抽出公共逻辑前的行为一致）。
func (s *Server) openaiUpstreamErrorWriter(w http.ResponseWriter) llmErrWriter {
	return func(status int, typ, code, msg string) { writeOpenAIErr(w, status, typ, code, msg) }
}

// checkLLMQuota 校验账号与令牌的月度额度。生成参数（max_tokens / n）的校验在调用前
// 由各端点自己完成：OpenAI 网关用 completionLimitError，Anthropic 端点在转换请求时校验。
func (s *Server) checkLLMQuota(w http.ResponseWriter, r *http.Request, k *Principal, quota *store.UserQuota, writeErr llmErrWriter) bool {
	if quota.MonthlyTokenLimit != nil {
		used, err := s.store.MonthTokensByUser(r.Context(), k.User.ID)
		if err != nil {
			s.log.Error("month tokens", "user", k.User.ID, "err", err)
			writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
			return false
		}
		if used >= *quota.MonthlyTokenLimit {
			msg := "本月 token 额度已用完，请联系管理员"
			if *quota.MonthlyTokenLimit == 0 {
				msg = "管理员还没有给你分配额度，请联系管理员"
			}
			writeErr(http.StatusTooManyRequests, "insufficient_quota", "monthly_quota_exceeded", msg)
			return false
		}
	}
	if k.Limit != nil {
		used, err := s.store.TokenMonthUsage(r.Context(), k.TokenID)
		if err != nil {
			s.log.Error("token month tokens", "token", k.TokenID, "err", err)
			writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
			return false
		}
		if used >= *k.Limit {
			writeErr(http.StatusTooManyRequests, "insufficient_quota", "monthly_quota_exceeded", "本月 token 额度已用完（此令牌），请联系管理员")
			return false
		}
	}
	return true
}

// loadModelCandidates 读对外模型的候选线路；出错或没有线路时已经按 writeErr 写好响应。
func (s *Server) loadModelCandidates(w http.ResponseWriter, r *http.Request, model string, writeErr llmErrWriter) ([]store.Candidate, bool) {
	cands, err := s.store.ModelCandidates(r.Context(), model)
	if err != nil {
		s.log.Error("model candidates", "err", err)
		writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		return nil, false
	}
	if len(cands) == 0 {
		writeErr(http.StatusNotFound, "invalid_request_error", "model_not_found", "没有可用的上游提供模型 "+model)
		return nil, false
	}
	return cands, true
}

// openChatUpstream 按模型候选列表依次尝试上游，返回可用的上游响应和实际使用的 provider。
// body 会按候选改写 model 字段（传入 bodyFn 时由它构造每条线路的请求体，例如 Chat
// Completions 的 DeepSeek Chatbot 注入）；一旦开始向客户端写响应就不再切换。失败时它已经
// 用 writeErr 写好响应并返回 ok=false，调用方只需关闭 resp.Body 并继续转发/转换。
func (s *Server) openChatUpstream(w http.ResponseWriter, r *http.Request, k *Principal, model string, body map[string]any, stream bool, start time.Time, writeErr llmErrWriter, bodyFn upstreamBodyFn) (*http.Response, *store.Provider, bool) {
	cands, ok := s.loadModelCandidates(w, r, model, writeErr)
	if !ok {
		return nil, nil, false
	}
	return s.openChatUpstreamCandidates(w, r, k, model, body, stream, start, cands, writeErr, bodyFn)
}

// openChatUpstreamCandidates 是 openChatUpstream 的候选线路版本，供已经取过候选列表的调用方
// （Anthropic 端点要先按 vendor 决定原生透传还是降级转换）复用，避免重复查库。
func (s *Server) openChatUpstreamCandidates(w http.ResponseWriter, r *http.Request, k *Principal, model string, body map[string]any, stream bool, start time.Time, cands []store.Candidate, writeErr llmErrWriter, bodyFn upstreamBodyFn) (*http.Response, *store.Provider, bool) {
	// 按优先级逐个尝试：连不上、429、5xx 时换下一条线路；一旦开始给客户端写响应就不再切换。
	var resp *http.Response
	var p *store.Provider
	for i, c := range cands {
		last := i == len(cands)-1
		p = c.Provider
		cooldownKey := providerCooldownKey{providerID: p.ID, model: c.UpstreamModel}
		cooldown := s.providerCD.snapshot(cooldownKey, time.Now())
		// 冷却中的线路直接跳过，免得冷却期内每个请求都先撞一次 429/超时；
		// 全部冷却时仍会尝试最后一条兜底，不会因为冷却而直接失败。
		if !last && cooldown.cooling() {
			continue
		}
		upstreamKey, err := s.box.Open(p.APIKeyEnc)
		if err != nil {
			s.log.Error("decrypt provider key", "provider", p.Name, "err", err)
			if last {
				writeErr(http.StatusInternalServerError, "server_error", "provider_misconfigured", "上游配置有误，请联系管理员")
				return nil, nil, false
			}
			continue
		}
		out, err := json.Marshal(candidateChatBody(body, c.UpstreamModel, p.Vendor, bodyFn))
		if err != nil {
			writeErr(http.StatusBadRequest, "invalid_request_error", "invalid_json", "请求体无法转发")
			return nil, nil, false
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(out))
		if err != nil {
			s.log.Error("build upstream request", "err", err)
			writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
			return nil, nil, false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+string(upstreamKey))
		if stream {
			req.Header.Set("Accept", "text/event-stream")
		}
		resp, err = s.upstream.Do(req)
		if err != nil {
			if r.Context().Err() == nil {
				s.providerCD.mark(providerCooldownKey{providerID: p.ID}, providerCooldownDefault, time.Now())
			}
			s.log.Warn("upstream request failed", "provider", p.Name, "err", err)
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
				s.log.Warn("upstream failover", "provider", p.Name, "status", resp.StatusCode)
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
				s.recordUsage(r.Context(), k, p, model, tokenUsage{}, resp.StatusCode, start)
				continue
			}
		} else if resp.StatusCode < http.StatusBadRequest {
			s.providerCD.clear(cooldownKey, cooldown)
		}
		break
	}
	return resp, p, true
}

// candidateChatBody 返回发往一条候选线路的请求体。bodyFn 为空时保持原行为：只把
// model 换成该线路的上游模型名、其余字段原样；Chat Completions 传入 s.upstreamChatBody，
// 在 vendor=deepseek 的线路上按部署配置补 safety_check / title（客户端已传则保留），
// 并且每次尝试都用独立 map，故障切换不会把注入字段带给其它线路。
func candidateChatBody(body map[string]any, upstreamModel, vendor string, bodyFn upstreamBodyFn) map[string]any {
	if bodyFn == nil {
		body["model"] = upstreamModel
		return body
	}
	return bodyFn(body, upstreamModel, vendor)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	k, quota, ok := s.authKey(w, r)
	if !ok {
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil || len(raw) > maxRequestBody {
		writeOpenAIErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large", "请求体过大")
		return
	}
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // 保留数字原样，转发时不改精度
	if err := dec.Decode(&body); err != nil {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "请求体不是合法 JSON")
		return
	}
	model, _ := body["model"].(string)
	if model == "" {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", "model_required", "缺少 model")
		return
	}
	if !modelAllowed(k, quota, model) {
		writeGatewayError(w, r, http.StatusForbidden, "permission_error", "model_not_allowed", "你的账号不能使用模型 "+model+"，可在 /ee-status 查看可用模型")
		return
	}
	// 生成参数在转发前收口：超额输出、多候选会让单次请求的用量脱离额度约束。
	if code, msg := completionLimitError(body); code != "" {
		writeGatewayError(w, r, http.StatusBadRequest, "invalid_request_error", code, msg)
		return
	}
	if !s.checkLLMQuota(w, r, k, quota, s.openaiErrorWriter(w, r)) {
		return
	}

	// 流式请求要求上游在最后一个分块里带上用量，否则统计不到 token。
	stream, _ := body["stream"].(bool)
	if stream {
		opts, _ := body["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		opts["include_usage"] = true
		body["stream_options"] = opts
	}

	// 复用与 Anthropic 端点相同的候选选择、故障转移与冷却逻辑；
	// Chat Completions 额外按 vendor 注入 DeepSeek Chatbot 选项（docs/dev/chatbot.md）。
	resp, p, ok := s.openChatUpstream(w, r, k, model, body, stream, start, s.openaiUpstreamErrorWriter(w), s.upstreamChatBody)
	if !ok {
		return
	}
	defer resp.Body.Close()

	var usage tokenUsage
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if stream && resp.StatusCode == http.StatusOK {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(resp.StatusCode)
		var streamed int
		usage, streamed = relaySSE(w, resp.Body)
		// 客户端点了停止或网络断开时，上游放在最后一块的 usage 收不到，但上游照样按已生成的计费。
		// 这时按请求和已转发的内容估一个数记账，免得这次调用记成 0、月度额度形同虚设。
		if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
			usage = tokenUsage{PromptTokens: estimateTokens(raw), CompletionTokens: streamed}
			s.log.Warn("stream ended without usage, recorded estimate", "provider", p.Name, "model", model,
				"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
		}
	} else {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		if err != nil {
			s.log.Warn("read upstream body", "err", err)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(data)
		if resp.StatusCode == http.StatusOK {
			var parsed struct {
				Usage *tokenUsage `json:"usage"`
			}
			if json.Unmarshal(data, &parsed) == nil && parsed.Usage != nil {
				usage = *parsed.Usage
			}
		}
	}
	s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
}

// relaySSE 逐行转发上游的 SSE，每个事件结束就 flush，同时从 data 行里捞出 usage。
// 第二个返回值是按已转发的增量内容估出的输出 token 数，上游没给 usage 时用来兜底。
func relaySSE(w http.ResponseWriter, body io.Reader) (tokenUsage, int) {
	var usage tokenUsage
	streamed := 0
	flusher, _ := w.(http.Flusher)
	br := bufio.NewReaderSize(body, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return usage, streamed // 客户端断开
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				if flusher != nil {
					flusher.Flush()
				}
			} else if payload, ok := bytes.CutPrefix(trimmed, []byte("data:")); ok {
				payload = bytes.TrimSpace(payload)
				if !bytes.Equal(payload, []byte("[DONE]")) {
					var chunk streamChunk
					if json.Unmarshal(payload, &chunk) == nil {
						if chunk.Usage != nil {
							usage = *chunk.Usage
						}
						streamed += chunk.deltaTokens()
					}
				}
			}
		}
		if err != nil {
			if flusher != nil {
				flusher.Flush()
			}
			return usage, streamed
		}
	}
}

// streamChunk 只取记账要用的字段：usage，以及增量里会计费的文字（正文、思考、工具参数）。
type streamChunk struct {
	Usage   *tokenUsage `json:"usage"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (c *streamChunk) deltaTokens() int {
	n := 0
	for _, ch := range c.Choices {
		d := ch.Delta
		n += estimateTokens([]byte(d.Content)) + estimateTokens([]byte(d.ReasoningContent))
		for _, tc := range d.ToolCalls {
			n += estimateTokens([]byte(tc.Function.Name)) + estimateTokens([]byte(tc.Function.Arguments))
		}
	}
	return n
}

// estimateTokens 粗估 token 数，只在上游没给 usage 时兜底用：
// ASCII 约 4 个字符一个 token，其余（中日韩等）按一个字符一个 token，宁可略多不少。
func estimateTokens(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	ascii, other := 0, 0
	for _, r := range string(b) {
		if r < 0x80 {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}

// recordUsage 在响应写完后记账；客户端提前断开也要记，所以脱离请求的取消信号。
func (s *Server) recordUsage(ctx context.Context, k *Principal, p *store.Provider, model string, u tokenUsage, status int, start time.Time) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	err := s.store.RecordUsage(ctx, store.Usage{
		APIKeyID:         k.LegacyID,
		UserID:           k.User.ID,
		ProviderID:       p.ID,
		TokenID:          k.TokenID,
		Model:            model,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		StatusCode:       status,
		LatencyMS:        int(time.Since(start).Milliseconds()),
	})
	if err == nil && k.LegacyID > 0 {
		err = s.store.TouchAPIKey(ctx, k.LegacyID)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		s.log.Error("record usage", "key", k.LegacyID, "token", k.TokenID, "err", err)
	}
}
