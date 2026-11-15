package api

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/embed"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// settingsCache 在内存里留几秒系统设置：npm 安装时每个依赖都要查一次路由规则。
type settingsCache struct {
	mu sync.Mutex
	v  *store.AppSettings
	at time.Time
}

const settingsTTL = 5 * time.Second

// appSettings 返回系统设置（副本，调用方可以修改）。
func (s *Server) appSettings(ctx context.Context) (*store.AppSettings, error) {
	s.settings.mu.Lock()
	defer s.settings.mu.Unlock()
	if s.settings.v != nil && time.Since(s.settings.at) < settingsTTL {
		return s.settings.v.Clone(), nil
	}
	v, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	s.settings.v, s.settings.at = v, time.Now()
	return v.Clone(), nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	v, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 嵌入来源/应用白名单是管理端信息：没有 settings.manage 的人（含嵌入会话）看不到。
	if !currentAccess(r).Has(rbac.SettingsManage) {
		v.Embed = store.AppSettings{}.Embed
	}
	writeJSON(w, http.StatusOK, v)
}

// GET /api/admin/embed-settings：管理端嵌入设置的「部署基线 + 有效值 + 数据库值」。
// env 只读；页面里展示锁定项，保存仍走 PUT /api/admin/settings 的 embed 段。
func (s *Server) handleAdminEmbedSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.embedConfig(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	envTTL := s.opts.EmbedSessionTTL
	if envTTL <= 0 {
		envTTL = defaultEmbedSessionTTL
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env": map[string]any{
			"crossSite":      s.opts.EmbedCrossSite,
			"allowedOrigins": nonNilStrings(s.opts.EmbedOrigins),
			"sessionSeconds": int(envTTL.Seconds()),
		},
		"effective": embedConfigView(cfg),
		"settings":  set.Embed,
	})
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// ValidateStoredSettings 在启动时校验库里的设置：手工改库写入非法嵌入来源等值会让服务拒绝启动。
func ValidateStoredSettings(ctx context.Context, st *store.Store) error {
	v, err := st.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("读取系统设置: %w", err)
	}
	if msg := validateSettings(v); msg != "" {
		return errors.New("系统设置无效：" + msg)
	}
	return nil
}

