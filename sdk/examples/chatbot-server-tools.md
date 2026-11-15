# DeepSeek Chatbot：server tools / safety_check / title / 拒答

DeepSeek 的「Build a Chatbot」能力通过 OpenAI chat completions 的扩展字段声明与返回：

| 能力 | 请求 | 响应 |
|---|---|---|
| 联网搜索 / 深度思考搜索 | `tools: [{"type": "deepseek_web_search"}]` | `message.server_tool_calls`（`kind: search` / `deep_search`）、`message.annotations`（引用） |
| 阅读链接 | `tools: [{"type": "deepseek_read_url"}]` | `message.server_tool_calls`（`kind: read_link`，含 `signature`） |
| 模型内容安全 | `safety_check: {"level": "high"}` | 命中风险时 `finish_reason: "content_filter"`、`message.refusal`、流式 `delta.refusal` |
| 会话标题 | `title: {"enabled": true}` | 非流式 `message.title`；流式最后一块 `delta.title` |
| 用户元信息标记 | `messages` 里放 `{"role": "latest_reminder", "content": "<date>,<weekday>,<location>,<platform>,<language>"}` | 无响应字段（只影响上下文） |
| 文件 / 文档输入 | `file` 内容块（`file_id` / `file_data`）或 Messages 的 `document` 块 | 按文件内容作答；文档需先用 DeepSeek Files API 上传 |

**边界**：搜索、打开页面、阅读链接都由 DeepSeek 服务端执行，dsh-ee 网关只做 OpenAI chat
completions 透传，**不解析、不改写、不执行**这些工具；本 SDK 只提供类型与解析，不实现服务端
工具逻辑。是否支持取决于上游模型/账号（DeepSeek 文档：`deepseek-flash` 等）。

## 声明工具并请求

```bash
curl -fsS "$DSH_EE_BASE/v1/chat/completions" \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "<模型名，以 /v1/models 为准>",
    "tools": [
      {"type": "deepseek_web_search"},
      {"type": "deepseek_read_url"}
    ],
    "safety_check": {"level": "high"},
    "title": {"enabled": true},
    "messages": [{"role": "user", "content": "DeepSeek V4.1 有什么新特性？"}]
  }'
```

- Chat Completions 的 `tools` 声明用 `{type: "deepseek_web_search" | "deepseek_read_url"}`（不需要
  `name`）；Messages API 用 `{type, name}`，dsh-ee 的 `/v1` 面按 Chat Completions 形状透传。
- 启用这些能力/参数后，上游会自行决定 `temperature`、`top_p`、`frequency_penalty`、
  `presence_penalty`、`stop`、`seed`、`n` 等采样参数（请求里的值被忽略）；`max_tokens` 仍生效，
  但限制的是每一步推理的输出。
- `tool_choice` 不能指定这两个服务端工具（会 400）；`tool_choice: "none"` 会关闭所有工具。

## TypeScript：非流式解析

```ts
import type { DeepSeekServerToolCall } from '@dsh-ee/harness-client';

const result = await client.createChatCompletion({
  model: 'company-bot',
  messages: [{ role: 'user', content: 'DeepSeek V4.1 有什么新特性？' }],
  tools: [{ type: 'deepseek_web_search' }, { type: 'deepseek_read_url' }],
  safety_check: { level: 'high' },
  title: { enabled: true },
});

const message = result.choices[0]?.message;
console.log('标题:', message?.title);
for (const step of message?.server_tool_calls ?? []) {
  for (const call of step.calls) {
    console.log('工具:', call.type, call.kind, call.action, call.status);
    for (const source of call.sources ?? []) console.log('  来源:', source.title, source.url);
    if (call.page) console.log('  页面:', call.page.title, call.page.url);
    if (call.signature) console.log('  signature:', call.signature);
  }
}
for (const annotation of message?.annotations ?? []) {
  console.log('引用:', annotation.url_citation.title, annotation.url_citation.url);
}
```

`message.server_tool_calls` 的每个 `call` 里有 `type`（声明的工具）、`kind`（`search` /
`deep_search` / `read_link` / `image_search` / `deep_image_search`）、`action`、`status` 与
`sources` / `page` / `signature` / `error`；`server_tool_use` 形状的类型在 Messages API 块
（`DeepSeekServerToolUseBlock` / `DeepSeekWebSearchToolResultBlock` /
`DeepSeekWebFetchToolResultBlock`）里，需要统一渲染时可直接复用。

## 多轮：原样回传 `server_tool_calls`

服务端工具轮次的下一次请求要把上一轮 assistant 消息**原样**带回，尤其是每个 call 的
`kind` 与 `signature`：

```ts
const messages = [{ role: 'user' as const, content: '总结 https://example.com/' }];
const first = await client.createChatCompletion({
  model: 'company-bot',
  messages,
  tools: [{ type: 'deepseek_read_url' }],
});
// 直接追加返回的 message：content / reasoning_content / server_tool_calls 都在里面。
messages.push(first.choices[0].message);
messages.push({ role: 'user', content: '引用第一句' });

await client.createChatCompletion({ model: 'company-bot', messages, tools: [{ type: 'deepseek_read_url' }] });
```

