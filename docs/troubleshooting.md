# dsh-ee 故障排查

> 状态：按 `origin/main`（20aafbb，2026-11-12）核对。按「发布 / 安装 / 鉴权 / Harness / 扩展 / 嵌入」组织；每节只列**症状 → 原因 → 处理**，不重复整篇文档。带「PR #N，合并后生效」标记的是对应独立 PR 里的行为（#118 嵌入 v0、#122 CLI、#123 Harness 发现、#127 CORS、#128 MCP 分页/写确认结构化），**尚未合入基线**；能力以对应 PR 合入为准。
>
> 先做三件事：看 `GET /healthz`；看服务端日志（[operations.md](./operations.md) §7）；确认版本（`dsh-ee version`）。涉及具体接口配置的，先读 [architecture.md](./architecture.md) §3、§5。

## 0. 通用症状

| 症状 | 常见原因 | 处理 |
|---|---|---|
| `/healthz` 正常，但登录/接口 500 | 数据库连不上或迁移失败；`/healthz` **不检查数据库** | 看日志里 `连接数据库:`、`internal error`、`启动失败`；检查 DSN、数据库状态、账号权限；重启会自动重试迁移 |
| 启动即退出，日志 `启动失败 err=…` | 缺 `DSH_EE_DSN` / `DSH_EE_SECRET_KEY`、数据库不可达、迁移失败、`DSH_EE_PUBLIC_URL` 格式错 | 按错误修环境变量；`DSH_EE_SECRET_KEY` 必须是 32 字节 base64 |
| 页面打不开，提示「这个二进制没有嵌入前端」 | 二进制是没跑 `make build` / 没拷 `web/dist` 的版本 | 用完整构建产物或官方镜像；接口 `/api`、`/v1`、`/npm` 不受影响 |
| 页面样式/资源 404 | 反向代理只转发了 `/api`，没转发静态资源；或 PUBLIC_URL 带了子路径但代理没配 | 代理把 `/` 全量转到 dsh-ee；子路径部署需要同时改代理与 `DSH_EE_PUBLIC_URL` |
| npm 源报「npm 源没有初始化（数据目录不可写）」 | `DSH_EE_DATA_DIR` 不存在/不可写 | 建目录并给运行用户写权限；容器检查 `/data` 卷 |
| 发布/上传大包 413 或代理超时 | 反向代理请求体上限/读超时太小 | 按 [operations.md](./operations.md) §2.5 调整（publish/离线包导入需要更大的上限） |
| 流式回答变成一次性返回、或中途断开 | 代理缓冲/超时 | 关代理缓冲（`proxy_buffering off`），调大读超时；dsh-ee 自身不设 WriteTimeout |
| 版本号显示 `dev` | 构建时没注入 `-X main.version=…` | 用 `make build`/官方镜像；`dev` 不影响功能，但 `doctor`/审计里的版本不可用 |

## 1. 发布（插件作者把包发到 dsh-ee）

### 1.1 发布前预检（`package check`，PR #122，合并后生效）

| 退出码 / 现象 | 含义 | 处理 |
|---|---|---|
| 退出码 0「预检通过」 | 无发布冲突、无 `fail` 项 | 可以提交（是否自动上架看审核模式） |
| 退出码 1「不能提交」 | 有发布冲突（包名/作用域/版本/owner/已存在/上游预载） | 按输出逐条改：企业作用域前缀不对就改 `name`；版本已存在就换版本号；上游预载同名包不能发；别人的包不能发 |
| 退出码 1「进入人工审核」 | 自动检查有 `fail` 项 | 见 §1.2 的 scan key；修完重新 `npm pack` 再检查 |
| 退出码 2 | 参数/文件/地址/令牌不对 | 文件必须是 `npm pack` 的 `.tgz`；地址令牌来源见 [cli.md](./cli.md) §2.1 |
| 退出码 3 | 调用服务失败（网络、401/403、5xx） | 先 `dsh-ee doctor`（#122）看连通/令牌/权限；401/403 见 §1.3 |

`--json` 输出的 `can_submit=false`、`conflicts`、`scan[].level` 是判断依据；同一份结果网页「发布」页也会展示。

### 1.2 自动检查项（scan key）

服务端在任何发布/预载/导入时都会做同一组检查（`server/internal/registry/tarball.go`）。`fail` 会阻塞自动上架（仍可人工审核），`warn` 需要人工看一眼。

