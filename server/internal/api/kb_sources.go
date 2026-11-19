package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 知识源（docs/plugins.md §6.2）：把一个服务扩展挂到知识库上。
//
//   - 同步（sync）：dsh-ee 定时拉取管理员勾选的范围，内容进知识库，之后照常用 kb__search / kb__read。
//     同步时没有员工，用系统身份和共享凭据。同步过来的内容不带源系统里的权限：谁能看由这个知识库的权限决定。
//   - 联邦检索（federated）：不复制。员工检索时用他自己的身份实时去查，源系统按它自己的权限返回。
//     每个来源有时间上限，超时或报错只在结果里注明，不影响别的来源。

const (
	auditKBSourceAdd    = "kb.source.add"
	auditKBSourceDelete = "kb.source.delete"

	kbSourceInterval = 30 * time.Minute
	// 一轮同步最多翻多少页、处理多少篇，免得一个写坏的扩展把同步拖死
	kbSourceMaxPages = 200
	kbSourceMaxDocs  = 5000
	kbSourceMaxBody  = 2 << 20
	// 联邦检索每个来源的时间上限
	kbFederatedTimeout = 3 * time.Second
	kbFederatedLimit   = 8
	// 联邦检索结果里文档的编号前缀：ext:<知识源 id>:<源系统里的 id>
	kbExternalPrefix = "ext:"
)

// kbSourceLocks 让同一个知识源同时只有一轮同步在跑。
var kbSourceLocks sync.Map

type kbSourceResult struct {
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Deleted int    `json:"deleted"`
	Skipped int    `json:"skipped"`
	Docs    int    `json:"docs"`
	Error   string `json:"error,omitempty"`
}

// sourceExtension 取知识源背后的扩展连接。
func (s *Server) sourceExtension(ctx context.Context, assetID int64) (*store.Asset, *assetSpec, error) {
	if isBuiltinSource(assetID) {
		a, sp := builtinFeishuDocsSource()
		return a, sp, nil
	}
	a, err := s.store.GetAsset(ctx, assetID)
	if err != nil {
		return nil, nil, err
	}
	if a.Kind != store.KindConnection || a.Status != store.AssetActive || a.LiveVersion == 0 {
		return nil, nil, errors.New("知识源背后的扩展已经停用或卸载")
	}
	lv, err := s.store.LiveAssetVersion(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}
	sp := parseSpec(lv.Spec)
	if !sp.Extension.service() {
		return nil, nil, errors.New("知识源背后的连接不是服务扩展")
	}
	return a, sp, nil
}

// syncKBSource 跑一轮同步。
func (s *Server) syncKBSource(ctx context.Context, src *store.KBSource, base string) *kbSourceResult {
	res := &kbSourceResult{}
	mu, _ := kbSourceLocks.LoadOrStore(src.ID, &sync.Mutex{})
	if !mu.(*sync.Mutex).TryLock() {
		res.Error = "这个知识源正在同步"
		return res
	}
	defer mu.(*sync.Mutex).Unlock()
	cursor, err := s.syncKBSourcePages(ctx, src, base, res)
	if err != nil {
		res.Error = err.Error()
	}
	mapped, mapErr := s.store.KBSourceDocs(ctx, src.ID)
	if mapErr == nil {
		res.Docs = len(mapped)
	}
	if err := s.store.SaveKBSourceSync(context.WithoutCancel(ctx), src.ID, cursor, res.Error, res.Docs); err != nil {
		s.log.Warn("保存知识源同步进度失败", "source", src.ID, "err", err)
	}
	if res.Added+res.Updated+res.Deleted > 0 {
		s.hub.notify()
	}
	return res
}

