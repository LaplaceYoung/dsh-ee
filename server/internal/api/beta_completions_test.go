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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DeepSeek FIM / Chat Prefix beta 透传（beta_completions.go）。
//
// fake 上游记录收到的路径、鉴权与请求体；测试用真实 HTTP 栈验证：模型名重写、流式
// 逐字节转发、usage 记账（含 FIM 缺 usage 的估算兜底）、鉴权/模型/额度拒绝、
// 无 DeepSeek 上游 503、上游错误、上游选择（默认优先级 / name / id）与限流。

const (
	betaUpstreamKey  = "sk-upstream-deepseek-main"
	betaFIMResponse  = `{"id":"fim-1","object":"text_completion","created":1730000000,"model":"deepseek-v4-flash","choices":[{"index":0,"text":"    return fib(a-1) + fib(a-2)","finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":9,"total_tokens":21}}`
	betaChatResponse = "{\"id\":\"chat-1\",\"object\":\"chat.completion\",\"created\":1730000001,\"model\":\"deepseek-v4-pro\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"print(1)\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":18,\"completion_tokens\":11,\"total_tokens\":29}}"
)

type betaRecordedRequest struct {
	path string
	auth string
	body map[string]any
	raw  []byte
}

type betaUpstreamRecorder struct {
	mu       sync.Mutex
	requests []betaRecordedRequest
}

func (rec *betaUpstreamRecorder) record(r *http.Request, raw []byte) {
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	rec.mu.Lock()
	rec.requests = append(rec.requests, betaRecordedRequest{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body, raw: raw})
	rec.mu.Unlock()
}

func (rec *betaUpstreamRecorder) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.requests)
}

func (rec *betaUpstreamRecorder) last(t *testing.T) betaRecordedRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.requests) == 0 {
		t.Fatal("fake 上游没有收到任何请求")
	}
	return rec.requests[len(rec.requests)-1]
}

// betaFixture 在 api.New 之前先建好上游，这样测试能拿到上游 id 去设置
// DSH_EE_DEEPSEEK_PROVIDER（name 或 id 两种写法都要覆盖）。
type betaFixture struct {
	*principalFixture
	t   *testing.T
	rec *betaUpstreamRecorder
	url string
	box *secret.Box
}

func newBetaFixture(t *testing.T, handle func(http.ResponseWriter, *http.Request, []byte), mutate func(*betaFixture, *api.Options)) *betaFixture {
	t.Helper()
	rec := &betaUpstreamRecorder{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.record(r, raw)
		handle(w, r, raw)
	}))
	t.Cleanup(fake.Close)

	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatalf("secret box: %v", err)
	}
	f := &betaFixture{t: t, rec: rec, url: fake.URL, box: box}
	f.principalFixture = &principalFixture{st: st}
	opts := api.Options{DataDir: t.TempDir(), Upstream: fake.Client()}
	if mutate != nil {
		mutate(f, &opts)
	}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	f.srv, f.ts, f.admin = srv, ts, newClient(t, ts.URL)
	code, body := f.admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	return f
}

// addProvider 直接写库建一个启用的上游；models 里的模型对外名与上游名相同。
func (f *betaFixture) addProvider(name, vendor, baseURL string, models []string) int64 {
	f.t.Helper()
	apiKey := "sk-upstream-" + name
	enc, err := f.box.Seal([]byte(apiKey))
	if err != nil {
		f.t.Fatalf("加密上游 key：%v", err)
	}
	hint := secret.Hint(apiKey)
	p := &store.Provider{Name: name, Vendor: vendor, BaseURL: baseURL, APIKeyEnc: enc, APIKeyHint: hint, Priority: 100, Enabled: true}
	if err := f.st.CreateProvider(context.Background(), p); err != nil {
		f.t.Fatalf("创建上游 %s：%v", name, err)
	}
	for _, m := range models {
		f.addModel(p.ID, m, m)
	}
	return p.ID
}

// addModel 给上游加一个模型：对外名 name，转发时换成 upstreamModel。
func (f *betaFixture) addModel(providerID int64, name, upstreamModel string) {
	f.t.Helper()
	if err := f.st.CreateModel(context.Background(), &store.Model{
		ProviderID: providerID, Name: name, UpstreamModel: upstreamModel, DisplayName: name, Enabled: true,
	}); err != nil {
		f.t.Fatalf("添加上游模型 %s → %s：%v", name, upstreamModel, err)
	}
}

