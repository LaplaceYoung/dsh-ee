package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DeepSeek Chatbot 网关选项与透传的端到端测试（docs/dev/chatbot.md）：
// 注入只发生在 vendor=deepseek 的线路上、客户端已有字段时保留、非 DeepSeek 不注入；
// 请求体与响应（含流式）里的服务端工具/安全/标题/拒绝/引用块原样转发，不解析改写。

const chatbotUpstreamKey = "chatbot-upstream-key"

// chatbotChatReq 是一条上游线路收到的 /chat/completions 请求，body 用 json.Number 解码保留数字精度。
type chatbotChatReq struct {
	body map[string]any
	raw  []byte
}

// chatbotUpstream 是可编程的假上游：记录收到的请求体，按设置返回普通 JSON 或 SSE。
type chatbotUpstream struct {
	srv *httptest.Server
	mu  sync.Mutex
	// 收到的原始请求体（断言时按 json.Number 解码，保留数字精度）。
	raws [][]byte
	// 初始请求体的 JSON 文本。
	responseJSON string
	streamChunks []string
	status       int
}

func newChatbotUpstream(t *testing.T) *chatbotUpstream {
	t.Helper()
	u := &chatbotUpstream{
		status:       http.StatusOK,
		responseJSON: `{"id":"chatcmpl-x","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
	}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+chatbotUpstreamKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.raws = append(u.raws, raw)
		status := u.status
		stream := append([]string(nil), u.streamChunks...)
		resp := u.responseJSON
		u.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"upstream failure"}}`)
			return
		}
		if stream != nil {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range stream {
				_, _ = io.WriteString(w, chunk)
				w.(http.Flusher).Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

// 记录收到的原始请求体；解码延迟到断言时做，避免 handler 里 t.Fatalf。
func (u *chatbotUpstream) requests(t *testing.T) []chatbotChatReq {
	t.Helper()
	u.mu.Lock()
	raws := append([][]byte(nil), u.raws...)
	u.mu.Unlock()
	out := make([]chatbotChatReq, 0, len(raws))
	for _, raw := range raws {
		out = append(out, chatbotChatReq{body: decodeNumberMap(t, raw), raw: raw})
	}
	return out
}

func (u *chatbotUpstream) last(t *testing.T) chatbotChatReq {
	t.Helper()
	reqs := u.requests(t)
	if len(reqs) == 0 {
		t.Fatal("上游没有收到请求")
	}
	return reqs[len(reqs)-1]
}

func (u *chatbotUpstream) setResponseJSON(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.responseJSON = body
	u.streamChunks = nil
}

func (u *chatbotUpstream) setStream(chunks ...string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.streamChunks = chunks
}

func (u *chatbotUpstream) setStatus(code int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status = code
}

// decodeNumberMap 按网关同样的方式解码（UseNumber），用于比较请求体与数字精度。
func decodeNumberMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("解码 JSON 失败：%v；%s", err, raw)
	}
	return body
}

// newChatbotGateway 起一个可以在指定 vendor 线路上对话的测试网关。
// publicModel 是客户端看到的模型名，upstreamModel 是这条上游实际收到的模型名。
func newChatbotGateway(t *testing.T, vendor, publicModel, upstreamModel string, mutate func(*api.Options)) (*principalFixture, *chatbotUpstream) {
	t.Helper()
	f := newPrincipalFixtureWith(t, mutate)
	up := newChatbotUpstream(t)
	code, body := f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "chatbot-" + vendor, "vendor": vendor, "base_url": up.srv.URL, "api_key": chatbotUpstreamKey,
	}, nil)
	expect(t, "建 chatbot 上游", code, 201, body)
	providerID := decode[struct{ ID int64 }](t, body).ID
	code, body = f.admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": providerID,
		"items":       []map[string]string{{"upstream_model": upstreamModel, "name": publicModel}},
	}, nil)
	expect(t, "建 chatbot 模型", code, 201, body)
	return f, up
}

// chatbotToken 给员工发一个 models scope 的令牌，并返回用户与请求头。
func chatbotToken(t *testing.T, f *principalFixture, username string) (store.User, map[string]string) {
	t.Helper()
	u := f.user(t, username, nil)
	plain := "ee_at_" + username
	f.token(t, u.ID, plain, []string{rbac.ScopeModels}, nil, nil)
	return u, bearer(plain)
}

func chatCall(t *testing.T, f *principalFixture, header map[string]string, body map[string]any) (int, []byte) {
	t.Helper()
	return newClient(t, f.ts.URL).do("POST", "/v1/chat/completions", body, header)
}

