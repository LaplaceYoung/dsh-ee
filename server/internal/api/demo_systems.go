package api

import (
	"crypto/subtle"
	"fmt"
	"math/rand"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 示例公司「星河科技」的演示业务系统：差旅、CRM、IT 工单。数据在内存里、由固定种子生成，
// 有趋势、有异常值，看起来像真的；重启后回到初始数据，示例环境不会被试坏。设定见 docs/demo-company.md。

const (
	demoCRMConn  = "demo-crm"
	demoITSMConn = "demo-itsm"
)

// demoAuthorizedFor 核对演示连接带来的密钥：只有导入示例时生成、存进那个连接凭据里的值能用。
func (s *Server) demoAuthorizedFor(w http.ResponseWriter, r *http.Request, conn string) bool {
	got := r.Header.Get(demoKeyHeader)
	if got != "" {
		if a, err := s.store.GetAssetBySlug(r.Context(), conn); err == nil {
			if sealed, err := s.store.AssetSecret(r.Context(), a.ID); err == nil && len(sealed) > 0 {
				if want, err := s.box.Open(sealed); err == nil && subtle.ConstantTimeCompare(want, []byte(got)) == 1 {
					return true
				}
			}
		}
	}
	writeErr(w, http.StatusUnauthorized, "演示系统只接受 dsh-ee 示例连接的调用")
	return false
}

func dayStr(t time.Time) string { return t.Format("2006-01-02") }

// ---- 差旅：近 6 个月的报销单，含超标、重复发票、缺票据等异常 ----

var demoTravelPeople = []struct{ name, dept string }{
	{"周航", "销售部"}, {"林娜", "销售部"}, {"赵磊", "研发部"}, {"孙悦", "研发部"}, {"何静", "财务部"},
	{"吴敏", "人力资源部"}, {"刘洋", "IT 部"}, {"许诺", "法务部"},
}

var demoCities = []struct {
	name  string
	hotel float64 // 普通员工住宿标准（与《差旅费管理办法》一致）
}{{"上海", 550}, {"北京", 550}, {"深圳", 550}, {"广州", 550}, {"杭州", 450}, {"南京", 450}, {"成都", 450}, {"武汉", 450}, {"西安", 450}, {"长沙", 350}}

var demoTripReasons = []string{"拜访客户", "续约谈判", "行业展会", "供应商审核", "项目交付", "区域培训", "客户上线支持", "招聘宣讲"}

func (d *demoTravel) list() []demoClaim {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claims == nil {
		d.claims, d.next = demoTravelClaims(time.Now()), 2001
	}
	return append([]demoClaim(nil), d.claims...)
}

func demoTravelClaims(now time.Time) []demoClaim {
	rng := rand.New(rand.NewSource(20261107))
	var out []demoClaim
	id := 1001
	// 每个月的单数逐月增加（业务在扩张），越近的月份越多
	for back := 5; back >= 0; back-- {
		n := 4 + (5-back)*2
		for i := 0; i < n; i++ {
			p := demoTravelPeople[rng.Intn(len(demoTravelPeople))]
			if rng.Intn(3) == 0 {
				p = demoTravelPeople[rng.Intn(2)] // 销售出差最多
			}
			city := demoCities[rng.Intn(len(demoCities))]
			days := 1 + rng.Intn(4)
			hotel := city.hotel * (0.75 + rng.Float64()*0.3)
			fare := 400 + rng.Float64()*1400
			amount := float64(int(hotel*float64(days)+fare+float64(days)*100) / 10 * 10)
			submitted := now.AddDate(0, -back, 0).AddDate(0, 0, -rng.Intn(25))
			if submitted.After(now) {
				submitted = now
			}
			c := demoClaim{
				ID: fmt.Sprintf("BX-%d", id), Applicant: p.name, Department: p.dept, City: city.name, Days: days,
				Amount: amount, Hotel: float64(int(hotel)), Reason: demoTripReasons[rng.Intn(len(demoTripReasons))],
				Status: "已通过", Submitted: dayStr(submitted), Invoice: fmt.Sprintf("FP%08d", 31000000+id*7), Flags: []string{},
			}
			if back == 0 && rng.Intn(3) == 0 {
				c.Status = "待审批"
			} else if back <= 1 && rng.Intn(6) == 0 {
				c.Status = "已驳回"
			}
			out = append(out, c)
			id++
		}
	}
	// 异常单据：超标住宿、重复发票、缺票据、周末提交的大额单
	add := func(c demoClaim) {
		c.ID = fmt.Sprintf("BX-%d", id)
		id++
		out = append(out, c)
	}
	add(demoClaim{Applicant: "周航", Department: "销售部", City: "上海", Days: 3, Hotel: 880, Amount: 4620, Reason: "大客户续约谈判（华东精密）", Status: "待审批", Submitted: dayStr(now.AddDate(0, 0, -2)), Invoice: "FP31099001", Flags: []string{"住宿超标：每晚 880 元，标准 550 元"}})
	add(demoClaim{Applicant: "林娜", Department: "销售部", City: "深圳", Days: 2, Hotel: 520, Amount: 2380, Reason: "客户上线支持", Status: "待审批", Submitted: dayStr(now.AddDate(0, 0, -1)), Invoice: "FP31007070", Flags: []string{"发票号与 BX-1010 重复"}})
	add(demoClaim{Applicant: "赵磊", Department: "研发部", City: "北京", Days: 4, Hotel: 540, Amount: 5160, Reason: "项目交付", Status: "待补票据", Submitted: dayStr(now.AddDate(0, 0, -6)), Invoice: "", Flags: []string{"缺住宿发票"}})
	add(demoClaim{Applicant: "孙悦", Department: "研发部", City: "成都", Days: 5, Hotel: 430, Amount: 12800, Reason: "区域培训", Status: "待审批", Submitted: dayStr(now.AddDate(0, 0, -3)), Invoice: "FP31099017", Flags: []string{"金额明显高于同城同天数的平均水平"}})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Submitted > out[j].Submitted })
	return out
}

