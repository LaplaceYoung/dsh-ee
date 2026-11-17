package api

// OpenAI Responses API 兼容层（v0）：POST /v1/responses。
//
// 目的：让按 Responses API 编写的客户端（Codex、OpenAI Agents SDK、DeepSeek
// Responses 兼容客户端等）可以用同一个企业令牌接 dsh-ee。请求在这里转换成
// OpenAI chat completions 调上游，响应再转回 Responses 形状；鉴权、模型权限、
// 账号/令牌月度额度、用量记账与上游故障转移与 /v1/chat/completions 完全一致
// （同一套 store 数据与规则）。
//
// v0 边界（完整说明见 docs/dev/responses-api.md）：
//   - 无状态：不保存 response / conversation；previous_response_id、conversation、
//     background、store 等字段被忽略，响应里 store 固定 false、previous_response_id
//     固定 null，多轮对话由客户端把完整历史放进 input（与 DeepSeek Responses API
//     的兼容策略一致）。
//   - input 支持字符串，或 message / function_call / function_call_output / reasoning
//     item；未知 item 忽略；input_image / input_file 等非文本内容 v0 不支持（400）。
//   - tools 只支持 function；其它工具类型忽略。text.format 支持 text / json_object /
//     json_schema，映射到上游 response_format。
//   - stream:true 时先拿到完整上游结果，再按 Responses 语义回放 SSE 事件
//     （response.created / response.in_progress / output_item.added / ... /
//     response.completed）；不是逐 token 转发，首字节延迟等于整段生成时间。
//   - 不做 conversations / files / batches / background。
//   - provider.vendor=deepseek 的候选走**原生透传**：请求 JSON 只改 model 后直接发到
//     <provider.base>/responses，响应与 SSE 逐字节转发（保留 web_search 等服务端工具、
//     citations/annotations、refusal、title、image/file 内容块）；dsh-ee 只做鉴权、
//     模型权限、额度、限流、用量旁路记账与同厂商故障转移。其它 vendor 仍走 chat 转换层。
//
// 复用关系：#156 合入后 gateway.go 已抽出 checkLLMQuota / loadModelCandidates /
// openChatUpstreamCandidates 与 llmErrWriter；转换层直接复用这些公共 helper（与
// chat、Anthropic 端点同一套冷却/故障转移/失败记账）。只有 Responses 原生透传因为
// 要换上游路径（<base>/responses）与 Accept 头，保留一份与 anthropic_native.go
// 结构相同的候选循环，失败与记账语义与公共 helper 一致。

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// isResponsesAPIPath 是 Responses API 兼容面的路径：主入口 /v1/responses，以及
// DeepSeek Responses API 习惯的 /responses 别名（base_url 填 dsh-ee 根地址时，客户端
// 会在后面拼 /responses）。两个路径走同一个 handler；限流、版本响应头、CORS 与错误
// 兜底也按这个判断把别名当同一个协议面处理。
func isResponsesAPIPath(path string) bool {
	return path == "/v1/responses" || path == "/responses"
}

// responsesErrorDetail 是 Responses 错误体里的 error 对象。code 沿用网关的稳定
// 错误码（token_invalid、model_not_allowed、monthly_quota_exceeded 等），客户端
// 可以按 code 判断是登录失效、权限还是额度问题。
type responsesErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    any    `json:"code"`
}

// writeResponsesErr 写 OpenAI Responses 形状的错误：{"error": {message,type,param,code}}。
// 与 chat completions 的 OpenAI 错误壳相比多一个 param 字段（Responses / OpenAI
// SDK 会读它，缺失时部分客户端会报解析错误）。
func writeResponsesErr(w http.ResponseWriter, status int, typ, code, param, msg string) {
	detail := responsesErrorDetail{Message: msg, Type: typ}
	if code != "" {
		detail.Code = code
	}
	if param != "" {
		detail.Param = param
	}
	writeJSON(w, status, map[string]any{"error": detail})
}

// responsesBadRequest 是请求参数校验失败的错误；code 是给客户端看的稳定错误码，
// param 指向出错的字段（OpenAI SDK 会展示 param）。
type responsesParamError struct {
	code  string
	param string
	msg   string
}

func responsesParamErr(code, param, msg string) *responsesParamError {
	return &responsesParamError{code: code, param: param, msg: msg}
}

func (e *responsesParamError) write(w http.ResponseWriter) {
	writeResponsesErr(w, http.StatusBadRequest, "invalid_request_error", e.code, e.param, e.msg)
}

// ---- 鉴权、权限与额度 ----

// responsesAuth 与 gateway.go 的 authKey 等价，区别只有错误壳（Responses 形状）。
// 复制的原因见文件头：gateway.go 正在被 #156 重构，先保持两边独立。
func (s *Server) responsesAuth(w http.ResponseWriter, r *http.Request) (*Principal, *store.UserQuota, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeResponsesErr(w, http.StatusUnauthorized, "authentication_error", tokenInvalid, "", "缺少或无效的 API key")
		return nil, nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		var be bearerError
		if errors.As(err, &be) {
			writeResponsesErr(w, http.StatusUnauthorized, "authentication_error", string(be), "", "API key 无效、已吊销或已过期")
		} else {
			s.log.Error("responses auth", "err", err)
			writeResponsesErr(w, http.StatusInternalServerError, "server_error", "internal_error", "", "服务器内部错误")
		}
		return nil, nil, false
	}
	if !p.Scopes.Has(rbac.ScopeModels) || !p.Access.Has(rbac.LLMUse) {
		writeResponsesErr(w, http.StatusForbidden, "permission_error", "permission_denied", "", "你的账号没有调用大模型的权限，请联系管理员")
		return nil, nil, false
	}
	quota := p.quota()
	if quota.QuotaExpiresAt != nil && !quota.QuotaExpiresAt.After(time.Now()) {
		writeResponsesErr(w, http.StatusForbidden, "permission_error", "quota_expired", "", "额度已过期，请联系管理员")
		return nil, nil, false
	}
	return p, quota, true
}

// responsesErrorWriter 把复用的公共 helper（额度校验、候选加载、故障转移）产生的
// 错误写成 Responses 错误壳：llmErrWriter 只有 status/type/code/message 四个字段，
// param 留空（JSON null），客户端仍可解析。
func (s *Server) responsesErrorWriter(w http.ResponseWriter) llmErrWriter {
	return func(status int, typ, code, msg string) {
		writeResponsesErr(w, status, typ, code, "", msg)
	}
}

// ---- 请求解析与转换 ----

