# @dsh-ee/harness-client

dsh-ee 的第三方 Harness 客户端（TypeScript / ESM），基于 `fetch`，**零运行时依赖**。

它把第三方 Harness / 自研 Agent 接入 dsh-ee 时要用到的 L1 协议面收在一个客户端里：

| 能力 | 端点 | 说明 |
|---|---|---|
| 服务发现 | `GET /.well-known/dsh-ee` | 版本、设备码端点、支持的 scope、模型/MCP/微应用能力、限制 |
| 能力摘要 | `GET /v1/capabilities` | 当前令牌可用模型、MCP 工具数、微应用、目录 revision |
| 模型目录 | `GET /v1/models` | dsh-ee 的**对外模型名**（不是上游厂商模型名） |
| 模型调用 | `POST /v1/chat/completions` | OpenAI chat completions 子集；支持非流式与 SSE 流式 |
| 思考模式 | `POST /v1/chat/completions` | `thinking` / `reasoning_effort` 原样转发；`reasoning_content` 原样返回 |
| DeepSeek Chatbot | `POST /v1/chat/completions` | server tools（`deepseek_web_search` / `deepseek_read_url`）、`safety_check`、`title` 原样转发；工具过程/引用/拒答/标题原样返回（不执行工具） |
| 设备码登录 | `POST /api/device/code`、`POST /api/device/token` | OAuth 风格设备码；SDK 负责轮询与 `slow_down` |
| MCP | `POST /mcp` | initialize、`tools/list` cursor 分页、`tools/call`、写确认 `_meta` |
| 可观测 | 响应头 | `X-Request-Id`、`Dsh-EE-API-Version`、`X-RateLimit-*`、`Retry-After` |

> 状态：`0.1.0`，随本仓库分发，**未发布到 npm**。API 会跟随 dsh-ee 的 v1 契约演进。
>
> 这是通用 HTTP/MCP 客户端，**不实现** Anthropic Messages、OpenAI Responses 等协议，
> 也不声称与 Claude Code / Codex 的专有协议兼容；模型面只覆盖 dsh-ee 转发的
> OpenAI chat completions 子集。MCP 面只实现客户端常用的 initialize / ping /
> `tools/list` / `tools/call`（dsh-ee 的 resources/prompts 目前返回空列表）。

## 依赖的服务端契约

SDK 按下列 dsh-ee 变更实现（均已在服务端 PR 中）：

| 能力 | 服务端变更 |
|---|---|
| 发现 / 能力 / API 版本头 / 中立错误壳 | #123：`/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps`、`Dsh-EE-API-Version: 1`、`X-Request-Id` |
| MCP `tools/list` 分页、写确认结果级 `_meta` | #128：默认 100/页、上限 500、`limit: 0` 取全量、`nextCursor` 不透明；`_meta["dsh-ee/confirmation"]` |
| 每令牌限流与请求号 | #129：`/v1/*` 与 `/mcp` 的 `X-RateLimit-*`、`Retry-After`、`X-Request-Id` |
| 令牌 `client_kind` | #132：第三方令牌应为 `third_party`，写操作始终走网页人工确认 |

部署较旧、没有 #123 时 `discovery()` / `capabilities()` 会返回 404；模型、chat、设备码、
MCP 等基础端点仍可用（`tools/list` 会是旧行为：不带分页）。

## 构建与测试

```sh
cd sdk/typescript
npm install          # 只装 devDependency: typescript
npm run build        # 产物在 dist/，不入库
npm test             # 构建后跑 node:test，全部 mock fetch，不访问网络
npm pack --dry-run   # 检查发布包内容（不会发布，也不会留下 tarball）
```

要求 Node.js >= 18（自带 `fetch` / `ReadableStream` / `TextDecoder`）；浏览器环境也可用，
但需要服务端配置 CORS 白名单（见下文「浏览器内使用」）。

本包尚未发布到 npm。在当前仓库内使用可以：

```sh
# 方式一：本地安装（构建产物在 dist/）
cd sdk/typescript && npm install && npm run build
cd /path/to/your-app && npm install /path/to/dsh-ee/sdk/typescript

# 方式二：打包后安装
cd sdk/typescript && npm pack
cd /path/to/your-app && npm install /path/to/dsh-ee/sdk/typescript/dsh-ee-harness-client-0.1.0.tgz
```

