package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DSH 接入插件：dsh-ee 按企业设置生成一个 DSH 组合包 <主作用域>/dsh-connect，发布到自己的 npm 源。
// 员工 `dsh plugin --profile web add <主作用域>/dsh-connect` 一次，之后技能、提示词、微应用、
// 企业连接与必装插件都由 dsh-ee 按人下发。内容（地址、模型、开关）变了就自动发一个新版本。

//go:embed connectpkg/index.js
var connectIndexJS []byte

//go:embed connectpkg/client.js
var connectClientJS []byte

const (
	connectStateKey  = "connect_bundle"
	connectTokenEnv  = "DSHEE_TOKEN"
	connectProfile   = "web"
	connectSkillRank = 350
)

type connectState struct {
	Package     string    `json:"package"`
	Version     string    `json:"version"`
	Hash        string    `json:"hash"`
	BaseURL     string    `json:"base_url"`
	PublishedAt time.Time `json:"published_at"`
	Error       string    `json:"error,omitempty"`
}

// connectPackage 是接入插件的包名。
func connectPackage(set *store.AppSettings) string { return set.Scope + "/" + registry.ConnectName }

// connectBase 是写进插件的 dsh-ee 地址：系统设置里的访问地址，其次启动参数 DSH_EE_PUBLIC_URL，
// 再次按当前请求推断（后台任务没有请求，就不生成）。
func (s *Server) connectBase(ctx context.Context, r *http.Request) (string, error) {
	set, err := s.appSettings(ctx)
	if err != nil {
		return "", err
	}
	switch {
	case set.BaseURL != "":
		return set.BaseURL, nil
	case s.opts.PublicURL != "":
		return s.opts.PublicURL, nil
	case r != nil:
		return s.publicBase(r), nil
	}
	return "", errors.New("还不知道员工访问 dsh-ee 的地址：请在系统设置里填写「访问地址」")
}

func yamlQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

