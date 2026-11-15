# 在宿主页面里嵌入微应用（同站 iframe v0）

面向企业内部的宿主页面（门户、工作台、自研 Harness）用 iframe 嵌入 dsh-ee 的微应用网页版 `/a/<标识>`。

| 形态 | 现状 | 说明 |
|---|---|---|
| 同站 iframe | ✅ v0 已实现 | 同一站点（scheme + 可注册域）的宿主，例如 `harness.acme.internal` 嵌 `ee.acme.internal`；沿用现有 `SameSite=Lax` 会话，零认证改动 |
| 顶层打开 / 飞书工作台 | ✅ 已有 | 不是嵌入，作为失败兜底 |
| 跨站 iframe（不同站点） | ✅ v1 已实现（默认关闭） | 一次性 code + 内存 Bearer + postMessage 握手，见文末「跨站嵌入 v1」；需要 `DSH_EE_EMBED_CROSS_SITE=1` 且宿主后端保管每用户 `ee_at_` 令牌 |

> 同站按浏览器定义：scheme + 可注册域，**端口和子域不影响**。`http://localhost:3000` 与 `http://localhost:8080` 同站；`https://harness.acme.internal` 与 `https://ee.acme.internal` 同站；`https://harness.example.com` 与 `https://ee.example.com` **不同站**。

## 1. 前提

1. **HTTPS**：HTTPS 的宿主页面不能框 HTTP 的 dsh-ee（混合内容会被浏览器拦）。建议 dsh-ee 部署在 HTTPS 后面并设置 `DSH_EE_COOKIE_SECURE=true`。
2. **同站**：跨站 iframe 里 `SameSite=Lax` 的会话 cookie 不会被发送，页面会返回 `auth_required`。跨站场景不要用 v0，等 v1。
3. **登录状态**：同站 iframe 直接用 dsh-ee 的登录状态（用户先访问过 dsh-ee 或在顶层登录过）；跨站 iframe 不用 cookie，由宿主后端 mint 一次性 code（见文末 v1）。会话过期时 iframe 不会跳登录页，而是把 `dsh-ee:auth-required` / `dsh-ee:error` 发给宿主。

## 2. 服务端配置

`DSH_EE_EMBED_ORIGINS`：允许嵌 `iframe` 的宿主来源，逗号分隔的**完整 origin**。

```bash
# 例：同站宿主 harness.acme.internal，外加本地联调页面
DSH_EE_EMBED_ORIGINS='https://harness.acme.internal,http://localhost:5173'
```

规则（非法值会让服务启动失败，fail closed）：

- 只接受 `https://host[:port]`；`http` 只允许回环地址（`localhost` / `127.0.0.1` / `[::1]`，本地联调）。
- 必须是完整 origin：不能带路径、查询串、片段、用户名密码；不接受 `*`、`null`、`*.example.com` 这类通配符。
- 自动去空白、大小写归一化、去重；末尾一个 `/` 等价于不带路径。
- 不设置该环境变量时只允许同站嵌入（`frame-ancestors 'self'`）。
- 跨站嵌入 v1 还需要 `DSH_EE_EMBED_CROSS_SITE=1`（默认关闭，且必须同时配置来源，否则启动失败）；会话 TTL 用 `DSH_EE_EMBED_SESSION_TTL`（默认 `2h`，1m–8h）。详见文末「跨站嵌入 v1」。
- 限流额度可配：`DSH_EE_EMBED_MINT_PER_MINUTE`（默认 30）、`DSH_EE_EMBED_EXCHANGE_PER_MINUTE`（默认 60），都是**每 IP 每分钟**、按数据库共享计数——多实例部署时是合计额度，不是每实例一份。非法值（非正整数）启动失败。
- **多实例部署**：一次性 code、嵌入会话、限流计数都在数据库里，多个实例（不同进程/端口）共享同一份状态，不需要 Redis 或负载均衡粘性。详见文末「多实例部署」。

**管理端覆盖（settings.embed）**：管理员可以在「系统设置 → 跨站嵌入」（`/admin/settings#settings-embed`）里追加来源、开关跨站、设置应用白名单与会话时长。有效配置 = 环境变量（只读基线）∪ 数据库设置：

