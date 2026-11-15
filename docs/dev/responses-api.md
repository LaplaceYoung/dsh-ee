---
slug: responses-api
category: harness
title: "OpenAI Responses API 兼容"
title_en: "OpenAI Responses API compatibility"
summary: "把 Codex / OpenAI Agents SDK 等 Responses 客户端接到 dsh-ee：POST /v1/responses 的请求映射、输出 item、SSE 事件、错误与 v0 边界。"
summary_en: "Connect Responses-style clients (Codex, OpenAI Agents SDK) to dsh-ee: request mapping, output items, SSE events, errors and v0 limits of POST /v1/responses."
tags: [Harness, Responses, Codex, 模型网关, 流式]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

## 这是什么

`POST /v1/responses` 是 dsh-ee 的 **OpenAI Responses API 兼容入口**（v0），另有 `POST /responses` 别名（DeepSeek Responses API 的 base URL 习惯是不带 `/v1`，客户端会在根地址后拼 `/responses`）。两个路径走同一个 handler，鉴权、模型权限、额度、限流、错误壳与事件完全一致。按 Responses 协议写的客户端——Codex、OpenAI Agents SDK、以及把 DeepSeek Responses 当上游的工具——可以用同一个企业令牌接进来，模型权限、账号/令牌月度额度、用量审计与上游故障转移和 `/v1/chat/completions` 完全一致。

具体怎么走取决于命中的上游 `vendor`（管理端建上游时配置，默认 `custom`）：

- **DeepSeek 上游（`vendor=deepseek`）：原生透传**。请求 JSON 只改 `model` 后直接发到 `<provider.base>/responses`，响应与 SSE 逐字节转发；web_search 等服务端工具、citations/annotations、refusal、title、image/file 内容块都原样保留。dsh-ee 只做鉴权、模型权限、额度、限流、model 改写、用量旁路记账与同厂商故障转移。
- **其它上游（默认）：chat completions 转换层（降级）**。请求转成 OpenAI chat completions，响应与流式事件再转回 Responses 形状；上游只需是 OpenAI chat completions 兼容服务。转换层只实现 Responses 的一个子集（见「请求字段与转换」与「v0 边界」）。

| 项 | 值 |
| --- | --- |
| 路径 | `POST /v1/responses`（主入口）或 `POST /responses`（DeepSeek 习惯的别名，等价） |
| 鉴权 | `Authorization: Bearer <ee_at_…>`；需要 `models` scope 与账号「调用大模型」权限 |
| 模型名 | 填 `GET /v1/models` 返回的 dsh-ee **对外模型名**（不做 `gpt-*` / `claude-*` 名称映射） |
| 请求体上限 | 16 MiB |
| 协议版本头 | 与其它 `/v1/*` 一样带 `Dsh-EE-API-Version: 1` |

```bash
curl -fsS "$BASE/v1/responses" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "instructions": "你是企业助手，回答简洁。",
    "input": "ping"
  }'
```

OpenAI 官方 SDK 会在 `base_url` 后面拼 `/responses`，两种填法都可以：

```python
from openai import OpenAI

# 推荐：base_url 指到 /v1 → 请求 POST /v1/responses
client = OpenAI(api_key=TOKEN, base_url=BASE + "/v1")
resp = client.responses.create(model="company-chat", input="ping")

# DeepSeek Responses API 习惯：base_url 是根地址 → 请求 POST /responses（等价别名）
deepseek_style = OpenAI(api_key=TOKEN, base_url=BASE)
resp = deepseek_style.responses.create(model="company-chat", input="ping")
print(resp.output_text)
```

别名同样要求 Bearer：`curl -fsS "$BASE/responses" -H "Authorization: Bearer $TOKEN" ...` 与 `/v1/responses` 返回完全一致。

## 上游模式：原生透传 vs 转换层

