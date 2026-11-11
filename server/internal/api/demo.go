package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例内容：管理员在概览页一键导入两个知识库、一个演示用的差旅系统连接、技能、提示词、
// 两个带工作台的微应用和两个演示员工，让第一次用的人能直接看到每样东西在 DSH 里是什么样。
// 全部走和网页上手动创建一样的存储路径；重复导入时已有的标识跳过，不覆盖。

const (
	demoConn      = "demo-travel"
	demoKeyHeader = "X-Demo-Key"
	demoCategory  = "示例"
)

// demoTravel 是演示用差旅系统的内存数据：重启后回到初始几张单子。
type demoTravel struct {
	mu     sync.Mutex
	claims []demoClaim
	next   int
}

type demoClaim struct {
	ID         string  `json:"id"`
	Applicant  string  `json:"applicant"`
	Department string  `json:"department"`
	City       string  `json:"city"`
	Days       int     `json:"days"`
	Amount     float64 `json:"amount"`
	// 住宿每晚（元）与发票号；Flags 是系统自动发现的异常
	Hotel     float64  `json:"hotel"`
	Invoice   string   `json:"invoice"`
	Reason    string   `json:"reason"`
	Status    string   `json:"status"`
	Submitted string   `json:"submitted_at"`
	Flags     []string `json:"flags"`
}

const demoBudget = 50000

func (d *demoTravel) add(c demoClaim) demoClaim {
	d.list()
	d.mu.Lock()
	defer d.mu.Unlock()
	c.ID = fmt.Sprintf("BX-%d", d.next)
	d.next++
	c.Status, c.Submitted, c.Flags = "待审批", time.Now().Format("2006-01-02"), []string{}
	// 演示数据只留最近 50 张，免得被反复提交撑大
	d.claims = append([]demoClaim{c}, d.claims...)
	if len(d.claims) > 50 {
		d.claims = d.claims[:50]
	}
	return c
}

// demoAuthorized 核对差旅演示系统的密钥，见 demoAuthorizedFor。
func (s *Server) demoAuthorized(w http.ResponseWriter, r *http.Request) bool {
	return s.demoAuthorizedFor(w, r, demoConn)
}

// GET /demo/travel/summary：本月差旅报销汇总。
func (s *Server) handleDemoSummary(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorized(w, r) {
		return
	}
	var total float64
	count, pending, flagged := 0, 0, 0
	month := time.Now().Format("2006-01")
	for _, c := range s.demo.list() {
		if len(c.Flags) > 0 && c.Status != "已通过" && c.Status != "已驳回" {
			flagged++
		}
		if c.Status == "已驳回" || !strings.HasPrefix(c.Submitted, month) {
			continue
		}
		total += c.Amount
		count++
		if c.Status == "待审批" {
			pending++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month": month, "total": total, "count": count, "pending": pending, "flagged": flagged,
		"budget": demoBudget, "budget_left": demoBudget - total,
	})
}

