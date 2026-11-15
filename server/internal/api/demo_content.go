package api

import (
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例内容本身：知识库正文、演示差旅系统的接口文档、技能、提示词与微应用。见 demo.go。

const (
	auditDemoImport   = "demo.import"
	demoMonthlyTokens = 1_000_000
)

type demoKDoc struct {
	Title, Folder, Markdown string
}

type demoKBase struct {
	Slug, Title, Description string
	Docs                     []demoKDoc
}

type demoUser struct {
	Username, DisplayName, Department string
	// 职务，只在导入结果与设定文档里出现
	Title string
}

type demoAsset struct {
	Kind, Slug, Delivery string
	Spec                 assetSpec
	// 连接：导入后打开的写工具
	EnableTools []string
	// 可用范围只给这几个部门；空表示全公司
	Departments []string
}

func demoAssets() []demoAsset {
	invocable := true
	return append([]demoAsset{
		{
			Kind: store.KindConnection, Slug: demoConn, Delivery: store.DeliveryAuto,
			EnableTools: []string{"create_claim"},
			Spec: assetSpec{
				Title:        "差旅系统（示例）",
				Description:  "dsh-ee 自带的演示接口：本月报销汇总、报销单列表、提交报销单。数据在内存里，重启后复原。",
				Type:         "openapi",
				OpenAPI:      demoTravelOpenAPI,
				Auth:         &connAuth{Type: "header", Header: demoKeyHeader},
				Instructions: "这是演示系统。提交报销单前先把单据内容给用户确认。",
			},
		},
		{
			Kind: store.KindConnection, Slug: demoCRMConn, Delivery: store.DeliveryAuto,
			EnableTools: []string{"log_visit"},
			Spec: assetSpec{
				Title:        "CRM 客户系统（示例）",
				Description:  "星河科技的客户、合同、拜访记录与续约日期，约 50 个客户。数据在内存里，重启后复原。",
				Type:         "openapi",
				OpenAPI:      demoCRMOpenAPI,
				Auth:         &connAuth{Type: "header", Header: demoKeyHeader},
				Instructions: "这是演示系统。记录拜访前先把纪要给用户确认。金额单位是元。",
			},
		},
		{
			Kind: store.KindConnection, Slug: demoITSMConn, Delivery: store.DeliveryAuto,
			EnableTools: []string{"create_ticket"},
			Spec: assetSpec{
				Title:        "IT 工单系统（示例）",
				Description:  "星河科技 IT 部的工单与常见故障分类，能新建工单。数据在内存里，重启后复原。",
				Type:         "openapi",
				OpenAPI:      demoITSMOpenAPI,
				Auth:         &connAuth{Type: "header", Header: demoKeyHeader},
				Instructions: "这是演示系统。先按 IT 排障手册帮用户自己排查，解决不了再建工单；建工单前把标题、分类、优先级给用户确认。",
			},
		},
		{
			Kind: store.KindSkill, Slug: "meeting-minutes", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Title:          "会议纪要整理（示例）",
				Description:    "把会议记录、录音转写整理成结构化纪要：结论、待办（负责人 + 截止时间）、遗留问题。",
				WhenToUse:      "用户给了一段会议记录、聊天记录或转写文字，想要纪要、待办或会议总结时",
				ModelInvocable: &invocable,
				Body:           demoMinutesSkill,
			},
		},
		{
			Kind: store.KindPrompt, Slug: "weekly-report", Delivery: store.DeliveryOptional,
			Spec: assetSpec{
				Title:       "写周报（示例）",
				Description: "按「本周完成 / 下周计划 / 风险与求助」三段写周报，先问清楚再写。",
				Body:        demoWeeklyPrompt,
			},
		},
	}, append(demoSampleApps(), demoExpertAssets()...)...)
}

const demoMinutesSkill = `# 会议纪要整理

把用户给的会议记录整理成下面的格式，不要编造记录里没有的内容。

## 输出格式

**会议**：主题（日期、参会人，记录里有才写）

**结论**
1. ……

**待办**

| 事项 | 负责人 | 截止时间 |
|---|---|---|

记录里没说负责人或时间的，写「待定」，并在最后提醒用户补上。

**遗留问题**
- ……

## 要求

- 结论只写会上明确定下来的事；还在讨论的放进「遗留问题」。
- 同一件事说了好几遍的，合并成一条。
- 口语改成书面语，但数字、日期、人名照原样写。`

