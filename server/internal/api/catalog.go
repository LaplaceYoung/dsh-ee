package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 能力市场、审核、npm 源管理、上游预载的 JSON 接口。

// catalogItem 是能力列表里的一项（前端 Plugin 类型）。
type catalogItem struct {
	Name             string     `json:"name"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Author           string     `json:"author"`
	Icon             string     `json:"icon"`
	Homepage         string     `json:"homepage"`
	Repository       string     `json:"repository"`
	Category         string     `json:"category"`
	Kind             string     `json:"kind"`
	Featured         bool       `json:"featured"`
	IsPlugin         bool       `json:"is_plugin"`
	Origin           string     `json:"origin"` // internal / upstream
	Stars            *int       `json:"stars"`
	Score            *int       `json:"score"`
	Version          string     `json:"version"`
	PublishedAt      *time.Time `json:"published_at"`
	Installs         int        `json:"installs"`
	DepCount         *int       `json:"dep_count"`
	Size             int64      `json:"size"`
	ClosureSize      *int64     `json:"closure_size"`
	OfflineReady     *bool      `json:"offline_ready"`
	HasInstallScript bool       `json:"has_install_script"`
	Status           string     `json:"status"`
	Pending          int        `json:"pending"`
	Owner            string     `json:"owner"`
	OwnerName        string     `json:"owner_name"`
	Department       string     `json:"department"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// versionItem 是版本列表里的一项。
type versionItem struct {
	*store.NpmVersion
	Publisher string `json:"publisher"`
	Reviewer  string `json:"reviewer"`
}

// packageStatus 由各版本状态得出包的状态：有上架版本就是已上架，否则看有没有待审 / 下架的。
func packageStatus(vs []*store.NpmVersion) string {
	has := map[string]bool{}
	for _, v := range vs {
		has[v.Status] = true
	}
	for _, st := range []string{store.StatusPublished, store.StatusPending, store.StatusUnpublished} {
		if has[st] {
			return st
		}
	}
	return store.StatusRejected
}

// visibleLatest 是员工看到的版本：latest 标签指向的已上架版本，否则最高的已上架版本。
func visibleLatest(p *store.NpmPackage, vs []*store.NpmVersion, statuses ...string) *store.NpmVersion {
	var pick []*store.NpmVersion
	for _, v := range vs {
		if len(statuses) == 0 || slices.Contains(statuses, v.Status) {
			pick = append(pick, v)
		}
	}
	if len(pick) == 0 {
		return nil
	}
	if tag := p.DistTags["latest"]; tag != "" {
		for _, v := range pick {
			if v.Version == tag {
				return v
			}
		}
	}
	names := make([]string, len(pick))
	for i, v := range pick {
		names[i] = v.Version
	}
	registry.SortVersions(names)
	want := names[len(names)-1]
	for i := len(names) - 1; i >= 0; i-- {
		if !strings.Contains(names[i], "-") {
			want = names[i]
			break
		}
	}
	for _, v := range pick {
		if v.Version == want {
			return v
		}
	}
	return pick[len(pick)-1]
}

type userIndex map[int64]*store.User

func (s *Server) userIndex(r *http.Request) (userIndex, error) {
	us, err := s.store.ListUsers(r.Context())
	if err != nil {
		return nil, err
	}
	out := userIndex{}
	for _, u := range us {
		out[u.ID] = u
	}
	return out, nil
}

func (ui userIndex) name(id *int64) string {
	if id == nil || ui[*id] == nil {
		return ""
	}
	return ui[*id].Username
}

// catalog 列出能力；admin 为 false 时只返回有已上架版本的。
func (s *Server) catalog(r *http.Request, admin bool) ([]*catalogItem, map[string][]*store.NpmVersion, error) {
	pkgs, err := s.store.ListPackages(r.Context(), store.PackageFilter{CatalogOnly: true})
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	versions, err := s.store.VersionsOf(r.Context(), names)
	if err != nil {
		return nil, nil, err
	}
	installs, err := s.store.InstallCounts(r.Context(), names)
	if err != nil {
		return nil, nil, err
	}
	users, err := s.userIndex(r)
	if err != nil {
		return nil, nil, err
	}
	out := []*catalogItem{}
	for _, p := range pkgs {
		vs := versions[p.Name]
		latest := visibleLatest(p, vs, store.StatusPublished)
		if latest == nil && !admin {
			continue
		}
		if latest == nil {
			latest = visibleLatest(p, vs)
		}
		if latest == nil {
			continue
		}
		item := catalogItemOf(p, latest, users)
		item.Installs = installs[p.Name]
		item.Status = packageStatus(vs)
		for _, v := range vs {
			if v.Status == store.StatusPending {
				item.Pending++
			}
		}
		out = append(out, item)
	}
	return out, versions, nil
}

func catalogItemOf(p *store.NpmPackage, v *store.NpmVersion, users userIndex) *catalogItem {
	item := &catalogItem{
		Name: p.Name, Title: p.Title, Description: p.Description, Author: p.Author, Icon: p.Icon,
		Homepage: p.Homepage, Repository: p.Repository, Category: p.Category, Kind: p.Kind, Featured: p.Featured,
		IsPlugin: p.IsPlugin, Origin: "upstream", Stars: p.UpstreamStars, Score: p.UpstreamScore,
		Version: v.Version, PublishedAt: v.PublishedAt, DepCount: p.DepCount, Size: v.Size, ClosureSize: p.ClosureSize,
		OfflineReady: p.OfflineReady, HasInstallScript: v.HasInstallScript, UpdatedAt: p.UpdatedAt,
	}
	if item.Title == "" {
		item.Title = p.Name
	}
	if p.Origin == store.OriginHosted {
		item.Origin = "internal"
		if u := users[derefID(p.OwnerID)]; u != nil {
			item.Owner, item.OwnerName, item.Department = u.Username, u.DisplayName, u.Department
		}
		if item.Author == "" {
			item.Author = item.Owner
		}
	}
	return item
}

func derefID(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// GET /api/catalog：能力市场（员工可见的已上架能力）。
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.catalog(r, false)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": items})
}

// GET /api/admin/packages：能力管理（全部状态）。
func (s *Server) handleAdminPackages(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.catalog(r, true)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": items})
}

// packageName 取 /…/{name...} 里的包名；前端用 encodeURIComponent，所以也接受 %2F。
func packageName(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if n, err := url.PathUnescape(name); err == nil {
		name = n
	}
	if !registry.ValidName(name) {
		writeErr(w, http.StatusBadRequest, "非法的包名")
		return "", false
	}
	return name, true
}

type packageDetail struct {
	*catalogItem
	Versions   []versionItem     `json:"versions"`
	DistTags   map[string]string `json:"dist_tags"`
	Scan       json.RawMessage   `json:"scan"`
	Readme     string            `json:"readme"`
	Engines    map[string]string `json:"engines"`
	Peers      map[string]string `json:"peer_dependencies"`
	Deps       map[string]string `json:"dependencies"`
	Deprecated *string           `json:"deprecated"`
}

func (s *Server) packageDetail(r *http.Request, name string, admin bool) (*packageDetail, error) {
	p, err := s.store.GetPackage(r.Context(), name)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListVersions(r.Context(), name)
	if err != nil {
		return nil, err
	}
	users, err := s.userIndex(r)
	if err != nil {
		return nil, err
	}
	var shown []*store.NpmVersion
	for _, v := range all {
		if admin || v.Status == store.StatusPublished {
			shown = append(shown, v)
		}
	}
	latest := visibleLatest(p, shown, store.StatusPublished)
	if latest == nil && admin {
		latest = visibleLatest(p, shown)
	}
	if latest == nil {
		return nil, store.ErrNotFound
	}
	installs, err := s.store.InstallCounts(r.Context(), []string{name})
	if err != nil {
		return nil, err
	}
	item := catalogItemOf(p, latest, users)
	item.Installs = installs[name]
	item.Status = packageStatus(all)
	d := &packageDetail{catalogItem: item, Versions: []versionItem{}, DistTags: p.DistTags, Scan: latest.Scan, Deprecated: latest.Deprecated}
	for i := len(shown) - 1; i >= 0; i-- {
		v := shown[i]
		d.Versions = append(d.Versions, versionItem{NpmVersion: v, Publisher: users.name(v.PublisherID), Reviewer: users.name(v.ReviewedBy)})
		if v.Status == store.StatusPending {
			item.Pending++
		}
	}
	m, _ := registry.ParseManifest(latest.Manifest)
	if m != nil {
		d.Engines, d.Peers, d.Deps = m.Engines, m.PeerDependencies, m.Dependencies
	}
	var raw struct {
		Readme string `json:"readme"`
	}
	_ = json.Unmarshal(latest.Manifest, &raw)
	if len(raw.Readme) > 64<<10 {
		raw.Readme = raw.Readme[:64<<10]
	}
	d.Readme = raw.Readme
	return d, nil
}

// GET /api/catalog/{name...}
func (s *Server) handleCatalogDetail(w http.ResponseWriter, r *http.Request) {
	name, ok := packageName(w, r)
	if !ok {
		return
	}
	p, err := s.store.GetPackage(r.Context(), name)
	if err == nil && !p.IsPlugin && p.Origin != store.OriginHosted {
		err = store.ErrNotFound
	}
	if err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "能力不存在或尚未上架")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	d, err := s.packageDetail(r, name, false)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "能力不存在或尚未上架")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// GET /api/admin/packages/{name...}
