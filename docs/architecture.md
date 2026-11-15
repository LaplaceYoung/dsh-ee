# dsh-ee 架构（当前实现）

> 状态：按 `origin/main`（20aafbb，2026-11-12）核对代码写成。本文只写**已经实现**的行为；还没做的能力标「设计」，并在 §7 汇总。文中标「PR #N，合并后生效」的能力来自对应独立 PR（#118 微应用嵌入 v0/v1、#122 CLI、#123 Harness 发现、#127 浏览器内 CORS、#128 MCP 分页/写确认结构化），**均未合入基线**；能力以对应 PR 合入为准。
>
> [design.md](./design.md) 是 2026-10-26 的设计稿，其中早期选型（SQLite / PostgreSQL / S3/MinIO、安装向导生成企业 CA 等）已经不是当前实现。两者冲突时以本文、[server/README.md](../server/README.md) 和代码为准。

## 1. 部署形态

dsh-ee 的运行时只有三样东西：

| 组成部分 | 说明 | 代码/产物 |
|---|---|---|
| 一个静态二进制 | Go 服务端，前端在构建时嵌进二进制；同一个端口提供页面、`/api`、`/v1`、`/npm/`、`/mcp`、`/index/dsh-index.json` | `dist/dsh-ee`（`make build`）或 Docker 镜像 `dsh-ee:<版本>` |
| 一个 MySQL 协议数据库 | **TiDB** 或 **OceanBase（MySQL 模式）**；库不存在时首次启动会尝试创建 | `DSH_EE_DSN` |
| 一个本地数据目录 | npm tarball（按 sha512 内容寻址）、上游 packument 缓存、企业 logo、DSH 安装包镜像 | `DSH_EE_DATA_DIR`，默认 `./data`，容器里 `/data` |

不依赖 Redis、消息队列、对象存储或单独的 Web 服务器。TLS 由前面的反向代理/负载均衡终止，dsh-ee 自己只监听 HTTP。

构建链路（`Dockerfile`、`Makefile`）：

1. `web/` 用 Vite + React 构建出 `web/dist`；
2. `dsh-plugin/`（`dsh-connect`）先构建，`web/` 会引用它的一部分源码（工作台渲染共用）；
3. Go 构建时把 `web/dist` 拷进 `server/internal/webui/build/`，由 `//go:embed all:build` 嵌进二进制；版本号用 `-ldflags "-X main.version=…"` 写入；
4. 运行镜像只有 `/usr/local/bin/dsh-ee`，`EXPOSE 8080`，`HEALTHCHECK` 请求 `/healthz`，数据卷 `/data`。

启动顺序（`server/cmd/dsh-ee/main.go`）：读环境变量 → 连数据库（`DSH_EE_DB_WAIT` 内每 3 秒重试）→ 跑迁移 → 创建初始管理员（用户表为空时）→ 初始化主密钥与数据目录 → 把上次进程中断的后台任务标为失败（不自动续跑；多实例下仍每实例执行，见 §7）→ 初始化后台任务租约执行器 → 起 HTTP 与后台循环（周期任务每轮先抢 `background_leases` 租约，见 §4.3）。收到 SIGINT/SIGTERM 后最多等 30 秒优雅关闭，并释放本实例持有的后台任务租约；流式响应可能持续数分钟，所以 HTTP 服务不设 `WriteTimeout`（`ReadHeaderTimeout` 10s、`IdleTimeout` 120s）。

## 2. 代码组件

