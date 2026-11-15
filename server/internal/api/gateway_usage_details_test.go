package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// newUsageDetailsUpstream 模拟会返回用量细项的上游。用请求体里的 metadata.variant
// 选择返回哪种格式，覆盖 OpenAI 嵌套细项、DeepSeek 顶层缓存字段、旧响应与流式。

type usageDetailsBodies struct {
	openai   string
	deepseek string
	legacy   string
	stream   string
}

func newUsageDetailsUpstream(t *testing.T) (*httptest.Server, usageDetailsBodies) {
	t.Helper()
	bodies := usageDetailsBodies{
		openai:   `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"答"}}],"usage":{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140,"prompt_tokens_details":{"cached_tokens":64},"completion_tokens_details":{"reasoning_tokens":20}}}`,
		deepseek: `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"答"}}],"usage":{"prompt_tokens":200,"completion_tokens":50,"total_tokens":250,"prompt_cache_hit_tokens":128,"prompt_cache_miss_tokens":72,"completion_tokens_details":{"reasoning_tokens":12}}}`,
		legacy:   `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"答"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}
	streamUsage := `{"prompt_tokens":300,"completion_tokens":60,"total_tokens":360,"prompt_cache_hit_tokens":256,"prompt_cache_miss_tokens":44,"completion_tokens_details":{"reasoning_tokens":30}}`
	bodies.stream = "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":" + streamUsage + "}\n\n" +
		"data: [DONE]\n\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamSecret {
			t.Errorf("上游收到的 Authorization 不对：%q", r.Header.Get("Authorization"))
			w.WriteHeader(401)
			return
		}
		var body struct {
			Stream   bool `json:"stream"`
			Metadata struct {
				Variant string `json:"variant"`
			} `json:"metadata"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			switch body.Metadata.Variant {
			case "openai":
				_, _ = io.WriteString(w, bodies.openai)
			case "deepseek":
				_, _ = io.WriteString(w, bodies.deepseek)
			default:
				_, _ = io.WriteString(w, bodies.legacy)
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if body.Metadata.Variant == "cutoff" {
			// 模拟客户端停止 / 上游掐断：只有增量，没有最后一块 usage。
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
			w.(http.Flusher).Flush()
			return
		}
		_, _ = io.WriteString(w, bodies.stream)
		w.(http.Flusher).Flush()
	}))
	t.Cleanup(ts.Close)
	return ts, bodies
}

// lastUsageRow 取最新一条 usage_logs，返回记账的细项列。
func lastUsageRow(t *testing.T, f *principalFixture) (prompt, completion, total, reasoning, cached, status int) {
	t.Helper()
	err := f.st.DB().QueryRowContext(context.Background(),
		`SELECT prompt_tokens, completion_tokens, total_tokens, reasoning_tokens, cached_tokens, status_code
		 FROM usage_logs ORDER BY id DESC LIMIT 1`).Scan(&prompt, &completion, &total, &reasoning, &cached, &status)
	if err != nil {
		t.Fatalf("查 usage_logs: %v", err)
	}
	return
}

