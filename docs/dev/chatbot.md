---
slug: chatbot
category: harness
title: "Chatbot 网关选项与透传"
title_en: "Chatbot gateway options and passthrough"
summary: "DeepSeek Chatbot 的联网工具、内容安全与会话标题：网关只透传，可按部署注入 safety_check / title，能力在发现文档声明。"
summary_en: "DeepSeek Chatbot web tools, content safety and conversation titles: the gateway passes them through, can inject safety_check/title defaults, and declares the capability in discovery."
tags: [Harness, 模型网关, DeepSeek, Chatbot, 联网工具, 内容安全]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

## 这是什么

DeepSeek 的 Build a Chatbot 能力（联网工具、深度思考搜索、网页读取、模型内容安全、会话标题）都挂在 **Chat Completions 的扩展字段**上：请求里用 `tools[].type` 声明服务端工具，用顶层 `safety_check` / `title` 打开安全审核与会话标题；响应里的 `title`、`annotations`（引用）、`refusal`、`server_tool_calls`、`server_tool_event` 等块由上游生成。

dsh-ee 的模型网关（`/v1/chat/completions`）对这些字段的处理只有两点：

1. **原样透传**：请求体除 `model` 换成该线路的上游模型名外不改写；响应按上游返回的 JSON / SSE 原样转发，只旁路提取 `usage` 记账。网关不执行、也不解析任何服务端工具结果。
2. **可选注入默认值**：部署可以要求「发往 DeepSeek 上游的请求默认打开安全审核 / 会话标题」。客户端已经带了对应字段时保留客户端值。

上游对字段的解释、模型是否支持、工具计费与拒绝行为都以 DeepSeek 文档为准：

