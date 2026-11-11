package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 应用自带数据（第一版 M13，docs/product-v1.md §10.3）。
//
// 应用定义里的 tables 声明几张小表；行存在通用表 app_rows 里（JSON）。能用这个应用的人就能用它的数据，
// 谁能看、能改哪些行由表的 visibility 决定，作者和管理员看全部。网页走 /api/apps/{slug}/data/…，
// DSH 里的模型走 /mcp 上的 app__rows（只读）与 app__write（新增 / 修改直接执行并记审计，删除要确认）。
// 工作台的数据源可以写 {"table": …}，表单的 submit 可以写 {"table": …} 直接插入一行。

const (
	// appConn 是应用数据工具在网关上的「系统连接」名：app__rows、app__write
	appConn = "app"

	appMaxTables   = 5
	appMaxFields   = 30
	appRowLimit    = 10000
	appRowMaxBytes = 16 << 10
	appTextMax     = 2000
	appWriteBatch  = 100
	appListMax     = 200
	// 删除的行（以及删除应用后它的数据）保留这么久，之后清掉
	appRowKeep = 30 * 24 * time.Hour
)

var (
	appFieldKinds = []string{"text", "number", "money", "date", "select", "bool", "user"}
	// 行里的这几个键由系统填，字段不能用这些名字
	appReserved     = []string{"id", "owner", "owner_id", "created_at", "updated_at", "mine"}
	appVisibilities = []string{"own", "shared", "author"}
	appDateRe       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$`)
)

type appTable struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// own：各自填写（每人只看自己的行）；shared：共同维护；author：作者发布（其他人只读）
	Visibility string     `json:"visibility,omitempty"`
	Fields     []appField `json:"fields"`
}

type appField struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
	// 不再用的字段改成隐藏：数据保留，不再显示、不能再写
	Hidden bool `json:"hidden,omitempty"`
}

func (t *appTable) field(name string) *appField {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

func findTable(tables []appTable, name string) *appTable {
	for i := range tables {
		if tables[i].Name == name {
			return &tables[i]
		}
	}
	return nil
}

// validateTables 检查并规范化应用的表定义。
func validateTables(tables []appTable) string {
	if len(tables) > appMaxTables {
		return fmt.Sprintf("一个应用最多 %d 张表", appMaxTables)
	}
	names := map[string]bool{}
	for i := range tables {
		t := &tables[i]
		where := fmt.Sprintf("第 %d 张表", i+1)
		t.Name, t.Title = strings.TrimSpace(t.Name), strings.TrimSpace(t.Title)
		if !wbFieldNameRe.MatchString(t.Name) || len(t.Name) > 40 {
			return where + "：name 只能用字母、数字和下划线，不能以数字开头，最多 40 个字符"
		}
		if names[t.Name] {
			return where + "：name " + t.Name + " 重复了"
		}
		names[t.Name] = true
		if t.Title == "" || !limitRunes(t.Title, 40) {
			return where + "：标题必填，最多 40 个字"
		}
		if t.Visibility == "" {
			t.Visibility = "own"
		}
		if !slices.Contains(appVisibilities, t.Visibility) {
			return where + "：visibility 只能是 own（各自填写）/ shared（共同维护）/ author（作者发布）"
		}
		if len(t.Fields) == 0 || len(t.Fields) > appMaxFields {
			return fmt.Sprintf("%s：要有 1～%d 个字段", where, appMaxFields)
		}
		fields := map[string]bool{}
		for k := range t.Fields {
			f := &t.Fields[k]
			fw := fmt.Sprintf("%s第 %d 个字段", where, k+1)
			f.Name, f.Title = strings.TrimSpace(f.Name), strings.TrimSpace(f.Title)
			if !wbFieldNameRe.MatchString(f.Name) || len(f.Name) > 40 {
				return fw + "：name 只能用字母、数字和下划线，不能以数字开头"
			}
			if slices.Contains(appReserved, f.Name) {
				return fw + "：name 不能用 " + f.Name + "（系统字段）"
			}
			if fields[f.Name] {
				return fw + "：name " + f.Name + " 重复了"
			}
			fields[f.Name] = true
			if f.Title == "" || !limitRunes(f.Title, 40) {
				return fw + "：显示名必填，最多 40 个字"
			}
			if !slices.Contains(appFieldKinds, f.Kind) {
				return fw + "：类型只能是 " + strings.Join(appFieldKinds, " / ")
			}
			if f.Kind == "select" {
				if len(f.Options) == 0 || len(f.Options) > 50 {
					return fw + "：下拉选项要有 1～50 项"
				}
				for j, o := range f.Options {
					o = strings.TrimSpace(o)
					if o == "" || !limitRunes(o, 100) {
						return fw + "：下拉选项不能为空，每项最多 100 个字"
					}
					f.Options[j] = o
				}
			} else {
				f.Options = nil
			}
			if f.Kind == "user" {
				f.Required = false
			}
		}
	}
	return ""
}

// widenable 是改字段类型时允许的方向：只能放宽，已有的数据不会变得不合法。
func widenable(from, to string) bool {
	if from == to {
		return true
	}
	switch to {
	case "text":
		return true
	case "money":
		return from == "number"
	case "number":
		return from == "money"
	}
	return false
}

// checkTableChange 对照线上版本检查新版本的表定义：字段只能加、改标题、加选项、隐藏、放宽类型；
// counts 是每张表现有的行数，有数据的表不能删。
func checkTableChange(old, next []appTable, counts map[string]int) string {
	for _, ot := range old {
		nt := findTable(next, ot.Name)
		if nt == nil {
			if counts[ot.Name] > 0 {
				return fmt.Sprintf("表「%s」里还有 %d 行数据，不能删；不用了可以不在工作台里显示", ot.Title, counts[ot.Name])
			}
			continue
		}
		for _, of := range ot.Fields {
			nf := nt.field(of.Name)
			if nf == nil {
				return fmt.Sprintf("表「%s」的字段「%s」不能删，数据会丢；不用了请改成隐藏（hidden）", ot.Title, of.Title)
			}
			if !widenable(of.Kind, nf.Kind) {
				return fmt.Sprintf("表「%s」的字段「%s」不能从 %s 改成 %s，已有的数据会不合法；只能改成 text，或在 number 和 money 之间换", ot.Title, of.Title, of.Kind, nf.Kind)
			}
			if of.Kind == "select" && nf.Kind == "select" {
				for _, o := range of.Options {
					if !slices.Contains(nf.Options, o) {
						return fmt.Sprintf("表「%s」的字段「%s」不能去掉选项「%s」，已有的数据会不合法", ot.Title, of.Title, o)
					}
				}
			}
		}
	}
	return ""
}

// ---- 谁能用 ----

// appTarget 是一次数据操作对着的应用与表。
type appTarget struct {
	asset *store.Asset
	spec  *assetSpec
	table *appTable
	// 作者或管理员：看全部行、改任何行
	admin bool
}

type appDataError struct {
	status int
	msg    string
}

func (e *appDataError) Error() string { return e.msg }

func appErr(status int, format string, args ...any) error {
	return &appDataError{status: status, msg: fmt.Sprintf(format, args...)}
}

// appTargetFor 找到员工能用的应用与表：已上架且在范围内、自己是作者，或有管理能力中心的权限。
func (s *Server) appTargetFor(ctx context.Context, v viewer, slug, table string) (*appTarget, error) {
	a, err := s.store.GetAssetBySlug(ctx, slug)
	if errors.Is(err, store.ErrNotFound) || (err == nil && a.Kind != store.KindApp) {
		return nil, appErr(http.StatusNotFound, "没有应用 %s", slug)
	}
	if err != nil {
		return nil, err
	}
	admin := v.owns(a) || v.manages(store.KindApp)
	if !admin && !available(v, a) {
		return nil, appErr(http.StatusNotFound, "没有应用 %s，或你不在它的可用范围内", slug)
	}
	if a.LiveVersion == 0 {
		return nil, appErr(http.StatusConflict, "应用 %s 还没有上线的版本", slug)
	}
	live, err := s.store.LiveAssetVersions(ctx, []int64{a.ID})
	if err != nil {
		return nil, err
	}
	lv := live[a.ID]
	if lv == nil {
		return nil, appErr(http.StatusConflict, "应用 %s 还没有上线的版本", slug)
	}
	sp := parseSpec(lv.Spec)
	t := findTable(sp.Tables, table)
	if t == nil {
		names := make([]string, 0, len(sp.Tables))
		for _, x := range sp.Tables {
			names = append(names, x.Name)
		}
		if len(names) == 0 {
			return nil, appErr(http.StatusNotFound, "应用 %s 没有数据表", slug)
		}
		return nil, appErr(http.StatusNotFound, "应用 %s 没有表 %s，有的是：%s", slug, table, strings.Join(names, "、"))
	}
	return &appTarget{asset: a, spec: sp, table: t, admin: admin}, nil
}

func (t *appTarget) canRead(v viewer, row *store.AppRow) bool {
	return t.admin || t.table.Visibility != "own" || row.OwnerID == v.user.ID
}

func (t *appTarget) canWrite(v viewer, row *store.AppRow) bool {
	switch {
	case t.admin:
		return true
	case t.table.Visibility == "shared":
		return true
	case t.table.Visibility == "own":
		return row == nil || row.OwnerID == v.user.ID
	}
	return false
}

func (t *appTarget) visibilityText() string {
	switch t.table.Visibility {
	case "shared":
		return "共同维护：能用这个应用的人都能看、都能改"
	case "author":
		return "作者发布：只有作者和管理员能写，其他人只读"
	}
	return "各自填写：每人只看到、只能改自己的行；作者和管理员看全部"
}

// ---- 值的校验 ----

// cleanRow 按字段定义校验一行（insert 时 full 为 true，要检查必填；update 时只看传了的字段）。
// user 字段由系统填成当前员工，传了也忽略。返回规范化后的值。
func cleanRow(t *appTable, in map[string]any, full bool, who *store.User) (map[string]any, string) {
	out := map[string]any{}
	for k, val := range in {
		f := t.field(k)
		if f == nil {
			if slices.Contains(appReserved, k) {
				continue
			}
			return nil, fmt.Sprintf("表「%s」没有字段 %s", t.Title, k)
		}
		if f.Hidden {
			return nil, fmt.Sprintf("字段「%s」已经停用，不能再写", f.Title)
		}
		if f.Kind == "user" {
			continue
		}
		if val == nil {
			out[k] = nil
			continue
		}
		v, msg := cleanValue(f, val)
		if msg != "" {
			return nil, fmt.Sprintf("字段「%s」%s", f.Title, msg)
		}
		out[k] = v
	}
	if full {
		for _, f := range t.Fields {
			if f.Kind == "user" && !f.Hidden {
				out[f.Name] = displayOf(who)
				continue
			}
			if f.Required && !f.Hidden {
				if v, ok := out[f.Name]; !ok || v == nil || v == "" {
					return nil, fmt.Sprintf("字段「%s」必填", f.Title)
				}
			}
		}
	}
	return out, ""
}

func displayOf(u *store.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

func cleanValue(f *appField, val any) (any, string) {
	switch f.Kind {
	case "text":
		s, ok := val.(string)
		if !ok {
			if n, isNum := val.(float64); isNum {
				s = strconv.FormatFloat(n, 'f', -1, 64)
			} else {
				return nil, "要是文字"
			}
		}
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) > appTextMax {
			return nil, fmt.Sprintf("最多 %d 个字", appTextMax)
		}
		return s, ""
	case "number", "money":
		switch n := val.(type) {
		case float64:
			return n, ""
		case int:
			return float64(n), ""
		case int64:
			return float64(n), ""
		case json.Number:
			x, err := n.Float64()
			if err != nil {
				return nil, "要是数字"
			}
			return x, ""
		case string:
			x, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(n), ",", ""), 64)
			if err != nil {
				return nil, "要是数字"
			}
			return x, ""
		}
		return nil, "要是数字"
	case "date":
		s, ok := val.(string)
		if !ok || !appDateRe.MatchString(strings.TrimSpace(s)) {
			return nil, "要是日期，例如 2026-11-07"
		}
		return strings.TrimSpace(s), ""
	case "select":
		s, ok := val.(string)
		if !ok || !slices.Contains(f.Options, strings.TrimSpace(s)) {
			return nil, "只能是：" + strings.Join(f.Options, " / ")
		}
		return strings.TrimSpace(s), ""
	case "bool":
		switch b := val.(type) {
		case bool:
			return b, ""
		case string:
			switch strings.TrimSpace(b) {
			case "true", "是", "1":
				return true, ""
			case "false", "否", "0":
				return false, ""
			}
		}
		return nil, "要是 true / false"
	}
	return nil, "类型不对"
}

// ---- 读写 ----

type appListReq struct {
	Filter  map[string]any `json:"filter"`
	Q       string         `json:"q"`
	Sort    string         `json:"sort"`
	Order   string         `json:"order"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	Mine    bool           `json:"mine"`
	GroupBy string         `json:"group_by"`
}

