# dsh-ee server

Go 写的 dsh-ee 服务端。目前实现：

- **用户登录**：本地账号（bcrypt），HttpOnly 会话 cookie，同一用户名连续失败 5 次锁定 15 分钟；管理员可建用户、禁用、重置密码（禁用 / 重置后立即踢下线）。
- **账号月度额度**：管理员给账号分配每月 token 上限（`NULL` 不限额、`0` 未分配）、允许的模型与有效期；额度按账号共享，账号下所有 key 从同一份里扣，每月 1 日（UTC）重置。
- **企业 API key 二次分发**：管理员录入企业在上游开通的模型服务（OpenAI 兼容，如 DeepSeek），上游 key 用主密钥 AES-256-GCM 加密存库；`sk-ee-…` 个人 key 由管理员代发或员工自助创建，随时吊销。
- **OpenAI 兼容网关**：`/v1/chat/completions`（含流式）与 `/v1/models`。员工用个人 key 调用，dsh-ee 换成企业 key 转发，按账号校验额度与模型，并按次记录用量（即调用审计）。
- **内置 npm 源**（`/npm/`）：企业作用域的托管与发布、逐版本审核、上游回源与镜像、上游预载（连同依赖闭包）、离线包导入导出，以及由已上架插件生成的 DSH 插件索引（`/index/dsh-index.json`）。详见下文「npm 源」。
- **LDAP / AD 登录**：员工用公司域账号登录网页与 `npm login`；首次登录自动开通，每次登录同步显示名、部门、邮箱与组映射的角色；每小时核对目录，离职（被删除或在 AD 里禁用）的账号自动禁用。详见下文「LDAP / AD」。
- **运营统计与审计日志**：按本地日期统计能力安装、模型调用与 Token，按部门、能力、模型、终端（npm 客户端 User-Agent 里的系统与架构）汇总；登录、账号与权限、模型接入、设置、发布与审核等操作写入审计日志。
- **能力中心**：技能、提示词、微应用与企业连接的版本、审核、可用范围（部门 / 角色 / 指定人）与下发方式（自动生效 / 自己启用）；插件也可以设范围与「自动安装」。详见下文「能力中心与 DSH 接入」。
- **单文件部署**：前端在构建时嵌进二进制（`internal/webui`），同一个端口提供界面与全部接口。部署方式见 [../deploy](../deploy/README.md)。

数据库走 MySQL 协议：**OceanBase（MySQL 模式）** 与 **TiDB** 通用。所有 SQL 都在 `internal/store`，以后接其它方言只改这一层。

## 权限（RBAC）

- **权限点**写在代码里（`internal/rbac`），每次启动同步进数据库；新权限点第一次出现时自动授予它声明的默认角色。管理员也可以建自定义权限点，给还没有后端的页面控制访问。
- **角色**存在数据库，可随时增删改；用户可以有多个角色，权限取并集。内置「超级管理员」自动拥有全部权限（包括以后新增的），内置「员工」是新用户的默认角色。
- **菜单 / 页面**存在 `menus` 表：标题、路由、图标、分组、排序、是否显示、访问所需权限都能在线改。前端只认 `page_key` 对应的组件，路由按菜单动态生成。
- 每个接口都在后端按权限点校验，菜单只决定显示。权限改动对已登录用户立即生效（每个请求现查）。
- 防提权：非超级管理员只能授予自己拥有的权限，不能修改或禁用权限比自己大的用户；最后一个启用中的超级管理员不能被取消或禁用。

## LDAP / AD

在「系统设置 → 登录与认证」里配置，服务账号密码用主密钥加密存库，接口不回显。

