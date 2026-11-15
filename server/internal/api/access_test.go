package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 应用的依赖与一键申请（M7）：详情里标出缺什么；申请写明应用；审批人按对象分（知识库维护者 / 连接管理员）；
// 通过后依赖变成可用，「自选」的能力顺手启用。
func TestAccessRequests(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"total":1}`)) }))
	defer rest.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	login := func(name string) (*client, int64) {
		t.Helper()
		code, body := admin.do("POST", "/api/admin/users", map[string]any{"username": name, "password": name + "-password-1", "department": "研发部"}, nil)
		expect(t, "建用户 "+name, code, 201, body)
		var u struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(body, &u)
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": name, "password": name + "-password-1"}, nil)
		expect(t, name+" 登录", code, 200, body)
		return c, u.ID
	}
	bob, _ := login("bob")
	carol, carolID := login("carol")

	sales := map[string]any{"departments": []string{"销售部"}}
	doc := `{"openapi":"3.0.0","info":{"title":"CRM"},"paths":{"/summary":{"get":{"operationId":"summary"}}}}`
	for _, slug := range []string{"crm", "erp"} {
		code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": slug, "delivery": "auto", "scope": sales,
			"spec": map[string]any{"title": slug + " 系统", "type": "openapi", "url": rest.URL, "openapi": doc, "auth": map[string]any{"type": "none"}}}, nil)
		expect(t, "接入连接 "+slug, code, 201, body)
	}
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "skill", "slug": "brief-skill", "delivery": "optional", "scope": sales,
		"spec": map[string]any{"title": "简报写法", "description": "怎么写拜访简报", "body": "先写合同，再写风险"}}, nil)
	expect(t, "建技能", code, 201, body)
	code, body = admin.do("POST", "/api/kb", map[string]any{"slug": "sales-kb", "title": "销售资料", "description": "产品与报价",
		"scope": sales, "maintainers": map[string]any{"users": []int64{carolID}}}, nil)
	expect(t, "建知识库", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "visit-brief", "delivery": "auto", "scope": map[string]any{"all": true},
		"spec": map[string]any{"title": "客户拜访简报", "description": "x", "persona": "x", "connections": []string{"crm", "erp"}, "skills": []string{"brief-skill"}, "knowledge": []string{"sales-kb"}}}, nil)
	expect(t, "建应用", code, 201, body)
	var created struct {
		Asset struct {
			ID int64 `json:"id"`
		} `json:"asset"`
	}
	_ = json.Unmarshal(body, &created)
	appPath := "/api/hub/" + itoa(created.Asset.ID)

	type dep struct {
		Kind    string `json:"kind"`
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		ID      int64  `json:"id"`
		Status  string `json:"status"`
		Request *struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		} `json:"request"`
	}
	deps := func(c *client) map[string]dep {
		t.Helper()
		code, body := c.do("GET", appPath, nil, nil)
		expect(t, "应用详情", code, 200, body)
		var d struct {
			Deps []dep `json:"deps"`
		}
		_ = json.Unmarshal(body, &d)
		out := map[string]dep{}
		for _, x := range d.Deps {
			out[x.Slug] = x
		}
		return out
	}
	d := deps(bob)
	for _, slug := range []string{"crm", "erp", "brief-skill", "sales-kb"} {
		if d[slug].Status != "missing" || d[slug].ID != 0 {
			t.Fatalf("员工缺 %s 的权限，应是 missing 且不给 id：%+v", slug, d[slug])
		}
	}
	if d["sales-kb"].Title != "销售资料" || d["crm"].Title != "crm 系统" {
		t.Fatalf("依赖应带名称：%+v", d)
	}
	if d := deps(admin); d["crm"].Status != "missing" || d["crm"].ID == 0 {
		t.Fatalf("依赖状态按范围算；管理员不在范围里也给 id 方便去改：%+v", d["crm"])
	}

	// 申请页：没权限也能看到名称与说明，看不到 id
	code, body = bob.do("GET", "/api/me/access-target?kind=connection&slug=crm&app=visit-brief", nil, nil)
	expect(t, "申请页信息", code, 200, body)
	var target map[string]any
	_ = json.Unmarshal(body, &target)
	if target["title"] != "crm 系统" || target["allowed"] != false || target["id"] != nil || target["app"].(map[string]any)["title"] != "客户拜访简报" {
		t.Fatalf("申请页信息不对：%s", body)
	}
	code, body = bob.do("GET", "/api/me/access-target?kind=connection&slug=nope", nil, nil)
	expect(t, "不存在的对象", code, 404, body)

	// 一键申请：写明应用时不用填理由；重复申请不新建
	request := func(c *client, kind, slug string) int64 {
		t.Helper()
		code, body := c.do("POST", "/api/me/access-requests", map[string]any{"kind": kind, "slug": slug, "app": "visit-brief"}, nil)
		expect(t, "申请 "+slug, code, 201, body)
		var r struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(body, &r)
		return r.ID
	}
	kbReq := request(bob, "knowledge", "sales-kb")
	crmReq := request(bob, "connection", "crm")
	erpReq := request(bob, "connection", "erp")
	skillReq := request(bob, "skill", "brief-skill")
	if again := request(bob, "connection", "crm"); again != crmReq {
		t.Fatalf("重复申请应返回原来那条：%d / %d", again, crmReq)
	}
	code, body = bob.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "crm"}, nil)
	expect(t, "没写应用也没写理由", code, 400, body)
	if d := deps(bob); d["crm"].Request == nil || d["crm"].Request.Status != "pending" {
		t.Fatalf("依赖上应显示在审的申请：%+v", d["crm"])
	}

	// 待我审批：知识库维护者只看到知识库的；不能处理连接的
	type lists struct {
		Mine []struct {
			ID       int64  `json:"id"`
			Status   string `json:"status"`
			AppTitle string `json:"app_title"`
			Title    string `json:"title"`
			Reason   string `json:"reason"`
		} `json:"mine"`
		ToReview []struct {
			ID   int64  `json:"id"`
			Kind string `json:"kind"`
		} `json:"to_review"`
	}
	get := func(c *client) lists {
		t.Helper()
		code, body := c.do("GET", "/api/me/access-requests", nil, nil)
		expect(t, "我的申请", code, 200, body)
		var l lists
		_ = json.Unmarshal(body, &l)
		return l
	}
	if l := get(carol); len(l.ToReview) != 1 || l.ToReview[0].ID != kbReq {
		t.Fatalf("知识库维护者只该看到知识库的申请：%+v", l.ToReview)
	}
	if l := get(admin); len(l.ToReview) != 4 {
		t.Fatalf("管理员看到全部 4 条：%+v", l.ToReview)
	}
	if l := get(bob); len(l.ToReview) != 0 || len(l.Mine) != 4 || l.Mine[0].AppTitle != "客户拜访简报" || l.Mine[0].Reason != "为了使用应用「客户拜访简报」" {
		t.Fatalf("申请人的列表不对：%+v", l)
	}
	resolve := func(c *client, id int64, decision, note string, want int) {
		t.Helper()
		code, body := c.do("POST", "/api/me/access-requests/"+itoa(id), map[string]any{"decision": decision, "note": note}, nil)
		expect(t, "处理申请", code, want, body)
	}
	resolve(carol, crmReq, "approve", "", 403)
	resolve(bob, kbReq, "approve", "", 403)
	resolve(carol, kbReq, "approve", "", 200)
	resolve(carol, kbReq, "approve", "", 404)
	resolve(admin, crmReq, "approve", "", 200)
	resolve(admin, skillReq, "approve", "", 200)
	resolve(admin, erpReq, "reject", "ERP 只给财务", 200)

	d = deps(bob)
	if d["sales-kb"].Status != "ok" || d["crm"].Status != "ok" || d["brief-skill"].Status != "ok" {
		t.Fatalf("通过后应能用（自选的技能顺手启用）：%+v", d)
	}
	if d["erp"].Status != "missing" || d["erp"].Request == nil || d["erp"].Request.Status != "rejected" || d["erp"].Request.Note != "ERP 只给财务" {
		t.Fatalf("被驳回的应显示原因：%+v", d["erp"])
	}
	code, body = bob.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "crm", "app": "visit-brief"}, nil)
	expect(t, "已经能用了还申请", code, 400, body)

	// 管理员原来的连接申请队列只列连接
	code, body = bob.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "erp", "reason": "我也要报销"}, nil)
	expect(t, "驳回后可以再申请", code, 201, body)
	code, body = carol.do("POST", "/api/me/access-requests", map[string]any{"kind": "skill", "slug": "brief-skill", "reason": "学习"}, nil)
	expect(t, "carol 申请技能", code, 201, body)
	code, body = admin.do("GET", "/api/admin/connection-requests", nil, nil)
	expect(t, "管理员的连接申请", code, 200, body)
	var adminList struct {
		Requests []struct {
			Kind string `json:"kind"`
			Slug string `json:"slug"`
		} `json:"requests"`
	}
	_ = json.Unmarshal(body, &adminList)
	if len(adminList.Requests) != 1 || adminList.Requests[0].Slug != "erp" {
		t.Fatalf("管理员的连接申请队列只列连接：%s", body)
	}
}

// ---- 审批「先 CAS 抢占、后授权」回归（并发审批同一条申请）----
//
// 两个审批人可能同时处理同一条申请（网页 approve 与飞书卡片 reject 各走各的入口）。
// 修复前是「先授权、后改状态」：一方把状态改成 rejected，另一方 CAS 失败回 409，
// 但它的授权动作已经生效，留下「申请已被驳回、员工却有了访问权」的脏数据。
// 下面的用例锁住两条不变式：①申请不在 pending 时，后到的 approve 不能再授权；
// ②并发 approve/reject 之后，只有最终状态为 approved 时才允许存在授权。

// accessCASEnv 是可审批的最小环境：管理员、研发部的申请人、一个只对销售部开放的知识库。
type accessCASEnv struct {
	t         *testing.T
	st        *store.Store
	admin     *client
	adminID   int64
	applicant *store.User
	kbID      int64
}

func newAccessCASEnv(t *testing.T) *accessCASEnv {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	adminUser, err := st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := api.HashPassword("applicant-password-1")
	if err != nil {
		t.Fatal(err)
	}
	applicant := &store.User{Username: "applicant", DisplayName: "申请人", Department: "研发部", AuthSource: store.AuthLocal, PasswordHash: hash}
	if err := st.CreateUser(ctx, applicant); err != nil {
		t.Fatal(err)
	}
	kb := &store.KBase{Slug: "cas-kb", Title: "审批回归库", Scope: store.Scope{Departments: []string{"销售部"}}}
	if err := st.CreateKBase(ctx, kb, adminUser.ID); err != nil {
		t.Fatal(err)
	}
	return &accessCASEnv{t: t, st: st, admin: admin, adminID: adminUser.ID, applicant: applicant, kbID: kb.ID}
}

// pending 直接建一条待处理的知识库申请（等价于员工在网页提交）。
func (e *accessCASEnv) pending() int64 {
	e.t.Helper()
	id, existed, err := e.st.CreateAccessRequest(context.Background(), e.applicant.ID, store.RequestKnowledge, e.kbID, 0, "审批回归")
	if err != nil {
		e.t.Fatal(err)
	}
	if existed {
		e.t.Fatalf("准备申请时已有待处理的申请 %d", id)
	}
	return id
}

func (e *accessCASEnv) granted() bool {
	e.t.Helper()
	b, err := e.st.GetKBase(context.Background(), e.kbID)
	if err != nil {
		e.t.Fatal(err)
	}
	return slices.Contains(b.Scope.Users, e.applicant.ID)
}

func (e *accessCASEnv) resetScope() {
	e.t.Helper()
	sc := store.Scope{Departments: []string{"销售部"}}.Normalize()
	if err := e.st.UpdateKBase(context.Background(), e.kbID, store.KBasePatch{Scope: &sc}); err != nil {
		e.t.Fatal(err)
	}
}

// doSoft 与 client.do 相同，但失败只返回错误，不结束测试（给并发 goroutine 用）。
func (c *client) doSoft(method, path string, body any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func TestAccessResolveCASOrder(t *testing.T) {
	e := newAccessCASEnv(t)
	ctx := context.Background()

	// 1) 已被别人处理：后到的 approve 不能再把权限授出去（修复前会先授权再 CAS 失败）。
	t.Run("已被别人处理", func(t *testing.T) {
		id := e.pending()
		// 另一位审批人先驳回（等价于飞书 reject 先到），后到的网页 approve 不能授权。
		if err := e.st.ResolveAccessRequest(ctx, id, "rejected", e.adminID, "别人先驳回"); err != nil {
			t.Fatal(err)
		}
		code, body := e.admin.do("POST", "/api/me/access-requests/"+itoa(id), map[string]any{"decision": "approve", "note": "我也批了"}, nil)
		// 申请已不是 pending 时，入口先按现有逻辑回 404「申请不存在或已经处理过」；
		// 409「这条申请刚被别人处理了」是并发窗口里 CAS 抢输的返回（见下一个子用例）。
		// 两种都算「处理入口拒绝了这次审批」，关键是它没有授权。
		if code != http.StatusNotFound && code != http.StatusConflict {
			t.Fatalf("已被别人处理的申请不该再处理成功：状态码 %d；响应 %s", code, body)
		}
		if e.granted() {
			t.Fatal("驳回在先，后到的 approve 不能把权限授出去（修复前会授出去）")
		}
		q, err := e.st.GetAccessRequest(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if q.Status != "rejected" || q.HandledBy == nil || *q.HandledBy != e.adminID || q.Note != "别人先驳回" {
			t.Fatalf("申请应保持被别人驳回的终态，不被后到的 approve 覆盖：%+v", q)
		}
	})

	// 2) 并发 approve / reject 同一条申请：CAS 只有一个赢家，授权只跟着赢家走。
	t.Run("并发 approve 与 reject", func(t *testing.T) {
		const rounds = 12
		saw409 := 0
		for round := 0; round < rounds; round++ {
			e.resetScope()
			id := e.pending()
			path := "/api/me/access-requests/" + itoa(id)
			var approveCode, rejectCode int
			var approveErr, rejectErr error
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				approveCode, approveErr = e.admin.doSoft("POST", path, map[string]any{"decision": "approve", "note": "并发通过"})
			}()
			go func() {
				defer wg.Done()
				rejectCode, rejectErr = e.admin.doSoft("POST", path, map[string]any{"decision": "reject", "note": "并发驳回"})
			}()
			wg.Wait()
			if approveErr != nil || rejectErr != nil {
				t.Fatalf("第 %d 轮请求失败：approve=%v reject=%v", round, approveErr, rejectErr)
			}
			// 两个请求都是同一会话的管理员：一个抢到 CAS 回 200，另一个看到 404（申请已处理）或 409（CAS 抢输了）。
			wins := 0
			for _, c := range []int{approveCode, rejectCode} {
				if c == http.StatusOK {
					wins++
				}
			}
			if wins != 1 {
				t.Fatalf("第 %d 轮应只有一个赢家：approve=%d reject=%d", round, approveCode, rejectCode)
			}
			if approveCode == http.StatusConflict || rejectCode == http.StatusConflict {
				saw409++
			}
			q, err := e.st.GetAccessRequest(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			granted := e.granted()
			switch q.Status {
			case "approved":
				if approveCode != http.StatusOK {
					t.Fatalf("第 %d 轮最终 approved，但 approve 没成功（%d）", round, approveCode)
				}
				if !granted {
					t.Fatalf("第 %d 轮最终 approved，却没有授权", round)
				}
			case "rejected":
				if rejectCode != http.StatusOK {
					t.Fatalf("第 %d 轮最终 rejected，但 reject 没成功（%d）", round, rejectCode)
				}
				if granted {
					t.Fatalf("第 %d 轮最终 rejected 却授出了权限（修复前的脏数据）", round)
				}
			default:
				t.Fatalf("第 %d 轮最终状态异常：%q", round, q.Status)
			}
		}
		t.Logf("%d 轮并发 approve/reject：CAS 抢输的一方有 %d 轮明确拿到 409（其余轮次按 404 走：申请已不在待处理）", rounds, saw409)
	})

	// 3) 抢占成功、授权失败：申请退回待处理（返回 500 说明可重试），不留「已通过但没授权」。
	//    目标指向不存在的能力，授权一定失败；修复前是「先授权」，会直接回 404 且不做任何回滚。
	t.Run("授权失败退回待处理", func(t *testing.T) {
		id, existed, err := e.st.CreateAccessRequest(ctx, e.applicant.ID, store.RequestSkill, 1<<40, 0, "目标不存在")
		if err != nil || existed {
			t.Fatalf("准备申请：existed=%v err=%v", existed, err)
		}
		code, body := e.admin.do("POST", "/api/me/access-requests/"+itoa(id), map[string]any{"decision": "approve", "note": "批准"}, nil)
		expect(t, "授权失败应回 500", code, http.StatusInternalServerError, body)
		if !strings.Contains(string(body), "退回待处理") || !strings.Contains(string(body), "重试") {
			t.Fatalf("授权失败要说明申请已退回、可以重试：%s", body)
		}
		q, err := e.st.GetAccessRequest(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if q.Status != "pending" || q.HandledBy != nil || q.HandledAt != nil || q.Note != "" {
			t.Fatalf("授权失败后申请应退回干净的待处理状态，不能停在 approved：%+v", q)
		}
	})
}
