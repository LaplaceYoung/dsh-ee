---
slug: thinking-mode
category: harness
title: "思考模式：thinking / reasoning_effort / reasoning_content"
title_en: "Thinking mode: thinking / reasoning_effort / reasoning_content"
summary: "DeepSeek 思考模式在 dsh-ee 网关里是原样透传：开关、思考强度、响应 reasoning_content 与工具轮次的回传要求，以及 /v1/models 的能力声明。"
summary_en: "Thinking mode through the dsh-ee gateway is pass-through: switches, reasoning effort, reasoning_content, the tool-turn pass-back rule, and capability discovery on /v1/models."
tags: [Harness, 思考模式, 模型网关, reasoning_content]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

## dsh-ee 做什么、不做什么

dsh-ee 的模型网关是 OpenAI chat completions 的**兼容转发层**：请求体除 `model` 外的字段原样转发给
上游，上游响应体原样返回。思考模式因此也是透传，**网关不加强校验、不做 effort 映射、不补写或删除
`reasoning_content`**：

- 上游支持什么参数、可用哪些思考强度，取决于上游模型与账号开通情况。
- 参数不合法、工具轮次缺少 `reasoning_content` 时，是**上游**返回错误；dsh-ee 原样返回该错误
  （状态码与错误体都不改写）。
- dsh-ee 只额外提供两件事：模型能力声明（`/v1/models` 等）与管理端配置入口；它们只用于展示与
  能力发现，不影响转发行为。

## 请求：开关与强度

| 参数 | 示例 | 说明 |
|---|---|---|
| `thinking` | `{"type": "enabled"}` / `{"type": "disabled"}` | 思考模式开关，原样转发 |
| `reasoning_effort` | `"high"` | 思考强度，原样转发 |

DeepSeek 的 OpenAI 格式请求示例：

```sh
curl -fsS "$DSH_EE_BASE/v1/chat/completions" \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "<模型名，以 /v1/models 为准>",
    "messages": [{"role": "user", "content": "9.11 和 9.8 哪个大？"}],
    "thinking": {"type": "enabled"},
    "reasoning_effort": "high"
  }'
```

DeepSeek 文档列出的请求值会映射到实际推理强度：`minimal`→`low`、`low`→`low`、`medium`→`high`、
`high`→`high`、`xhigh`→`high`、`max`→`max`、`ultra`→`max`。思考模式默认可能已打开，effort 默认
由上游决定（DeepSeek 当前默认 `high`）；以[上游思考模式文档](https://api-docs.deepseek.com/zh-cn/guides/thinking_mode)
为准。

> dsh-ee 网关走的是 OpenAI chat completions 形状；上游的 Anthropic / Responses 格式参数不在
> 这里定义，需要时以原始 JSON 透传并由上游解释。

思考模式下上游通常忽略 `temperature`、`presence_penalty`、`frequency_penalty` 等采样参数
（不报错、不生效）；`top_p` 等参数是否生效因模型而异。是否发送由客户端决定，dsh-ee 不改写。

## 响应：reasoning_content

思维链在 `reasoning_content` 字段返回，与最终回答 `content` 同级：

- 非流式：`choices[].message.reasoning_content` 与 `choices[].message.content`。
- 流式：`choices[].delta.reasoning_content` 与 `choices[].delta.content`，按 chunk 增量返回。

```json
{
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "reasoning_content": "先比较两个小数的整数部分……",
        "content": "9.11 比 9.8 大。"
      }
    }
  ]
}
```

网关逐行转发 SSE（含 `data:` 行与 `[DONE]`），只在最后一个分块上补
`stream_options.include_usage`；`reasoning_content` 的分块顺序与内容不变。

## 多轮：什么时候必须回传

是否回传历史 `reasoning_content`，取决于请求是否携带 `tools`：

| 请求 | 历史 `reasoning_content` | 上游行为 |
|---|---|---|
| **携带 `tools`**（函数调用/多轮工具） | 必须完整回传（每个历史 assistant 消息） | 拼进上下文；缺失会返回 400 |
| 不携带 `tools` | 可以不回传；传了也会被忽略 | 不拼进上下文 |

**工具轮次**是硬性要求：同一轮「思考 → tool_calls → tool 结果 → 继续思考」的每一次子请求，都要把
历史 assistant 消息的 `reasoning_content` 原样带上——即使该轮模型没有实际发起工具调用。缺失时的
典型错误：

```text
The `reasoning_content` in the thinking mode must be passed back to the API.
```

正确的客户端做法是**保存并原样回传模型返回的 assistant 消息**，不要只保留 `content`：

```python
# 非流式：返回的 message 已包含 content / reasoning_content / tool_calls
messages.append(response.choices[0].message)

# 流式：把各分块的 delta.reasoning_content 与 delta.content 拼接后，作为 assistant 消息回传
messages.append({
    "role": "assistant",
    "content": content,
    "reasoning_content": reasoning_content,
    "tool_calls": tool_calls,
})
```

跨轮次对话（新问题）如果仍然带 `tools`，历史 assistant 消息里的 `reasoning_content` 也应继续
保留；只有确定不再携带 `tools` 的会话才可以丢弃。

## 能力发现与管理端配置

管理端「模型接入」里可以为每个对外模型声明能力（只影响公布，不影响转发）：

| 字段 | 含义 |
|---|---|
| `supports_reasoning` | 该模型是否支持思考模式 |
| `reasoning_efforts` | 可用的 `reasoning_effort` 值（逗号分隔，如 `low,high,max`；空表示按上游默认） |

客户端通过以下 additive 字段发现能力：

| 位置 | 字段 |
|---|---|
| `GET /v1/models` 每项 | `capabilities.reasoning = {supported, efforts}` |
| `GET /v1/capabilities` | `reasoning = {supported, parameters, content_field, per_model}` |
| `GET /.well-known/dsh-ee` 的 `models` | 同上 `reasoning` 对象 |

- `parameters` 是请求参数名（`["thinking", "reasoning_effort"]`），`content_field` 是响应字段名
  （`"reasoning_content"`）。
- `/v1/models` 的 `owned_by`、`id`、`object` 等原有字段不变；旧客户端忽略 `capabilities` 即可。
- 旧部署没有这些能力字段时，`thinking` / `reasoning_effort` 仍然原样透传，SDK 类型按可选字段处理。

## 客户端实现清单

1. 从 `/v1/models` 读 `capabilities.reasoning` 决定是否展示「深度思考」开关与 effort 选项；
   不要硬编码模型名。
2. 请求里原样传 `thinking` / `reasoning_effort`，不做本地白名单；无效值交给上游报错并原样展示。
3. 非流式保留 `message.reasoning_content`；流式拼接 `delta.reasoning_content` 后再进入下一轮。
4. 带 `tools` 的会话：历史 assistant 消息必须保留 `reasoning_content`，再回传。
5. 上游 400（例如缺少历史 `reasoning_content`）原样展示，不要把错误改写成网络错误。

TypeScript / Python SDK 的类型与示例见 [`sdk/typescript/README.md`](https://github.com/hikariming/dsh-ee/blob/main/sdk/typescript/README.md)、
[`sdk/python/README.md`](https://github.com/hikariming/dsh-ee/blob/main/sdk/python/README.md) 与
[`sdk/examples/thinking-mode.md`](https://github.com/hikariming/dsh-ee/blob/main/sdk/examples/thinking-mode.md)。