- 阅读链接的页面内容加密在 `signature` 里；丢掉 `signature`（或整个 `server_tool_calls`）请求
  仍会成功，但之前读过的页面不会进入上下文。
- 历史里含工具块的 assistant 消息可以在**不声明**这些工具的请求里继续发送。
- 搜索/打开页面的过程块不会进入后续上下文，只有回答正文会；工具块用于展示与恢复页面。

## 流式：`server_tool_event` 与最后一块

流式响应里工具进度在 choice 级扩展字段 `server_tool_event`（与 `delta` 同级，一个 chunk 最多
一个事件）：`step_reasoning_done`（该步思考结束）、`call_started`（带完整 `action`）、
`call_completed`（带 `status` / `sources` / `page` / `error`，不带 `signature`）。
完整的 `server_tool_calls` 与 `annotations` 只在带 `finish_reason` 的最后一块 `delta` 里返回。

```ts
let title = '';
for await (const chunk of client.streamChatCompletion({
  model: 'company-bot',
  messages: [{ role: 'user', content: 'DeepSeek V4.1 有什么新特性？' }],
  tools: [{ type: 'deepseek_web_search' }],
  title: { enabled: true },
})) {
  for (const choice of chunk.choices) {
    if (choice.server_tool_event) console.log('事件:', choice.server_tool_event.type, choice.server_tool_event.step);
    if (choice.delta.title) title = choice.delta.title;
    if (choice.delta.server_tool_calls) console.log('完整工具列表:', choice.delta.server_tool_calls);
    process.stdout.write(choice.delta.content ?? '');
  }
}
```

## 拒答处理

带 `safety_check` 时，回答结束后服务端还会审核完整输出；命中风险时：

- Chat Completions：`finish_reason: "content_filter"`，`message.content` 为空串、`annotations`
  省略、`server_tool_calls` 保留（每步思考清空）；被拦截图片的说明在 `message.refusal`
  （流式在最后一块 `delta.refusal`）。
- Messages API 形状：`stop_reason: "refusal"` + `stop_details`（`DeepSeekStopDetails`）；
  Responses API 形状：`status: "incomplete"`、`incomplete_details.reason: "content_filter"`，
  拒答文本是 `type: "refusal"` 的 `DeepSeekRefusal` 块。

客户端处理建议：

1. 看到 `content_filter`（或 `refusal` / `stop_details`）就**移除已展示的回答**，换成自己的
   拒答提示；流式已经输出的文字无法撤回。
2. 不要重试同一请求期待不同结果；不要向用户展示具体风控原因（响应不提供）。
3. 拒答前的 token 照常计费；`usage.completion_tokens_details.safety_tokens` 是审核消耗。

```ts
if (result.choices[0]?.finish_reason === 'content_filter') {
  console.log(result.choices[0].message.refusal); // 例如被拦截的图片
  // 展示本地拒答文案
}
```

输入被审核拦截时，请求直接返回 `400 Content Exists Risk`（OpenAI 错误壳，dsh-ee 原样转发）。

## Python（仅非流式）

```python
result = client.chat_completion(
    "company-bot",
    [{"role": "user", "content": "DeepSeek V4.1 有什么新特性？"}],
    tools=[{"type": "deepseek_web_search"}, {"type": "deepseek_read_url"}],
    safety_check={"level": "high"},
    title={"enabled": True},
)
message = result["choices"][0]["message"]
print(message.get("title"))
for step in message.get("server_tool_calls", []):
    for call in step["calls"]:
        print(call["type"], call["kind"], call.get("signature"))
print(result["choices"][0]["finish_reason"])
```

Python 版只实现非流式；`server_tool_event` / `delta.title` 的类型在
`dsh_ee_harness.types` 里（`DeepSeekServerToolEvent` 等），需要流式请用 TypeScript 版或自行解析 SSE。

## 不做什么

- 不实现 `deepseek_web_search` / `deepseek_read_url` 的搜索、抓取、解密 `signature` 逻辑——这些
  全部由上游 DeepSeek 服务端完成。
- 不实现 Messages / Responses 协议的调用；只提供这些协议里工具块/拒答块的类型，供统一渲染参考。
- 不自动重试、不改写 `safety_check` / `title`；dsh-ee 是否注入这些参数由服务端配置决定（见服务端
  文档；本 SDK 不感知）。

## 用户元信息标记 latest_reminder

在 `messages` 里放一条 `latest_reminder` 消息，把当前日期、星期、位置、平台与语言告诉模型
（与 DeepSeek Chat 的行为一致）。`content` 格式固定为
`<date>,<weekday>,<location>,<platform>,<language>`：

| 字段 | 说明 |
|---|---|
| `date` | `YYYY-MM-DD` |
| `weekday` | 该日期的星期，用 `language` 写（如 `Monday` / `星期一`） |
| `location` | 可选、可为空；**允许包含逗号** |
| `platform` | `App` 或 `Web` |
| `language` | 语言名用该语言本身写（如 `English` / `中文`） |

