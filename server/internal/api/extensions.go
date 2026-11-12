package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 声明式扩展（docs/plugins.md §4.1、§9.4 第 1 批）：管理员上传清单 + 接口文档，dsh-ee 检查之后
// 装成一个 OpenAPI 连接。清单在调用时生效：工具说明、哪些是只读、不由模型填的参数、
// 返回只保留哪些字段、请求只能发往清单列出的地址。每次调用记下扩展的版本和摘要。

const auditExtInstall = "extension.install"

// connExtension 存在连接的版本内容里。清单原文要和接口文档一起算摘要，所以存字符串，不存解析后的 JSON
// （json.RawMessage 序列化时会被压缩，摘要就对不上了）。
type connExtension struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
	Manifest string `json:"manifest"`
	// 管理员统一设置的参数值（fill: admin），键是参数名（请求体字段去掉 body.）
	Admin map[string]json.RawMessage `json:"admin,omitempty"`
}

// label 是记进调用记录和审计的版本：作者/名字@版本#摘要前 12 位。
func (e *connExtension) label() string {
	if e == nil {
		return ""
	}
	d := e.Digest
	if len(d) > 12 {
		d = d[:12]
	}
	return e.ID + "@" + e.Version + "#" + d
}

func (e *connExtension) manifest() (*extension.Manifest, error) {
	return extension.ParseManifest([]byte(e.Manifest))
}

// validateExtension 检查扩展装的连接：清单和接口文档要和装的时候一模一样（摘要对得上），
// 请求根地址要在清单列出的地址之内。改接口文档、换清单要装新版本，不能在连接页面上单独改。
func validateExtension(sp *assetSpec) string {
	e := sp.Extension
	if e == nil {
		return ""
	}
	if sp.Type != "openapi" {
		return "只有 OpenAPI 连接能来自声明式扩展"
	}
	if extension.Digest([]byte(e.Manifest), []byte(sp.OpenAPI)) != e.Digest {
		return "这个连接来自扩展 " + e.ID + "，清单和接口文档不能在这里单独改；要改请装扩展的新版本"
	}
	m, err := e.manifest()
	if err != nil {
		return err.Error()
	}
	if m.ID != e.ID || m.Version != e.Version || m.Kind != extension.KindDeclarative {
		return "扩展的身份和清单对不上，请重新安装"
	}
	if sp.URL != "" && !slices.ContainsFunc(m.Servers, func(s string) bool { return extension.UnderServer(sp.URL, s) }) {
		return "接口根地址要在扩展清单列出的地址之内：" + strings.Join(m.Servers, "、")
	}
	// 用谁的身份由清单决定，连接页面上改不了；只有 shared 模式的共享凭据放在哪里由管理员选
	sp.Auth, sp.UserAuth = extCredential(m.Credential, sp.Auth)
	if sp.UserAuth != nil {
		if msg := validateUserAuth(sp); msg != "" {
			return msg
		}
	}
	keys := map[string]bool{}
	for _, p := range m.AdminParams() {
		keys[p.Key] = true
	}
	for k, v := range e.Admin {
		if !keys[k] {
			delete(e.Admin, k)
			continue
		}
		if !json.Valid(v) {
			return "参数 " + k + " 的设置值不是合法的 JSON"
		}
	}
	if len(e.Admin) == 0 {
		e.Admin = nil
	}
	return ""
}

// applyExtension 把清单叠加到接口文档生成的工具上。只读以清单为准：没标 readOnly 的一律当写操作，
// 默认不启用；GET 接口标了 readOnly 才算只读（和 `extension check` 的判断是同一份代码）。
func applyExtension(sp *assetSpec, tools []*store.ConnectionTool) error {
	m, err := sp.Extension.manifest()
	if err != nil {
		return err
	}
	res := extension.CheckFiles(map[string][]byte{extension.ManifestFile: []byte(sp.Extension.Manifest), m.OpenAPI: []byte(sp.OpenAPI)})
	ro := map[string]bool{}
	for _, ti := range res.Tools {
		ro[ti.Name] = ti.ReadOnly
	}
	for _, t := range tools {
		t.ReadOnly = ro[t.Name]
		t.Enabled = t.ReadOnly
		mt := m.Tool(t.Name)
		if mt == nil {
			continue
		}
		if d := mt.Description.Any(); d != "" {
			t.Description = d
		}
		schema, err := mt.Schema(t.InputSchema)
		if err != nil {
			return fmt.Errorf("工具 %s 的参数定义：%w", t.Name, err)
		}
		t.InputSchema = schema
	}
	return nil
}

