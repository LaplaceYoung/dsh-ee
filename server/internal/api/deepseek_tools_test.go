package api_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DeepSeek Files / ASR / TTS 代理（deepseek_tools.go）的端到端测试：
// fake 上游记录收到的请求，覆盖 multipart/路径/查询串/鉴权头/大小/错误透传/TTS 二进制。
const deepseekToolsUpstreamKey = "sk-deepseek-tools-upstream-secret"

// 与 deepseek_tools.go 里的上限一致；测试用裸 HTTP 只声明 Content-Length、不发 body，
// 验证代理在读取正文之前就拒绝超限请求。
const (
	deepseekFilesMaxBytesForTest  int64 = 100 << 20
	deepseekASRMaxBytesForTest    int64 = 8 << 20
	deepseekTTSMaxBytesForTest    int64 = 64 << 10
	deepseekSearchMaxBytesForTest int64 = 64 << 10
	deepseekFetchMaxBytesForTest  int64 = 64 << 10
	deepseekOCRMaxBytesForTest    int64 = 100 << 20
)

type deepseekToolsCall struct {
	Method           string
	Path             string
	RawQuery         string
	ContentType      string
	Authorization    string
	AnthropicBeta    string
	AnthropicVersion string
	ClientXAPIKey    string
	Body             []byte
}

// lockedBuffer 让测试在请求结束后安全读取服务端日志。
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type deepseekToolsFixture struct {
	st       *store.Store
	box      *secret.Box
	ts       *httptest.Server
	upstream *httptest.Server
	admin    *client
	logBuf   *lockedBuffer
	// failStatus 非 0 时 fake 上游对所有请求返回该状态码 + OpenAI 错误壳（测上游错误透传）。
	failStatus atomic.Int32
	mu         sync.Mutex
	calls      []deepseekToolsCall
	tokenSeq   int
}

func newDeepSeekToolsFixture(t *testing.T) *deepseekToolsFixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	f := &deepseekToolsFixture{st: st, logBuf: &lockedBuffer{}}
	f.upstream = httptest.NewServer(http.HandlerFunc(f.serveUpstream))
	t.Cleanup(f.upstream.Close)

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatalf("secret box: %v", err)
	}
	f.box = box

	f.ts = f.startServer(t, "")
	f.admin = newClient(t, f.ts.URL)
	code, body := f.admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	return f
}

// startServer 用同一个库起一个实例；provider 是 DSH_EE_DEEPSEEK_PROVIDER 的值。
// 默认不允许普通员工使用文件/工具接口（与生产默认一致）。
func (f *deepseekToolsFixture) startServer(t *testing.T, provider string) *httptest.Server {
	t.Helper()
	return f.startServerWith(t, provider, false)
}

// startServerWith 允许测试显式打开/关闭 DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE。
func (f *deepseekToolsFixture) startServerWith(t *testing.T, provider string, allowEmployee bool) *httptest.Server {
	t.Helper()
	srv := api.New(f.st, f.box, api.Options{
		DataDir:                    t.TempDir(),
		Upstream:                   f.upstream.Client(),
		DeepSeekProvider:           provider,
		DeepSeekToolsAllowEmployee: allowEmployee,
	}, slog.New(slog.NewTextHandler(f.logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func (f *deepseekToolsFixture) serveUpstream(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 128<<20))
	f.mu.Lock()
	f.calls = append(f.calls, deepseekToolsCall{
		Method:           r.Method,
		Path:             r.URL.Path,
		RawQuery:         r.URL.RawQuery,
		ContentType:      r.Header.Get("Content-Type"),
		Authorization:    r.Header.Get("Authorization"),
		AnthropicBeta:    r.Header.Get("anthropic-beta"),
		AnthropicVersion: r.Header.Get("anthropic-version"),
		ClientXAPIKey:    r.Header.Get("x-api-key"),
		Body:             body,
	})
	f.mu.Unlock()

	if got := r.Header.Get("Authorization"); got != "Bearer "+deepseekToolsUpstreamKey {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream key invalid","type":"authentication_error","code":"invalid_api_key"}}`)
		return
	}
	// 模拟 DeepSeek 的 Anthropic 兼容 Files：必须带 anthropic-beta 头，缺失返回 400。
	// 代理自己不校验、不补这个头，所以「带/不带都原样到上游」由这个 fake 验证。
	if strings.HasPrefix(r.URL.Path, "/anthropic/") && r.Header.Get("anthropic-beta") != "files-api-2025-04-14" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"anthropic-beta: files-api-2025-04-14 is required for Files API"}}`)
		return
	}
	if status := int(f.failStatus.Load()); status != 0 {
		w.Header().Set("Content-Type", "application/json")
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "3")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream boom","type":"server_error","code":"upstream_boom"}}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/files":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","object":"file","bytes":102400,"created_at":1700000000,"filename":"image.jpg","purpose":"user_data"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/files":
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"file-api-abc123","object":"file","bytes":102400,"created_at":1700000000,"filename":"image.jpg","purpose":"user_data"}],"first_id":"file-api-abc123","last_id":"file-api-abc123","has_more":false}`)
	case r.Method == http.MethodGet && r.URL.Path == "/files/file-api-abc123":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","object":"file","bytes":102400,"created_at":1700000000,"filename":"image.jpg","purpose":"user_data"}`)
	case r.Method == http.MethodDelete && r.URL.Path == "/files/file-api-abc123":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","object":"file","deleted":true}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/asr":
		_, _ = io.WriteString(w, `{"request_id":"req-asr-1","text":"上海今天天气怎么样？","duration_ms":3200}`)
	case r.Method == http.MethodPost && r.URL.Path == "/chat/completions":
		_, _ = io.WriteString(w, `{"id":"chat-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/tts":
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF\x24\x00\x00\x00WAVEfmt "))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0x03, 0xfe, 0xff})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/search":
		_, _ = io.WriteString(w, `{"results":[{"rerank_score":2.4,"title":"DeepSeek V4 模型发布","url":"https://example.com/deepseek-v4","snippet":"DeepSeek V4 已上线。","summary":"DeepSeek 发布了 V4 模型。","content":"","site_name":"Example News","site_icon":null,"date":null,"authority":3}]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/fetch":
		_, _ = io.WriteString(w, `{"title":"Example article","content":"# Example article\n\nArticle content."}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/ocr":
		// OCR 在识别期间持续发换行保活，识别完成后才给 JSON；字节要原样透传（含前导换行）。
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("\n\n"))
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, `{"id":"ocr-9b2f4c1e","object":"ocr.result","created":1762790000,"purpose":"text_recognition","page_count":1,"pages":[{"index":0,"text":"第一页文字","finish_reason":"stop"}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/anthropic/v1/files":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","type":"file","size_bytes":102400,"created_at":"2026-01-01T00:00:00+00:00","filename":"image.jpg","mime_type":"image/jpeg"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/anthropic/v1/files":
		_, _ = io.WriteString(w, `{"data":[{"id":"file-api-abc123","type":"file","size_bytes":102400,"created_at":"2026-01-01T00:00:00+00:00","filename":"image.jpg","mime_type":"image/jpeg"}],"first_id":"file-api-abc123","last_id":"file-api-abc123","has_more":false}`)
	case r.Method == http.MethodGet && r.URL.Path == "/anthropic/v1/files/file-api-abc123":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","type":"file","size_bytes":102400,"created_at":"2026-01-01T00:00:00+00:00","filename":"image.jpg","mime_type":"image/jpeg"}`)
	case r.Method == http.MethodDelete && r.URL.Path == "/anthropic/v1/files/file-api-abc123":
		_, _ = io.WriteString(w, `{"id":"file-api-abc123","type":"file_deleted"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"file not found","type":"invalid_request_error","code":"file_not_found"}}`)
	}
}

func (f *deepseekToolsFixture) lastCall(t *testing.T) deepseekToolsCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("fake 上游没有收到请求")
	}
	return f.calls[len(f.calls)-1]
}

func (f *deepseekToolsFixture) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// addProvider 通过管理端接口建一个指向 fake 上游的模型上游，返回 id。
func (f *deepseekToolsFixture) addProvider(t *testing.T, name, vendor string, enabled bool) int64 {
	t.Helper()
	return f.addProviderModels(t, name, vendor, enabled, nil)
}

// addProviderModels 同上，并顺手建几个对外模型（chat 引用拦截测试要能选到模型）。
func (f *deepseekToolsFixture) addProviderModels(t *testing.T, name, vendor string, enabled bool, models []string) int64 {
	t.Helper()
	req := map[string]any{
		"name": name, "vendor": vendor, "base_url": f.upstream.URL,
		"api_key": deepseekToolsUpstreamKey, "enabled": enabled,
	}
	if len(models) > 0 {
		req["models"] = models
	}
	code, body := f.admin.do("POST", "/api/admin/providers", req, nil)
	expect(t, "创建上游 "+name, code, 201, body)
	return decode[store.Provider](t, body).ID
}

// token 建一个普通员工（默认「员工」角色，月度额度设为不限额）并直接发一个已知明文的
// ee_at_ 令牌（scopes 决定接口权限）。
func (f *deepseekToolsFixture) token(t *testing.T, username string, scopes []string) string {
	t.Helper()
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "password": "employee-password-1",
	}, nil)
	expect(t, "创建员工 "+username, code, 201, body)
	u := decode[store.User](t, body)
	// 放开月度额度：工具接口不占 token，但 chat 引用拦截测试需要能走到上游。
	code, body = f.admin.do("PATCH", "/api/admin/users/"+itoa(u.ID)+"/quota", map[string]any{"monthly_token_limit": nil}, nil)
	expect(t, "放开员工额度", code, 200, body)
	return f.issueToken(t, &u, scopes)
}