// issueToken 建一个带 models scope 的员工和令牌。
// access_tokens.prefix 只有 24 个字符，令牌明文（协议里是 ee_at_ 前缀）据此截断。
func (f *betaFixture) issueToken(username string, userLimit *int64, models []string) (store.User, string) {
	f.t.Helper()
	u := f.user(f.t, username, userLimit)
	plain := "ee_at_" + strings.TrimPrefix(username, "beta-")
	if len(plain) > 24 {
		plain = plain[:24]
	}
	f.token(f.t, u.ID, plain, []string{rbac.ScopeModels}, nil, models)
	return u, plain
}

// doHTTP 发请求并保留响应头（流式、限流、错误壳测试要读头）。
func (c *client) doHTTP(t *testing.T, method, path string, body any, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

type betaUsageRow struct {
	model      string
	providerID int64
	prompt     int
	completion int
	total      int
	status     int
}

func lastBetaUsage(t *testing.T, f *betaFixture, userID int64) betaUsageRow {
	t.Helper()
	var row betaUsageRow
	err := f.st.DB().QueryRowContext(context.Background(),
		`SELECT model, provider_id, prompt_tokens, completion_tokens, total_tokens, status_code
		   FROM usage_logs WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).
		Scan(&row.model, &row.providerID, &row.prompt, &row.completion, &row.total, &row.status)
	if err != nil {
		t.Fatalf("读用量记录：%v", err)
	}
	return row
}

func betaErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("错误体不是 OpenAI 错误壳：%v；%s", err, body)
	}
	if resp.Error.Message == "" {
		t.Fatalf("错误体缺少 message：%s", body)
	}
	return resp.Error.Code
}

// TestBetaFIMCompletion 覆盖 FIM 非流式：路径/鉴权/模型名重写/原样响应/usage 记账。
func TestBetaFIMCompletion(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		if r.URL.Path != "/beta/completions" {
			t.Errorf("FIM 请求路径 %q，期望 /beta/completions", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+betaUpstreamKey {
			t.Errorf("上游收到的 Authorization = %q，期望 Bearer %s", got, betaUpstreamKey)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, betaFIMResponse)
	}, func(f *betaFixture, opts *api.Options) {
		providerID := f.addProvider("deepseek-main", "deepseek", f.url, nil)
		f.addModel(providerID, "deepseek-flash", "deepseek-v4-flash")
	})

	u, token := f.issueToken("beta-fim", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{
		"model": "deepseek-flash", "prompt": "def fib(a):",
		"suffix": "    return fib(a-1) + fib(a-2)", "max_tokens": 128,
	}, bearer(token))
	expect(t, "FIM 补全", code, 200, body)
	if string(body) != betaFIMResponse {
		t.Fatalf("FIM 响应没有原样转发：\n got %s\nwant %s", body, betaFIMResponse)
	}

	req := f.rec.last(t)
	if req.path != "/beta/completions" {
		t.Fatalf("上游路径 %q，期望 /beta/completions", req.path)
	}
	if req.auth != "Bearer "+betaUpstreamKey {
		t.Fatalf("上游鉴权 %q，期望 Bearer %s", req.auth, betaUpstreamKey)
	}
	if got := req.body["model"]; got != "deepseek-v4-flash" {
		t.Fatalf("model 没有重写成上游模型名：%v", got)
	}
	if got := req.body["prompt"]; got != "def fib(a):" {
		t.Fatalf("prompt 被改动：%v", got)
	}
	if got := req.body["suffix"]; got != "    return fib(a-1) + fib(a-2)" {
		t.Fatalf("suffix 被改动：%v", got)
	}
	if got, _ := req.body["max_tokens"].(float64); got != 128 {
		t.Fatalf("max_tokens 被改动：%v", req.body["max_tokens"])
	}
	if used, err := f.st.MonthTokensByUser(context.Background(), u.ID); err != nil || used != 21 {
		t.Fatalf("本月用量 %d，期望上游 usage 的 21（err=%v）", used, err)
	}
	row := lastBetaUsage(t, f, u.ID)
	if row.model != "deepseek-flash" || row.prompt != 12 || row.completion != 9 || row.total != 21 || row.status != 200 {
		t.Fatalf("用量记录不对：%+v", row)
	}
}

// TestBetaBaseURLWithBetaSuffix 兼容按 OpenAI SDK 的 beta base_url（…/beta）配置的上游。
func TestBetaBaseURLWithBetaSuffix(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		if r.URL.Path != "/beta/completions" {
			t.Errorf("base_url 已带 /beta 时仍应请求 /beta/completions，实际 %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, betaFIMResponse)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-beta-base", "deepseek", f.url+"/beta", []string{"deepseek-flash"})
	})
	_, token := f.issueToken("beta-base-url", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{
		"model": "deepseek-flash", "prompt": "def fib(a):",
	}, bearer(token))
	expect(t, "base_url 带 /beta 的 FIM 补全", code, 200, body)
}

// TestBetaFIMStreamRelay 覆盖 FIM 流式：逐字节转发、最后一个分块里的 usage 记账。
func TestBetaFIMStreamRelay(t *testing.T) {
	const stream = "data: {\"choices\":[{\"text\":\"func fib\"}]}\n\n" +
		"data: {\"choices\":[{\"text\":\"(a):\"}]}\n\n" +
		"data: {\"choices\":[{\"text\":\"\",\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":6,\"completion_tokens\":4,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n\n"
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range strings.SplitAfter(stream, "\n\n") {
			if line == "" {
				continue
			}
			_, _ = io.WriteString(w, line)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		}
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
	})
	u, token := f.issueToken("beta-fim-stream", nil, nil)
	resp, body := newClient(t, f.ts.URL).doHTTP(t, "POST", "/beta/completions", map[string]any{
		"model": "deepseek-flash", "prompt": "def fib(a):", "stream": true,
	}, bearer(token))
	if resp.StatusCode != 200 {
		t.Fatalf("流式 FIM 状态码 %d；%s", resp.StatusCode, body)
	}
	if string(body) != stream {
		t.Fatalf("流式响应没有逐字节转发：\n got %q\nwant %q", body, stream)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("流式 Content-Type = %q", ct)
	}
	req := f.rec.last(t)
	if req.path != "/beta/completions" || req.body["stream"] != true {
		t.Fatalf("流式请求没有原样转发：path=%q body=%v", req.path, req.body)
	}
	// FIM 文档保证最后一个分块自带 usage，不需要注入 stream_options。
	if _, ok := req.body["stream_options"]; ok {
		t.Fatalf("FIM 流式请求不应注入 stream_options：%v", req.body)
	}
	if used, err := f.st.MonthTokensByUser(context.Background(), u.ID); err != nil || used != 10 {
		t.Fatalf("流式用量 %d，期望 10（err=%v）", used, err)
	}
}

// TestBetaFIMUsageFallback 覆盖 FIM 缺 usage 时的估算兜底（非流式与流式各一次）。
func TestBetaFIMUsageFallback(t *testing.T) {
	t.Run("非流式", func(t *testing.T) {
		f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"fim-2","choices":[{"index":0,"text":"hello world","finish_reason":"stop"}]}`)
		}, func(f *betaFixture, opts *api.Options) {
			f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
		})
		u, token := f.issueToken("beta-fim-estimate", nil, nil)
		code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{
			"model": "deepseek-flash", "prompt": "def fib(a):",
		}, bearer(token))
		expect(t, "缺 usage 的 FIM 补全", code, 200, body)
		row := lastBetaUsage(t, f, u.ID)
		if row.prompt <= 0 || row.completion <= 0 || row.total != row.prompt+row.completion {
			t.Fatalf("缺 usage 时应按 choices[].text 估算并记账：%+v", row)
		}
	})

	t.Run("流式", func(t *testing.T) {
		const stream = "data: {\"choices\":[{\"text\":\"hello\"}]}\n\n" +
			"data: {\"choices\":[{\"text\":\" world\"}]}\n\n"
		f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, stream)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		}, func(f *betaFixture, opts *api.Options) {
			f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
		})
		u, token := f.issueToken("beta-fim-stream-estimate", nil, nil)
		resp, body := newClient(t, f.ts.URL).doHTTP(t, "POST", "/beta/completions", map[string]any{
			"model": "deepseek-flash", "prompt": "def fib(a):", "stream": true,
		}, bearer(token))
		if resp.StatusCode != 200 || string(body) != stream {
			t.Fatalf("流式 FIM 转发不对：%d %q", resp.StatusCode, body)
		}
		row := lastBetaUsage(t, f, u.ID)
		if row.prompt <= 0 || row.completion <= 0 || row.total != row.prompt+row.completion {
			t.Fatalf("流式缺 usage 时应按 choices[].text 估算并记账：%+v", row)
		}
	})
}