dsh-ee 按模型候选的**第一条线路**（按 provider `priority`）决定这次请求的协议，故障转移只在同协议/厂商的候选之间进行，不会因为首选线路失败就悄悄换协议。

| 上游 `vendor` | 模式 | 行为 | 保留能力 |
| --- | --- | --- | --- |
| `deepseek` | **原生透传** | 请求 JSON 语义透传（只改 `model`）→ `POST <provider.base>/responses`；响应与 SSE **逐字节转发** | web_search / 服务端工具、citations/annotations、refusal、title、image/file 内容块、上游自定义字段 |
| 其它（`custom` 等） | **转换层（降级）** | Responses → OpenAI chat completions → 响应转回 Responses | 本文件描述的 v0 子集：文本、reasoning、function 工具、text.format、usage |

配置方式：管理端录入上游时把 `vendor` 填成 `deepseek`（创建/更新 provider 的 `vendor` 字段，默认 `custom`）。同一对外模型可以挂多个上游；如果同厂商有多条线路，会在这些线路之间做故障转移（连不上、429、5xx），不会跨到转换层。

原生透传下 dsh-ee **不做**这些事（交给 DeepSeek 上游）：Responses 参数校验/裁剪、`max_output_tokens` 的 32768 单次上限、内容块/工具转换、SSE 事件重排。请求体上限仍是 16 MiB；鉴权、模型权限、账号/令牌月度额度、限流、用量记账与错误壳与转换层一致。

发现文档 `/.well-known/dsh-ee` 的 `models` 里会公布：

| 字段 | 说明 |
| --- | --- |
| `responses_endpoint` / `responses_alias_endpoint` | 两个 Responses 入口的绝对地址 |
| `responses_native_vendors` | 走原生透传的上游 vendor（当前 `["deepseek"]`） |
| `responses_fallback` | 其它 vendor 的降级方式：`openai-chat-completions-translation` |

### 原生透传的文件引用访问边界

DeepSeek 的文件属于**上游共享账号**，员工之间不隔离：普通员工即使不知道文件接口，只要知道一个 `file_id`，就能在 `input` 里引用它、让上游读该账号的文件。因此原生透传路径在**转发上游之前**检查请求 `input`（递归查找任何非空 `file_id`，覆盖 `input_file` / `input_image` 等内容块；流式与非流式同一份请求体）：

| 调用方 | 默认行为 |
| --- | --- |
| 普通员工（human，无管理权限点） | 403 `permission_error` / `file_access_denied`，不转发上游 |
| 管理员（账号带任一管理权限点） | 放行 |
| 机器人 / 服务账号（`account_type=bot`） | 放行 |
| 部署开启 `DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE`（默认关闭） | 所有登录账号放行；开启前请评估「上游文件对员工不隔离」的风险 |

- 只拦文件引用，不拦普通文本/图片 URL；不带 `file_id` 的请求不受影响。
- 转换层（非 DeepSeek vendor）的 v0 本来就不支持 `input_file` 等非文本内容块，会返回 400 `unsupported_content`；原生透传的拦截是额外的访问边界，不是格式问题。
- 与 #160（DeepSeek 工具代理）使用同一环境变量与语义（`file_access_denied`、管理员/机器人判定）；等 #160 合入后统一为它的实现，`docs/dev/deepseek-tools.md` 会有完整工具面说明。

## 请求字段与转换

**这一节只适用于转换层（非 DeepSeek vendor）**。原生透传时请求体整包转发，不做下面的字段转换；DeepSeek 的完整 Responses 能力（web_search、citations、title、image/file 等）以 DeepSeek 官方文档为准。

转换层只列出 v0 会处理的字段；请求体里其它 Responses 字段**被忽略**（见「v0 边界」），不会报错。

