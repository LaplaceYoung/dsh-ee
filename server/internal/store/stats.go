package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 运营统计。「安装」指插件包（声明了 dsh.bundle 的包）的 tarball 下载；依赖包的下载另计为 downloads。
// 按天分组时用调用方给的时区偏移（分钟，东八区为 480），偏移在 SQL 里是整数常量。

// StatsRange 是统计窗口：From 起（UTC），按 OffsetMin 时区分天。
type StatsRange struct {
	From      time.Time
	OffsetMin int
}

func (r StatsRange) dayExpr(col string) string {
	return fmt.Sprintf("DATE(DATE_ADD(%s, INTERVAL %d MINUTE))", col, r.OffsetMin)
}

type DailyStat struct {
	Date         string `json:"date"`
	Installs     int    `json:"installs"`
	InstallUsers int    `json:"install_users"`
	Downloads    int    `json:"downloads"`
	Calls        int    `json:"calls"`
	CallErrors   int    `json:"call_errors"`
	CallUsers    int    `json:"call_users"`
	Tokens       int64  `json:"tokens"`
	// 用量细项，Tokens 的子集
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

// Daily 按天汇总安装与模型调用；没有数据的日子不返回，由调用方补零。
func (s *Store) Daily(ctx context.Context, r StatsRange) (map[string]*DailyStat, error) {
	out := map[string]*DailyStat{}
	day := func(t time.Time) *DailyStat {
		k := t.Format("2006-01-02")
		if out[k] == nil {
			out[k] = &DailyStat{Date: k}
		}
		return out[k]
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+r.dayExpr("d.created_at")+` AS day, COUNT(*),
		  COALESCE(SUM(CASE WHEN p.is_plugin = 1 THEN 1 ELSE 0 END), 0),
		  COUNT(DISTINCT CASE WHEN p.is_plugin = 1 THEN d.user_id END)
		FROM npm_downloads d LEFT JOIN npm_packages p ON p.name = d.name
		WHERE d.created_at >= ? GROUP BY day`, r.From)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t time.Time
		var downloads, installs, users int
		if err := rows.Scan(&t, &downloads, &installs, &users); err != nil {
			rows.Close()
			return nil, err
		}
		d := day(t)
		d.Downloads, d.Installs, d.InstallUsers = downloads, installs, users
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `SELECT `+r.dayExpr("created_at")+` AS day, COUNT(*),
		  COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0),
		  COUNT(DISTINCT user_id), COALESCE(SUM(total_tokens), 0),
		  COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cached_tokens), 0)
		FROM usage_logs WHERE created_at >= ? GROUP BY day`, r.From)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t time.Time
		var calls, errs, users int
		var tokens, reasoning, cached int64
		if err := rows.Scan(&t, &calls, &errs, &users, &tokens, &reasoning, &cached); err != nil {
			return nil, err
		}
		d := day(t)
		d.Calls, d.CallErrors, d.CallUsers, d.Tokens = calls, errs, users, tokens
		d.ReasoningTokens, d.CachedTokens = reasoning, cached
	}
	return out, rows.Err()
}

type PluginStat struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Origin   string `json:"origin"`
	Installs int    `json:"installs"`
	Users    int    `json:"users"`
}