| 组件 | 位置 | 职责 |
|---|---|---|
| 进程入口 / CLI | `server/cmd/dsh-ee` | 不带子命令时启动服务；`version`、`extension`、`config`、`schema`、`admin recover` 等子命令见 [cli.md](./cli.md) |
| HTTP 层 | `server/internal/api` | 路由表、认证中间件、全部 handler；`routes.go` 里一行一条路由，同时生成 Agent 工具 |
| 存储层 | `server/internal/store` | 主库的业务 SQL 与迁移（`migrations/*.sql`，按文件名顺序执行，失败即启动失败）；换数据库方言主要改这一层 |
| 后台任务租约 | `server/internal/lease` | 多实例下周期任务的租约执行器：抢租约、按 TTL/3 续约、停机释放、DB 不可用时跳过；租约表 `background_leases` 与访问方法在 store 层 |
| 权限 | `server/internal/rbac` | 权限点目录、角色求并、scope 与权限求交（`ScopeFilter`） |
| 模型网关 | `server/internal/api/gateway.go`、`llm.go` | OpenAI 兼容的 `/v1`，上游故障转移、额度、用量记录 |
| npm 源 | `server/internal/registry` | packument / tarball / 发布 / 审核 / 回源 / 预载 / 离线包；`blobs.go` 管数据目录 |
| 扩展 | `server/internal/extension` | 清单解析、静态检查、报告、运行期；`exttest` 是 `extension dev/test/call` 用的模拟 dsh-ee |
| 主密钥与加密 | `server/internal/secret` | AES-256-GCM 加密存库的上游凭据/扩展共享凭据；sha256 存令牌摘要 |
| 员工身份令牌 | `server/internal/eejwt` | 服务扩展调用的 `ee_jwt` 签名、JWKS、密钥轮换 |
| 登录方式 | `server/internal/api/auth.go`、`ldap.go`、`oidc.go`、`feishu.go` | 本地账号、LDAP/AD、OIDC、飞书免登；会话 cookie |
| 知识库 | `server/internal/kb` | 文档解析、切片、检索索引（进程内）、知识库 API |
| 数据中台 | `server/internal/datahub` | 独立的 `<主库名>_data` 库、上传的数据集、只读账号 |
| MCP / OpenAPI 客户端 | `server/internal/mcp`、`server/internal/openapi` | `/mcp` 上游连接；把 OpenAPI 文档变成工具 |
| 前端嵌入 | `server/internal/webui` | `go:embed` 打包 `web/dist`；未嵌前端时返回提示页，接口照常可用 |
| 管理端前端 | `web/` | Vite + React + Tailwind（shadcn/ui），中文/英文 i18n；菜单与页面由数据库驱动 |
| DSH 接入插件 | `dsh-plugin/` | `dsh-connect`：写 profile 配置、MCP 桥、模型提供方、技能来源、`/ee-login` 等命令 |
| 扩展 SDK 与示例 | `extensions/` | `sdk-go`、`sdk-local`、`examples/`、`official/`、`schemas/`、`skills/` |
| 部署与测试 | `deploy/`、`e2e/`、`scripts/` | compose、离线安装包、systemd；Playwright e2e；开发/OceanBase 测试脚本 |

## 3. 请求面与数据流

### 3.1 网页

- 浏览器加载内嵌的 SPA；`/api/*` 走会话 cookie（`dsh_ee_session`，HttpOnly、SameSite=Lax，HTTPS 部署时 `Secure`）。
- 写接口必须 `Content-Type: application/json`（`readJSON` 强制），配合 SameSite=Lax 防 CSRF。
- 菜单与页面存在数据库里（`menus` 表），前端只认 `page_key` 对应的组件；每个接口仍在后端按权限点校验，菜单只决定显示。
- 未匹配的 `/api/*` 返回 JSON 404，不会落到前端 `index.html`；其余路径回退到 SPA 路由。

### 3.2 模型网关 `/v1`

```
客户端（DSH / 自研 Harness）
  │  Authorization: Bearer <ee_at_ / ee_npm_ / sk-ee->
  ▼
GET /v1/models · POST /v1/chat/completions      server/internal/api/gateway.go
  │  1. resolveBearer：令牌 → 账号；2. 账号 RBAC ∩ 令牌 scope
  │  3. 账号额度、令牌额度、可用模型
  ▼
models（对外名称）→ 候选上游（按优先级）           providers / models 表
  │  4. 主密钥解密上游 key；按原始 JSON 转发（不二次序列化）
  ▼
上游 OpenAI 兼容服务（DeepSeek、通义、vLLM、Ollama…）
  │  5. 连不上 / 429 / 5xx 换下一条线路；开始写响应后不再切换
  ▼
返回原始响应（含流式）→ 记 usage_logs（账号/令牌/模型/用量/状态/耗时）
```

- `/v1` 错误沿用 OpenAI 格式；`X-DshEE-Client: dsh-connect/…` 的请求会把部分错误映射成 DSH 能直接展示的 400 文案（`/ee-login`、额度、模型不可用）。
- 流式请求会强制上游返回 `stream_options.include_usage`，否则统计不到 token。
- 请求体上限 16 MiB。没有 `/v1/responses`、`/v1/embeddings`、`/v1/completions`。
- 浏览器内第三方 Harness 直连 `/v1`、`/mcp` 的 CORS 白名单（`DSH_EE_CORS_ORIGINS`，默认关闭、精确回显 origin、不带 credentials）见 PR #127，合并后生效。

