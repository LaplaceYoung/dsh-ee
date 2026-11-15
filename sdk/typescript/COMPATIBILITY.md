# 兼容性说明

本文件说明 `@dsh-ee/harness-client` 与 dsh-ee 服务端、MCP 协议和工具面的兼容边界。
服务端契约变更会先反映在 dsh-ee 仓库的 PR 与本文件，再随 SDK 版本发布。

## 1. SDK 与 dsh-ee API 版本

| 项 | 值 |
|---|---|
| SDK 版本 | `0.1.0` |
| dsh-ee API 主版本 | `api_version = 1` |
| 请求头 | `Dsh-EE-API-Version: 1`（`/v1/*` 与 `/.well-known/dsh-ee`） |
| 响应头 | `Dsh-EE-API-Version`、`X-Request-Id`、`X-RateLimit-*`、`Retry-After` |
| 版本协商 | 服务端只接受主版本 `1`；不支持时返回 `400` + `code: "unsupported_api_version"` + `supported_versions` |

SDK 按主版本 1 实现。服务端新增主版本时，SDK 会以新版本发布，不静默猜测。

## 2. 服务端能力依赖（在途 PR）

本仓库当前基线（`20aafbb`）上，下列能力由在途服务端 PR 提供；**旧部署上对应功能不可用**，
SDK 会按服务端实际响应退化，不影响基础端点：

| 客户端能力 | 服务端来源 | 未合并时的表现 |
|---|---|---|
| `discovery()` / `capabilities()` / `/v1/apps` / `Dsh-EE-API-Version` | #123 | `404`（SDK 抛 `DshApiError`） |
| MCP `tools/list` cursor 分页、写确认 `_meta` | #128 | 一次返回全量、无 `nextCursor`；确认信息只在文本里 |
| `/v1/*`、`/mcp` 的 `X-RateLimit-*` / `Retry-After` / `X-Request-Id` | #129 | 无这些响应头；SDK 不自动重试 |
| 令牌 `client_kind`（`dsh` / `third_party`） | #132 | 旧令牌按 `dsh` 语义；第三方写确认建议重新签发 |
| 思考模式能力声明（`/v1/models` 的 `capabilities.reasoning`、发现文档 `models.reasoning`） | 本仓库思考模式 PR | `/v1/models` 无 `capabilities` 字段、发现文档无 `models.reasoning`；SDK 类型可选，不报错；`thinking`/`reasoning_effort` 一直原样透传 |

`e2e/tests/35-harness-sdk.mjs` 对这些能力做了 mock/live 分层：mock 层不依赖服务端 PR；
live 层在探测到端点/响应头后才断言，未合并时打印跳过原因。

## 3. 模型面（OpenAI chat completions 子集）

- `GET /v1/models`：`{object:"list", data:[{id, object, owned_by, capabilities?}]}`。
  `capabilities.reasoning = {supported, efforts}` 是 additive 字段（旧部署可能没有）：
  `supported` 表示该模型支持思考模式，`efforts` 是可用 `reasoning_effort` 值（空表示按上游默认）。
- `POST /v1/chat/completions`：请求体按 OpenAI chat completions 形状转发；非流式返回上游 JSON，
  `stream: true` 时返回 SSE（`data: {...}`、最后 `[DONE]`），服务端强制
  `stream_options.include_usage = true`。
- 错误壳：OpenAI `{"error":{"message","type","code"}}`；HTTP 429 带 `Retry-After` 与
  `X-RateLimit-*`（#129 合并后）。
- 思考模式（DeepSeek 等上游）：请求里的 `thinking`（如 `{"type":"enabled"}`）与
  `reasoning_effort` 由网关原样转发，不做白名单/映射；非流式 `choices[].message.reasoning_content`
  与流式 `choices[].delta.reasoning_content` 原样返回。
- 请求携带 `tools` 时，历史 assistant 消息的 `reasoning_content` **必须原样回传**，否则上游会
  返回 400；不带 `tools` 时可省略（上游忽略）。这是客户端责任：dsh-ee 网关照转不校验。
- DeepSeek Chatbot 扩展同样是透传：请求 `tools` 里的服务端工具（`deepseek_web_search` /
  `deepseek_read_url`）、`safety_check`、`title` 原样转发；响应 `message.server_tool_calls` /
  `message.annotations` / `message.title` / `message.refusal`、流式 `server_tool_event` /
  `delta.title` 原样返回。dsh-ee 不解析、不改写、不执行这些工具（搜索/读链接由上游完成）。
- 服务端工具轮次的多轮请求要把历史 assistant 消息的 `server_tool_calls` 原样回传，尤其 `kind` 与
  `signature`（阅读链接的页面内容加密在 `signature` 里）；丢失 `signature` 请求仍成功但页面不进上下文。
