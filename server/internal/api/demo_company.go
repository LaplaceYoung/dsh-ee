package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例公司「星河科技」：约 200 人，研发、销售、财务、人力、IT、法务六个部门。
// 这里是账号、知识库、「自己用」的应用、30 天模拟使用数据与一键清除；设定见 docs/demo-company.md。

const (
	auditDemoClear = "demo.clear"
	// demo_records 里模拟使用数据的标记
	demoUsageMark = "usage"
)

var demoUsers = []demoUser{
	{"demo.zhao.lei", "赵磊", "研发部", "研发总监（部门负责人）"},
	{"demo.sun.yue", "孙悦", "研发部", "后端工程师"},
	{"demo.zheng.xiaoyu", "郑小雨", "研发部", "前端工程师（本周入职）"},
	{"demo.lin.na", "林娜", "销售部", "销售总监（部门负责人）"},
	{"demo.zhou.hang", "周航", "销售部", "客户经理"},
	{"demo.he.jing", "何静", "财务部", "财务经理（部门负责人）"},
	{"demo.chen.yi", "陈一", "财务部", "费用会计"},
	{"demo.wu.min", "吴敏", "人力资源部", "HRBP 负责人（部门负责人）"},
	{"demo.liu.yang", "刘洋", "IT 部", "IT 主管（部门负责人）"},
	{"demo.xu.nuo", "许诺", "法务部", "法务经理（部门负责人）"},
}

// 旧版示例内容的账号，清除时一并认出来
var demoLegacyUsers = []string{"demo.zhang", "demo.li"}

var demoKBases = []demoKBase{
	{
		Slug: "travel-policy", Title: "差旅与报销制度（示例）",
		Description: "财务部：出差住宿、伙食、交通标准，报销流程、票据要求与常见问题。示例内容，数字是编的。",
		Docs: []demoKDoc{
			{Title: "差旅费管理办法", Folder: "制度", Markdown: demoTravelPolicy},
			{Title: "报销流程与票据要求", Folder: "流程", Markdown: demoReimburseFlow},
			{Title: "费用报销常见问题", Folder: "FAQ", Markdown: demoExpenseFAQ},
		},
	},
	{
		Slug: "employee-handbook", Title: "员工手册（示例）",
		Description: "人力资源部：入职准备、考勤休假、福利培训、IT 与办公。示例内容。",
		Docs: []demoKDoc{
			{Title: "新员工入职指南", Folder: "入职", Markdown: demoOnboarding},
			{Title: "考勤与休假", Folder: "制度", Markdown: demoLeave},
			{Title: "福利、培训与晋升", Folder: "制度", Markdown: demoBenefits},
			{Title: "IT 与办公环境", Folder: "办公", Markdown: demoIT},
		},
	},
	{
		Slug: "it-helpdesk", Title: "IT 排障手册（示例）",
		Description: "IT 部：VPN、Wi-Fi、电脑、账号、打印机、软件安装的自助排障步骤。示例内容。",
		Docs: []demoKDoc{
			{Title: "网络与 VPN", Folder: "网络", Markdown: demoITVPN},
			{Title: "电脑、账号与打印", Folder: "终端", Markdown: demoITDevice},
		},
	},
	{
		Slug: "product-docs", Title: "产品资料（示例）",
		Description: "销售部：星河云各产品的介绍、价格、卖点，以及常见异议的应答。示例内容，价格是编的。",
		Docs: []demoKDoc{
			{Title: "星河云产品介绍", Folder: "产品", Markdown: demoProductIntro},
			{Title: "常见异议与应答", Folder: "销售", Markdown: demoObjections},
		},
	},
	{
		Slug: "legal-guide", Title: "法务指引（示例）",
		Description: "法务部：合同审批、用印规则与保密协议要点。示例内容。",
		Docs: []demoKDoc{
			{Title: "合同审批与用印规则", Folder: "合同", Markdown: demoContractFlow},
			{Title: "保密协议要点", Folder: "合同", Markdown: demoNDA},
		},
	},
	{
		Slug: "eng-handbook", Title: "研发规范（示例）",
		Description: "研发部：代码评审、发布窗口、值班与新人上手。示例内容。",
		Docs: []demoKDoc{
			{Title: "代码评审与发布", Folder: "规范", Markdown: demoEngReview},
		},
	},
}