// 每一段设置由哪个权限点管：npm 源页面能改路由与上游，系统设置页能改全部。
func sectionPerms(field string) []string {
	switch field {
	case "Scope", "Registry":
		return []string{rbac.SettingsManage, rbac.RegistryManage}
	case "Upstream":
		return []string{rbac.SettingsManage, rbac.RegistryManage, rbac.UpstreamManage}
	case "Knowledge":
		return []string{rbac.SettingsManage, rbac.KnowledgeManage}
	default:
		return []string{rbac.SettingsManage}
	}
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 请求里没带的段落保持原样（旧版网页不认识的新段落，例如 knowledge，不会被清空）
	next := *cur.Clone()
	// JSON 解进已有的 map 只会增加键，删不掉颜色；先清空，请求里没带再用原来的
	next.Brand.Theme.Colors, next.Brand.Theme.Tokens = nil, nil
	if !readJSON(w, r, &next) {
		return
	}
	if next.Brand.Theme.Colors == nil {
		next.Brand.Theme.Colors = maps.Clone(cur.Brand.Theme.Colors)
	}
	if next.Brand.Theme.Tokens == nil {
		next.Brand.Theme.Tokens = maps.Clone(cur.Brand.Theme.Tokens)
	}
	if msg := validateSettings(&next); msg != "" {
		writeErrCode(w, http.StatusBadRequest, codeSettingsInvalidValue, nil, msg)
		return
	}
	// 认证方式由 LDAP 配置的开关决定；LogoRev 只能由 logo 上传接口更新。
	next.Auth = cur.Auth
	next.Brand.LogoRev = cur.Brand.LogoRev
	// 只检查实际改动的段落
	a := currentAccess(r)
	cv, nv := reflect.ValueOf(*cur), reflect.ValueOf(next)
	var changed []string
	for i := 0; i < cv.NumField(); i++ {
		name := cv.Type().Field(i).Name
		if reflect.DeepEqual(cv.Field(i).Interface(), nv.Field(i).Interface()) {
			continue
		}
		if !a.Has(sectionPerms(name)...) {
			field := cv.Type().Field(i).Tag.Get("json")
			writeErrCode(w, http.StatusForbidden, codeSettingsForbidden, map[string]any{"field": field}, "没有权限修改「"+field+"」设置")
			return
		}
		changed = append(changed, cv.Type().Field(i).Tag.Get("json"))
	}
	if err := s.store.SaveSettings(r.Context(), &next, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.settings.mu.Lock()
	s.settings.v = nil
	s.settings.mu.Unlock()
	if len(changed) > 0 {
		s.audit(r, auditSettingsUpdate, strings.Join(changed, ", "), map[string]any{"sections": changed})
		s.hub.notify() // Agent 总开关等设置会改变工具与系统技能
		s.kickConnect()
		if slices.Contains(changed, "distribution") {
			s.kickDshReleases()
		}
	}
	writeJSON(w, http.StatusOK, &next)
}

func validateSettings(v *store.AppSettings) string {
	v.OrgName = strings.TrimSpace(v.OrgName)
	v.BaseURL = strings.TrimRight(strings.TrimSpace(v.BaseURL), "/")
	if len([]rune(v.OrgName)) > 64 {
		return "组织名称不能超过 64 个字符"
	}
	if v.BaseURL != "" {
		u, err := url.Parse(v.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "访问地址必须是 http(s):// 开头的完整地址"
		}
	}
	if !registry.ValidScope(v.Scope) {
		return "企业作用域格式不对，例如 @acme"
	}
	if v.Registry.ExtraScopes == nil {
		v.Registry.ExtraScopes = []string{}
	}
	for _, sc := range v.Registry.ExtraScopes {
		if !registry.ValidScope(sc) {
			return "内部作用域 " + sc + " 格式不对"
		}
	}
	if slices.Contains(v.Registry.ExtraScopes, v.Scope) {
		return "内部作用域与主作用域重复"
	}
	if len(v.Registry.ExtraScopes) > 20 {
		return "内部作用域最多 20 个"
	}
	checks := []struct {
		val, what string
		allowed   []string
	}{
		{v.AdmissionMode, "收录模式", []string{"allowlist", "open"}},
		{v.Upstream.Mode, "上游模式", []string{"online", "offline"}},
		{v.Upstream.Registry, "上游 registry", []string{"npmjs", "npmmirror", "custom"}},
		{v.Upstream.AutoSync, "新版本同步", []string{"auto", "review", "off"}},
		{v.Registry.ProxyMode, "回源模式", []string{"cache", "preloaded"}},
		{v.Auth, "认证方式", []string{"local", "ldap", "oidc"}},
	}
	for _, c := range checks {
		if !slices.Contains(c.allowed, c.val) {
			return c.what + "取值不对：" + c.val
		}
	}
	if v.Upstream.Registry == "custom" {
		u, err := url.Parse(v.Upstream.CustomRegistry)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return "自定义上游 registry 必须是 http(s) 地址，且不能在地址里带账号密码"
		}
		v.Upstream.CustomRegistry = strings.TrimRight(v.Upstream.CustomRegistry, "/")
	}
	if !slices.Contains([]string{"off", "notify", "mirror"}, v.Distribution.AutoSync) {
		return "Desktop 自动同步取值不对：" + v.Distribution.AutoSync
	}
	if v.Distribution.KeepVersions < 1 || v.Distribution.KeepVersions > 10 {
		return "Desktop 保留版本数应在 1–10 之间"
	}
	v.Brand.Name = strings.TrimSpace(v.Brand.Name)
	nameLen := len([]rune(v.Brand.Name))
	if v.Brand.Enabled && nameLen == 0 {
		return "启用企业品牌时必须填写名称"
	}
	if nameLen > 24 {
		return "品牌名称不能超过 24 个字符"
	}
	if strings.Contains(strings.ToLower(strings.Join(strings.Fields(v.Brand.Name), "")), "deepseekharness") {
		return "按 DSH 品牌规范，不能使用 DeepSeek Harness 全名，可以写『某某 AI · 基于 DSH』"
	}
	if msg := validateBrandLook(v); msg != "" {
		return msg
	}
	if v.Connect.PollSeconds < 15 || v.Connect.PollSeconds > 3600 {
		return "DSH 接入插件的检查间隔应在 15–3600 秒之间"
	}
	if v.Connect.DeviceTokenDays < 1 || v.Connect.DeviceTokenDays > 3650 {
		return "设备登录令牌有效天数应在 1–3650 天之间"
	}
	v.Connect.DefaultModel = strings.TrimSpace(v.Connect.DefaultModel)
	if v.Scoring.MinPreloadScore < 0 || v.Scoring.MinPreloadScore > 100 {
		return "预载最低评分应在 0–100 之间"
	}
	// 嵌入来源：与管理端页面、环境变量同一套规则（完整 origin；非法值拒绝保存/启动）。
	if len(v.Embed.AllowedOrigins) > 32 {
		return "嵌入来源最多 32 个"
	}
	origins := make([]string, 0, len(v.Embed.AllowedOrigins))
	seenOrigin := map[string]bool{}
	for _, raw := range v.Embed.AllowedOrigins {
		o, err := embed.NormalizeOrigin(raw)
		if err != nil {
			return "嵌入来源 " + raw + " 不合法：" + err.Error()
		}
		if seenOrigin[o] {
			continue
		}
		seenOrigin[o] = true
		origins = append(origins, o)
	}
	v.Embed.AllowedOrigins = origins
	if len(v.Embed.Apps) > 100 {
		return "嵌入应用白名单最多 100 个"
	}
	apps := make([]string, 0, len(v.Embed.Apps))
	seenApp := map[string]bool{}
	for _, raw := range v.Embed.Apps {
		app := strings.TrimSpace(raw)
		if app == "" {
			continue
		}
		if !slugRe.MatchString(app) || len(app) > maxSlug {
			return "嵌入应用标识 " + app + " 不合法：只能用小写字母、数字和连字符"
		}
		if seenApp[app] {
			continue
		}
		seenApp[app] = true
		apps = append(apps, app)
	}
	v.Embed.Apps = apps
	if v.Embed.SessionMinutes != 0 && (v.Embed.SessionMinutes < 1 || v.Embed.SessionMinutes > 480) {
		return "嵌入会话有效期应在 1–480 分钟之间（0 表示跟随部署配置）"
	}
	return ""
}