### 3.3 DSH 接入面

- `GET /api/dsh/catalog`：按当前令牌的员工身份返回此刻可用的技能、提示词、微应用、企业连接与 revision（带 ETag，没变化回 304）。
- `GET /api/dsh/skills/{name}`：取技能/提示词正文；`/api/dsh/apps/*`：微应用与订阅。
- `POST /mcp`：聚合式 MCP 网关（Streamable HTTP）。把该员工有权限的全部企业连接聚在一个端点，工具名 `<连接>__<工具>`；上游可以是 MCP 服务，也可以是导入的 OpenAPI 文档。凭据在 dsh-ee 解密后转发；写操作按 §5.5 的确认规则处理；`GET /mcp` 打开事件流，目录变化时推 `notifications/tools/list_changed`。基线 `tools/list` 一次返回全部工具、写确认只在文本里；`tools/list` 的 cursor 分页（`nextCursor`，`limit=0` 取全量）与写确认结构化 `_meta['dsh-ee/confirmation']`（id/url/expires_at）见 PR #128，合并后生效。
- `GET /setup.sh`、`GET /setup.ps1`：一键接入脚本（设备码登录 → 写 profile 的 `.npmrc` 与令牌 → 装接入插件）。
- `GET /index/dsh-index.json`：只含已上架插件元数据的 DSH 插件索引，不需要令牌。
- 第三方 Harness 的中立发现/能力摘要/只读应用描述与 `Dsh-EE-API-Version`（`/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps`）见 PR #123，合并后生效。
- `dsh-plugin/` 里的 `dsh-connect` 负责把这些能力装进员工 DSH；它每次请求现取 DSH 凭据库里的 `DSHEE_TOKEN`，所以服务端吊销/换发令牌不需要重装插件。

### 3.4 npm 源与插件发布

```
npm publish / 网页上传 .tgz
  ▼
解包 → 校验（包名作用域、name/version 与清单一致、sha512）
  ▼
自动检查（server/internal/registry/tarball.go）
  bundle / patch / cordis / cordis-peer / skills / install-scripts /
  secrets / license / engines / dependencies / size
  ▼
待审核（pending）──管理员审核──► 已上架（published）──► /npm packument 可见、可下载
  │                                │
  │ 开放模式且无失败项可自动上架      └─ 下架（unpublished）/ 驳回（rejected），可恢复
  ▼
tarball 按 sha512 存进 DSH_EE_DATA_DIR/blobs；download 记 npm_downloads
```

- 企业作用域（主作用域 + 其它内部作用域）**永不回源**；作用域之外的包按设置「按需回源并缓存」或「只提供已预载的包」。
- 上游预载会把插件与依赖闭包一起镜像，进入待审核；离线包导出/导入用于隔离网。
- 同一版本号永远不能重发；`npm unpublish` 返回 405，只能在网页下架。

### 3.5 扩展

三种扩展共用一个协议（见 [plugins.md](./plugins.md)、[extension-protocol.md](./extension-protocol.md)）：

| 形态 | 交付物 | 安装时 dsh-ee 做什么 |
|---|---|---|
| 声明式 | 清单 + OpenAPI 文档（zip） | 读清单和 OpenAPI，把每个操作变成一个工具；可选员工身份与共享凭据 |
| 服务扩展 | 一个可访问的 HTTP 服务 | 服务端去 `/.well-known/dsh-ee-extension.json` 取清单；调用时 dsh-ee 给凭据（共享 / `user_secret` / `ee_jwt` / OAuth），按 MCP 或约定接口转发 |
| 端侧扩展 | 员工本机运行的 npm 包 | 经企业 npm 源下发；本机 MCP 服务 + `/api/local/*` 回报审计 |

- 安装入口是管理员：`POST /api/admin/extensions/check|install`，网页「企业连接 → 安装扩展」。安装即完全信任；签名、信任根与升级回滚见 [plugins.md](./plugins.md) §9.3、§9.4。
- 服务扩展的 URL 由管理员填写；dsh-ee 会按清单声明的 `servers` 校验接口文档地址，调用时按清单声明的凭据方式带凭据。
- `ee_jwt` 的签名密钥由 `server/internal/eejwt` 定期从库里重读，轮换时新钥匙延迟生效（默认 10 分钟），避免正在处理的请求验签失败。