// responsesRequestMeta 是转换后还要回显到 response 对象里的请求参数。
type responsesRequestMeta struct {
	instructions      *string
	maxOutputTokens   *int64
	reasoningEffort   *string
	textFormat        map[string]any
	tools             []any
	toolChoice        any
	parallelToolCalls *bool
	temperature       *float64
	topP              *float64
	user              *string
}

// responsesOptionalString 读取可选的字符串字段；缺失或 null 时返回 nil。
func responsesOptionalString(body map[string]any, field, code, param string) (*string, *responsesParamError) {
	raw, ok := body[field]
	if !ok || raw == nil {
		return nil, nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil, responsesParamErr(code, param, field+" 必须是字符串")
	}
	return &s, nil
}

// responsesBuildChatRequest 校验 Responses 请求并转换成上游 chat completions 请求体。
// 返回的 chatBody 里 model 还是 dsh-ee 的对外模型名，真正转发前会按候选上游改写。
func responsesBuildChatRequest(body map[string]any, model string) (map[string]any, responsesRequestMeta, *responsesParamError) {
	var meta responsesRequestMeta

	instructions, perr := responsesOptionalString(body, "instructions", "invalid_instructions", "instructions")
	if perr != nil {
		return nil, meta, perr
	}
	meta.instructions = instructions

	messages, perr := responsesInputMessages(body["input"], instructions)
	if perr != nil {
		return nil, meta, perr
	}

	chat := map[string]any{"model": model, "messages": messages}

	// max_output_tokens → 上游 max_tokens；单次上限沿用网关的 maxTokensPerRequest。
	if v, present, ok := intParam(body, "max_output_tokens"); present {
		if !ok || v < 1 {
			return nil, meta, responsesParamErr("invalid_max_output_tokens", "max_output_tokens", "max_output_tokens 必须是正整数")
		}
		if v > maxTokensPerRequest {
			return nil, meta, responsesParamErr("max_output_tokens_too_large", "max_output_tokens",
				fmt.Sprintf("max_output_tokens 超过单次上限 %d，请调小", maxTokensPerRequest))
		}
		meta.maxOutputTokens = &v
		chat["max_tokens"] = json.Number(strconv.FormatInt(v, 10))
	}

	// reasoning.effort → 上游 reasoning_effort；reasoning.summary 等字段 v0 忽略。
	if raw, ok := body["reasoning"]; ok && raw != nil {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, meta, responsesParamErr("invalid_reasoning", "reasoning", "reasoning 必须是对象")
		}
		if effortRaw, exists := obj["effort"]; exists && effortRaw != nil {
			effort, ok := effortRaw.(string)
			if !ok || strings.TrimSpace(effort) == "" {
				return nil, meta, responsesParamErr("invalid_reasoning", "reasoning.effort", "reasoning.effort 必须是非空字符串")
			}
			effort = strings.TrimSpace(effort)
			meta.reasoningEffort = &effort
			chat["reasoning_effort"] = effort
		}
	}

	// text.format → 上游 response_format；text.verbosity 等字段 v0 忽略。
	textFormat := map[string]any{"type": "text"}
	var responseFormat any
	if raw, ok := body["text"]; ok && raw != nil {
		text, ok := raw.(map[string]any)
		if !ok {
			return nil, meta, responsesParamErr("invalid_text_format", "text", "text 必须是对象")
		}
		if formatRaw, exists := text["format"]; exists && formatRaw != nil {
			format, ok := formatRaw.(map[string]any)
			if !ok {
				return nil, meta, responsesParamErr("invalid_text_format", "text.format", "text.format 必须是对象")
			}
			formatType, _ := format["type"].(string)
			switch formatType {
			case "text":
				// 默认纯文本，无需转发
			case "json_object":
				textFormat = map[string]any{"type": "json_object"}
				responseFormat = map[string]any{"type": "json_object"}
			case "json_schema":
				name, _ := format["name"].(string)
				name = strings.TrimSpace(name)
				if name == "" {
					return nil, meta, responsesParamErr("invalid_text_format", "text.format.name", "text.format 为 json_schema 时需要 name")
				}
				schema, hasSchema := format["schema"]
				if !hasSchema || schema == nil {
					return nil, meta, responsesParamErr("invalid_text_format", "text.format.schema", "text.format 为 json_schema 时需要 schema")
				}
				echo := map[string]any{"type": "json_schema", "name": name, "schema": schema}
				js := map[string]any{"name": name, "schema": schema}
				if strict, ok := format["strict"].(bool); ok {
					echo["strict"] = strict
					js["strict"] = strict
				}
				textFormat = echo
				responseFormat = map[string]any{"type": "json_schema", "json_schema": js}
			default:
				return nil, meta, responsesParamErr("invalid_text_format", "text.format.type",
					"text.format.type 只支持 text / json_object / json_schema")
			}
		}
	}
	meta.textFormat = textFormat
	if responseFormat != nil {
		chat["response_format"] = responseFormat
	}

	// tools：Responses 的 function 工具字段是平铺的（name 在顶层），
	// 上游 chat completions 要嵌到 function 对象里；其它工具类型 v0 忽略。
	if raw, ok := body["tools"]; ok && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, meta, responsesParamErr("invalid_tools", "tools", "tools 必须是数组")
		}
		chatTools := []any{}
		echoTools := []any{}
		seen := map[string]bool{}
		for i, rawTool := range list {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				return nil, meta, responsesParamErr("invalid_tools", fmt.Sprintf("tools[%d]", i), "每个 tool 必须是对象")
			}
			toolType, _ := tool["type"].(string)
			if toolType != "function" {
				// web_search / custom（DeepSeek 扩展、Codex 的 apply_patch）等不在 v0 范围，
				// 忽略而不是报错，跟 DeepSeek Responses API 的兼容策略一致。
				continue
			}
			name, _ := tool["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" || len(name) > 128 {
				return nil, meta, responsesParamErr("invalid_tools", fmt.Sprintf("tools[%d].name", i),
					"function 工具需要 name（1 到 128 个字符）")
			}
			if seen[name] {
				return nil, meta, responsesParamErr("invalid_tools", fmt.Sprintf("tools[%d].name", i), "function 工具名重复："+name)
			}
			seen[name] = true

			description, _ := tool["description"].(string)
			fn := map[string]any{"name": name}
			echoTool := map[string]any{"type": "function", "name": name}
			if description != "" {
				fn["description"] = description
				echoTool["description"] = description
			}
			if parameters, exists := tool["parameters"]; exists && parameters != nil {
				fn["parameters"] = parameters
				echoTool["parameters"] = parameters
			}
			if strict, ok := tool["strict"].(bool); ok {
				fn["strict"] = strict
				echoTool["strict"] = strict
			}
			chatTools = append(chatTools, map[string]any{"type": "function", "function": fn})
			echoTools = append(echoTools, echoTool)
		}
		if len(chatTools) > 0 {
			chat["tools"] = chatTools
			meta.tools = echoTools
		}
	}

	if raw, ok := body["tool_choice"]; ok && raw != nil {
		switch v := raw.(type) {
		case string:
			if v != "auto" && v != "none" && v != "required" {
				return nil, meta, responsesParamErr("invalid_tool_choice", "tool_choice", "tool_choice 只支持 auto / none / required 或指定 function")
			}
			meta.toolChoice = v
			chat["tool_choice"] = v
		case map[string]any:
			toolType, _ := v["type"].(string)
			name, _ := v["name"].(string)
			name = strings.TrimSpace(name)
			if toolType != "function" || name == "" {
				return nil, meta, responsesParamErr("invalid_tool_choice", "tool_choice",
					"指定工具时必须形如 {\"type\":\"function\",\"name\":\"...\"}")
			}
			echo := map[string]any{"type": "function", "name": name}
			meta.toolChoice = echo
			chat["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": name}}
		default:
			return nil, meta, responsesParamErr("invalid_tool_choice", "tool_choice", "tool_choice 类型不支持")
		}
	}

	if raw, ok := body["parallel_tool_calls"]; ok && raw != nil {
		b, ok := raw.(bool)
		if !ok {
			return nil, meta, responsesParamErr("invalid_parallel_tool_calls", "parallel_tool_calls", "parallel_tool_calls 必须是布尔值")
		}
		meta.parallelToolCalls = &b
		chat["parallel_tool_calls"] = b
	}

	for _, field := range []string{"temperature", "top_p"} {
		raw, ok := body[field]
		if !ok || raw == nil {
			continue
		}
		num, ok := raw.(json.Number)
		if !ok {
			return nil, meta, responsesParamErr("invalid_"+field, field, field+" 必须是数字")
		}
		f, err := num.Float64()
		if err != nil {
			return nil, meta, responsesParamErr("invalid_"+field, field, field+" 必须是数字")
		}
		switch field {
		case "temperature":
			if f < 0 || f > 2 {
				return nil, meta, responsesParamErr("invalid_temperature", "temperature", "temperature 需要在 0 到 2 之间")
			}
			meta.temperature = &f
		case "top_p":
			if f < 0 || f > 1 {
				return nil, meta, responsesParamErr("invalid_top_p", "top_p", "top_p 需要在 0 到 1 之间")
			}
			meta.topP = &f
		}
		// 保留 json.Number 原样转发，避免浮点数转来转去改精度。
		chat[field] = num
	}

	if user, perr := responsesOptionalString(body, "user", "invalid_user", "user"); perr != nil {
		return nil, meta, perr
	} else if user != nil && *user != "" {
		meta.user = user
		chat["user"] = *user
	}

	return chat, meta, nil
}