// demoSelfApp 是某个演示员工自己搭建的应用：默认「自己用」（范围只有他本人，免审核）；
// Everyone 的是他做好、已经分享给全公司的。Rows 是导入时预置的数据（表名 → 谁填的 + 内容）。
type demoSelfApp struct {
	Owner    string
	Everyone bool
	Rows     map[string][]demoRow
	// 数据中台可用（示例数据集已导入）时加上的工作台，并开网页版
	DataWorkbench *appWorkbench
	demoAsset
}

type demoRow struct {
	By   string
	Data map[string]any
}

func demoSelfApps() []demoSelfApp {
	return []demoSelfApp{
		{Owner: "demo.zhou.hang", DataWorkbench: demoWeeklyWorkbench(), demoAsset: demoAsset{Kind: store.KindApp, Slug: "zhouhang-weekly", Delivery: store.DeliveryOptional, Spec: assetSpec{
			Title:       "我的客户周报",
			Description: "周航自己做的：每周五汇总负责客户的续约进度和拜访情况，生成发给销售总监的周报。只有他自己用。",
			Icon:        "📈",
			Persona:     "你是客户经理周航的助理。用户说「写周报」时：1）用 CRM 查周航负责的客户里 90 天内要续约的；2）列出本周拜访过的客户和下周计划；3）高风险客户单独列出原因和建议动作。输出一份简洁的周报，分「续约进展」「本周拜访」「风险与求助」三节。",
			Starters:    []string{"写这周的客户周报", "我负责的客户里哪些续约风险高？"},
			Connections: []string{demoCRMConn},
		}}},
		demoOutingApp(),
		{Owner: "demo.sun.yue", demoAsset: demoAsset{Kind: store.KindApp, Slug: "sunyue-review", Delivery: store.DeliveryOptional, Spec: assetSpec{
			Title:       "代码评审清单",
			Description: "孙悦自己做的：按研发规范检查合并请求，列出必须改和建议改的地方。只有她自己用。",
			Icon:        "🔍",
			Persona:     "你是代码评审助手。用户贴来代码或 diff 时，按《研发规范》的评审重点（正确性、测试、可读性）逐条检查，分「必须改」「建议改」两类列出，每条说明原因。改动超过 400 行的，先建议拆分。",
			Starters:    []string{"帮我看看这段 diff", "评审时最容易漏掉什么？"},
			Knowledge:   []string{"eng-handbook"},
		}}},
	}
}

const demoCRMOpenAPI = `{
  "openapi": "3.0.3",
  "info": { "title": "CRM 客户系统（示例）", "version": "1.0.0" },
  "paths": {
    "/demo/crm/summary": {
      "get": {
        "operationId": "get_summary",
        "summary": "客户总览：客户数、年合同额（元）、90 天内要续约的客户数、续约风险高的客户数",
        "parameters": [
          { "name": "owner", "in": "query", "required": false, "description": "只看某位客户经理负责的客户，填姓名，例如 周航", "schema": { "type": "string" } }
        ]
      }
    },
    "/demo/crm/customers": {
      "get": {
        "operationId": "list_customers",
        "summary": "客户列表（按续约日期排序）：行业、城市、负责人、年合同额、阶段、续约日期、最近拜访、健康度、续约风险",
        "parameters": [
          { "name": "owner", "in": "query", "required": false, "description": "负责人姓名", "schema": { "type": "string" } },
          { "name": "risk", "in": "query", "required": false, "description": "续约风险：高 / 中 / 低", "schema": { "type": "string" } },
          { "name": "q", "in": "query", "required": false, "description": "按客户名或行业搜索", "schema": { "type": "string" } }
        ]
      }
    },
    "/demo/crm/customers/{id}": {
      "get": {
        "operationId": "get_customer",
        "summary": "客户详情：合同明细与拜访记录。id 是客户编号（如 C001），也可以直接填客户名",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ]
      }
    },
    "/demo/crm/renewals": {
      "get": {
        "operationId": "list_renewals",
        "summary": "多少天内要续约的客户，续约风险高的排在前面",
        "parameters": [
          { "name": "days", "in": "query", "required": false, "description": "天数，默认 90", "schema": { "type": "integer" } },
          { "name": "owner", "in": "query", "required": false, "description": "负责人姓名", "schema": { "type": "string" } }
        ]
      }
    },
    "/demo/crm/visits": {
      "post": {
        "operationId": "log_visit",
        "summary": "给客户记一条拜访记录",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["customer_id", "by", "summary"],
                "properties": {
                  "customer_id": { "type": "string", "description": "客户编号，如 C001" },
                  "by": { "type": "string", "description": "拜访人姓名" },
                  "summary": { "type": "string", "description": "拜访纪要" }
                }
              }
            }
          }
        }
      }
    }
  }
}`