func (s *Server) syncKBSourcePages(ctx context.Context, src *store.KBSource, base string, res *kbSourceResult) (string, error) {
	cursor := src.Cursor
	a, sp, err := s.sourceExtension(ctx, src.AssetID)
	if err != nil {
		return cursor, err
	}
	b, err := s.store.GetKBase(ctx, src.BaseID)
	if err != nil {
		return cursor, err
	}
	mapped, err := s.store.KBSourceDocs(ctx, src.ID)
	if err != nil {
		return cursor, err
	}
	scope := make([]string, len(src.Scope))
	for i, n := range src.Scope {
		scope[i] = n.ID
	}
	handled := 0
	for page := 0; page < kbSourceMaxPages; page++ {
		var changes struct {
			Items []struct {
				ID      string `json:"id"`
				Title   string `json:"title"`
				Deleted bool   `json:"deleted"`
				Folder  string `json:"folder"`
			} `json:"items"`
			Cursor  string `json:"cursor"`
			HasMore bool   `json:"hasMore"`
		}
		if err := s.extCall(ctx, a, sp, viewer{}, base, "kb/changes", map[string]any{"scope": scope, "cursor": cursor}, &changes); err != nil {
			return cursor, fmt.Errorf("取增量失败：%w", err)
		}
		for _, it := range changes.Items {
			if it.ID == "" || len(it.ID) > 255 {
				res.Skipped++
				continue
			}
			if handled++; handled > kbSourceMaxDocs {
				return cursor, fmt.Errorf("一轮同步最多处理 %d 篇，剩下的下一轮接着同步", kbSourceMaxDocs)
			}
			docID, known := mapped[it.ID]
			if it.Deleted {
				if known {
					if err := s.store.DeleteKDoc(ctx, docID); err != nil && !notFound(err) {
						return cursor, err
					}
					_ = s.store.DeleteKBSourceDoc(ctx, src.ID, it.ID)
					delete(mapped, it.ID)
					res.Deleted++
				}
				continue
			}
			var doc struct {
				ID      string         `json:"id"`
				Title   string         `json:"title"`
				Format  string         `json:"format"`
				Content string         `json:"content"`
				URL     string         `json:"url"`
				Fields  map[string]any `json:"fields"`
			}
			if err := s.extCall(ctx, a, sp, viewer{}, base, "kb/document", map[string]any{"id": it.ID}, &doc); err != nil {
				if extCode(err) == "not_found" {
					res.Skipped++
					continue
				}
				return cursor, fmt.Errorf("取文档 %s 失败：%w", it.ID, err)
			}
			title := strings.TrimSpace(doc.Title)
			if title == "" {
				title = strings.TrimSpace(it.Title)
			}
			if title == "" {
				title = it.ID
			}
			title = cutUTF8(title, 200)
			body := normalizeNewlines(doc.Content)
			if strings.TrimSpace(body) == "" || len(body) > kbSourceMaxBody {
				res.Skipped++
				continue
			}
			folder, msg := cleanFolder(strings.Trim(src.Folder+"/"+it.Folder, "/"))
			if msg != "" {
				folder = src.Folder
			}
			note := "同步自 " + a.Title
			if known {
				cur, err := s.store.KDocText(ctx, docID, 0)
				if err == nil {
					old, _ := s.store.GetKDoc(ctx, docID)
					if cur.Markdown == body && old != nil && old.Title == title && old.Folder == folder {
						s.applySourceFields(ctx, sp, docID, doc.Fields)
						continue
					}
					patch := store.KDocPatch{Title: &title, Folder: &folder}
					if cur.Markdown != body {
						patch.Markdown, patch.Note = &body, note
					}
					if _, err := s.store.UpdateKDoc(ctx, docID, patch, 0); err != nil {
						return cursor, err
					}
					s.applySourceFields(ctx, sp, docID, doc.Fields)
					res.Updated++
					continue
				}
				if !notFound(err) {
					return cursor, err
				}
				// 文档被人在知识库里删掉了：重新建
			}
			d := &store.KDoc{BaseID: b.ID, Title: title, Folder: folder, SourceName: doc.URL, Mime: "text/markdown", Size: int64(len(body))}
			if err := s.store.CreateKDoc(ctx, d, body, note, 0); err != nil {
				return cursor, err
			}
			if err := s.store.SetKBSourceDoc(ctx, src.ID, it.ID, d.ID); err != nil {
				return cursor, err
			}
			mapped[it.ID] = d.ID
			s.applySourceFields(ctx, sp, d.ID, doc.Fields)
			res.Added++
		}
		// 一页处理完才前进：中途失败下一轮从这一页重来，不会漏
		if changes.Cursor != "" {
			cursor = changes.Cursor
		}
		if !changes.HasMore {
			return cursor, nil
		}
		if changes.Cursor == "" {
			return cursor, errors.New("扩展说还有更多变化，但没有给出下一页的 cursor")
		}
	}
	return cursor, fmt.Errorf("一轮同步最多翻 %d 页，剩下的下一轮接着同步", kbSourceMaxPages)
}

