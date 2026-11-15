package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 思考模式端到端：管理端能力声明 → /v1/models 公布 → 网关原样透传请求与 reasoning_content。
// 全部走 HTTP，上游是 httptest 假服务；用例不依赖任何具体模型名（用 company-think / vendor-think 这类测试名）。

const reasoningUpstreamKey = "sk-reasoning-upstream"

// reasoningUpstream 是 OpenAI 兼容的假上游：记录请求体，并按 mode 返回非流式 / 流式 / 错误响应。
type reasoningUpstream struct {
	mu     sync.Mutex
	bodies []map[string]any
	raw    [][]byte
	mode   string // json | stream | error
	srv    *httptest.Server
}

func newReasoningUpstream(t *testing.T) *reasoningUpstream {
	t.Helper()
	u := &reasoningUpstream{mode: "json"}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+reasoningUpstreamKey {
			t.Errorf("上游收到的 Authorization 不对：%q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		u.mu.Lock()
		u.bodies = append(u.bodies, body)
		u.raw = append(u.raw, raw)
		mode := u.mode
		u.mu.Unlock()

		switch mode {
		case "stream":
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range []string{
				`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"先想"}}]}`,
				`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"一下"}}]}`,
				`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"答案"}}]}`,
				`{"id":"chunk-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`,
				`[DONE]`,
			} {
				_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
				w.(http.Flusher).Flush()
			}
		case "error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, reasoningUpstreamErrorBody)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, reasoningUpstreamJSONBody)
		}
	}))
	return u
}

const reasoningUpstreamJSONBody = `{"id":"reasoning-1","object":"chat.completion","created":1700000000,"model":"vendor-think","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"先想一想","content":"答案"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`
const reasoningUpstreamErrorBody = `{"error":{"message":"The reasoning_content in the thinking mode must be passed back to the API.","type":"invalid_request_error","code":"invalid_reasoning_content"}}`

// reasoningStreamBody 是 stream 模式下上游发出的完整 SSE 文本，网关应逐字节转发。
func reasoningStreamBody() string {
	chunks := []string{
		`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"先想"}}]}`,
		`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"一下"}}]}`,
		`{"id":"chunk-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"答案"}}]}`,
		`{"id":"chunk-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`,
		`[DONE]`,
	}
	out := ""
	for _, chunk := range chunks {
		out += "data: " + chunk + "\n\n"
	}
	return out
}

func (u *reasoningUpstream) setMode(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.mode = mode
}

// last 返回最近一次上游收到的请求体（解码后）与原始字节。
func (u *reasoningUpstream) last(t *testing.T) (map[string]any, []byte) {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.bodies) == 0 {
		t.Fatal("上游没有收到任何请求")
	}
	return u.bodies[len(u.bodies)-1], u.raw[len(u.raw)-1]
}

// jsonValue 把测试里构造的值过一遍 JSON，得到与上游解码一致的形状（[]any/float64），便于逐字段比对。
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

type reasoningFixture struct {
	st       *store.Store
	ts       *httptest.Server
	admin    *client
	up       *reasoningUpstream
	token    string
	thinkID  int64
	chatID   int64
	provider int64
}