// adminToken 给内置管理员账号发一个已知明文的令牌（管理员默认可用文件/工具接口）。
func (f *deepseekToolsFixture) adminToken(t *testing.T, scopes []string) string {
	t.Helper()
	u, err := f.st.GetUserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("读取管理员: %v", err)
	}
	return f.issueToken(t, u, scopes)
}

// botToken 建一个机器人/服务账号（account_type=bot）并发令牌。
func (f *deepseekToolsFixture) botToken(t *testing.T, username string, scopes []string) string {
	t.Helper()
	admin, err := f.st.GetUserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("读取管理员: %v", err)
	}
	code, body := f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "account_type": "bot", "owner_id": admin.ID,
		"display_name": "服务机器人", "monthly_token_limit": nil,
	}, nil)
	expect(t, "创建机器人 "+username, code, 201, body)
	u := decode[store.User](t, body)
	if u.AccountType != store.AccountBot {
		t.Fatalf("账号类型 = %q，期望 bot", u.AccountType)
	}
	return f.issueToken(t, &u, scopes)
}

// issueToken 直接发一个已知明文的 ee_at_ 令牌；明文用短序号（prefix 列很短）。
func (f *deepseekToolsFixture) issueToken(t *testing.T, u *store.User, scopes []string) string {
	t.Helper()
	f.tokenSeq++
	plain := "ee_at_ds" + strconv.Itoa(f.tokenSeq)
	tok := &store.AccessToken{
		UserID: u.ID, Name: plain, Prefix: plain, Source: "manual", Scopes: scopes,
		ClientName: "test-client", CreatedBy: u.ID,
	}
	if err := f.st.CreateAccessToken(context.Background(), tok, secret.Hash(plain)); err != nil {
		t.Fatalf("创建令牌: %v", err)
	}
	return plain
}