// rowView 是回给网页和模型的一行：字段值加上系统字段。
func (s *Server) rowViews(ctx context.Context, v viewer, t *appTarget, rows []*store.AppRow) []map[string]any {
	names := map[int64]string{}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{}
		_ = json.Unmarshal(r.Data, &m)
		for _, f := range t.table.Fields {
			if f.Hidden {
				delete(m, f.Name)
			}
		}
		name, ok := names[r.OwnerID]
		if !ok {
			if u, err := s.store.GetUser(ctx, r.OwnerID); err == nil {
				name = displayOf(u)
			}
			names[r.OwnerID] = name
		}
		m["id"], m["owner"], m["mine"] = r.ID, name, r.OwnerID == v.user.ID
		m["created_at"], m["updated_at"] = r.CreatedAt, r.UpdatedAt
		out = append(out, m)
	}
	return out
}

func (s *Server) appList(ctx context.Context, v viewer, t *appTarget, req appListReq) (map[string]any, error) {
	q := store.AppRowQuery{Equal: map[string]string{}, Text: strings.TrimSpace(req.Q), Limit: req.Limit, Offset: req.Offset, Desc: req.Order != "asc"}
	if q.Limit <= 0 || q.Limit > appListMax {
		q.Limit = 50
	}
	if (t.table.Visibility == "own" && !t.admin) || req.Mine {
		q.OwnerID = &v.user.ID
	}
	for k, val := range req.Filter {
		f := t.table.field(k)
		if f == nil || f.Hidden {
			return nil, appErr(http.StatusBadRequest, "筛选的字段 %s 不在表「%s」里", k, t.table.Title)
		}
		switch x := val.(type) {
		case string:
			q.Equal[k] = x
		case float64:
			q.Equal[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			q.Equal[k] = strconv.FormatBool(x)
		default:
			return nil, appErr(http.StatusBadRequest, "筛选值只能是文字、数字或 true / false")
		}
	}
	if q.Text != "" {
		for _, f := range t.table.Fields {
			if !f.Hidden && (f.Kind == "text" || f.Kind == "select" || f.Kind == "user") {
				q.TextFields = append(q.TextFields, f.Name)
			}
		}
	}
	if req.GroupBy != "" {
		f := t.table.field(req.GroupBy)
		if f == nil || f.Hidden {
			return nil, appErr(http.StatusBadRequest, "分组的字段 %s 不在表「%s」里", req.GroupBy, t.table.Title)
		}
		groups, err := s.store.GroupAppRows(ctx, t.asset.ID, t.table.Name, f.Name, q)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(groups))
		total := 0
		for _, g := range groups {
			items = append(items, map[string]any{f.Name: g.Value, "count": g.Count})
			total += g.Count
		}
		return map[string]any{"total": total, "items": items}, nil
	}
	if req.Sort != "" {
		if f := t.table.field(req.Sort); (f == nil || f.Hidden) && req.Sort != "created_at" && req.Sort != "updated_at" {
			return nil, appErr(http.StatusBadRequest, "排序的字段 %s 不在表「%s」里", req.Sort, t.table.Title)
		}
		q.Sort = req.Sort
		if f := t.table.field(req.Sort); f != nil {
			q.SortNumeric = f.Kind == "number" || f.Kind == "money"
		}
	}
	rows, total, err := s.store.ListAppRows(ctx, t.asset.ID, t.table.Name, q)
	if err != nil {
		return nil, err
	}
	return map[string]any{"total": total, "items": s.rowViews(ctx, v, t, rows)}, nil
}

