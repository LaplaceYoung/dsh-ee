package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// doRaw 发一个原始字节请求，复用 client 的 cookie jar。
func (c *client) doRaw(method, path string, body []byte, contentType string) (int, []byte) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

type packageCheckResp struct {
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	IsPlugin       bool                `json:"is_plugin"`
	CanSubmit      bool                `json:"can_submit"`
	WillPublish    bool                `json:"will_publish"`
	RequiresReview bool                `json:"requires_review"`
	Scan           []registry.ScanItem `json:"scan"`
	Conflicts      []struct {
		Key    string         `json:"key"`
		Params map[string]any `json:"params"`
	} `json:"conflicts"`
}

func checkTgz(t *testing.T, c *client, tgz []byte) packageCheckResp {
	t.Helper()
	code, body := c.doRaw("POST", "/api/me/packages/check", tgz, "application/octet-stream")
	expect(t, "预检", code, 200, body)
	return decodeAs[packageCheckResp](t, body)
}

func scanLevel(r packageCheckResp, key string) string {
	for _, item := range r.Scan {
		if item.Key == key {
			return item.Level
		}
	}
	return ""
}

func conflictKeys(r packageCheckResp) map[string]bool {
	out := map[string]bool{}
	for _, c := range r.Conflicts {
		out[c.Key] = true
	}
	return out
}

func TestPackagePreflight(t *testing.T) {
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

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	t.Run("合法的插件可以发布", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@company/preflight-ok", "1.0.0", nil), bundleFile)
		r := checkTgz(t, admin, tgz)
		if !r.CanSubmit || !r.IsPlugin || r.WillPublish || !r.RequiresReview {
			t.Fatalf("预检结果不对：%+v", r)
		}
		if r.Name != "@company/preflight-ok" || r.Version != "1.0.0" {
			t.Fatalf("包名或版本不对：%+v", r)
		}
		if scanLevel(r, "bundle") != "pass" || scanLevel(r, "secrets") != "pass" {
			t.Fatalf("自动检查结果不对：%+v", r.Scan)
		}
		if len(r.Conflicts) != 0 {
			t.Fatalf("不应有冲突：%+v", r.Conflicts)
		}
		if _, err := st.GetVersion(ctx, "@company/preflight-ok", "1.0.0"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("预检不应写库：%v", err)
		}
	})

	t.Run("缺少组合清单会阻塞", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@company/preflight-no-patch", "1.0.0", nil), nil)
		r := checkTgz(t, admin, tgz)
		if !r.CanSubmit || r.WillPublish || !r.RequiresReview {
			t.Fatalf("检查失败不应算发布冲突，但也不能自动上架：%+v", r)
		}
		if scanLevel(r, "bundle") != "fail" {
			t.Fatalf("bundle 应为 fail：%+v", r.Scan)
		}
	})

	t.Run("疑似密钥会阻塞", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@company/preflight-secret", "1.0.0", nil), map[string]string{
			"cordis.patch.yml": "- {}\n",
			"src/index.js":     "const key = 'sk-" + strings.Repeat("a", 40) + "';\n",
		})
		r := checkTgz(t, admin, tgz)
		if !r.CanSubmit || r.WillPublish || !r.RequiresReview {
			t.Fatalf("疑似密钥不应自动上架，但仍可提交审核：%+v", r)
		}
		if scanLevel(r, "secrets") != "fail" {
			t.Fatalf("secrets 应为 fail：%+v", r.Scan)
		}
	})

	t.Run("作用域不对会阻塞", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@other/preflight-scope", "1.0.0", nil), bundleFile)
		r := checkTgz(t, admin, tgz)
		if r.CanSubmit || r.WillPublish || r.RequiresReview {
			t.Fatalf("作用域外的包会被直接拒绝：%+v", r)
		}
		if !conflictKeys(r)["scope"] {
			t.Fatalf("缺少 scope 冲突：%+v", r.Conflicts)
		}
		for _, c := range r.Conflicts {
			if c.Key == "scope" && c.Params["scope"] != "@company" {
				t.Fatalf("scope 参数应与企业作用域一致：%+v", c.Params)
			}
		}
	})

	t.Run("已存在的版本会阻塞", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@company/preflight-dup", "1.0.0", nil), bundleFile)
		code, body := admin.doRaw("POST", "/api/me/packages", tgz, "application/octet-stream")
		expect(t, "首次发布", code, 201, body)
		r := checkTgz(t, admin, tgz)
		if r.CanSubmit || r.WillPublish || r.RequiresReview {
			t.Fatalf("重复版本会被直接拒绝：%+v", r)
		}
		if !conflictKeys(r)["exists"] {
			t.Fatalf("缺少 exists 冲突：%+v", r.Conflicts)
		}
	})

	t.Run("开放模式会直接上架", func(t *testing.T) {
		code, body := admin.do("GET", "/api/settings", nil, nil)
		expect(t, "读设置", code, 200, body)
		var settings map[string]any
		if err := json.Unmarshal(body, &settings); err != nil {
			t.Fatal(err)
		}
		settings["admissionMode"] = "open"
		code, body = admin.do("PUT", "/api/admin/settings", settings, nil)
		expect(t, "开放模式", code, 200, body)
		defer func() {
			settings["admissionMode"] = "allowlist"
			_, _ = admin.do("PUT", "/api/admin/settings", settings, nil)
		}()

		tgz := makeTgz(t, pluginPkg("@company/preflight-open", "1.0.0", nil), bundleFile)
		r := checkTgz(t, admin, tgz)
		if !r.CanSubmit || !r.WillPublish || r.RequiresReview {
			t.Fatalf("开放模式下无失败项应直接上架：%+v", r)
		}
	})

	t.Run("错误的内容类型", func(t *testing.T) {
		tgz := makeTgz(t, pluginPkg("@company/preflight-type", "1.0.0", nil), bundleFile)
		code, body := admin.doRaw("POST", "/api/me/packages/check", tgz, "application/json")
		expect(t, "内容类型", code, 415, body)
	})

	t.Run("不是 tarball", func(t *testing.T) {
		code, body := admin.doRaw("POST", "/api/me/packages/check", []byte("not a tgz"), "application/octet-stream")
		expect(t, "坏包", code, 400, body)
	})
}

func TestPackagePreflightOwner(t *testing.T) {
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

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	create := func(username string) *client {
		t.Helper()
		code, body := admin.do("POST", "/api/admin/users", map[string]any{"username": username, "password": username + "-password-1"}, nil)
		expect(t, "建用户 "+username, code, 201, body)
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": username, "password": username + "-password-1"}, nil)
		expect(t, "登录 "+username, code, 200, body)
		return c
	}
	alice := create("alice")
	bob := create("bob")

	tgz := makeTgz(t, pluginPkg("@company/preflight-owner", "1.0.0", nil), bundleFile)
	code, body = alice.doRaw("POST", "/api/me/packages", tgz, "application/octet-stream")
	expect(t, "alice 发布", code, 201, body)

	next := makeTgz(t, pluginPkg("@company/preflight-owner", "1.0.1", nil), bundleFile)
	r := checkTgz(t, bob, next)
	if r.CanSubmit || r.WillPublish || r.RequiresReview {
		t.Fatal("别人的包不应允许发布新版本")
	}
	if !conflictKeys(r)["owner"] {
		t.Fatalf("缺少 owner 冲突：%+v", r.Conflicts)
	}
}
