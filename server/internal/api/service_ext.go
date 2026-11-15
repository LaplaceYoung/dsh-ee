package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/eejwt"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 服务扩展（docs/plugins.md §4.2，协议见 docs/extension-protocol.md）：企业自己部署的一个 HTTP 服务，
// dsh-ee 按协议调它。工具走它的 /mcp，工具之外的能力走 /ee/<名字>。
//
// dsh-ee 调扩展的每个请求都带一个现签的 ee_jwt（aud 是扩展的 id，sub 是当前员工；没有员工的调用
// sub 是 system、带 sys: true），扩展用 dsh-ee 的 JWKS 验签。源系统的凭据放在单独的请求头里，
// 只给这一次调用要用的：user_secret 的值、oauth 的 access token（refresh token 不给）、共享凭据。

const (
	extHeaderCredential = "X-DSH-EE-Credential"
	extHeaderShared     = "X-DSH-EE-Shared-Credential"
	extHeaderConfig     = "X-DSH-EE-Config"
	// 工具结果的 _meta 里放错误码的键
	extMetaError = "dsh-ee/error"

	extManifestPath = "/.well-known/dsh-ee-extension.json"
	// 清单和约定接口的响应上限
	extMaxManifest = 1 << 20
	extMaxResponse = 8 << 20

	// 连续失败这么多次就熔断，过这么久再放一个请求进去试
	extBreakerFails = 5
	extBreakerOpen  = 30 * time.Second
)

func (e *connExtension) service() bool { return e != nil && e.Kind == extension.KindService }

// has 报告服务扩展有没有声明某项能力。
func (e *connExtension) has(capability string) bool {
	if e != nil && e.Kind == extKindBuiltin {
		// 内置知识源（kb_feishu_docs.go）只做同步
		return capability == "knowledge.documents"
	}
	if !e.service() {
		return false
	}
	m, err := e.manifest()
	return err == nil && slices.Contains(m.Capabilities, capability)
}

// extError 是扩展按约定返回的错误（docs/plugins.md §6.6）。
type extError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *extError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("扩展返回 HTTP %d", e.Status)
}

// extCode 取错误里的约定错误码；不是扩展返回的错误就是空。
func extCode(err error) string {
	var ee *extError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ""
}

// ---- 熔断 ----

type extBreaker struct {
	mu sync.Mutex
	m  map[int64]*breakerState
}

type breakerState struct {
	fails int
	until time.Time
}

// allow 报告这个扩展现在能不能调；熔断期间直接返回不可用，不去碰扩展。
func (b *extBreaker) allow(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.m[id]
	return st == nil || st.fails < extBreakerFails || !time.Now().Before(st.until)
}

// report 记一次调用的成败。连不上、超时、5xx 算失败；扩展正常回了一个业务错误不算。
func (b *extBreaker) report(id int64, failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[int64]*breakerState{}
	}
	if !failed {
		delete(b.m, id)
		return
	}
	st := b.m[id]
	if st == nil {
		st = &breakerState{}
		b.m[id] = st
	}
	st.fails++
	if st.fails >= extBreakerFails {
		st.until = time.Now().Add(extBreakerOpen)
	}
}

var errExtBroken = &extError{Code: "unavailable", Message: "这个扩展连续出错，已暂时停用，稍后会自动重试"}

// ---- 配置 ----

// extSecretValue 是清单里 secret 类型的配置项在版本内容里的样子：主密钥加密后的密文。
type extSecretValue struct {
	Enc string `json:"$enc"`
}

