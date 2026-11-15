package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 专家 · 技能 · 连接器的管理接口（docs/expert-center.md §5.3）：受管分类、禁用策略、权限查询。
// 连接器分组看「管理企业连接」，其余看「管理能力与审核」，和 viewer.manages 一致。

const (
	auditCategoryEdit = "hub.category"
	auditBlockEdit    = "hub.block"
)

var categoryGroups = []string{store.CategoryGroupApp, store.CategoryGroupSkill, store.CategoryGroupConnection}

// managesGroup：能否管理某个分组的分类。
func (v viewer) managesGroup(group string) bool {
	if group == store.CategoryGroupConnection {
		return v.manages(store.KindConnection)
	}
	return v.manages(store.KindApp)
}

// resolveCategory 把请求里的分类（id 优先，其次是旧的分类文本）对到受管分类上。
// id 指向 0 表示取消归类。返回分类 id（nil 表示没归类）、要同步写进 category 列的文本，以及给提交人看的错误。
func (s *Server) resolveCategory(ctx context.Context, kind string, id *int64, name *string) (*int64, string, string, error) {
	group := store.CategoryGroupOf(kind)
	if id != nil {
		if *id == 0 {
			return nil, "", "", nil
		}
		c, err := s.store.GetCategory(ctx, *id)
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", "分类不存在", nil
		}
		if err != nil {
			return nil, "", "", err
		}
		if c.Group != group {
			return nil, "", "这个分类不属于这一类能力", nil
		}
		return &c.ID, c.Name, "", nil
	}
	text := ""
	if name != nil {
		text = truncateStr(strings.TrimSpace(*name), 32)
	}
	if text == "" {
		return nil, "", "", nil
	}
	cats, err := s.store.ListCategories(ctx)
	if err != nil {
		return nil, "", "", err
	}
	for _, c := range cats {
		if c.Group == group && c.Name == text {
			return &c.ID, c.Name, "", nil
		}
	}
	// 没有同名的受管分类：照旧只记文本，不受分类范围约束
	return nil, text, "", nil
}

// ---- 分类 ----

type categoryItem struct {
	*store.Category
	// 归在这个分类下的能力数
	Assets int `json:"assets"`
}

