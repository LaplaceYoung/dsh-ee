package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/i18n"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 用户语言偏好：读 / 写 / 非法值拒绝；” 表示跟随组织默认语言。
func TestUserLanguagePreferenceAPI(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)

	// 默认没有语言偏好（跟随组织默认），/api/me 必须把这个字段带给网页。
	code, body = c.do("GET", "/api/auth/me", nil, nil)
	expect(t, "读当前用户", code, http.StatusOK, body)
	if got := decodeAs[struct {
		Language string `json:"language"`
	}](t, body).Language; got != "" {
		t.Fatalf("默认语言偏好应为空（跟随组织默认），得到 %q", got)
	}

	// 浏览器写法 zh-CN 归一成规范短码再存，返回值就是库里存的规范码。
	code, body = c.do("PUT", "/api/me/preferences", map[string]string{"language": "zh-CN"}, nil)
	expect(t, "保存 zh-CN", code, http.StatusOK, body)
	if got := decodeAs[struct {
		Language string `json:"language"`
	}](t, body).Language; got != "zh" {
		t.Fatalf("zh-CN 应归一成 zh，得到 %q", got)
	}
	code, body = c.do("GET", "/api/auth/me", nil, nil)
	expect(t, "保存后读当前用户", code, http.StatusOK, body)
	if got := decodeAs[struct {
		Language string `json:"language"`
	}](t, body).Language; got != "zh" {
		t.Fatalf("/api/me 应返回保存的语言 zh，得到 %q", got)
	}
	if u, err := st.GetUser(ctx, 1); err != nil || u.Language != "zh" {
		t.Fatalf("语言偏好应落库：%+v err=%v", u, err)
	}

	// 非法值拒绝，且不覆盖已有的合法值。
	for _, bad := range []string{"fr", "zh-CN-extra-bad", "en;drop", "中文"} {
		code, body = c.do("PUT", "/api/me/preferences", map[string]string{"language": bad}, nil)
		expect(t, "非法语言 "+bad, code, http.StatusBadRequest, body)
	}
	code, body = c.do("PUT", "/api/me/preferences", map[string]string{"language": "en"}, nil)
	expect(t, "保存 en", code, http.StatusOK, body)

	// 空串 = 清掉个人偏好，回到跟随组织默认。
	code, body = c.do("PUT", "/api/me/preferences", map[string]string{"language": ""}, nil)
	expect(t, "清空语言偏好", code, http.StatusOK, body)
	if got := decodeAs[struct {
		Language string `json:"language"`
	}](t, body).Language; got != "" {
		t.Fatalf("清空后应返回空串，得到 %q", got)
	}
	code, body = c.do("GET", "/api/auth/me", nil, nil)
	expect(t, "清空后读当前用户", code, http.StatusOK, body)
	if got := decodeAs[struct {
		Language string `json:"language"`
	}](t, body).Language; got != "" {
		t.Fatalf("清空后 /api/me 应为空，得到 %q", got)
	}
}

// 组织默认语言：默认值 zh、保存规范化、非法值拒绝、旧请求不带字段时保持原值。
func TestSettingsDefaultLanguage(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	// 库里没有 app 设置时，默认语言是 zh。
	if set, err := st.GetSettings(ctx); err != nil || set.DefaultLanguage != "zh" {
		t.Fatalf("组织默认语言初始值应为 zh：%+v err=%v", set, err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)

	settings := decodeAs[map[string]any](t, mustGet(t, admin, "/api/settings"))
	if settings["defaultLanguage"] != "zh" {
		t.Fatalf("GET /api/settings 应带默认语言 zh：%v", settings["defaultLanguage"])
	}
	settings["defaultLanguage"] = "es-ES"
	code, body = admin.do("PUT", "/api/admin/settings", settings, nil)
	expect(t, "保存组织默认语言 es-ES", code, http.StatusOK, body)
	if got := decodeAs[map[string]any](t, body)["defaultLanguage"]; got != "es" {
		t.Fatalf("es-ES 应归一成 es，得到 %v", got)
	}
	if set, err := st.GetSettings(ctx); err != nil || set.DefaultLanguage != "es" {
		t.Fatalf("组织默认语言应落库为 es：%+v err=%v", set, err)
	}
	// 旧版网页不认识的新字段：请求里不带时保持库里原值，不能被零值覆盖。
	settings = decodeAs[map[string]any](t, mustGet(t, admin, "/api/settings"))
	delete(settings, "defaultLanguage")
	code, body = admin.do("PUT", "/api/admin/settings", settings, nil)
	expect(t, "请求不带 defaultLanguage", code, http.StatusOK, body)
	if got := decodeAs[map[string]any](t, body)["defaultLanguage"]; got != "es" {
		t.Fatalf("不带该字段时不应清空组织默认语言，得到 %v", got)
	}
	settings = decodeAs[map[string]any](t, mustGet(t, admin, "/api/settings"))
	settings["defaultLanguage"] = "fr"
	code, body = admin.do("PUT", "/api/admin/settings", settings, nil)
	expect(t, "非法组织默认语言", code, http.StatusBadRequest, body)
	if set, err := st.GetSettings(ctx); err != nil || set.DefaultLanguage != "es" {
		t.Fatalf("非法值不应落库：%+v err=%v", set, err)
	}
}

// 语言选择单测：用户有 language 用它 / 空则组织默认 / 都没有 zh。
// 端到端行为由 TestFeishuNotificationLanguage 覆盖；这里直接钉住服务端实际使用的优先级函数。
func TestUserLanguageResolution(t *testing.T) {
	cases := []struct{ user, org, want string }{
		{"en", "zh", "en"},
		{"", "es", "es"},
		{"", "", "zh"},
		{"pt-BR", "hi", "pt"},
		{"fr", "pt", "pt"},
		{"fr", "", "zh"},
		{"zh-CN", "en", "zh"},
	}
	for _, c := range cases {
		if got := i18n.Resolve(c.user, c.org); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q，应为 %q", c.user, c.org, got, c.want)
		}
	}
}

// 确认 /api/me/preferences 是登录用户的网页接口，未登录访问被拒。
func TestPreferencesRouteRequiresSession(t *testing.T) {
	st := openTestStore(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	code, _ := newClient(t, ts.URL).do("PUT", "/api/me/preferences", map[string]string{"language": "en"}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("未登录不能保存语言偏好，得到 %d", code)
	}
}
