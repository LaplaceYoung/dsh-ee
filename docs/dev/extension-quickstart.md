---
slug: extension-quickstart
category: extension
title: "写 dsh-ee 扩展：清单、检查与调用"
title_en: "Write a dsh-ee extension: manifest, checks and calls"
title_es: "Escribe una extensión de dsh-ee: manifiesto, comprobaciones y llamadas"
title_pt: "Escreva uma extensão do dsh-ee: manifesto, verificações e chamadas"
title_hi: "dsh-ee एक्सटेंशन लिखें: मैनिफ़ेस्ट, जाँच और कॉल"
summary: "声明式、服务式、本地三种扩展的骨架、CLI 命令与交付前检查。"
summary_en: "Declarative, service and local extensions: scaffold, CLI commands and pre-delivery checks."
summary_es: "Extensiones declarativas, de servicio y locales: esqueleto, comandos de CLI y comprobaciones previas a la entrega."
summary_pt: "Extensões declarativas, de serviço e locais: scaffold, comandos de CLI e verificações antes da entrega."
summary_hi: "डिक्लेरेटिव, सर्विस और लोकल एक्सटेंशन: स्कैफ़ोल्ड, CLI कमांड और डिलीवरी से पहले की जाँच।"
tags: [扩展, 清单, CLI]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 四种形态

dsh-ee 扩展把企业系统接进来。先选形态：前三种用清单交付，第四种是 dsh-ee 自己实现的官方集成，作者不用写清单。

| 形态 | 清单 `kind` | 跑在哪 | 交付物 | dsh-ee 怎么调它 |
| --- | --- | --- | --- | --- |
| 声明式 | `declarative` | dsh-ee 进程内发 HTTP | 清单 + OpenAPI 文档（zip） | 按 OpenAPI 生成工具，直接请求源系统 |
| 服务 | `service` | 企业自己部署的 HTTP 服务 | 一个实现了清单和约定接口的服务 | 取 `/.well-known/dsh-ee-extension.json`；工具走 `POST /mcp` |
| 端侧 | `local` | 员工电脑上的 npm 包 | npm 包 + 清单 | Agent 直连本机 MCP；写操作前问 dsh-ee 确认，调用后回报摘要 |
| 内置 | （无清单） | dsh-ee 进程内 Go | 随 dsh-ee 升级 | 与服务扩展同一套内部接口，不走网络（如飞书文档知识源） |

清单 schema 只认 `declarative` / `service` / `local` 三种 `kind`；「内置」由 dsh-ee 自己实现，不能通过清单交付。服务端不运行第三方代码：声明式由 dsh-ee 发请求，服务扩展在企业自己的服务里，端侧在员工电脑上。

## 起一个骨架

```bash
# 声明式：不写代码，有 OpenAPI 就能接
dsh-ee extension init ./my-ext --id acme/my-ext --kind declarative

# 服务式：要写逻辑，企业自己部署
dsh-ee extension init ./my-service --id acme/my-service --kind service
```

