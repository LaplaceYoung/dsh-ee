package api_test

// DeepSeek Files（file_id）访问拦截的集成测试。
//
// 覆盖：普通员工在 /v1/chat/completions 与 /v1/messages（含 /anthropic/v1/messages 别名）
// 引用上游共享文件时 403，且请求不能到上游；管理员、account_type=bot、显式开关放行；
// 没有 file_id 的请求不受影响（工具参数里出现 file_id 字样不算引用）。

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const (
	fileGuardChatResponse      = `{"id":"chatcmpl-file","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"好的"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	fileGuardChatStream        = "data: {\"choices\":[{\"delta\":{\"content\":\"好的\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n"
	fileGuardAnthropicResponse = `{"id":"msg_file","type":"message","role":"assistant","model":"upstream-model","content":[{"type":"text","text":"好的"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	fileGuardAnthropicStream   = `event: message_start
data: {"type":"message_start","message":{"id":"msg_file_stream","type":"message","role":"assistant","model":"upstream-model","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"好的"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`
)

// fileGuardUpstream 同时扮演 OpenAI chat completions 与 DeepSeek 原生 Anthropic 两种上游，
// 记录调用次数与最后一次请求体：用来断言被拦截的请求从来没有到过上游，放行的请求仍原样转发。
type fileGuardUpstream struct {
	url            string
	client         *http.Client
	chatCalls      atomic.Int64
	anthropicCalls atomic.Int64
	lastChat       atomic.Value // []byte
	lastAnthropic  atomic.Value // []byte
}

func newFileGuardUpstream(t *testing.T) *fileGuardUpstream {
	t.Helper()
	up := &fileGuardUpstream{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			up.chatCalls.Add(1)
			raw, _ := io.ReadAll(r.Body)
			up.lastChat.Store(raw)
			var req struct {
				Stream bool `json:"stream"`
			}
			_ = json.Unmarshal(raw, &req)
			if req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, fileGuardChatStream)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, fileGuardChatResponse)
		case "/anthropic/v1/messages":
			up.anthropicCalls.Add(1)
			raw, _ := io.ReadAll(r.Body)
			up.lastAnthropic.Store(raw)
			var req struct {
				Stream bool `json:"stream"`
			}
			_ = json.Unmarshal(raw, &req)
			if req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, fileGuardAnthropicStream)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, fileGuardAnthropicResponse)
		default:
			t.Errorf("上游收到意料之外的路径：%s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	up.client = ts.Client()
	up.url = ts.URL
	return up
}

// fileGuardFixture 建库、登录管理员、注册一个 vendor=deepseek 的上游（chat 与原生 Anthropic 都可用），
// 并保留 master key 与上游，方便用同一份库再起一个不同 Options 的 Server 验证开关。
type fileGuardFixture struct {
	*principalFixture
	up  *fileGuardUpstream
	box *secret.Box
}

func newFileGuardFixture(t *testing.T) *fileGuardFixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	up := newFileGuardUpstream(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), Upstream: up.client}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	f := &principalFixture{st: st, srv: srv, ts: ts, admin: newClient(t, ts.URL)}
	code, body := f.admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)
	code, body = f.admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "file-guard-upstream", "base_url": up.url, "api_key": upstreamSecret,
		"vendor": "deepseek", "models": []string{"chat"},
	}, nil)
	expect(t, "创建模型上游", code, http.StatusCreated, body)
	return &fileGuardFixture{principalFixture: f, up: up, box: box}
}

// startServer 用同一份库再起一个 Server（不重新迁移数据库），用于验证开关等不同 Options。
func (f *fileGuardFixture) startServer(t *testing.T, mutate func(*api.Options)) *httptest.Server {
	t.Helper()
	opts := api.Options{DataDir: t.TempDir(), Upstream: f.up.client}
	if mutate != nil {
		mutate(&opts)
	}
	srv := api.New(f.st, f.box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// botUser 建一个 account_type=bot 的机器人账号（不限额），自动流程用它引用共享文件。
func (f *principalFixture) botUser(t *testing.T, username string, ownerID int64) store.User {
	t.Helper()
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "department": "研发效能部", "account_type": "bot",
		"owner_id": ownerID, "monthly_token_limit": nil,
	}, nil)
	expect(t, "创建机器人账号 "+username, code, http.StatusCreated, body)
	return decode[store.User](t, body)
}

func chatFileBody(fileBlock map[string]any, stream bool) map[string]any {
	return map[string]any{
		"model":  "chat",
		"stream": stream,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "看下这个附件"},
				fileBlock,
			},
		}},
	}
}

func chatFileBlocks() map[string]map[string]any {
	return map[string]map[string]any{
		"chat file 块":     {"type": "file", "file_id": "file_shared_1"},
		"chat document 块": {"type": "document", "source": map[string]any{"type": "file", "file_id": "file_doc_1"}},
		"chat image 文件块":  {"type": "image", "source": map[string]any{"type": "file", "file_id": "file_image_1"}},
	}
}