const demoITSMOpenAPI = `{
  "openapi": "3.0.3",
  "info": { "title": "IT 工单系统（示例）", "version": "1.0.0" },
  "paths": {
    "/demo/itsm/summary": {
      "get": {
        "operationId": "get_summary",
        "summary": "工单概况：未解决、超时（超过 3 天未解决）、近 7 天解决的工单数",
        "parameters": [
          { "name": "requester", "in": "query", "required": false, "description": "只看某人报修的（处理人填自己的姓名，看派给他的），填姓名", "schema": { "type": "string" } }
        ]
      }
    },
    "/demo/itsm/tickets": {
      "get": {
        "operationId": "list_tickets",
        "summary": "工单列表，新的在前",
        "parameters": [
          { "name": "requester", "in": "query", "required": false, "description": "报修人姓名；处理人填自己的姓名，看派给他的工单", "schema": { "type": "string" } },
          { "name": "status", "in": "query", "required": false, "description": "待处理 / 处理中 / 已解决 / 已关闭，或「未解决」", "schema": { "type": "string" } }
        ]
      },
      "post": {
        "operationId": "create_ticket",
        "summary": "新建一张 IT 工单",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["title", "requester"],
                "properties": {
                  "title": { "type": "string", "description": "一句话描述问题" },
                  "category": { "type": "string", "description": "故障分类：vpn / mail / printer / account / laptop / software / meeting" },
                  "requester": { "type": "string", "description": "报修人姓名" },
                  "priority": { "type": "string", "description": "低 / 中 / 高" },
                  "detail": { "type": "string", "description": "现象、已经试过的排查步骤" }
                }
              }
            }
          }
        }
      }
    },
    "/demo/itsm/categories": {
      "get": {
        "operationId": "list_categories",
        "summary": "常见故障分类、近 30 天的工单数与自助排障提示"
      }
    }
  }
}`

// demoImportSelfApp 以员工本人的身份建一个「自己用」的应用（范围只有他，直接上架）。
func (s *Server) demoImportSelfApp(ctx context.Context, d demoSelfApp) (demoItem, error) {
	it := demoItem{Kind: d.Kind, Slug: d.Slug, Title: d.Spec.Title, Result: "exists"}
	existing, err := s.store.GetAssetBySlug(ctx, d.Slug)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return it, err
	}
	owner, err := s.store.GetUserByUsername(ctx, d.Owner)
	if existing != nil {
		// 自己用的应用里，带数据中台工作台的那个按环境换内容，不参与升级
		if err != nil || d.DataWorkbench != nil {
			return it, nil
		}
		return s.demoUpgradeApp(ctx, it, existing, d.Spec, owner.ID)
	}
	if err != nil {
		return it, fmt.Errorf("找不到演示员工 %s：%w", d.Owner, err)
	}
	sp := d.Spec
	if d.DataWorkbench != nil {
		if _, err := s.store.GetDataSet(ctx, "my-customers"); err == nil {
			sp.Workbench, sp.Web = d.DataWorkbench, true
		}
	}
	if msg := validateSpec(d.Kind, &sp); msg != "" {
		return it, errors.New(msg)
	}
	raw, _ := json.Marshal(sp)
	a := &store.Asset{Kind: d.Kind, Slug: d.Slug, Title: sp.Title, Description: sp.Description, Category: demoCategory,
		OwnerID: &owner.ID, Scope: store.Scope{Users: []int64{owner.ID}}, Delivery: store.DeliveryOptional}
	if d.Everyone {
		a.Scope, a.Delivery = store.Scope{All: true}, store.DeliveryAuto
		if len(d.Departments) > 0 {
			a.Scope = store.Scope{Departments: d.Departments}
		}
	}
	if err := s.store.CreateAsset(ctx, a, raw, store.StatusPublished, owner.ID); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return it, nil
		}
		return it, err
	}
	if !d.Everyone {
		if err := s.store.Subscribe(ctx, owner.ID, a.ID); err != nil {
			return it, err
		}
	}
	if err := s.store.RecordDemo(ctx, "asset", a.ID, a.Slug); err != nil {
		return it, err
	}
	it.Result = "created"
	return it, s.demoSeedRows(ctx, a, &sp, d.Rows)
}

