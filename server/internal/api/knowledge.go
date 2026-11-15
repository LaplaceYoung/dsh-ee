package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/kb"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 知识库的网页接口：建库、权限、上传与编辑文档、试搜、检索记录。
// 员工在 DSH 里的检索走 /mcp 的 kb__ 工具，见 knowledge_tools.go；设计见 docs/knowledge.md。

const (
	maxKBFile     = 50 << 20
	maxKBUpload   = 200 << 20
	maxKBMarkdown = 8 << 20
)

const (
	auditKBCreate    = "kb.create"
	auditKBEdit      = "kb.edit"
	auditKBDelete    = "kb.delete"
	auditKBDocCreate = "kb.doc_create"
	auditKBDocEdit   = "kb.doc_edit"
	auditKBDocDelete = "kb.doc_delete"
)

// kbIndex 是知识库的内存索引；某个库的修订号变了（本进程或别的副本改了文档），下次检索前整库重建。
type kbIndex struct {
	ix *kb.Index
	mu sync.Mutex
}

func (v viewer) kbManages() bool { return v.access.Has(rbac.KnowledgeManage) }

// kbReads：在可用范围内、是维护者、或能管理知识库。
func (v viewer) kbReads(b *store.KBase) bool {
	return v.kbManages() || v.inScope(b.Scope) || b.Maintainers.Allows(v.user.ID, v.access.RoleIDs)
}

func (v viewer) kbMaintains(b *store.KBase) bool {
	return v.kbManages() || b.Maintainers.Allows(v.user.ID, v.access.RoleIDs)
}

// readableKBases 是这个人能查的全部知识库。
func (s *Server) readableKBases(ctx context.Context, v viewer) ([]*store.KBase, error) {
	all, err := s.store.ListKBases(ctx)
	if err != nil {
		return nil, err
	}
	out := []*store.KBase{}
	for _, b := range all {
		if v.kbReads(b) {
			out = append(out, b)
		}
	}
	return out, nil
}

// kbSync 让索引里这些库与数据库一致。
func (s *Server) kbSync(ctx context.Context, bases []*store.KBase) error {
	s.kb.mu.Lock()
	defer s.kb.mu.Unlock()
	for _, b := range bases {
		if s.kb.ix.Revision(b.ID) == b.Revision {
			continue
		}
		docs, texts, err := s.store.KBaseTexts(ctx, b.ID)
		if err != nil {
			return err
		}
		list := make([]*kb.Doc, 0, len(docs))
		for _, d := range docs {
			list = append(list, &kb.Doc{ID: d.ID, Base: b.ID, Title: d.Title, Folder: d.Folder, Version: d.Version, Text: texts[d.ID]})
		}
		s.kb.ix.Load(b.ID, b.Revision, list)
	}
	return nil
}

func validKBText(title, desc string) string {
	if title == "" || !limitRunes(title, 64) {
		return "名称必填，最多 64 个字"
	}
	if !limitRunes(desc, 1000) {
		return "说明最多 1000 个字"
	}
	return ""
}

// cleanFolder 规范化目录：用 / 分隔，去掉空段和首尾斜杠。
func cleanFolder(f string) (string, string) {
	f = strings.ReplaceAll(strings.TrimSpace(f), "\\", "/")
	var parts []string
	for _, p := range strings.Split(f, "/") {
		if p = strings.TrimSpace(p); p != "" {
			if p == "." || p == ".." {
				return "", "目录名不能是 . 或 .."
			}
			parts = append(parts, p)
		}
	}
	out := strings.Join(parts, "/")
	if !limitRunes(out, 200) {
		return "", "目录最多 200 个字"
	}
	return out, ""
}

type kbItem struct {
	*store.KBase
	CanEdit   bool `json:"can_edit"`
	CanManage bool `json:"can_manage"`
	// 在可用范围内（能在 DSH 里查）
	Readable bool `json:"readable"`
	// 列表里才有：最近更新的几篇；维护者才有：待补文档有几条
	Recent []*store.KDocBrief `json:"recent,omitempty"`
	Gaps   *int               `json:"gaps,omitempty"`
}

func kbItemOf(v viewer, b *store.KBase) kbItem {
	return kbItem{KBase: b, CanEdit: v.kbMaintains(b), CanManage: v.kbManages(), Readable: v.kbReads(b)}
}

