package api_test

// OpenAI Responses API 兼容入口（POST /v1/responses）的集成测试。
//
// 覆盖：非流式文本、instructions/input 转换、tools/function_call、thinking
// （reasoning_content）、text.format 与 max_output_tokens 转发、流式事件序列、
// 鉴权/模型权限/额度/上游错误映射，以及用量记账是否与 chat 网关一致。
//
// 与其它 api_test 一样需要 DSH_EE_TEST_DSN（真实 MySQL 协议库，例如 TiDB）；
// 上游用 httptest 假服务，不依赖公网。

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
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// ---- 测试夹具 ----

// responsesFixture 起一个 dsh-ee 实例 + 一个可编程的假 OpenAI chat completions 上游。
// 上游记录每次收到的请求体，方便断言 Responses → chat 的转换结果。
type responsesFixture struct {
	st          *store.Store
	ts          *httptest.Server
	admin       *client
	upstreamURL string

	mu       sync.Mutex
	handler  func(body map[string]any) (status int, payload string)
	requests []map[string]any
	// nativeHandler 模拟 DeepSeek 的 Responses 上游（POST /responses）；记录原始字节
	// 与 Authorization，用于断言请求语义透传、响应逐字节转发。
	nativeHandler  func(raw []byte) (status int, contentType, payload string)
	nativeRequests [][]byte
	nativeAuth     []string
	nativeAccept   []string
}

func newResponsesFixture(t *testing.T) *responsesFixture {
	t.Helper()
	return newResponsesFixtureWith(t, nil)
}

// newResponsesFixtureWith 允许测试调整 api.Options（例如打开
// DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE 对应的文件引用开关）。
func newResponsesFixtureWith(t *testing.T, mutate func(*api.Options)) *responsesFixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	f := &responsesFixture{st: st}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/chat/completions":
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			f.mu.Lock()
			f.requests = append(f.requests, body)
			handler := f.handler
			f.mu.Unlock()
			status, payload := http.StatusOK, responsesChatCompletionJSON(
				map[string]any{"role": "assistant", "content": "你好"},
				"stop",
				map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
			)
			if handler != nil {
				status, payload = handler(body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, payload)
		case "/responses":
			f.mu.Lock()
			f.nativeRequests = append(f.nativeRequests, raw)
			f.nativeAuth = append(f.nativeAuth, r.Header.Get("Authorization"))
			f.nativeAccept = append(f.nativeAccept, r.Header.Get("Accept"))
			nativeHandler := f.nativeHandler
			f.mu.Unlock()
			if nativeHandler == nil {
				http.NotFound(w, r)
				return
			}
			status, contentType, payload := nativeHandler(raw)
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(status)
			_, _ = io.WriteString(w, payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	opts := api.Options{DataDir: t.TempDir(), Upstream: upstream.Client()}
	if mutate != nil {
		mutate(&opts)
	}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	f.ts = ts
	f.upstreamURL = upstream.URL
	f.admin = newClient(t, ts.URL)

	code, body := f.admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)
	code, body = f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "responses-upstream", "base_url": upstream.URL, "api_key": upstreamSecret,
	}, nil)
	expect(t, "创建上游", code, http.StatusCreated, body)
	provider := decode[struct {
		ID int64 `json:"id"`
	}](t, body)
	code, body = f.admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": provider.ID,
		"items":       []map[string]any{{"upstream_model": "upstream-chat", "name": "chat"}},
	}, nil)
	expect(t, "创建模型", code, http.StatusCreated, body)
	f.provider(t, "responses-native", "deepseek", upstream.URL, 100, "upstream-native", "native-chat")
	return f
}

// provider 创建一个上游并挂一个对外模型；vendor 决定 Responses 走原生（deepseek）
// 还是 chat 转换层（其它），priority 越小越先用。
func (f *responsesFixture) provider(t *testing.T, name, vendor, baseURL string, priority int, upstreamModel, modelName string) int64 {
	t.Helper()
	code, body := f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": name, "vendor": vendor, "base_url": baseURL, "api_key": upstreamSecret, "priority": priority,
	}, nil)
	expect(t, "创建上游 "+name, code, http.StatusCreated, body)
	provider := decode[struct {
		ID int64 `json:"id"`
	}](t, body)
	code, body = f.admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": provider.ID,
		"items":       []map[string]any{{"upstream_model": upstreamModel, "name": modelName}},
	}, nil)
	expect(t, "创建模型 "+modelName, code, http.StatusCreated, body)
	return provider.ID
}

func (f *responsesFixture) setNativeHandler(handler func(raw []byte) (int, string, string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nativeHandler = handler
}

// lastNativeRequest 返回原生 Responses 上游最近一次收到的原始请求体。
func (f *responsesFixture) lastNativeRequest(t *testing.T) []byte {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.nativeRequests) == 0 {
		t.Fatal("原生 Responses 上游没有收到请求")
	}
	return f.nativeRequests[len(f.nativeRequests)-1]
}

// nativeRequestCount 返回原生 Responses 上游收到的请求数（断言额度过期/权限拒绝时
// 不会打上游）。
func (f *responsesFixture) nativeRequestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.nativeRequests)
}