// TestBetaChatPrefixCompletion 覆盖 Chat Prefix 非流式：最后一条 assistant prefix:true 原样转发。
func TestBetaChatPrefixCompletion(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		if r.URL.Path != "/beta/chat/completions" {
			t.Errorf("Chat Prefix 请求路径 %q，期望 /beta/chat/completions", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+betaUpstreamKey {
			t.Errorf("上游收到的 Authorization = %q", got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, betaChatResponse)
	}, func(f *betaFixture, opts *api.Options) {
		providerID := f.addProvider("deepseek-main", "deepseek", f.url, nil)
		f.addModel(providerID, "deepseek-pro", "deepseek-v4-pro")
	})
	u, token := f.issueToken("beta-prefix", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/chat/completions", map[string]any{
		"model": "deepseek-pro",
		"messages": []map[string]any{
			{"role": "user", "content": "请写一段快速排序代码"},
			{"role": "assistant", "content": "```python\n", "prefix": true},
		},
		"stop": []string{"```"},
	}, bearer(token))
	expect(t, "Chat Prefix 续写", code, 200, body)
	if string(body) != betaChatResponse {
		t.Fatalf("Chat Prefix 响应没有原样转发：%s", body)
	}
	req := f.rec.last(t)
	if req.path != "/beta/chat/completions" {
		t.Fatalf("上游路径 %q，期望 /beta/chat/completions", req.path)
	}
	if got := req.body["model"]; got != "deepseek-v4-pro" {
		t.Fatalf("model 没有重写成上游模型名：%v", got)
	}
	msgs, _ := req.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages 被改动：%v", req.body["messages"])
	}
	last, _ := msgs[1].(map[string]any)
	if last["role"] != "assistant" || last["prefix"] != true {
		t.Fatalf("最后一条 assistant 的 prefix 没有原样转发：%v", last)
	}
	if used, err := f.st.MonthTokensByUser(context.Background(), u.ID); err != nil || used != 29 {
		t.Fatalf("本月用量 %d，期望 29（err=%v）", used, err)
	}
	if row := lastBetaUsage(t, f, u.ID); row.model != "deepseek-pro" || row.status != 200 {
		t.Fatalf("用量记录不对：%+v", row)
	}
}