| key | 级别 | 含义 | 怎么修 |
|---|---|---|---|
| `bundle` | fail/warn | `dsh.bundle` 缺失、无 `patch`、路径不安全、指向的文件不在包里 | 普通 npm 包不需要 `dsh.bundle`（warn）；DSH 插件要声明 `dsh.bundle.patch`，且 patch 文件要进 `files` |
| `patch` | fail/warn | `cordis.patch.yml` 不是合法 YAML / 不是数组 / 缺 row id 或 name / id 重复 / 没有一行引用本包名 | 按输出里的行号修 YAML；每行 `id` 必须引用本包的插件名 |
| `cordis` | fail | 把 Cordis 放进了 `dependencies` | 移到 `peerDependencies`（只构建用可放 `devDependencies`），否则 DSH 里会出现双份 Cordis |
| `cordis-peer` | warn | `peerDependencies` 的 Cordis 范围不包含当前 DSH 版本 | 放宽/修正范围，或确认只兼容旧宿主 |
| `skills` | warn | 包内带技能 | 技能会随插件注入，不会出现在能力中心技能 Tab、不能单独授权；确认这是你想要的分发方式 |
| `install-scripts` | warn | 有安装脚本 | 确认安装时不会下载/执行外部代码；企业环境谨慎 |
| `secrets` | **fail** | 扫到疑似密钥/敏感文件（`.env`、`.npmrc`、私钥、各家 token 格式） | 从包里删掉，用占位符/文档说明；重新发布新版本（已发布版本不能重发） |
| `license` | warn | 没声明 `license` | 在 `package.json` 写 `license` |
| `engines` | warn | 没声明 `engines.dsh` | 声明兼容的 DSH 版本范围 |
| `dependencies` | pass | 列出了非企业作用域的依赖 | 只是提示；离线环境要确认这些依赖可获取 |
| `size` | warn | 包/解包体积偏大（包 > 20 MiB 等） | 检查 `files` 是否带了构建缓存、测试数据、`node_modules` |

### 1.3 发布被拒 / 审核问题

| 响应/现象 | 原因 | 处理 |
|---|---|---|
| `401 发布需要登录：在 dsh-ee 的「接入指南」里生成「可发布」令牌` | 没带令牌 | 用 `ee_at_`（含 `registry:publish`）或旧版可发布令牌；写进 profile 的 `.npmrc` |
| `403 你的账号没有发布能力的权限` | 账号 RBAC 缺发布权限 | 管理员给 `registry.publish` 或相应角色 |
| `403 这个令牌是只读的` | 用了只读令牌 | 换带 `registry:publish` 的令牌 |
| `400 请求路径里的包名与发布内容不一致` | `npm publish` 的包名与 tarball 内 `package.json` 不一致 | 改 `package.json` 的 `name`（必须以企业作用域开头） |
| `404 没有这个包或版本（企业作用域的包只在这里发布；社区插件需要管理员审核上架）` | 包不在已上架列表/需要审核 | 先在「发布」或「待审核」处理；社区插件由管理员引入 |
| `405 dsh-ee 不支持用 npm unpublish 删除版本` | 协议不支持删除 | 在网页「能力管理」下架，或发新版本 + `npm deprecate` |
| 版本已存在 | 同一版本号永远不能重发（驳回/下架过的也不行） | 升版本号 |
| 发布后 DSH 里看不到 | 审核模式为白名单且还未通过；或 `latest` 指向的版本未上架 | 看「待审核」；通过后 packument 的 `latest` 会回落到最高的已上架正式版 |
| 审核通过但装不上 | tarball 在数据目录缺失（迁移/恢复不完整） | 检查 `DSH_EE_DATA_DIR/blobs`；重新发布或从离线包恢复 |

### 1.4 `dsh-ee doctor`（PR #122，合并后生效）报告失败

`doctor` 的检查项 id 对应修法：

| id | 失败含义 | 修法 |
|---|---|---|
| `endpoint` | 没地址/地址不是 http(s) | `--server` / `DSH_EE_URL` / 跑一次 `setup.sh` 写 `.npmrc` |
| `server` | `/healthz` 连不上或版本异常 | 检查地址、网络/VPN、反向代理；浏览器打开 `<地址>/healthz` |
| `token` | 没令牌 / 无效 / 过期 / 吊销 | 在 `<地址>/tokens` 建令牌；过期就换；401 用 `--token` 重试 |
| `publish-scope` | 令牌没有发布预检权限 | 令牌要 `registry:publish` 或 `agent`；或账号缺发布权限 |
| `npm-source` | 企业 npm 源不可达 | 检查反向代理与数据目录；`npm publish` 会走这个源 |
| `npmrc` | `.npmrc` 里企业源/令牌与当前不一致 | 重跑 `setup.sh` 或重新登录同步；只改 `_authToken` 那一行 |
| `node` | 没装 Node / 版本过旧 | 装 Node 22+；用上游 `dsh-primer` 脚手架需要 ≥24.9 |
| `dsh` | 没装 DSH / `dsh --version` 无输出 | 只发布 npm 包可忽略；要在本机验证插件时装 DSH |

## 2. 安装（员工/管理员把插件装进 DSH）

