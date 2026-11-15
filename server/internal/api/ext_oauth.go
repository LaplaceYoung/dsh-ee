package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// oauth 凭据模式（docs/plugins.md §7.3）：员工在「我的凭据」里授权一次，dsh-ee 按人保存令牌、到期前刷新。
//
//   - 标准 OAuth 2.0 授权码流程：清单的 oauth 里写授权地址、令牌地址、scope，dsh-ee 自己跳转、换令牌、刷新；
//     client_id / client_secret 由管理员安装时填。
//   - 流程不标准的：服务扩展实现 oauth/authorize-url、oauth/exchange、oauth/refresh、oauth/revoke 四个约定接口。
//
// 调用时只把 access token 带出去（声明式扩展放进请求头；服务扩展放在 X-DSH-EE-Credential），
// refresh token 和 client secret 一直留在 dsh-ee 里。

const (
	oauthCallbackPath = "/api/oauth/callback"
	oauthStateTTL     = 10 * time.Minute
	// 离到期不到这么久就先刷新
	oauthRefreshAhead = time.Minute
)

// errNeedAuthorize：这位员工还没授权，或授权过期了又刷新不了。
var errNeedAuthorize = errors.New("需要员工本人授权")

// extOAuthClient 是管理员填的 OAuth 客户端。secret 用主密钥加密。
type extOAuthClient struct {
	ID        string `json:"id"`
	SecretEnc string `json:"secret_enc,omitempty"`
}

func (s *Server) sealOAuthClient(id, plain string, old *extOAuthClient) (*extOAuthClient, string) {
	id, plain = strings.TrimSpace(id), strings.TrimSpace(plain)
	if id == "" && old != nil {
		id = old.ID
	}
	if id == "" || len(id) > 512 {
		return nil, "请填写 OAuth 客户端的 client_id（在对方系统的开放平台上建应用后得到）"
	}
	c := &extOAuthClient{ID: id}
	switch {
	case plain != "":
		sealed, err := s.box.Seal([]byte(plain))
		if err != nil {
			return nil, "加密 client_secret 失败：" + err.Error()
		}
		c.SecretEnc = base64.StdEncoding.EncodeToString(sealed)
	case old != nil:
		c.SecretEnc = old.SecretEnc
	}
	return c, ""
}

func (s *Server) oauthClientSecret(c *extOAuthClient) (string, error) {
	if c == nil || c.SecretEnc == "" {
		return "", nil
	}
	sealed, err := base64.StdEncoding.DecodeString(c.SecretEnc)
	if err != nil {
		return "", errors.New("client_secret 读不了")
	}
	plain, err := s.box.Open(sealed)
	if err != nil {
		return "", fmt.Errorf("解密 client_secret（主密钥是否换过？）：%w", err)
	}
	return string(plain), nil
}

// oauthTokens 是按人存的令牌，整个加密后放在 user_credentials 里。
type oauthTokens struct {
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Account   string    `json:"account,omitempty"`
}

func (s *Server) loadOAuthTokens(ctx context.Context, userID, assetID int64) (*oauthTokens, error) {
	enc, err := s.store.UserCredential(ctx, userID, assetID)
	if err != nil || len(enc) == 0 {
		return nil, err
	}
	plain, err := s.box.Open(enc)
	if err != nil {
		return nil, fmt.Errorf("解密员工的授权（主密钥是否换过？）：%w", err)
	}
	var t oauthTokens
	if json.Unmarshal(plain, &t) != nil || t.Access == "" {
		// 不是 oauth 存的（连接原来是 user_secret）：当作没授权
		return nil, nil
	}
	return &t, nil
}

func (s *Server) saveOAuthTokens(ctx context.Context, userID, assetID int64, t *oauthTokens) error {
	raw, _ := json.Marshal(t)
	if len(raw) > 12<<10 {
		return errors.New("对方给的令牌太长，存不下")
	}
	sealed, err := s.box.Seal(raw)
	if err != nil {
		return err
	}
	hint := "已授权"
	if t.Account != "" {
		hint = cutUTF8(t.Account, 16)
	}
	return s.store.SetUserCredential(ctx, userID, assetID, sealed, hint)
}

// oauthLocks 让同一位员工在同一个连接上的刷新排队：很多系统的 refresh token 只能用一次。
var oauthLocks sync.Map