// demoSeedRows 按演示员工写入应用的预置数据（字段照常校验，user 字段填成那位员工）。
func (s *Server) demoSeedRows(ctx context.Context, a *store.Asset, sp *assetSpec, rows map[string][]demoRow) error {
	for name, list := range rows {
		t := findTable(sp.Tables, name)
		if t == nil {
			return fmt.Errorf("示例应用 %s 没有表 %s", a.Slug, name)
		}
		for _, r := range list {
			u, err := s.store.GetUserByUsername(ctx, r.By)
			if err != nil {
				return fmt.Errorf("找不到演示员工 %s：%w", r.By, err)
			}
			clean, msg := cleanRow(t, r.Data, true, u)
			if msg != "" {
				return fmt.Errorf("示例数据不合法：%s", msg)
			}
			b, _ := json.Marshal(clean)
			if _, err := s.store.InsertAppRows(ctx, a.ID, t.Name, u.ID, [][]byte{b}); err != nil {
				return err
			}
		}
	}
	return nil
}

// demoOutingApp 是人力资源部吴敏做好、分享给全公司的「团建报名」：演示应用自带数据。
func demoOutingApp() demoSelfApp {
	morning, afternoon := "周六上午 · 徒步", "周六下午 · 烧烤"
	return demoSelfApp{Owner: "demo.wu.min", Everyone: true, demoAsset: demoAsset{Kind: store.KindApp, Slug: "team-outing", Delivery: store.DeliveryAuto, Spec: assetSpec{
		Title:       "团建报名",
		Description: "人力资源部吴敏发起的本月团建报名：选场次、填人数和忌口。每人只看到自己的报名，组织者看到全部；AI 可以帮你报名、改报名、查报名情况。",
		Icon:        "🏕",
		Web:         true,
		Persona: "你是星河科技本月团建的报名助手，组织者是人力资源部的吴敏。员工想报名时，问清场次（" + morning + " / " + afternoon + "）、同行人数和忌口，" +
			"把要写的内容复述一遍、得到确认后再用 app__write 写进报名表；想改报名时先用 app__rows 查出他自己的那一行再改。" +
			"组织者问报名情况时，用 app__rows 的 group_by 按场次统计人数，再列出有忌口的人。集合时间、地点以「公告」表为准，不要自己编。",
		Starters: []string{"帮我报名周六下午的烧烤，我不吃辣", "我报了哪一场？帮我改成上午", "现在各场次报了多少人？"},
		Tables: []appTable{
			{Name: "signups", Title: "报名", Visibility: "own", Fields: []appField{
				{Name: "who", Title: "姓名", Kind: "user"},
				{Name: "session", Title: "场次", Kind: "select", Options: []string{morning, afternoon}, Required: true},
				{Name: "people", Title: "人数（含本人）", Kind: "number", Required: true},
				{Name: "diet", Title: "忌口", Kind: "text"},
			}},
			{Name: "notes", Title: "公告", Visibility: "author", Fields: []appField{
				{Name: "title", Title: "标题", Kind: "text", Required: true},
				{Name: "content", Title: "内容", Kind: "text", Required: true},
			}},
		},
		Workbench: &appWorkbench{Panes: []wbPane{{ID: "main", Type: "panel", Title: "团建报名", Widgets: []wbWidget{
			{Type: "table", Title: "活动安排", Source: &wbSource{Table: "notes"}, Rows: "items", Width: "half", Columns: []wbColumn{{Field: "title", Title: "事项"}, {Field: "content", Title: "说明"}}},
			{Type: "chart", Title: "按场次统计（组织者看全部）", Chart: "pie", Source: &wbSource{Table: "signups", Args: json.RawMessage(`{"group_by":"session"}`)}, Rows: "items", X: "session", Y: []wbColumn{{Field: "count", Title: "报名数"}}, Width: "half"},
			{Type: "form", Title: "我要报名", Width: "third", Submit: &wbAction{Label: "报名", Table: "signups"}},
			{Type: "table", Title: "我的报名", Source: &wbSource{Table: "signups"}, Rows: "items", Width: "wide",
				Columns: []wbColumn{{Field: "session", Title: "场次", Format: "status"}, {Field: "people", Title: "人数"}, {Field: "diet", Title: "忌口"}},
				RowActions: []wbAction{
					{Label: "改报名", Prompt: "我想修改第 {{row.id}} 条报名（{{row.session}}，{{row.people}} 人）"},
					{Label: "取消报名", Table: "signups", Delete: true},
				},
				Empty: &wbEmpty{Text: "你还没有报名", Action: &wbAction{Label: "让 AI 帮我报名", Prompt: "帮我报名这次团建"}}},
		}}}},
	}}, Rows: map[string][]demoRow{
		"notes": {
			{By: "demo.wu.min", Data: map[string]any{"title": "集合", "content": "周六 8:30 公司楼下集合，徒步组请穿运动鞋、带水。"}},
			{By: "demo.wu.min", Data: map[string]any{"title": "烧烤", "content": "周六 14:00 滨江公园烧烤区，人数以周四中午 12 点前的报名为准。"}},
		},
		"signups": {
			{By: "demo.zhao.lei", Data: map[string]any{"session": morning, "people": 1}},
			{By: "demo.sun.yue", Data: map[string]any{"session": afternoon, "people": 2, "diet": "不吃辣"}},
			{By: "demo.lin.na", Data: map[string]any{"session": afternoon, "people": 1, "diet": "素食"}},
			{By: "demo.zhou.hang", Data: map[string]any{"session": morning, "people": 1}},
			{By: "demo.he.jing", Data: map[string]any{"session": afternoon, "people": 3, "diet": "海鲜过敏"}},
			{By: "demo.liu.yang", Data: map[string]any{"session": morning, "people": 1}},
		},
	}}
}

