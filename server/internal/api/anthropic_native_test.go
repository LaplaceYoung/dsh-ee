package api_test

// DeepSeek 原生 Anthropic 透传的集成测试：fake DeepSeek 原生端点，验证 dsh-ee 只做鉴权/
// 模型权限/额度/限流/model 改写与响应逐字节转发，响应里的 server tools、citations、safety、
// title、latest_reminder、file/vision 块都不被解析或改写；并覆盖同 vendor 故障转移与不跨协议。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// nativeJSONResponse 模拟 DeepSeek 原生非流式响应：包含转换层不支持的 server_tool_use /
// web_search_tool_result 等块，用来证明 dsh-ee 是逐字节转发而不是重新序列化。
const nativeJSONResponse = `{"id":"msg_native","type":"message","role":"assistant","model":"deepseek-flash","content":[{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"dsh-ee"},"caller":{"type":"direct"}},{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://example.com/","title":"dsh-ee"}]},{"type":"text","text":"原生答案"},{"type":"thinking","thinking":"先想一下","signature":"sig-native"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":100,"cache_creation_input_tokens":5,"cache_read_input_tokens":20,"output_tokens":30}}`

// nativeSSEResponse 模拟 DeepSeek 原生流式响应：事件顺序与字段都按 Anthropic 原样转发。
const nativeSSEResponse = `event: message_start
data: {"type":"message_start","message":{"id":"msg_native_stream","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"cache_read_input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好世界"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`

const nativeJSONError = `{"type":"error","error":{"type":"invalid_request_error","message":"原生参数错误"}}`

// nativeRequestCapture 记录原生上游实际收到的请求，用于断言 model 改写与头透传。
type nativeRequestCapture struct {
	Path   string
	Header http.Header
	Body   []byte
}

