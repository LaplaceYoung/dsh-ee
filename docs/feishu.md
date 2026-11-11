# 飞书：免登登录与组织同步（v0.2 D1 纵切）

> 对应 [roadmap.md](./roadmap.md) 阶段 D 的飞书免登登录、部门与人员同步，以及身份/组织接口需要可复用的要求。本文的范围不包括飞书机器人问答、通知、卡片审批和飞书工作台入口。

## 1. 功能边界

本次完成：

- 飞书预设的 OIDC 登录（授权码 + PKCE + state/nonce），复用现有多 IdP 登录、JWKS 验签、用户材料与角色映射。
- 管理员在同一个配置页填写 App ID / App Secret、Issuer 和通讯录 API 地址。
- 使用同一 App 凭据取 tenant token，分部门/人员分页获取目录，手动同步和可选每小时同步。
- 资料、部门和映射角色按目录更新；`open_id` 稳定绑定，本地改用户名不会重复建号。
- 飞书来源账号在目录中缺席、删除或冻结时按选择同步禁用，吊销会话与连接凭据。

不做：

- 飞书通知、审核与权限申请卡片内通过/驳回、飞书工作台打开网页应用；
- 飞书机器人问答；
- 外部 HR 字段自定义映射、按用户单字段高级访问控制（属第 3 批扩展字段）；
- 客户端专属登录接口或自己实现一套非 OIDC 登录状态机。

## 2. 配置与凭据

| 配置 | 用途 / 要求 |
|---|---|
| Feishu 预设 | `preset=feishu`。登录默认 Issuer 是 `https://accounts.feishu.cn`；可在测试或兼容环境替换 |
| Client ID / Client Secret | 飞书企业自建应用的 App ID / App Secret。OIDC 和通讯录 tenant token 共用；不回显，主密钥加密存 settings |
| Issuer | 用于 `/.well-known/openid-configuration`、授权、换 token、JWKS 和 userinfo |
| `feishuApiBase` | 通讯录 API 根地址，默认 `https://open.feishu.cn`。必须为完整 http(s) 地址，不能内嵌账号密码 |
| `autoCreate` | 首次登录或同步时，若本地尚无账号，是否自动创建（额度仍为 0）。关闭后只允许已预建/已同步账号 |
| `feishuSync` | 每小时执行完整一次组织同步；不影响手动按钮 |
| `feishuSyncDisable` | 允许对本次目录里确实缺席、删除、冻结的飞书来源账号同步禁用 |

所需飞书权限由企业管理员在开放平台批准：标准用户信息、部门与成员只读权限。实际需要的 scope 由飞书平台返回并以租户权限为准；本仓库不会用一个固定 scope 字符串绕过授权。

OIDC client secret 只在登录换取 token 与组织同步获取 tenant token 时使用，不写员工页面、工具结果或错误信息。token 与 secret 不经过 Agent 路由。

## 3. 身份绑定

- 登录仍遵循通用 `oidc.Config` 与 `provisionOIDC`：先按外部 subject 找人，只有找不到时才按 dsh-ee 用户名找；本地/LDAP/机器人同名账号拒绝冒用。
- 飞书 external subject 统一为 `feishu:<open_id>`。**不是裸 open_id**，这样 Keycloak 等其他 IdP 同样使用 OIDC 的用户不会被通讯录误匹配。
- dsh-ee 用户名优先取邮箱前缀，其次工号，最后 `open_id`，统一经既有的合法用户名规范化。部门名可能出现多个时按目录顺序用 ` / ` 连接；部门 ID 与名称同时给角色映射使用。
- 目录里的部门变更只更新资料与映射角色。管理员手工授予、没有纳入映射控制的角色保留。
- 首次开通额的 token 额度仍为 0，需管理员另行分配。默认有「员工」角色，除非角色本身也受映射控制。

## 4. 组织同步规则

### 拉取