func (s *Server) appInsert(ctx context.Context, r *http.Request, v viewer, t *appTarget, in []map[string]any) ([]map[string]any, error) {
	if len(in) == 0 {
		return nil, appErr(http.StatusBadRequest, "要写入的行不能为空")
	}
	if len(in) > appWriteBatch {
		return nil, appErr(http.StatusBadRequest, "一次最多写 %d 行", appWriteBatch)
	}
	if !t.canWrite(v, nil) {
		return nil, appErr(http.StatusForbidden, "表「%s」只有作者和管理员能写", t.table.Title)
	}
	n, err := s.store.CountAppRows(ctx, t.asset.ID)
	if err != nil {
		return nil, err
	}
	if n+len(in) > appRowLimit {
		return nil, appErr(http.StatusConflict, "应用「%s」的数据已经有 %d 行，上限是 %d 行；数据多了建议放进数据中台", t.spec.Title, n, appRowLimit)
	}
	datas := make([][]byte, 0, len(in))
	for i, row := range in {
		clean, msg := cleanRow(t.table, row, true, v.user)
		if msg != "" {
			if len(in) > 1 {
				msg = fmt.Sprintf("第 %d 行：%s", i+1, msg)
			}
			return nil, appErr(http.StatusBadRequest, "%s", msg)
		}
		b, _ := json.Marshal(clean)
		if len(b) > appRowMaxBytes {
			return nil, appErr(http.StatusBadRequest, "一行最多 %d KB", appRowMaxBytes>>10)
		}
		datas = append(datas, b)
	}
	rows, err := s.store.InsertAppRows(ctx, t.asset.ID, t.table.Name, v.user.ID, datas)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	s.auditAs(r, v.user, auditAppDataInsert, t.asset.Slug+"/"+t.table.Name, map[string]any{"ids": ids})
	return s.rowViews(ctx, v, t, rows), nil
}