### 3.6 知识库与数据中台

- 知识库：文档解析、切片与关键词索引在进程内（重启后从数据库重建），按访问规则过滤；`kb_*` 表存知识库、文档、正文、查询与待补。
- 数据中台：默认在同一个数据库服务器上使用 `<主库名>_data`，上传的数据集在独立库；dsh-ee 自动创建只有 SELECT 权限的只读账号执行查询，密码随机生成、加密存在设置里。开不起来只影响这一块，其余功能照常。

## 4. 数据与状态

### 4.1 数据库

数据库是**唯一持久化的业务状态**。主库 SQL 与迁移在 `server/internal/store`（迁移文件 `server/internal/store/migrations/`，基线共 31 个，到 `0031_admin_roles.sql`）；数据中台的独立库由 `server/internal/datahub` 直接管理。启动时自动执行未应用的迁移。迁移历史上有新增表/列，也有删列、改菜单等变更（例如 `0003_rbac.sql` 删了 `users.role`），**升级前必须备份，回滚前看发布说明**。

按域大致分组：

| 域 | 主要表 |
|---|---|
| 账号与权限 | `users`、`roles`、`role_permissions`、`permissions`、`user_roles`、`menus`、`recovery_accounts` |
| 登录与令牌 | `sessions`、`access_tokens`、`api_keys`、`npm_tokens`、`device_codes`、`agent_confirms`、`user_credentials`、`ephemeral_states`（一次性状态：web link / 接入码 / OIDC state） |
| 模型 | `providers`、`models`、`usage_logs` |
| npm 源 | `npm_packages`、`npm_versions`、`npm_downloads`、`sync_jobs` |
| 能力中心 | `assets`、`asset_versions`、`asset_subscriptions`、`asset_usage`、`app_rows` |
| 企业连接 | `connection_tools`、`connection_calls`、`connection_shared_credentials`、`write_confirmations` |
| 知识库/字段/权限配置 | `kb_*`、`field_defs`、`field_values`、`access_rules`、`change_requests` |
| 运维与审计 | `audit_logs`、`settings`、`feedback`、`data_*`、`dsh_releases`、`background_leases`、`shared_rate_limits`（`/v1`、`/mcp` 每令牌共享限流窗口） |

`schema_migrations` 记录已执行的迁移。**多实例同时首次启动会争抢迁移**：先起一个，等它完成再扩容（[server/README.md](../server/README.md) 已知限制）。

### 4.2 数据目录

`DSH_EE_DATA_DIR` 下的内容全部可以重建/同步，但删了会丢已发布的 tarball：

| 路径 | 内容 | 丢了会怎样 |
|---|---|---|
| `blobs/` | 已发布/已镜像的 npm tarball，按 sha512 内容寻址 | 已发布版本无法下载；需重新发布或从上游/离线包恢复 |
| `upstream/` | 上游 packument 的磁盘缓存（默认 5 分钟 TTL） | 只是重新回源，可安全删除 |
| `brand/` | 管理员上传的 logo | 页面图标回退到默认 |
| `dsh-releases/` | 可选的 DSH 安装包镜像（下载中的 `.part` 文件） | 只是重新下载 |

### 4.3 进程内状态（不持久化）

- 登录失败限流（按用户名 5 次失败锁 15 分钟、按登录 IP 固定窗口）与设备码按 IP 限流，仍在进程内存；`/v1/*` 与 `/mcp` 的每令牌限流已改为数据库共享固定窗口（`shared_rate_limits`），多实例合计一份额度。
- 上游 packument 内存缓存、知识库关键词索引、流式响应、MCP 事件流。
- 预载/导入任务：任务记录在 `sync_jobs`，进程重启时启动阶段会把「运行中」的任务标为失败（`FailRunningJobs`），不会自动续跑。
- **已经不在进程内**：一次性状态（从 DSH 打开网页的 handoff code、开始使用接入码、OIDC state/PKCE）存在共享表 `ephemeral_states`，多个实例共享同一份语义；一次性和过期判断都在数据库上做，数据库不可用时直接失败（见 [operations.md](./operations.md) §9.2）。
- 后台周期任务（LDAP/飞书同步、知识源同步、上游新版本检查、接入插件发布、Desktop 版本同步、到期与清理任务）不在进程内存里排队：每轮先抢数据库表 `background_leases` 里的一条租约，只有持租实例执行（`internal/lease`）；执行中续约，任务结束/优雅停机释放，进程崩溃则 TTL 到期后由其他实例接管。数据库不可用时该轮跳过并记日志，不会退化成每个实例都跑。
- 其余进程内状态（登录限流、packument 缓存、知识库索引、MCP 事件流、能力目录变化通知）仍是每个实例一份：多实例部署需要把它们移出进程内存（设计）。

