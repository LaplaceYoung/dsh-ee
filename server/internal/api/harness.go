package api

// 第三方 Harness 的 v1 中立契约（最小实现）：
//
//   - GET /.well-known/dsh-ee：公开的发现文档，说明这个 dsh-ee 提供哪些协议面
//   - GET /v1/capabilities：当前令牌可用的模型 / MCP 工具计数 / 微应用与目录 revision；
//     不带令牌时返回公开子集（只有服务标识与版本）
//   - GET /v1/apps、GET /v1/apps/{name}：只读的中立微应用描述
//
// 这里只新增中立端点，不改 DSH 原生入口（/api/dsh/*、/mcp、/setup.*）的形状与语义。
// 这些端点的响应都带 Dsh-EE-API-Version: 1，错误统一走 {"error": {...}} 壳；
// /v1 下既有网关接口只多一个版本响应头，错误格式保持 OpenAI 形状不变。

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const (
	// harnessAPIVersion 是契约主版本；客户端可在请求头里带 Dsh-EE-API-Version，缺省即当前主版本。
	harnessAPIVersion = "1"
	// harnessServerName 是中立契约里的服务名（不随企业品牌变化，方便客户端识别协议）。
	harnessServerName = "dsh-ee"
	// 发现文档允许共享缓存 5 分钟；能力变化后客户端最迟 5 分钟能看到。
	harnessDiscoveryMaxAge = 300

	harnessVersionHeader   = "Dsh-EE-API-Version"
	harnessRequestIDHeader = "X-Request-Id"

	// harnessOpenAPIPath 是机器可读契约（静态 OpenAPI 3.1）的地址；发现文档里也会给出绝对地址。
	harnessOpenAPIPath = "/v1/openapi.json"
	// maxMCPRequestBody 是 /mcp JSON-RPC 请求体上限；发现文档 limits 里公布的值必须与 dsh.go 实际读取的上限一致。
	maxMCPRequestBody = 4 << 20

	harnessErrAuthentication = "authentication_error"
	harnessErrPermission     = "permission_error"
	harnessErrInvalidRequest = "invalid_request_error"
	harnessErrNotFound       = "not_found_error"
	harnessErrServer         = "server_error"
)

// harnessDeviceScopes 是设备码流程可申请的 scope（与 device.go 的 validDeviceScopes 一致）。
// 发现文档公布这一组，未知 scope 由签发端拒绝；admin 只能限时开启，不在其中。
var harnessDeviceScopes = []string{
	rbac.ScopeModels,
	rbac.ScopeRegistryRead,
	rbac.ScopeRegistryPublish,
	rbac.ScopeHub,
	rbac.ScopeConnections,
	rbac.ScopeAgent,
}

// harnessErrorCodes 是新增中立端点当前会返回的稳定错误码（发现文档 errors.codes 公布这一组）。
var harnessErrorCodes = []string{
	tokenInvalid, tokenExpired, tokenRevoked,
	"scope_denied", "unsupported_api_version", "method_not_allowed", "not_found", "internal_error",
}

//go:embed openapi.json
var harnessOpenAPISpec []byte

// harnessOpenAPIETag 是静态契约的内容摘要；契约随版本发布，进程内不会变化。
var harnessOpenAPIETag = func() string {
	sum := sha256.Sum256(harnessOpenAPISpec)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}()

// ---- 版本头与请求号中间件 ----

type harnessRequestIDKey struct{}

// harnessRequestID 取中间件为中立端点生成的请求号；错误响应里带上它方便对日志。
func harnessRequestID(r *http.Request) string {
	id, _ := r.Context().Value(harnessRequestIDKey{}).(string)
	return id
}