// sealExtConfig 核对管理员填的配置并把 secret 类型的值加密。old 是原来的配置：secret 没重新填就沿用。
func (s *Server) sealExtConfig(m *extension.Manifest, in, old map[string]json.RawMessage) (map[string]json.RawMessage, string) {
	out := map[string]json.RawMessage{}
	for _, f := range m.Config {
		label := f.Label.Any()
		if label == "" {
			label = f.Key
		}
		raw, ok := in[f.Key]
		blank := !ok || isBlankJSON(raw)
		if blank && f.Type == "secret" && len(old[f.Key]) > 0 {
			out[f.Key] = old[f.Key]
			continue
		}
		if blank {
			if len(f.Default) > 0 && !isBlankJSON(f.Default) {
				out[f.Key] = f.Default
				continue
			}
			if f.Required {
				return nil, "配置项「" + label + "」必填"
			}
			continue
		}
		switch f.Type {
		case "boolean":
			var b bool
			if json.Unmarshal(raw, &b) != nil {
				return nil, "配置项「" + label + "」要填 是 / 否"
			}
		default:
			var str string
			if json.Unmarshal(raw, &str) != nil || len(str) > 4096 {
				return nil, "配置项「" + label + "」要填一段文字，最多 4096 个字符"
			}
			if f.Type == "url" {
				if u, err := url.Parse(str); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					return nil, "配置项「" + label + "」要填 http(s):// 开头的完整地址"
				}
			}
			if f.Type == "select" {
				var opts []string
				if json.Unmarshal(f.Options, &opts) == nil && len(opts) > 0 && !slices.Contains(opts, str) {
					return nil, "配置项「" + label + "」只能从清单给出的选项里选"
				}
			}
			if f.Type == "secret" {
				sealed, err := s.box.Seal([]byte(str))
				if err != nil {
					return nil, "加密配置项失败：" + err.Error()
				}
				raw, _ = json.Marshal(extSecretValue{Enc: base64.StdEncoding.EncodeToString(sealed)})
			}
		}
		out[f.Key] = raw
	}
	if len(out) == 0 {
		return nil, ""
	}
	return out, ""
}

func isBlankJSON(raw json.RawMessage) bool {
	v := strings.TrimSpace(string(raw))
	return v == "" || v == "null" || v == `""`
}

// openExtConfig 把存着的配置还原成明文（secret 解密），交给扩展。
func (s *Server) openExtConfig(e *connExtension) (map[string]any, error) {
	out := map[string]any{}
	for k, raw := range e.Config {
		var sv extSecretValue
		if json.Unmarshal(raw, &sv) == nil && sv.Enc != "" {
			sealed, err := base64.StdEncoding.DecodeString(sv.Enc)
			if err != nil {
				return nil, fmt.Errorf("配置项 %s 读不了", k)
			}
			plain, err := s.box.Open(sealed)
			if err != nil {
				return nil, fmt.Errorf("解密配置项 %s（主密钥是否换过？）：%w", k, err)
			}
			out[k] = string(plain)
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) == nil {
			out[k] = v
		}
	}
	return out, nil
}

// extConfigHeader 是带给扩展的配置。
func (s *Server) extConfigHeader(e *connExtension, h http.Header) error {
	if len(e.Config) == 0 {
		return nil
	}
	cfg, err := s.openExtConfig(e)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(cfg)
	h.Set(extHeaderConfig, base64.StdEncoding.EncodeToString(raw))
	return nil
}

// configView 是给管理员看的配置：secret 只说填没填。
func (e *connExtension) configView() map[string]any {
	out := map[string]any{}
	for k, raw := range e.Config {
		var sv extSecretValue
		if json.Unmarshal(raw, &sv) == nil && sv.Enc != "" {
			out[k] = map[string]bool{"set": true}
			continue
		}
		var v any
		_ = json.Unmarshal(raw, &v)
		out[k] = v
	}
	return out
}

// ---- 身份 ----

// signSystemJWT 签一个没有员工的令牌：健康检查、保存配置前的检查、知识同步、回调转发时用。
func (s *Server) signSystemJWT(ctx context.Context, aud, base string) (string, error) {
	key, _, _, err := s.eeKeys(ctx)
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", errors.New("还不知道 dsh-ee 的访问地址，签不了 ee_jwt：请在系统设置里填写「访问地址」")
	}
	now := time.Now()
	return eejwt.Sign(key, map[string]any{
		"iss": base, "aud": aud, "sub": "system", "sys": true, "name": "dsh-ee",
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(eejwt.Lifetime).Unix(), "jti": secret.Token(),
	})
}

// systemBase 是没有请求时（后台任务）dsh-ee 的访问地址。
func (s *Server) systemBase(ctx context.Context) string {
	base, err := s.connectBase(ctx, nil)
	if err != nil {
		return ""
	}
	return base
}