// responsesInputMessages 把 Responses 的 input（字符串或 item 数组）转成 chat
// completions 的 messages 数组。语义参考 DeepSeek Responses API 的兼容说明：
// function_call 合并进相邻 assistant 消息，function_call_output 变成 tool 消息；
// reasoning / 未知 item 忽略，未知的文本外内容块（图片、文件）返回 400。
func responsesInputMessages(input any, instructions *string) ([]map[string]any, *responsesParamError) {
	var messages []map[string]any
	switch v := input.(type) {
	case nil:
	case string:
		if strings.TrimSpace(v) != "" {
			messages = append(messages, map[string]any{"role": "user", "content": v})
		}
	case []any:
		for i, rawItem := range v {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return nil, responsesParamErr("invalid_input", "input", fmt.Sprintf("input[%d] 必须是对象", i))
			}
			itemType, _ := item["type"].(string)
			switch itemType {
			case "", "message":
				message, perr := responsesInputMessage(item, i)
				if perr != nil {
					return nil, perr
				}
				messages = append(messages, message)
			case "function_call":
				name, _ := item["name"].(string)
				name = strings.TrimSpace(name)
				if name == "" {
					return nil, responsesParamErr("invalid_input", fmt.Sprintf("input[%d].name", i), "function_call 需要 name")
				}
				callID, _ := item["call_id"].(string)
				callID = strings.TrimSpace(callID)
				if callID == "" {
					callID = responsesNewID("call")
				}
				toolCall := map[string]any{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": responsesRawArguments(item["arguments"]),
					},
				}
				messages = responsesAppendToolCall(messages, toolCall)
			case "function_call_output":
				callID, _ := item["call_id"].(string)
				callID = strings.TrimSpace(callID)
				if callID == "" {
					return nil, responsesParamErr("invalid_input", fmt.Sprintf("input[%d].call_id", i), "function_call_output 需要 call_id")
				}
				output, perr := responsesContentText(item["output"], fmt.Sprintf("input[%d].output", i))
				if perr != nil {
					return nil, perr
				}
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": callID, "content": output})
			case "reasoning":
				// v0 不把历史思维链回传给上游：DeepSeek Responses API 会把纯文本 content
				// 合并进相邻 assistant 消息，这里保持简单，直接忽略。
			default:
				// web_search_call / custom_tool_call 等不在 v0 范围，忽略而不是报错，
				// 避免客户端把上一轮 output item 原样带回时整个请求失败。
			}
		}
	default:
		return nil, responsesParamErr("invalid_input", "input", "input 必须是字符串或消息数组")
	}

	if instructions != nil && strings.TrimSpace(*instructions) != "" {
		// instructions 是「插入模型上下文的第一条 system 消息」。
		messages = append([]map[string]any{{"role": "system", "content": *instructions}}, messages...)
	}
	if len(messages) == 0 {
		return nil, responsesParamErr("invalid_input", "input", "input 不能为空：请传字符串、消息数组或 instructions")
	}
	return messages, nil
}

// responsesInputMessage 转换一条 message item。
func responsesInputMessage(item map[string]any, index int) (map[string]any, *responsesParamError) {
	param := fmt.Sprintf("input[%d]", index)
	role, _ := item["role"].(string)
	role = strings.TrimSpace(role)
	switch role {
	case "user", "assistant", "system":
	case "developer":
		// chat completions 上游普遍没有 developer 角色，按 OpenAI 的等价语义转 system。
		role = "system"
	default:
		return nil, responsesParamErr("invalid_input", param+".role", "消息 role 只支持 user / assistant / system / developer")
	}
	content, perr := responsesContentText(item["content"], param+".content")
	if perr != nil {
		return nil, perr
	}
	return map[string]any{"role": role, "content": content}, nil
}

