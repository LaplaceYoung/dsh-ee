---
slug: anthropic-api
category: harness
title: "Anthropic Messages 兼容（POST /v1/messages）"
title_en: "Anthropic Messages compatibility (POST /v1/messages)"
summary: "Claude Code / Anthropic SDK 用企业令牌直接调 dsh-ee：x-api-key 或 Bearer 鉴权、请求与响应转换、流式 SSE 事件、错误壳与 v0 边界。"
summary_en: "Use Claude Code / the Anthropic SDK against dsh-ee with a company token: x-api-key or Bearer auth, request/response translation, streaming SSE, error envelopes and v0 limits."
tags: [模型网关, Anthropic, Claude Code, SSE, 令牌]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

# Anthropic Messages 兼容（`POST /v1/messages`）

dsh-ee 在 OpenAI 兼容网关之外，提供 Anthropic Messages 格式的模型调用入口：Claude Code、Anthropic SDK
或任何按 Anthropic Messages 协议写的客户端，都可以用 **dsh-ee 的统一接入令牌**直接调用企业已接入的模型。
请求会转成一次 OpenAI chat completions 交给现有网关，所以模型权限、账号/令牌月度额度、上游故障转移、
用量记账与审计和 OpenAI 网关完全相同。

**v0 覆盖**：`POST /v1/messages` 的非流式与流式（SSE），鉴权、短信封、错误壳。
**不做**：`count_tokens`、Batches、Files、完整 beta 头、服务端工具（`web_search` 等）执行。

## 1. 用 Claude Code 接入

把 dsh-ee 当成 Anthropic API 的 base URL，模型名用 dsh-ee 的**对外模型名**（`GET /v1/models` 返回的 `id`，
不是上游厂商名）：

```sh
export ANTHROPIC_BASE_URL='https://dsh-ee.example.com'
export ANTHROPIC_API_KEY='ee_at_…'            # 客户端发 x-api-key
# 或者：export ANTHROPIC_AUTH_TOKEN='ee_at_…'   # 客户端发 Authorization: Bearer

# 关键：Claude Code 默认的 claude-* 模型名在 dsh-ee 里不存在，必须显式指定对外模型名
export ANTHROPIC_MODEL='company-chat'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='company-chat'
export ANTHROPIC_DEFAULT_SONNET_MODEL='company-chat'
export ANTHROPIC_DEFAULT_OPUS_MODEL='company-chat'
```

> dsh-ee **不做** `claude-*` 名称映射：模型名必须落在账号/令牌的可用模型里，否则 403。
> DeepSeek 等厂商的 Anthropic 兼容层会自动把 claude 名称映射到自家模型；dsh-ee 的模型是管理员接入的
> 多厂商模型，没有唯一映射目标，因此明确要求用对外模型名。

base URL 有两种等价写法（同一个 handler，行为、鉴权、错误壳完全一致）：

| `ANTHROPIC_BASE_URL` | 客户端实际请求 | 说明 |
|---|---|---|
| `https://dsh-ee.example.com` | `POST /v1/messages` | 标准 Anthropic base URL 写法 |
| `https://dsh-ee.example.com/anthropic` | `POST /anthropic/v1/messages` | 兼容 DeepSeek 的 Anthropic base URL 习惯（`<base>/anthropic`） |

也可以直接用 curl 验证：

```sh
curl -fsS "$BASE/v1/messages" \
  -H "x-api-key: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": [{"type": "text", "text": "ping"}]}]
  }'

# 同一个端点的别名（DeepSeek 风格 base URL：ANTHROPIC_BASE_URL=$BASE/anthropic）
curl -fsS "$BASE/anthropic/v1/messages" ...  # 参数与上面完全相同
```

## 2. 鉴权与权限

- 凭证：`x-api-key: <令牌>` 或 `Authorization: Bearer <令牌>`，两者等价；同时带上时优先 `x-api-key`。
- 令牌：统一接入令牌 `ee_at_…`（也接受旧 `sk-ee-…` / `ee_npm_…`，行为同 OpenAI 网关）。
- 权限：令牌要有 `models` scope，账号要有「调用大模型」（`llm.use`）权限，模型要在账号与令牌的可用范围内。
- 额度：账号/令牌月度额度、额度过期与 OpenAI 网关共用同一份；被拒绝的请求不转发上游、不记账。
- 限流：与 `/v1/*` 共用每令牌固定窗口限流（默认每分钟 60 次，`DSH_EE_RATE_LIMIT` 调整）。
  只带 `x-api-key` 的请求也按令牌计额度，不会退回按 IP 计数；超限同样返回 Anthropic 错误壳与 `Retry-After`。