// extHeaders 拼出调服务扩展约定接口要带的请求头。v.user 为 nil 时是系统调用。
func (s *Server) extHeaders(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base string) (http.Header, error) {
	h, err := s.connHeader(ctx, a.ID, sp, nil)
	if err != nil {
		return nil, err
	}
	if v.user != nil {
		uh, err := s.userAuthHeader(ctx, a, sp, v, base)
		if err != nil {
			return nil, err
		}
		for k, vs := range uh {
			h[k] = vs
		}
		return h, nil
	}
	tok, err := s.signSystemJWT(ctx, sp.Extension.ID, base)
	if err != nil {
		return nil, err
	}
	h.Set("Authorization", "Bearer "+tok)
	if err := s.extConfigHeader(sp.Extension, h); err != nil {
		return nil, err
	}
	return h, nil
}

// extCall 调服务扩展的一个约定接口：POST <服务地址>/ee/<名字>。out 为 nil 时不读响应内容。
func (s *Server) extCall(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base, name string, in, out any) error {
	if isBuiltinSource(a.ID) {
		return s.builtinKBCall(ctx, name, in, out)
	}
	e := sp.Extension
	if !e.service() {
		return errors.New("这个连接不是服务扩展")
	}
	if !s.extBreak.allow(a.ID) {
		return errExtBroken
	}
	h, err := s.extHeaders(ctx, a, sp, v, base)
	if err != nil {
		return err
	}
	err = s.extPost(ctx, s.connHTTP(sp), e.ServiceURL+"/ee/"+name, h, in, out)
	var ee *extError
	s.extBreak.report(a.ID, err != nil && (!errors.As(err, &ee) || ee.Status >= 500))
	return err
}

// extPost 发一个约定接口的请求并解析约定的错误。
func (s *Server) extPost(ctx context.Context, hc *http.Client, target string, h http.Header, in, out any) error {
	if in == nil {
		in = struct{}{}
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, vs := range h {
		req.Header[k] = vs
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, extMaxResponse+1))
	if err != nil {
		return err
	}
	if len(raw) > extMaxResponse {
		return fmt.Errorf("扩展的响应超过 %d MB", extMaxResponse>>20)
	}
	if resp.StatusCode >= 300 {
		var wrap struct {
			Error *extError `json:"error"`
		}
		if json.Unmarshal(raw, &wrap) == nil && wrap.Error != nil && wrap.Error.Code != "" {
			wrap.Error.Status = resp.StatusCode
			return wrap.Error
		}
		return &extError{Status: resp.StatusCode, Message: fmt.Sprintf("扩展返回 HTTP %d：%s", resp.StatusCode, cutUTF8(string(raw), 300))}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("扩展的响应不是约定的 JSON：%w", err)
	}
	return nil
}

// fetchExtManifest 从服务地址取清单。
func (s *Server) fetchExtManifest(ctx context.Context, serviceURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serviceURL+extManifestPath, nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Transport: s.upstream.Transport, Timeout: 15 * time.Second, CheckRedirect: sameHostRedirect}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 %s：%w", serviceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s%s 返回 HTTP %d：这个地址上不是一个 dsh-ee 服务扩展", serviceURL, extManifestPath, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, extMaxManifest+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > extMaxManifest {
		return nil, errors.New("扩展的清单超过 1 MB")
	}
	return raw, nil
}

// cleanServiceURL 规范化管理员填的服务地址。
func cleanServiceURL(raw string) (string, string) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	raw = strings.TrimSuffix(raw, extManifestPath)
	raw = strings.TrimSuffix(raw, "/mcp")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "服务地址要是 http(s):// 开头的完整地址，不带账号密码和查询参数"
	}
	return raw, ""
}