func (s *Server) handleAdminPackageDetail(w http.ResponseWriter, r *http.Request) {
	name, ok := packageName(w, r)
	if !ok {
		return
	}
	d, err := s.packageDetail(r, name, true)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "包不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type catalogPatchReq struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Category    *string `json:"category"`
	Kind        *string `json:"kind"`
	Icon        *string `json:"icon"`
	Featured    *bool   `json:"featured"`
}

// PATCH /api/admin/packages/{name...}：修改展示信息。
func (s *Server) handlePatchPackage(w http.ResponseWriter, r *http.Request) {
	name, ok := packageName(w, r)
	if !ok {
		return
	}
	var req catalogPatchReq
	if !readJSON(w, r, &req) {
		return
	}
	if req.Category != nil && *req.Category != "" && !slices.Contains(registry.Categories, *req.Category) {
		writeErr(w, http.StatusBadRequest, "未知的分类")
		return
	}
	if req.Kind != nil && *req.Kind != "" && !slices.Contains(registry.Kinds, *req.Kind) {
		writeErr(w, http.StatusBadRequest, "未知的能力形态")
		return
	}
	if req.Icon != nil && *req.Icon != "" {
		if u, err := url.Parse(*req.Icon); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			writeErr(w, http.StatusBadRequest, "图标必须是 http(s) 地址")
			return
		}
	}
	for _, f := range []*string{req.Title, req.Description} {
		if f != nil && len([]rune(*f)) > 1000 {
			writeErr(w, http.StatusBadRequest, "内容过长")
			return
		}
	}
	err := s.store.UpdatePackageCatalog(r.Context(), name, store.CatalogPatch{
		Title: req.Title, Description: req.Description, Category: req.Category, Kind: req.Kind, Icon: req.Icon, Featured: req.Featured,
	})
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "包不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	changed := map[string]any{}
	for k, v := range map[string]any{"title": req.Title, "description": req.Description, "category": req.Category, "kind": req.Kind, "icon": req.Icon} {
		if p, ok := v.(*string); ok && p != nil {
			changed[k] = *p
		}
	}
	if req.Featured != nil {
		changed["featured"] = *req.Featured
	}
	s.audit(r, auditPackageEdit, name, changed)
	s.handleAdminPackageDetail(w, r)
}

