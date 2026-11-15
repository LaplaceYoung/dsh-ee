# 设计文档索引（docs/ 42 篇）

> `docs/` 下的 Markdown 混着四类东西：**设计稿**、**实施方案**、**完成报告/交接单**、**验证清单**。这一页给每篇标注分类、当前状态、适用版本、是否已实现、从哪进，免得把设计稿当现状、把报告当承诺。
>
> 基线：`origin/main`（20aafbb，2026-11-12）。状态以代码和当篇文档头部的状态说明为准；本页只做索引，不改写设计稿。新增/修改设计稿时请回来更新本页。
>
> 相关页：当前架构见 [architecture.md](./architecture.md)（提示：本页与它随配套 PR 合入；合入前架构现状以 `README.md`、`server/README.md` 和代码为准）、开发环境见 [contributing.md](./contributing.md)、按角色读开发文档见 [dev/README.md](./dev/README.md)。

## 状态图例

| 标记 | 含义 |
|---|---|
| ✅ 已实现 | 文档描述的**主体能力**已在代码里（文档仍可能继续维护） |
| 🟡 部分实现 | 有已落地部分，也有明确未做项；缺口写在该行说明里 |
| ⚪ 设计 | 只有设计/建议，代码和接口都没有 |
| 📜 历史 | 交接单、完成报告，或已被新文档取代的过时设计；只作存档，不要按它实现 |
| 📋 计划/清单 | 路线图、任务清单、验证清单、发布计划；描述「要做/要验什么」，不代表实现状态 |

「适用版本」按 [roadmap.md](./roadmap.md) 的阶段：v0.1（阶段 A–C）、v0.2（阶段 D 飞书）、v0.3（阶段 E 扩展/多 Agent）、v0.4（阶段 F 知识库二期）；「—」表示不绑定版本（历史/计划）。

> **过时设计稿提示**：[design.md](./design.md) 是 2026-10-26 的 v0.1 设计稿，其中默认 SQLite/PostgreSQL + S3/MinIO、安装向导生成企业 CA 等选型**已不是现状**；现状看 [architecture.md](./architecture.md)，冲突时以 architecture、`server/README.md` 和代码为准。本文不重写它。

## 一、产品与路线（5）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [product-v1.md](./product-v1.md) | 🟡 | v0.1 | 第一版产品方案，多数能力已落地；未做项以任务清单和 roadmap 为准 | 根 `README.zh-CN.md` 的 docs 行、[roadmap.md](./roadmap.md) §2 |
| [product-v1-tasks.md](./product-v1-tasks.md) | 📋 | v0.1 | 任务清单：M1–M10、M12–M15 已完成或已验证；M9 待真实 DSH 复验；**M11 发布未开始** | roadmap §4；各 verify 文档的总入口 |
| [roadmap.md](./roadmap.md) | 📋 | v0.1–v0.4 | 阶段 A–F 的顺序与完成标准；阶段 C（v0.1 发布）尚未收口 | 根 README「Status」、[AGENTS.md](../AGENTS.md) 的补充；本索引的版本口径来源 |
| [launch-plan.md](./launch-plan.md) | 📋 | 发布后 | 发布后的获客/可理解性改进计划，不是功能实现说明 | 无其它文档入链；与 roadmap 阶段 C 部分重叠 |
| [compatibility.md](./compatibility.md) | 🟡 | v0.1 起持续 | 真实 DSH 场景的版本矩阵机制已建立（`result.json` 带 `dshVersion`）；矩阵条目随真机验证补充 | `e2e/README.md`、`e2e/dsh/scenarios/` |