- 每次使用 App ID / App Secret 请求一个 tenant token，调用 `/open-apis/auth/v3/tenant_access_token/internal`。
- 从部门 `0` 开始遍历 `/open-apis/contact/v3/departments/parentchildren`，分页 `page_size=50`，按 `has_more` / `page_token` 进入下一页。
- 同部门调用 `/open-apis/contact/v3/users/find_by_department`，同样全量分页。部门 ID 去重、`open_id` 去重后在本地按 id 排序，保证测试和同步顺序稳定。
- 任一 HTTP 非 200、JSON 解析失败、飞书 envelope 的 `code != 0`、token 错误或分页失败，整个目录拉取失败。
- **`users == 0` 也中止本轮**，视为配置/权限异常而不是“公司已没人”，不允许误禁用全公司账号。

### 更新

先完整拉取成功，再逐个员工更新。这样可以拒绝用半份目录进行批量禁用。

- 已在目录但被删除/冻结的人员，视同失效：不会重新建号、不会替换其它已有身份。
- 本地与外部 id 不同但用户名相同，或用户名与本地/LDAP 冲突，只记入 `conflicts`，不覆盖本地所有者。
- 本地已禁用账号不自动恢复，避免绕过管理员明确禁用。
- `autoCreate=false` 时不会通过同步批量创建新账号。
- HTTP 客户端直接从现有 OIDC 注入（测试用例替换），生产请求使用同一个受控 HTTP 配置。

### 禁用与恢复

禁用只在以下条件全部成立时执行：

1. `feishuSyncDisable=true`；
2. 本轮通讯录完整成功；
3. 本地账号 `auth_source=oidc` 且 `external_dn` 有 `feishu:` 前缀；
4. 外部账号明确不在目录、被删除或冻结。

操作包括：本地禁用、删除全部网页会话、撤销个人连接凭据。已有 dsh-ee 令牌依赖当前用户禁用状态而失效。记录被禁用的用户名与审计来源（系统/手动）。

`SyncDisable=false` 时只做资料和角色映射更新，不会因目录缺席而禁用账号。管理员对重建/恢复员工可在解除旧绑定并明确处理角色后按同一 open_id 重新认证，不自动覆盖已禁用账号。

## 5. 操作入口与状态

管理端「登录方式 → 企业统一登录 → 飞书预设」：

1. 打开企业统一登录并选择飞书。
2. 填写 Issuer、Client ID、Client Secret 与通讯录地址，保存。secret 不回显。
3. 需要后台每小时同步目录时打开 `feishuSync`；需要目录失效自动禁用时再单独打开 `feishuSyncDisable`。
4. 修改未保存时不能点「立即同步」，防止误把旧配置当新配置用。
5. 结果展示目录人数、新建、核对、禁用、冲突、跳过以及错误信息。错误不带有 client secret。

API（均要求系统设置权限，不走 Agent）：

- `GET /api/admin/auth/feishu/sync`：最近一次同步结果；
- `POST /api/admin/auth/feishu/sync`：手动同步一次；
- `GET /api/auth/methods` / `GET /api/auth/oidc/start`：登录按钮与 OIDC 起点；
- `GET /api/auth/oidc/callback`：OIDC 回调，state、PKCE、nonce、ID Token/JWKS 仍由现有逻辑校验。

后台只在 `enabled && preset=feishu && feishuSync` 时每小时同步；没有全局引入第三方 Cron 或常驻额外进程。

## 6. 自动化验证

最终执行结果：`DSH_EE_TEST_DSN='root:@tcp(127.0.0.1:4000)/' go test -timeout 30m -count=1 ./...` 全部通过；`go vet ./...` 通过；`dsh-plugin npm test` 150/150；真实浏览器全量 E2E 27/27。

### 客户端

- tenant token 的 App ID / Secret 校验；
- 部门与员工分页、员工排序、部门 ID → 名称映射；
- envelope `code != 0` 中止，绝不返回半份可用目录；
- `open_id` 外部身份前缀与冻结状态。

### 服务端集成（真实 TiDB）

`TestFeishuLoginAndOrganizationSync`：

1. 多名飞书员工完整同步，验证默认额度、部门和外部 id；
2. 真正调用 OIDC start + callback（授权码 + PKCE + state/nonce + JWKS），登录复用已同步账号；
3. 部门变更更新资料和映射角色，不新增重复账号；
4. 目录缺席员工、目录明确冻结员工分别触发同步禁用，仍在线员工不被误禁用；
5. 目录 API 失败后不批量禁用、不泄露 client secret。