// 网关把上游 usage 的细项写进 usage_logs：非流式 OpenAI 嵌套格式 / DeepSeek 顶层缓存字段 /
// 旧响应都兼容；响应 JSON 原样转发，不改形状。
func TestGatewayRecordsUsageDetails(t *testing.T) {
	f := newPrincipalFixture(t)
	up, bodies := newUsageDetailsUpstream(t)
	ctx := context.Background()
	if _, err := f.st.DB().ExecContext(ctx, "UPDATE providers SET base_url = ?", up.URL); err != nil {
		t.Fatal(err)
	}
	u := f.user(t, "usage-details", nil)
	f.token(t, u.ID, "ee_at_usage_details", []string{rbac.ScopeModels}, nil, nil)
	c := newClient(t, f.ts.URL)
	bearer := bearer("ee_at_usage_details")

	chat := func(variant string, stream bool) (int, []byte) {
		return c.do("POST", "/v1/chat/completions", map[string]any{
			"model":    "chat",
			"stream":   stream,
			"metadata": map[string]string{"variant": variant},
			"messages": []map[string]string{{"role": "user", "content": "你好"}},
		}, bearer)
	}

	// 非流式：OpenAI 的 *_tokens_details 细项。
	code, body := chat("openai", false)
	expect(t, "非流式 OpenAI 细项", code, 200, body)
	if string(body) != bodies.openai {
		t.Fatalf("非流式响应 JSON 应原样转发：\n got %s\nwant %s", body, bodies.openai)
	}
	if p, c, total, r, cached, status := lastUsageRow(t, f); p != 100 || c != 40 || total != 140 || r != 20 || cached != 64 || status != 200 {
		t.Fatalf("OpenAI 细项记账不对：prompt=%d completion=%d total=%d reasoning=%d cached=%d status=%d", p, c, total, r, cached, status)
	}

	// 非流式：DeepSeek 顶层的缓存命中 / 未命中。
	code, body = chat("deepseek", false)
	expect(t, "非流式 DeepSeek 缓存字段", code, 200, body)
	if p, c, total, r, cached, status := lastUsageRow(t, f); p != 200 || c != 50 || total != 250 || r != 12 || cached != 128 || status != 200 {
		t.Fatalf("DeepSeek 细项记账不对：prompt=%d completion=%d total=%d reasoning=%d cached=%d status=%d", p, c, total, r, cached, status)
	}

	// 非流式：没有细项的旧响应，两列为 0，且总量仍按原口径。
	code, body = chat("legacy", false)
	expect(t, "非流式旧响应", code, 200, body)
	if string(body) != bodies.legacy {
		t.Fatalf("旧响应应原样转发：%s", body)
	}
	if p, c, total, r, cached, status := lastUsageRow(t, f); p != 10 || c != 5 || total != 15 || r != 0 || cached != 0 || status != 200 {
		t.Fatalf("旧响应记账不对：prompt=%d completion=%d total=%d reasoning=%d cached=%d status=%d", p, c, total, r, cached, status)
	}

	// 流式：最后一块 usage 的细项也要记下来，转发内容不变。
	code, body = chat("deepseek", true)
	expect(t, "流式 DeepSeek 细项", code, 200, body)
	if string(body) != bodies.stream {
		t.Fatalf("流式转发内容被改动：\n got %q\nwant %q", body, bodies.stream)
	}
	if p, c, total, r, cached, status := lastUsageRow(t, f); p != 300 || c != 60 || total != 360 || r != 30 || cached != 256 || status != 200 {
		t.Fatalf("流式细项记账不对：prompt=%d completion=%d total=%d reasoning=%d cached=%d status=%d", p, c, total, r, cached, status)
	}

	// 流式兜底：没有 usage 块时按请求估算 prompt、按已转发增量估算 completion，细项保持 0。
	code, body = chat("cutoff", true)
	expect(t, "流式无 usage 兜底", code, 200, body)
	prompt, completion, total, reasoning, cached, status := lastUsageRow(t, f)
	if prompt <= 0 || completion <= 0 || total != prompt+completion || reasoning != 0 || cached != 0 || status != 200 {
		t.Fatalf("估算兜底记账不对：prompt=%d completion=%d total=%d reasoning=%d cached=%d status=%d", prompt, completion, total, reasoning, cached, status)
	}
}

// 细项累计进 /metrics，和写进 usage_logs 的调用一一对应。
func TestGatewayUsageDetailsMetrics(t *testing.T) {
	f := newPrincipalFixtureWith(t, func(o *api.Options) { o.Metrics = true })
	up, _ := newUsageDetailsUpstream(t)
	ctx := context.Background()
	if _, err := f.st.DB().ExecContext(ctx, "UPDATE providers SET base_url = ?", up.URL); err != nil {
		t.Fatal(err)
	}
	u := f.user(t, "usage-metrics", nil)
	f.token(t, u.ID, "ee_at_usage_metrics", []string{rbac.ScopeModels}, nil, nil)
	c := newClient(t, f.ts.URL)
	bearer := bearer("ee_at_usage_metrics")
	chat := func(variant string, stream bool) {
		t.Helper()
		code, body := c.do("POST", "/v1/chat/completions", map[string]any{
			"model":    "chat",
			"stream":   stream,
			"metadata": map[string]string{"variant": variant},
			"messages": []map[string]string{{"role": "user", "content": "你好"}},
		}, bearer)
		expect(t, "调用模型", code, 200, body)
	}
	// 20+12+30 = 62 reasoning；64+128+256 = 448 cached。
	chat("openai", false)
	chat("deepseek", false)
	chat("legacy", false)
	chat("deepseek", true)
	chat("cutoff", true)

	code, body := c.do("GET", "/metrics", nil, nil)
	expect(t, "抓取指标", code, 200, body)
	text := string(body)
	for _, want := range []string{
		"dsh_ee_usage_reasoning_tokens_total 62",
		"# TYPE dsh_ee_usage_reasoning_tokens_total counter",
		"dsh_ee_usage_cached_tokens_total 448",
		"# TYPE dsh_ee_usage_cached_tokens_total counter",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("指标里缺少 %q：\n%s", want, text)
		}
	}
}