func newHarnessRequestID() string {
	buf := make([]byte, 12)
	// crypto/rand 在受支持的平台上不会失败；真失败时返回空串，调用方会省略请求号。
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func isHarnessNeutralPath(path string) bool {
	switch path {
	case "/.well-known/dsh-ee", harnessOpenAPIPath, "/v1/capabilities", "/v1/apps":
		return true
	}
	return strings.HasPrefix(path, "/v1/apps/")
}

// isHarnessAPIPath 是这个契约覆盖的路径：新增中立端点，以及既有 /v1 网关面（只加响应头）。
func isHarnessAPIPath(path string) bool {
	return path == "/.well-known/dsh-ee" || strings.HasPrefix(path, "/v1/")
}

// harnessRequestVersionOK 校验请求头里的主版本：缺省表示服务端当前主版本，只接受主版本 1。
func harnessRequestVersionOK(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	major, _, _ := strings.Cut(raw, ".")
	return major == harnessAPIVersion
}

// setHarnessVersionHeader 用响应头名的原样大小写写入版本头（http.Header.Set 会规范成
// Dsh-Ee-Api-Version；HTTP 头大小写不敏感，但契约里写的是 Dsh-EE-API-Version，保持原样更好读）。
func setHarnessVersionHeader(w http.ResponseWriter) {
	w.Header()[harnessVersionHeader] = []string{harnessAPIVersion}
}

// harnessAPI 是响应版本头、请求版本校验与请求号中间件。
// 只对契约路径生效：/v1/* 与 /.well-known/dsh-ee 加版本响应头，新增中立端点再加请求号与版本校验；
// /api/*、/mcp、/healthz 等既有入口的响应不被改动。
func harnessAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if isHarnessAPIPath(path) {
			setHarnessVersionHeader(w)
		}
		if !isHarnessNeutralPath(path) {
			next.ServeHTTP(w, r)
			return
		}
		rid := newHarnessRequestID()
		if rid != "" {
			w.Header().Set(harnessRequestIDHeader, rid)
		}
		r = r.WithContext(context.WithValue(r.Context(), harnessRequestIDKey{}, rid))
		// 新增中立端点都是只读 GET；方法不对时也返回统一错误壳，不落到 mux 的 text/plain 405。
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeHarnessErr(w, r, http.StatusMethodNotAllowed, harnessErrInvalidRequest, "method_not_allowed", "这个接口只支持 GET")
			return
		}
		if !harnessRequestVersionOK(r.Header.Get(harnessVersionHeader)) {
			writeHarnessError(w, r, http.StatusBadRequest, harnessErrorBody{
				Type: harnessErrInvalidRequest, Code: "unsupported_api_version",
				Message:           "不支持这个 API 主版本，本服务当前支持主版本 " + harnessAPIVersion,
				SupportedVersions: []string{harnessAPIVersion},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- 统一错误壳（只给新增中立端点用） ----

// harnessErrorBody 是中立的错误对象；/api/* 与 /v1 网关的既有错误壳保持不变。
type harnessErrorBody struct {
	Type              string   `json:"type"`
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	RequestID         string   `json:"request_id,omitempty"`
	RetryAfter        *int     `json:"retry_after,omitempty"`
	Docs              string   `json:"docs,omitempty"`
	SupportedVersions []string `json:"supported_versions,omitempty"`
}

func writeHarnessError(w http.ResponseWriter, r *http.Request, status int, body harnessErrorBody) {
	if body.RequestID == "" {
		body.RequestID = harnessRequestID(r)
	}
	writeJSON(w, status, map[string]any{"error": body})
}

func writeHarnessErr(w http.ResponseWriter, r *http.Request, status int, typ, code, msg string) {
	writeHarnessError(w, r, status, harnessErrorBody{Type: typ, Code: code, Message: msg})
}

// harnessInternalErr 是中立端点的 500：日志走服务端，响应不暴露内部细节。
func (s *Server) harnessInternalErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
		s.log.Debug("请求已取消", "path", r.URL.Path)
		return
	}
	s.log.Error("harness api internal error", "path", r.URL.Path, "err", err)
	writeHarnessErr(w, r, http.StatusInternalServerError, harnessErrServer, "internal_error", "服务器内部错误")
}

// ---- 认证：Bearer 可选 / 必需 ----

// harnessBearerToken 取 Authorization: Bearer 后的令牌；没有或用别的方案时第二个返回值为 false。
func harnessBearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// harnessAuthErr 把 resolveBearer 的错误映射成中立错误壳；非法令牌 401，其余算服务端错误。
func (s *Server) harnessAuthErr(w http.ResponseWriter, r *http.Request, err error) {
	var be bearerError
	if errors.As(err, &be) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
		writeHarnessErr(w, r, http.StatusUnauthorized, harnessErrAuthentication, string(be), "令牌无效、已吊销或已过期")
		return
	}
	s.harnessInternalErr(w, r, err)
}

// harnessPrincipal 要求带有效令牌（/v1/apps 等只读中立端点用）。
func (s *Server) harnessPrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	token, ok := harnessBearerToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
		writeHarnessErr(w, r, http.StatusUnauthorized, harnessErrAuthentication, tokenInvalid, "缺少 Authorization: Bearer 令牌")
		return nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		s.harnessAuthErr(w, r, err)
		return nil, false
	}
	return p, true
}