// validateServiceExtension 检查服务扩展装出来的连接，并按清单定下凭据怎么带。
func validateServiceExtension(sp *assetSpec) string {
	e := sp.Extension
	if sp.Type != "mcp" {
		return "服务扩展的连接类型是 mcp"
	}
	if extension.Digest([]byte(e.Manifest), nil) != e.Digest {
		return "这个连接来自扩展 " + e.ID + "，清单不能在这里单独改；扩展升级后请在扩展页面确认新版本"
	}
	m, err := e.manifest()
	if err != nil {
		return err.Error()
	}
	if m.ID != e.ID || m.Version != e.Version || m.Kind != extension.KindService {
		return "扩展的身份和清单对不上，请重新安装"
	}
	if _, msg := cleanServiceURL(e.ServiceURL); msg != "" {
		return msg
	}
	if sp.URL != e.ServiceURL+"/mcp" {
		return "服务扩展的地址不能在连接页面上改；要换地址请在扩展页面重新安装"
	}
	if c := m.Credential; c != nil && c.Mode == extension.CredOAuth && m.OAuth != nil && e.OAuthClient == nil {
		return "这个扩展用标准的 OAuth 流程，要填 OAuth 客户端（client_id / client_secret）"
	}
	picked := sharedNameOf(sp)
	sp.Auth, sp.UserAuth = serviceCredential(m)
	if sp.Auth.Type != "none" {
		sp.Auth.SharedName = picked
	}
	sp.OpenAPI = ""
	return validateUserAuth(sp)
}

// serviceCredential 按清单定下调服务扩展时带什么：ee_jwt 每次都带；源系统的凭据按清单的模式另放一个请求头。
func serviceCredential(m *extension.Manifest) (*connAuth, *connUserAuth) {
	auth := &connAuth{Type: "none"}
	user := &connUserAuth{Mode: userAuthEEJWT, Audience: m.ID}
	c := m.Credential
	if c == nil {
		return auth, user
	}
	user.Claims = c.Claims
	switch c.Mode {
	case extension.CredShared:
		auth = &connAuth{Type: "header", Header: extHeaderShared}
	case extension.CredUserSecret:
		user.Extra, user.Hint = userAuthSecret, c.Hint.Any()
	case extension.CredOAuth:
		user.Extra = userAuthOAuth
	}
	if c.Shared != nil {
		auth = &connAuth{Type: "header", Header: extHeaderShared}
	}
	return auth, user
}

