package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 应用的依赖与权限申请（第一版 M7，docs/product-v1-tasks.md）。
//
// 应用详情列出它依赖的技能、连接、知识库，标出当前员工能不能用；缺权限的一键申请，写明是为了哪个应用。
// 审批人：连接看「管理企业连接」，技能看「管理能力与审核」，知识库看它的维护者（没有维护者时是知识库管理员）。
// 通过 = 把申请人加进对象的可用范围（自选的顺手替他启用），DSH 随 hub 通知刷新工具，不用重启。

// appDep 是应用依赖的一项，以及当前员工的状态。
type appDep struct {
	Kind  string `json:"kind"` // skill / connection / knowledge / plugin
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// 能打开详情时给 id（能力或知识库）
	ID int64 `json:"id,omitempty"`
	// ok 能用 / off 能用但还没启用 / missing 没权限，可以申请 / partial 连接能用但有的工具要特定角色 / gone 不存在或已下架
	Status string `json:"status"`
	// 连接：我能用的工具数 / 启用的工具数
	Tools      int `json:"tools,omitempty"`
	ToolsTotal int `json:"tools_total,omitempty"`
	// 最近一次申请：待处理的，或者被驳回的（还缺权限时）
	Request *store.AccessRequest `json:"request,omitempty"`
}

// appDeps 算出应用依赖的状态。
func (s *Server) appDeps(ctx context.Context, v viewer, sp *assetSpec) ([]appDep, error) {
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := s.store.Subscriptions(ctx, v.user.ID)
	if err != nil {
		return nil, err
	}
	reqs, err := s.store.UserAccessRequests(ctx, v.user.ID, 200)
	if err != nil {
		return nil, err
	}
	lastReq := func(kind string, id int64) *store.AccessRequest {
		for _, r := range reqs { // 新的在前
			if r.Kind == kind && r.AssetID == id && r.Status != "approved" {
				return r
			}
			if r.Kind == kind && r.AssetID == id {
				return nil
			}
		}
		return nil
	}
	bySlug := map[string]*store.Asset{}
	for _, a := range assets {
		bySlug[a.Kind+":"+a.Slug] = a
	}
	out := []appDep{}
	asset := func(kind, slug string) {
		d := appDep{Kind: kind, Slug: slug, Title: slug, Status: "gone"}
		a := bySlug[kind+":"+slug]
		if a != nil && a.Status == store.AssetActive && a.LiveVersion > 0 {
			d.Title = a.Title
			switch {
			case !available(v, a):
				d.Status = "missing"
				d.Request = lastReq(kind, a.ID)
				if v.manages(kind) {
					d.ID = a.ID // 管理员不在范围里也能打开详情去改
				}
			case a.Delivery == store.DeliveryAuto || subs[a.ID]:
				d.Status, d.ID = "ok", a.ID
			default:
				d.Status, d.ID = "off", a.ID
			}
		}
		if d.Status == "ok" && kind == store.KindConnection {
			tools, err := s.store.ConnectionTools(ctx, a.ID)
			if err == nil {
				for _, t := range tools {
					if !t.Enabled {
						continue
					}
					d.ToolsTotal++
					if toolAllowed(v, t) {
						d.Tools++
					}
				}
				if d.Tools < d.ToolsTotal {
					d.Status = "partial"
				}
			}
		}
		out = append(out, d)
	}
	for _, slug := range sp.Skills {
		asset(store.KindSkill, slug)
	}
	for _, slug := range sp.Connections {
		asset(store.KindConnection, slug)
	}
	for _, slug := range sp.Knowledge {
		d := appDep{Kind: store.RequestKnowledge, Slug: slug, Title: slug, Status: "gone"}
		b, err := s.store.GetKBaseBySlug(ctx, slug)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if b != nil {
			d.Title = b.Title
			if v.kbReads(b) {
				d.Status, d.ID = "ok", b.ID
			} else {
				d.Status = "missing"
				d.Request = lastReq(store.RequestKnowledge, b.ID)
			}
		}
		out = append(out, d)
	}
	for _, p := range sp.Plugins {
		out = append(out, appDep{Kind: "plugin", Slug: p, Title: p, Status: "ok"})
	}
	return out, nil
}