const demoWeeklyPrompt = `帮我写这周的周报。

先问我三个问题（一次问完）：这周做了哪些事、下周打算做什么、有没有风险或需要别人帮忙的地方。我回答后，按下面的格式写：

**本周完成**
- 每条一句话，先说结果，再说做了什么；有数字的带上数字

**下周计划**
- 每条带预计完成时间

**风险与求助**
- 没有就写「无」

总长度不超过 300 字，不要客套话。`

const demoTravelOpenAPI = `{
  "openapi": "3.0.3",
  "info": { "title": "差旅系统（示例）", "version": "1.0.0" },
  "paths": {
    "/demo/travel/summary": {
      "get": {
        "operationId": "get_summary",
        "summary": "本月差旅报销汇总：已报金额、单数、待审批数、待处理的异常单据数、部门预算与剩余预算"
      }
    },
    "/demo/travel/monthly": {
      "get": {
        "operationId": "monthly",
        "summary": "近 6 个月每月的报销金额与单数（不含驳回的）"
      }
    },
    "/demo/travel/claims": {
      "get": {
        "operationId": "list_claims",
        "summary": "报销单列表，最新的在前。flags 是系统发现的异常（超标、重复发票、缺票据等）",
        "parameters": [
          { "name": "status", "in": "query", "required": false, "description": "按状态过滤：待审批 / 已通过 / 已驳回 / 待补票据，或「异常」只看有异常的", "schema": { "type": "string" } },
          { "name": "applicant", "in": "query", "required": false, "description": "申请人姓名", "schema": { "type": "string" } }
        ]
      },
      "post": {
        "operationId": "create_claim",
        "summary": "提交一张差旅报销单，状态为待审批",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["applicant", "city", "days", "amount"],
                "properties": {
                  "applicant": { "type": "string", "description": "申请人姓名" },
                  "city": { "type": "string", "description": "出差城市" },
                  "days": { "type": "integer", "description": "出差天数" },
                  "amount": { "type": "number", "description": "报销金额（元），按制度算出的可报金额" },
                  "reason": { "type": "string", "description": "出差事由" }
                }
              }
            }
          }
        }
      }
    }
  }
}`

const demoTravelPolicy = `# 差旅费管理办法（示例）

> 本文是 dsh-ee 自带的示例内容，用来演示知识库和工作台，条款和金额都是编的。

## 第一章 总则

第一条 为规范员工因公出差的费用开支，制定本办法。适用于公司全体正式员工、试用期员工和实习生。

第二条 出差须事先在 OA 系统提交出差申请，经直属上级批准后方可出行。未经批准的出差，费用不予报销。

## 第二章 城市分类

第三条 出差目的地按以下三类城市执行不同标准：

| 类别 | 城市 |
|---|---|
| 一类城市 | 北京、上海、广州、深圳 |
| 二类城市 | 杭州、南京、苏州、成都、重庆、武汉、西安、天津、厦门、青岛 |
| 三类城市 | 其他城市 |

## 第三章 住宿费

第四条 住宿费按城市类别和职级实行上限标准，每人每晚：

| 类别 | 普通员工 | 经理及以上 |
|---|---|---|
| 一类城市 | 550 元 | 700 元 |
| 二类城市 | 450 元 | 550 元 |
| 三类城市 | 350 元 | 450 元 |

第五条 住宿晚数按出差天数减一计算（当天往返不报住宿费）。

第六条 实际住宿费低于标准的，按实际金额报销；超过标准的部分由个人承担。

第七条 两名同性别员工同行出差的，原则上合住一间，按一间房的标准报销。

## 第四章 伙食补助与市内交通

第八条 伙食补助按出差自然日计算，包干使用，无需票据：一类城市每天 100 元，二类城市每天 80 元，三类城市每天 60 元。

第九条 市内交通补助每天 50 元，包干使用，无需票据。在出差地乘坐出租车、网约车的费用不再另行报销，有客户接待等特殊情况的除外，需在报销单上说明。

## 第五章 往返交通

第十条 往返交通工具标准：

- 普通员工：高铁、动车二等座；单程乘车时间超过 6 小时的，可乘飞机经济舱。
- 经理及以上：高铁、动车一等座或飞机经济舱。

第十一条 超出标准乘坐交通工具的，按标准票价报销，差额由个人承担。

第十二条 自驾出差需事先批准，按每公里 0.8 元报销，过路费、停车费凭票据另行报销，油费不再另报。

## 第六章 附则

第十三条 本办法由财务部负责解释，自 2026 年 1 月 1 日起施行。`