- `anthropic-version` / `anthropic-beta`：DeepSeek 原生透传路径会把这两个头**原样透传**给上游（是否支持由 DeepSeek 决定，dsh-ee 不解析）；其它 provider 的降级转换路径忽略、不报错。
- **Files API 不在 v0 范围**：带 `anthropic-beta: files-api-2025-04-14` 也不会让 dsh-ee 实现 Files；降级转换路径的 `image.source.type=file` 文件引用返回 400（图片只支持 `base64` / `url`），原生透传路径的文件/vision 块原样交给 DeepSeek（能否引用见下一条）；Files / Batches 端点返回 404。
- **`file_id` 访问拦截（共享文件默认不开放）**：DeepSeek Files 是上游账号级共享资源，员工令牌与管理员共用上游 key，所以 dsh-ee 在**转发前**扫描请求内容块里的文件引用：`file` 块的 `file_id`、`document` / `image` 的 `source.file_id`（`tool_result` 里嵌套的内容块也扫）。普通员工默认 403 `file_access_denied`（Anthropic 错误壳 `permission_error`，并带 `error.code=file_access_denied`），**不请求上游**；管理员账号（账号有任一管理权限）与 `account_type=bot` 的机器人账号放行；需要全体员工引用共享文件时显式设置 `DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE=true`（默认 `false`，fail closed）。原生透传与降级转换两条路径都拦截；只判断「有没有引用文件」，不记录 `file_id`、不做内容审计。后续新增的模型入口（例如 `/v1/responses`，当前 main 还未实现）必须复用同一套判定，见 `server/internal/api/file_access.go`。
- 服务发现：`GET /.well-known/dsh-ee` 的 `models.anthropic_messages_endpoint` 给出本端点地址，
  `GET /v1/openapi.json` 里也有 `POST /v1/messages` 的请求/响应/错误描述。

## 3. 上游转发：DeepSeek 原生透传 / 其他 provider 降级转换

dsh-ee 按对外模型的候选线路决定这次请求怎么到上游（候选顺序即 `/admin/providers` 的优先级）：

| 候选线路第一条的 `provider.vendor` | 转发方式 | dsh-ee 做什么 |
|---|---|---|
| `deepseek` | **原生透传**：`POST <provider.base>/anthropic/v1/messages` | 只做鉴权、模型权限、额度、限流、`model` 改写（对外名→上游名）；`anthropic-version` / `anthropic-beta` 原样透传；请求体其余字段（server tools、citations、safety、title、latest_reminder、file/vision 块等）原样转发（file_id 访问拦截见 §2）；成功响应 JSON/SSE **逐字节转发**；旁路解析 usage 记账（无 usage 时估算）；上游错误也按原状态/body 原样转发。 |
| 其它 | **降级转换**：Anthropic Messages → OpenAI chat completions | 见 §4「请求转换」与 §5「响应与流式」；`anthropic-version` / `anthropic-beta` 被忽略；不支持的块返回 400 或忽略。 |

- 原生透传下 dsh-ee **不校验** `max_tokens` / `temperature` 等生成参数（交给 DeepSeek），也不解析内容块；月度额度、每令牌限流、模型权限仍由 dsh-ee 强制。
- **故障转移只在同协议内**：第一条线路是 `deepseek` 时只在其它 `deepseek` 线路之间切换，否则只在其它 vendor 的转换线路之间切换，**不跨协议**；全部失败时返回最后一条线路的错误。同一个对外模型挂多条线路时，第一条（最高优先级）决定协议。
- 原生透传同样支持 `/anthropic/v1/messages` 别名；`x-api-key` / Bearer 鉴权与转换层完全一致。
- 发现文档 `models.anthropic_messages_native_vendors`（默认 `["deepseek"]`）与 `models.anthropic_messages_conversion_fallback`（`openai-chat`）公布这一策略。

## 4. 请求转换（降级转换路径：Anthropic → OpenAI chat）

