package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 管理员向导用的接口：首次部署清单、日常巡检、审核时读内容、处理连接申请。
// 网页与 Agent 工具（ee_settings.setup_status、ee_stats.attention、ee_review.inspect_*、ee_connections_admin.*）共用。

type setupStep struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Done     bool   `json:"done"`
	Optional bool   `json:"optional,omitempty"`
	Detail   string `json:"detail,omitempty"`
	// 下一步怎么做：用哪个工具，或必须去网页
	How string `json:"how,omitempty"`
}

// GET /api/admin/setup-status：首次部署清单，每一步是否已经完成。
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	set, err := s.appSettings(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var steps []setupStep
	add := func(st setupStep) { steps = append(steps, st) }

	base := set.BaseURL
	source := "系统设置"
	if base == "" {
		base, source = s.opts.PublicURL, "启动参数 DSH_EE_PUBLIC_URL"
	}
	if base != "" {
		add(setupStep{ID: "base_url", Title: "访问地址", Done: true, Detail: base + "（来自" + source + "）"})
	} else {
		add(setupStep{ID: "base_url", Title: "访问地址", How: "ee_settings.put，changes={\"baseUrl\":\"https://…\"}：员工的 DSH 和 npm 都用它连回来，要写员工电脑能访问到的地址"})
	}

	providers, err := s.store.ListProviders(ctx, false)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	enabled, unhealthy := 0, 0
	for _, p := range providers {
		if p.Enabled {
			enabled++
			if p.LastCheckOK != nil && !*p.LastCheckOK {
				unhealthy++
			}
		}
	}
	switch {
	case enabled == 0:
		add(setupStep{ID: "provider", Title: "模型供应商", How: "必须在网页「模型接入」里接入：需要上游 API key，机密不经过 Agent"})
	case unhealthy > 0:
		add(setupStep{ID: "provider", Title: "模型供应商", Detail: fmt.Sprintf("%d 个启用的供应商，其中 %d 个最近检测不通", enabled, unhealthy), How: "ee_models.check_provider 重新检测；API key 有问题要在网页上改"})
	default:
		add(setupStep{ID: "provider", Title: "模型供应商", Done: true, Detail: fmt.Sprintf("%d 个启用的供应商", enabled)})
	}

	models, err := s.store.PublicModels(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	switch {
	case len(names) == 0:
		add(setupStep{ID: "models", Title: "上架对外模型", How: "ee_models.create_models 在供应商下上架模型"})
	default:
		add(setupStep{ID: "models", Title: "上架对外模型", Done: true, Detail: strings.Join(names, "、")})
	}
	switch {
	case set.Connect.DefaultModel == "":
		add(setupStep{ID: "default_model", Title: "默认模型", How: "ee_settings.put，changes={\"connect\":{\"defaultModel\":\"<对外模型名>\"}}；可选：" + strings.Join(names, "、")})
	case !slices.Contains(names, set.Connect.DefaultModel):
		add(setupStep{ID: "default_model", Title: "默认模型", Detail: "默认模型 " + set.Connect.DefaultModel + " 不在已上架的模型里", How: "ee_settings.put 改成已上架的模型：" + strings.Join(names, "、")})
	default:
		add(setupStep{ID: "default_model", Title: "默认模型", Done: true, Detail: set.Connect.DefaultModel})
	}

	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	roleList, err := s.store.ListRoles(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	superRoles := map[int64]bool{}
	for _, role := range roleList {
		if role.IsSuper {
			superRoles[role.ID] = true
		}
	}
	refs, err := s.store.UserRoleRefs(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	depts := map[string]bool{}
	canUse, zero, humans := 0, 0, 0
	for _, u := range users {
		if u.AccountType != store.AccountHuman || u.Disabled {
			continue
		}
		// 管理员自己的账号不限额度，不算「员工额度已分配」
		if slices.ContainsFunc(refs[u.ID], func(ref store.RoleRef) bool { return superRoles[ref.ID] }) {
			continue
		}
		humans++
		if u.Department != "" {
			depts[u.Department] = true
		}
		if u.MonthlyTokenLimit != nil && *u.MonthlyTokenLimit == 0 {
			zero++
		} else {
			canUse++
		}
	}
	if canUse > 0 {
		detail := fmt.Sprintf("%d 个账号有额度", canUse)
		if zero > 0 {
			detail += fmt.Sprintf("，%d 个还是 0（新账号默认不能调用模型）", zero)
		}
		add(setupStep{ID: "quota", Title: "员工额度", Done: true, Detail: detail})
	} else {
		add(setupStep{ID: "quota", Title: "员工额度", Detail: fmt.Sprintf("%d 个员工账号，都还没分配额度", humans), How: "ee_users.set_quota 按人分配每月额度（新账号默认 0，不能调用模型）"})
	}

	ldap, err := s.loadLDAP(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if ldap.Enabled && ldap.URL != "" {
		add(setupStep{ID: "ldap", Title: "LDAP / AD 登录", Optional: true, Done: true, Detail: ldap.URL})
	} else {
		add(setupStep{ID: "ldap", Title: "LDAP / AD 登录", Optional: true, How: "可选。服务地址、BaseDN 等用 ee_settings.ldap_put；绑定密码只能在网页上填；之后 ee_settings.ldap_test 试连"})
	}

	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	custom := 0
	for _, role := range roles {
		if !role.Builtin {
			custom++
		}
	}
	add(setupStep{ID: "roles", Title: "部门与角色", Optional: true, Done: custom > 0 || len(depts) > 0,
		Detail: fmt.Sprintf("%d 个自定义角色，%d 个部门", custom, len(depts)), How: "可选。ee_roles.create 建角色、ee_users.create / set_roles 分配"})

	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	conns := 0
	for _, a := range assets {
		if a.Kind == store.KindConnection && a.Status == store.AssetActive && a.LiveVersion > 0 {
			conns++
		}
	}
	add(setupStep{ID: "connections", Title: "企业连接", Optional: true, Done: conns > 0, Detail: fmt.Sprintf("%d 个", conns),
		How: "可选。新建连接要上游凭据，请在网页「企业连接」里接入；接入后可用 ee_connections_admin 试连、开关工具"})
	if _, imported := s.demoState(ctx); imported {
		add(setupStep{ID: "demo", Title: "示例公司", Optional: true, Done: true, Detail: "已导入示例公司「星河科技」：6 个部门知识库、差旅 / CRM / IT 工单三个演示系统、应用、10 个演示员工和 30 天的使用数据。转正式使用前可以一键清除"})
	} else {
		add(setupStep{ID: "demo", Title: "示例公司", Optional: true,
			How: "可选。一键导入示例公司「星河科技」：6 个部门的知识库、差旅 / CRM / IT 工单三个演示系统、带工作台的应用、10 个演示员工和 30 天的使用数据，打开 DSH 就能试。只能由超级管理员在网页部署清单里导入，试完可以一键清除"})
	}

	var st connectState
	if _, err := s.store.GetSettingJSON(ctx, connectStateKey, &st); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ready := false
	if st.Package == connectPackage(set) && st.Version != "" {
		if v, err := s.store.GetVersion(ctx, st.Package, st.Version); err == nil && v.Status == store.StatusPublished {
			ready = true
		}
	}
	switch {
	case !set.Connect.Enabled:
		add(setupStep{ID: "connect", Title: "DSH 接入插件", Detail: "DSH 接入已被关闭", How: "ee_settings.put，changes={\"connect\":{\"enabled\":true}}"})
	case ready:
		add(setupStep{ID: "connect", Title: "DSH 接入插件", Done: true, Detail: st.Package + "@" + st.Version})
	default:
		add(setupStep{ID: "connect", Title: "DSH 接入插件", Detail: nonEmpty(st.Error, "还没有发布"), How: "ee_settings.connect_publish 发布；前面几步（访问地址、默认模型）改过之后也要重新发布"})
	}

	next, requiredDone := "", true
	for _, st := range steps {
		if !st.Done && !st.Optional {
			requiredDone = false
			if next == "" {
				next = st.ID
			}
		}
	}
	resp := map[string]any{"steps": steps, "required_done": requiredDone, "next": next}
	if base != "" {
		resp["install_command"] = fmt.Sprintf("curl -fsSL %s/setup.sh | sh", strings.TrimRight(base, "/"))
		resp["onboarding"] = fmt.Sprintf("各位同事：公司的 AI 工作台已经开通。请在终端运行下面这行命令，按提示在浏览器里点一次「授权」即可在 DSH 里使用模型、技能和企业系统（Windows 用户请到 %s/setup 查看 PowerShell 命令）。\n\n  curl -fsSL %s/setup.sh | sh\n\n额度与能力范围由管理员分配，遇到问题可以在 DSH 里输入 /dsh-ee-doctor 自助诊断。",
			strings.TrimRight(base, "/"), strings.TrimRight(base, "/"))
	}
	writeJSON(w, http.StatusOK, resp)
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

type attentionItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Count  int    `json:"count"`
	Detail any    `json:"detail,omitempty"`
	How    string `json:"how,omitempty"`
}

// GET /api/admin/attention：日常巡检——今天有什么要处理的。按调用者的权限只给他管得了的部分。
func (s *Server) handleAttention(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a := currentAccess(r)
	items := []attentionItem{}
	add := func(it attentionItem) {
		if it.Count > 0 {
			items = append(items, it)
		}
	}

	if a.Has(rbac.PluginsManage) {
		pending, err := s.store.PendingAssetVersions(ctx)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var titles []string
		for _, v := range pending {
			if len(titles) < 5 {
				if as, err := s.store.GetAsset(ctx, v.AssetID); err == nil {
					titles = append(titles, fmt.Sprintf("%s「%s」v%d", as.Kind, as.Title, v.Version))
				}
			}
		}
		add(attentionItem{ID: "review_assets", Title: "待审核的技能 / 提示词 / 微应用", Count: len(pending), Detail: titles, How: "ee_review.queue_assets，再用 inspect_asset 逐个看"})
		pv, err := s.store.PendingVersions(ctx)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var names []string
		for i, v := range pv {
			if i < 5 {
				names = append(names, v.Name+"@"+v.Version)
			}
		}
		add(attentionItem{ID: "review_packages", Title: "待审核的插件版本", Count: len(pv), Detail: names, How: "ee_review.queue_packages，再用 inspect_package 看内容"})
		all, err := s.store.ListAssets(ctx)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var scoped []string
		for _, as := range all {
			if as.PendingScope != nil {
				scoped = append(scoped, fmt.Sprintf("%s「%s」", as.Kind, as.Title))
			}
		}
		add(attentionItem{ID: "review_scopes", Title: "申请扩大可用范围的能力", Count: len(scoped), Detail: firstN(scoped, 5), How: "ee_review.queue_assets 的 scopes，再用 decide_scope 通过或驳回"})
	}
	if a.Has(rbac.ConnectionsManage) {
		n, err := s.store.CountPendingAccessRequests(ctx, store.RequestConnection)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		add(attentionItem{ID: "connection_requests", Title: "员工申请使用企业连接", Count: n, How: "ee_connections_admin.requests 查看，resolve 通过或驳回"})
		health, err := s.store.ConnectionHealthSince(ctx, time.Now().Add(-24*time.Hour))
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var bad []map[string]any
		for _, h := range health {
			if h.Calls >= 5 && h.Errors*5 >= h.Calls {
				bad = append(bad, map[string]any{"connection": h.Slug, "calls": h.Calls, "errors": h.Errors})
			}
		}
		add(attentionItem{ID: "connection_errors", Title: "近 24 小时失败率 ≥ 20% 的企业连接", Count: len(bad), Detail: bad, How: "ee_connections_admin.test 试连，sync 重新同步工具"})
	}
	if a.Has(rbac.QuotasManage) || a.Has(rbac.UsersManage) || a.Has(rbac.UsersView) {
		users, err := s.store.ListUsers(ctx)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var near []map[string]any
		for _, u := range users {
			if u.Disabled || u.AccountType != store.AccountHuman || u.MonthlyTokenLimit == nil || *u.MonthlyTokenLimit <= 0 {
				continue
			}
			if u.MonthTokens*10 >= *u.MonthlyTokenLimit*9 {
				near = append(near, map[string]any{"username": u.Username, "used": u.MonthTokens, "limit": *u.MonthlyTokenLimit, "percent": u.MonthTokens * 100 / *u.MonthlyTokenLimit})
			}
		}
		sort.Slice(near, func(i, j int) bool { return near[i]["percent"].(int64) > near[j]["percent"].(int64) })
		total := len(near)
		if len(near) > 10 {
			near = near[:10]
		}
		add(attentionItem{ID: "quota_near", Title: "本月额度已用 90% 以上的员工", Count: total, Detail: near, How: "ee_users.set_quota 追加额度"})
	}
	if a.Has(rbac.UpstreamManage) {
		jobs, err := s.store.ListJobs(ctx, 50)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var failed []map[string]any
		for _, j := range jobs {
			if j.CreatedAt.After(time.Now().AddDate(0, 0, -7)) && (j.Status == "failed" || j.Failed > 0) {
				failed = append(failed, map[string]any{"id": j.ID, "kind": j.Kind, "status": j.Status, "failed": j.Failed, "total": j.Total, "at": j.CreatedAt})
			}
		}
		add(attentionItem{ID: "sync_failed", Title: "近 7 天有失败的上游同步任务", Count: len(failed), Detail: failed, How: "ee_registry.job 看详情，必要时重新 sync"})
	}
	if a.Has(rbac.ProvidersManage) {
		ps, err := s.store.ListProviders(ctx, false)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		var bad []string
		for _, p := range ps {
			if p.Enabled && p.LastCheckOK != nil && !*p.LastCheckOK {
				bad = append(bad, p.Name+"："+p.LastCheckMsg)
			}
		}
		add(attentionItem{ID: "provider_unhealthy", Title: "检测不通的模型供应商", Count: len(bad), Detail: bad, How: "ee_models.check_provider 重新检测"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "checked_at": time.Now().UTC()})
}

// ---- 审核时读内容 ----

type reviewFlag struct {
	Kind     string `json:"kind"`
	File     string `json:"file,omitempty"`
	Evidence string `json:"evidence"`
}

// 启发式的可疑点：只是提醒审核的人去看，不是结论。
var flagPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"shell-command", regexp.MustCompile(`(?i)\b(curl|wget)\s+[^\n]{0,60}|\b(rm\s+-rf|chmod\s+[0-7+x]+|sudo\s+\w+|Invoke-WebRequest|powershell\s+-)[^\n]{0,40}|\b(bash|sh)\s+-c\b[^\n]{0,40}`)},
	{"network-code", regexp.MustCompile(`\b(XMLHttpRequest|WebSocket|net\.connect|dgram|http\.request|https\.request)\b|\bfetch\s*\(\s*['"` + "`" + `]https?://[^'"` + "`" + `]{0,80}`)},
	{"process-exec", regexp.MustCompile(`\b(child_process|execSync|spawnSync|eval\s*\(|new Function\s*\()[^\n]{0,40}`)},
	{"reads-env-or-home", regexp.MustCompile(`process\.env\b[^\n]{0,40}|\.ssh/|\.npmrc|\.aws/credentials|~/\.|\$HOME/\.`)},
	{"external-url", regexp.MustCompile(`https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]{6,80}`)},
	{"prompt-injection", regexp.MustCompile(`(?i)ignore (all |any )?(the )?(previous|above|prior) (instructions|rules)|disregard (the )?(previous|above)|do not (tell|mention|reveal)[^\n]{0,30}user|忽略(以上|之前|前面|上述)的?(所有)?(指令|提示|规则)|不要(告诉|让)用户知道`)},
	{"data-exfiltration", regexp.MustCompile(`(?i)(send|post|upload|forward|发送|上传|提交)[^\n]{0,40}(to|到)\s*https?://[^\s]{4,60}`)},
	{"credential-handling", regexp.MustCompile(`(?i)\b(api[_-]?key|password|passwd|secret|token)\b\s*[:=]\s*\S{4,}|(?i)(读取|获取|询问|ask for)[^\n]{0,20}(密码|令牌|密钥|password|token|api key)`)},
}

var credentialValue = regexp.MustCompile(`(?i)(api[_-]?key|password|passwd|secret|token)\s*[:=]\s*\S+`)

func reviewFlags(file, text string, perKind map[string]int) []reviewFlag {
	var out []reviewFlag
	for _, p := range flagPatterns {
		if perKind[p.kind] >= 5 {
			continue
		}
		if m := p.re.FindString(text); m != "" {
			perKind[p.kind]++
			ev := strings.TrimSpace(m)
			if p.kind == "credential-handling" {
				// 证据里不带值，免得把正文里写死的密码当「证据」转给模型
				ev = credentialValue.ReplaceAllString(ev, "$1=***")
			}
			out = append(out, reviewFlag{Kind: p.kind, File: file, Evidence: clip(ev, 100)})
		}
	}
	return out
}

var installHooks = []string{"preinstall", "install", "postinstall", "prepare", "prepublish", "prepack", "postpack"}

// GET /api/admin/review/inspect?name=&version=&file=：看一个插件版本的内容——清单、脚本、依赖、疑似密钥、可疑点，以及指定文件的文本。
func (s *Server) handleInspectPackage(w http.ResponseWriter, r *http.Request) {
	if !s.registryReady(w) {
		return
	}
	q := r.URL.Query()
	name, version, file := q.Get("name"), q.Get("version"), strings.TrimPrefix(q.Get("file"), "/")
	if !registry.ValidName(name) {
		writeErrCode(w, http.StatusBadRequest, codeAdminSetupInvalidPackageName, nil, "非法的包名")
		return
	}
	v, err := s.store.GetVersion(r.Context(), name, version)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeAdminSetupVersionNotFound, nil, "版本不存在")
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
	flags := []reviewFlag{}
	perKind := map[string]int{}
	files := map[string]bool{}
	contents, err := registry.ReadTarball(f, file, 12<<10, func(rel string, b []byte) {
		if len(files) >= 40 {
			return
		}
		if fl := reviewFlags(rel, string(b), perKind); len(fl) > 0 {
			files[rel] = true
			flags = append(flags, fl...)
		}
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "读取包内容失败："+err.Error())
		return
	}
	manifest := dec(contents.PackageJSON)
	scripts, _ := manifest["scripts"].(map[string]any)
	hooks := map[string]any{}
	for _, h := range installHooks {
		if c, ok := scripts[h]; ok {
			hooks[h] = clip(fmt.Sprint(c), 200)
			flags = append(flags, reviewFlag{Kind: "install-script", File: "package.json", Evidence: h + ": " + clip(fmt.Sprint(c), 100)})
		}
	}
	resp := map[string]any{
		"name": name, "version": version, "status": v.Status, "size": v.Size, "is_plugin": v.IsPlugin,
		"files": contents.Files, "total_files": contents.Total, "files_truncated": contents.Truncated,
		"install_scripts": hooks, "scripts": scripts,
		"dependencies": manifest["dependencies"], "peerDependencies": manifest["peerDependencies"],
		"bin": manifest["bin"], "main": manifest["main"], "dsh": manifest["dsh"],
		"scan": v.Scan, "flags": flags,
		"note": "flags 是启发式的可疑点，只提示去哪里看，不是结论；scan 是上架前的自动检查（疑似密钥只报类型和文件）。",
	}
	if contents.File != nil {
		resp["file"] = contents.File
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/admin/hub/inspect?asset_id=&version=：看一个技能 / 提示词 / 微应用版本的全文与可疑点，并给出线上版本作对照。
func (s *Server) handleInspectAsset(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, _ := strconv.ParseInt(q.Get("asset_id"), 10, 64)
	ver, _ := strconv.Atoi(q.Get("version"))
	a, err := s.store.GetAsset(r.Context(), id)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeAdminSetupAssetNotFound, nil, "能力不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !viewerOf(r).manages(a.Kind) {
		writeErrCode(w, http.StatusForbidden, codeAdminSetupReviewForbidden, nil, "没有权限审核这类能力")
		return
	}
	av, err := s.store.GetAssetVersion(r.Context(), a.ID, ver)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeAdminSetupVersionNotFound, nil, "版本不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	sp := parseSpec(av.Spec)
	trim := func(sp *assetSpec) map[string]any {
		return map[string]any{"title": sp.Title, "description": sp.Description, "when_to_use": sp.WhenToUse,
			"body": clip(sp.Body, 12000), "persona": clip(sp.Persona, 8000), "starters": sp.Starters,
			"skills": sp.Skills, "connections": sp.Connections, "plugins": sp.Plugins}
	}
	flags := []reviewFlag{}
	perKind := map[string]int{}
	for field, text := range map[string]string{"body": sp.Body, "persona": sp.Persona, "description": sp.Description, "when_to_use": sp.WhenToUse, "starters": strings.Join(sp.Starters, "\n")} {
		flags = append(flags, reviewFlags(field, text, perKind)...)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Kind+flags[i].File < flags[j].Kind+flags[j].File })
	resp := map[string]any{
		"asset":   pick(dec(mustJSON(a)), "id", "kind", "slug", "title", "category", "scope", "delivery", "status"),
		"version": av.Version, "status": av.Status, "spec": trim(sp), "flags": flags,
		"note": "flags 是启发式的可疑点，只提示去哪里看，不是结论。",
	}
	if live, err := s.store.LiveAssetVersion(r.Context(), a.ID); err == nil && live.Version != av.Version {
		resp["live_version"] = live.Version
		resp["live_spec"] = trim(parseSpec(live.Spec))
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- 连接申请的处理 ----

// GET /api/admin/connection-requests：待处理的连接申请（技能、知识库的申请在 /api/me/access-requests 里）。
func (s *Server) handleListConnectionRequests(w http.ResponseWriter, r *http.Request) {
	reqs, err := s.store.PendingAccessRequests(r.Context(), 100, store.RequestConnection)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": reqs})
}

// POST /api/admin/connection-requests/{id}：{decision: approve|reject, note}。
// 通过 = 把申请人加进这个连接的可用范围（范围本来是全员就不用动）；驳回只关闭申请。
func (s *Server) handleResolveConnectionRequest(w http.ResponseWriter, r *http.Request) {
	s.resolveAccessRequest(w, r, store.RequestConnection)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}