// GET /api/kb：我能看到的知识库；能管理的人另外拿到「知识问答」的设置。
func (s *Server) handleKBList(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	bases, err := s.readableKBases(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	items := make([]kbItem, len(bases))
	var maintained []int64
	for i, b := range bases {
		items[i] = kbItemOf(v, b)
		if items[i].CanEdit {
			maintained = append(maintained, b.ID)
		}
		// 最近更新的几篇：员工最关心的是「什么制度改了」
		if items[i].Readable {
			if items[i].Recent, err = s.store.RecentKDocs(r.Context(), b.ID, 20); err == nil {
				items[i].Recent, err = filterDocs(r.Context(), s, v, b, items[i].Recent, func(d *store.KDocBrief) int64 { return d.ID })
				items[i].Recent = items[i].Recent[:min(3, len(items[i].Recent))]
			}
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
		}
	}
	// 维护的库：待补文档有几条
	if len(maintained) > 0 {
		gaps, err := s.kbGaps(r.Context(), maintained)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		for i := range items {
			if items[i].CanEdit {
				n := len(gaps[items[i].ID])
				items[i].Gaps = &n
			}
		}
	}
	resp := map[string]any{"items": items, "can_manage": v.kbManages(), "formats": kb.Formats}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	resp["qa"] = set.Knowledge
	writeJSON(w, http.StatusOK, resp)
}

type kbCreateReq struct {
	Slug        string             `json:"slug"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Scope       *store.Scope       `json:"scope"`
	Maintainers *store.Maintainers `json:"maintainers"`
}

// POST /api/kb
func (s *Server) handleKBCreate(w http.ResponseWriter, r *http.Request) {
	var req kbCreateReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Slug, req.Title, req.Description = strings.TrimSpace(req.Slug), strings.TrimSpace(req.Title), strings.TrimSpace(req.Description)
	if !slugRe.MatchString(req.Slug) || len(req.Slug) > maxSlug {
		writeErr(w, http.StatusBadRequest, "标识只能用小写字母、数字和连字符，例如 hr-policy，最多 48 个字符")
		return
	}
	if msg := validKBText(req.Title, req.Description); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	b := &store.KBase{Slug: req.Slug, Title: req.Title, Description: req.Description, Scope: store.Scope{All: true}}
	if req.Scope != nil {
		b.Scope = req.Scope.Normalize()
	}
	if req.Maintainers != nil {
		b.Maintainers = req.Maintainers.Normalize()
	}
	if err := s.store.CreateKBase(r.Context(), b, currentUser(r).ID); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "标识 "+req.Slug+" 已被占用")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBCreate, "kb:"+b.Slug, map[string]any{"id": b.ID, "scope": b.Scope})
	s.hub.notify()
	writeJSON(w, http.StatusCreated, kbItemOf(viewerOf(r), b))
}

func (s *Server) loadKBase(w http.ResponseWriter, r *http.Request) (*store.KBase, viewer, bool) {
	v := viewerOf(r)
	id, ok := pathID(w, r)
	if !ok {
		return nil, v, false
	}
	b, err := s.store.GetKBase(r.Context(), id)
	if notFound(err) || (err == nil && !v.kbReads(b)) {
		writeErr(w, http.StatusNotFound, "知识库不存在，或你没有查看权限")
		return nil, v, false
	}
	if err != nil {
		s.internalErr(w, r, err)
		return nil, v, false
	}
	return b, v, true
}

// GET /api/kb/{id}：库的信息与文档列表。
func (s *Server) handleKBDetail(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return
	}
	docs, err := s.store.ListKDocs(r.Context(), b.ID)
	if err == nil {
		docs, err = filterDocs(r.Context(), s, v, b, docs, func(d *store.KDoc) int64 { return d.ID })
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	users, err := s.userIndex(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type docItem struct {
		*store.KDoc
		UpdatedByName string `json:"updated_by_name"`
	}
	out := make([]docItem, len(docs))
	for i, d := range docs {
		out[i] = docItem{KDoc: d, UpdatedByName: users.name(d.UpdatedBy)}
	}
	used, err := s.appsUsing(r.Context(), v, store.RequestKnowledge, b.Slug)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	item := kbItemOf(v, b)
	if item.CanEdit {
		gaps, err := s.kbGaps(r.Context(), []int64{b.ID})
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		n := len(gaps[b.ID])
		item.Gaps = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{"kb": item, "docs": out, "used_by": used})
}

type kbPatchReq struct {
	Title       *string            `json:"title"`
	Description *string            `json:"description"`
	Scope       *store.Scope       `json:"scope"`
	Maintainers *store.Maintainers `json:"maintainers"`
	// 改范围或维护者时必须带上打开页面时读到的 updated_at：两者都是整段覆盖的，
	// 用旧快照保存会悄悄丢掉这期间别人新加的授权。
	UpdatedAt *time.Time `json:"updated_at"`
}

// PATCH /api/kb/{id}：维护者改名称与说明；范围与维护者要「管理知识库」。
func (s *Server) handleKBPatch(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return
	}
	var req kbPatchReq
	if !readJSON(w, r, &req) {
		return
	}
	if !v.kbMaintains(b) {
		writeErr(w, http.StatusForbidden, "只有维护者或知识库管理员能修改")
		return
	}
	if (req.Scope != nil || req.Maintainers != nil) && !v.kbManages() {
		writeErr(w, http.StatusForbidden, "修改谁能查、谁来维护需要「管理知识库」权限")
		return
	}
	// 范围与维护者是整段覆盖的：必须带上读到的 updated_at，避免旧快照覆盖新授权
	if (req.Scope != nil || req.Maintainers != nil) && req.UpdatedAt == nil {
		writeErr(w, http.StatusBadRequest, "改范围或维护者要带上 updated_at；请刷新页面后重试")
		return
	}
	p := store.KBasePatch{Title: req.Title, Description: req.Description, ExpectedUpdatedAt: req.UpdatedAt}
	title, desc := b.Title, b.Description
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
		p.Title = &title
	}
	if req.Description != nil {
		desc = strings.TrimSpace(*req.Description)
		p.Description = &desc
	}
	if msg := validKBText(title, desc); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	detail := map[string]any{"id": b.ID}
	if req.Scope != nil {
		sc := req.Scope.Normalize()
		p.Scope, detail["scope"] = &sc, sc
	}
	if req.Maintainers != nil {
		m := req.Maintainers.Normalize()
		p.Maintainers, detail["maintainers"] = &m, m
	}
	if err := s.store.UpdateKBase(r.Context(), b.ID, p); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "范围或维护者已被其他人修改，请刷新后重试")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBEdit, "kb:"+b.Slug, detail)
	s.hub.notify()
	updated, err := s.store.GetKBase(r.Context(), b.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, kbItemOf(v, updated))
}

// DELETE /api/kb/{id}
func (s *Server) handleKBDelete(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return
	}
	if !v.kbManages() {
		writeErr(w, http.StatusForbidden, "删除知识库需要「管理知识库」权限")
		return
	}
	if err := s.store.DeleteKBase(r.Context(), b.ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.kb.ix.Drop(b.ID)
	s.audit(r, auditKBDelete, "kb:"+b.Slug, map[string]any{"id": b.ID, "docs": b.Docs})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// kdocConvert 是文档上记的转换体检：转换结果，加上维护者选的转换设置、正文是否在网页上改过。
type kdocConvert struct {
	kb.Info
	HeaderOption string `json:"header_option,omitempty"`
	Edited       bool   `json:"edited,omitempty"`
}

func convertRecord(info kb.Info, opt kb.Options) json.RawMessage {
	raw, _ := json.Marshal(kdocConvert{Info: info, HeaderOption: opt.Header})
	return raw
}

func headerOption(v string) (string, bool) {
	switch v {
	case "", "first", "none":
		return v, true
	}
	return "", false
}

// convertUpload 把上传的文件转成正文，原件存进 blob 存储（npm 源不可用时只存正文）。
func (s *Server) convertUpload(name string, data []byte, opt kb.Options) (*kb.Converted, *store.KDoc, error) {
	conv, err := kb.Convert(name, data, opt)
	if err != nil {
		return nil, nil, err
	}
	if len(conv.Markdown) > maxKBMarkdown {
		return nil, nil, errors.New("转换出来的正文超过 8 MB，请拆成几个文件")
	}
	origin := &store.KDoc{SourceName: truncateStr(path.Base(strings.ReplaceAll(name, "\\", "/")), 255), Mime: conv.Mime, Size: int64(len(data)),
		ConvertInfo: convertRecord(conv.Info, opt)}
	if s.registry != nil {
		if d, err := s.registry.Blobs().Put(bytes.NewReader(data), maxKBFile); err == nil {
			origin.BlobKey = d.Key
		} else {
			s.log.Warn("保存知识库原件失败", "file", name, "err", err)
		}
	}
	return conv, origin, nil
}

type kbUploadResult struct {
	File  string      `json:"file"`
	Doc   *store.KDoc `json:"doc,omitempty"`
	Error string      `json:"error,omitempty"`
	// 同目录下有同名文档，这次更新成了它的新版本
	Updated bool `json:"updated,omitempty"`
}

// sameDoc 找同一个目录下跟上传的文件同名的文档：原文件名一样，或者标题就是文件名（去掉扩展名）。
func sameDoc(docs []*store.KDoc, folder, file string) *store.KDoc {
	base := path.Base(strings.ReplaceAll(file, "\\", "/"))
	stem := strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base)))
	var byTitle *store.KDoc
	for _, d := range docs {
		if d.Folder != folder {
			continue
		}
		if d.SourceName == base {
			return d
		}
		if byTitle == nil && d.Title == stem {
			byTitle = d
		}
	}
	return byTitle
}

type kbUploadCheckReq struct {
	Folder string   `json:"folder"`
	Files  []string `json:"files"`
}

// POST /api/kb/{id}/docs/check：上传前看哪些文件在这个目录下已经有同名文档，网页上让维护者选「更新」还是「另存一篇」。
func (s *Server) handleKBUploadCheck(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return
	}
	if !v.kbMaintains(b) {
		writeErr(w, http.StatusForbidden, "只有维护者或知识库管理员能上传文档")
		return
	}
	var req kbUploadCheckReq
	if !readJSON(w, r, &req) {
		return
	}
	folder, msg := cleanFolder(req.Folder)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if len(req.Files) > 50 {
		writeErr(w, http.StatusBadRequest, "一次最多上传 50 个文件")
		return
	}
	docs, err := s.store.ListKDocs(r.Context(), b.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type item struct {
		File string      `json:"file"`
		Doc  *store.KDoc `json:"doc"`
	}
	items := []item{}
	for _, f := range req.Files {
		if d := sameDoc(docs, folder, f); d != nil {
			items = append(items, item{File: f, Doc: d})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /api/kb/{id}/docs：multipart，可以一次传多个文件（字段 file）；字段 folder 是放进哪个目录。
// 同目录下已有同名文档时默认更新成它的新版本；字段 new（可以有多个，值是文件名）里列出的文件另存为新文档。
// 字段 header 是 Excel / CSV 的表头设置（first / none，不填自动判断）。
// 不带文件、带 title 和 markdown 时直接新建一篇 Markdown 文档。
func (s *Server) handleKBUpload(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return
	}
	if !v.kbMaintains(b) {
		writeErr(w, http.StatusForbidden, "只有维护者或知识库管理员能上传文档")
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "multipart/form-data" {
		writeErr(w, http.StatusUnsupportedMediaType, "请用 multipart/form-data 上传")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxKBUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "上传内容太大，一次最多 200 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	folder, msg := cleanFolder(r.FormValue("folder"))
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	uid := currentUser(r).ID
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		// 浏览器提交表单时会把换行变成 \r\n，统一回 \n（行号、引用核对、编辑时的比较都按 \n）
		title, md := strings.TrimSpace(r.FormValue("title")), normalizeNewlines(r.FormValue("markdown"))
		if title == "" || !limitRunes(title, 200) || strings.TrimSpace(md) == "" {
			writeErr(w, http.StatusBadRequest, "要上传文件，或者填写标题和正文")
			return
		}
		if len(md) > maxKBMarkdown {
			writeErr(w, http.StatusBadRequest, "正文不能超过 8 MB")
			return
		}
		d := &store.KDoc{BaseID: b.ID, Title: title, Folder: folder, Mime: "text/markdown"}
		if err := s.store.CreateKDoc(r.Context(), d, md, "网页新建", uid); err != nil {
			s.internalErr(w, r, err)
			return
		}
		s.audit(r, auditKBDocCreate, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"title": d.Title})
		writeJSON(w, http.StatusCreated, map[string]any{"results": []kbUploadResult{{File: title, Doc: d}}})
		return
	}
	if len(files) > 50 {
		writeErr(w, http.StatusBadRequest, "一次最多上传 50 个文件")
		return
	}
	header, okHeader := headerOption(r.FormValue("header"))
	if !okHeader {
		writeErr(w, http.StatusBadRequest, "header 只能是 first 或 none")
		return
	}
	opt := kb.Options{Header: header}
	asNew := r.MultipartForm.Value["new"]
	existing, err := s.store.ListKDocs(r.Context(), b.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	results := []kbUploadResult{}
	created := 0
	for _, fh := range files {
		res := kbUploadResult{File: fh.Filename}
		if fh.Size > maxKBFile {
			res.Error = "文件超过 50 MB"
			results = append(results, res)
			continue
		}
		f, err := fh.Open()
		if err != nil {
			res.Error = "读取文件失败"
			results = append(results, res)
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, maxKBFile+1))
		f.Close()
		if err != nil {
			res.Error = "读取文件失败"
			results = append(results, res)
			continue
		}
		conv, origin, err := s.convertUpload(fh.Filename, data, opt)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		if same := sameDoc(existing, folder, fh.Filename); same != nil && !slices.Contains(asNew, fh.Filename) {
			// 同名文档：出一个新版本，旧版本和旧引用都还在
			updated, err := s.store.UpdateKDoc(r.Context(), same.ID, store.KDocPatch{Markdown: &conv.Markdown, Note: "上传新版本 " + origin.SourceName, Origin: origin, ConvertInfo: origin.ConvertInfo}, uid)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			created++
			res.Doc, res.Updated = updated, true
			results = append(results, res)
			s.audit(r, auditKBDocEdit, "kb:"+b.Slug+"/"+strconv.FormatInt(updated.ID, 10), map[string]any{"file": origin.SourceName, "version": updated.Version})
			continue
		}
		d := &store.KDoc{BaseID: b.ID, Title: truncateStr(conv.Title, 200), Folder: folder, SourceName: origin.SourceName, Mime: origin.Mime, BlobKey: origin.BlobKey, Size: origin.Size, ConvertInfo: origin.ConvertInfo}
		if err := s.store.CreateKDoc(r.Context(), d, conv.Markdown, "上传 "+origin.SourceName, uid); err != nil {
			s.internalErr(w, r, err)
			return
		}
		created++
		res.Doc = d
		results = append(results, res)
		existing = append(existing, d)
		s.audit(r, auditKBDocCreate, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"title": d.Title, "file": origin.SourceName})
	}
	status := http.StatusCreated
	if created == 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{"results": results})
}

// loadKDoc 取文档与它所在的库，并检查查看权限。
func (s *Server) loadKDoc(w http.ResponseWriter, r *http.Request) (*store.KDoc, *store.KBase, viewer, bool) {
	v := viewerOf(r)
	id, ok := pathID(w, r)
	if !ok {
		return nil, nil, v, false
	}
	d, err := s.store.GetKDoc(r.Context(), id)
	var b *store.KBase
	if err == nil {
		b, err = s.store.GetKBase(r.Context(), d.BaseID)
	}
	if err == nil && v.kbReads(b) {
		// 按字段规则看不到的文档当不存在
		if seen, gerr := s.canSeeDoc(r.Context(), v, b, d.ID); gerr != nil {
			err = gerr
		} else if !seen {
			err = store.ErrNotFound
		}
	}
	if notFound(err) || (err == nil && !v.kbReads(b)) {
		writeErr(w, http.StatusNotFound, "文档不存在，或你没有查看权限")
		return nil, nil, v, false
	}
	if err != nil {
		s.internalErr(w, r, err)
		return nil, nil, v, false
	}
	return d, b, v, true
}

// GET /api/kb-docs/{id}：正文（?version= 取旧版本）、大纲与版本列表。
func (s *Server) handleKDocDetail(w http.ResponseWriter, r *http.Request) {
	d, b, v, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	version, _ := strconv.Atoi(r.URL.Query().Get("version"))
	text, err := s.store.KDocText(r.Context(), d.ID, version)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "没有这个版本")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	versions, err := s.store.KDocVersions(r.Context(), d.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	users, err := s.userIndex(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type versionItem struct {
		*store.KDocText
		CreatedByName string `json:"created_by_name"`
	}
	vs := make([]versionItem, len(versions))
	for i, x := range versions {
		vs[i] = versionItem{KDocText: x, CreatedByName: users.name(x.CreatedBy)}
	}
	resp := map[string]any{
		"doc": d, "kb": kbItemOf(v, b), "version": text.Version, "markdown": text.Markdown,
		"outline": kb.Outline(text.Markdown), "versions": vs, "format": docFormat(d),
	}
	// 维护者编辑前要知道：这篇最近被 AI 读过几次（改了以后，引用旧版本的答案会显示「原文已更新」）
	if v.kbMaintains(b) {
		reads, err := s.store.RecentKQueries(r.Context(), store.KQueryFilter{Tool: "read", Base: b.ID, Since: time.Now().AddDate(0, 0, -30), Limit: 5000})
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		n := 0
		for _, q := range reads {
			if slices.Contains(q.Docs, d.ID) {
				n++
			}
		}
		resp["reads_30d"] = n
	}
	writeJSON(w, http.StatusOK, resp)
}

type kdocPatchReq struct {
	Title    *string `json:"title"`
	Folder   *string `json:"folder"`
	Markdown *string `json:"markdown"`
	Note     string  `json:"note"`
}

func (s *Server) requireKDocEdit(w http.ResponseWriter, v viewer, b *store.KBase) bool {
	if !v.kbMaintains(b) {
		writeErr(w, http.StatusForbidden, "只有维护者或知识库管理员能修改文档")
		return false
	}
	return true
}

// PATCH /api/kb-docs/{id}：改标题、目录或正文（正文改了出一个新版本；回滚就是把旧版本的正文提交一次）。
func (s *Server) handleKDocPatch(w http.ResponseWriter, r *http.Request) {
	d, b, v, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	var req kdocPatchReq
	r.Body = http.MaxBytesReader(w, r.Body, maxKBMarkdown+(1<<20))
	if !readJSONLimit(w, r, &req, maxKBMarkdown+(1<<20)) {
		return
	}
	if !s.requireKDocEdit(w, v, b) {
		return
	}
	p := store.KDocPatch{Note: truncateStr(strings.TrimSpace(req.Note), 200)}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" || !limitRunes(t, 200) {
			writeErr(w, http.StatusBadRequest, "标题必填，最多 200 个字")
			return
		}
		p.Title = &t
	}
	if req.Folder != nil {
		f, msg := cleanFolder(*req.Folder)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		p.Folder = &f
	}
	if req.Markdown != nil {
		norm := normalizeNewlines(*req.Markdown)
		req.Markdown = &norm
		if strings.TrimSpace(*req.Markdown) == "" {
			writeErr(w, http.StatusBadRequest, "正文不能为空")
			return
		}
		if len(*req.Markdown) > maxKBMarkdown {
			writeErr(w, http.StatusBadRequest, "正文不能超过 8 MB")
			return
		}
		cur, err := s.store.KDocText(r.Context(), d.ID, 0)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if cur.Markdown != *req.Markdown {
			p.Markdown = req.Markdown
			if p.Note == "" {
				p.Note = "网页编辑"
			}
			// 记下「手工改过」：重新转换会覆盖这些修改，网页上要提醒
			if len(d.ConvertInfo) > 0 {
				var c kdocConvert
				if json.Unmarshal(d.ConvertInfo, &c) == nil && !c.Edited {
					c.Edited = true
					p.ConvertInfo, _ = json.Marshal(c)
				}
			}
		}
	}
	updated, err := s.store.UpdateKDoc(r.Context(), d.ID, p, currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBDocEdit, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"title": updated.Title, "version": updated.Version})
	writeJSON(w, http.StatusOK, updated)
}

// POST /api/kb-docs/{id}/file：重新上传这篇文档的文件，正文换成新文件的内容（出一个新版本）。
func (s *Server) handleKDocReplace(w http.ResponseWriter, r *http.Request) {
	d, b, v, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	if !s.requireKDocEdit(w, v, b) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxKBFile+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "文件太大，最多 50 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "要上传一个文件（字段 file）")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, maxKBFile+1))
	f.Close()
	if err != nil || len(data) > maxKBFile {
		writeErr(w, http.StatusBadRequest, "读取文件失败，或文件超过 50 MB")
		return
	}
	conv, origin, err := s.convertUpload(fh.Filename, data, kb.Options{Header: savedHeader(d)})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.UpdateKDoc(r.Context(), d.ID, store.KDocPatch{Markdown: &conv.Markdown, Note: "重新上传 " + origin.SourceName, Origin: origin, ConvertInfo: origin.ConvertInfo}, currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBDocEdit, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"file": origin.SourceName, "version": updated.Version})
	writeJSON(w, http.StatusOK, updated)
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// docFormat 是文档的格式（pdf、docx、xlsx……），阅读视图按它显示分页线、工作表、每页卡片；网页新建的是 md。
func docFormat(d *store.KDoc) string {
	var c kdocConvert
	if len(d.ConvertInfo) > 0 && json.Unmarshal(d.ConvertInfo, &c) == nil && c.Format != "" {
		return c.Format
	}
	switch ext := strings.TrimPrefix(strings.ToLower(path.Ext(d.SourceName)), "."); ext {
	case "pdf", "docx", "xlsx", "csv", "pptx", "txt":
		return ext
	}
	return "md"
}

// savedHeader 是维护者上次给这篇文档选的表头设置。
func savedHeader(d *store.KDoc) string {
	var c kdocConvert
	if len(d.ConvertInfo) > 0 && json.Unmarshal(d.ConvertInfo, &c) == nil {
		return c.HeaderOption
	}
	return ""
}

type kdocReconvertReq struct {
	Header string `json:"header"`
}

// POST /api/kb-docs/{id}/reconvert：用原件按新的设置（Excel / CSV 的表头）重新转换；正文变了就出一个新版本。
// 网页上改过的正文会被覆盖，网页上先提醒。
func (s *Server) handleKDocReconvert(w http.ResponseWriter, r *http.Request) {
	d, b, v, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	var req kdocReconvertReq
	if !readJSON(w, r, &req) {
		return
	}
	if !s.requireKDocEdit(w, v, b) {
		return
	}
	header, okHeader := headerOption(req.Header)
	if !okHeader {
		writeErr(w, http.StatusBadRequest, "header 只能是 first 或 none")
		return
	}
	if d.BlobKey == "" || s.registry == nil {
		writeErr(w, http.StatusBadRequest, "这篇文档没有原件，不能重新转换")
		return
	}
	f, err := s.registry.Blobs().Open(d.BlobKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "原件已经不在了，请重新上传")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, maxKBFile+1))
	f.Close()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	opt := kb.Options{Header: header}
	conv, err := kb.Convert(d.SourceName, data, opt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(conv.Markdown) > maxKBMarkdown {
		writeErr(w, http.StatusBadRequest, "转换出来的正文超过 8 MB，请拆成几个文件")
		return
	}
	cur, err := s.store.KDocText(r.Context(), d.ID, 0)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	p := store.KDocPatch{ConvertInfo: convertRecord(conv.Info, opt)}
	if cur.Markdown != conv.Markdown {
		p.Markdown, p.Note = &conv.Markdown, "重新转换"
	}
	updated, err := s.store.UpdateKDoc(r.Context(), d.ID, p, currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBDocEdit, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"reconvert": header, "version": updated.Version})
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/kb-docs/{id}
func (s *Server) handleKDocDelete(w http.ResponseWriter, r *http.Request) {
	d, b, v, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	if !s.requireKDocEdit(w, v, b) {
		return
	}
	if err := s.store.DeleteKDoc(r.Context(), d.ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBDocDelete, "kb:"+b.Slug+"/"+strconv.FormatInt(d.ID, 10), map[string]any{"title": d.Title})
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/kb-docs/{id}/original：下载上传时的原件。
func (s *Server) handleKDocOriginal(w http.ResponseWriter, r *http.Request) {
	d, _, _, ok := s.loadKDoc(w, r)
	if !ok {
		return
	}
	if d.BlobKey == "" || s.registry == nil {
		writeErr(w, http.StatusNotFound, "这篇文档没有原件")
		return
	}
	f, err := s.registry.Blobs().Open(d.BlobKey)
	if err != nil {
		writeErr(w, http.StatusNotFound, "原件已经不在了")
		return
	}
	defer f.Close()
	ct := d.Mime
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.SourceName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", d.UpdatedAt, f)
}

type kbSearchReq struct {
	Query  string  `json:"query"`
	Mode   string  `json:"mode"`
	KBIDs  []int64 `json:"kb_ids"`
	Folder string  `json:"folder"`
}

// POST /api/kb/search：网页上的「试搜」，结果与 DSH 里模型看到的一样（不记检索记录）。
func (s *Server) handleKBSearch(w http.ResponseWriter, r *http.Request) {
	var req kbSearchReq
	if !readJSON(w, r, &req) {
		return
	}
	v := viewerOf(r)
	bases, err := s.readableKBases(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	res, err := s.kbSearch(r.Context(), v, bases, req.KBIDs, nil, req.Query, req.Mode, req.Folder, 20)
	var qe *kb.QueryError
	if errors.As(err, &qe) {
		writeErr(w, http.StatusBadRequest, qe.Msg)
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// kbSearch 在 bases 里（ids 不为空时只搜其中这几个）检索。
func (s *Server) kbSearch(ctx context.Context, v viewer, bases []*store.KBase, ids, docs []int64, query, mode, folder string, limit int) (*kb.Result, error) {
	if !slices.Contains([]string{"", "any", "all", "phrase", "regex"}, mode) {
		return nil, &kb.QueryError{Msg: "mode 只能是 any、all、phrase 或 regex"}
	}
	if strings.TrimSpace(query) == "" {
		return nil, &kb.QueryError{Msg: "检索词不能为空"}
	}
	if len([]rune(query)) > 200 {
		return nil, &kb.QueryError{Msg: "检索词最多 200 个字"}
	}
	var pick []*store.KBase
	for _, b := range bases {
		if len(ids) == 0 || slices.Contains(ids, b.ID) {
			pick = append(pick, b)
		}
	}
	if err := s.kbSync(ctx, pick); err != nil {
		return nil, err
	}
	gates, err := s.kbGates(ctx, v, pick)
	if err != nil {
		return nil, err
	}
	q := kb.Query{Text: query, Mode: mode, Folder: folder, Docs: docs, Limit: limit, Allow: allowFunc(gates)}
	for _, b := range pick {
		q.Bases = append(q.Bases, b.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.kb.ix.Search(ctx, q)
}

// GET /api/kb/queries：检索记录。?kb= 看某个库的（维护者），否则看自己的；?all=1 看全部（审计权限或知识库管理员）。
func (s *Server) handleKBQueries(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	f := store.KQueryFilter{UserID: v.user.ID, Since: time.Now().AddDate(0, 0, -30), Limit: 200}
	if raw := r.URL.Query().Get("kb"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "kb 不对")
			return
		}
		b, err := s.store.GetKBase(r.Context(), id)
		if notFound(err) || (err == nil && !v.kbMaintains(b)) {
			writeErr(w, http.StatusForbidden, "只有维护者能看这个库的检索记录")
			return
		}
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		f.UserID, f.Base = 0, id
	} else if r.URL.Query().Get("all") == "1" {
		if !v.kbManages() && !v.access.Has(rbac.AuditView) {
			writeErr(w, http.StatusForbidden, "看全部检索记录需要「管理知识库」或「查看审计日志」权限")
			return
		}
		f.UserID = 0
	}
	if t := r.URL.Query().Get("tool"); t != "" {
		f.Tool = t
	}
	list, err := s.store.RecentKQueries(r.Context(), f)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	users, err := s.userIndex(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type item struct {
		*store.KQuery
		User string `json:"user"`
	}
	out := make([]item, len(list))
	for i, q := range list {
		out[i] = item{KQuery: q, User: users.name(&q.UserID)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type kbSettingsReq struct {
	QAEnabled  bool     `json:"qaEnabled"`
	QATitle    string   `json:"qaTitle"`
	QAIcon     string   `json:"qaIcon"`
	QAStarters []string `json:"qaStarters"`
	QAExtra    string   `json:"qaExtra"`
}

// PUT /api/kb/settings：「知识问答」的开关、名称、图标、示例问题与补充要求。
func (s *Server) handleKBSettings(w http.ResponseWriter, r *http.Request) {
	var req kbSettingsReq
	if !readJSON(w, r, &req) {
		return
	}
	req.QATitle, req.QAIcon, req.QAExtra = strings.TrimSpace(req.QATitle), strings.TrimSpace(req.QAIcon), strings.TrimSpace(req.QAExtra)
	if req.QATitle == "" || !limitRunes(req.QATitle, 32) {
		writeErr(w, http.StatusBadRequest, "名称必填，最多 32 个字")
		return
	}
	if !limitRunes(req.QAIcon, 8) {
		writeErr(w, http.StatusBadRequest, "图标最多 8 个字符")
		return
	}
	if !limitRunes(req.QAExtra, 2000) {
		writeErr(w, http.StatusBadRequest, "补充要求最多 2000 个字")
		return
	}
	starters := []string{}
	for _, q := range req.QAStarters {
		if q = strings.TrimSpace(q); q != "" && !slices.Contains(starters, q) {
			if !limitRunes(q, 200) {
				writeErr(w, http.StatusBadRequest, "示例问题每条最多 200 个字")
				return
			}
			starters = append(starters, q)
		}
	}
	if len(starters) > 6 {
		writeErr(w, http.StatusBadRequest, "示例问题最多 6 个")
		return
	}
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	cur.Knowledge.QAEnabled, cur.Knowledge.QATitle, cur.Knowledge.QAIcon = req.QAEnabled, req.QATitle, req.QAIcon
	cur.Knowledge.QAStarters, cur.Knowledge.QAExtra = starters, req.QAExtra
	if err := s.store.SaveSettings(r.Context(), cur, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.settings.mu.Lock()
	s.settings.v = nil
	s.settings.mu.Unlock()
	s.audit(r, auditSettingsUpdate, "knowledge", map[string]any{"sections": []string{"knowledge"}, "qa_enabled": req.QAEnabled})
	s.hub.notify()
	writeJSON(w, http.StatusOK, cur.Knowledge)
}