// ---- 审核 ----

type reviewItem struct {
	*store.NpmVersion
	Package    *catalogItem `json:"package"`
	Publisher  string       `json:"publisher"`
	PubName    string       `json:"publisher_name"`
	Department string       `json:"department"`
	// 当前已上架的版本，审核新版本时对照用
	Previous string            `json:"previous"`
	Engines  map[string]string `json:"engines"`
	Deps     map[string]string `json:"dependencies"`
	Peers    map[string]string `json:"peer_dependencies"`
	Scripts  map[string]string `json:"scripts"`
}

// GET /api/admin/review：待审核的版本。
func (s *Server) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	pending, err := s.store.PendingVersions(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	users, err := s.userIndex(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []reviewItem{}
	for _, pv := range pending {
		p, err := s.store.GetPackage(r.Context(), pv.Name)
		if err != nil {
			continue
		}
		full, err := s.store.GetVersion(r.Context(), pv.Name, pv.Version)
		if err != nil {
			continue
		}
		vs, err := s.store.ListVersions(r.Context(), pv.Name, store.StatusPublished)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		item := reviewItem{NpmVersion: pv, Package: catalogItemOf(p, pv, users)}
		if prev := visibleLatest(p, vs, store.StatusPublished); prev != nil {
			item.Previous = prev.Version
		}
		if u := users[derefID(pv.PublisherID)]; u != nil {
			item.Publisher, item.PubName, item.Department = u.Username, u.DisplayName, u.Department
		}
		if m, err := registry.ParseManifest(full.Manifest); err == nil {
			item.Engines, item.Deps, item.Peers, item.Scripts = m.Engines, m.Dependencies, m.PeerDependencies, m.Scripts
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type reviewReq struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Action  string `json:"action"`
	Note    string `json:"note"`
}

// POST /api/admin/review：通过 / 驳回 / 下架 / 恢复。version 为空时作用于整个包（下架、恢复）。
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req reviewReq
	if !readJSON(w, r, &req) {
		return
	}
	if !registry.ValidName(req.Name) {
		writeErr(w, http.StatusBadRequest, "非法的包名")
		return
	}
	n, err := s.registry.Review(r.Context(), currentUser(r).ID, req.Name, req.Version, req.Action, strings.TrimSpace(req.Note))
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	s.log.Info("review", "pkg", req.Name, "version", req.Version, "action", req.Action, "by", currentUser(r).Username, "count", n)
	target := req.Name
	if req.Version != "" {
		target += "@" + req.Version
	}
	detail := map[string]any{"count": n}
	if note := strings.TrimSpace(req.Note); note != "" {
		detail["note"] = truncate(note, 500)
	}
	switch req.Action {
	case "approve":
		s.audit(r, auditReviewApprove, target, detail)
	case "reject":
		s.audit(r, auditReviewReject, target, detail)
	default:
		detail["action"] = req.Action
		s.audit(r, auditPackageStatus, target, detail)
	}
	s.feishuNpmResultFromWeb(r.Context(), req.Name, req.Version, req.Action)
	writeJSON(w, http.StatusOK, map[string]any{"changed": n})
}

// GET /api/admin/tarball?name=&version=：下载任意状态的 tarball（审核时查看内容）。
func (s *Server) handleAdminTarball(w http.ResponseWriter, r *http.Request) {
	name, version := r.URL.Query().Get("name"), r.URL.Query().Get("version")
	v, err := s.store.GetVersion(r.Context(), name, version)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "版本不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	f, err := s.registry.Open(v)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	defer f.Close()
	file := registry.TarballFile(name, version)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file}))
	_, _ = io.Copy(w, f)
}