- **流程**：服务账号绑定 → 按用户过滤条件（`{username}` 按 RFC 4515 转义）找到唯一条目 → 读组（`memberOf` 属性，或在组的搜索起点下按过滤条件搜）→ 用用户 DN 与密码再绑定一次验证密码。空密码一律拒绝（目录会把空密码绑定当成「未认证绑定」并返回成功）；过滤条件匹配到多个条目也拒绝。
- **本地账号优先**：库里有同名本地账号时只用本地密码验证，即使开了 LDAP。管理员在目录故障时仍能登录，目录里同名的人也冒用不了本地账号。
- **组 → 角色**：映射里出现的角色由目录决定，每次登录和每次同步按所属组增删；其它角色保留管理员手动分配的。组可以写完整 DN 或只写组名（不区分大小写）。修改映射要有「管理角色与权限」权限，且只能映射自己能授予的角色。
- **账号**：首次登录自动开通（可关闭，关闭后只有在「用户」页预建的 LDAP 账号能登录）；新账号额度为 0，默认「员工」角色。LDAP 账号没有本地密码，不能在 dsh-ee 里改或重置。
- **目录同步**：每小时一次（也可以手动「立即同步」）。账号被删除或 AD 里 `userAccountControl` 带禁用位时，在本地禁用并踢下线，它的 API key 与 npm 令牌随之失效。连不上目录、搜索起点不存在、或超过一半的账号都找不到时整次放弃，不禁用任何人。
- **试连**：用页面上还没保存的配置连目录，可选地查一个账号的资料、所属组、映射到的角色，并验证密码。

## 能力中心与 DSH 接入

设计依据与取舍见 [docs/capability-hub.md](../docs/capability-hub.md)。

- **统一接入令牌**：`ee_at_…` 同时用于 npm 源、模型网关（`/v1`）、技能目录（`/api/dsh/*`）与企业连接（`/mcp`）。每次请求按账号 RBAC 与令牌 scope 求交集，可配置令牌级月度额度、模型范围和有效期；明文只显示一次，服务端只存 sha256。旧 `ee_npm_…`、`sk-ee-…` 保持原有权限与额度行为。
- **设备登录**：`POST /api/device/code` 发起 RFC 8628 设备码流程，员工在 `/device` 登录并确认；设备令牌只能领取一次，默认滑动有效 90 天，同账号同设备再次登录会吊销旧令牌。机器人账号由管理员创建并指定负责人，不能会话登录。
- **技能、提示词、微应用**：存在 `assets` / `asset_versions`；员工提交的版本在白名单模式下要审核。DSH 通过 `GET /api/dsh/catalog`（带 ETag，没变化回 304）拿到自己此刻应有的目录，`GET /api/dsh/skills/{name}` 取正文。提示词与微应用以「只给人调用」的 skill 下发（DSH 里输入 `/名字`）。
- **企业连接（MCP 网关）**：`/mcp` 是一个 Streamable HTTP MCP 服务，聚合员工有权使用的全部连接，工具名 `<连接>__<工具>`。上游可以是 MCP 服务，也可以是导入的 OpenAPI 文档（每个操作一个工具，写操作默认关闭）。凭据用主密钥加密存在 dsh-ee，按员工逐次校验后带上凭据转发，调用记入 `connection_calls`。同一连接可有多份命名共享凭据，`spec.auth.shared_name` 选择实际使用的一份，空值继续用原来的默认凭据；最近调用记录所选名称。详见 [docs/named-shared-credentials.md](../docs/named-shared-credentials.md)。GET `/mcp` 打开事件流，能力目录变化时推 `notifications/tools/list_changed`。
- **DSH 接入插件**：dsh-ee 按系统设置生成 `<主作用域>/dsh-connect`（远程技能来源、插件内 MCP 桥、可选模型提供方与插件索引、自动安装必装插件），内容变化时自动发布新版本。插件每次请求现取 DSH 凭据库中的 `DSHEE_TOKEN`，支持 `/ee-login`、`/ee-status`、`/ee-logout`，失效时清理旧凭据并同步 npm 配置。
- **一键接入**：macOS / Linux 执行 `curl -fsSL <dsh-ee>/setup.sh | sh`，Windows 执行 `irm <dsh-ee>/setup.ps1 | iex`。脚本走设备码登录，把 npm 源与令牌写进 DSH profile 目录的 `.npmrc`（`$DSH_HOME/profiles/<profile>/.npmrc`，DSH 跑 pnpm 时的工作目录；不改 `~/.npmrc`，`DSH_EE_GLOBAL_NPMRC=1` 时才额外写全局），以及权限为 0600 的 `$DSH_HOME/dsh-ee/token.pending`，插件首次启动将令牌导入凭据库后删除待导入文件。`setup.sh --token <令牌>` 或 `DSH_EE_TOKEN` 保留手动 / CI 流程，并继续写 `$DSH_HOME/.env`。服务地址由 `DSH_EE_PUBLIC_URL` 或请求地址决定。不想用终端的员工可以在 DSH 插件页「添加插件」里粘贴免登录地址 `<dsh-ee>/dl/connect/dsh-connect-<版本>.tgz`（接入指南页显示），接入插件登录后自己维护 profile 的 `.npmrc`。
- **升级与回滚**：升级数据库会自动新增令牌表和列，原主密钥必须保留。回滚到旧服务端后 `ee_at_` 不可用，需要改回旧 `ee_npm_` / `sk-ee-`；旧版会忽略新增表和列。
- 已知限制：镜像的上游包不带 npm 签名，经 dsh-ee 下载并校验签名的工具（如 corepack 自动下载 pnpm）会失败；请先装好 pnpm 再运行接入脚本。