// GET /api/admin/hub/categories
func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	cats, err := s.store.ListCategories(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	assets, err := s.store.ListAssets(r.Context(), hubKinds...)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	count := map[int64]int{}
	for _, a := range assets {
		if a.CategoryID != nil {
			count[*a.CategoryID]++
		}
	}
	out := []categoryItem{}
	for _, c := range cats {
		if v.managesGroup(c.Group) {
			out = append(out, categoryItem{Category: c, Assets: count[c.ID]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

type categoryReq struct {
	Group string       `json:"group"`
	Name  *string      `json:"name"`
	Sort  *int         `json:"sort"`
	Scope *store.Scope `json:"scope"`
}

func checkCategoryName(name string) string {
	if name == "" || !limitRunes(name, 32) {
		return "分类名必填，最多 32 个字符"
	}
	return ""
}

// POST /api/admin/hub/categories
func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryReq
	if !readJSON(w, r, &req) {
		return
	}
	v := viewerOf(r)
	if !slices.Contains(categoryGroups, req.Group) {
		writeErr(w, http.StatusBadRequest, "group 只能是 app / skill / connection")
		return
	}
	if !v.managesGroup(req.Group) {
		writeErr(w, http.StatusForbidden, "没有管理这一类能力的权限")
		return
	}
	c := &store.Category{Group: req.Group, Scope: store.Scope{All: true}}
	if req.Name != nil {
		c.Name = strings.TrimSpace(*req.Name)
	}
	if msg := checkCategoryName(c.Name); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if req.Sort != nil {
		c.Sort = *req.Sort
	}
	if req.Scope != nil {
		c.Scope = req.Scope.Normalize()
	}
	if err := s.store.CreateCategory(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "这一类里已经有叫「"+c.Name+"」的分类")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditCategoryEdit, c.Group+":"+c.Name, map[string]any{"id": c.ID, "op": "create", "scope": c.Scope})
	s.hub.notify()
	writeJSON(w, http.StatusCreated, c)
}

// loadCategory 取路径里的分类，并检查能否管理它所在的分组。
func (s *Server) loadCategory(w http.ResponseWriter, r *http.Request) (*store.Category, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, false
	}
	c, err := s.store.GetCategory(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "分类不存在")
		return nil, false
	}
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	if !viewerOf(r).managesGroup(c.Group) {
		writeErr(w, http.StatusForbidden, "没有管理这一类能力的权限")
		return nil, false
	}
	return c, true
}

// PATCH /api/admin/hub/categories/{id}：改名、排序、可用范围（分组不能改）。
func (s *Server) handlePatchCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCategory(w, r)
	if !ok {
		return
	}
	var req categoryReq
	if !readJSON(w, r, &req) {
		return
	}
	if req.Group != "" && req.Group != c.Group {
		writeErr(w, http.StatusBadRequest, "分类的分组不能改；需要的话新建一个")
		return
	}
	if req.Name != nil {
		c.Name = strings.TrimSpace(*req.Name)
		if msg := checkCategoryName(c.Name); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	}
	if req.Sort != nil {
		c.Sort = *req.Sort
	}
	if req.Scope != nil {
		c.Scope = req.Scope.Normalize()
	}
	if err := s.store.UpdateCategory(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "这一类里已经有叫「"+c.Name+"」的分类")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditCategoryEdit, c.Group+":"+c.Name, map[string]any{"id": c.ID, "op": "edit", "scope": c.Scope, "sort": c.Sort})
	s.hub.notify()
	updated, err := s.store.GetCategory(r.Context(), c.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/admin/hub/categories/{id}：归在这里的能力变成没归类。
func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := s.loadCategory(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteCategory(r.Context(), c.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditCategoryEdit, c.Group+":"+c.Name, map[string]any{"id": c.ID, "op": "delete"})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// ---- 禁用策略 ----

// GET /api/admin/hub/blocks
func (s *Server) handleListBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := s.store.ListBlocks(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

type blockReq struct {
	Scope    *store.Scope `json:"scope"`
	AssetIDs []int64      `json:"asset_ids"`
	Note     string       `json:"note"`
}

// checkBlockAssets：禁用策略里的每个能力都要存在，而且自己能管理那一类。
// old 是策略原来挡住的能力：也要自己能管理，但已经删掉的跳过。
func (s *Server) checkBlockAssets(ctx context.Context, v viewer, ids, old []int64) (string, error) {
	assets, err := s.store.ListAssets(ctx, hubKinds...)
	if err != nil {
		return "", err
	}
	byID := map[int64]*store.Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	for _, id := range append(append([]int64{}, ids...), old...) {
		a := byID[id]
		if a == nil && slices.Contains(ids, id) {
			return "能力 " + strconv.FormatInt(id, 10) + " 不存在", nil
		}
		if a == nil {
			continue
		}
		if !v.manages(a.Kind) {
			return "没有管理「" + a.Title + "」这一类能力的权限", nil
		}
	}
	return "", nil
}

// saveBlock 是新建与修改共用的部分；id 为 0 表示新建。
func (s *Server) saveBlock(w http.ResponseWriter, r *http.Request, b *store.Block) {
	var req blockReq
	if !readJSON(w, r, &req) {
		return
	}
	v := viewerOf(r)
	if req.Scope == nil {
		writeErr(w, http.StatusBadRequest, "需要填禁用的范围（scope）")
		return
	}
	sc := req.Scope.Normalize()
	if !sc.All && len(sc.Departments) == 0 && len(sc.Roles) == 0 && len(sc.Users) == 0 {
		writeErr(w, http.StatusBadRequest, "禁用的范围不能为空")
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	if req.Note == "" || !limitRunes(req.Note, 500) {
		writeErr(w, http.StatusBadRequest, "请写明禁用原因（最多 500 个字符），权限查询里会显示给其他管理员")
		return
	}
	if len(req.AssetIDs) == 0 || len(req.AssetIDs) > 200 {
		writeErr(w, http.StatusBadRequest, "一条策略要选 1–200 项能力")
		return
	}
	// 改一条已有的策略：原来挡住的能力也要自己能管理，免得借修改把别人管的那一类放开
	msg, err := s.checkBlockAssets(r.Context(), v, req.AssetIDs, b.AssetIDs)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	created := b.ID == 0
	b.Scope, b.AssetIDs, b.Note = sc, req.AssetIDs, req.Note
	if err := s.store.SaveBlock(r.Context(), b, v.user.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "禁用策略不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	op, code := "edit", http.StatusOK
	if created {
		op, code = "create", http.StatusCreated
	}
	s.audit(r, auditBlockEdit, "block:"+strconv.FormatInt(b.ID, 10), map[string]any{"id": b.ID, "op": op, "scope": b.Scope, "asset_ids": b.AssetIDs, "note": b.Note})
	s.hub.notify()
	writeJSON(w, code, b)
}

// POST /api/admin/hub/blocks
func (s *Server) handleCreateBlock(w http.ResponseWriter, r *http.Request) {
	s.saveBlock(w, r, &store.Block{})
}

func (s *Server) loadBlock(w http.ResponseWriter, r *http.Request) (*store.Block, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, false
	}
	b, err := s.store.GetBlock(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "禁用策略不存在")
		return nil, false
	}
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	return b, true
}

// PUT /api/admin/hub/blocks/{id}：整条替换。
func (s *Server) handlePutBlock(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBlock(w, r)
	if !ok {
		return
	}
	s.saveBlock(w, r, b)
}

// DELETE /api/admin/hub/blocks/{id}
func (s *Server) handleDeleteBlock(w http.ResponseWriter, r *http.Request) {
	b, ok := s.loadBlock(w, r)
	if !ok {
		return
	}
	msg, err := s.checkBlockAssets(r.Context(), viewerOf(r), nil, b.AssetIDs)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusForbidden, msg)
		return
	}
	if err := s.store.DeleteBlock(r.Context(), b.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditBlockEdit, "block:"+strconv.FormatInt(b.ID, 10), map[string]any{"id": b.ID, "op": "delete", "asset_ids": b.AssetIDs})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// ---- 权限查询 ----

type effectiveItem struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Form  string `json:"form,omitempty"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// enabled：在他的 DSH 里生效 / available：能用但要自己启用 / hidden：用不了
	Result string `json:"result"`
	// hidden 的原因：unpublished / not_live / out_of_scope / category / blocked
	Reason string `json:"reason,omitempty"`
	// reason 为 blocked 时，挡住他的策略
	BlockIDs []int64 `json:"block_ids,omitempty"`
}

// GET /api/admin/hub/effective?user=<id>：某个员工每项能力的最终结果，以及被哪一步挡住（§5.2 的顺序）。
// 只列出调用者能管理的那几类能力。
func (s *Server) handleEffective(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, err := strconv.ParseInt(r.URL.Query().Get("user"), 10, 64)
	if err != nil || uid <= 0 {
		writeErr(w, http.StatusBadRequest, "需要 user 参数（用户 id）")
		return
	}
	u, err := s.store.GetUser(ctx, uid)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	access, err := s.store.UserAccess(ctx, uid)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	me := viewerOf(r)
	target := viewer{user: u, access: access, account: access}
	assets, err := s.store.ListAssets(ctx, hubKinds...)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	subs, err := s.store.Subscriptions(ctx, uid)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	blocks, err := s.store.ListBlocks(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var ids []int64
	for _, a := range assets {
		if me.manages(a.Kind) && a.Kind == store.KindApp && a.LiveVersion > 0 {
			ids = append(ids, a.ID)
		}
	}
	live, err := s.store.LiveAssetVersions(ctx, ids)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []effectiveItem{}
	for _, a := range assets {
		if !me.manages(a.Kind) {
			continue
		}
		it := effectiveItem{ID: a.ID, Kind: a.Kind, Slug: a.Slug, Title: a.Title}
		if lv := live[a.ID]; lv != nil {
			it.Form = parseSpec(lv.Spec).Form
		}
		it.Reason = unavailableReason(target, a)
		switch {
		case it.Reason != "":
			it.Result = "hidden"
		case a.Delivery == store.DeliveryAuto || subs[a.ID]:
			it.Result = "enabled"
		default:
			it.Result = "available"
		}
		if it.Reason == store.GateBlocked {
			for _, b := range blocks {
				if slices.Contains(b.AssetIDs, a.ID) && b.Scope.Allows(u.ID, u.Department, access.RoleIDs) {
					it.BlockIDs = append(it.BlockIDs, b.ID)
				}
			}
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":  map[string]any{"id": u.ID, "username": u.Username, "display_name": u.DisplayName, "department": u.Department},
		"items": out,
	})
}

func init() {
	// 和 /api/dsh/apps/{name}/subscription 同理：只给接入插件的界面用，不做成 AI 工具
	agentExcluded["POST /api/hub/skill-package"] = "导入要选本机的技能包文件，在网页上做；解析结果只用来预填编辑器"
	for _, what := range []string{"skills", "connections"} {
		agentExcluded["POST /api/dsh/"+what+"/{name}/subscription"] = "员工在接入插件界面上亲手启用，不让模型被提示注入诱导去改员工的配置"
		agentExcluded["DELETE /api/dsh/"+what+"/{name}/subscription"] = "员工在接入插件界面上亲手停用，不让模型被提示注入诱导去改员工的配置"
	}
}

// 路由与权限点：管连接或管能力的人都能进来，具体到哪一类在处理函数里按 viewer.manages 再判断。
var expertCenterPerms = []string{rbac.PluginsManage, rbac.ConnectionsManage}
