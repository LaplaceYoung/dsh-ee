package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

func TestDemoImport(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	// 演示连接要从服务端访问自己，先拿到监听地址再建服务
	ts := httptest.NewUnstartedServer(nil)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), Addr: ts.Listener.Addr().String()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts.Config.Handler = srv.Handler()
	ts.Start()
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "示例状态", code, 200, body)
	if decodeAs[struct{ Imported bool }](t, body).Imported {
		t.Fatal("还没导入时不应显示已导入")
	}

	type result struct {
		Items []struct {
			Kind, Slug, Result, Error string
		}
		Accounts []struct {
			Username, Password string
		}
	}
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "导入示例", code, 200, body)
	res := decodeAs[result](t, body)
	for _, it := range res.Items {
		if it.Result != "created" {
			t.Fatalf("%s %s 没建成：%s %s", it.Kind, it.Slug, it.Result, it.Error)
		}
	}
	if len(res.Items) != 34 || len(res.Accounts) != 10 {
		t.Fatalf("应建 6 个库、15 项能力（含 2 个技能、4 位专家）、10 个账号、3 个员工自己做的应用：%s", body)
	}

	// 再导一次：全部跳过，不返回密码
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "重复导入", code, 200, body)
	again := decodeAs[result](t, body)
	for _, it := range again.Items {
		if it.Result != "exists" {
			t.Fatalf("重复导入时 %s 应跳过：%s", it.Slug, it.Result)
		}
	}
	if len(again.Accounts) != 0 {
		t.Fatalf("重复导入不应再给密码：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "示例状态", code, 200, body)
	if !decodeAs[struct{ Imported bool }](t, body).Imported {
		t.Fatalf("导入后应显示已导入：%s", body)
	}

	// 部署清单（/go）里「示例内容」一步变成已完成
	code, body = admin.do("GET", "/api/admin/setup-status", nil, nil)
	expect(t, "部署清单", code, 200, body)
	var setup struct {
		Steps []struct {
			ID       string
			Done     bool
			Optional bool
		}
	}
	_ = json.Unmarshal(body, &setup)
	if !slices.ContainsFunc(setup.Steps, func(s struct {
		ID       string
		Done     bool
		Optional bool
	}) bool {
		return s.ID == "demo" && s.Done && s.Optional
	}) {
		t.Fatalf("部署清单里应有已完成的可选步骤 demo：%s", body)
	}

	// 填了访问地址：微应用指令里教模型把出处写成网页链接
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["baseUrl"] = "https://ee.example.com"
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "填访问地址", code, 200, body)

	// 演示员工用返回的密码登录，接入 DSH：研发部的孙悦；销售部的周航在下面另接一个，用来调 CRM
	if res.Accounts[4].Username != "demo.zhou.hang" || res.Accounts[1].Username != "demo.sun.yue" {
		t.Fatalf("演示账号的顺序变了：%s", body)
	}
	zhang := newClient(t, ts.URL)
	code, body = zhang.do("POST", "/api/auth/login", map[string]string{"username": res.Accounts[1].Username, "password": res.Accounts[1].Password}, nil)
	expect(t, "演示员工登录", code, 200, body)
	code, body = zhang.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "接入令牌", code, 201, body)
	dsh := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}

	cat := dsh.catalog()
	var apps []string
	for _, a := range cat.Apps {
		apps = append(apps, a.Name)
		if len(a.Missing) > 0 {
			t.Fatalf("微应用 %s 缺能力：%v", a.Name, a.Missing)
		}
	}
	for _, want := range []string{"travel-expense", "onboarding-buddy", "contract-reviewer"} {
		if !slices.Contains(apps, want) {
			t.Fatalf("员工 DSH 里应有微应用 / 专家 %s：%v", want, apps)
		}
	}
	if !slices.Contains(cat.skillNames(), "contract-clause-check") || slices.Contains(cat.skillNames(), "customer-email") || slices.Contains(cat.skillNames(), "weekly-report") {
		t.Fatalf("自动生效的技能应下发、自选的技能和提示词不应下发：%v", cat.skillNames())
	}
	// 研发部的孙悦：示例的禁用策略把 CRM 和依赖它的应用、专家整个挡掉（上面已验没有缺能力的应用）；
	// 报销稽核专家只给财务部，专家团队是自选的
	for _, hidden := range []string{"customer-brief", "renewal-advisor", "expense-auditor", "deal-desk"} {
		if slices.Contains(apps, hidden) {
			t.Fatalf("研发部的孙悦不应拿到 %s：%v", hidden, apps)
		}
	}
	if r := dsh.rpc("tools/call", map[string]any{"name": "demo-crm__get_summary", "arguments": map[string]any{}}); r["error"] == nil {
		if out, _ := r["result"].(map[string]any); out == nil || out["isError"] != true {
			t.Fatalf("研发部调 CRM 应被挡住：%v", r)
		}
	}

	// 销售部的周航：有客户拜访简报和续约谈判专家
	zhouWeb := newClient(t, ts.URL)
	code, body = zhouWeb.do("POST", "/api/auth/login", map[string]string{"username": res.Accounts[4].Username, "password": res.Accounts[4].Password}, nil)
	expect(t, "销售员工登录", code, 200, body)
	code, body = zhouWeb.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
	expect(t, "销售员工的接入令牌", code, 201, body)
	zhou := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	var zhouApps []string
	for _, a := range zhou.catalog().Apps {
		zhouApps = append(zhouApps, a.Name)
		if len(a.Missing) > 0 {
			t.Fatalf("周航的 %s 缺能力：%v", a.Name, a.Missing)
		}
	}
	for _, want := range []string{"customer-brief", "renewal-advisor", "contract-reviewer", "zhouhang-weekly"} {
		if !slices.Contains(zhouApps, want) {
			t.Fatalf("周航的 DSH 里应有 %s：%v", want, zhouApps)
		}
	}
	if slices.Contains(zhouApps, "expense-auditor") {
		t.Fatalf("报销稽核专家只给财务部：%v", zhouApps)
	}
	code, body = admin.do("GET", "/api/admin/users", nil, nil)
	expect(t, "用户列表", code, 200, body)
	var sunID int64
	for _, u := range decodeAs[struct {
		Users []struct {
			ID       int64
			Username string
		}
	}](t, body).Users {
		if u.Username == "demo.sun.yue" {
			sunID = u.ID
		}
	}
	code, body = admin.do("GET", fmt.Sprintf("/api/admin/hub/effective?user=%d", sunID), nil, nil)
	expect(t, "权限查询", code, 200, body)
	blockedCRM := false
	for _, it := range decodeAs[struct {
		Items []struct {
			Slug, Result, Reason string
			BlockIDs             []int64 `json:"block_ids"`
		}
	}](t, body).Items {
		blockedCRM = blockedCRM || (it.Slug == "demo-crm" && it.Result == "hidden" && it.Reason == "blocked" && len(it.BlockIDs) == 1)
	}
	if !blockedCRM {
		t.Fatalf("权限查询应显示孙悦被禁用策略挡在 CRM 外：%s", body)
	}
	// 分类和精选
	code, body = admin.do("GET", "/api/admin/hub/categories", nil, nil)
	expect(t, "示例分类", code, 200, body)
	if n := len(decodeAs[struct{ Categories []any }](t, body).Categories); n != 3 {
		t.Fatalf("应有 3 个示例分类：%s", body)
	}
	if !slices.Contains(cat.skillNames(), "meeting-minutes") {
		t.Fatalf("自动生效的技能应下发：%v", cat.skillNames())
	}

	// 微应用指令里带着知识库规则和工作台
	code, body, _ = dsh.do("GET", "/api/dsh/skills/travel-expense", nil, nil)
	expect(t, "读微应用", code, 200, body)
	for _, want := range []string{"差旅与报销制度（示例）", "## 工作台", "ee_workbench", "demo-travel__", "https://ee.example.com/knowledge/docs/<文档编号>?v=<版本>#L<起始行>-<结束行>"} {
		if !strings.Contains(decodeAs[struct{ Content string }](t, body).Content, want) {
			t.Fatalf("微应用指令里应有 %q", want)
		}
	}

	// 演示差旅系统：读汇总、提交一张、列表里能看到
	call := func(tool string, args map[string]any) (string, bool) {
		r := dsh.rpc("tools/call", map[string]any{"name": "demo-travel__" + tool, "arguments": args})
		if r["error"] != nil {
			t.Fatalf("%s：%v", tool, r["error"])
		}
		out := r["result"].(map[string]any)
		isErr, _ := out["isError"].(bool)
		return out["content"].([]any)[0].(map[string]any)["text"].(string), isErr
	}
	text, isErr := call("get_summary", nil)
	var sum struct {
		Total      float64
		BudgetLeft float64 `json:"budget_left"`
		Pending    int
	}
	if isErr || json.Unmarshal([]byte(text), &sum) != nil || sum.Pending < 1 || sum.Total+sum.BudgetLeft != 50000 {
		t.Fatalf("汇总不对：%s", text)
	}
	text, _ = call("list_claims", map[string]any{"status": "异常"})
	for _, want := range []string{"住宿超标", "发票号与", "缺住宿发票"} {
		if !strings.Contains(text, want) {
			t.Fatalf("报销单里应有异常单据「%s」：%s", want, text)
		}
	}
	text, _ = call("monthly", nil)
	var monthly struct{ Items []struct{ Amount float64 } }
	if json.Unmarshal([]byte(text), &monthly) != nil || len(monthly.Items) != 6 || monthly.Items[5].Amount <= monthly.Items[0].Amount {
		t.Fatalf("近 6 个月应有上涨的趋势：%s", text)
	}
	text, isErr = call("create_claim", map[string]any{"body": map[string]any{"applicant": "张晓", "city": "杭州", "days": 2, "amount": 1260, "reason": "客户培训"}})
	if isErr || !strings.Contains(text, "待审批") {
		t.Fatalf("提交报销单失败：%s", text)
	}
	text, _ = call("list_claims", map[string]any{"status": "待审批"})
	if !strings.Contains(text, "客户培训") {
		t.Fatalf("新提交的单子应在待审批列表里：%s", text)
	}

	// CRM 与 IT 工单
	callOn := func(conn, tool string, args map[string]any) string {
		// CRM 用销售部的周航调：研发部被禁用策略挡着
		d := dsh
		if conn == "demo-crm" {
			d = zhou
		}
		r := d.rpc("tools/call", map[string]any{"name": conn + "__" + tool, "arguments": args})
		out, _ := r["result"].(map[string]any)
		if r["error"] != nil || out == nil || out["isError"] == true {
			t.Fatalf("%s__%s：%v", conn, tool, r)
		}
		return out["content"].([]any)[0].(map[string]any)["text"].(string)
	}
	var crm struct {
		Customers int
		HighRisk  int `json:"high_risk"`
	}
	if text := callOn("demo-crm", "get_summary", nil); json.Unmarshal([]byte(text), &crm) != nil || crm.Customers != 50 || crm.HighRisk < 3 {
		t.Fatalf("CRM 应有 50 个客户和几个高风险客户：%s", text)
	}
	if text := callOn("demo-crm", "list_renewals", map[string]any{"days": 60}); !strings.Contains(text, `"risk":"高"`) || !strings.Contains(text, "华东精密制造") {
		t.Fatalf("续约提醒里应有高风险客户：%s", text)
	}
	// 客户详情前后一致：行业对得上名字，续约日 = 在用合同的到期日，最近拜访 = 最新一条拜访记录
	for _, id := range []string{"C001", "C005", "C018", "C002"} {
		var c struct {
			Name, Industry string
			RenewAt        string `json:"renew_at"`
			LastVisit      string `json:"last_visit"`
			ARR            float64
			Contracts      []struct {
				End    string
				Amount float64
			}
			Visits []struct{ Date string }
		}
		text := callOn("demo-crm", "get_customer", map[string]any{"id": id})
		if json.Unmarshal([]byte(text), &c) != nil || len(c.Contracts) == 0 || len(c.Visits) == 0 {
			t.Fatalf("客户详情应带合同和拜访记录：%s", text)
		}
		for _, ct := range c.Contracts {
			if ct.End != c.RenewAt {
				t.Fatalf("%s 的合同到期日应等于续约日 %s：%s", c.Name, c.RenewAt, text)
			}
		}
		if c.Visits[0].Date != c.LastVisit {
			t.Fatalf("%s 的最近拜访应等于最新一条拜访记录：%s", c.Name, text)
		}
		if id == "C001" && (c.Industry != "制造" || c.ARR < 500000) {
			t.Fatalf("华东精密制造应是制造业的大客户：%s", text)
		}
		if id == "C005" && c.Industry != "金融" {
			t.Fatalf("瑞丰银行应是金融业：%s", text)
		}
	}
	// IT 主管查「我的工单」看到的是派给他的整个积压；没有报销单的员工不借别人的
	if text := callOn("demo-itsm", "list_tickets", map[string]any{"requester": "刘洋", "status": "未解决"}); !strings.Contains(text, "新员工电脑还没领到") {
		t.Fatalf("IT 主管应看到派给他的工单：%s", text)
	}
	if text, _ := call("list_claims", map[string]any{"applicant": "陈一"}); strings.Contains(text, "BX-") {
		t.Fatalf("陈一没有报销单，不应借别人的：%s", text)
	}
	if text := callOn("demo-itsm", "list_tickets", map[string]any{"requester": "郑小雨"}); !strings.Contains(text, "新员工电脑还没领到") {
		t.Fatalf("新人的工单：%s", text)
	}
	if text := callOn("demo-itsm", "create_ticket", map[string]any{"body": map[string]any{"title": "VPN 证书过期", "requester": "赵磊", "category": "vpn"}}); !strings.Contains(text, "待处理") {
		t.Fatalf("建工单：%s", text)
	}
	if text := callOn("demo-itsm", "list_categories", nil); !strings.Contains(text, "VPN 连不上") {
		t.Fatalf("故障分类：%s", text)
	}

	// 自己用的应用：只在作者自己的 DSH 里
	if slices.Contains(apps, "zhouhang-weekly") {
		t.Fatal("别人自己用的应用不应出现在赵磊的 DSH 里")
	}

	// 自带数据的应用：吴敏分享给全公司的团建报名，孙悦只看到自己的报名
	if !slices.Contains(apps, "team-outing") {
		t.Fatalf("团建报名应出现在全员的 DSH 里：%v", apps)
	}
	if out, isErr, text := dsh.appCall("rows", map[string]any{"app": "team-outing", "table": "signups"}); isErr || out["total"] != float64(1) || !strings.Contains(text, "不吃辣") {
		t.Fatalf("孙悦应只看到自己的 1 条报名：%s", text)
	}
	if out, isErr, text := dsh.appCall("rows", map[string]any{"app": "team-outing", "table": "notes"}); isErr || out["total"] != float64(2) {
		t.Fatalf("公告全员可读：%s", text)
	}
	if _, isErr, _ := dsh.appCall("write", map[string]any{"action": "insert", "app": "team-outing", "table": "signups", "rows": []map[string]any{{"session": "周六上午 · 徒步", "people": 1}}}); isErr {
		t.Fatal("孙悦应能再报一场")
	}

	// 30 天模拟使用数据：月度报告与运营数据一打开就有内容
	code, body = admin.do("GET", "/api/admin/report/monthly?month="+time.Now().UTC().Add(8*time.Hour).Format("2006-01")+"&tz=480", nil, nil)
	expect(t, "月度报告", code, 200, body)
	var rep struct {
		Report struct {
			ActiveUsers int `json:"active_users"`
			AppRuns     int `json:"app_runs"`
			Questions   []struct {
				Query  string
				Misses int
			}
		}
	}
	_ = json.Unmarshal(body, &rep)
	if rep.Report.ActiveUsers < 5 || rep.Report.AppRuns == 0 || len(rep.Report.Questions) == 0 {
		t.Fatalf("模拟数据应让报告有内容：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/stats?days=30&tz=480", nil, nil)
	expect(t, "运营数据", code, 200, body)
	if !strings.Contains(string(body), `"tokens":`) || strings.Contains(string(body), `"calls":0,"call_errors":0,"tokens":0,"users"`) {
		t.Fatalf("运营数据应有模型调用：%.300s", body)
	}

	// 演示接口不认外来调用
	code, body = newClient(t, ts.URL).do("GET", "/demo/travel/claims", nil, nil)
	expect(t, "直接访问演示接口", code, 401, body)

	// 知识库能查到住宿标准
	out, isErr, text := dsh.kbCall("search", map[string]any{"query": "住宿 一类城市"})
	if isErr || !strings.Contains(text, "差旅费管理办法") {
		t.Fatalf("知识库检索不到制度：%s", text)
	}
	_ = out

	// 员工不能导入
	code, body = zhang.do("POST", "/api/admin/demo", nil, nil)
	if code != 403 {
		t.Fatalf("员工导入示例应被拒：%d %s", code, body)
	}
	// ---- 一键清除：示例全删，用户自己建的一条不少；再导入和第一次一样 ----
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "real.user", "password": "real-password-1", "department": "销售部"}, nil)
	expect(t, "用户自己建的账号", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "prompt", "slug": "my-own-prompt", "spec": map[string]any{"title": "我的提示词", "description": "用户自己的", "body": "写日报"}}, nil)
	expect(t, "用户自己建的提示词", code, 201, body)
	code, body = admin.do("POST", "/api/kb", map[string]any{"slug": "my-own-kb", "title": "我的知识库", "description": "", "scope": map[string]any{"all": true}}, nil)
	expect(t, "用户自己建的知识库", code, 201, body)
	code, body = zhang.do("DELETE", "/api/admin/demo", nil, nil)
	expect(t, "员工不能清除", code, 403, body)
	code, body = admin.do("DELETE", "/api/admin/demo", nil, nil)
	expect(t, "清除示例", code, 200, body)
	cleared := decodeAs[struct{ Assets, KBases, Users int }](t, body)
	if cleared.Assets != 18 || cleared.KBases != 6 || cleared.Users != 10 {
		t.Fatalf("应删 18 项能力、6 个库、10 个账号：%s", body)
	}
	// 导入时建的分类和禁用策略一起删掉
	code, body = admin.do("GET", "/api/admin/hub/categories", nil, nil)
	expect(t, "清除后的分类", code, 200, body)
	if n := len(decodeAs[struct{ Categories []any }](t, body).Categories); n != 0 {
		t.Fatalf("清除后不应还有示例分类：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/hub/blocks", nil, nil)
	expect(t, "清除后的禁用策略", code, 200, body)
	if n := len(decodeAs[struct{ Blocks []any }](t, body).Blocks); n != 0 {
		t.Fatalf("清除后不应还有示例禁用策略：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/demo", nil, nil)
	expect(t, "清除后的状态", code, 200, body)
	if decodeAs[struct{ Present bool }](t, body).Present {
		t.Fatalf("清除后不应还有示例：%s", body)
	}
	for _, path := range []string{"/api/hub", "/api/kb", "/api/admin/users"} {
		code, body = admin.do("GET", path, nil, nil)
		expect(t, path, code, 200, body)
		for _, want := range []string{"my-own-prompt", "my-own-kb", "real.user"} {
			if (path == "/api/hub" && want == "my-own-prompt") || (path == "/api/kb" && want == "my-own-kb") || (path == "/api/admin/users" && want == "real.user") {
				if !strings.Contains(string(body), want) {
					t.Fatalf("用户自己的 %s 不应被删：%.300s", want, body)
				}
			}
		}
		for _, gone := range []string{"demo-crm", "travel-policy", "demo.zhou.hang", "zhouhang-weekly"} {
			if strings.Contains(string(body), `"`+gone+`"`) {
				t.Fatalf("清除后 %s 里不应还有 %s", path, gone)
			}
		}
	}
	code, body = admin.do("GET", "/api/admin/report/monthly?month="+time.Now().UTC().Add(8*time.Hour).Format("2006-01")+"&tz=480", nil, nil)
	_ = json.Unmarshal(body, &rep)
	if rep.Report.AppRuns != 0 {
		t.Fatalf("清除后模拟数据应一起删掉：%s", body)
	}
	var leftRows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM app_rows").Scan(&leftRows); err != nil || leftRows != 0 {
		t.Fatalf("清除后示例应用的数据应一起删掉：%d %v", leftRows, err)
	}
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "清除后再导入", code, 200, body)
	again2 := decodeAs[result](t, body)
	if len(again2.Items) != 34 || len(again2.Accounts) != 10 {
		t.Fatalf("清除后再导入应和第一次一样：%s", body)
	}
	for _, it := range again2.Items {
		if it.Result != "created" {
			t.Fatalf("清除后再导入 %s 应重新建：%s %s", it.Slug, it.Result, it.Error)
		}
	}

}
