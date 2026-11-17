package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
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

// keyCaptureUpstream 是假的 OpenAI 兼容上游：记录每次请求的 Authorization 头与路径，
// 用来断言「哪把 key 被送到了哪个地址」。/models 与 /chat/completions 只接受 allowedKey，
// 便于验证检查与网关用的是哪把 key。
type keyCaptureUpstream struct {
	allowedKey string
	mu         sync.Mutex
	auths      []string
	paths      []string
	srv        *httptest.Server
}

func newKeyCaptureUpstream(t *testing.T, allowedKey string) *keyCaptureUpstream {
	t.Helper()
	u := &keyCaptureUpstream{allowedKey: allowedKey}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.auths = append(u.auths, r.Header.Get("Authorization"))
		u.paths = append(u.paths, r.URL.Path)
		u.mu.Unlock()
		authorized := r.Header.Get("Authorization") == "Bearer "+u.allowedKey
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			if !authorized {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"vendor-chat"}]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			if !authorized {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *keyCaptureUpstream) calls() (auths, paths []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.auths...), append([]string(nil), u.paths...)
}

type providerTestEnv struct {
	admin  *client
	store  *store.Store
	srvURL string
}

func newProviderTestEnv(t *testing.T) *providerTestEnv {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, err := secret.NewBox(k)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(api.New(st, box, api.Options{Upstream: http.DefaultClient}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "登录", code, 200, body)
	return &providerTestEnv{admin: admin, store: st, srvURL: ts.URL}
}

func (e *providerTestEnv) createProvider(t *testing.T, name, baseURL, key string) int64 {
	t.Helper()
	code, body := e.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": name, "vendor": "custom", "base_url": baseURL, "api_key": key,
	}, nil)
	expect(t, "建上游 "+name, code, 201, body)
	return decode[struct{ ID int64 }](t, body).ID
}

func (e *providerTestEnv) patchProvider(t *testing.T, id int64, body map[string]any) (int, []byte) {
	t.Helper()
	return e.admin.do("PATCH", "/api/admin/providers/"+itoa(id), body, nil)
}

func (e *providerTestEnv) checkProvider(t *testing.T, id int64) (checkResultPayload, []byte) {
	t.Helper()
	code, body := e.admin.do("POST", "/api/admin/providers/"+itoa(id)+"/check", nil, nil)
	expect(t, "检查上游", code, 200, body)
	return decode[checkResultPayload](t, body), body
}

