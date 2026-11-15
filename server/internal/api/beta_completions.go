package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DeepSeek 的 FIM（Fill In the Middle）与 Chat Prefix Completion 还是 beta 能力：
// 上游接口挂在 {base_url}/beta 下，不是所有 OpenAI 兼容厂商都有等价语义。
//
//	POST {base}/beta/completions        FIM：prompt(/suffix) → choices[].text
//	POST {base}/beta/chat/completions   Chat Prefix：最后一条 assistant 带 prefix:true
//
// dsh-ee 只做透传与治理：请求必须落在一个 vendor=deepseek 的已启用上游上
// （DSH_EE_DEEPSEEK_PROVIDER 可按 name/id 指定，默认取优先级最高的那个），
// 不跨厂商做语义翻译，也不跨上游故障转移。鉴权、模型权限、额度、限流与用量记录
// 复用 /v1 网关的同一套实现，错误一律是 OpenAI 错误壳。
const deepSeekVendor = "deepseek"

// betaEndpoint 描述两条 beta 路由的差异：FIM 用 prompt/suffix，Chat Prefix 用 messages。
const (
	betaKindFIM        = "fim"
	betaKindChatPrefix = "chat_prefix"
)

type betaEndpoint struct {
	kind         string // betaKindFIM / betaKindChatPrefix
	upstreamPath string // 拼在 DeepSeek 上游 base_url 后面的路径
}

var (
	betaFIMEndpoint        = betaEndpoint{kind: betaKindFIM, upstreamPath: "/beta/completions"}
	betaChatPrefixEndpoint = betaEndpoint{kind: betaKindChatPrefix, upstreamPath: "/beta/chat/completions"}
)

func (s *Server) handleBetaCompletions(w http.ResponseWriter, r *http.Request) {
	s.serveBeta(w, r, betaFIMEndpoint)
}

func (s *Server) handleBetaChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.serveBeta(w, r, betaChatPrefixEndpoint)
}