// accessTarget 是申请的对象：能力或知识库。
type accessTarget struct {
	kind        string
	id          int64
	slug, title string
	description string
	asset       *store.Asset
	kb          *store.KBase
}

// findAccessTarget 按类型和标识找到可以申请的对象（已上架的连接、技能；存在的知识库）。
func (s *Server) findAccessTarget(ctx context.Context, kind, slug string) (*accessTarget, error) {
	switch kind {
	case store.RequestConnection, store.RequestSkill:
		a, err := s.store.GetAssetBySlug(ctx, slug)
		if errors.Is(err, store.ErrNotFound) || (err == nil && (a.Kind != kind || a.Status != store.AssetActive)) {
			return nil, appErr(http.StatusNotFound, "没有找到「%s」，可能已经下架", slug)
		}
		if err != nil {
			return nil, err
		}
		live, err := s.store.LiveAssetVersions(ctx, []int64{a.ID})
		if err != nil {
			return nil, err
		}
		if live[a.ID] == nil {
			return nil, appErr(http.StatusNotFound, "「%s」还没有上线", a.Title)
		}
		return &accessTarget{kind: kind, id: a.ID, slug: a.Slug, title: a.Title, description: a.Description, asset: a}, nil
	case store.RequestKnowledge:
		b, err := s.store.GetKBaseBySlug(ctx, slug)
		if errors.Is(err, store.ErrNotFound) {
			return nil, appErr(http.StatusNotFound, "没有找到知识库「%s」", slug)
		}
		if err != nil {
			return nil, err
		}
		return &accessTarget{kind: kind, id: b.ID, slug: b.Slug, title: b.Title, description: b.Description, kb: b}, nil
	}
	return nil, appErr(http.StatusBadRequest, "kind 只能是 connection / skill / knowledge")
}

func (t *accessTarget) allows(v viewer) bool {
	if t.kb != nil {
		return v.kbReads(t.kb)
	}
	return available(v, t.asset)
}