// GET /demo/travel/claims：报销单列表，可按状态过滤。
func (s *Server) handleDemoClaims(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorized(w, r) {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	who := strings.TrimSpace(r.URL.Query().Get("applicant"))
	people := make([]string, 0, len(demoTravelPeople))
	for _, p := range demoTravelPeople {
		people = append(people, p.name)
	}
	alt := demoStandIn(who, people, demoTravelPeople[0].name)
	items := []demoClaim{}
	for _, c := range s.demo.list() {
		if (status == "" || c.Status == status || (status == "异常" && len(c.Flags) > 0)) && demoMine(c.Applicant, who, alt) {
			if alt != "" && c.Applicant == alt {
				c.Applicant = who
			}
			items = append(items, c)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /demo/travel/claims：提交一张报销单（演示用，只存在内存里）。
func (s *Server) handleDemoCreateClaim(w http.ResponseWriter, r *http.Request) {
	if !s.demoAuthorized(w, r) {
		return
	}
	var req struct {
		Applicant string  `json:"applicant"`
		City      string  `json:"city"`
		Days      int     `json:"days"`
		Amount    float64 `json:"amount"`
		Reason    string  `json:"reason"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Applicant, req.City, req.Reason = strings.TrimSpace(req.Applicant), strings.TrimSpace(req.City), strings.TrimSpace(req.Reason)
	if req.Applicant == "" || req.City == "" || req.Days <= 0 || req.Days > 60 || req.Amount <= 0 || req.Amount > 1e6 {
		writeErr(w, http.StatusBadRequest, "申请人、城市必填，天数 1～60，金额要大于 0")
		return
	}
	c := s.demo.add(demoClaim{Applicant: truncateStr(req.Applicant, 32), City: truncateStr(req.City, 32), Days: req.Days, Amount: req.Amount, Reason: truncateStr(req.Reason, 200)})
	writeJSON(w, http.StatusCreated, c)
}

// demoLoopback 是 dsh-ee 访问自己的地址：演示连接从服务端调演示接口，不经过员工的网络。
func (s *Server) demoLoopback(r *http.Request) string {
	if host, port, err := net.SplitHostPort(s.opts.Addr); err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://" + r.Host
}

type demoItem struct {
	Kind  string `json:"kind"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// created：这次新建 / exists：已经有了，没动 / failed：没建成，见 error
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

type demoAccount struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Department  string `json:"department"`
	Title       string `json:"title"`
	// 只在新建时返回一次
	Password string `json:"password,omitempty"`
}

// GET /api/admin/demo：示例内容里各项现在有没有，以及还有没有可清除的示例数据。
func (s *Server) handleDemoStatus(w http.ResponseWriter, r *http.Request) {
	items, imported := s.demoState(r.Context())
	present := false
	for _, it := range items {
		present = present || it.Result == "exists"
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "imported": imported, "present": present, "guide": demoGuide()})
}

// demoState 列出示例内容里各项现在有没有，全都有算已导入。
func (s *Server) demoState(ctx context.Context) ([]demoItem, bool) {
	items := []demoItem{}
	for _, b := range demoKBases {
		items = append(items, s.demoExists(ctx, "kb", b.Slug, b.Title))
	}
	for _, a := range demoAssets() {
		if a.Kind == store.KindApp {
			items = append(items, s.demoAppState(ctx, a.Slug, a.Spec.Title, a.Spec))
			continue
		}
		items = append(items, s.demoExists(ctx, a.Kind, a.Slug, a.Spec.Title))
	}
	for _, u := range demoUsers {
		res := demoItem{Kind: "user", Slug: u.Username, Title: u.DisplayName, Result: "missing"}
		if _, err := s.store.GetUserByUsername(ctx, u.Username); err == nil {
			res.Result = "exists"
		}
		items = append(items, res)
	}
	for _, a := range demoSelfApps() {
		if a.Kind == store.KindApp && a.DataWorkbench == nil {
			items = append(items, s.demoAppState(ctx, a.Slug, a.Spec.Title, a.Spec))
			continue
		}
		items = append(items, s.demoExists(ctx, a.Kind, a.Slug, a.Spec.Title))
	}
	imported := true
	for _, it := range items {
		imported = imported && it.Result == "exists"
	}
	return items, imported
}

func (s *Server) demoExists(ctx context.Context, kind, slug, title string) demoItem {
	res := demoItem{Kind: kind, Slug: slug, Title: title, Result: "missing"}
	var err error
	if kind == "kb" {
		_, err = s.store.GetKBaseBySlug(ctx, slug)
	} else {
		_, err = s.store.GetAssetBySlug(ctx, slug)
	}
	if err == nil {
		res.Result = "exists"
	}
	return res
}

// demoAppState 同 demoExists，另外看示例应用是不是比这一版旧（升级 dsh-ee 后示例改过）：旧的报 outdated，
// 部署清单上的「导入示例」按钮会重新出现，点一下就升级。
func (s *Server) demoAppState(ctx context.Context, slug, title string, sp assetSpec) demoItem {
	res := s.demoExists(ctx, store.KindApp, slug, title)
	if res.Result != "exists" {
		return res
	}
	a, err := s.store.GetAssetBySlug(ctx, slug)
	if err != nil {
		return res
	}
	if next, ok, _ := s.demoAppNext(ctx, a, sp); ok && next != nil {
		res.Result = "outdated"
	}
	return res
}

// demoAppNext 返回示例应用要升级到的内容；不是导入的示例、或者线上已经是这一版时 ok 为假。
func (s *Server) demoAppNext(ctx context.Context, a *store.Asset, sp assetSpec) (json.RawMessage, bool, error) {
	recs, err := s.store.DemoRecords(ctx)
	if err != nil {
		return nil, false, err
	}
	ours := false
	for _, r := range recs {
		ours = ours || (r.Kind == "asset" && r.RefID == a.ID)
	}
	if !ours {
		return nil, false, nil
	}
	live, err := s.store.LiveAssetVersions(ctx, []int64{a.ID})
	if err != nil {
		return nil, false, err
	}
	lv := live[a.ID]
	if lv == nil {
		return nil, false, nil
	}
	if msg := validateSpec(store.KindApp, &sp); msg != "" {
		return nil, false, errors.New(msg)
	}
	next, _ := json.Marshal(sp)
	cur, _ := json.Marshal(parseSpec(lv.Spec))
	if bytes.Equal(cur, next) {
		return nil, false, nil
	}
	return next, true, nil
}

// demoUpgradeApp 把导入过的示例应用升级到这一版：发一个新版本，旧版本留在历史里，可以回退。
// 管理员自己改过的内容也会被这一版盖掉（样板本来就是拿来复制的，复制出来的副本不受影响）。
func (s *Server) demoUpgradeApp(ctx context.Context, it demoItem, a *store.Asset, sp assetSpec, by int64) (demoItem, error) {
	next, ok, err := s.demoAppNext(ctx, a, sp)
	if err != nil || !ok {
		return it, err
	}
	if msg, err := s.checkWorkbenchSources(ctx, parseSpec(next).Workbench); err != nil {
		return it, err
	} else if msg != "" {
		return it, errors.New(msg)
	}
	if msg, err := s.checkAppTablesChange(ctx, a, parseSpec(next)); err != nil {
		return it, err
	} else if msg != "" {
		return it, errors.New(msg)
	}
	if _, err := s.store.AddAssetVersion(ctx, a.ID, next, store.StatusPublished, by); err != nil {
		return it, err
	}
	it.Result = "updated"
	return it, nil
}

// POST /api/admin/demo：导入示例内容。要超级管理员：会建账号、连接，并把微应用推到全员的 DSH。
func (s *Server) handleDemoImport(w http.ResponseWriter, r *http.Request) {
	if !currentAccess(r).Super {
		writeErr(w, http.StatusForbidden, "导入示例内容需要超级管理员")
		return
	}
	ctx := r.Context()
	by := currentUser(r).ID
	items := []demoItem{}
	add := func(it demoItem, err error) {
		if err == nil {
			err = s.demoRecordCreated(ctx, it)
		}
		if err != nil {
			it.Result, it.Error = "failed", err.Error()
		}
		items = append(items, it)
	}

	for _, b := range demoKBases {
		it, err := s.demoImportKBase(ctx, b, by)
		add(it, err)
	}
	for _, a := range demoAssets() {
		it, err := s.demoImportAsset(r, a, by)
		add(it, err)
	}
	accounts := []demoAccount{}
	for _, u := range demoUsers {
		acc, it, err := s.demoImportUser(ctx, u)
		add(it, err)
		if acc != nil {
			acc.Title = u.Title
			accounts = append(accounts, *acc)
		}
	}
	// 数据中台的示例表和数据集（要用演示员工的名字，放在建账号之后）；没开数据中台时跳过
	items = append(items, s.demoImportData(ctx, by, time.Now().UTC())...)
	// 员工自己搭建的「自己用」应用，要等账号建好
	for _, a := range demoSelfApps() {
		it, err := s.demoImportSelfApp(ctx, a)
		if err == nil {
			// demoImportSelfApp 自己记过了
			items = append(items, it)
			continue
		}
		it.Result, it.Error = "failed", err.Error()
		items = append(items, it)
	}
	usage, err := s.demoSimulateUsage(ctx, time.Now().UTC())
	if err != nil {
		s.log.Warn("写入示例的模拟使用数据失败", "err", err)
	}
	s.audit(r, auditDemoImport, "demo", map[string]any{"items": len(items), "accounts": len(accounts), "usage_rows": usage})
	s.hub.notify()
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "accounts": accounts, "usage_rows": usage, "guide": demoGuide()})
}

func (s *Server) demoImportKBase(ctx context.Context, d demoKBase, by int64) (demoItem, error) {
	it := demoItem{Kind: "kb", Slug: d.Slug, Title: d.Title, Result: "exists"}
	if _, err := s.store.GetKBaseBySlug(ctx, d.Slug); err == nil {
		return it, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return it, err
	}
	b := &store.KBase{Slug: d.Slug, Title: d.Title, Description: d.Description, Scope: store.Scope{All: true}}
	if err := s.store.CreateKBase(ctx, b, by); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return it, nil
		}
		return it, err
	}
	for _, doc := range d.Docs {
		kd := &store.KDoc{BaseID: b.ID, Title: doc.Title, Folder: doc.Folder, Mime: "text/markdown"}
		if err := s.store.CreateKDoc(ctx, kd, doc.Markdown, "示例内容", by); err != nil {
			return it, err
		}
	}
	it.Result = "created"
	return it, nil
}