## 快速开始

```ts
import { DshHarnessClient } from '@dsh-ee/harness-client';

const client = new DshHarnessClient({
  baseUrl: process.env.DSH_EE_BASE ?? 'https://dsh-ee.example.com',
  token: process.env.DSH_EE_TOKEN, // ee_at_…
});

const [discovery, capabilities, models] = await Promise.all([
  client.discovery(),
  client.capabilities(),
  client.listModels(),
]);

console.log(discovery.version, discovery.mcp.protocol_versions);
console.log(capabilities.user?.username, capabilities.tools.count);
console.log(models.map((model) => model.id));
```

`baseUrl` 是 dsh-ee 的部署地址；`token` 可以之后通过 `setToken()` 设置，或通过设备码登录获得。

### 非流式模型调用

```ts
const completion = await client.createChatCompletion({
  model: 'company-chat',
  messages: [{ role: 'user', content: 'ping' }],
  temperature: 0.2,
});

console.log(completion.choices[0]?.message.content);
console.log(completion.usage?.total_tokens);
```

请求体按 OpenAI chat completions 的形状原样转发（`model` / `messages` 必填）；
dsh-ee 的额度、审计、上游故障转移都在服务端处理。

### 流式模型调用

```ts
for await (const chunk of client.streamChatCompletion({
  model: 'company-chat',
  messages: [{ role: 'user', content: '写一段自我介绍' }],
})) {
  const text = chunk.choices[0]?.delta?.content;
  if (text) process.stdout.write(text);
  if (chunk.usage) console.log('\nusage:', chunk.usage.total_tokens);
}
```

服务端会强制 `stream_options.include_usage = true`，最后一块通常带 `usage`。
流式请求默认没有整条流的总超时（长回答可能持续很久），用 `signal` 取消；
需要总超时可传 `streamChatCompletion(req, { timeoutMs: 120_000 })`。

### 思考模式（thinking / reasoning_effort）

dsh-ee 网关把思考模式参数**原样转发**给上游，不做白名单/映射；能力以服务端公布为准：

- 发现文档 `models.reasoning`、`GET /v1/capabilities` 的 `reasoning`：参数名（`thinking` /
  `reasoning_effort`）与响应字段（`reasoning_content`）。
- `GET /v1/models` 每项的 `capabilities.reasoning`：该模型是否支持、可用哪些
  `reasoning_effort`（`efforts`，可能是 `minimal` / `low` / `medium` / `high` / `xhigh` /
  `max` / `ultra` 等上游值）。

```ts
// 非流式：响应里的 message.reasoning_content 是思维链，content 是最终回答。
const result = await client.createChatCompletion({
  model: 'company-reasoner',
  messages: [{ role: 'user', content: '9.11 和 9.8 哪个大？' }],
  thinking: { type: 'enabled' },
  reasoning_effort: 'high',
});
const reasoning = result.choices[0]?.message.reasoning_content;
```

流式时思维链在 `delta.reasoning_content` 里逐块返回，跨 chunk 拼起来即可：

```ts
let reasoningText = '';
for await (const chunk of client.streamChatCompletion({
  model: 'company-reasoner',
  messages: [{ role: 'user', content: '9.11 和 9.8 哪个大？' }],
  thinking: { type: 'enabled' },
  reasoning_effort: 'medium',
})) {
  reasoningText += chunk.choices[0]?.delta?.reasoning_content ?? '';
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '');
}
```

**工具调用轮次必须回传历史 `reasoning_content`**：请求带 `tools` 时，后续请求要把历史
assistant 消息的 `reasoning_content` 原样带上（即使该轮没有实际调用工具），否则上游返回 400
（`The reasoning_content in the thinking mode must be passed back to the API.`）。不带 `tools`
的多轮对话可以不回传，上游会忽略。dsh-ee 网关不补、不删、不校验这个字段，保留它是调用方的责任：

