# Changelog

本包遵循语义化版本；`0.x` 期间不保证 API 完全稳定，破坏性变更会在 CHANGELOG 标注。
服务端契约版本见 [COMPATIBILITY.md](./COMPATIBILITY.md)。

## [Unreleased]

### Added

- 思考模式类型（`dsh_ee_harness/types.py`）：`ChatMessage.reasoning_content`、
  `ChatCompletionRequest.thinking` / `reasoning_effort`（`ThinkingConfig` / `ReasoningEffort`）、
  `ChatDelta.reasoning_content`、`ChatCompletion` / `ChatCompletionChunk` 等 TypedDict。
- `chat_completion()` 增加显式关键字参数 `thinking` / `reasoning_effort`（此前只能走 `**params`），
  文档说明请求带 `tools` 时历史 `reasoning_content` 必须回传。
- 新增思考模式请求体与 `reasoning_content` 透传用例。
- DeepSeek Chatbot 类型：服务端工具声明（`deepseek_web_search` / `deepseek_read_url`）、
  `DeepSeekSafetyCheck` / `DeepSeekTitleConfig`；响应 `server_tool_calls` 步骤与调用
  （`DeepSeekServerToolStep` / `DeepSeekServerToolCall`，含 `kind` / `signature` / `sources` /
  `page`）、`annotations` / `url_citation`、`message.title` / `message.refusal`；
  `usage.server_tool_use` / `completion_tokens_details.safety_tokens`。Messages API 的
  `server_tool_use` / `web_search_tool_result` / `web_fetch_tool_result` / citations /
  `refusal` / `stop_details` 也提供对应 TypedDict。
- `chat_completion()` 增加显式关键字参数 `tools` / `safety_check` / `title`（此前只能走 `**params`）。
- 新增 Chatbot 请求体、响应解析与拒答用例；新增 `sdk/examples/chatbot-server-tools.md`。
- `latest_reminder` 类型与 helper：`DeepSeekLatestReminder` / `DeepSeekLatestReminderMessage` /
  `format_latest_reminder()`；说明消息必须紧贴 user 消息、location 可含逗号。
- 文件/文档内容块类型：`DeepSeekFileContentPart`（`file_id` / `file_data`）、
  `DeepSeekDocumentBlock` / `DeepSeekDocumentSource`（`source.type = file / text`；
  Messages API 形状）。说明文件需先用 DeepSeek Files API 上传，dsh-ee/SDK 只透传、不代理 `/files`。
- 新增 latest_reminder 格式化与 file/document 内容块透传用例。

## [0.1.0] - 2026-11-13

首次加入本仓库（**未发布到 PyPI**）。

### Added

- `DshClient`：`base_url` / `token` / 可注入 `transport` / 默认头 / 超时；记录
  `last_request_id` / `last_rate_limit` / `last_api_version`。
- 发现与能力：`discovery(refresh=False)`（ETag 缓存 + `If-None-Match` 304）、`capabilities()`。
- 模型网关：`list_models()`、`chat_completion(model, messages, **params)`（非流式）。
- 设备码：`request_device_code()`、`poll_device_token()`、`wait_for_device_token()`
  （`slow_down` 自动退避、本地等待预算）、`login_with_device_code()`（自动保存令牌）。
- MCP：`connect_mcp()`（initialize、`Mcp-Session-Id`、`MCP-Protocol-Version`）、`ping()`、
  `list_tools()`（`limit`/`cursor`）、`list_all_tools()`、`call_tool()`、`close()`（DELETE `/mcp`）。
- 写确认：`extract_confirmation()` / `is_pending_confirmation()` 解析
  `_meta["dsh-ee/confirmation"]`。
- 错误：`DshClientError` / `DshJsonRpcError` / `DshNetworkError` / `DeviceFlowError`，
  统一携带 `status`、`request_id`、`api_version`、`rate_limit`、`retry_after`、`is_rate_limited`。
- 限流头解析：`parse_rate_limit_headers()`、`parse_retry_after()`。
- 测试：19 个 `unittest` 用例，全部使用假 transport。
