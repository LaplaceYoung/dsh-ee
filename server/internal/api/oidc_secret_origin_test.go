package api_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 本文件覆盖「登录设置保存路径」的 secret 外带：clientSecret 不传时后端沿用已存值，
// 而它同时是 OIDC 的 client secret（POST 到 Issuer 的 token 端点）与飞书应用的 app
// secret（POST 到 FeishuAPIBase 的 tenant token 接口）。若同一次保存把接收方换成请求
// 里指定的新地址又不重填 secret，旧 secret 就会被发往新地址。与 #213（改上游 base_url
// 沿用旧 API key）、#210（改目录地址沿用已存服务账号密码）同族，触发点在保存路径。

// storedLoginSettings 是 settings 表里 oidc 那一行的关键字段（含加密后的 secret）。
type storedLoginSettings struct {
	Enabled         bool   `json:"enabled"`
	Issuer          string `json:"issuer"`
	ClientID        string `json:"clientId"`
	FeishuAPIBase   string `json:"feishuApiBase"`
	ClientSecretEnc string `json:"clientSecretEnc"`
}

type loginSettingsView struct {
	Issuer          string `json:"issuer"`
	FeishuAPIBase   string `json:"feishuApiBase"`
	ClientSecretSet bool   `json:"clientSecretSet"`
}

type loginSettingsEnv struct {
	url   string
	admin *client
	store *store.Store
	box   *secret.Box
}

func newLoginSettingsEnv(t *testing.T, hc *http.Client) *loginSettingsEnv {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: hc}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	return &loginSettingsEnv{url: ts.URL, admin: admin, store: st, box: box}
}

func (e *loginSettingsEnv) stored(t *testing.T) storedLoginSettings {
	t.Helper()
	var st storedLoginSettings
	if _, err := e.store.GetSettingJSON(context.Background(), "oidc", &st); err != nil {
		t.Fatalf("读登录设置: %v", err)
	}
	return st
}

// secretPlain 解出存储里的 client secret 明文；未保存时为空串。
func (e *loginSettingsEnv) secretPlain(t *testing.T) string {
	t.Helper()
	enc := e.stored(t).ClientSecretEnc
	if enc == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("clientSecretEnc 不是 base64: %v", err)
	}
	pw, err := e.box.Open(raw)
	if err != nil {
		t.Fatalf("解密 client secret: %v", err)
	}
	return string(pw)
}

func (e *loginSettingsEnv) view(t *testing.T) loginSettingsView {
	t.Helper()
	code, body := e.admin.do("GET", "/api/admin/auth/oidc", nil, nil)
	expect(t, "读登录设置", code, 200, body)
	return decodeAs[loginSettingsView](t, body)
}

func oidcSettings(issuer, clientSecret string) map[string]any {
	return map[string]any{
		"enabled": true, "preset": "custom", "name": "企业统一登录", "issuer": issuer, "clientId": "dsh-ee",
		"clientSecret": clientSecret, "scopes": "profile email",
		"usernameClaim": "preferred_username", "nameClaim": "name", "emailClaim": "email",
		"departmentClaim": "department", "groupsClaim": "groups", "autoCreate": true,
		"roleMappings": []map[string]any{},
	}
}

// settingsPatch 复制一份设置再覆盖若干字段。
func settingsPatch(cfg map[string]any, kv map[string]any) map[string]any {
	out := make(map[string]any, len(cfg)+len(kv))
	for k, v := range cfg {
		out[k] = v
	}
	for k, v := range kv {
		out[k] = v
	}
	return out
}

// settingsWithoutSecret 去掉 clientSecret：设置页在 secret 输入框留空时就是这样提交的，
// 后端会沿用已存的值。
func settingsWithoutSecret(cfg map[string]any) map[string]any {
	out := settingsPatch(cfg, nil)
	delete(out, "clientSecret")
	return out
}

// feishuSecretCapture 是假的飞书开放平台：记录 tenant token 请求里的 app_secret，
// 用来断言「哪一把 app secret 被 POST 到了哪个地址」。其它接口一律 404，同步会失败，
// 但这不影响本用例关心的 tenant token 请求。
type feishuSecretCapture struct {
	srv     *httptest.Server
	mu      sync.Mutex
	secrets []string
}

