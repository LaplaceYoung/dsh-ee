"""dsh-ee Harness 客户端的类型（TypedDict，仅便于静态检查；运行时不校验）。

覆盖模型网关的 OpenAI chat completions 子集与思考模式：

- 请求：``thinking``（如 ``{"type": "enabled"}``）与 ``reasoning_effort`` 由 dsh-ee 原样转发给上游，
  网关不做白名单/映射。
- 非流式响应：``choices[].message.reasoning_content`` 是思维链，``content`` 是最终回答。
- 请求携带 ``tools`` 时，历史 assistant 消息的 ``reasoning_content`` 必须原样回传，否则上游会返回
  400；不带 ``tools`` 时可以省略（上游忽略）。
- 具体某个模型是否支持思考模式、可用哪些 effort，见 ``GET /v1/models`` 的
  ``capabilities.reasoning``。
"""

from __future__ import annotations

from typing import Any, Dict, List, Mapping, Optional, TypedDict, Union

__all__ = [
    "ChatCompletion",
    "ChatCompletionChoice",
    "ChatCompletionChunk",
    "ChatCompletionChunkChoice",
    "ChatCompletionRequest",
    "ChatDelta",
    "ChatMessage",
    "ChatToolDefinition",
    "ChatUsage",
    "DeepSeekCitation",
    "DeepSeekDocumentBlock",
    "DeepSeekDocumentSource",
    "DeepSeekDocumentSourceFile",
    "DeepSeekDocumentSourceText",
    "DeepSeekFileContentPart",
    "DeepSeekLatestReminder",
    "DeepSeekLatestReminderMessage",
    "DeepSeekRefusal",
    "DeepSeekSafetyCheck",
    "DeepSeekServerToolAction",
    "DeepSeekServerToolCall",
    "DeepSeekServerToolDeclaration",
    "DeepSeekServerToolEvent",
    "DeepSeekServerToolPage",
    "DeepSeekServerToolSource",
    "DeepSeekServerToolStep",
    "DeepSeekServerToolType",
    "DeepSeekServerToolUsage",
    "DeepSeekServerToolUseBlock",
    "DeepSeekStopDetails",
    "DeepSeekTitleConfig",
    "DeepSeekUrlCitation",
    "DeepSeekUrlCitationInfo",
    "DeepSeekWebFetchResult",
    "DeepSeekWebFetchToolResultBlock",
    "DeepSeekWebFetchToolResultError",
    "DeepSeekWebSearchResult",
    "DeepSeekWebSearchToolResultBlock",
    "DeepSeekWebSearchToolResultError",
    "ReasoningEffort",
    "ThinkingConfig",
    "format_latest_reminder",
]

# 思考模式开关：dsh-ee 原样转发给上游。DeepSeek 的格式是 {"type": "enabled"} /
# {"type": "disabled"}，其他字段（如 budget_tokens）也会原样透传，所以用 Dict 兜底。
ThinkingConfig = Dict[str, Any]

# 思考强度（reasoning_effort）：DeepSeek 文档列出 minimal / low / medium / high / xhigh /
# max / ultra；dsh-ee 网关不做白名单，其它值也会原样转发，具体可用值以 /v1/models 的
# capabilities.reasoning.efforts 为准。
ReasoningEffort = str


# ---- DeepSeek Chatbot 扩展（server tools / safety / title / 拒答） ----
#
# dsh-ee 的 /v1/chat/completions 是 OpenAI chat completions 的透传层：DeepSeek 的 Chatbot 能力
# 通过扩展字段声明与返回，dsh-ee 原样转发、不执行服务端工具逻辑（搜索/读链接由上游完成）。
# 这里分两组类型：
#   - Chat Completions 扩展：请求 tools / safety_check / title；响应 message.server_tool_calls /
#     message.annotations / message.title / message.refusal（Python 版只实现非流式）。
#   - Messages API 响应块：server_tool_use / web_search_tool_result / web_fetch_tool_result /
#     citations / refusal / stop_details。DeepSeek 各协议共享这些块的语义，需要统一渲染或对接
#     Messages API 时可以直接复用。

# DeepSeek 服务端工具：搜索与阅读链接都由服务端执行，客户端只声明与展示，不执行调用。
DeepSeekServerToolType = str  # "deepseek_web_search" | "deepseek_read_url"


class DeepSeekServerToolDeclaration(TypedDict, total=False):
    """tools 数组里的服务端工具声明（Chat Completions 用 {type}；Messages API 另带 name）。"""

    type: DeepSeekServerToolType
    name: str