// responsesAppendToolCall 把一条 function_call 并进上一条 assistant 消息；没有可合并的
// assistant 消息时新建一条 content 为 null 的 assistant。
func responsesAppendToolCall(messages []map[string]any, toolCall map[string]any) []map[string]any {
	if n := len(messages); n > 0 {
		last := messages[n-1]
		if role, _ := last["role"].(string); role == "assistant" {
			if toolCalls, ok := last["tool_calls"].([]any); ok {
				last["tool_calls"] = append(toolCalls, toolCall)
				return messages
			}
			if _, exists := last["tool_calls"]; !exists {
				last["tool_calls"] = []any{toolCall}
				return messages
			}
		}
	}
	return append(messages, map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{toolCall}})
}

// responsesContentText 把 Responses 的文本内容（字符串 / 内容块数组）拍平成字符串。
// v0 只支持文本内容块；图片、文件等返回 400，避免静默丢内容让模型给出错误答案。
func responsesContentText(raw any, param string) (string, *responsesParamError) {
	switch v := raw.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case map[string]any:
		return responsesContentPartText(v, param)
	case []any:
		var b strings.Builder
		for i, rawPart := range v {
			part, ok := rawPart.(map[string]any)
			if !ok {
				return "", responsesParamErr("invalid_content", param, "内容块必须是对象")
			}
			text, perr := responsesContentPartText(part, fmt.Sprintf("%s[%d]", param, i))
			if perr != nil {
				return "", perr
			}
			b.WriteString(text)
		}
		return b.String(), nil
	default:
		return "", responsesParamErr("invalid_content", param, "content 必须是字符串或内容块数组")
	}
}

func responsesContentPartText(part map[string]any, param string) (string, *responsesParamError) {
	partType, _ := part["type"].(string)
	switch partType {
	case "input_text", "output_text", "text", "":
		text, ok := part["text"].(string)
		if !ok {
			return "", responsesParamErr("invalid_content", param+".text", "文本内容块需要字符串 text")
		}
		return text, nil
	default:
		return "", responsesParamErr("unsupported_content", param, "v0 只支持文本内容块，"+partType+" 暂不支持")
	}
}

// responsesRawArguments 读取 function_call 的 arguments：正常是字符串，也兼容
// 客户端把 arguments 直接写成 JSON 对象的情况。
func responsesRawArguments(raw any) string {
	switch v := raw.(type) {
	case nil:
		return "{}"
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}

// ---- 上游调用 ----
//
// 转换层直接复用 gateway.go 的 loadModelCandidates / openChatUpstreamCandidates 与
// llmErrWriter；原生透传要换上游路径（<base>/responses）与 Accept 头，因此保留一份与
// anthropic_native.go 结构相同的候选循环，语义与公共 helper 一致。

// responsesNativeVendor 是支持原生 Responses 格式的 provider vendor。
const responsesNativeVendor = "deepseek"

// responsesNativeVendors 是发现文档公布的走原生 Responses 透传的 vendor 列表。
var responsesNativeVendors = []string{responsesNativeVendor}

// responsesNativeVendorAllowed 判断 provider 是否支持原生 Responses 透传（vendor
// 大小写不敏感）；默认 vendor=custom 的上游仍走 chat completions 转换层。
func responsesNativeVendorAllowed(p *store.Provider) bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.Vendor), responsesNativeVendor)
}

// splitResponsesCandidates 按 vendor 把候选线路分成「原生透传」与「降级转换」两组；
// 两组各自保持原有优先级顺序，调用方按第一条线路决定协议后只在同协议内故障转移，
// 不跨协议切换。
func splitResponsesCandidates(cands []store.Candidate) (native, converted []store.Candidate) {
	for _, c := range cands {
		if responsesNativeVendorAllowed(c.Provider) {
			native = append(native, c)
		} else {
			converted = append(converted, c)
		}
	}
	return native, converted
}

// responsesNativeURL 拼上游原生 Responses 地址：<base>/responses。管理端按 DeepSeek
// 官方文档把 base_url 配成 https://api.deepseek.com 即可；已经显式带 /responses
// 后缀的配置也兼容，避免重复拼接。
func responsesNativeURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/responses") {
		return base
	}
	return base + "/responses"
}

// openResponsesNative 按候选线路依次尝试 DeepSeek 原生 Responses 端点。策略与
// gateway.go 的 openChatUpstreamCandidates 一致：连不上、429、5xx 换下一条线路；
// 一旦开始向客户端写响应就不再切换；失败时已经按 writeErr 写好响应。
func (s *Server) openResponsesNative(w http.ResponseWriter, r *http.Request, k *Principal, model string, body map[string]any, stream bool, start time.Time, cands []store.Candidate, writeErr llmErrWriter) (*http.Response, *store.Provider, bool) {
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
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, responsesNativeURL(p.BaseURL), bytes.NewReader(out))
		if err != nil {
			s.log.Error("build native responses request", "provider", p.Name, "err", err)
			writeErr(http.StatusInternalServerError, "server_error", "internal_error", "服务器内部错误")
			return nil, nil, false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+string(upstreamKey))
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
			s.log.Warn("native responses upstream failed", "provider", p.Name, "err", err)
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
				s.log.Warn("native responses failover", "provider", p.Name, "status", resp.StatusCode)
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

// ---- 上游响应解析 ----