```ts
const messages: ChatMessage[] = [{ role: 'user', content: '杭州明天天气？' }];
const first = await client.createChatCompletion({
  model: 'company-reasoner',
  messages,
  tools,
  thinking: { type: 'enabled' },
  reasoning_effort: 'high',
});
// 非流式直接用返回的 message 追加：content / reasoning_content / tool_calls 都在里面。
messages.push(first.choices[0].message);
// ...执行 tool_calls 后追加 role: 'tool' 消息，再发下一轮；历史 assistant 的 reasoning_content 会一起回传。
```

### DeepSeek Chatbot 扩展（server tools / safety / title / 拒答）

DeepSeek 的 Chatbot 能力通过 chat completions 扩展字段声明与返回，dsh-ee 网关原样透传，
**SDK 不执行服务端工具逻辑**（搜索/读链接由上游完成）。完整示例见
[`../examples/chatbot-server-tools.md`](../examples/chatbot-server-tools.md)。

```ts
const result = await client.createChatCompletion({
  model: 'company-bot',
  messages: [{ role: 'user', content: 'DeepSeek V4.1 有什么新特性？' }],
  tools: [{ type: 'deepseek_web_search' }, { type: 'deepseek_read_url' }],
  safety_check: { level: 'high' },
  title: { enabled: true },
});

const message = result.choices[0]?.message;
console.log(message?.title); // 会话标题（非流式 message.title；流式最后一块 delta.title）
for (const step of message?.server_tool_calls ?? []) {
  for (const call of step.calls) console.log(call.kind, call.action, call.status, call.signature);
}
for (const annotation of message?.annotations ?? []) console.log(annotation.url_citation.url);
if (result.choices[0]?.finish_reason === 'content_filter') console.log('拒答:', message?.refusal);
```

- 多轮：把上一轮 assistant 消息原样回传，**保留 `server_tool_calls` 里的 `kind` 与 `signature`**；
  丢掉 `signature` 时请求仍成功，但之前读过的页面不会进入上下文。
- 流式：工具进度在 choice 级 `server_tool_event`，完整 `server_tool_calls` / `annotations` 在带
  `finish_reason` 的最后一块 `delta` 里；`delta.title` 也在最后一块。
- 拒答：`finish_reason: "content_filter"` + `message.refusal`（被拦截图片说明）；移除已展示回答。
- 类型命名：Chat Completions 扩展类型以 `DeepSeek*` 开头（`DeepSeekServerToolCall` /
  `DeepSeekServerToolStep` / `DeepSeekUrlCitation` / `DeepSeekRefusal` /
  `DeepSeekStopDetails` 等）；Messages API 的响应块（`server_tool_use` /
  `web_search_tool_result` / `web_fetch_tool_result` / citations）也提供同名类型，便于统一渲染。

#### 用户元信息标记（`latest_reminder`）

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

`formatLatestReminder` 生成 `<date>,<weekday>,<location>,<platform>,<language>`；
`location` 可为空、允许含逗号。消息必须紧贴对应的 `user` 消息之前（上游校验，400 表示位置/格式
不对）；历史轮次的 reminder 可原样带回以提高 context cache 命中。类型：
`DeepSeekLatestReminder` / `DeepSeekLatestReminderMessage` / `ChatRole`。

#### 文件 / 文档输入（DeepSeek Files API）

文件需先上传到 **DeepSeek Files API**（`POST /files`，`purpose` = `user_data` /
`text_recognition` / `rich_layout_recognition`）并解析拿到 `file-api-...`，再在 chat 请求里引用。
**dsh-ee 的 `/v1` 只透传 chat 请求，不代理 Files API；本 SDK 不实现 Files API**。

```ts
await client.createChatCompletion({
  model: 'company-bot',
  messages: [
    {
      role: 'user',
      content: [
        { type: 'text', text: '总结这份文档的重点。' },
        { type: 'file', file_id: 'file-api-xxxxxxxxxxxxxxxx' }, // 文档只能这样引用
      ],
    },
  ],
});
```

- `file` 内容块：`file_id` 与 `file_data` 恰好二选一。`file_data` 是内联 base64 data URL
  （`data:image/jpeg;base64,...`，可带 `filename`）；文档（PDF/Word/PPT 等）只能通过 `file_id` 引用。
- Messages API 形状的 `document` 块（`DeepSeekDocumentBlock` / `DeepSeekDocumentSource`：
  `source.type = "file"` 或 `"text"`）也提供类型，便于统一渲染或对接 Messages API。
