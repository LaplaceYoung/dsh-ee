package api

// DeepSeek Chatbot 网关选项与透传（docs/dev/chatbot.md）。
//
// DeepSeek 的 Build a Chatbot 能力（Web Tools、模型内容安全、会话标题）都挂在
// Chat Completions 的扩展字段上：服务端工具在 tools[] 里用 type 声明，安全审核和
// 会话标题分别用顶层 safety_check / title 打开；响应里的 title、annotations/citations、
// refusal、server_tool_calls 等块由上游生成。dsh-ee 的网关只做两件事：
//
//   - 按部署配置给 vendor=deepseek 的上游请求补默认值（客户端已有对应字段时不动）；
//   - 请求体与响应原样转发，不解析也不改写这些扩展字段。
//
// 注入只影响发往该条 DeepSeek 上游线路的那次请求：不改变模型名、额度与记账，
// 非 DeepSeek 上游不注入。响应（含流式）逐字节转发，只旁路提取 usage 记账。

const (
	// deepseekVendor 是 store.Providers.Vendor 里 DeepSeek 官方上游的标识。
	deepseekVendor = "deepseek"
	// deepseekWebSearchTool 是 DeepSeek 服务端内置的联网搜索工具类型，网关只透传。
	deepseekWebSearchTool = "deepseek_web_search"
	// deepseekReadURLTool 是 DeepSeek 服务端内置的网页读取工具类型，网关只透传。
	deepseekReadURLTool = "deepseek_read_url"
)

// upstreamChatBody 生成发往一条上游线路的请求体：复制客户端 body 的顶层字段，
// 把 model 换成这条线路的上游模型名；线路是 DeepSeek 时再按部署配置补默认的
// safety_check / title。
//
// 返回新 map，不改调用方传入的 body，也不影响其它线路去重后的字段：故障切换时
// 不会把 DeepSeek 线路的注入字段带给非 DeepSeek 上游。客户端已经带了
// safety_check / title（包括显式 null 或别的级别）时原样保留，网关不覆盖。
func (s *Server) upstreamChatBody(body map[string]any, upstreamModel, vendor string) map[string]any {
	out := make(map[string]any, len(body)+2)
	for k, v := range body {
		out[k] = v
	}
	out["model"] = upstreamModel
	if vendor != deepseekVendor {
		return out
	}
	if level := s.opts.SafetyCheckLevel; level != "" {
		if _, ok := out["safety_check"]; !ok {
			out["safety_check"] = map[string]any{"level": level}
		}
	}
	if s.opts.ChatTitle {
		if _, ok := out["title"]; !ok {
			out["title"] = map[string]any{"enabled": true}
		}
	}
	return out
}

// harnessChatbot 声明 Chatbot 能力在网关侧的支持方式：这些字段由上游 DeepSeek 解释，
// dsh-ee 只注入默认值并原样透传请求与响应。发现文档与 /v1/capabilities 共用这个形状。
type harnessChatbot struct {
	// UpstreamVendor 是唯一会注入/解释这些字段的上游类型。
	UpstreamVendor string `json:"upstream_vendor"`
	// Passthrough 表示 tools/safety_check/title 与响应扩展块原样转发，不解析、不改写。
	Passthrough bool `json:"passthrough"`
	// ServerTools 是 tools[].type 里透传给上游的服务端工具（由上游执行，客户端不执行）。
	ServerTools []string `json:"server_tools"`
	// SafetyCheck 是 SSE 内容安全审核（safety_check）的透传与本部署注入状态。
	SafetyCheck harnessChatbotSafety `json:"safety_check"`
	// Title 是会话标题（title）的透传与本部署注入状态。
	Title harnessChatbotTitle `json:"title"`
	// Refusal 表示响应里的拒绝标记（Chat Completions 的 finish_reason=content_filter /
	// message.refusal，Messages 的 stop_reason=refusal）原样透传。
	Refusal bool `json:"refusal"`
	// ResponseFields 是原样保留的响应扩展字段/块名（随协议不同，见 docs/dev/chatbot.md）。
	ResponseFields []string `json:"response_fields"`
	// Note 说明这些能力依赖上游 DeepSeek 支持、dsh-ee 不实现服务端工具，以及注入开关
	// 只覆盖 Chat Completions（/v1/messages 原生透传不注入）。
	Note string `json:"note"`
}

// harnessChatbotSafety 是 safety_check 的透传与注入状态。
type harnessChatbotSafety struct {
	// Supported 表示网关原样透传客户端的 safety_check（上游是否支持取决于上游与模型）。
	Supported bool `json:"supported"`
	// Injected 表示本部署是否在客户端未传时注入 safety_check（DSH_EE_SAFETY_CHECK=high）。
	Injected bool `json:"injected"`
	// Level 是注入的审核级别，仅在 Injected 时出现。
	Level string `json:"level,omitempty"`
}

// harnessChatbotTitle 是 title 的透传与注入状态。
type harnessChatbotTitle struct {
	// Supported 表示网关原样透传客户端的 title。
	Supported bool `json:"supported"`
	// Injected 表示本部署是否在客户端未传时注入 title（DSH_EE_CHAT_TITLE=on）。
	Injected bool `json:"injected"`
}

// chatbotCapability 返回当前部署的 Chatbot 能力声明。注入状态来自启动配置，
// 进程内固定；上游支持情况无法在网关侧探测，用 Note 说明「需要上游 DeepSeek 支持」。
func (s *Server) chatbotCapability() harnessChatbot {
	return harnessChatbot{
		UpstreamVendor: deepseekVendor,
		Passthrough:    true,
		ServerTools:    []string{deepseekWebSearchTool, deepseekReadURLTool},
		SafetyCheck: harnessChatbotSafety{
			Supported: true,
			Injected:  s.opts.SafetyCheckLevel != "",
			Level:     s.opts.SafetyCheckLevel,
		},
		Title: harnessChatbotTitle{
			Supported: true,
			Injected:  s.opts.ChatTitle,
		},
		Refusal: true,
		// Chat Completions 用 server_tool_calls/annotations，Messages/Responses 用
		// server_tool_use/citations；dsh-ee 只转 Chat Completions，这里都列出来方便客户端识别。
		ResponseFields: []string{
			"title",
			"annotations",
			"citations",
			"refusal",
			"server_tool_calls",
			"server_tool_use",
		},
		Note: "这些能力由上游 DeepSeek API 实现（如 deepseek-flash 的 Web Tools / Safety / Title）；dsh-ee 只透传请求字段与响应块，不解析、不改写，也不保证每个上游模型都支持。两个注入开关只作用于 /v1/chat/completions；/v1/messages（Anthropic 原生透传）只原样转发客户端自带的字段，不做注入。",
	}
}
