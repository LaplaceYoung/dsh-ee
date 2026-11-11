package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 演示站：登录页给出账号；会把别人锁在外面的操作被拒；对外请求只能连自己；一键导入示例照常可用。
func TestDemoSite(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	videoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(videoDir, "intro-zh.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	statsFile := filepath.Join(t.TempDir(), "stats.jsonl")
	ts := httptest.NewUnstartedServer(nil)
	srv := api.New(st, box, api.Options{
		DataDir:  t.TempDir(),
		Addr:     ts.Listener.Addr().String(),
		DemoSite: &api.DemoSite{Username: "admin", Password: "admin-password-123", Reset: "每 6 小时", ResetEvery: 6 * time.Hour, VideoDir: videoDir, StatsFile: statsFile},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts.Config.Handler = srv.Handler()
	ts.Start()
	defer ts.Close()

	anon := newClient(t, ts.URL)
	code, body := anon.do("GET", "/api/auth/methods", nil, nil)
	expect(t, "登录方式", code, 200, body)
	demo := decodeAs[struct {
		Demo *struct {
			Username, Password, Reset string
			NextReset                 time.Time `json:"next_reset"`
		}
	}](t, body).Demo
	if demo == nil || demo.Username != "admin" || demo.Password != "admin-password-123" || demo.Reset != "每 6 小时" {
		t.Fatalf("登录页应给出演示账号：%s", body)
	}
	if left := time.Until(demo.NextReset); left <= 0 || left > 6*time.Hour {
		t.Fatalf("应给出 6 小时内的下次重置时间：%s", body)
	}

	admin := newClient(t, ts.URL)
	code, body = admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("POST", "/api/auth/password", map[string]string{"old_password": "admin-password-123", "new_password": "something-else-456"}, nil)
	expect(t, "演示站改密码", code, 403, body)
	if !strings.Contains(string(body), "演示站") {
		t.Fatalf("应说明是演示站的限制：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/providers", map[string]any{"name": "x", "base_url": "https://api.example.com/v1", "api_key": "sk-x"}, nil)
	expect(t, "演示站加模型接入", code, 403, body)
	code, body = anon.do("PUT", "/npm/some-pkg", map[string]any{}, nil)
	expect(t, "演示站 npm 发布", code, 403, body)

	// 示例连接从服务端访问自己，演示站里也要能建成
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "导入示例", code, 200, body)
	for _, it := range decodeAs[struct {
		Items []struct{ Kind, Slug, Result, Error string }
	}](t, body).Items {
		if it.Result != "created" {
			t.Fatalf("%s %s 没建成：%s %s", it.Kind, it.Slug, it.Result, it.Error)
		}
	}

	// 首页介绍视频：只给目录里有的语言；支持 Range；别的文件名一律 404
	_, body = anon.do("GET", "/api/auth/methods", nil, nil)
	if v := decodeAs[struct{ Demo struct{ Videos []string } }](t, body).Demo.Videos; len(v) != 1 || v[0] != "zh" {
		t.Fatalf("应只给出有文件的视频语言：%s", body)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/demo/intro/zh", nil)
	req.Header.Set("Range", "bytes=2-4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(part) != "234" || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("视频应支持 Range：%d %q %s", resp.StatusCode, part, resp.Header.Get("Content-Type"))
	}
	for _, path := range []string{"/demo/intro/en", "/demo/intro/..%2fintro-zh.mp4", "/demo/intro/zh.mp4"} {
		if code, _ := anon.do("GET", path, nil, nil); code != 404 {
			t.Fatalf("%s 应为 404，实际 %d", path, code)
		}
	}

	// 一键以示例员工身份体验：只认示例员工，拿到的是员工自己的会话
	code, body = anon.do("GET", "/api/auth/methods", nil, nil)
	personas := decodeAs[struct {
		Demo struct {
			Personas []struct{ Username, App, Try string }
		}
	}](t, body).Demo.Personas
	if len(personas) == 0 || personas[0].App == "" || personas[0].Try == "" {
		t.Fatalf("登录页应给出场景卡片：%s", body)
	}
	visitor := newClient(t, ts.URL)
	code, body = visitor.do("POST", "/api/auth/demo-login", map[string]string{"username": "admin"}, nil)
	expect(t, "一键登录管理员", code, 400, body)
	code, body = visitor.do("POST", "/api/auth/demo-login", map[string]string{"username": personas[0].Username}, nil)
	expect(t, "一键以示例员工登录", code, 200, body)
	code, body = visitor.do("GET", "/api/auth/me", nil, nil)
	expect(t, "示例员工的会话", code, 200, body)
	if me := decodeAs[struct {
		Username string
		IsSuper  bool `json:"is_super"`
	}](t, body); me.Username != personas[0].Username || me.IsSuper {
		t.Fatalf("应以示例员工身份登录：%s", body)
	}
	code, body = visitor.do("GET", "/api/me/apps/"+personas[0].App+"/web", nil, nil)
	if code == 404 {
		t.Fatalf("示例员工应能打开 %s 的网页版：%s", personas[0].App, body)
	}
	// 用自己的 DSH 接上来：设备码登录走完算一次接入
	code, body = anon.do("POST", "/api/device/code", map[string]any{"client_name": "MacBook"}, nil)
	expect(t, "申请设备码", code, 200, body)
	device := decodeAs[struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}](t, body)
	code, body = visitor.do("POST", "/api/device/"+device.UserCode, map[string]any{"approve": true}, nil)
	expect(t, "确认设备登录", code, 200, body)
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "换设备令牌", code, 200, body)
	// 统计：点了哪张场景卡片、打开了哪个应用、接入了一次 DSH，各记一行；不记访客的任何信息
	raw, _ := os.ReadFile(statsFile)
	var events []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		events = append(events, decodeAs[map[string]string](t, []byte(line)))
	}
	if len(events) != 3 || events[0]["event"] != "persona" || events[0]["what"] != personas[0].Username ||
		events[1]["event"] != "app_open" || events[1]["what"] != personas[0].App ||
		events[2]["event"] != "dsh_connect" || events[2]["what"] != personas[0].Username ||
		len(events[0]) != 3 || len(events[1]) != 3 || len(events[2]) != 3 {
		t.Fatalf("统计应是场景卡片、应用打开、DSH 接入各一行，每行只有时间、事件、对象：%s", raw)
	}

	// 试连别的地址：哪怕同在本机、只是端口不同，也被拦在拨号这一步
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("演示站不应访问到外部地址")
	}))
	defer other.Close()
	spec := map[string]any{"title": "X", "description": "x", "type": "mcp", "url": other.URL, "auth": map[string]any{"type": "bearer"}}
	code, body = admin.do("POST", "/api/admin/connections/test", map[string]any{"spec": spec, "secret": "s"}, nil)
	expect(t, "试连外部地址", code, 200, body)
	if !strings.Contains(string(body), "演示站不允许访问外部地址") {
		t.Fatalf("应说明演示站不许访问外部地址：%s", body)
	}
}