`TestFeishuCardsAndCallbacks`：实际发出审批卡后模拟回调，覆盖批准、拒绝、错误消息号、未知操作者、普通员工越权、重复回调、权限申请授权及 JSON 字符串形式的 action_value。

`TestFeishuKBGapNotification`：相同语义的零命中检索去重通知，聚合最近一次文案与次数，不会在下次同步重复发送。

`e2e/tests/29-feishu-cards.mjs`：实际发出卡片、回调批准版本、重复回调 409；另有 `docs/images/feishu-approval-cards.gif` 记录页面和工作台入口。

### 浏览器交互与录屏

`e2e/tests/28-feishu-org-sync.mjs` 在本机起一个同时实现 OIDC discovery、JWKS、授权码 + PKCE、tenant token 和通讯录的假飞书：

- 管理员选择飞书预设，填写凭据并保存，secret 不显示；
- 点击立即同步，确认飞书员工和研发部建号成功；
- 从登录页进入假飞书授权端，回调后按既有“新用户引导”走到工作台，呈现「飞书员工 · 研发部」，且 `external_dn=feishu:ou-1`。

设置 `E2E_RECORD=1` 时使用 Playwright 实际录制上述交互。压缩 GIF 为 `docs/images/feishu-org-sync.gif`，用于 PR / review 附件；常规 CI 不强制生成视频文件。

## 7. 通知、审批卡与工作台（本批新增）

两个独立开关默认关闭：

- `feishuNotify`：是否发送结果通知和待补提醒；
- `feishuNotifyActions`：是否允许卡片内通过 / 驳回。开通结果通知不会隐式开通卡片写权限。

发送失败不回滚网页中已经落库的业务结果。通知目标按员工的 `feishu:<open_id>` 映射，缺失、禁用或未绑定身份账号不会被误投给他人。

本次支持资产新版本、能力范围分享、连接/技能/知识库权限申请、审核结果和待补检索词提醒。NPM 插件审核卡仍未实现，不能宣称所有资源都已覆盖。

### 卡片动作与验收规则

卡片使用 Card 2.0，发通过/驳回按钮。随机 action id 只以 SHA-256 摘要形式存储，绑定消息号、审批对象和状态，不包含 App Secret 或任何个人凭据。

动作状态为 `pending → processing → done/failed`：先校验卡号、动作是否存在、是否过期，再校验操作者和当前 RBAC，随后原子占位，最后按原有审批规则落库。并发或重放不能重复执行，失败不能伪称批准成功。

- 只处理 `card.action.trigger` 的 button 事件，action_value 兼容对象与 JSON 字符串。
- 操作者必须映射到未禁用的飞书来源 `open_id` 账号；当前管理员、维护者、申请审批人权限重新判定，不能靠旧卡片永远拥有权限。
- 同一审批 token 30 分钟过期、只允许执行一次真实决定；重复回调返回 409，并附明确 toast。
- 独立一次性 action id 是高熵授权凭据，**不把按钮中的对象 ID 当作权限证明，也不伪造 Feishu 签名算法**。生产回调应只经 HTTPS 使用。

### 工作台免登打开应用

`GET /api/admin/auth/feishu/workspace-url?next=/a/<slug>` 返回用于开发者后台配置的 OIDC 入口，也支持 `/go`。员工经标准 OIDC 回到本地会话，不必重复输入 dsh-ee 密码。工作台路径只容许 `/a/<应用标识>` 或 `/go`，请求不能借此越权打开管理接口。应用原有 Scope、只读/写表权限与「在 DSH 里继续」保持不变。

## 8. 尚未完成与后续对接

- NPM 插件审核卡；
- 额度告警的聚合与定时发送；
- 飞书机器人问答 D2；
- 钉钉、企业微信复用同一身份、外部 subject 与同步结果语义，而不是复制一套本地账户与权限体系。

不能因为飞书 D1 的登录、组织同步与卡片都已跑通，就把整个飞书能力和企业 IM 生态视作完成。