// chatRequestCount 返回 chat completions 转换上游收到的请求数。
func (f *responsesFixture) chatRequestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// lastNativeAuth 返回原生 Responses 上游最近一次收到的 Authorization 头。
func (f *responsesFixture) lastNativeAuth(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.nativeAuth) == 0 {
		t.Fatal("原生 Responses 上游没有收到请求")
	}
	return f.nativeAuth[len(f.nativeAuth)-1]
}

// lastNativeAccept 返回原生 Responses 上游最近一次收到的 Accept 头。
func (f *responsesFixture) lastNativeAccept(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.nativeAccept) == 0 {
		t.Fatal("原生 Responses 上游没有收到请求")
	}
	return f.nativeAccept[len(f.nativeAccept)-1]
}

func (f *responsesFixture) setHandler(handler func(body map[string]any) (int, string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

// upstreamRequest 返回上游最近一次收到的请求体。
func (f *responsesFixture) upstreamRequest(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("上游没有收到任何请求")
	}
	return f.requests[len(f.requests)-1]
}

func (f *responsesFixture) user(t *testing.T, username string, monthlyLimit *int64) store.User {
	t.Helper()
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "password": "employee-password-1", "department": "研发效能部",
	}, nil)
	expect(t, "创建员工 "+username, code, http.StatusCreated, body)
	u := decode[store.User](t, body)
	// 新用户默认额度 0（不能调模型）；显式 PATCH null 才是不限额，和 principal_test 一致。
	var limit any
	if monthlyLimit != nil {
		limit = *monthlyLimit
	}
	code, body = f.admin.do("PATCH", "/api/admin/users/"+itoa(u.ID)+"/quota", map[string]any{"monthly_token_limit": limit}, nil)
	expect(t, "设置账号额度", code, http.StatusOK, body)
	return u
}

func (f *responsesFixture) token(t *testing.T, userID int64, plain string, scopes, models []string, limit *int64) {
	t.Helper()
	token := &store.AccessToken{
		UserID: userID, Name: plain, Prefix: plain, Source: "manual", Scopes: scopes,
		MonthlyTokenLimit: limit, AllowedModels: models, ClientName: "test-client", CreatedBy: userID,
	}
	if err := f.st.CreateAccessToken(context.Background(), token, secret.Hash(plain)); err != nil {
		t.Fatal(err)
	}
}

// post 发一次 /v1/responses 请求；body 传 string 时按原样发送（用于测试坏 JSON）。
func (f *responsesFixture) post(t *testing.T, token string, body any) (int, []byte, http.Header) {
	t.Helper()
	return f.postPath(t, "/v1/responses", token, body)
}

// postPath 向指定 Responses 路径发请求：/v1/responses 主入口或 /responses 别名。
func (f *responsesFixture) postPath(t *testing.T, path, token string, body any) (int, []byte, http.Header) {
	t.Helper()
	var reader io.Reader
	if raw, ok := body.(string); ok {
		reader = strings.NewReader(raw)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, f.ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/responses: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, resp.Header
}

// ---- 小工具 ----

func responsesChatCompletionJSON(message map[string]any, finishReason string, usage map[string]any) string {
	payload := map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}},
	}
	if usage != nil {
		payload["usage"] = usage
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func responsesJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("响应不是合法 JSON：%v；%s", err, body)
	}
	return m
}

func responsesObject(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	obj, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("字段 %s 不是对象：%v；%v", key, m[key], m)
	}
	return obj
}

func responsesList(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	list, ok := m[key].([]any)
	if !ok {
		t.Fatalf("字段 %s 不是数组：%v；%v", key, m[key], m)
	}
	return list
}

func responsesString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, ok := m[key].(string)
	if !ok {
		t.Fatalf("字段 %s 不是字符串：%v；%v", key, m[key], m)
	}
	return s
}

func responsesNumber(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	n, ok := m[key].(float64)
	if !ok {
		t.Fatalf("字段 %s 不是数字：%v；%v", key, m[key], m)
	}
	return n
}

func responsesErrorObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	m := responsesJSON(t, body)
	return responsesObject(t, m, "error")
}

// responsesForwardedMessages 取上游收到的 messages（JSON 解码后是 []any）。
func responsesForwardedMessages(t *testing.T, req map[string]any) []map[string]any {
	t.Helper()
	raw, ok := req["messages"].([]any)
	if !ok {
		t.Fatalf("转发给上游的请求缺少 messages：%v", req)
	}
	out := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		msg, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("messages[%d] 不是对象：%v", i, item)
		}
		out = append(out, msg)
	}
	return out
}

// ---- 非流式：文本 ----