// ---- 员工：网页发布、我的发布、申请引入 ----

// POST /api/me/packages：网页上传 .tgz 发布。请求体是 tarball 本身（Content-Type: application/octet-stream，
// 浏览器跨站表单发不出这种请求，配合 SameSite cookie 防 CSRF）。
func (s *Server) handleUploadPackage(w http.ResponseWriter, r *http.Request) {
	raw, info, m, ok := s.readPackageTarball(w, r)
	if !ok {
		return
	}
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	doc := &registry.PublishDoc{
		Name:     m.Name,
		DistTags: map[string]string{tag: m.Version},
		Versions: map[string]json.RawMessage{m.Version: info.PackageJSON},
	}
	doc.Attachments = map[string]struct {
		ContentType string `json:"content_type"`
		Data        string `json:"data"`
		Length      int64  `json:"length"`
	}{registry.TarballFile(m.Name, m.Version): {ContentType: "application/octet-stream", Data: base64.StdEncoding.EncodeToString(raw), Length: int64(len(raw))}}
	u, a := currentUser(r), currentAccess(r)
	res, err := s.registry.Publish(r.Context(), registry.Publisher{UserID: u.ID, Username: u.Username, Manager: a.Has(rbac.PluginsManage)}, doc)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	s.audit(r, auditPackagePublish, res.Name+"@"+res.Version, map[string]any{"via": "web", "status": res.Status})
	s.feishuNpmPublish(r, res, u.ID)
	writeJSON(w, http.StatusCreated, res)
}