| 症状 | 原因 | 处理 |
|---|---|---|
| `npm` 安装 401「这个 npm 源需要令牌」「npm 令牌无效或已吊销」 | profile 的 `.npmrc` 没令牌/令牌被吊销 | 重跑 `curl -fsSL <dsh-ee>/setup.sh \| sh`，或重新登录接入插件同步令牌 |
| 403「你的账号没有安装能力的权限」 | 账号 RBAC 缺安装能力权限 | 管理员开权限或改用有权限的账号 |
| 404「没有这个包或版本…」 | 包未上架/版本不存在/企业作用域包不被允许 | 确认版本已上架；社区插件先审核上架 |
| 匿名安装被拒（401「这个 npm 源需要令牌/需要登录」） | 管理员开了「安装也需要令牌」，或包要求认证 | 生成只读令牌写进 profile 的 `.npmrc`，或让管理员关闭该设置 |
| `corepack`/`pnpm` 自举失败 | dsh-ee 镜像的上游包不带 npm 签名，经 dsh-ee 下载后校验签名会失败（已知限制） | 先在本机装好 pnpm 再运行接入脚本（`server/README.md` 已知限制） |
| `setup.sh` 提示「当前环境不能进行浏览器设备登录」 | CI 或非交互环境 | 用 `sh -s -- --token <令牌>` 或设 `DSH_EE_TOKEN` |
| `setup.sh` 提示「没有找到 Node.js / npm」 | PATH 里没有 Node | 安装 Node，或把 DSH 自带 Node 加进 PATH |
| `setup.sh` 提示「✘ 令牌无效或连不上 dsh-ee」 | 地址错、网络不通、令牌过期 | 浏览器打开 `<地址>/healthz`；重新登录换令牌；检查 VPN/代理 |
| 设备码登录一直转圈 | 没人批准 / 确认页没打开 / 设备码过期（10 分钟） | 打开脚本打印的 `verification_uri_complete`，登录后批准；过期重跑 |
| 设备码轮询报 `slow_down` / `rate_limited` | 轮询太快 / 按 IP 限流 | 脚本会自动放慢；手动实现时遵守 `interval`，429 后退避 |
| 设备码报 `access_denied` / `expired_token` | 员工拒绝 / 超时 | 重新发起 |
| `~/.npmrc` 被旧脚本改过，影响本地开发 | 旧版接入脚本写全局 npmrc | 删掉含企业源的 `registry=…` 与 `_authToken` 行；新版只改 profile 的 `.npmrc` |
| DSH 里装完没有企业来源/模型 | profile 不对或 `.npmrc` 没写对 | 确认 `--profile`；重跑 `setup.sh`；`doctor` 的 `npmrc` 项（#122） |

## 3. 鉴权（登录、会话、令牌、额度）

### 3.1 网页登录

| 症状 | 原因 / 原文 | 处理 |
|---|---|---|
| 401「用户名或密码错误」 | 本地密码错；或 LDAP 目录拒绝 | 重试（5 次失败锁 15 分钟）；LDAP 账号用域密码 |
| 403「你的企业目录账号还没有开通 dsh-ee…」 | LDAP 关了自动开通，且没预建账号 | 管理员预建或开自动开通 |
| 502「企业目录暂时无法验证…」 | LDAP/OIDC 目录不可达或服务账号失效 | 检查目录连通与「登录与认证」配置；服务账号密码 |
| 429「失败次数过多，请 15 分钟后再试」 | 登录失败限流（进程内存，按账号；设备码按 IP） | 等 15 分钟；多实例部署时限流不一致，需共享存储（设计） |
| 401「请先登录」/「登录已过期，请重新登录」 | 无会话或会话过期（默认 168h） | 重新登录 |
| 登录后立刻掉线 | Cookie `Secure` 与访问协议不匹配（http 访问却开了 Secure；或 https 没开 Secure 被浏览器策略拦） | HTTPS 部署设 `DSH_EE_COOKIE_SECURE=true` 且只用 https 访问；检查 `DSH_EE_PUBLIC_URL` |
| 403「没有权限执行这个操作」 | 账号 RBAC 缺权限点 | 管理员在「角色与权限」里加；菜单可见不代表有权限 |
| 改密码 400「这个账号的密码由企业目录或统一登录管理」 | LDAP/OIDC/飞书账号 | 到公司账号系统改 |
| 改密码 400「原密码不正确」 | 输错 | 重试；忘记密码找管理员重置 |

### 3.2 令牌 / scope

令牌错误码（`principal.go`）与对应处理：

| code | 含义 | 处理 |
|---|---|---|
| `token_invalid` | 令牌不存在/哈希不匹配/前缀不认识 | 重新签发；确认没漏字符、没把 `ee_npm_` 当 `ee_at_` 的格式用 |
| `token_expired` | 过期 | 重新签发，或改用长期令牌（有上限） |
| `token_revoked` | 已吊销 | 管理员/本人在「我的令牌」「令牌管理」里看；重新签发 |
| 403 缺 scope | 令牌 scope 不覆盖入口 | 例：`/mcp` 要 `connections`；`/v1` 要 `models`；`/api/dsh/*` 要 `hub`；`config` 命令要 `config`；发布要 `registry:publish` |
| 403 权限点不足 | scope 有但账号 RBAC 没有 | scope 与 RBAC 求交，两者都要有；管理员加权限 |
| 管理模式不生效 | `admin` scope 只在令牌开管理模式期间有效 | 网页给该令牌开 1/8 小时管理模式（要再输密码）；Agent 不能代办 |
| 旧令牌权限看起来过大 | `ee_npm_` / `sk-ee_` 固定带 models+hub+connections | 历史行为；新集成用 `ee_at_` 收窄 scope |