// harnessOptionalPrincipal 允许匿名：没有 Authorization 头时返回 (nil, true)；
// 带了但不合法时报 401，不静默退化成匿名，免得客户端把凭证问题误判成「没有权限」。
func (s *Server) harnessOptionalPrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return nil, true
	}
	token, ok := harnessBearerToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dsh-ee"`)
		writeHarnessErr(w, r, http.StatusUnauthorized, harnessErrAuthentication, tokenInvalid, "Authorization 必须是 Bearer 令牌")
		return nil, false
	}
	p, err := s.resolveBearer(r.Context(), token, s.clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host"))
	if err != nil {
		s.harnessAuthErr(w, r, err)
		return nil, false
	}
	return p, true
}

// viewerForPrincipal 与 dshViewer 构造的 viewer 一致：能力目录的内容（技能、revision）不因入口不同而变。
func (s *Server) viewerForPrincipal(ctx context.Context, p *Principal) viewer {
	return viewer{user: p.User, access: p.Access, agent: s.agentOn(ctx, p), canAdmin: p.CanAdmin, adminLive: p.AdminLive, account: p.Account}
}

// harnessAppViewer 认证并检查能力目录的入口级权限（与 /api/dsh/catalog 相同：hub scope + 工作台权限）。
func (s *Server) harnessAppViewer(w http.ResponseWriter, r *http.Request) (viewer, bool) {
	p, ok := s.harnessPrincipal(w, r)
	if !ok {
		return viewer{}, false
	}
	if !p.Scopes.Has(rbac.ScopeHub) || !p.Access.Has(rbac.PortalView) {
		writeHarnessErr(w, r, http.StatusForbidden, harnessErrPermission, "scope_denied", "这个令牌没有使用能力目录的权限")
		return viewer{}, false
	}
	return s.viewerForPrincipal(r.Context(), p), true
}

// ---- GET /.well-known/dsh-ee：公开发现文档 ----

type harnessDiscoveryDoc struct {
	Name                        string                  `json:"name"`
	Version                     string                  `json:"version"`
	APIVersion                  string                  `json:"api_version"`
	Auth                        harnessDiscoveredAuth   `json:"auth"`
	DeviceAuthorizationEndpoint string                  `json:"device_authorization_endpoint"`
	TokenEndpoint               string                  `json:"token_endpoint"`
	ScopesSupported             []string                `json:"scopes_supported"`
	Models                      harnessDiscoveredModels `json:"models"`
	MCP                         harnessDiscoveredMCP    `json:"mcp"`
	Apps                        harnessDiscoveredApps   `json:"apps"`
	// Chatbot 声明 DeepSeek Chatbot 能力的透传与注入状态（docs/dev/chatbot.md）：
	// 服务端工具、内容安全、会话标题与拒绝/引用块都依赖上游 DeepSeek，dsh-ee 只透传。
	Chatbot              harnessChatbot          `json:"chatbot"`
	CapabilitiesEndpoint string                  `json:"capabilities_endpoint"`
	OpenAPIURL           string                  `json:"openapi_url"`
	Limits               harnessLimits           `json:"limits"`
	Errors               harnessDiscoveredErrors `json:"errors"`
}

type harnessDiscoveredAuth struct {
	Type       string   `json:"type"`
	Header     string   `json:"header"`
	TokenTypes []string `json:"token_types"`
	TokenFlows []string `json:"token_flows"`
}

type harnessDiscoveredModels struct {
	Protocol                string `json:"protocol"`
	Streaming               bool   `json:"streaming"`
	Endpoint                string `json:"endpoint"`
	ChatCompletionsEndpoint string `json:"chat_completions_endpoint"`
	// AnthropicMessagesEndpoint 是 Anthropic Messages 兼容入口（x-api-key 或 Bearer）。
	AnthropicMessagesEndpoint string `json:"anthropic_messages_endpoint"`
	// AnthropicMessagesAliases 是同一 handler 的别名入口：DeepSeek 等厂商的 Anthropic
	// base URL 习惯是 <base>/anthropic，客户端拼出来即 /anthropic/v1/messages。
	AnthropicMessagesAliases []string `json:"anthropic_messages_aliases"`
	// AnthropicMessagesNativeVendors 是走原生透传的 provider vendor：这些线路直接转发到
	// <base>/anthropic/v1/messages，响应逐字节转发，不做 OpenAI chat 转换。
	AnthropicMessagesNativeVendors []string `json:"anthropic_messages_native_vendors"`
	// AnthropicMessagesConversionFallback 说明其它 vendor 的降级方式（转 OpenAI chat）。
	AnthropicMessagesConversionFallback string `json:"anthropic_messages_conversion_fallback"`
}