func chatBody(model string) map[string]any {
	return map[string]any{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": "What is new in DeepSeek V4.1?"}},
	}
}

// TestChatbotOptionInjection 覆盖注入开关、客户端覆盖与非 DeepSeek 上游。
func TestChatbotOptionInjection(t *testing.T) {
	cases := []struct {
		name       string
		vendor     string
		opt        api.Options
		extra      map[string]any
		wantSafety any
		wantTitle  any
	}{
		{name: "默认关闭不注入", vendor: "deepseek"},
		{
			name: "开启后给 DeepSeek 注入", vendor: "deepseek",
			opt:        api.Options{SafetyCheckLevel: "high", ChatTitle: true},
			wantSafety: map[string]any{"level": "high"},
			wantTitle:  map[string]any{"enabled": true},
		},
		{
			name: "非 DeepSeek 上游不注入", vendor: "custom",
			opt: api.Options{SafetyCheckLevel: "high", ChatTitle: true},
		},
		{
			name: "客户端已传则保留客户端值", vendor: "deepseek",
			opt: api.Options{SafetyCheckLevel: "high", ChatTitle: true},
			extra: map[string]any{
				"safety_check": map[string]any{"level": "low"},
				"title":        map[string]any{"enabled": false},
			},
			wantSafety: map[string]any{"level": "low"},
			wantTitle:  map[string]any{"enabled": false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutate := func(o *api.Options) {
				o.SafetyCheckLevel = tc.opt.SafetyCheckLevel
				o.ChatTitle = tc.opt.ChatTitle
			}
			f, up := newChatbotGateway(t, tc.vendor, "chatbot-chat", "deepseek-flash", mutate)
			u, header := chatbotToken(t, f, "chat-inj")
			body := chatBody("chatbot-chat")
			for k, v := range tc.extra {
				body[k] = v
			}
			code, resp := chatCall(t, f, header, body)
			expect(t, "调用 chatbot 模型", code, 200, resp)

			upReq := up.last(t)
			if got := upReq.body["model"]; got != "deepseek-flash" {
				t.Fatalf("上游收到的 model = %v，期望 deepseek-flash", got)
			}
			check := func(key string, want any) {
				t.Helper()
				got, exists := upReq.body[key]
				if want == nil {
					if exists {
						t.Fatalf("不应向上游注入 %q：%v（body=%s）", key, got, upReq.raw)
					}
					return
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%q = %#v，期望 %#v", key, got, want)
				}
			}
			check("safety_check", tc.wantSafety)
			check("title", tc.wantTitle)
			// 客户端 body 的其余字段照常转发。
			if !reflect.DeepEqual(upReq.body["messages"], decodeNumberMap(t, mustJSON(body))["messages"]) {
				t.Fatalf("messages 没有原样转发：%v", upReq.body["messages"])
			}
			// 注入不改记账：仍按上游返回的 usage 记在对外模型名下。
			used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
			if err != nil {
				t.Fatal(err)
			}
			if used != 5 {
				t.Fatalf("月度用量 = %d，期望 5（上游 usage）", used)
			}
			byModel, err := f.st.UserUsageByModel(context.Background(), u.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(byModel) != 1 || byModel[0].Name != "chatbot-chat" || byModel[0].Tokens != 5 {
				t.Fatalf("记账应记对外模型名 chatbot-chat：%+v", byModel)
			}
		})
	}
}

// TestChatbotRequestBodyPassthrough 覆盖服务端工具、safety_check、title 等字段的请求透传：
// 客户端传什么都不改写（除 model 换成该线路的上游模型名），数字精度也保留。
func TestChatbotRequestBodyPassthrough(t *testing.T) {
	f, up := newChatbotGateway(t, "deepseek", "chatbot-chat", "deepseek-flash", func(o *api.Options) {
		o.SafetyCheckLevel = "high"
		o.ChatTitle = true
	})
	_, header := chatbotToken(t, f, "chat-pass")
	body := map[string]any{
		"model": "chatbot-chat",
		"messages": []any{
			map[string]any{"role": "user", "content": "Summarize https://example.com/ in one sentence."},
			map[string]any{
				"role":    "assistant",
				"content": "The example.com domain is reserved for use in documentation examples.",
				"server_tool_calls": []any{
					map[string]any{
						"index": 0,
						"calls": []any{
							map[string]any{
								"index": 0, "id": "srvtoolu_rl_00_1", "type": "deepseek_read_url", "kind": "read_link",
								"action": map[string]any{"type": "read_link", "url": "https://example.com"},
								"status": "completed", "signature": "tQU8k+8Ko7MY5XDAvJd0",
							},
						},
					},
				},
			},
			map[string]any{"role": "user", "content": "Quote the first sentence of that page."},
		},
		"tools": []any{
			map[string]any{"type": "deepseek_web_search"},
			map[string]any{"type": "deepseek_read_url"},
		},
		"safety_check": map[string]any{"level": "low"},
		"title":        map[string]any{"enabled": false},
		"thinking":     map[string]any{"type": "enabled"},
		"temperature":  0.25,
		"max_tokens":   1234,
	}
	code, resp := chatCall(t, f, header, body)
	expect(t, "调用 chatbot 模型", code, 200, resp)

	upReq := up.last(t)
	// 期望 = 客户端 body 只把 model 换成该线路的上游模型名；safety_check/title 客户端已传，保留原值。
	want := decodeNumberMap(t, mustJSON(body))
	want["model"] = "deepseek-flash"
	if !reflect.DeepEqual(upReq.body, want) {
		t.Fatalf("请求体没有原样转发：\n上游 = %s\n期望 = %s", upReq.raw, mustJSON(want))
	}
	// 再明确断言几个关键字段，失败时更好定位。
	if !reflect.DeepEqual(upReq.body["tools"], want["tools"]) {
		t.Fatalf("tools 被改写：%v", upReq.body["tools"])
	}
	if !reflect.DeepEqual(upReq.body["safety_check"], map[string]any{"level": "low"}) {
		t.Fatalf("safety_check 应保留客户端值：%v", upReq.body["safety_check"])
	}
	if !reflect.DeepEqual(upReq.body["title"], map[string]any{"enabled": false}) {
		t.Fatalf("title 应保留客户端值：%v", upReq.body["title"])
	}
}

// TestChatbotResponsePassthrough 覆盖响应原样返回：title/annotations(citations)/refusal/
// server_tool_calls/server_tool_use 等扩展块不解析、不改写（字节级比较）。
func TestChatbotResponsePassthrough(t *testing.T) {
	cases := []struct {
		name string
		resp string
	}{
		{
			name: "联网工具与引用块",
			resp: `{"id":"chatcmpl-chatbot","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"答案","title":"DeepSeek V4.1 更新","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/news","title":"来源","start_index":0,"end_index":2}},{"type":"url_citation","url_citation":{"url":"https://example.com/post","title":null,"start_index":3,"end_index":5}}],"server_tool_calls":[{"index":0,"reasoning_content":"","calls":[{"index":0,"id":"srvtoolu_rl_00_1","type":"deepseek_read_url","kind":"read_link","action":{"type":"read_link","url":"https://example.com/post"},"status":"completed","page":{"url":"https://example.com/post","title":"Example","published_at":"2026-01-02"},"signature":"q3J0b1RmZ1c4cE1x"}]}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"server_tool_use":{"web_search_requests":1,"web_fetch_requests":1,"tool_search":1,"tool_fetch":1,"tool_image_search":0}},"title":"DeepSeek V4.1 更新"}`,
		},
		{
			name: "内容安全拒绝",
			resp: `{"id":"chatcmpl-refusal","choices":[{"index":0,"message":{"role":"assistant","content":"","refusal":"The following images were blocked by content moderation: messages.1.content.0"},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20,"completion_tokens_details":{"safety_tokens":8}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, up := newChatbotGateway(t, "deepseek", "chatbot-chat", "deepseek-flash", func(o *api.Options) {
				o.SafetyCheckLevel = "high"
				o.ChatTitle = true
			})
			_, header := chatbotToken(t, f, "chat-resp")
			up.setResponseJSON(tc.resp)
			code, resp := chatCall(t, f, header, chatBody("chatbot-chat"))
			expect(t, "调用 chatbot 模型", code, 200, resp)
			if string(resp) != tc.resp {
				t.Fatalf("响应被改写：\n上游 = %s\n网关 = %s", tc.resp, resp)
			}
			// 注入只多发请求字段，响应与 usage 记账保持上游给的数字。
		})
	}
}

// TestChatbotStreamPassthrough 覆盖流式响应的原样转发：delta.title、引用、拒绝、服务端
// 工具事件与 usage 都不解析改写，只旁路提取 usage 记账。
func TestChatbotStreamPassthrough(t *testing.T) {
	f, up := newChatbotGateway(t, "deepseek", "chatbot-chat", "deepseek-flash", func(o *api.Options) {
		o.SafetyCheckLevel = "high"
		o.ChatTitle = true
	})
	u, header := chatbotToken(t, f, "chat-stream")
	chunks := []string{
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"server_tool_event\":{\"type\":\"call_started\",\"step\":0,\"call\":{\"index\":0,\"id\":\"srvtoolu_s_00_1\",\"type\":\"deepseek_web_search\",\"kind\":\"search\",\"action\":{\"type\":\"search\",\"queries\":[\"DeepSeek V4.1\"]}}}}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"DeepSeek V4.1 更新了\"}}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"若干能力。\",\"annotations\":[{\"type\":\"url_citation\",\"url_citation\":{\"url\":\"https://example.com/news\",\"title\":\"来源\",\"start_index\":0,\"end_index\":5}}],\"title\":\"DeepSeek V4.1 更新\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15,\"server_tool_use\":{\"tool_search\":1}}}\n\n",
		"data: [DONE]\n\n",
	}
	up.setStream(chunks...)
	body := chatBody("chatbot-chat")
	body["stream"] = true
	code, resp := chatCall(t, f, header, body)
	expect(t, "流式调用 chatbot 模型", code, 200, resp)
	if want := strings.Join(chunks, ""); string(resp) != want {
		t.Fatalf("流式响应被改写：\n上游 = %q\n网关 = %q", want, resp)
	}
	// 上游请求体：注入 safety_check/title，并照旧补 stream_options.include_usage。
	upReq := up.last(t)
	if !reflect.DeepEqual(upReq.body["safety_check"], map[string]any{"level": "high"}) {
		t.Fatalf("流式请求也应注入 safety_check：%v", upReq.body["safety_check"])
	}
	if !reflect.DeepEqual(upReq.body["title"], map[string]any{"enabled": true}) {
		t.Fatalf("流式请求也应注入 title：%v", upReq.body["title"])
	}
	streamOptions, _ := upReq.body["stream_options"].(map[string]any)
	if streamOptions["include_usage"] != true {
		t.Fatalf("流式请求应照旧补 include_usage：%v", upReq.body["stream_options"])
	}
	used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 15 {
		t.Fatalf("流式 usage 记账 = %d，期望 15", used)
	}
}

// TestChatbotFailoverDoesNotLeakInjection 覆盖故障切换：DeepSeek 线路 5xx 后换到非 DeepSeek
// 线路时，不能把上一条线路的 safety_check/title 注入带过去。
func TestChatbotFailoverDoesNotLeakInjection(t *testing.T) {
	f := newPrincipalFixtureWith(t, func(o *api.Options) {
		o.SafetyCheckLevel = "high"
		o.ChatTitle = true
	})
	dsUp := newChatbotUpstream(t)
	customUp := newChatbotUpstream(t)
	dsUp.setStatus(http.StatusServiceUnavailable)

	addProvider := func(name, vendor, baseURL, upstreamModel string, priority int) int64 {
		code, body := f.admin.do("POST", "/api/admin/providers", map[string]any{
			"name": name, "vendor": vendor, "base_url": baseURL, "api_key": chatbotUpstreamKey, "priority": priority,
		}, nil)
		expect(t, "建上游 "+name, code, 201, body)
		id := decode[struct{ ID int64 }](t, body).ID
		code, body = f.admin.do("POST", "/api/admin/models", map[string]any{
			"provider_id": id,
			"items":       []map[string]string{{"upstream_model": upstreamModel, "name": "failover-chat"}},
		}, nil)
		expect(t, "建模型 "+name, code, 201, body)
		return id
	}
	addProvider("failover-deepseek", "deepseek", dsUp.srv.URL, "deepseek-flash", 10)
	addProvider("failover-custom", "custom", customUp.srv.URL, "vendor-chat", 20)

	_, header := chatbotToken(t, f, "chat-fail")
	code, resp := chatCall(t, f, header, chatBody("failover-chat"))
	expect(t, "故障切换后调用", code, 200, resp)

	dsReq, customReq := dsUp.last(t), customUp.last(t)
	if dsReq.body["model"] != "deepseek-flash" {
		t.Fatalf("DeepSeek 线路的 model 不对：%v", dsReq.body["model"])
	}
	if !reflect.DeepEqual(dsReq.body["safety_check"], map[string]any{"level": "high"}) ||
		!reflect.DeepEqual(dsReq.body["title"], map[string]any{"enabled": true}) {
		t.Fatalf("DeepSeek 线路应收到注入字段：%v", dsReq.body)
	}
	if customReq.body["model"] != "vendor-chat" {
		t.Fatalf("备用线路的 model 不对：%v", customReq.body["model"])
	}
	for _, key := range []string{"safety_check", "title"} {
		if _, ok := customReq.body[key]; ok {
			t.Fatalf("备用线路不应收到 %q 注入：%v", key, customReq.body)
		}
	}
}
