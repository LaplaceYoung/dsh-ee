package api

// DeepSeek Files（file_id）访问拦截。
//
// DeepSeek 的 Files 是上游账号级资源：管理员用同一个上游 key 上传的文件，任何带该 key 的
// 请求都能用 file_id 引用；而 dsh-ee 的普通员工令牌与管理员共用上游 key，所以「员工能不能
// 引用文件」只能由 dsh-ee 在转发前判断。这里提供共享判定与请求扫描：
//
//   - 管理员账号（账号本身有任一管理权限，rbac.HasAdminPerms）放行；
//   - account_type=bot 的机器人账号放行（自动化流程本来就要读共享资料）；
//   - 其它账号（普通员工）默认拒绝，fail closed；
//   - DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE=true 时才对全体员工放行。
//
// 当前覆盖 `/v1/chat/completions` 与 `/v1/messages`（含别名 `/anthropic/v1/messages`），
// 命中时在选线路、请求上游之前返回 403 file_access_denied。Files 端点本身不在 dsh-ee 实现
// （由引入 Files 代理的 PR 负责）；后续新增的模型入口（如 /v1/responses）必须复用
// fileAccessAllowed 与下面两个扫描函数，不能只依赖各自的转换/透传逻辑。
//
// 这里只判断「请求里有没有引用上游文件」，不记录 file_id、不做内容审计：审计属后续工作。

import (
	"net/http"
	"strings"
)

// fileAccessDeniedMessage 是员工引用上游共享文件时的统一说明。
const fileAccessDeniedMessage = "请求引用了上游共享文件（file_id），当前账号无权使用；共享文件默认只对管理员与机器人账号开放"

// fileAccessAllowed 判断当前令牌/账号能否在模型请求里引用上游共享文件（DeepSeek file_id）。
// 判定是「账号级」的：管理员账号即便这次没用管理令牌也放行，与令牌当前有没有开管理模式无关。
// 旧版 sk-ee / ee_npm_ 令牌的 Principal 不带账号管理标记，这里按员工处理（fail closed）；
// 管理员引用共享文件请用 ee_at_ 令牌，机器人流程用 account_type=bot 账号。
func (s *Server) fileAccessAllowed(p *Principal) bool {
	// 与 DeepSeek 工具代理共用同一条边界：管理员、机器人账号，或部署显式放开员工
	// （DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE）。默认 fail closed。
	return s.deepseekToolsAllowed(p)
}

// containsUpstreamFileRef 递归判断一个已解析的 JSON 值里有没有引用上游文件：对象里出现
// 非空 file_id 字段（例如 file 块的 {"type":"file","file_id":…}），或 source.file_id
// （document / image 等块的 {"source":{"type":"file","file_id":…}}）。
// 字符串不解析：工具参数是 JSON 字符串，里面出现 file_id 字样不代表在引用文件。
func containsUpstreamFileRef(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if fileID, ok := x["file_id"].(string); ok && strings.TrimSpace(fileID) != "" {
			return true
		}
		for _, value := range x {
			if containsUpstreamFileRef(value) {
				return true
			}
		}
	case []any:
		for _, item := range x {
			if containsUpstreamFileRef(item) {
				return true
			}
		}
	}
	return false
}

// chatRequestReferencesFiles 扫描 OpenAI chat completions 请求的 messages。DeepSeek 的
// file 块（{"type":"file","file_id":…}）与文档块
// （{"type":"document","source":{"type":"file","file_id":…}}）都在消息内容块里；
// 递归扫描能同时覆盖 content 数组、tool 结果等嵌套位置。
func chatRequestReferencesFiles(body map[string]any) bool {
	return containsUpstreamFileRef(body["messages"])
}

// anthropicContentReferencesFiles 扫描 Anthropic content 块：file 块的 file_id、
// document / image 块的 source.file_id；tool_result 里嵌套的 content 块也继续递归扫描。
// 只在这个函数里往下走 content，避免把客户端工具的 input 对象里恰好叫 file_id 的参数
// 误判成上游文件引用。
func anthropicContentReferencesFiles(content any) bool {
	switch x := content.(type) {
	case []any:
		for _, raw := range x {
			if anthropicContentReferencesFiles(raw) {
				return true
			}
		}
	case map[string]any:
		if fileID, ok := x["file_id"].(string); ok && strings.TrimSpace(fileID) != "" {
			return true
		}
		if source, ok := x["source"].(map[string]any); ok {
			if fileID, ok := source["file_id"].(string); ok && strings.TrimSpace(fileID) != "" {
				return true
			}
		}
		// tool_result 等块的 content 仍然是 content 块数组，继续往下扫。
		if nested, ok := x["content"]; ok && anthropicContentReferencesFiles(nested) {
			return true
		}
	}
	return false
}

// anthropicRequestReferencesFiles 扫描 Anthropic Messages 请求的 messages[].content。
func anthropicRequestReferencesFiles(body map[string]any) bool {
	messages, _ := body["messages"].([]any)
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		if msg == nil {
			continue
		}
		if anthropicContentReferencesFiles(msg["content"]) {
			return true
		}
	}
	return false
}

// writeOpenAIFileAccessDenied 按 OpenAI 错误壳返回 403。这里直接写 OpenAI 壳、不走
// writeGatewayError：DSH 接入插件会把未知 code 映射成 400，而文件越权要保留 403。
func writeOpenAIFileAccessDenied(w http.ResponseWriter) {
	writeOpenAIErr(w, http.StatusForbidden, "permission_error", "file_access_denied", fileAccessDeniedMessage)
}

// writeAnthropicFileAccessDenied 按 Anthropic 错误壳返回 403，并额外带 error.code，
// 让 Anthropic 客户端能区分「文件越权」与其它 permission_error。
func writeAnthropicFileAccessDenied(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    "permission_error",
			"code":    "file_access_denied",
			"message": fileAccessDeniedMessage,
		},
	})
}