## 二、总体设计、扩展与权限（8）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [design.md](./design.md) | 📜 | v0.1（历史） | 2026-10-26 设计稿；存储/部署选型已过时，**历史设计，现状见 architecture.md** | 根 `README.zh-CN.md` docs 行（历史链接）；不要再按它实现 |
| [plugins.md](./plugins.md) | 🟡 | v0.2 / v0.3 | 扩展体系第四稿：第 1 批进行中、第 2～5 批基本部分已实现（§12.1/§12.2）；签名最小闭环、企业目录登记、升级回滚已做最小部分，自动升级、自助分发**未实现** | [AGENTS.md](../AGENTS.md)、roadmap 阶段 E；与 access-control/agent-dev 配套 |
| [access-control.md](./access-control.md) | 🟡 | v0.3 | 可扩展权限草案：第 3、4 批已做出（见文档头）；字段先覆盖文档/知识库/用户，不做按人授权 | [AGENTS.md](../AGENTS.md)、plugins.md §12 |
| [agent-dev.md](./agent-dev.md) | ⚪ | 未定 | 让 Agent 完成扩展和权限配置的开发；文档头明确「设计稿，还没开工」；配置变更/审批部分有设计无实现 | [AGENTS.md](../AGENTS.md)、plugins.md §12（第 4 批） |
| [extension-protocol.md](./extension-protocol.md) | ✅ | v0.2 / v0.3 | 服务扩展与端侧扩展的协议第 1 版（清单、ee_jwt、约定接口、错误码、熔断） | [AGENTS.md](../AGENTS.md)、`server/README.md`「能力中心与 DSH 接入」 |
| [ee-jwt.md](./ee-jwt.md) | ✅ | v0.2 | 连接上的员工身份：`ee_jwt` 与 `user_secret`，协议说明与实现一致 | [plugins.md](./plugins.md) §7、`server/internal/api/user_identity.go` |
| [named-shared-credentials.md](./named-shared-credentials.md) | ✅ | v0.2 | 连接级多份命名共享凭据已实现；按应用指定哪一份**未做** | `server/README.md`「企业连接」 |
| [l1-client.md](./l1-client.md) | ✅ | v0.1 | 非 DSH 客户端（L1）接 `/v1` + `/mcp` 的用法；发现/能力/版本头属独立 PR #123，合入后以该分支内容为准 | 无旧入链；配套文档（`docs/dev/harness.md`、`docs/architecture.md`、`docs/harness-integration.md`）合入后互链 |

## 三、DSH 深度联动（8）

> 文档标题里的「v0.3 草案」是当时的规划名；阶段 A～D 已在 v0.1 前实现并留下报告。后续「支持其他 Agent」仍在 v0.3（见 roadmap 阶段 E）。

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [agent-connect-product.md](./agent-connect-product.md) | 🟡 | v0.1 | 产品规划：统一令牌、管理模式、Agent 能力；阶段 A～D 已实现，更多 Agent/界面能力未做 | roadmap §4 阶段 E 的「现状」 |
| [agent-connect-tech.md](./agent-connect-tech.md) | 🟡 | v0.1 | 技术规划与验证方式：设备码、凭据库、MCP 桥、写确认；主体已实现，个别上游细节过时（见文档头与配套文档） | 阶段 A 交接单、`docs/l1-client.md` |
| [agent-connect-phase-a.md](./agent-connect-phase-a.md) | 📜 | v0.1 | 阶段 A 实施规划（统一令牌与登录），已执行完 | [agent-connect-phase-a-handoff.md](./agent-connect-phase-a-handoff.md) |
| [agent-connect-phase-a-handoff.md](./agent-connect-phase-a-handoff.md) | 📜 | v0.1 | 阶段 A 给执行 Agent 的交接单；已完成，不再执行 | 阶段 A 实施规划 |
| [agent-connect-phase-a-report.md](./agent-connect-phase-a-report.md) | 📜 | v0.1 | 阶段 A 完成报告（提交、验证、遗留） | 交接单 |
| [agent-connect-phase-b-report.md](./agent-connect-phase-b-report.md) | 📜 | v0.1 | 阶段 B 完成报告（员工侧 `ee_*` 工具、诊断） | 技术规划 |
| [agent-connect-phase-c-report.md](./agent-connect-phase-c-report.md) | 📜 | v0.1 | 阶段 C 完成报告（管理模式、管理侧 Agent） | 技术规划 |
| [agent-connect-phase-d-report.md](./agent-connect-phase-d-report.md) | 📜 | v0.1 | 阶段 D 完成报告（管理员向导、巡检）；阶段 A～D 到此完成 | 技术规划 |