func anthropicFileBody(block map[string]any, stream bool) map[string]any {
	return map[string]any{
		"model": "chat", "max_tokens": 64, "stream": stream,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "看下这个文件"},
				block,
			},
		}},
	}
}

func anthropicFileBlocks() map[string]map[string]any {
	return map[string]map[string]any{
		"messages file 块":     {"type": "file", "file_id": "file_shared_1"},
		"messages document 块": {"type": "document", "source": map[string]any{"type": "file", "file_id": "file_doc_1"}},
		"messages image 文件块":  {"type": "image", "source": map[string]any{"type": "file", "file_id": "file_image_1"}},
	}
}

func assertOpenAIFileDenied(t *testing.T, name string, code int, resp []byte) {
	t.Helper()
	if code != http.StatusForbidden {
		t.Fatalf("%s：状态码 %d，期望 403；响应 %s", name, code, resp)
	}
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("%s：不是 OpenAI 错误壳：%v；%s", name, err, resp)
	}
	if parsed.Error.Type != "permission_error" || parsed.Error.Code != "file_access_denied" {
		t.Fatalf("%s：错误码不对（type=%q code=%q）：%s", name, parsed.Error.Type, parsed.Error.Code, resp)
	}
	if !strings.Contains(parsed.Error.Message, "file_id") {
		t.Fatalf("%s：错误说明应提到 file_id：%s", name, resp)
	}
}

func assertAnthropicFileDenied(t *testing.T, name string, code int, resp []byte) {
	t.Helper()
	if code != http.StatusForbidden {
		t.Fatalf("%s：状态码 %d，期望 403；响应 %s", name, code, resp)
	}
	var parsed struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("%s：不是 Anthropic 错误壳：%v；%s", name, err, resp)
	}
	if parsed.Type != "error" || parsed.Error.Type != "permission_error" || parsed.Error.Code != "file_access_denied" {
		t.Fatalf("%s：错误码不对（envelope=%q type=%q code=%q）：%s", name, parsed.Type, parsed.Error.Type, parsed.Error.Code, resp)
	}
	if !strings.Contains(parsed.Error.Message, "file_id") {
		t.Fatalf("%s：错误说明应提到 file_id：%s", name, resp)
	}
}

