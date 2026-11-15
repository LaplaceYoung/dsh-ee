---
slug: beta-completions
category: harness
title: "DeepSeek FIM / 前缀续写 beta 透传"
title_en: "DeepSeek FIM / chat-prefix beta passthrough"
summary: "dsh-ee 把 DeepSeek 的两个 beta 能力（FIM 补全、Chat Prefix 续写）挂到 /beta/* 透传：复用同一个员工令牌、模型权限、额度与限流，仅转发给一个 vendor=deepseek 的上游。"
summary_en: "dsh-ee exposes DeepSeek's two beta endpoints (FIM completion and chat-prefix completion) under /beta/*: same employee token, model permission, quota and rate limit, forwarded to one vendor=deepseek upstream only."
tags: [Harness, 模型网关, DeepSeek, FIM, 流式]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

## 这是什么

DeepSeek 有两个还标着 beta 的能力，接口挂在 `{base_url}/beta` 下：

| 能力 | dsh-ee 入口 | 上游 | 典型用途 |
| --- | --- | --- | --- |
| FIM 补全（Fill In the Middle） | `POST /beta/completions` | `{DeepSeek base}/beta/completions` | 编辑器 / IDE 的代码补全：给一段前缀（可选后缀），补中间内容 |
| Chat Prefix 续写 | `POST /beta/chat/completions` | `{DeepSeek base}/beta/chat/completions` | 强制模型从一段 assistant 前缀接着写（例如先把代码围栏的开头作为前缀） |

dsh-ee 只做**透传与治理**，不做多厂商语义翻译：一条请求只会转发给一个 **`vendor=deepseek` 的启用上游**，没有等价能力时不会调用其他厂商的上游。两个接口共用 `/v1` 网关的员工令牌、模型权限、额度、限流与调用审计。

> beta 边界：DeepSeek 侧的行为、参数和最大输出长度可能变化，dsh-ee 默认提供这两个入口但**不承诺 SLA**；升级说明以 DeepSeek 官方文档为准。这两个入口也不在第三方 Harness 的中立 `/v1` 契约里。

## 前置条件

1. 管理员在「系统设置 → 模型接入」里接入一个 **vendor 选 `deepseek`** 的启用上游（base_url 填 API 根，例如 `https://api.deepseek.com`；照着 OpenAI SDK 的 beta base_url 填成 `https://api.deepseek.com/beta` 也可以，dsh-ee 会去掉重复的 `/beta`）。
2. 在这个上游下配置模型：员工调用时写**对外名称**，转发时换成**上游模型名**。只配置在别的厂商上游上的对外模型不会走 `/beta/*`。
3. 员工账号有「调用大模型」权限、月度额度，并创建一个带 `models` scope 的 `ee_at_…` 令牌（「我的令牌」页）。
4. 可选：`DSH_EE_DEEPSEEK_PROVIDER=<上游 name 或 id>` 指定用哪个 DeepSeek 上游；不配时取**优先级最高**的启用 `vendor=deepseek` 上游。

## FIM：`POST /beta/completions`

请求体是 OpenAI `text_completion` 的子集，常见字段：

| 字段 | 说明 |
| --- | --- |
| `model` | **必填**。dsh-ee 的对外模型名 |
| `prompt` | **必填**。前缀文本 |
| `suffix` | 可选。后缀文本，模型补中间内容 |
| `max_tokens` | 可选。DeepSeek 文档目前限制 FIM 单次输出为 4K；dsh-ee 只按统一规则校验（1 到 32768），真正的上限以上游为准，超限会原样返回上游的 400 |
| `stream` | 可选。`true` 时按 SSE 逐字节转发 |
| `stream_options` / `stop` / `temperature` / `top_p` / `echo` / `logprobs` | 原样转发，语义以 DeepSeek 文档为准 |

响应是 `object=text_completion`，补出的内容在 `choices[].text`；`usage` 与 `/v1` 一样记入「模型调用」与月度额度。

```bash
curl -fsS "$BASE/beta/completions" \
  -H "Authorization: Bearer $EE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "deepseek-flash",
    "prompt": "def fib(a):",
    "suffix": "    return fib(a-1) + fib(a-2)",
    "max_tokens": 128
  }'
```

流式响应的分块形如 `data: {"choices":[{"text":"..."}]}`，最后一个内容分块会带 `usage`（DeepSeek 的 beta 协议如此），客户端照常处理 `data: [DONE]` 即可。

## Chat Prefix：`POST /beta/chat/completions`

和 `/v1/chat/completions` 相同的请求体，但有两条硬性要求（文档也这么规定）：