// ---- 30 天模拟使用数据 ----

// demoQuestions 是各部门员工会问知识库的问题；带 miss 的是知识库里没有答案的。
var demoQuestions = []struct {
	q, kb string
	miss  bool
}{
	{"一线城市住宿标准是多少", "travel-policy", false},
	{"当天往返能报住宿费吗", "travel-policy", false},
	{"出差打车能报吗", "travel-policy", false},
	{"报销多久到账", "travel-policy", false},
	{"发票丢了怎么报销", "travel-policy", false},
	{"请客户吃饭人均多少", "travel-policy", false},
	{"年假能不能跨年", "employee-handbook", false},
	{"入职第一天带什么材料", "employee-handbook", false},
	{"年度体检怎么预约", "employee-handbook", false},
	{"晋升评审什么时候", "employee-handbook", false},
	{"公司有健身房补贴吗", "employee-handbook", true},
	{"生日假有几天", "employee-handbook", true},
	{"加班可以调休吗", "employee-handbook", true},
	{"VPN 连不上怎么办", "it-helpdesk", false},
	{"打印机怎么用", "it-helpdesk", false},
	{"电脑很卡怎么办", "it-helpdesk", false},
	{"怎么申请第二块显示器", "it-helpdesk", true},
	{"续约能打几折", "product-docs", false},
	{"企业版多少钱一年", "product-docs", false},
	{"客户说太贵怎么回应", "product-docs", false},
	{"和竞品比优势是什么", "product-docs", false},
	{"智能客服支持多少并发", "product-docs", true},
	{"哪些合同要法务审", "legal-guide", false},
	{"什么时候可以用印", "legal-guide", false},
	{"对方的保密协议要看哪些条款", "legal-guide", false},
	{"电子合同怎么签", "legal-guide", true},
	{"生产发布是哪天", "eng-handbook", false},
	{"代码评审多久要给意见", "eng-handbook", false},
	// 山海工作室
	{"林栖一天能引雾几次", "novel-bible", false},
	{"陆沉舟是不是引雾者", "novel-bible", false},
	{"铜罗盘是第几章出现的", "novel-bible", false},
	{"白焰附近能不能引雾", "novel-bible", false},
	{"雾港有多少人口", "novel-bible", true},
	{"林栖的母亲叫什么", "novel-bible", true},
	{"一章写多少字", "novel-style", false},
	{"交稿要提前多久", "novel-style", false},
	{"存稿至少留几章", "novel-style", false},
	{"番外什么时候更新", "novel-style", true},
}

// demoPersona 决定一个演示员工每天大概做什么：用哪些应用、调哪些工具、问哪些库。
type demoPersona struct {
	activity float64 // 每个工作日活跃的概率
	apps     []string
	tools    []struct{ conn, tool string }
	kbs      []string
	calls    int // 每个活跃日大约多少次模型调用
}