- [Build a Chatbot](https://api-docs.deepseek.com/guides/build_a_chatbot.md)
- [Web Tools](https://api-docs.deepseek.com/guides/web_tools.md)
- [Model Content Safety](https://api-docs.deepseek.com/guides/safety.md)
- [Conversation Title](https://api-docs.deepseek.com/guides/conversation_title.md)

这些能力只在 **`vendor=deepseek` 的上游线路**上生效：dsh-ee 只对这类线路注入默认值，其它上游不注入、也不保证认识这些扩展字段。

## 开关

| 环境变量 | 取值 | 默认 | 作用 |
| --- | --- | --- | --- |
| `DSH_EE_SAFETY_CHECK` | `off` / `high` | `off` | `high` 时向 DeepSeek 上游请求注入 `safety_check:{"level":"high"}`；客户端已传 `safety_check`（包括显式 `null`）时不覆盖 |
| `DSH_EE_CHAT_TITLE` | `off` / `on` | `off` | `on` 时向 DeepSeek 上游请求注入 `title:{"enabled":true}`；客户端已传 `title`（包括显式 `null`）时不覆盖 |

规则：

- 非法取值让启动失败（fail closed），避免「以为开了其实没开」。改完需要重启进程。
- 两个开关只作用于 Chat Completions（`/v1/chat/completions`）。`/v1/messages`（Anthropic Messages 兼容端点）落到 DeepSeek 上游时是原生透传：客户端自己带的 `safety_check` / `title` 等字段原样转发，但 dsh-ee 不会按这里的开关自动注入；详见 [anthropic-api.md](./anthropic-api.md)。
- 注入只影响**当次发往该条上游线路的请求**，不改客户端原始语义：模型名、额度检查、`usage` 记账都不变；故障切换到非 DeepSeek 线路时不会带上注入字段。
- 一个对外模型可能同时挂在 DeepSeek 与其它上游下，是否注入按**实际命中的那条线路**的 `vendor` 判断。
- 如果 `vendor=deepseek` 的上游其实代理到不支持这些字段的旧接口，保持默认关闭即可，避免上游因未知字段返回 `400`。
- 安全审核本身由上游执行：审核消耗的 token 由上游计入 `usage`（Chat Completions 的 `usage.completion_tokens_details.safety_tokens`），dsh-ee 按上游返回的 `usage` 记账。
- 开启 `safety_check=high` 后，客户端要处理拒绝响应；开启 `title` 后，上游可能为等标题最多多等约 5 秒，建议只在会话第一轮打开。

开启方式的等价配置（`deploy/systemd/dsh-ee.env.example` 里也有注释示例）：

```sh
# 对所有 DeepSeek 上游默认开启基础风控
DSH_EE_SAFETY_CHECK=high
# 默认让会话第一轮请求生成会话标题
DSH_EE_CHAT_TITLE=on
```

## 请求侧（透传）

下面这些字段 dsh-ee 都原样转发，不做校验、不删改。完整字段说明以 DeepSeek Web Tools 文档为准。

```json
{
  "model": "<dsh-ee 的对外模型名>",
  "thinking": {"type": "enabled"},
  "tools": [
    {"type": "deepseek_web_search"},
    {"type": "deepseek_read_url"}
  ],
  "safety_check": {"level": "high"},
  "title": {"enabled": true},
  "messages": [
    {"role": "user", "content": "Summarize https://example.com/ in one sentence."},
    {
      "role": "assistant",
      "content": "The example.com domain is reserved for use in documentation examples.",
      "server_tool_calls": [
        {
          "index": 0,
          "reasoning_content": "",
          "calls": [
            {
              "index": 0,
              "id": "srvtoolu_rl_00_…",
              "type": "deepseek_read_url",
              "kind": "read_link",
              "action": {"type": "read_link", "url": "https://example.com"},
              "status": "completed",
              "page": {"url": "https://example.com", "title": "Example Domain", "published_at": null},
              "signature": "…"
            }
          ]
        }
      ]
    }
  ]
}
```

要点：

- 服务端工具（`deepseek_web_search` / `deepseek_read_url`）由上游执行，**客户端不执行工具**；`tool_choice` 不能强制点名这两个工具。
- 多轮对话要把上一轮的 `server_tool_calls`（包括 `kind`、`signature`）原样传回；网关不会丢弃或改写它们。
- 声明这些工具、或开启 `safety_check=high` / `title` 后，上游会忽略部分采样参数（如 `temperature`、`top_p`、`n` 等，见 Build a Chatbot 的 Ignored Parameters）。这是上游行为，dsh-ee 只是透传。
- 网关照旧：`stream=true` 时补 `stream_options.include_usage`，并校验 `max_tokens` / `n` 等入口边界；这些边界不因 Chatbot 选项而改变。

## 响应侧（不改写）

非流式与流式响应都由网关逐字节转发。客户端会看到的 DeepSeek 扩展字段包括：

| 字段 / 块 | 位置 | 说明 |
| --- | --- | --- |
| `title` | 非流式 `message.title`；流式最后一块的 `delta.title` | 会话标题，可能缺失（拒绝 / 错误 / 未生成），客户端自行兜底 |
| `annotations` | `message.annotations` / 流式 `delta.annotations` | `type=url_citation` 的引用，`start_index` / `end_index` 按 Unicode 码点计数 |
| `refusal` | `message.refusal` / 流式 `delta.refusal` | 图片等内容被安全审核拦截时的说明 |
| `finish_reason=content_filter` | `choices[0].finish_reason` | 内容安全拒绝；`content` 为空、`annotations` 省略、`server_tool_calls` 保留 |
| `server_tool_calls` | `message.server_tool_calls` / 流式最后一块的 `delta.server_tool_calls` | 每步思考与调用（`calls[]`），含 `sources` / `page` / `signature` |
| `server_tool_event` | 流式 `choices[0].server_tool_event` | `step_reasoning_done` / `call_started` / `call_completed` 进度事件 |
| `usage.server_tool_use` | `usage.server_tool_use` | 联网搜索 / 网页读取等计数；不是 token，dsh-ee 不据此记账 |

dsh-ee 不把 `annotations` 转成 Markdown、不改 `server_tool_calls`、不删 `signature`；需要渲染引用、折叠思考、展示搜索卡片时由客户端自己解析。拒绝响应也不会被网关切换成统一错误：上游返回 `200` + `finish_reason=content_filter` 时，网关同样按 `200` 原样返回（这是上游协议的一部分）。

> 注：`server_tool_use` 是 Messages / Responses 协议里的块名；Chat Completions 用上面的 `server_tool_calls` / `server_tool_event`。发现文档与能力摘要把两者都列在 `chatbot.response_fields` 里，方便不同客户端识别。

## 能力声明

`GET /.well-known/dsh-ee`（匿名）与 `GET /v1/capabilities`（Bearer 可选）都带一个 `chatbot` 对象，说明本实例对 Chatbot 能力是「透传 + 可注入」，并如实报告注入开关。能力本身需要上游 DeepSeek 支持，dsh-ee 不实现服务端工具。

```json
{
  "chatbot": {
    "upstream_vendor": "deepseek",
    "passthrough": true,
    "server_tools": ["deepseek_web_search", "deepseek_read_url"],
    "safety_check": {"supported": true, "injected": false},
    "title": {"supported": true, "injected": false},
    "refusal": true,
    "response_fields": ["title", "annotations", "citations", "refusal", "server_tool_calls", "server_tool_use"],
    "note": "这些能力由上游 DeepSeek API 实现…；dsh-ee 只透传请求字段与响应块，不解析、不改写，也不保证每个上游模型都支持。"
  }
}
```

| 字段 | 含义 |
| --- | --- |
| `upstream_vendor` | 唯一会注入/解释这些字段的上游类型，固定为 `deepseek` |
| `passthrough` | 请求字段与响应扩展块原样转发，不解析、不改写 |
| `server_tools` | `tools[].type` 里可透传给上游的服务端工具名 |
| `safety_check.supported` / `injected` / `level` | 网关是否透传客户端值；本部署是否注入默认值（`DSH_EE_SAFETY_CHECK=high`）及级别 |
| `title.supported` / `injected` | 网关是否透传客户端值；本部署是否注入 `{"enabled":true}`（`DSH_EE_CHAT_TITLE=on`） |
| `refusal` | 拒绝标记原样透传 |
| `response_fields` | 原样保留的响应扩展字段 / 块名 |
| `note` | 说明依赖上游 DeepSeek，dsh-ee 只透传 |

`chatbot` 是服务端级声明，匿名也能读到，不随令牌变化；`safety_check.injected` / `title.injected` 跟启动配置走，改配置重启后才会变。机器可读的完整形状见 `GET /v1/openapi.json` 的 `Chatbot` / `ChatbotSafety` / `ChatbotTitle` schema。

## 客户端最小示例

```sh
export BASE='https://<dsh-ee>'
export TOKEN='ee_at_…'

# 联网搜索 + 网页读取 + 内容安全 + 会话标题（safety_check/title 是否真的生效取决于上游）
curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "<DeepSeek 模型名>",
    "thinking": {"type": "enabled"},
    "tools": [{"type": "deepseek_web_search"}, {"type": "deepseek_read_url"}],
    "safety_check": {"level": "high"},
    "title": {"enabled": true},
    "messages": [{"role": "user", "content": "What is new in DeepSeek V4.1?"}],
    "stream": true
  }'
```

模型名以 `/v1/models` 为准；对 DeepSeek 官方上游（`vendor=deepseek`，如 `https://api.deepseek.com`）才会在客户端没传时补 `safety_check` / `title`。

## 边界与未做

- dsh-ee 不执行服务端工具、不抓网页、不做内容审核，也不生成会话标题；这些都由上游 DeepSeek 完成，网关只转发。
- 不解析 / 不改写响应里的 `annotations`、`server_tool_calls`、`server_tool_event`、`signature`、`refusal`、`title`，因此也不做引用渲染、网页正文缓存或标题存储。
- 两个注入开关只覆盖 Chat Completions（`/v1/chat/completions`）；`/v1/messages` 的 DeepSeek 原生透传对客户端自带字段同样原样转发，但不会自动注入（见上文「开关」）。Responses API 不在 dsh-ee 网关范围；`citations` / `server_tool_use` 等字段名是 Anthropic / Responses 协议的对照名，dsh-ee 只转 Chat Completions 的等价字段。
- 文件上传、OCR、ASR、TTS、用户元数据标记（`latest_reminder`）等 Chatbot 周边能力不在本功能范围；请求字段照常透传，但没有专门文档与测试。
- 上游是否支持某能力取决于模型（DeepSeek 文档以 `deepseek-flash` 为例）；dsh-ee 无法在网关侧探测，`chatbot.note` 已说明这一点。
- 注入不会改变额度与记账：额度按上游返回的 `usage` token 计算；联网工具、安全审核、会话标题的**上游单独计费**（如搜索 / 抓取次数）由企业跟 DeepSeek 结算，dsh-ee 不汇总、不重算。

## 代码与测试

| 位置 | 内容 |
| --- | --- |
| `server/internal/config/config.go` | 两个环境变量的解析与校验（非法值启动失败） |
| `server/internal/api/gateway_chatbot.go` | 每线路请求体构造与 DeepSeek 注入、能力声明结构 |
| `server/internal/api/gateway.go` | 网关转发：响应原样写出，只旁路提取 `usage` |
| `server/internal/api/harness.go` | 发现文档与 `/v1/capabilities` 的 `chatbot` 能力声明 |
| `server/internal/api/openapi.json` | `Chatbot` / `ChatbotSafety` / `ChatbotTitle` 机器可读形状 |
| `server/internal/api/gateway_chatbot_test.go` | 注入开 / 关 / 客户端覆盖 / 非 DeepSeek / 故障切换 / 请求与响应（含流式）字节透传 |
| `server/internal/api/gateway_chatbot_internal_test.go` | 请求体构造与能力声明的单元测试 |
| `server/internal/api/harness_test.go` | 发现文档 / 能力摘要里的 chatbot 声明与注入状态 |
