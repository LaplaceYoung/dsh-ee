package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// rateLimitFixture 起一个只含协议面假处理函数的 Server，用来验证中间件本身：
// 不需要数据库，也不依赖真实令牌，Authorization 里随便放一个字符串就是一个「令牌」。
func rateLimitFixture(t *testing.T, opts Options) *httptest.Server {
	t.Helper()
	s := &Server{opts: opts, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if opts.RateLimitPerMinute > 0 {
		s.tokenRate = newTokenRate(opts.RateLimitPerMinute, opts.RateLimitWindow)
	}
	ok := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
	s.routeTable = []route{
		{pattern: "GET /healthz", h: ok},
		{pattern: "GET /v1/models", h: ok},
		{pattern: "POST /mcp", h: ok},
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

type rateResponse struct {
	code   int
	header http.Header
	body   []byte
}

func doRateRequest(t *testing.T, base, method, path, token string, header map[string]string) rateResponse {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rateResponse{code: resp.StatusCode, header: resp.Header, body: body}
}

func wantHeader(t *testing.T, h http.Header, name, want string) {
	t.Helper()
	if got := h.Get(name); got != want {
		t.Fatalf("%s：得到 %q，期望 %q", name, got, want)
	}
}

func wantNoHeader(t *testing.T, h http.Header, name string) {
	t.Helper()
	if got := h.Get(name); got != "" {
		t.Fatalf("不该有 %s：%q", name, got)
	}
}

func resetUnix(t *testing.T, h http.Header) int64 {
	t.Helper()
	raw := h.Get("X-RateLimit-Reset")
	if raw == "" {
		t.Fatal("缺少 X-RateLimit-Reset")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("X-RateLimit-Reset 不是 Unix 秒：%q", raw)
	}
	return v
}

func retryAfter(t *testing.T, h http.Header) int {
	t.Helper()
	raw := h.Get("Retry-After")
	if raw == "" {
		t.Fatal("429 响应缺少 Retry-After")
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		t.Fatalf("Retry-After 应是不小于 1 的秒数：%q", raw)
	}
	return v
}

func TestRateLimitV1HitAndHeaders(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 2, RateLimitWindow: time.Minute})
	now := time.Now()

	first := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_a", nil)
	if first.code != http.StatusOK {
		t.Fatalf("第一次 /v1/models：%d %s", first.code, first.body)
	}
	wantHeader(t, first.header, "X-RateLimit-Limit", "2")
	wantHeader(t, first.header, "X-RateLimit-Remaining", "1")
	wantNoHeader(t, first.header, "Retry-After")
	if reset := resetUnix(t, first.header); reset <= now.Unix() || reset > now.Add(61*time.Second).Unix() {
		t.Fatalf("X-RateLimit-Reset 不在本窗口内：%d", reset)
	}

	second := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_a", nil)
	if second.code != http.StatusOK {
		t.Fatalf("第二次 /v1/models：%d %s", second.code, second.body)
	}
	wantHeader(t, second.header, "X-RateLimit-Remaining", "0")

	third := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_a", nil)
	if third.code != http.StatusTooManyRequests {
		t.Fatalf("超过额度应 429：%d %s", third.code, third.body)
	}
	wantHeader(t, third.header, "X-RateLimit-Limit", "2")
	wantHeader(t, third.header, "X-RateLimit-Remaining", "0")
	if reset := resetUnix(t, third.header); reset <= now.Unix() {
		t.Fatalf("429 的 X-RateLimit-Reset 应该是将来的 Unix 秒：%d", reset)
	}
	retryAfter(t, third.header)
	if ct := third.header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("429 的 Content-Type 不对：%q", ct)
	}

	// 错误体沿用 OpenAI 的壳：error.message / error.type / error.code
	var openaiErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(third.body, &openaiErr); err != nil {
		t.Fatalf("429 不是 JSON：%v %s", err, third.body)
	}
	if openaiErr.Error.Type != "rate_limit_error" || openaiErr.Error.Code != "rate_limit_exceeded" || openaiErr.Error.Message == "" {
		t.Fatalf("429 的 OpenAI 错误壳不对：%s", third.body)
	}
	if _, ok := third.header["X-Request-Id"]; !ok {
		t.Fatal("429 响应也应带 X-Request-Id")
	}
}

