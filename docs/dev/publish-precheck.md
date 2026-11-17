---
slug: publish-precheck
category: publish
title: "发布入口与预检结果怎么读"
title_en: "Publish entries and how to read the precheck"
title_es: "Entradas de publicación y cómo leer la verificación previa"
title_pt: "Entradas de publicação e como ler a pré-verificação"
title_hi: "प्रकाशन प्रवेश और प्रीचेक परिणाम कैसे पढ़ें"
summary: "三个发布入口、预检的扫描键、结果语义与提交前自查清单。"
summary_en: "The three publish entries, precheck scan keys, what results mean, and a pre-submit checklist."
summary_es: "Las tres entradas de publicación, las claves de escaneo de la verificación previa, el significado de los resultados y una lista de verificación antes de enviar."
summary_pt: "As três entradas de publicação, as chaves de verificação da pré-verificação, o significado dos resultados e uma lista de checagem antes do envio."
summary_hi: "तीन प्रकाशन प्रवेश, प्रीचेक की स्कैन कुंजियाँ, परिणामों का अर्थ और सबमिट से पहले की जाँच सूची।"
tags: [发布, 预检, 版本]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 三个发布入口

| 入口 | 路径 | 适合场景 | 预检何时跑 |
| --- | --- | --- | --- |
| 网页上传 | 工作台「分享插件」 | 第一次发布，想边看预检边改 | 上传后立刻跑 |
| npm 直发 | `npm publish --registry <企业源>` | 已有 CI，按版本流水线发布 | 发布时跑 |
| Agent 接口 | `POST /api/agent/packages` | 自动化发布与批量迁移 | 发布时跑 |

三条入口走同一套准入规则：包先做静态预检，再检查发布冲突，最后按收录模式决定直接上架还是进审核。

## 预检扫描什么

预检只解包做静态检查，**不执行插件代码**，不写库，也不占用版本号。结果分 pass / warn / fail 三级。

| 扫描键 | 它关心的事 | 严重度 |
| --- | --- | --- |
| `bundle` | `dsh.bundle` 与 patch 文件：缺失、路径不安全、文件不在包里 | 插件包是 fail；非插件包缺少时是提醒 |
| `patch` | `cordis.patch.yml` 的结构（数组、`insert`、`id`/`name`、重复 id） | 结构错误是 fail；名称不一致等是提醒 |
| `cordis` | `dependencies` 里是否混进了 Cordis 相关包 | 提醒 |
| `cordis-peer` | `peerDependencies` 的 Cordis 范围是否偏离推荐值 | 提醒 |
| `skills` | 随包分发的技能清单 | 提醒（提示审核会看到） |
| `install-scripts` | 安装期脚本（`preinstall` 等） | 提醒 |
| `secrets` | 包内是否出现疑似密钥 | **fail** |
| `license` | 是否声明 SPDX 许可 | **提醒**（不是阻断） |
| `engines` | 插件是否声明 `engines.dsh` | 提醒 |
| `dependencies` | 非企业作用域的依赖清单 | 信息 |
| `size` | 包体积（超过 20 MB 提醒） | 提醒 |

预检只回答「企业能不能安全地收下这个包」，不做质量评价，也不给插件定级或排序。规范与检查器属于上游工具链，结论是作者的自证材料，不并进企业预检。

## 结果怎么读

预检返回三个结论字段，对应发布接口的真实行为：

| 结论字段 | 含义 |
| --- | --- |
| `can_submit` | 没有发布冲突，发布接口会接受这个包 |
| `will_publish` | 开放收录模式下检查无 fail 项，发布即上架 |
| `requires_review` | 可以提交，但会进入待审核队列 |

真正决定「直接上架还是进审核」的是收录模式与 fail 项：

| 情况 | 白名单模式（默认） | 开放模式 |
| --- | --- | --- |
| 发布冲突（包名 / 版本占用 / 作用域 / 归属 / 同名上游包） | 发布接口拒绝 | 同左 |
| 有 fail 项（如 secrets、bundle 结构错误） | 可以提交，进待审核；**不是拒收** | 可以提交，进待审核 |
| 只有提醒项，或全部 pass | 进待审核 | 无 fail 项，发布即上架 |

版本号在提交成功时就会被占用；被驳回、下架的版本也不能重发，修完请升版本号。

## 提交前自查

1. `pnpm build` 之后再打包，确认 `files` 白名单包含全部产物。
2. `dsh-primer check` 与 `npx dsh-standards@1.6.9 check .` 在本地跑一遍，把结果留在作者仓 CI。
3. 包里不要有 `.tgz`、源码映射、测试夹具里的真实密钥与本机绝对路径。
4. 写清 `license`、`engines.dsh` 与 DSH 版本线，避免安装时才发现不匹配。
5. 需要安装期脚本时，在提交说明里写清脚本做什么。

## 常见问题

| 现象 | 常见原因 | 处理 |
| --- | --- | --- |
| 预检显示发布冲突 | 包名不在企业作用域、版本已存在、同名上游包 | 这些冲突会让发布接口直接拒绝；先改包名或升版本 |
| 预检有 fail 项 | bundle/patch 结构错误，或检出疑似密钥 | 先修再提交；带着 fail 项也能提交，但会进审核且版本号被占用 |
| 提交后进待审核 | 白名单模式，或开放模式下有 fail / 提醒项 | 按审核意见修；不要用新版本号反复试同一个问题 |
| 审核迟迟没有结论 | 安装脚本、依赖或敏感内容需要人工确认 | 在描述里补充说明，或联系管理员 |
| 装到企业后版本不对 | 客户端缓存或指定了旧版本 | 确认 `dsh plugin add <包>@<版本>` 里的版本 |

## 仓库里的完整正文

站内这篇只是导读；发布、审核与权限的完整正文在仓库文档里（链接指向合并后的文档）：

- [DSH 插件发布（docs/dev/plugin-publish.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [企业发布、审核与权限（docs/enterprise-publishing.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [插件作者指南（docs/plugin-authors.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [规范与准入门槛的映射（docs/plugin-quality.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [错误与 FAQ（docs/dev/errors-faq.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