// doRaw 发一个自定义 body/头的请求，不跟随重定向（测试 413 / 非法路径时不想被 mangle）。
func doRaw(t *testing.T, method, url string, body io.Reader, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("构造请求: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, data
}

// rawContentLengthRequest 用裸 TCP 发一个只声明超大 Content-Length、不发 body 的请求：
// 如果代理按 Content-Length 在读取正文前拒绝，就能拿到 413 而不用真的传 100 MiB。
func rawContentLengthRequest(t *testing.T, ts *httptest.Server, method, path, token string, contentLength int64) (int, []byte) {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("连接测试服务: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	head := method + " " + path + " HTTP/1.1\r\n" +
		"Host: dsh-ee.test\r\n" +
		"Authorization: Bearer " + token + "\r\n" +
		"Content-Type: multipart/form-data; boundary=deepseek-tools-test\r\n" +
		"Content-Length: " + strconv.FormatInt(contentLength, 10) + "\r\n\r\n"
	if _, err := io.WriteString(conn, head); err != nil {
		t.Fatalf("写请求头: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("读响应: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// rawBodyRequest 用裸 TCP 发完整的 body 再读响应：Go 的 http.Server 对没读完的小请求体
// （<256 KiB）会先 drain 再 flush 响应，只发头不发 body 会等不到响应。TTS 的 64 KiB 上限
// 属于这个区间，用它验证「Content-Length 超限即 413」。
func rawBodyRequest(t *testing.T, ts *httptest.Server, method, path, token, contentType string, body []byte) (int, []byte) {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("连接测试服务: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	head := method + " " + path + " HTTP/1.1\r\n" +
		"Host: dsh-ee.test\r\n" +
		"Authorization: Bearer " + token + "\r\n" +
		"Content-Type: " + contentType + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	if _, err := io.WriteString(conn, head); err != nil {
		t.Fatalf("写请求头: %v", err)
	}
	if _, err := conn.Write(body); err != nil {
		t.Fatalf("写请求体: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("读响应: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func openAIErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("错误体不是 OpenAI JSON 壳：%v；%s", err, body)
	}
	if e.Error.Message == "" || e.Error.Type == "" || e.Error.Code == "" {
		t.Fatalf("错误体缺字段：%s", body)
	}
	return e.Error.Code
}

func multipartBody(t *testing.T, fields [][2]string, fileField, filename string, file []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// 顺序由调用方决定：DeepSeek 要求上传文档时 purpose 排在 file 前面。
	for _, kv := range fields {
		if err := mw.WriteField(kv[0], kv[1]); err != nil {
			t.Fatalf("写表单字段: %v", err)
		}
	}
	fw, err := mw.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("写文件字段: %v", err)
	}
	if _, err := fw.Write(file); err != nil {
		t.Fatalf("写文件内容: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("关闭 multipart: %v", err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// ---- 无可用上游 ----

func TestDeepSeekToolsProviderUnavailable(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	header := bearer(token)

	// 默认不配 DSH_EE_DEEPSEEK_PROVIDER 且没有 vendor=deepseek 的上游：稳定 503。
	code, _, body := doRaw(t, "GET", f.ts.URL+"/v1/files", nil, header)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("无上游应 503，得到 %d；%s", code, body)
	}
	if got := openAIErrorCode(t, body); got != "deepseek_provider_unavailable" {
		t.Fatalf("错误码 = %q，期望 deepseek_provider_unavailable；%s", got, body)
	}
	// ASR / TTS / 文件详情也应给出同一错误码。
	for _, tc := range []struct{ method, path string }{
		{"POST", "/v1/tools/asr"},
		{"POST", "/v1/tools/tts"},
		{"GET", "/v1/files/file-api-abc123"},
		{"DELETE", "/v1/files/file-api-abc123"},
	} {
		code, _, body = doRaw(t, tc.method, f.ts.URL+tc.path, strings.NewReader("{}"), header)
		if code != http.StatusServiceUnavailable || openAIErrorCode(t, body) != "deepseek_provider_unavailable" {
			t.Fatalf("%s %s 应 503 deepseek_provider_unavailable，得到 %d；%s", tc.method, tc.path, code, body)
		}
	}

	// 只有 vendor 不是 deepseek 的启用上游时默认选不到它（显式配置才可以，见下一个测试）。
	f.addProvider(t, "openai-like", "custom", true)
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files", nil, header)
	if code != http.StatusServiceUnavailable || openAIErrorCode(t, body) != "deepseek_provider_unavailable" {
		t.Fatalf("没有 vendor=deepseek 的启用上游时应 503，得到 %d；%s", code, body)
	}

	// 禁用的 deepseek 上游也不参与默认选择。
	f.addProvider(t, "deepseek-disabled", "deepseek", false)
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files", nil, header)
	if code != http.StatusServiceUnavailable || openAIErrorCode(t, body) != "deepseek_provider_unavailable" {
		t.Fatalf("禁用的 upstream 不应被选中，得到 %d；%s", code, body)
	}
	// 全程都不该打到上游。
	if n := f.callCount(); n != 0 {
		t.Fatalf("没有可用上游时不应对上游发请求，实际 %d 次", n)
	}
}

// ---- 上游选择 ----

func TestDeepSeekToolsProviderSelection(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	header := bearer(token)

	deepseekID := f.addProvider(t, "deepseek-main", "deepseek", true)
	customID := f.addProvider(t, "deepseek-proxy", "custom", true)

	// 默认：第一个启用的 vendor=deepseek 上游。
	code, _, body := doRaw(t, "GET", f.ts.URL+"/v1/files", nil, header)
	if code != http.StatusOK {
		t.Fatalf("默认应选中 vendor=deepseek 上游，得到 %d；%s", code, body)
	}

	// 显式按 name 指定（可以指向 vendor=custom 的 DeepSeek 兼容网关）。
	for _, tc := range []struct {
		name     string
		provider string
		status   int
	}{
		{"按名称选 deepseek", "deepseek-main", http.StatusOK},
		{"按名称选自建代理", "deepseek-proxy", http.StatusOK},
		{"按 id 选 deepseek", strconv.FormatInt(deepseekID, 10), http.StatusOK},
		{"按 id 选自建代理", strconv.FormatInt(customID, 10), http.StatusOK},
		{"名称不存在", "no-such-provider", http.StatusServiceUnavailable},
		{"id 不存在", "999999", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := f.startServer(t, tc.provider)
			code, _, body := doRaw(t, "GET", ts.URL+"/v1/files", nil, header)
			if code != tc.status {
				t.Fatalf("状态码 = %d，期望 %d；%s", code, tc.status, body)
			}
			if tc.status == http.StatusServiceUnavailable && openAIErrorCode(t, body) != "deepseek_provider_unavailable" {
				t.Fatalf("错误码不对：%s", body)
			}
		})
	}
}

// ---- Files ----

func TestDeepSeekToolsFiles(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	header := bearer(token)

	// 上传：multipart 原样转发（purpose 在 file 前，验证连字段顺序都没被代理改动）。
	fileBytes := []byte("fake-image-bytes-not-to-be-logged")
	raw, contentType := multipartBody(t, [][2]string{{"purpose", "user_data"}}, "file", "image.jpg", fileBytes)
	code, respHeader, body := doRaw(t, "POST", f.ts.URL+"/v1/files", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	})
	if code != http.StatusOK {
		t.Fatalf("上传文件应 200，得到 %d；%s", code, body)
	}
	if !strings.Contains(string(body), `"file-api-abc123"`) {
		t.Fatalf("响应应原样带 upstream 文件对象：%s", body)
	}
	if ct := respHeader.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("响应 Content-Type 应沿用上游：%q", ct)
	}
	if cc := respHeader.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("文件元数据响应不应被缓存，Cache-Control=%q", cc)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/files" {
		t.Fatalf("上游收到的请求不对：%s %s", call.Method, call.Path)
	}
	if call.ContentType != contentType {
		t.Fatalf("multipart Content-Type（含 boundary）应原样转发：%q vs %q", call.ContentType, contentType)
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("上游 Authorization 应是企业配置的 key，得到 %q", call.Authorization)
	}
	if !bytes.Equal(call.Body, raw) {
		t.Fatalf("multipart body 应逐字节原样转发：%d vs %d 字节", len(call.Body), len(raw))
	}

	// 列表：查询串原样转发。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files?limit=5&order=desc&purpose=user_data", nil, header)
	if code != http.StatusOK || !strings.Contains(string(body), `"has_more":false`) {
		t.Fatalf("列文件应 200 且透传列表，得到 %d；%s", code, body)
	}
	call = f.lastCall(t)
	if call.Method != "GET" || call.Path != "/files" || call.RawQuery != "limit=5&order=desc&purpose=user_data" {
		t.Fatalf("列表查询串没有原样转发：%s %s?%s", call.Method, call.Path, call.RawQuery)
	}

	// 查询单个文件：路径透传。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files/file-api-abc123", nil, header)
	if code != http.StatusOK || !strings.Contains(string(body), `"filename":"image.jpg"`) {
		t.Fatalf("查文件应 200，得到 %d；%s", code, body)
	}
	if call = f.lastCall(t); call.Path != "/files/file-api-abc123" {
		t.Fatalf("文件路径没有透传：%s", call.Path)
	}

	// 删除文件。
	code, _, body = doRaw(t, "DELETE", f.ts.URL+"/v1/files/file-api-abc123", nil, header)
	if code != http.StatusOK || !strings.Contains(string(body), `"deleted":true`) {
		t.Fatalf("删文件应 200，得到 %d；%s", code, body)
	}
	if call = f.lastCall(t); call.Method != "DELETE" || call.Path != "/files/file-api-abc123" {
		t.Fatalf("删除请求不对：%s %s", call.Method, call.Path)
	}

	// 非法 file_id：不转发上游，直接 400（挡住路径注入与编码过的斜杠）。
	for _, id := range []string{".hidden", "a%2Fb", "%2E%2E"} {
		before := f.callCount()
		code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files/"+id, nil, header)
		if code != http.StatusBadRequest {
			t.Fatalf("非法 file_id %q 应 400，得到 %d；%s", id, code, body)
		}
		if got := openAIErrorCode(t, body); got != "invalid_file_id" {
			t.Fatalf("错误码 = %q，期望 invalid_file_id", got)
		}
		if f.callCount() != before {
			t.Fatalf("非法 file_id %q 不应转发到上游", id)
		}
	}

	// 上游 404：状态码与错误体原样透传。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files/file-api-missing", nil, header)
	if code != http.StatusNotFound {
		t.Fatalf("上游 404 应透传，得到 %d；%s", code, body)
	}
	if got := openAIErrorCode(t, body); got != "file_not_found" {
		t.Fatalf("上游错误体应原样返回，code=%q；%s", got, body)
	}

	// 上游 429：状态码、Retry-After 与错误体都透传。
	f.failStatus.Store(http.StatusTooManyRequests)
	code, respHeader, body = doRaw(t, "GET", f.ts.URL+"/v1/files", nil, header)
	f.failStatus.Store(0)
	if code != http.StatusTooManyRequests {
		t.Fatalf("上游 429 应透传，得到 %d；%s", code, body)
	}
	if ra := respHeader.Get("Retry-After"); ra != "3" {
		t.Fatalf("上游 Retry-After 应透传，得到 %q", ra)
	}
	if got := openAIErrorCode(t, body); got != "upstream_boom" {
		t.Fatalf("上游错误体应原样返回，code=%q", got)
	}

	// 超大请求：Content-Length 超过 100 MiB，读取正文之前就 413，且不打上游。
	before := f.callCount()
	code, body = rawContentLengthRequest(t, f.ts, "POST", "/v1/files", token, deepseekFilesMaxBytesForTest+1)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超过 100 MiB 应 413，得到 %d；%s", code, body)
	}
	if got := openAIErrorCode(t, body); got != "request_too_large" {
		t.Fatalf("错误码 = %q，期望 request_too_large", got)
	}
	if f.callCount() != before {
		t.Fatal("超限请求不应转发到上游")
	}

	// 鉴权：无令牌 401；令牌没有 models scope 403。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files", nil, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，得到 %d；%s", code, body)
	}
	weak := f.token(t, "ds-tools-files-weak", []string{rbac.ScopeRegistryRead})
	code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files", nil, bearer(weak))
	if code != http.StatusForbidden {
		t.Fatalf("没有 models scope 应 403，得到 %d；%s", code, body)
	}

	// 日志里不能出现文件内容或调用者的令牌明文。
	logs := f.logBuf.String()
	if strings.Contains(logs, "fake-image-bytes-not-to-be-logged") {
		t.Fatal("访问日志不应包含上传的文件内容")
	}
	if strings.Contains(logs, token) || strings.Contains(logs, deepseekToolsUpstreamKey) {
		t.Fatal("访问日志不应包含调用者令牌或上游 key")
	}
}

// ---- ASR ----

func TestDeepSeekToolsASR(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})

	audio := []byte("RIFF....WAVEfmt fake-audio-bytes")
	raw, contentType := multipartBody(t, nil, "file", "speech.wav", audio)
	code, header, body := doRaw(t, "POST", f.ts.URL+"/v1/tools/asr", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	})
	if code != http.StatusOK {
		t.Fatalf("ASR 应 200，得到 %d；%s", code, body)
	}
	if !strings.Contains(string(body), `"text":"上海今天天气怎么样？"`) {
		t.Fatalf("转写 JSON 应原样返回：%s", body)
	}
	if ct := header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("ASR 响应 Content-Type 不对：%q", ct)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/v1/tools/asr" {
		t.Fatalf("上游路径应为 /v1/tools/asr：%s %s", call.Method, call.Path)
	}
	if call.ContentType != contentType || !bytes.Equal(call.Body, raw) {
		t.Fatalf("ASR multipart 应原样转发：ct=%q bytes=%d/%d", call.ContentType, len(call.Body), len(raw))
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("ASR 上游 Authorization 不对：%q", call.Authorization)
	}

	// 超过 8 MiB：413 且不打上游。
	before := f.callCount()
	code, body = rawContentLengthRequest(t, f.ts, "POST", "/v1/tools/asr", token, deepseekASRMaxBytesForTest+1)
	if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "request_too_large" {
		t.Fatalf("ASR 超限应 413 request_too_large，得到 %d；%s", code, body)
	}
	if f.callCount() != before {
		t.Fatal("ASR 超限请求不应转发到上游")
	}

	// 上游 400（例如音频格式不支持）：状态码与错误体透传。
	f.failStatus.Store(http.StatusBadRequest)
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/asr", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	})
	f.failStatus.Store(0)
	if code != http.StatusBadRequest || openAIErrorCode(t, body) != "upstream_boom" {
		t.Fatalf("ASR 上游 400 应透传，得到 %d；%s", code, body)
	}

	// 没有 models scope：403。
	weak := f.token(t, "ds-tools-asr-weak", []string{rbac.ScopeRegistryRead})
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/asr", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + weak,
		"Content-Type":  contentType,
	})
	if code != http.StatusForbidden {
		t.Fatalf("ASR 无 models scope 应 403，得到 %d；%s", code, body)
	}
}