var demoPersonas = map[string]demoPersona{
	"demo.zhao.lei":     {0.8, []string{"meeting-minutes"}, nil, []string{"eng-handbook"}, 10},
	"demo.sun.yue":      {0.9, []string{"sunyue-review", "meeting-minutes"}, nil, []string{"eng-handbook", "it-helpdesk"}, 18},
	"demo.zheng.xiaoyu": {0.95, []string{"onboarding-buddy", "it-service-desk"}, []struct{ conn, tool string }{{demoITSMConn, "list_tickets"}}, []string{"employee-handbook", "it-helpdesk", "eng-handbook"}, 8},
	"demo.lin.na":       {0.85, []string{"travel-expense", "customer-brief", "meeting-minutes", demoRenewalExpert}, []struct{ conn, tool string }{{demoCRMConn, "get_summary"}, {demoCRMConn, "list_renewals"}}, []string{"product-docs"}, 9},
	"demo.zhou.hang":    {0.95, []string{"travel-expense", "customer-brief", "zhouhang-weekly", demoRenewalExpert}, []struct{ conn, tool string }{{demoCRMConn, "list_customers"}, {demoCRMConn, "get_customer"}, {demoConn, "list_claims"}}, []string{"product-docs", "travel-policy"}, 14},
	"demo.he.jing":      {0.7, []string{"travel-expense", demoAuditExpert}, []struct{ conn, tool string }{{demoConn, "get_summary"}, {demoConn, "list_claims"}}, []string{"travel-policy"}, 6},
	"demo.chen.yi":      {0.75, []string{"travel-expense", demoAuditExpert}, []struct{ conn, tool string }{{demoConn, "list_claims"}}, []string{"travel-policy"}, 7},
	"demo.wu.min":       {0.6, []string{"onboarding-buddy", "meeting-minutes"}, nil, []string{"employee-handbook"}, 5},
	"demo.liu.yang":     {0.7, []string{"it-service-desk"}, []struct{ conn, tool string }{{demoITSMConn, "list_tickets"}, {demoITSMConn, "get_summary"}, {demoITSMConn, "list_categories"}}, []string{"it-helpdesk"}, 6},
	"demo.xu.nuo":       {0.5, []string{demoContractExpert}, nil, []string{"legal-guide"}, 4},
	// 山海工作室
	"demo.shen.mo":      {0.9, []string{"story-bible", "continuity-editor", "chapter-board"}, nil, []string{"novel-bible", "novel-style"}, 16},
	"demo.gu.qing":      {0.85, []string{"chapter-board", "line-editor", "continuity-editor"}, nil, []string{"novel-style", "novel-bible"}, 12},
	"demo.ning.yuan":    {0.8, []string{"story-bible", "continuity-editor", "chapter-board"}, nil, []string{"novel-bible"}, 9},
	"demo.tang.xiaoman": {0.6, []string{"chapter-board"}, nil, []string{"novel-style"}, 4},
}

