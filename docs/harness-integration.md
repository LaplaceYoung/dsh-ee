# 第三方 Harness 接入 dsh-ee（H1 / H2 / H3）

> 给非 DSH 的 Agent / Harness 看：Claude Code、Codex、自研 Agent、企业集成方。按「只接 MCP」到「和 DSH 一样深接」分三层；**H1 和 H2 今天就能用，H3 是 DSH 专用、不作为第三方依赖提供**。
>
> 最小可用链路（令牌、模型网关、MCP 的 curl 示例）见 [l1-client.md](./l1-client.md)；扩展协议见 [extension-protocol.md](./extension-protocol.md)；扩展体系的设计与边界见 [plugins.md](./plugins.md)。

## 0. 三层接入形态

| 层 | 适用 | 最小契约 | 现状 |
|---|---|---|---|
| **H1 MCP-only** | Claude Code、Codex、自研 Agent、脚本 | 一个 `ee_at_` 令牌 → `POST /mcp`（Streamable HTTP / JSON-RPC）；工具名 `连接名__工具名`；写操作走 dsh-ee 服务端确认 | ✅ 已可用 |
| **H2 适配器** | 需要同时接本机端侧扩展、要技能文件、要一条命令生成的配置 | H1 + 本机 stdio MCP 扩展（`DSH_EE_URL` / `DSH_EE_TOKEN` / `DSH_EE_HARNESS`）+ 技能复制 | ✅ 有 Claude Code 官方样例 |
| **H3 DSH 深接** | 只限 DSH 客户端 | `dsh-ee-connect-plugin`（DSH 私有插件）+ DSH 专属 `/api/dsh/*` + 技能/应用分发/自动更新；设备码登录是公开可选的令牌获取方式（§1.1），不属于 H3 专属 | ✅ 已用于 DSH；**不供第三方依赖**（§3） |

**H1 覆盖什么**：企业连接工具、知识库工具、微应用的工具面（`app__*` / `data__*`）、模型网关。
**H1 不覆盖**：技能目录、工作台、微应用网页、插件自动安装和更新——这些是 H3 的能力，第三方 Harness 不要假设有。令牌既可以用管理员下发/「我的令牌」生成，也可以走公开的设备码流程（§1.1）。

## 1. H1：只接 `/mcp`

### 1.1 令牌

`ee_at_` 开头的接入令牌，对应到某位员工（权限按这个人的部门/角色/连接授权求交）。在网页「我的令牌」生成，或由管理员在「令牌管理」下发；**每个集成方单独发，按需最小化 scope**。

| scope | 用来做什么 |
|---|---|
| `connections` | **`/mcp` 入口必需**，决定能不能用企业连接工具 |
| `agent` | 放开 `/mcp` 上的 `ee_*` 工作台工具（额度、能力中心、使用记录等），不用可以不选 |
| `models` | 调 `/v1/models`、`/v1/chat/completions` |
| `hub` | 读能力目录等（可选） |

令牌等同密码：从环境变量读，不进仓库、日志、截图；按人发放，离职/换人时单独吊销。拿令牌的常用方式是管理员在「令牌管理」下发，或员工在「我的令牌」生成。**设备码流程也是公开的可选入口**（RFC 8628 子集，端点写在中立发现文档里，见 §4）：`POST /api/device/code` 拿 `device_code` / `user_code`，员工在 `/device` 页面确认，再按返回的 `interval`（默认 5 秒）轮询 `POST /api/device/token`；设备码 10 分钟内有效、令牌只能取一次，拿到的令牌有效期由服务端设置（滑动续期），没有 refresh token，过期要重新走流程。scope 全集可以读发现文档的 `scopes_supported`。

### 1.2 配置

Claude Code 的 `.mcp.json`（也可以让 §2 的适配器生成）：

```json
{
  "mcpServers": {
    "dsh-ee": {
      "type": "http",
      "url": "https://ee.example.com/mcp",
      "headers": { "Authorization": "Bearer ${DSH_EE_TOKEN}" }
    }
  }
}
```

- `type: "http"` 是 Streamable HTTP；请求 `POST /mcp` 带 JSON-RPC 2.0，`Accept: application/json, text/event-stream`。
- `GET /mcp` 是事件流（工具变化通知），`DELETE /mcp` 结束会话；`Mcp-Session-Id` 由服务端在 `initialize` 时下发（无状态直调也可用）。
- 不要把令牌写进文件；用运行环境变量或凭据库注入。

### 1.3 握手和工具

