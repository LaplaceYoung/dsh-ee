# 非 DSH 客户端接入（L1）：curl / 自研 Agent 怎么用

> 对应 e2e：`e2e/tests/28-l1-client.mjs`。这条链路不安装 DSH，只用**一个统一令牌**访问模型网关和 `/mcp`，适合 curl、脚本、自研 Agent 或企业集成方。
> 本文是 L1 的协议与端点参考：发现文档、令牌/scope、模型、MCP、错误码与限制。H1/H2/H3 分层、适配器与 Claude Code 等配置见 [harness-integration.md](./harness-integration.md)；产品内的开发入口速览见 [dev/harness.md](./dev/harness.md)。三篇分工不同，不要混读。

## 1. 准备令牌

在网页「我的令牌」生成，或由管理员在「令牌管理」下发。令牌以 `ee_at_` 开头，按用途最小化 scope：

| 用途 | 需要的 scope | 说明 |
|---|---|---|
| 读模型 / 调用模型 | `models` | `/v1/models`、`/v1/chat/completions`；账号本身还要有 `llm.use` 权限 |
| 使用 `/mcp`（企业连接工具、知识库与应用数据、`ee_*` 工具） | `connections` | **`/mcp` 入口按 `connections` 检查**：只有 `agent` 没有 `connections` 会 403 |
| 使用 `/mcp` 里的 `ee_*` Agent 工具 | `connections` + `agent` | 进 `/mcp` 仍要 `connections`；`agent` 决定 `ee_*` 工具出不出现，系统设置里的「Agent」总开关也要开着 |
| 读 `/v1/capabilities` 里的应用与 revision、`/v1/apps`（可选） | `hub` | 还要有工作台权限；第三方 Harness 不是必须依赖 |
| 安装 / 发布企业能力（可选） | `registry:read` / `registry:publish` | 走 `/npm/`，和 `/v1`、`/mcp` 相互独立 |

scope 只是上限：服务端每次请求都会把 scope 和账号 RBAC 求交，账号没有对应权限点（例如 `llm.use`）时仍然用不了。旧令牌（`ee_npm_`、`sk-ee-`）只为老 DSH 插件保留，第三方集成请一律用 `ee_at_`。

不要把管理员令牌写进共享脚本；给每个集成方单独发令牌，按需最小化 scope。

无人值守的集成方可以改用 API 客户端：管理员注册 `client_id` / `client_secret` 后，用
`POST /api/auth/client-token`（`client_credentials`）换 1 小时有效的 `ee_at_` 令牌，不需要人工维护令牌。
scope 取客户端注册范围 ∩ 所属账号当前权限，创建与换 token 的细节见
[token-lifecycle.md](./token-lifecycle.md) §6。

```sh
export BASE='https://dsh-ee.example.com'
export TOKEN='ee_at_…'
```

## 2. 发现与能力（中立契约 v1）

新增的中立端点（不改 DSH 原生入口 `/api/dsh/*`、`/mcp`、`/setup.*` 的形状）：

| 入口 | 认证 | 用途 |
|---|---|---|
| `GET /.well-known/dsh-ee` | 匿名 | 发现文档（`ETag`，`If-None-Match` → 304，`Cache-Control: private, max-age=300`） |
| `GET /v1/openapi.json` | 匿名 | 机器可读的 OpenAPI 3.1 契约（`ETag`，`public, max-age=300`） |
| `GET /v1/capabilities` | Bearer 可选 | 当前令牌的模型名单、`/mcp` 工具计数、微应用与技能摘要、目录 `revision`、scope；不带令牌只回公开子集（服务标识与契约版本）；`no-store`，非法令牌 401 不静默降级 |
| `GET /v1/apps`、`GET /v1/apps/{name}` | Bearer（`hub` + 工作台权限） | 只读的中立微应用描述（不含 DSH 专属字段）；看不到的 404 |