- 限制（上游）：单请求最多 20 个文档、文本合计 ≤8 MiB，超长 400；文档上传 ≤100 MiB、≤1000 页，
  上传/解析各 120 秒内完成；`rich_layout_recognition` 裁出的图占输入 token 与图片数量上限。

## 设备码登录

```ts
import { DeviceFlowError, DshHarnessClient } from '@dsh-ee/harness-client';

const client = new DshHarnessClient({ baseUrl: process.env.DSH_EE_BASE! });

const token = await client.loginWithDeviceCode(
  { clientName: 'my-harness', clientVersion: '1.0.0' },
  {
    onCode: (code) => {
      // 把 verification_uri_complete 展示/打开给用户，或提示输入 user_code
      console.log(`请在浏览器打开 ${code.verification_uri_complete}`);
    },
    onPending: ({ code }) => console.log(`等待确认（${code}）…`),
  },
);

console.log(token.user.username, token.expires_at);
// 登录成功后 client 已带令牌，后续 listModels() / connectMcp() 直接可用
```

也可以手动控制：

```ts
const code = await client.requestDeviceCode({ scopes: ['models', 'agent'] });
// …展示给用户…
const token = await client.waitForDeviceToken(code, { intervalMs: code.interval * 1000 });
client.setToken(token.access_token);
```

`waitForDeviceToken` 会处理 `authorization_pending`、`slow_down`（自动 +5 秒）、
`rate_limited` 与最终过期；`access_denied` / `expired_token` 立即抛 `DeviceFlowError`。

设备码申请的 scope 受服务端白名单限制：`models`、`registry:read`、`registry:publish`、
`hub`、`connections`、`agent`；不传时 dsh-ee 使用除 `registry:publish` 外的默认组合。
不要把管理员权限写进第三方 Harness 的令牌。

## MCP：初始化、工具列表与调用

```ts
const session = await client.connectMcp();

console.log(session.serverInfo);      // { name: 'dsh-ee', version: '…' }
console.log(session.instructions);    // dsh-ee 给的连接说明
console.log(session.sessionId);       // initialize 响应头里的 Mcp-Session-Id

const tools = await client.listAllTools();  // 自动跟随 nextCursor 取全量
for (const tool of tools) console.log(tool.name, tool.annotations?.readOnlyHint);

await client.close();                 // DELETE /mcp，尽力而为
```

### 分页

第三方 Harness 默认按页取工具（dsh-ee 默认 100/页、单页上限 500）：

```ts
const firstPage = await client.listTools();                 // 默认页
const nextPage = await client.listTools({ cursor: firstPage.nextCursor });
const everything = await client.listTools({ limit: 0 });    // 一次取全量
```

`nextCursor` 是不透明字符串，**不要解析**，原样回传即可；翻页途中工具清单变化时
服务端会让旧 cursor 失效（SDK 会收到 `-32602`，重新从头取）。
`sdk` 的 `listAllTools()` 会检测重复 cursor 并在异常时停止，避免死循环。

### 调用与写确认

```ts
import { extractConfirmation, isPendingConfirmation } from '@dsh-ee/harness-client';

const result = await client.callTool('crm__lookup', { q: '张三' });
if (result.isError) {
  if (isPendingConfirmation(result)) {
    const confirmation = extractConfirmation(result);
    // 展示 confirmation.url（validate 参数后点「同意」），expires_at 内有效
    console.log(`请确认：${confirmation?.url}（${confirmation?.expires_at} 前有效）`);
    // 用户确认后，用完全相同的参数再调一次才会真正执行；参数变了要重新确认
  } else {
    console.error(result.content.map((item) => item.text).join('\n'));
  }
}
```

dsh-ee 的写操作在需要人工确认时**不会执行**：结果保持旧的 `content` / `isError` 文本，
并在结果级 `_meta["dsh-ee/confirmation"]` 里给出 `{ id, url, expires_at }`。
`extractConfirmation()` 会把它解析成 `DshConfirmation`（`expiresAt` 是 `Date`）。
旧客户端可以忽略 `_meta`，行为不变。

### 自己控制握手