- 内容安全拒答：`finish_reason: "content_filter"` + `message.refusal` / 流式 `delta.refusal`；
  Messages API 形状为 `stop_reason: "refusal"` + `stop_details`（`DeepSeekStopDetails`）。
- 启用 Chatbot 能力后，上游会忽略部分采样参数（`temperature` / `top_p` / `frequency_penalty` /
  `presence_penalty` / `stop` / `seed` / `n` 等），`max_tokens` 仍生效；dsh-ee 不改写请求。
- `latest_reminder`：`messages` 里的 `{"role":"latest_reminder","content":"<date>,<weekday>,<location>,<platform>,<language>"}`
  （`DeepSeekLatestReminder` / `formatLatestReminder()`），必须紧贴其对应的 `user` 消息之前；位置/格式
  错误由上游 400，content 非字符串为 422。历史轮次可原样带回（提高 cache 命中，省略也允许）。
  dsh-ee/SDK 只透传，不校验时间或语言取值。
- `file` 内容块：引用 DeepSeek Files API 的 `file_id`，或内联 `file_data`（base64 data URL，可带
  `filename`），二者恰好二选一（类型上已约束）。**文件必须先通过 DeepSeek Files API 上传/解析；
  dsh-ee 的 `/v1` 只透传 chat 请求，不代理 `/files`，本 SDK 也不实现 Files API。**
  文档（PDF/Word/PPT 等）只能用 `file_id` 引用；Messages API 形状的 `document` 块
  （`source.type = file / text`）也提供类型（SDK 不调用 Messages API）。
- Chatbot 相关限制与错误由上游决定：单请求最多 20 个文档/文本 ≤8 MiB、文件上传大小与解析时限等
  见 DeepSeek 文档；dsh-ee 原样返回上游错误。
- 模型输入模态：`GET /v1/models` 每项新增 additive `capabilities.input`（`text` / `image` /
  `file`，`text` 始终存在）；发现文档 `models.input_modalities` 与 `/v1/capabilities` 的
  `input_modalities` 公布网关理解的取值（能力矩阵）。旧部署可能没有这些字段，SDK 类型为可选；
  网关不按声明校验或转换请求内容。
- **不支持** Anthropic Messages、OpenAI Responses 等其它协议；也不兼容 Claude Code / Codex
  的专有请求格式。Claude Code / Codex 只能作为 MCP 客户端使用 `/mcp` 工具面，见
  [`../examples/`](../examples/)。

## 4. MCP 面

| 项 | 值 |
|---|---|
| 端点 | `POST /mcp`（JSON-RPC 2.0，streamable HTTP） |
| 传输 | `Accept: application/json, text/event-stream`；POST 响应支持 JSON 或 SSE |
| SDK 默认协议版本 | `2025-06-18`（可用 `mcpProtocolVersion` 覆盖） |
| 服务端支持版本 | `2025-11-25`、`2025-06-18`、`2025-03-26`（以发现文档 `mcp.protocol_versions` 为准） |
| 会话 | initialize 响应头 `Mcp-Session-Id`，后续请求回带；`MCP-Protocol-Version` 随协商结果发送 |
| `tools/list` 分页 | 默认 100/页、单页上限 500、`limit: 0` 取全量、`nextCursor` 不透明（#128 合并后） |
| 写确认 | 需要人工确认时结果级 `_meta["dsh-ee/confirmation"] = {id,url,expires_at}`；`content`/`isError` 旧兼容（#128 合并后） |
| 限流 | 429 + JSON-RPC `-32000`，另带 `Retry-After` / `X-RateLimit-*`（#129 合并后） |

SDK 只实现客户端常用方法：`initialize`、`notifications/initialized`、`ping`、`tools/list`、
`tools/call`、`DELETE /mcp`。不实现 MCP 的 GET 事件流（持久 `notifications/tools/list_changed`）、
resources/prompts、sampling/roots。

## 5. 运行环境

| 项 | 要求 |
|---|---|
| Node.js | `>= 18`（全局 `fetch` / `ReadableStream` / `TextDecoder`） |
| 浏览器 | 支持 `fetch` 与 Fetch Streams；需要服务端配置 CORS 白名单（#127，`DSH_EE_CORS_ORIGINS`） |
| 打包格式 | ESM（`type: "module"`，`exports` 指向 `dist/index.js`，类型 `dist/index.d.ts`） |
| 运行时依赖 | 无 |

## 6. 兼容性策略

- SDK 对 dsh-ee 的 API 主版本做显式校验；服务端返回 `400 unsupported_api_version` 时原样暴露。
- SDK 新增能力优先做成可选/可探测（例如 rate limit 头缺失不报错），避免旧部署直接失败。
- 写操作/工具调用**不自动重试**；是否重试由调用方按幂等性决定，429 的 `Retry-After` 由调用方使用。
- 破坏性变更（例如强制新的 API 主版本）会升 SDK major 版本并在 CHANGELOG 标注。
