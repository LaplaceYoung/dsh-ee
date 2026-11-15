package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// parseUsageDoc 模拟网关把上游响应里的 usage 解成 tokenUsage 的过程。
func parseUsageDoc(t *testing.T, raw string) tokenUsage {
	t.Helper()
	var doc struct {
		Usage *tokenUsage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("解析 usage: %v；%s", err, raw)
	}
	if doc.Usage == nil {
		t.Fatalf("响应里没有 usage：%s", raw)
	}
	return *doc.Usage
}

func TestUsageDetailsOpenAI(t *testing.T) {
	u := parseUsageDoc(t, `{"choices":[{"message":{"content":"答"}}],
		"usage":{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140,
		  "prompt_tokens_details":{"cached_tokens":64},
		  "completion_tokens_details":{"reasoning_tokens":20}}}`)
	if u.PromptTokens != 100 || u.CompletionTokens != 40 || u.TotalTokens != 140 {
		t.Fatalf("总量字段应原样解析：%+v", u)
	}
	// 细项不重复计入 total：推理 token 是 completion 的一部分，缓存命中是 prompt 的一部分。
	if u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		t.Fatalf("total 口径被改动：%+v", u)
	}
	if got := u.reasoningTokens(); got != 20 {
		t.Fatalf("reasoningTokens = %d，应为 20", got)
	}
	if got := u.cachedTokens(); got != 64 {
		t.Fatalf("cachedTokens = %d，应为 64", got)
	}
}

func TestUsageDetailsDeepSeekCacheFields(t *testing.T) {
	// DeepSeek 兼容格式：缓存命中 / 未命中在顶层，且没有 *_tokens_details。
	u := parseUsageDoc(t, `{"usage":{"prompt_tokens":260,"completion_tokens":24,"total_tokens":284,
		"prompt_cache_hit_tokens":128,"prompt_cache_miss_tokens":132,
		"completion_tokens_details":{"reasoning_tokens":12}}}`)
	if got := u.cachedTokens(); got != 128 {
		t.Fatalf("缓存命中应取 prompt_cache_hit_tokens=128，得到 %d", got)
	}
	if got := u.reasoningTokens(); got != 12 {
		t.Fatalf("reasoningTokens = %d，应为 12", got)
	}
	if got := u.PromptCacheMissTokens; got != 132 {
		t.Fatalf("prompt_cache_miss_tokens 解析 = %d，应为 132", got)
	}
}

func TestUsageDetailsDeepSeekOnlyMiss(t *testing.T) {
	// 只给未命中数时按 prompt_tokens - miss 推命中数。
	u := parseUsageDoc(t, `{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,
		"prompt_cache_miss_tokens":40}}`)
	if got := u.cachedTokens(); got != 60 {
		t.Fatalf("只给 miss 时应推回 cached=60，得到 %d", got)
	}
}

func TestUsageDetailsPreferredNested(t *testing.T) {
	// 两种细项同时出现时以 OpenAI 的嵌套字段为准。
	u := parseUsageDoc(t, `{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,
		"prompt_tokens_details":{"cached_tokens":64},
		"prompt_cache_hit_tokens":60,"prompt_cache_miss_tokens":40}}`)
	if got := u.cachedTokens(); got != 64 {
		t.Fatalf("嵌套 cached_tokens 优先，得到 %d", got)
	}
}

func TestUsageDetailsMissingIsZero(t *testing.T) {
	// 旧上游 / 旧响应没有细项：字段解析为 0，总量不变，不影响记账。
	u := parseUsageDoc(t, `{"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`)
	if u.reasoningTokens() != 0 || u.cachedTokens() != 0 {
		t.Fatalf("没有细项时应为 0：reasoning=%d cached=%d", u.reasoningTokens(), u.cachedTokens())
	}
	if u.PromptTokens != 12 || u.CompletionTokens != 3 || u.TotalTokens != 15 {
		t.Fatalf("旧响应总量被改动：%+v", u)
	}
}

func TestUsageDetailsNegativeOrInconsistent(t *testing.T) {
	// miss 大于 prompt 时不做减法，返回 0 而不是负数。
	u := parseUsageDoc(t, `{"usage":{"prompt_tokens":10,"total_tokens":10,"prompt_cache_miss_tokens":20}}`)
	if got := u.cachedTokens(); got != 0 {
		t.Fatalf("miss 超过 prompt 时 cached 应为 0，得到 %d", got)
	}
}

func TestRelaySSEUsageDetails(t *testing.T) {
	cases := []struct {
		name        string
		usage       string
		reasoning   int
		cached      int
		promptTotal int
	}{
		{
			name:        "OpenAI 嵌套细项",
			usage:       `{"prompt_tokens":50,"completion_tokens":9,"total_tokens":59,"prompt_tokens_details":{"cached_tokens":32},"completion_tokens_details":{"reasoning_tokens":7}}`,
			reasoning:   7,
			cached:      32,
			promptTotal: 59,
		},
		{
			name:        "DeepSeek 顶层缓存字段",
			usage:       `{"prompt_tokens":80,"completion_tokens":20,"total_tokens":100,"prompt_cache_hit_tokens":64,"prompt_cache_miss_tokens":16,"completion_tokens_details":{"reasoning_tokens":11}}`,
			reasoning:   11,
			cached:      64,
			promptTotal: 100,
		},
		{
			name:        "没有细项的旧响应",
			usage:       `{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}`,
			reasoning:   0,
			cached:      0,
			promptTotal: 25,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":" + tc.usage + "}\n\n" +
				"data: [DONE]\n\n"
			rec := httptest.NewRecorder()
			u, _ := relaySSE(rec, strings.NewReader(stream))
			if u.TotalTokens != tc.promptTotal {
				t.Fatalf("最后一块 usage 的 total 应原样采用：%+v", u)
			}
			if got := u.reasoningTokens(); got != tc.reasoning {
				t.Fatalf("reasoningTokens = %d，应为 %d", got, tc.reasoning)
			}
			if got := u.cachedTokens(); got != tc.cached {
				t.Fatalf("cachedTokens = %d，应为 %d", got, tc.cached)
			}
			// 细项解析不能改动转发的 SSE 内容。
			if rec.Body.String() != stream {
				t.Fatalf("转发内容被改动：%q", rec.Body.String())
			}
		})
	}
}

func TestRelaySSEUsageDetailsOnLastChunkOnly(t *testing.T) {
	// 中间分块没有 usage 时不能把已有细项冲掉；最后一块才是计费用的 usage。
	stream := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12,\"prompt_tokens_details\":{\"cached_tokens\":8},\"completion_tokens_details\":{\"reasoning_tokens\":1}}}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
		"data: [DONE]\n\n"
	u, _ := relaySSE(httptest.NewRecorder(), strings.NewReader(stream))
	if u.reasoningTokens() != 1 || u.cachedTokens() != 8 || u.TotalTokens != 12 {
		t.Fatalf("usage 细项丢失：%+v", u)
	}
}