func (s *Server) demoImportAsset(r *http.Request, d demoAsset, by int64) (demoItem, error) {
	ctx := r.Context()
	it := demoItem{Kind: d.Kind, Slug: d.Slug, Title: d.Spec.Title, Result: "exists"}
	if a, err := s.store.GetAssetBySlug(ctx, d.Slug); err == nil {
		if d.Kind == store.KindApp {
			return s.demoUpgradeApp(ctx, it, a, d.Spec, by)
		}
		return it, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return it, err
	}
	sp := d.Spec
	key := ""
	if d.Kind == store.KindConnection {
		sp.URL = s.demoLoopback(r)
		key = secret.Token()
	}
	if msg := validateSpec(d.Kind, &sp); msg != "" {
		return it, errors.New(msg)
	}
	// 工作台的数据源要先有连接和工具，所以连接排在微应用前面
	if msg, err := s.checkWorkbenchSources(ctx, sp.Workbench); err != nil {
		return it, err
	} else if msg != "" {
		return it, errors.New(msg)
	}
	raw, _ := json.Marshal(sp)
	owner := by
	a := &store.Asset{Kind: d.Kind, Slug: d.Slug, Title: sp.Title, Description: sp.Description, Category: demoCategory,
		OwnerID: &owner, Scope: store.Scope{All: true}, Delivery: d.Delivery}
	if err := s.store.CreateAsset(ctx, a, raw, store.StatusPublished, by); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return it, nil
		}
		return it, err
	}
	it.Result = "created"
	if d.Kind == store.KindConnection {
		if err := s.saveConnSecret(r, a.ID, key); err != nil {
			return it, err
		}
		if _, err := s.syncConnectionTools(ctx, a.ID, viewerOf(r), s.callerBase(r)); err != nil {
			return it, fmt.Errorf("连接已建好，但同步工具失败：%w", err)
		}
		// 提交报销单是写操作，默认关着；示例里打开，好让人看到写操作要先过审批卡
		for _, name := range d.EnableTools {
			if err := s.store.SetConnectionTool(ctx, a.ID, name, true, nil); err != nil {
				return it, err
			}
		}
	}
	return it, nil
}