func (s *Server) appUpdate(ctx context.Context, r *http.Request, v viewer, t *appTarget, id int64, patch map[string]any) (map[string]any, error) {
	row, err := s.store.GetAppRow(ctx, t.asset.ID, t.table.Name, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, appErr(http.StatusNotFound, "表「%s」里没有第 %d 行", t.table.Title, id)
	}
	if err != nil {
		return nil, err
	}
	if !t.canRead(v, row) {
		return nil, appErr(http.StatusNotFound, "表「%s」里没有第 %d 行", t.table.Title, id)
	}
	if !t.canWrite(v, row) {
		return nil, appErr(http.StatusForbidden, "你不能改这一行（%s）", t.visibilityText())
	}
	clean, msg := cleanRow(t.table, patch, false, v.user)
	if msg != "" {
		return nil, appErr(http.StatusBadRequest, "%s", msg)
	}
	before := map[string]any{}
	_ = json.Unmarshal(row.Data, &before)
	after := map[string]any{}
	for k, val := range before {
		after[k] = val
	}
	for k, val := range clean {
		after[k] = val
	}
	for _, f := range t.table.Fields {
		if f.Required && !f.Hidden && f.Kind != "user" {
			if val, ok := after[f.Name]; !ok || val == nil || val == "" {
				return nil, appErr(http.StatusBadRequest, "字段「%s」必填", f.Title)
			}
		}
	}
	b, _ := json.Marshal(after)
	if len(b) > appRowMaxBytes {
		return nil, appErr(http.StatusBadRequest, "一行最多 %d KB", appRowMaxBytes>>10)
	}
	if err := s.store.UpdateAppRow(ctx, row.ID, b, v.user.ID); err != nil {
		return nil, err
	}
	s.auditAs(r, v.user, auditAppDataUpdate, t.asset.Slug+"/"+t.table.Name, map[string]any{"id": row.ID, "before": before, "after": after})
	row.Data, row.UpdatedAt, row.UpdatedBy = b, time.Now().UTC(), v.user.ID
	return s.rowViews(ctx, v, t, []*store.AppRow{row})[0], nil
}