func newFeishuSecretCapture(t *testing.T) *feishuSecretCapture {
	t.Helper()
	c := &feishuSecretCapture{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			http.NotFound(w, r)
			return
		}
		var body struct {
			AppID     string `json:"app_id"`
			AppSecret string `json:"app_secret"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		c.mu.Lock()
		c.secrets = append(c.secrets, body.AppSecret)
		c.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "captured-token"})
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *feishuSecretCapture) received() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.secrets...)
}

// 改 OIDC Issuer 却不重填 Client Secret 时，旧 secret 不能被自动带到新 Issuer：
// 修复前保存会成功（200），随后登录/回调时 oidc.Exchange 会把旧 secret POST 到新
// Issuer 的令牌端点。
func TestOIDCSaveDoesNotReuseStoredSecretOnChangedIssuer(t *testing.T) {
	oldIdP := newFakeIdP(t)
	newIdP := newFakeIdP(t) // 新 Issuer：换环境或被劫持后，旧 secret 会被 POST 到这里
	env := newLoginSettingsEnv(t, oldIdP.Client())

	code, body := env.admin.do("PUT", "/api/admin/auth/oidc", oidcSettings(oldIdP.URL, "idp-secret"), nil)
	expect(t, "保存初始 OIDC 配置", code, 200, body)
	if got := env.secretPlain(t); got != "idp-secret" {
		t.Fatalf("初始 client secret 没有加密保存：%q", got)
	}

	t.Run("改 Issuer 且未重填 Client Secret：拒绝并保持原配置", func(t *testing.T) {
		next := settingsWithoutSecret(oidcSettings(oldIdP.URL, ""))
		next["issuer"] = newIdP.URL
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		if code == http.StatusOK {
			// 修前复现：保存成功、旧 secret 被沿用。再真跑一次登录——start 会跳到新
			// Issuer，回调时 Exchange 把旧 secret POST 到新 Issuer 的 /token，新 IdP
			// 接受并签发令牌（登录成功），证明 secret 已被外带。
			loc := newIdP.login(t, newClient(t, env.url), map[string]any{"sub": "u-red-proof", "preferred_username": "red.proof"}, nil)
			t.Fatalf("改 Issuer 且未重填 Client Secret 时不应保存成功：修复前返回 200，load 出的 secret 仍是旧值 %q"+
				"（之后 Exchange 会把它 POST 到新 Issuer %s 的 token 端点；实测该端点用这把旧 secret 完成登录，最终跳转 %q）",
				env.secretPlain(t), newIdP.URL, loc)
		}
		expect(t, "改 Issuer 且未重填 Client Secret", code, 400, body)
		if !strings.Contains(string(body), "修改 Issuer 后需要重新填写 Client Secret") {
			t.Fatalf("400 文案应提示重新填写 Client Secret：%s", body)
		}
		// 拒绝后已存配置必须原样：Issuer 没变、secret 仍是原来的那把。
		if got := env.view(t); got.Issuer != oldIdP.URL || !got.ClientSecretSet {
			t.Fatalf("被拒绝的保存改动了已存配置：%+v", got)
		}
		if got := env.stored(t).Issuer; got != oldIdP.URL {
			t.Fatalf("存储里的 Issuer 被改了：%q", got)
		}
		if got := env.secretPlain(t); got != "idp-secret" {
			t.Fatalf("存储里的 secret 被改了：%q", got)
		}
		// 原 IdP 照常能登录：旧 secret 仍只在原 Issuer 生效。
		loc := oldIdP.login(t, newClient(t, env.url), map[string]any{"sub": "u-1", "preferred_username": "lin.na"}, nil)
		if loc != "/build" {
			t.Fatalf("原 Issuer 用已存 secret 应能照常登录，实际跳转 %q", loc)
		}
	})

	t.Run("Issuer 路径变化也算换了接收方", func(t *testing.T) {
		next := settingsWithoutSecret(oidcSettings(oldIdP.URL, ""))
		next["issuer"] = oldIdP.URL + "/realms/acme"
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "改 Issuer 路径且未重填 Client Secret", code, 400, body)
	})

	t.Run("Issuer 只是写法归一且未重填：允许沿用", func(t *testing.T) {
		next := settingsWithoutSecret(oidcSettings(oldIdP.URL, ""))
		next["issuer"] = oldIdP.URL + "/"
		next["name"] = "企业统一登录（改名）"
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "Issuer 尾部斜杠归一、未重填 secret", code, 200, body)
		if !strings.Contains(string(body), `"clientSecretSet":true`) {
			t.Fatalf("响应应保持 clientSecretSet=true：%s", body)
		}
		if got := env.view(t); got.Issuer != oldIdP.URL || !got.ClientSecretSet {
			t.Fatalf("归一后 Issuer 不应变化或丢 secret：%+v", got)
		}
		if got := env.secretPlain(t); got != "idp-secret" {
			t.Fatalf("未重填时应沿用已存 secret，实际存的是 %q", got)
		}
	})

	t.Run("改 Issuer 并重填 Client Secret：保存新值", func(t *testing.T) {
		next := oidcSettings(oldIdP.URL, "new-secret")
		next["issuer"] = newIdP.URL
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "改 Issuer 并重填 Client Secret", code, 200, body)
		if got := env.view(t); got.Issuer != newIdP.URL || !got.ClientSecretSet {
			t.Fatalf("新 Issuer 或 secret 没保存：%+v", got)
		}
		if got := env.secretPlain(t); got != "new-secret" {
			t.Fatalf("重填后应保存新 secret，实际存的是 %q", got)
		}
		// 还原成初始配置，后面的子用例继续用原来的 secret。
		code, body = env.admin.do("PUT", "/api/admin/auth/oidc", oidcSettings(oldIdP.URL, "idp-secret"), nil)
		expect(t, "还原初始 OIDC 配置", code, 200, body)
	})

	t.Run("显式清空 secret 时换 Issuer：允许（没有旧 secret 会被带走）", func(t *testing.T) {
		next := oidcSettings(oldIdP.URL, "")
		next["issuer"] = newIdP.URL
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "换 Issuer 并显式清空 secret", code, 200, body)
		if got := env.secretPlain(t); got != "" {
			t.Fatalf("显式清空后不应再存 secret，实际存的是 %q", got)
		}
	})

	t.Run("存储里没有 secret 时换 Issuer 不要求重填", func(t *testing.T) {
		next := settingsWithoutSecret(oidcSettings(oldIdP.URL, ""))
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "没有已存 secret 时换回原 Issuer", code, 200, body)
		if got := env.secretPlain(t); got != "" {
			t.Fatalf("没有 secret 时不应凭空存出 secret：%q", got)
		}
	})
}

// 改飞书 API 根地址却不重填 App Secret 时，旧 app secret 不能被自动带到新地址：
// 修复前保存会成功（200），随后组织同步/通知从 tenant token 接口把旧 app secret
// POST 到新 base。
func TestFeishuSaveDoesNotReuseStoredAppSecretOnChangedAPIBase(t *testing.T) {
	idp := newFakeIdP(t)
	env := newLoginSettingsEnv(t, idp.Client())
	capture := newFeishuSecretCapture(t)

	cfg := map[string]any{
		"enabled": true, "preset": "feishu", "name": "飞书", "issuer": idp.URL, "clientId": "dsh-ee",
		"clientSecret": "idp-secret", "feishuApiBase": idp.URL, "autoCreate": true,
		"roleMappings": []map[string]any{},
	}
	code, body := env.admin.do("PUT", "/api/admin/auth/oidc", cfg, nil)
	expect(t, "保存初始飞书配置", code, 200, body)
	if got := env.stored(t).FeishuAPIBase; got != idp.URL {
		t.Fatalf("初始飞书 API 根地址没保存：%q", got)
	}
	if got := env.secretPlain(t); got != "idp-secret" {
		t.Fatalf("初始 app secret 没有加密保存：%q", got)
	}

	t.Run("改飞书 API 根地址且未重填 App Secret：拒绝并保持原配置", func(t *testing.T) {
		next := settingsWithoutSecret(settingsPatch(cfg, map[string]any{"feishuApiBase": capture.srv.URL}))
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		if code == http.StatusOK {
			// 修前复现：保存成功、旧 app secret 被沿用。再跑一次组织同步——tenant token
			// 请求体里的 app_secret 会被 POST 到新地址。
			_, _ = env.admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
			t.Fatalf("改飞书 API 根地址且未重填 App Secret 时不应保存成功：修复前返回 200，"+
				"随后同步把旧 app secret %q POST 到了新地址 %s 的 tenant token 接口", capture.received(), capture.srv.URL)
		}
		expect(t, "改飞书 API 根地址且未重填 App Secret", code, 400, body)
		if !strings.Contains(string(body), "修改飞书 API 地址后需要重新填写 App Secret") {
			t.Fatalf("400 文案应提示重新填写 App Secret：%s", body)
		}
		if got := env.view(t); got.FeishuAPIBase != idp.URL || !got.ClientSecretSet {
			t.Fatalf("被拒绝的保存改动了已存配置：%+v", got)
		}
		if got := env.secretPlain(t); got != "idp-secret" {
			t.Fatalf("存储里的 app secret 被改了：%q", got)
		}
	})

	t.Run("同 origin 的 path 变化不算换接收方：允许沿用", func(t *testing.T) {
		next := settingsWithoutSecret(settingsPatch(cfg, map[string]any{"feishuApiBase": idp.URL + "/feishu-proxy"}))
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "飞书 API 根地址只改 path、未重填 secret", code, 200, body)
		if got := env.stored(t).FeishuAPIBase; got != idp.URL+"/feishu-proxy" {
			t.Fatalf("同 origin 的新 path 没保存：%q", got)
		}
		if got := env.secretPlain(t); got != "idp-secret" {
			t.Fatalf("未重填时应沿用已存 app secret，实际存的是 %q", got)
		}
		// 还原。
		code, body = env.admin.do("PUT", "/api/admin/auth/oidc", settingsWithoutSecret(cfg), nil)
		expect(t, "还原飞书 API 根地址", code, 200, body)
	})

	t.Run("尾部斜杠归一：允许沿用", func(t *testing.T) {
		next := settingsWithoutSecret(settingsPatch(cfg, map[string]any{"feishuApiBase": idp.URL + "/"}))
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "飞书 API 根地址尾部斜杠归一、未重填 secret", code, 200, body)
		if got := env.stored(t).FeishuAPIBase; got != idp.URL {
			t.Fatalf("尾部斜杠应被规范化掉：%q", got)
		}
		if got := env.secretPlain(t); got != "idp-secret" {
			t.Fatalf("未重填时应沿用已存 app secret，实际存的是 %q", got)
		}
	})

	t.Run("改飞书 API 根地址并重填 App Secret：保存新值", func(t *testing.T) {
		next := settingsPatch(cfg, map[string]any{"feishuApiBase": capture.srv.URL, "clientSecret": "new-app-secret"})
		code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
		expect(t, "改飞书 API 根地址并重填 App Secret", code, 200, body)
		if got := env.stored(t).FeishuAPIBase; got != capture.srv.URL {
			t.Fatalf("新飞书 API 根地址没保存：%q", got)
		}
		if got := env.secretPlain(t); got != "new-app-secret" {
			t.Fatalf("重填后应保存新 app secret，实际存的是 %q", got)
		}
	})
}

// 首次保存（存储里还没有配置）没有可沿用的旧 secret，不应被同一套规则挡住。
func TestLoginSettingsFirstSaveWithoutSecret(t *testing.T) {
	idp := newFakeIdP(t)
	env := newLoginSettingsEnv(t, idp.Client())

	next := settingsWithoutSecret(oidcSettings(idp.URL, ""))
	code, body := env.admin.do("PUT", "/api/admin/auth/oidc", next, nil)
	expect(t, "首次保存且不带 secret", code, 200, body)
	if !strings.Contains(string(body), `"clientSecretSet":false`) {
		t.Fatalf("首次保存没有 secret 时 clientSecretSet 应为 false：%s", body)
	}
	if got := env.secretPlain(t); got != "" {
		t.Fatalf("首次保存不应存出 secret：%q", got)
	}
}
