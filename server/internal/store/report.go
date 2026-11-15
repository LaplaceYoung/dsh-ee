package store

import (
	"context"
	"time"
)

// 月度 AI 落地报告的原始数据：一个时间窗口里谁在用、用了哪些应用、问了什么。
// 「活跃」指窗口内调用过模型、打开过应用或技能、调用过企业系统工具、查过知识库，或从企业源装过插件。

// activeUnion 是活跃行为的并集（每个子查询两个参数：from、to）。
const activeUnion = `
	SELECT user_id, created_at FROM usage_logs WHERE created_at >= ? AND created_at < ?
	UNION ALL SELECT user_id, created_at FROM asset_usage WHERE created_at >= ? AND created_at < ?
	UNION ALL SELECT user_id, created_at FROM connection_calls WHERE created_at >= ? AND created_at < ?
	UNION ALL SELECT user_id, created_at FROM kb_queries WHERE created_at >= ? AND created_at < ?
	UNION ALL SELECT user_id, created_at FROM npm_downloads WHERE created_at >= ? AND created_at < ? AND user_id IS NOT NULL`

func activeArgs(from, to time.Time) []any {
	out := make([]any, 0, 10)
	for range 5 {
		out = append(out, from, to)
	}
	return out
}

type ReportUsage struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Department string `json:"department"`
	Runs       int    `json:"runs"`
	Users      int    `json:"users"`
}

type ReportQuestion struct {
	Query string `json:"query"`
	Count int    `json:"count"`
	Users int    `json:"users"`
	// 这个问题有多少次一条都没查到
	Misses int `json:"misses"`
}