func TestRateLimitMCPHitAndHeaders(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})

	first := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_rate_mcp", nil)
	if first.code != http.StatusOK {
		t.Fatalf("第一次 /mcp：%d %s", first.code, first.body)
	}
	wantHeader(t, first.header, "X-RateLimit-Limit", "1")
	wantHeader(t, first.header, "X-RateLimit-Remaining", "0")
	wantNoHeader(t, first.header, "Retry-After")

	second := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_rate_mcp", nil)
	if second.code != http.StatusTooManyRequests {
		t.Fatalf("超过额度应 429：%d %s", second.code, second.body)
	}
	wantHeader(t, second.header, "X-RateLimit-Remaining", "0")
	retryAfter(t, second.header)

	// 错误体保持 JSON-RPC 形状，只把超限映射成服务端错误码
	var rpcErr struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(second.body, &rpcErr); err != nil {
		t.Fatalf("429 不是 JSON-RPC：%v %s", err, second.body)
	}
	if rpcErr.JSONRPC != "2.0" || rpcErr.ID != nil || rpcErr.Error.Code != -32000 || rpcErr.Error.Message == "" {
		t.Fatalf("429 的 JSON-RPC 错误体不对：%s", second.body)
	}
}

func TestRateLimitSeparateTokens(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})

	if r := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_token_a", nil); r.code != http.StatusOK {
		t.Fatalf("令牌 A 第一次：%d %s", r.code, r.body)
	}
	// A 的额度在 /v1 与 /mcp 之间共用
	if r := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_token_a", nil); r.code != http.StatusTooManyRequests {
		t.Fatalf("令牌 A 在 /mcp 应继续受限：%d %s", r.code, r.body)
	}
	// B 有自己的额度，不受 A 影响
	if r := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_token_b", nil); r.code != http.StatusOK {
		t.Fatalf("令牌 B 不该被 A 用掉的额度影响：%d %s", r.code, r.body)
	}
	if r := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_token_b", nil); r.code != http.StatusTooManyRequests {
		t.Fatalf("令牌 B 自己超限应 429：%d %s", r.code, r.body)
	}
}

func TestRateLimitWindowResets(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: 150 * time.Millisecond})

	first := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_reset", nil)
	if first.code != http.StatusOK {
		t.Fatalf("第一次：%d %s", first.code, first.body)
	}
	if r := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_reset", nil); r.code != http.StatusTooManyRequests {
		t.Fatalf("同一窗口第二次应 429：%d %s", r.code, r.body)
	}
	time.Sleep(250 * time.Millisecond)
	again := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_rate_reset", nil)
	if again.code != http.StatusOK {
		t.Fatalf("窗口过后应恢复：%d %s", again.code, again.body)
	}
	wantHeader(t, again.header, "X-RateLimit-Remaining", "0")
}

func TestRateLimitDisabled(t *testing.T) {
	ts := rateLimitFixture(t, Options{})
	for i := 0; i < 5; i++ {
		r := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_no_limit", nil)
		if r.code != http.StatusOK {
			t.Fatalf("关闭限流后不该 429：第 %d 次 %d %s", i+1, r.code, r.body)
		}
		wantNoHeader(t, r.header, "X-RateLimit-Limit")
	}
	for i := 0; i < 5; i++ {
		if r := doRateRequest(t, ts.URL, "POST", "/mcp", "ee_at_no_limit", nil); r.code != http.StatusOK {
			t.Fatalf("关闭限流后 /mcp 不该 429：第 %d 次 %d %s", i+1, r.code, r.body)
		}
	}
}

func TestRateLimitOnlyProtocolPaths(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})
	for i := 0; i < 3; i++ {
		r := doRateRequest(t, ts.URL, "GET", "/healthz", "ee_at_token_a", nil)
		if r.code != http.StatusOK {
			t.Fatalf("/healthz 不该被限流：第 %d 次 %d %s", i+1, r.code, r.body)
		}
		wantNoHeader(t, r.header, "X-RateLimit-Limit")
	}
	for i := 0; i < 3; i++ {
		r := doRateRequest(t, ts.URL, "GET", "/api/没有这个接口", "ee_at_token_a", nil)
		if r.code != http.StatusNotFound {
			t.Fatalf("不存在的接口应 404：第 %d 次 %d", i+1, r.code)
		}
		wantNoHeader(t, r.header, "X-RateLimit-Limit")
	}
}