// TopPlugins 是窗口内安装最多的插件。
func (s *Store) TopPlugins(ctx context.Context, from time.Time, limit int) ([]PluginStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.name, MAX(p.title), MAX(p.origin), COUNT(*) AS n, COUNT(DISTINCT d.user_id)
		FROM npm_downloads d JOIN npm_packages p ON p.name = d.name
		WHERE d.created_at >= ? AND p.is_plugin = 1
		GROUP BY d.name ORDER BY n DESC, d.name LIMIT ?`, from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginStat{}
	for rows.Next() {
		var p PluginStat
		if err := rows.Scan(&p.Name, &p.Title, &p.Origin, &p.Installs, &p.Users); err != nil {
			return nil, err
		}
		if p.Title == "" {
			p.Title = p.Name
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type ModelStat struct {
	Model  string `json:"model"`
	Calls  int    `json:"calls"`
	Users  int    `json:"users"`
	Tokens int64  `json:"tokens"`
	// 用量细项，Tokens 的子集
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

// TopModels 是窗口内 token 用得最多的模型（按员工调用时写的对外名称）。
func (s *Store) TopModels(ctx context.Context, from time.Time, limit int) ([]ModelStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT model, COUNT(*), COUNT(DISTINCT user_id), COALESCE(SUM(total_tokens), 0) AS t,
		  COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cached_tokens), 0)
		FROM usage_logs WHERE created_at >= ?
		GROUP BY model ORDER BY t DESC, model LIMIT ?`, from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelStat{}
	for rows.Next() {
		var m ModelStat
		if err := rows.Scan(&m.Model, &m.Calls, &m.Users, &m.Tokens, &m.ReasoningTokens, &m.CachedTokens); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type TokenStat struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Username    string `json:"username"`
	AccountType string `json:"account_type"`
	Tokens      int64  `json:"tokens"`
	// 用量细项，Tokens 的子集
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

func (s *Store) TokenStats(ctx context.Context, from time.Time, limit int) ([]TokenStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.name, u.username, u.account_type, COALESCE(SUM(l.total_tokens), 0) AS tokens,
		  COALESCE(SUM(l.reasoning_tokens), 0), COALESCE(SUM(l.cached_tokens), 0)
		FROM usage_logs l JOIN access_tokens t ON t.id = l.token_id JOIN users u ON u.id = t.user_id
		WHERE l.created_at >= ? GROUP BY t.id, t.name, u.username, u.account_type ORDER BY tokens DESC, t.id LIMIT ?`, from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TokenStat{}
	for rows.Next() {
		var stat TokenStat
		if err := rows.Scan(&stat.ID, &stat.Name, &stat.Username, &stat.AccountType, &stat.Tokens, &stat.ReasoningTokens, &stat.CachedTokens); err != nil {
			return nil, err
		}
		out = append(out, stat)
	}
	return out, rows.Err()
}

type DepartmentStat struct {
	Department  string `json:"department"`
	Users       int    `json:"users"`
	ActiveUsers int    `json:"active_users"`
	Installs    int    `json:"installs"`
	Calls       int    `json:"calls"`
	Tokens      int64  `json:"tokens"`
	// 用量细项，Tokens 的子集
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

// Departments 按部门汇总：启用中的账号数、窗口内活跃账号（装过插件或调用过模型）、安装与模型用量。
func (s *Store) Departments(ctx context.Context, from time.Time) ([]*DepartmentStat, error) {
	byDept := map[string]*DepartmentStat{}
	var order []string
	get := func(d string) *DepartmentStat {
		if byDept[d] == nil {
			byDept[d] = &DepartmentStat{Department: d}
			order = append(order, d)
		}
		return byDept[d]
	}
	type q struct {
		sql  string
		args []any
		fill func(d *DepartmentStat, a, b int64)
	}
	queries := []q{
		{"SELECT department, COUNT(*), 0 FROM users WHERE disabled = 0 GROUP BY department", nil,
			func(d *DepartmentStat, a, _ int64) { d.Users = int(a) }},
		{`SELECT u.department, COUNT(DISTINCT x.user_id), 0 FROM (
			  SELECT user_id FROM npm_downloads WHERE created_at >= ? AND user_id IS NOT NULL
			  UNION SELECT user_id FROM usage_logs WHERE created_at >= ?
			) x JOIN users u ON u.id = x.user_id GROUP BY u.department`, []any{from, from},
			func(d *DepartmentStat, a, _ int64) { d.ActiveUsers = int(a) }},
		{`SELECT u.department, COUNT(*), 0 FROM npm_downloads d
			JOIN npm_packages p ON p.name = d.name AND p.is_plugin = 1
			JOIN users u ON u.id = d.user_id
			WHERE d.created_at >= ? GROUP BY u.department`, []any{from},
			func(d *DepartmentStat, a, _ int64) { d.Installs = int(a) }},
		{`SELECT u.department, COUNT(*), COALESCE(SUM(l.total_tokens), 0) FROM usage_logs l
			JOIN users u ON u.id = l.user_id
			WHERE l.created_at >= ? GROUP BY u.department`, []any{from},
			func(d *DepartmentStat, a, b int64) { d.Calls, d.Tokens = int(a), b }},
	}
	for _, qq := range queries {
		rows, err := s.db.QueryContext(ctx, qq.sql, qq.args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var dept string
			var a, b int64
			if err := rows.Scan(&dept, &a, &b); err != nil {
				rows.Close()
				return nil, err
			}
			qq.fill(get(dept), a, b)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	// 用量细项单独查一次，不必为两个可选列改写上面的通用扫描循环。
	rows, err := s.db.QueryContext(ctx, `SELECT u.department, COALESCE(SUM(l.reasoning_tokens), 0), COALESCE(SUM(l.cached_tokens), 0)
		FROM usage_logs l JOIN users u ON u.id = l.user_id
		WHERE l.created_at >= ? GROUP BY u.department`, from)
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
		x := get(d)
		x.ReasoningTokens, x.CachedTokens = reasoning, cached
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]*DepartmentStat, 0, len(order))
	for _, d := range order {
		out = append(out, byDept[d])
	}
	return out, nil
}

type PlatformStat struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Users    int    `json:"users"`
	Installs int    `json:"installs"`
}

// Platforms 是窗口内下载过包的终端系统与架构（来自 npm 客户端的 User-Agent）。
// Users 按账号去重；匿名下载只计入 Installs。
func (s *Store) Platforms(ctx context.Context, from time.Time) ([]PlatformStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT os, arch, COUNT(DISTINCT user_id) AS u, COUNT(*) AS n
		FROM npm_downloads WHERE created_at >= ?
		GROUP BY os, arch ORDER BY u DESC, n DESC`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlatformStat{}
	for rows.Next() {
		var p PlatformStat
		if err := rows.Scan(&p.OS, &p.Arch, &p.Users, &p.Installs); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type Totals struct {
	Users       int `json:"users"`
	ActiveUsers int `json:"active_users"`
}

func (s *Store) UserTotals(ctx context.Context, from time.Time) (Totals, error) {
	var t Totals
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE disabled = 0").Scan(&t.Users); err != nil {
		return t, err
	}
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
		  SELECT user_id FROM npm_downloads WHERE created_at >= ? AND user_id IS NOT NULL
		  UNION SELECT user_id FROM usage_logs WHERE created_at >= ?
		) x`, from, from).Scan(&t.ActiveUsers)
	return t, err
}

type StorageStat struct {
	Origin   string `json:"origin"`
	Versions int    `json:"versions"`
	Bytes    int64  `json:"bytes"`
}

// Storage 按来源（hosted / upstream）统计已存的 tarball。
func (s *Store) Storage(ctx context.Context) ([]StorageStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.origin, COUNT(*), COALESCE(SUM(v.size), 0)
		FROM npm_versions v JOIN npm_packages p ON p.name = v.name GROUP BY p.origin`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StorageStat{}
	for rows.Next() {
		var st StorageStat
		if err := rows.Scan(&st.Origin, &st.Versions, &st.Bytes); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ---- 个人视角 ----

type InstalledPlugin struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
}

// UserInstalls 是用户装过的插件，每个插件取最近一次下载的版本。
// 只有下载记录，卸载不会通知服务端，所以这是「装过」而不是「当前已装」。
func (s *Store) UserInstalls(ctx context.Context, userID int64, limit int) ([]InstalledPlugin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.name, d.version, d.created_at FROM npm_downloads d
		JOIN (
		  SELECT x.name, MAX(x.id) AS mid FROM npm_downloads x
		  JOIN npm_packages p ON p.name = x.name AND p.is_plugin = 1
		  WHERE x.user_id = ? GROUP BY x.name
		) m ON m.mid = d.id
		ORDER BY d.created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InstalledPlugin{}
	for rows.Next() {
		var p InstalledPlugin
		if err := rows.Scan(&p.Name, &p.Version, &p.InstalledAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type PluginUsers struct {
	Name  string `json:"name"`
	Users int    `json:"users"`
}

// TeamPicks 是同部门同事窗口内装得最多、而这个用户还没装过的插件；department 为空时按全公司算。
func (s *Store) TeamPicks(ctx context.Context, userID int64, department string, from time.Time, limit int) ([]PluginUsers, error) {
	deptCond, args := "", []any{userID, from, userID}
	if department != "" {
		deptCond = " AND u.department = ?"
		args = append(args, department)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT d.name, COUNT(DISTINCT d.user_id) AS n FROM npm_downloads d
		JOIN users u ON u.id = d.user_id
		JOIN npm_packages p ON p.name = d.name AND p.is_plugin = 1
		WHERE d.user_id <> ? AND d.created_at >= ?
		  AND NOT EXISTS (SELECT 1 FROM npm_downloads m WHERE m.user_id = ? AND m.name = d.name)`+deptCond+`
		GROUP BY d.name ORDER BY n DESC, d.name LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginUsers{}
	for rows.Next() {
		var p PluginUsers
		if err := rows.Scan(&p.Name, &p.Users); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PackageUsers 统计这些包被多少位其他同事装过（按账号去重，不含 exclude 本人）。
func (s *Store) PackageUsers(ctx context.Context, names []string, exclude int64) (map[string]int, error) {
	out := map[string]int{}
	if len(names) == 0 {
		return out, nil
	}
	args := []any{exclude}
	for _, n := range names {
		args = append(args, n)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name, COUNT(DISTINCT user_id) FROM npm_downloads
		WHERE user_id IS NOT NULL AND user_id <> ? AND name IN (`+placeholders(len(names))+`) GROUP BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var c sql.NullInt64
		if err := rows.Scan(&n, &c); err != nil {
			return nil, err
		}
		out[n] = int(c.Int64)
	}
	return out, rows.Err()
}

// DistinctPackageUsers 统计装过这些包中任意一个的其他同事人数（不含 exclude 本人）。
func (s *Store) DistinctPackageUsers(ctx context.Context, names []string, exclude int64) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}
	args := []any{exclude}
	for _, n := range names {
		args = append(args, n)
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM npm_downloads
		WHERE user_id IS NOT NULL AND user_id <> ? AND name IN (`+placeholders(len(names))+`)`, args...).Scan(&n)
	return n, err
}