func oauthLock(userID, assetID int64) *sync.Mutex {
	mu, _ := oauthLocks.LoadOrStore(connKey{assetID, userID}, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// oauthAccessToken 取这位员工此刻能用的 access token，快到期就先刷新。
func (s *Server) oauthAccessToken(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base string) (string, error) {
	if v.user == nil {
		return "", errors.New("这个连接按员工本人的授权调用，需要知道是谁在调")
	}
	t, err := s.loadOAuthTokens(ctx, v.user.ID, a.ID)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", errNeedAuthorize
	}
	if t.ExpiresAt.IsZero() || time.Until(t.ExpiresAt) > oauthRefreshAhead {
		return t.Access, nil
	}
	mu := oauthLock(v.user.ID, a.ID)
	mu.Lock()
	defer mu.Unlock()
	// 排队的时候别人可能已经刷新过了
	if t, err = s.loadOAuthTokens(ctx, v.user.ID, a.ID); err != nil {
		return "", err
	} else if t == nil {
		return "", errNeedAuthorize
	}
	if t.ExpiresAt.IsZero() || time.Until(t.ExpiresAt) > oauthRefreshAhead {
		return t.Access, nil
	}
	if t.Refresh == "" {
		return "", errNeedAuthorize
	}
	fresh, err := s.oauthRefresh(ctx, a, sp, v, base, t.Refresh)
	if err != nil {
		s.log.Info("刷新员工的授权失败，要重新授权", "connection", a.Slug, "user", v.user.ID, "err", err)
		// 对方明确拒绝（不是网络问题）时，这份授权已经没用了
		var ee *extError
		if errors.As(err, &ee) && ee.Status >= 400 && ee.Status < 500 {
			_, _ = s.store.DeleteUserCredential(ctx, v.user.ID, a.ID)
		}
		return "", errNeedAuthorize
	}
	if fresh.Refresh == "" {
		fresh.Refresh = t.Refresh
	}
	if fresh.Account == "" {
		fresh.Account = t.Account
	}
	if err := s.saveOAuthTokens(ctx, v.user.ID, a.ID, fresh); err != nil {
		return "", err
	}
	return fresh.Access, nil
}

// oauthHookHeaders 是调服务扩展 oauth 约定接口时带的请求头：员工的 ee_jwt、共享凭据、配置，不带员工凭据（还没有）。
func (s *Server) oauthHookHeaders(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base string) (http.Header, error) {
	h, err := s.connHeader(ctx, a.ID, sp, nil)
	if err != nil {
		return nil, err
	}
	tok, err := s.signEEJWT(ctx, a, sp.UserAuth, v, base)
	if err != nil {
		return nil, err
	}
	h.Set("Authorization", "Bearer "+tok)
	if err := s.extConfigHeader(sp.Extension, h); err != nil {
		return nil, err
	}
	return h, nil
}

func (s *Server) oauthHook(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base, name string, in, out any) error {
	h, err := s.oauthHookHeaders(ctx, a, sp, v, base)
	if err != nil {
		return err
	}
	return s.extPost(ctx, s.connHTTP(sp), sp.Extension.ServiceURL+"/ee/oauth/"+name, h, in, out)
}

// oauthStandard 取清单里标准流程的参数；没有（由服务扩展的钩子处理）返回 nil。
func oauthStandard(sp *assetSpec) *extension.OAuthConfig {
	if sp.Extension == nil {
		return nil
	}
	m, err := sp.Extension.manifest()
	if err != nil {
		return nil
	}
	return m.OAuth
}

type hookTokens struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Account      string    `json:"account"`
}

func (h *hookTokens) tokens() (*oauthTokens, error) {
	if h.AccessToken == "" {
		return nil, errors.New("扩展没有返回 accessToken")
	}
	return &oauthTokens{Access: h.AccessToken, Refresh: h.RefreshToken, ExpiresAt: h.ExpiresAt, Account: h.Account}, nil
}