`init` 必须用 `--id 作者/名字` 指定身份；`--kind` 可选 `declarative`（默认）或 `service`。端侧扩展从仓库里的 [`extensions/official/legacy-local`](https://github.com/hikariming/dsh-ee/blob/main/extensions/official/legacy-local) 模板或示例复制。生成目录里带 `AGENTS.md`，用 Agent 打开就知道下一步。

| 命令 | 用途 | 在本机做什么 |
| --- | --- | --- |
| `dsh-ee extension init` | 从示例复制一个扩展 | 写文件 |
| `dsh-ee schema list` / `schema get manifest` | 离线查清单、错误码等规格 | 只读 |
| `dsh-ee extension check` | 静态检查清单、接口文档、只读标记、凭据写法 | 只读检查，不执行代码 |
| `dsh-ee extension test` | 把服务扩展跑起来按协议逐项测 | 启动本地进程 |
| `dsh-ee extension dev` | 起一个模拟的 dsh-ee（不需要数据库） | 启动本地进程 |
| `dsh-ee extension call` | 以某位员工身份调一次扩展工具 | 只读诊断 |
| `dsh-ee extension pack` | 把**声明式**扩展打成 zip 并算校验和 | 写 zip |
| `dsh-ee extension report` | 生成给管理员看的交付说明 | 写文件 |

`check` 有错误时退出码为 1，每条结果都带 `code`、`severity`、`where`、`message`、`fix`。以上命令都在作者机器或作者仓 CI 里执行；dsh-ee 服务端不执行第三方代码，也不在网页里运行检查器。

## 清单最小示例

下面的清单与 `dsh-ee extension init --kind declarative` 生成的形状一致，配一份 `servers` 覆盖得到的 `openapi.yaml` 后，`dsh-ee extension check` 结果是 0 错误、0 提醒。字段以 `dsh-ee schema get manifest` 为准；清单是严格 schema，多写一个规格里没有的字段也会报错。

```json
{
  "id": "acme/oa",
  "version": "0.1.0",
  "name": "Acme OA",
  "description": "查询和提交 Acme OA 的审批单。",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": ["https://oa.acme.internal"],
  "credential": { "mode": "ee_jwt" },
  "tools": [
    {
      "name": "list_items",
      "readOnly": true,
      "description": "列出当前员工的条目",
      "keep": ["items.id", "items.title", "total"]
    },
    {
      "name": "create_item",
      "description": "新建一个条目。这是写操作，调用前要员工确认。"
    }
  ]
}
```

几个容易写错的点：

- `id` 要写成「作者/名字」，只用小写字母、数字和连字符；`version` 是严格语义化版本。
- 声明式必须有 `openapi`（扩展目录里的相对路径）和至少一个 `servers`；OpenAPI 文档里的 `servers[0].url` 必须落在清单的 `servers` 之内。
- `tools[].name` 对应 OpenAPI 每个接口的 `operationId`；查询类工具标 `readOnly`，写操作不要标。
- 凭据模式（`credential.mode`）只在调用时需要：`shared` / `user_secret` / `ee_jwt` / `oauth`，按对方能接受的方式选。

## 交付前先确认

| 检查项 | 在哪里确认 |
| --- | --- |
| 清单字段与协议版本 | `dsh-ee schema get manifest`、`dsh-ee extension check --json` |
| 接口文档与工具映射 | `check` 的 `tools` 结果；写操作是否标了 `readOnly` |
| 凭据只从 dsh-ee 取，不写进清单 | 清单与代码里不出现明文 key |
| 服务接口的协议一致性 | `dsh-ee extension test` 的用例 |
| 声明式打包 | `dsh-ee extension pack` 产出 zip；服务扩展交付代码仓库或镜像 |
| 谁能用哪些工具 | 管理员在「企业连接」「字段与访问规则」里配置 |

## 与 DSH 技能的关系

为了给 Agent 一份可执行的说明，dsh-ee 把扩展开发流程写成了技能（`dsh-ee-extension-dev`），按「开发扩展」（`extensions.develop`）权限下发给 DSH。人和 Agent 看到的是同一套步骤，页面负责给人读，技能负责给 Agent 执行。

## 仓库里的完整正文

站内这篇只是导读；协议细节、安装审核与错误码以仓库文档为准（链接指向合并后的文档）：

- [扩展开发（docs/dev/extensions.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/extensions.md)
- [扩展协议 v1 概览（docs/extension-protocol/README.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [扩展协议正文（docs/extension-protocol.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [企业发布、审核与权限（docs/enterprise-publishing.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [扩展目录与示例索引（extensions/README.md）](https://github.com/hikariming/dsh-ee/blob/main/extensions/README.md)
- [清单 JSON Schema（extensions/schemas/manifest.schema.json）](https://github.com/hikariming/dsh-ee/blob/main/extensions/schemas/manifest.schema.json)
- [设计取舍（docs/plugins.md，设计稿，不是使用文档）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugins.md)