// buildConnect 生成接入插件的文件内容与内容摘要（摘要不含版本号）。
func (s *Server) buildConnect(ctx context.Context, base, version string) (string, map[string][]byte, string, error) {
	set, err := s.appSettings(ctx)
	if err != nil {
		return "", nil, "", err
	}
	pkg := connectPackage(set)
	server := s.mcpServerNameCtx(ctx)
	org := strings.TrimSpace(set.OrgName)
	if org == "" {
		org = "企业"
	}

	var p strings.Builder
	fmt.Fprintf(&p, "# %s 的 DSH 接入组合包，由 dsh-ee（%s）生成，请勿手改。\n", org, base)
	p.WriteString("- insert:\n")
	fmt.Fprintf(&p, "    - id: dsh-ee-connect\n      name: %s\n      config:\n", yamlQuote(pkg))
	fmt.Fprintf(&p, "        base: %s\n        org: %s\n        serverName: %s\n        tokenRef: %s\n        autoInstall: %t\n        version: %s\n",
		yamlQuote(base), yamlQuote(org), yamlQuote(server), connectTokenEnv, set.Connect.InstallRequired, yamlQuote(version))
	// 插件把这些作用域与令牌写进所在 profile 的 .npmrc，必装插件与升级才能从 dsh-ee 安装
	p.WriteString("        scopes:\n")
	for _, scope := range set.CompanyScopes() {
		fmt.Fprintf(&p, "          - %s\n", yamlQuote(scope))
	}
	if set.Connect.Market {
		p.WriteString("    - id: plugin-market\n      name: '@deepseek-ai/dsh-plugin-market'\n      disabled: !!js \"!ctx.get('profileContext')\"\n")
		fmt.Fprintf(&p, "      config:\n        indexes:\n          - %s\n", yamlQuote(base+"/index/dsh-index.json"))
		p.WriteString("    - id: ui-plugin-market\n      name: '@deepseek-ai/dsh-client-ui-plugin-market'\n")
	}
	// 企业品牌不再关掉官方品牌插件：接入插件在运行时按品牌设置用更小的 priority 盖过它（见 dsh-plugin/src/brand-client.ts），
	// 插件包内容也就不随品牌开关变化，管理员开关品牌不用员工更新插件。
	// 有微应用的工作台要嵌网页时，打开官方浏览器标签页；没有就不动员工的 DSH 设置
	web, err := s.anyLiveWebPane(ctx)
	if err != nil {
		return "", nil, "", err
	}
	if web {
		p.WriteString("- id: ui-sidebar-browser\n  disabled: false\n")
	}
	var models []string
	if set.Connect.Model {
		ms, err := s.store.PublicModels(ctx)
		if err != nil {
			return "", nil, "", err
		}
		for _, m := range ms {
			models = append(models, m.Name)
		}
	}
	if len(models) > 0 {
		def := set.Connect.DefaultModel
		if !slices.Contains(models, def) {
			def = models[0]
		}
		fmt.Fprintf(&p, "- id: llm-pi-ai\n  config:\n    providers:\n      dsh-ee:\n        apiKeyEnv: %s\n        api: openai-completions\n        baseURL: %s\n        headers:\n          X-DshEE-Client: %s\n          X-DshEE-Host: !!js \"process.versions.electron ? 'desktop/' + (process.env.DSH_DESKTOP_VERSION || process.env.DSH_VERSION || process.versions.electron) : 'cli'\"\n        models:\n",
			connectTokenEnv, yamlQuote(base+"/v1"), yamlQuote("dsh-connect/"+version))
		for _, m := range models {
			fmt.Fprintf(&p, "          - id: %s\n", yamlQuote(m))
		}
		fmt.Fprintf(&p, "- id: agent-default-model\n  config:\n    provider: dsh-ee\n    model: %s\n", yamlQuote(def))
	}
	patch := []byte(p.String())

	readme := fmt.Sprintf(`# %s · DSH 接入插件

由 dsh-ee 生成。装上之后，DSH 会：

- 加载 %s 在能力中心下发给你的技能、提示词与微应用（在 DSH 里用 /名字 调用）；
- 接入企业系统：工具名以 mcp__%s__ 开头，dsh-ee 按你的权限转发并记录；
%s%s- 自动安装企业设为「自动安装」的插件。

通过 setup 设备登录或在 DSH 对话里执行 /ee-login 后，令牌保存在凭据库（%s）；CI 也可在 .env 中手动设置同名变量。

dsh-ee：%s
`, org, org, server,
		map[bool]string{true: "- 在插件页的「发现」里列出企业插件索引；\n"}[set.Connect.Market],
		map[bool]string{true: "- 通过 dsh-ee 的模型网关调用模型（额度按账号计算）；\n"}[len(models) > 0],
		connectTokenEnv, base)

	clientJS := bytes.ReplaceAll(connectClientJS, []byte("__DSH_EE_PACKAGE__"), []byte(pkg))
	files := map[string][]byte{"index.js": connectIndexJS, "client.js": clientJS, "cordis.patch.yml": patch, "README.md": []byte(readme)}
	h := sha256.New()
	for _, name := range []string{"index.js", "client.js", "cordis.patch.yml", "README.md"} {
		content := files[name]
		if name == "cordis.patch.yml" {
			content = bytes.ReplaceAll(content, []byte(version), []byte(""))
		}
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(content))
		h.Write(content)
	}
	return pkg, files, hex.EncodeToString(h.Sum(nil))[:16], nil
}