// ---- TTS ----

func TestDeepSeekToolsTTS(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})

	payload := []byte(`{"text":"你好，我是 DeepSeek。","voice_id":"stella"}`)
	code, header, body := doRaw(t, "POST", f.ts.URL+"/v1/tools/tts", bytes.NewReader(payload), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	})
	if code != http.StatusOK {
		t.Fatalf("TTS 应 200，得到 %d；%s", code, body)
	}
	wantAudio := append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), 0x00, 0x01, 0x02, 0x03, 0xfe, 0xff)
	if !bytes.Equal(body, wantAudio) {
		t.Fatalf("TTS 音频应逐字节透传：%x vs %x", body, wantAudio)
	}
	if ct := header.Get("Content-Type"); ct != "audio/wav" {
		t.Fatalf("TTS 应透传 audio/wav，得到 %q", ct)
	}
	if cc := header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("TTS 音频不应被缓存，Cache-Control=%q", cc)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/v1/tools/tts" {
		t.Fatalf("上游路径应为 /v1/tools/tts：%s %s", call.Method, call.Path)
	}
	if call.ContentType != "application/json" {
		t.Fatalf("TTS 上游 Content-Type 应为 application/json，得到 %q", call.ContentType)
	}
	if !bytes.Equal(call.Body, payload) {
		t.Fatalf("TTS JSON 应原样转发：%s vs %s", call.Body, payload)
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("TTS 上游 Authorization 不对：%q", call.Authorization)
	}

	// 超过 64 KiB：413 且不打上游。
	before := f.callCount()
	overLimit := bytes.Repeat([]byte("a"), int(deepseekTTSMaxBytesForTest)+1)
	code, body = rawBodyRequest(t, f.ts, "POST", "/v1/tools/tts", token, "application/json", overLimit)
	if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "request_too_large" {
		t.Fatalf("TTS 超限应 413 request_too_large，得到 %d；%s", code, body)
	}
	if f.callCount() != before {
		t.Fatal("TTS 超限请求不应转发到上游")
	}

	// 上游 504（合成超时）：状态与错误体透传。
	f.failStatus.Store(http.StatusGatewayTimeout)
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/tts", bytes.NewReader(payload), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	})
	f.failStatus.Store(0)
	if code != http.StatusGatewayTimeout || openAIErrorCode(t, body) != "upstream_boom" {
		t.Fatalf("TTS 上游 504 应透传，得到 %d；%s", code, body)
	}

	// 无令牌：401。
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/tts", bytes.NewReader(payload), map[string]string{
		"Content-Type": "application/json",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("TTS 无令牌应 401，得到 %d；%s", code, body)
	}
}