## 四、能力中心、工作台与知识库（9）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [capability-hub.md](./capability-hub.md) | ✅ | v0.1 | 能力中心三阶段已实现，并与真实 DSH 端到端验证过；与设计的偏差见文档头 | 根 `README.zh-CN.md` docs 行、`server/README.md` |
| [expert-center.md](./expert-center.md) | 🟡 | 未定 | 专家 · 技能 · 连接器（参考 WorkBuddy）：第 1–4 批已实现（服务端、管理端页面、接入插件右侧栏面板、技能包导入与连接器模板），专家与管理策略都能让 DSH 里的 AI 建和改，专家团队是单会话兜底版；个人自定义 MCP 未做；插件面板未在真实 DSH 里验证 | [capability-hub.md](./capability-hub.md)、[dsh-plugin-ui.md](./dsh-plugin-ui.md) |
| [knowledge.md](./knowledge.md) | ✅ | v0.1 | 知识库与「知识问答」工作台第一期已实现（2026-11-06），实现与设计的差别见 §11 | 根 `README.zh-CN.md` docs 行、roadmap 阶段 B |
| [knowledge-next.md](./knowledge-next.md) | 🟡 | v0.1（二期） | 第二期方案头写「待实现」，但检索/格式/体验主体已有代码与 e2e（见 knowledge-verify）；OCR 不做、向量看评测（v0.4） | [knowledge.md](./knowledge.md)、[knowledge-verify.md](./knowledge-verify.md) |
| [knowledge-verify.md](./knowledge-verify.md) | 📋 | v0.1 | 第二期自测清单：网页部分已过 e2e；DSH 原文窗格/看板引用尚未在真实 DSH 里验证 | [knowledge-next.md](./knowledge-next.md) |
| [workbench-panels.md](./workbench-panels.md) | 🟡 | v0.1 | 业务工作台面板设计；交接 §14 的 F1～F4、B1～B6 已实现，组件/能力仍按需扩展 | [workbench-handoff.md](./workbench-handoff.md)、`README.zh-CN.md` |
| [workbench-handoff.md](./workbench-handoff.md) | 📜 | v0.1 | 工作台交接单：W0–W4 已完成（文档头有进度）；§13 及以前是历史 | 工作台面板设计 |
| [workbench-report.md](./workbench-report.md) | 📜 | v0.1 | 工作台报告（只覆盖 W0、W1）；后续轮次进度看 workbench-verify | 交接单 |
| [workbench-verify.md](./workbench-verify.md) | 📋 | v0.1 | 工作台与 M9/M12–M15 的验证清单：多项已在真实 `dsh web` 验证，Desktop/暗色等有欠账 | [product-v1-tasks.md](./product-v1-tasks.md) 各里程碑状态 |

## 五、接入界面与品牌（3）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [branding.md](./branding.md) | ✅ | v0.1 | 品牌与外观设置（logo/名称/首屏/配色）已实现；改动在管理端「系统设置 → 品牌与外观」 | [distribution.md](./distribution.md)、[nav-verify.md](./nav-verify.md) 链接到它 |
| [dsh-plugin-ui.md](./dsh-plugin-ui.md) | 🟡 | v0.1+ | 接入插件界面优化建议（草案）；品牌与工作台部分已落地，其余建议未逐项实现 | [workbench-handoff.md](./workbench-handoff.md) |
| [nav-verify.md](./nav-verify.md) | 📋 | v0.1 | 导航改版与品牌外观自测清单：代码已写，真实浏览器/DSH 待自测 | 无其它文档入链 |

## 六、插件作者与发布（4）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [plugin-authors.md](./plugin-authors.md) | ✅ | v0.1 | DSH 插件作者指南（清单、patch、预检、发布、审核、常见失败），对应现有 registry/审核能力 | `server/README.md`「npm 源」；本文是权威作者入口之一 |
| [plugin-quality.md](./plugin-quality.md) | ✅ | v0.1 | ENTRY/FLOW/BOOST 与 dsh-ee 的边界说明；只做映射，不做认证/评分 | [capability-hub.md](./capability-hub.md) §末尾、[plugin-authors.md](./plugin-authors.md) 开头 |
| [plugin-install-verify.md](./plugin-install-verify.md) | 📜 | v0.1 | 插件安装去全局 npm 配置的验证清单（2026-11-03）；描述的行为已在实现里，结论见报告 | 无其它文档入链 |
| [plugin-install-verify-report.md](./plugin-install-verify-report.md) | 📜 | v0.1 | Mac 插件安装验证报告（V1/V3/V5） | 验证清单 |

## 七、桌面版分发（3）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [distribution.md](./distribution.md) | 🟡 | v0.1 | 桌面版分发设计：macOS 已实现（M0–M3）；Windows 第二期未做 | [distribution-handoff.md](./distribution-handoff.md) |
| [distribution-handoff.md](./distribution-handoff.md) | 📜 | v0.1 | macOS 分发执行交接单（M0–M3），已完成 | 分发设计 |
| [distribution-report.md](./distribution-report.md) | 📜 | v0.1 | macOS 桌面版分发完成报告 | 交接单 |

