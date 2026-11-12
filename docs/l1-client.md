# 非 DSH 客户端接入（L1）：curl / 自研 Agent 怎么用

> 对应 e2e：`e2e/tests/28-l1-client.mjs`。这条链路不安装 DSH，只用**一个统一令牌**访问模型网关和 `/mcp`，适合 curl、脚本、自研 Agent 或企业集成方。

## 1. 准备令牌

在网页「我的令牌」生成，或由管理员在「令牌管理」下发。令牌以 `ee_at_` 开头，至少需要这些 scope：

| 用途 | scope |
|---|---|
| 读模型 / 调用模型 | `models` |
| 使用 `/mcp` 的 Agent 工具 | `agent` |
| 使用 `/mcp` 的企业连接工具 | `connections` |
| 读能力目录（可选） | `hub` |

不要把管理员令牌写进共享脚本；给每个集成方单独发令牌，按需最小化 scope。

```sh
export BASE='https://dsh-ee.example.com'
export TOKEN='ee_at_…'
```

## 2. 读模型目录

```sh
curl -fsS "$BASE/v1/models" \
  -H "Authorization: Bearer $TOKEN" | jq '.data[].id'
```

返回的是 dsh-ee 的**对外模型名**，不是上游厂商的模型名。

## 3. 调用模型

```sh
curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

协议是 OpenAI chat completions 子集；额度、审计、上游故障转移都由 dsh-ee 处理。

## 4. MCP 握手与工具列表

`POST /mcp` 是 JSON-RPC；`Accept` 要同时带 JSON 和 SSE：

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}'
```

拿到 `Mcp-Session-Id` 后列工具：

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

工具名形如 `crm__lookup`（企业连接）、`kb__search`（知识库）、`ee_me`（Agent 工具）；具体可见范围由令牌 owner 的部门 / 角色 / 连接授权决定。

## 5. 常见错误

| 现象 | 原因 | 处理 |
|---|---|---|
| 401 `token_invalid` / `token_revoked` | 令牌错误、过期或被吊销 | 重新生成，别复用旧值 |
| 403「没有使用工作台的权限」 | 令牌缺少 `/mcp` 需要的 `connections`，或账号没有 portal 权限 | 补 scope / 找管理员开权限 |
| 404 模型不存在 | 用了上游名或未导入的模型 | 先 `GET /v1/models` 看对外名 |
| 429 | 触发租户 / 令牌额度或上游限流 | 退避重试，或找管理员调额度 |

## 6. 与 DSH 的关系

同一套令牌也可以给 DSH 的接入插件用；DSH 只是多了一层本机 profile 和 `/ee-login` 设备确认。非 DSH 客户端不需要装插件，但**没有** DSH 的技能目录、工作台、自动更新等能力，只覆盖 L1 的模型与 MCP。

安全提示：令牌等同密码；示例里的 `$TOKEN` 不要写进仓库、日志或截图。需要按人撤销时，用管理员「令牌管理」按前缀/用户吊销。