// ---- Search / Fetch ----

func TestDeepSeekToolsSearchFetch(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	header := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	// Search：JSON 原样转发，结果数组原样返回。
	searchBody := []byte(`{"query":"DeepSeek V4 模型 发布","count":5,"need_content":true}`)
	code, respHeader, body := doRaw(t, "POST", f.ts.URL+"/v1/tools/search", bytes.NewReader(searchBody), header)
	if code != http.StatusOK || !strings.Contains(string(body), `"results"`) {
		t.Fatalf("Search 应 200 且返回结果数组，得到 %d；%s", code, body)
	}
	if ct := respHeader.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Search 响应 Content-Type 不对：%q", ct)
	}
	if cc := respHeader.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Search 响应不应被缓存：%q", cc)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/v1/tools/search" || call.RawQuery != "" {
		t.Fatalf("Search 上游请求不对：%s %s?%s", call.Method, call.Path, call.RawQuery)
	}
	if call.ContentType != "application/json" || !bytes.Equal(call.Body, searchBody) {
		t.Fatalf("Search JSON 应原样转发：ct=%q body=%s", call.ContentType, call.Body)
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("Search 上游 Authorization 不对：%q", call.Authorization)
	}

	// Fetch：JSON 原样转发，网页 Markdown 原样返回。
	fetchBody := []byte(`{"url":"https://example.com/article"}`)
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/fetch", bytes.NewReader(fetchBody), header)
	if code != http.StatusOK || !strings.Contains(string(body), `"content"`) {
		t.Fatalf("Fetch 应 200 且返回正文，得到 %d；%s", code, body)
	}
	call = f.lastCall(t)
	if call.Path != "/v1/tools/fetch" || !bytes.Equal(call.Body, fetchBody) {
		t.Fatalf("Fetch 上游请求不对：%s body=%s", call.Path, call.Body)
	}

	// 上游错误（429 + Retry-After）原样透传。
	f.failStatus.Store(http.StatusTooManyRequests)
	code, respHeader, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/search", bytes.NewReader(searchBody), header)
	f.failStatus.Store(0)
	if code != http.StatusTooManyRequests || respHeader.Get("Retry-After") != "3" || openAIErrorCode(t, body) != "upstream_boom" {
		t.Fatalf("Search 上游 429 应透传，得到 %d %q；%s", code, respHeader.Get("Retry-After"), body)
	}

	// 上限 64 KiB：超限 413 且不打上游（请求体超过 64 KiB，Go server 会先 drain，整包发出）。
	over := bytes.Repeat([]byte("a"), int(deepseekSearchMaxBytesForTest)+1)
	for _, path := range []string{"/v1/tools/search", "/v1/tools/fetch"} {
		before := f.callCount()
		code, body = rawBodyRequest(t, f.ts, "POST", path, token, "application/json", over)
		if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "request_too_large" {
			t.Fatalf("%s 超限应 413 request_too_large，得到 %d；%s", path, code, body)
		}
		if f.callCount() != before {
			t.Fatalf("%s 超限请求不应转发到上游", path)
		}
	}

	// 未带令牌 401。
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/search", bytes.NewReader(searchBody), map[string]string{"Content-Type": "application/json"})
	if code != http.StatusUnauthorized {
		t.Fatalf("Search 无令牌应 401，得到 %d；%s", code, body)
	}
}