`GET /api/me/doctor`（登录后的自检，基线已实现）会返回账号权限、额度、模型、DSH 接入开关、技能/连接目录等检查项；它**不是**发布链路的 `dsh-ee doctor`（#122）。

## 4. Harness（第三方 Harness / 自研客户端接 dsh-ee）

当前可用的 L1 面：一个 `ee_at_` 令牌 + `/v1`（OpenAI chat-completions 子集）+ `/mcp`（Streamable HTTP MCP）。细节见 [l1-client.md](./l1-client.md)。

### 4.1 `/v1` 模型网关

| HTTP / code | 含义 | 处理 |
|---|---|---|
| 401 `invalid_api_key` / `token_invalid` / `token_expired` / `token_revoked` | 令牌问题 | 见 §3.2 |
| 403 `permission_denied` | 账号没有 `llm.use` 或令牌没有 `models` scope | 加权限/换令牌 |
| 403 `quota_expired` | 额度已过期 | 管理员续期 |
| 403 `model_not_allowed` | 令牌/账号的模型范围不含该模型 | 换模型或管理员放开；`/v1/models` 看可用列表 |
| 429 `monthly_quota_exceeded` | 账号或令牌本月额度用完 | 下月（UTC 1 日）重置或管理员加额度；单次请求可能略微超额（先检查后调用） |
| 404 `model_not_found` | 没有可用的上游候选 | `/admin/providers`/`/admin/models` 检查模型是否启用、有上游 |
| 400 `model_required` / `invalid_json` / `request_too_large` | 请求体问题；上限 16 MiB | 按 OpenAI 格式传 `model`；压缩请求 |
| 502 `upstream_unreachable` | 所有上游线路失败 | 检查上游连通、key、`/admin/providers` 的「检查连接」 |
| 500 `provider_misconfigured` | 上游 key 解密失败（主密钥换过/数据损坏） | 重新录入上游 key（主密钥不能随便换） |
| DSH 里显示 `dshee_login_required` 等 | 接入插件收到 400 映射文案 | 按文案：重新 `/ee-login`、找管理员开权限/额度 |

注意：`/v1/chat/completions` 是 OpenAI chat-completions 子集；Claude Code / Anthropic SDK 走 **Anthropic Messages 兼容端点** `POST /v1/messages`（用同一个企业令牌，模型名填 `/v1/models` 的对外名，见 [dev/anthropic-api.md](./dev/anthropic-api.md)）。Codex（Responses）、`/v1/embeddings`、`/v1/responses` 仍没有。

### 4.2 `/mcp`

| 症状 | 原因 | 处理 |
|---|---|---|
| 401「企业账号登录已失效，在对话框输入 /ee-login 重新登录」 | 没带 Bearer / 令牌无效 | 重新登录；确认插件拿的是当前令牌 |
| 403「你的账号没有使用工作台的权限」 | 缺 `connections`/`hub` scope 或 `portal.view` | 见 §3.2 |
| JSON-RPC `-32700 不是合法的 JSON-RPC 请求` | 请求体不是 JSON-RPC | 按 MCP Streamable HTTP 发 `initialize`/`tools/list`/`tools/call`：`POST /mcp` 返回 JSON（不协商 `Accept`），`GET /mcp` 才是 SSE 事件流 |
| 写工具返回 `isError` 和确认链接 | 默认 `non_dsh`：非 DSH 客户端要先人工确认 | 打开确认链接，同一员工同参数再调一次；或管理员把写确认设为 `always`/`off`（`PUT /api/admin/write-confirm`） |
| 写完没生效 / 只生效一次 | 一次确认只执行一次，参数哈希变了要重新确认 | 保持参数一致；重新确认 |
| 工具列表不更新 | 目录变更通过 `GET /mcp` 的 SSE 推 `notifications/tools/list_changed` | 客户端要带 `Accept: text/event-stream` 订阅 GET `/mcp`；不支持就重连/重拉 `tools/list` |
| 扩展工具报 `unavailable` / 熔断 | 服务扩展连续出错被暂时停用 | 修扩展/网络后等自动重试；看服务端日志 |
| 浏览器里跨站调 `/v1`、`/mcp` 失败 | 基线**没有 CORS**；白名单见 PR #127（`DSH_EE_CORS_ORIGINS`，默认关闭），合并后生效 | 合并前从服务端/本机进程调用；合并后只对配置的 origin 放行，且不是认证边界 |
| 没有发现端点/版本头 | 基线没有 `/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps`、`Dsh-EE-API-Version`；见 PR #123，合并后生效 | 合入前以 `l1-client.md` 与 `/v1/models`、`/mcp` 为准；不要依赖未合入字段 |
| 没有速率限制/`Retry-After`/请求号 | 基线未实现（MCP 分页与写确认 `_meta` 见 PR #128，合并后生效；限流仍未实现） | 客户端自己做退避；多集成方场景见 [architecture.md](./architecture.md) §7 |
| `tools/list` 太大/想按页取 | 基线一次返回全部工具；cursor 分页见 PR #128（`nextCursor`、`limit=0` 取全量、DSH 客户端仍全量），合并后生效 | 合入后按 `nextCursor` 翻页；不需要分页可显式 `limit=0` |
| 写确认只有中文文本、拿不到链接/到期时间 | 基线文本确认；结构化 `_meta['dsh-ee/confirmation']`（id/url/expires_at）见 PR #128，合并后生效 | 合入后从结果级 `_meta` 取；确认文本/`isError` 与状态机不变 |

