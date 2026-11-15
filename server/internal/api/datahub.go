package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/datahub"
	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 数据中台（第一版 M15，docs/product-v1.md §10.5）：企业共用的「唯一数据源」，范围收得很小——
// 管理员上传 CSV / Excel 生成表，再把表写成带名字的只读查询（数据集）；工作台和 AI 只能用数据集。
// 放在「企业连接」那一页的「数据中台」页签，管理权限沿用「管理企业连接」。
// 在 DSH 里是系统连接 data：data__dataset；工作台数据源写 {"dataset": 标识}。

const (
	dataConn = "data"
	// 只读账号密码存在设置里（加密），多个实例共用
	dataReaderSetting = "datahub_reader"
	dataUploadMax     = 20 << 20
)

// dataAutoParams 是系统按当前员工自动填的参数，数据集里直接写 :me_department 等。
var dataAutoParams = map[string]string{
	"me_department": "当前员工的部门",
	"me_username":   "当前员工的用户名",
	"me_name":       "当前员工的姓名（显示名）",
}

// OpenDataHub 打开数据中台（main 与测试共用）：只读账号的密码第一次随机生成、加密存进设置。
func OpenDataHub(ctx context.Context, st *store.Store, box *secret.Box, dsn, readDSN string) (*datahub.Hub, error) {
	return datahub.Open(ctx, dsn, readDSN, func(ctx context.Context) (string, error) {
		var stored struct {
			Sealed string `json:"sealed"`
		}
		if ok, err := st.GetSettingJSON(ctx, dataReaderSetting, &stored); err != nil {
			return "", err
		} else if ok && stored.Sealed != "" {
			raw, err := base64.StdEncoding.DecodeString(stored.Sealed)
			if err == nil {
				if pw, err := box.Open(raw); err == nil {
					return string(pw), nil
				}
			}
		}
		pw := strings.NewReplacer("-", "", "_", "").Replace(secret.Token())
		sealed, err := box.Seal([]byte(pw))
		if err != nil {
			return "", err
		}
		stored.Sealed = base64.StdEncoding.EncodeToString(sealed)
		return pw, st.PutSettingJSON(ctx, dataReaderSetting, stored, 0)
	})
}

func (s *Server) dataReady() (string, bool) {
	if s.opts.DataHub == nil {
		if s.opts.DataHubErr != "" {
			return s.opts.DataHubErr, false
		}
		return "没有开启数据中台（DSH_EE_DATA_DSN 设成了 off）", false
	}
	return "", true
}

func (s *Server) requireDataHub(w http.ResponseWriter) bool {
	if msg, ok := s.dataReady(); !ok {
		writeErr(w, http.StatusServiceUnavailable, msg)
		return false
	}
	return true
}

// ---- 管理：表 ----