func TestResponsesNonStreamText(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-text", nil)
	f.token(t, u.ID, "ee_at_responses_text", []string{rbac.ScopeModels}, nil, nil)

	// 1) 字符串 input：转成一条 user 消息，model 换成上游模型名。
	code, body, _ := f.post(t, "ee_at_responses_text", map[string]any{
		"model": "chat", "input": "你好",
	})
	expect(t, "非流式文本", code, http.StatusOK, body)
	resp := responsesJSON(t, body)
	if got := responsesString(t, resp, "id"); !strings.HasPrefix(got, "resp_") {
		t.Fatalf("response id 不对：%q", got)
	}
	if got := responsesString(t, resp, "object"); got != "response" {
		t.Fatalf("object = %q，期望 response", got)
	}
	if got := responsesString(t, resp, "status"); got != "completed" {
		t.Fatalf("status = %q，期望 completed", got)
	}
	if got := responsesString(t, resp, "model"); got != "chat" {
		t.Fatalf("model = %q，期望对外模型名 chat", got)
	}
	output := responsesList(t, resp, "output")
	if len(output) != 1 {
		t.Fatalf("output 长度 = %d，期望 1：%v", len(output), output)
	}
	message := output[0].(map[string]any)
	if got := responsesString(t, message, "type"); got != "message" {
		t.Fatalf("output[0].type = %q，期望 message", got)
	}
	if got := responsesString(t, message, "role"); got != "assistant" {
		t.Fatalf("output[0].role = %q，期望 assistant", got)
	}
	content := responsesList(t, message, "content")
	if len(content) != 1 {
		t.Fatalf("message.content 长度 = %d：%v", len(content), content)
	}
	part := content[0].(map[string]any)
	if got := responsesString(t, part, "type"); got != "output_text" {
		t.Fatalf("content[0].type = %q，期望 output_text", got)
	}
	if got := responsesString(t, part, "text"); got != "你好" {
		t.Fatalf("content[0].text = %q，期望 你好", got)
	}
	usage := responsesObject(t, resp, "usage")
	if got := responsesNumber(t, usage, "input_tokens"); got != 10 {
		t.Fatalf("usage.input_tokens = %v，期望 10", got)
	}
	if got := responsesNumber(t, usage, "output_tokens"); got != 5 {
		t.Fatalf("usage.output_tokens = %v，期望 5", got)
	}
	if got := responsesNumber(t, usage, "total_tokens"); got != 15 {
		t.Fatalf("usage.total_tokens = %v，期望 15", got)
	}

	forwarded := f.upstreamRequest(t)
	if got := responsesString(t, forwarded, "model"); got != "upstream-chat" {
		t.Fatalf("上游收到的 model = %q，期望 upstream-chat", got)
	}
	if _, ok := forwarded["stream"]; ok {
		t.Fatalf("v0 转发上游不应带 stream：%v", forwarded)
	}
	messages := responsesForwardedMessages(t, forwarded)
	if len(messages) != 1 || responsesString(t, messages[0], "role") != "user" || responsesString(t, messages[0], "content") != "你好" {
		t.Fatalf("字符串 input 转换不对：%v", messages)
	}

	// 用量与 chat 网关记在同一张表：15 token。
	used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 15 {
		t.Fatalf("月度用量 = %d，期望 15", used)
	}

	// 1b) /responses 别名（DeepSeek Responses API 的 base_url 习惯）：与 /v1/responses
	// 同一个 handler，鉴权/转换/用量记账一致，别名响应也带协议版本头。
	code, body, header := f.postPath(t, "/responses", "ee_at_responses_text", map[string]any{
		"model": "chat", "input": "别名",
	})
	expect(t, "Responses 别名", code, http.StatusOK, body)
	if got := header.Get("Dsh-EE-API-Version"); got != "1" {
		t.Fatalf("/responses 别名的 Dsh-EE-API-Version = %q，期望 1", got)
	}
	aliasResp := responsesJSON(t, body)
	if got := responsesString(t, aliasResp, "object"); got != "response" {
		t.Fatalf("别名的 object = %q，期望 response", got)
	}
	if got := responsesString(t, aliasResp, "status"); got != "completed" {
		t.Fatalf("别名的 status = %q，期望 completed", got)
	}
	aliasOutput := responsesList(t, aliasResp, "output")
	aliasPart := responsesList(t, aliasOutput[0].(map[string]any), "content")[0].(map[string]any)
	if got := responsesString(t, aliasPart, "text"); got != "你好" {
		t.Fatalf("别名的输出文本 = %q", got)
	}
	aliasMessages := responsesForwardedMessages(t, f.upstreamRequest(t))
	if got := responsesString(t, aliasMessages[0], "content"); got != "别名" {
		t.Fatalf("别名请求转发给上游的 input = %q", got)
	}
	// 别名与主入口共用鉴权：没有 Bearer 也是 401 + Responses 错误壳。
	code, body, _ = f.postPath(t, "/responses", "", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "Responses 别名鉴权", code, http.StatusUnauthorized, body)
	if got := responsesString(t, responsesErrorObject(t, body), "type"); got != "authentication_error" {
		t.Fatalf("别名 401 的 error.type = %q", got)
	}

	// 2) instructions + 数组 input（多段文本）：instructions 变第一条 system，
	// developer 按 OpenAI 语义降级为 system，文本内容块拼起来。
	code, body, _ = f.post(t, "ee_at_responses_text", map[string]any{
		"model":        "chat",
		"instructions": "你是助手",
		"input": []any{
			map[string]any{"type": "message", "role": "developer", "content": []any{
				map[string]any{"type": "input_text", "text": "规则 A"},
			}},
			map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "问题"},
				map[string]any{"type": "input_text", "text": "补充"},
			}},
		},
	})
	expect(t, "instructions + 数组 input", code, http.StatusOK, body)
	messages = responsesForwardedMessages(t, f.upstreamRequest(t))
	if len(messages) != 3 {
		t.Fatalf("转换后 messages 长度 = %d，期望 3：%v", len(messages), messages)
	}
	if got := responsesString(t, messages[0], "role"); got != "system" {
		t.Fatalf("instructions 没有变成第一条 system：%v", messages)
	}
	if got := responsesString(t, messages[0], "content"); got != "你是助手" {
		t.Fatalf("instructions 内容不对：%q", got)
	}
	if got := responsesString(t, messages[1], "role"); got != "system" {
		t.Fatalf("developer 消息应降级为 system：%v", messages)
	}
	if got := responsesString(t, messages[2], "content"); got != "问题补充" {
		t.Fatalf("多段文本拼接不对：%q", got)
	}

	// 3) 请求里带 v0 不支持的顶层参数（previous_response_id / store / metadata）时
	// 按无状态语义忽略，请求仍然成功。
	code, body, _ = f.post(t, "ee_at_responses_text", map[string]any{
		"model": "chat", "input": "hi", "previous_response_id": "resp_prev", "store": true, "metadata": map[string]any{"k": "v"},
	})
	expect(t, "忽略无状态参数", code, http.StatusOK, body)
	resp = responsesJSON(t, body)
	if resp["previous_response_id"] != nil {
		t.Fatalf("previous_response_id 应固定 null：%v", resp["previous_response_id"])
	}
	if got, ok := resp["store"].(bool); !ok || got {
		t.Fatalf("store 应固定 false：%v", resp["store"])
	}
}

