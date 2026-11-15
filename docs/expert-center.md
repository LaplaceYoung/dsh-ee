# 专家 · 技能 · 连接器设计（草案，2026-11-13）

> 状态：**第 1–4 批已实现，第 5 批做了专家团队的兜底版；专家与管理策略都能让 DSH 里的 AI 建和改**；个人自定义 MCP 没做（见 [§8.1](#81-实现情况2026-11-13)）。本文参考腾讯 WorkBuddy 的「专家 · 技能 · 连接器」，说明 dsh-ee 要补什么、分几批做。代码现状按 `main`（06dd768）核对。
>
> 实现和下文设计的差别：分类分组用 `app`（专家与微应用）而不是 `expert`；目录不另列 `experts`，专家留在 `apps` / `available_apps` 里带 `form: "expert"`；精选是每一项上的 `featured`，不另列表；连接器授权状态只有 `ready` / `needs_auth`（库里没有凭据到期时间，`expired` 做不了）；任务示例直接复用 `starters`；禁用策略的接口是 `/api/admin/hub/blocks`；成员自建策略没做；接入插件的面板放在右侧栏标签页（`sidebar.right.pane.tab`，企业面板已经在用），没有用未验证的 `sidebar.panellist`。
>
> **没在真实 DSH 里验证过**：接入插件的「专家 · 技能 · 连接器」标签页只跑了单元测试，见 [§9](#9-动手前的验证)。

## 1. 结论

dsh-ee 已经有技能、提示词、企业连接和微应用（见 [capability-hub.md](./capability-hub.md)），底子比 WorkBuddy 的三样东西只差一层：

- 微应用就是「专家」的超集，但没有按专家的方式呈现和分类；
- 技能和连接只能在网页上看和启用，员工在 DSH 里看不到有哪些、哪些要自己授权；
- 管理员只有「可用范围 + 下发方式 + 审核」，没有分类、精选、禁用策略和「这个人为什么看不到」的查询。

所以不新建一套系统，而是在能力中心上扩展：专家作为微应用的一种形态，接入插件加一个「专家 · 技能 · 连接器」面板，管理端加一页策略配置。

## 2. WorkBuddy 是怎么做的

资料来自 WorkBuddy 官方文档和腾讯云企业版文档（链接见 [§10](#10-参考资料)）。

| 概念 | 做法 |
|---|---|
| 专家 | 围绕一个领域的 AI 角色：角色说明、工作方法、任务要求，靠技能完成任务。专家中心按分类浏览、按关键词搜索；卡片上有名字、角色简介、领域标签；详情里有能力介绍、擅长领域、任务示例。点「召唤」进入会话，示例任务预填在输入框里，员工改完再发 |
| 专家团队 | 一个组长加若干成员。组长拆任务、分派、汇总；互不依赖的步骤并行，有依赖的串行 |
| 技能 | 来源分内置、市场、上传（`.zip`，内含 `SKILL.md`）。员工可以安装、停用、卸载；上传的要过安全扫描，高风险的必须人工确认 |
| 连接器 | 每个外部服务一张卡片，多数是 MCP。按人授权（OAuth、API Key、扫码），卡片上有启用开关。另有「自定义连接器」可以加任意 MCP 服务 |
| 企业管理（技能） | 分类（分类自己有可见范围）；每个技能的可见范围（部门、成员）；禁用策略和限制策略（禁用优先，多条限制取交集）；精选区最多 20 个；成员能不能上传个人技能的策略；权限查询（某成员最终能看到什么、被哪条规则挡住）；风险技能强行下发时记审计 |

没找到腾讯关于**连接器和专家**的企业管理文档：连接器文档只写了员工自己的操作，没有白名单、下发范围或审计。这部分 dsh-ee 按自己的连接网关来设计，不照搬。

## 3. 和 dsh-ee 现状的对应

| WorkBuddy | dsh-ee 现状 | 缺口 |
|---|---|---|
| 专家 | 微应用（`kind=app`）：人设、示例问题、依赖的技能/连接/插件、知识库、图标、要点（`assetSpec`，`server/internal/api/hub.go`） | 没有「专家」这层呈现；没有擅长领域标签和受管分类；DSH 里没有详情卡和「召唤」 |
| 专家团队 | 没有 | 全新；取决于 DSH 的子 Agent 能力 |
| 技能 | `skill` / `prompt` 资产，有可用范围、下发方式、审核、版本 | DSH 里没有技能列表；自选技能只能在网页启用（目录里只有 `available_apps`，没有 `available_skills`）；不支持带附件的技能包，没有扫描 |
| 连接器 | `connection` 资产走 `/mcp` 聚合网关；凭据在服务端；支持员工自己的 `user_secret` 和 OAuth；工具级角色限制 | DSH 里没有连接器面板和授权状态；没有模板库；没有「个人自定义 MCP」的策略 |
| 管理策略 | 可用范围（全公司/部门/角色/人）、下发（`auto` / `optional`）、版本审核、范围审核 | 分类只是一个自由文本字段；没有精选、禁用策略、成员自建策略、权限查询 |

## 4. 设计决定

### 4.1 专家是微应用的一种形态，不是新的资产类型

`assetSpec` 加一个字段 `form`：空或 `app` 是现在的微应用，`expert` 是专家。专家只用其中一部分字段（人设、工作方法、示例任务、技能、连接、知识库、图标、要点），不带工作台、数据表和网页版。

这样做的理由：

- 可用范围、下发方式、审核、订阅、导入导出、「缺少的能力」提示都直接能用；
- 旧版接入插件不认识 `form`，会把专家当普通微应用显示，仍然能用；
- `/v1/apps`（第三方 Harness）不用改就能拿到专家。

代价是「专家」和「微应用」共用一张表和一套校验，`validateSpec` 要按 `form` 分支：专家不允许出现 `workbench`、`tables`、`web`。

另一个选择是新增 `kind=expert`。概念上更干净，但目录、范围、审核、接入插件的列表和启动逻辑都要再写一遍，不选。

### 4.2 连接器的凭据留在服务端

WorkBuddy 让员工在自己电脑上加任意 MCP 服务。dsh-ee 的连接都经过 `/mcp` 网关，凭据不落到员工电脑，写操作有确认，调用有审计。个人自定义 MCP 会绕开这三样，所以放在最后一批，并且默认关闭、由管理员策略放开，走现有的本机扩展通道（`dsh-plugin/src/local-mcp.ts`，[plugins.md](./plugins.md) §8）。

### 4.3 专家团队后做

接入插件的会话状态里能读到子 Agent 目录（`session-status.ts`），但还没验证插件能不能按配置发起一组角色。第一批到第四批都不依赖它。

## 5. 服务端

### 5.1 数据模型

新增一个迁移（编号接在现有最大号之后）：

- `asset_categories`：`id, grp(app|skill|connection), name, sort, scope(JSON), created_at, updated_at`。分类有自己的可用范围：不在范围内的人看不到这个分类和里面的东西。分类只能收窄，不能把资产本身的可用范围放宽。
- `assets` 加两列：`category_id BIGINT NULL`（现有的 `category` 文本列保留，迁移时按名字归并成分类）、`featured TINYINT(1) NOT NULL DEFAULT 0`。
- `asset_blocks`：`id, scope(JSON), asset_ids(JSON), note, updated_by, created_at, updated_at`。第一版只做禁用策略：范围内的人即使在资产的可用范围里也用不了。WorkBuddy 的「限制策略」（只能用名单里的）先不做，可用范围加禁用策略已经能表达大多数情况。
- （第 2 批）`settings` 里加成员自建策略：`member_custom.{expert,skill,mcp}`，取值 `all` / `allow_list` / `deny_list` / `none`，名单是部门和人。

`assetSpec` 新增字段（只对 `form=expert` 有意义）：

| 字段 | 含义 |
|---|---|
| `form` | `app`（默认）或 `expert` |
| `expertise` | 擅长领域标签，最多 6 个 |
| `method` | 工作方法（Markdown），和 `persona` 一起拼进指令 |
| （任务示例） | 不加新字段，复用 `starters`（最多 6 条）；第一条在「召唤」时预填 |

### 5.2 判定顺序

一个员工能不能用某个资产：

1. 资产已上架、没下架；
2. 在资产的可用范围内；
3. 资产所在分类的可用范围包含他（没有分类的跳过）；
4. 没有命中禁用策略；
5. 下发方式是 `auto`，或者他自己启用了。

第 3、4 步是新增的，加在 `available()` 和 `userCatalog()`（`hub.go`、`dsh.go`）里，`/mcp` 的工具列表走同一个函数，所以禁用一个连接会同时让它的工具消失。

### 5.3 接口

| 接口 | 变化 |
|---|---|
| `GET /api/dsh/catalog` | 新增 `available_skills`、`connections`（标识、标题、工具数、`writes`、`enabled`、`auth`：`ready` / `needs_auth`、`auth_mode`、下发方式）、`categories`；`apps` / `skills` 的每一项加 `form`、`expertise`、`category_id`、`featured`、`delivery`。旧字段不动，专家就在 `apps` 里，旧插件照常可用 |
| `POST` / `DELETE /api/dsh/skills/{name}/subscription` | 新增，和现有的 `/api/dsh/apps/{name}/subscription` 同样的规则 |
| `POST` / `DELETE /api/dsh/connections/{name}/subscription` | 新增 |
| `GET/POST/PATCH/DELETE /api/admin/hub/categories` | 新增，分类管理 |
| `GET/POST /api/admin/hub/blocks`、`PUT/DELETE /api/admin/hub/blocks/{id}` | 新增，禁用策略（必须写原因；只能挡自己能管理的那几类） |
| `GET /api/admin/hub/effective?user=…` | 新增，权限查询：返回这个人每个资产的最终结果和被哪一步挡住 |
| `PATCH /api/hub/{id}` | 支持 `category_id`、`featured`（只有管理员能设精选） |

新路由按 [architecture.md](./architecture.md) §5.4 的约定生成对应的 `ee_*` Agent 工具或登记在 `agentExcluded`；策略类写接口需要管理模式。分类、策略、精选的改动都写 `audit_logs`。

## 6. 管理端网页

- **能力中心加「专家」Tab**，卡片上显示角色简介、擅长领域、依赖的技能和连接数。
- **专家编辑器**：现有资产编辑器（`web/src/pages/asset-editor.tsx`）的精简版，只留角色、工作方法、任务示例、选技能、选连接、选知识库、图标、分类、可用范围、下发方式。
- **新增管理页「专家 · 技能 · 连接器策略」**（`web/src/pages/admin/`，菜单走 `menus` 表的迁移）：
  - 分类：增删改、排序、可用范围；
  - 精选：每类最多 20 个，拖动排序；
  - 禁用策略：选人或部门、选资产、填原因；
  - 成员自建：专家、技能、自定义 MCP 三项各一个开关加名单；
  - 权限查询：选一个员工，列出他能用的和不能用的，以及原因。
- 现有的审核队列、范围审核、连接管理页不变。

## 7. 接入插件（DSH 里）

一个面板，三个 Tab，顶部搜索和分类筛选，精选排在最前。

| Tab | 卡片 | 操作 |
|---|---|---|
| 专家 | 图标、名字、角色简介、擅长领域 | 点卡片看详情（能力介绍、任务示例、用到的技能和连接）；「召唤」新开会话并预填第一条示例，复用 `app-launcher.ts`；缺少的能力显示「申请使用」，跳到网页 |
| 技能 | 名字、用途、维护部门 | 自选的有启用开关；默认生效的显示为锁定；可以预览正文 |
| 连接器 | 系统、工具数、只读/读写、状态 | `needs_auth` 时显示「去授权」，用现有的 `web-link.ts` 打开网页上的 OAuth 或填凭据页；自选的有启用开关 |

实现要点：

- 新文件 `dsh-plugin/src/center-view.ts`、`center-client.ts`，数据仍从 `skills.ts` 的目录轮询里取，不另起请求；
- 启用和停用成功后调 `refreshNow()`，技能列表和 `/mcp` 工具列表随之刷新；
- 现有的首屏应用横排和侧栏「企业应用」保留；专家默认也出现在这两处，管理员可以在接入设置里关掉；
- 服务端没有新字段时（旧版 dsh-ee），面板只显示专家 Tab，数据来自 `apps`。

## 8. 分批

每一批单独能发布。

| 批 | 内容 | 完成标准 |
|---|---|---|
| 1 服务端 | §5 的迁移、`form=expert`、判定顺序、目录新字段、订阅接口、分类/策略/权限查询接口 | Go 单测覆盖判定顺序的每一步；旧插件对着新服务端跑现有 e2e 不变 |
| 2 管理端 | §6 全部 | 管理员能建分类、建专家、设精选、加禁用策略，并用权限查询看到结果；Playwright e2e |
| 3 接入插件 | §7 全部 | 在真实 DSH 里：召唤专家、启用一个自选技能、给一个 OAuth 连接授权，三件事都不用打开网页以外的地方找入口；结果记进 [compatibility.md](./compatibility.md) 的版本矩阵 |
| 4 技能包与模板 | 导入 `.zip` 技能包（`SKILL.md` 加附件），进审核前做静态检查（复用 `registry/tarball.go` 里的 secrets、size 等检查思路）；连接器模板库，管理员从模板开始而不是空表单 | 能导入一个带附件的公开技能包并在 DSH 里用起来 |
| 5 以后 | 专家团队；个人自定义 MCP（策略放开时） | 先过 §9 的验证再定 |

### 8.1 实现情况（2026-11-13）

| 批 | 做了什么 | 验证 |
|---|---|---|
| 1 服务端 | 迁移 `0036_expert_center.sql`；`form=expert` 与校验；判定顺序（`unavailableReason`，所有 `available()` 调用点都生效，包括 `/mcp`）；目录新字段；技能、连接的订阅接口；分类、禁用策略、权限查询接口 | Go 集成测试 `TestExpertCenter`、单测 `TestAssetGate`；全量 Go 测试 |
| 2 管理端 | 「企业应用」页的专家/应用筛选与新建专家；编辑器的专家形态；详情页的专家信息；管理页「专家与连接器策略」（分类、精选与归类、禁用策略、权限查询），菜单 `admin-expert-center` | Playwright `e2e/tests/39-expert-center.mjs`；本地预览里手动走过 |
| 3 接入插件 | 右侧栏「专家 · 技能 · 连接器」标签页（`center-view.ts`、`center-client.ts`）：召唤专家、带任务示例开始、启用/停用自选的专家、技能、连接器，待授权连接器跳网页授权 | 插件单元测试；**没在真实 DSH 里跑过** |
| 4 技能包与模板 | `POST /api/hub/skill-package` 解析 `.zip`（SKILL.md + 文本附件，密钥扫描、路径与大小检查）预填编辑器，提交仍走审核；连接器编辑器里 5 个模板 | 单测 `TestParseSkillPackage*`；本地预览里调通接口、模板 |
| 5 专家团队 | 专家可以设最多 5 位成员（`members`）。DSH 还不能由插件发起多角色协作，所以组长的指令里带上各成员的角色说明与工作方法，由组长在一个会话里依次完成、最后汇总；成员用到的技能和系统一并列给组长 | `TestExpertCenter` 里的团队部分 |
| 让 Agent 做和改 | 和微应用一样，员工和管理员都可以在 DSH 里让 AI 建、改专家：`ee_hub` 的 create / new_version 认识 `form`、`expertise`、`method`、`members`，detail 返回这些字段（改版本时不会被清掉）；新增系统技能 `dsh-ee-expert-design` 教 AI 怎么写专家；网页「新建专家」默认是「让 AI 帮我做」，生成发给 DSH 的提示词。管理员在管理模式里用新工具 `ee_expert_admin` 管分类、禁用策略、查权限，`ee_hub_admin` 的 patch 可以设分类和精选；写操作都要在审批框里确认 | Go 集成测试 `TestExpertAgentTools`；管理模式工具体积预算测试 `TestAdminTools` 仍通过 |
| 5 个人自定义 MCP | **没做**。员工自己加的 MCP 服务不经过 `/mcp` 网关：凭据在员工电脑上，没有写确认和调用审计。要不要做、默认开不开，需要产品上定 | — |

## 9. 动手前的验证

1. **主区面板槽位**：现在的应用入口用 `sidebar.footer.action` 和 `conversation.hero.options`。[dsh-plugin-ui.md](./dsh-plugin-ui.md) 提到 `sidebar.panellist` 是打开主区面板的图标，适合放这个面板，但没实测过。第三批开始前在 Desktop 上放一个最小组件确认；不行就退回右侧栏标签页（`sidebar.right.pane.tab`，已在用）。
2. **专家团队**：确认接入插件能不能按配置创建一组子 Agent 并指定组长。不能的话，团队只能做成「组长专家的指令里写明什么时候加载哪个成员专家」，效果弱一些。
3. **旧字段合并**：`category` 文本归并成 `category_id` 的迁移（`0036_expert_center.sql`），要用一份真实部署的数据试跑。

## 10. 参考资料

- [WorkBuddy：专家中心](https://www.workbuddy.ai/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Expert-Center)
- [WorkBuddy：技能](https://www.workbuddy.ai/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Skills)
- [WorkBuddy：连接器](https://www.workbuddy.ai/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Connector)
- [腾讯云：企业 Skill 管理](https://cloud.tencent.com/document/product/1831/134420)
- [腾讯云：WorkBuddy 企业版连接器](https://cloud.tencent.com/document/product/1831/134525)

以上页面的内容是 2026-11-13 检索时的情况；产品在快速变化，按它实现前先重看一遍。