// appDeletable 返回要删的行里员工能删的那些；有不能删的就报错，不做部分删除。
func (s *Server) appDeletable(ctx context.Context, v viewer, t *appTarget, ids []int64) ([]*store.AppRow, error) {
	if len(ids) == 0 {
		return nil, appErr(http.StatusBadRequest, "要指定删除哪些行（ids）")
	}
	if len(ids) > appWriteBatch {
		return nil, appErr(http.StatusBadRequest, "一次最多删 %d 行", appWriteBatch)
	}
	var rows []*store.AppRow
	for _, id := range ids {
		row, err := s.store.GetAppRow(ctx, t.asset.ID, t.table.Name, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && !t.canRead(v, row)) {
			return nil, appErr(http.StatusNotFound, "表「%s」里没有第 %d 行", t.table.Title, id)
		}
		if err != nil {
			return nil, err
		}
		if !t.canWrite(v, row) {
			return nil, appErr(http.StatusForbidden, "你不能删第 %d 行（%s）", id, t.visibilityText())
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Server) appDelete(ctx context.Context, r *http.Request, v viewer, t *appTarget, rows []*store.AppRow) (int, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	n, err := s.store.DeleteAppRows(ctx, t.asset.ID, t.table.Name, ids)
	if err != nil {
		return 0, err
	}
	s.auditAs(r, v.user, auditAppDataDelete, t.asset.Slug+"/"+t.table.Name, map[string]any{"ids": ids})
	return n, nil
}

// runAppRowPurge 每天清一次删除超过 30 天的行。
func (s *Server) runAppRowPurge(ctx context.Context) {
	for {
		if n, err := s.store.PurgeAppRows(ctx, appRowKeep); err != nil {
			s.log.Warn("清理已删除的应用数据失败", "err", err)
		} else if n > 0 {
			s.log.Info("清理了已删除超过 30 天的应用数据", "rows", n)
		}
		timer := time.NewTimer(24 * time.Hour)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// ---- 网页接口 ----

func (s *Server) writeAppErr(w http.ResponseWriter, r *http.Request, err error) {
	var ae *appDataError
	if errors.As(err, &ae) {
		writeErr(w, ae.status, ae.msg)
		return
	}
	s.internalErr(w, r, err)
}

func (s *Server) appTargetOf(w http.ResponseWriter, r *http.Request) (*appTarget, bool) {
	t, err := s.appTargetFor(r.Context(), viewerOf(r), r.PathValue("slug"), r.PathValue("table"))
	if err != nil {
		s.writeAppErr(w, r, err)
		return nil, false
	}
	return t, true
}

// GET /api/apps/{slug}/data：应用有哪些表、各有多少行、我能做什么。
func (s *Server) handleAppTables(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	a, err := s.store.GetAssetBySlug(r.Context(), r.PathValue("slug"))
	if err == nil && a.Kind != store.KindApp {
		err = store.ErrNotFound
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "没有这个应用")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	admin := v.owns(a) || v.manages(store.KindApp)
	if !admin && !available(v, a) {
		writeErr(w, http.StatusNotFound, "没有这个应用，或你不在它的可用范围内")
		return
	}
	out := []map[string]any{}
	if a.LiveVersion > 0 {
		live, err := s.store.LiveAssetVersions(r.Context(), []int64{a.ID})
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		counts := map[string]int{}
		if admin {
			if counts, err = s.store.AppTableCounts(r.Context(), a.ID); err != nil {
				s.internalErr(w, r, err)
				return
			}
		}
		if lv := live[a.ID]; lv != nil {
			for _, t := range parseSpec(lv.Spec).Tables {
				tgt := &appTarget{table: &t, admin: admin}
				item := map[string]any{"name": t.Name, "title": t.Title, "visibility": t.Visibility, "fields": t.Fields, "can_insert": tgt.canWrite(v, nil)}
				if admin {
					item["rows"] = counts[t.Name]
				}
				out = append(out, item)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": out, "admin": admin, "row_limit": appRowLimit})
}

// GET /api/apps/{slug}/data/{table}?q=&sort=&order=&limit=&offset=&mine=&group_by=&f.<字段>=值
func (s *Server) handleAppRows(w http.ResponseWriter, r *http.Request) {
	t, ok := s.appTargetOf(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := appListReq{Q: q.Get("q"), Sort: q.Get("sort"), Order: q.Get("order"), Mine: q.Get("mine") == "1", GroupBy: q.Get("group_by"), Filter: map[string]any{}}
	req.Limit, _ = strconv.Atoi(q.Get("limit"))
	req.Offset, _ = strconv.Atoi(q.Get("offset"))
	for k, vals := range q {
		if name, ok := strings.CutPrefix(k, "f."); ok && len(vals) > 0 {
			req.Filter[name] = vals[0]
		}
	}
	out, err := s.appList(r.Context(), viewerOf(r), t, req)
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/apps/{slug}/data/{table} {row} 或 {rows: […]}
func (s *Server) handleAppInsert(w http.ResponseWriter, r *http.Request) {
	t, ok := s.appTargetOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Row  map[string]any   `json:"row"`
		Rows []map[string]any `json:"rows"`
	}
	if !readJSONLimit(w, r, &req, 2<<20) {
		return
	}
	rows := req.Rows
	if req.Row != nil {
		rows = append([]map[string]any{req.Row}, rows...)
	}
	out, err := s.appInsert(r.Context(), r, viewerOf(r), t, rows)
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": out})
}

// PATCH /api/apps/{slug}/data/{table}/{id} {row}
func (s *Server) handleAppUpdate(w http.ResponseWriter, r *http.Request) {
	t, ok := s.appTargetOf(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "行号不对")
		return
	}
	var req struct {
		Row map[string]any `json:"row"`
	}
	if !readJSONLimit(w, r, &req, 1<<20) {
		return
	}
	out, err := s.appUpdate(r.Context(), r, viewerOf(r), t, id, req.Row)
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": out})
}

// DELETE /api/apps/{slug}/data/{table}/{id}
func (s *Server) handleAppDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := s.appTargetOf(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "行号不对")
		return
	}
	v := viewerOf(r)
	rows, err := s.appDeletable(r.Context(), v, t, []int64{id})
	if err == nil {
		_, err = s.appDelete(r.Context(), r, v, t, rows)
	}
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/apps/{slug}/data/{table}/export：作者和管理员导出 CSV（全部行，含隐藏字段）。
func (s *Server) handleAppExport(w http.ResponseWriter, r *http.Request) {
	t, ok := s.appTargetOf(w, r)
	if !ok {
		return
	}
	if !t.admin {
		writeErr(w, http.StatusForbidden, "只有应用的作者和管理员能导出数据")
		return
	}
	v := viewerOf(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, t.asset.Slug, t.table.Name))
	// 带 BOM，Excel 打开中文不乱码
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	header := []string{"id"}
	for _, f := range t.table.Fields {
		header = append(header, f.Title)
	}
	header = append(header, "填写人", "创建时间", "更新时间")
	_ = cw.Write(header)
	for offset := 0; ; offset += 500 {
		rows, _, err := s.store.ListAppRows(r.Context(), t.asset.ID, t.table.Name, store.AppRowQuery{Limit: 500, Offset: offset, Sort: "id"})
		if err != nil {
			s.log.Warn("导出应用数据失败", "err", err)
			break
		}
		for _, m := range s.rowViewsAll(r.Context(), v, rows) {
			line := []string{fmt.Sprint(m["id"])}
			for _, f := range t.table.Fields {
				line = append(line, csvCell(m[f.Name]))
			}
			line = append(line, fmt.Sprint(m["owner"]), m["created_at"].(time.Time).Format(time.RFC3339), m["updated_at"].(time.Time).Format(time.RFC3339))
			_ = cw.Write(line)
		}
		if len(rows) < 500 {
			break
		}
	}
	cw.Flush()
	s.audit(r, auditAppDataExport, t.asset.Slug+"/"+t.table.Name, nil)
}

// rowViewsAll 同 rowViews，但保留隐藏字段（导出用）。
func (s *Server) rowViewsAll(ctx context.Context, v viewer, rows []*store.AppRow) []map[string]any {
	all := &appTarget{table: &appTable{}}
	return s.rowViews(ctx, v, all, rows)
}

func csvCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		// 防止 Excel 把 = + - @ 开头的内容当公式执行
		if x != "" && strings.ContainsRune("=+-@", rune(x[0])) {
			return "'" + x
		}
		return x
	}
	return fmt.Sprint(v)
}

// ---- DSH 里的模型：/mcp 上的 app__ 工具 ----

// appToolTargets 是员工 DSH 里应用的数据表（目录里启用了的、自己是作者的），写进工具说明。
func (s *Server) appToolTargets(ctx context.Context, v viewer, c *dshCatalog) []string {
	var lines []string
	seen := map[string]bool{}
	add := func(slug string, sp *assetSpec) {
		if seen[slug] || sp == nil || len(sp.Tables) == 0 {
			return
		}
		seen[slug] = true
		for _, t := range sp.Tables {
			var fields []string
			for _, f := range t.Fields {
				if f.Hidden {
					continue
				}
				desc := f.Name + "（" + f.Title + "，" + f.Kind
				if f.Kind == "select" {
					desc += "：" + strings.Join(f.Options, "/")
				}
				if f.Required {
					desc += "，必填"
				}
				fields = append(fields, desc+"）")
			}
			lines = append(lines, fmt.Sprintf("- 应用 %s「%s」的表 %s「%s」（%s）：%s", slug, sp.Title, t.Name, t.Title, t.Visibility, strings.Join(fields, "、")))
		}
	}
	for _, app := range c.Apps {
		add(app.Name, c.specs[app.Name])
	}
	return lines
}

func appToolDefs(targets []string) []map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	integer := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	list := strings.Join(targets, "\n")
	common := map[string]any{"app": str("应用标识"), "table": str("表名")}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range common {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	return []map[string]any{
		{
			"name": appConn + "__rows",
			"description": "【应用数据】查应用自带的数据表。返回 total 和 items（每行带 id、owner 填写人、mine 是不是我的、created_at）。" +
				"group_by 按一个字段分组计数（返回 [{字段: 值, count: n}]），适合画图和统计。各自填写（own）的表只能看到自己的行，作者和管理员看全部。可用的表：\n" + list,
			"inputSchema": map[string]any{"type": "object", "required": []string{"app", "table"}, "properties": with(map[string]any{
				"filter":   map[string]any{"type": "object", "description": "按字段等值筛选，例如 {\"session\": \"周六上午\"}"},
				"q":        str("在文字字段里找包含这段文字的行"),
				"sort":     str("排序字段（也可以是 created_at / updated_at）"),
				"order":    map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
				"limit":    integer("最多返回几行，默认 50，最多 200"),
				"offset":   integer("跳过前几行，翻页用"),
				"mine":     map[string]any{"type": "boolean", "description": "只看我填的"},
				"group_by": str("按这个字段分组计数"),
			})},
			"annotations": map[string]any{"readOnlyHint": true},
		},
		{
			"name": appConn + "__write",
			"description": "【应用数据】写应用自带的数据表。insert 新增（rows 一次最多 100 行，user 类型的字段自动填成当前员工）；update 改一行（id + row，只传要改的字段）；" +
				"delete 删除（ids），删除要先拿预览给用户确认，再带 confirm 调一次。新增和修改会直接生效并记录审计，写之前先跟用户确认内容。可用的表见 app__rows 的说明。",
			"inputSchema": map[string]any{"type": "object", "required": []string{"action", "app", "table"}, "properties": with(map[string]any{
				"action":  map[string]any{"type": "string", "enum": []string{"insert", "update", "delete"}},
				"rows":    map[string]any{"type": "array", "items": map[string]any{"type": "object"}, "description": "insert：要新增的行，字段名见表定义"},
				"id":      integer("update：要改的行"),
				"row":     map[string]any{"type": "object", "description": "update：要改的字段和新值"},
				"ids":     map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "delete：要删的行"},
				"confirm": str("delete 的第二步：上一步返回的确认码"),
			})},
			"annotations": map[string]any{"readOnlyHint": false, "destructiveHint": true},
			"_meta":       map[string]any{"dshee": map[string]any{"actions": map[string]any{"delete": map[string]any{"e": "danger", "zh": "删除应用里的数据", "en": "Delete app data"}}}},
		},
	}
}

type appToolArgs struct {
	App     string           `json:"app"`
	Table   string           `json:"table"`
	Action  string           `json:"action"`
	Rows    []map[string]any `json:"rows"`
	ID      int64            `json:"id"`
	Row     map[string]any   `json:"row"`
	IDs     []int64          `json:"ids"`
	Confirm string           `json:"confirm"`
	appListReq
}

// appCall 执行一次 app__ 工具调用。
func (s *Server) appCall(r *http.Request, v viewer, p *Principal, name string, raw json.RawMessage) json.RawMessage {
	ctx := r.Context()
	var args appToolArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return mcp.TextResult("参数不对："+err.Error(), true)
		}
	}
	var t *appTarget
	fail := func(err error) json.RawMessage {
		var ae *appDataError
		if errors.As(err, &ae) {
			var assetID int64
			if t != nil && t.asset != nil {
				assetID = t.asset.ID
			}
			if rerr := s.store.RecordUnanswered(context.WithoutCancel(ctx), assetID, "", "tool_error", ae.msg); rerr != nil {
				s.log.Warn("记录工具报错失败", "err", rerr)
			}
			return mcp.TextResult(ae.msg, true)
		}
		s.log.Error("应用数据工具出错", "tool", name, "err", err)
		return mcp.TextResult("服务器内部错误", true)
	}
	t, terr := s.appTargetFor(ctx, v, strings.TrimSpace(args.App), strings.TrimSpace(args.Table))
	if terr != nil {
		return fail(terr)
	}
	switch name {
	case appConn + "__rows":
		out, err := s.appList(ctx, v, t, args.appListReq)
		if err != nil {
			return fail(err)
		}
		return jsonText(out, false)
	case appConn + "__write":
	default:
		return mcp.TextResult("没有这个工具："+name, true)
	}
	switch args.Action {
	case "insert":
		out, err := s.appInsert(ctx, r, v, t, args.Rows)
		if err != nil {
			return fail(err)
		}
		return jsonText(map[string]any{"inserted": len(out), "items": out}, false)
	case "update":
		if args.ID == 0 || len(args.Row) == 0 {
			return mcp.TextResult("update 要给 id 和 row", true)
		}
		out, err := s.appUpdate(ctx, r, v, t, args.ID, args.Row)
		if err != nil {
			return fail(err)
		}
		return jsonText(map[string]any{"updated": out}, false)
	case "delete":
		rows, err := s.appDeletable(ctx, v, t, args.IDs)
		if err != nil {
			return fail(err)
		}
		hashArgs := map[string]any{"app": t.asset.Slug, "table": t.table.Name, "ids": args.IDs}
		hash := argsHash(name, hashArgs)
		if args.Confirm == "" {
			nonce := secret.Token()
			if err := s.store.CreateAgentConfirm(ctx, v.user.ID, p.TokenID, name, "delete", hash, secret.Hash(nonce), agentConfirmTTL); err != nil {
				return fail(err)
			}
			views := s.rowViews(ctx, v, t, rows)
			var lines []string
			for _, m := range views {
				var parts []string
				for _, f := range t.table.Fields {
					if !f.Hidden && m[f.Name] != nil && m[f.Name] != "" {
						parts = append(parts, f.Title+"="+fmt.Sprint(m[f.Name]))
					}
					if len(parts) >= 3 {
						break
					}
				}
				lines = append(lines, fmt.Sprintf("#%v %s", m["id"], strings.Join(parts, "，")))
			}
			preview := fmt.Sprintf("将从应用「%s」的表「%s」删除 %d 行（30 天内可找管理员恢复）：\n%s", t.spec.Title, t.table.Title, len(rows), strings.Join(lines, "\n"))
			return jsonText(map[string]any{
				"confirm_required": true, "confirm": nonce, "preview": preview, "expires_in": int(agentConfirmTTL.Seconds()),
				"next": "把 preview 原样告诉用户，得到同意后，用相同参数再调用一次并带上 confirm。",
			}, false)
		}
		ok, err := s.store.ConsumeAgentConfirm(ctx, v.user.ID, p.TokenID, name, "delete", hash, secret.Hash(args.Confirm))
		if err != nil {
			return fail(err)
		}
		if !ok {
			return mcp.TextResult("确认码无效、已经用过、已过期，或与这次的参数不一致。请不带 confirm 重新调用，拿到新的预览。", true)
		}
		n, err := s.appDelete(ctx, r, v, t, rows)
		if err != nil {
			return fail(err)
		}
		return jsonText(map[string]any{"deleted": n}, false)
	}
	return mcp.TextResult("action 只能是 insert / update / delete", true)
}