// oauthTokenRequest 向标准的令牌地址换或刷新令牌。
func (s *Server) oauthTokenRequest(ctx context.Context, sp *assetSpec, cfg *extension.OAuthConfig, form url.Values) (*oauthTokens, error) {
	client := sp.Extension.OAuthClient
	if client == nil {
		return nil, errors.New("这个连接还没有填 OAuth 客户端")
	}
	sec, err := s.oauthClientSecret(client)
	if err != nil {
		return nil, err
	}
	form.Set("client_id", client.ID)
	if sec != "" {
		form.Set("client_secret", sec)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.connHTTP(sp).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		// 有的系统把秒数写成字符串
		ExpiresIn json.Number `json:"expires_in"`
		Error     string      `json:"error"`
		ErrorDesc string      `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &body)
	if resp.StatusCode >= 300 || body.AccessToken == "" {
		msg := body.Error
		if body.ErrorDesc != "" {
			msg += "：" + body.ErrorDesc
		}
		if msg == "" {
			msg = fmt.Sprintf("令牌地址返回 HTTP %d", resp.StatusCode)
		}
		status := resp.StatusCode
		if status < 300 {
			status = http.StatusBadGateway
		}
		return nil, &extError{Code: "authorize", Message: msg, Status: status}
	}
	t := &oauthTokens{Access: body.AccessToken, Refresh: body.RefreshToken}
	if n, err := body.ExpiresIn.Int64(); err == nil && n > 0 {
		t.ExpiresAt = time.Now().Add(time.Duration(n) * time.Second).UTC()
	}
	return t, nil
}

func (s *Server) oauthRefresh(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base, refresh string) (*oauthTokens, error) {
	if cfg := oauthStandard(sp); cfg != nil {
		return s.oauthTokenRequest(ctx, sp, cfg, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	}
	var out hookTokens
	if err := s.oauthHook(ctx, a, sp, v, base, "refresh", map[string]any{"refreshToken": refresh}, &out); err != nil {
		return nil, err
	}
	return out.tokens()
}

// oauthRevoke 在删掉授权前通知对方撤销（尽力而为：对方不支持或失败都不拦着删）。
func (s *Server) oauthRevoke(ctx context.Context, a *store.Asset, sp *assetSpec, user *store.User, base string) {
	if sp.UserAuth.personal() != userAuthOAuth || !sp.Extension.has("oauth") || base == "" {
		return
	}
	t, err := s.loadOAuthTokens(ctx, user.ID, a.ID)
	if err != nil || t == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	in := map[string]any{"tokens": hookTokens{AccessToken: t.Access, RefreshToken: t.Refresh, ExpiresAt: t.ExpiresAt, Account: t.Account}}
	if err := s.oauthHook(ctx, a, sp, viewer{user: user}, base, "revoke", in, nil); err != nil && extCode(err) != "not_implemented" {
		s.log.Warn("撤销员工在源系统的授权失败", "connection", a.Slug, "user", user.ID, "err", err)
	}
}

// oauthState 是授权跳转带出去、回调带回来的 state：用主密钥加密，别人伪造不了，也看不到里面是谁。
type oauthState struct {
	User  int64  `json:"u"`
	Asset int64  `json:"a"`
	Exp   int64  `json:"e"`
	Nonce string `json:"n"`
}

func (s *Server) sealOAuthState(st oauthState) (string, error) {
	raw, _ := json.Marshal(st)
	sealed, err := s.box.Seal(raw)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Server) openOAuthState(state string) (*oauthState, bool) {
	sealed, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return nil, false
	}
	raw, err := s.box.Open(sealed)
	if err != nil {
		return nil, false
	}
	var st oauthState
	if json.Unmarshal(raw, &st) != nil || time.Now().Unix() > st.Exp {
		return nil, false
	}
	return &st, true
}

// loadOAuthConnection 取出员工要授权的连接：在自己的可用范围内，而且凭据模式是 oauth。
func (s *Server) loadOAuthConnection(ctx context.Context, v viewer, id int64) (*store.Asset, *assetSpec, string) {
	a, err := s.store.GetAsset(ctx, id)
	if err != nil || a.Kind != store.KindConnection || a.Status != store.AssetActive || a.LiveVersion == 0 || (!available(v, a) && !v.manages(a.Kind)) {
		return nil, nil, "连接不存在"
	}
	lv, err := s.store.LiveAssetVersion(ctx, a.ID)
	if err != nil {
		return nil, nil, "连接不存在"
	}
	sp := parseSpec(lv.Spec)
	if sp.UserAuth.personal() != userAuthOAuth {
		return nil, nil, "这个连接不需要授权"
	}
	return a, sp, ""
}

// POST /api/me/connections/{id}/oauth/start：返回要跳去的授权地址。
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireFullSession(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "连接 id 不对")
		return
	}
	v := viewerOf(r)
	a, sp, msg := s.loadOAuthConnection(r.Context(), v, id)
	if msg != "" {
		writeErr(w, http.StatusNotFound, msg)
		return
	}
	base := s.callerBase(r)
	if base == "" {
		writeErr(w, http.StatusBadRequest, "还不知道 dsh-ee 的访问地址：请管理员在系统设置里填写「访问地址」")
		return
	}
	state, err := s.sealOAuthState(oauthState{User: v.user.ID, Asset: a.ID, Exp: time.Now().Add(oauthStateTTL).Unix(), Nonce: secret.Token()})
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	redirect := base + oauthCallbackPath
	var target string
	if cfg := oauthStandard(sp); cfg != nil {
		if sp.Extension.OAuthClient == nil {
			writeErr(w, http.StatusBadRequest, "这个连接还没有填 OAuth 客户端，请联系管理员")
			return
		}
		u, err := url.Parse(cfg.AuthorizeURL)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		q := u.Query()
		q.Set("response_type", "code")
		q.Set("client_id", sp.Extension.OAuthClient.ID)
		q.Set("redirect_uri", redirect)
		q.Set("state", state)
		if len(cfg.Scopes) > 0 {
			q.Set("scope", strings.Join(cfg.Scopes, " "))
		}
		u.RawQuery = q.Encode()
		target = u.String()
	} else {
		var out struct {
			URL string `json:"url"`
		}
		if err := s.oauthHook(r.Context(), a, sp, v, base, "authorize-url", map[string]any{"redirectUri": redirect, "state": state}, &out); err != nil {
			writeErr(w, http.StatusBadGateway, "取授权地址失败："+err.Error())
			return
		}
		target = out.URL
	}
	if u, err := url.Parse(target); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		writeErr(w, http.StatusBadGateway, "扩展给的授权地址不对")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": target})
}

// GET /api/oauth/callback：对方系统把员工带回来。校验 state（必须是当前登录的这个人发起的）、换令牌、按人保存。
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// 回调是浏览器往返，state 里没有会话类型；发起授权已被 handleOAuthStart 拦住，
	// 这里再按当前会话类型拦一次，防 limited 会话拿旧 state（10 分钟有效）直接回放完成绑定。
	if !s.requireFullSession(w, r) {
		return
	}
	q := r.URL.Query()
	back := func(assetID int64, problem string) {
		target := "/workbench"
		if assetID > 0 {
			target = "/assets/" + strconv.FormatInt(assetID, 10)
		}
		if problem != "" {
			target += "?oauth_error=" + url.QueryEscape(problem)
		} else {
			target += "?oauth=ok"
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
	st, ok := s.openOAuthState(q.Get("state"))
	v := viewerOf(r)
	if !ok || st.User != v.user.ID {
		back(0, "授权请求已过期或不是你发起的，请回到连接页面重新点「授权」")
		return
	}
	a, sp, msg := s.loadOAuthConnection(r.Context(), v, st.Asset)
	if msg != "" {
		back(0, msg)
		return
	}
	if e := q.Get("error"); e != "" {
		back(a.ID, "对方系统没有完成授权："+cutUTF8(e+" "+q.Get("error_description"), 200))
		return
	}
	base := s.callerBase(r)
	redirect := base + oauthCallbackPath
	var t *oauthTokens
	var err error
	if cfg := oauthStandard(sp); cfg != nil {
		code := q.Get("code")
		if code == "" {
			back(a.ID, "对方系统没有带回授权码")
			return
		}
		t, err = s.oauthTokenRequest(r.Context(), sp, cfg, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}})
	} else {
		params := map[string]string{}
		for k, vs := range q {
			if k != "state" && len(vs) > 0 {
				params[k] = vs[0]
			}
		}
		var out hookTokens
		if err = s.oauthHook(r.Context(), a, sp, v, base, "exchange", map[string]any{"redirectUri": redirect, "params": params}, &out); err == nil {
			t, err = out.tokens()
		}
	}
	if err != nil {
		back(a.ID, "换取令牌失败："+cutUTF8(err.Error(), 200))
		return
	}
	if err := s.saveOAuthTokens(r.Context(), v.user.ID, a.ID, t); err != nil {
		back(a.ID, err.Error())
		return
	}
	s.dropUserConnClient(a.ID, v.user.ID)
	s.audit(r, auditCredentialSet, a.Slug, map[string]any{"mode": userAuthOAuth})
	back(a.ID, "")
}