class DeepSeekSafetyCheck(TypedDict, total=False):
    """safety_check：模型内容安全审核；high 命中风险时返回拒答。"""

    level: str  # "low" | "high"


class DeepSeekTitleConfig(TypedDict, total=False):
    """title：回答结束时附带会话标题（Chat Completions 在 message.title）。"""

    enabled: bool


class DeepSeekServerToolAction(TypedDict, total=False):
    """服务端工具调用的 action：搜索 / 打开页面 / 页内查找 / 阅读链接。"""

    type: str  # "search" | "open_page" | "find_in_page" | "read_link"
    queries: List[str]
    url: str
    pattern: str


class DeepSeekServerToolPage(TypedDict, total=False):
    """服务端工具打开或阅读的页面（仅供展示；恢复页面内容靠 signature）。"""

    url: str
    title: Optional[str]
    published_at: Optional[str]


class DeepSeekServerToolSource(TypedDict, total=False):
    """搜索结果条目；site_name / site_icon / snippet 是展示扩展，回传时会被忽略。"""

    type: str
    url: str
    title: Optional[str]
    snippet: Optional[str]
    site_name: Optional[str]
    site_icon: Optional[str]
    published_at: Optional[str]
    image_url: str


class DeepSeekServerToolCall(TypedDict, total=False):
    """Chat Completions 的一次服务端工具调用（message.server_tool_calls[].calls[]）。"""

    index: int
    id: str
    type: DeepSeekServerToolType
    kind: str  # "search" | "deep_search" | "read_link" | "image_search" | "deep_image_search"
    action: DeepSeekServerToolAction
    status: str  # "completed" | "failed"
    sources: List[DeepSeekServerToolSource]
    page: DeepSeekServerToolPage
    # 阅读链接时携带的加密页面；多轮必须原样回传，否则页面不会进入上下文。
    signature: str
    error: Dict[str, Any]


class DeepSeekServerToolStep(TypedDict, total=False):
    """Chat Completions 的一个工具步骤：该步的思考 + 调用列表。"""

    index: int
    reasoning_content: str
    calls: List[DeepSeekServerToolCall]


class DeepSeekUrlCitationInfo(TypedDict, total=False):
    """annotations[].url_citation：content 里的 URL 引用（start/end 以 Unicode 码点计）。"""

    url: str
    title: Optional[str]
    start_index: int
    end_index: int


class DeepSeekUrlCitation(TypedDict, total=False):
    """Chat Completions 的 annotations 条目。"""

    type: str  # "url_citation"
    url_citation: DeepSeekUrlCitationInfo


class DeepSeekServerToolEvent(TypedDict, total=False):
    """流式 choice 级的 server_tool_event（Python 版不解析流式，保留类型给调用方）。"""

    type: str  # "step_reasoning_done" | "call_started" | "call_completed"
    step: int
    call: DeepSeekServerToolCall


class DeepSeekServerToolUsage(TypedDict, total=False):
    """usage.server_tool_use：服务端工具计数（展示与计费核对用）。"""

    web_search_requests: int
    web_fetch_requests: int
    tool_search: int
    tool_fetch: int
    tool_image_search: int


class DeepSeekServerToolUseBlock(TypedDict, total=False):
    """Messages API 的 server_tool_use 块。"""

    type: str  # "server_tool_use"
    id: str
    name: DeepSeekServerToolType
    input: DeepSeekServerToolAction
    caller: Dict[str, Any]
    kind: str


class DeepSeekWebSearchResult(TypedDict, total=False):
    """web_search_tool_result 的结果条目。"""

    type: str  # "web_search_result"
    title: Optional[str]
    url: str
    encrypted_content: str
    page_age: Optional[str]
    snippet: Optional[str]
    site_name: Optional[str]
    site_icon: Optional[str]
    image_url: str


class DeepSeekWebSearchToolResultError(TypedDict, total=False):
    """搜索失败条目。"""

    type: str  # "web_search_tool_result_error"
    error_code: str


class DeepSeekWebSearchToolResultBlock(TypedDict, total=False):
    """Messages API 的 web_search_tool_result 块。"""

    type: str  # "web_search_tool_result"
    tool_use_id: str
    content: List[Dict[str, Any]]
    kind: str


class DeepSeekWebFetchResult(TypedDict, total=False):
    """web_fetch_tool_result 的成功内容。"""

    type: str  # "web_fetch_result"
    url: str
    content: Dict[str, Any]
    published_at: Optional[str]


