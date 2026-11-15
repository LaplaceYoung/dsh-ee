---
slug: harness-quickstart
category: harness
title: "接第三方 Harness / 自研客户端"
title_en: "Connect a third-party Harness or your own client"
summary: "五分钟路径：一个 ee_at_ 令牌接 /v1 模型、/mcp 工具与服务端写确认，含发现端点和限制。"
summary_en: "Five-minute path: one ee_at_ token for /v1 models, /mcp tools and server-side write confirmation, plus discovery and limits."
tags: [Harness, 令牌, MCP, 模型网关]
updated_at: 2026-11-13
status: draft
since: 2026-11-13
lang: zh-CN
---

## 五分钟路径（H1：只接 `/mcp`）

第三方 Harness（Claude Code、Codex、自研 Agent、脚本）接 dsh-ee，最短路径是 **H1：一个 `ee_at_` 令牌 + `/mcp`**。需要技能文件、本机端侧扩展或一条命令生成配置时，再按仓库文档走 H2；DSH 专用深接（H3）不供第三方依赖，不要按 H3 设计集成。

| 步骤 | 做什么 | 看哪 |
| --- | --- | --- |
| 1 | 在「我的令牌」创建 `ee_at_` 令牌，scope 按用途最小化 | 下面「令牌与 scope」 |
| 2 | 读匿名发现文档 `GET /.well-known/dsh-ee`，确认 `api_version`、设备码端点、能力与限制 | 「发现与版本」 |
| 3 | 用 `models` scope 调 `/v1/models`、`/v1/chat/completions` | 「模型网关」 |
| 4 | 用 `connections` scope 连 `/mcp`，按会话动态 `tools/list` | 「MCP」 |
| 5 | 写工具走服务端确认：先拿「请在网页确认」链接，员工同意后用相同参数重调 | 「写操作」 |

## 令牌与 scope

令牌以 `ee_at_` 开头，对应用户本人；scope 只是上限，服务端每次请求还会与账号权限求交（账号没有对应权限点时仍用不了）。

| 用途 | 需要的 scope | 说明 |
| --- | --- | --- |
| 调模型 | `models` | `/v1/models`、`/v1/chat/completions`，账号还需要「调用大模型」权限 |
| 用 `/mcp` 的企业连接、知识库、应用数据工具 | `connections` | `/mcp` 入口按 `connections` 检查 |
| 用 `/mcp` 里的 `ee_*` 工作台工具 | `connections` + `agent` | 系统设置里的 Agent 总开关也要开着 |
| 读能力目录（可选） | `hub` | `/v1/apps`、`/v1/capabilities` 里的应用与 revision |

令牌等同密码：从环境变量或凭据库读，不进仓库、日志和截图；每个集成方单独发，按人吊销。

## 发现与版本

匿名端点 `GET /.well-known/dsh-ee` 返回 `api_version`、设备码端点、`scopes_supported`、模型/MCP/应用能力、请求大小限制与弃用策略。`GET /v1/capabilities`（Bearer 可选）返回当前令牌可见的模型、工具数与 revision；两者响应都带 `Dsh-EE-API-Version: 1` 头。

设备码登录是公开的可选入口：`POST /api/device/code` 拿 `device_code` / `user_code`，员工在 `/device` 页面确认后，按返回的 `interval` 轮询 `POST /api/device/token`。设备码 10 分钟内有效、令牌只能取一次，没有 refresh token，过期重新走流程。

## 模型网关（`/v1`）

OpenAI 兼容，最小调用：

```sh
export BASE='https://<dsh-ee>'
export TOKEN='ee_at_…'

curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"

curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"<模型名>","messages":[{"role":"user","content":"你好"}]}'
```

模型名以 `/v1/models` 返回为准；流式、限额与错误码按 OpenAI 兼容约定处理。

需要思考模式（`thinking` / `reasoning_effort`）时，看每个模型在 `/v1/models` 里公布的
`capabilities.reasoning`；请求参数与响应 `reasoning_content` 都由网关原样转发，工具轮次必须
回传历史 `reasoning_content`。详见[思考模式](./thinking-mode.md)。

## 模型输入模态（图片 / 文件）

管理端可以给每个对外模型声明接受的输入模态：`text`（始终包含）、`image`、`file`。声明只用于
**能力公布**与 dsh-connect 生成配置：网关仍按 OpenAI chat completions 原样透传，不按声明校验、
转换或拒绝请求内容；也不按模型名硬编码能力。