- 响应头 `Dsh-EE-API-Version: 1` 覆盖发现文档与所有 `/v1/*`（含 `openapi.json` 与既有模型网关；网关错误格式不变）；发现文档、`/v1/capabilities`、`/v1/apps*` 还接受同名请求头协商：缺省 / `1` / `1.x` 接受，不支持的主版本返回 400 `unsupported_api_version` + `supported_versions`。
- 中立端点的错误是统一壳 `{"error": {"type", "code", "message", "request_id", ...}}`，并带 `X-Request-Id`；`/api/*` 与 `/v1` 网关的既有错误壳保持不变（见 §7）。
- 发现文档的完整字段、缓存策略、设备码端点与令牌获取方式见文末「附录」。`scopes_supported` 是设备码可申请的 scope（`admin` 不在其中，只能网页限时开启）。

## 3. 读模型目录

```sh
curl -fsS "$BASE/v1/models" \
  -H "Authorization: Bearer $TOKEN" | jq '.data[].id'
```

返回的是 dsh-ee 的**对外模型名**，不是上游厂商的模型名；只返回额度与令牌允许的模型。

## 4. 调用模型

```sh
curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

协议是 OpenAI chat completions 子集：请求体原样转发上游，只改 `model`，流式时补 `stream_options.include_usage`（否则统计不到 token）；额度、审计、上游故障转移都由 dsh-ee 处理。不提供 `/v1/embeddings`、`/v1/responses`、`/v1/completions`。

错误沿用 OpenAI 的壳：`{"error":{"message","type","code"}}`（响应头仍带 `Dsh-EE-API-Version: 1`）。常见 `code` 见 §7。

## 5. MCP 握手、工具列表与调用

`POST /mcp` 是 JSON-RPC 2.0：

- **POST** 收 JSON-RPC，回 JSON（服务端不按 `Accept` 协商，带不带 `text/event-stream` 都回 JSON）；
- **GET** 打开事件流（需要 `Accept: text/event-stream`），能力目录变化时推 `notifications/tools/list_changed`，每 25 秒发一行注释保活；
- **DELETE** 结束（服务端没有会话状态，直接回 200）。

`initialize` 支持的协议版本是 `2025-11-25`、`2025-06-18`、`2025-03-26`（新的在前），请求里带哪个受支持的版本就回哪个；不认识的版本回最新的。响应里的 `Mcp-Session-Id` 只是下发，后续请求**不需要**回传——服务端每次按 Bearer 重新认证，保持无状态直调即可。

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}'
```

列工具。工具多时 `tools/list` 按 MCP 的 cursor 分页：第三方 Harness 不带参数时第一页默认 100 个工具、单页最多 500 个，响应带 `nextCursor`（不透明字符串，原样放进下一页的 `cursor` 即可，不要自己解析）；没有下一页时不返回 `nextCursor`。`limit` 是 dsh-ee 的扩展参数：可显式指定页大小，`limit: 0` 表示一次取全量。翻页途中工具清单有增删时旧 cursor 会失效并返回 `-32602`，重新从第一页开始即可。