class DeepSeekWebFetchToolResultError(TypedDict, total=False):
    """阅读/打开页面失败的内容。"""

    type: str  # "web_fetch_tool_result_error"
    error_code: str  # "url_not_accessible" | "url_not_allowed" | "unavailable"


class DeepSeekWebFetchToolResultBlock(TypedDict, total=False):
    """Messages API 的 web_fetch_tool_result 块；阅读链接时 signature 携带加密页面。"""

    type: str  # "web_fetch_tool_result"
    tool_use_id: str
    content: Dict[str, Any]
    kind: str
    signature: str


class DeepSeekCitation(TypedDict, total=False):
    """Messages API 的 citations 条目（web_search_result_location）。"""

    type: str  # "web_search_result_location"
    url: str
    title: Optional[str]
    encrypted_index: str
    cited_text: str


class DeepSeekRefusal(TypedDict, total=False):
    """Responses / Messages API 的拒答内容块（type: refusal）。"""

    type: str  # "refusal"
    refusal: str


class DeepSeekStopDetails(TypedDict, total=False):
    """Messages API 的 stop_details：拒答时给出 type / category / explanation。"""

    type: str  # "refusal"
    category: Optional[str]
    explanation: Optional[str]


# ---- DeepSeek latest_reminder（用户元信息标记） ----
#
# 在 messages 里放一条 {"role": "latest_reminder", "content": "..."}，把当前日期、星期、位置、
# 平台与语言告诉模型。content 格式固定为 `<date>,<weekday>,<location>,<platform>,<language>`；
# 必须紧贴在其对应的 user 消息之前，否则上游返回 400。dsh-ee 只透传。


class DeepSeekLatestReminder(TypedDict, total=False):
    """latest_reminder 的结构化字段（content 由这些字段按顺序拼成）。"""

    date: str  # YYYY-MM-DD
    weekday: str  # 用 language 写，如 Monday / 星期一
    location: str  # 可选，可为空串，允许包含逗号
    platform: str  # "App" | "Web"
    language: str  # 语言名用该语言写，如 English / 中文


class DeepSeekLatestReminderMessage(TypedDict, total=False):
    """请求 messages 里的 latest_reminder 消息；必须紧贴其对应的 user 消息之前。"""

    role: str  # "latest_reminder"
    content: str  # date,weekday,location,platform,language


def format_latest_reminder(reminder: Mapping[str, Any]) -> str:
    """把 latest_reminder 字段拼成请求用的 content。

    location 允许包含逗号（位于第三段），所以不要用 ``split(",")`` 反解字段；需要结构化数据时
    保留生成它的 :class:`DeepSeekLatestReminder`。
    """
    return ",".join(
        [
            str(reminder.get("date", "") or ""),
            str(reminder.get("weekday", "") or ""),
            str(reminder.get("location", "") or ""),
            str(reminder.get("platform", "") or ""),
            str(reminder.get("language", "") or ""),
        ]
    )


# ---- DeepSeek 文件 / 文档内容块 ----
#
# Chat Completions 用 file 内容块引用 Files API 上传的文件（file_id），或内联 base64（file_data）；
# Messages API 用 document 内容块引用文档（source.type = file / text）。
# 文件必须先通过 DeepSeek Files API（POST /files）上传并解析；dsh-ee 的 /v1 只透传 chat 请求，
# 不代理 Files API，也不做解析。


class DeepSeekFileContentPart(TypedDict, total=False):
    """Chat Completions 的 file 内容块：file_id 与 file_data 恰好二选一（运行期由上游校验）。"""

    type: str  # "file"
    file_id: str  # Files API 返回的 file-api-...
    file_data: str  # 内联 base64 data URL（如 data:image/jpeg;base64,...）
    filename: str  # 仅与 file_data 一起使用


class DeepSeekDocumentSourceFile(TypedDict, total=False):
    """Messages API document 块的 source：引用已上传文件。"""

    type: str  # "file"
    file_id: str


class DeepSeekDocumentSourceText(TypedDict, total=False):
    """Messages API document 块的 source：直接给纯文本。"""

    type: str  # "text"
    media_type: str
    data: str


# document 块的 source 二选一：已上传文件（file_id）或纯文本。
DeepSeekDocumentSource = Union[DeepSeekDocumentSourceFile, DeepSeekDocumentSourceText]


