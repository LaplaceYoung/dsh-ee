# 接入插件界面优化建议（草案，2026-11-03）

## 现状

接入插件在 DSH 界面上只做了品牌：侧栏和首屏的 logo、名称（`client.ts` 占用 `sidebar.brand.mark`、`sidebar.brand.name`、`conversation.hero.brand.mark`）。技能、`ee_*` 工具、模型网关、`/ee-*` 命令都在后台工作，员工在 DSH 里看不出：

- 当前登录的是谁、额度还剩多少、用的是哪个企业模型；
- 企业有哪些微应用、提示词；
- 接入是否正常、管理模式是否开着。

下面按价值从高到低列出建议。插槽名都在上游源码（`.tools/upstream/deepseek-harness`）里出现过，但每个插槽在界面上的具体位置、能放什么内容还没逐个核实，见文末「动手前的验证」。

## 1. 企业面板：账号、额度、模型（最先做）

- **插槽**：`sidebar.right.pane.tab`，在右侧栏加一个企业名的标签页。
- **内容**：
  - 账号：姓名、部门、令牌什么时候到期、是否开着管理模式（开着的话显示剩余时间）；
  - 本月额度：进度条，用到 90% 变黄；
  - 能用哪些模型、默认模型是哪个；
  - 接入状态：插件版本、有没有新版本，以及一个「诊断」按钮（等同 `/dsh-ee-doctor`）。
- **未登录时**：面板只显示一个「登录」按钮，走 `/ee-login` 同一套设备登录，员工不用记命令。
- **数据**：`/api/dsh/whoami`、`/api/me/usage`、`/api/dsh/catalog` 等现成接口，服务端基本不用改。

## 2. 微应用放到首屏和侧栏

- `conversation.hero.agentPreset`：首屏空对话处把企业微应用列成卡片，点一下用对应的人设、工具和提示词开新会话。上游的 Agent preset（`packages/preset`）正好对应 dsh-ee 的微应用。
- `sidebar.panellist`、`sidebar.panellist.actions`：侧栏加一个「企业应用」分组。

## 3. 提示词变成 `/命令`

- 能力中心里「提示词」类型的能力，用 `ctx.commands.register` 注册成 `/命令`。现在它们只是技能，员工不知道怎么调用。
- 输入框按 `/` 时，企业命令带企业名前缀显示；可以用 `conversation.input.dock` 在输入框边放一个「常用」小条。
- 能力启用或停用后，跟技能一样随目录轮询更新。

## 4. 会话头部显示状态

- `conversation.session.header.utilities`：显示一个小标记，比如「企业模型 · 剩余 32%」或「管理模式 · 还剩 40 分钟」。
- 登录失效时标记变红，点一下重新登录。
- 用来解决「为什么突然用不了」，比让员工自己去跑 `/dsh-ee-doctor` 直观。

## 5. 设置页

- `settings.section`：加一个企业设置页，可以切换默认模型（只列出允许用的）、开关自动安装、看已装上的企业插件，并放上「打开网页工作台」「我的令牌」的链接。
- `settings.onboarding`：首次打开 DSH 的欢迎页，有可能直接嵌入接入码登录，和网页「开始使用」页走同一套流程。分发设计里记过「欢迎页插件换不了」（docs/distribution.md），这一项要先验证能不能用。

## 6. 写操作之后的提示

- `conversation.chat.turnTail`：Agent 每做完一次写操作，在这轮对话末尾显示「已在 dsh-ee 记录审计」和对应的链接。
- 额度快用完时，也在这里提示一次。

## 建议顺序

1 → 4 → 2 → 3 → 5 → 6。1 和 4 主要是展示现成数据，开发量小；2 要把微应用对应到 Agent preset，3 要动态注册命令，工作量大一些。

## 动手前的验证

先做一个最小验证：在 `dsh-plugin/src/client.ts` 里往 `sidebar.right.pane.tab` 和 `conversation.session.header.utilities` 各放一个最简单的组件，重新发布接入插件，在 Mac 的 Desktop 上确认：

1. 两个组件都能显示，位置符合预期；
2. 组件里能通过 `remote` 调到插件后端（参照品牌的 `DshEEBrandService`）拿到 `whoami`；
3. 同时占用这些插槽不会和官方插件冲突（品牌插槽是 single slot，曾经需要禁用 `ui-brand-official`）。

三条都通过再按上面的顺序往下做。

## 进度（2026-11-11）

| 项 | 状态 | 说明 |
|---|---|---|
| 1 企业面板 | 已做 | `status-view.ts`，`sidebar.right.pane.tab` |
| 2 微应用 | 已做，换了插槽 | 首屏用 `conversation.hero.options`（输入框下的横排），侧栏用 `sidebar.footer.action`。**不用** `conversation.hero.agentPreset`：它是 single 槽位，占了会顶掉 DSH 自带的 Agent preset 选择器；`sidebar.panellist` 是打开主区面板的图标，不适合列应用 |
| 3 提示词变 `/命令` | 已由技能覆盖 | 目录里的提示词以 `user_invocable` 技能下发，DSH 的 `/` 菜单本来就列出可由用户调用的技能，不再另注册命令。企业名前缀和「常用」小条没做 |
| 4 会话头部 | 已做 | `conversation.session.header.utilities` |
| 5 设置页 | 已做 | `settings-view.ts`，`settings.section` |
| 6 写操作之后的提示 | 已做，DSH Desktop 0.2.2-rc.1 实测通过 | 接入插件后端在调用 dsh-ee 写工具时按会话记到 `$DSH_HOME/dsh-ee/writes/<会话>.json`（`write-log.ts`）；`conversation.chat.turnTail` 按这一轮的起止时间取出，显示「已在 dsh-ee 留痕」和连接的网页详情链接（`turn-writes-*.ts`）。服务端在 `tools/list` 的 `_meta.dshee` 里带上连接的 `asset` 和 `system`。额度提示没做 |
