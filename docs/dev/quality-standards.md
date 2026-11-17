---
slug: quality-standards
category: quality
title: "规范与检查：ENTRY / FLOW / BOOST"
title_en: "Standards and checks: ENTRY / FLOW / BOOST"
title_es: "Normas y comprobaciones: ENTRY / FLOW / BOOST"
title_pt: "Normas e verificações: ENTRY / FLOW / BOOST"
title_hi: "मानक और जाँच: ENTRY / FLOW / BOOST"
summary: "上游规范三层与 dsh-ee 预检的边界：哪些是作者自证，哪些是企业准入。"
summary_en: "Where upstream standards end and the dsh-ee precheck begins: author evidence vs company admission."
summary_es: "Dónde terminan las normas upstream y dónde empieza la verificación previa de dsh-ee: evidencia del autor frente a admisión de la empresa."
summary_pt: "Onde terminam as normas upstream e onde começa a pré-verificação do dsh-ee: evidência do autor versus admissão pela empresa."
summary_hi: "अपस्ट्रीम मानक कहाँ समाप्त होते हैं और dsh-ee प्रीचेक कहाँ शुरू होता है: लेखक द्वारा प्रमाण बनाम कंपनी प्रवेश।"
tags: [规范, 检查, 样例]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 三层规范各自回答什么

「规范」来自上游 dsh-plugin-guide / dsh-primer 与 dsh-standards。dsh-ee **不在服务端内置执行 checker**；仓库文档可以按各自许可引用或改编上游正文——CC BY 4.0 需保留 TASL、改动说明（Change Notice）与同视图免责，Apache-2.0 需保留 LICENSE/NOTICE。站内导读只写摘要与链接，改编正文与逐篇署名在仓库 [`docs/plugin-dev/standards/`（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) 与 [`docs/dev/standards.md`（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md)。

| 层 | 回答的问题 | 谁在用 | 在本系统里的位置 |
| --- | --- | --- | --- |
| ENTRY | 一个插件的最小形状是否成立（入口、清单、许可） | 作者、作者仓 CI | 与发布预检的包形状检查对齐 |
| FLOW | 插件在真实使用里的流程是否站得住（安装、启用、卸载、失败恢复） | 作者、测试 | 样例导读里说明怎么复现 |
| BOOST | 体验与可组合性是否到位（命令、渲染、技能、多端） | 作者、评审人 | 样例按涉及的面上分类展示 |

## ENTRY / FLOW / BOOST 在企业侧怎么用

| 用法 | 具体做法 | 不要做什么 |
| --- | --- | --- |
| 作者自证 | 在作者仓 CI 跑上游 checker（`dsh-standards@1.6.9`，要求 Node ≥24.9），把命令与版本固定下来 | 不要把上游检查结果当成企业的准入结论 |
| 企业准入 | 上传后看 dsh-ee 自己的预检明细（pass / warn / fail 与发布冲突） | 不要用上游检查代替预检 |
| 评审参考 | 审核时对照样例与规范条款提问 | 不要在页面里给插件或样例定级、排序 |
| 学习路径 | 按样例从入门到进阶读代码 | 不要把样例当官方推荐 |

## dsh-ee 不做的事

- 不在服务端执行第三方检查器，也不提供可运行检查的沙箱。
- 不把上游检查结果转成质量等级、评分或排序。
- 不把改编后的规范正文当成上游原文：引用或改编必须保留来源、许可（CC BY 4.0 / Apache-2.0）与 Change Notice，改编件逐篇署名。
- 不承诺「官方推荐」：样例只说明它展示了哪一类能力。

## 作者怎么留下证据

1. 在本机固定 `dsh-standards@1.6.9`（Node ≥24.9）运行，把命令写进作者仓 CI。
2. 保存 JSON 或 HTML 报告作为构建产物，记录生成时间与宿主版本。
3. 在发布说明里引用报告位置，而不是只写一句「已检查」之类的结论。
4. 上游规范升级时重新跑一遍，旧报告标注版本，不要混用。

```bash
# 固定版本，避免 npx 同名包冲突
npx dsh-standards@1.6.9 check .
npx dsh-standards@1.6.9 check . --json > report.json
```

checker 的退出码、报告字段与「not-run / NA / static 不等于通过」的读法，以仓库里的作者参考为准（见下方链接）。

## 与样例页的关系

样例页展示的是「一个完整可运行的最小走廊」：每个样例写清它能学到什么、前置条件、固定版本与安装命令，并在详情页给出来源与许可。样例不附带检查结论，页面也不会用检查结果给样例排序。

## 仓库里的完整正文

站内这篇只是导读；规范正文、条款索引与 checker 用法在仓库文档与上游仓库里（链接指向合并后的文档）：

- [规范与检查（docs/dev/standards.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md)
- [规范与检查器作者参考（docs/plugin-dev/standards/README.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md)
- [checker 命令与报告字段（docs/plugin-dev/standards/checker-cli.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/checker-cli.md)
- [证据边界与常见误读（docs/plugin-dev/standards/evidence-and-misreadings.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/evidence-and-misreadings.md)
- [规范与准入门槛的映射（docs/plugin-quality.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [上游：dsh-standards 仓库](https://github.com/PerryLink/dsh-standards) · [npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)
- [上游：dsh-primer（DSH 插件开发指南与工具链）](https://github.com/PerryLink/dsh-primer)
