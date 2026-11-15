# Changelog

本包遵循语义化版本；`0.x` 期间不保证 API 完全稳定，破坏性变更会在 CHANGELOG 标注。
服务端契约版本见 [COMPATIBILITY.md](./COMPATIBILITY.md)。

## [Unreleased]

### Added

- 思考模式类型：`ChatMessage.reasoning_content`、`ChatCompletionRequest.thinking` /
  `reasoning_effort`（`ThinkingConfig` / `ReasoningEffort`）、流式 `delta.reasoning_content`；
  说明请求带 `tools` 时历史 `reasoning_content` 必须回传，否则上游 400。
- 能力类型：`HarnessReasoningCapability`（发现文档 `models.reasoning`、`/v1/capabilities` 的
  `reasoning`）与 `ModelInfo.capabilities`（`/v1/models` 每项的 `capabilities.reasoning`，
  可选、旧部署没有），SDK 对缺失字段保持兼容。
- 新增思考模式 mock 用例（请求透传、非流式/流式 `reasoning_content`）。
- DeepSeek Chatbot 类型：服务端工具声明（`deepseek_web_search` / `deepseek_read_url`）、
  `DeepSeekSafetyCheck`、`DeepSeekTitleConfig`；响应 `server_tool_calls` 步骤/调用
  （`DeepSeekServerToolStep` / `DeepSeekServerToolCall`，含 `kind` / `signature` / `sources` /
  `page`）、`annotations`（`DeepSeekUrlCitation`）、`message.title`、`message.refusal`、流式
  `server_tool_event` 与 `delta.title`/`delta.refusal`；`usage.server_tool_use` /
  `completion_tokens_details.safety_tokens`。Messages API 的 `server_tool_use` /
  `web_search_tool_result` / `web_fetch_tool_result` / citations / `refusal` / `stop_details`
  也提供对应类型（统一渲染用）。
- 新增 `sdk/examples/chatbot-server-tools.md` 与 mock 用例（非流式/流式解析、多轮回传、拒答）。
- `latest_reminder`：`DeepSeekLatestReminder` / `DeepSeekLatestReminderMessage` / `ChatRole` 与
  `formatLatestReminder()` helper；说明消息必须紧贴 user 消息、location 可含逗号。
- 文件/文档内容块：`DeepSeekFileContentPart`（`file_id` 与 `file_data` 二选一，类型上约束）、
  `DeepSeekDocumentBlock` / `DeepSeekDocumentSource`（Messages API 形状）；`ChatMessage.content`
  支持这些扩展块。说明文件需先用 DeepSeek Files API 上传，dsh-ee/SDK 只透传、不代理 `/files`。
- `npm run typecheck` 增加 `tsconfig.test.json`，编译检查 `test/types-example.ts` 中的类型示例；
  新增 `test/files-reminder.test.mjs`（格式化、多轮透传、file_id/file_data/document source）。
- 模型输入模态类型：`ModelInfo.capabilities.input`（`text` / `image` / `file`）、发现文档
  `models.input_modalities` 与 `/v1/capabilities` 的 `input_modalities`（均 optional，兼容旧部署）。

## [0.1.0] - 2026-11-13

首次加入本仓库（**未发布到 npm**）。

### Added

- `DshHarnessClient`：`baseUrl` / `token` / 可注入 `fetch` / 超时 / 默认头 / `onResponse` 观测钩子。
- 发现与能力：`discovery()`（ETag 缓存 + `If-None-Match` 304）、`capabilities()`、`listApps()`、`getApp()`。
- 模型网关：`listModels()`、`createChatCompletion()`（非流式）、`streamChatCompletion()`（SSE，跨 chunk/CRLF）。
- 设备码：`requestDeviceCode()`、`pollDeviceToken()`、`waitForDeviceToken()`（`slow_down` 自动退避、
  本地等待预算、AbortSignal）、`loginWithDeviceCode()`（自动保存令牌）。
- MCP：`connectMcp()`（initialize + `notifications/initialized`、`Mcp-Session-Id`、
  `MCP-Protocol-Version`）、`ping()`、`listTools()`（`limit`/`cursor`）、`listAllTools()`、
  `callTool()`、`close()`（DELETE `/mcp`）、SSE 响应解析。
- 写确认：`extractConfirmation()` / `isPendingConfirmation()` 解析
  `_meta["dsh-ee/confirmation"]`。
- 错误：`DshApiError` / `DshJsonRpcError` / `DshNetworkError` / `DeviceFlowError`，
  统一携带 `status`、`requestId`、`apiVersion`、`rateLimit`、`retryAfter`、`isRateLimited`。
- 限流头解析：`parseRateLimitHeaders()`、`parseRetryAfter()`；客户端记录
  `lastRequestId` / `lastRateLimit` / `lastApiVersion`。
- 测试：33 个 `node:test` 用例，全部 mock `fetch`；可选 `sdk/examples/generic-harness.mjs` 示例。
