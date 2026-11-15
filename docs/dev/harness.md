---
title: Harness 接入
status: draft
since: 2026-11-12
lang: zh-CN
---

# Harness 接入

「Harness」指调用 dsh-ee 的客户端运行时：DSH、Claude Code、Codex、自研 Agent 或脚本。dsh-ee 对外提供统一的**接入令牌**，让非 DSH 客户端也能用模型网关和 MCP 工具；DSH 另外有接入插件负责登录确认和凭据保管。

## 1. 两条接入路径

| 路径 | 适合 | 入口 |
|---|---|---|
| 非 DSH 客户端（L1） | curl、脚本、自研 Agent、企业集成 | 统一令牌 → `/v1` 模型 + `/mcp` 工具；见 [l1-client.md](../l1-client.md) |
| DSH 接入插件 | 员工在 DSH 里使用 dsh-ee 的模型、工具、技能 | `dsh-plugin/`，用 `/ee-login` 设备确认登录拿到企业能力 |

同一条链路对 DSH 也开放：DSH 只是多了一层本机 profile 和登录确认，非 DSH 客户端不需要装插件，但也不会有技能目录、工作台、自动更新等能力，只覆盖 L1 的模型与 MCP。

本文是**产品内速览**；第三方接手的完整手册（H1/H2/H3 分层、适配器写法、写确认、最小验证清单）见 [harness-integration.md](../harness-integration.md)（companion PR #117 引入），两篇分工，不重复细节。

## 2. 令牌与 scope

在网页「我的令牌」生成，或由管理员在「令牌管理」下发。令牌以 `ee_at_` 开头，按用途最小化 scope：

| 用途 | scope |
|---|---|
| 读模型 / 调用模型 | `models`（账号本身还需要 `llm.use`） |
| 使用 `/mcp`（企业连接、知识库、应用数据、`ee_*`） | `connections`（`/mcp` 入口按它检查） |
| 使用 `/mcp` 里的 `ee_*` Agent 工具 | 额外 `agent`（还要打开系统设置里的「Agent」开关） |
| 读能力目录（可选，DSH 插件用） | `hub` |

scope 只是上限：服务端每次请求还会把 scope 和账号 RBAC 求交，账号没有对应权限点时仍然用不了。只有 `agent`、没有 `connections` 的令牌进不了 `/mcp`（403）。旧令牌（`ee_npm_`、`sk-ee-`）只为老 DSH 插件保留，第三方集成一律用 `ee_at_`。

不要给多个集成方共用同一枚管理员令牌；按集成方单独发、单独吊销。令牌等同密码，不写进仓库、日志、截图。

## 3. 模型网关

对外是 OpenAI chat completions 子集，模型名用 dsh-ee 的**对外模型名**，不是上游厂商名：

```sh
export BASE='https://dsh-ee.example.com'
export TOKEN='ee_at_…'

curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"

curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"company-chat","messages":[{"role":"user","content":"ping"}]}'
```

额度、审计、上游故障转移由 dsh-ee 处理；错误与常见原因见 [l1-client.md](../l1-client.md) 的「常见错误」表。

