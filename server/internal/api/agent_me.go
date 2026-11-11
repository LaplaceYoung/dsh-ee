package api

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 员工自己的额度、使用记录、企业连接与接入诊断。网页和 Agent 工具（ee_me / ee_records / ee_connections / ee_doctor）共用。

func limitParam(r *http.Request, def, max int) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 {
		return def
	}
	return min(n, max)
}

// GET /api/me/usage：额度、本月用量，以及按天、按模型、按令牌的明细。
func (s *Server) handleMyUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(r)
	used, err := s.store.MonthTokensByUser(ctx, u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	days, err := s.store.UserUsageByDay(ctx, u.ID, time.Now().UTC().AddDate(0, 0, -30))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	models, err := s.store.UserUsageByModel(ctx, u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	tokens, err := s.store.UserUsageByToken(ctx, u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	resp := map[string]any{
		"quota": myQuota{
			MonthlyTokenLimit: u.MonthlyTokenLimit,
			MonthTokens:       used,
			AllowedModels:     nonNil(u.AllowedModels),
			ExpiresAt:         u.QuotaExpiresAt,
		},
		"by_day":   days,
		"by_model": models,
		"by_token": tokens,
	}
	if info, ok := agentFrom(ctx); ok && info.Principal.TokenID > 0 {
		tokenUsed, err := s.store.TokenMonthUsage(ctx, info.Principal.TokenID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		resp["current_token"] = map[string]any{"name": info.Principal.Client, "prefix": info.Principal.Prefix, "monthly_token_limit": info.Principal.Limit, "month_tokens": tokenUsed}
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/me/calls/model?limit=
func (s *Server) handleMyModelCalls(w http.ResponseWriter, r *http.Request) {
	calls, err := s.store.UserModelCalls(r.Context(), currentUser(r).ID, limitParam(r, 20, 100))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"calls": calls})
}

// GET /api/me/calls/connection?limit=
func (s *Server) handleMyConnectionCalls(w http.ResponseWriter, r *http.Request) {
	calls, err := s.store.UserConnectionCalls(r.Context(), currentUser(r).ID, limitParam(r, 20, 100))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"calls": calls})
}

type myConnection struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// 在我的 DSH 里生效（自动生效，或我启用了）
	Enabled bool `json:"enabled"`
	// 我能用的工具
	Tools      []string `json:"tools"`
	ToolsTotal int      `json:"tools_total"`
	// 这个连接按员工本人的身份调用时：用哪种方式，我的凭据填了没有（没填时工具会提示先去填）
	Credential *myCredential `json:"credential,omitempty"`
}