| Responses 字段 | 处理 |
| --- | --- |
| `model` | 必填；按 dsh-ee 对外模型名校验权限，转发时换成该上游的模型名 |
| `input` | 字符串按一条 `user` 消息处理；数组见下面的 item 表；`input` 与 `instructions` 至少有一个 |
| `instructions` | 作为模型上下文的**第一条 system 消息**（chat completions 的 `system`） |
| `tools` | Responses 平铺形状 `{type:"function", name, description, parameters, strict}` → chat 的 `{type:"function", function:{...}}`；只支持 `function`，其它类型忽略 |
| `tool_choice` | `auto` / `none` / `required` 原样；`{type:"function", name}` → `{type:"function", function:{name}}` |
| `parallel_tool_calls` | 透传上游 `parallel_tool_calls` |
| `max_output_tokens` | 转成上游 `max_tokens`；复用网关单次上限 32768（超过返回 400） |
| `reasoning.effort` | 原样转成上游 `reasoning_effort`（`none` / `low` / `high` / `max` 等语义由上游决定；DeepSeek 会把 `minimal` 映射为 `low`、`medium`/`xhigh` 映射为 `high`） |
| `text.format` | `{"type":"text"}` 不转发；`json_object` → 上游 `response_format:{"type":"json_object"}`；`json_schema`（平铺 `name`/`schema`/`strict`）→ 上游 `response_format:{"type":"json_schema","json_schema":{...}}` |
| `temperature` / `top_p` | 透传（分别校验 0–2 / 0–1） |
| `user` | 透传上游 `user` |
| `stream` | `true` 时返回 Responses SSE 事件，见「流式」 |

### `input` item 转换

| input item | 转成 chat messages |
| --- | --- |
| `message`（role `user`/`assistant`/`system`） | 对应 role，文本内容拼成一个字符串 |
| `message`（role `developer`） | 降级为 `system`（chat 上游普遍没有 developer 角色） |
| `function_call` | 并进相邻 assistant 消息的 `tool_calls`（没有则新建一条 content 为 null 的 assistant） |
| `function_call_output` | `{role:"tool", tool_call_id:<call_id>, content:<output 文本>}` |
| `reasoning` | 忽略（v0 不把历史思维链回传上游） |
| 其它 item（`web_search_call`、`custom_tool_call` 等） | 忽略 |

文本内容块支持 `input_text` / `output_text` / `text`（数组会拼起来）；`input_image` / `input_file` 等非文本内容块 v0 返回 400 `unsupported_content`，避免静默丢图让模型给出错误答案。

## 响应

转换层返回 dsh-ee 转换出的标准 Responses `response` 对象（与 OpenAI / DeepSeek 的 response 结构对齐，OpenAI SDK 可直接反序列化）；**原生透传时上游响应体原样返回**（可能包含 `web_search_call` / `custom_tool_call` / `title` 等上游字段，`usage` 也是 DeepSeek 的 `input_tokens`/`output_tokens` 形状，dsh-ee 不改字节）。

下面是转换层的响应示例：

```json
{
  "id": "resp_…",
  "object": "response",
  "created_at": 1793084842,
  "status": "completed",
  "error": null,
  "incomplete_details": null,
  "instructions": "你是企业助手",
  "max_output_tokens": null,
  "model": "company-chat",
  "output": [
    {
      "type": "message",
      "id": "msg_…",
      "status": "completed",
      "role": "assistant",
      "content": [{"type": "output_text", "text": "你好", "annotations": [], "logprobs": []}]
    }
  ],
  "parallel_tool_calls": true,
  "previous_response_id": null,
  "reasoning": {"effort": null, "summary": null},
  "store": false,
  "temperature": 1.0,
  "text": {"format": {"type": "text"}, "verbosity": null},
  "tool_choice": "auto",
  "tools": [],
  "top_p": 1.0,
  "truncation": "disabled",
  "usage": {
    "input_tokens": 10,
    "input_tokens_details": {"cached_tokens": 0},
    "output_tokens": 5,
    "output_tokens_details": {"reasoning_tokens": 0},
    "total_tokens": 15
  },
  "user": null,
  "metadata": {}
}
```