// prepare 在发请求之前做清单要求的事：根地址必须在清单列出的地址之内；补上不由模型填的参数。
// 返回这个工具要保留的返回字段。
func (e *connExtension) prepare(root, tool string, args map[string]any) ([]string, error) {
	m, err := e.manifest()
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(m.Servers, func(s string) bool { return extension.UnderServer(root, s) }) {
		return nil, fmt.Errorf("请求地址 %s 不在扩展 %s 允许的地址之内，已拦下", root, e.ID)
	}
	mt := m.Tool(tool)
	if mt == nil {
		return nil, nil
	}
	if err := mt.Fix(args, e.Admin); err != nil {
		return nil, err
	}
	return mt.Keep, nil
}

// ---- 管理接口 ----

// extPackage 是上传的扩展：一个 zip 包（base64），或几个文件（相对路径 → 文本内容）。
type extPackage struct {
	Zip   string            `json:"zip"`
	Files map[string]string `json:"files"`
}

func (p *extPackage) read() (map[string][]byte, error) {
	if p.Zip != "" {
		b, err := base64.StdEncoding.DecodeString(p.Zip)
		if err != nil {
			return nil, errors.New("zip 不是合法的 base64")
		}
		return extension.ReadZip(b)
	}
	if len(p.Files) == 0 {
		return nil, errors.New("请选择扩展的 zip 包，或清单和接口文档两个文件")
	}
	out := map[string][]byte{}
	for name, text := range p.Files {
		out[strings.TrimPrefix(name, "./")] = []byte(text)
	}
	return out, nil
}

// extPlan 是安装前给管理员看的：检查结果、要设置的参数、装成哪个连接、升级时变了什么。
type extPlan struct {
	Check       *extension.Result      `json:"check"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	AdminParams []extension.AdminParam `json:"admin_params"`
	// 建议的连接标识（新装时）
	Slug string `json:"slug"`
	// 已经装过这个扩展：升级哪个连接，原来是哪个版本
	Existing *extExisting `json:"existing,omitempty"`
	// 升级时变了的地方；sensitive 里的要管理员勾选确认才能装
	Changes   []string `json:"changes"`
	Sensitive []string `json:"sensitive"`
	// 原来就在用的参数值，升级时预先填上
	Admin map[string]json.RawMessage `json:"admin,omitempty"`
}

type extExisting struct {
	ID      int64  `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
}

// findExtension 找已经用这个扩展装过的连接。
func (s *Server) findExtension(ctx context.Context, id string) (*store.Asset, *assetSpec, error) {
	conns, err := s.store.ListAssets(ctx, store.KindConnection)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(conns))
	for _, a := range conns {
		ids = append(ids, a.ID)
	}
	live, err := s.store.LiveAssetVersions(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range conns {
		if lv := live[a.ID]; lv != nil {
			if sp := parseSpec(lv.Spec); sp.Extension != nil && sp.Extension.ID == id {
				return a, sp, nil
			}
		}
	}
	return nil, nil, nil
}