// GET /api/me/packages：我发布的包与每个版本的审核状态。
func (s *Server) handleMyPackages(w http.ResponseWriter, r *http.Request) {
	me := currentUser(r).ID
	items, versions, err := s.catalog(r, true)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type mine struct {
		*catalogItem
		Versions []*store.NpmVersion `json:"versions"`
	}
	out := []mine{}
	for _, it := range items {
		var vs []*store.NpmVersion
		for _, v := range versions[it.Name] {
			if derefID(v.PublisherID) == me {
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			continue
		}
		slices.Reverse(vs)
		out = append(out, mine{catalogItem: it, Versions: vs})
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": out})
}

type importRequestReq struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

// POST /api/me/import-requests：员工申请引入社区插件。插件连同依赖镜像进来，插件本身进入待审核。
func (s *Server) handleImportRequest(w http.ResponseWriter, r *http.Request) {
	var req importRequestReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !registry.ValidName(req.Name) {
		writeErr(w, http.StatusBadRequest, "包名不合法")
		return
	}
	if req.Version != "" && !registry.ValidVersion(req.Version) {
		writeErr(w, http.StatusBadRequest, "版本号不合法")
		return
	}
	if v, err := s.store.ListVersions(r.Context(), req.Name, store.StatusPublished, store.StatusPending); err == nil && len(v) > 0 && req.Version == "" {
		writeErr(w, http.StatusConflict, req.Name+" 已经在能力市场或审核队列里了")
		return
	}
	j, err := s.registry.StartSync(r.Context(), "request", registry.SyncParams{
		Roots:       []registry.SyncRoot{{Name: req.Name, Version: req.Version}},
		RootStatus:  store.StatusPending,
		ForceReview: true,
		Reason:      truncate(req.Reason, 500),
	}, currentUser(r).ID)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	target := req.Name
	if req.Version != "" {
		target += "@" + req.Version
	}
	s.audit(r, auditImportRequest, target, map[string]any{"job": j.ID, "reason": truncate(req.Reason, 500)})
	writeJSON(w, http.StatusAccepted, j)
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) > n {
		rs = rs[:n]
	}
	return string(rs)
}

// ---- npm 令牌 ----

func (s *Server) createNpmToken(r *http.Request, userID int64, name, access string) (string, *store.NpmToken, error) {
	plain := npmTokenPrefix + secret.Token()
	t := &store.NpmToken{UserID: userID, Name: name, Prefix: plain[:len(npmTokenPrefix)+6], Access: access}
	if err := s.store.CreateNpmToken(r.Context(), t, secret.Hash(plain)); err != nil {
		return "", nil, err
	}
	return plain, t, nil
}

func (s *Server) handleMyNpmTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.ListNpmTokens(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNil(ts)})
}

type createNpmTokenReq struct {
	Name   string `json:"name"`
	Access string `json:"access"`
}

const maxNpmTokensPerUser = 20

func (s *Server) handleCreateMyNpmToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireFullSession(w, r) {
		return
	}
	var req createNpmTokenReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		writeErr(w, http.StatusBadRequest, "名称不能为空，且不超过 128 个字符")
		return
	}
	switch req.Access {
	case "", "read":
		req.Access = "read"
	case "publish":
		if !currentAccess(r).Has(rbac.RegistryPublish) {
			writeErr(w, http.StatusForbidden, "你的账号没有发布能力的权限，只能创建只读令牌")
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "access 只能是 read 或 publish")
		return
	}
	existing, err := s.store.ListNpmTokens(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if len(existing) >= maxNpmTokensPerUser {
		writeErr(w, http.StatusBadRequest, "最多同时保留 "+strconv.Itoa(maxNpmTokensPerUser)+" 个令牌，请先吊销不用的")
		return
	}
	plain, t, err := s.createNpmToken(r, currentUser(r).ID, req.Name, req.Access)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	t.Username = currentUser(r).Username
	s.audit(r, auditNpmTokenCreate, t.Name, map[string]any{"id": t.ID, "access": t.Access, "prefix": t.Prefix})
	writeJSON(w, http.StatusCreated, map[string]any{"token": plain, "info": t})
}

