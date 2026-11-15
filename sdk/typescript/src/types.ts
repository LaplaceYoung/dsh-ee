// 本 SDK 用到的 dsh-ee L1 契约类型（HTTP + MCP）。
//
// 契约来源：
//   - GET /.well-known/dsh-ee、GET /v1/capabilities、GET /v1/apps（harness v1 中立契约）
//   - GET /v1/models、POST /v1/chat/completions（OpenAI chat completions 子集）
//   - POST /api/device/code、POST /api/device/token（OAuth 设备码风格）
//   - POST /mcp（MCP streamable HTTP；tools/list 支持 cursor 分页，写确认结果带结果级 _meta）
//
// 这里只描述 dsh-ee 实际返回的字段；第三方上游模型参数等透传字段用索引签名兜底。

/** 可注入的 fetch 实现；默认使用全局 fetch（Node 18+ / 浏览器）。 */
export type FetchLike = (url: string, init?: RequestInit) => Promise<Response>;

/** 服务端返回的限流状态（X-RateLimit-*，见 dsh-ee 的 /v1/* 与 /mcp 令牌级限流）。 */
export interface RateLimitInfo {
  /** 窗口内允许的请求数（X-RateLimit-Limit）。 */
  limit: number;
  /** 当前窗口剩余请求数（X-RateLimit-Remaining）。 */
  remaining: number;
  /** 窗口重置时间，Unix 秒（X-RateLimit-Reset）。 */
  reset: number;
  /** 窗口重置时间（由 reset 换算）。 */
  resetAt: Date;
}

// ---- 发现与能力 ----

/**
 * 思考模式能力声明。
 *
 * 出现在 `GET /.well-known/dsh-ee` 的 `models.reasoning` 与 `GET /v1/capabilities` 的 `reasoning`：
 * 网关把 `thinking` / `reasoning_effort` 原样转发给上游，响应里的思维链在 `reasoning_content` 字段
 * （非流式在 `choices[].message`，流式在 `choices[].delta`）。具体每个模型是否支持、可用哪些
 * effort，见 `GET /v1/models` 每项的 `capabilities.reasoning`。
 */
export interface HarnessReasoningCapability {
  /** 网关是否原样转发思考模式参数（不拦不改、不做参数强校验）。 */
  supported: boolean;
  /** 请求体里可用的参数名，例如 `["thinking", "reasoning_effort"]`。 */
  parameters: string[];
  /** 响应里思维链内容的字段名，通常是 `"reasoning_content"`。 */
  content_field: string;
  /** 为 true 时按模型的能力在 `GET /v1/models` 的 `capabilities.reasoning` 里公布。 */
  per_model: boolean;
}

/** `GET /v1/models` 每项新增的模型级能力声明（旧部署可能缺省）。 */
export interface ModelCapabilities {
  /** 该模型是否支持思考模式、可传哪些 `reasoning_effort`（旧部署可能缺省）。 */
  reasoning?: {
    supported: boolean;
    efforts: string[];
  };
  /** 该模型接受的请求输入模态（`text` / `image` / `file`，text 始终存在）；只用于能力发现，网关不据此校验。 */
  input?: string[];
}

/** GET /.well-known/dsh-ee 的响应。 */
export interface HarnessDiscovery {
  name: string;
  version: string;
  api_version: string;
  device_authorization_endpoint: string;
  token_endpoint: string;
  scopes_supported: string[];
  models: {
    protocol: string;
    streaming: boolean;
    /** 思考模式能力声明；旧部署（未合并该能力）没有这个字段。 */
    reasoning?: HarnessReasoningCapability;
    /** 网关理解的模型输入模态取值（能力矩阵，如 `["text","image","file"]`）；旧部署可能缺省。 */
    input_modalities?: string[];
  };
  mcp: {
    transport: string;
    protocol_versions: string[];
  };
  apps: {
    web: boolean;
    data_tools: boolean;
  };
  limits: {
    max_request_bytes: number;
    max_response_bytes: number;
  };
  deprecation: {
    sunset_header: string;
  };
}

/** 中立微应用描述（GET /v1/capabilities 的 apps[]、GET /v1/apps）。 */
export interface HarnessApp {
  name: string;
  title: string;
  description?: string;
  icon?: string;
  starters: string[];
  launch: string;
  delivery: string;
  version: number;
  web: boolean;
}