| 项 | 合并规则 |
|---|---|
| `crossSite` | 数据库显式 true/false 覆盖环境变量；未设置时跟随 `DSH_EE_EMBED_CROSS_SITE` |
| `allowedOrigins` | 数据库来源**追加**在 `DSH_EE_EMBED_ORIGINS` 之后（环境变量只读，不能在网页上删改） |
| `apps` | 允许嵌入的应用 slug 列表；空 = 所有开放网页版的应用 |
| `sessionMinutes` | 非 0 时覆盖 `DSH_EE_EMBED_SESSION_TTL`（0 = 跟随部署配置） |

非法值在保存时拒绝；库里被手工改坏的设置会让服务**启动失败**（fail closed）。CSP 与 mint/exchange 读同一份有效配置，保存后立即生效（设置缓存 5 秒，保存时会主动失效）。

**管理端运行概览（E5）**：同一分区下方展示跨站嵌入的运行数据（活跃会话 / 今日创建 / 今日过期 / 今日吊销 / 24h 拒绝 + 活跃应用 top 10），带手动刷新；数据来自 `GET /api/admin/embed/stats`（需要 `settings.manage`）：

| 字段 | 含义 |
|---|---|
| `active_sessions` | 当前有效（未过期、未吊销）的嵌入会话数 |
| `created_today` / `expired_today` / `revoked_today` | 本地时区「今天」新建 / 到期 / 被吊销的会话数 |
| `denied_24h` | 最近 24 小时 `embed.deny` 审计的条数（来源/应用不匹配、嵌入关闭、code 无效等拒绝） |
| `top_apps` | 活跃会话最多的应用，最多 10 个，只有应用 slug 与数量 |

- 只返回聚合数量：**不含用户、IP、令牌或宿主 origin**；`tz` 是本地时区相对 UTC 的分钟数（界面按浏览器时区请求），一次统计的总查询预算 2 秒，响应 `Cache-Control: no-store`。
- 过期/被吊销的会话行会保留 7 天再物理清理（每小时清理任务），所以「今天过期/吊销」不会在当天就被清理掉；活跃会话的判定不受保留期影响。
- 监控抓取（`DSH_EE_METRICS=true` 且本机 / 内网来源）另外输出两个低基数指标：`dsh_ee_embed_sessions_active`（gauge）与 `dsh_ee_embed_denied_total`（counter）。前者是数据库里的全库活跃会话数，多实例是同一个值（Prometheus 用 `max()` 而不是 `sum()`）；后者是进程内累计，多实例各自独立、重启清零。

**作用范围**：只影响微应用网页版 `/a/<标识>` 的 CSP；其它 HTML（`/admin/*` 管理页、`/login`、`/go` 等）始终保持 `frame-ancestors 'self'`，配置过的宿主 origin **不能**框管理后台。

**可选的完整 CSP（`DSH_EE_CSP`）**：默认 `off` 时这里只加 `frame-ancestors`。设成 `report` / `enforce` 后会再给 HTML 加一份完整内容安全策略，`frame-ancestors` 与上面的嵌入来源**合并**：`report` 模式用 `Content-Security-Policy-Report-Only` 报告违规，但嵌入收口仍放在会执行的 CSP 里（不会因为「只报告」被放开）。模式、策略内容与上线顺序见 [operations.md](./operations.md) §10.1。

### 反代 / CDN 注意

- **不要**在反代再设置 `X-Frame-Options`（`ALLOW-FROM` 已废弃，和 `frame-ancestors` 叠加只会冲突）。
- **不要**在反代重复添加 `Content-Security-Policy`：多份 CSP 按交集执行，可能让这里的配置静默失效。
- 验收：`curl -sI https://ee.acme.internal/a/xxx | grep -i content-security-policy`，默认应看到 `frame-ancestors 'self'`，配置后带宿主 origin。

## 3. 宿主写法

```html
<iframe
  id="ee-frame"
  title="报名"
  src="https://ee.acme.internal/a/signup?embed=1&host=https%3A%2F%2Fharness.acme.internal&fit=auto&theme=dark"
  style="width: 100%; border: 0; min-height: 480px"
  referrerpolicy="strict-origin-when-cross-origin"
></iframe>
```