// ---- OCR ----

func TestDeepSeekToolsOCR(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})

	fileBytes := []byte("%PDF-1.7 fake-pdf-bytes")
	raw, contentType := multipartBody(t, [][2]string{{"purpose", "text_recognition"}}, "file", "doc.pdf", fileBytes)
	code, respHeader, body := doRaw(t, "POST", f.ts.URL+"/v1/tools/ocr", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	})
	if code != http.StatusOK {
		t.Fatalf("OCR 应 200，得到 %d；%s", code, body)
	}
	// 上游先发两个换行保活再给 JSON：代理要逐字节透传，客户端收到的响应体仍是「换行 + JSON」。
	if !bytes.HasPrefix(body, []byte("\n\n")) || !strings.Contains(string(body), `"ocr-9b2f4c1e"`) {
		t.Fatalf("OCR 响应应原样透传前导换行与 JSON：%q", body)
	}
	if ct := respHeader.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("OCR 响应 Content-Type 不对：%q", ct)
	}
	if cc := respHeader.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("OCR 响应不应被缓存：%q", cc)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/v1/tools/ocr" {
		t.Fatalf("OCR 上游路径应为 /v1/tools/ocr：%s %s", call.Method, call.Path)
	}
	if call.ContentType != contentType || !bytes.Equal(call.Body, raw) {
		t.Fatalf("OCR multipart 应原样转发：ct=%q bytes=%d/%d", call.ContentType, len(call.Body), len(raw))
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("OCR 上游 Authorization 不对：%q", call.Authorization)
	}

	// 超过 100 MiB：读取正文前 413，且不打上游。
	before := f.callCount()
	code, body = rawContentLengthRequest(t, f.ts, "POST", "/v1/tools/ocr", token, deepseekOCRMaxBytesForTest+1)
	if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "request_too_large" {
		t.Fatalf("OCR 超限应 413 request_too_large，得到 %d；%s", code, body)
	}
	if f.callCount() != before {
		t.Fatal("OCR 超限请求不应转发到上游")
	}

	// 上游 413（文件过大）原样透传。
	f.failStatus.Store(http.StatusRequestEntityTooLarge)
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/ocr", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  contentType,
	})
	f.failStatus.Store(0)
	if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "upstream_boom" {
		t.Fatalf("OCR 上游 413 应透传，得到 %d；%s", code, body)
	}

	// 无令牌 401。
	code, _, body = doRaw(t, "POST", f.ts.URL+"/v1/tools/ocr", bytes.NewReader(raw), map[string]string{"Content-Type": contentType})
	if code != http.StatusUnauthorized {
		t.Fatalf("OCR 无令牌应 401，得到 %d；%s", code, body)
	}
}

// ---- Anthropic 兼容 Files ----