/** GET /v1/capabilities 的响应；匿名请求只得到公开子集。 */
export interface HarnessCapabilities {
  api_version: string;
  authenticated: boolean;
  server: {
    name: string;
    version: string;
  };
  user: {
    username: string;
    department?: string;
  } | null;
  scopes: string[];
  models: string[];
  /** 思考模式能力声明；旧部署没有这个字段。 */
  reasoning?: HarnessReasoningCapability;
  /** 网关理解的模型输入模态取值（能力矩阵）；每个模型的实际声明见 /v1/models 的 capabilities.input。 */
  input_modalities?: string[];
  tools: {
    count: number;
  };
  apps: HarnessApp[];
  revision: string;
}

/** GET /v1/apps 的响应。 */
export interface HarnessAppsPage {
  api_version: string;
  revision: string;
  apps: HarnessApp[];
}

/** GET /v1/apps/{name} 的响应。 */
export interface HarnessAppDetail {
  api_version: string;
  app: HarnessApp;
}

// ---- 模型网关 ----

/** GET /v1/models 的一项（对外模型名，不是上游厂商模型名）。 */
export interface ModelInfo {
  id: string;
  object: string;
  owned_by: string;
  /** additive 能力声明；旧部署没有这个字段。 */
  capabilities?: ModelCapabilities;
}

/**
 * 思考模式开关。dsh-ee 原样转发给上游；DeepSeek 的格式是
 * `{"type":"enabled"}` / `{"type":"disabled"}`，其他字段（如 `budget_tokens`）也原样透传。
 */
export interface ThinkingConfig {
  type: 'enabled' | 'disabled';
  [key: string]: unknown;
}

/**
 * 思考强度（`reasoning_effort`）。
 *
 * DeepSeek 文档列出的请求值：`minimal` / `low` / `medium` / `high` / `xhigh` / `max` / `ultra`
 * （服务端映射到模型实际推理强度）。dsh-ee 网关不做白名单/映射，其它值也会原样转发，
 * 因此这里保留 string 兜底；具体某模型可用的值以 `/v1/models` 的
 * `capabilities.reasoning.efforts` 为准。
 */
export type ReasoningEffort =
  | 'minimal'
  | 'low'
  | 'medium'
  | 'high'
  | 'xhigh'
  | 'max'
  | 'ultra'
  | (string & {});

// ---- DeepSeek Chatbot 扩展（server tools / safety / title / 拒答） ----
//
// dsh-ee 的 /v1/chat/completions 是 OpenAI chat completions 的透传层：DeepSeek 的 Chatbot 能力
// 通过扩展字段声明与返回，dsh-ee 原样转发、不执行服务端工具逻辑（搜索/读链接由上游完成）。
// 这里分两组类型：
//   - Chat Completions 扩展：请求 `tools` / `safety_check` / `title`；响应
//     `message.server_tool_calls` / `message.annotations` / `message.title` / `message.refusal`，
//     流式还有 choice 级 `server_tool_event` 与最后一块的 `delta.title`。
//   - Messages API 响应块：server_tool_use / web_search_tool_result / web_fetch_tool_result /
//     citations / refusal / stop_details。DeepSeek 各协议共享这些块的语义；如果你的 Harness
//     同时接 Messages API 或想统一渲染工具过程，可以直接复用这些类型。

/** DeepSeek 服务端工具：搜索与阅读链接都由服务端执行，客户端只声明与展示，不执行调用。 */
export type DeepSeekServerToolType = 'deepseek_web_search' | 'deepseek_read_url';

/** `tools` 数组里的服务端工具声明（Chat Completions 用 `{type}`；Messages API 另带 `name`）。 */
export interface DeepSeekServerToolDeclaration {
  type: DeepSeekServerToolType | (string & {});
  /** Messages API 需要显式 name；Chat Completions 可省略。 */
  name?: string;
  [key: string]: unknown;
}

/** `safety_check`：模型内容安全审核；`high` 命中风险时返回拒答（refusal / content_filter）。 */
export interface DeepSeekSafetyCheck {
  level: 'low' | 'high';
  [key: string]: unknown;
}

/** `title`：回答结束时附带会话标题（Chat Completions 在 message.title / 最后一块 delta.title）。 */
export interface DeepSeekTitleConfig {
  enabled: boolean;
  [key: string]: unknown;
}