DSH 接入插件（`X-DshEE-Client: dsh-connect/...`）不带 `cursor`/`limit` 时仍一次返回全量、顺序不变；显式给了 `cursor` 或 `limit` 才走分页。分页与写确认 `_meta` 的实现和兼容性说明见 [PR #128](https://github.com/hikariming/dsh-ee/pull/128)。

```sh
# 第一页（第三方 Harness，默认分页）
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'

# 下一页：把上一页返回的 nextCursor 原样传给 cursor
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{"cursor":"<nextCursor>","limit":100}}'
```

工具名与来源：

| 前缀 | 来源 |
|---|---|
| `连接名__工具名` | 企业连接（MCP 服务或 OpenAPI 接口，如 `crm__lookup`） |
| `kb__*` | 知识库：`list`、`browse`、`search`、`read`、`popular`、`verify` |
| `app__*` / `data__*` | 微应用自带数据表 / 数据中台 |
| `home__*` | 工作台只读：`apps`、`new`、`usage` |
| `ee_*` | 工作台 Agent 工具（需要 `agent` scope，见 §6） |

工具定义带 `annotations.readOnlyHint`：没标只读的一律当写操作。可见范围由令牌持有人的部门 / 角色 / 连接授权决定，同名工具在不同人那里可能不一样。

调用工具：

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kb__search","arguments":{"query":"住宿标准"}}}'
```

**写操作需要员工本人在网页确认**：调用方不是 DSH 时，没标只读的工具不会直接执行，`tools/call` 返回 `isError: true` 的文本结果，里面有确认链接（`/confirm/<id>`）。需要确认时，结果级 `_meta` 还会带结构化的确认信息：

```jsonc
{
  "content": [{"type": "text", "text": "…请在网页上确认…"}],
  "isError": true,
  "_meta": {
    "dsh-ee/confirmation": { "id": 123, "url": "https://<dsh-ee>/confirm/123", "expires_at": "2026-11-13T08:00:00Z" }
  }
}
```

`expires_at` 是 RFC3339 UTC，`url` 可直接给员工打开。文本与 `isError` 保持原样，旧客户端忽略 `_meta` 即可。员工在网页上核对参数并点「同意」后，用**完全相同的参数**再调一次才会执行；一次同意只能执行一次，参数变了要重新确认。实现细节见 [PR #128](https://github.com/hikariming/dsh-ee/pull/128)。

- 不要伪造 DSH 的客户端标识（`X-DshEE-Client: dsh-connect/...`）：它只该由 DSH 接入插件发送，第三方一律按普通 MCP 客户端处理。
- 判断调用是否成功看 `result.isError`，不是只看 HTTP 200；错误文案是给模型/员工看的中文，可以原样展示。
- 扩展返回的 `_meta["dsh-ee/error"].code`（如 `authorize`、`forbidden`）会被 dsh-ee 换成可读文案，一般不需要自己解析。

## 6. 工具命名与可见范围

- `ee_*` 工具按 `action` 参数区分子操作（例如 `ee_me`、`ee_hub`、`ee_tokens`）；管理类 `ee_*` 工具还需要令牌带 `admin` scope 且已开启管理模式（`admin_until > now`），否则不出现。
- 知识库、微应用、连接工具都按当前员工的权限过滤；同一份令牌在换人使用场景下不要共享。
- 没有企业连接、没有知识库时，`tools/list` 只返回当前可用的那部分工具，不会报错。

## 7. 常见错误

| 现象 | 原因 | 处理 |
|---|---|---|
| 401 `token_invalid` / `token_revoked` / `token_expired` | 令牌错误、过期或被吊销 | 重新生成；别复用旧值 |
| 403 `permission_denied` | 令牌缺 `models`，或账号没有 `llm.use` | 补 scope / 找管理员开权限 |
| 403 `model_not_allowed` | 用的模型不在账号或令牌的可用模型里 | 先 `GET /v1/models` 看对外名 |
| 403 `quota_expired` | 账号额度过期 | 找管理员续期 |
| 403「你的账号没有使用工作台的权限」 | 令牌缺 `/mcp` 需要的 `connections`，或账号没有 portal 权限 | 补 scope / 找管理员开权限 |
| 404 `model_not_found` | 对外模型没有可用上游，或用了上游名/未导入的模型 | 先 `GET /v1/models` 看对外名；找管理员检查模型接入 |
| 429 `code=rate_limit_exceeded`（`/v1`）或 JSON-RPC `-32000`（`/mcp`） | 该令牌的请求频率超过服务端限流 | 读 `Retry-After` 秒数退避；见下节 |
| 429 `code=monthly_quota_exceeded` 等 | 触发账号 / 令牌月度额度或上游限流 | 退避重试，或找管理员调额度 |
| 中立端点 400 `unsupported_api_version` | 请求头 `Dsh-EE-API-Version` 的主版本不是 1 | 去掉请求头（默认当前版本）或按 `supported_versions` 改 |
| 中立端点 403 `scope_denied` | `/v1/apps` 要求 `hub` scope + 工作台权限，缺任一项 | 补 scope / 找管理员开权限 |
| 中立端点 405（带 `Allow: GET, HEAD`） | 对中立端点用了 POST 等非 GET/HEAD 方法 | 改用 `GET` |
| `/mcp` JSON-RPC `-32602`「没有这个工具，或你没有使用它的权限」 | 工具名写错，或当前账号看不到这个工具 | 重新 `tools/list`；确认连接授权 |
| `/mcp` JSON-RPC `-32602`「cursor 无效」/「工具清单在翻页途中变了」 | 翻页时没有原样回传 `nextCursor`，或翻页途中工具增删 | 不带 `cursor` 从第一页重新取 |
| `/mcp` JSON-RPC `-32601` | 不支持的方法（目前有 `initialize`、`ping`、`tools/list`、`tools/call`，以及空的 `resources/list`、`resources/templates/list`、`prompts/list`） | 只用已实现的方法 |
| `/mcp` JSON-RPC `-32700` | 请求体不是合法 JSON-RPC | 检查请求体 |
| `tools/call` 返回 `isError:true` + 确认链接（`_meta["dsh-ee/confirmation"]` 带 `id`/`url`/`expires_at`） | 写操作还没经员工确认 | 让员工打开链接确认，再用相同参数重调 |

### 限流与请求 ID

`/v1/*` 与 `/mcp` 按令牌限流（同一令牌共用一份额度，不同令牌互不影响），默认每分钟 60 次；服务端可用 `DSH_EE_RATE_LIMIT` 调整或关闭，实际值以响应头为准。多实例部署时计数在服务端数据库里合计，额度不会按实例数量放大；窗口按分钟对齐，客户端按 `Retry-After` 退避即可。超限时：

- HTTP 429 + `Retry-After`（秒）；
- `X-RateLimit-Limit`（窗口额度）、`X-RateLimit-Remaining`（本次之后的剩余次数）、`X-RateLimit-Reset`（窗口重置的 Unix 秒）；
- 错误体形状不变：`/v1` 仍是 OpenAI 的 `{"error":{"message","type","code"}}`（`code=rate_limit_exceeded`），`/mcp` 仍是 JSON-RPC 错误对象（`code=-32000`）。

正常响应也会带 `X-RateLimit-Limit/Remaining/Reset`，客户端可以在 `Remaining` 接近 0 时主动降速。超限后按 `Retry-After` 退避，不要立即重试。

每个响应还带 `X-Request-Id`：请求里带了这个头就原样沿用（不覆盖），否则服务端生成。建议集成方自己生成并记录这个 id，出问题时把它连同时间、接口一起提供给管理员，服务端访问日志里能按同一个 id 找到请求。

## 8. 当前限制

- 统一错误壳与 `request_id` 字段目前只覆盖新增的中立端点（`/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps*`）；`/api/*` 仍是 `{"error":"中文"}`，`/v1` 网关仍是 OpenAI 壳，设备码是 `{"error":code}`。`X-Request-Id` 响应头已覆盖所有响应（见 §7「限流与请求 ID」）。
- CORS **默认关闭**：浏览器内的第三方 Harness 不能跨域直连，除非管理员用 `DSH_EE_CORS_ORIGINS` 配了白名单（逗号分隔的完整 origin，如 `https://harness.acme.internal`；`http://localhost[:port]` 作为本地联调例外）。白名单只对 `/v1/*` 与 `/mcp` 生效、不使用 cookie 凭据；未配置时行为与以前一样，不加任何 CORS 响应头。
- 契约版本目前只有主版本 `api_version: "1"`；弃用策略见附录，当前 v1 没有弃用计划。

## 9. 与 DSH 的关系

同一套令牌也可以给 DSH 的接入插件用；DSH 只是多了一层本机 profile 和 `/ee-login` 设备确认。非 DSH 客户端不需要装插件，但**没有** DSH 的技能目录、工作台、自动更新等能力，只覆盖 L1 的模型与 MCP。

安全提示：令牌等同密码；示例里的 `$TOKEN` 不要写进仓库、日志或截图。需要按人撤销时，用管理员「令牌管理」按前缀/用户吊销。

## 附录：发现文档字段与入口（契约 v1 速查）

`GET /.well-known/dsh-ee`（匿名）是运行时事实源：下面的地址、scope、模型与应用以它和 `/v1/capabilities` 的返回为准。从 0 接入的步骤（H1/H2、Claude Code / Codex 等配置）见 [harness-integration.md](./harness-integration.md)，本文只列协议字段，不重复接入流程。

### 发现文档字段

| 字段 | 说明 |
|---|---|
| `name` / `version` / `api_version` | 服务名（固定 `dsh-ee`）、构建版本、契约主版本（当前 `1`） |
| `auth` | `type:"bearer"`、`header:"Authorization: Bearer <token>"`、`token_types:["ee_at_"]`、`token_flows:["device_code","web_manual"]` |
| `device_authorization_endpoint` / `token_endpoint` | 设备码流程：`/api/device/code`、`/api/device/token`（公开） |
| `scopes_supported` | 设备码可申请的 scope（`admin` 只能网页限时开启，不在其中） |
| `models.protocol` / `models.streaming` / `models.endpoint` / `models.chat_completions_endpoint` | OpenAI chat completions 子集；模型列表与对话补全的实际地址 |
| `mcp.transport` / `mcp.endpoint` / `mcp.protocol_versions` | MCP Streamable HTTP 端点与支持的协议版本 |
| `apps.web` / `apps.data_tools` / `apps.endpoint` / `apps.web_url` | 微应用网页版（`/a/<slug>`）、数据工具、只读描述列表地址 |
| `capabilities_endpoint` | 当前令牌能力摘要 `/v1/capabilities` |
| `openapi_url` | 机器可读契约 `/v1/openapi.json`（OpenAPI 3.1，匿名只读） |
| `limits.max_request_bytes` / `limits.max_response_bytes` / `limits.max_mcp_request_bytes` | `/v1/chat/completions` 与 `/mcp` 的请求/响应体上限 |
| `errors.format` / `errors.documentation_url` / `errors.codes` | 中立统一错误壳形状、错误文档地址、当前稳定错误码 |

### 入口一览

| 入口 | 认证 | 用途 |
|---|---|---|
| `GET /.well-known/dsh-ee` | 匿名 | 发现文档（`ETag` + `private, max-age=300`） |
| `GET /v1/openapi.json` | 匿名 | 静态 OpenAPI 3.1 契约（`ETag` + `public, max-age=300`） |
| `GET /v1/capabilities` | Bearer 可选 | 当前令牌可用模型 / 工具计数 / 微应用 / 技能摘要 / revision |
| `GET /v1/apps`、`GET /v1/apps/{name}` | Bearer（`hub`） | 只读微应用描述 |
| `GET /v1/models` | Bearer（`models`） | 对外模型名 |
| `POST /v1/chat/completions` | Bearer（`models`） | 对话补全 |
| `/mcp`（POST / GET / DELETE） | Bearer（`connections`） | MCP JSON-RPC：初始化、工具列表与调用、事件流 |
| `POST /api/device/code`、`POST /api/device/token` | 匿名 | 设备码流程获取令牌 |

### 令牌获取方式

- `device_code`：`POST /api/device/code` 申请设备码，员工在浏览器打开返回的 `verification_uri_complete`（`/device?code=…`）确认，客户端再 `POST /api/device/token` 轮询领取；错误码 `authorization_pending` / `slow_down` / `access_denied` / `expired_token`，令牌只能取一次。scope 用发现文档的 `scopes_supported`。
- `web_manual`：员工在网页「我的令牌」生成，或管理员在「令牌管理」下发（需要网页会话，没有程序化签发/续期 API）。第三方长期集成建议按集成方单独发 `ee_at_` 令牌、最小化 scope。

### 版本、缓存与弃用

- 响应头 `Dsh-EE-API-Version: 1` 覆盖发现文档与所有 `/v1/*`；新增中立端点还接受请求头 `Dsh-EE-API-Version` 协商（缺省/`1`/`1.x` 接受，不支持的主版本返回 400 `unsupported_api_version` + `supported_versions`）。
- 发现文档带按请求推断的端点地址，只允许私有缓存（`private, max-age=300`）；`/v1/openapi.json` 是静态内容，可共享缓存（`public, max-age=300`）；`/v1/capabilities`、`/v1/apps*` 与令牌/权限相关，`Cache-Control: no-store`。
- 弃用策略：破坏性变更升 `api_version` 主版本、旧主版本保留，届时再通过 `Deprecation` / `Sunset` 响应头与变更日志通知。当前 v1 **没有**弃用计划，服务端不发送这些头，发现文档也不再声明该字段，避免虚假承诺。
- 每令牌限流与 `X-RateLimit-*` / `Retry-After` 已实现（见 §7「限流与请求 ID」）；发现文档的 `limits` 只描述请求 / 响应体上限，限流参数不在发现文档里声明。
