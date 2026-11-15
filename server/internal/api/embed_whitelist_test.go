package api

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestEmbedRouteWhitelist(t *testing.T) {
	allowed := []struct{ method, path string }{
		{"GET", "/api/auth/me"},
		{"GET", "/api/auth/methods"},
		{"GET", "/api/settings"},
		{"GET", "/api/me/menus"},
		{"GET", "/api/me/apps/signup/web"},
		{"POST", "/api/me/apps/signup/source"},
		{"GET", "/api/apps/signup/data"},
		{"GET", "/api/apps/signup/data/notes"},
		{"POST", "/api/apps/signup/data/notes"},
		{"PATCH", "/api/apps/signup/data/notes/12"},
		{"DELETE", "/api/apps/signup/data/notes/12"},
		{"GET", "/api/apps/signup/data/notes/export"},
		{"POST", "/api/auth/embed/logout"},
	}
	for _, c := range allowed {
		if !embedRouteAllowed(c.method, c.path) {
			t.Errorf("%s %s 应该在嵌入白名单里", c.method, c.path)
		}
	}

	blocked := []struct{ method, path string }{
		{"GET", "/api/admin/users"},
		{"POST", "/api/admin/users"},
		{"GET", "/api/me/tokens"},
		{"POST", "/api/me/tokens"},
		{"GET", "/api/hub"},
		{"PUT", "/api/admin/settings"},
		{"GET", "/api/auth/me/extra"},
		{"POST", "/api/settings"},
		{"GET", "/api/apps/signup/data/notes/12"},
		{"POST", "/api/apps/signup/data/notes/12"},
		{"GET", "/api/me/apps/signup/other"},
		{"GET", "/api/auth/embed/logout"},
		{"POST", "/mcp"},
	}
	for _, c := range blocked {
		if embedRouteAllowed(c.method, c.path) {
			t.Errorf("%s %s 不应该在嵌入白名单里", c.method, c.path)
		}
	}
}

func TestEmbedAppScopeSlug(t *testing.T) {
	// 正常路由：PathValue 已由 mux 填好，直接用它（与处理函数看到的值一致）。
	req := httptest.NewRequest("GET", "/api/apps/app-b/data/notes", nil)
	req.SetPathValue("slug", "app-b")
	if got := embedAppScopeSlug(req); got != "app-b" {
		t.Errorf("PathValue 优先：得到 %q，期望 app-b", got)
	}

	// PathValue 没填（旧版 mux 兼容模式）：按路径形状兜底，不能被编码/多段路径绕过。
	cases := map[string]string{
		"/api/apps/app-b/data":       "app-b",
		"/api/apps/app-b/data/notes": "app-b",
		"/api/apps/app-b/data/n/1":   "app-b",
		"/api/me/apps/app-b/web":     "app-b",
		"/api/me/apps/app-b/source":  "app-b",
		"/api/auth/me":               "",
		"/api/auth/methods":          "",
		"/api/settings":              "",
		"/api/me/menus":              "",
		"/api/apps":                  "",
		"/api/me/apps":               "",
	}
	for path, want := range cases {
		if got := embedAppScopeSlug(httptest.NewRequest("GET", path, nil)); got != want {
			t.Errorf("%s => %q，期望 %q", path, got, want)
		}
	}
}

func TestEmbedNextMatches(t *testing.T) {
	for _, ok := range []string{"/a/signup", "/a/signup/", "/a/signup?tab=main", "/a/signup#x"} {
		if !embedNextMatches(ok, "signup") {
			t.Errorf("%q 应该允许", ok)
		}
	}
	for _, bad := range []string{"/a/other", "/a/signupx", "/admin/users", "/api/admin/users", "/a/signup/../admin"} {
		if embedNextMatches(bad, "signup") {
			t.Errorf("%q 应该拒绝", bad)
		}
	}
}

func TestEmbedConfigEnvBaseline(t *testing.T) {
	s := newBareServer(t)
	ctx := context.Background()
	if cfg, err := s.embedConfig(ctx); err != nil || cfg.enabled() {
		t.Fatalf("默认应关闭：%+v %v", cfg, err)
	}
	s.opts.EmbedCrossSite = true
	if cfg, _ := s.embedConfig(ctx); cfg.enabled() {
		t.Fatal("没有来源时仍应关闭")
	}
	s.opts.EmbedOrigins = []string{"https://harness.example.com"}
	cfg, err := s.embedConfig(ctx)
	if err != nil || !cfg.enabled() || !cfg.allowsApp("any-app") {
		t.Fatalf("开关 + 来源都配好后应开启：%+v %v", cfg, err)
	}
}