// ---- 工作台 ----

// appWorkbenchForDSH 把工作台里的 {"table": …} 数据源换成 app__rows 调用，接入插件按普通只读工具取数。
func appWorkbenchForDSH(slug string, wb *appWorkbench, u *store.User) *appWorkbench {
	if wb == nil {
		return nil
	}
	b, _ := json.Marshal(wb)
	var out appWorkbench
	_ = json.Unmarshal(b, &out)
	for pi := range out.Panes {
		for wi := range out.Panes[pi].Widgets {
			src := out.Panes[pi].Widgets[wi].Source
			if src != nil && u != nil {
				src.Args = fillMe(src.Args, u)
			}
			if src != nil && src.Dataset != "" {
				params := map[string]any{}
				_ = json.Unmarshal(src.Args, &params)
				raw, _ := json.Marshal(map[string]any{"name": src.Dataset, "params": params})
				out.Panes[pi].Widgets[wi].Source = &wbSource{Connection: dataConn, Tool: "dataset", Args: raw, Dataset: src.Dataset}
				continue
			}
			if src == nil || src.Table == "" {
				continue
			}
			args := map[string]any{}
			_ = json.Unmarshal(src.Args, &args)
			args["app"], args["table"] = slug, src.Table
			raw, _ := json.Marshal(args)
			out.Panes[pi].Widgets[wi].Source = &wbSource{Connection: appConn, Tool: "rows", Args: raw, Table: src.Table}
		}
	}
	return &out
}