// getProvider 直接读库，用来断言 key 绑定的 origin 等不对外暴露的字段。
func (e *providerTestEnv) getProvider(t *testing.T, id int64) *store.Provider {
	t.Helper()
	p, err := e.store.GetProvider(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// forceProviderBaseURL 绕过接口直接改库，模拟改址守卫上线前已被改过 origin 的历史记录。
func (e *providerTestEnv) forceProviderBaseURL(t *testing.T, id int64, baseURL string) {
	t.Helper()
	if _, err := e.store.DB().ExecContext(context.Background(), "UPDATE providers SET base_url = ? WHERE id = ?", baseURL, id); err != nil {
		t.Fatal(err)
	}
}

func (e *providerTestEnv) clearProviderKeyOrigin(t *testing.T, id int64) {
	t.Helper()
	if _, err := e.store.DB().ExecContext(context.Background(), "UPDATE providers SET api_key_origin = '' WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
}

func (e *providerTestEnv) createModel(t *testing.T, providerID int64, upstreamModel, name string) {
	t.Helper()
	code, body := e.admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": providerID,
		"items":       []map[string]string{{"upstream_model": upstreamModel, "name": name}},
	}, nil)
	expect(t, "建模型 "+name, code, 201, body)
}

// newEmployeeKey 建一个带额度的员工并发一把自助 key，返回明文 key（网关用）。
func (e *providerTestEnv) newEmployeeKey(t *testing.T) string {
	t.Helper()
	code, body := e.admin.do("POST", "/api/admin/users", map[string]string{
		"username": "origin-user", "password": "origin-password-1", "display_name": "残余测试",
	}, nil)
	expect(t, "创建员工", code, 201, body)
	var user struct{ ID int64 }
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatal(err)
	}
	code, body = e.admin.do("PATCH", "/api/admin/users/"+itoa(user.ID)+"/quota", map[string]int{"monthly_token_limit": 10000}, nil)
	expect(t, "分配额度", code, 200, body)
	employee := newClient(t, e.srvURL)
	code, body = employee.do("POST", "/api/auth/login", map[string]string{"username": "origin-user", "password": "origin-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)
	code, body = employee.do("POST", "/api/me/keys", map[string]string{"name": "origin"}, nil)
	expect(t, "自助创建 key", code, 201, body)
	return decode[struct{ Key string }](t, body).Key
}

// gatewayChat 用员工 key 调 /v1/chat/completions，返回状态码与响应体。
func (e *providerTestEnv) gatewayChat(t *testing.T, employeeKey, model string) (int, []byte) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"model": model, "messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srvURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+employeeKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("网关请求失败：%v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

type checkResultPayload struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Models  []string `json:"models"`
}

// 改 base_url 的 origin（协议/域名/端口）却不重填 api_key 时，旧的企业 key 不能被
// 自动带到新地址：修复前这条测试是红的——PATCH 返回 200，随后 check 会把旧 key
// 当成 Bearer 发到新地址。
func TestProviderUpdateOriginChangeRequiresNewKey(t *testing.T) {
	env := newProviderTestEnv(t)
	oldUp := newKeyCaptureUpstream(t, "sk-old")
	newUp := newKeyCaptureUpstream(t, "sk-new") // 新地址（攻击者控制）
	pid := env.createProvider(t, "旧上游", oldUp.srv.URL, "sk-old")

	code, body := env.patchProvider(t, pid, map[string]any{"base_url": newUp.srv.URL})
	if code == http.StatusOK {
		// 旧实现允许改址且保留旧 key。调用一次 check，把「旧 key 被送到新地址」的
		// 证据留下再失败，说明修复前为什么是红的。
		env.admin.do("POST", "/api/admin/providers/"+itoa(pid)+"/check", nil, nil)
		if auths, _ := newUp.calls(); len(auths) > 0 {
			t.Fatalf("漏洞复现：base_url 改到 %s 后，旧 API Key 被发送到新地址（Authorization: %q）；期望 400/409 拒绝", newUp.srv.URL, auths[0])
		}
		t.Fatalf("origin 变化且未提供 api_key 时应拒绝，实际 200：%s", body)
	}
	if code != http.StatusBadRequest && code != http.StatusConflict {
		t.Fatalf("origin 变化且未提供 api_key 时状态码应为 400/409，实际 %d：%s", code, body)
	}
	if !strings.Contains(string(body), "重新填写 API Key") {
		t.Fatalf("拒绝文案应提示重新填写 API Key：%s", body)
	}
	if auths, paths := newUp.calls(); len(paths) > 0 {
		t.Fatalf("被拒绝的改址不应向新地址发任何请求（否则旧 key 已外送）：%v", auths)
	}

	// 被拒绝后旧配置保持不变；检查仍然用旧 key 请求旧地址。
	pv, err := env.store.GetProvider(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if pv.BaseURL != oldUp.srv.URL {
		t.Fatalf("被拒绝的改址不应修改 base_url，实际 %q", pv.BaseURL)
	}
	if pv.APIKeyHint != "…-old" {
		t.Fatalf("被拒绝的改址不应修改 api_key_hint，实际 %q", pv.APIKeyHint)
	}
	if pv.APIKeyOrigin != oldUp.srv.URL {
		t.Fatalf("被拒绝的改址不应修改 api_key_origin，实际 %q", pv.APIKeyOrigin)
	}
	res, _ := env.checkProvider(t, pid)
	if !res.OK {
		t.Fatalf("origin 未变时检查应能沿用旧 key，实际：%+v", res)
	}
	auths, paths := oldUp.calls()
	if len(auths) == 0 || auths[len(auths)-1] != "Bearer sk-old" || paths[len(paths)-1] != "/models" {
		t.Fatalf("检查旧上游应使用旧 key，实际 Authorization=%q path=%q", auths, paths)
	}
	if auths, _ := newUp.calls(); len(auths) > 0 {
		t.Fatalf("新地址不应收到任何旧 key：%v", auths)
	}

	// 协议变化同样算换 origin，必须重填 key。
	code, body = env.patchProvider(t, pid, map[string]any{"base_url": strings.Replace(oldUp.srv.URL, "http://", "https://", 1)})
	if code != http.StatusBadRequest && code != http.StatusConflict {
		t.Fatalf("scheme 变化且未提供 api_key 时应拒绝，实际 %d：%s", code, body)
	}
}

// origin 变化时显式重填 key 是允许的：更新后 check 必须用新 key 请求新地址，
// 旧地址不再收到请求；审计记录 base_url 变化但不得包含 key 明文。
func TestProviderUpdateOriginChangeWithNewKey(t *testing.T) {
	env := newProviderTestEnv(t)
	oldUp := newKeyCaptureUpstream(t, "sk-old")
	newUp := newKeyCaptureUpstream(t, "sk-new")
	pid := env.createProvider(t, "旧上游", oldUp.srv.URL, "sk-old")

	code, body := env.patchProvider(t, pid, map[string]any{"base_url": newUp.srv.URL, "api_key": "sk-new"})
	expect(t, "改址并重填 key", code, 200, body)
	var pv struct {
		BaseURL    string `json:"base_url"`
		APIKeyHint string `json:"api_key_hint"`
	}
	if err := json.Unmarshal(body, &pv); err != nil {
		t.Fatalf("解析 PATCH 响应：%v；%s", err, body)
	}
	if pv.BaseURL != newUp.srv.URL {
		t.Fatalf("base_url 应为 %q，实际 %q", newUp.srv.URL, pv.BaseURL)
	}
	if pv.APIKeyHint != "…-new" {
		t.Fatalf("api_key_hint 应更新为新 key 的末四位，实际 %q", pv.APIKeyHint)
	}
	if stored := env.getProvider(t, pid); stored.APIKeyOrigin != newUp.srv.URL {
		t.Fatalf("重填 key 后 api_key_origin 应更新为新地址的 origin %q，实际 %q", newUp.srv.URL, stored.APIKeyOrigin)
	}
	if strings.Contains(string(body), "api_key_origin") {
		t.Fatalf("api_key_origin 不应回显给前端：%s", body)
	}

	res, _ := env.checkProvider(t, pid)
	if !res.OK {
		t.Fatalf("重填 key 后检查应成功，实际：%+v", res)
	}
	auths, paths := newUp.calls()
	if len(auths) != 1 || auths[0] != "Bearer sk-new" || paths[0] != "/models" {
		t.Fatalf("检查新地址应且只应使用新 key，实际 Authorization=%v path=%v", auths, paths)
	}
	if auths, _ := oldUp.calls(); len(auths) > 0 {
		t.Fatalf("改址后旧地址不应再收到请求：%v", auths)
	}

	logs, err := env.store.ListAudit(context.Background(), store.AuditFilter{Action: "provider.update", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var updateLog *store.AuditLog
	for _, l := range logs {
		if l.Detail["base_url_changed"] == true {
			updateLog = l
			break
		}
	}
	if updateLog == nil {
		t.Fatalf("origin 变化应记一条 provider.update 审计，detail.base_url_changed=true：%+v", logs)
	}
	if updateLog.Detail["key_changed"] != true {
		t.Fatalf("重填 key 应记 key_changed=true：%+v", updateLog.Detail)
	}
	raw, _ := json.Marshal(updateLog.Detail)
	if strings.Contains(string(raw), "sk-new") || strings.Contains(string(raw), "sk-old") {
		t.Fatalf("审计 detail 不得包含 key 明文：%s", raw)
	}
}

// origin 不变、只有路径变化（包括尾部斜杠规范化）时不必重填 key，可以继续沿用旧 key。
func TestProviderUpdatePathChangeKeepsOldKey(t *testing.T) {
	env := newProviderTestEnv(t)
	up := newKeyCaptureUpstream(t, "sk-old")
	pid := env.createProvider(t, "上游", up.srv.URL, "sk-old")

	// 尾部斜杠会被规范掉，等于没改地址。
	code, body := env.patchProvider(t, pid, map[string]any{"base_url": up.srv.URL + "/"})
	expect(t, "尾部斜杠规范化", code, 200, body)
	// 同 origin 换 path：允许沿用旧 key。
	code, body = env.patchProvider(t, pid, map[string]any{"base_url": up.srv.URL + "/v1"})
	expect(t, "同 origin 改 path", code, 200, body)

	pv, err := env.store.GetProvider(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if pv.BaseURL != up.srv.URL+"/v1" {
		t.Fatalf("base_url 应为 %q，实际 %q", up.srv.URL+"/v1", pv.BaseURL)
	}
	if pv.APIKeyHint != "…-old" {
		t.Fatalf("仅路径变化不应改 key，api_key_hint 实际 %q", pv.APIKeyHint)
	}
	if pv.APIKeyOrigin != up.srv.URL {
		t.Fatalf("仅路径变化不应改 api_key_origin，实际 %q", pv.APIKeyOrigin)
	}

	res, _ := env.checkProvider(t, pid)
	if !res.OK {
		t.Fatalf("仅路径变化后检查应沿用旧 key 成功，实际：%+v", res)
	}
	auths, paths := up.calls()
	if len(auths) != 1 || auths[0] != "Bearer sk-old" || paths[0] != "/v1/models" {
		t.Fatalf("检查应使用旧 key 请求新路径，实际 Authorization=%v path=%v", auths, paths)
	}
}

// 历史数据里 base_url 无法解析成 origin 时，检查接口不能猜、更不能解密外送 key，
// 而是提示重新填写（响应结构与普通检查失败一致：HTTP 200 + ok=false）。
func TestProviderCheckRefusesUnparsableStoredBaseURL(t *testing.T) {
	env := newProviderTestEnv(t)
	up := newKeyCaptureUpstream(t, "sk-old")
	pid := env.createProvider(t, "旧上游", up.srv.URL, "sk-old")

	// 模拟守卫上线前的历史/损坏数据：绕过接口直接改库。
	env.forceProviderBaseURL(t, pid, "://broken")
	res, body := env.checkProvider(t, pid)
	if res.OK {
		t.Fatalf("base_url 无法判定时检查不应成功：%s", body)
	}
	if !strings.Contains(res.Message, "重新填写") {
		t.Fatalf("无法判定 origin 时应提示重新填写，实际：%q", res.Message)
	}
	if auths, _ := up.calls(); len(auths) > 0 {
		t.Fatalf("无法判定 origin 时不得解密外送 key：%v", auths)
	}
}

// 新建与重填 key 时记录它绑定的 origin；沿用旧 key 时保留原记录；该列不对外暴露。
func TestProviderKeyOriginRecordedOnSave(t *testing.T) {
	env := newProviderTestEnv(t)
	up := newKeyCaptureUpstream(t, "sk-old")
	pid := env.createProvider(t, "上游", up.srv.URL, "sk-old")

	if pv := env.getProvider(t, pid); pv.APIKeyOrigin != up.srv.URL {
		t.Fatalf("新建上游应记录 key 绑定 origin %q，实际 %q", up.srv.URL, pv.APIKeyOrigin)
	}
	code, body := env.admin.do("GET", "/api/admin/providers", nil, nil)
	expect(t, "读上游列表", code, 200, body)
	if strings.Contains(string(body), "api_key_origin") {
		t.Fatalf("api_key_origin 不应回显给前端：%s", body)
	}

	// 仅 path 变化、沿用旧 key：key 绑定的 origin 不变。
	code, body = env.patchProvider(t, pid, map[string]any{"base_url": up.srv.URL + "/v1"})
	expect(t, "同 origin 改 path", code, 200, body)
	if pv := env.getProvider(t, pid); pv.APIKeyOrigin != up.srv.URL {
		t.Fatalf("沿用旧 key 时 api_key_origin 应保持 %q，实际 %q", up.srv.URL, pv.APIKeyOrigin)
	}

	// 只轮换 key（当前地址 origin 未变）：origin 按当前 base_url 写入。
	code, body = env.patchProvider(t, pid, map[string]any{"api_key": "sk-rotated"})
	expect(t, "轮换 key", code, 200, body)
	if pv := env.getProvider(t, pid); pv.APIKeyOrigin != up.srv.URL {
		t.Fatalf("轮换 key 后 api_key_origin 应为 %q，实际 %q", up.srv.URL, pv.APIKeyOrigin)
	}
}

// 残余风险：改址守卫上线前 key 已被留在旧 origin、base_url 已被改到新 origin 的历史记录。
// 修复前 check 仍会解密旧 key 并发往新地址；修复后必须返回可读错误且一个请求都不发。
func TestProviderCheckRefusesKeyBoundToOtherOrigin(t *testing.T) {
	env := newProviderTestEnv(t)
	oldUp := newKeyCaptureUpstream(t, "sk-old")
	newUp := newKeyCaptureUpstream(t, "sk-new")
	pid := env.createProvider(t, "旧上游", oldUp.srv.URL, "sk-old")

	// 模拟守卫上线前的历史记录：key 绑定在 old origin，base_url 已被改到新 origin。
	env.forceProviderBaseURL(t, pid, newUp.srv.URL)

	res, body := env.checkProvider(t, pid)
	if auths, _ := newUp.calls(); len(auths) > 0 {
		t.Fatalf("残余漏洞复现：api_key_origin=%q 与 base_url origin 不一致，旧 key 仍被发到新地址（Authorization: %q）：%s", oldUp.srv.URL, auths[0], body)
	}
	if res.OK {
		t.Fatalf("key 绑定 origin 与 base_url 不一致时检查不应成功：%s", body)
	}
	if !strings.Contains(res.Message, "上游地址已变更") || !strings.Contains(res.Message, "重新填写") {
		t.Fatalf("应提示上游地址已变更、重新填写 API Key，实际：%q", res.Message)
	}
	if auths, _ := oldUp.calls(); len(auths) > 0 {
		t.Fatalf("key 绑定 origin 不一致时不应访问任何上游：%v", auths)
	}
}

// 同上，但走网关转发路径（员工 key 调 /v1/chat/completions）：修复前旧 key 会被发到
// 新地址；修复后按现有 provider_misconfigured 错误壳拒绝，且不向上游发任何请求。
func TestProviderGatewayRefusesKeyBoundToOtherOrigin(t *testing.T) {
	env := newProviderTestEnv(t)
	oldUp := newKeyCaptureUpstream(t, "sk-old")
	newUp := newKeyCaptureUpstream(t, "sk-new")
	pid := env.createProvider(t, "旧上游", oldUp.srv.URL, "sk-old")
	env.createModel(t, pid, "vendor-chat", "chat-residual")
	employeeKey := env.newEmployeeKey(t)

	env.forceProviderBaseURL(t, pid, newUp.srv.URL)

	code, body := env.gatewayChat(t, employeeKey, "chat-residual")
	if auths, _ := newUp.calls(); len(auths) > 0 {
		t.Fatalf("残余漏洞复现：网关把绑定在 %q 的 key 发到了新地址（Authorization: %q），HTTP %d：%s", oldUp.srv.URL, auths[0], code, body)
	}
	if code != http.StatusInternalServerError {
		t.Fatalf("key 绑定 origin 不一致时网关应返回 provider_misconfigured 错误壳（HTTP 500），实际 %d：%s", code, body)
	}
	if !strings.Contains(string(body), "provider_misconfigured") {
		t.Fatalf("网关错误应为 provider_misconfigured：%s", body)
	}
	if auths, _ := oldUp.calls(); len(auths) > 0 {
		t.Fatalf("key 绑定 origin 不一致时不应访问任何上游：%v", auths)
	}
}

// 加列前的旧记录（api_key_origin 为空）读侧不阻断；管理员重填 key 的保存会补写 origin，
// 之后这类记录再被改址就能被识别。普通保存（不带新 key）不得凭空补写。
func TestProviderLegacyEmptyKeyOriginBackfilledOnSave(t *testing.T) {
	env := newProviderTestEnv(t)
	up := newKeyCaptureUpstream(t, "sk-old")
	newUp := newKeyCaptureUpstream(t, "sk-new")
	pid := env.createProvider(t, "旧记录", up.srv.URL, "sk-old")

	// 模拟加列前的旧记录：origin 未知。
	env.clearProviderKeyOrigin(t, pid)

	// 读侧不阻断：正常检查仍可用旧 key。
	res, _ := env.checkProvider(t, pid)
	if !res.OK {
		t.Fatalf("origin 未知的旧记录不应阻断检查，实际：%+v", res)
	}
	auths, _ := up.calls()
	if len(auths) != 1 || auths[0] != "Bearer sk-old" {
		t.Fatalf("旧记录检查应使用原 key，实际 Authorization=%v", auths)
	}

	// 不带新 key 的普通保存不会补写 origin（否则等于把无法判定的记录当可信）。
	code, body := env.patchProvider(t, pid, map[string]any{"priority": 120})
	expect(t, "普通保存", code, 200, body)
	if pv := env.getProvider(t, pid); pv.APIKeyOrigin != "" {
		t.Fatalf("没有重填 key 时不应补写 api_key_origin，实际 %q", pv.APIKeyOrigin)
	}

	// 重填 key 的保存补写 origin，之后改 origin 即可被识别。
	code, body = env.patchProvider(t, pid, map[string]any{"api_key": "sk-old"})
	expect(t, "重填 key", code, 200, body)
	if pv := env.getProvider(t, pid); pv.APIKeyOrigin != up.srv.URL {
		t.Fatalf("重填 key 后应补写 api_key_origin=%q，实际 %q", up.srv.URL, pv.APIKeyOrigin)
	}
	env.forceProviderBaseURL(t, pid, newUp.srv.URL)
	res, body = env.checkProvider(t, pid)
	if auths, _ := newUp.calls(); len(auths) > 0 {
		t.Fatalf("补写 origin 后历史改址应被识别，但仍把 key 发到新地址：%v（%s）", auths, body)
	}
	if res.OK || !strings.Contains(res.Message, "上游地址已变更") {
		t.Fatalf("补写 origin 后历史改址应被拒绝，实际：%+v", res)
	}
}