例如 `2026-11-11,Wednesday,Singapore,Web,English`。必须**紧贴**在它对应的 `user` 消息之前，
否则上游返回 400；不能放在 `tool_use` 与其 `tool_result` 之间。历史轮次的 reminder 可以原样
带回（回传可提高 context cache 命中，省略也允许）。`content` 必须是字符串，否则 request body
无法解析（422）。Chat Completions 与 Messages API 支持；Responses API 不接受。

```ts
import { formatLatestReminder } from '@dsh-ee/harness-client';

const messages = [
  {
    role: 'latest_reminder',
    content: formatLatestReminder({
      date: '2026-11-11',
      weekday: 'Wednesday',
      location: 'Singapore',
      platform: 'Web',
      language: 'English',
    }),
  },
  { role: 'user', content: '今天有什么新闻？' },
];

await client.createChatCompletion({ model: 'company-bot', messages });
```

```python
from dsh_ee_harness import format_latest_reminder

messages = [
    {
        "role": "latest_reminder",
        "content": format_latest_reminder(
            {
                "date": "2026-11-11",
                "weekday": "Wednesday",
                "location": "Singapore",
                "platform": "Web",
                "language": "English",
            }
        ),
    },
    {"role": "user", "content": "今天有什么新闻？"},
]
client.chat_completion("company-bot", messages)
```

> `location` 允许含逗号且位于第三段，所以不要用 `split(",")` 反解 content；保留生成它的结构化
> 字段（TS 的 `DeepSeekLatestReminder` / Python 的 `DeepSeekLatestReminder`）。

## 文件 / 文档输入（DeepSeek Files API）

文件需要**两步**：先上传到 DeepSeek Files API 并完成解析拿到 `file_id`，再在 chat 请求里引用。
dsh-ee 的 `/v1` 只透传 chat 请求，**不代理 Files API、也不解析文件**；SDK 不实现 Files API，
上传请直接调用 DeepSeek（或你所连上游）的 `POST /files`（OpenAI 兼容 Files 形状）。

1. `POST /files`（`multipart/form-data`）：`file` + `purpose`；文档的 `purpose` 要放在 `file`
   之前。`purpose` 取值：
   - `user_data`：图片（JPEG/PNG/GIF/WebP）、纯文本（txt/md/csv/json/源码）、表格（XLSX/XLS）与
     电子书（EPUB/MOBI）；不需要解析。
   - `text_recognition`：PDF/Word/PPT 等，只给模型文字，文档里的图被丢弃。
   - `rich_layout_recognition`：PDF/Word/PPT 等，文字 + 按原位置裁出的图一起给模型。
   解析成功后返回 `{"id": "file-api-...", ...}`。文档上传限制：≤100 MiB、≤1000 页、上传/解析各
   120 秒内完成；纯文本/表格/电子书 ≤32 MiB。上传的文件与 `file_id` 属于你的 DeepSeek 账号
   （同一账号下所有 API key 共享），原始文件与解析结果不能下载。
2. 在 chat 请求里引用：

```ts
// Chat Completions：user message 的 content 数组里放 file 块。
const fileId = 'file-api-xxxxxxxxxxxxxxxx'; // 上一步 POST /files 的返回值
await client.createChatCompletion({
  model: 'company-bot',
  messages: [
    {
      role: 'user',
      content: [
        { type: 'text', text: '总结这份文档的重点。' },
        { type: 'file', file_id: fileId },
      ],
    },
  ],
});
```

- `file` 块的 `file_id` 与 `file_data` **恰好二选一**：`file_id` 引用上传的文件；`file_data` 是
  内联 base64 data URL（如 `data:image/jpeg;base64,...`，可带 `filename`），二者同时出现或都不给
  会 400。文档（PDF/Word/PPT 等）只能通过 `file_id` 引用，不支持请求里直接传文件内容或 URL。
- 单个请求最多引用 20 个文档、文档文本合计 ≤8 MiB；同一 `file_id` 重复引用分别计数。文档的
  完整解析内容会保留在多轮上下文里，超长直接 400（不会静默截断）。
- 上传为 `text_recognition` / `rich_layout_recognition` 的文档不能当图片用（会 400）；
  `rich_layout_recognition` 裁出的图会占用输入 token 与图片数量上限。
- Messages API 用 `document` 内容块：`{"type": "document", "source": {"type": "file", "file_id": "..."}}`，
  纯文本也可直接给 `{"type": "document", "source": {"type": "text", "media_type": "text/plain", "data": "..."}}`；
  对应类型为 `DeepSeekDocumentBlock` / `DeepSeekDocumentSource`（SDK 不调用 Messages API，仅供复用）。

```python
# Python 非流式：file_id 引用已上传文件
result = client.chat_completion(
    "company-bot",
    [{"role": "user", "content": [{"type": "text", "text": "总结这份文档的重点。"}, {"type": "file", "file_id": file_id}]}],
)
```

> dsh-ee 与 SDK 都不校验 `file_id` 是否存在、也不做上传代理；上游的错误（400/413/429 等）原样返回。