/** 服务端工具调用的 action：搜索 / 打开页面 / 页内查找 / 阅读链接。 */
export interface DeepSeekServerToolAction {
  type: 'search' | 'open_page' | 'find_in_page' | 'read_link' | (string & {});
  /** action=search 时的查询词。 */
  queries?: string[];
  /** action=open_page / find_in_page / read_link 时的页面地址。 */
  url?: string;
  /** action=find_in_page 时的查找文本。 */
  pattern?: string;
  [key: string]: unknown;
}

/** 服务端工具打开或阅读的页面（仅供展示；恢复页面内容靠 signature）。 */
export interface DeepSeekServerToolPage {
  url: string;
  title?: string | null;
  published_at?: string | null;
  [key: string]: unknown;
}

/** 搜索结果条目；`site_name` / `site_icon` / `snippet` 是 DeepSeek 展示扩展，回传时会被忽略。 */
export interface DeepSeekServerToolSource {
  type?: 'url' | (string & {});
  url: string;
  title?: string | null;
  snippet?: string | null;
  site_name?: string | null;
  site_icon?: string | null;
  published_at?: string | null;
  /** 图片搜索结果里原图地址（可能缺省，不保证可访问）。 */
  image_url?: string;
  [key: string]: unknown;
}

/** Chat Completions 的一次服务端工具调用（`message.server_tool_calls[].calls[]`）。 */
export interface DeepSeekServerToolCall {
  index: number;
  id: string;
  type: DeepSeekServerToolType | (string & {});
  kind: 'search' | 'deep_search' | 'read_link' | 'image_search' | 'deep_image_search' | (string & {});
  action: DeepSeekServerToolAction;
  status: 'completed' | 'failed' | (string & {});
  /** 搜索结果的展示字段。 */
  sources?: DeepSeekServerToolSource[];
  /** 打开/阅读的页面。 */
  page?: DeepSeekServerToolPage;
  /** 阅读链接时携带的加密页面；多轮必须原样回传，否则页面不会进入上下文。 */
  signature?: string;
  error?: { code: string; [key: string]: unknown };
  [key: string]: unknown;
}

/** Chat Completions 的一个工具步骤：该步的思考 + 调用列表。 */
export interface DeepSeekServerToolStep {
  /** 步骤序号，从 0 开始。 */
  index: number;
  /** 该步思考；思考未开启时省略，开启时可能是空串。 */
  reasoning_content?: string;
  calls: DeepSeekServerToolCall[];
  [key: string]: unknown;
}

