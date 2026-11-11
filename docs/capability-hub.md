# 能力中心设计（v0.2，2026-10-31）

> 状态：三个阶段都已实现，并用真实的 DSH 0.2.1-rc.5 端到端验证过（setup.sh → 从 dsh-ee 装接入插件 → DSH 任务加载企业技能、经 /mcp 调企业系统、模型走 dsh-ee 网关）。与下文设计不同的地方：
> - 微应用没有落成 agent preset（上游的 preset 只能写在静态配置里），而是以「只给人调用」的 skill 下发：`/名字` 加载人设、可用的技能与企业系统说明；依赖的插件进入必装列表。
> - 接入令牌直接复用 npm 令牌（`ee_npm_…`），没有另建一种令牌。
> - 技能暂不支持附件资源；知识类按约定暂不做。

把插件、技能、提示词、企业连接（MCP / API）、微应用汇聚到一个「能力中心」，按类型分 Tab，按授权范围下发到员工的 DSH。原「能力市场」改名为「能力中心」。知识库暂不做。

## 设计依据：DSH 上游能消费什么

读的是 `deepseek-ai/deepseek-harness` 源码（MIT，2026-10-30 的 main）。

| DSH 机制 | 位置 | 对 dsh-ee 的意义 |
|---|---|---|
| Skill 注册表 `ctx.skills`，`SkillProvider` 可以是远程来源 | `packages/skill`、`docs/subsystems/skills.md` | dsh-ee 可以做一个 skill 提供方，按人实时列出有权限的技能，**不用打包成 npm** |
| skill frontmatter `disable-model-invocation`，`/name` 直接调用 | 同上 | 「提示词」= 只给人调用的 skill，复用同一套机制 |
| `dsh-mcp-client`：streamable-http，`url` + `headers`（可 `!!js` 读环境变量），工具名 `mcp__<serverName>__<tool>`，处理 `tools/list_changed` | `packages/mcp/mcp-client` | dsh-ee 做**一个聚合的 MCP 端点**，员工 DSH 只配一行；授权变化通过通知实时生效 |
| 插件索引服务 `plugin-market`，Config `indexes` 可写在 profile 配置里；索引版本 `source.registry` | `packages/boot/plugin-market`、`docs/user/guide/plugin-market.md` | 企业索引源可以预置；索引里写明 dsh-ee 的 npm 源 |
| Profile 分层：组合包 patch → profile `cordis.patch.yml` → `$DSH_HOME/cordis.patch.yml`；web profile 热重载 | `packages/boot/app-boot` | 一个「企业接入组合包」就能把以上全部配好 |
| 自定义模型 API：`baseURL` + `openai-completions` | `docs/user/guide/providers.md` | DSH 的模型请求可以直接走 dsh-ee 网关（额度、审计已有） |
| Agent preset：人设 + 工具 + 提示词 + 技能的声明式组合 | `packages/preset` | 「微应用」对应一个 agent preset |

限制：DSH「发现」页读插件详情只走 npm 公网 CDN 或 Git，私有 registry 上的包在 DSH 里看不到 README，详情以 dsh-ee 网页为准。

## 核心思路：一个接入组合包 + 服务端实时下发

员工只装一次 **企业接入组合包**（`<主作用域>/dsh-connect`，由 dsh-ee 按企业设置自动生成并托管在自己的 npm 源）。`setup.sh` 在写 `.npmrc` 之后执行 `dsh plugin add`。它的 `dsh.bundle.patch` 插入四行配置：

1. **插件索引**：`plugin-market` 的 `indexes` 预置为 `<dsh-ee>/index/dsh-index.json`。
2. **企业连接**：一行 `dsh-mcp-client`，`serverName: <企业短名>`，`url: <dsh-ee>/mcp`，`Authorization` 从环境变量读取个人令牌。
3. **技能与提示词**：dsh-ee 自带的小插件，注册一个远程 `SkillProvider`，从 `<dsh-ee>/api/dsh/skills` 列出该员工有权限的技能和提示词，`resourceBase` 指向 dsh-ee。
4. **模型**：自定义提供商 `dsh-ee`，`baseURL: <dsh-ee>/v1`，协议 `openai-completions`。

这样一来，**只有插件需要「安装」**；技能、提示词、连接都由 dsh-ee 按人实时提供。管理员一授权，员工下一次刷新就能用；取消授权，立即消失。这就是首页示意图里「能力下发」那一步的真正实现。

令牌：员工在「接入指南」生成一个**接入令牌**，同时用于 npm 源、MCP、技能与模型（现在的 npm 令牌和 sk-ee key 分开管理，接入令牌是两者的统一入口；原有令牌继续可用）。

## 页面

员工侧 `/hub`（原 `/market`，旧路由跳转）：