// serveBeta 是两条 beta 路由共用的处理链：鉴权 → 请求体 → 模型权限 → beta 形状校验 →
// 额度 → 选上游 → 模型名重写 → 转发 → 用量记账。
func (s *Server) serveBeta(w http.ResponseWriter, r *http.Request, ep betaEndpoint) {
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
	if code, msg := validateBetaRequest(ep, body); code != "" {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", code, msg)
		return
	}
	// 上游共享文件（file_id）与 /v1/chat/completions 同一条访问边界（file_access.go）：
	// Chat Prefix 的 messages 同样会被上游按 file_id 读取，普通员工默认不放行。
	if !s.fileAccessAllowed(k) && chatRequestReferencesFiles(body) {
		writeOpenAIFileAccessDenied(w)
		return
	}
	// max_tokens / n 等生成参数沿用网关的收口规则：超额输出会脱离额度约束。
	if code, msg := completionLimitError(body); code != "" {
		writeGatewayError(w, r, http.StatusBadRequest, "invalid_request_error", code, msg)
		return
	}
	// 额度口径与 /v1 网关共用同一套实现（checkLLMQuota）：两条路径的准入规则不能跑偏。
	if !s.checkLLMQuota(w, r, k, quota, s.openaiErrorWriter(w, r)) {
		return
	}

	// 上游选择：只认一个 vendor=deepseek 的启用上游，不做跨上游故障转移。
	p, unavailable, err := s.betaProvider(r.Context())
	if err != nil {
		s.log.Error("beta provider", "err", err)
		writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		return
	}
	if p == nil {
		writeOpenAIErr(w, http.StatusServiceUnavailable, "server_error", "beta_unavailable", unavailable)
		return
	}
	upstreamModel, err := s.betaUpstreamModel(r.Context(), p.ID, model)
	if err != nil {
		s.log.Error("beta upstream model", "provider", p.Name, "model", model, "err", err)
		writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		return
	}
	if upstreamModel == "" {
		writeGatewayError(w, r, http.StatusNotFound, "invalid_request_error", "model_not_found",
			"模型 "+model+" 没有配置在上游 "+p.Name+" 上；/beta/* 只转发给一个 vendor=deepseek 上游")
		return
	}
	key, err := s.box.Open(p.APIKeyEnc)
	if err != nil {
		s.log.Error("decrypt provider key", "provider", p.Name, "err", err)
		writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "provider_misconfigured", "上游配置有误，请联系管理员")
		return
	}

	stream, _ := body["stream"].(bool)
	// Chat Prefix 走 chat completions 的流式协议：要求上游在最后一个分块里带 usage，
	// 否则统计不到 token。FIM 的文档保证最后一个 content 分块自带 usage，不额外改请求。
	if stream && ep.kind == betaKindChatPrefix {
		opts, _ := body["stream_options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		opts["include_usage"] = true
		body["stream_options"] = opts
	}
	// 对外模型名换成这条上游的上游模型名，其余字段原样转发。
	body["model"] = upstreamModel
	out, err := json.Marshal(body)
	if err != nil {
		writeOpenAIErr(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "请求体无法转发")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, betaUpstreamURL(p.BaseURL, ep.upstreamPath), bytes.NewReader(out))
	if err != nil {
		s.log.Error("build beta upstream request", "err", err)
		writeOpenAIErr(w, http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+string(key))
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := s.upstream.Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			return // 客户端已断开
		}
		s.log.Warn("beta upstream request failed", "provider", p.Name, "err", err)
		s.recordUsage(r.Context(), k, p, model, tokenUsage{}, http.StatusBadGateway, start)
		writeOpenAIErr(w, http.StatusBadGateway, "server_error", "upstream_unreachable", "上游服务不可达")
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
		// 客户端点了停止或上游提前断开时收不到最后一个 usage 分块，但上游照样计费；
		// 按请求体与已转发的 choices[].text 估一个数记账，避免这次调用记成 0。
		if isEmptyUsage(usage) {
			usage = tokenUsage{PromptTokens: estimateTokens(raw), CompletionTokens: streamed}
			s.log.Warn("beta stream ended without usage, recorded estimate", "provider", p.Name, "model", model,
				"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
		}
	} else {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		if err != nil {
			s.log.Warn("read beta upstream body", "err", err)
		}
		if resp.StatusCode == http.StatusOK {
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(data)
			usage = betaUsageFromBody(data)
			if isEmptyUsage(usage) {
				// FIM 正常会带 usage；兼容实现漏掉时按请求与 choices[].text / message.content 估算。
				usage = tokenUsage{PromptTokens: estimateTokens(raw), CompletionTokens: betaTextTokens(data)}
				s.log.Warn("beta response missing usage, recorded estimate", "provider", p.Name, "model", model,
					"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
			}
		} else {
			writeBetaUpstreamError(w, resp.StatusCode, resp.Header.Get("Content-Type"), data)
		}
	}
	s.recordUsage(r.Context(), k, p, model, usage, resp.StatusCode, start)
}

// validateBetaRequest 按路由校验 beta 能力自己的形状：FIM 需要 prompt，Chat Prefix
// 需要最后一条消息是带 prefix:true 的 assistant。返回空 code 表示通过。
func validateBetaRequest(ep betaEndpoint, body map[string]any) (code, msg string) {
	if ep.kind == betaKindChatPrefix {
		return validateBetaChatPrefix(body)
	}
	return validateBetaFIM(body)
}

func validateBetaFIM(body map[string]any) (code, msg string) {
	prompt, exists := body["prompt"]
	if !exists || prompt == nil {
		return "prompt_required", "FIM 补全需要 prompt"
	}
	if _, ok := prompt.(string); !ok {
		return "invalid_prompt", "prompt 必须是字符串"
	}
	if suffix, exists := body["suffix"]; exists && suffix != nil {
		if _, ok := suffix.(string); !ok {
			return "invalid_suffix", "suffix 必须是字符串"
		}
	}
	return "", ""
}

func validateBetaChatPrefix(body map[string]any) (code, msg string) {
	raw, exists := body["messages"]
	if !exists || raw == nil {
		return "messages_required", "Chat Prefix 续写需要 messages"
	}
	msgs, ok := raw.([]any)
	if !ok || len(msgs) == 0 {
		return "messages_invalid", "messages 必须是非空数组"
	}
	last, ok := msgs[len(msgs)-1].(map[string]any)
	if !ok {
		return "messages_invalid", "messages 的每一项必须是对象"
	}
	if role, _ := last["role"].(string); role != "assistant" {
		return "prefix_required", "Chat Prefix 续写的最后一条消息必须是 assistant"
	}
	if prefix, _ := last["prefix"].(bool); !prefix {
		return "prefix_required", `Chat Prefix 续写的最后一条 assistant 消息必须带 "prefix": true`
	}
	return "", ""
}

// betaProvider 选出这次 beta 透传唯一的上游：默认 vendor=deepseek 里优先级最高的启用上游
// （ListProviders 已按 priority、id 排序）；配置了 DSH_EE_DEEPSEEK_PROVIDER 时只认这个
// name 或 id，且它仍必须是 vendor=deepseek。返回 (nil, 原因, nil) 表示当前不可用（503）。
func (s *Server) betaProvider(ctx context.Context) (*store.Provider, string, error) {
	ps, err := s.store.ListProviders(ctx, true)
	if err != nil {
		return nil, "", err
	}
	want := strings.TrimSpace(s.opts.DeepSeekBetaProvider)
	if want == "" {
		for _, p := range ps {
			if p.Vendor == deepSeekVendor {
				return p, "", nil
			}
		}
		return nil, "还没有启用中的 DeepSeek 上游：/beta/* 需要先接入 vendor=deepseek 的上游", nil
	}
	for _, p := range ps {
		if p.Name != want && strconv.FormatInt(p.ID, 10) != want {
			continue
		}
		if p.Vendor != deepSeekVendor {
			return nil, "DSH_EE_DEEPSEEK_PROVIDER=" + want + " 指向的上游不是 vendor=deepseek", nil
		}
		return p, "", nil
	}
	return nil, "DSH_EE_DEEPSEEK_PROVIDER=" + want + " 没有匹配到已启用的上游", nil
}

// betaUpstreamModel 把对外模型名换成指定上游的上游模型名；没配在这条上游上时返回空串。
func (s *Server) betaUpstreamModel(ctx context.Context, providerID int64, model string) (string, error) {
	cands, err := s.store.ModelCandidates(ctx, model)
	if err != nil {
		return "", err
	}
	for _, c := range cands {
		if c.Provider.ID == providerID {
			return c.UpstreamModel, nil
		}
	}
	return "", nil
}

// betaUpstreamURL 拼上游地址。管理员配的 base_url 一般是 API 根（如 https://api.deepseek.com）；
// 如果照着 OpenAI SDK 的 beta base_url 填了 …/beta，也接受，去掉重复的那一段。
func betaUpstreamURL(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return path
	}
	if strings.HasSuffix(base, "/beta") {
		path = strings.TrimPrefix(path, "/beta")
	}
	return base + path
}