## 飞书（v0.2 分批实现）

`docs/feishu.md` 记录当前飞书范围：OIDC 免登登录、部门与人员的完整分页同步、手动/定时同步、目录缺席/删除/冻结后的按配置禁用。管理员在「登录方式」选择飞书，填写 App ID / App Secret 与通讯录地址，secret 不回显；登录和组织同步复用它，但不把这一纵切视为通知、卡片审批或飞书工作台都已完成。

`GET /api/admin/auth/feishu/sync` 返回最近一次同步结果；`POST` 手动执行一次完整同步。后台只在 `preset=feishu` 且 `feishuSync` 开启时每小时执行。外部 subject 统一 `feishu:<open_id>`，不触及其它 IdP；目录失败或为空不批量禁用。

`feishuSync`（组织同步）之外还有 `feishuNotify`（普通通知）与 `feishuNotifyActions`（卡片审批）两个独立开关，默认均关闭。卡片批准用一次性随机 action ID、消息号匹配、当前 RBAC、过期/状态检查和原子状态机，不用按钮或网页 cookie 代替权限判断。

`POST /api/auth/feishu/card` 接收卡片回调；`/api/admin/auth/feishu/workspace-url?next=/a/<slug>` 生成飞书工作台可打开的 OIDC 入口，只允许 `/a/` 或 `/go`，目标应用继续沿用原 Scope，不要求飞书服务端跑 AI。详见 [../docs/feishu.md](../docs/feishu.md)。

## 大模型接入

- 上游一律走 OpenAI 兼容接口（DeepSeek、通义千问、智谱、豆包、Kimi、硅基流动、OpenAI、Ollama、vLLM 等都提供），网关按原始 JSON 透传，不经 SDK 二次序列化，厂商扩展字段（如 `reasoning_content`）原样保留。
- 模型单独成表：员工调用时写**对外名称**，转发时换成**上游模型名**。同一个对外名称可以挂在多个上游上，按上游优先级依次尝试，连不上、429、5xx 时自动切到下一条（开始向客户端写响应后不再切换）。
- 「检查连接」请求上游 `/models`；上游不提供该接口时，用一个已配置的模型发一次 `max_tokens=1` 的对话验证。结果会存下来显示在管理端。

## npm 源

DSH 的 `dsh plugin add` 在 profile 目录里转发给 pnpm，所以「能力市场」的核心就是一个 npm registry。dsh-ee 自己实现了 npm 协议需要的那部分：