// GET /api/me/connections：我能用的企业连接与工具，以及还没权限、可以申请的连接。
func (s *Server) handleMyConnections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := viewerOf(r)
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	subs, err := s.store.Subscriptions(ctx, v.user.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var ids []int64
	for _, a := range assets {
		if a.Kind == store.KindConnection {
			ids = append(ids, a.ID)
		}
	}
	tools := map[int64][]*store.ConnectionTool{}
	if len(ids) > 0 {
		all, err := s.store.ConnectionTools(ctx, ids...)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		for _, t := range all {
			tools[t.AssetID] = append(tools[t.AssetID], t)
		}
	}
	creds, err := s.store.UserCredentialInfos(ctx, v.user.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var onIDs []int64
	for _, a := range assets {
		if a.Kind == store.KindConnection && a.Status == store.AssetActive && a.LiveVersion > 0 && available(v, a) {
			onIDs = append(onIDs, a.ID)
		}
	}
	live, err := s.store.LiveAssetVersions(ctx, onIDs)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	mine, requestable := []myConnection{}, []map[string]string{}
	for _, a := range assets {
		if a.Kind != store.KindConnection || a.Status != store.AssetActive || a.LiveVersion == 0 {
			continue
		}
		if !available(v, a) {
			requestable = append(requestable, map[string]string{"slug": a.Slug, "title": a.Title, "description": a.Description})
			continue
		}
		c := myConnection{ID: a.ID, Slug: a.Slug, Title: a.Title, Description: a.Description, Enabled: a.Delivery == store.DeliveryAuto || subs[a.ID], Tools: []string{}, ToolsTotal: len(tools[a.ID])}
		for _, t := range tools[a.ID] {
			if toolAllowed(v, t) {
				c.Tools = append(c.Tools, t.Name)
			}
		}
		if lv := live[a.ID]; lv != nil {
			c.Credential = credentialView(parseSpec(lv.Spec).UserAuth, creds, a.ID)
		}
		mine = append(mine, c)
	}
	all, err := s.store.UserAccessRequests(ctx, v.user.ID, 50)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	reqs := []*store.AccessRequest{}
	for _, q := range all {
		if q.Kind == store.RequestConnection && len(reqs) < 20 {
			reqs = append(reqs, q)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": mine, "requestable": requestable, "requests": reqs, "server_name": s.mcpServerName(r)})
}

type connectionRequestReq struct {
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// POST /api/me/connection-requests：申请使用某个企业连接。申请记在审计日志里，管理员在能力中心为他开放范围。
func (s *Server) handleCreateConnectionRequest(w http.ResponseWriter, r *http.Request) {
	var req connectionRequestReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Slug, req.Reason = strings.TrimSpace(req.Slug), strings.TrimSpace(req.Reason)
	if req.Reason == "" || !limitRunes(req.Reason, 500) {
		writeErr(w, http.StatusBadRequest, "请写明用途（最多 500 个字符），管理员据此决定是否开放")
		return
	}
	ctx := r.Context()
	v := viewerOf(r)
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var target *store.Asset
	for _, a := range assets {
		if a.Kind == store.KindConnection && a.Slug == req.Slug && a.Status == store.AssetActive && a.LiveVersion > 0 {
			target = a
		}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, "没有找到企业连接 "+req.Slug+"，可以先用 ee_connections 的 mine 查看有哪些")
		return
	}
	if available(v, target) {
		writeErr(w, http.StatusBadRequest, "你已经可以使用这个连接了，不需要申请")
		return
	}
	id, existed, err := s.store.CreateAccessRequest(ctx, v.user.ID, store.RequestConnection, target.ID, 0, req.Reason)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !existed {
		s.audit(r, auditConnRequest, target.Slug, map[string]any{"id": id, "reason": req.Reason})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending", "already_pending": existed, "connection": target.Slug})
}

type doctorCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"` // ok / warn / fail
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// GET /api/me/doctor：服务端能看到的接入状态。本机的部分（凭据、.npmrc、已装插件）由接入插件的 ee_local_doctor 检查。
func (s *Server) handleMyDoctor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, a := currentUser(r), currentAccess(r)
	set, err := s.appSettings(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var checks []doctorCheck
	add := func(id, status, title, detail, fix string) {
		checks = append(checks, doctorCheck{ID: id, Status: status, Title: title, Detail: detail, Fix: fix})
	}

	if !a.Has(rbac.LLMUse) {
		add("account", "fail", "账号没有调用大模型的权限", "", "联系管理员开通「调用大模型」权限")
	} else {
		add("account", "ok", "账号 "+u.Username+" 可以调用大模型", "", "")
	}

	used, err := s.store.MonthTokensByUser(ctx, u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	switch {
	case u.QuotaExpiresAt != nil && !u.QuotaExpiresAt.After(time.Now()):
		add("quota", "fail", "额度已过期", u.QuotaExpiresAt.Format("2006-01-02"), "联系管理员续期")
	case u.MonthlyTokenLimit == nil:
		add("quota", "fail", "管理员还没有给你分配额度", "", "联系管理员分配每月额度")
	case used >= *u.MonthlyTokenLimit:
		add("quota", "fail", "本月额度已用完", strconv.FormatInt(used, 10)+" / "+strconv.FormatInt(*u.MonthlyTokenLimit, 10), "联系管理员追加额度，或等下个月重置")
	case used*10 >= *u.MonthlyTokenLimit*9:
		add("quota", "warn", "本月额度已用 90% 以上", strconv.FormatInt(used, 10)+" / "+strconv.FormatInt(*u.MonthlyTokenLimit, 10), "")
	default:
		add("quota", "ok", "额度充足", strconv.FormatInt(used, 10)+" / "+strconv.FormatInt(*u.MonthlyTokenLimit, 10), "")
	}

	models, err := s.store.PublicModels(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var allowed []string
	for _, m := range models {
		if len(u.AllowedModels) == 0 || slices.Contains(u.AllowedModels, m.Name) {
			allowed = append(allowed, m.Name)
		}
	}
	switch {
	case len(allowed) == 0:
		add("models", "fail", "没有可用的模型", "", "联系管理员上架模型，或放开你的可用模型")
	case set.Connect.DefaultModel != "" && !slices.Contains(allowed, set.Connect.DefaultModel):
		add("models", "warn", "企业默认模型 "+set.Connect.DefaultModel+" 你用不了", "可用："+strings.Join(allowed, "、"), "在 DSH 里改选可用的模型，或联系管理员")
	default:
		add("models", "ok", "可用模型 "+strings.Join(allowed, "、"), "", "")
	}

	if !set.Connect.Enabled {
		add("connect", "fail", "管理员关闭了 DSH 接入", "", "联系管理员")
	}
	catalog, err := s.userCatalog(ctx, viewerOf(r))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	conns := 0
	for _, c := range catalog.conns {
		if len(c.tools) > 0 {
			conns++
		}
	}
	add("catalog", "ok", "DSH 里生效 "+strconv.Itoa(len(catalog.Skills))+" 项技能 / 提示词 / 微应用，"+strconv.Itoa(conns)+" 个企业连接", "", "")

	resp := map[string]any{
		"checks":           checks,
		"default_model":    set.Connect.DefaultModel,
		"required_plugins": catalog.Required,
		"server_name":      catalog.ServerName,
		"registry":         set.Scope,
	}
	var st connectState
	if _, err := s.store.GetSettingJSON(ctx, connectStateKey, &st); err == nil && st.Package == connectPackage(set) {
		resp["connect_package"], resp["connect_version"] = st.Package, st.Version
	}
	if info, ok := agentFrom(ctx); ok {
		resp["token"] = map[string]any{"prefix": info.Principal.Prefix, "client": info.Principal.Client, "expires_at": info.Principal.ExpiresAt}
	}
	writeJSON(w, http.StatusOK, resp)
}