// TestBetaChatPrefixStream 覆盖 Chat Prefix 流式：请求注入 include_usage，响应逐字节转发并记账。
func TestBetaChatPrefixStream(t *testing.T) {
	const stream = "data: {\"choices\":[{\"delta\":{\"content\":\"print\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"(1)\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n\n"
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		opts, _ := body["stream_options"].(map[string]any)
		if opts["include_usage"] != true {
			t.Errorf("Chat Prefix 流式请求应注入 stream_options.include_usage=true：%s", raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range strings.SplitAfter(stream, "\n\n") {
			if line == "" {
				continue
			}
			_, _ = io.WriteString(w, line)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		}
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-pro"})
	})
	u, token := f.issueToken("beta-prefix-stream", nil, nil)
	resp, body := newClient(t, f.ts.URL).doHTTP(t, "POST", "/beta/chat/completions", map[string]any{
		"model": "deepseek-pro", "stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": "请写一段代码"},
			{"role": "assistant", "content": "```python\n", "prefix": true},
		},
	}, bearer(token))
	if resp.StatusCode != 200 || string(body) != stream {
		t.Fatalf("Chat Prefix 流式转发不对：%d %q", resp.StatusCode, body)
	}
	if used, err := f.st.MonthTokensByUser(context.Background(), u.ID); err != nil || used != 10 {
		t.Fatalf("流式用量 %d，期望 10（err=%v）", used, err)
	}
}