// demoSimulateUsage 写入近 30 天的模型调用、应用打开、企业系统调用、知识库检索与启用记录，
// 让管理总览、运营数据和月度报告一打开就有曲线。只写一次（demo_records 里有标记），数据都挂在演示员工名下，清除时随账号删掉。
// 每个场景各写一次：mark 是 demo_records 里的 ref_id（示例公司是 0）。
func (s *Server) demoSimulateUsage(ctx context.Context, now time.Time, staff []demoUser, mark int64) (int, error) {
	if done, err := s.store.HasDemoRecord(ctx, demoUsageMark, mark); err != nil || done {
		return 0, err
	}
	users := map[string]int64{}
	for _, u := range staff {
		if x, err := s.store.GetUserByUsername(ctx, u.Username); err == nil {
			users[u.Username] = x.ID
		}
	}
	assets := map[string]int64{}
	all, err := s.store.ListAssets(ctx)
	if err != nil {
		return 0, err
	}
	for _, a := range all {
		assets[a.Slug] = a.ID
	}
	rng := rand.New(rand.NewSource(now.Unix() / 86400))
	var logs []store.UsageLogRow
	var uses []store.AssetUseRow
	var calls []store.ConnCallRow
	var queries []store.KBQueryRow
	// 工作时间（东八区 9:00～19:00）里的一个时刻，不晚于现在
	at := func(day time.Time) time.Time {
		t := time.Date(day.Year(), day.Month(), day.Day(), 1, 0, 0, 0, time.UTC).Add(time.Duration(rng.Intn(10*3600)) * time.Second)
		if t.After(now) {
			t = now.Add(-time.Duration(rng.Intn(3600)) * time.Second)
		}
		return t
	}
	for back := 29; back >= 0; back-- {
		day := now.AddDate(0, 0, -back)
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		// 越近越活跃：一个月里用的人和次数都在涨
		growth := 0.55 + 0.45*float64(29-back)/29
		for _, u := range staff {
			uid, ok := users[u.Username]
			p, has := demoPersonas[u.Username]
			if !ok || !has {
				continue
			}
			chance := p.activity * growth
			if weekend {
				chance *= 0.12
			}
			// 新人本周才入职
			if u.Username == "demo.zheng.xiaoyu" && back > 5 {
				continue
			}
			if rng.Float64() > chance {
				continue
			}
			n := 1 + rng.Intn(p.calls) + int(float64(p.calls)*growth/2)
			for i := 0; i < n; i++ {
				model, tokens := "deepseek-chat", 900+rng.Intn(5200)
				if rng.Intn(5) == 0 {
					model, tokens = "deepseek-reasoner", 3000+rng.Intn(9000)
				}
				status := 200
				if rng.Intn(80) == 0 {
					status = 429
				}
				logs = append(logs, store.UsageLogRow{UserID: uid, Model: model, Tokens: tokens, Status: status, At: at(day)})
			}
			for _, slug := range p.apps {
				if id, ok := assets[slug]; ok && rng.Float64() < 0.7 {
					for k := 0; k <= rng.Intn(3); k++ {
						uses = append(uses, store.AssetUseRow{AssetID: id, UserID: uid, At: at(day)})
					}
				}
			}
			for _, t := range p.tools {
				if id, ok := assets[t.conn]; ok && rng.Float64() < 0.6 {
					for k := 0; k <= rng.Intn(3); k++ {
						ok := rng.Intn(25) != 0
						errText := ""
						if !ok {
							errText = "上游 5 秒没有响应"
						}
						calls = append(calls, store.ConnCallRow{AssetID: id, UserID: uid, Tool: t.tool, OK: ok, Error: errText, LatencyMS: 80 + rng.Intn(600), At: at(day)})
					}
				}
			}
			for _, kb := range p.kbs {
				if rng.Float64() > 0.45 {
					continue
				}
				var pool []int
				for i, q := range demoQuestions {
					if q.kb == kb {
						pool = append(pool, i)
					}
				}
				q := demoQuestions[pool[rng.Intn(len(pool))]]
				hits := 2 + rng.Intn(6)
				if q.miss {
					hits = 0
				}
				bases, _ := json.Marshal([]string{kb})
				queries = append(queries, store.KBQueryRow{UserID: uid, Tool: "search", Query: q.q, Bases: string(bases), Hits: hits, At: at(day)})
				if hits > 0 {
					queries = append(queries, store.KBQueryRow{UserID: uid, Tool: "read", Bases: string(bases), Hits: 1, At: at(day)})
				}
			}
		}
	}
	if err := s.store.InsertUsageLogs(ctx, logs); err != nil {
		return 0, err
	}
	if err := s.store.InsertAssetUses(ctx, uses); err != nil {
		return 0, err
	}
	if err := s.store.InsertConnCalls(ctx, calls); err != nil {
		return 0, err
	}
	if err := s.store.InsertKBQueries(ctx, queries); err != nil {
		return 0, err
	}
	// 「能力安装」：演示员工在头两周陆续启用了周报提示词
	if id, ok := assets["weekly-report"]; ok && mark == 0 {
		for i, u := range staff {
			if uid, ok := users[u.Username]; ok && i%2 == 0 {
				if err := s.store.SubscribeAt(ctx, uid, id, now.AddDate(0, 0, -25+i)); err != nil {
					return 0, err
				}
			}
		}
	}
	return len(logs) + len(uses) + len(calls) + len(queries), s.store.RecordDemo(ctx, demoUsageMark, mark, "30 天模拟使用数据")
}

// ---- 一键清除 ----

type demoClearResult struct {
	Assets int `json:"assets"`
	KBases int `json:"kbases"`
	Users  int `json:"users"`
	// 数据中台里的示例表和数据集
	Data int `json:"data"`
}