| Anthropic 字段 | 处理 |
|---|---|
| `model` | 按 dsh-ee 对外模型名校验权限，转发时换成上游模型名 |
| `max_tokens` | 必填正整数，单次上限与 OpenAI 网关一致（32768）；超限 400 |
| `system` | 字符串或文本块数组合并成一条 OpenAI `system` 消息 |
| `messages[].content` | 字符串原样；内容块按下表转换 |
| `temperature` / `top_p` | 原样透传（`temperature` 范围由上游决定） |
| `top_k` | OpenAI chat 没有对应参数，忽略 |
| `stop_sequences` | 数组转成 OpenAI `stop` |
| `tools` | 客户端工具 `{name, description, input_schema}` → `{type:"function", function:{name, description, parameters}}` |
| `tool_choice` | `auto`→`auto`、`any`→`required`、`none`→`none`、`tool`→指定 function；`disable_parallel_tool_use`→`parallel_tool_calls=false` |
| `metadata.user_id` | 转成 OpenAI 的 `user`，便于上游按终端用户限流/隔离 |
| `thinking` | `enabled` / `adaptive` → OpenAI 扩展参数 `thinking:{"type":"enabled"}`；`disabled` → `thinking:{"type":"disabled"}`；`budget_tokens` 忽略 |
| `reasoning_effort` / `output_config.effort` | 转成 OpenAI `reasoning_effort`；`none` 转成关思考；`xhigh` / `max` / `ultra` 归一到 `high`（值域 `minimal/low/medium/high`） |
| `stream` | 转发流式；自动补 `stream_options.include_usage=true` 以便记账 |
| `container` / `mcp_servers` / `service_tier` / `top_k` / `cache_control` | 忽略 |

内容块：

| 块类型 | 位置 | 转换 |
|---|---|---|
| `text` | user / assistant | 文本（同一消息多个 text 块用换行合并） |
| `image` | user | `source.type=base64` / `url` → OpenAI `image_url` 内容段（上游需支持视觉） |
| `tool_use` | assistant | → OpenAI `tool_calls`（`input` 序列化成 `arguments` JSON 字符串） |
| `tool_result` | user | → OpenAI `role:"tool"` 消息（`tool_use_id` → `tool_call_id`；内容摊平成文本，图片用 `[image]` 占位） |
| `thinking` | assistant | → 该 assistant 消息的 `reasoning_content`（DeepSeek 等上游在带 tools 的续轮要求回传） |
| `redacted_thinking` | assistant | 无法还原成文本，忽略 |
| `document` 等其它块 | — | 400 `invalid_request_error`（v0 不支持文件/文档块） |

服务端工具（`type` 为 `web_search_20250305`、`bash_20250124` 等由 Anthropic 侧执行的工具）会被**忽略**
而不是整单报错——这样 Claude Code 的其它功能仍可用，但联网搜索/代码执行类工具不会真正生效。

## 5. 响应与流式（降级转换路径）

非流式响应是标准的 Anthropic `message`：`id`（`msg_…`）、`type:"message"`、`role:"assistant"`、
`model`（客户端请求的对外模型名）、`content` 块数组、`stop_reason`、`stop_sequence`、`usage`。

- `content` 顺序：`thinking`（上游返回 `reasoning_content` / `reasoning` 时）→ `text` → `tool_use`。
  thinking 块的 `signature` 是 dsh-ee 生成的不透明占位值（服务端不校验签名），客户端原样回传即可。
- `stop_reason`：有工具调用 → `tool_use`；`finish_reason=length` → `max_tokens`；`content_filter` → `refusal`；
  其它 → `end_turn`。OpenAI 无法区分命中 `stop_sequences`，因此不会返回 `stop_sequence`。
- `usage`：`output_tokens` 取上游 `completion_tokens`；`input_tokens = prompt_tokens - 缓存命中`，
  `cache_read_input_tokens` 取上游 `prompt_tokens_details.cached_tokens`（没有该字段时为 0），
  `cache_creation_input_tokens` 固定 0。**记账**仍按上游 `prompt_tokens + completion_tokens` 计入月度额度。

流式 `Content-Type: text/event-stream`，事件序列（与 Anthropic 官方一致）：

```text
event: message_start          # message 骨架，usage.input_tokens 是估算值
event: content_block_start    # 依次出现 thinking / text / tool_use 块
event: content_block_delta    # thinking_delta / text_delta / input_json_delta / signature_delta
event: content_block_stop
event: message_delta          # delta.stop_reason + usage（最终用量，以这里为准）
event: message_stop
```

