# 错误码与错误面索引

> 这一页只做索引：看到一个错误，先在这里按「哪个错误面、哪个域」定位码、字段和严重度，再点进专页看修法。不复制专页正文。
>
> 状态标记：
>
> - **已实现**：基线 `origin/main`（20aafbb）代码里已有；
> - **独立 PR**：有对应 PR 但尚未合入基线，能力以 PR 合入为准；
> - **设计**：只有设计，代码和接口都还没有。
>
> 本文是仓库 `docs/` 下的索引，不是产品页面：站内 `/docs/errors` 路由尚未实现。错误码会随版本变化；文档与行为不一致时，以代码和 `dsh-ee schema get errors` 的输出为准。

## 0. 先看这几个入口

| 场景 | 用哪条路 | 说明 |
|---|---|---|
| 扩展静态检查项 | `dsh-ee schema get errors`（离线） | 62 个检查项的 code / severity / 含义，是这张表的**单一事实源** |
| 扩展检查结果 | `dsh-ee extension check <目录> --json` | 每条带 `code`、`severity`、`where`、`message`、`fix`；退出码 0/1/2 |
| 扩展协议问题 | `dsh-ee extension test <目录\|地址>` | 静态检查 + 按协议逐项测；退出码 0/1/2 |
| 发布链路自检 | `dsh-ee doctor`、`dsh-ee package check <tgz>` | 独立 PR（#122）；退出码分别 0/1/2 与 0/1/2/3，见 [cli.md](./cli.md) §3 |
| 登录后的账号自检 | `GET /api/me/doctor` | 基线已实现；额度、模型、DSH 接入、目录等检查项，不是发布链路的 doctor |
| 管理动作追查 | 审计日志页（`audit.view`） | 审核、上下架、扩展安装/升级、令牌与权限等动作有记录 |

按域跳：