type harnessDiscoveredMCP struct {
	Transport        string   `json:"transport"`
	Endpoint         string   `json:"endpoint"`
	ProtocolVersions []string `json:"protocol_versions"`
}

type harnessDiscoveredApps struct {
	Web       bool   `json:"web"`
	DataTools bool   `json:"data_tools"`
	Endpoint  string `json:"endpoint"`
	WebURL    string `json:"web_url"`
}

type harnessLimits struct {
	MaxRequestBytes    int64 `json:"max_request_bytes"`
	MaxResponseBytes   int64 `json:"max_response_bytes"`
	MaxMCPRequestBytes int64 `json:"max_mcp_request_bytes"`
}

type harnessDiscoveredErrors struct {
	Format           string   `json:"format"`
	DocumentationURL string   `json:"documentation_url"`
	Codes            []string `json:"codes"`
}

func (s *Server) handleHarnessDiscovery(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase(r)
	doc := harnessDiscoveryDoc{
		Name:                        harnessServerName,
		Version:                     s.opts.Version,
		APIVersion:                  harnessAPIVersion,
		Auth:                        harnessDiscoveredAuth{Type: "bearer", Header: "Authorization: Bearer <token>", TokenTypes: []string{"ee_at_"}, TokenFlows: []string{"device_code", "web_manual"}},
		DeviceAuthorizationEndpoint: base + "/api/device/code",
		TokenEndpoint:               base + "/api/device/token",
		ScopesSupported:             append([]string(nil), harnessDeviceScopes...),
		Models: harnessDiscoveredModels{
			Protocol: "openai-chat-completions", Streaming: true,
			Endpoint: base + "/v1/models", ChatCompletionsEndpoint: base + "/v1/chat/completions",
			AnthropicMessagesEndpoint:           base + "/v1/messages",
			AnthropicMessagesAliases:            []string{base + "/anthropic/v1/messages"},
			AnthropicMessagesNativeVendors:      []string{anthropicNativeVendor},
			AnthropicMessagesConversionFallback: "openai-chat",
		},
		MCP: harnessDiscoveredMCP{Transport: "streamable-http", Endpoint: base + "/mcp", ProtocolVersions: append([]string(nil), mcp.Versions...)},
		Apps: harnessDiscoveredApps{
			Web: true, DataTools: true,
			Endpoint: base + "/v1/apps", WebURL: base + "/a/{slug}",
		},
		Chatbot:              s.chatbotCapability(),
		CapabilitiesEndpoint: base + "/v1/capabilities",
		OpenAPIURL:           base + harnessOpenAPIPath,
		Limits:               harnessLimits{MaxRequestBytes: maxRequestBody, MaxResponseBytes: maxResponseBody, MaxMCPRequestBytes: maxMCPRequestBody},
		Errors: harnessDiscoveredErrors{
			Format:           `{"error":{"type","code","message","request_id"}}`,
			DocumentationURL: base + harnessOpenAPIPath,
			Codes:            append([]string(nil), harnessErrorCodes...),
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		s.harnessInternalErr(w, r, err)
		return
	}
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	// 端点地址来自 publicBase：未配置 PublicURL 时按请求的 Host / X-Forwarded-* 推断，
	// 共享缓存按 URL 缓存会有缓存投毒风险；只允许私有缓存（浏览器/客户端本地）。
	w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(harnessDiscoveryMaxAge))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// ---- GET /v1/openapi.json：静态机器可读契约 ----

// handleHarnessOpenAPI 输出编译进二进制的 OpenAPI 3.1 文档（openapi.json）。
// 内容是静态的（servers 用相对地址 "/"），不含按请求推断的信息，因此可以公开缓存。
func (s *Server) handleHarnessOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("ETag", harnessOpenAPIETag)
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(harnessDiscoveryMaxAge))
	if r.Header.Get("If-None-Match") == harnessOpenAPIETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(harnessOpenAPISpec)
}

// ---- GET /v1/capabilities：当前令牌的能力摘要 ----

type harnessServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harnessUserInfo struct {
	Username   string `json:"username"`
	Department string `json:"department,omitempty"`
}

type harnessToolCap struct {
	// Count 是 /mcp tools/list 会返回的工具数（企业连接 + 知识库 + 应用 + 数据中台 + 工作台 + ee_*）。
	Count int `json:"count"`
}

type harnessCapabilities struct {
	APIVersion    string            `json:"api_version"`
	Authenticated bool              `json:"authenticated"`
	Server        harnessServerInfo `json:"server"`
	User          *harnessUserInfo  `json:"user"`
	Scopes        []string          `json:"scopes"`
	// models 是当前令牌允许的对外模型名，顺序与 /v1/models 一致。
	Models []string       `json:"models"`
	Tools  harnessToolCap `json:"tools"`
	// apps 是当前用户已启用 / 自动生效的中立微应用描述（不含 DSH 专属系统应用）。
	Apps []harnessApp `json:"apps"`
	// skills 是当前用户可见的技能 / 提示词摘要（只有元数据，不含正文）；完整技能接口见后续 PR。
	Skills []harnessSkill `json:"skills"`
	// chatbot 是服务端级的 Chatbot 能力声明（不随令牌变化）：服务端工具、safety_check、
	// title 与拒绝/引用块由上游 DeepSeek 实现，dsh-ee 只注入默认值并原样透传。
	Chatbot  harnessChatbot `json:"chatbot"`
	Revision string         `json:"revision"`
}

// 中立微应用描述：只保留第三方 Harness 能用的字段，DSH 专属的 AssetID/Workbench/Missing/
// Highlights/New/Todo 不下发（品牌、必装插件等 DSH 目录字段也不在这里）。
type harnessApp struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Starters    []string `json:"starters"`
	Launch      string   `json:"launch"`
	Delivery    string   `json:"delivery"`
	Version     int      `json:"version"`
	Web         bool     `json:"web"`
}

// harnessApps 把 DSH 目录里的微应用转成中立描述；「工作台」「知识问答」是 DSH 页面自带的
// 系统应用，依赖 DSH 的 /命令 与正文接口，不下发到中立契约。
func harnessApps(c *dshCatalog) []harnessApp {
	out := make([]harnessApp, 0, len(c.Apps))
	for _, a := range c.Apps {
		if a.Name == workbenchHomeName || a.Name == kbQAName {
			continue
		}
		starters := a.Starters
		if starters == nil {
			starters = []string{}
		}
		out = append(out, harnessApp{
			Name: a.Name, Title: a.Title, Description: a.Description, Icon: a.Icon,
			Starters: starters, Launch: a.Launch, Delivery: a.Delivery, Version: a.Version, Web: a.Web,
		})
	}
	return out
}

// harnessSkill 是技能 / 提示词的能力摘要项；与 /v1/capabilities 一样只给元数据，不含正文。
type harnessSkill struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// Type 是 skill 或 prompt；微应用在 apps 里，DSH 系统技能不下发。
	Type string `json:"type"`
}

// harnessSkills 从能力目录里挑出技能与提示词摘要：排除 DSH 系统技能（依赖 /命令 与正文接口）
// 和微应用（已有 /v1/apps）。
func harnessSkills(c *dshCatalog) []harnessSkill {
	out := make([]harnessSkill, 0, len(c.Skills))
	for _, sk := range c.Skills {
		if _, system := c.system[sk.Name]; system {
			continue
		}
		a := c.assets[sk.Name]
		if a == nil || (a.Kind != store.KindSkill && a.Kind != store.KindPrompt) {
			continue
		}
		out = append(out, harnessSkill{Name: sk.Name, Title: sk.Title, Type: a.Kind})
	}
	return out
}

// harnessToolCount 复刻 /mcp tools/list 的组成，用于能力摘要里的工具计数。
// 改动 tools/list 时这个函数要跟着改；harness_test.go 用同一令牌交叉校验两边数量一致。
func (s *Server) harnessToolCount(ctx context.Context, v viewer, p *Principal, c *dshCatalog) (int, error) {
	n := len(homeToolDefs())
	for _, cn := range c.conns {
		n += len(cn.tools)
	}
	if len(c.kbases) > 0 {
		n += len(kbToolDefs())
	}
	if targets := s.appToolTargets(ctx, v, c); len(targets) > 0 {
		n += len(appToolDefs(targets))
	}
	sets, err := s.visibleDataSets(ctx, v)
	if err != nil {
		return 0, err
	}
	if len(sets) > 0 {
		n += len(dataToolDefs(sets))
	}
	n += len(s.agentToolList(ctx, p))
	return n, nil
}