// upstreamChatCompletion 只取 Responses 转换需要的字段；content / arguments
// 用 RawMessage 兼容字符串或数组（避免上游偶尔返回非标准形状时整包解析失败）。
type upstreamChatCompletion struct {
	Choices []struct {
		Message struct {
			Content          json.RawMessage `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func responsesParseUpstreamCompletion(data []byte) (*upstreamChatCompletion, bool) {
	var cc upstreamChatCompletion
	if err := json.Unmarshal(data, &cc); err != nil {
		return nil, false
	}
	if len(cc.Choices) == 0 {
		return nil, false
	}
	return &cc, true
}

// responsesRawText 把上游可能出现的字符串 / 内容块数组 / {text:...} 拍平成文本。
func responsesRawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Text
	}
	return ""
}

func (cc *upstreamChatCompletion) tokenUsage() tokenUsage {
	u := tokenUsage{}
	if cc.Usage != nil {
		u.PromptTokens = cc.Usage.PromptTokens
		u.CompletionTokens = cc.Usage.CompletionTokens
		u.TotalTokens = cc.Usage.TotalTokens
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// ---- Responses 响应对象 ----

type responsesIncompleteDetails struct {
	Reason string `json:"reason"`
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens        int `json:"output_tokens"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	TotalTokens int `json:"total_tokens"`
}

type responsesReasoningEcho struct {
	Effort  *string `json:"effort"`
	Summary any     `json:"summary"`
}

type responsesTextEcho struct {
	Format    map[string]any `json:"format"`
	Verbosity any            `json:"verbosity"`
}

// responsesResponse 是 Responses API 的 response 对象。字段对齐 OpenAI / DeepSeek
// 的 response 结构；v0 不支持的字段用固定值（store=false、previous_response_id=null、
// parallel_tool_calls=true ...），客户端可以按 OpenAI SDK 的类型直接反序列化。
type responsesResponse struct {
	ID                 string                      `json:"id"`
	Object             string                      `json:"object"`
	CreatedAt          int64                       `json:"created_at"`
	Status             string                      `json:"status"`
	Background         bool                        `json:"background"`
	CompletedAt        *int64                      `json:"completed_at"`
	Error              any                         `json:"error"`
	IncompleteDetails  *responsesIncompleteDetails `json:"incomplete_details"`
	Instructions       *string                     `json:"instructions"`
	MaxOutputTokens    *int64                      `json:"max_output_tokens"`
	Model              string                      `json:"model"`
	Output             []any                       `json:"output"`
	ParallelToolCalls  bool                        `json:"parallel_tool_calls"`
	PreviousResponseID any                         `json:"previous_response_id"`
	Reasoning          responsesReasoningEcho      `json:"reasoning"`
	ServiceTier        string                      `json:"service_tier"`
	Store              bool                        `json:"store"`
	Temperature        float64                     `json:"temperature"`
	Text               responsesTextEcho           `json:"text"`
	ToolChoice         any                         `json:"tool_choice"`
	Tools              []any                       `json:"tools"`
	TopLogprobs        int                         `json:"top_logprobs"`
	TopP               float64                     `json:"top_p"`
	Truncation         string                      `json:"truncation"`
	Usage              *responsesUsage             `json:"usage"`
	User               *string                     `json:"user"`
	Metadata           map[string]any              `json:"metadata"`
	FrequencyPenalty   float64                     `json:"frequency_penalty"`
	PresencePenalty    float64                     `json:"presence_penalty"`
}

type responsesOutputText struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
	Logprobs    []any  `json:"logprobs"`
}

type responsesMessageItem struct {
	Type    string                `json:"type"`
	ID      string                `json:"id"`
	Status  string                `json:"status"`
	Role    string                `json:"role"`
	Content []responsesOutputText `json:"content"`
}

type responsesReasoningText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesReasoningItem struct {
	Type    string                   `json:"type"`
	ID      string                   `json:"id"`
	Status  string                   `json:"status"`
	Summary []any                    `json:"summary"`
	Content []responsesReasoningText `json:"content"`
}

type responsesFunctionCallItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// responsesNewID 生成 Responses 形状的 id（resp_ / msg_ / rs_ / fc_ / call_ 前缀）。
func responsesNewID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// responsesBuildResponse 把上游 chat completion 转成 Responses response 对象。
func responsesBuildResponse(model string, meta *responsesRequestMeta, cc *upstreamChatCompletion, createdAt time.Time) *responsesResponse {
	completedAt := time.Now().Unix()
	choice := cc.Choices[0]
	message := choice.Message

	status := "completed"
	var incomplete *responsesIncompleteDetails
	switch choice.FinishReason {
	case "length":
		status = "incomplete"
		incomplete = &responsesIncompleteDetails{Reason: "max_output_tokens"}
	case "content_filter":
		status = "incomplete"
		incomplete = &responsesIncompleteDetails{Reason: "content_filter"}
	}

	output := []any{}
	reasoning := strings.TrimSpace(message.ReasoningContent)
	if reasoning == "" {
		reasoning = strings.TrimSpace(responsesRawText(message.Reasoning))
	}
	if reasoning != "" {
		// 思考链按 DeepSeek Responses 的形状放在 reasoning item 的 content（reasoning_text）；
		// v0 不生成 summary / encrypted_content。
		output = append(output, &responsesReasoningItem{
			Type:    "reasoning",
			ID:      responsesNewID("rs"),
			Status:  "completed",
			Summary: []any{},
			Content: []responsesReasoningText{{Type: "reasoning_text", Text: reasoning}},
		})
	}

	content := responsesRawText(message.Content)
	toolCalls := message.ToolCalls
	if content != "" || (len(toolCalls) == 0 && reasoning == "") {
		output = append(output, &responsesMessageItem{
			Type:   "message",
			ID:     responsesNewID("msg"),
			Status: "completed",
			Role:   "assistant",
			Content: []responsesOutputText{{
				Type:        "output_text",
				Text:        content,
				Annotations: []any{},
				Logprobs:    []any{},
			}},
		})
	}
	for _, toolCall := range toolCalls {
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = responsesNewID("call")
		}
		output = append(output, &responsesFunctionCallItem{
			Type:      "function_call",
			ID:        responsesNewID("fc"),
			Status:    "completed",
			CallID:    callID,
			Name:      toolCall.Function.Name,
			Arguments: responsesRawFunctionArguments(toolCall.Function.Arguments),
		})
	}

	temperature, topP := 1.0, 1.0
	if meta.temperature != nil {
		temperature = *meta.temperature
	}
	if meta.topP != nil {
		topP = *meta.topP
	}
	parallelToolCalls := true
	if meta.parallelToolCalls != nil {
		parallelToolCalls = *meta.parallelToolCalls
	}
	textFormat := meta.textFormat
	if textFormat == nil {
		textFormat = map[string]any{"type": "text"}
	}
	tools := meta.tools
	if tools == nil {
		tools = []any{}
	}
	toolChoice := meta.toolChoice
	if toolChoice == nil {
		toolChoice = "auto"
	}

	resp := &responsesResponse{
		ID:                 responsesNewID("resp"),
		Object:             "response",
		CreatedAt:          createdAt.Unix(),
		Status:             status,
		Background:         false,
		CompletedAt:        &completedAt,
		Error:              nil,
		IncompleteDetails:  incomplete,
		Instructions:       meta.instructions,
		MaxOutputTokens:    meta.maxOutputTokens,
		Model:              model,
		Output:             output,
		ParallelToolCalls:  parallelToolCalls,
		PreviousResponseID: nil,
		Reasoning:          responsesReasoningEcho{Effort: meta.reasoningEffort, Summary: nil},
		ServiceTier:        "default",
		Store:              false,
		Temperature:        temperature,
		Text:               responsesTextEcho{Format: textFormat, Verbosity: nil},
		ToolChoice:         toolChoice,
		Tools:              tools,
		TopLogprobs:        0,
		TopP:               topP,
		Truncation:         "disabled",
		User:               meta.user,
		Metadata:           map[string]any{},
		FrequencyPenalty:   0,
		PresencePenalty:    0,
	}
	if cc.Usage != nil {
		resp.Usage = &responsesUsage{
			InputTokens:  cc.Usage.PromptTokens,
			OutputTokens: cc.Usage.CompletionTokens,
			TotalTokens:  cc.Usage.TotalTokens,
		}
		resp.Usage.InputTokensDetails.CachedTokens = cc.Usage.PromptTokensDetails.CachedTokens
		resp.Usage.OutputTokensDetails.ReasoningTokens = cc.Usage.CompletionTokensDetails.ReasoningTokens
		if resp.Usage.TotalTokens == 0 {
			resp.Usage.TotalTokens = resp.Usage.InputTokens + resp.Usage.OutputTokens
		}
	} else {
		resp.Usage = &responsesUsage{}
	}
	return resp
}