// runKBSourceSync 定时同步所有「同步」方式的知识源。
func (s *Server) runKBSourceSync(ctx context.Context) {
	t := time.NewTicker(kbSourceInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runLeased(ctx, leaseKBSourceSync, func(ctx context.Context) {
				base := s.systemBase(ctx)
				if base == "" {
					return
				}
				sources, err := s.store.ListKBSources(ctx, 0, store.KBSourceSync)
				if err != nil {
					s.log.Warn("列出知识源失败", "err", err)
					return
				}
				for _, src := range sources {
					if res := s.syncKBSource(ctx, src, base); res.Error != "" {
						s.log.Warn("知识源同步没有完成", "source", src.ID, "err", res.Error)
					}
				}
			})
		}
	}
}

// ---- 管理接口 ----

// loadKBForSources：管知识源要是这个库的维护者。
func (s *Server) loadKBForSources(w http.ResponseWriter, r *http.Request) (*store.KBase, viewer, bool) {
	b, v, ok := s.loadKBase(w, r)
	if !ok {
		return nil, v, false
	}
	if !v.kbMaintains(b) {
		writeErr(w, http.StatusForbidden, "只有这个知识库的维护者能管理知识源")
		return nil, v, false
	}
	return b, v, true
}

// knowledgeExtensions 列出这个人能挂成知识源的扩展：服务扩展、实现了知识源接口、他能用或能管。
func (s *Server) knowledgeExtensions(ctx context.Context, v viewer) ([]map[string]any, error) {
	exts, specs, err := s.serviceExtensions(ctx)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	// 内置的飞书文档：用飞书应用的身份读知识空间，内容同步进来后按库的权限给人看，所以只给能管理连接的人挂
	if v.manages(store.KindConnection) {
		if c, _ := s.feishuDocsClient(ctx); c != nil {
			a, sp := builtinFeishuDocsSource()
			out = append(out, map[string]any{"asset_id": a.ID, "slug": a.Slug, "title": a.Title, "extension": sp.Extension.ID, "modes": []string{store.KBSourceSync}, "builtin": true})
		}
	}
	for _, a := range exts {
		e := specs[a.ID].Extension
		modes := []string{}
		// 同步用扩展的系统身份读，读到的内容按库的权限给人看：和内置的飞书文档一样，只给能管理连接的人挂
		if e.has("knowledge.documents") && v.manages(store.KindConnection) {
			modes = append(modes, store.KBSourceSync)
		}
		if e.has("knowledge.search") {
			modes = append(modes, store.KBSourceFederated)
		}
		if len(modes) == 0 || (!available(v, a) && !v.manages(a.Kind)) {
			continue
		}
		out = append(out, map[string]any{"asset_id": a.ID, "slug": a.Slug, "title": a.Title, "extension": e.ID, "modes": modes})
	}
	return out, nil
}

// GET /api/kb/{id}/sources：这个库挂了哪些知识源，以及还能挂哪些扩展。
func (s *Server) handleKBSources(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBForSources(w, r)
	if !ok {
		return
	}
	sources, err := s.store.ListKBSources(r.Context(), b.ID, "")
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	exts, err := s.knowledgeExtensions(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "extensions": exts})
}

// syncSourceForbidden：同步方式的知识源用扩展的系统身份（服务账号）读源系统，读到的内容进库后
// 按库的权限给人看，等于把服务账号能读到的东西开放给库里的人。和内置的飞书文档一样，只有能管理
// 企业连接的人能挂；联邦检索用员工自己的身份查，库的维护者就能挂。
const syncSourceForbidden = "同步方式用扩展的系统身份读源系统，只有能管理企业连接的人能挂；你可以挂联邦检索（按每个人自己的权限实时查）"

// usableSourceExtension 取出请求里要挂的扩展，并确认这个人能用它。
func (s *Server) usableSourceExtension(ctx context.Context, v viewer, assetID int64) (*store.Asset, *assetSpec, string) {
	a, sp, err := s.sourceExtension(ctx, assetID)
	if err == nil && isBuiltinSource(assetID) {
		if !v.manages(store.KindConnection) {
			return nil, nil, "内置的飞书文档只有能管理企业连接的人能挂"
		}
		if _, why := s.feishuDocsClient(ctx); why != "" {
			return nil, nil, "飞书文档用不了：" + why
		}
		return a, sp, ""
	}
	if err != nil || (!available(v, a) && !v.manages(a.Kind)) {
		return nil, nil, "没有这个扩展，或你没有使用它的权限"
	}
	return a, sp, ""
}

