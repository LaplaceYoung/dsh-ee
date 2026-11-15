package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 从 DSH 打开的网页会话（limited）只能看连接，不能替员工换凭据、也不能发起 / 完成 OAuth 授权：
// 个人凭据会以员工身份发给上游，OAuth 授权会把连接绑到某个账号上。
// 这里直接建一个 limited 会话（和 /api/auth/handoff 换成的是同一种会话），
// 再确认同一个人用完整登录时原行为不变。

// limitedSessionOf 给已登录的 c 换一个同一员工的 limited 网页会话。
func limitedSessionOf(t *testing.T, f *svcFixture, c *client, wantUser string) *client {
	t.Helper()
	code, body := c.do("GET", "/api/auth/me", nil, nil)
	expect(t, "读当前登录人", code, 200, body)
	id := decodeAs[struct{ ID int64 }](t, body).ID
	raw := secret.Token()
	if err := f.st.CreateLimitedSession(context.Background(), secret.Hash(raw), id, time.Hour); err != nil {
		t.Fatalf("建 limited 会话：%v", err)
	}
	lc := newClient(t, f.ts.URL)
	base, err := url.Parse(f.ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	lc.hc.Jar.SetCookies(base, []*http.Cookie{{Name: "dsh_ee_session", Value: raw}})

	code, body = lc.do("GET", "/api/auth/me", nil, nil)
	expect(t, "limited 会话读 me", code, 200, body)
	me := decodeAs[struct {
		ID       int64
		Username string
		Limited  bool
	}](t, body)
	if !me.Limited || me.Username != wantUser || me.ID != id {
		t.Fatalf("应该是 %s 的 limited 会话：%s", wantUser, body)
	}
	return lc
}

// doNoRedirect 发一个不跟随跳转的请求，返回状态码与 Location（OAuth 回调是浏览器往返）。
func doNoRedirect(t *testing.T, c *client, method, path string) (int, string) {
	t.Helper()
	hc := *c.hc
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequest(method, c.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

// limited 的错误体必须和 requireFullSession 一致：403 + reauth_required。
func expectReauthRequired(t *testing.T, what string, code int, body []byte) {
	t.Helper()
	expect(t, what, code, 403, body)
	if got := decodeAs[struct{ Code string }](t, body).Code; got != "reauth_required" {
		t.Fatalf("%s：应返回 reauth_required，得到 %s", what, body)
	}
}

// myCredential 读「我的连接」里某个连接的凭据状态（拿不到全文，只有填没填与尾号）。
func myCredential(t *testing.T, c *client, assetID int64) (filled bool, tail string) {
	t.Helper()
	code, body := c.do("GET", "/api/me/connections", nil, nil)
	expect(t, "我的连接", code, 200, body)
	var resp struct {
		Connections []struct {
			ID         int64
			Credential *struct {
				Filled bool
				Tail   string
			}
		}
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("解析我的连接：%v（%s）", err, body)
	}
	for _, conn := range resp.Connections {
		if conn.ID == assetID {
			if conn.Credential == nil {
				return false, ""
			}
			return conn.Credential.Filled, conn.Credential.Tail
		}
	}
	t.Fatalf("连接 %d 不在我的连接里：%s", assetID, body)
	return false, ""
}

// PUT/DELETE /api/me/connections/{id}/credential：limited 会话不能替换 / 删除员工个人凭据。
func TestMyCredentialRequiresFullSession(t *testing.T) {
	f := newSvcFixture(t)
	admin := f.admin
	alice, _ := f.user("alice")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer up.Close()
	oaDoc := `{"openapi":"3.0.0","info":{"title":"OA"},"paths":{"/todo":{"get":{"operationId":"todo"}}}}`
	code, body := admin.do("POST", "/api/hub", map[string]any{
		"kind": "connection", "delivery": "auto", "slug": "oa",
		"spec": map[string]any{
			"title": "OA", "type": "openapi", "url": up.URL, "openapi": oaDoc,
			"user_auth": map[string]any{"mode": "user_secret", "hint": "在 OA 的个人设置里生成"},
		},
	}, nil)
	expect(t, "建 user_secret 连接", code, 201, body)
	oaID := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	credPath := fmt.Sprintf("/api/me/connections/%d/credential", oaID)

	// 完整会话：填、看、删都照旧
	code, body = alice.do("PUT", credPath, map[string]string{"secret": "full-token-1234"}, nil)
	expect(t, "完整会话填自己的凭据", code, 204, body)
	if filled, tail := myCredential(t, alice, oaID); !filled || tail != "…1234" {
		t.Fatalf("完整会话填完应显示已填与尾号：%v %q", filled, tail)
	}

	limited := limitedSessionOf(t, f, alice, "alice")
	// 只读员工功能仍然可用，否则页面里的「我的连接」都打不开
	code, body = limited.do("GET", "/api/me/connections", nil, nil)
	expect(t, "limited 会话仍能看我的连接", code, 200, body)

	code, body = limited.do("PUT", credPath, map[string]string{"secret": "evil-token-9999"}, nil)
	expectReauthRequired(t, "limited 会话不能改凭据", code, body)
	// 没被真的写进去：完整会话看到的还是原来的凭据
	if filled, tail := myCredential(t, alice, oaID); !filled || tail != "…1234" {
		t.Fatalf("limited 会话不应改到凭据：%v %q", filled, tail)
	}

	code, body = limited.do("DELETE", credPath, nil, nil)
	expectReauthRequired(t, "limited 会话不能删凭据", code, body)
	if filled, tail := myCredential(t, alice, oaID); !filled || tail != "…1234" {
		t.Fatalf("limited 会话不应删掉凭据：%v %q", filled, tail)
	}

	// 完整会话删除照旧
	code, body = alice.do("DELETE", credPath, nil, nil)
	expect(t, "完整会话删自己的凭据", code, 204, body)
	if filled, _ := myCredential(t, alice, oaID); filled {
		t.Fatal("完整会话删完应该显示没填")
	}
}

// POST .../oauth/start 与 GET /api/oauth/callback：limited 会话既不能发起授权，
// 也不能拿完整会话发起的旧 state 直接被带回完成绑定。
func TestOAuthRequiresFullSession(t *testing.T) {
	f := newSvcFixture(t)
	admin := f.admin

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"std-1","refresh_token":"r-1","expires_in":3600}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer up.Close()
	manifest := fmt.Sprintf(`{
  "id": "acme/itsm", "version": "1.0.0", "name": "工单", "description": "查工单", "kind": "declarative",
  "openapi": "openapi.yaml", "servers": [%q],
  "credential": { "mode": "oauth" },
  "oauth": { "authorizeUrl": "%s/oauth/authorize", "tokenUrl": "%s/oauth/token", "scopes": ["tickets.read"] },
  "tools": [{ "name": "list_tickets", "readOnly": true }]
}`, up.URL, up.URL, up.URL)
	pkg := map[string]any{
		"files":               map[string]string{"dsh-ee.extension.json": manifest, "openapi.yaml": extOpenAPI},
		"oauth_client_id":     "cid",
		"oauth_client_secret": "csecret",
	}
	code, body := admin.do("POST", "/api/admin/extensions/install", pkg, nil)
	expect(t, "安装 oauth 的声明式扩展", code, 201, body)
	itsm := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID

	alice, _ := f.user("alice")
	code, body = alice.do("POST", fmt.Sprintf("/api/hub/%d/subscription", itsm), nil, nil)
	expect(t, "订阅这个连接", code, 204, body)

	limited := limitedSessionOf(t, f, alice, "alice")
	startPath := fmt.Sprintf("/api/me/connections/%d/oauth/start", itsm)
	code, body = limited.do("POST", startPath, nil, nil)
	expectReauthRequired(t, "limited 会话不能发起 OAuth 授权", code, body)

	// 完整会话发起授权照旧：拿到授权地址与 state
	code, body = alice.do("POST", startPath, nil, nil)
	expect(t, "完整会话发起 OAuth 授权", code, 200, body)
	target := decodeAs[struct{ URL string }](t, body).URL
	tu, err := url.Parse(target)
	if err != nil {
		t.Fatalf("授权地址不对：%s", target)
	}
	state := tu.Query().Get("state")
	if !strings.HasPrefix(target, up.URL+"/oauth/authorize") || state == "" {
		t.Fatalf("授权地址不对：%s", target)
	}

	// 拿完整会话发起的 state 让 limited 会话直接回调：不认（state 本身没有会话类型，
	// 所以回调按当前会话类型判断，防的是旧 state 被直接回放）。
	callback := "/api/oauth/callback?error=access_denied&state=" + url.QueryEscape(state)
	code, body = limited.do("GET", callback, nil, nil)
	expectReauthRequired(t, "limited 会话不能完成 OAuth 回调", code, body)

	// 同一个 state 由完整会话回调：照旧回到连接页并报对方没完成授权（不换令牌）
	status, loc := doNoRedirect(t, alice, "GET", callback)
	if status != http.StatusFound || !strings.HasPrefix(loc, fmt.Sprintf("/assets/%d", itsm)) || !strings.Contains(loc, "oauth_error=") {
		t.Fatalf("完整会话回调应回到连接页并报错：%d %q", status, loc)
	}
}