// responsesRawFunctionArguments 把上游 tool_calls[].function.arguments 转成字符串。
func responsesRawFunctionArguments(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// ---- 流式回放 ----

// responsesEmitEvent 写一个 Responses SSE 事件并立即 flush（事件格式：
// event: <type> + data: <json> + 空行）。v0 事件是拿到完整上游结果后回放的，
// 所以每个 delta 只发一次、内容是整段文本；事件顺序与字段仍按 Responses 语义。
func responsesEmitEvent(w http.ResponseWriter, eventType string, sequence int, fields map[string]any) {
	payload := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		payload[k] = v
	}
	payload["type"] = eventType
	payload["sequence_number"] = sequence
	data, err := json.Marshal(payload)
	if err != nil {
		// 字段都由本文件构造，序列化失败只可能是内部类型错误；不写半截事件，结束回放。
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// responsesInProgressCopy 返回一个用于 response.created / response.in_progress 的
// 浅拷贝：status=in_progress，output 为空，usage 与完成时间都为 null。
func responsesInProgressCopy(final *responsesResponse) *responsesResponse {
	copied := *final
	copied.Status = "in_progress"
	copied.Output = []any{}
	copied.Usage = nil
	copied.CompletedAt = nil
	copied.IncompleteDetails = nil
	copied.Error = nil
	return &copied
}

// responsesWriteStream 把完整结果按 Responses SSE 事件回放给客户端。
func responsesWriteStream(w http.ResponseWriter, final *responsesResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sequence := 0
	inProgress := responsesInProgressCopy(final)
	responsesEmitEvent(w, "response.created", sequence, map[string]any{"response": inProgress})
	sequence++
	responsesEmitEvent(w, "response.in_progress", sequence, map[string]any{"response": inProgress})
	sequence++

	for outputIndex, rawItem := range final.Output {
		switch item := rawItem.(type) {
		case *responsesMessageItem:
			added := &responsesMessageItem{Type: "message", ID: item.ID, Status: "in_progress", Role: "assistant", Content: []responsesOutputText{}}
			responsesEmitEvent(w, "response.output_item.added", sequence, map[string]any{"output_index": outputIndex, "item": added})
			sequence++
			text := ""
			if len(item.Content) > 0 {
				text = item.Content[0].Text
			}
			emptyPart := responsesOutputText{Type: "output_text", Text: "", Annotations: []any{}, Logprobs: []any{}}
			responsesEmitEvent(w, "response.content_part.added", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "part": emptyPart,
			})
			sequence++
			responsesEmitEvent(w, "response.output_text.delta", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "delta": text, "logprobs": []any{},
			})
			sequence++
			responsesEmitEvent(w, "response.output_text.done", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "text": text, "logprobs": []any{},
			})
			sequence++
			part := emptyPart
			if len(item.Content) > 0 {
				part = item.Content[0]
			}
			responsesEmitEvent(w, "response.content_part.done", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "part": part,
			})
			sequence++
			responsesEmitEvent(w, "response.output_item.done", sequence, map[string]any{"output_index": outputIndex, "item": item})
			sequence++
		case *responsesReasoningItem:
			added := &responsesReasoningItem{Type: "reasoning", ID: item.ID, Status: "in_progress", Summary: []any{}, Content: []responsesReasoningText{}}
			responsesEmitEvent(w, "response.output_item.added", sequence, map[string]any{"output_index": outputIndex, "item": added})
			sequence++
			text := ""
			if len(item.Content) > 0 {
				text = item.Content[0].Text
			}
			emptyPart := responsesReasoningText{Type: "reasoning_text", Text: ""}
			responsesEmitEvent(w, "response.content_part.added", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "part": emptyPart,
			})
			sequence++
			responsesEmitEvent(w, "response.reasoning_text.delta", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "delta": text,
			})
			sequence++
			responsesEmitEvent(w, "response.reasoning_text.done", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "text": text,
			})
			sequence++
			part := emptyPart
			if len(item.Content) > 0 {
				part = item.Content[0]
			}
			responsesEmitEvent(w, "response.content_part.done", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "content_index": 0, "part": part,
			})
			sequence++
			responsesEmitEvent(w, "response.output_item.done", sequence, map[string]any{"output_index": outputIndex, "item": item})
			sequence++
		case *responsesFunctionCallItem:
			added := &responsesFunctionCallItem{Type: "function_call", ID: item.ID, Status: "in_progress", CallID: item.CallID, Name: item.Name, Arguments: ""}
			responsesEmitEvent(w, "response.output_item.added", sequence, map[string]any{"output_index": outputIndex, "item": added})
			sequence++
			responsesEmitEvent(w, "response.function_call_arguments.delta", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "delta": item.Arguments,
			})
			sequence++
			responsesEmitEvent(w, "response.function_call_arguments.done", sequence, map[string]any{
				"item_id": item.ID, "output_index": outputIndex, "arguments": item.Arguments,
			})
			sequence++
			responsesEmitEvent(w, "response.output_item.done", sequence, map[string]any{"output_index": outputIndex, "item": item})
			sequence++
		}
	}

	finalEvent := "response.completed"
	if final.Status == "incomplete" {
		finalEvent = "response.incomplete"
	}
	responsesEmitEvent(w, finalEvent, sequence, map[string]any{"response": final})
}

// ---- DeepSeek 原生 Responses 透传 ----
//
// provider.vendor=deepseek 的上游自己提供 Responses API（例如 DeepSeek 官方
// /responses）。这类候选不走 chat completions 转换：请求 JSON 语义透传（只把
// model 换成上游模型名），响应与 SSE 逐字节转发，dsh-ee 只做鉴权、模型权限、
// 额度、限流、用量旁路记账与同厂商故障转移。这样 web_search 等服务端工具、
// citations/annotations、refusal、title、image/file 内容块都不会被转换层丢掉。