// ---- 非流式：tools / function_call ----

func TestResponsesToolCall(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-tools", nil)
	f.token(t, u.ID, "ee_at_responses_tools", []string{rbac.ScopeModels}, nil, nil)

	const toolCallJSON = `{
		"choices": [{
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 12, "completion_tokens": 3, "total_tokens": 15}
	}`
	f.setHandler(func(map[string]any) (int, string) { return http.StatusOK, toolCallJSON })

	code, body, _ := f.post(t, "ee_at_responses_tools", map[string]any{
		"model": "chat",
		"tools": []any{map[string]any{
			"type":        "function",
			"name":        "get_weather",
			"description": "查天气",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
			"strict":      true,
		}},
		"tool_choice": "auto",
		"input": []any{
			map[string]any{"type": "message", "role": "developer", "content": "规则"},
			map[string]any{"type": "message", "role": "user", "content": "北京天气？"},
			// 上一轮的工具调用历史：function_call 应并进 assistant，output 变 tool 消息。
			map[string]any{"type": "function_call", "call_id": "call_prev", "name": "get_weather", "arguments": "{\"city\":\"Shanghai\"}"},
			map[string]any{"type": "function_call_output", "call_id": "call_prev", "output": "晴 25 度"},
			// reasoning item 不在 v0 范围内，应被忽略而不是报错。
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": "想一下"}}},
		},
	})
	expect(t, "工具调用", code, http.StatusOK, body)
	resp := responsesJSON(t, body)
	output := responsesList(t, resp, "output")
	if len(output) != 1 {
		t.Fatalf("工具调用响应的 output 长度 = %d，期望只有 function_call：%v", len(output), output)
	}
	call := output[0].(map[string]any)
	if got := responsesString(t, call, "type"); got != "function_call" {
		t.Fatalf("output[0].type = %q，期望 function_call", got)
	}
	if got := responsesString(t, call, "call_id"); got != "call_abc" {
		t.Fatalf("call_id = %q，期望 call_abc", got)
	}
	if got := responsesString(t, call, "name"); got != "get_weather" {
		t.Fatalf("name = %q，期望 get_weather", got)
	}
	if got := responsesString(t, call, "arguments"); got != `{"city":"Beijing"}` {
		t.Fatalf("arguments = %q", got)
	}
	tools := responsesList(t, resp, "tools")
	if len(tools) != 1 || responsesString(t, tools[0].(map[string]any), "name") != "get_weather" {
		t.Fatalf("response.tools 回显不对：%v", tools)
	}

	// 转发给上游：tools 转成 chat 的嵌套 function 形状，tool_choice=auto，
	// developer→system、function_call→assistant.tool_calls、output→tool 消息。
	forwarded := f.upstreamRequest(t)
	chatTools, ok := forwarded["tools"].([]any)
	if !ok || len(chatTools) != 1 {
		t.Fatalf("上游 tools 不对：%v", forwarded["tools"])
	}
	chatTool := chatTools[0].(map[string]any)
	if got := responsesString(t, chatTool, "type"); got != "function" {
		t.Fatalf("上游 tool.type = %q", got)
	}
	fn := responsesObject(t, chatTool, "function")
	if got := responsesString(t, fn, "name"); got != "get_weather" {
		t.Fatalf("上游 function.name = %q", got)
	}
	if strict, ok := fn["strict"].(bool); !ok || !strict {
		t.Fatalf("上游 function.strict 没有透传：%v", fn)
	}
	if got := responsesString(t, forwarded, "tool_choice"); got != "auto" {
		t.Fatalf("上游 tool_choice = %v，期望 auto", forwarded["tool_choice"])
	}
	messages := responsesForwardedMessages(t, forwarded)
	if len(messages) != 4 {
		t.Fatalf("转发 messages 长度 = %d，期望 4：%v", len(messages), messages)
	}
	if got := responsesString(t, messages[0], "role"); got != "system" {
		t.Fatalf("developer 消息应转 system：%v", messages[0])
	}
	if got := responsesString(t, messages[2], "role"); got != "assistant" {
		t.Fatalf("function_call 应并进 assistant：%v", messages[2])
	}
	toolCalls := responsesList(t, messages[2], "tool_calls")
	prev := toolCalls[0].(map[string]any)
	if got := responsesString(t, prev, "id"); got != "call_prev" {
		t.Fatalf("历史 tool_call id = %q", got)
	}
	if got := responsesString(t, messages[3], "role"); got != "tool" {
		t.Fatalf("function_call_output 应转 tool 消息：%v", messages[3])
	}
	if got := responsesString(t, messages[3], "tool_call_id"); got != "call_prev" {
		t.Fatalf("tool 消息 tool_call_id = %q", got)
	}
	if got := responsesString(t, messages[3], "content"); got != "晴 25 度" {
		t.Fatalf("tool 消息 content = %q", got)
	}

	// tool_choice 指定函数：Responses 的平铺形状 → chat 的嵌套形状。
	code, body, _ = f.post(t, "ee_at_responses_tools", map[string]any{
		"model":       "chat",
		"input":       "hi",
		"tools":       []any{map[string]any{"type": "function", "name": "get_weather"}},
		"tool_choice": map[string]any{"type": "function", "name": "get_weather"},
	})
	expect(t, "指定工具的 tool_choice", code, http.StatusOK, body)
	choice := responsesObject(t, f.upstreamRequest(t), "tool_choice")
	if got := responsesString(t, choice, "type"); got != "function" {
		t.Fatalf("上游 tool_choice.type = %q", got)
	}
	if got := responsesString(t, responsesObject(t, choice, "function"), "name"); got != "get_weather" {
		t.Fatalf("上游 tool_choice.function.name = %q", got)
	}
}