func TestRequestIDGeneratedAndPassthrough(t *testing.T) {
	ts := rateLimitFixture(t, Options{})

	first := doRateRequest(t, ts.URL, "GET", "/healthz", "", nil)
	if id := first.header.Get("X-Request-Id"); len(id) != 32 {
		t.Fatalf("应生成 32 位十六进制 X-Request-Id：%q", id)
	}
	second := doRateRequest(t, ts.URL, "GET", "/healthz", "", nil)
	if a, b := first.header.Get("X-Request-Id"), second.header.Get("X-Request-Id"); a == b {
		t.Fatalf("两个请求的 X-Request-Id 不该相同：%q", a)
	}

	const trace = "trace-abc.123"
	passed := doRateRequest(t, ts.URL, "GET", "/healthz", "", map[string]string{"X-Request-Id": trace})
	wantHeader(t, passed.header, "X-Request-Id", trace)
}

func TestRequestIDEchoedOnRateLimitedResponse(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})
	const trace = "client-trace-9"
	if r := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_token_a", map[string]string{"X-Request-Id": trace}); r.code != http.StatusOK {
		t.Fatalf("第一次：%d %s", r.code, r.body)
	}
	limited := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_token_a", map[string]string{"X-Request-Id": trace})
	if limited.code != http.StatusTooManyRequests {
		t.Fatalf("第二次应 429：%d", limited.code)
	}
	wantHeader(t, limited.header, "X-Request-Id", trace)
}

func TestRateLimitIgnoresCORSPreflight(t *testing.T) {
	ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})
	// 预检（OPTIONS）不占额度：即使先来很多次，后面的真实请求仍然可用。
	for i := 0; i < 5; i++ {
		r := doRateRequest(t, ts.URL, "OPTIONS", "/v1/models", "ee_at_token_a", nil)
		if r.code == http.StatusTooManyRequests {
			t.Fatalf("OPTIONS 不该被限流：第 %d 次", i+1)
		}
		wantNoHeader(t, r.header, "X-RateLimit-Limit")
	}
	first := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_token_a", nil)
	if first.code != http.StatusOK {
		t.Fatalf("预检不该占用额度：%d %s", first.code, first.body)
	}
	if second := doRateRequest(t, ts.URL, "GET", "/v1/models", "ee_at_token_a", nil); second.code != http.StatusTooManyRequests {
		t.Fatalf("预检之后真实请求仍应受限：%d %s", second.code, second.body)
	}
}

func TestTokenRateBoundedKeyCount(t *testing.T) {
	l := newTokenRate(60, time.Minute)
	for i := 0; i < maxRateKeys+100; i++ {
		l.take("token-" + strconv.Itoa(i))
	}
	if len(l.items) > maxRateKeys+1 {
		t.Fatalf("限流器的 key 数应有上限：%d", len(l.items))
	}
	// 填满之后的陌生 key 进溢出桶，仍受限流约束
	for i := 0; i < 61; i++ {
		if _, ok := l.take("another-token-" + strconv.Itoa(i)); ok {
			t.Fatalf("溢出桶超过额度后不应再放行（第 %d 次）", i+1)
		}
	}
}

func TestValidRequestID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"trace-abc.123", "trace-abc.123"},
		{"  padded  ", "padded"},
		{"", ""},
		{"   ", ""},
		{"bad id", ""},
		{"bad\nid", ""},
		{"中文", ""},
		{strings.Repeat("a", 128), strings.Repeat("a", 128)},
		{strings.Repeat("a", 129), ""},
	}
	for _, c := range cases {
		if got := validRequestID(c.in); got != c.want {
			t.Fatalf("validRequestID(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 共享限流的 key 上限：每窗口最多 maxRateKeys 个 key，超出的陌生 key 进同一个溢出 key，
// 已有的 key 继续用自己的 key；窗口翻页后重新计数。
func TestRateKeyAdmissionBounded(t *testing.T) {
	var a rateKeyAdmission
	start := time.Now().UTC().Truncate(time.Minute)
	for i := 0; i < maxRateKeys; i++ {
		key := "token-" + strconv.Itoa(i)
		if got := a.admit(key, start); got != key {
			t.Fatalf("第 %d 个 key 应原样返回：%q", i+1, got)
		}
	}
	if got := a.admit("token-new", start); got != sharedRateOverflowKey {
		t.Fatalf("超过上限后的陌生 key 应进溢出 key，得到 %q", got)
	}
	if got := a.admit("token-5", start); got != "token-5" {
		t.Fatalf("已有的 key 应继续用自己的 key，得到 %q", got)
	}
	if len(a.keys) > maxRateKeys {
		t.Fatalf("key 集合应有上限：%d", len(a.keys))
	}
	// 窗口翻页后集合重置，可以重新登记新 key。
	next := start.Add(time.Minute)
	if got := a.admit("token-new", next); got != "token-new" {
		t.Fatalf("窗口翻页后应重新登记：%q", got)
	}
}