// plan 检查扩展并算出安装计划。检查不通过时 plan.Check.OK 为 false，其余字段尽量给全。
func (s *Server) planExtension(ctx context.Context, files map[string][]byte) (*extPlan, error) {
	res := extension.CheckFiles(files)
	p := &extPlan{Check: res, AdminParams: []extension.AdminParam{}, Changes: []string{}, Sensitive: []string{}}
	m := res.Manifest
	if m == nil {
		return p, nil
	}
	if m.Kind != extension.KindDeclarative && res.OK {
		res.OK = false
		res.Errors++
		res.Results = append([]extension.Diagnostic{{Code: "kind.unsupported", Severity: extension.SevError, Where: extension.ManifestFile + "#/kind",
			Message: "这个版本的 dsh-ee 只能安装声明式扩展", Fix: "服务扩展、端侧扩展在后面的版本支持；现有的 MCP 服务可以直接在企业连接里接入"}}, res.Results...)
	}
	p.Name, p.Description = m.Name.Any(), m.Description.Any()
	if p.AdminParams = m.AdminParams(); p.AdminParams == nil {
		p.AdminParams = []extension.AdminParam{}
	}
	if _, name, ok := strings.Cut(m.ID, "/"); ok {
		p.Slug = name
	}
	old, osp, err := s.findExtension(ctx, m.ID)
	if err != nil || old == nil {
		return p, err
	}
	p.Existing = &extExisting{ID: old.ID, Slug: old.Slug, Title: old.Title, Version: osp.Extension.Version, URL: osp.URL}
	p.Slug = old.Slug
	p.Admin = osp.Extension.Admin
	om, err := osp.Extension.manifest()
	if err != nil {
		return p, nil
	}
	if om.Version == m.Version && osp.Extension.Digest == res.Digest {
		p.Changes = append(p.Changes, "和已装的版本完全一样")
		return p, nil
	}
	if om.Version == m.Version {
		p.Sensitive = append(p.Sensitive, "版本号没变（"+m.Version+"），但清单或接口文档的内容变了")
	}
	oldRes := extension.CheckFiles(map[string][]byte{extension.ManifestFile: []byte(osp.Extension.Manifest), om.OpenAPI: []byte(osp.OpenAPI)})
	oldRO := map[string]bool{}
	for _, t := range oldRes.Tools {
		oldRO[t.Name] = t.ReadOnly
	}
	for _, t := range res.Tools {
		was, existed := oldRO[t.Name]
		switch {
		case !existed && t.ReadOnly:
			p.Changes = append(p.Changes, "新增只读工具 "+t.Name)
		case !existed:
			p.Changes = append(p.Changes, "新增写操作工具 "+t.Name+"（装好后默认不启用）")
		case was && !t.ReadOnly:
			p.Changes = append(p.Changes, "工具 "+t.Name+" 从只读变成了写操作（装好后先停用，确认后再打开）")
		}
		delete(oldRO, t.Name)
	}
	for name := range oldRO {
		p.Changes = append(p.Changes, "去掉了工具 "+name)
	}
	if credModeOf(om.Credential) != credModeOf(m.Credential) {
		p.Sensitive = append(p.Sensitive, fmt.Sprintf("凭据模式从 %s 变成了 %s", credModeOf(om.Credential), credModeOf(m.Credential)))
	}
	for _, sv := range m.Servers {
		if !slices.Contains(om.Servers, sv) {
			p.Sensitive = append(p.Sensitive, "请求可以发往新的地址 "+sv)
		}
	}
	slices.Sort(p.Changes)
	return p, nil
}

func credModeOf(c *extension.Credential) string {
	if c == nil {
		return "（不带凭据）"
	}
	return c.Mode
}