// nativeAnthropicUpstream 是假的 DeepSeek 原生 /anthropic/v1/messages 端点。
// 请求体里带不同 marker 时返回错误 / 无 usage 响应，其余按 stream 返回固定 JSON 或 SSE。
func nativeAnthropicUpstream(t *testing.T, captures chan<- nativeRequestCapture) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Errorf("原生透传应请求 /anthropic/v1/messages，实际 %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamSecret {
			t.Errorf("原生上游收到的 Authorization 不对：%q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if captures != nil {
			captures <- nativeRequestCapture{Path: r.URL.Path, Header: r.Header.Clone(), Body: raw}
		}
		text := string(raw)
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(raw, &req)

		if strings.Contains(text, "native-400") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, nativeJSONError)
			return
		}
		if strings.Contains(text, "no-usage") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"type":"message","role":"assistant","content":[{"type":"text","text":"没有 usage 的回复"}],"stop_reason":"end_turn"}`)
			return
		}
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, nativeSSEResponse)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, nativeJSONResponse)
	})
}

// updateUpstreamModel 把对外模型的上游名改掉，用于验证原生透传只改 model、其余字段不动。
func updateUpstreamModel(t *testing.T, f *principalFixture, public, upstream string) {
	t.Helper()
	code, body := f.admin.do("GET", "/api/admin/models", nil, nil)
	expect(t, "列出模型", code, http.StatusOK, body)
	var list struct {
		Models []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("解析模型列表失败：%v；%s", err, body)
	}
	for _, m := range list.Models {
		if m.Name != public {
			continue
		}
		code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(m.ID), map[string]any{"upstream_model": upstream}, nil)
		expect(t, "改上游模型名", code, http.StatusOK, body)
		return
	}
	t.Fatalf("没有找到对外模型 %q：%s", public, body)
}

// TestAnthropicNativeDeepSeekPassthrough 覆盖：x-api-key/Bearer、anthropic 头透传、model 改写、
// 非流式/流式逐字节转发、原生错误逐字节转发、usage 旁路记账（含无 usage 估算）与 401/403/429/404。
// 文件/vision 块只有管理员与机器人账号能引用（file_id 访问拦截），这里用机器人账号覆盖原生透传。
func TestAnthropicNativeDeepSeekPassthrough(t *testing.T) {
	captures := make(chan nativeRequestCapture, 8)
	f := newAnthropicFixtureVendor(t, nativeAnthropicUpstream(t, captures), "deepseek", nil)
	updateUpstreamModel(t, f, "chat", "deepseek-flash-native")
	ctx := context.Background()
	admin, err := f.st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	u := f.botUser(t, "anthropic-native", admin.ID)
	f.token(t, u.ID, "ee_at_native", []string{rbac.ScopeModels}, nil, nil)
	monthTokens := func() int64 {
		used, err := f.st.MonthTokensByUser(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		return used
	}

	reqBody := map[string]any{
		"model": "chat", "max_tokens": 64,
		"system": []map[string]any{{"type": "text", "text": "你是助手"}},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{
				{"type": "text", "text": "搜索一下"},
				// file/vision 块不在降级转换层支持范围；原生透传必须原样转发。
				{"type": "image", "source": map[string]any{"type": "file", "file_id": "file_1"}},
			}},
		},
		"tools":           []map[string]any{{"type": "web_search_20250305", "name": "web_search", "max_uses": 1}},
		"safety_check":    map[string]any{"level": "high"},
		"title":           map[string]any{"enabled": true},
		"latest_reminder": map[string]any{"x": 1},
	}
	headers := map[string]string{
		"X-Api-Key":         "ee_at_native",
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":    "files-api-2025-04-14",
	}
	// 额度等错误断言用不带文件引用的请求体：普通员工引用 file_id 会被 403 file_access_denied
	// 提前拦下，会盖住既有的 429/403 断言。
	plainBody := map[string]any{
		"model": "chat", "max_tokens": 8,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	}
	code, body := newClient(t, f.ts.URL).do("POST", "/v1/messages", reqBody, headers)
	expect(t, "原生非流式透传", code, http.StatusOK, body)
	if string(body) != nativeJSONResponse {
		t.Fatalf("响应不是逐字节透传：\n%s", body)
	}
	captured := <-captures
	var upstreamReq map[string]any
	if err := json.Unmarshal(captured.Body, &upstreamReq); err != nil {
		t.Fatalf("上游请求不是 JSON：%v", err)
	}
	if upstreamReq["model"] != "deepseek-flash-native" {
		t.Fatalf("model 没有改写成上游名：%v", upstreamReq["model"])
	}
	for _, needle := range []string{`"web_search_20250305"`, `"file_1"`, `"safety_check"`, `"latest_reminder"`, `"title"`} {
		if !strings.Contains(string(captured.Body), needle) {
			t.Fatalf("原生请求内容被改写，缺少 %s：%s", needle, captured.Body)
		}
	}
	if got := captured.Header.Get("Anthropic-Version"); got != "2023-06-01" {
		t.Fatalf("Anthropic-Version 没有透传：%q", got)
	}
	if got := captured.Header.Get("Anthropic-Beta"); got != "files-api-2025-04-14" {
		t.Fatalf("Anthropic-Beta 没有透传：%q", got)
	}
	// usage 记账：input 100 + cache_creation 5 + cache_read 20，output 30。
	if got := monthTokens(); got != 155 {
		t.Fatalf("非流式 usage 记账 = %d，期望 155", got)
	}

	// 流式（别名 + Bearer）：SSE 逐字节转发，usage 从 message_start/message_delta 旁路读取。
	streamBody := map[string]any{
		"model": "chat", "max_tokens": 32, "stream": true,
		"messages": []map[string]any{{"role": "user", "content": "流式问题"}},
	}
	code, body = newClient(t, f.ts.URL).do("POST", "/anthropic/v1/messages", streamBody, bearer("ee_at_native"))
	expect(t, "原生流式透传", code, http.StatusOK, body)
	if string(body) != nativeSSEResponse {
		t.Fatalf("SSE 不是逐字节透传：\n%s", body)
	}
	streamCapture := <-captures
	if got := streamCapture.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("流式请求的 Accept 不对：%q", got)
	}
	// 50 + cache_read 10 + output 7 = 67，累计 222。
	if got := monthTokens(); got != 222 {
		t.Fatalf("流式 usage 记账累计 = %d，期望 222", got)
	}

	// 原生错误壳原样转发（状态码与 body 都不改写）。
	errorBody := map[string]any{
		"model": "chat", "max_tokens": 8,
		"messages": []map[string]any{{"role": "user", "content": "native-400"}},
	}
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", errorBody, headers)
	if code != http.StatusBadRequest || string(body) != nativeJSONError {
		t.Fatalf("原生错误没有原样转发：%d %s", code, body)
	}

	// 无 usage 响应：按请求与正文估算记账，额度不能被记成 0。
	before := monthTokens()
	noUsageBody := map[string]any{
		"model": "chat", "max_tokens": 8,
		"messages": []map[string]any{{"role": "user", "content": "no-usage"}},
	}
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", noUsageBody, headers)
	expect(t, "无 usage 原生响应", code, http.StatusOK, body)
	if after := monthTokens(); after <= before {
		t.Fatalf("无 usage 时应估算记账：before=%d after=%d", before, after)
	}

	// 鉴权/权限/额度/模型错误仍由 dsh-ee 以 Anthropic 错误壳返回。
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", reqBody, nil)
	assertAnthropicErr(t, "原生缺凭证", code, 401, body, "authentication_error", "API key")
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", reqBody, map[string]string{"X-Api-Key": "ee_at_nope"})
	assertAnthropicErr(t, "原生无效 key", code, 401, body, "authentication_error", "无效")
	f.token(t, u.ID, "ee_at_native_hub", []string{rbac.ScopeHub}, nil, nil)
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", reqBody, map[string]string{"X-Api-Key": "ee_at_native_hub"})
	assertAnthropicErr(t, "原生缺 models scope", code, 403, body, "permission_error", "没有调用大模型")
	f.token(t, u.ID, "ee_at_native_model", []string{rbac.ScopeModels}, nil, []string{"other"})
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", reqBody, map[string]string{"X-Api-Key": "ee_at_native_model"})
	assertAnthropicErr(t, "原生模型不在范围", code, 403, body, "permission_error", "不能使用模型")
	noQuota := f.user(t, "anthropic-native-noquota", new(int64))
	f.token(t, noQuota.ID, "ee_at_native_noquota", []string{rbac.ScopeModels}, nil, nil)
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", plainBody, map[string]string{"X-Api-Key": "ee_at_native_noquota"})
	assertAnthropicErr(t, "原生未分配额度", code, 429, body, "rate_limit_error", "额度")
	missing := map[string]any{"model": "missing-model", "max_tokens": 8,
		"messages": []map[string]any{{"role": "user", "content": "hi"}}}
	code, body = newClient(t, f.ts.URL).do("POST", "/v1/messages", missing, headers)
	assertAnthropicErr(t, "原生模型没有上游", code, 404, body, "not_found_error", "上游")
}

// TestAnthropicNativeDeepSeekFailover 覆盖同 vendor 故障转移，并确认不会跨协议切到自定义 provider。
func TestAnthropicNativeDeepSeekFailover(t *testing.T) {
	var aCalls, bCalls, cCalls atomic.Int64

	writeJSON := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
	// A：deepseek 优先线路；failover marker 返回 429，both500 marker 返回 500。
	upstreamA := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aCalls.Add(1)
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Errorf("A 收到的路径不对：%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		text := string(raw)
		switch {
		case strings.Contains(text, "failover"):
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"A 限流"}}`)
		case strings.Contains(text, "both500"):
			writeJSON(w, http.StatusInternalServerError, `{"type":"error","error":{"type":"api_error","message":"A 挂了"}}`)
		default:
			writeJSON(w, http.StatusOK, `{"type":"message","role":"assistant","content":[{"type":"text","text":"A"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
		}
	})
	// B：第二个 deepseek 线路；failover 时返回 200，其它 marker 返回 500。
	upstreamB := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bCalls.Add(1)
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Errorf("B 收到的路径不对：%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "failover") {
			writeJSON(w, http.StatusOK, `{"type":"message","role":"assistant","content":[{"type":"text","text":"from-B"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
			return
		}
		writeJSON(w, http.StatusInternalServerError, `{"type":"error","error":{"type":"api_error","message":"B 挂了"}}`)
	})
	// C：custom provider；原生路径 + 同 vendor 故障转移都不应该调用它。
	upstreamC := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cCalls.Add(1)
		writeJSON(w, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"C"},"finish_reason":"stop"}]}`)
	})

	f := newAnthropicFixtureVendor(t, upstreamA, "deepseek", nil)
	serverB := httptest.NewServer(upstreamB)
	t.Cleanup(serverB.Close)
	serverC := httptest.NewServer(upstreamC)
	t.Cleanup(serverC.Close)
	code, body := f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "deepseek-b", "vendor": "deepseek", "base_url": serverB.URL,
		"api_key": upstreamSecret, "models": []string{"chat"},
	}, nil)
	expect(t, "创建第二个 deepseek 上游", code, http.StatusCreated, body)
	code, body = f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "custom-c", "vendor": "custom", "base_url": serverC.URL,
		"api_key": upstreamSecret, "models": []string{"chat"},
	}, nil)
	expect(t, "创建 custom 上游", code, http.StatusCreated, body)

	u := f.user(t, "anthropic-native-failover", nil)
	f.token(t, u.ID, "ee_at_native_fo", []string{rbac.ScopeModels}, nil, nil)

	call := func(marker string) (int, []byte) {
		return newClient(t, f.ts.URL).do("POST", "/v1/messages", map[string]any{
			"model": "chat", "max_tokens": 16,
			"messages": []map[string]any{{"role": "user", "content": marker}},
		}, map[string]string{"X-Api-Key": "ee_at_native_fo"})
	}

	// 同 vendor 故障转移：A 429 → B 200。
	code, body = call("failover")
	expect(t, "原生同 vendor 故障转移", code, http.StatusOK, body)
	if !strings.Contains(string(body), "from-B") {
		t.Fatalf("故障转移后应返回 B 的原生响应：%s", body)
	}
	if aCalls.Load() != 1 || bCalls.Load() != 1 || cCalls.Load() != 0 {
		t.Fatalf("failover 调用次数不对：A=%d B=%d C=%d", aCalls.Load(), bCalls.Load(), cCalls.Load())
	}

	// 不跨协议：A 冷却跳过、B 500（最后一条 deepseek 线路）→ 原生 500 原样返回，C 不被调用。
	code, body = call("both500")
	if code != http.StatusInternalServerError || !strings.Contains(string(body), "B 挂了") {
		t.Fatalf("最后一条 deepseek 线路的错误应原样返回：%d %s", code, body)
	}
	if cCalls.Load() != 0 {
		t.Fatalf("不应跨协议切到 custom provider：C 被调用 %d 次", cCalls.Load())
	}
}
