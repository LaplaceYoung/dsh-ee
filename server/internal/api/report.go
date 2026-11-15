package api

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 月度 AI 落地报告：活跃率、各应用使用量、热门问题、好评率、估算节省的工时。
// 网页端渲染成可打印的页面，浏览器另存 PDF；这里只出数据和估算。

const reportParamsKey = "report_params"

// reportParams 是估算节省工时的参数，管理员可以按公司实际情况调整。
type reportParams struct {
	// 打开一次应用、调用一次企业系统工具、查到一次资料，分别替员工省下多少分钟
	MinutesPerAppRun   float64 `json:"minutesPerAppRun"`
	MinutesPerToolCall float64 `json:"minutesPerToolCall"`
	MinutesPerKBHit    float64 `json:"minutesPerKbHit"`
	// 每小时人力成本（元），0 表示不估算金额
	HourlyCost float64 `json:"hourlyCost"`
}

func defaultReportParams() reportParams {
	return reportParams{MinutesPerAppRun: 10, MinutesPerToolCall: 3, MinutesPerKBHit: 5, HourlyCost: 0}
}

func (s *Server) loadReportParams(r *http.Request) (reportParams, error) {
	p := defaultReportParams()
	_, err := s.store.GetSettingJSON(r.Context(), reportParamsKey, &p)
	return p, err
}

type reportEstimate struct {
	AppHours  float64 `json:"app_hours"`
	ToolHours float64 `json:"tool_hours"`
	KBHours   float64 `json:"kb_hours"`
	Hours     float64 `json:"hours"`
	// 估算金额（元）；没设人力成本时为 0
	Cost float64 `json:"cost"`
}

func estimate(rep *store.MonthlyReport, p reportParams) reportEstimate {
	round := func(x float64) float64 { return math.Round(x*10) / 10 }
	e := reportEstimate{
		AppHours:  round(float64(rep.AppRuns) * p.MinutesPerAppRun / 60),
		ToolHours: round(float64(rep.ToolCalls-rep.ToolErrors) * p.MinutesPerToolCall / 60),
		KBHours:   round(float64(rep.KBHits) * p.MinutesPerKBHit / 60),
	}
	e.Hours = round(e.AppHours + e.ToolHours + e.KBHours)
	e.Cost = math.Round(e.Hours * p.HourlyCost)
	return e
}

// monthWindow 解析 month（YYYY-MM，默认上个月）与 tz（时区偏移分钟），返回那个月在 UTC 的 [from, to)
// 与上个月的起点。月份按当地时间的月初算，不能拿 UTC 时刻直接减一个月（10 月 31 日减一个月会变成 10 月 1 日）。
func monthWindow(r *http.Request) (string, time.Time, time.Time, time.Time, int, string) {
	q := r.URL.Query()
	tz := 0
	if v := q.Get("tz"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < -14*60 || n > 14*60 {
			return "", time.Time{}, time.Time{}, time.Time{}, 0, "tz 应是 -840 到 840 之间的分钟数"
		}
		tz = n
	}
	off := time.Duration(tz) * time.Minute
	local := time.Now().UTC().Add(off)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	if v := q.Get("month"); v != "" {
		t, err := time.Parse("2006-01", v)
		if err != nil {
			return "", time.Time{}, time.Time{}, time.Time{}, 0, "month 的格式是 YYYY-MM"
		}
		start = t
	}
	return start.Format("2006-01"), start.Add(-off), start.AddDate(0, 1, 0).Add(-off), start.AddDate(0, -1, 0).Add(-off), tz, ""
}

// GET /api/admin/report/monthly?month=2026-10&tz=480
func (s *Server) handleMonthlyReport(w http.ResponseWriter, r *http.Request) {
	month, from, to, prevFrom, tz, msg := monthWindow(r)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	rep, err := s.store.Report(r.Context(), from, to, tz)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 上个月做对比
	prev, err := s.store.Report(r.Context(), prevFrom, from, tz)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	p, err := s.loadReportParams(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"month":    month,
		"org_name": set.OrgName,
		"report":   rep,
		"previous": map[string]any{"users": prev.Users, "active_users": prev.ActiveUsers, "app_runs": prev.AppRuns, "tool_calls": prev.ToolCalls, "kb_searches": prev.KBSearches, "tokens": prev.Tokens,
			"reasoning_tokens": prev.ReasoningTokens, "cached_tokens": prev.CachedTokens},
		"params":   p,
		"estimate": estimate(rep, p),
		// 好评率：还没有收集评价（👍👎 在后续版本上线），为 null 时页面写明
		"satisfaction": nil,
		"generated_at": time.Now().UTC(),
	})
}

// PUT /api/admin/report/params：调整估算参数。
func (s *Server) handlePutReportParams(w http.ResponseWriter, r *http.Request) {
	var p reportParams
	if !readJSON(w, r, &p) {
		return
	}
	for _, v := range []float64{p.MinutesPerAppRun, p.MinutesPerToolCall, p.MinutesPerKBHit} {
		if v < 0 || v > 480 {
			writeErr(w, http.StatusBadRequest, "每次节省的分钟数应在 0–480 之间")
			return
		}
	}
	if p.HourlyCost < 0 || p.HourlyCost > 100000 {
		writeErr(w, http.StatusBadRequest, "每小时人力成本应在 0–100000 之间")
		return
	}
	if err := s.store.PutSettingJSON(r.Context(), reportParamsKey, p, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditSettingsUpdate, "report_params", map[string]any{"app": p.MinutesPerAppRun, "tool": p.MinutesPerToolCall, "kb": p.MinutesPerKBHit, "hourly_cost": p.HourlyCost})
	writeJSON(w, http.StatusOK, p)
}