- 不发送 OpenAI 的 `data: [DONE]`；连接在 `message_stop` 之前断开即表示响应不完整。
- `message_start.usage.input_tokens` 是请求体估算值（流开始时还不知道上游用量）；
  `message_delta.usage` 里的 `input_tokens` / `output_tokens` 是最终值。
- 上游没给流式 usage（例如客户端中途停止）时，dsh-ee 按请求和已转发内容估算记账，避免额度漏记。

## 6. 错误

错误体是 Anthropic 形状，HTTP 状态码与 `error.type` 对应：

```json
{"type":"error","error":{"type":"rate_limit_error","message":"请求过于频繁，请稍后重试"}}
```

| HTTP | `error.type` | 触发 |
|---|---|---|
| 400 | `invalid_request_error` | 请求体不是 JSON、缺 `model` / `max_tokens`、参数越界、不支持的块类型 |
| 401 | `authentication_error` | 缺少或无效的令牌 |
| 403 | `permission_error` | 令牌缺 `models` scope、账号缺 `llm.use`、模型不在范围、额度过期 |
| 403 + `error.code=file_access_denied` | `permission_error` | 请求引用了上游共享文件（`file_id` / `source.file_id`），当前账号不是管理员/机器人且未打开 `DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE`（见 §2） |
| 404 | `not_found_error` | 对外模型没有可用上游；`count_tokens` 等未实现的兼容端点 |
| 413 | `request_too_large` | 请求体超过 16 MiB |
| 429 | `rate_limit_error` | 账号/令牌月度额度用完、每令牌请求限流、上游返回 429 |
| 5xx | `api_error` / `overloaded_error` | 所有上游线路不可用、上游 5xx、上游响应无法解析 |

`/v1/messages/count_tokens` 会先鉴权，再返回 404 `not_found_error`（v0 不实现），
而不是落到 `/v1/` 兜底的 OpenAI 错误壳。

DeepSeek 原生透传路径下，上游返回的错误（DeepSeek 本身就是 Anthropic 错误壳）会按**原状态码与 body**
转发，不重新包装；dsh-ee 自己产生的错误（鉴权/权限/额度/限流/模型没有上游/上游不可达）仍用上表。

## 7. 已知限制（v0）

- 只实现 `POST /v1/messages`（及别名 `POST /anthropic/v1/messages`）；`count_tokens`（含别名）、
  Batches、Files、`/v1/messages/batches` 等返回 404。
- 服务端工具不执行（见 §3）；`container`、`mcp_servers`、`service_tier`、`top_k` 被忽略。
- 不做 `claude-*` 模型名映射：客户端必须把模型名配成 `/v1/models` 里的对外模型名。
- thinking 内容通过上游的 `reasoning_content` / `reasoning` 字段识别；上游不返回思考内容时，
  响应里不会有 `thinking` 块。`budget_tokens` 不生效，思考强度用 `reasoning_effort` / `output_config.effort` 控制。
- 图片会转成 OpenAI 的 `image_url` 内容段，上游模型是否支持图片由上游决定；`document` 等文件块不支持。
- 上游共享文件（`file_id`）默认只对管理员与 `account_type=bot` 的机器人账号开放，普通员工 403；这是共享文件访问的 hotfix，完整 Files 代理（上传 / 列举 / 删除）不在本仓库 main 上。
- `stop_sequences` 命中与普通结束在上游都是 `finish_reason=stop`，无法区分，统一返回 `end_turn`。
- 签名不校验：thinking 块的 `signature` 是占位值，服务端不做验证也不依赖它。
- DeepSeek 原生透传路径不在 dsh-ee 做生成参数校验与内容块解析，`max_tokens` 等上限由 DeepSeek 决定；
  上游错误按原样转发；原生路径的响应不经过 dsh-ee 的 Anthropic 转换层。
- 故障转移不跨协议：高优先级线路决定协议后，低优先级的另一种协议线路不会被尝试。

## 8. 相关

- OpenAI 兼容网关、令牌与 scope：[Harness 接入](./harness.md)
- 第三方客户端完整手册与错误表：[l1-client.md](../l1-client.md)
- 模型网关错误码索引：[errors.md](../errors.md)
- 实现：`server/internal/api/anthropic.go`、`server/internal/api/anthropic_convert.go`、`server/internal/api/anthropic_native.go`、`server/internal/api/file_access.go`