// GET /demo/travel/monthly：近 6 个月每月的报销金额与单数（不含驳回）。
func (s *Server) handleDemoMonthly(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorized(w, r) {
		return
	}
	now := time.Now()
	type month struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
		Count  int     `json:"count"`
	}
	items := make([]month, 6)
	idx := map[string]int{}
	for i := range items {
		m := now.AddDate(0, i-5, 0).Format("2006-01")
		items[i] = month{Month: m}
		idx[m] = i
	}
	for _, c := range s.demo.list() {
		if i, ok := idx[strings.TrimSpace(c.Submitted)[:7]]; ok && c.Status != "已驳回" {
			items[i].Amount += c.Amount
			items[i].Count++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// demoStandIn：来的人不是示例公司的员工时（比如管理员自己导入示例后来试），借一位示例员工的数据给他看，
// 免得「我的客户」「我的工单」全是 0。返回空表示不用借；借来的记录在返回里换成他本人的名字。
func demoStandIn(who string, people []string, standIn string) string {
	if who == "" || slices.Contains(people, who) || isDemoStaff(who) {
		return ""
	}
	return standIn
}

// isDemoStaff：是不是示例公司的员工（按姓名）。他们在某个系统里没有记录就是真的没有（新人没有报销单、
// 研发没有客户），不借别人的数据。
func isDemoStaff(name string) bool {
	for _, u := range demoAllUsers() {
		if u.DisplayName == name {
			return true
		}
	}
	return false
}

// demoMine：这条记录算不算 who 的（含借来的）。
func demoMine(name, who, alt string) bool {
	return who == "" || name == who || (alt != "" && name == alt)
}

var demoITSMPeople = []string{"周航", "林娜", "赵磊", "孙悦", "何静", "陈一", "吴敏", "郑小雨", "许诺"}

// ---- CRM：50 个客户、合同、拜访记录、续约日期 ----

type demoCustomer struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Industry   string          `json:"industry"`
	City       string          `json:"city"`
	Owner      string          `json:"owner"`
	ARR        float64         `json:"arr"`
	Stage      string          `json:"stage"`
	RenewAt    string          `json:"renew_at"`
	LastVisit  string          `json:"last_visit"`
	Health     int             `json:"health"`
	Risk       string          `json:"risk"`
	RiskReason string          `json:"risk_reason,omitempty"`
	Contact    string          `json:"contact"`
	Contracts  []demoContract  `json:"contracts,omitempty"`
	Visits     []demoVisitNote `json:"visits,omitempty"`
}

type demoContract struct {
	ID      string  `json:"id"`
	Product string  `json:"product"`
	Amount  float64 `json:"amount"`
	Start   string  `json:"start"`
	End     string  `json:"end"`
}

type demoVisitNote struct {
	Date    string `json:"date"`
	By      string `json:"by"`
	Summary string `json:"summary"`
}

type demoCRM struct {
	mu        sync.Mutex
	customers []demoCustomer
}

var (
	demoCRMNames = []string{
		"华东精密制造", "远航物流", "青禾教育", "安和医疗", "瑞丰银行", "云杉零售", "北辰汽车零部件", "海川食品", "德茂化工", "启明软件",
		"嘉禾地产", "恒信证券", "绿源农业", "星火电子", "长风航空服务", "明德学校", "康泰药业", "金穗粮油", "天成建设", "博远咨询",
		"华彩印务", "东升纺织", "睿思科技", "鼎力机械", "悦家家居", "九州通信", "新桥物业", "银河保险", "百川环保", "锦程旅游",
		"中科仪器", "万象商业", "润泽水务", "凌云光电", "盛世传媒", "和光新能源", "同舟船舶", "立方数据", "白鹭酒店集团", "汇通支付",
		"峰岭矿业", "清源检测", "雪域乳业", "光谷半导体", "南山体育", "紫金出版", "瀚海贸易", "如意服饰", "晨曦养老", "远景风电",
	}
	// 行业和客户名一一对应（顺序同 demoCRMNames），免得「银行」被随机成「教育」
	demoIndustries = []string{
		"制造", "物流", "教育", "医疗", "金融", "零售", "制造", "食品", "化工", "软件",
		"地产", "金融", "农业", "制造", "交通", "教育", "医疗", "食品", "建筑", "咨询",
		"制造", "制造", "软件", "制造", "零售", "通信", "地产", "金融", "环保", "文旅",
		"制造", "零售", "公用事业", "制造", "传媒", "能源", "制造", "软件", "文旅", "金融",
		"能源", "咨询", "食品", "制造", "文旅", "传媒", "贸易", "零售", "医疗", "能源",
	}
	demoCRMCities  = []string{"上海", "杭州", "南京", "苏州", "深圳", "广州", "北京", "成都", "武汉", "西安"}
	demoCRMOwners  = []string{"周航", "林娜", "马骁", "杜若"}
	demoProducts   = []string{"星河云·协同版", "星河云·企业版", "星河数据中台", "星河智能客服"}
	demoVisitLines = []string{"确认二期需求，客户关心数据迁移周期", "季度回访，使用率稳定，提出单点登录需求", "客户换了 IT 负责人，需要重新建立关系", "演示智能客服模块，客户有兴趣试用", "处理上月工单投诉，客户满意度回升", "讨论续约价格，客户希望打九折", "陪同上线验收，签署验收单"}
)

func (c *demoCRM) list() []demoCustomer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.customers == nil {
		c.customers = demoCRMCustomers(time.Now())
	}
	return c.customers
}