| 参数 | 必填 | 说明 |
|---|---|---|
| `embed=1` | 是 | 进入嵌入模式：隐藏返回、管理、演示身份条、CSV 导出、`dsh://` 链接；保留标题、页签、内容与表单 |
| `host` | 消息协议必填 | 宿主页面的 **origin**（`scheme://host[:port]`，`encodeURIComponent` 编码）。iframe 只把消息发到这个 origin；不传或非法时不发任何消息，页面仍可正常浏览 |
| `fit` | 否 | `fill`（默认，由宿主固定高度）或 `auto`（iframe 主动上报**内容高度**，见 `dsh-ee:resize`；auto 下嵌入页头部不吸顶、不再用 `min-h-screen`，高度贴合内容） |
| `theme` | 否 | `light` / `dark` / `system`：只作用于本次 iframe 渲染（**不写 localStorage**），首屏即生效。`system` 跟随 `prefers-color-scheme` 实时变化；宿主 `dsh-ee:theme` 消息仍可覆盖 |

- `host` 必须与 `DSH_EE_EMBED_ORIGINS` 中配置的来源一致，否则 iframe 会被 CSP 直接拦下。
- 不要给 iframe 加 `sandbox`（或至少给足 `allow-scripts allow-same-origin allow-forms`）：嵌入依赖同站 cookie 与脚本。
- 移动端：窄屏（< 640px）或移动 UA 建议**宿主在顶层打开**（`window.open` / 跳转），手机 iframe 的可用面积和输入体验有限。
  dsh-ee **不会**在 iframe 里自动顶层跳转（会被弹窗拦截，也会丢宿主上下文），而是在嵌入页顶部显示常驻的「在浏览器打开」入口，内容照常显示；
  `dsh-ee:ready` / `dsh-ee:hello` 的 `viewport.mobile`（以及 `dsh-ee:resize.mobile`）供宿主决定是否再展示自己的入口。

### fit=auto 的高度与布局

- 仅在 `embed=1&fit=auto` 时启用：iframe 用 ResizeObserver 观察页面根节点、按 rAF 节流上报
  `dsh-ee:resize { height, width, mobile }`。`height` 是**内容高度**而不是 iframe 视口高度，上限 4000px；
  内容变化（切页签、数据加载、字体/图片完成、视口旋转）会重新上报，内容变少时也会回缩。
- 嵌入页在 auto 下把头部从 `sticky` 改为静态、去掉整页 `min-h-screen`，页面由内容撑开，宿主按消息设置 iframe 高度即可贴合。
- `fill` 模式仍由宿主固定高度，嵌入页保持整页布局与吸顶头部。

## 4. postMessage 协议（v1 + v2 增量）

通用约定：

- v1 消息形如 `{ v: 1, type: "dsh-ee:<name>", ... }`；v2 新增消息用 `v: 2`（老宿主按未知消息忽略，不破坏 v0/v1）。未知 `type`、不兼容的 `v` 一律忽略。
- **iframe → 宿主**：只发往 URL 参数 `host` 的 origin，绝不使用 `"*"`；`host` 缺省或非法时不发。
- **宿主 → iframe**：必须用 `iframe.contentWindow.postMessage(msg, dshEeOrigin)`；iframe 会校验 `event.origin === host origin` 且 `event.source === window.parent`，不匹配直接丢弃。
- v0 消息里不携带 cookie、token 或业务数据。