**上游共享文件（DeepSeek `file_id`）默认不开放**：DeepSeek Files 是上游账号级资源，员工令牌与管理员共用上游 key，所以 dsh-ee 在转发前扫描请求里的文件引用（`file` 块的 `file_id`、`document` / `image` 的 `source.file_id`）。普通员工默认 403 `file_access_denied` 且不请求上游；管理员（账号有任一管理权限）与 `account_type=bot` 的机器人账号放行。需要全体员工引用共享文件时显式设置 `DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE=true`（默认 `false`，fail closed）。覆盖 `/v1/chat/completions` 与 `/v1/messages`；后续新增的模型入口必须复用同一判定。详见 [Anthropic Messages 兼容](./anthropic-api.md#2-鉴权与权限)。

同一组模型也有 **Anthropic Messages 兼容端点** `POST /v1/messages`（别名 `POST /anthropic/v1/messages`，
兼容 DeepSeek 的 `ANTHROPIC_BASE_URL=<base>/anthropic` 习惯）：Claude Code / Anthropic SDK 用
`x-api-key` 或 `Authorization: Bearer` 带上同一个企业令牌即可，模型名仍填 dsh-ee 的对外模型名
（不做 `claude-*` 名称映射）。优先线路是 DeepSeek provider 时走原生透传（请求/响应逐字节转发），其它 provider 降级转成 OpenAI chat；
模型权限、额度与用量记账与上面完全一致。请求/响应/SSE 的字段映射、原生透传与降级转换的区别、
Anthropic 错误壳和 v0 限制见 [Anthropic Messages 兼容](./anthropic-api.md)。

## 4. MCP 工具

`/mcp` 是 JSON-RPC 2.0（Streamable HTTP），服务端**无状态**：

- **POST** 收发 JSON-RPC，回 JSON；服务端不按 `Accept` 协商，请求头带不带 `text/event-stream` 都回 JSON。
- **GET** 打开事件流（需要 `Accept: text/event-stream`），能力目录变化时推 `notifications/tools/list_changed`，每 25 秒发一行注释保活。
- **DELETE** 结束；没有会话状态，直接回 200。

`initialize` 支持 `2025-11-25`、`2025-06-18`、`2025-03-26`（新的在前），请求里带哪个受支持的版本就回哪个；不认识的版本回最新的。响应里的 `Mcp-Session-Id` 只是下发，后续请求**不需要**回传——每次按 Bearer 重新认证即可。

```sh
# 握手
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-agent","version":"1.0"}}}'

# 列工具（默认 100/页；带 cursor 取下一页，limit: 0 取全量）
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

`tools/list` 默认分页：一页 100 个、单页上限 500；结果带 `nextCursor` 就把它原样填进 `params.cursor` 取下一页，直到没有 `nextCursor`。想一次拿全量传 `limit: 0`；工具清单在翻页途中变化会让旧 cursor 失效。DSH 接入插件（`X-DshEE-Client: dsh-connect/…`）不带参数时仍一次拿全量，行为不变。

工具名前缀：`连接名__工具名`（企业连接，如 `crm__lookup`）、`kb__*`（知识库）、`app__*` / `data__*`（微应用自带数据表 / 数据中台）、`home__*`（工作台只读）、`ee_*`（Agent 工具）。具体能调到哪些工具，由令牌持有人的部门、角色、连接授权决定，不是令牌里写死的。

没标只读的工具是**写操作**：非 DSH 客户端调用时不会直接执行，`tools/call` 返回 `isError: true` 的文本结果并带 `/confirm/<id>` 确认链接；结果级 `_meta["dsh-ee/confirmation"]` 还带结构化信息 `{id, url, expires_at}`（`expires_at` 是 RFC3339 UTC），适配器可以直接用 `url` 渲染确认按钮，不读 `_meta` 的客户端继续用文本里的链接也兼容。员工在网页确认后，用**完全相同的参数**再调一次才会执行；一次同意只能执行一次，10 分钟内有效。判断调用是否成功看 `result.isError`，不能只看 HTTP 200；不要伪造 DSH 的 `X-DshEE-Client` 头。完整说明与错误表见 [l1-client.md](../l1-client.md)。

## 5. DSH 接入插件

DSH 走接入插件：首次接入由 `setup.sh` 在浏览器里确认登录，之后令牌失效时在 DSH 对话框输入 `/ee-login` 重新确认。登录一次后，模型、技能、企业连接按令牌的能力范围直接可用；令牌由 DSH 的凭据库保管，不写进 `.env` 或项目配置。

- 接入插件的安装与更新由 DSH 负责，dsh-ee 只提供登录确认和能力下发。
- DSH 自己有审批卡；端侧扩展的写操作确认在 `non_dsh` 档下对 DSH 不再重复确认，见 [extension-local.md](../extension-local.md)。
- 插件源码在 [`dsh-plugin/`](../../dsh-plugin)；它和 dsh-ee 服务端同仓维护，接口随版本变化。产品设计见 [agent-connect-product.md](../agent-connect-product.md)。

## 6. 第三方 Harness 的中立契约与边界

v1 的第三方中立契约（以 `/v1`、`/mcp` 与 `/.well-known/dsh-ee` 为准）：

- `GET /.well-known/dsh-ee`：公开发现文档，给出服务名/版本、契约版本、设备授权与令牌端点、支持的 scope、模型/MCP/微应用能力、限额与弃用头；模型能力里同时给出 `anthropic_messages_endpoint`（Anthropic Messages 兼容入口）；带 `ETag`，可私有缓存 5 分钟。
- `GET /v1/capabilities`：当前令牌的能力摘要（模型名单、`/mcp` 工具计数、微应用与目录 revision、scope）；不带令牌时只返回公开子集（服务标识与契约版本）。
- `GET /v1/apps`、`GET /v1/apps/{name}`：只读的中立微应用描述（需要令牌 + `hub` scope）。
- 版本、错误与请求号：以上端点及 `/v1/*` 的响应都带 `Dsh-EE-API-Version: 1`；请求可以用同名头声明主版本；中立端点的错误统一为 `{"error": {...}}` 并带 `X-Request-Id`（#129 合并后所有响应都会回写 `X-Request-Id`：请求头带合法值时沿用，否则服务端生成）。
- **CORS（浏览器内 Harness）**：默认关闭；设置 `DSH_EE_CORS_ORIGINS`（逗号分隔的完整 origin，只允许 `https`，`http` 只放行 localhost）只为 `/v1/*` 与 `/mcp` 放行跨域，`/api/*` 不开放。使用 Bearer 令牌、不使用 cookie（没有 `Access-Control-Allow-Credentials`），允许来源按白名单精确回显而不是 `*`；`Access-Control-Expose-Headers` 放出 `Mcp-Session-Id`、`X-Request-Id`、`X-RateLimit-*`、`Retry-After`、`Dsh-EE-API-Version`。

边界：

- 第三方只依赖本页与 [l1-client.md](../l1-client.md) 写明的形状；`/api/dsh/*`、`/ee-login` 插件体验、`X-DshEE-Client` 属于 DSH 深接（H3），不是第三方契约，不要照着实现。**设备码是公开可选的令牌获取方式**（RFC 8628 子集）：`POST /api/device/code` 申请、员工在 `/device` 页面确认、`POST /api/device/token` 轮询领取；端点和可申请 scope 写在发现文档 `/.well-known/dsh-ee` 里，第三方可以用它，也可以用管理员下发或「我的令牌」生成的静态令牌。完整说明见 [harness-integration.md](../harness-integration.md)（companion PR #117）与 `l1-client.md`（#125 补充的发现/设备码说明）。
- 完整接入手册见 [harness-integration.md](../harness-integration.md)（companion PR #117 引入）；本页只做速览，不重复手册里的适配器和验证清单。
- **限流（#129 实现中，合并后生效）**：`/v1/*` 与 `/mcp` 按令牌做固定窗口限流（同一令牌共用一份额度、不同令牌互不影响），默认每分钟 60 次，可用 `DSH_EE_RATE_LIMIT` 调整（`0` / `off` 关闭）；正常和 429 响应都带 `X-RateLimit-Limit` / `X-RateLimit-Remaining` / `X-RateLimit-Reset`，429 另带 `Retry-After`（`/v1` 是 OpenAI 壳 `rate_limit_exceeded`，`/mcp` 是 JSON-RPC `-32000`）。#129 未合并前不要把限流头当现成契约；合并后以代码和 `server/README.md` 为准。
- 当前仍未实现，不要依赖：对外 SDK、跨站嵌入（iframe / postMessage）。
- 请求出错时按 [错误与 FAQ](./errors-faq.md) 先区分 401 / 403 / 404 / 429，再决定是重试还是找管理员。

## 7. 相关文档

- L1 接入完整说明（令牌 scope、MCP 握手、错误表）：[l1-client.md](../l1-client.md)
- 扩展与连接的权限模型：[access-control.md](../access-control.md)
- DSH 兼容矩阵（e2e 实测记录）：[compatibility.md](../compatibility.md)
- DSH 接入插件：[`dsh-plugin/`](../../dsh-plugin)
