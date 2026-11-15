package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 第三方 Harness 的中立 v1 契约：公开发现文档、能力摘要、只读微应用、API 版本头。
// 这里只走 HTTP，不依赖 DSH 前端；令牌就是设备码 / 网页签发的 ee_at_。

// harnessVersion 固定测试服务的构建版本，发现文档与能力摘要都读它。
const harnessVersion = "test-1.0.0"

// harnessFixture 起一个初始化好管理员、版本号固定的测试服务（数据库用例）。
func harnessFixture(t *testing.T) (*httptest.Server, *client) {
	t.Helper()
	return harnessFixtureWith(t, nil)
}

// harnessFixtureWith 允许测试调整 api.Options（例如 Chatbot 注入开关、版本号）。
func harnessFixtureWith(t *testing.T, mutate func(*api.Options)) (*httptest.Server, *client) {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	opts := api.Options{DataDir: t.TempDir(), Version: harnessVersion}
	if mutate != nil {
		mutate(&opts)
	}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	return ts, admin
}

// harnessDo 发一个请求；header 可为 nil，返回状态码、响应头与原始响应体。
func harnessDo(t *testing.T, method, url string, header map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

// harnessGet 发一个匿名 / 带令牌的 GET。
func harnessGet(t *testing.T, url string, header map[string]string) (int, http.Header, []byte) {
	t.Helper()
	return harnessDo(t, http.MethodGet, url, header)
}

// harnessLoginUser 建一个员工并登录，返回它的网页客户端。
func harnessLoginUser(t *testing.T, ts *httptest.Server, admin *client, username string) *client {
	t.Helper()
	code, body := admin.do("POST", "/api/admin/users", map[string]any{
		"username": username, "password": username + "-password-1", "department": "研发效能部"}, nil)
	expect(t, "建用户 "+username, code, 201, body)
	c := newClient(t, ts.URL)
	code, body = c.do("POST", "/api/auth/login", map[string]string{"username": username, "password": username + "-password-1"}, nil)
	expect(t, "登录 "+username, code, 200, body)
	return c
}

// harnessIssueToken 用网页会话签发一个接入令牌，返回明文与 info.id。
func harnessIssueToken(t *testing.T, c *client, name string, scopes []string) (string, string) {
	t.Helper()
	code, body := c.do("POST", "/api/me/tokens", map[string]any{"name": name, "scopes": scopes}, nil)
	expect(t, "签发令牌 "+name, code, 201, body)
	issued := decode[struct {
		Token string `json:"token"`
		Info  struct {
			ID string `json:"id"`
		} `json:"info"`
	}](t, body)
	if !strings.HasPrefix(issued.Token, "ee_at_") || issued.Info.ID == "" {
		t.Fatalf("令牌响应不对：%s", body)
	}
	return issued.Token, issued.Info.ID
}

// harnessErrorView 是中立错误壳里的 error 对象。
type harnessErrorView struct {
	Type              string   `json:"type"`
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	RequestID         string   `json:"request_id"`
	SupportedVersions []string `json:"supported_versions"`
}

// harnessError 解出中立错误壳，并检查必填的 message 不为空。
func harnessError(t *testing.T, body []byte) harnessErrorView {
	t.Helper()
	var v struct {
		Error harnessErrorView `json:"error"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("错误响应不是 JSON：%v；%s", err, body)
	}
	if v.Error.Message == "" {
		t.Fatalf("错误响应缺少 message：%s", body)
	}
	return v.Error
}

// TestHarnessDiscovery 覆盖公开发现文档：匿名 200、字段/入口、ETag/max-age、条件请求、版本协商。
func TestHarnessDiscovery(t *testing.T) {
	ts, _ := harnessFixture(t)
	url := ts.URL + "/.well-known/dsh-ee"

	code, header, body := harnessGet(t, url, nil)
	expect(t, "匿名读发现文档", code, 200, body)
	if got := header.Get("Dsh-EE-API-Version"); got != "1" {
		t.Fatalf("发现文档应带 Dsh-EE-API-Version: 1，得到 %q", got)
	}
	if header.Get("X-Request-Id") == "" {
		t.Fatal("中立端点响应应带 X-Request-Id")
	}
	if cc := header.Get("Cache-Control"); !strings.Contains(cc, "private") || !strings.Contains(cc, "max-age=300") || strings.Contains(cc, "public") {
		t.Fatalf("发现文档应只允许私有缓存（端点地址按请求推断）：%q", cc)
	}
	etag := header.Get("ETag")
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) || len(etag) < 8 {
		t.Fatalf("ETag 不对：%q", etag)
	}

	doc := decode[harnessDiscoveryView](t, body)

	if doc.Name != "dsh-ee" || doc.Version != harnessVersion || doc.APIVersion != "1" {
		t.Fatalf("发现文档标识不对：%+v", doc)
	}
	// 鉴权与令牌获取方式：唯一线上凭证是 Bearer ee_at_，可通过设备码流程或网页生成。
	if doc.Auth.Type != "bearer" || !strings.Contains(doc.Auth.Header, "Authorization: Bearer") ||
		!slices.Equal(doc.Auth.TokenTypes, []string{"ee_at_"}) || !slices.Equal(doc.Auth.TokenFlows, []string{"device_code", "web_manual"}) {
		t.Fatalf("auth 说明不对：%+v", doc.Auth)
	}
	if doc.DeviceAuth != ts.URL+"/api/device/code" || doc.TokenURL != ts.URL+"/api/device/token" {
		t.Fatalf("设备码端点不对：%+v", doc)
	}
	wantScopes := []string{"models", "registry:read", "registry:publish", "hub", "connections", "agent"}
	if !slices.Equal(doc.Scopes, wantScopes) {
		t.Fatalf("scopes_supported 不对：%v", doc.Scopes)
	}
	// 模型 / MCP / 微应用入口：发现文档必须直接给出实际地址，第三方不用再猜路径。
	if doc.Models.Protocol != "openai-chat-completions" || !doc.Models.Streaming ||
		doc.Models.Endpoint != ts.URL+"/v1/models" || doc.Models.ChatEndpoint != ts.URL+"/v1/chat/completions" {
		t.Fatalf("models 入口不对：%+v", doc.Models)
	}
	if doc.Models.AnthropicEndpoint != ts.URL+"/v1/messages" ||
		!slices.Contains(doc.Models.AnthropicAliases, ts.URL+"/anthropic/v1/messages") ||
		!slices.Contains(doc.Models.NativeVendors, "deepseek") ||
		doc.Models.ConversionFallback != "openai-chat" {
		t.Fatalf("Anthropic 兼容入口/原生透传说明不对：%+v", doc.Models)
	}
	if doc.MCP.Transport != "streamable-http" || doc.MCP.Endpoint != ts.URL+"/mcp" || !slices.Contains(doc.MCP.ProtocolVersions, "2025-11-25") {
		t.Fatalf("mcp 入口不对：%+v", doc.MCP)
	}
	if !doc.Apps.Web || !doc.Apps.DataTools || doc.Apps.Endpoint != ts.URL+"/v1/apps" || doc.Apps.WebURL != ts.URL+"/a/{slug}" {
		t.Fatalf("apps 入口不对：%+v", doc.Apps)
	}
	// Chatbot 能力：默认不注入，但声明透传服务端工具/安全/标题/拒绝与响应字段。
	// 能力依赖上游 DeepSeek，dsh-ee 不实现服务端工具，用 upstream_vendor + note 说清楚。
	cb := doc.Chatbot
	if cb.UpstreamVendor != "deepseek" || !cb.Passthrough || !cb.Refusal {
		t.Fatalf("chatbot 能力声明不对：%+v", cb)
	}
	if !slices.Equal(cb.ServerTools, []string{"deepseek_web_search", "deepseek_read_url"}) {
		t.Fatalf("chatbot server_tools 不对：%v", cb.ServerTools)
	}
	if !cb.SafetyCheck.Supported || cb.SafetyCheck.Injected || cb.SafetyCheck.Level != "" || !cb.Title.Supported || cb.Title.Injected {
		t.Fatalf("chatbot 默认不应注入 safety_check/title：%+v", cb)
	}
	for _, field := range []string{"title", "citations", "refusal", "server_tool_use", "server_tool_calls"} {
		if !slices.Contains(cb.ResponseFields, field) {
			t.Fatalf("chatbot response_fields 缺少 %q：%v", field, cb.ResponseFields)
		}
	}
	if !strings.Contains(cb.Note, "DeepSeek") {
		t.Fatalf("chatbot note 应说明依赖上游 DeepSeek：%q", cb.Note)
	}
	if doc.CapabilitiesEndpoint != ts.URL+"/v1/capabilities" || doc.OpenAPIURL != ts.URL+"/v1/openapi.json" {
		t.Fatalf("能力/契约入口不对：capabilities=%q openapi=%q", doc.CapabilitiesEndpoint, doc.OpenAPIURL)
	}
	if doc.Limits.MaxRequestBytes != 16<<20 || doc.Limits.MaxResponseBytes != 32<<20 || doc.Limits.MaxMCPRequestBytes != 4<<20 {
		t.Fatalf("limits 不对：%+v", doc.Limits)
	}
	// 错误格式与稳定错误码，且文档 URL 指向可读的机器契约。
	if !strings.Contains(doc.Errors.Format, `"error"`) || doc.Errors.DocumentationURL != doc.OpenAPIURL {
		t.Fatalf("errors 说明不对：%+v", doc.Errors)
	}
	for _, code := range []string{"token_invalid", "scope_denied", "unsupported_api_version", "method_not_allowed", "not_found", "internal_error"} {
		if !slices.Contains(doc.Errors.Codes, code) {
			t.Fatalf("errors.codes 缺少 %q：%v", code, doc.Errors.Codes)
		}
	}
	// v1 没有弃用计划：不再声明服务端从不发送的 Deprecation/Sunset 头（避免虚假承诺）。
	if len(doc.Deprecation) != 0 {
		t.Fatalf("发现文档不应再声明 deprecation：%s", doc.Deprecation)
	}

	// 发现文档里的契约入口必须能匿名读到（openapi_url 直接可用）。
	code, header, body = harnessGet(t, doc.OpenAPIURL, nil)
	expect(t, "按发现文档读 OpenAPI", code, 200, body)
	if header.Get("Dsh-EE-API-Version") != "1" {
		t.Fatalf("OpenAPI 端点应带版本头：%v", header)
	}

	// 同一份文档 ETag 稳定；带 If-None-Match 时 304，且不再返 body。
	_, header2, _ := harnessGet(t, url, nil)
	if header2.Get("ETag") != etag {
		t.Fatalf("同一文档 ETag 应稳定：%q vs %q", etag, header2.Get("ETag"))
	}
	code, header3, body3 := harnessGet(t, url, map[string]string{"If-None-Match": etag})
	if code != http.StatusNotModified || len(body3) != 0 {
		t.Fatalf("条件请求应 304 且无 body：%d %s", code, body3)
	}
	if header3.Get("Dsh-EE-API-Version") != "1" || header3.Get("ETag") != etag {
		t.Fatalf("304 也要带版本头与 ETag：%v", header3)
	}
	if header3.Get("Content-Type") != "" {
		t.Fatalf("304 不应带 Content-Type：%v", header3)
	}

	// 请求版本头：当前主版本与「1.x」都接受；不支持的主版本 400 并给出 supported_versions。
	if code, _, body := harnessGet(t, url, map[string]string{"Dsh-EE-API-Version": "1.0.0"}); code != 200 {
		t.Fatalf("1.x 应接受：%d %s", code, body)
	}
	code, header4, body4 := harnessGet(t, url, map[string]string{"Dsh-EE-API-Version": "2"})
	if code != http.StatusBadRequest {
		t.Fatalf("不支持的主版本应 400：%d %s", code, body4)
	}
	if header4.Get("Dsh-EE-API-Version") != "1" {
		t.Fatalf("400 响应也要带服务端版本头：%v", header4)
	}
	e := harnessError(t, body4)
	if e.Type != "invalid_request_error" || e.Code != "unsupported_api_version" || !slices.Equal(e.SupportedVersions, []string{"1"}) {
		t.Fatalf("不支持的版本错误不对：%+v", e)
	}
	if e.RequestID == "" || e.RequestID != header4.Get("X-Request-Id") {
		t.Fatalf("错误 request_id 应与响应头一致：%+v", e)
	}

	// 只支持 GET：405 也是统一错误壳，并带 Allow 与版本头。
	code, header, body = harnessDo(t, http.MethodPost, url, nil)
	expect(t, "发现文档只支持 GET", code, 405, body)
	if header.Get("Allow") != "GET, HEAD" || header.Get("Dsh-EE-API-Version") != "1" || header.Get("X-Request-Id") == "" {
		t.Fatalf("405 响应头不对：%v", header)
	}
	if e := harnessError(t, body); e.Type != "invalid_request_error" || e.Code != "method_not_allowed" {
		t.Fatalf("405 错误壳不对：%+v", e)
	}

	// 既有入口的响应形状不变：/healthz 不带版本头，/v1/models 的 401 仍是 OpenAI 壳。
	code, header, body = harnessGet(t, ts.URL+"/healthz", nil)
	expect(t, "healthz", code, 200, body)
	if header.Get("Dsh-EE-API-Version") != "" {
		t.Fatalf("/healthz 不应带中立 API 版本头：%v", header)
	}
	code, header, body = harnessGet(t, ts.URL+"/v1/models", nil)
	expect(t, "未带令牌读模型", code, 401, body)
	if header.Get("Dsh-EE-API-Version") != "1" {
		t.Fatalf("/v1 面应带版本响应头：%v", header)
	}
	openai := decode[struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}](t, body)
	if openai.Error.Message == "" {
		t.Fatalf("/v1/models 的 401 应保持 OpenAI 错误壳：%s", body)
	}
	// /v1 下未知路径仍是 JSON 404，并带版本头（契约覆盖整个 /v1 面）。
	code, header, body = harnessGet(t, ts.URL+"/v1/does-not-exist", nil)
	expect(t, "未知 /v1 路径", code, 404, body)
	if header.Get("Dsh-EE-API-Version") != "1" {
		t.Fatalf("未知 /v1 路径也应带版本头：%v", header)
	}
}

// TestHarnessChatbotInjection 覆盖开启注入后的能力声明：发现文档与能力摘要都要如实
// 报告本部署会注入的 safety_check / title，客户端才能判断上游请求里会不会多出字段。
func TestHarnessChatbotInjection(t *testing.T) {
	ts, _ := harnessFixtureWith(t, func(o *api.Options) {
		o.SafetyCheckLevel = "high"
		o.ChatTitle = true
	})

	code, _, body := harnessGet(t, ts.URL+"/.well-known/dsh-ee", nil)
	expect(t, "读发现文档", code, 200, body)
	disc := decode[harnessDiscoveryView](t, body)
	if !disc.Chatbot.SafetyCheck.Injected || disc.Chatbot.SafetyCheck.Level != "high" {
		t.Fatalf("safety_check 注入声明不对：%+v", disc.Chatbot.SafetyCheck)
	}
	if !disc.Chatbot.Title.Injected {
		t.Fatalf("title 注入声明不对：%+v", disc.Chatbot.Title)
	}

	code, _, body = harnessGet(t, ts.URL+"/v1/capabilities", nil)
	expect(t, "匿名能力摘要", code, 200, body)
	caps := decode[capabilitiesView](t, body)
	if !caps.Chatbot.SafetyCheck.Injected || caps.Chatbot.SafetyCheck.Level != "high" || !caps.Chatbot.Title.Injected {
		t.Fatalf("能力摘要里的注入声明应与发现文档一致：%+v", caps.Chatbot)
	}
}

// chatbotCapView 是发现文档与 /v1/capabilities 里 chatbot 能力声明的形状（测试用）。
type chatbotCapView struct {
	UpstreamVendor string   `json:"upstream_vendor"`
	Passthrough    bool     `json:"passthrough"`
	ServerTools    []string `json:"server_tools"`
	SafetyCheck    struct {
		Supported bool   `json:"supported"`
		Injected  bool   `json:"injected"`
		Level     string `json:"level"`
	} `json:"safety_check"`
	Title struct {
		Supported bool `json:"supported"`
		Injected  bool `json:"injected"`
	} `json:"title"`
	Refusal        bool     `json:"refusal"`
	ResponseFields []string `json:"response_fields"`
	Note           string   `json:"note"`
}

// harnessDiscoveryView 是发现文档的响应形状（测试用）。
type harnessDiscoveryView struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	Auth       struct {
		Type       string   `json:"type"`
		Header     string   `json:"header"`
		TokenTypes []string `json:"token_types"`
		TokenFlows []string `json:"token_flows"`
	} `json:"auth"`
	DeviceAuth string   `json:"device_authorization_endpoint"`
	TokenURL   string   `json:"token_endpoint"`
	Scopes     []string `json:"scopes_supported"`
	Models     struct {
		Protocol           string   `json:"protocol"`
		Streaming          bool     `json:"streaming"`
		Endpoint           string   `json:"endpoint"`
		ChatEndpoint       string   `json:"chat_completions_endpoint"`
		AnthropicEndpoint  string   `json:"anthropic_messages_endpoint"`
		AnthropicAliases   []string `json:"anthropic_messages_aliases"`
		NativeVendors      []string `json:"anthropic_messages_native_vendors"`
		ConversionFallback string   `json:"anthropic_messages_conversion_fallback"`
	} `json:"models"`
	MCP struct {
		Transport        string   `json:"transport"`
		Endpoint         string   `json:"endpoint"`
		ProtocolVersions []string `json:"protocol_versions"`
	} `json:"mcp"`
	Apps struct {
		Web       bool   `json:"web"`
		DataTools bool   `json:"data_tools"`
		Endpoint  string `json:"endpoint"`
		WebURL    string `json:"web_url"`
	} `json:"apps"`
	Chatbot              chatbotCapView `json:"chatbot"`
	CapabilitiesEndpoint string         `json:"capabilities_endpoint"`
	OpenAPIURL           string         `json:"openapi_url"`
	Limits               struct {
		MaxRequestBytes    int64 `json:"max_request_bytes"`
		MaxResponseBytes   int64 `json:"max_response_bytes"`
		MaxMCPRequestBytes int64 `json:"max_mcp_request_bytes"`
	} `json:"limits"`
	Errors struct {
		Format           string   `json:"format"`
		DocumentationURL string   `json:"documentation_url"`
		Codes            []string `json:"codes"`
	} `json:"errors"`
	// v1 没有弃用计划，字段应完全缺失。
	Deprecation json.RawMessage `json:"deprecation"`
}

// assertRefsResolve 检查 JSON 里所有 $ref 都能解析到文档内的定义，防止静态契约写错引用。
func assertRefsResolve(t *testing.T, spec any) {
	t.Helper()
	var walk func(v any, where string)
	walk = func(v any, where string) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatalf("%s: 只支持文档内 $ref，得到 %q", where, ref)
				}
				var cur any = spec
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					// JSON Pointer 转义：~1 是 "/"，~0 是 "~"（path item 的 $ref 会用到）。
					token := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
					m, ok := cur.(map[string]any)
					if !ok {
						t.Fatalf("%s: $ref %q 路径不是对象", where, ref)
					}
					if cur, ok = m[token]; !ok {
						t.Fatalf("%s: $ref %q 无法解析", where, ref)
					}
				}
			}
			for k, vv := range x {
				walk(vv, where+"/"+k)
			}
		case []any:
			for i, vv := range x {
				walk(vv, fmt.Sprintf("%s/%d", where, i))
			}
		}
	}
	walk(spec, "#")
}

// TestHarnessOpenAPI 覆盖静态机器契约：匿名只读、ETag/缓存、版本头、OpenAPI 3.1 结构与入口一致性。
func TestHarnessOpenAPI(t *testing.T) {
	ts, _ := harnessFixture(t)
	url := ts.URL + "/v1/openapi.json"

	code, header, body := harnessGet(t, url, nil)
	expect(t, "匿名读 OpenAPI", code, 200, body)
	if ct := header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("OpenAPI Content-Type 不对：%q", ct)
	}
	if header.Get("Dsh-EE-API-Version") != "1" || header.Get("X-Request-Id") == "" {
		t.Fatalf("OpenAPI 端点应带版本头与请求号：%v", header)
	}
	if cc := header.Get("Cache-Control"); !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=300") {
		t.Fatalf("OpenAPI 是静态内容，应允许共享缓存：%q", cc)
	}
	etag := header.Get("ETag")
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Fatalf("OpenAPI ETag 不对：%q", etag)
	}
	_, header2, _ := harnessGet(t, url, nil)
	if header2.Get("ETag") != etag {
		t.Fatalf("OpenAPI ETag 应稳定：%q vs %q", etag, header2.Get("ETag"))
	}
	code, header3, body3 := harnessGet(t, url, map[string]string{"If-None-Match": etag})
	if code != http.StatusNotModified || len(body3) != 0 {
		t.Fatalf("OpenAPI 条件请求应 304 且无 body：%d %s", code, body3)
	}
	if header3.Get("Dsh-EE-API-Version") != "1" || header3.Get("ETag") != etag {
		t.Fatalf("304 也要带版本头与 ETag：%v", header3)
	}

	// 必须是可解析的 OpenAPI 3.1 文档，覆盖现有中立/网关端点与统一错误壳。
	var spec struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
			Schemas         map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("OpenAPI 不是合法 JSON：%v", err)
	}
	if spec.OpenAPI != "3.1.0" || spec.Info.Title == "" || spec.Info.Version == "" {
		t.Fatalf("OpenAPI 头信息不对：%+v", spec)
	}
	for _, path := range []string{
		"/.well-known/dsh-ee", "/v1/capabilities", "/v1/models", "/v1/chat/completions", "/v1/messages",
		"/anthropic/v1/messages", "/v1/apps", "/v1/apps/{name}", "/v1/openapi.json",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("OpenAPI 缺少路径 %s（现有 %v）", path, len(spec.Paths))
		}
	}
	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Fatal("OpenAPI 缺少 bearerAuth 安全定义")
	}
	for _, schema := range []string{"Discovery", "Capabilities", "App", "Error", "ErrorDetail", "SkillSummary"} {
		if _, ok := spec.Components.Schemas[schema]; !ok {
			t.Fatalf("OpenAPI 缺少 schema %q", schema)
		}
	}
	// 所有 $ref 都必须能解析到 components 里的定义（手工维护的静态契约最容易在这里写错）。
	var anySpec any
	if err := json.Unmarshal(body, &anySpec); err != nil {
		t.Fatalf("OpenAPI 不是合法 JSON：%v", err)
	}
	assertRefsResolve(t, anySpec)

	// 发现文档公布的入口必须与 OpenAPI 的 paths 对得上（MCP / 设备码 / 网页 URL 不在 OpenAPI 内）。
	code, _, discBody := harnessGet(t, ts.URL+"/.well-known/dsh-ee", nil)
	expect(t, "读发现文档", code, 200, discBody)
	disc := decode[harnessDiscoveryView](t, discBody)
	paths := map[string]bool{}
	for _, u := range append([]string{
		disc.Models.Endpoint, disc.Models.ChatEndpoint, disc.Models.AnthropicEndpoint, disc.Apps.Endpoint,
		disc.CapabilitiesEndpoint, disc.OpenAPIURL,
	}, disc.Models.AnthropicAliases...) {
		path := strings.TrimPrefix(u, ts.URL)
		if path == u {
			t.Fatalf("发现文档入口应是绝对地址：%q", u)
		}
		paths[path] = true
	}
	paths["/.well-known/dsh-ee"] = true
	for path := range paths {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("发现文档入口 %q 不在 OpenAPI paths 里", path)
		}
	}
	if disc.Errors.DocumentationURL != disc.OpenAPIURL {
		t.Fatalf("错误码文档 URL 应指向 OpenAPI：%q", disc.Errors.DocumentationURL)
	}

	// OpenAPI 端点同样支持请求版本协商与统一 405。
	if code, _, errBody := harnessGet(t, url, map[string]string{"Dsh-EE-API-Version": "2"}); code != http.StatusBadRequest {
		t.Fatalf("OpenAPI 不支持的主版本应 400：%d %s", code, errBody)
	}
	code, header, errBody := harnessDo(t, http.MethodPost, url, nil)
	expect(t, "OpenAPI 只支持 GET", code, 405, errBody)
	if header.Get("Allow") != "GET, HEAD" {
		t.Fatalf("OpenAPI 405 应带 Allow：%v", header)
	}
	if e := harnessError(t, errBody); e.Code != "method_not_allowed" {
		t.Fatalf("OpenAPI 405 错误码不对：%+v", e)
	}
}

// TestHarnessCapabilities 覆盖 /v1/capabilities：匿名公开子集、鉴权失败、按令牌收窄的模型/工具/应用。
func TestHarnessCapabilities(t *testing.T) {
	ts, admin := harnessFixture(t)

	// 一个对外模型，一个全员自动生效的微应用，一个自选（未启用）的微应用。
	code, body := admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "harness-llm", "vendor": "custom", "base_url": "http://127.0.0.1:1/v1",
		"api_key": "sk-harness-upstream", "models": []string{"harness-chat"}}, nil)
	expect(t, "建模型上游", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "app", "slug": "expense", "delivery": "auto",
		"spec": map[string]any{"title": "报销助手", "description": "报销政策与流程", "persona": "你是报销助手。",
			"icon": "🧾", "starters": []string{"怎么报销"}}}, nil)
	expect(t, "建自动生效微应用", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "app", "slug": "optional-app", "delivery": "optional",
		"spec": map[string]any{"title": "自选应用", "description": "员工自己启用", "persona": "你是自选应用。"}}, nil)
	expect(t, "建自选微应用", code, 201, body)
	// 技能与提示词：能力摘要里应给元数据摘要（正文不在摘要里）。
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "policy-skill", "delivery": "auto",
		"spec": map[string]any{"title": "报销制度", "description": "查报销标准", "body": "住宿每晚 1300 元以内。"}}, nil)
	expect(t, "建技能", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "prompt", "slug": "weekly-prompt", "delivery": "auto",
		"spec": map[string]any{"title": "周报", "description": "整理周报", "body": "按项目整理本周进展。"}}, nil)
	expect(t, "建提示词", code, 201, body)

	bob := harnessLoginUser(t, ts, admin, "bob")
	token, _ := harnessIssueToken(t, bob, "harness", []string{"models", "hub", "connections", "agent"})

	// 匿名：公开子集，用户数据为空，但结构完整。
	code, header, body := harnessGet(t, ts.URL+"/v1/capabilities", nil)
	expect(t, "匿名能力摘要", code, 200, body)
	if header.Get("Dsh-EE-API-Version") != "1" || header.Get("X-Request-Id") == "" {
		t.Fatalf("能力摘要应带版本头与请求号：%v", header)
	}
	if header.Get("Cache-Control") != "no-store" {
		t.Fatalf("用户相关的能力摘要不应被缓存：%v", header)
	}
	anon := decode[capabilitiesView](t, body)
	if anon.APIVersion != "1" || anon.Authenticated || anon.User != nil {
		t.Fatalf("匿名摘要不对：%+v", anon)
	}
	if anon.Scopes == nil || anon.Models == nil || anon.Apps == nil || anon.Skills == nil || anon.Revision != "" || anon.Tools.Count != 0 {
		t.Fatalf("匿名摘要的用户数据应全为空：%s", body)
	}
	if anon.Server.Name != "dsh-ee" || anon.Server.Version != harnessVersion {
		t.Fatalf("匿名摘要也应带服务标识：%+v", anon.Server)
	}
	// Chatbot 能力是服务端级声明，匿名也可读；默认不注入，注入状态跟启动配置走。
	if anon.Chatbot.UpstreamVendor != "deepseek" || !anon.Chatbot.Passthrough ||
		anon.Chatbot.SafetyCheck.Injected || anon.Chatbot.Title.Injected {
		t.Fatalf("匿名能力摘要的 chatbot 声明不对：%+v", anon.Chatbot)
	}

	// 令牌不合法：401 + 中立错误壳；带错凭证不会静默退化成匿名。
	code, header, body = harnessGet(t, ts.URL+"/v1/capabilities", bearer("nope"))
	expect(t, "非法令牌", code, 401, body)
	if header.Get("WWW-Authenticate") == "" {
		t.Fatal("401 应带 WWW-Authenticate")
	}
	if e := harnessError(t, body); e.Type != "authentication_error" || e.Code != "token_invalid" {
		t.Fatalf("非法令牌错误不对：%+v", e)
	}
	code, _, body = harnessGet(t, ts.URL+"/v1/capabilities", map[string]string{"Authorization": "Basic abc"})
	expect(t, "非 Bearer 凭证", code, 401, body)
	if e := harnessError(t, body); e.Code != "token_invalid" {
		t.Fatalf("非 Bearer 凭证错误不对：%+v", e)
	}

	// 带令牌：模型、能力目录（应用 + revision）、MCP 工具计数都与既有入口一致。
	code, _, body = harnessGet(t, ts.URL+"/v1/capabilities", bearer(token))
	expect(t, "令牌能力摘要", code, 200, body)
	caps := decode[capabilitiesView](t, body)
	if !caps.Authenticated || caps.User == nil || caps.User.Username != "bob" || caps.User.Department != "研发效能部" {
		t.Fatalf("账号信息不对：%+v", caps)
	}
	if !slices.Equal(caps.Scopes, []string{"agent", "connections", "hub", "models"}) {
		t.Fatalf("令牌 scope 不对：%v", caps.Scopes)
	}
	if !slices.Equal(caps.Models, []string{"harness-chat"}) {
		t.Fatalf("可用模型不对：%v", caps.Models)
	}
	if len(caps.Apps) != 1 || caps.Apps[0].Name != "expense" {
		t.Fatalf("可用微应用不对（不应含系统应用/未启用的自选应用）：%+v", caps.Apps)
	}
	if caps.Apps[0].Title != "报销助手" || caps.Apps[0].Icon != "🧾" || !slices.Equal(caps.Apps[0].Starters, []string{"怎么报销"}) {
		t.Fatalf("微应用描述不对：%+v", caps.Apps[0])
	}
	// 技能 / 提示词摘要：只有元数据，不含微应用与 DSH 系统技能。
	skills := map[string]string{}
	for _, sk := range caps.Skills {
		skills[sk.Name] = sk.Type
	}
	if skills["policy-skill"] != "skill" || skills["weekly-prompt"] != "prompt" {
		t.Fatalf("技能/提示词摘要不对：%+v", caps.Skills)
	}
	for _, absent := range []string{"expense", "workbench", "kb-qa", "dsh-ee-guide", "dsh-ee-doctor"} {
		if _, ok := skills[absent]; ok {
			t.Fatalf("技能摘要不应包含 %q：%+v", absent, caps.Skills)
		}
	}
	if caps.Revision == "" {
		t.Fatal("带令牌的摘要应有目录 revision")
	}
	if caps.Tools.Count <= 0 {
		t.Fatalf("带 connections scope 的令牌应能看到工具计数：%+v", caps.Tools)
	}

	// 交叉校验：模型列表等于 /v1/models，工具数等于 /mcp tools/list，revision 等于 DSH 目录。
	code, _, modelsBody := harnessGet(t, ts.URL+"/v1/models", bearer(token))
	expect(t, "读模型列表", code, 200, modelsBody)
	models := decode[struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}](t, modelsBody)
	var modelIDs []string
	for _, m := range models.Data {
		modelIDs = append(modelIDs, m.ID)
	}
	if !slices.Equal(modelIDs, caps.Models) {
		t.Fatalf("能力摘要模型 %v 与 /v1/models %v 不一致", caps.Models, modelIDs)
	}
	dsh := dshClient{t: t, base: ts.URL, token: token}
	tools := dsh.rpc("tools/list", nil)
	listed := tools["result"].(map[string]any)["tools"].([]any)
	if caps.Tools.Count != len(listed) {
		var names []string
		for _, item := range listed {
			names = append(names, item.(map[string]any)["name"].(string))
		}
		t.Fatalf("能力摘要工具数 %d 与 /mcp tools/list %d 不一致：%v", caps.Tools.Count, len(listed), names)
	}
	catalog := dsh.catalog()
	if caps.Revision != catalog.Revision {
		t.Fatalf("能力摘要 revision 与目录不一致：%q vs %q", caps.Revision, catalog.Revision)
	}

	// 只有 models scope 的令牌：模型可用，目录/工具为空（不越权聚合）。
	modelsOnly, _ := harnessIssueToken(t, bob, "models-only", []string{"models"})
	code, _, body = harnessGet(t, ts.URL+"/v1/capabilities", bearer(modelsOnly))
	expect(t, "models-only 能力摘要", code, 200, body)
	mo := decode[capabilitiesView](t, body)
	if !slices.Equal(mo.Models, []string{"harness-chat"}) || len(mo.Apps) != 0 || len(mo.Skills) != 0 || mo.Tools.Count != 0 || mo.Revision != "" {
		t.Fatalf("models-only 令牌不应聚合目录能力：%+v", mo)
	}
	if mo.Apps == nil || mo.Scopes == nil || mo.Skills == nil {
		t.Fatalf("空数组不应是 null：%s", body)
	}

	// 吊销后的令牌 401，错误码区分 token_revoked。
	revokedToken, revokedID := harnessIssueToken(t, bob, "revoked", []string{"models"})
	code, body = bob.do("DELETE", "/api/me/tokens/"+revokedID, nil, nil)
	expect(t, "吊销令牌", code, 204, body)
	code, _, body = harnessGet(t, ts.URL+"/v1/capabilities", bearer(revokedToken))
	expect(t, "吊销令牌读摘要", code, 401, body)
	if e := harnessError(t, body); e.Code != "token_revoked" {
		t.Fatalf("吊销令牌错误码不对：%+v", e)
	}
}

// capabilitiesView 是 /v1/capabilities 的响应形状。
type capabilitiesView struct {
	APIVersion    string `json:"api_version"`
	Authenticated bool   `json:"authenticated"`
	Server        struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"server"`
	User *struct {
		Username   string `json:"username"`
		Department string `json:"department"`
	} `json:"user"`
	Scopes []string `json:"scopes"`
	Models []string `json:"models"`
	Tools  struct {
		Count int `json:"count"`
	} `json:"tools"`
	Apps     []harnessAppView   `json:"apps"`
	Skills   []harnessSkillView `json:"skills"`
	Chatbot  chatbotCapView     `json:"chatbot"`
	Revision string             `json:"revision"`
}

type harnessSkillView struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type harnessAppView struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Starters    []string `json:"starters"`
	Launch      string   `json:"launch"`
	Delivery    string   `json:"delivery"`
	Version     int      `json:"version"`
	Web         bool     `json:"web"`
}

// TestHarnessApps 覆盖只读中立微应用：401/403、列表、详情、404、DSH 专属字段不下发。
func TestHarnessApps(t *testing.T) {
	ts, admin := harnessFixture(t)
	code, body := admin.do("POST", "/api/hub", map[string]any{
		"kind": "app", "slug": "expense", "delivery": "auto",
		"spec": map[string]any{"title": "报销助手", "description": "报销政策与流程", "persona": "你是报销助手。",
			"icon": "🧾", "starters": []string{"怎么报销"}}}, nil)
	expect(t, "建微应用", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "app", "slug": "optional-app", "delivery": "optional",
		"spec": map[string]any{"title": "自选应用", "description": "员工自己启用", "persona": "你是自选应用。"}}, nil)
	expect(t, "建自选微应用", code, 201, body)
	// 技能与提示词：能力摘要里应给元数据摘要（正文不在摘要里）。
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "policy-skill", "delivery": "auto",
		"spec": map[string]any{"title": "报销制度", "description": "查报销标准", "body": "住宿每晚 1300 元以内。"}}, nil)
	expect(t, "建技能", code, 201, body)
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "prompt", "slug": "weekly-prompt", "delivery": "auto",
		"spec": map[string]any{"title": "周报", "description": "整理周报", "body": "按项目整理本周进展。"}}, nil)
	expect(t, "建提示词", code, 201, body)

	bob := harnessLoginUser(t, ts, admin, "bob")
	hubToken, _ := harnessIssueToken(t, bob, "harness", []string{"hub", "connections"})
	modelsOnly, _ := harnessIssueToken(t, bob, "models-only", []string{"models"})

	// 匿名 401；无 hub scope 403；错误壳是中立的。
	code, header, body := harnessGet(t, ts.URL+"/v1/apps", nil)
	expect(t, "匿名读微应用", code, 401, body)
	if header.Get("WWW-Authenticate") == "" {
		t.Fatal("匿名 401 应带 WWW-Authenticate")
	}
	if e := harnessError(t, body); e.Type != "authentication_error" || e.Code != "token_invalid" {
		t.Fatalf("匿名错误不对：%+v", e)
	}
	code, _, body = harnessGet(t, ts.URL+"/v1/apps", bearer(modelsOnly))
	expect(t, "无 hub scope 读微应用", code, 403, body)
	if e := harnessError(t, body); e.Type != "permission_error" || e.Code != "scope_denied" {
		t.Fatalf("403 错误不对：%+v", e)
	}

	code, header, body = harnessGet(t, ts.URL+"/v1/apps", bearer(hubToken))
	expect(t, "读微应用列表", code, 200, body)
	if header.Get("Dsh-EE-API-Version") != "1" || header.Get("X-Request-Id") == "" {
		t.Fatalf("微应用列表应带版本头与请求号：%v", header)
	}
	if header.Get("Cache-Control") != "no-store" {
		t.Fatalf("用户相关的微应用列表不应被缓存：%v", header)
	}
	list := decode[struct {
		APIVersion string           `json:"api_version"`
		Revision   string           `json:"revision"`
		Apps       []harnessAppView `json:"apps"`
	}](t, body)
	if list.APIVersion != "1" || list.Revision == "" {
		t.Fatalf("微应用列表元信息不对：%s", body)
	}
	if len(list.Apps) != 1 || list.Apps[0].Name != "expense" {
		t.Fatalf("只应下发已启用的企业微应用：%+v", list.Apps)
	}
	app := list.Apps[0]
	if app.Title != "报销助手" || app.Description != "报销政策与流程" || app.Icon != "🧾" || app.Launch != "new" ||
		app.Delivery != "auto" || app.Version == 0 || !slices.Equal(app.Starters, []string{"怎么报销"}) {
		t.Fatalf("中立微应用描述不对：%+v", app)
	}
	// DSH 专属字段不下发：AssetID、工作台配置、缺权限提示、品牌与必装插件。
	for _, forbidden := range []string{"asset_id", "workbench", "\"missing\"", "brand", "required_plugins", "highlights"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("中立响应不应包含 DSH 专属字段 %q：%s", forbidden, body)
		}
	}

	// 详情：已启用的能读，未启用/不存在的 404。
	code, header, body = harnessGet(t, ts.URL+"/v1/apps/expense", bearer(hubToken))
	expect(t, "读微应用详情", code, 200, body)
	detail := decode[struct {
		APIVersion string         `json:"api_version"`
		App        harnessAppView `json:"app"`
	}](t, body)
	if header.Get("Dsh-EE-API-Version") != "1" || detail.APIVersion != "1" || detail.App.Name != "expense" || detail.App.Title != "报销助手" {
		t.Fatalf("微应用详情不对：%s", body)
	}
	code, _, body = harnessGet(t, ts.URL+"/v1/apps/optional-app", bearer(hubToken))
	expect(t, "未启用的自选微应用", code, 404, body)
	if e := harnessError(t, body); e.Type != "not_found_error" || e.Code != "not_found" {
		t.Fatalf("404 错误不对：%+v", e)
	}
	code, _, body = harnessGet(t, ts.URL+"/v1/apps/nope", bearer(hubToken))
	expect(t, "不存在的微应用", code, 404, body)
}