// TestBetaRequestValidation 覆盖两条路由自己的形状校验：不合法时不打上游。
func TestBetaRequestValidation(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		t.Errorf("请求形状不合法时不应转发到上游：%s", raw)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
	})
	_, token := f.issueToken("beta-validation", nil, nil)
	c := newClient(t, f.ts.URL)

	cases := []struct {
		name string
		path string
		body map[string]any
		code string
	}{
		{
			name: "FIM 缺 prompt", path: "/beta/completions", code: "prompt_required",
			body: map[string]any{"model": "deepseek-flash"},
		},
		{
			name: "FIM prompt 不是字符串", path: "/beta/completions", code: "invalid_prompt",
			body: map[string]any{"model": "deepseek-flash", "prompt": 1},
		},
		{
			name: "FIM max_tokens 为 0", path: "/beta/completions", code: "invalid_max_tokens",
			body: map[string]any{"model": "deepseek-flash", "prompt": "x", "max_tokens": 0},
		},
		{
			name: "前缀续写缺 messages", path: "/beta/chat/completions", code: "messages_required",
			body: map[string]any{"model": "deepseek-flash"},
		},
		{
			name: "前缀续写最后一条不是 assistant", path: "/beta/chat/completions", code: "prefix_required",
			body: map[string]any{"model": "deepseek-flash", "messages": []map[string]any{{"role": "user", "content": "hi"}}},
		},
		{
			name: "前缀续写没有 prefix", path: "/beta/chat/completions", code: "prefix_required",
			body: map[string]any{"model": "deepseek-flash", "messages": []map[string]any{
				{"role": "user", "content": "hi"}, {"role": "assistant", "content": "x"},
			}},
		},
		{
			name: "缺 model", path: "/beta/completions", code: "model_required",
			body: map[string]any{"prompt": "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := c.do("POST", tc.path, tc.body, bearer(token))
			expect(t, tc.name, code, 400, body)
			if got := betaErrorCode(t, body); got != tc.code {
				t.Fatalf("错误码 %q，期望 %q；%s", got, tc.code, body)
			}
		})
	}
	if n := f.rec.count(); n != 0 {
		t.Fatalf("校验失败时上游收到了 %d 个请求，期望 0", n)
	}
}

// TestBetaAdmissionControl 覆盖复用 /v1 鉴权：缺令牌、缺 models scope、模型被拒、额度用完。
func TestBetaAdmissionControl(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, betaFIMResponse)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
	})
	c := newClient(t, f.ts.URL)
	call := func(token, prompt string) (int, []byte) {
		header := map[string]string{}
		if token != "" {
			header = bearer(token)
		}
		return c.do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": prompt}, header)
	}

	code, body := call("", "x")
	expect(t, "缺令牌", code, 401, body)
	if got := betaErrorCode(t, body); got != "token_invalid" {
		t.Fatalf("缺令牌错误码 %q", got)
	}

	noScope := f.user(t, "beta-no-scope", nil)
	f.token(t, noScope.ID, "ee_at_beta_no_scope", []string{rbac.ScopeHub}, nil, nil)
	code, body = call("ee_at_beta_no_scope", "x")
	expect(t, "没有 models scope", code, 403, body)
	if got := betaErrorCode(t, body); got != "permission_denied" {
		t.Fatalf("没有 models scope 错误码 %q", got)
	}

	_, deniedModelToken := f.issueToken("beta-model-denied", nil, []string{"other"})
	code, body = call(deniedModelToken, "x")
	expect(t, "模型不在令牌允许列表", code, 403, body)
	if got := betaErrorCode(t, body); got != "model_not_allowed" {
		t.Fatalf("模型被拒错误码 %q", got)
	}

	zero := int64(0)
	_, noAllowanceToken := f.issueToken("beta-no-allowance", &zero, nil)
	code, body = call(noAllowanceToken, "x")
	expect(t, "没有额度", code, 429, body)
	if got := betaErrorCode(t, body); got != "monthly_quota_exceeded" {
		t.Fatalf("没有额度错误码 %q", got)
	}

	if n := f.rec.count(); n != 0 {
		t.Fatalf("准入被拒时上游收到了 %d 个请求，期望 0", n)
	}
}

// TestBetaModelNotOnDeepSeekProvider 覆盖模型只挂在别的厂商上游：不能借用，返回 404。
func TestBetaModelNotOnDeepSeekProvider(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		t.Errorf("模型没有配在 DeepSeek 上游时不应转发：%s", raw)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
		f.addProvider("custom-gateway", "custom", f.url, []string{"chat"})
	})
	_, token := f.issueToken("beta-model-miss", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{"model": "chat", "prompt": "hi"}, bearer(token))
	expect(t, "模型不在 DeepSeek 上游", code, 404, body)
	if got := betaErrorCode(t, body); got != "model_not_found" {
		t.Fatalf("错误码 %q，期望 model_not_found", got)
	}
	if n := f.rec.count(); n != 0 {
		t.Fatalf("上游收到了 %d 个请求，期望 0", n)
	}
}