| 方向 | type | 载荷 | 说明 |
|---|---|---|---|
| iframe → 宿主 | `dsh-ee:ready` | `{ app: { slug }, theme, supports: ["connect-v1"], viewport: { mobile, width, height } }` | 嵌入页一加载就发（早于认证）。同站宿主可据此收起 loading；跨站宿主据此 mint code 再发 connect；`viewport.mobile` 用于移动端策略 |
| 宿主 → iframe | `dsh-ee:connect` | `{ code }` | 仅跨站 v1：宿主后端 mint 的一次性 code（60 秒，绑定用户+应用+宿主 origin） |
| iframe → 宿主 | `dsh-ee:session` | `{ user: { display_name }, limited, expires_in? }` | 会话可用（cookie 或 exchange 成功）后发一次；不含 token |
| iframe → 宿主 | `dsh-ee:auth-required` | `{ reason: "missing" \| "expired" }` | 没有可用会话/会话过期：宿主 mint 新 code 后重新发 connect（只重试一次） |
| iframe → 宿主 | `dsh-ee:resize` | `{ height, width, mobile }` | 仅 `fit=auto`：ResizeObserver 上报**内容高度**（不是视口高度），节流到一帧，`height` 上限 4000px |
| iframe → 宿主 | `dsh-ee:error` | `{ code, message, retryable, stage }` | 失败态，见下表；同一条错误只发一次；`stage` 标明卡在加载/会话/code 交换哪一步 |
| 宿主 → iframe | `dsh-ee:theme` | `{ theme: "light" \| "dark" }` | 切换主题，只改 DOM、不写 localStorage |
| iframe → 宿主 | `dsh-ee:theme` | `{ theme }` | 确认当前实际主题（初始一次 + 每次变更） |
| iframe → 宿主 | `dsh-ee:hello` (v2) | `{ protocol: 2, supports: [...], app: { slug }, theme, viewport, timeouts: { handshake_ms: 10000, host_fallback_ms: 8000 } }` | 能力声明：`connect-v1` / `navigate` / `navigate-request` / `open-external` / `logout` / `deeplink` / `theme` / `resize`；`timeouts` 让宿主按同一口径设兜底。老宿主忽略即可 |
| 宿主 → iframe | `dsh-ee:navigate` (v2) | `{ path: "/a/<slug>?tab=…" }` | 只允许本应用 `/a/<slug>`；iframe 用 `history.pushState`/前端路由切换，**不整页刷新**（刷新会丢内存 Bearer） |
| iframe → 宿主 | `dsh-ee:navigate-request` (v2) | `{ path }` | iframe 内点击同源、但不属于本应用的站内链接时发出，由宿主决定顶层打开/路由 |
| iframe → 宿主 | `dsh-ee:open-external` (v2) | `{ url }` | 只发 `https://` 或登记过的 `dsh://`；宿主确认后再打开，iframe 不直接跳 |
| 宿主 → iframe | `dsh-ee:logout` (v2) | `{}` | 只撤销本次嵌入会话（cookie 会话模式下不会退出网页登录） |
| iframe → 宿主 | `dsh-ee:logged-out` (v2) | `{ revoked: boolean }` | 撤销结果：`revoked=true` 表示 ee_emb_ 会话已在服务端删除 |
| iframe → 宿主 | `dsh-ee:deeplink` (v2) | `{ path }` | mint 时传了 `next`（站内路径），exchange 后 iframe 消费并 pushState 到该路径；此消息用于通知宿主 |

`dsh-ee:error` 的 `code`：

| code | 含义 | retryable | 宿主建议 |
|---|---|---|---|
| `auth_required` | 未登录或会话过期（iframe 内 401 / 跨站没拿到 code） | false | 跨站宿主走 v1 握手；仍失败则在**顶层**打开应用（不要在 iframe 里打开登录页） |
| `forbidden` | 应用不存在、未开放网页版，或当前用户不在可用范围（404/403） | false | 提示无权限，或引导申请权限 |
| `load_failed` | 网络不可达、5xx 等 | true | 显示重试；页面内也有「重试」按钮 |
| `embed_code_invalid` / `embed_code_expired` | 跨站握手的 code 无效/过期/已用 | 过期可重试一次 | 重新 mint 后发新的 connect；仍失败则顶层打开 |
| `embed_disabled` / `origin_not_allowed` | 服务端没开跨站嵌入，或宿主来源不在白名单 | false | 检查 `DSH_EE_EMBED_CROSS_SITE` / `DSH_EE_EMBED_ORIGINS`（mint 阶段返回，通常先于 iframe） |
| `embed_scope` | 嵌入会话访问了白名单外的接口，或访问的不是 mint 时绑定的应用 | false | 宿主/前端问题；嵌入会话只能访问被嵌入的那个应用 |
| `app_not_allowed` | 应用不在管理端配置的嵌入白名单里 | false | 管理员在系统设置里加白名单 |
| `not_embed_session` | 对非嵌入会话调用 `/api/auth/embed/logout` | false | 只在 iframe 内调用 |

`dsh-ee:error` 的 `stage`（iframe 侧）：

| stage | 含义 | 典型 code |
|---|---|---|
| `load` | 页面/接口加载阶段：应用不存在、无权限、网络/5xx | `forbidden`、`load_failed`、部分 `auth_required` |
| `session` | 会话阶段：未登录、会话过期、握手超时（等不到 connect/session） | `auth_required` |
| `exchange` | 跨站一次性 code 交换失败 | `embed_code_invalid`、`embed_code_expired`、`embed_disabled`、`rate_limited`、`network` |

