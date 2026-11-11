package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 给员工本机 DSH 用的接口：技能目录（/api/dsh/…）与企业连接的 MCP 网关（/mcp）。
// 用接入令牌（ee_npm_…，接入指南里生成）或 API key（sk-ee-…）认证。

// dshPrincipal 识别 DSH 带来的令牌，并按入口检查 hub / connections scope。
func (s *Server) dshPrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "企业账号登录已失效，在对话框输入 /ee-login 重新登录", "code": tokenInvalid})
		return nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		var be bearerError
		if errors.As(err, &be) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "企业账号登录已失效，在对话框输入 /ee-login 重新登录", "code": string(be)})
		} else {
			s.internalErr(w, r, err)
		}
		return nil, false
	}
	required := rbac.ScopeHub
	if r.URL.Path == "/mcp" {
		required = rbac.ScopeConnections
	}
	if !p.Scopes.Has(required) || !p.Access.Has(rbac.PortalView) {
		writeErr(w, http.StatusForbidden, "你的账号没有使用工作台的权限")
		return nil, false
	}
	return p, true
}

func (s *Server) dshAuth(w http.ResponseWriter, r *http.Request) (viewer, bool) {
	v, _, ok := s.dshViewer(w, r)
	return v, ok
}

// dshViewer 同 dshAuth，并返回主体（Agent 工具要用）。
func (s *Server) dshViewer(w http.ResponseWriter, r *http.Request) (viewer, *Principal, bool) {
	p, ok := s.dshPrincipal(w, r)
	if !ok {
		return viewer{}, nil, false
	}
	return viewer{user: p.User, access: p.Access, agent: s.agentOn(r.Context(), p), canAdmin: p.CanAdmin, adminLive: p.AdminLive}, p, true
}

var serverNameRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// mcpServerName 是 DSH 里企业连接工具的命名空间（mcp__<名字>__…），取自企业主作用域，如 @acme → acme。
func (s *Server) mcpServerName(r *http.Request) string { return s.mcpServerNameCtx(r.Context()) }

func (s *Server) mcpServerNameCtx(ctx context.Context) string {
	set, err := s.appSettings(ctx)
	if err != nil {
		return "ee"
	}
	name := serverNameRe.ReplaceAllString(strings.TrimPrefix(set.Scope, "@"), "-")
	name = strings.Trim(name, "-_")
	if name == "" || name == "company" {
		return "ee"
	}
	if len(name) > 24 {
		name = name[:24]
	}
	return name
}

// ---- 每个员工的能力目录 ----

type dshSkill struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	WhenToUse      string `json:"when_to_use,omitempty"`
	ModelInvocable bool   `json:"model_invocable"`
	UserInvocable  bool   `json:"user_invocable"`
	Version        int    `json:"version"`
}

type dshApp struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon,omitempty"`
	Starters    []string `json:"starters"`
	Launch      string   `json:"launch"`
	// auto（默认生效，不能停用）或 optional（员工自己启用的）
	Delivery string `json:"delivery"`
	Version  int    `json:"version"`
	AssetID  int64  `json:"asset_id"`
	// 依赖的技能、连接里，当前员工没有权限的，前端提示「部分能力不可用」
	Missing []string `json:"missing,omitempty"`
	// 右侧栏业务工作台；没有配置就不带。员工没权限的数据源由插件显示「无权限」，这里不删
	Workbench *appWorkbench `json:"workbench,omitempty"`
	// 开放了网页版（/a/<标识>）
	Web bool `json:"web,omitempty"`
	// 「它能帮你做什么」的三个要点
	Highlights []string `json:"highlights,omitempty"`
	// 最近 14 天上线（启动台显示「新」）
	New bool `json:"new,omitempty"`
	// 勾选上首页的组件数（启动台的「需要你处理」）
	Todo int `json:"todo,omitempty"`
}

type dshPlugin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// 为什么要装：插件本身设了自动安装，或某个微应用依赖它
	Reason string `json:"reason"`
}

type dshConn struct {
	asset   *store.Asset
	version int
	spec    *assetSpec
	tools   []*store.ConnectionTool
}

