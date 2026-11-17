---
slug: access-rules
category: access
title: "权限与访问规则速查"
title_en: "Permissions and access rules: a quick reference"
title_es: "Permisos y reglas de acceso: referencia rápida"
title_pt: "Permissões e regras de acesso: referência rápida"
title_hi: "अनुमतियाँ और एक्सेस नियम: त्वरित संदर्भ"
summary: "权限点、字段访问规则、管理约束三层的判断顺序与配置入口。"
summary_en: "How permission codes, field access rules and admin constraints are evaluated, and where to configure them."
summary_es: "Cómo se evalúan los códigos de permiso, las reglas de acceso a campos y las restricciones de administración, y dónde configurarlos."
summary_pt: "Como códigos de permissão, regras de acesso a campos e restrições administrativas são avaliados, e onde configurá-los."
summary_hi: "अनुमति कोड, फ़ील्ड एक्सेस नियम और प्रशासनिक प्रतिबंध किस क्रम में लागू होते हैं, और उन्हें कहाँ कॉन्फ़िगर करें।"
tags: [权限, 字段, 规则]
updated_at: 2026-11-12
status: draft
since: 2026-11-12
lang: zh-CN
---

## 三个层次

dsh-ee 的访问控制分成三层，修改入口都在管理后台，配置完即时生效。

| 层次 | 回答的问题 | 配置入口 | 配置后谁受影响 |
| --- | --- | --- | --- |
| 权限点 | 这个人能不能用这个功能 | 「角色与权限」 | 页面显隐、接口调用 |
| 字段与访问规则 | 这个字段、这条数据能不能看、能不能改 | 「字段与访问规则」 | 文档、知识库、用户等扩展字段 |
| 管理约束 | 修改关键配置要不要审批 | 「字段与访问规则」 | 改字段值、改配置的操作 |

## 权限点怎么落到页面和接口

菜单只决定「看不看得见」，真正的边界在接口。页面上的按钮隐藏不构成安全边界。

| 常见权限点 | 作用 | 员工默认有没有 |
| --- | --- | --- |
| `portal.view` | 使用工作台、企业应用、开发文档等员工页面 | 有 |
| `registry.publish` | 发布插件、技能、提示词与应用 | 有 |
| `extensions.develop` | 拿到扩展开发技能与命令说明 | 没有，需要管理员授予 |
| `plugins.manage` | 审核、上下架能力 | 没有 |
| `connections.manage` | 接入与配置企业连接 | 没有 |

权限点由代码内置目录同步进数据库；新版本新增的权限点会自动授予「员工」角色（如果目录里标了默认角色）。

## 访问规则的判断顺序

1. 超级管理员：不受规则限制（除非系统关闭了超级管理员）。
2. 角色权限：先判断这个功能本身是否可见。
3. 字段访问规则：逐字段判断 read / write / deny，规则可以带条件。
4. 管理约束：写操作是否要先提交变更、由谁审批。

排在前面的层得到结论后，后面的层不会把结论放宽。

## 配置入口与示例

| 我想 | 去哪里 |
| --- | --- |
| 给某类员工开放扩展开发资料 | 「角色与权限」→ 授予 `extensions.develop` |
| 让某个字段只有本部门能看 | 「字段与访问规则」→ 新建 read 规则，条件选部门 |
| 文档里的「机密」字段禁止导出 | 同上，加一条 deny 规则并检查接口返回 |
| 改关键字段要先审批 | 「字段与访问规则」→ 管理约束 → 绑定变更申请 |

## 写扩展时怎么配合

- 清单里声明需要的权限点与凭据名，不把凭据写进代码或清单。
- 字段的读写规则由管理员配置，扩展只按接口返回的可见字段处理，不自己猜测权限。
- 错误码区分「没权限」「不存在」「参数错」，不要把它们都返回成 500。

## 仓库里的完整正文

站内这篇只是速查；规则细节、扩展安装审批与权限点目录在仓库文档里（链接指向合并后的文档）：

- [访问控制设计（docs/access-control.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/access-control.md)
- [企业发布、审核与权限（docs/enterprise-publishing.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [扩展协议 v1 概览·凭据与写操作（docs/extension-protocol/README.md，合并后可用）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [扩展协议正文（docs/extension-protocol.md）](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [权限配置技能（extensions/skills/dsh-ee-access-config/SKILL.md）](https://github.com/hikariming/dsh-ee/blob/main/extensions/skills/dsh-ee-access-config/SKILL.md)