// ---- 非流式：thinking / text.format / max_output_tokens ----

func TestResponsesReasoningAndFormat(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-reasoning", nil)
	f.token(t, u.ID, "ee_at_resp_reasoning", []string{rbac.ScopeModels}, nil, nil)

	const withReasoning = `{
		"choices": [{
			"message": {"role": "assistant", "content": "答案", "reasoning_content": "先分析问题"},
			"finish_reason": "length"
		}],
		"usage": {
			"prompt_tokens": 20, "completion_tokens": 9, "total_tokens": 29,
			"prompt_tokens_details": {"cached_tokens": 3},
			"completion_tokens_details": {"reasoning_tokens": 7}
		}
	}`
	f.setHandler(func(map[string]any) (int, string) { return http.StatusOK, withReasoning })

	code, body, _ := f.post(t, "ee_at_resp_reasoning", map[string]any{
		"model":             "chat",
		"input":             "问题",
		"max_output_tokens": 256,
		"reasoning":         map[string]any{"effort": "high"},
		"text":              map[string]any{"format": map[string]any{"type": "json_object"}},
	})
	expect(t, "thinking + json_object", code, http.StatusOK, body)

	// 转发给上游：max_output_tokens→max_tokens、reasoning.effort→reasoning_effort、
	// text.format(json_object)→response_format。
	forwarded := f.upstreamRequest(t)
	if got := responsesNumber(t, forwarded, "max_tokens"); got != 256 {
		t.Fatalf("上游 max_tokens = %v，期望 256", got)
	}
	if got := responsesString(t, forwarded, "reasoning_effort"); got != "high" {
		t.Fatalf("上游 reasoning_effort = %q，期望 high", got)
	}
	format := responsesObject(t, forwarded, "response_format")
	if got := responsesString(t, format, "type"); got != "json_object" {
		t.Fatalf("上游 response_format.type = %q", got)
	}

	// responses 对象：status/incomplete_details/usage/思考链转换。
	resp := responsesJSON(t, body)
	if got := responsesString(t, resp, "status"); got != "incomplete" {
		t.Fatalf("finish_reason=length 时 status = %q，期望 incomplete", got)
	}
	incomplete := responsesObject(t, resp, "incomplete_details")
	if got := responsesString(t, incomplete, "reason"); got != "max_output_tokens" {
		t.Fatalf("incomplete_details.reason = %q", got)
	}
	output := responsesList(t, resp, "output")
	if len(output) != 2 {
		t.Fatalf("thinking 响应应有 reasoning + message 两个 item：%v", output)
	}
	reasoningItem := output[0].(map[string]any)
	if got := responsesString(t, reasoningItem, "type"); got != "reasoning" {
		t.Fatalf("output[0].type = %q，期望 reasoning", got)
	}
	reasoningContent := responsesList(t, reasoningItem, "content")
	if got := responsesString(t, reasoningContent[0].(map[string]any), "type"); got != "reasoning_text" {
		t.Fatalf("reasoning content type = %q", got)
	}
	if got := responsesString(t, reasoningContent[0].(map[string]any), "text"); got != "先分析问题" {
		t.Fatalf("reasoning 文本 = %q", got)
	}
	messageItem := output[1].(map[string]any)
	if got := responsesString(t, messageItem, "type"); got != "message" {
		t.Fatalf("output[1].type = %q，期望 message", got)
	}
	usage := responsesObject(t, resp, "usage")
	if got := responsesNumber(t, usage, "input_tokens"); got != 20 {
		t.Fatalf("usage.input_tokens = %v", got)
	}
	if got := responsesNumber(t, usage, "output_tokens"); got != 9 {
		t.Fatalf("usage.output_tokens = %v", got)
	}
	if got := responsesNumber(t, responsesObject(t, usage, "output_tokens_details"), "reasoning_tokens"); got != 7 {
		t.Fatalf("reasoning_tokens = %v，期望 7", got)
	}
	if got := responsesNumber(t, responsesObject(t, usage, "input_tokens_details"), "cached_tokens"); got != 3 {
		t.Fatalf("cached_tokens = %v，期望 3", got)
	}
	// 请求参数回显。
	reasoning := responsesObject(t, resp, "reasoning")
	if got := responsesString(t, reasoning, "effort"); got != "high" {
		t.Fatalf("response.reasoning.effort = %q", got)
	}
	if got := responsesNumber(t, resp, "max_output_tokens"); got != 256 {
		t.Fatalf("response.max_output_tokens = %v", got)
	}
	if got := responsesString(t, responsesObject(t, responsesObject(t, resp, "text"), "format"), "type"); got != "json_object" {
		t.Fatalf("response.text.format.type 回显不对")
	}

	// json_schema 映射：Responses 的平铺 schema → chat 的 json_schema 包装。
	code, body, _ = f.post(t, "ee_at_resp_reasoning", map[string]any{
		"model": "chat",
		"input": "hi",
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "answer", "schema": map[string]any{"type": "object"}, "strict": true,
		}},
	})
	expect(t, "json_schema", code, http.StatusOK, body)
	format = responsesObject(t, f.upstreamRequest(t), "response_format")
	jsonSchema := responsesObject(t, format, "json_schema")
	if got := responsesString(t, jsonSchema, "name"); got != "answer" {
		t.Fatalf("上游 json_schema.name = %q", got)
	}
	if strict, ok := jsonSchema["strict"].(bool); !ok || !strict {
		t.Fatalf("上游 json_schema.strict 没有透传：%v", jsonSchema)
	}
}