type dshCatalog struct {
	Skills []dshSkill `json:"skills"`
	// 员工可用的微应用，给接入插件画「企业应用」入口；旧插件忽略这个字段
	Apps []dshApp `json:"apps"`
	// 在可用范围内、需要员工自己启用、但还没启用的微应用；只有展示信息，/名字 加载不到
	AvailableApps []dshApp    `json:"available_apps"`
	Required      []dshPlugin `json:"required_plugins"`
	Revision      string      `json:"revision"`
	// 企业连接在 DSH 里的工具前缀（mcp__<server_name>__）
	ServerName string   `json:"server_name"`
	Brand      dshBrand `json:"brand"`

	assets map[string]*store.Asset
	specs  map[string]*assetSpec
	conns  []*dshConn
	// 系统技能的正文，按名字
	system map[string]string
	// 员工能查的知识库（kb__ 工具与「知识问答」）
	kbases []*store.KBase
	// dsh-ee 网页地址，用来给知识库引用拼原文链接；不知道时为空，不出链接
	base string
}

// userCatalog 算出某个员工的 DSH 此刻应有的技能、连接与必装插件：
// 已上架、没下架、在可用范围内，并且是「自动生效」或他自己启用了的。
func (s *Server) userCatalog(ctx context.Context, v viewer) (*dshCatalog, error) {
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := s.store.Subscriptions(ctx, v.user.ID)
	if err != nil {
		return nil, err
	}
	var on, offered []*store.Asset
	var ids []int64
	for _, a := range assets {
		switch {
		case !available(v, a):
		case a.Delivery == store.DeliveryAuto || subs[a.ID]:
			on = append(on, a)
			ids = append(ids, a.ID)
		case a.Kind == store.KindApp && a.Delivery == store.DeliveryOptional:
			offered = append(offered, a)
			ids = append(ids, a.ID)
		}
	}
	live, err := s.store.LiveAssetVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	c := &dshCatalog{Skills: []dshSkill{}, Apps: []dshApp{}, AvailableApps: []dshApp{}, Required: []dshPlugin{}, ServerName: s.mcpServerNameCtx(ctx),
		assets: map[string]*store.Asset{}, specs: map[string]*assetSpec{}}
	if c.kbases, err = s.readableKBases(ctx, v); err != nil {
		return nil, err
	}
	c.base, _ = s.connectBase(ctx, nil)
	var connIDs []int64
	for _, a := range on {
		lv := live[a.ID]
		if lv == nil {
			continue
		}
		sp := parseSpec(lv.Spec)
		c.assets[a.Slug], c.specs[a.Slug] = a, sp
		switch a.Kind {
		case store.KindSkill:
			model := sp.ModelInvocable == nil || *sp.ModelInvocable
			c.Skills = append(c.Skills, dshSkill{Name: a.Slug, Title: sp.Title, Kind: a.Kind, Description: sp.Description, WhenToUse: sp.WhenToUse, ModelInvocable: model, UserInvocable: true, Version: lv.Version})
		case store.KindPrompt, store.KindApp:
			c.Skills = append(c.Skills, dshSkill{Name: a.Slug, Title: sp.Title, Kind: a.Kind, Description: sp.Description, ModelInvocable: false, UserInvocable: true, Version: lv.Version})
			if a.Kind == store.KindApp {
				c.Apps = append(c.Apps, appEntry(a, sp, lv.Version, v.user))
			}
		case store.KindConnection:
			c.conns = append(c.conns, &dshConn{asset: a, version: lv.Version, spec: sp})
			connIDs = append(connIDs, a.ID)
		}
	}
	for i := range c.Apps {
		c.Apps[i].Missing = missingAppCapabilities(c.specs[c.Apps[i].Name], c)
	}
	// 可启用的微应用不进 assets / specs：没启用前 /名字 加载不到，也不算进必装插件
	for _, a := range offered {
		if lv := live[a.ID]; lv != nil {
			sp := parseSpec(lv.Spec)
			app := appEntry(a, sp, lv.Version, v.user)
			app.Missing = missingAppCapabilities(sp, c)
			c.AvailableApps = append(c.AvailableApps, app)
		}
	}
	if len(connIDs) > 0 {
		tools, err := s.store.ConnectionTools(ctx, connIDs...)
		if err != nil {
			return nil, err
		}
		byID := map[int64]*dshConn{}
		for _, cn := range c.conns {
			byID[cn.asset.ID] = cn
		}
		for _, t := range tools {
			if toolAllowed(v, t) {
				byID[t.AssetID].tools = append(byID[t.AssetID].tools, t)
			}
		}
	}

	// 必装插件：插件策略设了自动安装且在范围内的，加上启用中的微应用依赖的
	policies, err := s.store.PluginPolicies(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]string{}
	for name, p := range policies {
		if p.Delivery == store.DeliveryAuto && v.inScope(p.Scope) {
			want[name] = "policy"
		}
	}
	for slug, a := range c.assets {
		if a.Kind == store.KindApp {
			for _, p := range c.specs[slug].Plugins {
				if _, ok := want[p]; !ok {
					want[p] = "app:" + slug
				}
			}
		}
	}
	if len(want) > 0 {
		names := make([]string, 0, len(want))
		for n := range want {
			names = append(names, n)
		}
		pkgs, err := s.store.ListPackages(ctx, store.PackageFilter{Names: names})
		if err != nil {
			return nil, err
		}
		versions, err := s.store.VersionsOf(ctx, names)
		if err != nil {
			return nil, err
		}
		for _, p := range pkgs {
			if lv := visibleLatest(p, versions[p.Name], store.StatusPublished); lv != nil && p.IsPlugin {
				c.Required = append(c.Required, dshPlugin{Name: p.Name, Version: lv.Version, Reason: want[p.Name]})
			}
		}
	}
	// 系统技能：名字归系统，同名的员工能力让位
	c.system = map[string]string{}
	if v.agent {
		set, err := s.appSettings(ctx)
		if err != nil {
			return nil, err
		}
		for _, ss := range systemSkills(set, c.ServerName, v) {
			c.Skills = slices.DeleteFunc(c.Skills, func(k dshSkill) bool { return k.Name == ss.skill.Name })
			c.Skills = append(c.Skills, ss.skill)
			c.system[ss.skill.Name] = ss.body
		}
	}
	// 预置的「知识问答」：名字归系统，同名的员工能力让位
	set, err := s.appSettings(ctx)
	if err != nil {
		return nil, err
	}
	if app, skill, body := workbenchHomeApp(c, c.base); app != nil {
		c.Apps = slices.DeleteFunc(c.Apps, func(a dshApp) bool { return a.Name == workbenchHomeName })
		c.Skills = slices.DeleteFunc(c.Skills, func(k dshSkill) bool { return k.Name == workbenchHomeName })
		c.Apps = append([]dshApp{*app}, c.Apps...)
		c.Skills = append(c.Skills, *skill)
		c.system[workbenchHomeName] = body
	}
	if app, skill, body := kbQAApp(set, c.ServerName, c.base, c.kbases); app != nil {
		c.Skills = slices.DeleteFunc(c.Skills, func(k dshSkill) bool { return k.Name == kbQAName })
		c.Apps = slices.DeleteFunc(c.Apps, func(a dshApp) bool { return a.Name == kbQAName })
		c.AvailableApps = slices.DeleteFunc(c.AvailableApps, func(a dshApp) bool { return a.Name == kbQAName })
		delete(c.assets, kbQAName)
		c.Skills = append(c.Skills, *skill)
		c.Apps = append(c.Apps, *app)
		c.system[kbQAName] = body
	}
	sort.Slice(c.Skills, func(i, j int) bool { return c.Skills[i].Name < c.Skills[j].Name })
	sort.Slice(c.Apps, func(i, j int) bool {
		if c.Apps[i].Title == c.Apps[j].Title {
			return c.Apps[i].Name < c.Apps[j].Name
		}
		return c.Apps[i].Title < c.Apps[j].Title
	})
	sort.Slice(c.AvailableApps, func(i, j int) bool {
		if c.AvailableApps[i].Title == c.AvailableApps[j].Title {
			return c.AvailableApps[i].Name < c.AvailableApps[j].Name
		}
		return c.AvailableApps[i].Title < c.AvailableApps[j].Title
	})
	sort.Slice(c.Required, func(i, j int) bool { return c.Required[i].Name < c.Required[j].Name })
	sort.Slice(c.conns, func(i, j int) bool { return c.conns[i].asset.Slug < c.conns[j].asset.Slug })

	brand, err := s.brandInfo(ctx, "")
	if err != nil {
		return nil, err
	}
	c.Brand = brand
	// 修订号：目录里任何会影响 DSH 的东西变了，它就变
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(c.Skills)
	_ = json.NewEncoder(h).Encode(c.Apps)
	_ = json.NewEncoder(h).Encode(c.AvailableApps)
	_ = json.NewEncoder(h).Encode(c.Required)
	_ = json.NewEncoder(h).Encode(c.Brand)
	for _, name := range slices.Sorted(maps.Keys(c.system)) {
		fmt.Fprintf(h, "sys:%s:%s;", name, c.system[name])
	}
	// 能查的知识库变了，kb__ 工具会出现或消失
	fmt.Fprintf(h, "kb:%d;", len(c.kbases))
	for _, cn := range c.conns {
		fmt.Fprintf(h, "%s@%d:", cn.asset.Slug, cn.version)
		for _, t := range cn.tools {
			fmt.Fprintf(h, "%s,", t.Name)
		}
	}
	c.Revision = hex.EncodeToString(h.Sum(nil))[:16]
	return c, nil
}