class DeepSeekDocumentBlock(TypedDict, total=False):
    """Messages API 的 document 内容块（Chat Completions 用 file 内容块引用文件）。"""

    type: str  # "document"
    source: DeepSeekDocumentSource
    title: Optional[str]


class ChatToolDefinition(TypedDict, total=False):
    """OpenAI 客户端侧 function tool 声明（dsh-ee 原样转发；服务端工具用上面的类型）。"""

    type: str  # "function"
    function: Dict[str, Any]


class ChatUsage(TypedDict, total=False):
    """OpenAI usage 字段（只列常用键）。"""

    prompt_tokens: int
    completion_tokens: int
    total_tokens: int
    # 输出 token 细分：safety_tokens 是内容安全审核消耗（按输出 token 计费）。
    completion_tokens_details: Dict[str, Any]
    # 服务端工具计数（展示/计费核对用）。
    server_tool_use: DeepSeekServerToolUsage


class ChatMessage(TypedDict, total=False):
    """OpenAI chat completions 消息（content 可为字符串或多段内容数组）。"""

    role: str
    content: Optional[Union[str, List[Dict[str, Any]]]]
    name: str
    tool_call_id: str
    tool_calls: List[Dict[str, Any]]
    # 思考模式的历史思维链：请求带 tools 时必须原样回传，否则上游 400；不带 tools 可省略。
    reasoning_content: Optional[str]
    # ---- DeepSeek Chatbot 响应的 assistant 消息扩展（dsh-ee 只透传，不执行工具） ----
    # 会话标题（请求 title.enabled 时由上游生成；可能缺省）。
    title: Optional[str]
    # 拒答说明（例如被内容安全拦截的图片）；finish_reason 为 content_filter 时展示。
    refusal: Optional[str]
    # content 的 URL 引用（type=url_citation）。
    annotations: List[DeepSeekUrlCitation]
    # 服务端工具步骤与调用；多轮要原样回传（含 kind / signature）。
    server_tool_calls: List[DeepSeekServerToolStep]


class ChatDelta(TypedDict, total=False):
    """流式响应的 delta（Python 客户端当前不实现流式 chat，保留类型给调用方解析）。"""

    role: str
    content: Optional[str]
    # 思考模式的思维链增量；跨 chunk 拼接后作为下一轮 assistant 消息的 reasoning_content 回传。
    reasoning_content: Optional[str]
    # 会话标题 / 拒答说明：只在最后一块出现（请求 title.enabled / safety_check 时）。
    title: Optional[str]
    refusal: Optional[str]
    # 服务端工具完整列表：只在带 finish_reason 的最后一块出现。
    server_tool_calls: List[DeepSeekServerToolStep]
    annotations: List[DeepSeekUrlCitation]


class ChatCompletionChoice(TypedDict, total=False):
    """非流式响应的一个 choice。"""

    index: int
    message: ChatMessage
    finish_reason: Optional[str]


class ChatCompletionChunkChoice(TypedDict, total=False):
    """流式响应的一个 choice。"""

    index: int
    delta: ChatDelta
    finish_reason: Optional[str]


class ChatCompletion(TypedDict, total=False):
    """非流式 chat completion 响应；dsh-ee 原样转发上游响应体。"""

    id: str
    object: str
    created: int
    model: str
    choices: List[ChatCompletionChoice]
    usage: Optional[ChatUsage]


class ChatCompletionChunk(TypedDict, total=False):
    """流式 chat completion 的一个 SSE data 分块；最后一块可能带 usage。"""

    id: str
    object: str
    created: int
    model: str
    choices: List[ChatCompletionChunkChoice]
    usage: Optional[ChatUsage]


class ChatCompletionRequest(TypedDict, total=False):
    """POST /v1/chat/completions 的请求体（dsh-ee 只校验 model/messages，其余原样转发）。"""

    model: str
    messages: List[ChatMessage]
    stream: bool
    thinking: ThinkingConfig
    reasoning_effort: ReasoningEffort
    # OpenAI function tools 或 DeepSeek 服务端工具（{type: "deepseek_web_search"} 等），原样转发；
    # 服务端工具由上游执行，dsh-ee 与 SDK 都不实现搜索/读链接逻辑。
    tools: List[Union[ChatToolDefinition, DeepSeekServerToolDeclaration]]
    # 模型内容安全审核 / 会话标题，原样转发。
    safety_check: DeepSeekSafetyCheck
    title: DeepSeekTitleConfig