type ReportDept struct {
	Department  string `json:"department"`
	Users       int    `json:"users"`
	ActiveUsers int    `json:"active_users"`
	AppRuns     int    `json:"app_runs"`
	Tokens      int64  `json:"tokens"`
	// 用量细项，Tokens 的子集：推理 token 含在 completion 里、缓存命中含在 prompt 里
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

type ReportDay struct {
	Date        string `json:"date"`
	ActiveUsers int    `json:"active_users"`
	AppRuns     int    `json:"app_runs"`
}

type MonthlyReport struct {
	Users       int `json:"users"`
	ActiveUsers int `json:"active_users"`
	// 应用（微应用）被打开的次数与人数；技能、提示词另计
	AppRuns    int   `json:"app_runs"`
	AppUsers   int   `json:"app_users"`
	SkillRuns  int   `json:"skill_runs"`
	ToolCalls  int   `json:"tool_calls"`
	ToolErrors int   `json:"tool_errors"`
	KBSearches int   `json:"kb_searches"`
	KBHits     int   `json:"kb_hits"`
	ModelCalls int   `json:"model_calls"`
	Tokens     int64 `json:"tokens"`
	// 用量细项，Tokens 的子集；上游没给细项或旧数据时为 0
	ReasoningTokens int64            `json:"reasoning_tokens"`
	CachedTokens    int64            `json:"cached_tokens"`
	Apps            []ReportUsage    `json:"apps"`
	Capabilities    []ReportUsage    `json:"capabilities"`
	Questions       []ReportQuestion `json:"questions"`
	Departments     []ReportDept     `json:"departments"`
	Days            []ReportDay      `json:"days"`
}

// Report 汇总 [from, to) 的数据；按 offsetMin 时区分天。
func (s *Store) Report(ctx context.Context, from, to time.Time, offsetMin int) (*MonthlyReport, error) {
	rg := StatsRange{From: from, OffsetMin: offsetMin}
	rep := &MonthlyReport{Apps: []ReportUsage{}, Capabilities: []ReportUsage{}, Questions: []ReportQuestion{}, Departments: []ReportDept{}, Days: []ReportDay{}}
	scalar := func(q string, args []any, dst ...any) error {
		return s.db.QueryRowContext(ctx, q, args...).Scan(dst...)
	}
	if err := scalar("SELECT COUNT(*) FROM users WHERE disabled = 0 AND account_type = 'human'", nil, &rep.Users); err != nil {
		return nil, err
	}
	if err := scalar("SELECT COUNT(DISTINCT x.user_id) FROM ("+activeUnion+") x JOIN users u ON u.id = x.user_id WHERE u.account_type = 'human'", activeArgs(from, to), &rep.ActiveUsers); err != nil {
		return nil, err
	}
	if err := scalar(`SELECT COALESCE(SUM(CASE WHEN a.kind = 'app' THEN 1 ELSE 0 END), 0),
		  COUNT(DISTINCT CASE WHEN a.kind = 'app' THEN g.user_id END),
		  COALESCE(SUM(CASE WHEN a.kind IN ('skill', 'prompt') THEN 1 ELSE 0 END), 0)
		FROM asset_usage g JOIN assets a ON a.id = g.asset_id WHERE g.created_at >= ? AND g.created_at < ?`, []any{from, to},
		&rep.AppRuns, &rep.AppUsers, &rep.SkillRuns); err != nil {
		return nil, err
	}
	if err := scalar(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN ok = 0 THEN 1 ELSE 0 END), 0) FROM connection_calls WHERE created_at >= ? AND created_at < ?`,
		[]any{from, to}, &rep.ToolCalls, &rep.ToolErrors); err != nil {
		return nil, err
	}
	if err := scalar(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN hits > 0 THEN 1 ELSE 0 END), 0) FROM kb_queries WHERE tool = 'search' AND created_at >= ? AND created_at < ?`,
		[]any{from, to}, &rep.KBSearches, &rep.KBHits); err != nil {
		return nil, err
	}
	if err := scalar(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cached_tokens), 0)
		FROM usage_logs WHERE created_at >= ? AND created_at < ?`,
		[]any{from, to}, &rep.ModelCalls, &rep.Tokens, &rep.ReasoningTokens, &rep.CachedTokens); err != nil {
		return nil, err
	}

	usage := func(kinds string, limit int) ([]ReportUsage, error) {
		rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.kind, a.slug, a.title, COALESCE(u.department, ''), COUNT(*), COUNT(DISTINCT g.user_id)
			FROM asset_usage g JOIN assets a ON a.id = g.asset_id LEFT JOIN users u ON u.id = a.owner_id
			WHERE a.kind IN (`+kinds+`) AND g.created_at >= ? AND g.created_at < ?
			GROUP BY a.id, a.kind, a.slug, a.title, u.department ORDER BY COUNT(*) DESC, a.id LIMIT ?`, from, to, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []ReportUsage{}
		for rows.Next() {
			var x ReportUsage
			if err := rows.Scan(&x.ID, &x.Kind, &x.Slug, &x.Title, &x.Department, &x.Runs, &x.Users); err != nil {
				return nil, err
			}
			out = append(out, x)
		}
		return out, rows.Err()
	}
	var err error
	if rep.Apps, err = usage("'app'", 20); err != nil {
		return nil, err
	}
	if rep.Capabilities, err = usage("'skill', 'prompt'", 10); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT query, COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(CASE WHEN hits = 0 THEN 1 ELSE 0 END), 0)
		FROM kb_queries WHERE tool = 'search' AND query <> '' AND created_at >= ? AND created_at < ?
		GROUP BY query ORDER BY COUNT(*) DESC, query LIMIT 15`, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var q ReportQuestion
		if err := rows.Scan(&q.Query, &q.Count, &q.Users, &q.Misses); err != nil {
			rows.Close()
			return nil, err
		}
		rep.Questions = append(rep.Questions, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 部门
	byDept := map[string]*ReportDept{}
	var order []string
	dept := func(d string) *ReportDept {
		if byDept[d] == nil {
			byDept[d] = &ReportDept{Department: d}
			order = append(order, d)
		}
		return byDept[d]
	}
	for _, q := range []struct {
		sql  string
		args []any
		fill func(d *ReportDept, a, b int64)
	}{
		{"SELECT department, COUNT(*), 0 FROM users WHERE disabled = 0 AND account_type = 'human' GROUP BY department", nil,
			func(d *ReportDept, a, _ int64) { d.Users = int(a) }},
		{"SELECT u.department, COUNT(DISTINCT x.user_id), 0 FROM (" + activeUnion + ") x JOIN users u ON u.id = x.user_id WHERE u.account_type = 'human' GROUP BY u.department",
			activeArgs(from, to), func(d *ReportDept, a, _ int64) { d.ActiveUsers = int(a) }},
		{`SELECT u.department, COUNT(*), 0 FROM asset_usage g JOIN assets a ON a.id = g.asset_id AND a.kind = 'app'
			JOIN users u ON u.id = g.user_id WHERE g.created_at >= ? AND g.created_at < ? GROUP BY u.department`, []any{from, to},
			func(d *ReportDept, a, _ int64) { d.AppRuns = int(a) }},
		{`SELECT u.department, 0, COALESCE(SUM(l.total_tokens), 0) FROM usage_logs l JOIN users u ON u.id = l.user_id
			WHERE l.created_at >= ? AND l.created_at < ? GROUP BY u.department`, []any{from, to},
			func(d *ReportDept, _, b int64) { d.Tokens = b }},
	} {
		rows, err := s.db.QueryContext(ctx, q.sql, q.args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var d string
			var a, b int64
			if err := rows.Scan(&d, &a, &b); err != nil {
				rows.Close()
				return nil, err
			}
			q.fill(dept(d), a, b)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	// 部门用量细项单独查一次，不必为两个可选列改写上面的通用扫描循环。
	rows, err = s.db.QueryContext(ctx, `SELECT u.department, COALESCE(SUM(l.reasoning_tokens), 0), COALESCE(SUM(l.cached_tokens), 0)
		FROM usage_logs l JOIN users u ON u.id = l.user_id
		WHERE l.created_at >= ? AND l.created_at < ? GROUP BY u.department`, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d string
		var reasoning, cached int64
		if err := rows.Scan(&d, &reasoning, &cached); err != nil {
			rows.Close()
			return nil, err
		}
		x := dept(d)
		x.ReasoningTokens, x.CachedTokens = reasoning, cached
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, d := range order {
		rep.Departments = append(rep.Departments, *byDept[d])
	}

	// 每天的活跃人数与应用打开次数
	days := map[string]*ReportDay{}
	day := func(t time.Time) *ReportDay {
		k := t.Format("2006-01-02")
		if days[k] == nil {
			days[k] = &ReportDay{Date: k}
		}
		return days[k]
	}
	rows, err = s.db.QueryContext(ctx, "SELECT "+rg.dayExpr("x.created_at")+" AS day, COUNT(DISTINCT x.user_id) FROM ("+activeUnion+") x JOIN users u ON u.id = x.user_id WHERE u.account_type = 'human' GROUP BY day", activeArgs(from, to)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t time.Time
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			rows.Close()
			return nil, err
		}
		day(t).ActiveUsers = n
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, "SELECT "+rg.dayExpr("g.created_at")+` AS day, COUNT(*) FROM asset_usage g JOIN assets a ON a.id = g.asset_id AND a.kind = 'app'
		WHERE g.created_at >= ? AND g.created_at < ? GROUP BY day`, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t time.Time
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			rows.Close()
			return nil, err
		}
		day(t).AppRuns = n
	}
	rows.Close()
	off := time.Duration(offsetMin) * time.Minute
	for d := from.Add(off); d.Before(to.Add(off)); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		if x := days[k]; x != nil {
			rep.Days = append(rep.Days, *x)
		} else {
			rep.Days = append(rep.Days, ReportDay{Date: k})
		}
	}
	return rep, nil
}
