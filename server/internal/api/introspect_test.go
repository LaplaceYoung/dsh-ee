package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

type introspectBody struct {
	Active     bool       `json:"active"`
	Reason     string     `json:"reason"`
	Kind       string     `json:"kind"`
	TokenID    string     `json:"token_id"`
	Legacy     bool       `json:"legacy"`
	ClientKind string     `json:"client_kind"`
	ClientName string     `json:"client_name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	User       *struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		Department  string `json:"department"`
		AccountType string `json:"account_type"`
	} `json:"user"`
}

func decodeIntrospect(t *testing.T, body []byte) introspectBody {
	t.Helper()
	var out introspectBody
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("解析 introspection 响应：%v %s", err, body)
	}
	return out
}

// 令牌自省：Bearer 自身或管理员查询；返回 active/scopes/client_kind/expires_at/user，不回哈希。
func TestTokenIntrospection(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "introspect-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	scopes := []string{rbac.ScopeModels, rbac.ScopeRegistryRead, rbac.ScopeHub, rbac.ScopeConnections}
	code, body = c.do("POST", "/api/me/tokens", map[string]any{
		"name": "claude-code", "scopes": scopes, "client_kind": "third_party", "client_name": "Claude Code",
	}, nil)
	expect(t, "建 third_party 令牌", code, 201, body)
	created := decodeAs[struct {
		Token string `json:"token"`
		Info  struct {
			ID         string `json:"id"`
			ClientKind string `json:"client_kind"`
		} `json:"info"`
	}](t, body)
	if created.Token == "" || created.Info.ID == "" {
		t.Fatalf("创建令牌响应不对：%s", body)
	}
	anon := newClient(t, f.ts.URL)

	// 没有凭据 / 无效 Bearer：401
	code, body = anon.do("POST", "/api/auth/introspect", nil, nil)
	expect(t, "没有凭据", code, 401, body)
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer("ee_at_missing"))
	expect(t, "无效令牌", code, 401, body)
	if !strings.Contains(string(body), "token_invalid") {
		t.Fatalf("无效令牌错误码不对：%s", body)
	}

	// self-introspection：Bearer 自己，body 留空
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer(created.Token))
	expect(t, "self introspection", code, 200, body)
	info := decodeIntrospect(t, body)
	if !info.Active || info.Kind != "access" || info.ClientKind != "third_party" || info.ClientName != "Claude Code" || info.Legacy {
		t.Fatalf("self 结果不对：%s", body)
	}
	if info.TokenID != created.Info.ID || info.User == nil || info.User.Username != u.Username || info.User.Department != u.Department {
		t.Fatalf("self 归属不对：%s", body)
	}
	if len(info.Scopes) != len(scopes) {
		t.Fatalf("self scope 不对：%v", info.Scopes)
	}
	if strings.Contains(string(body), secret.Hash(created.Token)) || strings.Contains(string(body), "token_hash") {
		t.Fatalf("introspection 不能返回令牌哈希：%s", body)
	}
	// body 与 Bearer 是同一枚令牌：按 self 处理
	code, body = anon.do("POST", "/api/auth/introspect", map[string]any{"token": created.Token}, bearer(created.Token))
	expect(t, "同一枚令牌按 self", code, 200, body)
	if !decodeIntrospect(t, body).Active {
		t.Fatalf("同一枚令牌应按 self 返回 active：%s", body)
	}

	// 普通用户不能用 body 查别的令牌（网页会话或 Bearer 都是 403）
	f.token(t, u.ID, "ee_at_other", scopes, nil, nil)
	code, body = c.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_other"}, nil)
	expect(t, "普通会话查别的令牌", code, 403, body)
	code, body = anon.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_other"}, bearer(created.Token))
	expect(t, "普通 Bearer 查别的令牌", code, 403, body)

	// 管理员网页会话可以查任意有效令牌
	code, body = f.admin.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_other"}, nil)
	expect(t, "管理员查令牌", code, 200, body)
	adminView := decodeIntrospect(t, body)
	if !adminView.Active || adminView.Kind != "access" || adminView.User == nil || adminView.User.Username != u.Username {
		t.Fatalf("管理员查到的令牌不对：%s", body)
	}

	// 过期：管理员查 → active:false + reason=expired，仍给出 expires_at，不带哈希/明文
	expired := f.token(t, u.ID, "ee_at_introspect_expired", scopes, nil, nil)
	past := time.Now().Add(-time.Hour)
	if _, err := f.st.DB().ExecContext(context.Background(), "UPDATE access_tokens SET expires_at = ? WHERE id = ?", past, expired.ID); err != nil {
		t.Fatal(err)
	}
	code, body = f.admin.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_introspect_expired"}, nil)
	expect(t, "查过期令牌", code, 200, body)
	expiredView := decodeIntrospect(t, body)
	if expiredView.Active || expiredView.Reason != "expired" || expiredView.ExpiresAt == nil {
		t.Fatalf("过期令牌结果不对：%s", body)
	}
	if strings.Contains(string(body), secret.Hash("ee_at_introspect_expired")) {
		t.Fatalf("introspection 不能返回令牌哈希：%s", body)
	}

	// 吊销：reason=revoked
	revoked := f.token(t, u.ID, "ee_at_introspect_revoked", scopes, nil, nil)
	if err := f.st.RevokeToken(context.Background(), revoked.ID); err != nil {
		t.Fatal(err)
	}
	code, body = f.admin.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_introspect_revoked"}, nil)
	expect(t, "查吊销令牌", code, 200, body)
	if v := decodeIntrospect(t, body); v.Active || v.Reason != "revoked" {
		t.Fatalf("吊销令牌结果不对：%s", body)
	}

	// 不认识的值：active:false + not_found，不猜内容
	code, body = f.admin.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_does_not_exist"}, nil)
	expect(t, "查不存在的令牌", code, 200, body)
	if v := decodeIntrospect(t, body); v.Active || v.Reason != "not_found" {
		t.Fatalf("不存在令牌结果不对：%s", body)
	}

	// 管理员令牌（Bearer，开了管理模式）同样可以查别人的令牌
	var me struct {
		ID int64 `json:"id"`
	}
	code, body = f.admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "管理员 me", code, 200, body)
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatal(err)
	}
	adminTok := f.token(t, me.ID, "ee_at_introspect_admin", []string{rbac.ScopeHub}, nil, nil)
	code, body = f.admin.do("POST", "/api/me/tokens/at:"+itoa(adminTok.ID)+"/elevate", map[string]any{"password": "admin-password-123", "hours": 1}, nil)
	expect(t, "管理员令牌开启管理模式", code, 200, body)
	code, body = anon.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_other"}, bearer("ee_at_introspect_admin"))
	expect(t, "管理员 Bearer 查令牌", code, 200, body)
	if v := decodeIntrospect(t, body); !v.Active {
		t.Fatalf("管理员 Bearer 应能查到 active 令牌：%s", body)
	}

	// 旧版 npm 令牌也能 self-introspection：kind=npm、legacy、client_kind=dsh
	code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "legacy-npm", "access": "read"}, nil)
	expect(t, "建旧 npm 令牌", code, 201, body)
	npmToken := decodeAs[struct {
		Token string `json:"token"`
	}](t, body).Token
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer(npmToken))
	expect(t, "旧 npm 令牌自省", code, 200, body)
	npmView := decodeIntrospect(t, body)
	if !npmView.Active || npmView.Kind != "npm" || !npmView.Legacy || npmView.ClientKind != "dsh" {
		t.Fatalf("旧 npm 令牌结果不对：%s", body)
	}
	if len(npmView.Scopes) == 0 || npmView.User == nil || npmView.User.Username != u.Username {
		t.Fatalf("旧 npm 令牌 scope/归属不对：%s", body)
	}
	for _, want := range []string{rbac.ScopeModels, rbac.ScopeRegistryRead} {
		if !strings.Contains(string(body), `"`+want+`"`) {
			t.Fatalf("默认（未收敛）的旧 npm 令牌应有 %s scope：%s", want, body)
		}
	}

	// 旧版 API key 也能 self-introspection：kind=key、legacy
	code, body = c.do("POST", "/api/me/keys", map[string]any{"name": "legacy-key"}, nil)
	expect(t, "建旧 API key", code, 201, body)
	keyPlain := decodeAs[struct {
		Key string `json:"key"`
	}](t, body).Key
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer(keyPlain))
	expect(t, "旧 API key 自省", code, 200, body)
	keyView := decodeIntrospect(t, body)
	if !keyView.Active || keyView.Kind != "key" || !keyView.Legacy || keyView.TokenID == "" {
		t.Fatalf("旧 API key 结果不对：%s", body)
	}
}

// 旧令牌 scope 收敛（DSH_EE_RESTRICT_LEGACY_TOKEN_SCOPES）：默认不改；打开后 ee_npm_ 只剩 registry、
// sk-ee- 只剩模型网关，自省能看出收窄后的 scope。
func TestLegacyTokenScopesRestricted(t *testing.T) {
	f := newPrincipalFixtureWith(t, func(o *api.Options) { o.RestrictLegacyTokenScopes = true })
	u := f.user(t, "restrict-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)
	anon := newClient(t, f.ts.URL)

	// npm 令牌：只剩 registry:read（+ publish 视 access），仍可用于 npm 协议
	code, body = c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "restricted-npm", "access": "read"}, nil)
	expect(t, "建 npm 令牌", code, 201, body)
	npmToken := decodeAs[struct {
		Token string `json:"token"`
	}](t, body).Token
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer(npmToken))
	expect(t, "收敛后 npm 令牌自省", code, 200, body)
	npmView := decodeIntrospect(t, body)
	if !npmView.Active || npmView.Kind != "npm" || len(npmView.Scopes) != 1 || npmView.Scopes[0] != rbac.ScopeRegistryRead {
		t.Fatalf("收敛后的 npm 令牌应只有 registry:read：%s", body)
	}
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("x:"+npmToken))
	code, body = anon.do("GET", "/npm/-/whoami", nil, map[string]string{"Authorization": basic})
	expect(t, "收敛后 npm 协议仍可用", code, 200, body)
	// 但不能再读工作台目录 / 调 MCP
	code, body = anon.do("GET", "/api/dsh/catalog", nil, bearer(npmToken))
	expect(t, "收敛后 npm 令牌不能读工作台", code, 403, body)
	code, body = anon.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, map[string]string{
		"Authorization": "Bearer " + npmToken, "Accept": "application/json, text/event-stream",
	})
	expect(t, "收敛后 npm 令牌不能调 MCP", code, 403, body)

	// API key：只剩 models，仍可调模型网关，不能读工作台
	code, body = c.do("POST", "/api/me/keys", map[string]any{"name": "restricted-key"}, nil)
	expect(t, "建 API key", code, 201, body)
	keyPlain := decodeAs[struct {
		Key string `json:"key"`
	}](t, body).Key
	code, body = anon.do("POST", "/api/auth/introspect", nil, bearer(keyPlain))
	expect(t, "收敛后 API key 自省", code, 200, body)
	keyView := decodeIntrospect(t, body)
	if !keyView.Active || keyView.Kind != "key" || len(keyView.Scopes) != 1 || keyView.Scopes[0] != rbac.ScopeModels {
		t.Fatalf("收敛后的 API key 应只有 models：%s", body)
	}
	code, body = anon.do("GET", "/v1/models", nil, bearer(keyPlain))
	expect(t, "收敛后 API key 仍可调模型", code, 200, body)
	code, body = anon.do("GET", "/api/dsh/catalog", nil, bearer(keyPlain))
	expect(t, "收敛后 API key 不能读工作台", code, 403, body)

	// whoami 暴露收敛开关，便于客户端诊断（whoami 需要 hub scope，所以用一个新令牌查）
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "diagnose", "scopes": []string{rbac.ScopeHub}}, nil)
	expect(t, "建诊断令牌", code, 201, body)
	diagnose := decodeAs[struct {
		Token string `json:"token"`
	}](t, body).Token
	code, body = anon.do("GET", "/api/dsh/whoami", nil, bearer(diagnose))
	expect(t, "whoami", code, 200, body)
	if !strings.Contains(string(body), `"legacy_scopes_restricted":true`) {
		t.Fatalf("whoami 应暴露 legacy_scopes_restricted：%s", body)
	}
}

// 从 DSH 打开的 limited 网页会话只带员工权限：即使账号本身是超管（有 keys.manage），
// 也不能用 POST /api/auth/introspect 查别人的令牌（修复前会被当成全量权限会话，
// 返回 200 和属主用户名/部门、client_name、scope 等元数据）。
func TestIntrospectLimitedWebSession(t *testing.T) {
	f := newPrincipalFixture(t)
	// 账号本身有管理权限（超管），是这条越权路径成立的前提
	var me struct {
		ID      int64 `json:"id"`
		IsSuper bool  `json:"is_super"`
	}
	code, body := f.admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "管理员 me", code, 200, body)
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatal(err)
	}
	if !me.IsSuper {
		t.Fatalf("测试需要超级管理员账号：%s", body)
	}

	// 另一个用户的令牌：limited 会话不应该通过 introspect 看到它的元数据
	target := f.user(t, "introspect-limited-target", nil)
	f.token(t, target.ID, "ee_at_limited_target", []string{rbac.ScopeHub}, nil, nil)

	// 走真实链路换 limited 网页会话：超管的设备令牌 → 一次性链接 → handoff 写 cookie
	f.token(t, me.ID, "ee_at_limited_device", []string{rbac.ScopeAgent, rbac.ScopeHub}, nil, nil)
	dsh := newClient(t, f.ts.URL)
	code, body = dsh.do("POST", "/api/dsh/web-link", map[string]string{"next": "/go"}, bearer("ee_at_limited_device"))
	expect(t, "换受限网页登录链接", code, 200, body)
	var link struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &link); err != nil || link.URL == "" {
		t.Fatalf("网页登录链接响应不对：%v %s", err, body)
	}
	b := newClient(t, f.ts.URL)
	resp, err := b.hc.Get(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// fixture 自检：会话确实是 limited、账号本身有管理权限（reauth=true）
	code, body = b.do("GET", "/api/auth/me", nil, nil)
	expect(t, "受限会话读 me", code, 200, body)
	var limitedMe struct {
		IsSuper bool `json:"is_super"`
		Limited bool `json:"limited"`
		Reauth  bool `json:"reauth"`
	}
	if err := json.Unmarshal(body, &limitedMe); err != nil {
		t.Fatal(err)
	}
	if limitedMe.IsSuper || !limitedMe.Limited || !limitedMe.Reauth {
		t.Fatalf("fixture 应是「账号是超管但会话受限」：%s", body)
	}

	// 越权点：查别人的令牌必须 403，且响应里不能带出令牌元数据
	code, body = b.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_limited_target"}, nil)
	expect(t, "受限会话查别人的令牌", code, 403, body)
	if strings.Contains(string(body), target.Username) || strings.Contains(string(body), "ee_at_limited_target") {
		t.Fatalf("403 响应不能泄露令牌元数据：%s", body)
	}
	// 查自己名下的令牌也一样：不能借网页会话的账号权限绕过 keys.manage 检查（比修复前更严）
	code, body = b.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_limited_device"}, nil)
	expect(t, "受限会话查自己的令牌", code, 403, body)

	// 回归：同样的请求换成完整网页会话（输入密码登录后）仍然 200，字段完整
	code, body = f.admin.do("POST", "/api/auth/introspect", map[string]any{"token": "ee_at_limited_target"}, nil)
	expect(t, "完整会话查令牌", code, 200, body)
	view := decodeIntrospect(t, body)
	if !view.Active || view.Kind != "access" || view.TokenID == "" || view.ClientName != "test-client" ||
		view.User == nil || view.User.Username != target.Username || view.User.Department != target.Department {
		t.Fatalf("完整会话查到的令牌元数据不全：%s", body)
	}
}