// GET /api/admin/data：数据中台的状态、表和数据集。
func (s *Server) handleDataOverview(w http.ResponseWriter, r *http.Request) {
	msg, ok := s.dataReady()
	out := map[string]any{"enabled": ok, "error": msg, "tables": []any{}, "datasets": []any{}, "auto_params": dataAutoParams}
	if ok {
		h := s.opts.DataHub
		out["database"], out["read_error"] = h.DBName, h.ReadErr
		tables, err := s.store.ListDataTables(r.Context())
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		sets, err := s.store.ListDataSets(r.Context())
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		out["tables"], out["datasets"] = tables, sets
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/admin/data/tables（multipart：file、name、title、description、mode = create / replace / append）
func (s *Server) handleDataUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataHub(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, dataUploadMax+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("上传失败：文件最大 %d MB", dataUploadMax>>20))
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	title := strings.TrimSpace(r.FormValue("title"))
	desc := strings.TrimSpace(r.FormValue("description"))
	mode := r.FormValue("mode")
	if !datahub.TableNameRe.MatchString(name) || strings.HasSuffix(name, "__new") {
		writeErr(w, http.StatusBadRequest, "表名只能用小写字母、数字和下划线，字母开头，最多 40 个字符，例如 customers")
		return
	}
	if !slices.Contains([]string{"create", "replace", "append"}, mode) {
		writeErr(w, http.StatusBadRequest, "mode 只能是 create / replace / append")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请选择要上传的 CSV 或 .xlsx 文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, dataUploadMax+1))
	if err != nil || len(data) > dataUploadMax {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("文件最大 %d MB", dataUploadMax>>20))
		return
	}
	var sheet *datahub.Sheet
	switch strings.ToLower(filepath.Ext(hdr.Filename)) {
	case ".csv", ".txt":
		sheet, err = datahub.ParseCSV(data)
	case ".xlsx":
		sheet, err = datahub.ParseXLSX(data)
	default:
		err = errors.New("只支持 .csv 和 .xlsx（老的 .xls 请先另存为 .xlsx）")
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	existing, err := s.store.GetDataTable(ctx, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalErr(w, r, err)
		return
	}
	switch {
	case mode == "create" && existing != nil:
		writeErr(w, http.StatusConflict, "已经有表 "+name+"；要覆盖请选「替换」，要加数据请选「追加」")
		return
	case mode != "create" && existing == nil:
		writeErr(w, http.StatusNotFound, "没有表 "+name)
		return
	}
	h := s.opts.DataHub
	t := &store.DataTable{Name: name, Title: title, Description: desc}
	if existing != nil {
		if title == "" {
			t.Title = existing.Title
		}
		if desc == "" {
			t.Description = existing.Description
		}
	}
	if t.Title == "" || !limitRunes(t.Title, 64) {
		writeErr(w, http.StatusBadRequest, "标题必填，最多 64 个字")
		return
	}
	if mode == "append" {
		cols := make([]datahub.Column, len(existing.Columns))
		for i, c := range existing.Columns {
			cols[i] = datahub.Column{Name: c.Name, Type: c.Type}
		}
		rows, err := datahub.Align(cols, sheet)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.AppendRows(ctx, name, cols, rows); err != nil {
			writeErr(w, http.StatusBadRequest, "写入失败："+err.Error())
			return
		}
		t.Columns = existing.Columns
	} else {
		cols, rows := datahub.Infer(sheet)
		if err := h.ReplaceTable(ctx, name, cols, rows); err != nil {
			writeErr(w, http.StatusBadRequest, "建表失败："+err.Error())
			return
		}
		for _, c := range cols {
			t.Columns = append(t.Columns, store.DataColumn{Name: c.Name, Type: c.Type})
		}
	}
	if t.RowCount, err = h.CountRows(ctx, name); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.SaveDataTable(ctx, t, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditDataUpload, name, map[string]any{"mode": mode, "rows": len(sheet.Rows), "file": hdr.Filename})
	writeJSON(w, http.StatusOK, map[string]any{"table": t, "added": len(sheet.Rows)})
}

// GET /api/admin/data/tables/{name}：表的前 50 行。
func (s *Server) handleDataTablePreview(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataHub(w) {
		return
	}
	t, err := s.store.GetDataTable(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "没有这张表")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	res, err := s.opts.DataHub.Preview(r.Context(), t.Name, 50)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "读表失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"table": t, "preview": res})
}

// DELETE /api/admin/data/tables/{name}
func (s *Server) handleDataTableDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataHub(w) {
		return
	}
	name := r.PathValue("name")
	if _, err := s.store.GetDataTable(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "没有这张表")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if err := s.opts.DataHub.DropTable(r.Context(), name); err != nil {
		writeErr(w, http.StatusBadGateway, "删表失败："+err.Error())
		return
	}
	if err := s.store.DeleteDataTable(r.Context(), name); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditDataDrop, name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 管理：数据集 ----

type dataSetReq struct {
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	SQL         string            `json:"sql"`
	Params      []store.DataParam `json:"params"`
	Scope       *store.Scope      `json:"scope"`
	Values      map[string]any    `json:"values"`
	// 覆盖已有数据集时要带上打开页面时读到的 updated_at：范围与 SQL 都是整段覆盖的。
	UpdatedAt *time.Time `json:"updated_at"`
}

// cleanDataSet 校验数据集，并按 SQL 里出现的参数整理参数表（自动参数标上 auto，其余保留填写的标题和默认值）。
func cleanDataSet(req *dataSetReq) (*store.DataSet, string) {
	d := &store.DataSet{Slug: strings.TrimSpace(req.Slug), Title: strings.TrimSpace(req.Title), Description: strings.TrimSpace(req.Description), Scope: store.Scope{All: true}}
	if !slugRe.MatchString(d.Slug) || len(d.Slug) > maxSlug {
		return nil, "标识只能用小写字母、数字和连字符，例如 customers，最多 48 个字符"
	}
	if d.Title == "" || !limitRunes(d.Title, 64) {
		return nil, "标题必填，最多 64 个字"
	}
	if !limitRunes(d.Description, 500) {
		return nil, "说明最多 500 个字"
	}
	q, err := datahub.CheckSelect(req.SQL)
	if err != nil {
		return nil, err.Error()
	}
	if len(q) > 20000 {
		return nil, "SQL 最多 20000 个字符"
	}
	d.SQL = q
	names, err := datahub.Params(q)
	if err != nil {
		return nil, err.Error()
	}
	given := map[string]store.DataParam{}
	for _, p := range req.Params {
		given[p.Name] = p
	}
	d.Params = []store.DataParam{}
	for _, n := range names {
		if desc, ok := dataAutoParams[n]; ok {
			d.Params = append(d.Params, store.DataParam{Name: n, Title: desc, Auto: true})
			continue
		}
		p := given[n]
		p.Name, p.Auto = n, false
		p.Title, p.Default = strings.TrimSpace(p.Title), strings.TrimSpace(p.Default)
		if p.Title == "" {
			p.Title = n
		}
		if !limitRunes(p.Title, 40) || !limitRunes(p.Default, 200) {
			return nil, "参数 " + n + " 的标题最多 40 个字、默认值最多 200 个字"
		}
		d.Params = append(d.Params, p)
	}
	if req.Scope != nil {
		d.Scope = req.Scope.Normalize()
	}
	return d, ""
}