// ---- 流式事件 ----

type responsesSSEEvent struct {
	event string
	data  map[string]any
}

func responsesParseSSE(t *testing.T, raw []byte) []responsesSSEEvent {
	t.Helper()
	text := strings.TrimSpace(string(raw))
	if text == "" {
		t.Fatal("流式响应为空")
	}
	var events []responsesSSEEvent
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		ev := responsesSSEEvent{}
		var dataLines []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			}
		}
		if ev.event == "" || len(dataLines) == 0 {
			t.Fatalf("SSE 事件块不完整：%q", block)
		}
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &ev.data); err != nil {
			t.Fatalf("SSE 事件 %s 的 data 不是合法 JSON：%v；%q", ev.event, err, dataLines)
		}
		events = append(events, ev)
	}
	return events
}

func TestResponsesStreamEvents(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-stream", nil)
	f.token(t, u.ID, "ee_at_responses_stream", []string{rbac.ScopeModels}, nil, nil)

	code, body, header := f.post(t, "ee_at_responses_stream", map[string]any{
		"model": "chat", "input": "你好", "stream": true,
	})
	expect(t, "流式请求", code, http.StatusOK, body)
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("流式 Content-Type = %q，期望 text/event-stream", ct)
	}
	if strings.Contains(string(body), "[DONE]") {
		t.Fatalf("Responses 流式不应有 [DONE]：%s", body)
	}

	events := responsesParseSSE(t, body)
	wantOrder := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(events) != len(wantOrder) {
		t.Fatalf("事件数 = %d，期望 %d：%v", len(events), len(wantOrder), responsesSSEEventNames(events))
	}
	for i, want := range wantOrder {
		if events[i].event != want {
			t.Fatalf("第 %d 个事件 = %q，期望 %q；完整序列：%v", i, events[i].event, want, responsesSSEEventNames(events))
		}
		if got := responsesNumber(t, events[i].data, "sequence_number"); got != float64(i) {
			t.Fatalf("第 %d 个事件 sequence_number = %v，期望 %d", i, got, i)
		}
		if got := responsesString(t, events[i].data, "type"); got != want {
			t.Fatalf("event.data.type = %q，期望 %q", got, want)
		}
	}
	// 事件里的事件名与 data.type 一致（opens/DeepSeek 的语义化 SSE 约定）。
	for _, ev := range events {
		if got := responsesString(t, ev.data, "type"); got != ev.event {
			t.Fatalf("事件 %s 的 data.type = %q", ev.event, got)
		}
	}

	created := responsesObject(t, events[0].data, "response")
	if got := responsesString(t, created, "status"); got != "in_progress" {
		t.Fatalf("created.response.status = %q，期望 in_progress", got)
	}
	if output := responsesList(t, created, "output"); len(output) != 0 {
		t.Fatalf("created.response.output 应为空：%v", output)
	}
	delta := events[4].data
	if got := responsesString(t, delta, "delta"); got != "你好" {
		t.Fatalf("output_text.delta = %q", got)
	}
	done := events[7].data
	doneItem := responsesObject(t, done, "item")
	if got := responsesString(t, doneItem, "type"); got != "message" {
		t.Fatalf("output_item.done item.type = %q", got)
	}
	completed := responsesObject(t, events[8].data, "response")
	if got := responsesString(t, completed, "status"); got != "completed" {
		t.Fatalf("completed.response.status = %q", got)
	}
	completedOutput := responsesList(t, completed, "output")
	if len(completedOutput) != 1 {
		t.Fatalf("completed.response.output 长度 = %d", len(completedOutput))
	}
	part := responsesList(t, completedOutput[0].(map[string]any), "content")[0].(map[string]any)
	if got := responsesString(t, part, "text"); got != "你好" {
		t.Fatalf("completed 文本 = %q", got)
	}
	if got := responsesNumber(t, responsesObject(t, completed, "usage"), "total_tokens"); got != 15 {
		t.Fatalf("completed.response.usage.total_tokens = %v", got)
	}

	// 流式请求也按用量记账。
	used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 15 {
		t.Fatalf("流式请求的月度用量 = %d，期望 15", used)
	}

	// 工具调用的流式事件：function_call_arguments.delta/done，item 是 function_call。
	const toolCallJSON = `{
		"choices": [{
			"message": {"role": "assistant", "content": null, "tool_calls": [{
				"id": "call_stream", "type": "function",
				"function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}
			}]},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 12, "completion_tokens": 3, "total_tokens": 15}
	}`
	f.setHandler(func(map[string]any) (int, string) { return http.StatusOK, toolCallJSON })
	code, body, header = f.post(t, "ee_at_responses_stream", map[string]any{
		"model": "chat", "input": "hi", "stream": true,
		"tools": []any{map[string]any{"type": "function", "name": "get_weather"}},
	})
	expect(t, "工具调用流式请求", code, http.StatusOK, body)
	events = responsesParseSSE(t, body)
	var sawDelta, sawDone bool
	for _, ev := range events {
		switch ev.event {
		case "response.output_item.added":
			item := responsesObject(t, ev.data, "item")
			if got := responsesString(t, item, "type"); got != "function_call" {
				t.Fatalf("工具流式 output_item.added item.type = %q", got)
			}
		case "response.function_call_arguments.delta":
			if got := responsesString(t, ev.data, "delta"); got != `{"city":"Beijing"}` {
				t.Fatalf("function_call_arguments.delta = %q", got)
			}
			sawDelta = true
		case "response.function_call_arguments.done":
			if got := responsesString(t, ev.data, "arguments"); got != `{"city":"Beijing"}` {
				t.Fatalf("function_call_arguments.done = %q", got)
			}
			sawDone = true
		}
	}
	if !sawDelta || !sawDone {
		t.Fatalf("工具流式事件缺少 arguments delta/done：%v", responsesSSEEventNames(events))
	}
	if last := events[len(events)-1]; last.event != "response.completed" {
		t.Fatalf("工具流式最后一个事件 = %q", last.event)
	}
}