```
能力中心                                                  [分享能力 ▾]
 全部 │ 插件 │ 技能 │ 提示词 │ 连接 │ 微应用
 搜索…   来源：全部 / 同事 / 社区   范围：我可用 / 全部   排序：部门在用 ▾
```

| Tab | 卡片上显示 | 员工的操作 | 提交方式 |
|---|---|---|---|
| 插件 | 版本、安装数、自动检查 | 复制安装命令；DSH「发现」里也能装 | 上传 .tgz / `npm publish`（现状） |
| 技能 | 用途、维护部门、可用范围 | 已授权的自动出现在 DSH；可预览全文 | **网页编辑器**写 SKILL.md，可带附件 |
| 提示词 | 指令模板、变量 | 自动出现在 DSH 的 `/` 命令里；网页上可复制 | 网页表单 |
| 连接 | 系统、工具数、只读 / 读写、调用量 | 已授权的自动出现；未授权的「申请使用」 | 登记 MCP 地址或导入 OpenAPI，管理员配凭据 |
| 微应用 | 场景、依赖的能力、运行次数 | 一键启用（落到 DSH 的 agent preset） | 选人设 + 依赖能力 |

管理侧：审核队列与能力管理按类型统一；新增「连接管理」（凭据、工具清单与工具级授权、调用日志）。

## 授权与下发

每个能力都有：

- **可用范围**：全公司 / 部门 / 角色 / 指定人。
- **下发方式**：
  - **自选**：在能力中心或 DSH「发现」里自己装（插件）或自己启用（其它类型）。
  - **推荐**：出现在对应员工工作台「为你推荐」，并发通知。
  - **默认启用**：技能、提示词、连接对范围内的人直接生效，无需任何操作。插件的「必装」需要接入组合包调用插件管理器自动安装，放到第 3 阶段。

## 连接网关（第 2 阶段的核心）

照搬模型网关的模式：

- `<dsh-ee>/mcp` 是一个 MCP 服务器，**聚合**该员工有权限的全部连接，工具名 `<连接>__<工具>`。
- 上游可以是企业已有的 MCP 服务（streamable-http），也可以是导入的 OpenAPI（每个接口生成一个工具）。
- 企业凭据只存在 dsh-ee（主密钥加密），不落到员工电脑。
- 工具级授权：同一个连接里，查询全员可用，写操作只给指定角色。
- 每次调用写审计日志与运营统计；授权变化时向在线会话发 `tools/list_changed`。

## 数据模型

新增统一目录层，现有 npm 表不动：

- `assets`：`id, kind(plugin|skill|prompt|connection|app), slug, title, description, owner_id, department, status, visibility(JSON), delivery(self|recommend|default), npm_name(插件类), spec(JSON), created_at, updated_at`
- `asset_versions`：技能、提示词、微应用的版本与审核状态（插件的版本仍在 `npm_versions`）
- `asset_grants`：可用范围展开后的授权（部门 / 角色 / 用户）
- `connections`、`connection_tools`、`connection_calls`：连接网关（第 2 阶段）

迁移：现有插件包各生成一条 `kind=plugin` 的 asset 记录，`dsh plugin add` 与索引不受影响。

## 分阶段

1. **能力中心 + 技能与提示词**：改名与 Tab 页面；`assets` 模型与可用范围；技能、提示词的网页编辑、审核、版本；接入组合包的第 1、3、4 行（索引、技能提供方、模型）；索引补 `source.registry`；接入令牌。
2. **企业连接**：MCP 聚合网关、OpenAPI 导入、凭据托管、工具级授权、调用审计；接入组合包第 2 行。
3. **微应用 + 主动下发 + DSH 分发**：微应用落到 agent preset；插件「必装」；企业定制版 DSH（预置接入组合包、品牌）。

## 待验证

- 技能提供方插件：需要按上游的 Cordis 插件规范写一个最小插件，验证远程 `SkillProvider` 在 web profile 里的加载、缓存失效与 `/name` 调用。
- 接入令牌怎么交给 DSH：环境变量（`!!js process.env`）与上游 `packages/credentials` 二选一，先读后者的文档。
- 品牌：上游规范禁止在产品名中使用完整「DeepSeek Harness」商标，企业定制版命名用「DSH」或「基于 DeepSeek Harness 构建」。

## 与插件质量三层（ENTRY / FLOW / BOOST）的关系

能力中心**不显示、不认证**三层等级：预检对应 ENTRY 的静态部分，审核 / 可见范围 / 安装 / 调用审计对应 FLOW 的企业侧，BOOST 不在 dsh-ee 的范围内，只作为作者证据阅读。边界和映射见 [plugin-quality.md](./plugin-quality.md)。
