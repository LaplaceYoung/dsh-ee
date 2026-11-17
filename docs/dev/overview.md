---
slug: overview
category: start
title: "从哪开始：三条开发路径"
title_en: "Where to start: three development paths"
title_es: "Por dónde empezar: tres rutas de desarrollo"
title_pt: "Por onde começar: três caminhos de desenvolvimento"
title_hi: "शुरुआत कहाँ से करें: विकास के तीन मार्ग"
summary: "插件作者、扩展作者、非 DSH 客户端三条路径的入口、交付物与本系统里的落点。"
summary_en: "Entry points, deliverables and where each one lands in dsh-ee: plugins, extensions, non-DSH clients."
summary_es: "Puntos de entrada, entregables y dónde encaja cada ruta en dsh-ee: plugins, extensiones y clientes que no son DSH."
summary_pt: "Pontos de entrada, entregáveis e onde cada caminho se encaixa no dsh-ee: plugins, extensões e clientes que não são DSH."
summary_hi: "प्रवेश बिंदु, डिलीवरेबल और dsh-ee में हर मार्ग का स्थान: प्लगइन, एक्सटेंशन और गैर-DSH क्लाइंट।"
tags: [入口, 插件, 扩展, 客户端]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 三条路径

dsh-ee 的「开发」按交付物分三条路径：写 DSH 插件、写 dsh-ee 扩展、接非 DSH 客户端。它们共用同一个企业身份与令牌，但代码、检查与发布入口不同。

| 路径 | 你交付什么 | 在本系统的落点 | 起步工具 |
| --- | --- | --- | --- |
| DSH 插件作者 | 一个 DSH 插件包（工具、客户端、服务、命令等） | 企业 npm 源、发布预检、审核、下发 | 先 `dsh plugin add dsh-primer`，再用 `dsh-primer new` 起骨架、`dsh-primer check` 自查 |
| dsh-ee 扩展作者 | 一个扩展：声明式 / 服务 / 端侧（内置由 dsh-ee 自己实现） | 企业连接、字段与访问规则、Agent 技能 | `dsh-ee extension init` 起骨架，`dsh-ee extension check` 自查 |
| 非 DSH 客户端 | 一个用 HTTP 调用模型与 MCP 的客户端 | 我的令牌、模型网关、`/v1`、`/mcp` 与发现端点 | 先在工作台「我的令牌」里创建令牌，路径见[接第三方 Harness](./harness-quickstart.md) |

扩展本身有四种形态（声明式 / 服务 / 端侧 / 内置），清单里只能写前三种 `kind`；细节见[写 dsh-ee 扩展](./extension-quickstart.md)。

## 先分清「插件」与「扩展」

**DSH 插件**装在 DSH 客户端里，由作者在自己的机器上构建与验证；dsh-ee 负责企业侧的准入、托管与下发。

**dsh-ee 扩展**是接入 dsh-ee 服务端的集成单元，由 dsh-ee 自己的 CLI 管理，管理员在「企业连接」「字段与访问规则」里配置。

| 对比项 | DSH 插件 | dsh-ee 扩展 |
| --- | --- | --- |
| 运行位置 | DSH 客户端所在机器 | dsh-ee 进程内发请求（声明式）、企业自己的服务（服务）、员工电脑（端侧） |
| 清单文件 | `package.json` 与 `cordis.patch.yml` | `dsh-ee.extension.json` |
| 发布入口 | 企业 npm 源，或工作台「分享插件」 | 声明式上传 zip；服务扩展由管理员填服务地址；端侧交付 npm 包与清单 |
| 谁能安装 | 有「安装能力」权限的员工 | 由管理员在连接与规则里配置 |

## 建议阅读顺序

1. 第一次发插件：[写 DSH 插件并发布到企业源](./plugin-quickstart.md)
2. 第一次写扩展：[写 dsh-ee 扩展：清单、检查与调用](./extension-quickstart.md)
3. 要接第三方 Harness 或自研客户端：[接第三方 Harness / 自研客户端](./harness-quickstart.md)
4. 要确认权限边界：[权限与访问规则速查](./access-rules.md)
5. 提交前想先知道会被问什么：[发布入口与预检结果怎么读](./publish-precheck.md)
6. 想弄清规范和检查的边界：[规范与检查：ENTRY / FLOW / BOOST](./quality-standards.md)

## 还要准备什么

| 事项 | 说明 |
| --- | --- |
| 账号与权限 | 发布插件需要「发布能力」（`registry.publish`）；扩展开发资料需要「开发扩展」（`extensions.develop`） |
| 企业源地址 | 从「接入指南」获取 npm 源地址、令牌配置与 `setup.sh` 用法 |
| 本地工具链 | Node ≥24.9（`dsh-primer`、`dsh-standards` 的 engines 要求）与 pnpm；只有写服务式扩展才需要 Go 1.27+ |
| 网络 | 内网部署时 GitHub 与公网 npm 可能不可达，先确认能取到依赖，必要时走「上游预载」 |

## 站内导读与仓库正文

站内页面只写 dsh-ee 视角的摘要与入口，不复制上游正文；仓库文档可以按各自许可引用或改编（CC BY 4.0 保留 TASL 与 Change Notice，Apache-2.0 保留 LICENSE/NOTICE），正文、许可与上游版本以仓库文档和上游仓库为准（链接指向合并后的文档）：

| 内容 | 在哪里 |
| --- | --- |
| 开发文档索引（快速开始 / 扩展 / Harness / 发布 / 规范 / 错误与 FAQ） | [docs/dev/README.md（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/README.md) |
| DSH 插件开发指南（社区指南的本地参考层） | [docs/plugin-dev/README.md（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md) |
| 规范与检查器作者参考（ENTRY / FLOW / BOOST、DS-C、checker） | [docs/plugin-dev/standards/README.md（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) |
| 扩展协议 v1 概览 | [docs/extension-protocol/README.md（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md) |
| 企业发布、审核与权限 | [docs/enterprise-publishing.md（合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md) |

上游工具固定版本：`dsh-primer`（[npm 包页](https://www.npmjs.com/package/dsh-primer)）与 `dsh-standards@1.6.9`（[npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)）；dsh-ee 只固定版本并给出用法，不复刻上游的检查逻辑。