- **packument / tarball**：`GET /npm/<包名>`（支持 `Accept: application/vnd.npm.install-v1+json` 精简文档）、`GET /npm/<包名>/-/<文件>.tgz`。tarball 按 sha512 内容寻址存在 `DSH_EE_DATA_DIR/blobs`。
- **发布**：`npm publish` / `pnpm publish`，或网页上传 `npm pack` 的 `.tgz`。只能发布到企业作用域（系统设置里的主作用域 + 其它内部作用域）；包的第一个发布者是负责人，之后只有他和有「管理能力与审核」权限的人能发新版本；同一版本号永远不能重发（被驳回、下架的也不行）。对外的清单以 **tarball 里的 package.json 为准**，与发布元数据不一致（name / version / 摘要）直接拒收，防 manifest confusion。
- **审核状态**（逐版本）：`pending` 待审核 → `published` 已上架 / `rejected` 已驳回；已上架可 `unpublished` 下架，下架与驳回可恢复。只有 `published` 的版本出现在 packument、能下载、进 DSH 索引；`latest` 指向未上架版本时自动回落到最高的已上架正式版。收录模式为「开放」时发布即上架（自动检查有失败项的仍进待审核）。
- **自动检查**（发布、预载、导入时做，审核页展示）：`dsh.bundle` 与 `patch` 文件是否存在且路径安全、安装脚本、疑似密钥与敏感文件（`.env`、`.npmrc`、私钥、各家 token 格式）、License、`engines.dsh`、社区依赖、体积。
- **企业作用域永不回源**：作用域内的包名、以及曾经在本地托管过的包名，无论设置怎么改都只查本地，防依赖混淆。
- **回源**：「按需回源并缓存」时，作用域之外的包取上游 packument（磁盘缓存 5 分钟，带 ETag 重新验证，上游不可用时用旧缓存），tarball 第一次下载时拉取、校验 integrity 后存下；「只提供已预载的包」或离线模式下只用本地已有的版本。**声明了 `dsh.bundle` 的上游版本（即社区插件）在白名单模式下必须先在本地审核上架才可见**，普通依赖不需要审核。
- **上游预载**：从 dshfind 插件目录（DSH 索引 format 1）挑插件，镜像插件本身与依赖闭包（dependencies、optionalDependencies、必需的 peerDependencies，按 npm 的 semver 规则解析，含预发布版本的规则），记录闭包大小与是否离线可装。插件进入待审核（或勾选直接上架），依赖直接可用。每 6 小时按「新版本」设置检查已预载插件的新版本。员工也可以「申请引入」某个包，走同样的流程进入待审核。
- **离线包**：能上网的实例导出 tar（`dsh-ee-bundle.json` 清单 + 按 sha512 命名的 tarball），拷进隔离网实例导入；导入时逐个重算 sha512，与文件名、清单 integrity 都一致才入库，插件版本进入待审核。
- **令牌**：统一 `ee_at_…` 令牌按 `registry:read` / `registry:publish` scope 控制 npm 权限；旧 `ee_npm_…` 只读 / 可发布令牌与 `npm login --auth-type=legacy` 继续支持。设置里「安装也需要令牌」关闭后匿名可装，发布总是要令牌。每次 tarball 下载记入 `npm_downloads`（安装审计与企业内热门）。
- **接入脚本**：`curl -fsSL <dsh-ee>/setup.sh | sh`（Windows：`irm <dsh-ee>/setup.ps1 | iex`）通过设备确认后用 `npm config` 写好 registry、作用域与令牌；无浏览器环境用 `sh -s -- --token <令牌>`。
- **DSH 插件索引**：`GET /index/dsh-index.json`，按 dshfind 的 `dsh-index.mjs` 同一口径生成并自检（读端任何一条违规就整份拒收）。只含已上架插件的元数据，**不需要令牌**（DSH 请求索引源时不一定能带认证头）；下载 tarball 仍然要令牌。
- **插件作者指南**：`package.json` / `dsh.bundle` / 发布前预检 / 常见失败见 [docs/plugin-authors.md](../docs/plugin-authors.md)。

不支持：`npm unpublish`（返回 405，请在网页下架）、npm 的 web 登录、组织与团队权限、漏洞审计（`/-/npm/v1/security/*` 返回空结果）、部门级可见范围（P1）。

## 本地开发

需要 Go 1.27+ 和一个 TiDB 或 OceanBase。

```bash
# 1. 起一个单机 TiDB（数据保存在 tag dsh-ee 下，下次同样命令会复用）
~/.tiup/bin/tiup playground v8.5.8 --tag dsh-ee --tiflash 0 --without-monitor --host 127.0.0.1

# 2. 配置：复制示例并填好密钥
cp .env.example .env.local
#   DSH_EE_SECRET_KEY 用 openssl rand -base64 32 生成；丢了它，已存的上游 key 就解不开了

# 3. 运行（首次启动会自动建库、建表、创建管理员）
set -a; . ./.env.local; set +a
go run ./cmd/dsh-ee

# 4. 测试（每次建随机库，跑完删除）
DSH_EE_TEST_DSN='root:@tcp(127.0.0.1:4000)/' go test ./...

# 5. 补演示数据（角色、员工、额度、上游模型与 key；可重复执行）
go run ./cmd/seed
```