func (s *Server) revokeNpmToken(w http.ResponseWriter, r *http.Request, own bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.store.GetNpmToken(r.Context(), id)
	if notFound(err) || (err == nil && own && t.UserID != currentUser(r).ID) {
		writeErr(w, http.StatusNotFound, "令牌不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.RevokeNpmToken(r.Context(), id); err != nil && !notFound(err) {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditNpmTokenRevoke, t.Name, map[string]any{"id": id, "owner": s.usernameOf(r, t.UserID), "prefix": t.Prefix, "self": own})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeMyNpmToken(w http.ResponseWriter, r *http.Request) {
	s.revokeNpmToken(w, r, true)
}
func (s *Server) handleRevokeNpmToken(w http.ResponseWriter, r *http.Request) {
	s.revokeNpmToken(w, r, false)
}

func (s *Server) handleAllNpmTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.ListNpmTokens(r.Context(), 0)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNil(ts)})
}

// ---- npm 源总览 ----

// GET /api/admin/registry：统计与当前路由。
func (s *Server) handleRegistryOverview(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.RegistryStats(r.Context())
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
		"stats":        st,
		"scopes":       set.CompanyScopes(),
		"upstream":     registry.UpstreamBase(set),
		"proxying":     set.Registry.ProxyMode == "cache" && registry.UpstreamBase(set) != "",
		"registry_url": s.publicBase(r) + "/npm/",
		"index_url":    s.publicBase(r) + "/index/dsh-index.json",
	})
}

// POST /api/admin/registry/check：检查上游 registry 能不能连上。
func (s *Server) handleRegistryCheck(w http.ResponseWriter, r *http.Request) {
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	base := registry.UpstreamBase(set)
	if base == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "离线模式，不访问上游"})
		return
	}
	start := time.Now()
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/-/ping", nil)
	resp, err := s.registryClient().Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latency_ms": ms, "message": err.Error()})
		return
	}
	resp.Body.Close()
	ok := resp.StatusCode < 400
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "latency_ms": ms, "status": resp.StatusCode, "upstream": base})
}

func (s *Server) registryClient() *http.Client {
	if s.opts.NpmUpstream != nil {
		return s.opts.NpmUpstream
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: s.opts.DemoSite.guard(&http.Transport{Proxy: http.ProxyFromEnvironment})}
}

// ---- 上游预载 ----

type feedItem struct {
	registry.FeedPlugin
	// 本地状态：已镜像的版本与审核状态
	LocalVersion string `json:"local_version"`
	LocalStatus  string `json:"local_status"`
	// 上游 registry 的 latest（只查已镜像的包）；dshfind 目录的版本常常落后
	UpstreamLatest string `json:"upstream_latest,omitempty"`
	// 上游有比本地更新的版本
	Newer        bool   `json:"newer"`
	DepCount     *int   `json:"dep_count"`
	ClosureSize  *int64 `json:"closure_size"`
	OfflineReady *bool  `json:"offline_ready"`
}

// GET /api/admin/upstream/feed：dshfind 插件目录 + 本地镜像状态。
func (s *Server) handleUpstreamFeed(w http.ResponseWriter, r *http.Request) {
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !set.Upstream.DshfindFeed || set.Upstream.Mode == "offline" {
		writeJSON(w, http.StatusOK, map[string]any{"plugins": []feedItem{}, "disabled": true})
		return
	}
	plugins, at, err := s.registry.Feed(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"plugins": []feedItem{}, "error": err.Error()})
		return
	}
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
	}
	pkgs, err := s.store.ListPackages(r.Context(), store.PackageFilter{Names: names})
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	byName := map[string]*store.NpmPackage{}
	var have []string
	for _, p := range pkgs {
		byName[p.Name] = p
		have = append(have, p.Name)
	}
	versions, err := s.store.VersionsOf(r.Context(), have)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var mirrored []string
	for _, p := range pkgs {
		if p.Origin == store.OriginUpstream {
			mirrored = append(mirrored, p.Name)
		}
	}
	latest := s.registry.UpstreamLatest(r.Context(), mirrored)
	out := make([]feedItem, 0, len(plugins))
	for _, p := range plugins {
		it := feedItem{FeedPlugin: p, UpstreamLatest: latest[p.Name]}
		if pkg := byName[p.Name]; pkg != nil {
			if v := visibleLatest(pkg, versions[p.Name]); v != nil {
				it.LocalVersion, it.LocalStatus = v.Version, v.Status
			}
			it.DepCount, it.ClosureSize, it.OfflineReady = pkg.DepCount, pkg.ClosureSize, pkg.OfflineReady
		}
		upVer := it.UpstreamLatest
		if upVer == "" {
			upVer = p.Version
		}
		it.Newer = it.LocalVersion != "" && upVer != "" && registry.CompareVersions(upVer, it.LocalVersion) > 0
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": out, "fetched_at": at})
}

