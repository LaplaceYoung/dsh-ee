package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// /responses 是 Responses API 的无版本前缀别名（DeepSeek Responses API 的 base URL 习惯）。
// 它必须和 /v1/responses 一样被当成公开协议面：限流、版本响应头、CORS 与 OpenAI 错误壳
// 都按同一个判断处理，避免别名绕过限流或丢协议头。
func TestResponsesAliasIsProtocolPath(t *testing.T) {
	paths := []string{"/v1/responses", "/responses"}
	for _, path := range paths {
		if !isResponsesAPIPath(path) {
			t.Fatalf("%s 应被识别为 Responses API 路径", path)
		}
		if !rateLimitedPath(path) {
			t.Fatalf("%s 应参与每令牌限流", path)
		}
		if !isHarnessAPIPath(path) {
			t.Fatalf("%s 应带 Dsh-EE-API-Version 响应头", path)
		}
		if !isCORSRoute(path) {
			t.Fatalf("%s 应纳入 CORS 白名单范围", path)
		}
		if isHarnessNeutralPath(path) {
			t.Fatalf("%s 不是中立只读端点，不应套中立错误壳/方法检查", path)
		}
	}
	// 相近但不等价的路径不能被误判：别名只匹配精确的 /responses。
	for _, path := range []string{"/responses/", "/responses/foo", "/v1/responses/foo", "/response", "/v1"} {
		if isResponsesAPIPath(path) {
			t.Fatalf("%s 不应被识别为 Responses API 路径", path)
		}
	}
	for _, path := range []string{"/healthz", "/api/me/tokens"} {
		if rateLimitedPath(path) || isCORSRoute(path) {
			t.Fatalf("%s 不应被当成公开协议面", path)
		}
	}
}

// /responses 别名必须和 /v1/responses 一样参与每令牌限流，超限返回 OpenAI 错误壳
// （Responses 客户端能解析），而不是被当成未限流的普通路径。
func TestResponsesAliasRateLimited(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/responses"} {
		t.Run(path, func(t *testing.T) {
			ts := rateLimitFixture(t, Options{RateLimitPerMinute: 1, RateLimitWindow: time.Minute})
			first := doRateRequest(t, ts.URL, http.MethodPost, path, "ee_at_rate_"+path, nil)
			wantHeader(t, first.header, "X-RateLimit-Limit", "1")
			wantHeader(t, first.header, "X-RateLimit-Remaining", "0")

			second := doRateRequest(t, ts.URL, http.MethodPost, path, "ee_at_rate_"+path, nil)
			if second.code != http.StatusTooManyRequests {
				t.Fatalf("%s 第二次请求状态码 = %d，期望 429；%s", path, second.code, second.body)
			}
			retryAfter(t, second.header)
			var parsed struct {
				Error struct {
					Type string `json:"type"`
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(second.body, &parsed); err != nil {
				t.Fatalf("%s 的 429 不是 OpenAI 错误壳：%v；%s", path, err, second.body)
			}
			if parsed.Error.Type != "rate_limit_error" || parsed.Error.Code != "rate_limit_exceeded" {
				t.Fatalf("%s 的 429 错误体不对：%s", path, second.body)
			}
		})
	}
}