// TestFileAccessGuard 覆盖普通员工拦截、无 file_id 不受影响、管理员/机器人放行，
// 以及同一份库上把 DeepSeekToolsAllowEmployee 打开后员工放行。共用一份测试库，避免重复迁移。
func TestFileAccessGuard(t *testing.T) {
	f := newFileGuardFixture(t)
	up := f.up
	ctx := context.Background()
	c := newClient(t, f.ts.URL)

	employee := f.user(t, "file-guard-employee", nil)
	f.token(t, employee.ID, "ee_at_file_emp", []string{rbac.ScopeModels}, nil, nil)
	adminUser, err := f.st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	f.token(t, adminUser.ID, "ee_at_file_admin", []string{rbac.ScopeModels}, nil, nil)
	bot := f.botUser(t, "file-guard-bot", employee.ID)
	f.token(t, bot.ID, "ee_at_file_bot", []string{rbac.ScopeModels}, nil, nil)
	employeeAuth := bearer("ee_at_file_emp")
	employeeHeader := map[string]string{"X-Api-Key": "ee_at_file_emp"}

	// ---- 普通员工引用 file_id：chat / messages、非流式 / 流式都 403，且请求不能到上游 ----
	for name, block := range chatFileBlocks() {
		for _, stream := range []bool{false, true} {
			what := name
			if stream {
				what += "（流式）"
			}
			code, resp := c.do("POST", "/v1/chat/completions", chatFileBody(block, stream), employeeAuth)
			assertOpenAIFileDenied(t, what, code, resp)
		}
	}
	for name, block := range anthropicFileBlocks() {
		for _, stream := range []bool{false, true} {
			what := name
			if stream {
				what += "（流式）"
			}
			for _, path := range []string{"/v1/messages", "/anthropic/v1/messages"} {
				code, resp := c.do("POST", path, anthropicFileBody(block, stream), employeeHeader)
				assertAnthropicFileDenied(t, path+" "+what, code, resp)
			}
		}
	}
	if got := up.chatCalls.Load(); got != 0 {
		t.Fatalf("被拦截的 chat 请求不应到上游，实际调用 %d 次", got)
	}
	if got := up.anthropicCalls.Load(); got != 0 {
		t.Fatalf("被拦截的 messages 请求不应到上游，实际调用 %d 次", got)
	}

	// ---- 没有真实文件引用的请求不受影响：普通文本、工具 arguments / 工具 input 里出现 file_id 字样 ----
	code, resp := c.do("POST", "/v1/chat/completions", map[string]any{
		"model":    "chat",
		"messages": []map[string]any{{"role": "user", "content": "ping"}},
	}, employeeAuth)
	expect(t, "普通 chat 请求", code, http.StatusOK, resp)
	code, resp = c.do("POST", "/v1/chat/completions", map[string]any{
		"model": "chat",
		"messages": []map[string]any{
			{"role": "assistant", "tool_calls": []map[string]any{{
				"id": "call_1", "type": "function",
				"function": map[string]any{"name": "lookup", "arguments": `{"file_id":"file_not_referenced"}`},
			}}},
			{"role": "tool", "tool_call_id": "call_1", "content": "ok"},
			{"role": "user", "content": "继续"},
		},
	}, employeeAuth)
	expect(t, "工具 arguments 里出现 file_id 字样的 chat 请求", code, http.StatusOK, resp)
	code, resp = c.do("POST", "/v1/messages", map[string]any{
		"model": "chat", "max_tokens": 32,
		"messages": []map[string]any{{"role": "user", "content": "ping"}},
	}, employeeHeader)
	expect(t, "普通 messages 请求", code, http.StatusOK, resp)
	// Anthropic 客户端工具 input 是工具参数，不是文件引用：原生透传路径应照常放行。
	code, resp = c.do("POST", "/v1/messages", map[string]any{
		"model": "chat", "max_tokens": 32,
		"messages": []map[string]any{
			{"role": "assistant", "content": []map[string]any{{
				"type": "tool_use", "id": "toolu_1", "name": "lookup",
				"input": map[string]any{"file_id": "file_not_referenced"},
			}}},
			{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"}}},
		},
	}, employeeHeader)
	expect(t, "工具 input 里出现 file_id 字样的 messages 请求", code, http.StatusOK, resp)
	if up.chatCalls.Load() != 2 || up.anthropicCalls.Load() != 2 {
		t.Fatalf("无 file_id 的请求应转发上游：chat=%d messages=%d", up.chatCalls.Load(), up.anthropicCalls.Load())
	}

	// ---- 管理员与机器人账号默认放行，文件块原样转发给上游 ----
	file := map[string]any{"type": "file", "file_id": "file_shared_1"}
	doc := map[string]any{"type": "document", "source": map[string]any{"type": "file", "file_id": "file_doc_1"}}
	for name, token := range map[string]string{"管理员": "ee_at_file_admin", "机器人账号": "ee_at_file_bot"} {
		chatBefore, msgBefore := up.chatCalls.Load(), up.anthropicCalls.Load()

		code, resp = c.do("POST", "/v1/chat/completions", chatFileBody(file, false), bearer(token))
		expect(t, name+"引用 file_id 的 chat 请求", code, http.StatusOK, resp)
		if !strings.Contains(string(up.lastChat.Load().([]byte)), "file_shared_1") {
			t.Fatalf("%s：file 块应原样转发上游：%s", name, up.lastChat.Load())
		}
		code, resp = c.do("POST", "/v1/chat/completions", chatFileBody(file, true), bearer(token))
		expect(t, name+"引用 file_id 的流式 chat 请求", code, http.StatusOK, resp)

		code, resp = c.do("POST", "/v1/messages", anthropicFileBody(doc, false), map[string]string{"X-Api-Key": token})
		expect(t, name+"引用 file_id 的非流式 messages 请求", code, http.StatusOK, resp)
		if !strings.Contains(string(up.lastAnthropic.Load().([]byte)), "file_doc_1") {
			t.Fatalf("%s：document 块应原样转发上游：%s", name, up.lastAnthropic.Load())
		}
		code, resp = c.do("POST", "/anthropic/v1/messages", anthropicFileBody(doc, true), map[string]string{"X-Api-Key": token})
		expect(t, name+"引用 file_id 的流式 messages 请求", code, http.StatusOK, resp)

		if up.chatCalls.Load() != chatBefore+2 || up.anthropicCalls.Load() != msgBefore+2 {
			t.Fatalf("%s：放行后请求应转发上游：chat=%d messages=%d", name, up.chatCalls.Load(), up.anthropicCalls.Load())
		}
	}

	// ---- 开关打开（同一份库、另一个 Server）：普通员工也被放行 ----
	switchTS := f.startServer(t, func(o *api.Options) { o.DeepSeekToolsAllowEmployee = true })
	switchClient := newClient(t, switchTS.URL)
	chatBefore := up.chatCalls.Load()
	code, resp = switchClient.do("POST", "/v1/chat/completions", chatFileBody(file, true), employeeAuth)
	expect(t, "开关打开后员工引用 file_id 的 chat 请求", code, http.StatusOK, resp)
	code, resp = switchClient.do("POST", "/v1/messages", anthropicFileBody(file, false), employeeHeader)
	expect(t, "开关打开后员工引用 file_id 的 messages 请求", code, http.StatusOK, resp)
	if up.chatCalls.Load() != chatBefore+1 {
		t.Fatalf("开关打开后员工请求应转发上游：chat=%d", up.chatCalls.Load())
	}
}