```sh
# initialize
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-harness","version":"1.0"}}}'

# tools/list（带上 initialize 返回的 Mcp-Session-Id）
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

- 工具名是 `连接名__工具名`；`kb__*` 是知识库，`app__*` / `data__*` 是微应用，`ee_*` 是工作台工具。
- 可见工具由令牌主人本人的权限决定，不同员工的 `tools/list` 结果可能不一样，所以**按会话动态取工具**，不要写死。
- `tools/list` 默认分页：一页 100 个、单页最多 500；结果里带 `nextCursor` 就把 `params.cursor` 填成它取下一页，直到没有 `nextCursor`。想一次拿全量传 `limit: 0`；不跟 `nextCursor` 的旧客户端只拿第一页（工具不超过一页时与全量一致）。DSH 接入插件仍一次拿全量。
- 每个工具带 `annotations.readOnlyHint`（只读提示）和 `_meta.dshee`；`readOnlyHint` 缺失按写操作处理。
- `tools/call` 的参数直接按工具的 `inputSchema` 传。

### 1.4 写操作：服务端确认

客户端能被设成自动同意（Claude Code、Codex 都可以），所以 dsh-ee 不在客户端做唯一一道：**默认设置下（`non_dsh`），不是 DSH 的调用方，写工具不会直接执行**，而是返回一条 `isError: true` 的结果，里面有「请在网页确认」的链接。员工在 `/confirm/<id>` 上核对参数并点同意后，用**完全相同的参数**再调一次才真正执行；一次同意只能用一次，10 分钟内有效；参数变了要重新确认。

确认结果除了 `content` / `isError` 和原来的中文文本，还在结果级 `_meta` 里带结构化信息：`{"dsh-ee/confirmation": {"id": …, "url": …, "expires_at": "…"}}`（`expires_at` 是 RFC3339 UTC）。适配器可以直接用 `url` 渲染确认按钮、用 `expires_at` 控制有效期；不读 `_meta` 的旧客户端继续用文本里的链接也兼容。

适配器要做的事：

1. 把确认链接和参数原样交给员工（优先用 `_meta["dsh-ee/confirmation"]` 的 `url`；不要自动点同意，也不要自己重试同一调用绕过）；
2. 员工确认后按原参数重调；
3. 员工拒绝时把结果原样回给模型，不要自动换工具重试。

管理员可以把写确认设成 `off`（不确认）/ `non_dsh`（默认，非 DSH 才确认）/ `always`（一律确认）。这是企业安全设置，适配器不要试图猜测或绕过；也不要把自己伪装成 DSH（见 §2.3）。

`/mcp` 上写操作的确认与审计语义见 [plugins.md](./plugins.md) §7.5；扩展自己的错误码（`authorize` / `setup` / `forbidden` / `unavailable` 等）会以工具结果文本返回，按 [extension-protocol.md](./extension-protocol.md) §6 处理。

### 1.5 常见错误

| 现象 | 原因 | 处理 |
|---|---|---|
| 401 `token_invalid` / `token_revoked` / `token_expired` | 令牌错、过期或被吊销 | 重新生成/换发，不要复用旧值 |
| 403「没有使用工作台的权限」 | 令牌缺 `connections` scope，或账号没有 portal 权限 | 补 scope / 找管理员开权限 |
| `/mcp` 返回 `-32601` / `-32602` | 方法或参数不对 | 按 MCP 规范修 JSON-RPC 请求 |
| 写工具返回「需要确认」 | 服务端确认（正常流程） | 把链接给员工，确认后按原参数重调 |
| 429（模型网关） | 触发租户/令牌额度或上游限流 | 退避重试；持续出现找管理员调额度 |

完整错误表见 [l1-client.md](./l1-client.md) §5。

## 2. H2：适配器

### 2.1 Claude Code 样例

```bash
node extensions/official/claude-code/setup.mjs \
  --server https://ee.acme.com \
  --dir . \
  [--local @acme/legacy-oa-local]