## 5. 认证、授权与信任边界

### 5.1 网页会话

- 登录方式：本地账号（bcrypt）、LDAP/AD、OIDC、飞书免登；本地账号优先——库里有同名本地账号时不会落到目录验证。
- 会话 cookie：HttpOnly、SameSite=Lax、`Secure` 由 `DSH_EE_COOKIE_SECURE` 控制；`DSH_EE_SESSION_TTL` 默认 168h；密码修改后所有会话失效；禁用/重置用户会踢下线。
- 成功/失败登录都写审计（`auth.login` / `auth.login_failed`）；连续失败 5 次锁 15 分钟。

### 5.2 令牌

服务端只存令牌的 sha256，明文只显示一次。`resolveBearer`（`server/internal/api/principal.go`）识别三种前缀：

| 前缀 | 来源 | scope | 备注 |
|---|---|---|---|
| `ee_at_` | 「我的令牌」/管理员令牌管理/设备码流程 | 可选的 `models`、`registry:read`、`registry:publish`、`hub`、`connections`、`agent`、`config`；`admin` 只能配合管理模式短时生效 | 可设令牌级月度 token 上限、模型范围、到期时间；支持按 ID / 用户 / 设备 / 部门批量吊销 |
| `ee_npm_` | 旧版 npm 令牌（只读/可发布） | 固定为 `models` + `registry:read` + `hub` + `connections`（可发布再加 `registry:publish`） | 权限比名称看起来宽，属于历史行为；新集成请用 `ee_at_` |
| `sk-ee_` | 旧版个人 API key | 固定为 `models` + `hub` + `connections` | 同上 |

设备码流程是 RFC 8628 的子集：`POST /api/device/code` 发起，员工在 `/device` 网页登录后批准，`POST /api/device/token` 领取（只能领一次，轮询过快返回 `slow_down`，默认滑动有效 90 天）。

### 5.3 权限 = scope ∩ RBAC

- 每个账号有角色，权限取角色并集；超级管理员拥有全部权限点。
- 令牌 scope 与账号权限**求交集**（`rbac.ScopeFilter`）：令牌不能给持有者扩大权限，只能收窄。
- 每个接口在 `routes.go` 声明认证方式和权限点，后端逐请求校验；菜单/页面权限只是显示控制。
- 防提权：非超级管理员只能授予自己拥有的权限；不能修改或禁用权限比自己大的用户；最后一个启用的超级管理员不能被取消/禁用。

### 5.4 Agent 与「管理模式」

- 路由表里的每条路由通常会生成同名的 Agent 工具（`ee_*`）；不能暴露给 Agent 的路由必须登记在 `agentExcluded` 并写明原因，测试会卡住未登记的路由。
- Agent 用带 `agent` scope 的 `ee_at_` 令牌，等价于「代这位员工操作」，权限仍是账号 RBAC ∩ scope。
- 管理模式：有管理权限的人在网页给**自己的**令牌开 1 或 8 小时管理权限（`POST /api/me/tokens/{id}/elevate`，要再输一次密码），到期自动退回员工权限。Agent 不能开启，也不能替人输密码。
- 权限配置即代码（`dsh-ee config …`）可以导出、试算、提交变更请求，但**批准永远在网页上由人完成**。

### 5.5 信任边界