`output` 里可能出现三类 item：

| output item | 说明 |
| --- | --- |
| `reasoning` | 上游 `reasoning_content` / `reasoning` 的思考链，纯文本放在 `content:[{type:"reasoning_text",text}]`，`summary` 固定空数组（v0 不生成摘要、不做加密） |
| `message` | assistant 文本；`content` 为 `output_text` 部分（带空的 `annotations` / `logprobs`）。有工具调用且没有文本时不出现 message item |
| `function_call` | `call_id` / `name` / `arguments`（JSON 字符串），客户端把结果作为 `function_call_output` 放进下一轮 `input` 即可 |

`status`：正常 `completed`；上游 `finish_reason=length` → `incomplete` + `incomplete_details.reason=max_output_tokens`；`content_filter` → `incomplete` + `content_filter`。

`usage` 与 `/v1/chat/completions` 的 `usage` 同源：`input_tokens`/`output_tokens` 就是上游 prompt/completion token，缓存命中与思考 token 分别放在 `input_tokens_details.cached_tokens` / `output_tokens_details.reasoning_tokens`；记账按 prompt+completion 总 token 计入月度额度，与 chat 网关一致。

**原生透传的用量旁路**：非流式从响应体的 `usage` 取；流式从最后的 `response.completed` / `response.incomplete` / `response.failed` 事件里的 `response.usage` 取，换算成同一份月度用量记账（`input_tokens`→prompt、`output_tokens`→completion、`total_tokens`→total）。客户端中途断开导致拿不到 usage 时，按请求体与已转发的 delta 文本估算并记日志；这只影响兜底记账，不影响转发的内容。

## 流式

`stream:true` 时返回 `text/event-stream`。**原生透传时 dsh-ee 逐字节转发上游 SSE**（事件、字段、行尾都以 DeepSeek 为准，`[DONE]` 之类也以上游实际行为为准，dsh-ee 不增删）；下面描述的是转换层的 v0 行为。转换层每个事件是 `event: <type>` + `data: <json>`（JSON 里的 `type` 与事件名一致，带递增的 `sequence_number`），最后一个事件是 `response.completed`（被截断时 `response.incomplete`），**没有 `data: [DONE]`**。

v0 事件序列（文本回答）：

```text
response.created            # status=in_progress、output=[]
response.in_progress
response.output_item.added  # message item，status=in_progress、content=[]
response.content_part.added
response.output_text.delta  # 文本通常一次发完（见下）
response.output_text.done
response.content_part.done
response.output_item.done   # status=completed、content 带完整文本
response.completed          # 完整 response 对象 + usage
```

有思考链时在 message 之前多出 `reasoning` item 的事件：`response.output_item.added` → `response.content_part.added` → `response.reasoning_text.delta` / `response.reasoning_text.done` → `response.content_part.done` → `response.output_item.done`。有工具调用时是 `response.function_call_arguments.delta` / `.done`。

> **v0 限制**：dsh-ee 先拿到完整上游结果，再按上面的语义回放 SSE 事件；不是逐 token 转发。客户端的事件解析逻辑不变，但**首字节延迟等于整段生成时间**，`output_text.delta` 一般只有一个、内容是整段文本。逐 token 转发留待后续版本。

流式请求如果上游在开始写 SSE 之前就失败（鉴权、模型、上游 4xx/5xx），返回的是普通 JSON 错误壳而不是 SSE 事件。原生透传的上游错误走同一套错误壳：状态码保留、尽量透传上游 `error.message/type/code`。

## 错误

错误是 Responses 形状 `{"error":{"message","type","param","code"}}`（比 chat 网关多一个 `param`；没有时是 null）。

