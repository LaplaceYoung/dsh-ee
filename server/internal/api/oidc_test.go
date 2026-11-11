package api_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// fakeIdP 是一个最小的 OIDC 身份提供方：发现配置、JWKS、换令牌（校验 PKCE）、userinfo。
type fakeIdP struct {
	*httptest.Server
	key *rsa.PrivateKey
	mu  sync.Mutex
	// code → 这次登录要签发的声明与 PKCE 挑战
	codes map[string]fakeGrant
	// 签名时故意用另一把钥匙
	forge bool
}

type fakeGrant struct {
	claims    map[string]any
	challenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, codes: map[string]fakeGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": f.URL, "authorization_endpoint": f.URL + "/authorize", "token_endpoint": f.URL + "/token",
			"jwks_uri": f.URL + "/jwks", "userinfo_endpoint": f.URL + "/userinfo",
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := f.key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		g, ok := f.codes[r.Form.Get("code")]
		delete(f.codes, r.Form.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || r.Form.Get("client_secret") != "idp-secret" || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		sub, _ := g.claims["sub"].(string)
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": f.sign(g.claims), "access_token": "at-" + sub, "token_type": "Bearer"})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		// 部门只在 userinfo 里给
		sub := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": sub, "department": "销售部"})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeIdP) sign(claims map[string]any) string {
	key := f.key
	if f.forge {
		key, _ = rsa.GenerateKey(rand.Reader, 2048)
	}
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	p, _ := json.Marshal(claims)
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	sum := sha256.Sum256([]byte(signed))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// login 走一遍浏览器的跳转：start → IdP（这里直接发码）→ callback；返回最后跳到的地址。
func (f *fakeIdP) login(t *testing.T, c *client, claims map[string]any, tamper func(q url.Values)) string {
	t.Helper()
	noFollow := *c.hc
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Get(c.base + "/api/auth/oidc/start?next=/build")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != 302 || !strings.HasPrefix(loc.String(), f.URL+"/authorize") {
		t.Fatalf("start 应跳到 IdP：%d %s", resp.StatusCode, loc)
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "dsh-ee" || !strings.Contains(q.Get("scope"), "openid") || q.Get("redirect_uri") != c.base+"/api/auth/oidc/callback" {
		t.Fatalf("授权请求参数不对：%s", loc)
	}
	full := map[string]any{"iss": f.URL, "aud": "dsh-ee", "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce")}
	for k, v := range claims {
		full[k] = v
	}
	code := secret.Token()
	f.mu.Lock()
	f.codes[code] = fakeGrant{claims: full, challenge: q.Get("code_challenge")}
	f.mu.Unlock()
	back := url.Values{"code": {code}, "state": {q.Get("state")}}
	if tamper != nil {
		tamper(back)
	}
	resp, err = noFollow.Get(c.base + "/api/auth/oidc/callback?" + back.Encode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("callback 应跳转：%d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func TestOIDCLogin(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: idp.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("GET", "/api/auth/methods", nil, nil)
	expect(t, "登录方式", code, 200, body)
	if !strings.Contains(string(body), `"oidc":null`) {
		t.Fatalf("没开时 oidc 应为 null：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "sales-lead", "name": "销售主管", "description": "", "permissions": []string{"dashboard.view"}}, nil)
	expect(t, "建角色", code, 201, body)
	roleID := decodeAs[struct{ ID int64 }](t, body).ID

	cfg := map[string]any{
		"enabled": true, "preset": "keycloak", "name": "星河 SSO", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"scopes": "profile email", "usernameClaim": "preferred_username", "nameClaim": "name", "emailClaim": "email",
		"departmentClaim": "department", "groupsClaim": "groups", "autoCreate": true,
		"roleMappings": []map[string]any{{"group": "sales-leads", "roleId": roleID}},
	}
	code, body = admin.do("POST", "/api/admin/auth/oidc/test", cfg, nil)
	expect(t, "试连", code, 200, body)
	if !strings.Contains(string(body), `"ok":true`) || !strings.Contains(string(body), idp.URL+"/token") {
		t.Fatalf("试连应读到端点：%s", body)
	}
	code, body = admin.do("PUT", "/api/admin/auth/oidc", cfg, nil)
	expect(t, "保存 OIDC", code, 200, body)
	if strings.Contains(string(body), "idp-secret") || !strings.Contains(string(body), `"clientSecretSet":true`) {
		t.Fatalf("不应回显 client secret：%s", body)
	}
	var stored map[string]any
	_, _ = st.GetSettingJSON(ctx, "oidc", &stored)
	if raw, _ := json.Marshal(stored); strings.Contains(string(raw), "idp-secret") || stored["clientSecretEnc"] == nil {
		t.Fatal("client secret 应加密保存")
	}
	code, body = admin.do("GET", "/api/auth/methods", nil, nil)
	if !strings.Contains(string(body), `"name":"星河 SSO"`) {
		t.Fatalf("登录页应看到按钮：%s", body)
	}

	// ---- 首次登录：建号、资料来自 ID Token 与 userinfo、组映射到角色 ----
	emp := newClient(t, ts.URL)
	loc := idp.login(t, emp, map[string]any{"sub": "u-1", "preferred_username": "Lin.Na@xinghe.com", "name": "林娜", "email": "lin.na@xinghe.com", "groups": []string{"/xinghe/sales-leads"}}, nil)
	if loc != "/build" {
		t.Fatalf("登录后应回到 next：%s", loc)
	}
	code, body = emp.do("GET", "/api/auth/me", nil, nil)
	expect(t, "登录后的 me", code, 200, body)
	me := decodeAs[struct {
		Username    string
		DisplayName string `json:"display_name"`
		Department  string
		AuthSource  string `json:"auth_source"`
		Roles       []struct{ ID int64 }
	}](t, body)
	if me.Username != "lin.na" || me.DisplayName != "林娜" || me.Department != "销售部" || me.AuthSource != "oidc" {
		t.Fatalf("资料不对：%s", body)
	}
	hasRole := func(want int64) bool {
		_, b := emp.do("GET", "/api/auth/me", nil, nil)
		for _, r := range decodeAs[struct{ Roles []struct{ ID int64 } }](t, b).Roles {
			if r.ID == want {
				return true
			}
		}
		return false
	}
	if !hasRole(roleID) || len(me.Roles) != 2 {
		t.Fatalf("应有员工 + 映射的销售主管：%s", body)
	}
	code, body = emp.do("POST", "/api/auth/password", map[string]string{"old_password": "x", "new_password": "new-password-123"}, nil)
	expect(t, "OIDC 账号不能改本地密码", code, 400, body)
	code, body = newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"username": "lin.na", "password": ""}, nil)
	if code == 200 {
		t.Fatal("OIDC 账号不能用空密码登录")
	}

	// 再次登录：离开了组 → 映射的角色收回，资料更新
	again := newClient(t, ts.URL)
	idp.login(t, again, map[string]any{"sub": "u-1", "preferred_username": "lin.na", "name": "林娜（销售）", "groups": []string{}}, nil)
	emp = again
	if hasRole(roleID) {
		t.Fatal("离开组后应收回映射的角色")
	}

	// ---- 用户名声明变了、sub 没变：沿用原账号，不新建第二个 ----
	renamedFirst := newClient(t, ts.URL)
	if loc := idp.login(t, renamedFirst, map[string]any{"sub": "u-8", "preferred_username": "ren.ame", "name": "任阿梅"}, nil); loc != "/build" {
		t.Fatalf("改名用例首次登录：%s", loc)
	}
	code, body = renamedFirst.do("GET", "/api/auth/me", nil, nil)
	expect(t, "改名用例 me", code, 200, body)
	before := decodeAs[struct {
		ID       int64
		Username string
	}](t, body)
	renamedSecond := newClient(t, ts.URL)
	if loc := idp.login(t, renamedSecond, map[string]any{"sub": "u-8", "preferred_username": "ren.ame.changed", "name": "任阿梅（改名）"}, nil); loc != "/build" {
		t.Fatalf("改名后应仍登录原账号：%s", loc)
	}
	code, body = renamedSecond.do("GET", "/api/auth/me", nil, nil)
	expect(t, "改名后 me", code, 200, body)
	after := decodeAs[struct {
		ID          int64
		Username    string
		DisplayName string `json:"display_name"`
	}](t, body)
	if after.ID != before.ID || after.Username != "ren.ame" || after.DisplayName != "任阿梅（改名）" {
		t.Fatalf("用户名声明变化不应新建账号：before=%+v after=%+v", before, after)
	}
	code, body = admin.do("GET", "/api/admin/users", nil, nil)
	expect(t, "用户列表", code, 200, body)
	names := map[string]int{}
	for _, u := range decodeAs[struct {
		Users []struct{ Username string }
	}](t, body).Users {
		names[u.Username]++
	}
	if names["ren.ame"] != 1 || names["ren.ame.changed"] != 0 {
		t.Fatalf("改名后不应出现第二个账号：%v", names)
	}

	// ---- 失败的情况都回到登录页并说明原因，不建会话 ----
	fail := func(what string, claims map[string]any, tamper func(url.Values), want string) {
		t.Helper()
		c := newClient(t, ts.URL)
		loc := idp.login(t, c, claims, tamper)
		if !strings.HasPrefix(loc, "/login?oidc_error=") {
			t.Fatalf("%s：应回到登录页：%s", what, loc)
		}
		msg, _ := url.QueryUnescape(strings.TrimPrefix(loc, "/login?oidc_error="))
		if !strings.Contains(msg, want) {
			t.Fatalf("%s：原因应包含「%s」：%s", what, want, msg)
		}
		if code, _ := c.do("GET", "/api/auth/me", nil, nil); code != 401 {
			t.Fatalf("%s：不应登录成功", what)
		}
	}
	fail("和本地管理员重名", map[string]any{"sub": "u-2", "preferred_username": "admin"}, nil, "同名")
	fail("state 被换掉", map[string]any{"sub": "u-3", "preferred_username": "x.y"}, func(q url.Values) { q.Set("state", "forged") }, "失效")
	idp.forge = true
	fail("签名不对", map[string]any{"sub": "u-4", "preferred_username": "x.z"}, nil, "签名")
	idp.forge = false
	fail("nonce 不对", map[string]any{"sub": "u-5", "preferred_username": "x.w", "nonce": "other"}, nil, "nonce")
	fail("IdP 返回错误", map[string]any{"sub": "u-6", "preferred_username": "x.v"}, func(q url.Values) { q.Set("error", "access_denied") }, "access_denied")

	// ---- 账号认的是 sub：用户名相同、sub 不同的人登不进别人的账号 ----
	fail("用户名相同、sub 不同", map[string]any{"sub": "u-other", "preferred_username": "lin.na@partner.example", "name": "冒名"}, nil, "另一个")
	fail("没有 sub", map[string]any{"preferred_username": "no.sub"}, nil, "sub")
	code, body = emp.do("GET", "/api/auth/me", nil, nil)
	expect(t, "原主人的会话不受影响", code, 200, body)
	if !strings.Contains(string(body), "林娜（销售）") {
		t.Fatalf("资料不应被别人的声明改掉：%s", body)
	}
	lin := decodeAs[struct{ ID int64 }](t, body).ID
	if loc := idp.login(t, newClient(t, ts.URL), map[string]any{"sub": "u-1", "preferred_username": "lin.na"}, nil); loc != "/build" {
		t.Fatalf("原主人应照常登录：%s", loc)
	}
	// IdP 里重建了账号（sub 变了）：管理员解除绑定后由新的 sub 重新绑定，旧会话下线
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", lin), map[string]any{"unbind_identity": true}, nil)
	expect(t, "解除身份绑定", code, 200, body)
	if code, _ := emp.do("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatal("解除绑定后旧会话应下线")
	}
	if loc := idp.login(t, newClient(t, ts.URL), map[string]any{"sub": "u-1b", "preferred_username": "lin.na"}, nil); loc != "/build" {
		t.Fatalf("解除绑定后新的 sub 应能登录并绑定：%s", loc)
	}
	fail("重新绑定后的旧 sub", map[string]any{"sub": "u-1", "preferred_username": "lin.na"}, nil, "另一个")
	code, body = admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "管理员 me", code, 200, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", decodeAs[struct{ ID int64 }](t, body).ID), map[string]any{"unbind_identity": true}, nil)
	expect(t, "本地账号没有可解除的绑定", code, 400, body)

	// 关掉自动建号：没预先建好的账号不能登录；管理员预建的 OIDC 账号可以
	cfg["autoCreate"] = false
	delete(cfg, "clientSecret")
	code, body = admin.do("PUT", "/api/admin/auth/oidc", cfg, nil)
	expect(t, "关自动建号", code, 200, body)
	fail("没开通", map[string]any{"sub": "u-7", "preferred_username": "new.guy"}, nil, "还没有开通")
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "new.guy", "auth_source": "oidc"}, nil)
	expect(t, "预建 OIDC 账号", code, 201, body)
	ok := newClient(t, ts.URL)
	if loc := idp.login(t, ok, map[string]any{"sub": "u-7", "preferred_username": "new.guy"}, nil); loc != "/build" {
		t.Fatalf("预建的账号应能登录（client secret 沿用已存的）：%s", loc)
	}
	code, body = admin.do("GET", "/api/admin/audit?action=auth.login", nil, nil)
	expect(t, "审计", code, 200, body)
	if !strings.Contains(string(body), `"via":"oidc"`) {
		t.Fatalf("审计里应记下 OIDC 登录：%s", body)
	}
}