| 边界 | 事实 | 含义 |
|---|---|---|
| 上游模型 | 上游 key 用主密钥加密存库，网关解密后转发 | 主密钥和数据库要同等保护；换主密钥会让已存凭据解不开 |
| 企业连接凭据 | 共享凭据/员工 `user_secret` 加密存库；`ee_jwt` 是 dsh-ee 现签的 5 分钟 JWT | 源系统仍要自己鉴权；dsh-ee 只负责「告诉它这是谁」 |
| 写操作 | 默认 `non_dsh`：非 DSH 客户端先返回确认链接，本人确认后同参数再调一次才执行；方式可设 `off` / `always` | 判断客户端靠 `X-DshEE-Client`，可伪造；这道确认防的是「客户端被设成自动同意」，不是防恶意客户端 |
| 扩展 | 安装 = 管理员完全信任；服务扩展可拿到 `ee_jwt` / `user_secret`，URL 由管理员填 | 签名 / 信任根 / 升级回滚见 [plugins.md](./plugins.md) §9.3、§9.4；回滚会把历史快照当新版本重新检查 |
| 反向代理 | `clientIP` 只在直连对端是回环/内网地址时才取 `X-Forwarded-For` 的最后一跳 | 代理必须正确设置 XFF；否则审计里可能是代理 IP |
| 公开地址 | `DSH_EE_PUBLIC_URL` 为空时按请求 Host/`X-Forwarded-*` 推断 | 生产必须显式配置，避免 Host 伪造影响生成的外部 URL |
| 浏览器跨域 | 基线没有 CORS；浏览器内第三方 Harness 的跨站直连见 PR #127（`DSH_EE_CORS_ORIGINS` 白名单，默认关闭，只覆盖 `/v1/*` 与 `/mcp`） | 合并前第三方 Harness 应在服务端/本机进程调用；合并后 CORS 也不是认证边界 |

## 6. 审计与可观测

| 数据 | 位置 | 内容 |
|---|---|---|
| 操作审计 | `audit_logs`，`GET /api/admin/audit`（按动作、操作人、时间、关键词过滤） | 登录/密码、用户/角色/权限/菜单、模型接入、设置、发布与审核、扩展与连接、数据中台、DSH 分发、npm 令牌等；`via` 区分 `user`/`token`/`agent`/`system`/`cli`，Agent 与令牌操作会记工具名和令牌 ID |
| 模型用量 | `usage_logs` | 账号、令牌、模型、上游、prompt/completion/token 数、状态码、耗时；`/api/admin/stats` 与运营报表读它 |
| npm 下载 | `npm_downloads` | 每次 tarball 下载（安装审计与企业内热门） |
| 连接调用 | `connection_calls` | 用户、工具、连接、扩展、auth_mode；写确认另记 `args_sha256` |
| 运行日志 | 进程 stderr（slog TextHandler） | 启动/关闭、每个 HTTP 请求（method/path/status/ms）、后台任务告警、审计写失败等；没有日志级别配置、轮转或 syslog（设计） |
| 健康检查 | `GET /healthz` | 公开，返回 `{"status":"ok","version":"…"}`；**只证明进程活着，不检查数据库** |

没有指标（Prometheus）、链路追踪、慢请求日志、审计导出/保留期设置——这些在 [roadmap.md](./roadmap.md) §4「企业级运行能力」里列为后续工作。

## 7. 独立 PR（尚未合入）与明确没有实现

以下能力**当前基线代码里没有**，分两类：有独立 PR、尚未合入的标「PR #N，合并后生效」；没有 PR 的实现计划标「设计」。文档、PR 或路线图里出现时都不要当成已发布能力。