### 4.3 用 DSH 接入时

| 症状 | 处理 |
|---|---|
| DSH 里没有企业技能/连接 | `/api/me/doctor` 看 `catalog`/`connect` 项；确认管理员开了 DSH 接入、账号有权限；刷新目录（catalog 带 ETag，304 表示没变化） |
| 模型列表为空 | `/v1/models` 用同一令牌试；检查账号额度/模型范围与上游模型 |
| 装不上接入插件 | 见 §2；或从「接入指南」复制免登录 tgz 地址手动装 |
| 令牌换了但 DSH 还用旧令牌 | 接入插件每次现取凭据库里的 `DSHEE_TOKEN`；重新 `/ee-login` 或让它同步 npm 配置才会更新 |

## 5. 扩展（声明式 / 服务 / 端侧）

### 5.1 `dsh-ee extension check` 报错

退出码 1 表示有 error；`--json` 的 `results[]` 每条有 `code`/`severity`/`where`/`message`/`fix`。常见 code（完整列表：`dsh-ee schema get errors`）：

| code | 含义 | 处理 |
|---|---|---|
| `manifest.missing` | 目录里没有 `dsh-ee.extension.json` | 放到扩展根目录；声明式/服务扩展都要 |
| `manifest.invalid_json` / `manifest.type_mismatch` / `manifest.unknown_field` | 清单 JSON 错/字段类型错/拼错字段 | 修 JSON；未知字段是 warning（写了不生效），删掉或按 spec 改名 |
| `id.invalid` / `version.invalid` / `min_version.invalid` | id 不是「作者/名字」、版本不是 semver | 照 `dsh-ee schema get manifest` 改 |
| `kind.invalid` / `kind.field_mismatch` | kind 不是 declarative/service/local；写了别的形态的字段 | 选对 kind，删掉不适用的字段 |
| `openapi.missing` / `openapi.invalid` / `servers.missing` / `servers.invalid` / `servers.mismatch` | 声明式扩展的接口文档缺失/不可用；服务器地址缺失/不合法/与 OpenAPI 不一致 | 补 OpenAPI 3.x 文档；`servers` 用完整 http(s) 地址、不带账号密码/查询串，且覆盖接口文档里的 server |
| `service.url_invalid` | 服务扩展没写/写错服务地址 | 填部署好的服务地址；安装时也可覆盖 |
| `package.invalid` | 端侧扩展的 npm 包名不合法 | 改成合法 npm 包名 |
| `credential.mode_invalid` / `credential.header_invalid` / `credential.header_conflict` / `credential.shared_invalid` / `credential.claims_invalid` / `credential.field_unused` / `credential.hint_missing` | 凭据模式/请求头/申请的员工信息不对 | 按 [plugins.md](./plugins.md) §7 与 `schema get manifest` 改；共享凭据和员工身份不能放同一个头 |
| `oauth.config_missing` / `oauth.url_invalid` | 选 oauth 但没写授权/令牌地址，或地址不合法 | 补 URL；服务扩展实现了 oauth 约定接口时可不写 |
| `config.*`（key/type/label/secret_default/options/dynamic） | 管理员配置项写错 | 按 spec 改；`secret` 类型不能给默认值；声明式不支持动态选项 |
| `permissions.invalid` | 申请了不存在的反向权限 | 只能申请 `kb`/`approval`/`audit`/`notify`/`endpoint` 支持的能力 |
| `protocol.unsupported` | 清单协议版本 dsh-ee 不认识 | 升级 dsh-ee 或改用支持的协议版本 |
| `kind.not_checked`（info） | 服务/端侧扩展只查了清单 | 继续用 `extension test` 按协议测 |

### 5.2 `dsh-ee extension test` 失败

| 输出 | 原因 | 处理 |
|---|---|---|
| 「扩展启动后就退出了」/「等了 120 秒，扩展还没在 … 上提供清单」 | `--run` 命令起不来/没监听 `EXTENSION_ADDR`；或没提供 `/.well-known/dsh-ee-extension.json` | 看输出 tail；启动命令要能在目录下运行并监听 `EXTENSION_ADDR`；SDK 的 `ListenAndServe("")` 会读它 |
| 「这是一个服务扩展的目录：加 --run …」 | 目录里是服务扩展但没给启动命令 | `--run "go run ."` 等 |
| 声明式扩展只跑 check | 声明式没有代码 | check 通过即合格；装进 dsh-ee 后用「试连」看接口通不通 |
| 凭据相关失败 | 没给 `--credential`，或扩展期望 `user_secret` | 传 `--credential`；`--config` 传管理员配置 JSON |
| 写工具默认不真调 | 保护数据 | `--write` 才会真的调用写工具（会改数据） |
| 场景测试失败 | 目录下 `tests/*.yaml` 的期望不满足 | 按失败的 case 修扩展或场景文件 |