服务端 v1 错误体（mint / exchange / logout / 嵌入会话白名单）的 `stage` 与这里同名对齐（`mint` / `exchange` / `logout` / `session`），
宿主日志把两边对起来即可定位是「宿主后端 mint 失败」「iframe 换 code 失败」还是「页面加载失败」。服务端错误体统一带可观测字段：

```json
{ "error": "宿主来源不在允许列表里", "code": "origin_not_allowed", "stage": "mint", "request_id": "9f1c2ab34d5e6f70" }
```

`X-Request-Id` 响应头与 `request_id` 一致，便于把宿主日志与服务端审计对账；`stage` 取值 `mint` / `exchange` / `logout` / `session`。

宿主侧参考代码：

```js
const EE_ORIGIN = "https://ee.acme.internal";
const frame = document.getElementById("ee-frame");

// 手动兜底：不要自动 window.open（会被弹窗拦截），由用户点按钮打开顶层窗口。
function showOpenInBrowser(reason) {
  document.getElementById("ee-fallback").hidden = false;
  document.getElementById("ee-fallback-reason").textContent = reason;
}

// 从 iframe 创建/设置 src 起计时：被 CSP/sandbox 拦下时宿主可能收不到任何事件。
let fallbackTimer = setTimeout(() => showOpenInBrowser("iframe 没有响应，可能被 CSP/sandbox 拦截"), 8000);

window.addEventListener("message", (event) => {
  if (event.origin !== EE_ORIGIN || event.source !== frame.contentWindow) return;
  const msg = event.data;
  if (!msg || (msg.v !== 1 && msg.v !== 2)) return;
  switch (msg.type) {
    case "dsh-ee:hello":
      // v2：msg.viewport.mobile 判断手机；msg.timeouts 给出 10s 握手 / 8s 宿主兜底
      break;
    case "dsh-ee:ready":
      // 收起 loading；可在这里根据 msg.theme 同步宿主主题
      clearTimeout(fallbackTimer);
      break;
    case "dsh-ee:resize":
      // fit=auto：height 是内容高度，宿主决定是否撑开，注意自己也要有上限
      frame.style.height = `${Math.min(msg.height, 4000)}px`;
      break;
    case "dsh-ee:error":
      // msg.stage 区分 load / session / exchange；auth_required 顶层登录；forbidden 提示权限；load_failed 可重试
      clearTimeout(fallbackTimer);
      break;
    case "dsh-ee:theme":
      // iframe 当前主题；宿主可据此切自己的主题
      break;
  }
});

// 宿主 → iframe：等 iframe load 之后再发（或收到 ready 后发）
frame.addEventListener("load", () => {
  frame.contentWindow.postMessage({ v: 1, type: "dsh-ee:theme", theme: "dark" }, EE_ORIGIN);
});
```

```html
<!-- 宿主页里常驻一个手动兜底，8 秒仍没有 ready/error 时显示 -->
<div id="ee-fallback" hidden>
  <p id="ee-fallback-reason"></p>
  <a href="https://ee.acme.internal/a/signup" target="_blank" rel="noreferrer">在浏览器打开</a>
</div>
```

建议的失败兜底：

- 监听 `dsh-ee:error`；`auth_required` 时顶层打开登录/直接打开应用地址（去掉 `embed` 参数）后重新加载 iframe；`stage` 用于区分加载 / 会话 / code 交换。
- **超时与 CSP 拦截**：iframe 自身在 **10 秒**后进入失败态、发 `dsh-ee:error`；宿主从创建 iframe 起 **8 秒**仍没有 `ready` 也没有 `error` 就先显示手动的「在浏览器打开」。
  两个时间是有意错开的：8 秒是宿主的安全网（尤其 iframe 被 CSP/sandbox 拦下时，浏览器可能连 `load` 都不触发，宿主收不到任何消息），10 秒是 iframe 自己的失败态；收到 `ready`/`error` 后以消息为准。
- 嵌入页在任何失败态都自带「在浏览器打开」（去掉嵌入参数、顶层打开），移动端顶部也有常驻入口，不依赖宿主实现。

## 5. 失败态与「在浏览器打开」

