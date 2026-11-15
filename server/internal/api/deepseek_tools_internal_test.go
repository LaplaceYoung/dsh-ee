package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDeepSeekLimitedBody 覆盖文件上传的流式大小边界：恰好到达上限要正常读完，
// 超过上限时只转发到上限、返回哨兵错误并标记 exceeded（handler 据此回 413）。
func TestDeepSeekLimitedBody(t *testing.T) {
	t.Run("恰好等于上限", func(t *testing.T) {
		body := &deepseekLimitedBody{r: strings.NewReader("12345"), remaining: 5}
		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("恰好到达上限不应报错：%v", err)
		}
		if string(got) != "12345" {
			t.Fatalf("转发内容不对：%q", got)
		}
		if body.exceeded.Load() {
			t.Fatal("恰好到达上限不应标记超限")
		}
	})

	t.Run("超过上限一字节", func(t *testing.T) {
		body := &deepseekLimitedBody{r: strings.NewReader("123456"), remaining: 5}
		got, err := io.ReadAll(body)
		if !errors.Is(err, errDeepSeekBodyTooLarge) {
			t.Fatalf("超限应返回 errDeepSeekBodyTooLarge，得到 %v", err)
		}
		if !body.exceeded.Load() {
			t.Fatal("超限应标记 exceeded")
		}
		if string(got) != "12345" {
			t.Fatalf("只应转发到上限，实际 %q", got)
		}
	})

	t.Run("空 body", func(t *testing.T) {
		body := &deepseekLimitedBody{r: strings.NewReader(""), remaining: 5}
		got, err := io.ReadAll(body)
		if err != nil || len(got) != 0 || body.exceeded.Load() {
			t.Fatalf("空 body 应正常结束：%q %v exceeded=%v", got, err, body.exceeded.Load())
		}
	})
}

// TestReadDeepSeekBody 覆盖 ASR/TTS 这类小请求体读入时的 413。
func TestReadDeepSeekBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/asr", strings.NewReader("12345678"))
	raw, ok := readDeepSeekBody(rec, req, 8, "8 B")
	if !ok || string(raw) != "12345678" {
		t.Fatalf("恰好 8 字节应通过：ok=%v raw=%q", ok, raw)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tools/asr", strings.NewReader("123456789"))
	if _, ok := readDeepSeekBody(rec, req, 8, "8 B"); ok {
		t.Fatal("9 字节应被拒绝")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限应 413，得到 %d；%s", rec.Code, rec.Body.String())
	}
	var e struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("错误体不是 JSON：%v；%s", err, rec.Body.String())
	}
	if e.Error.Type != "invalid_request_error" || e.Error.Code != "request_too_large" {
		t.Fatalf("错误体不对：%+v", e.Error)
	}
}

// TestDeepSeekFileIDValidation 确保拼进上游 URL 的 file_id 不能带路径/编码字符。
func TestDeepSeekFileIDValidation(t *testing.T) {
	for _, id := range []string{
		"file-api-abc123", "file-api-abc_123.4", "A1", strings.Repeat("a", 128),
	} {
		if !deepseekFileIDRe.MatchString(id) {
			t.Fatalf("%q 应是合法 file_id", id)
		}
	}
	for _, id := range []string{
		"", ".", "..", "../chat", "a/b", "a%2Fb", "file api", "-leading-dash",
		strings.Repeat("a", 129),
	} {
		if deepseekFileIDRe.MatchString(id) {
			t.Fatalf("%q 不应通过 file_id 校验", id)
		}
	}
}

// TestDeepSeekFileTooLargeErrorBody 确认 413 用的是网关的 OpenAI 错误壳。
func TestDeepSeekFileTooLargeErrorBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDeepSeekTooLarge(rec, "100 MiB")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("应 413，得到 %d", rec.Code)
	}
	var e struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Error.Code != "request_too_large" || !strings.Contains(e.Error.Message, "100 MiB") {
		t.Fatalf("错误体不对：%s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type 不对：%q", ct)
	}
}
