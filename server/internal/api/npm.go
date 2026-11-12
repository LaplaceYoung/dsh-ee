package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// npm 协议（/npm/…）：packument、tarball、publish、dist-tag、whoami 与旧式 npm login。
// 客户端配置：registry=<dsh-ee>/npm/，//<host>/npm/:_authToken=ee_npm_…

const npmTokenPrefix = "ee_npm_"

// npmCaller 是一次 npm 请求的调用者；匿名时为 nil。
type npmCaller struct {
	user      *store.User
	access    *rbac.Access
	token     *store.NpmToken // 旧 npm 令牌；会话与新 ee_at 令牌为 nil
	principal *Principal
}

func (c *npmCaller) userID() *int64 {
	if c == nil {
		return nil
	}
	return &c.user.ID
}

func (c *npmCaller) tokenID() *int64 {
	if c == nil {
		return nil
	}
	if c.principal != nil && c.principal.TokenID > 0 {
		return &c.principal.TokenID
	}
	if c.token != nil {
		return &c.token.ID
	}
	return nil
}

// npmAuth 识别调用者：Bearer 令牌（npm / pnpm 的 _authToken）、Basic（密码位放令牌），或网页会话。
func (s *Server) npmAuth(r *http.Request) (*npmCaller, error) {
	token := ""
	auth := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(auth, "Bearer "); ok {
		token = strings.TrimSpace(t)
	} else if b, ok := strings.CutPrefix(auth, "Basic "); ok {
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b)); err == nil {
			_, pw, _ := strings.Cut(string(raw), ":")
			token = pw
		}
	}
	if token != "" {
		p, err := s.resolveBearer(r.Context(), token, clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
		var be bearerError
		if errors.As(err, &be) {
			return nil, errBadToken
		}
		if err != nil {
			return nil, err
		}
		if p.Legacy == "key" {
			return nil, errBadToken
		}
		caller := &npmCaller{user: p.User, access: p.Access, principal: p}
		if p.Legacy == "npm" {
			caller.token, _, _ = s.store.ActiveNpmToken(r.Context(), secret.Hash(token))
		}
		return caller, nil
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		u, limited, err := s.store.SessionInfo(r.Context(), secret.Hash(c.Value))
		if notFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		a, err := s.store.UserAccess(r.Context(), u.ID)
		if err != nil {
			return nil, err
		}
		if limited {
			a = limitedAccess(a)
		}
		return &npmCaller{user: u, access: a}, nil
	}
	return nil, nil
}

var errBadToken = errors.New("bad token")

// npmPath 拆出包名与剩余路径段。作用域包可能写成 @scope%2fname 或 @scope/name。
func npmPath(r *http.Request) (special bool, name string, rest []string, ok bool) {
	p := strings.TrimPrefix(r.URL.EscapedPath(), "/npm/")
	p = strings.NewReplacer("%2f", "/", "%2F", "/").Replace(p)
	var segs []string
	for _, raw := range strings.Split(p, "/") {
		seg, err := url.PathUnescape(raw)
		if err != nil || strings.Contains(seg, "/") {
			return false, "", nil, false
		}
		segs = append(segs, seg)
	}
	if len(segs) == 0 || segs[0] == "" {
		return false, "", nil, true
	}
	if segs[0] == "-" {
		return true, "", segs[1:], true
	}
	if strings.HasPrefix(segs[0], "@") {
		if len(segs) < 2 {
			return false, "", nil, false
		}
		return false, segs[0] + "/" + segs[1], segs[2:], true
	}
	return false, segs[0], segs[1:], true
}