- 嵌入模式下 iframe **不会**跳转 `/login`：未登录/过期时给出 `auth_required` 和错误页，页面上有「在浏览器打开」（顶层打开，去掉嵌入参数）。
- 加载失败、无权限同样有错误页和重试入口；错误码见上表。
- `dsh-ee:error` 带 `stage`（`load` / `session` / `exchange`）；服务端 HTTP 错误体带 `stage`（`mint` / `exchange` / `logout` / `session`）和 `request_id`，两边同名对齐，便于宿主日志与支持排查。
- **CSP/sandbox 拦截**：浏览器会直接拒绝加载 iframe，宿主拿不到可用的 `message`（部分浏览器连 `load` 都不触发）。因此宿主侧必须从创建 iframe 起按 §4 的 8 秒计时显示手动的「在浏览器打开」按钮；嵌入页自身的「在浏览器打开」在这种情况下不会出现。
- 移动端（窄屏 <640px 或移动 UA）：嵌入页顶部常驻「在浏览器打开」提示，内容照常可用；dsh-ee 不在 iframe 里自动顶层跳转，宿主应在选择 iframe 之前就决定是否改用顶层打开（或监听 `viewport.mobile` 后提示用户）。
- 移除这些条件后，用户也可以直接用 `https://ee.acme.internal/a/<标识>` 顶层访问，行为与以前一致。

## 6. 验收清单

1. `curl -sI https://ee.acme.internal/a/<标识>` 的 CSP 含 `frame-ancestors 'self' <配置的 origin>`；`curl -sI https://ee.acme.internal/admin/...` 仍只有 `'self'`。
2. 宿主页 iframe 加载后：隐藏返回/管理/导出，标题与页签正常。
3. 宿主控制台能收到 `dsh-ee:ready`（含 `viewport`）、`dsh-ee:theme`、`dsh-ee:resize`（`fit=auto` 时）；v2 宿主还能收到 `dsh-ee:hello`（含 `viewport`、`timeouts`），`navigate`/`logout` 生效。
4. **fit=auto**：`dsh-ee:resize.height` 是内容高度（把 iframe 拉高到明显大于内容时应小于 iframe 视口高度，而不是跟着变高），≤ 4000；切换页签/加载数据后重新上报；嵌入页 `header` 不再是 sticky。
5. **theme**：`theme=dark/light` 首屏生效；`theme=system` 跟系统深浅色切换，且 iframe 的 localStorage 里没有 `dsh-ee:theme`；宿主发 `dsh-ee:theme` 后以宿主为准，系统主题再变也不回退。
6. **移动端**：把 iframe 宽度改成 375–390px（或移动 UA）时，嵌入页顶部出现常驻「在浏览器打开」，内容仍可用，宿主页地址不变（没有自动顶层跳转）。
7. 管理端「系统设置 → 跨站嵌入」保存后：`curl -sI` 的 CSP 立即带上新来源；`/api/admin/embed-settings` 能看到 env 基线与有效值。
8. 嵌入会话 logout 后原 token 调 `/api/auth/me` 返回 401（服务端会话已删）。
9. 退出 dsh-ee 会话或换无痕窗口：iframe 显示失败态，宿主收到带 `stage` 的 `dsh-ee:error`（或 `auth_required`），iframe 的地址仍停在 `/a/...`。
10. 失败可观测：制造 404/无权限时宿主收到 `dsh-ee:error {code: "forbidden", stage: "load"}`；把宿主 origin 去掉后重新加载，iframe 被 CSP 拦下、宿主 8 秒后显示手动「在浏览器打开」。
11. 管理端「系统设置 → 跨站嵌入 → 运行概览」：完成一次握手后点「刷新」，活跃会话与 top apps 出现；`GET /api/admin/embed/stats` 不返回用户、IP、令牌或宿主 origin。

## 7. 跨站嵌入 v1（不同站点的 iframe）

不同站点的宿主（例如 `https://harness.example.com` 嵌 `https://ee.example.com`）里，`SameSite=Lax` 的网页会话 cookie 不会在 iframe 中发送。v1 用一条 **cookie-free** 的认证面解决：宿主后端 mint 一次性 code → iframe exchange 出只存内存的 Bearer。

**前提（硬性）**

- 宿主是**可信的企业系统**，宿主**后端**保管每个用户的 dsh-ee 令牌（`ee_at_`，含 `agent` scope）。浏览器里永远不出现 `ee_at_`。
- 服务端开启 `DSH_EE_EMBED_CROSS_SITE=1`，并在 `DSH_EE_EMBED_ORIGINS` 里列出宿主 origin（HTTPS；本地联调允许 `http://localhost[:port]` / `http://127.0.0.1[:port]` / `http://[::1][:port]`）。
- HTTPS 是硬前提；iframe 的 CSP `frame-ancestors` 由 dsh-ee 自己加，反代不要重复写 CSP/XFO。