// prepareService 在调服务扩展的工具前补上不由模型填的参数。
func (e *connExtension) prepareService(tool string, args json.RawMessage) (json.RawMessage, error) {
	m, err := e.manifest()
	if err != nil {
		return nil, err
	}
	mt := m.Tool(tool)
	if mt == nil || !slices.ContainsFunc(mt.Params, func(p extension.Param) bool { return p.Fill == extension.FillApp || p.Fill == extension.FillAdmin }) {
		return args, nil
	}
	obj := map[string]any{}
	if len(args) > 0 && string(args) != "null" {
		dec := json.NewDecoder(bytes.NewReader(args))
		dec.UseNumber()
		if err := dec.Decode(&obj); err != nil {
			return nil, errors.New("参数不是 JSON 对象")
		}
	}
	if err := mt.Fix(obj, e.Admin); err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// applyServiceExtension 把清单叠加到扩展的 MCP 工具上：只读以清单为准（没标的一律当写操作），
// 说明用清单里的；清单里没有的工具不收（清单是给管理员看、给权限和审批用的，两边要一致）。
// 返回两边对不上的地方。
func applyServiceExtension(m *extension.Manifest, tools []*store.ConnectionTool) ([]*store.ConnectionTool, []string) {
	var out []*store.ConnectionTool
	var diff []string
	seen := map[string]bool{}
	for _, t := range tools {
		mt := m.Tool(t.Name)
		if mt == nil {
			diff = append(diff, "扩展的 MCP 里有工具 "+t.Name+"，清单里没有：没有收进来")
			continue
		}
		seen[t.Name] = true
		if t.ReadOnly != mt.ReadOnly {
			diff = append(diff, fmt.Sprintf("工具 %s 是不是只读，清单和 MCP 说的不一样：以清单为准", t.Name))
		}
		t.ReadOnly = mt.ReadOnly
		// 只读的默认启用；写操作要管理员看过再打开
		t.Enabled = mt.ReadOnly
		if d := mt.Description.Any(); d != "" {
			t.Description = d
		}
		// 不由模型填的参数不该出现在给模型的定义里；扩展没拿掉的这里再拿一次
		if schema, err := mt.Schema(t.InputSchema); err == nil {
			t.InputSchema = schema
		}
		out = append(out, t)
	}
	for _, mt := range m.Tools {
		if !seen[mt.Name] {
			diff = append(diff, "清单里有工具 "+mt.Name+"，扩展的 MCP 里没有")
		}
	}
	return out, diff
}

// extToolResult 把服务扩展带错误码的工具结果换成模型能看懂的话（docs/plugins.md §6.6）。
// 返回换过的结果和错误码；没有错误码就原样返回。
func extToolResult(a *store.Asset, base string, raw json.RawMessage) (json.RawMessage, string) {
	var r struct {
		IsError bool                       `json:"isError"`
		Meta    map[string]json.RawMessage `json:"_meta"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.IsError || len(r.Meta[extMetaError]) == 0 {
		return raw, ""
	}
	var code struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(r.Meta[extMetaError], &code) != nil || code.Code == "" {
		return raw, ""
	}
	msg := ""
	for _, c := range r.Content {
		if c.Type == "text" {
			msg += c.Text
		}
	}
	return mcp.TextResult(extErrorText(a, base, code.Code, msg), true), code.Code
}

// extErrorText 是按错误码给模型看的说明。
func extErrorText(a *store.Asset, base, code, msg string) string {
	page := base + "/assets/" + strconv.FormatInt(a.ID, 10)
	switch code {
	case "authorize":
		return "调用 " + a.Title + " 需要你本人先授权（或授权已过期）。请打开 " + page + " ，在「我的凭据」里完成授权后再重试。不要用别人的凭据，也不要改用其他工具绕过。"
	case "setup":
		return a.Title + " 还没有配置好，暂时用不了，请联系管理员。" + parenthesize(msg) + "不要重试。"
	case "forbidden":
		return a.Title + " 说你没有权限做这件事。" + parenthesize(msg) + "这是对方系统的判断，不要重试，也不要换别的工具绕过。"
	case "unavailable":
		return a.Title + " 暂时不可用。" + parenthesize(msg) + "可以稍后再试一次；还是不行就把情况告诉用户。"
	}
	return "调用 " + a.Title + " 失败：" + msg
}

func parenthesize(msg string) string {
	if msg = strings.TrimSpace(msg); msg == "" {
		return ""
	}
	return "（" + msg + "）"
}

// ---- 健康检查和版本变化 ----

// extHealth 是一次健康检查的结果。
type extHealth struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
	// 扩展现在提供的清单和装的时候不一样了：新版本要管理员确认后才生效
	Changed    bool      `json:"changed"`
	NewVersion string    `json:"new_version,omitempty"`
	NewDigest  string    `json:"new_digest,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

func extHealthKey(id int64) string { return "ext_health:" + strconv.FormatInt(id, 10) }

// checkExtHealth 调扩展的 health 接口，并重新取清单看有没有变。结果存起来给扩展页面看。
func (s *Server) checkExtHealth(ctx context.Context, a *store.Asset, sp *assetSpec, base string) *extHealth {
	h := &extHealth{CheckedAt: time.Now().UTC()}
	e := sp.Extension
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var resp struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := s.extCall(ctx, a, sp, viewer{}, base, "health", nil, &resp); err != nil {
		h.Error = err.Error()
	} else {
		h.OK, h.Version = resp.OK, resp.Version
		if !resp.OK {
			h.Error = "扩展说自己不正常"
		}
	}
	if raw, err := s.fetchExtManifest(ctx, e.ServiceURL); err == nil {
		if d := extension.Digest(raw, nil); d != e.Digest {
			h.Changed, h.NewDigest = true, d
			if m, err := extension.ParseManifest(raw); err == nil {
				h.NewVersion = m.Version
			}
		}
	} else if h.Error == "" {
		h.OK, h.Error = false, err.Error()
	}
	if err := s.store.PutSettingJSON(context.WithoutCancel(ctx), extHealthKey(a.ID), h, 0); err != nil {
		s.log.Warn("保存扩展健康检查结果失败", "asset", a.ID, "err", err)
	}
	return h
}

// serviceExtensions 列出所有服务扩展装出来的连接。
func (s *Server) serviceExtensions(ctx context.Context) ([]*store.Asset, map[int64]*assetSpec, error) {
	conns, err := s.store.ListAssets(ctx, store.KindConnection)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(conns))
	for _, a := range conns {
		ids = append(ids, a.ID)
	}
	live, err := s.store.LiveAssetVersions(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	var out []*store.Asset
	specs := map[int64]*assetSpec{}
	for _, a := range conns {
		if lv := live[a.ID]; lv != nil && a.Status == store.AssetActive {
			if sp := parseSpec(lv.Spec); sp.Extension.service() {
				out = append(out, a)
				specs[a.ID] = sp
			}
		}
	}
	return out, specs, nil
}

// runExtHealth 每小时检查一遍服务扩展；发现清单变了只记下来，等管理员确认。
func (s *Server) runExtHealth(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runLeased(ctx, leaseExtHealth, func(ctx context.Context) {
				base := s.systemBase(ctx)
				if base == "" {
					return
				}
				exts, specs, err := s.serviceExtensions(ctx)
				if err != nil {
					s.log.Warn("列出服务扩展失败", "err", err)
					return
				}
				for _, a := range exts {
					if h := s.checkExtHealth(ctx, a, specs[a.ID], base); !h.OK {
						s.log.Warn("服务扩展健康检查没通过", "extension", specs[a.ID].Extension.ID, "err", h.Error)
					} else if h.Changed {
						s.log.Info("服务扩展的清单变了，等管理员确认", "extension", specs[a.ID].Extension.ID, "version", h.NewVersion)
					}
				}
			})
		}
	}
}

// POST /api/admin/connections/{id}/extension/health：现在检查一次。
func (s *Server) handleExtHealth(w http.ResponseWriter, r *http.Request) {
	a, sp, ok := s.loadServiceExtension(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.checkExtHealth(r.Context(), a, sp, s.callerBase(r)))
}

// GET /api/admin/connections/{id}/extension：扩展的清单、配置（secret 不回显）、回调地址、最近一次健康检查。
func (s *Server) handleExtInfo(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	lv, err := s.store.LiveAssetVersion(r.Context(), a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	sp := parseSpec(lv.Spec)
	e := sp.Extension
	if e == nil {
		writeErr(w, http.StatusNotFound, "这个连接不是扩展装的")
		return
	}
	m, err := e.manifest()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	kind := e.Kind
	if kind == "" {
		kind = extension.KindDeclarative
	}
	out := map[string]any{
		"id": e.ID, "version": e.Version, "digest": e.Digest, "kind": kind,
		"manifest": m, "config": e.configView(), "admin": e.Admin, "service_url": e.ServiceURL,
	}
	if e.Endpoint != "" {
		out["endpoint_url"] = s.callerBase(r) + "/x/" + e.Endpoint + "/"
	}
	if e.OAuthClient != nil {
		out["oauth_client_id"] = e.OAuthClient.ID
		out["oauth_redirect_uri"] = s.callerBase(r) + oauthCallbackPath
	}
	var health *extHealth
	if _, err := s.store.GetSettingJSON(r.Context(), extHealthKey(a.ID), &health); err == nil && health != nil {
		out["health"] = health
	}
	writeJSON(w, http.StatusOK, out)
}

// loadServiceExtension 取出路径里的连接，它要是服务扩展装的。
func (s *Server) loadServiceExtension(w http.ResponseWriter, r *http.Request) (*store.Asset, *assetSpec, bool) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return nil, nil, false
	}
	lv, err := s.store.LiveAssetVersion(r.Context(), a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return nil, nil, false
	}
	sp := parseSpec(lv.Spec)
	if !sp.Extension.service() {
		writeErr(w, http.StatusBadRequest, "这个连接不是服务扩展")
		return nil, nil, false
	}
	return a, sp, true
}