func TestDeepSeekToolsAnthropicFiles(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	beta := map[string]string{
		"Authorization":     "Bearer " + token,
		"anthropic-beta":    "files-api-2025-04-14",
		"anthropic-version": "2023-06-01",
	}

	// 上传：multipart 原样转发，anthropic-beta / anthropic-version 带给上游。
	fileBytes := []byte("fake-image-bytes-anthropic")
	raw, contentType := multipartBody(t, nil, "file", "image.jpg", fileBytes)
	uploadHeader := map[string]string{
		"Authorization":     beta["Authorization"],
		"anthropic-beta":    beta["anthropic-beta"],
		"anthropic-version": beta["anthropic-version"],
		"Content-Type":      contentType,
	}
	code, _, body := doRaw(t, "POST", f.ts.URL+"/anthropic/v1/files", bytes.NewReader(raw), uploadHeader)
	if code != http.StatusOK || !strings.Contains(string(body), `"type":"file"`) {
		t.Fatalf("Anthropic Files 上传应 200，得到 %d；%s", code, body)
	}
	call := f.lastCall(t)
	if call.Method != "POST" || call.Path != "/anthropic/v1/files" {
		t.Fatalf("Anthropic 上传上游路径不对：%s %s", call.Method, call.Path)
	}
	if call.AnthropicBeta != "files-api-2025-04-14" || call.AnthropicVersion != "2023-06-01" {
		t.Fatalf("anthropic-beta / anthropic-version 没有转发：%q / %q", call.AnthropicBeta, call.AnthropicVersion)
	}
	if call.ContentType != contentType || !bytes.Equal(call.Body, raw) {
		t.Fatalf("Anthropic 上传 multipart 应原样转发：ct=%q bytes=%d/%d", call.ContentType, len(call.Body), len(raw))
	}
	if call.Authorization != "Bearer "+deepseekToolsUpstreamKey {
		t.Fatalf("Anthropic 上游 Authorization 不对：%q", call.Authorization)
	}
	if call.ClientXAPIKey != "" {
		t.Fatalf("客户端的 x-api-key 不应转发到上游：%q", call.ClientXAPIKey)
	}

	// 列表：查询串原样转发，beta 头继续带上。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files?limit=20&after_id=file-api-abc123", nil, beta)
	if code != http.StatusOK || !strings.Contains(string(body), `"first_id":"file-api-abc123"`) {
		t.Fatalf("Anthropic Files 列表应 200，得到 %d；%s", code, body)
	}
	call = f.lastCall(t)
	if call.Method != "GET" || call.Path != "/anthropic/v1/files" || call.RawQuery != "limit=20&after_id=file-api-abc123" {
		t.Fatalf("Anthropic 列表查询串没有原样转发：%s %s?%s", call.Method, call.Path, call.RawQuery)
	}
	if call.AnthropicBeta != "files-api-2025-04-14" {
		t.Fatalf("列表请求的 anthropic-beta 没有转发：%q", call.AnthropicBeta)
	}

	// 查单个 / 删除文件。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files/file-api-abc123", nil, beta)
	if code != http.StatusOK || !strings.Contains(string(body), `"mime_type":"image/jpeg"`) {
		t.Fatalf("Anthropic 查文件应 200，得到 %d；%s", code, body)
	}
	if call = f.lastCall(t); call.Path != "/anthropic/v1/files/file-api-abc123" {
		t.Fatalf("Anthropic 文件路径没有透传：%s", call.Path)
	}
	code, _, body = doRaw(t, "DELETE", f.ts.URL+"/anthropic/v1/files/file-api-abc123", nil, beta)
	if code != http.StatusOK || !strings.Contains(string(body), `"type":"file_deleted"`) {
		t.Fatalf("Anthropic 删文件应 200，得到 %d；%s", code, body)
	}
	if call = f.lastCall(t); call.Method != "DELETE" || call.Path != "/anthropic/v1/files/file-api-abc123" {
		t.Fatalf("Anthropic 删除请求不对：%s %s", call.Method, call.Path)
	}

	// 缺 anthropic-beta：dsh-ee 不补也不拦，原样转给上游，由上游返回 400；错误体原样透传。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files", nil, bearer(token))
	if code != http.StatusBadRequest || !strings.Contains(string(body), "anthropic-beta") {
		t.Fatalf("缺 beta 头应透传上游 400，得到 %d；%s", code, body)
	}

	// Anthropic SDK 默认用 x-api-key 鉴权：等价于 Bearer，但不会转发给上游。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files", nil, map[string]string{
		"x-api-key":      token,
		"anthropic-beta": "files-api-2025-04-14",
	})
	if code != http.StatusOK {
		t.Fatalf("x-api-key 应等价 Bearer，得到 %d；%s", code, body)
	}
	if call = f.lastCall(t); call.ClientXAPIKey != "" {
		t.Fatalf("客户端的 x-api-key 不应转发到上游：%q", call.ClientXAPIKey)
	}

	// 非法 file_id：不转发上游，直接 400。
	for _, id := range []string{".hidden", "a%2Fb"} {
		before := f.callCount()
		code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files/"+id, nil, beta)
		if code != http.StatusBadRequest || openAIErrorCode(t, body) != "invalid_file_id" {
			t.Fatalf("非法 file_id %q 应 400 invalid_file_id，得到 %d；%s", id, code, body)
		}
		if f.callCount() != before {
			t.Fatalf("非法 file_id %q 不应转发到上游", id)
		}
	}

	// 无任何鉴权：401。
	code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files", nil, map[string]string{"anthropic-beta": "files-api-2025-04-14"})
	if code != http.StatusUnauthorized {
		t.Fatalf("Anthropic Files 无鉴权应 401，得到 %d；%s", code, body)
	}

	// 上传超过 100 MiB：413 且不打上游。
	before := f.callCount()
	code, body = rawContentLengthRequest(t, f.ts, "POST", "/anthropic/v1/files", token, deepseekFilesMaxBytesForTest+1)
	if code != http.StatusRequestEntityTooLarge || openAIErrorCode(t, body) != "request_too_large" {
		t.Fatalf("Anthropic 上传超限应 413 request_too_large，得到 %d；%s", code, body)
	}
	if f.callCount() != before {
		t.Fatal("Anthropic 上传超限请求不应转发到上游")
	}
}

// ---- 访问边界 / 聊天 file_id 拦截 / 审计 ----

func TestDeepSeekToolsAccessPolicy(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProviderModels(t, "deepseek-main", "deepseek", true, []string{"ds-chat"})
	admin := f.adminToken(t, []string{rbac.ScopeModels})
	bot := f.botToken(t, "ds-tools-bot", []string{rbac.ScopeModels})
	employee := f.token(t, "ds-tools-employee", []string{rbac.ScopeModels})
	employeeAllowed := f.startServerWith(t, "", true)

	// 默认只有管理员/机器人：普通员工调用文件与工具接口 403，且不打上游、不写文件审计。
	before := f.callCount()
	for _, tc := range []struct{ method, path string }{
		{"GET", "/v1/files"},
		{"POST", "/v1/tools/search"},
		{"GET", "/anthropic/v1/files"},
	} {
		headers := map[string]string{"Authorization": "Bearer " + employee, "Content-Type": "application/json"}
		code, _, body := doRaw(t, tc.method, f.ts.URL+tc.path, bytes.NewReader([]byte(`{"query":"x"}`)), headers)
		if code != http.StatusForbidden {
			t.Fatalf("员工调用 %s %s 应 403，得到 %d；%s", tc.method, tc.path, code, body)
		}
		if got := openAIErrorCode(t, body); got != "deepseek_tools_forbidden" {
			t.Fatalf("错误码 = %q，期望 deepseek_tools_forbidden；%s", got, body)
		}
	}
	if f.callCount() != before {
		t.Fatal("被访问边界拒绝的请求不应转发到上游")
	}
	if logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "files.", Limit: 20}); err != nil || len(logs) != 0 {
		t.Fatalf("被拒绝的员工请求不应写文件审计：logs=%d err=%v", len(logs), err)
	}

	// 管理员 / 机器人默认可用；员工在开启开关的实例上可用。
	for _, tc := range []struct {
		name, base, token string
	}{
		{"管理员", f.ts.URL, admin},
		{"机器人", f.ts.URL, bot},
		{"员工+allowEmployee", employeeAllowed.URL, employee},
	} {
		code, _, body := doRaw(t, "GET", tc.base+"/v1/files", nil, bearer(tc.token))
		if code != http.StatusOK {
			t.Fatalf("%s 应 200，得到 %d；%s", tc.name, code, body)
		}
	}

	// 聊天引用 file_id：员工默认 403 file_access_denied；管理员 / 机器人 / 开关打开后的员工放行。
	fileRef := map[string]any{
		"model": "ds-chat",
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "总结这个文件"},
			map[string]any{"type": "file", "file_id": "file-api-abc123"},
		}}},
	}
	plainChat := map[string]any{
		"model":    "ds-chat",
		"messages": []any{map[string]any{"role": "user", "content": "你好"}},
	}
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/v1/chat/completions", fileRef, bearer(employee))
	if code != http.StatusForbidden {
		t.Fatalf("员工引用 file_id 应 403，得到 %d；%s", code, body)
	}
	if got := openAIErrorCode(t, body); got != "file_access_denied" {
		t.Fatalf("错误码 = %q，期望 file_access_denied；%s", got, body)
	}
	// 不带 file_id 的聊天照常可用：只拦文件引用，不拦整个聊天。
	if code, body := c.do("POST", "/v1/chat/completions", plainChat, bearer(employee)); code != http.StatusOK {
		t.Fatalf("员工普通聊天应 200，得到 %d；%s", code, body)
	}
	for _, tc := range []struct {
		name, base, token string
	}{
		{"管理员引用", f.ts.URL, admin},
		{"机器人引用", f.ts.URL, bot},
		{"allowEmployee 员工引用", employeeAllowed.URL, employee},
	} {
		cc := newClient(t, tc.base)
		if code, body := cc.do("POST", "/v1/chat/completions", fileRef, bearer(tc.token)); code != http.StatusOK {
			t.Fatalf("%s 应 200，得到 %d；%s", tc.name, code, body)
		}
	}

	// Anthropic 风格 document 块（source.file_id 嵌套）同样被识别。
	nestedRef := map[string]any{
		"model": "ds-chat",
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "document", "source": map[string]any{"type": "file", "file_id": "file-api-abc123"}},
		}}},
	}
	code, body = c.do("POST", "/v1/chat/completions", nestedRef, bearer(employee))
	if code != http.StatusForbidden || openAIErrorCode(t, body) != "file_access_denied" {
		t.Fatalf("员工引用嵌套 document.file_id 应 403 file_access_denied，得到 %d；%s", code, body)
	}
}