`cmd/seed` 通过 HTTP 接口写入，需要管理员密码：默认读 `DSH_EE_BOOTSTRAP_ADMIN_PASSWORD`，也可以用
`DSH_EE_SEED_ADMIN_PASSWORD` 指定；员工初始密码用 `DSH_EE_SEED_USER_PASSWORD`（默认 `dsh-ee-demo-2026`）。
上游 key 默认是占位值，接真实上游时传 `DSH_EE_SEED_DEEPSEEK_KEY` / `DSH_EE_SEED_GATEWAY_KEY`。

前端开发时用 Vite（见仓库根目录 README）；要一个嵌好前端的单文件，在仓库根目录 `make build`，产物是 `dist/dsh-ee`。

### 接 OceanBase

用 OceanBase 社区版的 MySQL 模式租户，DSN 形如：

```
DSH_EE_DSN='user@tenant:password@tcp(ob-host:2881)/dsh_ee'
```

OceanBase 单机最低也要几 GB 内存，本地机器紧张时先用 TiDB 调试。在 OceanBase 上跑一遍完整测试（Go 集成测试 + 浏览器端到端测试）：找一台 2 核 8 GB 以上、装了 docker 的机器执行 `scripts/test-oceanbase.sh`，或在 GitHub Actions 里手动触发 `oceanbase` 工作流（每周日也会自动跑一次）。

## 配置

| 环境变量 | 说明 |
|---|---|
| `DSH_EE_DSN` | 必填。MySQL 协议 DSN；库不存在时会尝试创建 |
| `DSH_EE_SECRET_KEY` | 必填。32 字节 base64 主密钥，加密上游 key |
| `DSH_EE_ADDR` | 监听地址，默认 `127.0.0.1:8080` |
| `DSH_EE_BOOTSTRAP_ADMIN` / `_PASSWORD` | 用户表为空时创建的管理员，默认用户名 `admin` |
| `DSH_EE_COOKIE_SECURE` | 部署在 HTTPS 后面时设为 `true` |
| `DSH_EE_SESSION_TTL` | 会话有效期，默认 `168h` |
| `DSH_EE_DATA_DIR` | 数据目录（npm tarball、上游缓存），默认 `./data`；容器里是 `/data` |
| `DSH_EE_PUBLIC_URL` | 对外地址，如 `https://dsh.acme.internal`；npm 的 tarball 地址按它生成，不填时按请求的 Host / `X-Forwarded-*` 推断 |
| `DSH_EE_DB_WAIT` | 启动时等数据库就绪的最长时间，默认 `2m`（compose 里是 `15m`） |

## 接口