// POST /api/admin/connections/{id}/extension/options {field, config}：取动态选项。
func (s *Server) handleExtOptions(w http.ResponseWriter, r *http.Request) {
	a, sp, ok := s.loadServiceExtension(w, r)
	if !ok {
		return
	}
	var req struct {
		Field string `json:"field"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var out struct {
		Options []map[string]any `json:"options"`
	}
	if err := s.extCall(r.Context(), a, sp, viewerOf(r), s.callerBase(r), "options", map[string]any{"field": req.Field}, &out); err != nil {
		writeErr(w, http.StatusBadGateway, "取选项失败："+err.Error())
		return
	}
	if out.Options == nil {
		out.Options = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- 回调地址（docs/plugins.md §6.3）----

const (
	extEndpointBody      = 1 << 20
	extEndpointPerMinute = 120
)

// handleExtEndpoint：/x/<随机串>/…，第三方（飞书开放平台、审批系统）打进来的回调，原样转给扩展的 endpoint 接口。
// 不需要登录，所以有限制：请求体上限、按地址限流、只转给这个扩展自己；扩展停用或卸载后地址立即失效。
func (s *Server) handleExtEndpoint(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if len(token) < 20 {
		http.NotFound(w, r)
		return
	}
	// 限流键必须用客户端 IP，不能用 URL 里的 token：这个入口不需要登录，token 是
	// 攻击者可控的——每换一个随机 token 就是一个新键，等于没有限流；而且新键会让
	// keyRate 的 map 无界增长。clientIP 只采信可信代理写的 X-Forwarded-For，
	// 直连的客户端伪造不了。
	if !s.extEndpointRate.allow(s.clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "请求太频繁")
		return
	}
	ctx := r.Context()
	exts, specs, err := s.serviceExtensions(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var a *store.Asset
	for _, x := range exts {
		if e := specs[x.ID].Extension; e.Endpoint != "" && subtle.ConstantTimeCompare([]byte(e.Endpoint), []byte(token)) == 1 {
			a = x
		}
	}
	if a == nil {
		http.NotFound(w, r)
		return
	}
	sp := specs[a.ID]
	body, err := io.ReadAll(io.LimitReader(r.Body, extEndpointBody+1))
	if err != nil || len(body) > extEndpointBody {
		writeErr(w, http.StatusRequestEntityTooLarge, "请求体不能超过 1 MB")
		return
	}
	in := map[string]any{"method": r.Method, "path": "/" + r.PathValue("rest"), "query": r.URL.RawQuery, "headers": forwardHeaders(r.Header), "body": body}
	var out struct {
		Status  int                 `json:"status"`
		Headers map[string][]string `json:"headers"`
		Body    []byte              `json:"body"`
	}
	base := s.callerBase(r)
	if err := s.extCall(ctx, a, sp, viewer{}, base, "endpoint", in, &out); err != nil {
		s.log.Warn("转发扩展回调失败", "extension", sp.Extension.ID, "err", err)
		writeErr(w, http.StatusBadGateway, "扩展没有处理这个回调")
		return
	}
	for k, vs := range out.Headers {
		// 扩展不能借回调往 dsh-ee 的域名下种 cookie 或跳转
		switch http.CanonicalHeaderKey(k) {
		case "Set-Cookie", "Location", "Content-Length", "Transfer-Encoding", "Connection":
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if out.Status < 200 || out.Status > 599 || (out.Status >= 300 && out.Status < 400) {
		out.Status = http.StatusOK
	}
	w.WriteHeader(out.Status)
	_, _ = w.Write(out.Body)
}

// forwardHeaders 是转给扩展的请求头：去掉员工浏览器可能带上的 dsh-ee 会话和令牌。
func forwardHeaders(h http.Header) map[string][]string {
	out := map[string][]string{}
	for k, vs := range h {
		switch http.CanonicalHeaderKey(k) {
		case "Cookie", "Authorization", "Proxy-Authorization":
			continue
		}
		out[k] = vs
	}
	return out
}

// keyRate 是按键的每分钟限流（进程内）。
//
// 键是客户端 IP（/x/<token>/… 回调地址的限流），可能被大量不同的来源填充，所以
// map 必须有上界：与 rate_limit.go 的 tokenRate 同一套做法——键数达到 maxExtRateKeys
// 后，新的键一律记进 extRateOverflowID 溢出桶；过期窗口只在 lastPurge 满一分钟时
// 清扫一次，内存有界，也不会每个请求都扫一遍 map。
type keyRate struct {
	mu        sync.Mutex
	limit     int
	m         map[string]*keyWindow
	lastPurge time.Time
}

const (
	// 一个限流窗口内最多记多少个键；超出后新键共用一个溢出桶。
	maxExtRateKeys    = 10000
	extRateOverflowID = "\x00overflow"
)

type keyWindow struct {
	start time.Time
	n     int
}

func (k *keyRate) allow(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = map[string]*keyWindow{}
	}
	now := time.Now()
	if len(k.m) >= maxExtRateKeys && now.Sub(k.lastPurge) >= time.Minute {
		// 顺手清掉过期窗口；限频，避免大量新键时每个请求都扫一遍 map
		for name, w := range k.m {
			if now.Sub(w.start) > time.Minute {
				delete(k.m, name)
			}
		}
		k.lastPurge = now
	}
	if len(k.m) >= maxExtRateKeys {
		// 已经不认识的键一律进溢出桶，已有的键继续用自己的窗口
		if _, ok := k.m[key]; !ok {
			key = extRateOverflowID
		}
	}
	w := k.m[key]
	if w == nil || now.Sub(w.start) > time.Minute {
		k.m[key] = &keyWindow{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= k.limit
}

// ---- 安装 ----

// prepareServiceInstall 在装服务扩展前定下连接的内容：地址、配置（secret 加密）、回调地址，
// 并在扩展实现了 validate 时让它检查一遍配置。返回给管理员看的问题。
func (s *Server) prepareServiceInstall(r *http.Request, m *extension.Manifest, sp, old *assetSpec, serviceURL string, req *extInstallReq) string {
	e := sp.Extension
	e.Kind, e.ServiceURL = extension.KindService, serviceURL
	e.Digest = extension.Digest([]byte(e.Manifest), nil)
	sp.Type, sp.URL, sp.OpenAPI = "mcp", serviceURL+"/mcp", ""
	var oldCfg map[string]json.RawMessage
	if old != nil && old.Extension != nil {
		oldCfg = old.Extension.Config
		e.Endpoint = old.Extension.Endpoint
	}
	cfg, msg := s.sealExtConfig(m, req.Config, oldCfg)
	if msg != "" {
		return msg
	}
	e.Config = cfg
	var perms map[string]any
	_ = json.Unmarshal(m.Permissions, &perms)
	if perms["endpoint"] == true && slices.Contains(m.Capabilities, "endpoint") {
		if e.Endpoint == "" {
			e.Endpoint = secret.Token()
		}
	} else {
		e.Endpoint = ""
	}
	if !slices.Contains(m.Capabilities, "validate") {
		return ""
	}
	// 保存前让扩展检查配置和共享凭据，配错了当场就知道
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	base := s.callerBase(r)
	tok, err := s.signSystemJWT(ctx, m.ID, base)
	if err != nil {
		return err.Error()
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+tok)
	if req.Secret != nil && strings.TrimSpace(*req.Secret) != "" {
		h.Set(extHeaderShared, strings.TrimSpace(*req.Secret))
	}
	plain, err := s.openExtConfig(e)
	if err != nil {
		return err.Error()
	}
	var out struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := s.extPost(ctx, s.connHTTP(sp), serviceURL+"/ee/validate", h, map[string]any{"config": plain}, &out); err != nil {
		return "扩展检查配置时出错：" + err.Error()
	}
	if !out.OK {
		if out.Message == "" {
			out.Message = "没有说明原因"
		}
		return "扩展说配置不对：" + out.Message
	}
	return ""
}

// serviceClient 是以系统身份连服务扩展 /mcp 的客户端：同步工具清单时用，不需要哪位员工的凭据。
func (s *Server) serviceClient(ctx context.Context, a *store.Asset, sp *assetSpec, base string) (*mcp.Client, error) {
	if _, err := s.extHeaders(ctx, a, sp, viewer{}, base); err != nil {
		return nil, err
	}
	return &mcp.Client{URL: sp.URL, HC: s.connHTTP(sp), Dynamic: func(ctx context.Context) (http.Header, error) {
		return s.extHeaders(ctx, a, sp, viewer{}, base)
	}}, nil
}

// serviceToolDiff 列出清单和扩展 MCP 工具对不上的地方，装完后提醒管理员。
func (s *Server) serviceToolDiff(ctx context.Context, a *store.Asset, sp *assetSpec, base string) []string {
	out := []string{}
	client, err := s.serviceClient(ctx, a, sp, base)
	if err != nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return out
	}
	m, err := sp.Extension.manifest()
	if err != nil {
		return out
	}
	var listed []*store.ConnectionTool
	for _, t := range tools {
		listed = append(listed, &store.ConnectionTool{Name: t.Name, ReadOnly: t.ReadOnly(), InputSchema: t.InputSchema})
	}
	_, diff := applyServiceExtension(m, listed)
	return append(out, diff...)
}
