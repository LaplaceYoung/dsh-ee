# 插件质量三层：ENTRY / FLOW / BOOST 与 dsh-ee 的关系

> 来源：本文的三层模型、术语与清单结构来自 PerryLink 维护的 [dsh-standards](https://github.com/PerryLink/dsh-standards) 与 [dsh-plugin-guide](https://github.com/PerryLink/dsh-plugin-guide)（Apache-2.0）。本文只做 dsh-ee 企业侧映射，不代表 dsh-ee 对三层做认证。

> 本文说明作者的三层质量指南（ENTRY / FLOW / BOOST）在 dsh-ee 里**放在哪、起什么作用、不起什么作用**。三层标准本身由 PerryLink 的 [`dsh-standards`](https://github.com/PerryLink/dsh-standards) 维护；本文只写 dsh-ee 这一侧的边界，避免把企业市场写成认证机构。

## 1. 先说边界

- dsh-ee 是**企业能力中心/私有 registry**：负责准入、审核、可见范围、安装、权限、审计和运行。
- 三层标准回答的是「插件能不能装、用起来顺不顺、有没有用好 Cordis」，是**作者/市场工具**，不是 dsh-ee 的评分、等级或认证。
- dsh-ee 的预检和审核结果只有 `pass` / `warn` / `fail` / 待审 / 已上架这些状态，**不显示 ENTRY/FLOW/BOOST 等级**，也不据此给插件排名。
- `security` 是 dsh-standards 的一个 scope，不是第四层；dsh-ee 的密钥扫描、安装脚本检查也不能当作安全审计。

## 2. 三层分别落在 dsh-ee 的哪个面

| 层 | 回答的问题 | 在 dsh-ee 里对应什么 | dsh-ee 能证明什么 / 不能证明什么 |
|---|---|---|---|
| **ENTRY** | 能不能被识别、安装、启动 | 发布前预检（`POST /api/me/packages/check`）：`package.json`、`dsh.bundle.patch`、`engines.dsh`、安装脚本、密钥、License、体积；审核页 | 能证明包结构、声明和静态风险；**不执行插件**，`recognized` 不等于 runtime |
| **FLOW** | 用户能不能顺利安装、配置、使用、升级、卸载 | 审核、可见范围、下发方式、版本/下架、员工安装、调用审计、权限申请 | 能覆盖企业侧的安装与治理流程；作者自己的 README、配置体验、升级/回滚体验仍要作者给证据，dsh-ee 不替作者证明 |
| **BOOST** | 有没有用好 Cordis（effect、inject、配置、事件、并行等） | 无 | dsh-ee 不做运行时基准，也不应该把 BOOST 当准入门槛 |
| **security**（scope） | 静态安全边界 | 预检的密钥/安装脚本/敏感文件检查；企业内网的运行审计 | 只是静态已知模式检查，不是渗透测试或安全认证 |

## 3. 作者怎么用

1. **发布前**：先按 [`dsh-standards`](https://github.com/PerryLink/dsh-standards) 跑 ENTRY，再按 [`dsh-plugin-guide/docs/levels.md`](https://github.com/PerryLink/dsh-plugin-guide/blob/main/docs/levels.md) 的 ENTRY/FLOW/BOOST 清单自查。
2. **在 dsh-ee 提交时**：预检页出现 fail 先修；warn 要在审核说明里解释（例如安装脚本、体积、没有 `engines.dsh`）。
3. **审核时**：把 FLOW 的 J1–J8 当成**审核问题**，不是自动化门禁。作者可以在 PR/审核备注里附证据链接（录屏、命令输出、证据文件），dsh-ee 目前没有字段存这些证据，管理员按企业需要人工看。
4. **不要**在 dsh-ee 页面上宣称「ENTRY 通过」「BOOST 达标」；当前没有这个数据，也不应该用 dsh-ee 的审核状态暗示三层结论。

## 4. 审核员怎么用

- 用三层清单**提问和记录**，不要把它变成打分表。
- 对 ENTRY：以预检结果为准；预检没覆盖的运行时启动，要求作者给出真实 `DSH_HOME` 的安装/启动证据。
- 对 FLOW：至少确认安装、升级、卸载、配置错误恢复、权限披露；企业侧再叠加可见范围和审计。
- 对 BOOST：只作为作者自述/证据阅读，不阻断上架。
- 对 security：预检 fail 必须处理；但要明确它不等于安全审计，不能对外写「已通过安全认证」。

## 5. 为什么不把三层全文搬进 dsh-ee

- 三层标准的正文、条款号、检查器和版本矩阵在 `dsh-standards` / `dsh-plugin-spec` / `dsh-plugin-guide` 维护；dsh-ee 复制一份必然过期。
- dsh-ee 的文档面向「企业管理员 + 内部作者」，需要的是**映射和边界**，不是完整规范。
- 一旦 dsh-ee 内置等级，用户会自然读成「企业认证/排序」，这与三层模型「不是三个等级、不是证书」的定位冲突。

## 6. 相关链接

- 三层模型说明：[dsh-plugin-guide/docs/levels.md](https://github.com/PerryLink/dsh-plugin-guide/blob/main/docs/levels.md)
- 检查器与条款：`dsh-standards`（npm / GitHub）
- 规则文本：`dsh-plugin-spec`
- dsh-ee 作者指南：[plugin-authors.md](./plugin-authors.md)
- dsh-ee 预检实现：`server/internal/registry/tarball.go`、`server/internal/registry/patch_check.go`

## 7. 如果以后要在 dsh-ee 展示三层

应该做的是**只读的作者证据链接/描述**，不是准入分或排行：

- 作者在提交时可选填一个 `quality_evidence` 链接（外部证据文件/录屏），审核员可见；
- 不改变 `can_submit` / `will_publish`，不进入市场排序；
- 需要先定字段、权限和过期策略，属于后续设计，不在 v0.1 范围内。
