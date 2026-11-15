# 跨站嵌入 v1：实现参考与设计说明

> **状态：已实现（独立 PR #118，默认关闭）。** 同站 v0 与跨站 v1 由 #118 一起实现：CSP/安全头、`?embed=1` 模式、最小 postMessage 协议、一次性 code + 内存 Bearer 的跨站握手。跨站部分需要 `DSH_EE_EMBED_CROSS_SITE=1` 且配置 `DSH_EE_EMBED_ORIGINS` 才生效；**合入 main 前仍以 #118 分支为准**，合入后即为已发布能力（默认关闭）。
>
> 阅读分工：**怎么接**（宿主写法、消息示例、验收清单）以 [embedding.md](./embedding.md) §7 为准；**本文**是设计说明与实现参考：为什么这么设计、安全边界、设计决策与实现的对照、还没做的项。两份不一致时以实现（`server/internal/api/embed.go`、`web/src/lib/embed-provider.tsx`）和 `embedding.md` 为准，并回来改本文。错误面索引见 [errors.md](./errors.md#7-嵌入与-iframe)。
>
> 本文不包含绕过鉴权/安全策略的操作步骤。

## 1. 问题与三种信任模型

「在宿主页面里嵌 dsh-ee 微应用」按浏览器对**站点（scheme + 可注册域）**的划分是三种：

| 模型 | 典型宿主 | cookie 上下文 | 当前状态（#118 合入后） | 处置 |
|---|---|---|---|---|
| T1 顶层文档 | 飞书工作台、`window.open` 打开 `/a/<slug>`、现有 handoff | 首方，`SameSite=Lax` 正常 | ✅ 已有 | 保留，作为嵌入失败时的兜底 |
| T2 跨站 iframe | 另一个域名的企业 Harness、门户、自研工作台 | 三方，Lax cookie 默认不发送 | ✅ v1 已实现，默认关闭 | 本文；接入见 `embedding.md` §7 |
| T3 同站 iframe | `portal.example.com` 嵌 `ee.example.com` | 同站，Lax 正常 | ✅ v0 已实现 | 接入见 `embedding.md` §1–§6 |

同站按浏览器定义：scheme + 可注册域，**端口和子域不影响**；`https://portal.example.com` 与 `https://ee.example.com` 同站；`https://harness.example.net` 与 `https://ee.example.com` 不同站。

跨站的难点只有一个：**三方上下文不发送 `SameSite=Lax` 的网页会话 cookie**。v1 因此不复用网页会话，而是单独开一条 cookie-free 的认证面（一次性 code + 内存 Bearer，见 §5）。

**明确不做**（这些路都被否过）：

- 不改 `SameSite=None`（三方 cookie 正在被浏览器限制，且削弱现有 CSRF 防线）；
- 不为了嵌入放开 CORS（浏览器内第三方 Harness 直连 `/v1`、`/mcp` 是另一个问题，由 CORS 白名单处理）；
- 不做匿名公开链接、不把 dsh-ee 反代到宿主域名下；
- 不把 `ee_at_` 或网页会话 cookie 交给宿主页面脚本。

**适用对象**：企业内**可信**的宿主系统（企业门户、内部 Harness），宿主后端保管每用户令牌。面向公开互联网站点、匿名嵌入不在范围内。

## 2. 实现状态一览

| 能力 | 状态 | 实现位置 |
|---|---|---|
| 同站 iframe：`?embed=1&host=&fit=&theme=`、隐藏返回/管理/演示条/导出/`dsh://`、主题首屏生效 | ✅ 已实现 | `web/src/lib/embed.ts`、`web/src/pages/app-web.tsx`、`web/index.html`、`web/src/components/{app-data,live-pane}.tsx` |
| CSP：`/a/*` 加 `frame-ancestors 'self' <DSH_EE_EMBED_ORIGINS>`，其它 HTML 保持 `'self'`；不设 XFO | ✅ 已实现 | `server/internal/api/security_headers.go` |
| 来源解析/校验（完整 origin、https、回环 http、拒绝 `*`/`null`/路径，fail closed） | ✅ 已实现 | `server/internal/embed/origin.go`、`server/internal/config/config.go` |
| v0 消息：`ready` / `theme` 双向 / `resize`（仅 `fit=auto`）/ `error` | ✅ 已实现 | `web/src/lib/use-embed.ts`、`web/src/lib/embed-provider.tsx` |
| v1 握手：`ready{supports:["connect-v1"]}` → `connect{code}` → `session` / `auth-required` | ✅ 已实现 | `web/src/lib/embed-provider.tsx` |
| mint 一次性 code：`POST /api/embed/sessions`，60 秒、一次性、绑定用户+应用+宿主 origin | ✅ 已实现 | `server/internal/api/embed.go` |
| 内存 Bearer：`POST /api/auth/embed/exchange` → `ee_emb_…`，不 Set-Cookie、只存 SPA 内存 | ✅ 已实现 | `embed.go`、`web/src/lib/api.ts` |
| 嵌入会话存储：`embed_sessions` 表（迁移 `0032`），limited 权限，过期/吊销清理 | ✅ 已实现 | `server/internal/store/embed_sessions.go`、`server/internal/store/migrations/0032_embed_sessions.sql`、`server/cmd/dsh-ee/main.go`（每小时清理） |
| 路由白名单 + 应用绑定：白名单外、或路径应用标识 ≠ mint 绑定应用 403 `embed_scope` | ✅ 已实现 | `embed.go` 的 `embedRouteAllowed` / `embedAppScopeSlug`、`embed_whitelist_test.go`、`embed_test.go` |
| 注销：`POST /api/auth/embed/logout`（204） | ✅ 已实现（接口） | `embed.go` |
| 审计：`embed.session_create` / `auth.login(via=embed)` / `embed.revoke` / `embed.deny` | ✅ 已实现 | `server/internal/api/audit.go`、`web/src/lib/audit.ts`、i18n 文案 |
| 改密/禁用/解绑身份同时失效嵌入会话 | ✅ 已实现 | `server/internal/store/users.go`（`DeleteUserSessions`） |
| 限流：mint 30 次/分钟/IP、exchange 60 次/分钟/IP（进程内） | ✅ 已实现 | `embed.go` 的 `embedMintRate` / `embedExchangeRate` |
| 测试：Go 单测 + e2e 同站/跨站 | ✅ 已实现 | `server/internal/api/embed_test.go`、`embed_whitelist_test.go`、`security_headers_test.go`、`server/internal/embed/origin_test.go`、`server/internal/config/config_test.go`、`e2e/tests/{34-embed,35-embed-v1}.mjs` |
| 管理端设置页（开关/origin/TTL 在线编辑） | ❌ 未做 | 目前只有环境变量 |
| 扩展消息：`navigate` / `navigate-request` / `open-external` / `logout` 的宿主↔iframe 协议 | ❌ 未做 | 目前只有服务器注销接口，没有对应的 postMessage 协议 |
| `fit=auto` 时的 sticky 头处理 | ❌ 未做 | `fit=auto` 只上报高度；页面头部仍 `sticky` |
| 移动端自动策略（<640px/移动 UA 自动顶层打开） | ❌ 未做 | 目前由宿主自己判断（`embedding.md` 给建议） |
| 多实例共享一次性 code | ❌ 未做 | code 存在进程内存（单实例） |

## 3. 设计决策与实现的对照

这张表是设计说明的核心：每条决策当初为什么这么选，以及 #118 里怎么落地、和设计有没有差异。

| # | 决策 | 实现方式 | 与设计/边界的差异 |
|---|---|---|---|
| D1 | 一次性 code 由**宿主后端**申请，浏览器不接触 `ee_at_` | mint 走 `POST /api/embed/sessions`，用每用户 `ee_at_`；宿主页面只拿 code | 与设计一致 |
| D2 | exchange 换**内存 Bearer**，不 Set-Cookie | `POST /api/auth/embed/exchange` 返回 `ee_emb_`，`Cache-Control: no-store`；测试断言无 Set-Cookie | 与设计一致 |
| D3 | embed 会话 = limited 会话（员工权限、无 `keys.self`、无管理） | `withEmbedUser` 复用 `limitedAccess`；`/api/auth/me` 返回 `limited` + `embed{app,host}` | 与设计一致 |
| D4 | code 60 秒、一次性、绑定用户+应用+宿主 origin | 只存哈希在内存 map，取用即删；过期与无效区分 `embed_code_expired` / `embed_code_invalid`；`next` 校验为 `/a/<slug>` 前缀 | `next` 已被校验并绑定，但 exchange/前端当前**未消费**（保留字段） |
| D5 | 会话短 TTL、不提供 refresh、可吊销 | `DSH_EE_EMBED_SESSION_TTL` 默认 `2h`，范围 `1m–8h`；过期后 401 → `auth-required{reason:"expired"}`；logout 删会话 | 与设计一致；TTL 放在环境变量而非设置页 |
| D6 | 只能访问**路由白名单**，且只能访问 mint 时绑定的应用 | `embedRouteAllowed` 固定清单；路径里的 `{slug}` 必须等于会话的 `app_slug`，不相等 403 `embed_scope` 并记 `embed.deny`；两类测试固定清单与边界 | 与设计一致；白名单只校验形状，应用绑定由 `requireEmbedUser` 单独校验 |
| D7 | CSP `frame-ancestors` 只对 `/a/*` 放行，默认 `'self'` | `security_headers.go` 按路径加头；管理页/登录页保持 `'self'` | 与设计一致；XFO 不设 |
| D8 | 每用户令牌是硬前提，不提供共享令牌 | mint 要求 human 账号 + `agent` scope；机器人令牌 403 并记 `embed.deny` | 与设计一致；**实际还要求 `hub` scope**（mint 走 `dshPrincipal`），令牌需同时带 `hub` + `agent` |
| D9 | 默认关闭，按配置启用 | `DSH_EE_EMBED_CROSS_SITE` 默认 false；开启但未配 `DSH_EE_EMBED_ORIGINS` 时**启动失败** | 与设计一致；只有环境变量，没有管理端设置页（见 §11） |
| D10 | 一次性 code 存内存（单实例） | `embedCodes` 进程内 map；多实例需共享存储 | 与设计一致，见 §11 |

## 4. 信任边界

- **宿主后端**：可信，保管每用户 `ee_at_`，代表用户申请 code；它必须是 HTTPS，且不能把令牌下发给浏览器。
- **宿主前端**：不完全可信（XSS、第三方脚本），因此只能拿到短时 `code`/`ee_emb_`，且 embed 会话权限被 limited + 路由白名单收窄。
- **iframe 内容**：dsh-ee 自己的 `/a/*` 页面，与宿主跨站；消息只走 `postMessage`，双向校验 origin/source。
- **dsh-ee 服务端**：不信任宿主传来的任何身份声明，只认 `ee_at_`（mint）与 `ee_emb_`（会话）两种凭据，权限按账号 RBAC ∩ 令牌 scope。

## 5. 服务端接口（已实现）

新增接口都不改现有 `/api/dsh/*`、`/mcp`、`/v1`、`/a/*` 的行为；`DSH_EE_EMBED_CROSS_SITE` 关闭时 mint/exchange 返回 404 `embed_disabled`，不参与现有链路。

| 方法/路径 | 认证 | 请求 | 成功 | 失败 code |
|---|---|---|---|---|
| `POST /api/embed/sessions` | `Authorization: Bearer ee_at_`；human 账号；需 `hub` + `agent` scope | `{app, host_origin, next?}` | `{code, expires_in:60, app:{slug,title}, user:{username,display_name}, session_ttl}`；`Cache-Control: no-store` | `embed_disabled`(404) / `rate_limited`(429) / `forbidden`(403，含机器人/缺 agent) / `app_required`(400) / `origin_not_allowed`(403) / `app_not_found`(404) / `app_not_web` / `next_invalid`(400)；令牌/权限错误见 [errors.md](./errors.md#3-鉴权会话与令牌) |
| `POST /api/auth/embed/exchange` | code 即凭证；必须 `Content-Type: application/json`（沿用 CSRF 防线） | `{code}` | `{token:"ee_emb_…", token_type:"Bearer", expires_in, app, user, limited:true}`；**不 Set-Cookie** | `embed_code_invalid` / `embed_code_expired` / `embed_disabled` / `rate_limited`（均为 4xx/429 JSON） |
| `POST /api/auth/embed/logout` | `ee_emb_` Bearer | `{}` | `204`（删会话 + `embed.revoke` 审计） | 400 `not_embed_session`；401 会话无效/过期 |
| `GET /api/auth/me` | 两种 | — | 增加 `limited:true` 与 `embed:{app,host}` | — |

补充（实现细节，接入时按 `embedding.md` §7 为准）：

- `next` 只接受 `/a/<应用标识>` 前缀的站内路径（不得出现 `..`/`.` 段），否则 400 `next_invalid`；当前不消费，默认绑定 `/a/<slug>`。
- mint/exchange 的失败是**宿主后端/iframe fetch 看到的 HTTP JSON**（`{"error","code"}`）；iframe 再按情况转成 `dsh-ee:error` 通知宿主（见 §7）。
- mint/exchange 的业务失败响应是 `{"error","code"}`（请求格式错误等通用 4xx 只有 `{"error"}`）；mint/exchange 成功响应带 `Cache-Control: no-store`。

## 6. 会话与路由白名单（已实现）

| 项 | 实现 |
|---|---|
| 存储 | `embed_sessions` 表：`token_hash` 主键、`user_id`、`app_slug`、`host_origin`、`expires_at`、`created_at`、`revoked_at`；token 只存 sha256，明文只在 exchange 返回一次 |
| 权限 | limited：员工权限、无 `keys.self`、无管理权限；`/api/auth/me` 与 `sessionLimited` 都把它当 limited |
| 失效 | 改密码、禁用账号、解绑身份 → `DeleteUserSessions` 同时删网页会话与嵌入会话；后台每小时清理过期嵌入会话 |
| 读取顺序 | `requireUser`：先看会话 cookie（同站/顶层不受影响），没有 cookie 时只认 `Authorization: Bearer ee_emb_…` |
| 审计 | mint → `embed.session_create`（app、host_origin）；exchange 成功 → `auth.login`（`via=embed`、app、host_origin、limited）；logout → `embed.revoke`；拒绝 → `embed.deny`（reason；不记 code/令牌明文） |
| 白名单 | `GET /api/auth/me`、`GET /api/auth/methods`、`GET /api/settings`、`GET /api/me/menus`、`GET /api/me/apps/{slug}/web`、`POST /api/me/apps/{slug}/source`、`/api/apps/{slug}/data…`（表清单/行读写/导出 CSV）、`POST /api/auth/embed/logout`；其余 403 `{"error":"嵌入会话不能访问这个接口","code":"embed_scope"}`。带 `{slug}` 的接口还要求 `slug` 等于 mint 时绑定的应用，否则 403 `{"error":"这个嵌入会话只能访问被嵌入的应用","code":"embed_scope"}` |

白名单是**额外**约束，不是替代 RBAC/scope：应用可用范围、部门/角色权限继续按现有逻辑校验；新增页面接口时要显式评估并加进白名单（有测试固定清单）。

## 7. postMessage 协议（已实现的消息集）

在 `embedding.md` §4 有完整的宿主侧写法；这里只列实现边界与设计原因。

| 方向 | type | 载荷 | 实现说明 |
|---|---|---|---|
| iframe → 宿主 | `dsh-ee:ready` | `{ app:{slug}, theme, supports:["connect-v1"] }` | 嵌入页一挂载就发（早于认证）；同站宿主可只用 theme/resize |
| 宿主 → iframe | `dsh-ee:connect` | `{ code }` | 仅跨站；iframe 校验 origin/source 后 exchange |
| iframe → 宿主 | `dsh-ee:session` | `{ user:{display_name}, limited, expires_in? }` | 认证成功后发一次；不含 token |
| iframe → 宿主 | `dsh-ee:auth-required` | `{ reason:"missing"\|"expired" }` | 无会话或会话过期；宿主 mint 后重发 connect |
| iframe → 宿主 | `dsh-ee:error` | `{ code, message, retryable }` | 同一条错误只发一次 |
| 宿主 ↔ iframe | `dsh-ee:theme` | `{ theme }` | 双向；只改 DOM，不写 localStorage |
| iframe → 宿主 | `dsh-ee:resize` | `{ height, width }` | 仅 `fit=auto`，上限 4000px |

实现约束：

- 消息只发给 URL 参数 `host` 的 origin，**绝不 `"*"`**；`host` 缺失/非法时不发消息。
- 宿主 → iframe 的消息必须由 `host` origin 的父窗口发出；iframe 校验 `event.origin` 与 `event.source`，不匹配丢弃。
- 跨站握手超时 **10 秒**；连续 connect 最多 **2 次**（第一次失败会发 `auth-required` 让宿主重新 mint 一次，再失败转失败态）。
- `dsh-ee:error` 的 code 来源分三类：
  - 页面加载/接口错误映射：`auth_required`（401）、`forbidden`（403/404）、`load_failed`（其它）；
  - exchange 失败：`embed_code_invalid`、`embed_code_expired`、`embed_disabled`、`rate_limited`、`network`；
  - 握手超时/无 host：`auth_required`。
- **未实现**的消息：`navigate` / `navigate-request` / `open-external` / `logout` / `logged-out`（见 §11）。iframe 内没有站内导航和「从宿主触发登出」的协议；服务器注销接口是给 iframe 自己/宿主后端调用的。

## 8. CSP 与安全头（已实现）

```
# 只对 /a/* 的 HTML 生效
Content-Security-Policy: frame-ancestors 'self' https://host1.example.com https://host2.example.com:8443
Referrer-Policy: strict-origin-when-cross-origin
X-Content-Type-Options: nosniff
Permissions-Policy: camera=(), microphone=(), geolocation=()
```

- 来源来自 `DSH_EE_EMBED_ORIGINS`（逗号分隔完整 origin）；**CSP 白名单与 mint 校验共用同一份解析**（`server/internal/embed/origin.go`），避免两处口径不一致。
- `DSH_EE_EMBED_ORIGINS` 只接受 `https://host[:port]`，`http` 仅回环地址（`localhost`/`127.0.0.1`/`[::1]`）；拒绝 `*`、`null`、通配符、路径/查询/用户信息，非法值**启动失败**。
- `DSH_EE_EMBED_CROSS_SITE` **只控制 mint/exchange 是否可用**；CSP 的来源追加由 `DSH_EE_EMBED_ORIGINS` 决定。只配 origins 不配 cross-site 时，同站/顶层嵌入的 CSP 生效，但跨站 mint 返回 `embed_disabled`。
- 默认 `frame-ancestors 'self'`；管理页、登录页、`/go` 等永远只允许 `'self'`。
- **不设 `X-Frame-Options`**；反代/CDN 不要重复添加 CSP 或 XFO；HTTPS 是硬前提。
- 可选的完整内容安全策略（`DSH_EE_CSP=report|enforce`，默认 `off`）：在同一中间件里给 HTML 加 `default-src` / `script-src`（含 index.html 内联脚本的 sha256 哈希）等指令，`frame-ancestors` 与本节来源合并；`report` 模式的完整策略走 `Content-Security-Policy-Report-Only`，但 `frame-ancestors` 收口仍放在会执行的 CSP 里。见 [operations.md](./operations.md) §10.1。

## 9. 失败态与兜底（已实现）

| 步骤/现象 | code | 谁看到 | 兜底 |
|---|---|---|---|
| 未开启跨站 / 无来源 | `embed_disabled` / `origin_not_allowed` | 宿主后端（mint HTTP）；`origin_not_allowed` 也可能被 CSP 拦在 iframe 之前 | 检查两个环境变量；宿主按建议 8 秒超时兜底（iframe 自身等待握手 10 秒） |
| 应用不存在 / 未开放网页版 / 无权限 | `app_not_found` / `app_not_web` / `forbidden` | 宿主后端（mint）或 iframe（403/404 → `forbidden`） | 页内说明；顶层打开或引导申请权限 |
| code 过期/已用、会话过期/被吊销 | `embed_code_expired` / `embed_code_invalid` / `auth_required{expired}` | iframe（exchange/401） | 宿主重新 mint 一次；仍失败则顶层打开 |
| 网络不可达、5xx | `load_failed` / `network` | iframe | 页内重试；宿主超时兜底 |
| 白名单外接口、或路径应用 ≠ 会话应用 | 403 `embed_scope` | iframe fetch | 属前端/宿主问题，不应出现；宿主必须为它实际嵌入的应用 mint |
| 移动端窄屏 | — | 宿主 | **没有自动策略**：宿主自行顶层打开（`embedding.md` 给建议，见 §11） |

原则：嵌入模式下 iframe **不跳登录页**；任何失败都提供「在浏览器打开」（去掉嵌入参数、顶层打开）；验证以 `embedding.md` §6 的清单为准。

## 10. 安全边界与风险（实现后）

已落地的边界：

- 新增 `ee_emb_` 认证面只是 limited 会话的另一种载体：无管理权限、无 `keys.self`、路由白名单、短 TTL、可吊销；
- 宿主页面即使被 XSS/第三方脚本注入，拿到的也只是短时 token，不能发插件/改配置/管用户；`ee_at_` 永远只在宿主后端；
- 跨站 iframe 不使用 cookie，现有网页会话的 CSRF 防线（`SameSite=Lax` + JSON Content-Type）不变；
- 管理页不可被配置的宿主框住（CSP 按 `/a/*` 收口）；
- code 一次性 + 60 秒 + 绑定用户/应用/origin；mint/exchange 有限流；拒绝动作记 `embed.deny`。

剩余风险与缓解：

| # | 风险 | 现状/缓解 |
|---|---|---|
| R1 | 宿主页面被注入后窃取内存 `ee_emb_` | 短 TTL、limited 权限、路由白名单、可吊销；`ee_at_` 不进浏览器 |
| R2 | CSP 配置错误（`*`、http、路径）造成点击劫持 | 解析层 fail closed + 单测；默认 `'self'`；验收 `curl -I` |
| R3 | 反代重复 CSP / 残留 XFO | 文档要求代理不写；`embedding.md` §2 有反代注意 |
| R4 | iframe 加载失败宿主不可观测 | 8 秒超时 + 常驻「在浏览器打开」 |
| R5 | 浏览器三方存储差异 | 不依赖三方 cookie，只用内存 + postMessage |
| R6 | 宿主把 embed 会话当普通令牌复用 / 跨应用访问 | 路由白名单 + 应用绑定：白名单外或应用标识不符均 403 `embed_scope`；宿主侧说明只嵌应用 |
| R7 | 多实例下内存 code 不共享 | 单实例可用；多实例需共享存储（§11） |
| R8 | 误把「嵌入可用」当成「安全审计通过」 | 文档与 UI 明确边界；签名/供应链是另一条线（见 [plugin-signing.md](./plugin-signing.md)） |

## 11. 未做项与后续

| 未做项 | 现状 | 后续建议 |
|---|---|---|
| **管理端设置页** | 开关/来源/TTL 只有环境变量，改配置要重启 | 在「系统设置」里加 embedding 设置（origin 列表、应用白名单、TTL），走 `settings.manage` 权限与审计；保留环境变量作部署期兜底 |
| **扩展消息**（`navigate` / `navigate-request` / `open-external` / `logout` 等） | 服务器注销接口可用；没有宿主↔iframe 的这些消息协议。`next` 已校验但未被消费，iframe 内没有站内导航/宿主触发登出的协议 | 先定协议版本（`v:1` 增加 type）与版本协商，再实现 iframe 侧；`logout` 与现有 `POST /api/auth/embed/logout` 对齐；mint 的 `next` 已校验但未消费，导航协议一起定 |
| **`fit=auto` 的 sticky 头** | `fit=auto` 只上报高度；页面头部仍 `sticky`，在按内容高度撑开的嵌入里体验不佳 | 嵌入 + `fit=auto` 时把头部改成 static（与 `embedding.md` 的尺寸说明一起改） |
| **移动端自动策略** | dsh-ee 不做特殊布局/认证；由宿主判断（`<640px` 或移动 UA 顶层打开） | 可在 `ready` 里带建议或提供 `mobile` 提示；是否自动顶层打开需产品确认 |
| **多实例共享 code 存储** | code 在进程内存（`embedCodes`），多实例部署时 code 可能落在另一实例 | 把 code 与会话放共享存储（DB/Redis）；限流也要从进程内改为共享计数，否则多实例下限流失效 |

## 12. 验证（已实现部分）

代码与测试在 #118 分支；合入后按本节复核：

- **Go 单测**：`server/internal/api/embed_test.go`（`TestEmbedMintAndExchange`、`TestEmbedCodeExpiry`、`TestEmbedSessionExpiry`、`TestEmbedSessionInvalidatedByPasswordChange`、`TestEmbedMintRequiresHumanUser`）、`embed_whitelist_test.go`（`TestEmbedRouteWhitelist`、`TestEmbedNextMatches`、`TestEmbedCodesOneTimeAndExpired`、`TestEmbedCrossSiteEnabled`）、`security_headers_test.go`（默认/追加来源/仅 `/a/*`/非 HTML 不加/3xx 跳过）、`server/internal/embed/origin_test.go`、`server/internal/config/config_test.go`（fail closed）；
- **e2e**：`e2e/tests/34-embed.mjs`（同站 v0：隐藏入口、`ready`/`resize`/`theme`/`error`、不跳登录）、`e2e/tests/35-embed-v1.mjs`（跨站：宿主页在 `http://localhost:8099`，`ready(connect-v1)` → `auth-required` → 宿主 mint → `connect` → `session(limited)`；exchange 无 Set-Cookie；浏览器无 cookie 也能取数；code 一次性；失败码）；
- **CI**：`e2e` job 通过 `DSH_EE_EMBED_CROSS_SITE=1`、`DSH_EE_EMBED_ORIGINS=http://localhost:8099` 启用 35 号用例。

## 13. 维护者待决

1. 管理端设置页的优先级与权限（`settings.manage` 是否够；origin 列表是否需要审批/双人确认）。
2. 扩展消息的范围：`navigate`/`open-external` 只允许本应用站内路径，`logout` 只撤销 embed 会话——协议先定后写。
3. `fit=auto` 的 sticky 头与移动端策略一起改，还是先维持宿主自理。
4. 多实例：code/会话/限流全部放共享存储，还是把跨站嵌入限制为单实例部署（写进运维文档）。
5. 宿主侧是否需要「来自第三方」标识与撤销入口（产品/合规口径）。
