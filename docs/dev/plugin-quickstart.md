---
slug: plugin-quickstart
category: plugin
title: "写 DSH 插件并发布到企业源"
title_en: "Write a DSH plugin and publish it to the company registry"
title_es: "Escribe un plugin de DSH y publícalo en el registro de la empresa"
title_pt: "Escreva um plugin DSH e publique-o no registro da empresa"
title_hi: "DSH प्लगइन लिखें और कंपनी रजिस्ट्री में प्रकाशित करें"
summary: "从骨架、清单字段、本地自查到三个发布入口；只讲企业侧必看的部分。"
summary_en: "From scaffold and manifest fields to local checks and three publish entries — only what the company side needs."
summary_es: "Desde el esqueleto y los campos del manifiesto hasta las comprobaciones locales y las tres entradas de publicación: solo lo esencial del lado de la empresa."
summary_pt: "Do scaffold e dos campos do manifesto às verificações locais e às três entradas de publicação: apenas o essencial do lado da empresa."
summary_hi: "स्कैफ़ोल्ड और मैनिफ़ेस्ट फ़ील्ड से लेकर स्थानीय जाँच और तीन प्रकाशन प्रवेशों तक — केवल कंपनी पक्ष के लिए ज़रूरी बातें।"
tags: [插件, package.json, 发布]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 前置条件

| 依赖 | 版本 | 为什么需要 |
| --- | --- | --- |
| Node | ≥ 24.9.0 | `dsh-primer` 与 `dsh-standards` 的 `engines` 都要求 ≥24.9；DSH 0.2.2 线也要求 Node ≥24.9，更早的 0.2.x 线可跑在 22.19+ |
| DSH | 0.2.x 线（0.2.1–0.2.2） | 运行插件与 `dsh plugin add`；`dsh-primer` 声明支持 `>=0.2.1-0 <0.3.0`，`engines.dsh` 按实际验证过的范围填写 |
| pnpm | 任意近期版本 | 安装依赖与打包 |
| dsh-primer | 0.4.x | 骨架、静态检查与安装冒烟（旧名 `dsh-plugin-guide` 0.3.x，CLI 旧别名 `dsh-plugin-dev`） |
| 企业账号 | 有「发布能力」（`registry.publish`） | 上传插件、申请引入社区插件 |

## 五分钟起一个骨架

```bash
# 0. dsh-primer 是可安装的 DSH bundle，不是 DSH 自带命令；先装进你用的 profile
dsh plugin --profile web add dsh-primer

# 1. 生成骨架（上游模板，TypeScript）
dsh-primer new my-plugin

# 2. 本地装依赖并跑通
cd my-plugin
pnpm install
pnpm build

# 3. 自查包形状（静态检查，在本地执行）
dsh-primer check

# 4. 装进一个测试 profile 验证加载/卸载
dsh-primer verify
```

`dsh-primer` 属于上游工具链，dsh-ee 只固定版本并给出用法，不复刻它的检查逻辑。

## 清单里企业最关心的字段

| 字段 | 写什么 | 企业侧为什么要看 |
| --- | --- | --- |
| `name` | 企业 scope 下的包名，例如 `@company/hello` | 决定能否路由到企业 npm 源 |
| `version` | 语义化版本，一次发布一个版本 | 版本号一旦占用不会回退；发布冲突会拒绝提交 |
| `main` / `files` | 真正要发布的入口与文件白名单 | 避免把源码、测试、`.tgz` 一起发出去 |
| `cordis.patch.yml` | 插件对 Cordis 的补丁声明 | 预检会检查 bundle 与 patch 的结构 |
| `dsh.bundle` | DSH 插件清单（如有） | 决定客户端能否识别这个包 |
| `engines.dsh` | 实际验证过的 DSH 版本范围 | 与企业的 DSH 版本线核对；没声明会得到提醒 |
| `peerDependencies` | DSH 相关 peer | 预检会检查 peer 范围是否可解析 |
| `license` | `Apache-2.0` 等 SPDX 标识 | 缺少 license 是预检的**提醒**项（不阻断），但会进人工审核 |

## 本地自查都查什么

| 命令 | 查什么 | 在哪跑 |
| --- | --- | --- |
| `dsh-primer check` | 包形状与入口是否齐全 | 作者机器或作者仓 CI |
| `dsh-primer verify` | 安装、启用、卸载的冒烟 | 作者机器或作者仓 CI |
| `npx dsh-standards@1.6.9 check .` | 上游规范条款的静态证据（要求 Node ≥24.9） | 作者机器或作者仓 CI，结论是作者自证材料 |

dsh-ee 的服务端不执行插件代码，也不执行这些检查器；网页上的「分享插件」预检只看包本身的静态形状与企业准入条件。

## 发布到企业源

| 入口 | 怎么做 | 适合谁 |
| --- | --- | --- |
| 工作台「分享插件」 | 上传 `.tgz`，先看预检结果再提交 | 第一次发布、想先看预检 |
| npm 直发 | `npm publish --registry <企业源>`，令牌在「我的令牌」创建 | 已有 CI 流水线 |
| Agent 接口 | `POST /api/agent/packages`，用企业令牌调用 | 自动化发布 |

版本号在审核与上架期间会被占用；重复版本或冲突会被拒绝。预检结论有三项：`can_submit`（没有发布冲突，发布接口会接受）、`will_publish`（开放收录模式下检查无 fail 项，发布即上架）、`requires_review`（提交后进审核）。

## 常见问题

| 现象 | 常见原因 | 处理 |
| --- | --- | --- |
| 预检提示缺入口文件 | `files` 白名单没包含构建产物，或 `main` 指向未构建的文件 | 先 `pnpm build`，再检查 `files` |
| 预检有 fail 项 | bundle / patch 结构错误、检出疑似密钥等 | 先修再提交；带着 fail 项也能提交，但版本号会被占用，审核通常也过不了 |
| 预检只有提醒项 | 缺 license、未声明 `engines.dsh`、有安装脚本等 | 不阻断：白名单模式进人工审核，开放模式可直接上架 |
| peer 范围无法解析 | 同时声明了两个不兼容的 DSH 版本线 | 只保留一条版本线，或补齐可解析的范围 |
| 安装时报 Node 版本错 | `engines.node` 和员工电脑的 Node 不匹配 | 按实际验证结果修正 `engines.node`（示例：`^22.19.0 \|\| >=24.9.0`） |
| 企业里装不上 | 令牌权限不足或 npmrc 未配置企业源 | 检查「安装能力」权限与 `.npmrc` |

## 仓库里的完整正文

站内这篇只是导读；完整正文与许可在仓库文档里（链接指向合并后的文档）：

- [快速开始（docs/dev/quickstart.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/quickstart.md)
- [DSH 插件发布（docs/dev/plugin-publish.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [错误与 FAQ（docs/dev/errors-faq.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
- [插件作者指南（docs/plugin-authors.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [DSH 插件开发指南（docs/plugin-dev/README.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md)
- [上游：dsh-primer 仓库](https://github.com/PerryLink/dsh-primer) · [npm 包页](https://www.npmjs.com/package/dsh-primer)
- [上游：dsh-plugin-guide（0.3.x 旧名指向页）](https://github.com/PerryLink/dsh-plugin-guide)