// GET /api/me/access-target?kind=&slug=：申请页上显示的对象名称与说明，以及我现在能不能用、有没有在审的申请。
// 没权限的人也能看到名称和说明（与 ee_connections 的可申请列表一致），看不到内容。
func (s *Server) handleAccessTarget(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	ctx := r.Context()
	t, err := s.findAccessTarget(ctx, r.URL.Query().Get("kind"), strings.TrimSpace(r.URL.Query().Get("slug")))
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	resp := map[string]any{"kind": t.kind, "slug": t.slug, "title": t.title, "description": t.description, "allowed": t.allows(v)}
	if t.allows(v) {
		resp["id"] = t.id
	}
	reqs, err := s.store.UserAccessRequests(ctx, v.user.ID, 200)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	for _, q := range reqs {
		if q.Kind == t.kind && q.AssetID == t.id {
			resp["request"] = q
			break
		}
	}
	if app := strings.TrimSpace(r.URL.Query().Get("app")); app != "" {
		if a, err := s.store.GetAssetBySlug(ctx, app); err == nil && a.Kind == store.KindApp && (available(v, a) || v.owns(a)) {
			resp["app"] = map[string]any{"slug": a.Slug, "title": a.Title}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type accessRequestReq struct {
	Kind   string `json:"kind"`
	Slug   string `json:"slug"`
	App    string `json:"app"`
	Reason string `json:"reason"`
}

// POST /api/me/access-requests {kind, slug, app?, reason?}：申请用某个连接、技能或知识库。
// 写明了应用时理由可以不填（默认「为了使用应用 X」）。
func (s *Server) handleCreateAccessRequest(w http.ResponseWriter, r *http.Request) {
	var req accessRequestReq
	if !readJSON(w, r, &req) {
		return
	}
	v := viewerOf(r)
	ctx := r.Context()
	t, err := s.findAccessTarget(ctx, req.Kind, strings.TrimSpace(req.Slug))
	if err != nil {
		s.writeAppErr(w, r, err)
		return
	}
	var app *store.Asset
	if slug := strings.TrimSpace(req.App); slug != "" {
		a, err := s.store.GetAssetBySlug(ctx, slug)
		if err == nil && a.Kind == store.KindApp && (available(v, a) || v.owns(a)) {
			app = a
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" && app != nil {
		reason = "为了使用应用「" + app.Title + "」"
	}
	if reason == "" || !limitRunes(reason, 500) {
		writeErr(w, http.StatusBadRequest, "请写明用途（最多 500 个字符），审批人据此决定是否开放")
		return
	}
	if t.allows(v) {
		writeErr(w, http.StatusBadRequest, "你已经可以使用「"+t.title+"」了，不需要申请")
		return
	}
	var appID int64
	if app != nil {
		appID = app.ID
	}
	id, existed, err := s.store.CreateAccessRequest(ctx, v.user.ID, t.kind, t.id, appID, reason)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !existed {
		s.feishuAccessRequest(ctx, &store.AccessRequest{ID: id, Kind: t.kind, UserID: v.user.ID, Username: v.user.Username, AssetID: t.id, Slug: t.slug, Title: t.title, Reason: reason, AppID: appID})
	}
	if !existed {
		action, target := auditAccessRequest, t.kind+":"+t.slug
		if t.kind == store.RequestConnection {
			action, target = auditConnRequest, t.slug
		}
		detail := map[string]any{"id": id, "kind": t.kind, "reason": reason}
		if app != nil {
			detail["app"] = app.Slug
		}
		s.audit(r, action, target, detail)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending", "already_pending": existed, "kind": t.kind, "slug": t.slug, "title": t.title})
}

// canApprove 判断这个人能不能处理这条申请。
func (s *Server) canApprove(ctx context.Context, v viewer, q *store.AccessRequest) (bool, error) {
	switch q.Kind {
	case store.RequestConnection:
		return v.manages(store.KindConnection), nil
	case store.RequestSkill:
		return v.manages(store.KindSkill), nil
	case store.RequestKnowledge:
		if v.kbManages() {
			return true, nil
		}
		b, err := s.store.GetKBase(ctx, q.AssetID)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return v.kbMaintains(b), nil
	}
	return false, nil
}

// reviewableRequests 是这个人能处理的待处理申请。
func (s *Server) reviewableRequests(ctx context.Context, v viewer, kinds ...string) ([]*store.AccessRequest, error) {
	all, err := s.store.PendingAccessRequests(ctx, 500, kinds...)
	if err != nil {
		return nil, err
	}
	out := []*store.AccessRequest{}
	for _, q := range all {
		ok, err := s.canApprove(ctx, v, q)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, q)
		}
	}
	return out, nil
}

// GET /api/me/access-requests：我的申请，以及待我审批的（员工视角合并成一个列表，管理员原来的审核队列不变）。
func (s *Server) handleMyAccessRequests(w http.ResponseWriter, r *http.Request) {
	v := viewerOf(r)
	ctx := r.Context()
	mine, err := s.store.UserAccessRequests(ctx, v.user.ID, 50)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	review, err := s.reviewableRequests(ctx, v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mine": mine, "to_review": review})
}

type resolveAccessReq struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

// POST /api/me/access-requests/{id} {decision: approve|reject, note}：审批人处理一条申请。
func (s *Server) handleResolveAccessRequest(w http.ResponseWriter, r *http.Request) {
	s.resolveAccessRequest(w, r, "")
}

// resolveAccessRequest 处理申请；onlyKind 非空时只处理这一类（管理员的连接申请接口）。
func (s *Server) resolveAccessRequest(w http.ResponseWriter, r *http.Request, onlyKind string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req resolveAccessReq
	if !readJSON(w, r, &req) {
		return
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		writeErr(w, http.StatusBadRequest, "decision 只能是 approve 或 reject")
		return
	}
	v := viewerOf(r)
	ctx := r.Context()
	q, err := s.store.GetPendingAccessRequest(ctx, id)
	if err == nil && onlyKind != "" && q.Kind != onlyKind {
		err = store.ErrNotFound
	}
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "申请不存在或已经处理过")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if ok, err := s.canApprove(ctx, v, q); err != nil {
		s.internalErr(w, r, err)
		return
	} else if !ok {
		writeErr(w, http.StatusForbidden, "你不是这条申请的审批人")
		return
	}
	// 先 CAS 抢占状态、再授权：两个审批人并发处理同一条申请（网页 approve 与飞书 reject 完全可能同时发生）时，
	// 只有把 pending 改成终态的那一方可以去授权。反过来做会留下「申请已驳回、权限却授出去了」的脏数据。
	status := "rejected"
	if req.Decision == "approve" {
		status = "approved"
	}
	if err := s.store.ResolveAccessRequest(ctx, id, status, v.user.ID, req.Note); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusConflict, "这条申请刚被别人处理了")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	// 抢占成功后授权；授权失败要把申请退回待处理，不能让「记录已通过、权限没授出去」静默发生。
	if req.Decision == "approve" {
		if err := s.grantAccess(ctx, q); err != nil {
			if rerr := s.store.ReopenAccessRequest(ctx, id); rerr != nil {
				s.log.Error("授权失败后退回申请失败", "id", id, "err", rerr)
			}
			s.log.Error("授权失败", "id", id, "err", err)
			var ae *appDataError
			if errors.As(err, &ae) {
				writeErr(w, http.StatusInternalServerError, "授权失败（"+ae.msg+"），申请已退回待处理，可以重试")
			} else {
				writeErr(w, http.StatusInternalServerError, "授权失败，申请已退回待处理，可以重试")
			}
			return
		}
	}
	action, target := "access.request_"+req.Decision, q.Kind+":"+q.Slug
	if q.Kind == store.RequestConnection {
		action, target = "connection.request_"+req.Decision, q.Slug
	}
	s.notifyFeishuResult(ownerPtr(q.UserID), q.Title, req.Decision)
	s.audit(r, action, target, map[string]any{"id": id, "user": q.Username, "app": q.AppSlug, "note": truncate(req.Note, 500)})
	s.hub.notify()
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "user": q.Username, "kind": q.Kind, "slug": q.Slug, "connection": q.Slug})
}