## 八、飞书（1）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [feishu.md](./feishu.md) | 🟡 | v0.2 | v0.2 D1 分批实现：免登/组织同步/审核卡/工作台入口已做；额度告警、批量预载通知、机器人问答**未做** | `server/README.md`「飞书」、roadmap 阶段 D |

## 九、示例数据（1）

| 文档 | 状态 | 适用版本 | 实现情况 | 入口/说明 |
|---|---|---|---|---|
| [demo-company.md](./demo-company.md) | ✅ | v0.1 | 示例公司「星河科技」的设定是所有示例数据的口径来源，已实现 | `server/cmd/seed`、`server/README.md` 本地开发 |

## 推荐阅读顺序

| 你是谁 | 从这里开始 | 接着读 |
|---|---|---|
| 新接触项目 | 根 `README.md` → [roadmap.md](./roadmap.md) §0–§2 | [architecture.md](./architecture.md)（现状）、[design-index.md](./design-index.md)（本页） |
| 要改服务端/前端 | [contributing.md](./contributing.md) | [architecture.md](./architecture.md)、[dev/quickstart.md](./dev/quickstart.md) |
| DSH 插件作者 | [plugin-authors.md](./plugin-authors.md) | [dev/plugin-publish.md](./dev/plugin-publish.md)、[plugin-dev/README.md](./plugin-dev/README.md)（改编指南） |
| dsh-ee 扩展作者 | [plugins.md](./plugins.md) §4 | [extension-protocol.md](./extension-protocol.md)、[extension-authors.md](./extension-authors.md)、[dev/extensions.md](./dev/extensions.md) |
| 第三方 Harness / 自研客户端 | [l1-client.md](./l1-client.md)、[harness-integration.md](./harness-integration.md) | [dev/harness.md](./dev/harness.md)、[errors.md](./errors.md) |
| 管理员（发布/审核/权限） | [enterprise-publishing.md](./enterprise-publishing.md) | [feishu.md](./feishu.md)、[troubleshooting.md](./troubleshooting.md) |
| 评审设计稿 | 本页「状态」列 + [design.md](./design.md) 历史提示 | 对应验证清单（workbench/knowledge/nav/plugin-install） |

## 本批新增文档（不属于上述 42 篇设计稿）

| 文档 | 类型 | 状态 | 说明 |
|---|---|---|---|
| [errors.md](./errors.md) | 索引 | 已随本 PR | 错误面/错误码索引，按域链到专页 |
| [contributing.md](./contributing.md) | 指南 | 已随本 PR | 开发环境、构建、测试、CI、发布、常见坑 |
| [design-index.md](./design-index.md) | 索引 | 已随本 PR | 本页 |
| [cross-site-embedding.md](./cross-site-embedding.md) | 设计说明 / 实现参考 | ✅ 已实现（#118，默认关闭） | 跨站嵌入 v1：一次性 code + 内存 Bearer + postMessage 握手已落地；管理端设置页、扩展消息、fit=auto sticky、移动端自动策略、多实例共享 code 未做 |
| [plugin-signing.md](./plugin-signing.md) | 设计 | **设计，未实现** | 插件/扩展签名、企业目录、自助分发、回滚的选项与风险 |

配套的使用/开发文档不在本页的 42 篇内，随各自 PR 合入：`docs/dev/`（#120）、`docs/plugin-dev/`（#119/#121）、`docs/extension-protocol/` 与 `docs/enterprise-publishing.md`（#125）、`docs/embedding.md`（#118）、`docs/architecture.md`、`docs/operations.md`、`docs/cli.md`、`docs/troubleshooting.md`、`docs/examples-walkthrough.md`（#126）。合入后这些页面由 `docs/dev/README.md` 与根 README 负责索引，本页不重复维护。

## 维护约定

- 新增设计稿：放 `docs/`，在本页对应分类表加一行（状态、适用版本、实现情况、入口）；改了状态（设计→部分→已实现）要回来改。
- 过时设计稿：在文档头写明「历史设计，现状见 architecture.md / 新文档」，并同步本页标 📜；不要重写原稿。
- 完成报告/交接单：加日期和范围；不把报告里的「完成」当成今天仍成立的状态，状态以代码和验证清单为准。
- 本页只索引，不复制其它文档正文；相对链接与锚点要能通过 `node scripts/check-docs.mjs`（随 #120 合入）。