// isEmptyUsage 判断上游有没有给出有效用量（三个字段全为 0 视为没给）。
func isEmptyUsage(u tokenUsage) bool {
	return u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0
}

// betaCompletionResponse 只取记账要用的字段：usage，以及回退估算用的正文
// （FIM 在 choices[].text，Chat Prefix 在 choices[].message.content）。
type betaCompletionResponse struct {
	Usage   *tokenUsage `json:"usage"`
	Choices []struct {
		Text    string `json:"text"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// betaUsageFromBody 从非流式响应里取上游 usage；没有时返回零值，交给调用方估算。
func betaUsageFromBody(data []byte) tokenUsage {
	var parsed betaCompletionResponse
	if json.Unmarshal(data, &parsed) != nil || parsed.Usage == nil {
		return tokenUsage{}
	}
	return *parsed.Usage
}

// betaTextTokens 按 choices 里的正文粗估输出 token（非流式、上游没给 usage 时兜底）。
func betaTextTokens(data []byte) int {
	var parsed betaCompletionResponse
	if json.Unmarshal(data, &parsed) != nil {
		return 0
	}
	n := 0
	for _, ch := range parsed.Choices {
		n += estimateTokens([]byte(ch.Text)) + estimateTokens([]byte(ch.Message.Content))
	}
	return n
}

// writeBetaUpstreamError 按 OpenAI 错误壳回传上游错误：上游本来就是 JSON 错误对象时原样转发；
// 缺 Content-Type 或不是 JSON（反向代理的 HTML 错误页等）时包一层，客户端 SDK 才能解析。
func writeBetaUpstreamError(w http.ResponseWriter, status int, contentType string, data []byte) {
	mt, _, _ := mime.ParseMediaType(contentType)
	if mt == "application/json" && json.Valid(data) && bytes.Contains(data, []byte(`"error"`)) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return
	}
	msg := snippet(data)
	if msg == "" {
		msg = "上游返回了无法解析的错误"
	}
	writeOpenAIErr(w, status, "upstream_error", "upstream_error", msg)
}