### 5.3 安装/升级扩展

| 症状 | 原因 | 处理 |
|---|---|---|
| 404/「取不到扩展的清单」 | 服务地址错、路径不对、服务没起 | 浏览器打开 `<地址>/.well-known/dsh-ee-extension.json` 确认 |
| 「端侧扩展是一个 npm 包，不在这里安装」 | 端侧扩展要发布到企业 npm 源，不能当 zip 装 | 打到企业源后按插件下发 |
| 「服务扩展要填它的服务地址来安装，不是上传文件」 | 上传了服务扩展的 zip | 在「安装扩展 → 服务地址」填地址 |
| 「已经装着的 X 是服务扩展，不能直接换成声明式扩展」 | 同一 id 换形态 | 先删旧连接再装，或换 id |
| 升级提示「这次升级有需要确认的变化」 | 服务地址变了/新申请反向权限/同版本内容变了/只读变写/凭据模式变了/新地址 | 看过变化后带 `confirm` 再提交（网页上勾确认）；别跳过 |
| 装完工具不可用 | 服务扩展不可达、凭据/配置没填、员工没授权 | 看连接详情；试连；员工在连接页面填 `user_secret` 或 OAuth 授权 |
| 想回滚到上一版 | **没有版本回滚/签名校验**（设计） | 保留旧 zip/代码，重新安装旧版本；升级会记录 `upgraded_from`，但不保留旧版本可一键回滚 |
| 服务扩展报 `authorize` / `setup` / `forbidden` / `unavailable` / `not_found` / `invalid` / `not_implemented` / `unauthenticated` | 扩展按协议返回的错误码 | 按 [extension-protocol.md](./extension-protocol.md) §6：去连接页授权、联系管理员、修权限、稍后重试等；`unauthenticated` 是 dsh-ee 自身验签问题，看日志 |
| `ee_jwt` 验签失败 | 扩展用的 JWKS/`aud` 不对；时钟偏差；dsh-ee 刚轮换密钥 | 扩展从 `<dsh-ee>/.well-known/…` 的 JWKS 取公钥；`aud` 用清单里的值（服务扩展固定 `m.ID`，声明式用 `credential.audience`）；重试会在新钥匙生效后恢复 |

## 6. 微应用嵌入（iframe / `/a/<slug>`）

> 基线（20aafbb）**没有** embed 模式、CSP `frame-ancestors`、`DSH_EE_EMBED_ORIGINS`、postMessage 或 `fit=auto`。这些由独立 PR #118 实现：**同站 v0 + 跨站 v1**（跨站默认关闭，需 `DSH_EE_EMBED_CROSS_SITE=1` 且配置 `DSH_EE_EMBED_ORIGINS`，会话 TTL 用 `DSH_EE_EMBED_SESSION_TTL`）；**尚未合入基线**，能力以 #118 合入为准。宿主接入见 #118 新增的 [embedding.md](./embedding.md)，跨站设计与未做项见 #130 新增的 [cross-site-embedding.md](./cross-site-embedding.md)（两份文档随 PR 合入后可用）。下面前两小节是基线行为，后两小节是 #118 合入后的行为。

### 6.1 基线：iframe 里打开 `/a/<slug>`

| 症状 | 原因 | 处理 |
|---|---|---|
| iframe 空白/跳到登录页 | `/a/:slug` 需要登录；跨站 iframe 不带 `SameSite=Lax` 的会话 cookie | 同站 iframe（同 origin 或同站子域）会带 cookie；跨站 iframe 走 v1（#118，默认关闭，见 §6.3/§6.4），不要往 iframe 里嵌登录页 |
| 登录页出现在 iframe 里，宿主看不到 | 未登录时 iframe 内跳 `/login` | 宿主先让用户在顶层登录，或同站嵌入；#118 合入后 iframe 不再跳登录页：同站发 `dsh-ee:auth-required`/`dsh-ee:session`，跨站由宿主 mint code 后发 `dsh-ee:connect`（见 §6.3） |
| iframe 高度不合适 | 基线没有自动高度/`fit=auto` | 宿主固定高度/自算；#118 同站与跨站都支持 `fit=auto` + `dsh-ee:resize`（`fit=auto` 的 sticky 头处理未做） |
| 移动端体验差 | #118 未做移动端专项策略（窄屏建议顶层打开，任何失败态都提供「在浏览器打开」） | 宿主按视口/UA 选择顶层打开，不要用手机 iframe |
| 嵌入方想控制主题 | 基线没有 `theme` 参数 | PR #118 的 `?embed=1&theme=light\|dark` 与 `dsh-ee:theme` 双向消息（同站/跨站同一协议），合并后生效 |
| 跨站嵌入被浏览器拦 | 基线没有跨站认证面；三方 cookie 限制使 `SameSite=Lax` 会话不可用 | 跨站 v1 已由 #118 实现（默认关闭，需 `DSH_EE_EMBED_CROSS_SITE=1` + `DSH_EE_EMBED_ORIGINS`）：宿主后端用每用户 `ee_at_`（`agent` scope）mint 一次性 code，iframe 换内存 `ee_emb_` Bearer，不使用 cookie；排查见 §6.4 |

