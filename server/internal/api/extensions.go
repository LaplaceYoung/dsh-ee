package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 声明式扩展（docs/plugins.md §4.1、§9.4 第 1 批）：管理员上传清单 + 接口文档，dsh-ee 检查之后
// 装成一个 OpenAPI 连接。清单在调用时生效：工具说明、哪些是只读、不由模型填的参数、
// 返回只保留哪些字段、请求只能发往清单列出的地址。每次调用记下扩展的版本和摘要。

const (
	auditExtInstall  = "extension.install"
	auditExtRollback = "extension.rollback"
)

// connExtension 存在连接的版本内容里。清单原文要和接口文档一起算摘要，所以存字符串，不存解析后的 JSON
// （json.RawMessage 序列化时会被压缩，摘要就对不上了）。
type connExtension struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
	Manifest string `json:"manifest"`
	// 管理员统一设置的参数值（fill: admin），键是参数名（请求体字段去掉 body.）
	Admin map[string]json.RawMessage `json:"admin,omitempty"`

	// 空是声明式；service 是服务扩展（service_ext.go）
	Kind string `json:"kind,omitempty"`
	// 服务扩展：管理员安装时填的服务地址（不带末尾的 /），工具走它的 /mcp，约定接口走 /ee/…
	ServiceURL string `json:"service_url,omitempty"`
	// 服务扩展：管理员填的配置（清单的 config）。secret 类型的值是 {"$enc": 密文}
	Config map[string]json.RawMessage `json:"config,omitempty"`
	// 服务扩展：回调地址里的随机串（清单申请了 endpoint 权限才有），第三方打到 /x/<它>/…
	Endpoint string `json:"endpoint,omitempty"`
	// oauth 模式：管理员填的 OAuth 客户端
	OAuthClient *extOAuthClient `json:"oauth_client,omitempty"`

	// 安装时验签用的 signature.json 原文和当时的签名状态（audit 口径，如 valid:abc123）。
	// 回滚要把历史快照当成一次新安装重新检查，凭这两项才能在升级后复核签名、按撤销状态拦下；
	// 只含公开的签名信息，不含私钥。没签名的扩展这里是空串。
	Signature       string `json:"signature,omitempty"`
	SignatureStatus string `json:"signature_status,omitempty"`
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
	if e.service() {
		return validateServiceExtension(sp)
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
	// 用谁的身份由清单决定，连接页面上改不了；只有 shared 模式的共享凭据放在哪里由管理员选。
	// 用哪一份命名共享凭据是管理员的选择，清单不管
	picked := sharedNameOf(sp)
	sp.Auth, sp.UserAuth = extCredential(m.Credential, sp.Auth)
	if sp.Auth.Type != "none" {
		sp.Auth.SharedName = picked
	}
	if sp.UserAuth != nil {
		if msg := validateUserAuth(sp); msg != "" {
			return msg
		}
	}
	params := map[string]extension.AdminParam{}
	for _, p := range m.AdminParams() {
		params[p.Key] = p
	}
	for k, v := range e.Admin {
		p, ok := params[k]
		// 没填（null、空串）当作没设置：用清单里的 default，必填又没有 default 的由安装那一步拦下
		if blank := strings.TrimSpace(string(v)); !ok || blank == "" || blank == "null" || blank == `""` {
			delete(e.Admin, k)
			continue
		}
		if !json.Valid(v) {
			return "参数 " + k + " 的设置值不是合法的 JSON"
		}
		if msg := p.CheckValue(v); msg != "" {
			return msg
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
	// 这里只拿了两份文件：清单和 openapi。完整安装时已经检查过 apps/icon，
	// 但同步时它们必然报 missing；只把与工具/接口文档相关的 error 当失败。
	for _, d := range res.Results {
		if d.Severity != extension.SevError {
			continue
		}
		if strings.HasPrefix(d.Code, "apps.") || strings.HasPrefix(d.Code, "icon.") {
			continue
		}
		return fmt.Errorf("扩展检查未通过（%s）：%s", d.Code, d.Message)
	}
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

// extensionCompat 检查扩展要求的最低 dsh-ee 版本。开发版（版本号不是 x.y.z）不拦。
func extensionCompat(m *extension.Manifest, current string) string {
	if m.MinVersion == "" {
		return ""
	}
	cur, ok1 := semverCore(current)
	min, ok2 := semverCore(m.MinVersion)
	if ok1 && ok2 && slices.Compare(cur[:], min[:]) < 0 {
		return fmt.Sprintf("这个扩展要求 dsh-ee %s 以上，当前是 %s；请先升级 dsh-ee", m.MinVersion, current)
	}
	return ""
}

func semverCore(v string) (out [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// checkExtensionChange 拦下会让一个连接「换了身份」的新版本：扩展装出来的连接只能升级成同一个扩展的另一个版本。
// 应用和员工是按连接名用它的工具的，连接名背后换成另一个扩展、或悄悄变回手工连接
// （清单规定的地址、固定参数、返回筛选就都不管用了），都不该发生。
func (s *Server) checkExtensionChange(ctx context.Context, a *store.Asset, sp *assetSpec) (string, error) {
	lv, err := s.store.LiveAssetVersion(ctx, a.ID)
	if err != nil {
		return "", err
	}
	old := parseSpec(lv.Spec).Extension
	switch {
	case old == nil:
		return "", nil
	case sp.Extension == nil:
		return "这个连接是扩展 " + old.ID + " 装出来的，不能改成手工连接；要换掉请删除后重新接入", nil
	case sp.Extension.ID != old.ID:
		return fmt.Sprintf("这个连接装的是扩展 %s，不能换成另一个扩展 %s；请把它作为新的扩展安装", old.ID, sp.Extension.ID), nil
	}
	return "", nil
}

// ---- 管理接口 ----

// extPackage 是上传的扩展：一个 zip 包（base64），或几个文件（相对路径 → 文本内容）。
type extPackage struct {
	Zip   string            `json:"zip"`
	Files map[string]string `json:"files"`
	// 服务扩展：服务地址，清单从它的 /.well-known/dsh-ee-extension.json 取
	ServiceURL string `json:"service_url"`
}

// readExtPackage 读上传的扩展，或从服务地址取清单。返回文件和（服务扩展的）服务地址。
func (s *Server) readExtPackage(ctx context.Context, p *extPackage) (map[string][]byte, string, error) {
	if strings.TrimSpace(p.ServiceURL) == "" {
		files, err := p.read()
		return files, "", err
	}
	svc, msg := cleanServiceURL(p.ServiceURL)
	if msg != "" {
		return nil, "", errors.New(msg)
	}
	raw, err := s.fetchExtManifest(ctx, svc)
	if err != nil {
		return nil, "", err
	}
	return map[string][]byte{extension.ManifestFile: raw}, svc, nil
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

	// 以下是服务扩展和 oauth 才有的
	Kind       string `json:"kind"`
	ServiceURL string `json:"service_url,omitempty"`
	// 管理员要填的配置项，和原来填的值（secret 只说填没填）
	Config       []extension.ConfigField `json:"config"`
	ConfigValues map[string]any          `json:"config_values,omitempty"`
	// 扩展实现了哪些约定接口、申请了哪些反过来调 dsh-ee 的权限
	Capabilities []string        `json:"capabilities"`
	Permissions  json.RawMessage `json:"permissions,omitempty"`
	// 会交给扩展什么凭据（docs/plugins.md §7.4），给管理员看清楚
	Handover []string `json:"handover"`
	// 要管理员填 OAuth 客户端（client_id / client_secret）；redirect 是要登记到对方系统的回调地址
	OAuthClient   bool   `json:"oauth_client"`
	OAuthClientID string `json:"oauth_client_id,omitempty"`
	// 扩展带的扩展字段，安装时建好
	Fields []extension.Field `json:"fields"`
	// 签名状态：unsigned / valid / invalid。默认不阻断安装；配了 DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1
	// 时没签名或验不过会作为错误写进 Check。
	Signature *extension.SignatureStatus `json:"signature"`
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
func (s *Server) planExtension(ctx context.Context, files map[string][]byte, serviceURL string) (*extPlan, error) {
	res := extension.CheckFiles(files)
	p := &extPlan{Check: res, AdminParams: []extension.AdminParam{}, Changes: []string{}, Sensitive: []string{},
		Config: []extension.ConfigField{}, Capabilities: []string{}, Handover: []string{}, Fields: []extension.Field{}, ServiceURL: serviceURL}
	trusted, err := s.trustedExtensionKeys(ctx)
	if err != nil {
		return nil, err
	}
	p.Signature = extension.VerifyFilesWithKeys(files, trusted)
	if d := signatureProblem(s.opts.ExtensionRequireSignature, p.Signature); d != nil {
		res.OK = false
		res.Errors++
		res.Results = append([]extension.Diagnostic{*d}, res.Results...)
	}
	m := res.Manifest
	if m == nil {
		return p, nil
	}
	p.Kind = m.Kind
	reject := func(code, msg, fix string) {
		if res.OK {
			res.OK = false
			res.Errors++
			res.Results = append([]extension.Diagnostic{{Code: code, Severity: extension.SevError, Where: extension.ManifestFile + "#/kind", Message: msg, Fix: fix}}, res.Results...)
		}
	}
	switch {
	case m.Kind == extension.KindLocal:
		reject("kind.unsupported", "端侧扩展是一个 npm 包，不在这里安装", "把包发布到企业 npm 源，在能力中心里按插件下发；见 docs/extension-local.md")
	case m.Kind == extension.KindService && serviceURL == "":
		reject("kind.unsupported", "服务扩展要填它的服务地址来安装，不是上传文件", "先把扩展部署好，再在「安装扩展 → 服务地址」里填它的地址")
	case m.Kind == extension.KindDeclarative && serviceURL != "":
		reject("kind.unsupported", "这个地址上的清单是声明式扩展，请用上传的方式安装", "")
	}
	if m.Kind == extension.KindService {
		p.Config = append(p.Config, m.Config...)
		p.Capabilities = append(p.Capabilities, m.Capabilities...)
		p.Permissions = m.Permissions
		p.Handover = append(p.Handover, "每次调用带一个 5 分钟有效的身份令牌（ee_jwt），扩展据此知道是哪位员工")
		if c := m.Credential; c != nil {
			switch c.Mode {
			case extension.CredShared:
				p.Handover = append(p.Handover, "管理员配的共享凭据会原样交给扩展（这类凭据没有短期形式）")
			case extension.CredUserSecret:
				p.Handover = append(p.Handover, "员工自己填的凭据会原样交给扩展（这类凭据没有短期形式）；能换成 ee_jwt 或 oauth 就换")
			case extension.CredOAuth:
				p.Handover = append(p.Handover, "员工授权后的 access token 会交给扩展；refresh token 和 client secret 留在 dsh-ee")
			}
			if c.Shared != nil && c.Mode != extension.CredShared {
				p.Handover = append(p.Handover, "另有一份管理员配的共享凭据会原样交给扩展")
			}
		}
		if len(m.Config) > 0 {
			p.Handover = append(p.Handover, "管理员填的配置（含密钥类型的配置项）每次调用都带给扩展")
		}
	}
	if m.Credential != nil && m.Credential.Mode == extension.CredOAuth && m.OAuth != nil {
		p.OAuthClient = true
	}
	p.Fields = append(p.Fields, m.Fields...)
	if msg := extensionCompat(m, s.opts.Version); msg != "" && res.OK {
		res.OK = false
		res.Errors++
		res.Results = append([]extension.Diagnostic{{Code: "min_version.too_new", Severity: extension.SevError, Where: extension.ManifestFile + "#/minVersion",
			Message: msg, Fix: "升级 dsh-ee，或装这个扩展的旧版本"}}, res.Results...)
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
	p.ConfigValues = osp.Extension.configView()
	if osp.Extension.OAuthClient != nil {
		p.OAuthClientID = osp.Extension.OAuthClient.ID
	}
	om, err := osp.Extension.manifest()
	if err != nil {
		return p, nil
	}
	if om.Kind != m.Kind {
		reject("kind.unsupported", fmt.Sprintf("已经装着的 %s 是 %s 扩展，不能直接换成 %s 扩展", m.ID, om.Kind, m.Kind), "先删掉原来的连接再装，或者换一个 id")
		return p, nil
	}
	if m.Kind == extension.KindService {
		if osp.Extension.ServiceURL != serviceURL {
			p.Sensitive = append(p.Sensitive, "服务地址从 "+osp.Extension.ServiceURL+" 换成了 "+serviceURL)
		}
		if string(om.Permissions) != string(m.Permissions) && len(m.Permissions) > 0 {
			var was, now map[string]any
			_ = json.Unmarshal(om.Permissions, &was)
			_ = json.Unmarshal(m.Permissions, &now)
			for k := range now {
				if _, had := was[k]; !had {
					p.Sensitive = append(p.Sensitive, "扩展新申请了反过来调 dsh-ee 的权限 "+k)
				}
			}
		}
	}
	if om.Version == m.Version && osp.Extension.Digest == res.Digest {
		p.Changes = append(p.Changes, "和已装的版本完全一样")
		return p, nil
	}
	if om.Version == m.Version {
		p.Sensitive = append(p.Sensitive, "版本号没变（"+m.Version+"），但清单或接口文档的内容变了")
	}
	oldFiles := map[string][]byte{extension.ManifestFile: []byte(osp.Extension.Manifest)}
	if om.OpenAPI != "" {
		oldFiles[om.OpenAPI] = []byte(osp.OpenAPI)
	}
	oldRes := extension.CheckFiles(oldFiles)
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

// signatureAudit 是写进审计的签名状态：状态和签名者的 key id，不含签名内容。
func signatureAudit(st *extension.SignatureStatus) string {
	if st == nil {
		return ""
	}
	if st.KeyID == "" {
		return st.Status
	}
	return st.Status + ":" + st.KeyID
}

// signatureProblem 按签名策略给出要报的错误。
// 撤销过的签名公钥对应的包一律不能新装 / 升级（不管有没有开强制签名）；默认「警告后允许」
// 下没签名、验不过只在计划里显示状态；配了强制验证签名时它们也不通过（fail closed）。
func signatureProblem(require bool, st *extension.SignatureStatus) *extension.Diagnostic {
	valid := st != nil && st.Status == extension.SignatureValid
	revoked := st != nil && st.Status == extension.SignatureRevoked
	if valid || (!require && !revoked) {
		return nil
	}
	reason := ""
	if st != nil {
		reason = st.Message
	}
	if reason == "" {
		reason = "这个扩展没有签名"
	}
	d := &extension.Diagnostic{
		Code:     "signature.required",
		Severity: extension.SevError,
		Where:    extension.SignatureFile,
		Message:  "dsh-ee 配成了必须验证签名（DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1），这个扩展不能安装：" + reason,
		Fix:      "让作者用 `dsh-ee extension sign` 签名，并把签名者的公钥加进受信任的签名公钥（管理 API / DSH_EE_EXTENSION_TRUSTED_KEYS）；或者由管理员关闭强制签名",
	}
	if revoked {
		d.Code = "signature.revoked"
		d.Message = "这个扩展的签名者的公钥已撤销，不能安装或升级：" + reason
		d.Fix = "让作者用受信任的新公钥重新签名；撤销过的公钥不能重新加回来"
	}
	return d
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
	files, svc, err := s.readExtPackage(r.Context(), &req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.planExtension(r.Context(), files, svc)
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
	// 服务扩展：清单 config 里各项的值；secret 留空表示沿用原来的
	Config map[string]json.RawMessage `json:"config"`
	// oauth：管理员在对方系统开放平台上建的应用；secret 留空表示沿用原来的
	OAuthClientID     string `json:"oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret"`
}

// extInstallRun 是「装 / 升级一个扩展」的执行输入。网页上传（handleExtInstall）和企业目录安装
// （directory_install.go）共用同一条执行路径：连接校验、管理员参数、凭据、OAuth、签名、审计
// 完全一致，区别只在包从哪里来、审计动作叫什么、返回里多带什么。
type extInstallRun struct {
	req   extInstallReq
	files map[string][]byte
	// 服务扩展的服务地址；从 zip 装的声明式扩展为空
	svc string
	p   *extPlan
	// 审计动作，空表示 extension.install；企业目录用 directory.install / directory.upgrade
	auditAction string
	// 审计目标，空表示 connection:<连接标识>；企业目录用 directory:<目录id>#<条目id>
	auditTarget string
	// 额外写进审计的字段（企业目录带登记来源）
	auditExtra map[string]any
}

// POST /api/admin/extensions/install：检查通过后装成连接；装过的就升级成新版本。
func (s *Server) handleExtInstall(w http.ResponseWriter, r *http.Request) {
	var req extInstallReq
	if !readJSONLimit(w, r, &req, 12<<20) {
		return
	}
	ctx := r.Context()
	files, svc, err := s.readExtPackage(ctx, &req.extPackage)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.planExtension(ctx, files, svc)
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
	resp, err := s.runExtInstall(ctx, r, &extInstallRun{req: req, files: files, svc: svc, p: p})
	if err != nil {
		s.writeProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// runExtInstall 执行扩展安装 / 升级，返回要写回的响应体。失败时返回带状态码的 httpProblem
// （其余错误按 500 处理）。会改库的动作都在最后：参数、连接、OAuth、管理员参数都校验通过才写。
func (s *Server) runExtInstall(ctx context.Context, r *http.Request, in *extInstallRun) (map[string]any, error) {
	p, files, req := in.p, in.files, in.req
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
			// 存下这次验签的材料和状态：升级后要回滚时，按当时的签名重新核对一遍（见 extensions_rollback.go）
			Signature: string(files[extension.SignatureFile]), SignatureStatus: signatureAudit(p.Signature),
		},
	}
	var osp *assetSpec
	var err error
	if p.Existing != nil {
		if _, osp, err = s.findExtension(ctx, m.ID); err != nil {
			return nil, err
		}
	}
	if p.OAuthClient {
		var old *extOAuthClient
		if osp != nil {
			old = osp.Extension.OAuthClient
		}
		client, msg := s.sealOAuthClient(req.OAuthClientID, req.OAuthClientSecret, old)
		if msg != "" {
			return nil, statusErr(http.StatusBadRequest, msg)
		}
		sp.Extension.OAuthClient = client
	}
	if m.Kind == extension.KindService {
		if msg := s.prepareServiceInstall(r, m, sp, osp, in.svc, &req); msg != "" {
			return nil, statusErr(http.StatusBadRequest, msg)
		}
	}
	if sp.URL == "" && p.Existing != nil {
		sp.URL = p.Existing.URL
	}
	if sp.URL == "" && len(m.Servers) == 1 {
		sp.URL = m.Servers[0]
	}
	if m.Kind == extension.KindService {
		sp.Auth, sp.UserAuth = serviceCredential(m)
	} else {
		sp.Auth, sp.UserAuth = extCredential(m.Credential, req.Auth)
	}
	if p.Existing != nil {
		// 升级：沿用原来的共享凭据放法（清单没规定时）、超时和使用说明
		if osp != nil {
			if m.Kind == extension.KindService {
				if sp.Auth.Type != "none" {
					sp.Auth.SharedName = sharedNameOf(osp)
				}
				sp.TimeoutSec, sp.Instructions = osp.TimeoutSec, osp.Instructions
				if req.Admin == nil {
					sp.Extension.Admin = osp.Extension.Admin
				}
			} else {
				if m.Credential == nil || (m.Credential.Mode == extension.CredShared && m.Credential.Header == "" && req.Auth == nil) {
					sp.Auth = osp.Auth
				}
				// 升级不改变管理员选的那一份命名共享凭据
				if sp.Auth != nil && sp.Auth.Type != "none" {
					sp.Auth.SharedName = sharedNameOf(osp)
				}
				sp.TimeoutSec, sp.Instructions = osp.TimeoutSec, osp.Instructions
				if req.Admin == nil {
					sp.Extension.Admin = osp.Extension.Admin
				}
			}
		}
	}
	if msg := validateSpec(store.KindConnection, sp); msg != "" {
		return nil, statusErr(http.StatusBadRequest, msg)
	}
	for _, ap := range p.AdminParams {
		if _, ok := sp.Extension.Admin[ap.Key]; !ok && len(ap.Default) == 0 {
			return nil, statusErr(http.StatusBadRequest, "参数「"+ap.Label+"」要由管理员设置，请填上")
		}
	}
	raw, _ := json.Marshal(sp)
	v := viewerOf(r)
	var a *store.Asset
	resp := map[string]any{"identity": m.Identity(), "digest": p.Check.Digest, "signature": p.Signature}
	if p.Existing != nil {
		if a, err = s.store.GetAsset(ctx, p.Existing.ID); err != nil {
			return nil, err
		}
		ver, err := s.store.AddAssetVersion(ctx, a.ID, raw, store.StatusPublished, v.user.ID)
		if err != nil {
			return nil, err
		}
		if err := s.store.UpdateAsset(ctx, a.ID, store.AssetPatch{Title: &sp.Title, Description: &sp.Description}); err != nil {
			return nil, err
		}
		resp["version"], resp["upgraded_from"] = ver, p.Existing.Version
	} else {
		slug := strings.TrimSpace(req.Slug)
		if slug == "" {
			slug = p.Slug
		}
		if msg := connSlugProblem(slug); msg != "" {
			return nil, statusErr(http.StatusBadRequest, msg)
		}
		scope := store.Scope{All: true}
		if req.Scope != nil {
			scope = req.Scope.Normalize()
		}
		owner := v.user.ID
		a = &store.Asset{Kind: store.KindConnection, Slug: slug, Title: sp.Title, Description: sp.Description, OwnerID: &owner, Scope: scope, Delivery: store.DeliveryOptional}
		if err := s.store.CreateAsset(ctx, a, raw, store.StatusPublished, v.user.ID); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return nil, statusErr(http.StatusConflict, "连接标识 "+slug+" 已被占用，换一个")
			}
			return nil, err
		}
		// 新建的连接默认给自己启用；订阅失败不影响创建，记录待查。
		if err := s.store.Subscribe(ctx, v.user.ID, a.ID); err != nil {
			s.log.Warn("自动订阅能力失败", "user", v.user.ID, "asset", a.ID, "err", err)
		}
	}
	if req.Secret != nil {
		if err := s.saveConnSecret(r, a.ID, *req.Secret); err != nil {
			return nil, err
		}
	}
	s.dropConnClient(a.ID)
	if n, err := s.syncConnectionTools(ctx, a.ID, v, s.callerBase(r)); err != nil {
		resp["sync_error"] = err.Error()
	} else {
		resp["tools"] = n
	}
	if sp.Extension.service() {
		resp["warnings"] = s.serviceToolDiff(ctx, a, sp, s.callerBase(r))
		if sp.Extension.Endpoint != "" {
			resp["endpoint_url"] = s.callerBase(r) + "/x/" + sp.Extension.Endpoint + "/"
		}
	}
	if sp.Extension.OAuthClient != nil {
		resp["oauth_redirect_uri"] = s.callerBase(r) + oauthCallbackPath
	}
	if created, err := s.installExtFields(ctx, m, v.user.ID); err != nil {
		resp["fields_error"] = err.Error()
	} else if len(created) > 0 {
		resp["fields"] = created
	}
	resp["asset"] = a
	detail := map[string]any{
		"id": a.ID, "extension": sp.Extension.label(), "upgrade": p.Existing != nil, "confirmed": p.Sensitive,
		"signature": signatureAudit(p.Signature),
	}
	for k, val := range in.auditExtra {
		detail[k] = val
	}
	action := in.auditAction
	if action == "" {
		action = auditExtInstall
	}
	target := in.auditTarget
	if target == "" {
		target = "connection:" + a.Slug
	}
	s.audit(r, action, target, detail)
	s.hub.notify()
	return resp, nil
}

// extCredential 把清单的凭据模式换成连接的共享凭据和员工身份设置。
func extCredential(c *extension.Credential, shared *connAuth) (*connAuth, *connUserAuth) {
	none := &connAuth{Type: "none"}
	if c == nil {
		return none, nil
	}
	if c.Mode == extension.CredShared {
		// 清单写了共享凭据放在哪个请求头的，以清单为准：对方要 X-API-Key 时不用管理员去猜
		if c.Header != "" {
			return &connAuth{Type: "header", Header: c.Header}, nil
		}
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
	case reservedConnSlug(slug):
		return "标识 " + slug + " 留给系统连接或内置工具，请换一个"
	}
	return ""
}