| 能力 | 现状 | 参考 |
|---|---|---|
| 多实例完整横向扩展 | 周期后台任务已有租约选主（`background_leases`，见 §4.3 与 [operations.md](./operations.md) §9.2）；登录/设备码限流、缓存、预载任务、MCP 事件流仍在进程内存，每令牌限流已用数据库共享固定窗口（`shared_rate_limits`）；tarball 在本地磁盘，迁移要求先起一个，作业级归属（`FailRunningJobs`、管理员触发的镜像任务）未做 | [operations.md](./operations.md) §9.2、[server/README.md](../server/README.md) 已知限制 |
| 对象存储 | tarball 只写本地 `DSH_EE_DATA_DIR` | 同上 |
| 备份/恢复工具 | 仓库没有内置命令；`.env` 必须和数据库一起备份 | [operations.md](./operations.md)、[roadmap.md](./roadmap.md) 阶段 C |
| 指标/追踪/慢请求日志 | 只有文本日志和 `/healthz` | roadmap 阶段「企业级运行能力」 |
| 日志级别/轮转/syslog | 固定 Info 级别写 stderr | 同上 |
| 审计导出、保留期、外发 | 只有网页查询接口 | roadmap 阶段「企业级运行能力」 |
| MCP `tools/list` 分页、写确认结构化 `_meta` | 基线一次全量 + 文本确认；见 PR #128，合并后生效 | [troubleshooting.md](./troubleshooting.md) §4.2 |
| CORS（浏览器内第三方 Harness） | 见 PR #127（白名单 `DSH_EE_CORS_ORIGINS`，默认关闭），合并后生效 | [l1-client.md](./l1-client.md) |
| Git 安装、自动升级、作者自助发布 | 没有；安装只经管理员。扩展签名、企业目录登记、版本回滚已做最小闭环 | [plugins.md](./plugins.md) §9 |
| 第三方 Harness 发现/能力/版本头端点 | 见 PR #123（`/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps`、`Dsh-EE-API-Version`），合并后生效 | 合入前以 [l1-client.md](./l1-client.md) 与 `/mcp`、`/v1` 为准 |
| 微应用嵌入 v0/v1 | 见 PR #118：同站 v0（CSP/安全头、`?embed=1&theme=`、`DSH_EE_EMBED_ORIGINS`、postMessage v1）+ 跨站 v1（默认关闭，需 `DSH_EE_EMBED_CROSS_SITE=1` 且配置 `DSH_EE_EMBED_ORIGINS`；会话 TTL `DSH_EE_EMBED_SESSION_TTL`）。宿主接入见 [embedding.md](./embedding.md)，跨站设计与未做项见 [cross-site-embedding.md](./cross-site-embedding.md)（两份文档由 #118/#130 新增，合并后可用）；能力以 #118 合入为准 | 合入前 `server/README.md` 没有这些配置 |
| 跨站嵌入 v1 | 已由 PR #118 实现，默认关闭，需 `DSH_EE_EMBED_CROSS_SITE=1` + `DSH_EE_EMBED_ORIGINS`；未做项：管理端设置页、完整扩展消息（`navigate`/`navigate-request`/`open-external`/`logout`、深链 tab 同步）、`fit=auto` sticky 头处理、移动端策略、多实例共享 code/会话存储 | [cross-site-embedding.md](./cross-site-embedding.md)（#130 设计/实现参考） |
| `dsh-ee package check` / `doctor` | 见 PR #122，合并后生效；本文 [cli.md](./cli.md) 按该 PR 代码写 | [cli.md](./cli.md) |
| 编程式令牌签发/续期/introspection | 没有；设备码批准仍需网页 | [l1-client.md](./l1-client.md) §1 |

> 上表中标「PR #N，合并后生效」的能力以对应 PR 合入为准；合入后需要同步改本表与相关章节，并删掉临时的「见 PR」表述。微应用嵌入的宿主接入看 #118 的 `docs/embedding.md`，跨站设计与未做项看 #130 的 `docs/cross-site-embedding.md`（文档随 PR 合入后可用）。

## 8. 与 design.md 的差异

| design.md 的说法 | 当前实现 |
|---|---|
| 默认 SQLite，可选 PostgreSQL + S3/MinIO | 只有 MySQL 协议的 TiDB / OceanBase；tarball 在本地数据目录 |
| 安装向导生成企业 CA 相关客户端配置 | 没有安装向导；`setup.sh`/`setup.ps1` 走设备码并写 profile 的 `.npmrc` |
| 自研 registry 是核心 | 仍然成立，但另外有模型网关、MCP 网关、知识库、数据中台、DSH 分发等模块 |
| 评分可开关 | 没有评分/排行；能力市场只展示已上架内容 |
| 认证：网页本地/LDAP/OIDC + pnpm 个人 token | 另有飞书免登、统一 `ee_at_` 令牌、设备码、机器人账号、管理模式 |
| 一键部署默认 SQLite | compose + 离线包；数据库是外部/独立容器 |

## 9. 延伸阅读

- 部署与运维：[operations.md](./operations.md)
- 命令行：[cli.md](./cli.md)
- 故障排查：[troubleshooting.md](./troubleshooting.md)
- 插件作者：[plugin-authors.md](./plugin-authors.md)
- 扩展协议：[extension-protocol.md](./extension-protocol.md)、[plugins.md](./plugins.md)
- 权限配置：[access-control.md](./access-control.md)
- 第三方 Harness / L1 客户端：[l1-client.md](./l1-client.md)
- 员工身份（`ee_jwt` / `user_secret`）：[ee-jwt.md](./ee-jwt.md)
- 能力中心与 DSH 接入：[capability-hub.md](./capability-hub.md)、[agent-connect-tech.md](./agent-connect-tech.md)
- 路线图与已知限制：[roadmap.md](./roadmap.md)、[server/README.md](../server/README.md)