```ts
const session = await client.connectMcp({ initialize: false });
await session.start();
await session.ping();
await session.callTool('ee_me', {});
await session.close();
```

## 错误与限流

```ts
import {
  DeviceFlowError,
  DshApiError,
  DshJsonRpcError,
  DshNetworkError,
} from '@dsh-ee/harness-client';

try {
  await client.listModels();
} catch (error) {
  if (error instanceof DshApiError || error instanceof DshJsonRpcError) {
    console.error('status:', error.status);
    console.error('request id:', error.requestId);
    console.error('message:', error.message);
    if (error.isRateLimited) {
      // 服务端 429 会带 Retry-After；没有时可用 rateLimit.resetAt 估算
      const waitMs = (error.retryAfter ?? 60) * 1000;
      await new Promise((resolve) => setTimeout(resolve, waitMs));
    }
  }
}
```

错误类型：

| 类型 | 场景 | 关键字段 |
|---|---|---|
| `DshApiError` | HTTP 非 2xx（OpenAI 壳 / 中立错误壳 / 通用 `{"error":"…"}`） | `status`、`apiType`、`apiCode`、`requestId`、`retryAfter`、`rateLimit` |
| `DshJsonRpcError` | MCP JSON-RPC error（HTTP 200 的 error body，或 429 的 `-32000`） | `rpcCode`、`rpcData`、`status`、`retryAfter`、`rateLimit` |
| `DshNetworkError` | fetch 失败 / 超时 / 调用方取消 | `networkCode: 'network' \| 'timeout' \| 'aborted'` |
| `DeviceFlowError` | 设备码轮询业务错误 | `flowCode`、`retryable` |

所有错误都继承 `DshError`，带可选的 `status`、`requestId`、`apiVersion`、`rateLimit`、
`retryAfter`、`body` 与 `isRateLimited`。

限流头也可以直接解析：

```ts
import { parseRateLimitHeaders, parseRetryAfter } from '@dsh-ee/harness-client';

const info = parseRateLimitHeaders(response.headers); // { limit, remaining, reset, resetAt } | undefined
const seconds = parseRetryAfter(response.headers);    // number | undefined
```

`X-RateLimit-*` 的 `reset` 是 Unix 秒；`RateLimitInfo.resetAt` 已换算成 `Date`。
同一令牌在 `/v1/*` 与 `/mcp` 共用一份额度（服务端行为），SDK 没有自动重试：
写操作 / 工具调用是否重试应由调用方按幂等性决定。

## 观测与调试

```ts
const client = new DshHarnessClient({
  baseUrl,
  token,
  onResponse: (info) => {
    // info: { status, ok, requestId, apiVersion, rateLimit, retryAfter, contentType, method, path }
    console.debug(`${info.method} ${info.path} ${info.status} request-id=${info.requestId}`);
  },
});

await client.listModels();
console.log(client.lastRequestId, client.lastRateLimit?.remaining);
```

## 浏览器内使用

dsh-ee 默认不开启 CORS；浏览器内 Harness 需要管理员配置
`DSH_EE_CORS_ORIGINS`（逗号分隔的完整 origin）。允许后跨域预检（OPTIONS）不消耗令牌额度，
服务端会暴露 `X-Request-Id` / `X-RateLimit-*` / `Retry-After` 等响应头，SDK 可正常读取。
Bearer 令牌只放在 `Authorization` 头里，不使用 Cookie 凭据；不要把它写进
`localStorage` 以外的地方（按企业安全要求选择内存/会话存储）。

## 安全建议

- 令牌等同密码：不要写进仓库、日志、截图；每个集成方单独签发，按需最小化 scope。
- 第三方 Harness 建议由管理员签发 `client_kind: "third_party"` 的令牌（#132）：
  写操作始终要求员工在网页确认，服务端不再采信客户端自报的 `X-DshEE-Client`。
- 设备码登录默认签发 `dsh` 属性令牌（DSH 自己带审批卡的快捷语义）；
  自研 Harness 如果不想被当作 DSH，应使用管理员签发 / 网页创建的第三方令牌。
- 生产环境走 HTTPS；SDK 不会把令牌写入磁盘或日志（是否记录响应由 `onResponse` 使用方决定）。