// POST /api/kb/{id}/sources/browse {asset_id, parent}：看知识源里有哪些空间、文件夹和文档，用来勾选范围。
func (s *Server) handleKBSourceBrowse(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.loadKBForSources(w, r); !ok {
		return
	}
	var req struct {
		AssetID int64  `json:"asset_id"`
		Parent  string `json:"parent"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	v := viewerOf(r)
	a, sp, msg := s.usableSourceExtension(r.Context(), v, req.AssetID)
	if msg != "" {
		writeErr(w, http.StatusNotFound, msg)
		return
	}
	// 浏览用的是扩展的系统身份，只为挑同步的范围；挂同步要能管理连接，浏览也一样
	if !v.manages(store.KindConnection) {
		writeErr(w, http.StatusForbidden, syncSourceForbidden)
		return
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := s.extCall(r.Context(), a, sp, viewer{}, s.callerBase(r), "kb/browse", map[string]any{"parent": req.Parent}, &out); err != nil {
		if extCode(err) == "not_implemented" {
			writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "unsupported": true})
			return
		}
		writeErr(w, http.StatusBadGateway, "取目录失败："+err.Error())
		return
	}
	if out.Items == nil {
		out.Items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/kb/{id}/sources {asset_id, mode, scope, folder, title}：挂一个知识源。同步方式的立刻先同步一轮。
func (s *Server) handleKBSourceCreate(w http.ResponseWriter, r *http.Request) {
	b, v, ok := s.loadKBForSources(w, r)
	if !ok {
		return
	}
	var req struct {
		AssetID int64                `json:"asset_id"`
		Mode    string               `json:"mode"`
		Scope   []store.KBSourceNode `json:"scope"`
		Folder  string               `json:"folder"`
		Title   string               `json:"title"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	a, sp, msg := s.usableSourceExtension(r.Context(), v, req.AssetID)
	if msg != "" {
		writeErr(w, http.StatusNotFound, msg)
		return
	}
	need := map[string]string{store.KBSourceSync: "knowledge.documents", store.KBSourceFederated: "knowledge.search"}[req.Mode]
	if need == "" {
		writeErr(w, http.StatusBadRequest, "方式只能是 sync（同步）或 federated（联邦检索）")
		return
	}
	if !sp.Extension.has(need) {
		writeErr(w, http.StatusBadRequest, "这个扩展不支持这种方式")
		return
	}
	if req.Mode == store.KBSourceSync && !v.manages(store.KindConnection) {
		writeErr(w, http.StatusForbidden, syncSourceForbidden)
		return
	}
	folder, msg := cleanFolder(req.Folder)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if len(req.Scope) > 200 {
		writeErr(w, http.StatusBadRequest, "最多勾选 200 个节点；范围太散的话勾上一层")
		return
	}
	scope := []store.KBSourceNode{}
	for _, n := range req.Scope {
		if n.ID = strings.TrimSpace(n.ID); n.ID != "" && len(n.ID) <= 255 {
			scope = append(scope, store.KBSourceNode{ID: n.ID, Title: cutUTF8(strings.TrimSpace(n.Title), 100)})
		}
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = a.Title
	}
	src := &store.KBSource{BaseID: b.ID, AssetID: a.ID, Mode: req.Mode, Title: cutUTF8(title, 120), Scope: scope, Folder: folder, CreatedBy: v.user.ID}
	if err := s.store.CreateKBSource(r.Context(), src); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBSourceAdd, "kb:"+b.Slug, map[string]any{"source": src.ID, "extension": sp.Extension.ID, "mode": src.Mode, "scope": len(scope)})
	out := map[string]any{"source": src}
	if src.Mode == store.KBSourceSync {
		out["sync"] = s.syncKBSource(r.Context(), src, s.callerBase(r))
		if fresh, err := s.store.GetKBSource(r.Context(), src.ID); err == nil {
			out["source"] = fresh
		}
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) loadKBSource(w http.ResponseWriter, r *http.Request) (*store.KBase, *store.KBSource, bool) {
	b, _, ok := s.loadKBForSources(w, r)
	if !ok {
		return nil, nil, false
	}
	sid, err := strconv.ParseInt(r.PathValue("source"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "知识源 id 不对")
		return nil, nil, false
	}
	src, err := s.store.GetKBSource(r.Context(), sid)
	if notFound(err) || (err == nil && src.BaseID != b.ID) {
		writeErr(w, http.StatusNotFound, "没有这个知识源")
		return nil, nil, false
	}
	if err != nil {
		s.internalErr(w, r, err)
		return nil, nil, false
	}
	return b, src, true
}

// POST /api/kb/{id}/sources/{source}/sync：现在同步一轮。
func (s *Server) handleKBSourceSync(w http.ResponseWriter, r *http.Request) {
	_, src, ok := s.loadKBSource(w, r)
	if !ok {
		return
	}
	if src.Mode != store.KBSourceSync {
		writeErr(w, http.StatusBadRequest, "联邦检索的知识源不用同步")
		return
	}
	writeJSON(w, http.StatusOK, s.syncKBSource(r.Context(), src, s.callerBase(r)))
}

// DELETE /api/kb/{id}/sources/{source}?docs=delete：摘掉知识源。默认保留已经同步进来的文档，docs=delete 时一起删。
func (s *Server) handleKBSourceDelete(w http.ResponseWriter, r *http.Request) {
	b, src, ok := s.loadKBSource(w, r)
	if !ok {
		return
	}
	removed := 0
	if r.URL.Query().Get("docs") == "delete" {
		mapped, err := s.store.KBSourceDocs(r.Context(), src.ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		for _, docID := range mapped {
			if err := s.store.DeleteKDoc(r.Context(), docID); err == nil {
				removed++
			}
		}
	}
	if err := s.store.DeleteKBSource(r.Context(), src.ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKBSourceDelete, "kb:"+b.Slug, map[string]any{"source": src.ID, "docs_removed": removed})
	s.hub.notify()
	writeJSON(w, http.StatusOK, map[string]any{"docs_removed": removed})
}

// ---- 联邦检索 ----

// kbFederated 是一个外部来源这一次检索的结果。
type kbFederated struct {
	Source string           `json:"source"`
	KB     string           `json:"kb"`
	Hits   []map[string]any `json:"hits"`
	// 这个来源这一次没查成：超时、对方不可用、员工还没授权
	Error string `json:"error,omitempty"`
}

// callerOf 取出这次调用的人访问 dsh-ee 的地址（签 ee_jwt 用）；没有就用系统设置里的。
func (s *Server) callerOf(ctx context.Context) string {
	if who, ok := ctx.Value(connCallerKey{}).(connCaller); ok && who.base != "" {
		return who.base
	}
	return s.systemBase(ctx)
}

// kbFederatedSearch 用这位员工自己的身份去查这些库上挂的联邦检索来源。各来源并行，互不影响。
// 结果不参与本地的排序打分：按来源分组排在本地结果后面，各来源内部保持源系统给的顺序。
func (s *Server) kbFederatedSearch(ctx context.Context, v viewer, bases []*store.KBase, query string) []kbFederated {
	var sources []*store.KBSource
	slugOf := map[int64]string{}
	for _, b := range bases {
		slugOf[b.ID] = b.Slug
		list, err := s.store.ListKBSources(ctx, b.ID, store.KBSourceFederated)
		if err != nil {
			s.log.Warn("列出联邦检索来源失败", "err", err)
			continue
		}
		sources = append(sources, list...)
	}
	if len(sources) == 0 {
		return nil
	}
	base := s.callerOf(ctx)
	out := make([]kbFederated, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		out[i] = kbFederated{Source: src.Title, KB: slugOf[src.BaseID], Hits: []map[string]any{}}
		wg.Add(1)
		go func(i int, src *store.KBSource) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, kbFederatedTimeout)
			defer cancel()
			a, sp, err := s.sourceExtension(cctx, src.AssetID)
			if err != nil {
				out[i].Error = "这个来源暂时不可用"
				return
			}
			// 员工要在这个扩展的可用范围内：联邦检索用的是他自己的身份
			if !available(v, a) {
				out[i].Error = "你不在这个来源的可用范围内"
				return
			}
			var resp struct {
				Items []struct {
					ID      string `json:"id"`
					Title   string `json:"title"`
					Snippet string `json:"snippet"`
					URL     string `json:"url"`
				} `json:"items"`
			}
			err = s.extCall(cctx, a, sp, v, base, "kb/search", map[string]any{"query": query, "limit": kbFederatedLimit}, &resp)
			switch {
			case errors.Is(err, errNeedCredential), errors.Is(err, errNeedAuthorize), extCode(err) == "authorize":
				out[i].Error = "你还没有在「" + a.Title + "」上授权：打开 " + base + "/assets/" + strconv.FormatInt(a.ID, 10) + " 完成后再查"
			case extCode(err) == "forbidden":
				out[i].Error = "对方系统说你没有权限"
			case err != nil:
				out[i].Error = "这个来源暂时不可用"
				s.log.Info("联邦检索的一个来源没查成", "source", src.ID, "err", err)
			}
			for n, it := range resp.Items {
				if n >= kbFederatedLimit || it.ID == "" {
					break
				}
				hit := map[string]any{"doc": kbExternalPrefix + strconv.FormatInt(src.ID, 10) + ":" + it.ID, "title": cutUTF8(it.Title, 200), "snippet": cutUTF8(it.Snippet, 600)}
				if it.URL != "" {
					hit["url"] = it.URL
				}
				out[i].Hits = append(out[i].Hits, hit)
			}
		}(i, src)
	}
	wg.Wait()
	return out
}

