package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 0039：usage_logs 增加 reasoning_tokens / cached_tokens 两列（只增列、默认 0）。
func TestUsageDetailsMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0039_usage_details.sql")
	if err != nil {
		t.Fatalf("读 0039 迁移: %v", err)
	}
	stmts := splitStatements(string(body))
	if len(stmts) == 0 {
		t.Fatal("0039 迁移没有可执行语句")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"ALTER TABLE usage_logs",
		"ADD COLUMN reasoning_tokens INT NOT NULL DEFAULT 0",
		"cached_tokens    INT NOT NULL DEFAULT 0",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0039 迁移缺少 %q：\n%s", want, joined)
		}
	}
}

// 迁移后的表结构：两列都存在、非空、默认 0，旧行不用回填。
func TestUsageDetailsColumns(t *testing.T) {
	st := openTokenTestStore(t)
	rows, err := st.DB().QueryContext(context.Background(), `SELECT column_name, is_nullable, column_default
		FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
		AND column_name IN ('reasoning_tokens', 'cached_tokens') ORDER BY column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]struct {
		nullable string
		def      string
	}{}
	for rows.Next() {
		var name, nullable, def string
		if err := rows.Scan(&name, &nullable, &def); err != nil {
			t.Fatal(err)
		}
		got[name] = struct{ nullable, def string }{nullable, def}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("usage_logs 应有 reasoning_tokens / cached_tokens 两列，实际：%v", got)
	}
	for _, name := range []string{"reasoning_tokens", "cached_tokens"} {
		col := got[name]
		if col.nullable != "NO" || col.def != "0" {
			t.Fatalf("%s 应为 NOT NULL DEFAULT 0：%+v", name, col)
		}
	}
}

// RecordUsage 写入细项；旧格式（不传细项）仍是 0，额度口径只看 total_tokens。
func TestRecordUsageDetails(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)
	if err := st.RecordUsage(ctx, Usage{
		UserID: u.ID, Model: "deepseek-reasoner", PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140,
		StatusCode: 200, ReasoningTokens: 20, CachedTokens: 64,
	}); err != nil {
		t.Fatal(err)
	}
	// 旧调用方 / 旧上游没有细项：记 0。
	if err := st.RecordUsage(ctx, Usage{UserID: u.ID, Model: "deepseek-chat", PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, StatusCode: 200}); err != nil {
		t.Fatal(err)
	}

	rows, err := st.DB().QueryContext(ctx, `SELECT model, reasoning_tokens, cached_tokens FROM usage_logs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		model     string
		reasoning int
		cached    int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.model, &r.reasoning, &r.cached); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		got = append(got, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []row{{"deepseek-reasoner", 20, 64}, {"deepseek-chat", 0, 0}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("usage_logs 细项 = %+v，应为 %+v", got, want)
	}

	// 细项不影响配额口径：额度仍按 total_tokens 求和，不能被推理 / 缓存重复计入。
	if n, err := st.MonthTokensByUser(ctx, u.ID); err != nil || n != 155 {
		t.Fatalf("本月用量应为 140+15=155：%d %v", n, err)
	}
}

// 报表 / 统计 / 个人用量都聚合新字段，旧行（0）与缺省行为不变。
func TestUsageDetailsAggregations(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()

	alice := tokenTestUser(t, st, false)
	bob := &User{Username: "bob", DisplayName: "鲍勃", Department: "财务部", AccountType: AccountHuman}
	if err := st.CreateUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	token := &AccessToken{UserID: alice.ID, Name: "笔记本", Prefix: "ee_at_details", Source: "manual",
		Scopes: []string{"models"}, CreatedBy: alice.ID}
	if err := st.CreateAccessToken(ctx, token, "hash-not-used"); err != nil {
		t.Fatal(err)
	}

	record := func(userID, tokenID int64, model string, prompt, completion int, reasoning, cached int, status int) {
		t.Helper()
		if err := st.RecordUsage(ctx, Usage{
			UserID: userID, TokenID: tokenID, Model: model,
			PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion,
			ReasoningTokens: reasoning, CachedTokens: cached, StatusCode: status,
		}); err != nil {
			t.Fatal(err)
		}
	}
	record(alice.ID, token.ID, "deepseek-reasoner", 100, 40, 20, 64, 200)
	record(alice.ID, token.ID, "deepseek-reasoner", 200, 60, 30, 128, 200)
	record(alice.ID, 0, "deepseek-chat", 10, 5, 0, 0, 200)
	// 旧调用：直接按旧列插一行，两列走默认 0。
	if _, err := st.DB().ExecContext(ctx, `INSERT INTO usage_logs
		(api_key_id, user_id, provider_id, model, prompt_tokens, completion_tokens, total_tokens, status_code, latency_ms, created_at)
		VALUES (0, ?, 0, 'legacy-model', 7, 3, 10, 200, 1, ?)`, bob.ID, now()); err != nil {
		t.Fatal(err)
	}

	from := now().Add(-24 * time.Hour)
	daily, err := st.Daily(ctx, StatsRange{From: from, OffsetMin: 0})
	if err != nil {
		t.Fatal(err)
	}
	today := daily[now().UTC().Format("2006-01-02")]
	if today == nil {
		t.Fatalf("没有今天的统计：%+v", daily)
	}
	if today.Calls != 4 || today.Tokens != 425 || today.ReasoningTokens != 50 || today.CachedTokens != 192 {
		t.Fatalf("每日统计细项不对：%+v", today)
	}

	models, err := st.TopModels(ctx, from, 10)
	if err != nil {
		t.Fatal(err)
	}
	gotModel := map[string]ModelStat{}
	for _, m := range models {
		gotModel[m.Model] = m
	}
	if m := gotModel["deepseek-reasoner"]; m.Tokens != 400 || m.ReasoningTokens != 50 || m.CachedTokens != 192 {
		t.Fatalf("模型统计细项不对：%+v", m)
	}
	if m := gotModel["deepseek-chat"]; m.Tokens != 15 || m.ReasoningTokens != 0 || m.CachedTokens != 0 {
		t.Fatalf("无细项模型应保持 0：%+v", m)
	}
	if m := gotModel["legacy-model"]; m.ReasoningTokens != 0 || m.CachedTokens != 0 {
		t.Fatalf("旧行两列应为 0：%+v", m)
	}

	tokenStats, err := st.TokenStats(ctx, from, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokenStats) != 1 || tokenStats[0].Tokens != 400 || tokenStats[0].ReasoningTokens != 50 || tokenStats[0].CachedTokens != 192 {
		t.Fatalf("按令牌统计细项不对：%+v", tokenStats)
	}

	depts, err := st.Departments(ctx, from)
	if err != nil {
		t.Fatal(err)
	}
	byDept := map[string]*DepartmentStat{}
	for _, d := range depts {
		byDept[d.Department] = d
	}
	if d := byDept["研发效能部"]; d == nil || d.Tokens != 415 || d.ReasoningTokens != 50 || d.CachedTokens != 192 {
		t.Fatalf("研发效能部细项不对：%+v", d)
	}
	if d := byDept["财务部"]; d == nil || d.Tokens != 10 || d.ReasoningTokens != 0 || d.CachedTokens != 0 {
		t.Fatalf("财务部细项不对：%+v", d)
	}

	rep, err := st.Report(ctx, from, now().Add(time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModelCalls != 4 || rep.Tokens != 425 || rep.ReasoningTokens != 50 || rep.CachedTokens != 192 {
		t.Fatalf("月报总量细项不对：%+v", rep)
	}
	repDept := map[string]ReportDept{}
	for _, d := range rep.Departments {
		repDept[d.Department] = d
	}
	if d := repDept["研发效能部"]; d.Tokens != 415 || d.ReasoningTokens != 50 || d.CachedTokens != 192 {
		t.Fatalf("月报部门细项不对：%+v", d)
	}

	// 个人视角
	days, err := st.UserUsageByDay(ctx, alice.ID, from)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Calls != 3 || days[0].Tokens != 415 || days[0].ReasoningTokens != 50 || days[0].CachedTokens != 192 {
		t.Fatalf("个人按天细项不对：%+v", days)
	}
	byModel, err := st.UserUsageByModel(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byModel) != 2 {
		t.Fatalf("个人按模型应有 2 个：%+v", byModel)
	}
	if byModel[0].Name != "deepseek-reasoner" || byModel[0].ReasoningTokens != 50 || byModel[0].CachedTokens != 192 {
		t.Fatalf("个人按模型细项不对：%+v", byModel[0])
	}
	byToken, err := st.UserUsageByToken(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotToken := map[string]UsageGroup{}
	for _, g := range byToken {
		gotToken[g.Name] = g
	}
	if g := gotToken["笔记本"]; g.Tokens != 400 || g.ReasoningTokens != 50 || g.CachedTokens != 192 {
		t.Fatalf("个人按令牌细项不对：%+v", g)
	}
	if g := gotToken["旧版 key"]; g.Tokens != 15 || g.ReasoningTokens != 0 || g.CachedTokens != 0 {
		t.Fatalf("个人按令牌（旧 key）细项不对：%+v", g)
	}
	calls, err := st.UserModelCalls(ctx, alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 {
		t.Fatalf("个人调用记录应有 3 条：%+v", calls)
	}
	// 按 id 倒序：最后一条是 deepseek-chat（无细项），往前是两条 reasoner。
	if calls[0].Model != "deepseek-chat" || calls[0].ReasoningTokens != 0 || calls[0].CachedTokens != 0 {
		t.Fatalf("个人调用记录旧行细项应为 0：%+v", calls[0])
	}
	if calls[1].ReasoningTokens != 30 || calls[1].CachedTokens != 128 {
		t.Fatalf("个人调用记录细项不对：%+v", calls[1])
	}
}