export DSH_EE_TOKEN=<在 dsh-ee「我的令牌」里建的令牌>
claude
```

它做三件事：

- 把 dsh-ee 的 `/mcp` 写进 `.mcp.json`（Streamable HTTP），令牌写成 `${DSH_EE_TOKEN}`，从环境变量取，不进文件；已有的其他 MCP 服务器不动。
- `--local` 列出的端侧扩展作为本机 stdio MCP 服务写进去，带 `DSH_EE_URL`、`DSH_EE_TOKEN`、`DSH_EE_HARNESS=claude-code` 三个环境变量。
- 把 `extensions/skills/` 下的技能复制到 `.claude/skills/`。

`--dry-run` 只打印要写哪些文件，不落盘。见 [extensions/official/claude-code/README.md](../extensions/official/claude-code/README.md)。

### 2.2 端侧扩展在 H2 里的语义

- 端侧扩展是**跑在员工电脑上**的 npm 包，用 stdio MCP 直接对接 Agent，不经过 dsh-ee 的 `/mcp`；dsh-ee 只提供写操作确认（`POST /api/local/confirm`）和审计回报（`POST /api/local/audit`）。
- 没标 `readOnly` 的工具，SDK 在执行前调 dsh-ee 确认；员工同意、同样的参数再调一次才执行。SDK 在每次调用后回报摘要（不带参数和返回内容）。
- 这是**弱保证**：本机进程可以不问、不报。企业要求强审计时只用声明式和服务扩展。详见 [extensions/sdk-local/README.md](../extensions/sdk-local/README.md) 和 [extension-protocol.md](./extension-protocol.md) §9。

### 2.3 写另一个 Harness 适配器

只要 H1 的令牌和 `/mcp` 通了，剩下三块：

1. **写配置**：把 `<base>/mcp` 作为 Streamable HTTP MCP 服务器写进目标 Harness 的配置，`Authorization` 从环境变量/凭据库注入；令牌按人发放。
2. **接本机扩展**（可选）：需要端侧扩展时，把对应的 npm 包作为 stdio MCP 服务器写进配置，带 `DSH_EE_URL`、`DSH_EE_TOKEN`、`DSH_EE_HARNESS=<harness 名>`。
3. **处理确认**：把写操作的确认链接展示给员工，确认后按原参数重调。

**不要冒充 DSH**：第三方适配器不要带 DSH 的客户端标识头（`X-DshEE-Client` 等）。写操作的确认由客户端自己弹还是走 dsh-ee 服务端，正是按这个区分；第三方客户端没有 DSH 的审批卡，一律走服务端确认（见 §1.4）。

## 3. H3：DSH 深接（不供第三方依赖）

H3 是 DSH 客户端和 dsh-ee 之间的深接，包括：

- `dsh-ee-connect-plugin`（`dsh-plugin/`，cordis 插件）：MCP 桥、审批卡、技能提供方、本机诊断、必装插件安装、自动更新；
- DSH 专属接口：`/api/dsh/catalog`、`/api/dsh/skills/*`、`/api/dsh/whoami`、`/api/dsh/web-link`、`/api/dsh/feedback`、应用订阅等；
- 分发入口：`/setup.sh`、`/setup.ps1`、`/dl/connect/*`、`/api/dsh/releases`、`/index/dsh-index.json`；
- 首次接入与令牌失效后的重新登录：DSH 插件用设备码流程（公开入口，见 §1.1）加 `/ee-login` 做确认；设备码本身不是 DSH 专属；
- 工作台视角的技能目录、微应用、审批卡、自动更新。

**为什么第三方 Harness 不把它当依赖**：

- 接入插件是 DSH 专用的 cordis 插件（包是 `private`，不发布到 npm），由 dsh-ee 构建、通过 DSH 的插件体系安装和更新；别的 Harness 没有这套宿主接口。
- `/api/dsh/*` 的形状是给 DSH 前端和插件用的，不是中立契约；DSH 专属头（`X-DshEE-Client`、`X-DshEE-Host`）只用于区分 DSH，第三方不该伪造（见 §2.3）。
- `/setup.sh` / `/setup.ps1` 写的是 DSH 的本机 profile，别的 Harness 没有这些文件；设备码流程本身是公开入口（§1.1），但 DSH 的 `/ee-login` 体验和令牌保管属于插件。
- 技能目录、微应用网页、自动更新是 DSH 独有的分发/展示能力，**不在第三方接入契约里**。

第三方想要 H3 里的某些能力，现在能用的是它们的协议面：企业连接/知识库/微应用的工具走 `/mcp`，模型走 `/v1`；要知道这台 dsh-ee 支持哪些协议面，用 §4 的中立发现和能力摘要。不要直接照 `/api/dsh/*` 实现；更完整的技能/应用契约、限流头等在后续版本再定。

## 4. 发现、能力摘要和 API 版本（已实现）

服务端已提供中立发现、能力摘要和 API 版本头：

- **`GET /.well-known/dsh-ee`（匿名）**：公开发现文档，带 `ETag` + `Cache-Control: private, max-age=300`，支持 `If-None-Match` → `304`。字段：
  - `name` / `version` / `api_version`（当前 `"1"`）；
  - `device_authorization_endpoint` / `token_endpoint`（设备码流程，绝对地址，与 §1.1 一致）、`scopes_supported`；
  - `models`：`protocol: "openai-chat-completions"`、`streaming`；
  - `mcp`：`transport: "streamable-http"`、`protocol_versions`；
  - `apps`：`web`、`data_tools`；
  - `limits`：`max_request_bytes` / `max_response_bytes`；
  - `deprecation`：`sunset_header`。
- **`GET /v1/capabilities`（Bearer 可选）**：不带令牌返回服务标识和 `api_version` 的公开子集；带有效令牌返回该令牌视角的 `models`、`tools.count`、`apps`、`revision`、`user`、`scopes`。带了无效令牌返回 `401`，不会静默降级成匿名。
- **`GET /v1/apps`、`GET /v1/apps/{name}`**：只读的中立微应用描述（需要 `hub` scope + 工作台权限），字段不含 DSH 专属的 `asset_id` / `workbench` / `brand` / `required_plugins` 等。

版本协商与限制：

- 响应头：`/v1/*` 和 `/.well-known/dsh-ee` 的响应带 `Dsh-EE-API-Version: 1`；`/api/*`、`/mcp`、`/healthz` 等既有入口不加。
- 请求头：新增中立端点接受 `Dsh-EE-API-Version`，缺省 / `1` / `1.x` 接受；不支持的主版本返回 `400`、`code: "unsupported_api_version"`、带 `supported_versions: ["1"]`，不静默降级。
- 错误壳：新增中立端点的错误是 `{"error": {"type","code","message","request_id",…}}`；只读端点只支持 `GET` / `HEAD`（其他方法返回 `405` + `Allow`）。`/api/*` 和 `/v1` 网关既有的响应体与错误格式不变。
- `/mcp` 仍按 MCP 自己的 `initialize` 协商 `protocolVersion`（2025-11-25 / 2025-06-18 / 2025-03-26，不认识的回最新），与上面的 `api_version` 是两条独立的轴。
- `/.well-known/jwks.json` 是 `ee_jwt` 验签用的，不是 Harness 契约；`/api/dsh/*` 和内部页面接口仍然不是第三方契约，改动不预告。
- 已实现（默认可直接用，或用配置打开）：
  - MCP `tools/list` 分页（默认 100/页、单页上限 500、`limit: 0` 取全量；不跟 `nextCursor` 的旧客户端只拿第一页）和写确认的结构化 `_meta`（`dsh-ee/confirmation`，旧客户端可忽略，不影响文本结果）。
  - CORS 白名单（默认关闭；设 `DSH_EE_CORS_ORIGINS` 为逗号分隔的完整 origin 才打开，只对 `/v1/*` 和 `/mcp` 生效；Bearer 令牌、不使用 credentials，允许来源精确回显、不是 `*`；`/api/*` 跨域仍不开放）。
  - 每令牌限流（`DSH_EE_RATE_LIMIT`，默认 60 次/分钟）与 `X-RateLimit-*` / `Retry-After`；计数在数据库 `shared_rate_limits` 里，多实例部署时是合计额度。
- 还没有的（后续版本，不要依赖）：`client_kind` 令牌属性、SDK、跨站嵌入（一次性 code + 内存 Bearer）。

更一般的 Harness 接入说明（令牌、模型、MCP、DSH 插件）见 [docs/dev/harness.md](./dev/harness.md)；本文只写 H1/H2/H3 的分层和边界，两篇不重复。

## 5. 最小验证清单

拿一个员工令牌，按顺序跑：

```sh
export BASE='https://ee.example.com'
export TOKEN='ee_at_…'

curl -fsS "$BASE/healthz"                                             # 服务活着
curl -fsS "$BASE/.well-known/dsh-ee"                                  # 公开发现（§4）：协议面、版本、设备码端点、限制
curl -fsS "$BASE/v1/capabilities" -H "Authorization: Bearer $TOKEN"   # 当前令牌的能力摘要：模型、工具数、微应用、revision
curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"         # 能拿到对外模型名（要 models scope）
# 再用 1.3 的 initialize / tools/list 确认工具列表；挑一个只读工具 tools/call 试一次
# 再挑一个写工具：应该返回「请在网页确认」，确认后同参数重调才执行
```

仓库里对应的冒烟测试是 [e2e/tests/28-l1-client.mjs](../e2e/tests/28-l1-client.mjs)（只用一个 `ee_at_` 令牌走模型和 `/mcp`，不安装 DSH），可以当接入方的参考实现。

## 6. 相关文档

- 通用 Harness 接入（令牌、模型、MCP、DSH 插件）：[docs/dev/harness.md](./dev/harness.md)
- 最小链路、curl 示例、常见错误：[l1-client.md](./l1-client.md)
- 扩展协议（服务扩展、端侧扩展、错误码）：[extension-protocol.md](./extension-protocol.md)
- 扩展体系设计和边界：[plugins.md](./plugins.md)
- 扩展作者手册：[extension-authors.md](./extension-authors.md)
- DSH 深接的完整技术细节（DSH 专用，不是第三方契约）：[agent-connect-tech.md](./agent-connect-tech.md)
