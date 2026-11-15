package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// statsRange 解析 days（7–90，默认 14）与 tz（时区偏移分钟，东八区 480，默认 0）：
// 窗口从 tz 时区里 days-1 天前的零点开始，含今天。
func statsRange(r *http.Request, defDays int) (store.StatsRange, []string, string) {
	q := r.URL.Query()
	days := defDays
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			return store.StatsRange{}, nil, "days 应在 1–90 之间"
		}
		days = n
	}
	tz := 0
	if v := q.Get("tz"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < -14*60 || n > 14*60 {
			return store.StatsRange{}, nil, "tz 应是 -840 到 840 之间的分钟数"
		}
		tz = n
	}
	off := time.Duration(tz) * time.Minute
	local := time.Now().UTC().Add(off)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	dates := make([]string, days)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	return store.StatsRange{From: start.Add(-off), OffsetMin: tz}, dates, ""
}

type statsTotals struct {
	Installs    int   `json:"installs"`
	Downloads   int   `json:"downloads"`
	Calls       int   `json:"calls"`
	CallErrors  int   `json:"call_errors"`
	Tokens      int64 `json:"tokens"`
	Users       int   `json:"users"`
	ActiveUsers int   `json:"active_users"`
	// 用量细项，Tokens 的子集
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
}

// GET /api/admin/stats?days=14&tz=480：管理总览与运营数据。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	rg, dates, msg := statsRange(r, 14)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	daily, err := s.store.Daily(ctx, rg)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var totals statsTotals
	series := make([]*store.DailyStat, len(dates))
	for i, d := range dates {
		v := daily[d]
		if v == nil {
			v = &store.DailyStat{Date: d}
		}
		series[i] = v
		totals.Installs += v.Installs
		totals.Downloads += v.Downloads
		totals.Calls += v.Calls
		totals.CallErrors += v.CallErrors
		totals.Tokens += v.Tokens
		totals.ReasoningTokens += v.ReasoningTokens
		totals.CachedTokens += v.CachedTokens
	}
	ut, err := s.store.UserTotals(ctx, rg.From)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	totals.Users, totals.ActiveUsers = ut.Users, ut.ActiveUsers

	top, err := s.store.TopPlugins(ctx, rg.From, 10)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	models, err := s.store.TopModels(ctx, rg.From, 8)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	depts, err := s.store.Departments(ctx, rg.From)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	platforms, err := s.store.Platforms(ctx, rg.From)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	storage, err := s.store.Storage(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	payload := map[string]any{
		"from":        rg.From,
		"daily":       series,
		"totals":      totals,
		"top_plugins": top,
		"top_models":  models,
		"departments": nonNil(depts),
		"platforms":   platforms,
		"storage":     storage,
	}
	if r.URL.Query().Get("group_by") == "token" {
		tokenStats, err := s.store.TokenStats(ctx, rg.From, 100)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		payload["token_stats"] = nonNil(tokenStats)
	}
	writeJSON(w, http.StatusOK, payload)
}

type installedItem struct {
	store.InstalledPlugin
	Title string `json:"title"`
	// 员工当前能装到的版本；插件已下架时为空。
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
}

type pickItem struct {
	store.PluginUsers
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Category    string `json:"category"`
	Origin      string `json:"origin"`
}

type contributionItem struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Pending int    `json:"pending"`
	// 装过它的其他同事人数
	Users int `json:"users"`
}

// GET /api/me/overview：工作台的个人数据——装过的插件、同部门在用、我分享的。
func (s *Server) handleMyOverview(w http.ResponseWriter, r *http.Request) {
	me := currentUser(r)
	ctx := r.Context()
	items, _, err := s.catalog(r, true)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	byName := map[string]*catalogItem{}
	for _, it := range items {
		byName[it.Name] = it
	}

	inst, err := s.store.UserInstalls(ctx, me.ID, 50)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	installed := make([]installedItem, 0, len(inst))
	updates := 0
	for _, p := range inst {
		it := installedItem{InstalledPlugin: p, Title: p.Name}
		if c := byName[p.Name]; c != nil {
			it.Title = c.Title
			if c.Status == store.StatusPublished {
				it.Latest = c.Version
				it.HasUpdate = registry.CompareVersions(c.Version, p.Version) > 0
			}
		}
		if it.HasUpdate {
			updates++
		}
		installed = append(installed, it)
	}

	// 先按部门找，部门里没人装过别的就退回全公司
	scope := "department"
	picks, err := s.store.TeamPicks(ctx, me.ID, me.Department, time.Now().AddDate(0, 0, -90), 20)
	if err == nil && (me.Department == "" || len(picks) == 0) {
		scope = "company"
		picks, err = s.store.TeamPicks(ctx, me.ID, "", time.Now().AddDate(0, 0, -90), 20)
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	pickOut := []pickItem{}
	for _, p := range picks {
		c := byName[p.Name]
		if c == nil || c.Status != store.StatusPublished {
			continue
		}
		pickOut = append(pickOut, pickItem{PluginUsers: p, Title: c.Title, Description: c.Description, Kind: c.Kind, Category: c.Category, Origin: c.Origin})
		if len(pickOut) == 5 {
			break
		}
	}

	var mineNames []string
	for _, it := range items {
		if it.Origin == "internal" && it.Owner == me.Username {
			mineNames = append(mineNames, it.Name)
		}
	}
	users, err := s.store.PackageUsers(ctx, mineNames, me.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	reach, err := s.store.DistinctPackageUsers(ctx, mineNames, me.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	mine := make([]contributionItem, 0, len(mineNames))
	for _, n := range mineNames {
		c := byName[n]
		mine = append(mine, contributionItem{Name: n, Title: c.Title, Version: c.Version, Status: c.Status, Pending: c.Pending, Users: users[n]})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"installed":     installed,
		"updates":       updates,
		"picks":         pickOut,
		"picks_scope":   scope,
		"contributions": mine,
		"reach":         reach,
	})
}