// nativeResponsesUsage 是 DeepSeek 原生 Responses 响应里的 usage 形状
// （与 gateway 的 chat usage 字段不同：input_tokens / output_tokens）。
type nativeResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (u *nativeResponsesUsage) tokenUsage() tokenUsage {
	t := tokenUsage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, TotalTokens: u.TotalTokens}
	if t.TotalTokens == 0 {
		t.TotalTokens = t.PromptTokens + t.CompletionTokens
	}
	return t
}

// responsesNativeBodyUsage 从非流式原生响应体里取 usage。
func responsesNativeBodyUsage(data []byte) (tokenUsage, bool) {
	var parsed struct {
		Usage *nativeResponsesUsage `json:"usage"`
	}
	if json.Unmarshal(data, &parsed) != nil || parsed.Usage == nil {
		return tokenUsage{}, false
	}
	return parsed.Usage.tokenUsage(), true
}

// relayNativeResponsesSSE 逐字节转发上游 Responses SSE，同时旁路解析
// response.completed / incomplete / failed 里的 response.usage。第二个返回值表示
// 是否解析到 usage；第三个返回值是按已转发的 delta 文本估出的输出 token 数，
// 客户端中途断开拿不到 usage 时兜底记账。
func relayNativeResponsesSSE(w http.ResponseWriter, body io.Reader) (tokenUsage, bool, int) {
	var usage tokenUsage
	parsedUsage := false
	completionEstimate := 0
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReaderSize(body, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return usage, parsedUsage, completionEstimate // 客户端断开
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				if flusher != nil {
					flusher.Flush()
				}
			} else if payload, ok := bytes.CutPrefix(trimmed, []byte("data:")); ok {
				payload = bytes.TrimSpace(payload)
				if len(payload) > 0 && !bytes.Equal(payload, []byte("[DONE]")) {
					var event struct {
						Type     string `json:"type"`
						Delta    string `json:"delta"`
						Response *struct {
							Usage *nativeResponsesUsage `json:"usage"`
						} `json:"response"`
					}
					if json.Unmarshal(payload, &event) == nil {
						if event.Response != nil && event.Response.Usage != nil {
							usage = event.Response.Usage.tokenUsage()
							parsedUsage = true
						}
						if event.Delta != "" {
							completionEstimate += estimateTokens([]byte(event.Delta))
						}
					}
				}
			}
		}
		if err != nil {
			if flusher != nil {
				flusher.Flush()
			}
			return usage, parsedUsage, completionEstimate
		}
	}
}

// responsesFileAccessDeniedCode 与 #160（DeepSeek 工具代理）约定的错误码一致：
// 普通员工引用已上传文件时返回 403 file_access_denied，客户端可据此提示。
// #160 合入后本文件的本地实现应替换成它的 deepseekToolsAllowed / hasNonEmptyFileID，
// 避免两套判断漂移。
const responsesFileAccessDeniedCode = "file_access_denied"

// responsesFileAccessAllowed 与 DeepSeek 工具代理共用同一条访问边界（deepseekToolsAllowed）：
// 默认只允许管理员与机器人/服务账号引用上游共享文件。
func (s *Server) responsesFileAccessAllowed(p *Principal) bool { return s.deepseekToolsAllowed(p) }

// responsesReferencesFileID 报告 Responses 请求的 input 里是否引用了已上传文件的
// file_id（input_file / input_image 等任何带非空 file_id 的内容块；递归查找）。
// 只扫描 input，不扫描 tools 声明的 JSON Schema，避免把属性名误判成文件引用。
// 流式与非流式请求共用同一份解析后的 body，因此都在转发上游之前被拦截。
func responsesReferencesFileID(body map[string]any) bool {
	input, ok := body["input"]
	if !ok {
		return false
	}
	return responsesHasNonEmptyFileID(input)
}

func responsesHasNonEmptyFileID(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if id, ok := x["file_id"].(string); ok && strings.TrimSpace(id) != "" {
			return true
		}
		for _, vv := range x {
			if responsesHasNonEmptyFileID(vv) {
				return true
			}
		}
	case []any:
		for _, vv := range x {
			if responsesHasNonEmptyFileID(vv) {
				return true
			}
		}
	}
	return false
}

