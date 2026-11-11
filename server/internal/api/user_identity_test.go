package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/eejwt"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// eeVerifier 是企业系统那一侧：从 dsh-ee 的 JWKS 取公钥，验每个请求带的 ee_jwt。
type eeVerifier struct {
	issuer string
}

func (v eeVerifier) verify(t *testing.T, token, aud string) (map[string]any, error) {
	t.Helper()
	resp, err := http.Get(v.issuer + "/.well-known/jwks.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var set eejwt.JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		t.Fatal(err)
	}
	return eejwt.Verify(token, set, eejwt.Expect{Issuer: v.issuer, Audience: aud})
}

// 按员工本人的身份调用企业系统：ee_jwt（dsh-ee 现签的身份令牌）和 user_secret（员工自己填的凭据）。
func TestPerUserIdentity(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), EEKeyDelay: 200 * time.Millisecond}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	verifier := eeVerifier{issuer: ts.URL}

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"], set["baseUrl"] = "@acme", ts.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "设置访问地址", code, 200, body)

	users := map[string]*client{}
	dsh := map[string]dshClient{}
	ids := map[string]int64{}
	for _, u := range []struct{ name, dept string }{{"alice", "研发效能部"}, {"bob", "财务部"}} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": u.name, "password": u.name + "-password-1", "department": u.dept, "display_name": strings.ToUpper(u.name)}, nil)
		expect(t, "建用户", code, 201, body)
		ids[u.name] = decodeAs[struct{ ID int64 }](t, body).ID
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": u.name, "password": u.name + "-password-1"}, nil)
		expect(t, "登录", code, 200, body)
		code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
		expect(t, "接入令牌", code, 201, body)
		users[u.name] = c
		dsh[u.name] = dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	alice, bob := users["alice"], users["bob"]
	call := func(who, tool string, args map[string]any) (string, bool) {
		t.Helper()
		res := dsh[who].rpc("tools/call", map[string]any{"name": tool, "arguments": args})
		r, _ := res["result"].(map[string]any)
		if r == nil {
			t.Fatalf("%s 调 %s：%v", who, tool, res)
		}
		text := ""
		for _, c := range r["content"].([]any) {
			text += fmt.Sprint(c.(map[string]any)["text"])
		}
		isErr, _ := r["isError"].(bool)
		return text, isErr
	}
	connect := func(what string, req map[string]any, want int) (int64, string) {
		t.Helper()
		req["kind"], req["delivery"] = "connection", "auto"
		code, body := admin.do("POST", "/api/hub", req, nil)
		expect(t, what, code, want, body)
		return decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID, string(body)
	}

	// ---- OpenAPI 连接 + ee_jwt：共享的 API key 和员工身份各放一个请求头 ----
	var hrAud atomic.Value
	hrAud.Store("hr")
	var lastToken atomic.Value
	hr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "hr-key" {
			http.Error(w, "bad api key", 401)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if h := r.Header.Get("X-EE-Token"); h != "" {
			token = h
		}
		claims, err := verifier.verify(t, token, hrAud.Load().(string))
		if err != nil {
			http.Error(w, "bad ee_jwt: "+err.Error(), 401)
			return
		}
		lastToken.Store(token)
		_ = json.NewEncoder(w).Encode(claims)
	}))
	defer hr.Close()
	doc := `{"openapi":"3.0.0","info":{"title":"HR"},"paths":{"/me":{"get":{"operationId":"whoami"}}}}`
	hrSpec := func(userAuth map[string]any) map[string]any {
		return map[string]any{"title": "HR", "type": "openapi", "url": hr.URL, "openapi": doc,
			"auth": map[string]any{"type": "header", "header": "X-API-Key"}, "user_auth": userAuth}
	}
	hrID, _ := connect("接入 HR", map[string]any{"slug": "hr", "spec": hrSpec(map[string]any{"mode": "ee_jwt", "claims": []string{"dept", "roles"}}), "secret": "hr-key"}, 201)

	whoami := func(who string) map[string]any {
		t.Helper()
		text, isErr := call(who, "hr__whoami", nil)
		if isErr {
			t.Fatalf("%s 调 HR 失败：%s", who, text)
		}
		return decodeAs[map[string]any](t, []byte(text))
	}
	// 全新的部署上第一次用到签名密钥就是一次工具调用：钥匙现生成，这一次就要能签
	got := whoami("bob")

	// ---- JWKS 不用登录就能取 ----
	code, body = newClient(t, ts.URL).do("GET", "/.well-known/jwks.json", nil, nil)
	expect(t, "JWKS", code, 200, body)
	first := decodeAs[eejwt.JWKS](t, body)
	if len(first.Keys) != 1 || first.Keys[0].Alg != "RS256" || first.Keys[0].Kid == "" {
		t.Fatalf("JWKS 应有一把 RS256 公钥：%s", body)
	}
	if got["preferred_username"] != "bob" || got["name"] != "BOB" || got["dept"] != "财务部" || got["sub"] != fmt.Sprint(ids["bob"]) ||
		got["iss"] != ts.URL || got["aud"] != "hr" || got["jti"] == "" || got["jti"] == nil {
		t.Fatalf("bob 的令牌内容不对：%v", got)
	}
	if roles, _ := got["roles"].([]any); len(roles) == 0 {
		t.Fatalf("打开了 roles，令牌里应有角色：%v", got)
	}
	if exp, iat := got["exp"].(float64), got["iat"].(float64); exp-iat != eejwt.Lifetime.Seconds() {
		t.Fatalf("令牌有效期应是 %v：exp-iat=%v", eejwt.Lifetime, exp-iat)
	}
	if got := whoami("alice"); got["preferred_username"] != "alice" || got["dept"] != "研发效能部" {
		t.Fatalf("alice 调用时令牌应是 alice 的：%v", got)
	}
	bobToken := lastToken.Load()

	// 换个请求头、指定 aud、不带额外信息
	newVersion := func(what string, id int64, spec map[string]any, want int) string {
		t.Helper()
		code, body := admin.do("POST", fmt.Sprintf("/api/hub/%d/versions", id), map[string]any{"spec": spec}, nil)
		expect(t, what, code, want, body)
		return string(body)
	}
	hrAud.Store("urn:acme:hr")
	newVersion("改成自定义请求头和 aud", hrID, hrSpec(map[string]any{"mode": "ee_jwt", "header": "X-EE-Token", "audience": "urn:acme:hr"}), 201)
	got = whoami("bob")
	if got["aud"] != "urn:acme:hr" || got["dept"] != nil || got["roles"] != nil {
		t.Fatalf("没打开的员工信息不应出现在令牌里，aud 应是配置的：%v", got)
	}

	// 共享凭据和员工身份撞在同一个请求头上、模式或员工信息写错，都存不进去
	for _, bad := range []struct {
		what string
		spec map[string]any
		want string
	}{
		{"同一个请求头", map[string]any{"title": "X", "type": "openapi", "url": hr.URL, "openapi": doc, "auth": map[string]any{"type": "bearer"}, "user_auth": map[string]any{"mode": "ee_jwt"}}, "同一个请求头"},
		{"模式不对", hrSpec(map[string]any{"mode": "oauth"}), "ee_jwt 或 user_secret"},
		{"员工信息不对", hrSpec(map[string]any{"mode": "ee_jwt", "claims": []string{"salary"}}), "dept / roles / idp_sub"},
		{"请求头名字不对", hrSpec(map[string]any{"mode": "user_secret", "header": "X Bad"}), "请求头名字"},
	} {
		if out := newVersion(bad.what, hrID, bad.spec, 400); !strings.Contains(out, bad.want) {
			t.Fatalf("%s：应提示「%s」：%s", bad.what, bad.want, out)
		}
	}

	// ---- 轮换签名密钥：新公钥先公开，过一会儿才开始用它签名；旧公钥之后继续公开，轮换前签的令牌照样能验 ----
	code, body = alice.do("POST", "/api/admin/ee-jwt/rotate", nil, nil)
	expect(t, "员工不能轮换密钥", code, 403, body)
	oldKid := func(tok string) string { return strings.Split(tok, ".")[0] }
	code, body = admin.do("POST", "/api/admin/ee-jwt/rotate", nil, nil)
	expect(t, "轮换密钥", code, 200, body)
	type keyInfo struct {
		Issuer  string
		JwksURL string `json:"jwks_url"`
		Keys    []struct{ Kid, State string }
	}
	info := decodeAs[keyInfo](t, body)
	if info.Issuer != ts.URL || info.JwksURL != ts.URL+"/.well-known/jwks.json" || len(info.Keys) != 2 || info.Keys[0].State != "active" || info.Keys[1].State != "pending" {
		t.Fatalf("刚轮换时应是旧的在用、新的排队：%s", body)
	}
	if strings.Contains(string(body), "private") {
		t.Fatalf("密钥信息里不能有私钥：%s", body)
	}
	// 还没到时间：公钥已经有两把，签名还是用旧的
	code, body = newClient(t, ts.URL).do("GET", "/.well-known/jwks.json", nil, nil)
	if set := decodeAs[eejwt.JWKS](t, body); len(set.Keys) != 2 {
		t.Fatalf("新公钥应马上公开：%s", body)
	}
	whoami("bob")
	if oldKid(lastToken.Load().(string)) != oldKid(bobToken.(string)) {
		t.Fatal("新钥匙还没到时间，应继续用旧钥匙签名")
	}
	// 已经有一把在排队，再点一次不会再多一把
	code, body = admin.do("POST", "/api/admin/ee-jwt/rotate", nil, nil)
	expect(t, "重复轮换", code, 200, body)
	if again := decodeAs[keyInfo](t, body); len(again.Keys) != 2 || again.Keys[1].Kid != info.Keys[1].Kid {
		t.Fatalf("排队中的钥匙只应有一把：%s", body)
	}
	// 到时间后换用新钥匙；轮换前签的令牌在宽限期内照样能验
	time.Sleep(300 * time.Millisecond)
	whoami("bob")
	if oldKid(lastToken.Load().(string)) == oldKid(bobToken.(string)) {
		t.Fatal("到时间后应换用新钥匙签名（kid 应变）")
	}
	code, body = admin.do("GET", "/api/admin/ee-jwt", nil, nil)
	expect(t, "密钥信息", code, 200, body)
	if after := decodeAs[keyInfo](t, body); len(after.Keys) != 2 || after.Keys[0].State != "retired" || after.Keys[1].State != "active" {
		t.Fatalf("到时间后应是旧的换下、新的在用：%s", body)
	}
	hrAud.Store("hr")
	if _, err := verifier.verify(t, bobToken.(string), "hr"); err != nil {
		t.Fatalf("轮换前签的令牌在宽限期内应还能验：%v", err)
	}
	hrAud.Store("urn:acme:hr")

	// ---- OpenAPI 连接 + user_secret：每人用自己填的凭据 ----
	var oaCalls atomic.Int32
	oa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oaCalls.Add(1)
		_, _ = fmt.Fprintf(w, `{"auth":%q}`, r.Header.Get("Authorization"))
	}))
	defer oa.Close()
	oaDoc := `{"openapi":"3.0.0","info":{"title":"OA"},"paths":{"/todo":{"get":{"operationId":"todo"}}}}`
	oaID, _ := connect("接入 OA", map[string]any{"slug": "oa", "scope": map[string]any{"departments": []string{"财务部"}},
		"spec": map[string]any{"title": "OA", "type": "openapi", "url": oa.URL, "openapi": oaDoc, "user_auth": map[string]any{"mode": "user_secret", "hint": "在 OA 的「个人设置 → 开放接口」里生成"}}}, 201)
	cred := fmt.Sprintf("/api/me/connections/%d/credential", oaID)

	text, isErr := call("bob", "oa__todo", nil)
	if !isErr || !strings.Contains(text, "需要你本人的凭据") || !strings.Contains(text, fmt.Sprintf("%s/assets/%d", ts.URL, oaID)) || oaCalls.Load() != 0 {
		t.Fatalf("没填凭据时应提示去填、不发请求：%s（上游被调 %d 次）", text, oaCalls.Load())
	}
	code, body = bob.do("PUT", cred, map[string]string{"secret": "  "}, nil)
	expect(t, "空凭据", code, 400, body)
	code, body = alice.do("PUT", cred, map[string]string{"secret": "alice-token"}, nil)
	expect(t, "范围外的人不能给这个连接填凭据", code, 404, body)
	code, body = bob.do("PUT", fmt.Sprintf("/api/me/connections/%d/credential", hrID), map[string]string{"secret": "x"}, nil)
	expect(t, "ee_jwt 的连接不用填凭据", code, 400, body)
	code, body = bob.do("PUT", cred, map[string]string{"secret": "bob-token-1234"}, nil)
	expect(t, "bob 填凭据", code, 204, body)
	if text, isErr := call("bob", "oa__todo", nil); isErr || !strings.Contains(text, "Bearer bob-token-1234") {
		t.Fatalf("应带上 bob 自己的凭据：%s", text)
	}
	code, body = admin.do("PUT", cred, map[string]string{"secret": "admin-token-9999"}, nil)
	expect(t, "管理员填自己的", code, 204, body)
	if text, _ := call("bob", "oa__todo", nil); !strings.Contains(text, "bob-token-1234") {
		t.Fatalf("别人填了凭据不影响 bob：%s", text)
	}

	// 员工看得到自己填没填，看不到全文；管理员看得到谁填了，看不到内容
	code, body = bob.do("GET", "/api/me/connections", nil, nil)
	expect(t, "我的连接", code, 200, body)
	mine := decodeAs[struct {
		Connections []struct {
			ID         int64
			Slug       string
			Credential *struct {
				Mode, Hint, Tail string
				Filled           bool
			}
		}
	}](t, body)
	seen := map[string]bool{}
	for _, c := range mine.Connections {
		seen[c.Slug] = true
		switch c.Slug {
		case "oa":
			if c.Credential == nil || c.Credential.Mode != "user_secret" || !c.Credential.Filled || c.Credential.Tail == "" || !strings.Contains(c.Credential.Hint, "开放接口") || c.ID != oaID {
				t.Fatalf("OA 应显示已填：%s", body)
			}
		case "hr":
			if c.Credential == nil || c.Credential.Mode != "ee_jwt" || !c.Credential.Filled {
				t.Fatalf("HR 用 ee_jwt，不用填：%s", body)
			}
		}
	}
	if !seen["oa"] || !seen["hr"] || strings.Contains(string(body), "bob-token-1234") {
		t.Fatalf("我的连接：%s", body)
	}
	code, body = bob.do("GET", fmt.Sprintf("/api/hub/%d", oaID), nil, nil)
	expect(t, "连接详情", code, 200, body)
	if !strings.Contains(string(body), `"my_credential":{"mode":"user_secret"`) || strings.Contains(string(body), "bob-token-1234") {
		t.Fatalf("连接详情应带我的凭据状态、不带内容：%s", body)
	}
	holders := fmt.Sprintf("/api/admin/connections/%d/credentials", oaID)
	code, body = bob.do("GET", holders, nil, nil)
	expect(t, "员工看不到谁填了凭据", code, 403, body)
	code, body = admin.do("GET", holders, nil, nil)
	expect(t, "谁填了凭据", code, 200, body)
	if !strings.Contains(string(body), `"username":"bob"`) || !strings.Contains(string(body), `"username":"admin"`) || strings.Contains(string(body), "token") || strings.Contains(string(body), "1234") {
		t.Fatalf("管理员只能看到谁填了：%s", body)
	}
	code, body = admin.do("GET", fmt.Sprintf("/api/hub/%d", oaID), nil, nil)
	expect(t, "管理员看连接详情", code, 200, body)
	if !strings.Contains(string(body), `"auth_mode":"user_secret"`) {
		t.Fatalf("调用记录应记下员工身份的方式：%s", body)
	}

	// 管理员收回、员工自己删，之后都要重新填
	code, body = admin.do("DELETE", fmt.Sprintf("%s/%d", holders, ids["bob"]), nil, nil)
	expect(t, "收回 bob 的凭据", code, 204, body)
	if text, isErr := call("bob", "oa__todo", nil); !isErr || !strings.Contains(text, "需要你本人的凭据") {
		t.Fatalf("收回后应要求重新填：%s", text)
	}
	code, body = admin.do("DELETE", fmt.Sprintf("%s/%d", holders, ids["bob"]), nil, nil)
	expect(t, "没填过的收不回", code, 404, body)
	code, body = bob.do("PUT", cred, map[string]string{"secret": "bob-token-5678"}, nil)
	expect(t, "bob 重新填", code, 204, body)
	code, body = bob.do("DELETE", cred, nil, nil)
	expect(t, "bob 自己删", code, 204, body)
	if text, isErr := call("bob", "oa__todo", nil); !isErr || !strings.Contains(text, "需要你本人的凭据") {
		t.Fatalf("自己删掉后应要求重新填：%s", text)
	}

	// 停用员工时，他按人存的凭据一起删掉
	code, body = bob.do("PUT", cred, map[string]string{"secret": "bob-token-5678"}, nil)
	expect(t, "bob 再填一次", code, 204, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", ids["bob"]), map[string]any{"disabled": true}, nil)
	expect(t, "停用 bob", code, 200, body)
	code, body = admin.do("GET", holders, nil, nil)
	if strings.Contains(string(body), `"username":"bob"`) {
		t.Fatalf("停用后 bob 的凭据应已删除：%s", body)
	}
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", ids["bob"]), map[string]any{"disabled": false}, nil)
	expect(t, "恢复 bob", code, 200, body)

	// 删掉连接，上面的员工凭据一起删
	code, body = admin.do("DELETE", fmt.Sprintf("/api/hub/%d", oaID), nil, nil)
	expect(t, "删除连接", code, 204, body)
	var left int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM user_credentials WHERE asset_id = ?", oaID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("连接删了，员工凭据不应留着：%d %v", left, err)
	}

	// ---- MCP 连接 + ee_jwt：每个请求都带现签的令牌，每位员工各用各的会话 ----
	var mu sync.Mutex
	sessions := map[string]string{} // 会话 → 建会话的人
	var crossed atomic.Value
	nextSession := atomic.Int32{}
	mcpUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := verifier.verify(t, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "crm")
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}
		who := claims["preferred_username"].(string)
		var m struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any
		switch m.Method {
		case "initialize":
			sid := fmt.Sprintf("s%d", nextSession.Add(1))
			mu.Lock()
			sessions[sid] = who
			mu.Unlock()
			w.Header().Set("Mcp-Session-Id", sid)
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "crm"}}
		case "tools/list":
			ro := true
			result = map[string]any{"tools": []map[string]any{{"name": "mine", "description": "My customers", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": ro}}}}
		case "tools/call":
			mu.Lock()
			owner := sessions[r.Header.Get("Mcp-Session-Id")]
			mu.Unlock()
			if owner != who {
				crossed.Store(fmt.Sprintf("%s 的请求用了 %s 的会话", who, owner))
			}
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "customers of " + who}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": result})
	}))
	defer mcpUp.Close()
	_, created := connect("接入 CRM（MCP）", map[string]any{"slug": "crm", "spec": map[string]any{"title": "CRM", "type": "mcp", "url": mcpUp.URL, "user_auth": map[string]any{"mode": "ee_jwt"}}}, 201)
	if !strings.Contains(created, `"tools":1`) {
		t.Fatalf("同步工具时应用管理员自己的身份去取：%s", created)
	}
	for round := 0; round < 2; round++ {
		for _, who := range []string{"alice", "bob", "alice"} {
			if text, isErr := call(who, "crm__mine", nil); isErr || text != "customers of "+who {
				t.Fatalf("%s 调 CRM：%s", who, text)
			}
		}
	}
	if msg := crossed.Load(); msg != nil {
		t.Fatalf("每位员工应各用各的上游会话：%v", msg)
	}
	mu.Lock()
	n := len(sessions)
	mu.Unlock()
	if n != 3 {
		t.Fatalf("应有管理员、alice、bob 三个会话，实际 %d 个：%v", n, sessions)
	}

	// ---- MCP 连接 + user_secret：同步工具要用管理员自己的凭据 ----
	secretUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("X-User-Token"), "tok-") {
			w.WriteHeader(401)
			return
		}
		var m struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "wiki"}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "pages", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": true}}}}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "pages for " + r.Header.Get("X-User-Token")}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": result})
	}))
	defer secretUp.Close()
	wikiID, created := connect("接入 Wiki（MCP）", map[string]any{"slug": "wiki", "spec": map[string]any{"title": "Wiki", "type": "mcp", "url": secretUp.URL, "user_auth": map[string]any{"mode": "user_secret", "header": "X-User-Token"}}}, 201)
	if !strings.Contains(created, "先在连接页面填上你自己的凭据") {
		t.Fatalf("管理员还没填自己的凭据，同步工具应说明原因：%s", created)
	}
	wikiCred := fmt.Sprintf("/api/me/connections/%d/credential", wikiID)
	code, body = admin.do("PUT", wikiCred, map[string]string{"secret": "tok-admin"}, nil)
	expect(t, "管理员填自己的凭据", code, 204, body)
	code, body = admin.do("POST", fmt.Sprintf("/api/admin/connections/%d/sync", wikiID), nil, nil)
	expect(t, "同步工具", code, 200, body)
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("填了凭据后应能同步：%s", body)
	}
	code, body = alice.do("PUT", wikiCred, map[string]string{"secret": "tok-alice"}, nil)
	expect(t, "alice 填凭据", code, 204, body)
	if text, isErr := call("alice", "wiki__pages", nil); isErr || text != "pages for tok-alice" {
		t.Fatalf("alice 调 Wiki 应带自己的凭据：%s", text)
	}
	// 换了凭据立刻生效，不沿用旧的会话
	code, body = alice.do("PUT", wikiCred, map[string]string{"secret": "tok-alice-2"}, nil)
	expect(t, "alice 换凭据", code, 204, body)
	if text, _ := call("alice", "wiki__pages", nil); text != "pages for tok-alice-2" {
		t.Fatalf("换了凭据后应立刻用新的：%s", text)
	}
	if text, isErr := call("bob", "wiki__pages", nil); !isErr || !strings.Contains(text, "需要你本人的凭据") {
		t.Fatalf("bob 没填，应提示去填：%s", text)
	}

	// ---- 员工调岗后，令牌里的部门跟着变（上游客户端是缓存的，身份不能跟着缓存）----
	var lastDept atomic.Value
	deptUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := verifier.verify(t, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "erp")
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}
		var m struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "erp"}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "dept", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": true}}}}
		case "tools/call":
			lastDept.Store(claims["dept"])
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprint(claims["dept"])}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": result})
	}))
	defer deptUp.Close()
	connect("接入 ERP（MCP）", map[string]any{"slug": "erp", "spec": map[string]any{"title": "ERP", "type": "mcp", "url": deptUp.URL, "user_auth": map[string]any{"mode": "ee_jwt", "claims": []string{"dept"}}}}, 201)
	if text, _ := call("alice", "erp__dept", nil); text != "研发效能部" {
		t.Fatalf("令牌里应带 alice 的部门：%s", text)
	}
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", ids["alice"]), map[string]any{"department": "销售部"}, nil)
	expect(t, "alice 调岗", code, 200, body)
	if text, _ := call("alice", "erp__dept", nil); text != "销售部" {
		t.Fatalf("调岗后令牌里的部门应跟着变：%s", text)
	}

	// ---- 上游重定向到别的主机：不跟过去，凭据不会被带到别处 ----
	var leaked atomic.Value
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("X-API-Key") + "|" + r.Header.Get("X-EE-Token"))
	}))
	defer elsewhere.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusFound)
	}))
	defer redirecting.Close()
	connect("接入一个会重定向的系统", map[string]any{"slug": "moved", "secret": "shared-key",
		"spec": map[string]any{"title": "Moved", "type": "openapi", "url": redirecting.URL, "openapi": oaDoc,
			"auth": map[string]any{"type": "header", "header": "X-API-Key"}, "user_auth": map[string]any{"mode": "ee_jwt", "header": "X-EE-Token"}}}, 201)
	if text, isErr := call("alice", "moved__todo", nil); !isErr || !strings.Contains(text, "重定向到了另一个地址") || leaked.Load() != nil {
		t.Fatalf("重定向到别的主机时应停下，不把凭据带过去：%s（对方收到 %v）", text, leaked.Load())
	}

	// 审计里有填、删、收回、轮换
	code, body = admin.do("GET", "/api/admin/audit?action=connection.&limit=50", nil, nil)
	expect(t, "审计", code, 200, body)
	for _, want := range []string{"connection.credential_set", "connection.credential_delete", "connection.credential_revoke", "connection.ee_jwt_rotate"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("审计里应有 %s", want)
		}
	}
	if strings.Contains(string(body), "tok-") || strings.Contains(string(body), "bob-token") {
		t.Fatalf("审计里不能有凭据内容：%s", body)
	}
}