func (s *Server) handleNpm(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeErr(w, http.StatusServiceUnavailable, "npm 源没有初始化（数据目录不可写），请检查服务端日志")
		return
	}
	if s.demoSiteNpmWrite(w, r) {
		return
	}
	caller, err := s.npmAuth(r)
	if errors.Is(err, errBadToken) {
		writeErr(w, http.StatusUnauthorized, "npm 令牌无效或已吊销，请在 dsh-ee 的「接入指南」里重新生成")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	special, name, rest, ok := npmPath(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "非法的路径")
		return
	}
	if special {
		s.handleNpmSpecial(w, r, caller, rest)
		return
	}
	if name == "" {
		writeJSON(w, http.StatusOK, map[string]string{"db_name": "dsh-ee"})
		return
	}
	if !registry.ValidName(name) {
		writeErr(w, http.StatusBadRequest, "非法的包名")
		return
	}

	switch {
	case len(rest) == 0 && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		if s.npmRead(w, r, caller) {
			s.servePackument(w, r, name)
		}
	case len(rest) == 0 && r.Method == http.MethodPut:
		s.handleNpmPublish(w, r, caller, name)
	case len(rest) == 2 && rest[0] == "-" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		if s.npmRead(w, r, caller) {
			s.serveTarball(w, r, caller, name, rest[1])
		}
	case len(rest) == 1 && rest[0] != "-" && r.Method == http.MethodGet:
		if s.npmRead(w, r, caller) {
			s.serveVersion(w, r, name, rest[0])
		}
	case len(rest) >= 1 && (rest[0] == "-rev" || r.Method == http.MethodDelete):
		writeErr(w, http.StatusMethodNotAllowed, "dsh-ee 不支持用 npm unpublish 删除版本；请在 dsh-ee 网页的「能力管理」里下架，或发布新版本后用 npm deprecate")
	default:
		writeErr(w, http.StatusNotFound, "接口不存在")
	}
}

// npmRead 检查读权限：要求令牌时必须登录且有「安装能力」权限。
func (s *Server) npmRead(w http.ResponseWriter, r *http.Request, c *npmCaller) bool {
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return false
	}
	if !set.Registry.RequireToken {
		return true
	}
	if c == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
		writeErr(w, http.StatusUnauthorized, "这个 npm 源需要令牌：在 dsh-ee 的「接入指南」里生成后写进 .npmrc")
		return false
	}
	if !c.access.Has(rbac.RegistryUse) || c.principal != nil && !c.principal.Scopes.Has(rbac.ScopeRegistryRead) {
		writeErr(w, http.StatusForbidden, "你的账号没有安装能力的权限，请联系管理员")
		return false
	}
	return true
}

func (s *Server) registryErr(w http.ResponseWriter, r *http.Request, err error) {
	var re *registry.Error
	switch {
	case errors.As(err, &re):
		writeErr(w, re.Status, re.Msg)
	case errors.Is(err, registry.ErrNotFound), notFound(err):
		writeErr(w, http.StatusNotFound, "没有这个包或版本（企业作用域的包只在这里发布；社区插件需要管理员审核上架）")
	default:
		s.internalErr(w, r, err)
	}
}

const corgiType = "application/vnd.npm.install-v1+json"