func responsesSSEEventNames(events []responsesSSEEvent) []string {
	names := make([]string, 0, len(events))
	for _, ev := range events {
		names = append(names, ev.event)
	}
	return names
}

// ---- 鉴权 / 权限 / 额度 / 错误映射 ----

func TestResponsesAuthAndErrors(t *testing.T) {
	f := newResponsesFixture(t)
	user := f.user(t, "responses-errors", nil)
	f.token(t, user.ID, "ee_at_responses_ok", []string{rbac.ScopeModels}, nil, nil)
	f.token(t, user.ID, "ee_at_resp_no_models", []string{rbac.ScopeHub}, nil, nil)
	f.token(t, user.ID, "ee_at_resp_model_lim", []string{rbac.ScopeModels}, []string{"other-model"}, nil)
	quotaUser := f.user(t, "responses-zero-quota", ptrInt64(0))
	f.token(t, quotaUser.ID, "ee_at_resp_zero_quota", []string{rbac.ScopeModels}, nil, nil)

	// 1) 缺少 Bearer → 401 authentication_error / token_invalid。
	code, body, _ := f.post(t, "", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "缺少 Bearer", code, http.StatusUnauthorized, body)
	errObj := responsesErrorObject(t, body)
	if got := responsesString(t, errObj, "type"); got != "authentication_error" {
		t.Fatalf("缺少 Bearer 的 error.type = %q", got)
	}
	if got := responsesString(t, errObj, "code"); got != "token_invalid" {
		t.Fatalf("缺少 Bearer 的 error.code = %q", got)
	}
	if _, exists := errObj["param"]; !exists {
		t.Fatalf("Responses 错误壳应包含 param 字段（可为 null）：%v", errObj)
	}

	// 2) 无效令牌 → 401。
	code, body, _ = f.post(t, "ee_at_does_not_exist", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "无效令牌", code, http.StatusUnauthorized, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "token_invalid" {
		t.Fatalf("无效令牌 error.code = %q", got)
	}

	// 3) 没有 models scope → 403 permission_denied。
	code, body, _ = f.post(t, "ee_at_resp_no_models", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "没有 models scope", code, http.StatusForbidden, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "permission_denied" {
		t.Fatalf("无 models scope 的 error.code = %q", got)
	}

	// 4) 令牌模型限制 → 403 model_not_allowed。
	code, body, _ = f.post(t, "ee_at_resp_model_lim", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "模型不在令牌允许范围", code, http.StatusForbidden, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "model_not_allowed" {
		t.Fatalf("模型限制的 error.code = %q", got)
	}

	// 5) 缺 model / 坏 JSON / 超限参数 / 未知模型 → 400/404。
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{"input": "hi"})
	expect(t, "缺少 model", code, http.StatusBadRequest, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "model_required" {
		t.Fatalf("缺 model 的 error.code = %q", got)
	}
	code, body, _ = f.post(t, "ee_at_responses_ok", "{not json")
	expect(t, "坏 JSON", code, http.StatusBadRequest, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "invalid_json" {
		t.Fatalf("坏 JSON 的 error.code = %q", got)
	}
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{
		"model": "chat", "input": "hi", "max_output_tokens": 1000000,
	})
	expect(t, "max_output_tokens 超限", code, http.StatusBadRequest, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "max_output_tokens_too_large" {
		t.Fatalf("超限参数 error.code = %q", got)
	}
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{"model": "missing-model", "input": "hi"})
	expect(t, "没有可用上游", code, http.StatusNotFound, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "model_not_found" {
		t.Fatalf("缺上游 error.code = %q", got)
	}
	// 图片内容块 v0 不支持，给明确的 400 而不是静默丢弃。
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{
		"model": "chat",
		"input": []any{map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_image", "image_url": "https://example.com/a.png"},
		}}},
	})
	expect(t, "图片内容块", code, http.StatusBadRequest, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "unsupported_content" {
		t.Fatalf("图片内容块 error.code = %q", got)
	}

	// 6) 额度为 0 → 429 insufficient_quota / monthly_quota_exceeded。
	code, body, _ = f.post(t, "ee_at_resp_zero_quota", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "额度用完", code, http.StatusTooManyRequests, body)
	errObj = responsesErrorObject(t, body)
	if got := responsesString(t, errObj, "type"); got != "insufficient_quota" {
		t.Fatalf("额度错误 type = %q", got)
	}
	if got := responsesString(t, errObj, "code"); got != "monthly_quota_exceeded" {
		t.Fatalf("额度错误 code = %q", got)
	}

	// 7) 上游 4xx：保留状态码，沿用上游 error.message/type/code。
	upstreamBadRequest := `{"error":{"message":"model not supported","type":"invalid_request_error","code":"model_not_found"}}`
	f.setHandler(func(map[string]any) (int, string) { return http.StatusBadRequest, upstreamBadRequest })
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "上游 400", code, http.StatusBadRequest, body)
	errObj = responsesErrorObject(t, body)
	if got := responsesString(t, errObj, "message"); got != "model not supported" {
		t.Fatalf("上游错误 message 没有透传：%q", got)
	}
	if got := responsesString(t, errObj, "code"); got != "model_not_found" {
		t.Fatalf("上游错误 code 没有透传：%q", got)
	}

	// 8) 上游 5xx：状态保留，type 归为 api_error。
	f.setHandler(func(map[string]any) (int, string) {
		return http.StatusInternalServerError, `{"error":{"message":"boom"}}`
	})
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "上游 500", code, http.StatusInternalServerError, body)
	if got := responsesString(t, responsesErrorObject(t, body), "type"); got != "api_error" {
		t.Fatalf("上游 500 的 error.type = %q", got)
	}

	// 9) 上游 200 但不是 chat completion → 502 upstream_bad_response。
	f.setHandler(func(map[string]any) (int, string) { return http.StatusOK, `{"unexpected":true}` })
	code, body, _ = f.post(t, "ee_at_responses_ok", map[string]any{"model": "chat", "input": "hi"})
	expect(t, "上游响应无法解析", code, http.StatusBadGateway, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "upstream_bad_response" {
		t.Fatalf("坏上游响应的 error.code = %q", got)
	}

	// 10) 流式请求在上游失败时，还没开始写 SSE，返回普通 JSON 错误壳。
	f.setHandler(func(map[string]any) (int, string) {
		return http.StatusInternalServerError, `{"error":{"message":"boom"}}`
	})
	code, body, header := f.post(t, "ee_at_responses_ok", map[string]any{"model": "chat", "input": "hi", "stream": true})
	expect(t, "流式请求上游失败", code, http.StatusInternalServerError, body)
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("流式失败时的 Content-Type = %q，期望 application/json", ct)
	}
	if _, err := json.Marshal(responsesJSON(t, body)); err != nil {
		t.Fatalf("流式失败响应不是 JSON 错误壳：%v", err)
	}
}

func ptrInt64(v int64) *int64 { return &v }