### 6.2 基线：反向代理/安全头

| 症状 | 处理 |
|---|---|
| 宿主页面被浏览器报混合内容 | dsh-ee 只提供 HTTP，必须在 HTTPS 反代后；`DSH_EE_PUBLIC_URL=https://…` |
| iframe 里的请求 401 | 同 §3.1；跨站 v1 由 `ee_emb_` Bearer 认证，401 时 iframe 会发 `dsh-ee:auth-required` 让宿主重新 mint |
| 静态资源在 iframe 里 404 | 反代只转发了部分路径；把 `/` 全量转给 dsh-ee |

### 6.3 PR #118（合并后生效）：嵌入 v0/v1 的配置错误

合入后：

- `DSH_EE_EMBED_ORIGINS` 只接受完整 origin：`https://host[:port]`；`http` 只允许 `localhost`。非法值**启动失败**（fail closed），错误形如：

  ```text
  DSH_EE_EMBED_ORIGINS: 不接受 "*"：请写宿主页面的完整 origin，例如 https://harness.example.com
  DSH_EE_EMBED_ORIGINS: "https://a.example.com/path" 必须是完整的 origin（scheme://host[:port]），不能带通配符、路径、查询串或用户信息
  ```

  修法：写成 `https://host[:port]`，逗号分隔、不许空项、不许 `*`/`null`/带路径。
- `DSH_EE_EMBED_CROSS_SITE=1` 时**必须**同时配置 `DSH_EE_EMBED_ORIGINS`，否则启动失败；`DSH_EE_EMBED_SESSION_TTL` 默认 `2h`，允许 `1m`–`8h`，超范围同样启动失败。
- 只有 `/a/` 开头的页面会带上配置的 `frame-ancestors`；管理页等仍是 `'self'`。宿主 origin 没配对 → 浏览器控制台报 CSP 拒绝。
- `?embed=1` 隐藏返回/管理/DemoBar/导出等入口，保留标题与 tabs；`theme=light|dark` 只影响首屏与运行时主题、不写 localStorage。
- 仍然没有 XFO（不要配 `X-Frame-Options`，以 CSP 为唯一事实源，避免两处不一致）。
- postMessage 协议 v1（消息带 `v: 1`）：`dsh-ee:ready`（含 `supports:["connect-v1"]`）、`dsh-ee:session`、`dsh-ee:auth-required`、`dsh-ee:resize`（仅 `fit=auto`）、`dsh-ee:error`、`dsh-ee:theme` 双向；跨站另有宿主→iframe 的 `dsh-ee:connect{code}`。只发往 URL 参数 `host` 指定的 origin，宿主要校验 `event.origin` 与 `event.source`；接入步骤与宿主示例见 [embedding.md](./embedding.md)（合并后可用）。
- 跨站 v1 已由 #118 实现、**默认关闭**：宿主后端持有每用户 `ee_at_`（含 `agent` scope），调 `POST /api/embed/sessions` mint 60 秒一次性 code（绑定用户+应用+宿主 origin），iframe 用 `POST /api/auth/embed/exchange` 换只存内存的 `ee_emb_` Bearer（**不 Set-Cookie**），会话只放行白名单路由、按 limited 会话处理；mint/exchange 在开关关闭时返回 `embed_disabled`，来源不在白名单返回 `origin_not_allowed`。未做项：管理端「嵌入」设置页、完整扩展消息（`navigate`/`navigate-request`/`open-external`/`logout`、深链 tab 同步）、`fit=auto` sticky 头处理、移动端策略、多实例共享 code/会话存储；设计与边界见 [cross-site-embedding.md](./cross-site-embedding.md)（合并后可用）。

### 6.4 PR #118（合并后生效）：症状对照