func TestDeepSeekFilesAudit(t *testing.T) {
	f := newDeepSeekToolsFixture(t)
	f.addProvider(t, "deepseek-main", "deepseek", true)
	token := f.adminToken(t, []string{rbac.ScopeModels})
	header := bearer(token)

	// OpenAI 兼容：上传 / 删除 / 列表。
	raw, contentType := multipartBody(t, [][2]string{{"purpose", "user_data"}}, "file", "image.jpg", []byte("fake-image-bytes"))
	code, _, body := doRaw(t, "POST", f.ts.URL+"/v1/files", bytes.NewReader(raw), map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": contentType,
	})
	if code != http.StatusOK {
		t.Fatalf("上传文件应 200，得到 %d；%s", code, body)
	}
	if code, _, body = doRaw(t, "DELETE", f.ts.URL+"/v1/files/file-api-abc123", nil, header); code != http.StatusOK {
		t.Fatalf("删除文件应 200，得到 %d；%s", code, body)
	}
	if code, _, body = doRaw(t, "GET", f.ts.URL+"/v1/files?purpose=user_data", nil, header); code != http.StatusOK {
		t.Fatalf("列文件应 200，得到 %d；%s", code, body)
	}

	// Anthropic 兼容：上传 / 删除 / 列表（beta 头带上才放行）。
	anthRaw, anthCT := multipartBody(t, nil, "file", "image.jpg", []byte("fake-image-bytes"))
	betaHeader := map[string]string{
		"Authorization":  "Bearer " + token,
		"anthropic-beta": "files-api-2025-04-14",
	}
	code, _, body = doRaw(t, "POST", f.ts.URL+"/anthropic/v1/files", bytes.NewReader(anthRaw), map[string]string{
		"Authorization":  "Bearer " + token,
		"anthropic-beta": "files-api-2025-04-14",
		"Content-Type":   anthCT,
	})
	if code != http.StatusOK {
		t.Fatalf("Anthropic 上传应 200，得到 %d；%s", code, body)
	}
	if code, _, body = doRaw(t, "DELETE", f.ts.URL+"/anthropic/v1/files/file-api-abc123", nil, betaHeader); code != http.StatusOK {
		t.Fatalf("Anthropic 删除应 200，得到 %d；%s", code, body)
	}
	if code, _, body = doRaw(t, "GET", f.ts.URL+"/anthropic/v1/files", nil, betaHeader); code != http.StatusOK {
		t.Fatalf("Anthropic 列表应 200，得到 %d；%s", code, body)
	}

	logs, err := f.st.ListAudit(context.Background(), store.AuditFilter{Action: "files.", Limit: 20})
	if err != nil {
		t.Fatalf("读取审计日志: %v", err)
	}
	if len(logs) != 6 {
		t.Fatalf("应写 6 条文件审计，实际 %d", len(logs))
	}
	type auditKey struct{ action, api string }
	got := map[auditKey]*store.AuditLog{}
	for _, l := range logs {
		api, _ := l.Detail["api"].(string)
		got[auditKey{l.Action, api}] = l
		// 详情只允许 api / status；绝不记录文件名、文件内容或识别文本。
		for k := range l.Detail {
			if k != "api" && k != "status" {
				t.Fatalf("审计详情不应有 %q：%+v", k, l.Detail)
			}
		}
		if strings.Contains(l.Target, "image.jpg") || strings.Contains(l.Target, "fake-image-bytes") {
			t.Fatalf("审计对象不应包含文件名/内容：%q", l.Target)
		}
	}
	check := func(action, api, target string) {
		t.Helper()
		l := got[auditKey{action, api}]
		if l == nil {
			t.Fatalf("缺少审计 %s/%s（实际 %+v）", action, api, got)
		}
		if l.Target != target {
			t.Fatalf("%s/%s 的 target = %q，期望 %q", action, api, l.Target, target)
		}
		if l.ActorName != "admin" {
			t.Fatalf("%s/%s 的操作人 = %q，期望 admin", action, api, l.ActorName)
		}
		if l.TokenID == nil {
			t.Fatalf("%s/%s 应记录令牌 id", action, api)
		}
		if l.Via != "token" {
			t.Fatalf("%s/%s 的来源 = %q，期望 token", action, api, l.Via)
		}
		if status, _ := l.Detail["status"].(float64); status != http.StatusOK {
			t.Fatalf("%s/%s 的状态 = %v，期望 200", action, api, l.Detail["status"])
		}
	}
	check("files.upload", "openai", "file-api-abc123")
	check("files.delete", "openai", "file-api-abc123")
	check("files.list", "openai", "files")
	check("files.upload", "anthropic", "file-api-abc123")
	check("files.delete", "anthropic", "file-api-abc123")
	check("files.list", "anthropic", "files")
}