// grantAccess 把申请人加进对象的可用范围；「自选」的能力顺手替他启用，这样应用马上能用上。
func (s *Server) grantAccess(ctx context.Context, q *store.AccessRequest) error {
	if q.Kind == store.RequestKnowledge {
		b, err := s.store.GetKBase(ctx, q.AssetID)
		if errors.Is(err, store.ErrNotFound) {
			return appErr(http.StatusNotFound, "这个知识库已经删除了")
		}
		if err != nil {
			return err
		}
		if b.Scope.All || slices.Contains(b.Scope.Users, q.UserID) {
			return nil
		}
		sc := b.Scope
		sc.Users = append(append([]int64{}, sc.Users...), q.UserID)
		sc = sc.Normalize()
		return s.store.UpdateKBase(ctx, b.ID, store.KBasePatch{Scope: &sc})
	}
	a, err := s.store.GetAsset(ctx, q.AssetID)
	if errors.Is(err, store.ErrNotFound) {
		return appErr(http.StatusNotFound, "这项能力已经删除了")
	}
	if err != nil {
		return err
	}
	if !a.Scope.All && !slices.Contains(a.Scope.Users, q.UserID) {
		sc := a.Scope
		sc.Users = append(append([]int64{}, sc.Users...), q.UserID)
		sc = sc.Normalize()
		if err := s.store.UpdateAsset(ctx, a.ID, store.AssetPatch{Scope: &sc}); err != nil {
			return err
		}
	}
	if a.Delivery == store.DeliveryOptional {
		return s.store.Subscribe(ctx, q.UserID, a.ID)
	}
	return nil
}