// DELETE /api/admin/demo：把示例的知识库、连接、应用、员工和模拟统计全部删掉，用户自己建的内容不动。
// 只删导入时记下的那些；更早版本导入的（没有记录）按标识认出来：能力要是「示例」分类，库和账号要带「（示例）」字样。
func (s *Server) handleDemoClear(w http.ResponseWriter, r *http.Request) {
	if !s.topAdmin(currentAccess(r)) {
		writeErr(w, http.StatusForbidden, "清除示例数据需要超级管理员")
		return
	}
	ctx := r.Context()
	recs, err := s.store.DemoRecords(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	targets := map[string]map[int64]bool{"asset": {}, "kb": {}, "user": {}}
	for _, rec := range recs {
		if targets[rec.Kind] != nil {
			targets[rec.Kind][rec.RefID] = true
		}
	}
	// 没有记录的旧版示例
	known := map[string]bool{}
	for _, a := range demoAssets() {
		known[a.Slug] = true
	}
	for _, a := range demoSelfApps() {
		known[a.Slug] = true
	}
	if assets, err := s.store.ListAssets(ctx); err == nil {
		for _, a := range assets {
			if known[a.Slug] && a.Category == demoCategory {
				targets["asset"][a.ID] = true
			}
		}
	}
	for _, b := range demoKBases {
		if kb, err := s.store.GetKBaseBySlug(ctx, b.Slug); err == nil && strings.Contains(kb.Title, "（示例）") {
			targets["kb"][kb.ID] = true
		}
	}
	for _, name := range demoLegacyUsers {
		if u, err := s.store.GetUserByUsername(ctx, name); err == nil && strings.Contains(u.DisplayName, "（示例）") {
			targets["user"][u.ID] = true
		}
	}
	me := currentUser(r).ID
	res := demoClearResult{}
	// 先删应用再删连接：顺序不影响结果，只是让审计日志读起来顺
	for id := range targets["asset"] {
		if err := s.store.PurgeAssetActivity(ctx, id); err != nil {
			s.internalErr(w, r, err)
			return
		}
		if err := s.store.PurgeAssetAppRows(ctx, id); err != nil {
			s.internalErr(w, r, err)
			return
		}
		if err := s.store.DeleteAsset(ctx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internalErr(w, r, err)
			return
		}
		s.dropConnClient(id)
		res.Assets++
	}
	for id := range targets["kb"] {
		if err := s.store.DeleteKBase(ctx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internalErr(w, r, err)
			return
		}
		s.kb.ix.Drop(id)
		res.KBases++
	}
	for id := range targets["user"] {
		if id == me {
			continue
		}
		if err := s.store.PurgeUser(ctx, id); err != nil {
			s.internalErr(w, r, err)
			return
		}
		res.Users++
	}
	if err := s.demoClearPolicies(ctx, recs); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if res.Data, err = s.demoClearData(ctx, recs); err != nil {
		s.internalErr(w, r, err)
		return
	}
	for _, rec := range recs {
		if err := s.store.DeleteDemoRecord(ctx, rec.Kind, rec.RefID); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	s.audit(r, auditDemoClear, "demo", map[string]any{"assets": res.Assets, "kbases": res.KBases, "users": res.Users})
	s.hub.notify()
	writeJSON(w, http.StatusOK, res)
}

// demoGuide 是导入完成后的「下一步」：用哪个演示员工登录、先试哪个应用。
type demoGuideStep struct {
	// 属于哪个示例场景（demoScenarios 里的标识和名字），登录页按它分组
	Scenario      string `json:"scenario"`
	ScenarioTitle string `json:"scenario_title"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Department    string `json:"department"`
	Title         string `json:"title"`
	App           string `json:"app"`
	AppTitle      string `json:"app_title"`
	Try           string `json:"try"`
}

func demoGuide() []demoGuideStep {
	return demoGuideSteps(demoScenarioCompany, demoUsers, [][4]string{
		{"demo.zhou.hang", "customer-brief", "客户拜访简报", "给华东精密制造生成一份拜访简报"},
		{"demo.zheng.xiaoyu", "onboarding-buddy", "新人入职助手", "我第一周要做什么？"},
		{"demo.chen.yi", "travel-expense", "出差报销助手", "去上海出差 3 天，住宿每晚 650，坐高铁二等座来回 1100，能报多少？"},
		{"demo.liu.yang", "it-service-desk", "IT 服务台", "VPN 连不上，提示证书错误"},
	})
}

// demoRecordCreated 把这次新建的示例项记进 demo_records。
func (s *Server) demoRecordCreated(ctx context.Context, it demoItem) error {
	if it.Result != "created" {
		return nil
	}
	switch it.Kind {
	case "kb":
		b, err := s.store.GetKBaseBySlug(ctx, it.Slug)
		if err != nil {
			return err
		}
		return s.store.RecordDemo(ctx, "kb", b.ID, it.Slug)
	case "user":
		u, err := s.store.GetUserByUsername(ctx, it.Slug)
		if err != nil {
			return err
		}
		return s.store.RecordDemo(ctx, "user", u.ID, it.Slug)
	default:
		if !slices.Contains([]string{store.KindApp, store.KindSkill, store.KindPrompt, store.KindConnection}, it.Kind) {
			return nil
		}
		a, err := s.store.GetAssetBySlug(ctx, it.Slug)
		if err != nil {
			return err
		}
		return s.store.RecordDemo(ctx, "asset", a.ID, it.Slug)
	}
}