type syncReq struct {
	Names []string `json:"names"`
	// true：同步后直接上架；false：进入待审核
	Publish bool `json:"publish"`
}

// POST /api/admin/upstream/sync：预载选中的插件（连同依赖闭包）。
func (s *Server) handleUpstreamSync(w http.ResponseWriter, r *http.Request) {
	var req syncReq
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Names) == 0 || len(req.Names) > 500 {
		writeErr(w, http.StatusBadRequest, "一次预载 1–500 个包")
		return
	}
	feedByName := map[string]registry.FeedPlugin{}
	if plugins, _, err := s.registry.Feed(r.Context(), false); err == nil {
		for _, p := range plugins {
			feedByName[p.Name] = p
		}
	}
	var roots []registry.SyncRoot
	for _, n := range req.Names {
		root := registry.SyncRoot{Name: n}
		if fp, ok := feedByName[n]; ok {
			root.Catalog = &fp
		}
		roots = append(roots, root)
	}
	status := store.StatusPending
	if req.Publish {
		status = store.StatusPublished
	}
	j, err := s.registry.StartSync(r.Context(), "preload", registry.SyncParams{Roots: roots, RootStatus: status}, currentUser(r).ID)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	s.audit(r, auditUpstreamSync, strings.Join(req.Names, ", "), map[string]any{"job": j.ID, "count": len(req.Names), "publish": req.Publish})
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	js, err := s.store.ListJobs(r.Context(), 30)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": nonNil(js)})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	j, err := s.store.GetJob(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// POST /api/admin/upstream/check-updates：立即检查已预载插件的新版本。
func (s *Server) handleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	j, err := s.registry.CheckUpdates(r.Context(), currentUser(r).ID)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": j})
}

// GET /api/admin/upstream/export?since=…：导出离线包。
func (s *Server) handleExportBundle(w http.ResponseWriter, r *http.Request) {
	var since *time.Time
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			if t, err = time.Parse(time.DateOnly, v); err != nil {
				writeErr(w, http.StatusBadRequest, "since 应该是日期，例如 2026-10-01")
				return
			}
		}
		since = &t
	}
	file := "dsh-ee-bundle-" + time.Now().UTC().Format("20060102-150405") + ".tar"
	detail := map[string]any{}
	if since != nil {
		detail["since"] = since.Format(time.DateOnly)
	}
	s.audit(r, auditUpstreamExport, file, detail)
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file}))
	if _, err := s.registry.ExportBundle(r.Context(), w, since); err != nil {
		// 头已经发出去了，只能记日志
		s.log.Error("导出离线包失败", "err", err)
	}
}

// POST /api/admin/upstream/import：导入离线包（请求体是 tar）。
func (s *Server) handleImportBundle(w http.ResponseWriter, r *http.Request) {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/x-tar" && mt != "application/octet-stream" {
		writeErr(w, http.StatusUnsupportedMediaType, "请求体必须是离线包（application/x-tar）")
		return
	}
	res, j, err := s.registry.ImportBundle(r.Context(), r.Body, currentUser(r).ID)
	if err != nil {
		var re *registry.Error
		if errors.As(err, &re) || j == nil {
			s.registryErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "job": j})
		return
	}
	s.audit(r, auditUpstreamImport, "", map[string]any{"packages": res.Packages, "added": res.Added, "pending": res.Pending})
	writeJSON(w, http.StatusOK, map[string]any{"result": res, "job": j})
}