// newReasoningFixture 建一个带思考能力声明的上游、两个对外模型（一个声明支持思考、一个不支持）、
// 一个有额度且带 models scope 的接入令牌。
func newReasoningFixture(t *testing.T) *reasoningFixture {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	up := newReasoningUpstream(t)
	t.Cleanup(up.srv.Close)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), Upstream: up.srv.Client()},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "reasoning-upstream", "vendor": "custom", "base_url": up.srv.URL, "api_key": reasoningUpstreamKey,
	}, nil)
	expect(t, "创建上游", code, 201, body)
	providerID := decode[struct{ ID int64 }](t, body).ID

	code, body = admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": providerID,
		"items": []map[string]any{
			{"upstream_model": "vendor-think", "name": "company-think", "supports_reasoning": true, "reasoning_efforts": []string{"low", "high", "max"}},
			{"upstream_model": "vendor-chat", "name": "company-chat"},
		},
	}, nil)
	expect(t, "创建对外模型", code, 201, body)
	created := decode[struct {
		Created []struct{ ID int64 } `json:"created"`
	}](t, body)
	if len(created.Created) != 2 {
		t.Fatalf("应创建 2 个模型：%s", body)
	}

	code, body = admin.do("POST", "/api/admin/users", map[string]any{
		"username": "reasoning-user", "password": "reasoning-password-1", "department": "研发效能部",
	}, nil)
	expect(t, "创建员工", code, 201, body)
	user := decode[store.User](t, body)
	limit := int64(1_000_000)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(user.ID)+"/quota", map[string]any{"monthly_token_limit": limit}, nil)
	expect(t, "分配额度", code, 200, body)

	token := &store.AccessToken{
		UserID: user.ID, Name: "ee_at_reasoning", Prefix: "ee_at_reasoning", Source: "manual",
		Scopes: []string{rbac.ScopeModels}, ClientName: "test-client", CreatedBy: user.ID,
	}
	if err := st.CreateAccessToken(context.Background(), token, secret.Hash("ee_at_reasoning")); err != nil {
		t.Fatal(err)
	}
	return &reasoningFixture{
		st: st, ts: ts, admin: admin, up: up, token: "ee_at_reasoning",
		thinkID: created.Created[0].ID, chatID: created.Created[1].ID, provider: providerID,
	}
}