// checkAppTablesChange 对照线上版本检查新版本的表定义（提交新版本时）。
func (s *Server) checkAppTablesChange(ctx context.Context, a *store.Asset, next *assetSpec) (string, error) {
	live, err := s.store.LiveAssetVersions(ctx, []int64{a.ID})
	if err != nil {
		return "", err
	}
	lv := live[a.ID]
	if lv == nil {
		return "", nil
	}
	old := parseSpec(lv.Spec).Tables
	if len(old) == 0 {
		return "", nil
	}
	counts, err := s.store.AppTableCounts(ctx, a.ID)
	if err != nil {
		return "", err
	}
	if msg := checkTableChange(old, next.Tables, counts); msg != "" {
		return "数据表：" + msg, nil
	}
	return "", nil
}

// appTablesContent 是 /名字 指令里「数据表」一节：让模型知道这个应用有哪些表、怎么读写。
func appTablesContent(slug string, tables []appTable) string {
	if len(tables) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## 数据表\n\n这个应用自带数据表，用 app__rows 查（统计用 group_by），用 app__write 写（app 填 " + slug + "）。写之前先把要写的内容告诉用户并得到同意；删除要先拿预览再确认。\n")
	for _, t := range tables {
		vis := (&appTarget{table: &t}).visibilityText()
		fmt.Fprintf(&b, "\n- %s「%s」（%s）：", t.Name, t.Title, vis)
		var fields []string
		for _, f := range t.Fields {
			if f.Hidden {
				continue
			}
			desc := f.Name + " " + f.Title + "（" + f.Kind
			if f.Kind == "select" {
				desc += "：" + strings.Join(f.Options, "/")
			}
			if f.Required {
				desc += "，必填"
			}
			if f.Kind == "user" {
				desc += "，自动填写"
			}
			fields = append(fields, desc+"）")
		}
		b.WriteString(strings.Join(fields, "、"))
	}
	return b.String()
}