// demoVisitAgo 是一条拜访记录：多少天前、聊了什么。
type demoVisitAgo struct {
	ago  int
	note string
}

// demoRiskCustomer 是续约风险高的几个客户：快到期、很久没拜访、健康度低。合同和拜访记录按这里的日期生成，
// 所以「续约日期 = 在用合同的到期日」「最近拜访 = 最新一条拜访记录」对得上。
type demoRiskCustomer struct {
	days, visit, health int
	// 年合同额（元）；0 表示用随机出来的
	arr    float64
	reason string
	// 拜访记录，第一条就是最近一次（天数和 visit 一致）
	visits []demoVisitAgo
}

// 按 demoCRMNames 的下标
var demoRiskCustomers = map[int]demoRiskCustomer{
	0: {21, 75, 38, 680000, "合同快到期，两个多月没拜访；对接人刚换，使用率下降 40%", []demoVisitAgo{
		{75, "季度回访：原来的 IT 对接人离职，新对接人还没见到；协同版活跃人数比上季度少了约四成"},
		{140, "讨论续约价格，客户希望打九折"},
	}},
	4: {35, 52, 45, 0, "合同快到期；上月有 3 张未解决的投诉工单", []demoVisitAgo{
		{52, "客户反馈上月提的 3 张工单还没解决，要求给出处理时间"},
		{83, "季度回访，使用率稳定，提出单点登录需求"},
	}},
	9: {12, 40, 41, 0, "合同快到期；客户在评估竞品，要求降价 20%", []demoVisitAgo{
		{40, "客户说在评估竞品，要求续约降价 20%；答应回去申请"},
		{71, "演示智能客服模块，客户有兴趣试用"},
	}},
	17: {48, 90, 50, 0, "合同快到期，三个月没拜访", []demoVisitAgo{
		{90, "陪同上线验收，签署验收单；之后没有再回访"},
	}},
	38: {27, 63, 44, 0, "合同快到期；付款延迟两个月", []demoVisitAgo{
		{63, "催收上期尾款，客户财务说流程在走，已经延迟两个月"},
		{95, "季度回访，使用率稳定"},
	}},
}