// TestBetaNoProvider 覆盖没有任何 vendor=deepseek 上游：503，且不借用其他厂商上游。
func TestBetaNoProvider(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		t.Errorf("没有 DeepSeek 上游时不应转发：%s", raw)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("custom-gateway", "custom", f.url, []string{"deepseek-flash"})
	})
	_, token := f.issueToken("beta-no-provider", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "hi"}, bearer(token))
	expect(t, "没有 DeepSeek 上游", code, 503, body)
	if got := betaErrorCode(t, body); got != "beta_unavailable" {
		t.Fatalf("错误码 %q，期望 beta_unavailable", got)
	}
	if n := f.rec.count(); n != 0 {
		t.Fatalf("上游收到了 %d 个请求，期望 0", n)
	}
}

// TestBetaProviderSelection 覆盖默认取优先级最高的 DeepSeek 上游，以及 DSH_EE_DEEPSEEK_PROVIDER 的 name/id 指定。
func TestBetaProviderSelection(t *testing.T) {
	// 两个上游的对外模型名相同，但上游模型名不同 → 从转发体判断选中的是哪条线路。
	newSelectionFixture := func(t *testing.T, selector func(firstID, secondID int64) string) *betaFixture {
		t.Helper()
		return newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, betaFIMResponse)
		}, func(f *betaFixture, opts *api.Options) {
			firstID := f.addProvider("deepseek-first", "deepseek", f.url, nil)
			f.addModel(firstID, "deepseek-flash", "upstream-first")
			secondID := f.addProvider("deepseek-second", "deepseek", f.url, nil)
			f.addModel(secondID, "deepseek-flash", "upstream-second")
			if selector != nil {
				opts.DeepSeekBetaProvider = selector(firstID, secondID)
			}
		})
	}
	request := func(t *testing.T, f *betaFixture) string {
		t.Helper()
		_, token := f.issueToken("beta-sel-"+strconv.FormatInt(time.Now().UnixNano()%1e8, 36), nil, nil)
		code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{
			"model": "deepseek-flash", "prompt": "hi",
		}, bearer(token))
		expect(t, "选择上游", code, 200, body)
		req := f.rec.last(t)
		model, _ := req.body["model"].(string)
		return model
	}

	t.Run("默认按优先级与 id 取第一条", func(t *testing.T) {
		f := newSelectionFixture(t, nil)
		if got := request(t, f); got != "upstream-first" {
			t.Fatalf("默认应使用优先级最高的 deepseek-first，实际模型 %q", got)
		}
	})
	t.Run("按 name 指定", func(t *testing.T) {
		f := newSelectionFixture(t, func(int64, int64) string { return "deepseek-second" })
		if got := request(t, f); got != "upstream-second" {
			t.Fatalf("DSH_EE_DEEPSEEK_PROVIDER=deepseek-second 应选中第二条，实际模型 %q", got)
		}
	})
	t.Run("按 id 指定", func(t *testing.T) {
		f := newSelectionFixture(t, func(_, secondID int64) string { return strconv.FormatInt(secondID, 10) })
		if got := request(t, f); got != "upstream-second" {
			t.Fatalf("DSH_EE_DEEPSEEK_PROVIDER=上游 id 应选中第二条，实际模型 %q", got)
		}
	})
	t.Run("配置指向不存在的上游", func(t *testing.T) {
		f := newSelectionFixture(t, func(int64, int64) string { return "nope" })
		_, token := f.issueToken("beta-selector-missing", nil, nil)
		code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "hi"}, bearer(token))
		expect(t, "选择不存在的上游", code, 503, body)
		if got := betaErrorCode(t, body); got != "beta_unavailable" {
			t.Fatalf("错误码 %q，期望 beta_unavailable", got)
		}
		if n := f.rec.count(); n != 0 {
			t.Fatalf("上游收到了 %d 个请求，期望 0", n)
		}
	})
	t.Run("配置指向非 DeepSeek 上游", func(t *testing.T) {
		f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
			t.Errorf("配置指向非 DeepSeek 上游时不应转发：%s", raw)
		}, func(f *betaFixture, opts *api.Options) {
			f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
			f.addProvider("custom-gateway", "custom", f.url, []string{"deepseek-flash"})
			opts.DeepSeekBetaProvider = "custom-gateway"
		})
		_, token := f.issueToken("beta-selector-custom", nil, nil)
		code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "hi"}, bearer(token))
		expect(t, "配置指向非 DeepSeek 上游", code, 503, body)
		if got := betaErrorCode(t, body); got != "beta_unavailable" {
			t.Fatalf("错误码 %q，期望 beta_unavailable", got)
		}
	})
}