**流程**

```
宿主(origin H)                                dsh-ee(origin E)
宿主 UI ──① 要 code ──► 宿主后端 ──② POST /api/embed/sessions ──► E
宿主 UI ◄──③ {code}（60s，绑定 H + app）────────┘   （Bearer 每用户 ee_at_）
宿主 UI ──④ <iframe src="E/a/<slug>?embed=1&host=H&…"> ──► SPA
SPA ──⑤ dsh-ee:ready{supports:["connect-v1"]} ──► 宿主 UI
宿主 UI ──⑥ dsh-ee:connect{code} ──► SPA
SPA ──⑦ POST /api/auth/embed/exchange {code}（同源，匿名）──► E
SPA ◄──⑧ {token:"ee_emb_…", limited:true, expires_in}（不 Set-Cookie）
后续 /api 请求带 Authorization: Bearer ee_emb_…（只存在 SPA 内存里）
```

**服务端接口**

| 方法/路径 | 认证 | 请求 | 成功 | 失败 code |
|---|---|---|---|---|
| `POST /api/embed/sessions` | `Authorization: Bearer ee_at_`（需 `agent` scope、human 账号） | `{app, host_origin, next?}` | `{code, expires_in:60, app, user, session_ttl}` | `embed_disabled` / `origin_not_allowed` / `app_not_found` / `app_not_web` / `forbidden` |
| `POST /api/auth/embed/exchange` | code 即凭证；必须 `Content-Type: application/json` | `{code}` | `{token:"ee_emb_…", token_type:"Bearer", expires_in, app, user, limited:true}`；**不 Set-Cookie** | `embed_code_invalid` / `embed_code_expired` / `embed_disabled` |
| `POST /api/auth/embed/logout` | `Authorization: Bearer ee_emb_…` | `{}` | `204`（删会话 + 审计） | `401` |
| `GET /api/admin/embed/stats?tz=` | 管理员网页会话（`settings.manage`） | 无 | `{active_sessions, created_today, expired_today, revoked_today, denied_24h, top_apps[], day_start, generated_at}`；只有聚合数量 | `400`（tz 非法）/ `403` |

- code 60 秒、一次性，只存 sha256 在数据库 `embed_codes` 表（明文只在 mint 响应里出现一次）；一次性由 `UPDATE … SET consumed_at=now() WHERE consumed_at IS NULL AND expires_at>now()` 的 affected rows 保证，**跨实例只有一个 exchange 能成功**。
- `ee_emb_` 会话默认 2 小时（`DSH_EE_EMBED_SESSION_TTL`，1m–8h），不提供 refresh；过期/被吊销后 iframe 发 `dsh-ee:auth-required {reason:"expired"}`，宿主重新 mint 一次。每个用户同时有效的嵌入会话默认**最多 5 个**，超过时撤销最旧的；logout 用 `revoked_at` 软删除，所有实例立刻看到 401。
- 嵌入会话 = **limited 会话**（员工权限、无 `keys.self`、无管理权限），并且只能访问路由白名单：`GET /api/auth/me`、`GET /api/auth/methods`、`GET /api/settings`、`GET /api/me/menus`、`GET /api/me/apps/{slug}/web`、`POST /api/me/apps/{slug}/source`、`/api/apps/{slug}/data…`（表读写/导出）、`POST /api/auth/embed/logout`；带 `{slug}` 的接口还要求 `slug` 等于 mint 时绑定的应用，其余接口 403 `{"code":"embed_scope"}`。
- 审计：mint 记 `embed.session_create`；exchange 成功记 `auth.login`（`via=embed`，含 app/host_origin、limited）；logout 记 `embed.revoke`；拒绝记 `embed.deny`。不记 code/令牌明文。
- 改密码、禁用账号、解绑身份会同时删除该用户的网页会话与嵌入会话。

**多实例部署**

多个 dsh-ee 实例（不同进程 / 不同端口 / 多副本）连同一个数据库时，跨站嵌入的三份状态都在共享表里：