// estimateResponsesNativeContent 在没有 usage 时按响应正文的 output 文本/思考/工具
// 参数估算输出 token，避免原生调用记成 0 让月度额度形同虚设。
func estimateResponsesNativeContent(data []byte) int {
	var parsed struct {
		Output []struct {
			Arguments string `json:"arguments"`
			Content   []struct {
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
		} `json:"output"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return estimateTokens(data)
	}
	n := 0
	for _, item := range parsed.Output {
		for _, part := range item.Content {
			n += estimateTokens([]byte(part.Text))
		}
		for _, part := range item.Summary {
			n += estimateTokens([]byte(part.Text))
		}
		n += estimateTokens([]byte(item.Arguments))
	}
	if n == 0 {
		n = estimateTokens(data)
	}
	return n
}

// handleResponsesNative 转发一次 DeepSeek 原生 Responses 请求：请求 JSON 只改 model
// 后转发 <provider.base>/responses，成功响应与 SSE 逐字节转发，错误映射成 Responses
// 错误壳，usage 旁路记账。候选已限定为同厂商（见 splitResponsesCandidates）。
func (s *Server) handleResponsesNative(w http.ResponseWriter, r *http.Request, k *Principal, model string, rawRequest []byte, body map[string]any, start time.Time, cands []store.Candidate) {
	stream, _ := body["stream"].(bool)
	upstreamResp, provider, ok := s.openResponsesNative(w, r, k, model, body, stream, start, cands, s.responsesErrorWriter(w))
	if !ok {
		return
	}
	defer upstreamResp.Body.Close()

	if upstreamResp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(io.LimitReader(upstreamResp.Body, maxResponseBody))
		s.recordUsage(r.Context(), k, provider, model, tokenUsage{}, upstreamResp.StatusCode, start)
		responsesWriteUpstreamError(w, upstreamResp.StatusCode, data)
		return
	}

	contentType := upstreamResp.Header.Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	promptEstimate := estimateTokens(rawRequest)
	if strings.HasPrefix(strings.ToLower(contentType), "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(upstreamResp.StatusCode)
		usage, parsedUsage, deltaTokens := relayNativeResponsesSSE(w, upstreamResp.Body)
		if !parsedUsage {
			usage = tokenUsage{PromptTokens: promptEstimate, CompletionTokens: deltaTokens}
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			s.log.Warn("native responses stream ended without usage, recorded estimate",
				"provider", provider.Name, "model", model,
				"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
		}
		s.recordUsage(r.Context(), k, provider, model, usage, upstreamResp.StatusCode, start)
		return
	}

	data, err := io.ReadAll(io.LimitReader(upstreamResp.Body, maxResponseBody))
	if err != nil {
		s.log.Warn("native responses read upstream body", "err", err)
		s.recordUsage(r.Context(), k, provider, model, tokenUsage{}, http.StatusBadGateway, start)
		writeResponsesErr(w, http.StatusBadGateway, "server_error", "upstream_bad_response", "", "上游响应读取失败")
		return
	}
	w.WriteHeader(upstreamResp.StatusCode)
	_, _ = w.Write(data) // 非流式响应逐字节转发
	usage, parsedUsage := responsesNativeBodyUsage(data)
	if !parsedUsage {
		usage = tokenUsage{
			PromptTokens:     promptEstimate,
			CompletionTokens: estimateResponsesNativeContent(data),
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		s.log.Warn("native responses body without usage, recorded estimate",
			"provider", provider.Name, "model", model,
			"prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
	}
	s.recordUsage(r.Context(), k, provider, model, usage, upstreamResp.StatusCode, start)
}

// ---- 错误映射 ----

// responsesWriteUpstreamError 把上游的非 200 响应转成 Responses 错误壳：能解析出
// 上游 error.message/type/code 就沿用（客户端能看懂「模型不存在」这类原因），
// 解析不出来时按 HTTP 状态给一个通用 type。状态码保留上游的，便于客户端区分
// 4xx（请求/权限问题）和 5xx（上游故障）。流式请求在开始写 SSE 之前失败时也走这里。
func responsesWriteUpstreamError(w http.ResponseWriter, status int, body []byte) {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)

	message := strings.TrimSpace(parsed.Error.Message)
	if message == "" {
		message = fmt.Sprintf("上游返回 HTTP %d：%s", status, snippet(body))
	}
	typ := parsed.Error.Type
	if typ == "" {
		typ = responsesUpstreamErrorType(status)
	}
	code := parsed.Error.Code
	if code == "" {
		code = "upstream_error"
	}
	writeResponsesErr(w, status, typ, code, "", message)
}

func responsesUpstreamErrorType(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status == http.StatusForbidden:
		return "permission_error"
	case status == http.StatusNotFound:
		return "not_found_error"
	case status >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}

// ---- 入口 ----

// handleResponses 处理 POST /v1/responses：鉴权 → 校验/转换请求 → 调上游 →
// 转回 Responses response（非流式 JSON，或 v0 的完整结果 SSE 回放）。
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	k, quota, ok := s.responsesAuth(w, r)
	if !ok {
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil || len(raw) > maxRequestBody {
		writeResponsesErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large", "", "请求体过大")
		return
	}
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // 保留数字原样，转发时不改精度
	if err := dec.Decode(&body); err != nil {
		writeResponsesErr(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "请求体不是合法 JSON")
		return
	}

	model, _ := body["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" {
		writeResponsesErr(w, http.StatusBadRequest, "invalid_request_error", "model_required", "model", "缺少 model")
		return
	}
	if !modelAllowed(k, quota, model) {
		writeResponsesErr(w, http.StatusForbidden, "permission_error", "model_not_allowed", "model",
			"你的账号不能使用模型 "+model+"，可在 /ee-status 查看可用模型")
		return
	}
	cands, ok := s.loadModelCandidates(w, r, model, s.responsesErrorWriter(w))
	if !ok {
		return
	}
	// 首选线路决定协议：DeepSeek 候选走上游原生 Responses 透传，其它厂商走 chat
	// 转换层；故障转移只在同协议/厂商的候选之间进行，不跨协议。
	nativeCands, convertedCands := splitResponsesCandidates(cands)
	if responsesNativeVendorAllowed(cands[0].Provider) {
		// 原生路径：不解析/不改写请求内容块，只改 model；响应逐字节转发 + usage 旁路。
		// 但文件属于上游共享账号、员工之间不隔离：普通员工即使知道 file_id，也不能借
		// input 里的 input_file / input_image 让上游读取该账号的文件，因此在转发前拦截。
		if !s.responsesFileAccessAllowed(k) && responsesReferencesFileID(body) {
			writeResponsesErr(w, http.StatusForbidden, "permission_error", responsesFileAccessDeniedCode, "input",
				"当前账号不能引用已上传的文件（file_id）；DeepSeek 文件默认只允许管理员与机器人账号使用")
			return
		}
		if !s.checkLLMQuota(w, r, k, quota, s.responsesErrorWriter(w)) {
			return
		}
		s.handleResponsesNative(w, r, k, model, raw, body, start, nativeCands)
		return
	}

	stream, _ := body["stream"].(bool)
	chatBody, meta, perr := responsesBuildChatRequest(body, model)
	if perr != nil {
		perr.write(w)
		return
	}
	if !s.checkLLMQuota(w, r, k, quota, s.responsesErrorWriter(w)) {
		return
	}
	// 转换层复用 gateway.go 的候选线路 helper（与 chat / Anthropic 端点同一套冷却、
	// 故障转移与失败记账），只传非 DeepSeek 候选，保证不跨协议。
	upstreamResp, provider, ok := s.openChatUpstreamCandidates(w, r, k, model, chatBody, false, start, convertedCands, s.responsesErrorWriter(w), nil)
	if !ok {
		return
	}
	defer upstreamResp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(upstreamResp.Body, maxResponseBody))
	if err != nil {
		s.log.Warn("responses read upstream body", "err", err)
		s.recordUsage(r.Context(), k, provider, model, tokenUsage{}, upstreamResp.StatusCode, start)
		writeResponsesErr(w, http.StatusBadGateway, "server_error", "upstream_bad_response", "", "上游响应读取失败")
		return
	}
	if upstreamResp.StatusCode != http.StatusOK {
		s.recordUsage(r.Context(), k, provider, model, tokenUsage{}, upstreamResp.StatusCode, start)
		responsesWriteUpstreamError(w, upstreamResp.StatusCode, data)
		return
	}

	completion, ok := responsesParseUpstreamCompletion(data)
	if !ok {
		s.recordUsage(r.Context(), k, provider, model, tokenUsage{}, http.StatusBadGateway, start)
		writeResponsesErr(w, http.StatusBadGateway, "server_error", "upstream_bad_response", "", "上游返回了无法解析的响应")
		return
	}

	usage := completion.tokenUsage()
	s.recordUsage(r.Context(), k, provider, model, usage, http.StatusOK, start)

	response := responsesBuildResponse(model, &meta, completion, start)
	if stream {
		responsesWriteStream(w, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