func demoCRMCustomers(now time.Time) []demoCustomer {
	rng := rand.New(rand.NewSource(7))
	out := make([]demoCustomer, 0, len(demoCRMNames))
	for i, name := range demoCRMNames {
		// 行业按客户名定；这次随机数照旧取掉，别的字段才不会跟着变
		_ = rng.Intn(6)
		c := demoCustomer{
			ID: fmt.Sprintf("C%03d", i+1), Name: name, Industry: demoIndustries[i],
			City: demoCRMCities[rng.Intn(len(demoCRMCities))], Owner: demoCRMOwners[i%len(demoCRMOwners)],
			Contact: []string{"王总", "李经理", "陈主任", "刘总监", "张经理"}[rng.Intn(5)],
		}
		// 金额长尾：少数大客户
		c.ARR = float64((5 + rng.Intn(60)) * 10000)
		if rng.Intn(8) == 0 {
			c.ARR = float64((120 + rng.Intn(200)) * 10000)
		}
		renew := now.AddDate(0, 0, -20+rng.Intn(380))
		last := now.AddDate(0, 0, -rng.Intn(120))
		c.Health = 55 + rng.Intn(45)
		// 已经过了续约日的算续上了，合同顺延一年
		if renew.Before(now.AddDate(0, 0, 7)) {
			renew = renew.AddDate(1, 0, 0)
		}
		risk, risky := demoRiskCustomers[i]
		if risky {
			renew, last, c.Health, c.RiskReason = now.AddDate(0, 0, risk.days), now.AddDate(0, 0, -risk.visit), risk.health, risk.reason
			if risk.arr > 0 {
				c.ARR = risk.arr
			}
		}
		c.RenewAt, c.LastVisit = dayStr(renew), dayStr(last)
		c.Stage = "合作中"
		if renew.Before(now.AddDate(0, 0, 90)) {
			c.Stage = "续约中"
		}
		if i%11 == 3 && !risky {
			c.Stage = "新客户"
		}
		// 在用的合同：同一个合同期（到续约日为止的一年），一到三个产品，金额加起来是年合同额。
		// 协同版和企业版二选一，再加数据中台、智能客服
		nContracts := 1 + rng.Intn(3)
		products := []string{demoProducts[i%2], demoProducts[2], demoProducts[3]}
		start := renew.AddDate(-1, 0, 1)
		for k := 0; k < nContracts; k++ {
			c.Contracts = append(c.Contracts, demoContract{ID: fmt.Sprintf("HT-%s-%d", c.ID, k+1), Product: products[k],
				Amount: float64(int(c.ARR/float64(nContracts)/1000) * 1000), Start: dayStr(start), End: dayStr(renew)})
		}
		for k := 0; k < rng.Intn(4); k++ {
			c.Visits = append(c.Visits, demoVisitNote{Date: dayStr(last.AddDate(0, 0, -30*k)), By: c.Owner, Summary: demoVisitLines[rng.Intn(len(demoVisitLines))]})
		}
		if risky {
			c.Visits = nil
			for _, v := range risk.visits {
				c.Visits = append(c.Visits, demoVisitNote{Date: dayStr(now.AddDate(0, 0, -v.ago)), By: c.Owner, Summary: v.note})
			}
		}
		// 「最近拜访」有日期，就至少有那一条记录
		if len(c.Visits) == 0 {
			c.Visits = []demoVisitNote{{Date: c.LastVisit, By: c.Owner, Summary: demoVisitLines[i%len(demoVisitLines)]}}
		}
		switch {
		case c.Health < 52:
			c.Risk = "高"
		case c.Health < 70:
			c.Risk = "中"
		default:
			c.Risk = "低"
		}
		out = append(out, c)
	}
	return out
}