// GET /api/dsh/whoami：/ee-status 显示当前账号、令牌范围与设备信息。
func (s *Server) handleDshWhoami(w http.ResponseWriter, r *http.Request) {
	p, ok := s.dshPrincipal(w, r)
	if !ok {
		return
	}
	scopes := make([]string, 0, len(p.Scopes))
	for scope := range p.Scopes {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	writeJSON(w, http.StatusOK, map[string]any{
		"username": p.User.Username, "department": p.User.Department, "scopes": scopes,
		"token_prefix": p.Prefix, "expires_at": p.ExpiresAt, "client_name": p.Client, "legacy": p.Legacy,
		"agent": s.agentOn(r.Context(), p), "admin_live": p.AdminLive, "admin_until": p.AdminUntil,
	})
}

// GET /api/dsh/catalog：DSH 接入插件轮询这个接口；带 If-None-Match 时目录没变返回 304。
func (s *Server) handleDshCatalog(w http.ResponseWriter, r *http.Request) {
	v, ok := s.dshAuth(w, r)
	if !ok {
		return
	}
	c, err := s.userCatalog(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	c.Brand, err = s.brandInfo(r.Context(), s.publicBase(r))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	etag := `"` + c.Revision + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// GET /api/dsh/skills/{name}：技能、提示词或微应用的正文。
func (s *Server) handleDshSkill(w http.ResponseWriter, r *http.Request) {
	v, ok := s.dshAuth(w, r)
	if !ok {
		return
	}
	c, err := s.userCatalog(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	name := r.PathValue("name")
	if body, ok := c.system[name]; ok {
		for _, sk := range c.Skills {
			if sk.Name == name {
				writeJSON(w, http.StatusOK, map[string]any{"skill": sk, "content": body})
				return
			}
		}
	}
	a, sp := c.assets[name], c.specs[name]
	if a == nil || a.Kind == store.KindConnection {
		writeErr(w, http.StatusNotFound, "没有这项技能，或它不在你的可用范围内")
		return
	}
	content := sp.Body
	if a.Kind == store.KindApp {
		content = appContent(sp, c) + appTablesContent(a.Slug, sp.Tables) + currentUserContent(v.user)
	}
	if err := s.store.RecordAssetUsage(context.WithoutCancel(r.Context()), a.ID, v.user.ID); err != nil {
		s.log.Warn("记录能力使用失败", "err", err)
	}
	var skill dshSkill
	for _, sk := range c.Skills {
		if sk.Name == name {
			skill = sk
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill": skill, "content": content})
}

// appEntry 是目录里一个微应用的展示信息。
// u 是当前员工：工作台数据源参数里的 {{me.name}} 等按他填。
func appEntry(a *store.Asset, sp *assetSpec, version int, u *store.User) dshApp {
	launch := sp.Launch
	if launch == "" {
		launch = "new"
	}
	starters := sp.Starters
	if starters == nil {
		starters = []string{}
	}
	wb := appWorkbenchForDSH(a.Slug, sp.Workbench, u)
	todo := 0
	if wb != nil {
		for _, p := range wb.Panes {
			for _, w := range p.Widgets {
				if w.Home {
					todo++
				}
			}
		}
	}
	return dshApp{Name: a.Slug, Title: sp.Title, Description: sp.Description, Icon: sp.Icon, Starters: starters, Launch: launch, Delivery: a.Delivery, Version: version, AssetID: a.ID, Workbench: wb, Web: webEnabled(sp), Highlights: sp.Highlights, New: a.CreatedAt.After(time.Now().AddDate(0, 0, -homeNewDays)), Todo: todo}
}

// POST / DELETE /api/dsh/apps/{name}/subscription：员工在 DSH 里启用或停用「自选」微应用。
// 与网页上的启用是同一件事（同一张订阅表、同样的范围检查）；只给接入插件的界面用，不做成 AI 工具，
// 免得模型被提示注入诱导去改员工的配置。令牌需要 hub 范围（dshPrincipal 统一检查）。
func (s *Server) handleDshAppSubscription(w http.ResponseWriter, r *http.Request) {
	v, ok := s.dshAuth(w, r)
	if !ok {
		return
	}
	a, err := s.store.GetAssetBySlug(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		a, err = nil, nil
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if a == nil || a.Kind != store.KindApp || !available(v, a) {
		writeErr(w, http.StatusNotFound, "没有这个微应用，或它不在你的可用范围内")
		return
	}
	if a.Delivery == store.DeliveryAuto {
		writeErr(w, http.StatusBadRequest, "这个微应用对你默认生效，不需要启用，也不能停用")
		return
	}
	if r.Method == http.MethodPost {
		err = s.store.Subscribe(r.Context(), v.user.ID, a.ID)
	} else {
		err = s.store.Unsubscribe(r.Context(), v.user.ID, a.ID)
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.hub.notify()
	c, err := s.userCatalog(r.Context(), v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": c.Revision, "enabled": r.Method == http.MethodPost})
}

func missingAppCapabilities(sp *assetSpec, c *dshCatalog) []string {
	var missing []string
	for _, slug := range append(append([]string{}, sp.Skills...), sp.Connections...) {
		if c.assets[slug] == nil {
			missing = append(missing, slug)
		}
	}
	for _, slug := range sp.Knowledge {
		if !slices.ContainsFunc(c.kbases, func(b *store.KBase) bool { return b.Slug == slug }) {
			missing = append(missing, slug)
		}
	}
	return missing
}

// appKBases 是微应用绑定的、这个员工能查的知识库。
func appKBases(sp *assetSpec, c *dshCatalog) []*store.KBase {
	var out []*store.KBase
	for _, b := range c.kbases {
		if slices.Contains(sp.Knowledge, b.Slug) {
			out = append(out, b)
		}
	}
	return out
}

// appContent 把微应用写成一段 /命令 指令：人设与工作方式，加上它能用的企业能力。
func appContent(sp *assetSpec, c *dshCatalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", sp.Title, sp.Persona)
	var lines []string
	for _, slug := range sp.Skills {
		if a := c.assets[slug]; a != nil && a.Kind == store.KindSkill {
			lines = append(lines, fmt.Sprintf("- 技能 `%s`（%s）：需要时用 skill 工具加载", slug, c.specs[slug].Title))
		}
	}
	for _, slug := range sp.Connections {
		if a := c.assets[slug]; a != nil && a.Kind == store.KindConnection {
			lines = append(lines, fmt.Sprintf("- 企业系统「%s」：工具名以 `mcp__%s__%s__` 开头", c.specs[slug].Title, c.ServerName, slug))
		}
	}
	for _, p := range sp.Plugins {
		lines = append(lines, fmt.Sprintf("- 插件 `%s`", p))
	}
	if len(lines) > 0 {
		b.WriteString("\n## 可用的企业能力\n\n")
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
	}
	if bases := appKBases(sp, c); len(bases) > 0 {
		board := false
		if sp.Workbench != nil {
			for _, p := range sp.Workbench.Panes {
				for _, w := range p.Widgets {
					board = board || w.Type == "board"
				}
			}
		}
		b.WriteString(kbRules(c.ServerName, c.base, bases, board))
	}
	if missing := missingAppCapabilities(sp, c); len(missing) > 0 {
		fmt.Fprintf(&b, "\n注意：这些能力不在当前用户的可用范围内，不要尝试调用：%s\n", strings.Join(missing, "、"))
	}
	if len(sp.Starters) > 0 {
		b.WriteString("\n## 用户可以这样问\n\n")
		for _, q := range sp.Starters {
			fmt.Fprintf(&b, "- %s\n", q)
		}
	}
	b.WriteString(workbenchContent(sp.Workbench))
	return b.String()
}

// ---- MCP 网关 ----

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, rpcErr *mcp.Error) {
	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		resp["error"] = rpcErr
	} else {
		resp["result"] = result
	}
	writeJSON(w, http.StatusOK, resp)
}

// mcpToolName 是连接工具在网关上的名字：<连接>__<工具>。
func mcpToolName(conn, tool string) string { return conn + "__" + tool }

// /mcp：POST 收 JSON-RPC；GET 打开事件流，目录变化时推 tools/list_changed；DELETE 结束会话。
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	case http.MethodGet:
		s.handleMCPStream(w, r)
		return
	case http.MethodDelete:
		w.WriteHeader(http.StatusOK)
		return
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		writeErr(w, http.StatusMethodNotAllowed, "不支持的方法")
		return
	}
	v, pr, ok := s.dshViewer(w, r)
	if !ok {
		return
	}
	var msg rpcMsg
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&msg); err != nil || msg.Method == "" {
		writeRPC(w, json.RawMessage("null"), nil, &mcp.Error{Code: -32700, Message: "不是合法的 JSON-RPC 请求"})
		return
	}
	// 通知与响应不需要回复
	if len(msg.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch msg.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		version := mcp.Versions[0]
		if slices.Contains(mcp.Versions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		c, err := s.userCatalog(r.Context(), v)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		w.Header().Set("Mcp-Session-Id", secret.Token())
		writeRPC(w, msg.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
			"serverInfo":      map[string]any{"name": "dsh-ee", "version": s.opts.Version},
			"instructions":    mcpInstructions(c, v.agent),
		}, nil)
	case "ping":
		writeRPC(w, msg.ID, map[string]any{}, nil)
	case "tools/list":
		c, err := s.userCatalog(r.Context(), v)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		tools := []map[string]any{}
		for _, cn := range c.conns {
			for _, t := range cn.tools {
				tools = append(tools, map[string]any{
					"name":        mcpToolName(cn.asset.Slug, t.Name),
					"description": "【" + cn.spec.Title + "】" + t.Description,
					"inputSchema": t.InputSchema,
					"annotations": map[string]any{"readOnlyHint": t.ReadOnly},
				})
			}
		}
		if len(c.kbases) > 0 {
			tools = append(tools, kbToolDefs()...)
		}
		if targets := s.appToolTargets(r.Context(), v, c); len(targets) > 0 {
			tools = append(tools, appToolDefs(targets)...)
		}
		if sets, err := s.visibleDataSets(r.Context(), v); err != nil {
			s.internalErr(w, r, err)
			return
		} else if len(sets) > 0 {
			tools = append(tools, dataToolDefs(sets)...)
		}
		tools = append(tools, homeToolDefs()...)
		for _, def := range s.agentToolList(r.Context(), pr) {
			tools = append(tools, def)
		}
		writeRPC(w, msg.ID, map[string]any{"tools": tools}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			writeRPC(w, msg.ID, nil, &mcp.Error{Code: -32602, Message: "参数格式不对"})
			return
		}
		if strings.HasPrefix(p.Name, "ee_") {
			writeRPC(w, msg.ID, s.agentCall(r, pr, p.Name, p.Arguments), nil)
			return
		}
		if strings.HasPrefix(p.Name, dataConn+"__") {
			writeRPC(w, msg.ID, s.dataCall(r.Context(), v, p.Name, p.Arguments), nil)
			return
		}
		if strings.HasPrefix(p.Name, appConn+"__") {
			writeRPC(w, msg.ID, s.appCall(r, v, pr, p.Name, p.Arguments), nil)
			return
		}
		if strings.HasPrefix(p.Name, homeConn+"__") {
			c, err := s.userCatalog(r.Context(), v)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			writeRPC(w, msg.ID, s.homeCall(r.Context(), v, c, p.Name), nil)
			return
		}
		if strings.HasPrefix(p.Name, kbConn+"__") {
			bases, err := s.readableKBases(r.Context(), v)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			writeRPC(w, msg.ID, s.kbCall(r.Context(), v, bases, p.Name, p.Arguments), nil)
			return
		}
		c, err := s.userCatalog(r.Context(), v)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		conn, tool, _ := strings.Cut(p.Name, "__")
		for _, cn := range c.conns {
			if cn.asset.Slug != conn {
				continue
			}
			for _, t := range cn.tools {
				if t.Name == tool {
					ctx := r.Context()
					writeRPC(w, msg.ID, s.callConnectionTool(ctx, cn.asset, t, v, s.callerBase(r), p.Arguments), nil)
					return
				}
			}
		}
		writeRPC(w, msg.ID, nil, &mcp.Error{Code: -32602, Message: "没有这个工具，或你没有使用它的权限：" + p.Name})
	case "resources/list":
		writeRPC(w, msg.ID, map[string]any{"resources": []any{}}, nil)
	case "resources/templates/list":
		writeRPC(w, msg.ID, map[string]any{"resourceTemplates": []any{}}, nil)
	case "prompts/list":
		writeRPC(w, msg.ID, map[string]any{"prompts": []any{}}, nil)
	default:
		writeRPC(w, msg.ID, nil, &mcp.Error{Code: -32601, Message: "不支持的方法：" + msg.Method})
	}
}

func mcpInstructions(c *dshCatalog, agent bool) string {
	var b strings.Builder
	if agent {
		b.WriteString("ee_* 工具用来操作企业工作台 dsh-ee 本身（额度、能力中心、插件、使用记录），用法见技能 dsh-ee-guide。\n")
	}
	if len(c.kbases) > 0 {
		b.WriteString(kbMCPInstructions())
	}
	if len(c.conns) == 0 {
		b.WriteString("当前账号没有可用的企业连接。")
		return b.String()
	}
	b.WriteString("这些工具连着企业内部系统，经 dsh-ee 按当前员工的权限转发，每次调用都会记录。工具名的第一段是系统名：\n")
	for _, cn := range c.conns {
		fmt.Fprintf(&b, "- %s：%s", cn.asset.Slug, cn.spec.Title)
		if cn.spec.Description != "" {
			fmt.Fprintf(&b, "。%s", cn.spec.Description)
		}
		if cn.spec.Instructions != "" {
			fmt.Fprintf(&b, "\n  %s", strings.ReplaceAll(cn.spec.Instructions, "\n", "\n  "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// handleMCPStream 打开服务端到 DSH 的事件流：能力目录变化时推 notifications/tools/list_changed，
// DSH 收到后重新 tools/list。每 25 秒发一行注释保活。
func (s *Server) handleMCPStream(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		w.Header().Set("Allow", "POST, DELETE")
		writeErr(w, http.StatusMethodNotAllowed, "GET 只用于事件流")
		return
	}
	if _, ok := s.dshAuth(w, r); !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "不支持流式响应")
		return
	}
	ch, cancel := s.hub.subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": dsh-ee\n\n")
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streamsDone:
			return
		case <-ping.C:
			_, _ = io.WriteString(w, ": ping\n\n")
		case <-ch:
			_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
		}
		flusher.Flush()
	}
}

// currentUserContent 是应用指令末尾的「当前员工」：人设里说「按当前员工的姓名查」「申请人填当前员工」时用得上。
func currentUserContent(u *store.User) string {
	if u == nil {
		return ""
	}
	dept := u.Department
	if dept == "" {
		dept = "未填"
	}
	return fmt.Sprintf("\n\n## 当前员工\n\n姓名：%s（用户名 %s），部门：%s。查「我的 …」、填申请人 / 报修人 / 拜访人时用这个名字，不要再问用户是谁。\n", displayOf(u), u.Username, dept)
}