func connectManifest(pkg, version, org string) []byte {
	m := map[string]any{
		"name":        pkg,
		"version":     version,
		"description": org + " 的 DSH 接入插件：企业技能、提示词、微应用、企业连接与模型",
		"type":        "module",
		"main":        "index.js",
		"license":     "UNLICENSED",
		"files":       []string{"index.js", "client.js", "cordis.patch.yml", "README.md"},
		"keywords":    []string{"dsh-plugin", "dsh-ee"},
		"exports": map[string]any{
			".":        map[string]any{"default": "./index.js"},
			"./client": map[string]any{"default": "./client.js"},
		},
		"dsh": map[string]any{
			"bundle": map[string]any{"patch": "./cordis.patch.yml"},
			"client": map[string]any{
				"inject":   []string{"@deepseek-ai/dsh-client-ui-renderer", "@deepseek-ai/dsh-client-ui-sidebar", "@deepseek-ai/dsh-api-remotes"},
				"platform": "web",
			},
		},
		"dsh-ee": map[string]any{"title": "DSH 接入插件", "category": "tools", "kind": "connector"},
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return append(b, '\n')
}

func tarGz(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	// package.json 放第一个，其余按名字
	slices.SortFunc(names, func(a, b string) int {
		if a == "package.json" {
			return -1
		}
		if b == "package.json" {
			return 1
		}
		return strings.Compare(a, b)
	})
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, n := range names {
		if err := tw.WriteHeader(&tar.Header{Name: "package/" + n, Mode: 0o644, Size: int64(len(files[n])), ModTime: mtime, Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// nextConnectVersion 取 1.0.N 里还没用过的下一个 N（被驳回或下架的版本号也不能复用）。
func (s *Server) nextConnectVersion(ctx context.Context, pkg string) (string, error) {
	vs, err := s.store.ListVersions(ctx, pkg)
	if err != nil {
		return "", err
	}
	n := 0
	for _, v := range vs {
		if rest, ok := strings.CutPrefix(v.Version, "1.0."); ok {
			if x, err := strconv.Atoi(rest); err == nil && x > n {
				n = x
			}
		}
	}
	return "1.0." + strconv.Itoa(n+1), nil
}

// ensureConnect 内容变了（或 force）就发布新版本。返回最新状态。
func (s *Server) ensureConnect(ctx context.Context, r *http.Request, force bool, by int64) (*connectState, error) {
	// 网页上点发布与后台检查可能同时进来：两边会算出同一个新版本号，输的一方发布失败后
	// 把读到的旧状态连同错误写回，接入插件就短暂显示成「未发布」。串行执行，后到的直接看到已发布的状态。
	s.connectMu.Lock()
	defer s.connectMu.Unlock()
	if s.registry == nil {
		return nil, errors.New("npm 源不可用")
	}
	set, err := s.appSettings(ctx)
	if err != nil {
		return nil, err
	}
	var st connectState
	if _, err := s.store.GetSettingJSON(ctx, connectStateKey, &st); err != nil {
		return nil, err
	}
	if !set.Connect.Enabled {
		return &st, nil
	}
	base, err := s.connectBase(ctx, r)
	if err != nil {
		st.Error = err.Error()
		return &st, nil
	}
	pkg := connectPackage(set)
	version, err := s.nextConnectVersion(ctx, pkg)
	if err != nil {
		return nil, err
	}
	pkg, files, hash, err := s.buildConnect(ctx, base, version)
	if err != nil {
		return nil, err
	}
	if !force && st.Package == pkg && st.Hash == hash && st.Error == "" {
		if v, err := s.store.GetVersion(ctx, pkg, st.Version); err == nil && v.Status == store.StatusPublished {
			return &st, nil
		}
	}
	org := strings.TrimSpace(set.OrgName)
	if org == "" {
		org = "企业"
	}
	manifest := connectManifest(pkg, version, org)
	files["package.json"] = manifest
	tgz, err := tarGz(files)
	if err != nil {
		return nil, err
	}
	doc := &registry.PublishDoc{
		Name:     pkg,
		DistTags: map[string]string{"latest": version},
		Versions: map[string]json.RawMessage{version: manifest},
		Readme:   string(files["README.md"]),
	}
	doc.Attachments = map[string]struct {
		ContentType string `json:"content_type"`
		Data        string `json:"data"`
		Length      int64  `json:"length"`
	}{registry.TarballFile(pkg, version): {ContentType: "application/octet-stream", Data: base64.StdEncoding.EncodeToString(tgz), Length: int64(len(tgz))}}
	if _, err := s.registry.Publish(ctx, registry.Publisher{UserID: by, Username: "dsh-ee", Manager: true, AutoPublish: true}, doc); err != nil {
		st.Error = err.Error()
		// 发布错误要落库，管理页读这份状态；后台检查路径不展示返回值，写不进去就只能靠日志发现。
		if serr := s.store.PutSettingJSON(ctx, connectStateKey, st, by); serr != nil {
			s.log.Warn("记录 DSH 接入插件发布错误失败", "package", pkg, "err", serr)
		}
		return &st, nil
	}
	featured := true
	// 精选标记只影响包目录展示，不影响接入插件发布与安装，失败记录后继续。
	if err := s.store.UpdatePackageCatalog(ctx, pkg, store.CatalogPatch{Featured: &featured}); err != nil {
		s.log.Warn("同步 DSH 接入插件精选标记失败", "package", pkg, "err", err)
	}
	st = connectState{Package: pkg, Version: version, Hash: hash, BaseURL: base, PublishedAt: time.Now().UTC()}
	if err := s.store.PutSettingJSON(ctx, connectStateKey, st, by); err != nil {
		return nil, err
	}
	s.log.Info("已发布 DSH 接入插件", "package", pkg, "version", version)
	if r != nil {
		s.audit(r, auditConnectPublish, pkg+"@"+version, map[string]any{"base_url": base})
	} else {
		s.auditSystem(ctx, auditConnectPublish, pkg+"@"+version, map[string]any{"base_url": base})
	}
	return &st, nil
}

// kickConnect 让后台在设置或模型变化后检查一次接入插件（合并短时间内的多次触发）。
func (s *Server) kickConnect() {
	select {
	case s.connectKick <- struct{}{}:
	default:
	}
}

// connectRecheckInterval 是多实例下触发型任务的兜底周期：非 leader 实例收到的
// kick 不会跨实例传递，靠这个周期让抢到租约的实例补跑一次（内容没变时是 no-op）。
const connectRecheckInterval = time.Hour

// runConnect 启动时、每次被触发时以及每小时检查接入插件是否需要发新版本。
func (s *Server) runConnect(ctx context.Context) {
	check := func(ctx context.Context) {
		s.runLeased(ctx, leaseConnectPublish, func(ctx context.Context) {
			if _, err := s.ensureConnect(ctx, nil, false, 0); err != nil {
				s.log.Warn("检查 DSH 接入插件失败", "err", err)
			}
		})
	}
	// 启动检查在另一实例持有租约（例如对方正在发布，或本进程刚被强杀重启、旧租约未到期）时
	// 隔一会儿重试，避免要等到下一次触发/兜底才发布。
	if !s.runLeasedRetry(ctx, leaseConnectPublish, func(ctx context.Context) {
		if _, err := s.ensureConnect(ctx, nil, false, 0); err != nil {
			s.log.Warn("检查 DSH 接入插件失败", "err", err)
		}
	}) && ctx.Err() == nil {
		s.log.Debug("启动时检查 DSH 接入插件跳过（租约由其他实例持有或数据库不可用）")
	}
	t := time.NewTicker(connectRecheckInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.connectKick:
			// 等一会儿，把连续的修改合成一次发布
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			check(ctx)
		case <-t.C:
			check(ctx)
		}
	}
}

type connectInfo struct {
	Package    string `json:"package"`
	Version    string `json:"version"`
	Ready      bool   `json:"ready"`
	Enabled    bool   `json:"enabled"`
	TokenEnv   string `json:"token_env"`
	Profile    string `json:"profile"`
	ServerName string `json:"server_name"`
	// TarballURL 是免登录的安装地址：在 DSH 插件页直接粘贴，不需要先配置 npm 源
	TarballURL string `json:"tarball_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

// GET /api/connect：接入指南用，告诉员工装哪个包、令牌写进哪个环境变量。
func (s *Server) handleConnectInfo(w http.ResponseWriter, r *http.Request) {
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	info, err := s.connectInfoFor(r.Context(), r, set)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// GET /api/admin/connect：接入插件的发布状态。
func (s *Server) handleAdminConnect(w http.ResponseWriter, r *http.Request) {
	var st connectState
	if _, err := s.store.GetSettingJSON(r.Context(), connectStateKey, &st); err != nil {
		s.internalErr(w, r, err)
		return
	}
	resp := map[string]any{"state": st}
	if base, err := s.connectBase(r.Context(), r); err == nil {
		if pkg, files, hash, err := s.buildConnect(r.Context(), base, st.Version); err == nil {
			resp["package"], resp["stale"], resp["patch"] = pkg, hash != st.Hash || pkg != st.Package, string(files["cordis.patch.yml"])
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /api/admin/connect/publish：立即生成并发布（内容没变时不发，除非 force）。
func (s *Server) handlePublishConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 && !readJSON(w, r, &req) {
		return
	}
	st, err := s.ensureConnect(r.Context(), r, req.Force, currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func connectTarballFile(version string) string { return "dsh-connect-" + version + ".tgz" }

// GET /dl/connect/dsh-connect-<版本>.tgz：免登录下载接入插件，员工在 DSH 插件页粘贴这个地址即可安装，
// 不用先配置 npm 源。包里只有 dsh-ee 地址、组织名、作用域与模型名，没有凭据；插件登录后自己写好
// profile 的 .npmrc，之后的升级与必装插件都走 npm 源。地址带版本号：pnpm 按地址记录完整性校验值，
// 同一地址换内容会让重装失败。只提供当前接入插件已发布的版本。
func (s *Server) handleConnectTarball(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeErr(w, http.StatusServiceUnavailable, "npm 源不可用")
		return
	}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	file := r.PathValue("file")
	version, ok := strings.CutPrefix(file, "dsh-connect-")
	version, ok2 := strings.CutSuffix(version, ".tgz")
	if !set.Connect.Enabled || !ok || !ok2 || version == "" {
		writeErr(w, http.StatusNotFound, "没有这个接入插件")
		return
	}
	pkg := connectPackage(set)
	if meta, err := s.store.GetVersion(r.Context(), pkg, version); err != nil || meta.Status != store.StatusPublished {
		writeErr(w, http.StatusNotFound, "没有这个接入插件版本")
		return
	}
	f, v, err := s.registry.Tarball(r.Context(), pkg, version)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+v.Shasum+`"`)
	http.ServeContent(w, r, file, v.CreatedAt, f)
}
