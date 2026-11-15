package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 报表 / 运营统计 / 个人用量在原有字段之外多返回用量细项（reasoning_tokens、
// cached_tokens），旧的 total_tokens 口径与额度不受影响。
func TestUsageDetailsReports(t *testing.T) {
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

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	ids := map[string]int64{}
	for _, u := range []struct{ name, dept string }{{"details-a", "研发效能部"}, {"details-b", "财务部"}} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": u.name, "password": "details-password-1", "department": u.dept}, nil)
		expect(t, "建用户", code, 201, body)
		ids[u.name] = decodeAs[struct{ ID int64 }](t, body).ID
	}
	alice := ids["details-a"]
	bob := ids["details-b"]
	now := time.Now().UTC()

	token := &store.AccessToken{UserID: alice, Name: "明细令牌", Prefix: "ee_at_details", Source: "manual",
		Scopes: []string{"models"}, CreatedBy: alice}
	if err := st.CreateAccessToken(ctx, token, secret.Hash("ee_at_details")); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := st.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO usage_logs (api_key_id, user_id, provider_id, model, prompt_tokens, completion_tokens, total_tokens, status_code, latency_ms, created_at, token_id, reasoning_tokens, cached_tokens)
		VALUES (0, ?, 0, 'deepseek-reasoner', 100, 40, 140, 200, 10, ?, ?, 20, 64)`, alice, now, token.ID)
	exec(`INSERT INTO usage_logs (api_key_id, user_id, provider_id, model, prompt_tokens, completion_tokens, total_tokens, status_code, latency_ms, created_at, token_id, reasoning_tokens, cached_tokens)
		VALUES (0, ?, 0, 'deepseek-reasoner', 200, 60, 260, 200, 10, ?, ?, 30, 128)`, alice, now, token.ID)
	// 旧数据：不写细项列，走默认 0。
	exec(`INSERT INTO usage_logs (api_key_id, user_id, provider_id, model, prompt_tokens, completion_tokens, total_tokens, status_code, latency_ms, created_at)
		VALUES (0, ?, 0, 'deepseek-chat', 10, 5, 15, 200, 5, ?)`, bob, now)

	month := now.Format("2006-01")
	code, body = admin.do("GET", "/api/admin/report/monthly?month="+month+"&tz=0", nil, nil)
	expect(t, "月度报告", code, 200, body)
	type reportResp struct {
		Report   store.MonthlyReport
		Previous map[string]any
	}
	rep := decodeAs[reportResp](t, body)
	if rep.Report.ModelCalls != 3 || rep.Report.Tokens != 415 || rep.Report.ReasoningTokens != 50 || rep.Report.CachedTokens != 192 {
		t.Fatalf("月报总量细项不对：%+v", rep.Report)
	}
	byDept := map[string]store.ReportDept{}
	for _, d := range rep.Report.Departments {
		byDept[d.Department] = d
	}
	if d := byDept["研发效能部"]; d.Tokens != 400 || d.ReasoningTokens != 50 || d.CachedTokens != 192 {
		t.Fatalf("月报部门细项不对：%+v", d)
	}
	if d := byDept["财务部"]; d.Tokens != 15 || d.ReasoningTokens != 0 || d.CachedTokens != 0 {
		t.Fatalf("月报旧数据应保持 0：%+v", d)
	}
	// 上个月的对比数据也带新键（本月没有上月数据时为 0）。
	for _, k := range []string{"reasoning_tokens", "cached_tokens"} {
		if _, ok := rep.Previous[k]; !ok {
			t.Fatalf("previous 缺少 %s：%v", k, rep.Previous)
		}
		if v, _ := rep.Previous[k].(float64); v != 0 {
			t.Fatalf("previous.%s 应为 0，得到 %v", k, rep.Previous[k])
		}
	}

	code, body = admin.do("GET", "/api/admin/stats?days=2&tz=0", nil, nil)
	expect(t, "运营统计", code, 200, body)
	type statsResp struct {
		Daily  []store.DailyStat
		Totals struct {
			Tokens          int64 `json:"tokens"`
			ReasoningTokens int64 `json:"reasoning_tokens"`
			CachedTokens    int64 `json:"cached_tokens"`
		}
		TopModels   []store.ModelStat      `json:"top_models"`
		Departments []store.DepartmentStat `json:"departments"`
	}
	stats := decodeAs[statsResp](t, body)
	if stats.Totals.Tokens != 415 || stats.Totals.ReasoningTokens != 50 || stats.Totals.CachedTokens != 192 {
		t.Fatalf("统计合计细项不对：%+v", stats.Totals)
	}
	var today *store.DailyStat
	for i := range stats.Daily {
		if stats.Daily[i].Date == now.Format("2006-01-02") {
			today = &stats.Daily[i]
		}
	}
	if today == nil || today.Tokens != 415 || today.ReasoningTokens != 50 || today.CachedTokens != 192 {
		t.Fatalf("每日统计细项不对：%+v", stats.Daily)
	}
	var reasoner *store.ModelStat
	for i := range stats.TopModels {
		if stats.TopModels[i].Model == "deepseek-reasoner" {
			reasoner = &stats.TopModels[i]
		}
	}
	if reasoner == nil || reasoner.Tokens != 400 || reasoner.ReasoningTokens != 50 || reasoner.CachedTokens != 192 {
		t.Fatalf("模型排行细项不对：%+v", stats.TopModels)
	}
	statDept := map[string]store.DepartmentStat{}
	for _, d := range stats.Departments {
		statDept[d.Department] = d
	}
	if d := statDept["研发效能部"]; d.Tokens != 400 || d.ReasoningTokens != 50 || d.CachedTokens != 192 {
		t.Fatalf("统计部门细项不对：%+v", d)
	}
	if d := statDept["财务部"]; d.Tokens != 15 || d.ReasoningTokens != 0 || d.CachedTokens != 0 {
		t.Fatalf("统计旧数据应保持 0：%+v", d)
	}

	code, body = admin.do("GET", "/api/admin/stats?days=2&tz=0&group_by=token", nil, nil)
	expect(t, "按令牌统计", code, 200, body)
	type tokenStatsResp struct {
		TokenStats []store.TokenStat `json:"token_stats"`
	}
	tokenStats := decodeAs[tokenStatsResp](t, body).TokenStats
	if len(tokenStats) != 1 || tokenStats[0].Name != "明细令牌" || tokenStats[0].Tokens != 400 ||
		tokenStats[0].ReasoningTokens != 50 || tokenStats[0].CachedTokens != 192 {
		t.Fatalf("按令牌统计细项不对：%+v", tokenStats)
	}

	// 员工自己的用量：额度仍只按 total_tokens 计，明细接口返回细项。
	emp := newClient(t, ts.URL)
	code, body = emp.do("POST", "/api/auth/login", map[string]string{"username": "details-a", "password": "details-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)
	code, body = emp.do("GET", "/api/me/usage", nil, nil)
	expect(t, "我的用量", code, 200, body)
	type myUsage struct {
		Quota struct {
			MonthTokens int64 `json:"month_tokens"`
		} `json:"quota"`
		ByDay   []store.UsageDay   `json:"by_day"`
		ByModel []store.UsageGroup `json:"by_model"`
		ByToken []store.UsageGroup `json:"by_token"`
	}
	mine := decodeAs[myUsage](t, body)
	if mine.Quota.MonthTokens != 400 {
		t.Fatalf("额度口径应只算 total_tokens=400，得到 %d", mine.Quota.MonthTokens)
	}
	if len(mine.ByDay) != 1 || mine.ByDay[0].Tokens != 400 || mine.ByDay[0].ReasoningTokens != 50 || mine.ByDay[0].CachedTokens != 192 {
		t.Fatalf("我的按天用量细项不对：%+v", mine.ByDay)
	}
	if len(mine.ByModel) != 1 || mine.ByModel[0].Name != "deepseek-reasoner" ||
		mine.ByModel[0].ReasoningTokens != 50 || mine.ByModel[0].CachedTokens != 192 {
		t.Fatalf("我的按模型用量细项不对：%+v", mine.ByModel)
	}
	if len(mine.ByToken) != 1 || mine.ByToken[0].Name != "明细令牌" ||
		mine.ByToken[0].ReasoningTokens != 50 || mine.ByToken[0].CachedTokens != 192 {
		t.Fatalf("我的按令牌用量细项不对：%+v", mine.ByToken)
	}

	code, body = emp.do("GET", "/api/me/calls/model?limit=5", nil, nil)
	expect(t, "我的调用记录", code, 200, body)
	type callsResp struct {
		Calls []store.ModelCall `json:"calls"`
	}
	calls := decodeAs[callsResp](t, body).Calls
	if len(calls) != 2 || calls[0].ReasoningTokens != 30 || calls[0].CachedTokens != 128 {
		t.Fatalf("我的调用记录细项不对：%+v", calls)
	}
}