func customerBrief(c demoCustomer) demoCustomer {
	c.Contracts, c.Visits = nil, nil
	return c
}

// GET /demo/crm/summary?owner=：客户数、年合同额、90 天内续约、高风险客户数。
func (s *Server) handleDemoCRMSummary(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoCRMConn) {
		return
	}
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	alt := demoStandIn(owner, demoCRMOwners, demoCRMOwners[0])
	limit := time.Now().AddDate(0, 0, 90).Format("2006-01-02")
	var n, renewals, high int
	var arr float64
	for _, c := range s.crm.list() {
		if !demoMine(c.Owner, owner, alt) {
			continue
		}
		n++
		arr += c.ARR
		if c.RenewAt <= limit {
			renewals++
		}
		if c.Risk == "高" {
			high++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": n, "arr": arr, "renewals_90d": renewals, "high_risk": high})
}

// GET /demo/crm/customers?owner=&risk=&q=：客户列表，按续约日期排序。
func (s *Server) handleDemoCRMCustomers(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoCRMConn) {
		return
	}
	q := r.URL.Query()
	owner, risk, kw := strings.TrimSpace(q.Get("owner")), strings.TrimSpace(q.Get("risk")), strings.TrimSpace(q.Get("q"))
	alt := demoStandIn(owner, demoCRMOwners, demoCRMOwners[0])
	items := []demoCustomer{}
	for _, c := range s.crm.list() {
		if demoMine(c.Owner, owner, alt) && (risk == "" || c.Risk == risk) && (kw == "" || strings.Contains(c.Name, kw) || strings.Contains(c.Industry, kw)) {
			c = customerBrief(c)
			if alt != "" {
				c.Owner = owner
			}
			items = append(items, c)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].RenewAt < items[j].RenewAt })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /demo/crm/customers/{id}：客户详情，含合同与拜访记录。
func (s *Server) handleDemoCRMCustomer(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoCRMConn) {
		return
	}
	id := r.PathValue("id")
	for _, c := range s.crm.list() {
		if c.ID == id || c.Name == id {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "没有这个客户")
}