| HTTP | type / code（示例） | 原因 |
| --- | --- | --- |
| 401 | `authentication_error` / `token_invalid`·`token_expired`·`token_revoked` | 缺少/无效/过期/吊销的 Bearer |
| 403 | `permission_error` / `permission_denied` | 令牌没有 `models` scope，或账号没有「调用大模型」权限 |
| 403 | `permission_error` / `model_not_allowed` | 模型不在账号/令牌允许范围 |
| 403 | `permission_error` / `quota_expired` | 额度已过期 |
| 403 | `permission_error` / `file_access_denied` | 普通员工在原生透传的 `input` 里引用已上传文件的 `file_id`（默认只允许管理员/机器人） |
| 404 | `invalid_request_error` / `model_not_found` | 没有启用的上游候选 |
| 400 | `invalid_request_error` / `model_required`·`invalid_json`·`invalid_input`·`invalid_tools`·`invalid_text_format`·`unsupported_content`… | 请求体问题，`param` 指出字段 |
| 400 | `invalid_request_error` / `max_output_tokens_too_large` | 单次输出上限 32768 |
| 413 | `invalid_request_error` / `request_too_large` | 请求体超过 16 MiB |
| 429 | `insufficient_quota` / `monthly_quota_exceeded` | 账号或令牌本月额度用完 |
| 502 | `server_error` / `upstream_unreachable`·`upstream_bad_response` | 所有上游线路失败 / 上游返回无法解析的响应 |
| 上游状态 | 上游 error 的 type/code 尽量原样透传 | 上游 4xx/5xx；状态码保留，便于客户端区分 |

常见原因与修法见 [错误码索引](../errors.md) 与 [错误与 FAQ](./errors-faq.md)。

## v0 边界

**转换层（非 DeepSeek vendor）**：

- **无状态**：不保存 response / conversation；`previous_response_id`、`conversation`、`background`、`store`、`metadata`、`include`、`truncation`、`service_tier`、`prompt` 等字段被忽略（和 DeepSeek Responses API 的兼容策略一致）。响应里 `store` 固定 `false`、`previous_response_id` 固定 `null`、`metadata` 固定 `{}`。多轮对话请由客户端把完整历史放进 `input`（`function_call` / `function_call_output` 已在 v0 支持）。
- **不做** conversations / files / batches / background / embeddings / `/v1/completions`。
- 只支持 `function` 工具；`custom` 工具（如 Codex 的 `apply_patch`）、`web_search` 等服务端工具忽略。
- `input_image` / `input_file` 等非文本内容、`custom_tool_call*` item 不在 v0 范围。
- 流式是完整结果回放，不是逐 token（见「流式」）。
- 上游仍需是 OpenAI chat completions 兼容服务；Responses → chat 的映射在 dsh-ee 内完成，用量与额度规则不变。

**原生透传（DeepSeek vendor）**：

- 请求/响应不做 Responses 子集裁剪：web_search / 服务端工具、citations、refusal、title、image/file 内容块等以上游为准。
- 不做 `max_output_tokens` 的 32768 网关单次上限与响应格式校验；上游自己会校验参数，错误按上游返回映射。
- 请求体上限仍为 16 MiB；故障转移只在 DeepSeek 候选之间；月度额度、令牌额度、限流、审计与转换层共用同一套。

## 与其它入口的关系

| 入口 | 适合的客户端 |
| --- | --- |
| `POST /v1/chat/completions` | OpenAI chat completions 客户端、DSH 接入插件 |
| `POST /v1/responses`（别名 `POST /responses`） | Codex、OpenAI Agents SDK、DeepSeek Responses 习惯的客户端 |
| `POST /v1/messages`（别名 `/anthropic/v1/messages`） | Claude Code / Anthropic SDK，见 [Anthropic Messages 兼容](./anthropic-api.md) |
| `/mcp` + `/.well-known/dsh-ee` | 只需要工具面 / 能力发现的第三方 Harness |

模型权限、额度、用量记账与故障转移在三个模型入口之间完全一致；换协议不改变企业侧的权限与计费。