1. `messages` 的**最后一条**必须是 `assistant`；
2. 这条消息必须带 `"prefix": true`。

dsh-ee 在转发前校验这两条，不满足时直接返回 400（`prefix_required` / `messages_required`），不会把请求打到上游。

```bash
curl -fsS "$BASE/beta/chat/completions" \
  -H "Authorization: Bearer $EE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "deepseek-flash",
    "messages": [
      {"role": "user", "content": "请写一段快速排序代码"},
      {"role": "assistant", "content": "```python\n", "prefix": true}
    ],
    "stop": ["```"]
  }'
```

流式时 dsh-ee 会给上游补上 `stream_options.include_usage=true`（和 `/v1` 一致），以便把最后一个分块里的 `usage` 记进额度。

## 治理与边界

| 能力 | 行为 |
| --- | --- |
| 鉴权 | 与 `/v1` 相同：`Authorization: Bearer ee_at_…`，需要 `models` scope + 账号「调用大模型」权限 |
| 模型权限 | 先校验账号额度与令牌的可用模型（`model_not_allowed`），再按对外名解析上游模型 |
| 额度 | 与 `/v1` 共用账号 / 令牌的月度 token 额度；超限 429 `monthly_quota_exceeded` |
| 限流 | 与 `/v1`、`/mcp` 共用 `DSH_EE_RATE_LIMIT` 的每令牌限流（429 + `Retry-After` / `X-RateLimit-*`） |
| 用量记录 | 与 `/v1` 同一个 `usage_logs`，`model` 记对外名、`provider` 记实际使用的 DeepSeek 上游，按次记状态码与耗时 |
| 请求体上限 | 16 MiB（与 `/v1` 相同），超限 413 `request_too_large` |
| 上游选择 | 只选一个 `vendor=deepseek` 的启用上游；**不跨上游 / 跨厂商故障转移** |
| 上游错误 | 上游返回的 OpenAI JSON 错误原样转发；非 JSON 错误包成 OpenAI 错误壳（`upstream_error`） |
| usage 兜底 | 上游没给 `usage` 时（少见）按请求体与 `choices[].text` / `message.content` 粗估记账，只用于额度统计，不影响响应内容 |
| CORS | 不对浏览器开放跨域（`DSH_EE_CORS_ORIGINS` 只作用于 `/v1/*` 与 `/mcp`）；适合服务端 / 本机客户端调用 |

错误码都是 OpenAI 错误壳（`{"error":{"message","type","code"}}`）：

| 状态码 | code | 含义 |
| --- | --- | --- |
| 400 | `model_required` / `invalid_json` / `prompt_required` / `invalid_prompt` / `invalid_suffix` / `messages_required` / `messages_invalid` / `prefix_required` / `invalid_max_tokens` | 请求体或 beta 请求形状不合法 |
| 401 | `token_invalid` / `token_expired` / `token_revoked` | 令牌缺失、无效、过期或已吊销 |
| 403 | `permission_denied` / `model_not_allowed` / `quota_expired` | 账号没有模型权限、模型不在允许列表、额度已过期 |
| 404 | `model_not_found` | 对外模型没有配在当前选中的 DeepSeek 上游上 |
| 413 | `request_too_large` | 请求体超过 16 MiB |
| 429 | `monthly_quota_exceeded` / `rate_limit_exceeded` | 月度额度用完 / 触发限流 |
| 502 | `upstream_unreachable` | 连不上 DeepSeek 上游 |
| 503 | `beta_unavailable` | 没有启用的 `vendor=deepseek` 上游，或 `DSH_EE_DEEPSEEK_PROVIDER` 没有匹配到可用上游 |
| 上游状态码 | 原样 | 上游 4xx/5xx 的 OpenAI 错误体直接转发 |

## 不做什么

- 不承诺 beta 能力的兼容性或 SLA；上游参数、最大输出长度、流式分块形状变化时以 DeepSeek 文档为准。
- 不把 FIM / Chat Prefix 翻译给其他厂商的上游，也不做「FIM → chat」这类语义转换。
- 不在 `/beta/*` 上做多上游故障转移：配了多个 DeepSeek 上游时只会用选中的那一个，避免把 beta 请求发到行为不一致的线路上。
- 不新增模型、额度和权限概念：员工仍是同一套令牌、同一份额度、同一条调用记录。

## 相关文档

- 第三方 Harness / 自研客户端接入模型网关：[非 DSH 客户端接入 L1](../l1-client.md)
- 错误与常见问题（网关部分）：[错误与常见问题](../errors.md)
- 五分钟接入一个令牌：[接第三方 Harness / 自研客户端](./harness-quickstart.md)