// runDataSet 以员工的身份执行数据集：自动参数按员工填，其余参数用传进来的值或默认值。
func (s *Server) runDataSet(ctx context.Context, v viewer, d *store.DataSet, values map[string]any) (*datahub.Result, error) {
	q, order, err := datahub.Bind(d.SQL)
	if err != nil {
		return nil, appErr(http.StatusBadRequest, "%s", err.Error())
	}
	defaults := map[string]string{}
	for _, p := range d.Params {
		defaults[p.Name] = p.Default
	}
	args := make([]any, 0, len(order))
	for _, n := range order {
		switch n {
		case "me_department":
			args = append(args, v.user.Department)
			continue
		case "me_username":
			args = append(args, v.user.Username)
			continue
		case "me_name":
			args = append(args, displayOf(v.user))
			continue
		}
		val, ok := values[n]
		if !ok || val == nil || val == "" {
			if defaults[n] == "" {
				return nil, appErr(http.StatusBadRequest, "数据集「%s」要填参数 %s", d.Title, n)
			}
			val = defaults[n]
		}
		switch x := val.(type) {
		case string, float64, bool:
			args = append(args, x)
		default:
			return nil, appErr(http.StatusBadRequest, "参数 %s 只能是文字或数字", n)
		}
	}
	res, err := s.opts.DataHub.Query(ctx, q, args)
	if err != nil {
		return nil, appErr(http.StatusBadGateway, "查询失败：%s", err.Error())
	}
	return res, nil
}

// POST /api/admin/data/run {sql, values}：试运行（以当前管理员的身份填自动参数）。
func (s *Server) handleDataRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataHub(w) {
		return
	}
	var req dataSetReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Slug, req.Title = "preview", "试运行"
	d, msg := cleanDataSet(&req)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	res, err := s.runDataSet(r.Context(), viewerOf(r), d, req.Values)
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res, "params": d.Params})
}

// POST /api/admin/data/datasets 与 PUT /api/admin/data/datasets/{slug}
func (s *Server) handleDataSetSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataHub(w) {
		return
	}
	var req dataSetReq
	if !readJSON(w, r, &req) {
		return
	}
	update := r.Method == http.MethodPut
	if update {
		req.Slug = r.PathValue("slug")
		if req.UpdatedAt == nil {
			writeErr(w, http.StatusBadRequest, "改数据集要带上 updated_at；请刷新页面后重试")
			return
		}
	}
	d, msg := cleanDataSet(&req)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	// 保存前按默认值试跑一次，SQL 写错了马上知道
	if _, err := s.runDataSet(r.Context(), viewerOf(r), d, nil); err != nil {
		var ae *appDataError
		if errors.As(err, &ae) && !strings.Contains(ae.msg, "要填参数") {
			writeErr(w, http.StatusBadRequest, ae.msg)
			return
		}
	}
	ctx := r.Context()
	by := currentUser(r).ID
	var err error
	action := auditDataSetCreate
	if update {
		err, action = s.store.UpdateDataSet(ctx, d, by, req.UpdatedAt), auditDataSetUpdate
	} else {
		err = s.store.CreateDataSet(ctx, d, by)
	}
	switch {
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "数据集已被其他人修改，请刷新后重试")
		return
	case errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, "已经有数据集 "+d.Slug)
		return
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "没有数据集 "+d.Slug)
		return
	case err != nil:
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, action, d.Slug, map[string]any{"title": d.Title})
	s.hub.notify()
	saved, err := s.store.GetDataSet(ctx, d.Slug)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	status := http.StatusCreated
	if update {
		status = http.StatusOK
	}
	writeJSON(w, status, saved)
}