// POST /api/admin/extensions/check：检查上传的扩展，返回安装计划。不改任何东西。
func (s *Server) handleExtCheck(w http.ResponseWriter, r *http.Request) {
	var req extPackage
	if !readJSONLimit(w, r, &req, 12<<20) {
		return
	}
	files, err := req.read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.planExtension(r.Context(), files)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type extInstallReq struct {
	extPackage
	// 新装时的连接标识；升级时忽略（沿用原来的）
	Slug string `json:"slug"`
	// 接口根地址：空则用接口文档里的 servers；要在清单列出的地址之内
	URL string `json:"url"`
	// shared 模式的共享凭据放在哪里（清单里 credential.shared 写了的，以清单为准）
	Auth   *connAuth                  `json:"auth"`
	Secret *string                    `json:"secret"`
	Admin  map[string]json.RawMessage `json:"admin"`
	Scope  *store.Scope               `json:"scope"`
	// 升级时有要确认的变化（凭据模式、新地址），要传 true
	Confirm bool `json:"confirm"`
}

// POST /api/admin/extensions/install：检查通过后装成连接；装过的就升级成新版本。
func (s *Server) handleExtInstall(w http.ResponseWriter, r *http.Request) {
	var req extInstallReq
	if !readJSONLimit(w, r, &req, 12<<20) {
		return
	}
	files, err := req.read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	p, err := s.planExtension(ctx, files)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !p.Check.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "扩展没有通过检查，改好后再装", "plan": p})
		return
	}
	if len(p.Sensitive) > 0 && !req.Confirm {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "这次升级有需要确认的变化：" + strings.Join(p.Sensitive, "；"), "plan": p})
		return
	}
	m := p.Check.Manifest
	sp := &assetSpec{
		Title:       truncateStr(m.Name.Any(), 64),
		Description: truncateStr(m.Description.Any(), 1000),
		Type:        "openapi",
		URL:         strings.TrimSpace(req.URL),
		OpenAPI:     string(p.Check.OpenAPIDoc),
		Extension: &connExtension{
			ID: m.ID, Version: m.Version, Digest: p.Check.Digest,
			Manifest: string(files[extension.ManifestFile]), Admin: req.Admin,
		},
	}
	if sp.URL == "" && p.Existing != nil {
		sp.URL = p.Existing.URL
	}
	if sp.URL == "" && len(m.Servers) == 1 {
		sp.URL = m.Servers[0]
	}
	sp.Auth, sp.UserAuth = extCredential(m.Credential, req.Auth)
	if p.Existing != nil {
		// 升级：沿用原来的共享凭据放法（清单没规定时）、超时和使用说明
		if _, osp, err := s.findExtension(ctx, m.ID); err == nil && osp != nil {
			if m.Credential == nil || (m.Credential.Mode == extension.CredShared && req.Auth == nil) {
				sp.Auth = osp.Auth
			}
			sp.TimeoutSec, sp.Instructions = osp.TimeoutSec, osp.Instructions
			if req.Admin == nil {
				sp.Extension.Admin = osp.Extension.Admin
			}
		}
	}
	if msg := validateSpec(store.KindConnection, sp); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	for _, ap := range p.AdminParams {
		if _, ok := sp.Extension.Admin[ap.Key]; !ok && len(ap.Default) == 0 {
			writeErr(w, http.StatusBadRequest, "参数「"+ap.Label+"」要由管理员设置，请填上")
			return
		}
	}
	raw, _ := json.Marshal(sp)
	v := viewerOf(r)
	var a *store.Asset
	resp := map[string]any{"identity": m.Identity(), "digest": p.Check.Digest}
	if p.Existing != nil {
		if a, err = s.store.GetAsset(ctx, p.Existing.ID); err != nil {
			s.internalErr(w, r, err)
			return
		}
		ver, err := s.store.AddAssetVersion(ctx, a.ID, raw, store.StatusPublished, v.user.ID)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if err := s.store.UpdateAsset(ctx, a.ID, store.AssetPatch{Title: &sp.Title, Description: &sp.Description}); err != nil {
			s.internalErr(w, r, err)
			return
		}
		resp["version"], resp["upgraded_from"] = ver, p.Existing.Version
	} else {
		slug := strings.TrimSpace(req.Slug)
		if slug == "" {
			slug = p.Slug
		}
		if msg := connSlugProblem(slug); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		scope := store.Scope{All: true}
		if req.Scope != nil {
			scope = req.Scope.Normalize()
		}
		owner := v.user.ID
		a = &store.Asset{Kind: store.KindConnection, Slug: slug, Title: sp.Title, Description: sp.Description, OwnerID: &owner, Scope: scope, Delivery: store.DeliveryOptional}
		if err := s.store.CreateAsset(ctx, a, raw, store.StatusPublished, v.user.ID); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeErr(w, http.StatusConflict, "连接标识 "+slug+" 已被占用，换一个")
				return
			}
			s.internalErr(w, r, err)
			return
		}
		_ = s.store.Subscribe(ctx, v.user.ID, a.ID)
	}
	if req.Secret != nil {
		if err := s.saveConnSecret(r, a.ID, *req.Secret); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	s.dropConnClient(a.ID)
	if n, err := s.syncConnectionTools(ctx, a.ID, v, s.callerBase(r)); err != nil {
		resp["sync_error"] = err.Error()
	} else {
		resp["tools"] = n
	}
	resp["asset"] = a
	s.audit(r, auditExtInstall, "connection:"+a.Slug, map[string]any{
		"id": a.ID, "extension": sp.Extension.label(), "upgrade": p.Existing != nil, "confirmed": p.Sensitive,
	})
	s.hub.notify()
	writeJSON(w, http.StatusCreated, resp)
}

// extCredential 把清单的凭据模式换成连接的共享凭据和员工身份设置。
func extCredential(c *extension.Credential, shared *connAuth) (*connAuth, *connUserAuth) {
	none := &connAuth{Type: "none"}
	if c == nil {
		return none, nil
	}
	if c.Mode == extension.CredShared {
		if shared == nil {
			shared = &connAuth{Type: "bearer"}
		}
		return shared, nil
	}
	auth := none
	if c.Shared != nil {
		auth = &connAuth{Type: c.Shared.Type, Header: c.Shared.Header}
	}
	return auth, &connUserAuth{Mode: c.Mode, Header: c.Header, Audience: c.Audience, Claims: c.Claims, Hint: c.Hint.Any()}
}

// connSlugProblem 检查新连接的标识，和「新建连接」页面的规则一样。
func connSlugProblem(slug string) string {
	switch {
	case strings.HasPrefix(slug, systemSkillPrefix):
		return "标识不能以 " + systemSkillPrefix + " 开头，这是系统技能的前缀"
	case !slugRe.MatchString(slug) || len(slug) > maxSlug:
		return "标识只能用小写字母、数字和连字符，例如 oa，最多 48 个字符"
	case slug == kbConn || slug == kbQAName:
		return "标识 " + slug + " 留给知识库使用"
	case slug == appConn || slug == dataConn:
		return "标识 " + slug + " 留给应用数据和数据中台使用"
	}
	return ""
}