| 症状 | 原因 | 处理 |
|---|---|---|
| 宿主 iframe 报 `frame-ancestors` 拒绝 | 宿主 origin 不在 `DSH_EE_EMBED_ORIGINS`，或写成了带路径/通配符的形式 | 按 §6.3 改；重启生效 |
| 启动失败报 `DSH_EE_EMBED_ORIGINS` | 配置非法（fail closed） | 按错误提示改成完整 https origin；本地联调用 `http://localhost:端口` |
| 嵌入模式下仍有导出/管理按钮 | 用的不是 `/a/` 页面，或没带 `?embed=1` | 确认路由与参数 |
| 主题不生效 | 参数值不是 `light`/`dark`；或页面不支持该参数 | 用 `?embed=1&theme=dark`；基线没有该参数（#118，合并后生效） |
| 配置了 origin 但管理页也能被框 | PR #118 合入后只有 `/a/` 放行，管理页保持 `self` | 若仍出现，确认部署的是合入后的版本 |
| 跨站 iframe 一直停在 `dsh-ee:auth-required` | 宿主没开跨站（`DSH_EE_EMBED_CROSS_SITE`）、origin 不在白名单、宿主后端没 mint 或没发 `dsh-ee:connect` | 按 §6.3 检查配置；看 mint 返回码（`embed_disabled`/`origin_not_allowed`）；`ready` 里应有 `supports:["connect-v1"]` |
| mint 接口 403/4xx | 用了共享服务令牌、令牌没有 `agent` scope、`host_origin` 不在白名单、app 未上线/未开放网页版 | 用当前用户的 `ee_at_` 调 `POST /api/embed/sessions`；错误码 `embed_disabled`、`origin_not_allowed`、`app_not_found`、`app_not_web`、`forbidden` |
| exchange 失败 | code 过期（60 秒）/已用/跨实例状态不一致；浏览器里出现 cookie 说明走错了路径 | 重新 mint；错误码 `embed_code_invalid`/`embed_code_expired`；exchange **不 Set-Cookie**、token 只存内存 |
| 嵌入会话 403 `embed_scope` | `ee_emb_` 是 limited 会话，只能访问白名单接口（`/api/auth/me`、`/api/settings`、`/api/me/menus`、`/api/me/apps/{slug}/*`、`/api/apps/{slug}/data…` 等） | 应用只用被嵌入应用自己的网页/数据接口；其它接口需要顶层登录 |
| 跨站仍被浏览器拦 | 宿主或 iframe 不是 HTTPS；反代另配了 `X-Frame-Options` 或第二份 CSP | HTTPS；不要在反代重复配 CSP/XFO（多份 CSP 按交集执行） |

## 7. 错误码速查

| 域 | 码 | 出处 |
|---|---|---|
| 令牌 | `token_invalid`、`token_expired`、`token_revoked` | `server/internal/api/principal.go`；`/v1`、`/mcp`、管理接口 |
| 模型网关 | `invalid_api_key`、`permission_denied`、`model_not_allowed`、`monthly_quota_exceeded`、`quota_expired`、`model_not_found`、`upstream_unreachable`、`provider_misconfigured`、`request_too_large`、`invalid_json`、`model_required`、`not_found` | `gateway.go`；DSH 插件映射：`dshee_login_required`、`dshee_forbidden`、`dshee_model_denied`、`dshee_no_allowance`、`dshee_monthly_limit_reached`、`dshee_allowance_expired` |
| 设备码 | `authorization_pending`、`slow_down`、`access_denied`、`expired_token`、`rate_limited` | `server/internal/api/device.go` |
| 扩展静态检查 | 见 §5.1；完整列表 `dsh-ee schema get errors` | `server/internal/extension/codes.go` |
| 扩展运行期 | `authorize`、`setup`、`forbidden`、`unavailable`、`not_found`、`invalid`、`not_implemented`、`unauthenticated` | [extension-protocol.md](./extension-protocol.md) §6 |
| CLI 退出码 | check/test/pack/report：0/1/2；`package check`：0/1/2/3；`doctor`：0/1/2 | [cli.md](./cli.md) §3 |

## 8. 独立 PR（尚未合入）与还没有实现

- 备份/恢复命令、日志轮转/级别、指标/追踪、审计导出（设计，见 [operations.md](./operations.md) §12）。
- 多实例部署的一致性、对象存储（设计）；每令牌限流已改为数据库共享计数（`shared_rate_limits`，多实例合计），登录限流仍在进程内存。
- 有独立 PR、尚未合入基线（能力以对应 PR 合入为准）：`/.well-known/dsh-ee`、`/v1/capabilities`、`/v1/apps`、`Dsh-EE-API-Version`（#123）；`dsh-ee package check`/`doctor`（#122）；微应用嵌入 v0/v1（#118；跨站默认关闭，需 `DSH_EE_EMBED_CROSS_SITE` + `DSH_EE_EMBED_ORIGINS`，接入见 [embedding.md](./embedding.md)、设计与未做项见 [cross-site-embedding.md](./cross-site-embedding.md)）；浏览器内 CORS 白名单 `DSH_EE_CORS_ORIGINS`（#127）；MCP `tools/list` 分页与写确认 `_meta`（#128）。
- #118 范围之外（未做）：管理端「嵌入」设置页、完整扩展消息（`navigate`/`navigate-request`/`open-external`/`logout`、深链 tab 同步）、`fit=auto` sticky 头处理、移动端策略、多实例共享 code/会话存储。
- 扩展签名、版本回滚、Git 安装、作者自助发布；`extension check` 不接受 URL。

遇到文档与行为不一致时，以代码和 `dsh-ee help` / `dsh-ee schema get errors` 的输出为准，并提 issue/PR 修文档。