/** Chat Completions 的 `annotations`：`content` 里的 URL 引用（start/end 以 Unicode 码点计）。 */
export interface DeepSeekUrlCitation {
  type: 'url_citation';
  url_citation: {
    url: string;
    title?: string | null;
    start_index: number;
    end_index: number;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

/** 流式 choice 级的 `server_tool_event`：工具进度事件；完整列表在最后一块的 delta 里。 */
export interface DeepSeekServerToolEvent {
  type: 'step_reasoning_done' | 'call_started' | 'call_completed' | (string & {});
  step: number;
  call?: DeepSeekServerToolCall;
  [key: string]: unknown;
}

/** `usage.server_tool_use`：服务端工具计数（展示与计费核对用）。 */
export interface DeepSeekServerToolUsage {
  web_search_requests?: number;
  web_fetch_requests?: number;
  tool_search?: number;
  tool_fetch?: number;
  tool_image_search?: number;
  [key: string]: unknown;
}

/** OpenAI 客户端侧 function tool 声明（dsh-ee 原样转发；服务端工具用上面的类型）。 */
export interface ChatToolDefinition {
  type: 'function';
  function: {
    name: string;
    description?: string;
    parameters?: Record<string, unknown>;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

/** Messages API 的 `server_tool_use` 块。 */
export interface DeepSeekServerToolUseBlock {
  type: 'server_tool_use';
  id: string;
  name: DeepSeekServerToolType | (string & {});
  input: DeepSeekServerToolAction;
  caller?: { type: 'direct' | (string & {}); [key: string]: unknown };
  kind: DeepSeekServerToolCall['kind'];
  [key: string]: unknown;
}

/** `web_search_tool_result` 的结果条目。 */
export interface DeepSeekWebSearchResult {
  type: 'web_search_result';
  title?: string | null;
  url: string;
  /** 原样保留；回传结果时需要。 */
  encrypted_content: string;
  page_age?: string | null;
  snippet?: string | null;
  site_name?: string | null;
  site_icon?: string | null;
  image_url?: string;
  [key: string]: unknown;
}

/** 搜索失败条目。 */
export interface DeepSeekWebSearchToolResultError {
  type: 'web_search_tool_result_error';
  error_code:
    | 'invalid_tool_input'
    | 'unavailable'
    | 'max_uses_exceeded'
    | 'too_many_requests'
    | 'query_too_long'
    | 'request_too_large'
    | (string & {});
  [key: string]: unknown;
}

/** Messages API 的 `web_search_tool_result` 块。 */
export interface DeepSeekWebSearchToolResultBlock {
  type: 'web_search_tool_result';
  tool_use_id: string;
  content: Array<DeepSeekWebSearchResult | DeepSeekWebSearchToolResultError>;
  kind: DeepSeekServerToolCall['kind'];
  [key: string]: unknown;
}

/** `web_fetch_tool_result` 的成功内容。 */
export interface DeepSeekWebFetchResult {
  type: 'web_fetch_result';
  url: string;
  content: {
    type: 'document';
    source: { type: 'text'; media_type: string; data: string; [key: string]: unknown };
    title?: string | null;
    [key: string]: unknown;
  };
  published_at?: string | null;
  [key: string]: unknown;
}

/** 阅读/打开页面失败的内容。 */
export interface DeepSeekWebFetchToolResultError {
  type: 'web_fetch_tool_result_error';
  error_code: 'url_not_accessible' | 'url_not_allowed' | 'unavailable' | (string & {});
  [key: string]: unknown;
}

/** Messages API 的 `web_fetch_tool_result` 块；阅读链接时 `signature` 携带加密页面。 */
export interface DeepSeekWebFetchToolResultBlock {
  type: 'web_fetch_tool_result';
  tool_use_id: string;
  content: DeepSeekWebFetchResult | DeepSeekWebFetchToolResultError;
  kind: DeepSeekServerToolCall['kind'];
  signature?: string;
  [key: string]: unknown;
}

/** Messages API 的 citations 条目（`web_search_result_location`）。 */
export interface DeepSeekCitation {
  type: 'web_search_result_location';
  url: string;
  title?: string | null;
  encrypted_index?: string;
  cited_text?: string;
  [key: string]: unknown;
}

/** Responses / Messages API 的拒答内容块（`type: "refusal"`）。 */
export interface DeepSeekRefusal {
  type: 'refusal';
  /** 拒答说明（例如被拦截的图片）；不含具体风控原因。 */
  refusal: string;
  [key: string]: unknown;
}

/** Messages API 的 `stop_details`：拒答时给出 type / category / explanation。 */
export interface DeepSeekStopDetails {
  type: 'refusal';
  category?: string | null;
  explanation?: string | null;
  [key: string]: unknown;
}

/** Messages API assistant content 里的服务端工具相关块。 */
export type DeepSeekServerToolContentBlock =
  | DeepSeekServerToolUseBlock
  | DeepSeekWebSearchToolResultBlock
  | DeepSeekWebFetchToolResultBlock;

// ---- DeepSeek latest_reminder（用户元信息标记） ----
//
// 在 messages 里放一条 {"role":"latest_reminder","content":"..."}，把当前日期、星期、位置、
// 平台与语言告诉模型。content 格式固定为 `<date>,<weekday>,<location>,<platform>,<language>`；
// 必须紧贴在其对应的 user 消息之前，否则上游返回 400。dsh-ee 只透传。

/** latest_reminder 的结构化字段（content 由这些字段按顺序拼成）。 */
export interface DeepSeekLatestReminder {
  /** 日期，`YYYY-MM-DD`。 */
  date: string;
  /** 该日期的星期，用 language 写（如 `Monday` / `星期一`）。 */
  weekday: string;
  /** 可选，可为空串；允许包含逗号。 */
  location?: string;
  /** 平台：`App` 或 `Web`。 */
  platform: 'App' | 'Web';
  /** 语言名，用该语言本身写（如 `English` / `中文`）。 */
  language: string;
}

/** 请求 messages 里的 latest_reminder 消息；必须紧贴其对应的 user 消息之前。 */
export interface DeepSeekLatestReminderMessage {
  role: 'latest_reminder';
  /** `<date>,<weekday>,<location>,<platform>,<language>`，例如 `2026-11-11,Wednesday,Singapore,Web,English`。 */
  content: string;
  [key: string]: unknown;
}

/**
 * 把 latest_reminder 字段拼成请求用的 content。
 *
 * location 允许包含逗号（位于第三段），所以不要用 split(',') 反解字段；需要结构化数据时保留
 * 生成它的 {@link DeepSeekLatestReminder}。
 */
export function formatLatestReminder(reminder: DeepSeekLatestReminder): string {
  return [reminder.date, reminder.weekday, reminder.location ?? '', reminder.platform, reminder.language].join(',');
}

/** 请求消息的 role：OpenAI 角色 + DeepSeek 的 latest_reminder。 */
export type ChatRole = 'system' | 'user' | 'assistant' | 'tool' | 'latest_reminder' | (string & {});

// ---- DeepSeek 文件 / 文档内容块 ----
//
// Chat Completions 用 `file` 内容块引用 Files API 上传的文件（file_id），或内联 base64（file_data）；
// Messages API 用 `document` 内容块引用文档（source.type = file / text）。
// 文件必须先通过 DeepSeek Files API（POST /files）上传并解析；dsh-ee 的 /v1 只透传 chat 请求，
// 不代理 Files API，也不做解析。

/**
 * Chat Completions 的 file 内容块：`file_id` 与 `file_data` **恰好二选一**（类型上已约束）。
 *
 * - `file_id`：引用 Files API 上传的文件（`file-api-...`）。文档（PDF/Word/PPT 等）只能这样引用。
 * - `file_data`：内联 base64 data URL（如 `data:image/jpeg;base64,...`），可带 `filename`。
 */
export type DeepSeekFileContentPart =
  | ({ type: 'file'; file_id: string; filename?: never; file_data?: never } & Record<string, unknown>)
  | ({ type: 'file'; file_data: string; filename?: string; file_id?: never } & Record<string, unknown>);

/** Messages API document 块的 source：引用已上传文件，或直接给纯文本。 */
export type DeepSeekDocumentSource =
  | { type: 'file'; file_id: string; [key: string]: unknown }
  | { type: 'text'; media_type: string; data: string; [key: string]: unknown };

/** Messages API 的 document 内容块（Chat Completions 用 file 内容块引用文件）。 */
export interface DeepSeekDocumentBlock {
  type: 'document';
  source: DeepSeekDocumentSource;
  /** 可选标题；以服务端支持为准。 */
  title?: string | null;
  [key: string]: unknown;
}

/** 用户/工具消息 content 数组里可透传的扩展内容块。 */
export type ChatContentPartUnion = ChatContentPart | DeepSeekFileContentPart | DeepSeekDocumentBlock;

/** OpenAI chat completions 消息；content 也可为多段内容数组，原样透传。 */
export interface ChatMessage {
  role: ChatRole;
  content?: string | null | ChatContentPartUnion[];
  name?: string;
  tool_call_id?: string;
  /**
   * 思考模式的历史思维链（assistant 消息）。
   *
   * 请求携带 `tools` 时，后续请求必须把历史 assistant 消息的 `reasoning_content` 原样带回，
   * 否则上游会返回 400（`The reasoning_content in the thinking mode must be passed back to the API.`）；
   * 不带 `tools` 时可以不回传（上游会忽略）。
   */
  reasoning_content?: string | null;
  // ---- DeepSeek Chatbot 响应的 assistant 消息扩展（dsh-ee 只透传，不执行工具） ----
  /** 会话标题（请求 title.enabled 时由上游生成；可能缺省，例如拒答/出错时）。 */
  title?: string | null;
  /** 拒答说明（例如被内容安全拦截的图片）；finish_reason 为 content_filter 时展示。 */
  refusal?: string | null;
  /** content 的 URL 引用（`type: "url_citation"`）。 */
  annotations?: DeepSeekUrlCitation[];
  /** 服务端工具步骤与调用；多轮要原样回传（含每个 call 的 kind / signature）。 */
  server_tool_calls?: DeepSeekServerToolStep[];
  [key: string]: unknown;
}

/** 通用内容块（`text` / `image_url` 等）；`file` / `document` 用上面的专用类型。 */
export interface ChatContentPart {
  type: string;
  text?: string;
  [key: string]: unknown;
}

/**
 * POST /v1/chat/completions 的请求体。
 *
 * dsh-ee 只实现 OpenAI chat completions 的一个子集，其余字段会原样转发给上游；
 * 因此这里保留索引签名，`temperature`、`max_tokens`、`tools` 等字段可以直接传。
 */
export interface ChatCompletionRequest {
  model: string;
  messages: ChatMessage[];
  stream?: boolean;
  /** 思考模式开关，原样转发给上游；不支持思考模式的模型/旧部署由上游决定行为。 */
  thinking?: ThinkingConfig;
  /** 思考强度，原样转发给上游；可用值以 `/v1/models` 的 capabilities.reasoning.efforts 为准。 */
  reasoning_effort?: ReasoningEffort;
  /**
   * 工具声明：OpenAI function tools 或 DeepSeek 服务端工具
   * （`{type: "deepseek_web_search"}` / `{type: "deepseek_read_url"}`），原样转发给上游。
   * 服务端工具由上游执行，dsh-ee 与 SDK 都不实现搜索/读链接逻辑。
   */
  tools?: Array<ChatToolDefinition | DeepSeekServerToolDeclaration>;
  /** 模型内容安全审核，原样转发；`high` 时命中风险返回拒答。 */
  safety_check?: DeepSeekSafetyCheck;
  /** 会话标题，原样转发；回答结束附带 title（message.title / 最后一块 delta.title）。 */
  title?: DeepSeekTitleConfig;
  [key: string]: unknown;
}

export interface ChatUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  /** 输出 token 细分：`safety_tokens` 是内容安全审核消耗（按输出 token 计费）。 */
  completion_tokens_details?: { safety_tokens?: number; [key: string]: unknown };
  /** 服务端工具计数（展示/计费核对用；不是计费金额）。 */
  server_tool_use?: DeepSeekServerToolUsage;
  [key: string]: unknown;
}

export interface ChatCompletionChoice {
  index: number;
  message: ChatMessage;
  /**
   * `stop` 正常结束、`length` 达到输出上限、`content_filter` 内容审核拒答、
   * `tool_calls` 客户端工具调用等；以服务端上游返回为准。
   */
  finish_reason?: string | null;
  logprobs?: unknown;
  [key: string]: unknown;
}

/** 非流式 chat completion 响应；dsh-ee 原样转发上游响应。 */
export interface ChatCompletion {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: ChatCompletionChoice[];
  usage?: ChatUsage;
  [key: string]: unknown;
}

export interface ChatCompletionChunkChoice {
  index: number;
  delta: {
    role?: string;
    content?: string | null;
    /**
     * 流式思维链增量；思考模式下与 `content` 同级出现，原样转发自上游。
     * 跨 chunk 拼接后作为下一轮 assistant 消息的 `reasoning_content` 回传（工具轮次必须回传）。
     */
    reasoning_content?: string | null;
    /** 会话标题：只在最后一块的 delta 出现（请求 title.enabled 时）。 */
    title?: string | null;
    /** 拒答说明：只在最后一块的 delta 出现（例如被拦截的图片）。 */
    refusal?: string | null;
    /** 服务端工具完整列表：只在带 finish_reason 的最后一块出现。 */
    server_tool_calls?: DeepSeekServerToolStep[];
    /** content 的 URL 引用：随文本/最后一块返回。 */
    annotations?: DeepSeekUrlCitation[];
    [key: string]: unknown;
  };
  /** 服务端工具进度事件（choice 级扩展字段，与 delta 同级）。 */
  server_tool_event?: DeepSeekServerToolEvent;
  finish_reason?: string | null;
  [key: string]: unknown;
}

/** 流式 chat completion 的一个 SSE data 分块；最后一块可能带 usage。 */
export interface ChatCompletionChunk {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: ChatCompletionChunkChoice[];
  usage?: ChatUsage | null;
  [key: string]: unknown;
}

// ---- 设备码 ----

/** 设备码流程可申请的 scope（与 dsh-ee 设备码端点一致）。 */
export type DshDeviceScope =
  | 'models'
  | 'registry:read'
  | 'registry:publish'
  | 'hub'
  | 'connections'
  | 'agent';

/** POST /api/device/code 的请求体。 */
export interface DeviceCodeRequest {
  /** 显示在网页确认页上的客户端名，例如 "my-harness"。 */
  clientName?: string;
  clientOs?: string;
  clientVersion?: string;
  /** 不传时 dsh-ee 使用 models, registry:read, hub, connections, agent。 */
  scopes?: DshDeviceScope[];
  /** 「开始使用」页的一次性接入码，可省略。 */
  setupCode?: string;
}

/** POST /api/device/code 的响应。 */
export interface DeviceCode {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  /** 建议轮询间隔（秒）。 */
  interval: number;
  /** 设备码有效期（秒）。 */
  expires_in: number;
}

/** 设备码轮询可能返回的错误码。 */
export type DeviceFlowErrorCode =
  | 'authorization_pending'
  | 'slow_down'
  | 'access_denied'
  | 'expired_token'
  | 'rate_limited';

/** POST /api/device/token 成功时的响应。 */
export interface DeviceToken {
  access_token: string;
  scopes: string[];
  user: {
    username: string;
    department?: string;
  };
  /** 令牌过期时间（RFC3339 字符串）。 */
  expires_at: string;
}

/** waitForDeviceToken 的进度回调。 */
export interface DeviceTokenWaitOptions {
  signal?: AbortSignal;
  /** 覆盖轮询间隔（毫秒）；默认取设备码响应里的 interval。 */
  intervalMs?: number;
  /** 本地最多等待多久（毫秒）；默认取设备码响应的 expires_in。 */
  timeoutMs?: number;
  /** 每次轮询前回调，便于打印「还在等待」或展示授权码。 */
  onPending?: (info: { code: DeviceFlowErrorCode; attempt: number }) => void;
}

// ---- MCP ----

export type JsonRpcId = number | string;

export interface JsonRpcRequest {
  jsonrpc: '2.0';
  id?: JsonRpcId;
  method: string;
  params?: unknown;
}

export interface JsonRpcErrorObject {
  code: number;
  message: string;
  data?: unknown;
}

export interface McpServerInfo {
  name: string;
  version: string;
}

/** MCP initialize 的 result；dsh-ee 会回 protocolVersion / capabilities / serverInfo / instructions。 */
export interface McpInitializeResult {
  protocolVersion: string;
  capabilities: Record<string, unknown>;
  serverInfo: McpServerInfo;
  instructions?: string;
  [key: string]: unknown;
}

/** tools/list 的一项。 */
export interface McpTool {
  name: string;
  title?: string;
  description?: string;
  inputSchema: unknown;
  annotations?: {
    readOnlyHint?: boolean;
    [key: string]: unknown;
  };
  _meta?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface McpToolsPage {
  tools: McpTool[];
  /** 不透明游标，原样传给下一次 tools/list；没有下一页时不存在。 */
  nextCursor?: string;
}

/** tools/call 结果里的内容块（目前 dsh-ee 只返回 text）。 */
export interface McpToolContent {
  type: string;
  text?: string;
  [key: string]: unknown;
}

/** tools/call 的结果（MCP CallToolResult）。 */
export interface McpToolCallResult {
  content: McpToolContent[];
  isError?: boolean;
  _meta?: Record<string, unknown>;
  [key: string]: unknown;
}

/**
 * dsh-ee 写操作需要人工确认时，结果级 `_meta["dsh-ee/confirmation"]` 的结构。
 * 拿到后展示 url 让用户确认，再用完全相同的参数重试工具调用。
 */
export interface DshConfirmation {
  id: number | string;
  url: string;
  /** 确认失效时间（RFC3339 UTC）。 */
  expires_at: string;
  /** 解析后的失效时间。 */
  expiresAt?: Date;
}

/** listTools 的选项。limit=0 表示不分页，一次取全量。 */
export interface ListToolsOptions {
  /** 第一页可显式指定页大小；服务端默认 100，上限 500。 */
  limit?: number;
  /** 从上一页拿到的 nextCursor 继续翻页。 */
  cursor?: string;
  signal?: AbortSignal;
}

/** listAllTools 的选项。 */
export interface ListAllToolsOptions {
  /** 每次请求的页大小；不传用服务端默认值。 */
  pageSize?: number;
  signal?: AbortSignal;
  /** 每拿到一页回调一次，便于边翻边用。 */
  onPage?: (page: McpToolsPage) => void;
}

/** callTool 的选项。 */
export interface CallToolOptions {
  signal?: AbortSignal;
}

/** MCP 客户端信息，initialize 时上报。 */
export interface McpClientInfo {
  name: string;
  version: string;
}