// TestModelReasoningCapabilities 覆盖能力声明的管理端读写与 /v1/models 的 additive 公布。
func TestModelReasoningCapabilities(t *testing.T) {
	f := newReasoningFixture(t)
	c := newClient(t, f.ts.URL)

	// 管理端列表带两个新字段（additive：原字段不变）。
	code, body := f.admin.do("GET", "/api/admin/models", nil, nil)
	expect(t, "管理端模型列表", code, 200, body)
	models := decode[struct {
		Models []struct {
			ID                int64    `json:"id"`
			Name              string   `json:"name"`
			Enabled           bool     `json:"enabled"`
			SupportsReasoning bool     `json:"supports_reasoning"`
			ReasoningEfforts  []string `json:"reasoning_efforts"`
		} `json:"models"`
	}](t, body)
	byID := map[int64]string{}
	for _, m := range models.Models {
		byID[m.ID] = m.Name
		if m.Name == "company-think" {
			if !m.SupportsReasoning || !reflect.DeepEqual(m.ReasoningEfforts, []string{"low", "high", "max"}) {
				t.Fatalf("company-think 的能力字段不对：%+v", m)
			}
			if !m.Enabled {
				t.Fatalf("新建模型默认应启用：%+v", m)
			}
		}
		if m.Name == "company-chat" {
			if m.SupportsReasoning || len(m.ReasoningEfforts) != 0 {
				t.Fatalf("company-chat 默认不应声明思考能力：%+v", m)
			}
			if m.ReasoningEfforts == nil {
				t.Fatalf("思考强度应返回 [] 而不是 null：%s", body)
			}
		}
	}
	if byID[f.thinkID] != "company-think" {
		t.Fatalf("模型 ID 顺序与创建响应不一致：%v", byID)
	}

	// PATCH 规范化：去空白、转小写、去重；更新后列表读到规范化结果。
	code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(f.thinkID), map[string]any{
		"reasoning_efforts": []string{" HIGH ", "low", "HIGH", "max "},
	}, nil)
	expect(t, "更新思考强度", code, 200, body)
	patched := decode[struct {
		SupportsReasoning bool     `json:"supports_reasoning"`
		ReasoningEfforts  []string `json:"reasoning_efforts"`
	}](t, body)
	if !patched.SupportsReasoning || !reflect.DeepEqual(patched.ReasoningEfforts, []string{"high", "low", "max"}) {
		t.Fatalf("PATCH 后能力字段不对：%+v", patched)
	}

	// 非法值 400，且不写入。
	code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(f.thinkID), map[string]any{
		"reasoning_efforts": []string{"has space"},
	}, nil)
	expect(t, "非法思考强度", code, 400, body)

	// /v1/models：原字段（id/object/owned_by）保持不变，新增 capabilities.reasoning。
	code, body = c.do("GET", "/v1/models", nil, bearer(f.token))
	expect(t, "网关模型列表", code, 200, body)
	list := decode[struct {
		Object string `json:"object"`
		Data   []struct {
			ID           string `json:"id"`
			Object       string `json:"object"`
			OwnedBy      string `json:"owned_by"`
			Capabilities struct {
				Reasoning struct {
					Supported bool     `json:"supported"`
					Efforts   []string `json:"efforts"`
				} `json:"reasoning"`
			} `json:"capabilities"`
		} `json:"data"`
	}](t, body)
	if list.Object != "list" {
		t.Fatalf("模型列表外壳不对：%s", body)
	}
	caps := map[string]struct {
		supported bool
		efforts   []string
	}{}
	for _, m := range list.Data {
		if m.Object != "model" || m.OwnedBy == "" || m.ID == "" {
			t.Fatalf("原有模型字段被破坏：%+v", m)
		}
		caps[m.ID] = struct {
			supported bool
			efforts   []string
		}{m.Capabilities.Reasoning.Supported, m.Capabilities.Reasoning.Efforts}
	}
	if got := caps["company-think"]; !got.supported || !reflect.DeepEqual(got.efforts, []string{"high", "low", "max"}) {
		t.Fatalf("company-think 的 capabilities.reasoning 不对：%+v", got)
	}
	if got := caps["company-chat"]; got.supported || len(got.efforts) != 0 || got.efforts == nil {
		t.Fatalf("company-chat 的 capabilities.reasoning 应为 supported=false、efforts=[]：%+v", got)
	}

	// 关掉能力声明后，/v1/models 不再公布 effort（网关行为不受影响，见透传用例）。
	code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(f.thinkID), map[string]any{
		"supports_reasoning": false,
	}, nil)
	expect(t, "关闭思考能力声明", code, 200, body)
	code, body = c.do("GET", "/v1/models", nil, bearer(f.token))
	expect(t, "关闭后模型列表", code, 200, body)
	after := decode[struct {
		Data []struct {
			ID           string `json:"id"`
			Capabilities struct {
				Reasoning struct {
					Supported bool     `json:"supported"`
					Efforts   []string `json:"efforts"`
				} `json:"reasoning"`
			} `json:"capabilities"`
		} `json:"data"`
	}](t, body)
	for _, m := range after.Data {
		if m.ID == "company-think" && (m.Capabilities.Reasoning.Supported || len(m.Capabilities.Reasoning.Efforts) != 0) {
			t.Fatalf("关闭能力声明后不应再公布 supported/efforts：%+v", m)
		}
	}
}