func (s *Server) demoImportUser(ctx context.Context, d demoUser) (*demoAccount, demoItem, error) {
	it := demoItem{Kind: "user", Slug: d.Username, Title: d.DisplayName, Result: "exists"}
	if _, err := s.store.GetUserByUsername(ctx, d.Username); err == nil {
		return nil, it, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, it, err
	}
	password := "demo-" + secret.Token()[:10]
	hash, err := HashPassword(password)
	if err != nil {
		return nil, it, err
	}
	limit := int64(demoMonthlyTokens)
	u := &store.User{Username: d.Username, DisplayName: d.DisplayName, Department: d.Department, PasswordHash: hash,
		MonthlyTokenLimit: &limit, AuthSource: store.AuthLocal, AccountType: store.AccountHuman}
	if err := s.store.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, it, nil
		}
		return nil, it, err
	}
	if member, err := s.store.GetRoleByCode(ctx, rbac.RoleMember); err == nil {
		if err := s.store.SetUserRoles(ctx, u.ID, []int64{member.ID}); err != nil {
			return nil, it, err
		}
	} else if !notFound(err) {
		return nil, it, err
	}
	it.Result = "created"
	return &demoAccount{Username: u.Username, DisplayName: u.DisplayName, Department: u.Department, Password: password}, it, nil
}