管理与员工接口用会话 cookie；写操作必须 `Content-Type: application/json`（配合 SameSite=Lax 防 CSRF）。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | `{username, password}` |
| POST | `/api/auth/logout` | |
| GET | `/api/auth/me` | 当前用户、角色、有效权限点、是否超级管理员 |
| GET | `/api/me/menus` | 当前用户能访问的菜单（已按权限过滤，含隐藏页面） |
| POST | `/api/auth/password` | `{old_password, new_password}`，成功后所有会话失效 |
| GET | `/api/me/keys` | 旧版 key、本月用量与额度 |
| POST | `/api/me/keys` | 自助创建旧版 key `{name, expires_at?}`，明文只返回这一次 |
| DELETE | `/api/me/keys/{id}` | 吊销自己的旧版 key |
| GET / POST | `/api/me/tokens` | 统一新旧令牌列表 / 创建 `ee_at_` `{name, scopes, monthly_token_limit?, allowed_models?, expires_in_days?}` |
| DELETE | `/api/me/tokens/{id}` | 按 `at:` / `npm:` / `key:` 前缀吊销自己的令牌 |
| GET / POST | `/api/admin/users` | 用户列表（含角色） / 新建 `{username, password, display_name, department, role_ids?, monthly_token_limit?}`；机器人用 `account_type: bot` + `owner_id` 且不设密码；不传 `role_ids` 给默认的「员工」角色，不传额度默认 `0`（未分配） |
| PATCH | `/api/admin/users/{id}` | `{display_name, department, disabled, password}` 任选 |
| PUT | `/api/admin/users/{id}/roles` | `{role_ids}` 整体替换用户的角色 |
| GET / POST | `/api/admin/permissions` | 权限点列表 / 新建自定义权限点 `{code, name, group, description}` |
| DELETE | `/api/admin/permissions/{code}` | 删除自定义权限点（仍被菜单引用时拒绝） |
| GET / POST | `/api/admin/roles` | 角色列表（含权限与人数） / 新建 `{code, name, description, permissions[]}` |
| PATCH / DELETE | `/api/admin/roles/{id}` | 修改 `{name, description, permissions}` / 删除（内置角色不能删） |
| GET / POST | `/api/admin/menus` | 全部菜单 / 新建 `{kind, title, path, page_key, icon, permission, parent_id, sort, visible}` |
| PATCH / DELETE | `/api/admin/menus/{id}` | 修改 / 删除（分组下有子项时拒绝） |
| PUT | `/api/admin/menus/order` | `{items: [{id, parent_id, sort}]}` 调整排序与分组 |
| POST | `/api/admin/menus/reset` | 恢复默认菜单 |
| PATCH | `/api/admin/users/{id}/quota` | `{monthly_token_limit, allowed_models, quota_expires_at}` 整份替换；显式 `null` 表示不限额 / 全部模型 / 永不过期 |
| GET / POST | `/api/admin/providers` | 上游列表 / 新建 `{name, vendor, base_url, api_key, priority, models?[]}`；不返回明文 key |
| PATCH / DELETE | `/api/admin/providers/{id}` | 修改 `{vendor, base_url, api_key, priority, enabled}` / 删除（连同模型） |
| POST | `/api/admin/providers/{id}/check` | 检查连接，返回上游提供的模型列表 |
| GET / POST | `/api/admin/models` | 全部模型 / 批量添加 `{provider_id, items: [{upstream_model, name?, display_name?}]}`，重名的跳过 |
| PATCH / DELETE | `/api/admin/models/{id}` | 修改 `{name, upstream_model, display_name, enabled}` / 删除 |
| GET / POST | `/api/admin/keys` | 旧版 key 列表 / 代发 `{user_id, name, expires_at?}`，明文只返回这一次 |
| DELETE | `/api/admin/keys/{id}` | 吊销旧版 key |
| GET / POST | `/api/admin/tokens` | 统一令牌筛选 / 给员工或机器人下发令牌 |
| POST | `/api/admin/tokens/revoke` | 按 id、用户、设备或部门批量吊销 |
| POST | `/api/device/code`、`/api/device/token` | 设备码发起与轮询领取（无需会话） |
| GET / POST | `/api/device/{user_code}` | 查看设备信息 / 登录后批准或拒绝 |
| GET | `/api/settings` | 系统设置（登录即可读） |
| PUT | `/api/admin/settings` | 整份替换；按改动的段落校验权限（npm 源 / 上游段落也可由对应管理员修改） |
| GET | `/api/catalog`、`/api/catalog/{包名}` | 能力市场：已上架的能力 / 详情（版本、检查结果、README） |
| GET / POST | `/api/me/packages` | 我发布的包与各版本状态 / 网页发布（请求体是 `.tgz`，`Content-Type: application/octet-stream`） |
| POST | `/api/me/packages/check` | 发布前预检：请求体同样是 `.tgz`，只解包并返回自动检查与发布冲突，不落库、不占用版本号 |
| POST | `/api/me/import-requests` | 申请引入社区包 `{name, version?, reason}` |
| GET / POST / DELETE | `/api/me/npm-tokens[/{id}]` | 我的 npm 令牌 / 新建 `{name, access: read\|publish}`（明文只返回一次）/ 吊销 |
| GET | `/api/admin/packages[/{包名}]` | 全部能力（含各种状态）/ 详情 |
| PATCH | `/api/admin/packages/{包名}` | 修改展示信息 `{title, description, category, kind, icon, featured}` |
| GET / POST | `/api/admin/review` | 待审核版本 / 审核 `{name, version, action: approve\|reject\|unpublish\|restore, note}`；`version` 为空时下架或恢复整个包 |
| GET | `/api/admin/tarball?name=&version=` | 下载任意状态的 tarball（审核用） |
| GET | `/api/admin/registry` | npm 源统计与当前路由 |
| POST | `/api/admin/registry/check` | 检查上游 registry 连通性 |
| GET / DELETE | `/api/admin/npm-tokens[/{id}]` | 全部令牌 / 吊销 |
| GET | `/api/admin/upstream/feed` | dshfind 插件目录 + 本地镜像状态 |
| POST | `/api/admin/upstream/sync` | 预载 `{names, publish}`，返回任务 |
| GET | `/api/admin/upstream/jobs[/{id}]` | 预载 / 引入 / 导入任务与日志 |
| POST | `/api/admin/upstream/check-updates` | 立即检查已预载插件的新版本 |
| GET | `/api/admin/upstream/export?since=` | 导出离线包（tar） |
| POST | `/api/admin/upstream/import` | 导入离线包（请求体是 tar） |
| * | `/npm/…` | npm 协议：packument、tarball、publish、deprecate、dist-tag、whoami、`npm login` |
| GET | `/index/dsh-index.json` | DSH 插件索引（format 1，不需要令牌） |
| GET | `/setup.sh`、`/setup.ps1` | 员工电脑的一键接入脚本 |
| GET | `/v1/models` | `Authorization: Bearer sk-ee-…` |
| POST | `/v1/chat/completions` | 同上，OpenAI 格式，支持 `stream` |