// TestGatewayThinkingPassthrough 覆盖非流式请求/响应与上游错误原样透传。
func TestGatewayThinkingPassthrough(t *testing.T) {
	f := newReasoningFixture(t)
	c := newClient(t, f.ts.URL)

	messages := []map[string]any{
		{"role": "user", "content": "查一下明天的天气"},
		{"role": "assistant", "content": "", "reasoning_content": "先查日期", "tool_calls": []map[string]any{
			{"id": "call_1", "type": "function", "function": map[string]any{"name": "get_date", "arguments": "{}"}},
		}},
		{"role": "tool", "tool_call_id": "call_1", "content": "2026-11-14"},
	}
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "get_weather", "parameters": map[string]any{"type": "object"}}},
	}
	thinking := map[string]any{"type": "enabled", "budget_tokens": 4096}
	req := map[string]any{
		"model":            "company-think",
		"messages":         messages,
		"tools":            tools,
		"thinking":         thinking,
		"reasoning_effort": "ultra", // 网关不做强度白名单/映射，原样转发
		"temperature":      0.2,
	}
	code, body := c.do("POST", "/v1/chat/completions", req, bearer(f.token))
	expect(t, "思考模式非流式请求", code, 200, body)
	if string(body) != reasoningUpstreamJSONBody {
		t.Fatalf("非流式响应应原样返回（含 reasoning_content）：\n得到 %s\n期望 %s", body, reasoningUpstreamJSONBody)
	}
	sent, raw := f.up.last(t)
	if sent["model"] != "vendor-think" {
		t.Fatalf("模型名应替换成上游名：%v", sent["model"])
	}
	for _, key := range []string{"thinking", "reasoning_effort", "tools", "temperature"} {
		if !reflect.DeepEqual(sent[key], jsonValue(t, req[key])) {
			t.Fatalf("%s 应原样转发：得到 %v 期望 %v；body=%s", key, sent[key], jsonValue(t, req[key]), raw)
		}
	}
	if !reflect.DeepEqual(sent["messages"], jsonValue(t, messages)) {
		t.Fatalf("历史消息（含 reasoning_content）应原样转发：%s", raw)
	}
	// 请求体里必须有历史 assistant 轮的 reasoning_content；否则上游会 400。
	gotMessages, _ := sent["messages"].([]any)
	if len(gotMessages) != 3 {
		t.Fatalf("消息条数不对：%s", raw)
	}
	assistant, _ := gotMessages[1].(map[string]any)
	if assistant["reasoning_content"] != "先查日期" {
		t.Fatalf("工具轮次的历史 reasoning_content 丢失：%v", assistant)
	}

	// 未声明思考能力的模型也照常透传参数：能力声明只用于公布，不做请求校验。
	chatReq := map[string]any{
		"model":            "company-chat",
		"messages":         []map[string]any{{"role": "user", "content": "hi"}},
		"thinking":         thinking,
		"reasoning_effort": "high",
	}
	code, body = c.do("POST", "/v1/chat/completions", chatReq, bearer(f.token))
	expect(t, "未声明能力的模型透传", code, 200, body)
	sent, _ = f.up.last(t)
	if !reflect.DeepEqual(sent["thinking"], jsonValue(t, thinking)) || sent["reasoning_effort"] != "high" || sent["model"] != "vendor-chat" {
		t.Fatalf("未声明思考能力的模型也应原样透传参数：%v", sent)
	}

	// 上游 400 原样返回：状态码与错误体都不改写。
	f.up.setMode("error")
	code, body = c.do("POST", "/v1/chat/completions", map[string]any{
		"model": "company-think", "messages": messages, "tools": tools,
		"thinking": map[string]any{"type": "enabled"}, "reasoning_effort": "high",
	}, bearer(f.token))
	expect(t, "上游 400", code, 400, body)
	if string(body) != reasoningUpstreamErrorBody {
		t.Fatalf("上游错误体应原样返回：\n得到 %s\n期望 %s", body, reasoningUpstreamErrorBody)
	}
}

// TestGatewayThinkingStream 覆盖流式 delta.reasoning_content 的逐字节转发与记账。
func TestGatewayThinkingStream(t *testing.T) {
	f := newReasoningFixture(t)
	c := newClient(t, f.ts.URL)
	f.up.setMode("stream")

	code, body := c.do("POST", "/v1/chat/completions", map[string]any{
		"model":            "company-think",
		"messages":         []map[string]any{{"role": "user", "content": "查天气"}},
		"stream":           true,
		"thinking":         map[string]any{"type": "enabled"},
		"reasoning_effort": "medium",
	}, bearer(f.token))
	expect(t, "思考模式流式请求", code, 200, body)
	if want := reasoningStreamBody(); string(body) != want {
		t.Fatalf("流式响应应逐字节转发（含 reasoning_content 增量）：\n得到 %q\n期望 %q", body, want)
	}
	sent, _ := f.up.last(t)
	if sent["stream"] != true {
		t.Fatalf("上游应收到 stream=true：%v", sent)
	}
	opts, _ := sent["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Fatalf("流式请求应补 stream_options.include_usage：%v", sent["stream_options"])
	}
	if !reflect.DeepEqual(sent["thinking"], map[string]any{"type": "enabled"}) || sent["reasoning_effort"] != "medium" {
		t.Fatalf("流式请求的思考参数应原样转发：%v", sent)
	}
}