1. [发布预检](#1-发布预检11-个-scan-key与发布冲突) · 2. [审核/上架/安装](#2-审核上架与安装) · 3. [鉴权/会话/令牌](#3-鉴权会话与令牌) · 4. [模型网关/额度](#4-模型网关与额度) · 5. [MCP/JSON-RPC](#5-mcp--json-rpc) · 6. [扩展 check/运行期](#6-扩展检查与运行期) · 7. [嵌入/iframe](#7-嵌入与-iframe) · 8. [CLI/文档工具链](#8-cli-与文档工具链)

## 1. 发布预检（11 个 scan key）与发布冲突

**入口**：`POST /api/me/packages/check`（网页「分享插件 → 网页上传」的预检；CLI `dsh-ee package check <tgz>` 属独立 PR #122）。请求体是 `npm pack` 的 `.tgz`（`application/octet-stream`）。网页预检用网页会话、需要账号的「发布能力」；CLI `package check` 用令牌（独立 PR #122），需要 `registry:publish` scope。预检不写库、不执行插件代码、不占版本号。

**响应字段**（`server/internal/api/package_check.go`）：

| 字段 | 类型 | 含义 |
|---|---|---|
| `name` / `version` / `is_plugin` | string / string / bool | 从 tarball 里的 `package.json` 读出 |
| `scan` | `[{key, level, params}]` | 自动检查项；`level` 是 `pass` / `warn` / `fail` |
| `conflicts` | `[{key, params}]` | 会让发布接口直接拒绝的冲突 |
| `can_submit` | bool | 没有硬冲突，发布接口会接受 |
| `will_publish` | bool | 开放模式且无 `fail` 项，发布即上架 |
| `requires_review` | bool | 能提交但要进人工审核（`can_submit && !will_publish`） |

**11 个 scan key**（`server/internal/registry/tarball.go` 的 `Check`；`fail` 阻塞自动上架但仍可进人工审核）：

| key | 级别 | 检查什么 | 细节与修法 |
|---|---|---|---|
| `bundle` | fail / warn | `dsh.bundle` 缺失（普通 npm 包是 warn）、没有 `patch`、patch 路径不安全、文件不在包里 | [plugin-publish.md](./dev/plugin-publish.md#2-发布前预检)、[troubleshooting.md](./troubleshooting.md#12-自动检查项scan-key) |
| `patch` | fail / warn / pass | `cordis.patch.yml` 不是合法 YAML、不是数组、行缺 `id`/`name`、`id` 重复、没有引用本包名；`config` 不是对象或行 `name` 与包名不一致是 warn | 同上 |
| `cordis` | warn | 把 Cordis 放进了 `dependencies`（应与宿主共用，改用 peer / 构建依赖） | 同上 |
| `cordis-peer` | warn | Cordis 的 peer 范围不包含当前宿主版本 | 同上 |
| `skills` | warn | 包内带 `skills/`：技能随插件注入，不进能力中心、不能单独授权 | [plugin-authors.md](./plugin-authors.md) §3 |
| `install-scripts` | warn | `package.json` 声明了安装脚本 | [plugin-dev/security.md](./plugin-dev/security.md)、[troubleshooting.md](./troubleshooting.md#12-自动检查项scan-key) |
| `secrets` | **fail** | 扫到 `.env`、`.npmrc`、私钥、各家 token 格式等敏感文件/密钥 | [troubleshooting.md](./troubleshooting.md#12-自动检查项scan-key) |
| `license` | warn | 没有声明 `license` | 同上 |
| `engines` | warn | 没有声明 `engines.dsh` | 同上 |
| `dependencies` | pass | 列出非企业作用域的依赖（只提示，不阻塞） | 同上 |
| `size` | warn / pass | tarball > 20 MiB 或体积异常 | 同上 |

**发布冲突 key**（`packageConflicts`，与 `Publish` 的拒绝规则一致）：`name`（包名不合法）、`scope`（不在企业作用域）、`version`（不是 semver）、`upstream`（同名包已被上游镜像占用）、`owner`（包由其他人维护且调用者没有「管理能力与审核」）、`exists`（同版本号已存在；**被驳回或下架的版本号也不能重发**）。

**HTTP 错误**：请求体不是 `.tgz` → 415；tarball 过大 → 413；不是合法的 `npm pack` 包 / `package.json` 缺 `name`/`version` → 400；数据目录不可写 → 503。逐条见 [troubleshooting.md](./troubleshooting.md#12-自动检查项scan-key)。

> 网页上 scan key 的文案在 `web/src/i18n/{zh,en}/registry.ts`（`registry.scan.*`），发布冲突的文案在 `web/src/i18n/{zh,en}/distribution.ts`（`publish.conflict.*`）；新增检查项/冲突时两边要一起加。

## 2. 审核、上架与安装

### 2.1 版本状态与审核动作（已实现）

`npm_versions.status` 四个值（`server/internal/store/registry.go`）：`pending` 待审核、`published` 已上架、`rejected` 已驳回、`unpublished` 已下架。

管理端 `POST /api/admin/review`（权限 `plugins.manage`）请求字段：`name`、`version`、`action`、`note`：

| action | 允许的起始状态 | 结果 | 额外要求 |
|---|---|---|---|
| `approve` | `pending` | `published` | 必须指定 `version` |
| `reject` | `pending` | `rejected` | 必须指定 `version`，必须填 `note` |
| `unpublish` | `published` / `pending` | `unpublished` | 必须填 `note`；`version` 为空表示整个包 |
| `restore` | `unpublished` / `rejected` | `published` | `version` 为空时只恢复上过架的版本 |

状态不满足或动作名不认识时返回 400/409（如「未知的审核动作」「驳回与下架需要填写原因」「审核要指定版本」「没有处于可…状态的版本」）。审计动作名：`review.approve`、`review.reject`、其余动作记 `package.status`。

### 2.2 发布 / 安装常见 HTTP 错误

| 域 | 状态与说明 | 专页 |
|---|---|---|
| npm 发布 | 401 未登录 / 缺可发布令牌；403 账号无「发布能力」、令牌只读、包名留给 dsh-ee、非企业作用域、别人的包；400 包名或版本非法、一次多版本、tarball 与声明不符、package.json 与发布信息不一致；409 版本已存在、上游同名镜像、并发发布；413 tarball 超过上限；405 `npm unpublish`（请网页下架或 `npm deprecate`） | [troubleshooting.md](./troubleshooting.md#13-发布被拒--审核问题)、[plugin-authors.md](./plugin-authors.md) §8 |
| npm 安装 | 401 需要令牌；403 账号没有安装权限；404 没有这个包或版本（未上架、社区插件未审核）；tarball 404 | [troubleshooting.md](./troubleshooting.md#2-安装员工管理员把插件装进-dsh) |
| 扩展安装 | 403 没有 `connections.manage`；400「扩展没有通过检查」「参数「…」要由管理员设置」等；409「连接标识 … 已被占用」「这次升级有需要确认的变化」（带 `confirm:true` 重装）；401/403 见鉴权一节；服务地址取不到清单 / 清单超过 1 MB 等见专页 | [extension-protocol/errors.md](./extension-protocol/errors.md#3-安装--升级阶段的错误)、[enterprise-publishing.md](./enterprise-publishing.md#6-扩展的安装审核与权限) |

审计口径与审核边界见 [enterprise-publishing.md](./enterprise-publishing.md#3-审核与状态)；自动检查**不是安全审计**，不能对外说成 ENTRY / FLOW / BOOST 通过（见 [plugin-quality.md](./plugin-quality.md)）。

## 3. 鉴权、会话与令牌

### 3.1 三类凭据（已实现）

| 前缀 | 签发入口 | 权限口径 |
|---|---|---|
| `ee_at_` | 「我的令牌」/ 管理员「令牌管理」/ 设备码登录 | 令牌 scope ∩ 账号 RBAC；可限额度、模型、有效期；明文只显示一次，服务端只存 sha256 |
| `ee_npm_` | npm 登录 / 网页令牌页 | 旧版 npm 令牌，只读或可发布；权限行为保持旧版 |
| `sk-ee_` | 管理员代发 / 员工自助 | 旧版 API key；**不受 scope 收窄**，固定带 models + hub + connections |

scope 常量在 `server/internal/rbac/scopes.go`：`models`、`registry:read`、`registry:publish`、`hub`、`connections`、`agent`、`config`；`admin` 不能通过创建令牌接口下发，只能由网页给令牌开「管理模式」（1 或 8 小时，需再输密码）后生效。scope 与账号权限求交：令牌永远不能超过账号本身。

### 3.2 常见错误

| 错误 | 出现位置 | 处理 |
|---|---|---|
| `token_invalid` / `token_expired` / `token_revoked` | `/v1/*`、`/mcp`、`/api/dsh/*`、管理接口（`server/internal/api/principal.go`） | 重新签发 / 重新登录；不要复用旧值 |
| 401「请先登录」/「登录已过期，请重新登录」 | 网页会话接口 | 重新登录；默认会话 7 天 |
| 403「没有权限执行这个操作」 | 网页接口权限点不足 | 管理员加角色/权限；菜单可见不代表有权限 |
| 403 `reauth_required`「这个操作需要输入密码登录：你是从 DSH 直接打开的网页」 | limited 会话做敏感操作（`server/internal/api/web_link.go`） | 在顶层浏览器重新登录 |
| 415「请求体必须是 application/json」/ 400「请求体格式错误：…」 | 所有写接口（`readJSON`，含 CSRF 防线） | 按 JSON 发请求，不要多发未声明字段 |
| 401「用户名或密码错误」/ 429「失败次数过多，请 15 分钟后再试」/ 403 目录账号未开通 / 502 目录不可达 | 登录 | [troubleshooting.md](./troubleshooting.md#31-网页登录) |

### 3.3 设备码（RFC 8628 子集，已实现）

`POST /api/device/code` → 员工在 `/device?code=…` 确认 → `POST /api/device/token` 轮询。轮询错误码（`server/internal/api/device.go`）：`authorization_pending`、`slow_down`、`access_denied`、`expired_token`，以及入口限流的 `rate_limited`（429）。令牌只能取一次；设备令牌默认滑动有效 90 天。细节与 curl 示例见 [l1-client.md](./l1-client.md#2-发现与能力中立契约-v1)。

### 3.4 扩展身份（ee_jwt）

服务扩展收到的 `ee_jwt` 由 dsh-ee 现签：RS256、5 分钟有效、`kid` 选公钥，扩展必须验签；验不过返回 401 `{"code":"unauthenticated"}`。协议见 [extension-protocol.md](./extension-protocol.md) §3；员工身份与 `user_secret` 见 [ee-jwt.md](./ee-jwt.md)。

## 4. 模型网关与额度

**错误形状**：OpenAI 兼容 `{"error":{"message","type","code"}}`（`server/internal/api/gateway.go`）。`X-DshEE-Client: dsh-connect/…` 的请求额外把错误映射成 DSH 能直接展示的 400 文案。

**Anthropic 兼容端点** `POST /v1/messages`（别名 `/anthropic/v1/messages`）用 Anthropic 错误壳 `{"type":"error","error":{"type","message"}}`
（`server/internal/api/anthropic.go`），状态码与 OpenAI 网关相同，`error.type` 按状态码映射：
401 `authentication_error`、403 `permission_error`、404 `not_found_error`、429 `rate_limit_error`、
5xx `api_error`、其它 4xx `invalid_request_error`（请求体过大是 `request_too_large`）。
`/v1/messages/count_tokens` 鉴权后返回 404 `not_found_error`（v0 不实现）。DeepSeek 原生透传路径的上游错误按原状态码与 body 转发（DeepSeek 本身是 Anthropic 错误壳），dsh-ee 侧错误仍按上表。细节见
[dev/anthropic-api.md](./dev/anthropic-api.md#6-错误)。

| HTTP | code | 触发 | 专页 |
|---|---|---|---|
| 401 | `token_invalid` / `token_expired` / `token_revoked` | 缺 Bearer 或令牌无效 | [l1-client.md](./l1-client.md#7-常见错误) |
| 403 | `permission_denied` | 令牌缺 `models` 或账号缺 `llm.use` | 同上 |
| 403 | `model_not_allowed` | 模型不在账号或令牌的可用范围 | 同上 |
| 403 | `file_access_denied` | 普通员工在对话 / Responses 原生透传的 `input` 里引用已上传文件的 `file_id`（默认只允许管理员与机器人账号） | [dev/responses-api.md](./dev/responses-api.md#原生透传的文件引用访问边界) |
| 403 | `quota_expired` | 账号额度过期 | 同上 |
| 429 | `monthly_quota_exceeded` | 账号或令牌本月额度用完（`0` 表示未分配） | 同上 |
| 404 | `model_not_found` | 对外模型没有可用上游 | 同上 |
| 400 | `model_required` / `invalid_json` / `request_too_large` | 请求体问题（上限 16 MiB） | 同上 |
| 502 | `upstream_unreachable` | 所有上游线路连不上/失败 | 同上 |
| 500 | `provider_misconfigured` | 上游 key 解密失败（主密钥换过等） | 同上 |

DSH 接入插件看到的映射码：`dshee_login_required`、`dshee_forbidden`、`dshee_model_denied`、`dshee_no_allowance`、`dshee_monthly_limit_reached`、`dshee_allowance_expired`（统一以 400 返回）。逐条症状见 [troubleshooting.md](./troubleshooting.md#4-harness第三方-harness--自研客户端接-dsh-ee)、[dev/harness.md](./dev/harness.md#3-模型网关)。

## 5. MCP / JSON-RPC

`POST /mcp` 收 JSON-RPC 2.0，回 JSON；`GET /mcp` 是 SSE 事件流。错误形状是标准 JSON-RPC error（`server/internal/api/dsh.go`）：

| 码 | 含义 | 备注 |
|---|---|---|
| `-32700` | 不是合法的 JSON-RPC 请求 | 请求体超过 4 MiB 或不是 JSON |
| `-32601` | 不支持的方法 | 已实现 `initialize`、`ping`、`tools/list`、`tools/call`，以及空的 `resources/*`、`prompts/list` |
| `-32602` | 参数格式不对 / 没有这个工具或没有权限 / cursor 无效（独立 PR #128） | 工具不存在与权限不足不区分，避免探测 |

HTTP 层：缺 Bearer 或令牌无效 → 401 `{"error":"企业账号登录已失效…","code":"token_invalid"}`（`token_expired` / `token_revoked` 同理）；缺 `connections` scope 或没有工作台权限 → 403「你的账号没有使用工作台的权限」（无 code 字段）。

工具调用的业务失败放在结果里：`result.isError: true` + `result._meta["dsh-ee/error"].code`；需要人工确认的写操作返回确认链接（基线是文本确认；结构化 `_meta["dsh-ee/confirmation"]` 属独立 PR #128）。逐条见 [l1-client.md](./l1-client.md#5-mcp-握手工具列表与调用)、[harness-integration.md](./harness-integration.md#15-常见错误)。

## 6. 扩展检查与运行期

### 6.1 `dsh-ee extension check`（已实现，62 个检查项）

检查项按 code 分族（完整列表用 `dsh-ee schema get errors`，源码 `server/internal/extension/codes.go`）：

| 前缀/族 | 例子 | severity |
|---|---|---|
| `manifest.*` | `manifest.missing`、`manifest.invalid_json`、`manifest.type_mismatch`、`manifest.unknown_field` | error / warning |
| `id.*` / `version.*` / `min_version.*` / `name.*` / `description.*` | `id.invalid`、`version.invalid`、`min_version.invalid` | error / warning |
| `kind.*` / `repo.*` | `kind.invalid`、`kind.field_mismatch`、`kind.not_checked`、`repo.invalid` | error / info |
| `openapi.*` / `servers.*` / `service.url_invalid` / `package.invalid` | 声明式扩展的接口文档与允许地址 | error |
| `credential.*` / `config.*` / `permissions.*` / `capabilities.*` | 凭据模式、配置项、反向权限、能力声明 | error / warning |
| `tool.*` / `param.*` / `keep.*` | 工具只读标记、参数 `fill`/类型/默认值、返回裁剪 | error / warning / info |
| `apps.*` / `icon.*` / `fields.*` / `ignore.*` / `protocol.*` | 附带应用、图标、扩展字段、`checkIgnore`、协议版本 | error / warning / info |

退出码：`0` 通过、`1` 有 error 项、`2` 用法错误。严重度是 `error` / `warning` / `info`；可选忽略的 error 必须按规格写 `checkIgnore` 理由。**这不是安全审计**，也不等于上游三层标准。专页：[extension-protocol/errors.md](./extension-protocol/errors.md#4-静态检查)、[plugin-dev/errors.md](./plugin-dev/errors.md#6-dsh-ee-预检--发布错误)、[dev/extensions.md](./dev/extensions.md#3-开发流程)。

### 6.2 安装 / 升级错误（已实现）

| 错误 | 含义 | 处理 |
|---|---|---|
| `kind.unsupported` | 端侧扩展不在这里安装 / 服务扩展要填地址而不是上传 / 声明式扩展不能按服务地址装 | 按提示换安装方式 |
| `min_version.too_new` | 扩展要求更高的 dsh-ee | 升级 dsh-ee 或装旧版本 |
| 400「扩展没有通过检查，改好后再装」 | `check.OK=false`，响应里带完整 `plan.check` | 按 `code/where/fix` 修 |
| 409「这次升级有需要确认的变化：…」 | 服务地址、新反向权限、同版本内容、凭据模式、允许地址等变化 | 核对后带 `confirm:true` 重装 |
| 400「参数「…」要由管理员设置，请填上」 | 清单 `config` 的必填项没填 | 补齐管理员配置 |
| 409「连接标识 … 已被占用」 | 新装时的连接名重复 | 换一个标识 |

服务扩展与运行期的完整口径见 [extension-protocol/errors.md](./extension-protocol/errors.md#3-安装--升级阶段的错误)。

### 6.3 运行期错误码（已实现）

扩展在工具结果 `_meta` 或约定接口响应体里返回 `code`（`extension-protocol.md` §6）：

| code | 何时用 | dsh-ee 的处理 |
|---|---|---|
| `authorize` | 员工未授权/授权过期 | 提示员工去连接页授权 |
| `setup` | 管理员没配好 | 提示联系管理员，不重试 |
| `forbidden` | 源系统判定员工无权限 | 原样告知，不绕过 |
| `unavailable` | 源系统暂不可用 | 联邦检索跳过该来源；工具调用提示稍后重试 |
| `not_found` | 文档/记录不存在 | 同步时跳过该篇 |
| `invalid` | 参数或配置不对 | 原样交给调用方 |
| `not_implemented` | 没实现该约定接口 | 当作不支持 |
| `unauthenticated` | 扩展没验过 `ee_jwt` | 链路问题，查扩展的 `DSH_EE_URL`、时钟、公钥缓存 |
| `internal` | 扩展内部错误（SDK 默认码） | 普通失败，不自动重试 |

熔断与超时：约定接口连续失败 5 次 → 该扩展熔断 30 秒（期间返回 `unavailable`），只影响这一个扩展；单次调用沿用连接超时（默认 30 秒），工具结果上限 64 KB、约定接口响应上限 8 MB。逐条见 [extension-protocol/errors.md](./extension-protocol/errors.md#2-扩展返回的错误码)、[extension-protocol.md](./extension-protocol.md) §7。

## 7. 嵌入与 iframe

**状态**：基线（20aafbb）没有嵌入支持；同站 v0 与跨站 v1 由独立 PR #118 实现（合入前以 #118 为准）。跨站 v1 **默认关闭**：需要 `DSH_EE_EMBED_CROSS_SITE=1` 且配置 `DSH_EE_EMBED_ORIGINS`。宿主接入（参数、消息、验收）见 [embedding.md](./embedding.md)；设计说明与未做项见 [cross-site-embedding.md](./cross-site-embedding.md)。

嵌入的错误分三层，别混：

| 层 | 形状 | 典型 code |
|---|---|---|
| iframe → 宿主 `postMessage` | `dsh-ee:error` `{code,message,retryable}` | `auth_required`、`forbidden`、`load_failed`、`embed_code_invalid`、`embed_code_expired`、`embed_disabled`、`rate_limited`、`network` |
| 宿主后端 mint / iframe exchange 的 HTTP | `{"error","code"}` + 状态码 | `embed_disabled`(404)、`rate_limited`(429)、`origin_not_allowed`(403)、`app_not_found`(404)、`app_not_web`、`app_required`(400)、`next_invalid`(400)、`forbidden`(403)、`embed_code_invalid` / `embed_code_expired`(400) |
| 嵌入会话访问接口 | `{"error","code"}` | 白名单外 403 `embed_scope`；会话无效/过期 401「嵌入会话已过期，请在宿主页面重新打开」 |

- **配置错误**（启动失败，fail closed）：`DSH_EE_EMBED_ORIGINS` 含 `*`/`null`/通配符/路径/非 https；`DSH_EE_EMBED_CROSS_SITE=1` 但没配 origins；
- **CSP 拒绝**：宿主 origin 不在 `DSH_EE_EMBED_ORIGINS`，或宿主框的不是 `/a/` 页面（管理页永远 `'self'`）；
- **失败兜底**：iframe 不跳登录页；8 秒内没有 `ready`/`error` 时宿主显示「在浏览器打开」；移动端由宿主顶层打开（dsh-ee 尚无自动策略）；
- **审计**：mint 记 `embed.session_create`、exchange 记 `auth.login`（`via=embed`）、注销记 `embed.revoke`、拒绝记 `embed.deny`（不记 code/令牌明文）。

逐条排查见 [troubleshooting.md](./troubleshooting.md)；接口与协议细则见 [embedding.md](./embedding.md)、[cross-site-embedding.md](./cross-site-embedding.md)。

## 8. CLI 与文档工具链

### 8.1 CLI 退出码

| 命令 | 退出码 | 说明 |
|---|---|---|
| `dsh-ee extension check` | 0 / 1 / 2 | 通过 / 有 error 项 / 用法错误 |
| `dsh-ee extension test` | 0 / 1 / 2 | 全部通过 / 有失败 / 用法错误 |
| 其它扩展子命令（init/dev/call/pack/report） | 0 / 1 / 2 | 成功 / 执行失败 / 用法错误 |
| `dsh-ee package check`（独立 PR #122） | 0 / 1 / 2 / 3 | 通过 / 包问题或阻塞 / 用法配置 / 调用服务失败 |
| `dsh-ee doctor`（独立 PR #122） | 0 / 1 / 2 | 全部正常 / 有失败项 / 用法错误 |
| `dsh-ee schema get <名字>` | 0 / 1 / 2 | 输出规格 / 名字不存在 / 用法错误 |

完整命令矩阵与配置来源见 [cli.md](./cli.md)；扩展命令细节见 [dev/extensions.md](./dev/extensions.md)、[plugin-dev/cli-matrix.md](./plugin-dev/cli-matrix.md)。

### 8.2 文档检查（独立 PR #120）

合并后 `node scripts/check-docs.mjs` 会检查 `docs/**/*.md` 的相对链接与锚点、`docs/plugin-dev/SOURCES.md` 的 pin/改编清单、`docs/dev/*.md` 的 front matter、NOTICE 与索引。常见失败 kind：`broken-link`、`bad-anchor`、`broken-anchor`、`pin-row`、`front-matter`、`dev-index`、`missing-notice`。失败时按文件与行号修；规则见 [dev/README.md](./dev/README.md#维护规则)、[contributing.md](./contributing.md#6-文档检查)。

## 9. 维护规则

新增或修改错误码时，一次性检查这些位置，避免只有一处更新：

1. **产生错误码的代码**（`writeErr` / `writeOpenAIErr` / `writeRPC` / 扩展 `Error` / API 的 `code` 字段）与对应测试；
2. **本页与专页**：本页只登记索引，细节写进对应专页（发布、扩展、Harness、嵌入、排障）；
3. **前端文案**：scan key 在 `web/src/i18n/{zh,en}/registry.ts`，发布冲突在 `web/src/i18n/{zh,en}/distribution.ts`；审计动作在 `web/src/lib/audit.ts` 与 `web/src/i18n/{zh,en}/audit.ts`（两边成对加，否则 `tsc` 会报错）；
4. **审计动作**：`server/internal/api/audit.go` 的常量与前端映射同步；
5. **文档检查**：提交前跑 [文档检查](#82-文档检查独立-pr-120)，改过的相对链接和锚点要能解析。
