# 思考模式：thinking / reasoning_effort / reasoning_content

dsh-ee 的模型网关（`POST /v1/chat/completions`）对思考模式参数**原样透传**：既不加强校验，
也不做 effort 映射；是否生效、可用哪些强度由上游模型决定。能力以服务端公布为准：

| 看哪里 | 内容 |
|---|---|
| 发现文档 `GET /.well-known/dsh-ee` 的 `models.reasoning` | 参数名（`thinking` / `reasoning_effort`）与响应字段（`reasoning_content`） |
| `GET /v1/capabilities` 的 `reasoning` | 同上（协议面能力） |
| `GET /v1/models` 每项的 `capabilities.reasoning` | 该模型 `supported` 与可用 `efforts`（旧部署可能没有该字段） |

## 最小请求

```bash
curl -fsS "$DSH_EE_BASE/v1/chat/completions" \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-reasoner",
    "messages": [{"role": "user", "content": "9.11 和 9.8 哪个大？"}],
    "thinking": {"type": "enabled"},
    "reasoning_effort": "high"
  }'
```

- `thinking`：`{"type":"enabled"}` 打开、`{"type":"disabled"}` 关闭（DeepSeek 等上游默认可能已打开，
  以服务端/上游文档为准）。
- `reasoning_effort`：请求值 `minimal` / `low` / `medium` / `high` / `xhigh` / `max` / `ultra`
  （上游会映射到实际推理强度）；具体可用值见 `/v1/models` 的 `capabilities.reasoning.efforts`。
- 响应：思维链在 `choices[].message.reasoning_content`，最终回答在 `choices[].message.content`。

## TypeScript（`@dsh-ee/harness-client`）

```ts
import type { ChatMessage, DshHarnessClient } from '@dsh-ee/harness-client';

const messages: ChatMessage[] = [{ role: 'user', content: '杭州明天天气？' }];
const completion = await client.createChatCompletion({
  model: 'company-reasoner',
  messages,
  thinking: { type: 'enabled' },
  reasoning_effort: 'high',
});
const message = completion.choices[0]?.message;
console.log('思维链:', message?.reasoning_content);
console.log('回答:', message?.content);
```

流式时 `delta.reasoning_content` 逐块返回，拼起来后与 `content` 分开处理：

```ts
let reasoning = '';
for await (const chunk of client.streamChatCompletion({
  model: 'company-reasoner',
  messages: [{ role: 'user', content: '9.11 和 9.8 哪个大？' }],
  thinking: { type: 'enabled' },
  reasoning_effort: 'medium',
})) {
  reasoning += chunk.choices[0]?.delta?.reasoning_content ?? '';
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '');
}
```

## Python（`dsh-ee-harness`，仅非流式）

```python
completion = client.chat_completion(
    "company-reasoner",
    [{"role": "user", "content": "9.11 和 9.8 哪个大？"}],
    thinking={"type": "enabled"},
    reasoning_effort="high",
)
print(completion["choices"][0]["message"].get("reasoning_content"))
```

## 工具轮次：必须回传历史 `reasoning_content`

请求**携带 `tools`** 时，后续所有请求都要把历史 assistant 消息的 `reasoning_content` 原样带上
（即使该轮没有实际调用工具），否则上游返回 400：
`The reasoning_content in the thinking mode must be passed back to the API.`
请求**不带 `tools`** 时可以不回传，上游会忽略。

```ts
const messages: ChatMessage[] = [{ role: 'user', content: '杭州明天天气？' }];

const first = await client.createChatCompletion({
  model: 'company-reasoner',
  messages,
  tools,
  thinking: { type: 'enabled' },
  reasoning_effort: 'high',
});

// 非流式直接追加返回的 message：content / reasoning_content / tool_calls 都在里面。
messages.push(first.choices[0].message);
// 执行 tool_calls 后追加 { role: 'tool', tool_call_id, content }，再发下一轮：
// 历史 assistant 的 reasoning_content 会随 messages 一起回传。
```

```python
messages.append(first["choices"][0]["message"])
# 执行 tool_calls 后追加 role=tool 消息，再发下一轮。
```

流式场景下，把各分块的 `delta.reasoning_content` 拼接起来，作为该轮 assistant 消息的
`reasoning_content` 保存并回传。

> dsh-ee 网关对 `thinking` / `reasoning_effort` / 历史 `reasoning_content` 不做补写或删除，
> 上游的 400 也会原样返回；保留/回传这些字段是 Harness 客户端的责任。

## 与 `/mcp` 无关

`/mcp` 是企业工具面（服务端工具调用与写确认），不是模型侧的 `tools`/`function calling`。
上面说的「工具调用轮次回传 `reasoning_content`」只适用于 Harness 自己发起的
`/v1/chat/completions` 多轮请求。
