---
title: 开发文档索引与站内 /docs 生成管线
status: draft
since: 2026-11-12
lang: zh-CN
---

# dsh-ee 开发文档

本目录是 dsh-ee 面向使用者与开发者的文档正文来源（单一事实源）。`docs/` 下的其它文件大多是设计稿、评审记录和交付报告，不是这里的使用文档；`docs/plugin-dev/` 是按许可改编自社区指南/规范的上游参考层（来源与署名见 [SOURCES](../plugin-dev/SOURCES.md)），两者不要混读。

站内「开发文档」页面（`/docs`、`/docs/:slug`）也从本目录取正文：构建期由 `scripts/docs/build.mjs` 按发布白名单生成 `web/src/generated/dev-docs.ts`，页面不再放单独的内容副本。

文档状态：`draft` 待评审 · `review` 评审中 · `stable` 已随版本核实。当前各篇均为 `draft`，内容按仓库当前代码整理，接口在 v0.x 仍可能变化。

## 按角色读

| 你要做的事 | 从这里开始 | 接着读 |
|---|---|---|
| 把内部 DSH 插件发布到 dsh-ee | [快速开始](./quickstart.md) | [DSH 插件发布](./plugin-publish.md) |
| 给企业系统写扩展（声明式 / 服务 / 端侧） | [扩展开发](./extensions.md) | [extension-protocol.md](../extension-protocol.md)、[plugins.md](../plugins.md) |
| 让第三方 Harness / 自研客户端接入 dsh-ee | [Harness 接入](./harness.md) | [l1-client.md](../l1-client.md) |
| 用 DeepSeek Chatbot 能力（联网工具 / 内容安全 / 会话标题） | [Chatbot 网关选项与透传](./chatbot.md) | [Web Tools](https://api-docs.deepseek.com/guides/web_tools.md) |
| 隔一层看 dsh-ee 侧的三条路径与阅读顺序 | [从哪开始](./overview.md) | [权限与访问规则速查](./access-rules.md) |
| 搞清 ENTRY / FLOW / BOOST 是什么、不是什么 | [规范与检查](./standards.md) | [plugin-quality.md](../plugin-quality.md) |
| 查错误码和常见失败 | [错误与 FAQ](./errors-faq.md) | — |

## 站内 `/docs` 页面（发布白名单）

哪些文档上站内页面由 [`scripts/docs/manifest.json`](../../scripts/docs/manifest.json) 的 `docs` 数组决定（发布白名单，不在名单里的文件不会上站内页面）；正文、标题、摘要、分类、标签都只在本目录维护。

| 文件 | 站内分类 | 内容 |
|---|---|---|
| [overview.md](./overview.md) | 开始 | 三条开发路径的入口与阅读顺序 |
| [harness-quickstart.md](./harness-quickstart.md) | Harness 接入 | 一个 `ee_at_` 令牌接 `/v1`、`/mcp` 与写确认 |
| [anthropic-api.md](./anthropic-api.md) | Harness 接入 | Claude Code / Anthropic SDK 调 `POST /v1/messages`：鉴权、转换、流式与错误壳 |
| [plugin-quickstart.md](./plugin-quickstart.md) | DSH 插件 | 插件骨架、清单、自查与发布入口 |
| [extension-quickstart.md](./extension-quickstart.md) | dsh-ee 扩展 | 三种扩展形态、CLI 与交付前检查 |
| [access-rules.md](./access-rules.md) | 权限 | 权限点、字段访问规则、管理约束速查 |
| [publish-precheck.md](./publish-precheck.md) | 发布 | 发布入口、预检结果语义与自查清单 |
| [quality-standards.md](./quality-standards.md) | 规范 | 上游规范三层与 dsh-ee 预检的边界 |
| [chatbot.md](./chatbot.md) | Harness 接入 | DeepSeek Chatbot 的联网工具、内容安全与会话标题：注入开关、透传规则与能力声明 |

站内页面还会做标题、摘要、标签与正文的客户端全文搜索（中文两字词直接子串匹配，多词要求全部命中；`⌘K` 里也有一组「文档」），逻辑见 `web/src/data/dev-docs-search.ts`，不需要另建搜索索引。

构建期同时生成 `web/public/llms.txt`：给 Agent 的入口（站内文档清单 + 发现/模型/MCP/错误码等契约链接），部署后 `/llms.txt` 匿名可读。契约入口在 [`scripts/docs/manifest.json`](../../scripts/docs/manifest.json) 的 `llms.contracts` 里配置：站内路径写 `url`，仓库文档写 `docs`（生成器会检查该文件存在，避免 llms.txt 断链）；`repository` 决定「查看源文件」和仓库契约链接指向的仓库。

站内页面用的 front matter 字段：`slug`（站内路由 `/docs/<slug>`，只允许小写字母、数字、连字符）、`category`（`start` / `harness` / `plugin` / `extension` / `access` / `publish` / `quality`）、`title`、`title_en`、`summary`、`summary_en`、`tags`（可选，用于搜索）、`updated_at`（可选，`YYYY-MM-DD`，展示在列表与详情页）。整块 front matter 是可选的：没有时 slug 取文件名、标题取正文第一个一/二/三级标题、摘要取第一段、分类用白名单里的 `defaultCategory`；但站内页面建议写全。

新增或修改一篇站内文档的流程：

1. 在 `docs/dev/` 新建 Markdown 或在已有文件里改正文，按上面的字段补 front matter（每篇仍必须有 `title` / `status` / `since` / `lang`，见维护规则）。
2. 在 [`scripts/docs/manifest.json`](../../scripts/docs/manifest.json) 的 `docs` 数组里登记相对 `docs/` 的路径。
3. 重新生成生成物并提交：`cd web && pnpm docs:build`（`pnpm build` 会经 prebuild 自动跑；一次生成 `web/src/generated/dev-docs.ts` 与 `web/public/llms.txt` 两个文件）。
4. 本地自查：`cd web && pnpm docs:check && pnpm docs:test`（CI 的 web job 也会跑这两条）。

## 文档清单

| 文件 | 内容 | 状态 |
|---|---|---|
| [quickstart.md](./quickstart.md) | 5 分钟判断该走哪条路径；本地起 dsh-ee 与常用命令 | draft |
| [extensions.md](./extensions.md) | dsh-ee 扩展的四种形态、SDK、开发与安装流程 | draft |
| [harness.md](./harness.md) | 非 DSH 客户端（L1）的令牌、模型与 MCP 接入；DSH 插件接入 | draft |
| [plugin-publish.md](./plugin-publish.md) | DSH 插件发布到 dsh-ee：清单、预检、发布、审核 | draft |
| [standards.md](./standards.md) | 上游三层标准的边界；dsh-ee 只做映射与链接 | draft |
| [errors-faq.md](./errors-faq.md) | 发布、扩展、接入常见错误与 FAQ | draft |
| [overview.md](./overview.md) | 站内导读：三条开发路径、插件与扩展的区别、阅读顺序 | draft |
| [harness-quickstart.md](./harness-quickstart.md) | 站内导读：第三方 Harness / 自研客户端的五分钟路径与限制 | draft |
| [anthropic-api.md](./anthropic-api.md) | Anthropic Messages 兼容端点：Claude Code 接入、请求/响应/SSE 转换、错误与 v0 边界 | draft |
| [plugin-quickstart.md](./plugin-quickstart.md) | 站内导读：插件骨架、清单字段、本地自查与发布入口 | draft |
| [extension-quickstart.md](./extension-quickstart.md) | 站内导读：扩展清单、CLI 命令与交付前检查 | draft |
| [access-rules.md](./access-rules.md) | 站内导读：权限点、字段访问规则、管理约束速查 | draft |
| [publish-precheck.md](./publish-precheck.md) | 站内导读：发布入口与预检结果语义 | draft |
| [quality-standards.md](./quality-standards.md) | 站内导读：上游规范三层与 dsh-ee 预检的边界 | draft |
| [chatbot.md](./chatbot.md) | Chatbot 网关选项与透传：注入开关、请求/响应透传、能力声明与边界 | draft |

## 术语表

| 术语 | 一句话定义 |
|---|---|
| DSH 插件 | cordis 插件（npm 包），dsh-ee 的发布对象；清单里用 `dsh.bundle` / `dsh.client` 声明形态 |
| 企业包 | 发布到企业 npm 作用域（如 `@company/...`）、经 dsh-ee 审核后上架的 DSH 插件 |
| dsh-ee 扩展 | 把企业系统接进来的能力单元，分声明式、服务、内置（`server/` 内）与端侧四种；见 [plugins.md](../plugins.md) §4、§8 |
| 端侧扩展 | 跑在员工电脑上的扩展（npm 包 + 本机 MCP 服务），见 [extension-local.md](../extension-local.md) |
| Harness | 调用 dsh-ee 的客户端运行时：DSH、Claude Code、Codex、自研 Agent 等 |
| ENTRY / FLOW / BOOST | 上游三条采用深度轨道，不是 dsh-ee 的等级、评分或认证；见 [plugin-quality.md](../plugin-quality.md) |
| 预检 | dsh-ee 对 `.tgz` 的静态检查（`POST /api/me/packages/check`），不执行插件代码，不等于任何层级判定 |

## 来源与许可

- 本目录文档为 dsh-ee 原创，随仓库按 Apache-2.0 发布；引用上游材料只给固定版本的链接，不复制正文、图片或条款。
- 上游来源、版本、commit、许可与修改说明：[plugin-dev/SOURCES.md](../plugin-dev/SOURCES.md)；仓库根许可声明：[NOTICE](../../NOTICE)。
- 文档里的外部链接按「版本 + commit」固定，不写 `main`；上游发版或改名时先更新 SOURCES。
- 站内页面与仓库正文同源：生成物 `web/src/generated/dev-docs.ts` 只由本目录生成，不复制上游正文或图片。

## 维护规则

- 每篇必须有 front matter：`title`、`status`、`since`、`lang`；新增文档要同时在上面的「站内 `/docs` 页面」表与「文档清单」里登记。
- 相对链接必须能解析、锚点必须存在；pin 表行必须完整。本地运行 `node scripts/check-docs.mjs` 检查，CI 的 `docs` job 也会跑。
- 站内发布（白名单）文档改动后要重新生成并提交：`cd web && pnpm docs:build`；CI 用 `node scripts/docs/build.mjs --check` 校验 `dev-docs.ts` 与 `llms.txt` 两个生成物，改了文档没重新生成会失败。发布集内的相对 `.md` 链接在生成时改写成站内路由 `/docs/<slug>`，源文件保持 GitHub 可读。
- 新增/调整 llms.txt 的契约入口时改 `manifest.json` 的 `llms.contracts`，并确认引用的 `docs/` 文件存在（生成器会报错）。详情页的「查看源文件」链接由生成物的 `source` 字段给出，不需要手写。
- 没有生成物时站内页面回退到旧静态副本（`web/src/data/dev-docs-fallback.ts`），仅作兜底、不再新增内容。
- 本目录只写 dsh-ee 视角的摘要与外链，不复制上游正文；仓库内按许可改编的上游参考统一放 `docs/plugin-dev/`，来源与署名登记在 SOURCES。