const demoReimburseFlow = `# 报销流程与票据要求（示例）

> 示例内容，用来演示知识库。

## 一、提交时限

出差结束后 30 天内在费控系统提交报销单。超过 30 天的，需部门负责人写明原因并签字；超过 90 天的不予报销。

## 二、需要的票据

| 费用 | 票据 |
|---|---|
| 住宿费 | 酒店开具的增值税发票（抬头为公司全称和税号）+ 酒店水单 |
| 火车、高铁 | 电子客票报销凭证 |
| 飞机 | 电子客票行程单或航空公司开具的发票 |
| 自驾 | 过路费、停车费发票，以及行驶里程截图 |
| 伙食补助、市内交通补助 | 不需要票据 |

发票丢失的，需提供付款记录和情况说明，由部门负责人签字确认，单张金额不超过 500 元。

## 三、审批流程

1. 员工提交报销单，上传票据照片；
2. 直属上级审批；
3. 单笔报销金额超过 5000 元的，增加部门负责人审批；
4. 财务部复核票据；
5. 复核通过后 5 个工作日内打款到员工工资卡。

## 四、常见退回原因

- 发票抬头或税号错误；
- 住宿超标未注明个人承担金额；
- 没有出差申请记录；
- 票据照片不清楚。`

const demoOnboarding = `# 新员工入职指南（示例）

> 示例内容，用来演示知识库。

## 入职当天带齐这些材料

1. 身份证原件；
2. 最高学历、学位证书原件；
3. 上一家单位的离职证明（应届生不需要）；
4. 入职前一个月内的体检报告；
5. 本人名下的招商银行储蓄卡（用于发工资），没有的可以入职后一周内补交；
6. 一寸白底证件照 2 张。

入职当天上午 9:30 到前台报到，HR 会带你办理手续、领取电脑和工牌。

## 第一周

- **第一天**：办入职手续、领电脑和工牌、开通账号（见《IT 与办公环境》）、和导师见面。
- **前三天**：完成线上新人培训（信息安全、财务报销、考勤制度三门必修课）。
- **第一周内**：和直属上级沟通试用期目标，在 OA 里确认。

## 试用期

试用期一般为 3 个月。试用期满前两周，由直属上级发起转正评估。`

const demoLeave = `# 考勤与休假（示例）

> 示例内容，用来演示知识库。

## 工作时间

实行弹性工作制：上午 9:00～10:00 之间到岗，每天工作满 8 小时（不含午休 1 小时）。每天上下班在企业微信打卡。

忘记打卡的，当天在 OA 提交补卡申请，每月最多补卡 3 次。

## 年假

| 累计工作年限 | 年假天数 |
|---|---|
| 满 1 年不满 10 年 | 5 天 |
| 满 10 年不满 20 年 | 10 天 |
| 满 20 年 | 15 天 |

新员工入职当年的年假按剩余日历天数折算，不足一天的部分不计。当年的年假可以顺延到次年 3 月 31 日，过期作废。

年假要提前 3 个工作日在 OA 申请；连续请 5 天以上的，要提前 2 周申请。

## 病假与事假

- 病假：需提供医院证明，每年 12 天以内按全额工资发放。
- 事假：无薪，每次最多 3 天，需直属上级批准。`

const demoIT = `# IT 与办公环境（示例）

> 示例内容，用来演示知识库。

## 入职要开通的账号

| 账号 | 用途 | 怎么开通 |
|---|---|---|
| 企业邮箱 | 收发邮件、登录各系统 | 入职当天自动开通，初始密码发到手机 |
| 企业微信 | 沟通、打卡、审批 | 用手机号登录 |
| VPN | 在公司外访问内网 | 在 IT 服务台提交申请，一个工作日内开通 |
| DSH 与 dsh-ee | 企业 AI 工作台 | 用企业邮箱登录 dsh-ee，按「开始使用」页面接入 DSH |

## 连接 VPN

1. 在 IT 服务台下载「企业 VPN」客户端；
2. 用企业邮箱和密码登录，首次登录要绑定手机令牌；
3. 连上以后就能访问内网系统。连不上请先确认手机令牌时间准确。

## 信息安全

- 公司电脑不得安装来路不明的软件；
- 客户资料、合同不得上传到个人网盘或公共 AI 服务；用公司的 DSH 处理这些资料。
- 离开座位锁屏（Windows：Win + L；Mac：Control + Command + Q）。

## 遇到问题

IT 服务台：分机 8000，工作日 9:00～18:00。`