// GET /demo/crm/renewals?days=90：多少天内要续约的客户，风险高的在前。
func (s *Server) handleDemoCRMRenewals(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoCRMConn) {
		return
	}
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days <= 0 || days > 365 {
		days = 90
	}
	limit := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	alt := demoStandIn(owner, demoCRMOwners, demoCRMOwners[0])
	items := []demoCustomer{}
	for _, c := range s.crm.list() {
		if c.RenewAt <= limit && demoMine(c.Owner, owner, alt) {
			c = customerBrief(c)
			if alt != "" {
				c.Owner = owner
			}
			items = append(items, c)
		}
	}
	rank := map[string]int{"高": 0, "中": 1, "低": 2}
	sort.SliceStable(items, func(i, j int) bool {
		if rank[items[i].Risk] != rank[items[j].Risk] {
			return rank[items[i].Risk] < rank[items[j].Risk]
		}
		return items[i].RenewAt < items[j].RenewAt
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /demo/crm/visits：记一条拜访（写操作，员工确认后才执行）。
func (s *Server) handleDemoCRMVisit(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoCRMConn) {
		return
	}
	var req struct {
		CustomerID string `json:"customer_id"`
		By         string `json:"by"`
		Summary    string `json:"summary"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Summary, req.By = strings.TrimSpace(req.Summary), strings.TrimSpace(req.By)
	if req.Summary == "" || req.By == "" {
		writeErr(w, http.StatusBadRequest, "拜访人和纪要必填")
		return
	}
	s.crm.list()
	s.crm.mu.Lock()
	defer s.crm.mu.Unlock()
	for i := range s.crm.customers {
		c := &s.crm.customers[i]
		if c.ID == req.CustomerID {
			v := demoVisitNote{Date: dayStr(time.Now()), By: truncateStr(req.By, 32), Summary: truncateStr(req.Summary, 300)}
			c.Visits = append([]demoVisitNote{v}, c.Visits...)
			if len(c.Visits) > 20 {
				c.Visits = c.Visits[:20]
			}
			c.LastVisit = v.Date
			writeJSON(w, http.StatusCreated, v)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "没有这个客户")
}

// ---- IT 工单：常见故障分类与工单，能新建工单（写操作走审批卡） ----

type demoTicket struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Requester string `json:"requester"`
	Assignee  string `json:"assignee"`
	Priority  string `json:"priority"`
	Status    string `json:"status"`
	Created   string `json:"created_at"`
	Updated   string `json:"updated_at"`
	Overdue   bool   `json:"overdue"`
	Detail    string `json:"detail"`
}

type demoITSM struct {
	mu      sync.Mutex
	tickets []demoTicket
	next    int
}

var demoTicketCategories = []struct{ id, name, selfHelp string }{
	{"vpn", "VPN 连不上", "IT 排障手册「VPN 连不上」：先确认 DSH 外的网页能打开，再重装客户端证书"},
	{"mail", "邮箱收发异常", "先看垃圾邮件和收件规则；邮箱容量超过 50 GB 要先归档。还不行就建工单"},
	{"printer", "打印机", "IT 排障手册「打印机」：3 楼 HP 打印机要先刷工牌"},
	{"account", "账号锁定 / 密码", "IT 排障手册「账号被锁」：连续输错 5 次锁 15 分钟，可在自助门户解锁"},
	{"laptop", "电脑卡慢 / 故障", "IT 排障手册「电脑卡慢」：先看磁盘剩余空间，低于 10% 清理下载目录"},
	{"software", "软件安装申请", "常用软件在自助软件中心安装；不在清单里的要走软件申请"},
	{"meeting", "会议室设备", "会议室投屏用 HDMI 或无线投屏器，遥控器在桌下抽屉"},
}

func (d *demoITSM) list() []demoTicket {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tickets == nil {
		d.tickets, d.next = demoTickets(time.Now()), 3100
	}
	return append([]demoTicket(nil), d.tickets...)
}

func demoTickets(now time.Time) []demoTicket {
	rng := rand.New(rand.NewSource(11))
	titles := map[string][]string{
		"vpn":      {"在家连不上 VPN", "VPN 连上后打不开内网", "VPN 客户端提示证书过期"},
		"mail":     {"收不到外部客户的邮件", "邮箱提示容量已满"},
		"printer":  {"3 楼打印机卡纸", "打印机找不到"},
		"account":  {"OA 账号被锁", "忘记邮箱密码"},
		"laptop":   {"电脑开机很慢", "笔记本屏幕闪烁", "风扇一直响"},
		"software": {"申请安装 Visio", "申请 IntelliJ IDEA 授权"},
		"meeting":  {"大会议室投屏没声音", "视频会议摄像头不工作"},
	}
	people := demoITSMPeople
	var out []demoTicket
	for i := 0; i < 26; i++ {
		cat := demoTicketCategories[rng.Intn(len(demoTicketCategories))]
		if rng.Intn(3) == 0 {
			cat = demoTicketCategories[0] // VPN 问题最多
		}
		created := now.Add(-time.Duration(rng.Intn(30*24)) * time.Hour)
		t := demoTicket{
			ID: fmt.Sprintf("IT-%d", 3000+i), Title: titles[cat.id][rng.Intn(len(titles[cat.id]))], Category: cat.name,
			Requester: people[rng.Intn(len(people))], Assignee: "刘洋", Priority: []string{"低", "中", "中", "高"}[rng.Intn(4)],
			Status: "已解决", Created: created.Format("2006-01-02 15:04"), Updated: created.Add(time.Duration(2+rng.Intn(30)) * time.Hour).Format("2006-01-02 15:04"),
		}
		age := now.Sub(created)
		if age < 72*time.Hour {
			t.Status = []string{"待处理", "处理中"}[rng.Intn(2)]
		} else if rng.Intn(9) == 0 {
			t.Status = "处理中"
		}
		// 超过 3 天还没解决的算超时
		t.Overdue = (t.Status == "待处理" || t.Status == "处理中") && age > 72*time.Hour
		out = append(out, t)
	}
	// 新人郑小雨的两张单子：入职场景用得到
	out = append(out,
		demoTicket{ID: "IT-3090", Title: "新员工电脑还没领到", Category: "电脑卡慢 / 故障", Requester: "郑小雨", Assignee: "刘洋", Priority: "中", Status: "处理中", Created: now.Add(-26 * time.Hour).Format("2006-01-02 15:04"), Updated: now.Add(-3 * time.Hour).Format("2006-01-02 15:04")},
		demoTicket{ID: "IT-3091", Title: "开通 GitLab 与 VPN 账号", Category: "账号锁定 / 密码", Requester: "郑小雨", Assignee: "刘洋", Priority: "中", Status: "待处理", Created: now.Add(-20 * time.Hour).Format("2006-01-02 15:04"), Updated: now.Add(-20 * time.Hour).Format("2006-01-02 15:04")},
	)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

// GET /demo/itsm/tickets?requester=&status=：工单列表，新的在前。
func (s *Server) handleDemoTickets(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoITSMConn) {
		return
	}
	q := r.URL.Query()
	who, status := strings.TrimSpace(q.Get("requester")), strings.TrimSpace(q.Get("status"))
	alt := demoStandIn(who, demoITSMPeople, "郑小雨")
	items := []demoTicket{}
	for _, t := range s.itsm.list() {
		open := t.Status == "待处理" || t.Status == "处理中"
		// 处理人查「我的工单」看到的是派给他的：IT 主管刘洋打开就是整个积压
		if (demoMine(t.Requester, who, alt) || t.Assignee == who) && (status == "" || t.Status == status || (status == "未解决" && open)) {
			if alt != "" {
				t.Requester = who
			}
			items = append(items, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /demo/itsm/summary：未解决、超时、近 7 天解决的工单数。
func (s *Server) handleDemoTicketSummary(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoITSMConn) {
		return
	}
	who := strings.TrimSpace(r.URL.Query().Get("requester"))
	alt := demoStandIn(who, demoITSMPeople, "郑小雨")
	week := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var open, overdue, resolved int
	for _, t := range s.itsm.list() {
		if !demoMine(t.Requester, who, alt) && t.Assignee != who {
			continue
		}
		switch {
		case t.Status == "待处理" || t.Status == "处理中":
			open++
			if t.Overdue {
				overdue++
			}
		case t.Updated >= week:
			resolved++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"open": open, "overdue": overdue, "resolved_7d": resolved})
}

// GET /demo/itsm/categories：常见故障分类、近 30 天的工单数与自助排障提示。
func (s *Server) handleDemoTicketCategories(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoITSMConn) {
		return
	}
	count := map[string]int{}
	for _, t := range s.itsm.list() {
		count[t.Category]++
	}
	type cat struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Count    int    `json:"count"`
		SelfHelp string `json:"self_help"`
	}
	items := []cat{}
	for _, c := range demoTicketCategories {
		items = append(items, cat{c.id, c.name, count[c.name], c.selfHelp})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /demo/itsm/tickets：新建工单（写操作，员工在审批卡上确认后才执行）。
func (s *Server) handleDemoCreateTicket(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorizedFor(w, r, demoITSMConn) {
		return
	}
	var req struct {
		Title     string `json:"title"`
		Category  string `json:"category"`
		Requester string `json:"requester"`
		Priority  string `json:"priority"`
		Detail    string `json:"detail"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Title, req.Requester = strings.TrimSpace(req.Title), strings.TrimSpace(req.Requester)
	if req.Title == "" || req.Requester == "" {
		writeErr(w, http.StatusBadRequest, "标题和报修人必填")
		return
	}
	if !slices.Contains([]string{"低", "中", "高"}, req.Priority) {
		req.Priority = "中"
	}
	cat := "其他"
	for _, c := range demoTicketCategories {
		if c.id == req.Category || c.name == req.Category {
			cat = c.name
		}
	}
	s.itsm.list()
	s.itsm.mu.Lock()
	defer s.itsm.mu.Unlock()
	now := time.Now().Format("2006-01-02 15:04")
	t := demoTicket{ID: fmt.Sprintf("IT-%d", s.itsm.next), Title: truncateStr(req.Title, 80), Category: cat, Requester: truncateStr(req.Requester, 32),
		Assignee: "刘洋", Priority: req.Priority, Status: "待处理", Created: now, Updated: now, Detail: truncateStr(strings.TrimSpace(req.Detail), 500)}
	s.itsm.next++
	s.itsm.tickets = append([]demoTicket{t}, s.itsm.tickets...)
	if len(s.itsm.tickets) > 80 {
		s.itsm.tickets = s.itsm.tickets[:80]
	}
	writeJSON(w, http.StatusCreated, t)
}