## API 摘要

```ts
new DshHarnessClient({ baseUrl, token?, fetch?, headers?, timeoutMs?, apiVersion?,
                       mcpProtocolVersion?, mcpClientInfo?, onResponse?, sleep? })
client.setToken(token) / client.hasToken
client.discovery({ refresh? })          // 默认 ETag 缓存
client.capabilities()                   // 无令牌时返回公开子集
client.listApps() / client.getApp(name)
client.listModels()
client.createChatCompletion(req)
client.streamChatCompletion(req, opts?) // AsyncGenerator<ChatCompletionChunk>
client.requestDeviceCode(req?)
client.pollDeviceToken(deviceCode)      // 单次轮询
client.waitForDeviceToken(code, opts?)  // 轮询直到确认/过期
client.loginWithDeviceCode(req?, opts?) // 申请 + 等待 + 保存令牌
client.connectMcp(opts?)                // 默认自动 initialize
client.listTools(opts?) / client.listAllTools(opts?) / client.callTool(name, args?, opts?)
client.close()                          // 关闭 MCP 会话
```

`DshClientOptions` 常用项：

| 选项 | 默认 | 说明 |
|---|---|---|
| `baseUrl` | 必填 | dsh-ee 部署地址 |
| `token` | — | 统一令牌 `ee_at_…` |
| `fetch` | 全局 fetch | 注入自定义 fetch（代理、测试） |
| `headers` | `{}` | 附加到每个请求的默认头 |
| `timeoutMs` | `60000` | 单次请求超时；`0` 表示不限时 |
| `apiVersion` | `"1"` | `Dsh-EE-API-Version` 请求头；`null` 不发送 |
| `mcpProtocolVersion` | `"2025-06-18"` | MCP initialize 期望版本 |
| `mcpClientInfo` | SDK 自身标识 | initialize 上报的客户端名/版本 |
| `onResponse` | — | 每个响应的观测回调 |
| `sleep` | 真实等待 | 覆盖设备码轮询 sleep（测试用） |

## 测试与验证

- `npm test`：33 个用例，全部使用 mock `fetch`，覆盖发现/ETag 缓存、能力、模型、chat
  非流式/流式（含跨 chunk SSE 与 CRLF）、设备码全流程、错误壳、429 限流头、
  MCP initialize/会话头/分页/写确认 `_meta`/JSON-RPC 错误/SSE 响应/关闭。
- `npm pack --dry-run`：包内只含 `dist/`、`README.md`、`LICENSE`、`package.json`，
  无测试与源码（源码随仓库）。
- 未连接真实服务端做端到端测试；契约按 #123 / #128 / #129 / #132 的服务端实现与
  `docs/l1-client.md` 核对。发布前建议对目标部署跑一次冒烟（discovery + models +
  MCP tools/list）。

## 已知边界

- 不自动重试（429/网络错误交给调用方，避免写操作重复执行）。
- 不实现 MCP 的 GET 事件流（`notifications/tools/list_changed` 的持久订阅）、
  resources/prompts 的完整客户端、sampling/roots。
- 不实现刷新令牌；设备码令牌过期后重新登录或由管理员重新签发。
- 设备码轮询的等待/过期时间在客户端本地计算，服务端的 `expires_in` 与 `interval` 为准。
- 不会向 `X-DshEE-Client` 发送 `dsh-connect/…`（那是 DSH 接入插件标识，会影响
  `tools/list` 的全量/分页语义）；SDK 走第三方 Harness 的默认分页行为。

## 示例与兼容性

- 示例（MCP-only Claude Code / Codex、通用 Harness、浏览器 CORS）：[`../examples/`](../examples/README.md)
- 与 dsh-ee API 版本、MCP、工具面的兼容矩阵：[`COMPATIBILITY.md`](./COMPATIBILITY.md)
- 各版本变更记录：[`CHANGELOG.md`](./CHANGELOG.md)
- 发布元数据：`publishConfig.access = "public"`、`repository`/`homepage`/`bugs` 指向
  <https://github.com/hikariming/dsh-ee>；本包**尚未发布**。

## License

Apache-2.0，见包内 [LICENSE](./LICENSE) 与仓库根 `LICENSE`。
