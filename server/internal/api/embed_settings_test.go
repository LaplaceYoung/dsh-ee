package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const embedHostB = "https://portal.example.com"

// htmlStub 让服务端像真实部署一样返回 HTML，用来断言 CSP 头。
func htmlStub() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html></html>"))
	})
}

func cspOf(t *testing.T, base, path string) string {
	t.Helper()
	resp, err := http.Get(base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.Header.Get("Content-Security-Policy")
}

func (f *embedFixture) adminSettings(t *testing.T) map[string]any {
	t.Helper()
	code, body := f.admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	return decode[map[string]any](t, body)
}

// saveSettings 以管理员身份整份保存设置（在现有设置上改 embed 段）。
func (f *embedFixture) saveSettings(t *testing.T, embedPatch map[string]any) (int, []byte) {
	t.Helper()
	cur := f.adminSettings(t)
	embed, _ := cur["embed"].(map[string]any)
	if embed == nil {
		embed = map[string]any{}
	}
	for k, v := range embedPatch {
		if v == nil {
			delete(embed, k)
		} else {
			embed[k] = v
		}
	}
	cur["embed"] = embed
	return f.admin.do("PUT", "/api/admin/settings", cur, nil)
}

func TestEmbedSettingsPermissionAndVisibility(t *testing.T) {
	f := newEmbedFixture(t, func(o *api.Options) { o.Web = htmlStub() })
	f.user(t, "embed-settings-emp")
	empClient := newClient(t, f.ts.URL)
	code, body := empClient.do("POST", "/api/auth/login", map[string]string{"username": "embed-settings-emp", "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	// 普通员工看不到管理端嵌入段，也改不了设置
	code, body = empClient.do("GET", "/api/settings", nil, nil)
	expect(t, "员工读设置", code, 200, body)
	var empSet store.AppSettings
	if err := json.Unmarshal(body, &empSet); err != nil {
		t.Fatal(err)
	}
	if empSet.Embed.CrossSite != nil || len(empSet.Embed.AllowedOrigins) != 0 || len(empSet.Embed.Apps) != 0 {
		t.Fatalf("非管理员不应看到嵌入设置：%+v", empSet.Embed)
	}
	if code, body := empClient.do("GET", "/api/admin/embed-settings", nil, nil); code != 403 {
		t.Fatalf("非管理员不能读嵌入有效配置：%d %s", code, body)
	}
	if code, body := empClient.do("PUT", "/api/admin/settings", f.adminSettings(t), nil); code != 403 {
		t.Fatalf("非管理员不能保存设置：%d %s", code, body)
	}

	// 管理员能读「环境基线 + 有效值」，并看到数据库段为空
	code, body = f.admin.do("GET", "/api/admin/embed-settings", nil, nil)
	expect(t, "管理员读嵌入有效配置", code, 200, body)
	var view struct {
		Env struct {
			CrossSite      bool     `json:"crossSite"`
			AllowedOrigins []string `json:"allowedOrigins"`
			SessionSeconds int      `json:"sessionSeconds"`
		}
		Effective struct {
			CrossSite      bool     `json:"crossSite"`
			AllowedOrigins []string `json:"allowedOrigins"`
			Apps           []string `json:"apps"`
			SessionSeconds int      `json:"sessionSeconds"`
		}
		Settings store.AppSettings `json:"settings"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatal(err)
	}
	if !view.Env.CrossSite || len(view.Env.AllowedOrigins) != 1 || view.Env.AllowedOrigins[0] != embedHost || view.Env.SessionSeconds != 3600 {
		t.Fatalf("环境基线不对：%s", body)
	}
	if !view.Effective.CrossSite || view.Effective.Apps == nil {
		t.Fatalf("有效值不对：%s", body)
	}

	// 非法来源/应用/TTL 都拒绝保存
	for _, patch := range []map[string]any{
		{"allowedOrigins": []string{"https://bad.example.com/path"}},
		{"allowedOrigins": []string{"*"}},
		{"apps": []string{"Bad Slug"}},
		{"sessionMinutes": 999},
	} {
		code, body := f.saveSettings(t, patch)
		if code != 400 {
			t.Fatalf("非法设置应 400：%v → %d %s", patch, code, body)
		}
	}

	// 合法保存：DB 追加来源、应用白名单、TTL
	code, body = f.saveSettings(t, map[string]any{
		"crossSite": true, "allowedOrigins": []string{embedHostB}, "apps": []string{f.slug}, "sessionMinutes": 30,
	})
	expect(t, "保存嵌入设置", code, 200, body)
	code, body = f.admin.do("GET", "/api/admin/embed-settings", nil, nil)
	expect(t, "再读嵌入有效配置", code, 200, body)
	view = decode[struct {
		Env struct {
			CrossSite      bool     `json:"crossSite"`
			AllowedOrigins []string `json:"allowedOrigins"`
			SessionSeconds int      `json:"sessionSeconds"`
		}
		Effective struct {
			CrossSite      bool     `json:"crossSite"`
			AllowedOrigins []string `json:"allowedOrigins"`
			Apps           []string `json:"apps"`
			SessionSeconds int      `json:"sessionSeconds"`
		}
		Settings store.AppSettings `json:"settings"`
	}](t, body)
	if !contains(view.Effective.AllowedOrigins, embedHost) || !contains(view.Effective.AllowedOrigins, embedHostB) {
		t.Fatalf("有效来源应是 env ∪ DB：%v", view.Effective.AllowedOrigins)
	}
	if view.Effective.SessionSeconds != 30*60 || len(view.Effective.Apps) != 1 || view.Effective.Apps[0] != f.slug {
		t.Fatalf("有效 TTL/apps 不对：%s", body)
	}

	// 管理员能看到保存后的段；员工仍然看不到
	code, body = f.admin.do("GET", "/api/settings", nil, nil)
	expect(t, "管理员读设置", code, 200, body)
	adminSet := decode[store.AppSettings](t, body)
	if len(adminSet.Embed.AllowedOrigins) != 1 || adminSet.Embed.SessionMinutes != 30 {
		t.Fatalf("管理员应看到嵌入设置：%+v", adminSet.Embed)
	}
	if code, body := empClient.do("GET", "/api/settings", nil, nil); code != 200 {
		t.Fatalf("员工读设置：%d %s", code, body)
	} else if empSet := decode[store.AppSettings](t, body); len(empSet.Embed.AllowedOrigins) != 0 {
		t.Fatalf("员工不应看到嵌入来源：%+v", empSet.Embed)
	}

	// 审计：settings.update 里带 embed 段
	logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "settings.update"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if secs, ok := l.Detail["sections"].([]any); ok {
			for _, s := range secs {
				if s == "embed" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("保存嵌入设置应记 settings.update 审计（含 embed 段）")
	}

	// 非法值直接写库：启动校验失败（fail closed）
	bad := store.DefaultSettings()
	bad.Embed.AllowedOrigins = []string{"https://bad.example.com/path"}
	if err := f.st.SaveSettings(context.Background(), bad, 1); err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateStoredSettings(context.Background(), f.st); err == nil {
		t.Fatal("库里非法嵌入来源应让启动校验失败")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestEmbedSettingsDynamicCSPAndMint(t *testing.T) {
	// env：来源有 embedHost，但跨站开关关闭；DB 可以打开并追加来源、覆盖 TTL
	f := newEmbedFixture(t, func(o *api.Options) {
		o.EmbedCrossSite = false
		o.EmbedOrigins = []string{embedHost}
		o.Web = htmlStub()
	})
	emp := f.user(t, "embed-dyn-emp")
	f.token(t, emp.ID, "ee_at_embed_dyn", []string{"hub", "agent"})

	if code, body := f.mint(t, "ee_at_embed_dyn", map[string]any{"app": f.slug, "host_origin": embedHost}); code != 404 {
		t.Fatalf("未开启跨站时 mint 应 404：%d %s", code, body)
	}
	// 未开启跨站时 CSP 仍保留 env 来源（v0 同站嵌入要用）。未设置 DSH_EE_CSP 时默认
	// enforce：响应同时带完整基线策略与合并后的 frame-ancestors。
	if got := cspOf(t, f.ts.URL, "/a/"+f.slug); !strings.HasPrefix(got, "default-src 'self';") || !strings.HasSuffix(got, "frame-ancestors 'self' "+embedHost) {
		t.Fatalf("默认策略应包含完整基线 + env 来源：%q", got)
	}
	if got := cspOf(t, f.ts.URL, "/admin/menus"); !strings.HasPrefix(got, "default-src 'self';") || !strings.HasSuffix(got, "frame-ancestors 'self'") || strings.Contains(got, embedHost) {
		t.Fatalf("管理页只能同站：%q", got)
	}

	// 管理员打开跨站并追加来源：CSP 立即生效（无需等缓存过期）
	code, body := f.saveSettings(t, map[string]any{"crossSite": true, "allowedOrigins": []string{embedHostB}, "sessionMinutes": 90})
	expect(t, "打开跨站嵌入", code, 200, body)
	got := cspOf(t, f.ts.URL, "/a/"+f.slug)
	if !strings.HasPrefix(got, "default-src 'self';") || !strings.HasSuffix(got, "frame-ancestors 'self' "+embedHost+" "+embedHostB) {
		t.Fatalf("保存后 CSP 应立即带上 DB 追加来源：%q", got)
	}
	if got := cspOf(t, f.ts.URL, "/admin/menus"); !strings.HasSuffix(got, "frame-ancestors 'self'") || strings.Contains(got, embedHost) {
		t.Fatalf("管理页仍只能同站：%q", got)
	}

	// DB 打开后 mint 可用；TTL 用 DB 的 90 分钟
	code, body = f.mint(t, "ee_at_embed_dyn", map[string]any{"app": f.slug, "host_origin": embedHostB})
	expect(t, "DB 打开后 mint", code, 200, body)
	var minted struct {
		Code       string `json:"code"`
		SessionTTL int    `json:"session_ttl"`
	}
	if err := json.Unmarshal(body, &minted); err != nil {
		t.Fatal(err)
	}
	if minted.SessionTTL != 90*60 {
		t.Fatalf("session_ttl 应取 DB 的 90 分钟：%d %s", minted.SessionTTL, body)
	}
	// 应用白名单：只允许 f.slug，其它应用拒绝
	code, body = f.saveSettings(t, map[string]any{"apps": []string{"other-app"}})
	expect(t, "保存应用白名单", code, 200, body)
	if code, body := f.mint(t, "ee_at_embed_dyn", map[string]any{"app": f.slug, "host_origin": embedHostB}); code != 403 {
		t.Fatalf("不在白名单的应用应被拒绝：%d %s", code, body)
	} else if code := decode[struct{ Code string }](t, body).Code; code != "app_not_allowed" {
		t.Fatalf("错误码 = %q", code)
	}

	// DB 可以反向关掉 env 打开的跨站（管理端 kill switch）
	f2 := newEmbedFixture(t, func(o *api.Options) { o.EmbedCrossSite = true })
	emp2 := f2.user(t, "embed-dyn-emp2")
	f2.token(t, emp2.ID, "ee_at_embed_dyn2", []string{"hub", "agent"})
	code, body = f2.saveSettings(t, map[string]any{"crossSite": false})
	expect(t, "关闭跨站", code, 200, body)
	if code, body := f2.mint(t, "ee_at_embed_dyn2", map[string]any{"app": f2.slug, "host_origin": embedHost}); code != 404 {
		t.Fatalf("DB 关闭后 mint 应 404：%d %s", code, body)
	}
}

func TestEmbedSettingsValidationRejectsBadTTLAndOrigins(t *testing.T) {
	f := newEmbedFixture(t, nil)
	for _, patch := range []map[string]any{
		{"allowedOrigins": []string{"null"}},
		{"allowedOrigins": []string{"http://example.com"}},
		{"sessionMinutes": -1},
		{"sessionMinutes": 481},
	} {
		if code, body := f.saveSettings(t, patch); code != 400 {
			t.Fatalf("%v 应拒绝保存：%d %s", patch, code, body)
		}
	}
	// 合法会话时长边界
	code, body := f.saveSettings(t, map[string]any{"sessionMinutes": 480})
	expect(t, "480 分钟合法", code, 200, body)
}

// 确保 session TTL 边界和「未设置」语义：DB 0 = 跟随 env。
func TestEmbedSettingsSessionMinutesFollowEnv(t *testing.T) {
	f := newEmbedFixture(t, func(o *api.Options) { o.EmbedSessionTTL = 45 * time.Minute })
	emp := f.user(t, "embed-ttl-emp")
	f.token(t, emp.ID, "ee_at_embed_ttl2", []string{"hub", "agent"})
	code, body := f.mint(t, "ee_at_embed_ttl2", map[string]any{"app": f.slug, "host_origin": embedHost})
	expect(t, "mint", code, 200, body)
	if got := decode[struct {
		SessionTTL int `json:"session_ttl"`
	}](t, body).SessionTTL; got != 2700 {
		t.Fatalf("DB 未设置 TTL 时应跟随 env 45m，实际 %d", got)
	}
}