| 状态 | 表 | 多实例语义 |
|---|---|---|
| 一次性 code | `embed_codes`（只存 sha256、绑定用户/应用/宿主 origin/next、60 秒过期） | A 实例 mint、B 实例 exchange 成功；同一个 code 并发 exchange 只有一个实例的 `UPDATE` 生效，其余返回 `embed_code_invalid` |
| 嵌入会话 | `embed_sessions`（token 只存 sha256、`revoked_at` 软删除） | 任一实例建立的会话，其它实例都能读；A 实例 logout 后，B 实例立刻 401；每用户会话上限（默认 5）是全局的 |
| 限流 | `embed_rates`（固定窗口，按 IP / 用户分键） | mint 30 次/分钟、exchange 60 次/分钟是**所有实例合计**；不会因为起 3 个副本就变成 90/180 |

- 清理：每个实例启动时清一次、之后每分钟清理过期的 code 与上一分钟的限流窗口（`DELETE` 条件相同，多实例重复执行幂等）；过期/已吊销的嵌入会话每小时清理一次，终态行保留 7 天供管理端按天统计，之后才物理删除。会话/code 的有效性判断不依赖清理任务，清理只回收空间。
- 兼容性：这三张表由迁移 `0034_embed_multi.sql` 创建；升级时所有实例跑同一版迁移；滚动发布期间新旧实例共存时，旧实例仍用内存 code（跨实例 exchange 会失败），建议一次性切换或先让所有实例升级到本版本。
- 仍不需要 Redis：当前规模用 MySQL 共享行锁足够；以后如果限流行成为瓶颈，再考虑把限流换成 Redis（code/session 的一次性语义仍以数据库为准）。

**宿主后端 mint 示例**

```bash
# 宿主后端（可信环境）持有当前用户的 ee_at_ 令牌；浏览器不接触它
curl -sS -X POST https://ee.acme.internal/api/embed/sessions \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $USER_EE_AT_TOKEN" \
  -d '{"app":"signup","host_origin":"https://harness.example.com"}'
# → {"code":"…","expires_in":60,"app":{"slug":"signup","title":"报名"},"user":{…},"session_ttl":7200}
```

**宿主页面处理 connect / auth-required / session**

```js
const EE_ORIGIN = "https://ee.acme.internal";
let minted = 0;
const frame = document.getElementById("ee-frame");

async function mintCode() {
  const r = await fetch("/api/ee-embed/mint", { method: "POST" }); // 宿主自己的后端
  const { code } = await r.json();
  frame.contentWindow.postMessage({ v: 1, type: "dsh-ee:connect", code }, EE_ORIGIN);
}

window.addEventListener("message", (event) => {
  if (event.origin !== EE_ORIGIN || event.source !== frame.contentWindow) return;
  const msg = event.data;
  if (!msg || msg.v !== 1) return;
  switch (msg.type) {
    case "dsh-ee:ready":
      if (msg.supports?.includes("connect-v1")) mintCode(); // 跨站：拿到 code 才能取数
      break;
    case "dsh-ee:session":
      // 已登录（跨站时 limited=true）；可以收起 loading
      break;
    case "dsh-ee:auth-required":
      // 会话过期：重新 mint 一次；仍失败则顶层打开应用
      if (++minted <= 1) mintCode();
      else location.assign(EE_ORIGIN + "/a/signup");
      break;
  }
});
```

**限制与安全边界**

- 每用户 `ee_at_` 是硬前提；不要用共享服务令牌 mint（会让所有嵌入变成令牌主人的身份、审计失真）。
- 共享状态在 MySQL/TiDB/OceanBase 的 `embed_codes` / `embed_sessions` / `embed_rates` 三张表里，只需要各实例连同一个库；固定窗口限流依赖各实例时钟同步（部署时开 NTP），窗口边界有几秒偏差时额度最多多放出一小截，不会成倍放大。
- `ee_emb_` Bearer 只存在 iframe 内存：不写 cookie / localStorage / URL；不通过 postMessage 回传给宿主。
- 不引入 `SameSite=None`、不放开 CORS、不做匿名公开链接、不把 dsh-ee 反代到宿主域名；现有网页会话与 CSRF 防线不变。
- 移动端（视口 < 640px 或移动 UA）建议宿主顶层打开而不是 iframe；嵌入页会在顶部常驻「在浏览器打开」，任何失败态也都有该入口（iframe 不被 CSP/sandbox 拦下时）。