// quotaUsable 与 /v1/models 的检查一致：额度过期后模型列表为空（网关会 403）。
func quotaUsable(q *store.UserQuota) bool {
	return q == nil || q.QuotaExpiresAt == nil || q.QuotaExpiresAt.After(time.Now())
}

func (s *Server) handleHarnessCapabilities(w http.ResponseWriter, r *http.Request) {
	// 能力摘要与令牌相关，响应不应进入任何缓存。
	w.Header().Set("Cache-Control", "no-store")
	p, ok := s.harnessOptionalPrincipal(w, r)
	if !ok {
		return
	}
	caps := harnessCapabilities{
		APIVersion: harnessAPIVersion,
		Server:     harnessServerInfo{Name: harnessServerName, Version: s.opts.Version},
		Chatbot:    s.chatbotCapability(),
		Scopes:     []string{},
		Models:     []string{},
		Apps:       []harnessApp{},
		Skills:     []harnessSkill{},
	}
	if p == nil {
		// 匿名只返回公开子集：服务标识与契约版本，用户数据为空。
		writeJSON(w, http.StatusOK, caps)
		return
	}
	caps.Authenticated = true
	caps.User = &harnessUserInfo{Username: p.User.Username, Department: p.User.Department}
	for scope := range p.Scopes {
		caps.Scopes = append(caps.Scopes, scope)
	}
	sort.Strings(caps.Scopes)

	// 模型：与 /v1/models 同样的入口条件（models scope + llm.use + 额度未过期）
	ctx := r.Context()
	quota := p.quota()
	if p.Scopes.Has(rbac.ScopeModels) && p.Access.Has(rbac.LLMUse) && quotaUsable(quota) {
		ms, err := s.store.PublicModels(ctx)
		if err != nil {
			s.harnessInternalErr(w, r, err)
			return
		}
		for _, m := range ms {
			if modelAllowed(p, quota, m.Name) {
				caps.Models = append(caps.Models, m.Name)
			}
		}
	}

	// 能力目录：与 /api/dsh/catalog、/mcp 相同的入口条件（工作台权限 + hub / connections scope）
	if p.Access.Has(rbac.PortalView) && (p.Scopes.Has(rbac.ScopeHub) || p.Scopes.Has(rbac.ScopeConnections)) {
		v := s.viewerForPrincipal(ctx, p)
		c, err := s.userCatalog(ctx, v)
		if err != nil {
			s.harnessInternalErr(w, r, err)
			return
		}
		if p.Scopes.Has(rbac.ScopeHub) {
			caps.Apps = harnessApps(c)
			caps.Skills = harnessSkills(c)
			caps.Revision = c.Revision
		}
		if p.Scopes.Has(rbac.ScopeConnections) {
			n, err := s.harnessToolCount(ctx, v, p, c)
			if err != nil {
				s.harnessInternalErr(w, r, err)
				return
			}
			caps.Tools.Count = n
		}
	}
	writeJSON(w, http.StatusOK, caps)
}

// ---- GET /v1/apps、GET /v1/apps/{name}：只读中立微应用 ----

func (s *Server) handleHarnessApps(w http.ResponseWriter, r *http.Request) {
	// 微应用可见范围与用户权限相关，响应不应进入任何缓存。
	w.Header().Set("Cache-Control", "no-store")
	v, ok := s.harnessAppViewer(w, r)
	if !ok {
		return
	}
	c, err := s.userCatalog(r.Context(), v)
	if err != nil {
		s.harnessInternalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api_version": harnessAPIVersion,
		"revision":    c.Revision,
		"apps":        harnessApps(c),
	})
}

func (s *Server) handleHarnessApp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	v, ok := s.harnessAppViewer(w, r)
	if !ok {
		return
	}
	c, err := s.userCatalog(r.Context(), v)
	if err != nil {
		s.harnessInternalErr(w, r, err)
		return
	}
	name := r.PathValue("name")
	for _, app := range harnessApps(c) {
		if app.Name != name {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{"api_version": harnessAPIVersion, "app": app})
		return
	}
	writeHarnessErr(w, r, http.StatusNotFound, harnessErrNotFound, "not_found", "没有这个微应用，或它没有对你启用")
}