// TestBetaUpstreamErrors 覆盖上游错误：OpenAI JSON 错误原样转发；非 JSON 错误包成 OpenAI 壳；
// 不做跨上游故障转移，用量仍按失败状态记账。
func TestBetaUpstreamErrors(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["prompt"] == "html" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "<html>bad gateway</html>")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream boom","type":"server_error","code":"internal"}}`)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
	})
	u, token := f.issueToken("beta-upstream-error", nil, nil)
	c := newClient(t, f.ts.URL)

	code, body := c.do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "json"}, bearer(token))
	expect(t, "上游 500", code, 500, body)
	var upstreamErr struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &upstreamErr); err != nil || upstreamErr.Error.Message != "upstream boom" || upstreamErr.Error.Code != "internal" {
		t.Fatalf("上游 JSON 错误应原样转发：%s", body)
	}
	if row := lastBetaUsage(t, f, u.ID); row.status != 500 {
		t.Fatalf("上游错误的用量记录状态码 %d，期望 500", row.status)
	}

	code, body = c.do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "html"}, bearer(token))
	expect(t, "上游非 JSON 错误", code, 502, body)
	if got := betaErrorCode(t, body); got != "upstream_error" {
		t.Fatalf("非 JSON 错误应包成 OpenAI 壳，错误码 %q；%s", got, body)
	}
	if row := lastBetaUsage(t, f, u.ID); row.status != 502 {
		t.Fatalf("上游错误的用量记录状态码 %d，期望 502", row.status)
	}
	if n := f.rec.count(); n != 2 {
		t.Fatalf("两次失败共应只打上游 2 次（不故障转移），实际 %d", n)
	}
}

// TestBetaUpstreamUnreachable 覆盖上游不可达：502 + OpenAI 错误壳。
func TestBetaUpstreamUnreachable(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		t.Error("上游不可达的用例不应有请求到达 fake 上游")
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-down", "deepseek", "http://127.0.0.1:1", []string{"deepseek-flash"})
	})
	_, token := f.issueToken("beta-unreachable", nil, nil)
	code, body := newClient(t, f.ts.URL).do("POST", "/beta/completions", map[string]any{"model": "deepseek-flash", "prompt": "hi"}, bearer(token))
	expect(t, "上游不可达", code, 502, body)
	if got := betaErrorCode(t, body); got != "upstream_unreachable" {
		t.Fatalf("错误码 %q，期望 upstream_unreachable", got)
	}
}

// TestBetaRateLimit 覆盖复用公开协议面的每令牌限流。
func TestBetaRateLimit(t *testing.T) {
	f := newBetaFixture(t, func(w http.ResponseWriter, r *http.Request, raw []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, betaFIMResponse)
	}, func(f *betaFixture, opts *api.Options) {
		f.addProvider("deepseek-main", "deepseek", f.url, []string{"deepseek-flash"})
		opts.RateLimitPerMinute = 1
		opts.RateLimitWindow = time.Minute
	})
	_, token := f.issueToken("beta-rate-limit", nil, nil)
	c := newClient(t, f.ts.URL)
	body := map[string]any{"model": "deepseek-flash", "prompt": "hi"}

	code, resp := c.do("POST", "/beta/completions", body, bearer(token))
	expect(t, "限流窗口内第一次请求", code, 200, resp)
	code, resp = c.do("POST", "/beta/completions", body, bearer(token))
	expect(t, "限流窗口内第二次请求", code, 429, resp)
	if got := betaErrorCode(t, resp); got != "rate_limit_exceeded" {
		t.Fatalf("限流错误码 %q，期望 rate_limit_exceeded", got)
	}
	// 第三次带上响应头，确认 Retry-After 与 X-RateLimit-* 都在。
	respH, _ := c.doHTTP(t, "POST", "/beta/completions", body, bearer(token))
	if respH.StatusCode != 429 || respH.Header.Get("Retry-After") == "" || respH.Header.Get("X-RateLimit-Limit") != "1" {
		t.Fatalf("429 应带 Retry-After / X-RateLimit-*：%d %v", respH.StatusCode, respH.Header)
	}
	if n := f.rec.count(); n != 1 {
		t.Fatalf("只有第一次请求应到达上游，实际 %d 次", n)
	}
}