// DELETE /api/admin/data/datasets/{slug}
func (s *Server) handleDataSetDelete(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if _, err := s.store.GetDataSet(r.Context(), slug); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "没有数据集 "+slug)
		return
	}
	if err := s.store.DeleteDataSet(r.Context(), slug); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditDataSetDelete, slug, nil)
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// ---- 员工：能用哪些数据集 ----

func (v viewer) seesDataSet(d *store.DataSet) bool {
	return v.inScope(d.Scope) || v.manages(store.KindConnection)
}

// visibleDataSets 是员工能用的数据集；数据中台没开或只读账号不可用时为空。
func (s *Server) visibleDataSets(ctx context.Context, v viewer) ([]*store.DataSet, error) {
	if _, ok := s.dataReady(); !ok || s.opts.DataHub.ReadErr != "" {
		return nil, nil
	}
	all, err := s.store.ListDataSets(ctx)
	if err != nil {
		return nil, err
	}
	var out []*store.DataSet
	for _, d := range all {
		if v.seesDataSet(d) {
			out = append(out, d)
		}
	}
	return out, nil
}

// GET /api/data/datasets：我能用的数据集（编辑器里提示数据源用）。
func (s *Server) handleMyDataSets(w http.ResponseWriter, r *http.Request) {
	sets, err := s.visibleDataSets(r.Context(), viewerOf(r))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	items := []map[string]any{}
	for _, d := range sets {
		items = append(items, map[string]any{"slug": d.Slug, "title": d.Title, "description": d.Description, "params": d.Params})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// findDataSet 按标识找员工能用的数据集。
func (s *Server) findDataSet(ctx context.Context, v viewer, slug string) (*store.DataSet, error) {
	if msg, ok := s.dataReady(); !ok {
		return nil, appErr(http.StatusServiceUnavailable, "%s", msg)
	}
	d, err := s.store.GetDataSet(ctx, slug)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !v.seesDataSet(d)) {
		return nil, appErr(http.StatusNotFound, "没有数据集 %s，或你没有权限用它", slug)
	}
	return d, err
}

// ---- DSH：/mcp 上的 data__dataset ----

func dataToolDefs(sets []*store.DataSet) []map[string]any {
	var lines []string
	for _, d := range sets {
		line := fmt.Sprintf("- %s「%s」", d.Slug, d.Title)
		if d.Description != "" {
			line += "：" + d.Description
		}
		var ps []string
		for _, p := range d.Params {
			if p.Auto {
				continue
			}
			x := p.Name + "（" + p.Title
			if p.Default != "" {
				x += "，默认 " + p.Default
			}
			ps = append(ps, x+"）")
		}
		if len(ps) > 0 {
			line += "。参数：" + strings.Join(ps, "、")
		}
		lines = append(lines, line)
	}
	return []map[string]any{{
		"name": dataConn + "__dataset",
		"description": "【数据中台】查企业共用的数据集（唯一数据源），返回 columns、items（最多 1000 行）。只读。可用的数据集：\n" + strings.Join(lines, "\n") +
			"\n客户、人员这类主数据以这里为准；用数据回答时说明来自哪个数据集。",
		"inputSchema": map[string]any{"type": "object", "required": []string{"name"}, "properties": map[string]any{
			"name":   map[string]any{"type": "string", "description": "数据集标识"},
			"params": map[string]any{"type": "object", "description": "数据集的参数，例如 {\"since\": \"2026-01-01\"}"},
		}},
		"annotations": map[string]any{"readOnlyHint": true},
	}}
}

func (s *Server) dataCall(ctx context.Context, v viewer, name string, raw json.RawMessage) json.RawMessage {
	if name != dataConn+"__dataset" {
		return mcp.TextResult("没有这个工具："+name, true)
	}
	var args struct {
		Name   string         `json:"name"`
		Params map[string]any `json:"params"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return mcp.TextResult("参数不对："+err.Error(), true)
		}
	}
	d, err := s.findDataSet(ctx, v, strings.TrimSpace(args.Name))
	if err == nil {
		var res *datahub.Result
		if res, err = s.runDataSet(ctx, v, d, args.Params); err == nil {
			return jsonText(map[string]any{"dataset": d.Slug, "title": d.Title, "columns": res.Columns, "items": res.Items, "total": res.Total, "truncated": res.Truncated}, false)
		}
	}
	var ae *appDataError
	if errors.As(err, &ae) {
		return mcp.TextResult(ae.msg, true)
	}
	s.log.Error("数据集查询出错", "err", err)
	return mcp.TextResult("服务器内部错误", true)
}