func (s *Server) servePackument(w http.ResponseWriter, r *http.Request, name string) {
	p, err := s.registry.Packument(r.Context(), name)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	abbreviated := strings.Contains(r.Header.Get("Accept"), corgiType)
	body, err := p.Encode(s.publicBase(r)+"/npm", abbreviated)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if abbreviated {
		w.Header().Set("Content-Type", corgiType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept, Authorization")
	_, _ = w.Write(body)
}

func (s *Server) serveVersion(w http.ResponseWriter, r *http.Request, name, spec string) {
	p, err := s.registry.Packument(r.Context(), name)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	body, err := p.Encode(s.publicBase(r)+"/npm", false)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var doc struct {
		DistTags map[string]string          `json:"dist-tags"`
		Versions map[string]json.RawMessage `json:"versions"`
	}
	_ = json.Unmarshal(body, &doc)
	var names []string
	for v := range doc.Versions {
		names = append(names, v)
	}
	v := registry.ResolveRange(spec, names, doc.DistTags)
	if v == "" {
		writeErr(w, http.StatusNotFound, "没有满足 "+spec+" 的版本")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(doc.Versions[v])
}

func (s *Server) serveTarball(w http.ResponseWriter, r *http.Request, c *npmCaller, name, file string) {
	version, ok := registry.VersionFromFile(name, file)
	if !ok {
		writeErr(w, http.StatusNotFound, "tarball 文件名不对")
		return
	}
	f, v, err := s.registry.Tarball(r.Context(), name, version)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	defer f.Close()
	if r.Method == http.MethodGet {
		ua := registry.ParseUserAgent(r.UserAgent())
		d := store.Download{Name: name, Version: version, UserID: c.userID(), TokenID: c.tokenID(), Client: ua.Name, OS: ua.OS, Arch: ua.Arch}
		if err := s.store.RecordDownload(r.Context(), d); err != nil {
			s.log.Warn("记录下载失败", "err", err)
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+v.Shasum+`"`)
	http.ServeContent(w, r, file, v.CreatedAt, f)
}

// ---- 发布 ----

func (s *Server) publisher(c *npmCaller) registry.Publisher {
	return registry.Publisher{UserID: c.user.ID, Username: c.user.Username, Manager: c.access.Has(rbac.PluginsManage)}
}

// requirePublish 检查发布权限：登录、有「发布能力」，用令牌时令牌必须是可发布的。
func (s *Server) requirePublish(w http.ResponseWriter, c *npmCaller) bool {
	if c == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
		writeErr(w, http.StatusUnauthorized, "发布需要登录：在 dsh-ee 的「接入指南」里生成「可发布」令牌")
		return false
	}
	if !c.access.Has(rbac.RegistryPublish) {
		writeErr(w, http.StatusForbidden, "你的账号没有发布能力的权限，请联系管理员")
		return false
	}
	if c.token != nil && c.token.Access != "publish" || c.principal != nil && !c.principal.Scopes.Has(rbac.ScopeRegistryPublish) {
		writeErr(w, http.StatusForbidden, "这个令牌是只读的，发布需要「可发布」令牌")
		return false
	}
	return true
}

func (s *Server) handleNpmPublish(w http.ResponseWriter, r *http.Request, c *npmCaller, name string) {
	if !s.requirePublish(w, c) {
		return
	}
	limit := s.registry.MaxTarball*4/3 + 4<<20
	var doc registry.PublishDoc
	if err := json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(&doc); err != nil {
		writeErr(w, http.StatusBadRequest, "发布内容不是合法的 JSON："+err.Error())
		return
	}
	if doc.Name == "" {
		doc.Name = name
	}
	if doc.Name != name {
		writeErr(w, http.StatusBadRequest, "请求路径里的包名与发布内容不一致")
		return
	}
	// 没有 tarball 的 PUT 是 npm deprecate / undeprecate
	if len(doc.Attachments) == 0 {
		n, err := s.registry.Deprecate(r.Context(), s.publisher(c), name, doc.Versions)
		if err != nil {
			s.registryErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": n})
		return
	}
	res, err := s.registry.Publish(r.Context(), s.publisher(c), &doc)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	s.log.Info("npm publish", "pkg", res.Name, "version", res.Version, "status", res.Status, "user", c.user.Username)
	s.auditAs(r, c.user, auditPackagePublish, res.Name+"@"+res.Version, map[string]any{"via": "npm", "status": res.Status})
	if res.Status == store.StatusPending {
		s.feishuNpmPublish(r, res, c.user.ID)
		// npm 会把这个头原样打印成 npm notice。HTTP 头按 Latin-1 解码，中文会乱码，只能写 ASCII
		w.Header().Set("npm-notice", res.Name+"@"+res.Version+" is pending review: it becomes installable after an admin approves it in dsh-ee (Review queue)")
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": res.Name, "version": res.Version, "status": res.Status})
}

// ---- /npm/-/… ----

func (s *Server) handleNpmSpecial(w http.ResponseWriter, r *http.Request, c *npmCaller, rest []string) {
	path := strings.Join(rest, "/")
	switch {
	case path == "ping":
		writeJSON(w, http.StatusOK, map[string]any{})
	case path == "whoami" && r.Method == http.MethodGet:
		if c == nil {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"username": c.user.Username})
	case len(rest) == 2 && rest[0] == "user" && strings.HasPrefix(rest[1], "org.couchdb.user:") && r.Method == http.MethodPut:
		s.handleNpmLogin(w, r)
	case strings.HasPrefix(path, "npm/v1/security/"):
		// npm install 会顺带做漏洞审计；内网没有漏洞库，返回空结果让它安静地跳过
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 8<<20))
		writeJSON(w, http.StatusOK, map[string]any{})
	case path == "v1/search" && r.Method == http.MethodGet:
		if s.npmRead(w, r, c) {
			s.handleNpmSearch(w, r)
		}
	case len(rest) >= 3 && rest[0] == "package" && rest[len(rest)-1] == "dist-tags" || len(rest) >= 4 && rest[0] == "package" && rest[len(rest)-2] == "dist-tags":
		s.handleDistTags(w, r, c, rest[1:])
	default:
		writeErr(w, http.StatusNotFound, "接口不存在")
	}
}

// handleDistTags：GET -/package/<pkg>/dist-tags，PUT / DELETE -/package/<pkg>/dist-tags/<tag>。
func (s *Server) handleDistTags(w http.ResponseWriter, r *http.Request, c *npmCaller, segs []string) {
	var name, tag string
	i := slices.Index(segs, "dist-tags")
	switch i {
	case 1:
		name = segs[0]
	case 2:
		name = segs[0] + "/" + segs[1]
	default:
		writeErr(w, http.StatusBadRequest, "非法的路径")
		return
	}
	if len(segs) > i+1 {
		tag = segs[i+1]
	}
	if r.Method == http.MethodGet {
		if !s.npmRead(w, r, c) {
			return
		}
		p, err := s.registry.Packument(r.Context(), name)
		if err != nil {
			s.registryErr(w, r, err)
			return
		}
		body, _ := p.Encode("", true)
		var doc struct {
			DistTags map[string]string `json:"dist-tags"`
		}
		_ = json.Unmarshal(body, &doc)
		writeJSON(w, http.StatusOK, doc.DistTags)
		return
	}
	if tag == "" || (r.Method != http.MethodPut && r.Method != http.MethodDelete) {
		writeErr(w, http.StatusMethodNotAllowed, "不支持的操作")
		return
	}
	if !s.requirePublish(w, c) {
		return
	}
	version := ""
	if r.Method == http.MethodPut {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&version); err != nil || version == "" {
			writeErr(w, http.StatusBadRequest, "请求体应该是 JSON 字符串形式的版本号")
			return
		}
	}
	tags, err := s.registry.SetDistTag(r.Context(), s.publisher(c), name, tag, version)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// handleNpmLogin 支持 npm login --auth-type=legacy：用 dsh-ee 账号密码换一个令牌。
func (s *Server) handleNpmLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	username := strings.ToLower(strings.TrimSpace(body.Name))
	if s.limiter.locked(username) {
		writeErr(w, http.StatusTooManyRequests, "失败次数过多，请 15 分钟后再试")
		return
	}
	u, ok := s.login(w, r, username, body.Password, "npm")
	if !ok {
		return
	}
	a, err := s.store.UserAccess(r.Context(), u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !a.Has(rbac.RegistryUse, rbac.RegistryPublish) {
		writeErr(w, http.StatusForbidden, "你的账号没有使用 npm 源的权限")
		return
	}
	access := "read"
	if a.Has(rbac.RegistryPublish) {
		access = "publish"
	}
	plain, t, err := s.createNpmToken(r, u.ID, "npm login", access)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.log.Info("npm login", "user", u.Username, "token", t.ID)
	s.auditAs(r, u, auditNpmTokenCreate, t.Name, map[string]any{"id": t.ID, "access": access, "via": "npm login"})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": "org.couchdb.user:" + u.Username, "token": plain})
}

func (s *Server) handleNpmSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("text")))
	pkgs, err := s.store.ListPackages(r.Context(), store.PackageFilter{CatalogOnly: true})
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	type obj struct {
		Package map[string]any `json:"package"`
	}
	objects := []obj{}
	for _, p := range pkgs {
		if q != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Title+" "+p.Description), q) {
			continue
		}
		latest := p.DistTags["latest"]
		if v, err := s.store.GetVersion(r.Context(), p.Name, latest); err != nil || v.Status != store.StatusPublished {
			continue
		}
		objects = append(objects, obj{Package: map[string]any{"name": p.Name, "version": latest, "description": p.Description}})
		if len(objects) >= 50 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": objects, "total": len(objects)})
}

// publicBase 是对外地址（不带结尾斜杠），用于生成 tarball 地址与门户链接。
func (s *Server) publicBase(r *http.Request) string {
	if s.opts.PublicURL != "" {
		return s.opts.PublicURL
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return scheme + "://" + host
}

// handleDshIndex 输出 DSH 插件索引（format 1）。只含已上架插件的元数据，不需要令牌：
// DSH 请求索引源时不一定能带认证头；下载 tarball 仍然要令牌。
func (s *Server) handleDshIndex(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeErr(w, http.StatusServiceUnavailable, "npm 源没有初始化")
		return
	}
	idx, err := s.registry.BuildIndex(r.Context(), s.publicBase(r))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, idx)
}