| 看哪里 | 字段 |
| --- | --- |
| `GET /v1/models` 每项 | `capabilities.input = ["text", "image"]` |
| `GET /v1/capabilities` | `input_modalities = ["text", "image", "file"]`（能力矩阵） |
| `GET /.well-known/dsh-ee` 的 `models` | 同上 `input_modalities` |

dsh-connect 生成的 DSH profile 会把它写进 `llm-pi-ai` 的模型条目：

```yaml
        models:
          - id: 'company-vision'
            input:
              - text
              - image
```

- DSH 0.2.x 的模型模态词表只有 `text` / `image`（`packages/llm/llm-pi-ai` 的 `MODALITY_GATE`），
  所以 `file` 不会写进 DSH 配置（写入未知模态会让 DSH 配置校验失败）；声明 `file` 的模型在 DSH
  里仍按 `text` 对待。
- **图片**：走 OpenAI chat completions 的 `image_url`（URL 或 base64 data URL）或 `file` 图片块，
  dsh-ee 原样转发，不做尺寸/格式转换。
- **文件/文档**：先通过 DeepSeek Files API（`POST /files`）上传并解析，再在 chat 请求里用
  `file_id` 引用；dsh-ee 只透传 chat 请求，不代理 `/files`，也不解析文件内容。
- 当前 dsh-ee 只有 OpenAI Chat Completions 协议面，没有 Anthropic Messages / Responses 路由，
  也没有跨协议转换层；这两类协议**原生透传**支持后按同样方式透传输入块（本次不涉及）。
- DSH 目前把会话里的 file 块投影成带文件名/路径的文本句柄，文件不会作为 provider 原生输入发送；
  因此 DSH 配置只声明 `text` / `image` 与当前转换层一致。其它 Harness 可自行构造 `file` 内容块
  并通过 `/v1/chat/completions` 透传（见 SDK 的 `sdk/examples/chatbot-server-tools.md`）。

## MCP（`/mcp`）

`/mcp` 是 Streamable HTTP + JSON-RPC 2.0：`POST /mcp` 发起请求，`Accept: application/json, text/event-stream`；`initialize` 返回的 `Mcp-Session-Id` 用于后续请求。

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-harness","version":"1.0"}}}'
```

| 工具前缀 | 来源 |
| --- | --- |
| `连接名__工具名` | 企业连接里的工具 |
| `kb__*` | 知识库工具 |
| `app__*` / `data__*` | 微应用的工具面 |
| `ee_*` | 工作台工具（需要 `agent` scope 与 Agent 总开关） |

可见工具由令牌主人本人的权限决定，不同员工结果可能不同：**每次会话动态 `tools/list`，不要写死**。工具的 `annotations.readOnlyHint` 缺失时按写操作处理。

## 写操作：服务端确认

第三方 Harness 默认（`non_dsh`）不能直接执行写工具：`tools/call` 会返回 `isError: true` 和「请在网页确认」链接。员工在 `/confirm/<id>` 页面核对参数并同意后，用**完全相同的参数**再调一次才真正执行；一次同意只能用一次，10 分钟内有效，参数变了要重新确认。DSH 客户端可以按自己的确认开关放行，但不要假设第三方 Harness 有这条捷径。

## 限制

| 限制 | 说明 |
| --- | --- |
| H1 不覆盖 | 技能目录、工作台、微应用网页、插件自动安装与更新属于 DSH 专用深接；第三方 Harness 不要假设有 |
| 请求大小 | `max_request_bytes` / `max_response_bytes` 以发现文档为准 |
| 令牌生命周期 | 没有 refresh token，过期重新签发；设备码流程只能取一次令牌 |
| scope 与权限 | scope 是上限，最终以「账号权限 ∩ scope」为准 |
| 接口稳定性 | `/v1`、`/.well-known/dsh-ee` 与 `/mcp` 的既有形状不变；新增能力以发现文档为准 |
| 写操作 | 未标 `readOnlyHint` 一律按写操作处理，走确认流程 |

## 仓库里的完整正文

站内这篇只是导读；协议与端点细节在仓库文档里（标「合并后可用」的文档由配套文档 PR 引入）：

- [第三方 Harness 接入（docs/harness-integration.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/harness-integration.md)
- [非 DSH 客户端接入 L1（docs/l1-client.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- [发现与能力（docs/l1-client.md §2，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md#2-发现与能力中立契约-v1)
- [开发文档里的 Harness 一页（docs/dev/harness.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- [扩展协议概览（docs/extension-protocol/README.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