// kbReadExternal 读联邦检索来源里的一篇文档：ext:<知识源 id>:<源系统里的 id>。用员工自己的身份去取，不落库。
func (s *Server) kbReadExternal(ctx context.Context, v viewer, bases []*store.KBase, ref string, from, to int) (map[string]any, string) {
	rest := strings.TrimPrefix(ref, kbExternalPrefix)
	sidText, extID, ok := strings.Cut(rest, ":")
	sid, err := strconv.ParseInt(sidText, 10, 64)
	if !ok || err != nil || extID == "" {
		return nil, "外部文档的编号不对：要原样用检索结果里的 doc"
	}
	src, err := s.store.GetKBSource(ctx, sid)
	if err != nil || src.Mode != store.KBSourceFederated || !slices.ContainsFunc(bases, func(b *store.KBase) bool { return b.ID == src.BaseID }) {
		return nil, "没有这篇外部文档，或你没有查看权限"
	}
	a, sp, err := s.sourceExtension(ctx, src.AssetID)
	if err != nil || !available(v, a) {
		return nil, "这个来源暂时不可用，或你不在它的可用范围内"
	}
	base := s.callerOf(ctx)
	var doc struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		URL     string `json:"url"`
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.extCall(cctx, a, sp, v, base, "kb/document", map[string]any{"id": extID}, &doc); err != nil {
		switch {
		case errors.Is(err, errNeedCredential), errors.Is(err, errNeedAuthorize), extCode(err) == "authorize":
			return nil, extErrorText(a, base, "authorize", "")
		case extCode(err) == "forbidden":
			return nil, extErrorText(a, base, "forbidden", err.Error())
		case extCode(err) == "not_found":
			return nil, "源系统里没有这篇文档了"
		}
		return nil, extErrorText(a, base, "unavailable", "")
	}
	lines := strings.Split(strings.TrimRight(normalizeNewlines(doc.Content), "\n"), "\n")
	if from < 1 {
		from = 1
	}
	if from > len(lines) {
		return nil, fmt.Sprintf("这篇文档只有 %d 行", len(lines))
	}
	if to <= 0 {
		to = from + kbReadDefault - 1
	}
	to = min(to, len(lines), from+kbReadMaxLines-1)
	var body strings.Builder
	for i := from; i <= to; i++ {
		line := fmt.Sprintf("%d| %s\n", i, lines[i-1])
		if body.Len()+len(line) > kbReadMaxBytes && i > from {
			to = i - 1
			break
		}
		body.WriteString(line)
	}
	out := map[string]any{"doc": ref, "title": doc.Title, "source": src.Title, "external": true, "total_lines": len(lines), "from": from, "to": to,
		"note": kbNote + "这篇来自外部系统「" + src.Title + "」，是按你本人的权限实时取的，没有版本号，引用时写明来源和标题。", "text": body.String()}
	if doc.URL != "" {
		out["url"] = doc.URL
	}
	if to < len(lines) {
		out["more"] = fmt.Sprintf("后面还有 %d 行，继续读用 from=%d", len(lines)-to, to+1)
	}
	return out, ""
}