### 走一遍

```bash
# 登录
curl -c ck -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}' localhost:8080/api/auth/login

# 录入企业的 DeepSeek key
curl -b ck -H 'Content-Type: application/json' localhost:8080/api/admin/providers \
  -d '{"name":"deepseek","base_url":"https://api.deepseek.com","api_key":"sk-…","models":["deepseek-flash","deepseek-v4-pro"]}'

# 建员工，并给账号分配额度（每月 200 万 token，只允许两个模型）
curl -b ck -H 'Content-Type: application/json' localhost:8080/api/admin/users \
  -d '{"username":"zhang.wei","password":"至少十个字符的密码","department":"研发效能部"}'
curl -b ck -X PATCH -H 'Content-Type: application/json' localhost:8080/api/admin/users/2/quota \
  -d '{"monthly_token_limit":2000000,"allowed_models":["deepseek-chat","deepseek-reasoner"]}'

# 代发一个 key，或者让员工登录后自己 POST /api/me/keys 自助创建（额度共用账号的那一份）
curl -b ck -H 'Content-Type: application/json' localhost:8080/api/admin/keys \
  -d '{"user_id":2,"name":"zhang.wei 的 DSH"}'

# 员工用自己的 key 调用，任何 OpenAI 兼容客户端把 base_url 指向 http://<dsh-ee>/v1 即可
curl localhost:8080/v1/chat/completions -H 'Authorization: Bearer sk-ee-…' -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-flash","messages":[{"role":"user","content":"你好"}]}'
```

## 已知限制

- 额度按 UTC 自然月重置，检查在请求前做，单次请求可能略微超出额度。
- 登录限流在进程内存里，多实例部署时需改为共享存储。
- 管理操作（建用户、分配额度、代发 / 吊销 key）尚未写审计日志；网关调用已逐条记录在 `usage_logs`。
- 只支持本地账号；LDAP / OIDC 后续加。
- 多实例部署：登录限流、上游 packument 的内存缓存、预载任务都在单个进程里；多个实例同时第一次启动时数据库迁移会互相冲突（先起一个再扩容）。tarball 存在本地磁盘，多实例需要共享存储（NFS 等）或以后改成对象存储。
- 离线包没有签名（导入时只校验每个 tarball 的 sha512）；dshfind 签名种子包对接后再加 ed25519 校验。
- 上游 registry 只支持匿名访问，不支持需要认证的 Nexus / Artifactory 代理仓库。
