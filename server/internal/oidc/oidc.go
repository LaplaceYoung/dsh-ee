// Package oidc 是 OpenID Connect 登录（授权码 + PKCE）：发现配置、换令牌、校验 ID Token、读声明。
//
// 不引入第三方库：只实现企业 IdP（Keycloak、Authing、Azure AD / Entra ID 等）登录要用的那一小部分。
// ID Token 支持 RS256/384/512 与 ES256/384 签名；声明可以来自 ID Token 与 userinfo 两处。
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Config 是管理员在系统设置里填的内容。ClientSecret 不参与 JSON，由调用方加密保存。
type Config struct {
	Enabled bool `json:"enabled"`
	// keycloak / authing / azure / custom：只影响页面上的默认值
	Preset string `json:"preset"`
	// 登录页按钮上的名字，例如「企业统一登录」
	Name         string `json:"name"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"-"`
	// 除 openid 以外要申请的 scope，空格分隔
	Scopes string `json:"scopes"`

	// 声明映射：从哪个声明取用户名、显示名、邮箱、部门与组。组声明可以用点号取嵌套字段（如 realm_access.roles）
	UsernameClaim   string `json:"usernameClaim"`
	NameClaim       string `json:"nameClaim"`
	EmailClaim      string `json:"emailClaim"`
	DepartmentClaim string `json:"departmentClaim"`
	GroupsClaim     string `json:"groupsClaim"`

	// 组（或角色）声明的值 → dsh-ee 角色，与 LDAP 组映射同样的规则
	RoleMappings []RoleMapping `json:"roleMappings"`
	// IdP 里有、dsh-ee 还没有的账号首次登录时自动创建
	AutoCreate bool `json:"autoCreate"`
	// 飞书：通讯录 API 根地址与组织同步开关。其它 IdP 忽略。
	FeishuAPIBase       string `json:"feishuApiBase,omitempty"`
	FeishuSync          bool   `json:"feishuSync"`
	FeishuSyncDisable   bool   `json:"feishuSyncDisable"`
	FeishuNotify        bool   `json:"feishuNotify"`
	FeishuNotifyActions bool   `json:"feishuNotifyActions"`
	// 飞书开发者后台「事件与回调 → 加密策略」里的 Verification Token。卡片回调是公开接口，
	// 靠它确认请求确实是飞书发来的；开卡片审批必须填。
	FeishuVerificationToken string `json:"feishuVerificationToken,omitempty"`
}

type RoleMapping struct {
	Group  string `json:"group"`
	RoleID int64  `json:"roleId"`
}

func Defaults() *Config {
	return &Config{
		Name:            "企业统一登录",
		Preset:          "custom",
		Scopes:          "profile email",
		UsernameClaim:   "preferred_username",
		NameClaim:       "name",
		EmailClaim:      "email",
		DepartmentClaim: "department",
		GroupsClaim:     "groups",
		RoleMappings:    []RoleMapping{},
		AutoCreate:      true,
	}
}

// Validate 规范化配置，返回给管理员看的错误。
func (c *Config) Validate() string {
	c.FeishuAPIBase = strings.TrimRight(strings.TrimSpace(c.FeishuAPIBase), "/")
	c.FeishuVerificationToken = strings.TrimSpace(c.FeishuVerificationToken)
	for _, p := range []*string{&c.Name, &c.Issuer, &c.ClientID, &c.Scopes, &c.UsernameClaim, &c.NameClaim, &c.EmailClaim, &c.DepartmentClaim, &c.GroupsClaim, &c.Preset} {
		*p = strings.TrimSpace(*p)
	}
	c.Issuer = strings.TrimRight(c.Issuer, "/")
	if c.RoleMappings == nil {
		c.RoleMappings = []RoleMapping{}
	}
	if c.Name == "" {
		c.Name = "企业统一登录"
	}
	if len([]rune(c.Name)) > 32 {
		return "按钮名字最多 32 个字符"
	}
	if c.UsernameClaim == "" {
		c.UsernameClaim = "preferred_username"
	}
	if !c.Enabled && c.Issuer == "" {
		return ""
	}
	u, err := url.Parse(c.Issuer)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "Issuer 必须是 http(s):// 开头的完整地址，例如 https://sso.acme.com/realms/acme"
	}
	if c.ClientID == "" {
		return "Client ID 必填"
	}
	if c.FeishuAPIBase != "" {
		u, err := url.Parse(c.FeishuAPIBase)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "飞书通讯录 API 地址必须是完整且不带账号密码的 http(s) 地址"
		}
	}
	if c.Preset == "feishu" && c.FeishuNotifyActions && c.FeishuVerificationToken == "" {
		return "开启卡片内通过 / 驳回，要先填飞书的 Verification Token（开发者后台 → 事件与回调 → 加密策略）"
	}
	for i, m := range c.RoleMappings {
		c.RoleMappings[i].Group = strings.TrimSpace(m.Group)
		if c.RoleMappings[i].Group == "" || m.RoleID <= 0 {
			return "组与角色的映射有空项"
		}
	}
	return ""
}

// MatchRoles 返回声明里的组映射到的角色。不区分大小写；Keycloak 的组路径（/dsh/admins）也可以只写最后一段。
func (c *Config) MatchRoles(groups []string) []int64 {
	var out []int64
	for _, m := range c.RoleMappings {
		for _, g := range groups {
			if GroupMatches(m.Group, g) && !slices.Contains(out, m.RoleID) {
				out = append(out, m.RoleID)
				break
			}
		}
	}
	return out
}

// ManagedRoles 是映射里出现的角色：登录时按组增删，其余角色保留管理员手动分配的。
func (c *Config) ManagedRoles() []int64 {
	var out []int64
	for _, m := range c.RoleMappings {
		if !slices.Contains(out, m.RoleID) {
			out = append(out, m.RoleID)
		}
	}
	return out
}

func GroupMatches(want, got string) bool {
	want, got = strings.TrimSpace(want), strings.TrimSpace(got)
	if strings.EqualFold(want, got) || strings.EqualFold(strings.TrimPrefix(want, "/"), strings.TrimPrefix(got, "/")) {
		return true
	}
	if i := strings.LastIndex(got, "/"); i >= 0 && !strings.Contains(want, "/") {
		return strings.EqualFold(want, got[i+1:])
	}
	return false
}

// ---- 发现与密钥 ----

// Provider 是 IdP 的端点，来自 /.well-known/openid-configuration。
type Provider struct {
	Issuer      string `json:"issuer"`
	AuthURL     string `json:"authorization_endpoint"`
	TokenURL    string `json:"token_endpoint"`
	JWKSURL     string `json:"jwks_uri"`
	UserinfoURL string `json:"userinfo_endpoint"`
}

// Client 缓存发现结果与公钥，一个 dsh-ee 进程一个。
type Client struct {
	HTTP *http.Client
	mu   sync.Mutex
	disc map[string]cached[*Provider]
	keys map[string]cached[map[string]crypto.PublicKey]
}

type cached[T any] struct {
	v  T
	at time.Time
}

const cacheFor = 10 * time.Minute

func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{HTTP: hc, disc: map[string]cached[*Provider]{}, keys: map[string]cached[map[string]crypto.PublicKey]{}}
}

func (c *Client) getJSON(ctx context.Context, u string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 返回 %d：%s", u, resp.StatusCode, clip(string(body), 200))
	}
	return json.Unmarshal(body, dst)
}

// Discover 读取 IdP 的发现配置（缓存 10 分钟）。
func (c *Client) Discover(ctx context.Context, issuer string) (*Provider, error) {
	issuer = strings.TrimRight(issuer, "/")
	c.mu.Lock()
	if x, ok := c.disc[issuer]; ok && time.Since(x.at) < cacheFor {
		c.mu.Unlock()
		return x.v, nil
	}
	c.mu.Unlock()
	var p Provider
	if err := c.getJSON(ctx, issuer+"/.well-known/openid-configuration", &p); err != nil {
		return nil, fmt.Errorf("读取发现配置失败：%w", err)
	}
	if p.AuthURL == "" || p.TokenURL == "" || p.JWKSURL == "" {
		return nil, errors.New("发现配置里缺少 authorization_endpoint / token_endpoint / jwks_uri")
	}
	// Azure AD 多租户的 issuer 里带 {tenantid} 占位，这时以管理员填的为准
	if p.Issuer == "" || strings.Contains(p.Issuer, "{tenantid}") {
		p.Issuer = issuer
	}
	if strings.TrimRight(p.Issuer, "/") != issuer {
		return nil, fmt.Errorf("发现配置里的 issuer（%s）与填写的不一致", p.Issuer)
	}
	c.mu.Lock()
	c.disc[issuer] = cached[*Provider]{&p, time.Now()}
	c.mu.Unlock()
	return &p, nil
}

func (c *Client) jwks(ctx context.Context, u string, refresh bool) (map[string]crypto.PublicKey, error) {
	c.mu.Lock()
	if x, ok := c.keys[u]; ok && !refresh && time.Since(x.at) < cacheFor {
		c.mu.Unlock()
		return x.v, nil
	}
	c.mu.Unlock()
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := c.getJSON(ctx, u, &set); err != nil {
		return nil, fmt.Errorf("读取签名公钥失败：%w", err)
	}
	out := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if pub, err := k.publicKey(); err == nil {
			out[k.Kid] = pub
		}
	}
	c.mu.Lock()
	c.keys[u] = cached[map[string]crypto.PublicKey]{out, time.Now()}
	c.mu.Unlock()
	return out, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func b64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func (k jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err := b64(k.N)
		if err != nil {
			return nil, err
		}
		e, err := b64(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		default:
			return nil, fmt.Errorf("不支持的曲线 %s", k.Crv)
		}
		x, err := b64(k.X)
		if err != nil {
			return nil, err
		}
		y, err := b64(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
	}
	return nil, fmt.Errorf("不支持的密钥类型 %s", k.Kty)
}

// ---- 授权码流程 ----

// AuthCodeURL 是跳转到 IdP 登录页的地址（带 PKCE 的 S256 挑战）。
func AuthCodeURL(p *Provider, cfg *Config, redirect, state, nonce, verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	scopes := []string{"openid"}
	for _, s := range strings.Fields(cfg.Scopes) {
		if s != "openid" && !slices.Contains(scopes, s) {
			scopes = append(scopes, s)
		}
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirect},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(p.AuthURL, "?") {
		sep = "&"
	}
	return p.AuthURL + sep + q.Encode()
}

type tokens struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// Exchange 用授权码换令牌，校验 ID Token，再合并 userinfo 的声明（userinfo 优先补缺，不覆盖 ID Token）。
func (c *Client) Exchange(ctx context.Context, p *Provider, cfg *Config, redirect, code, verifier, nonce string) (Claims, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("换取令牌失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tk tokens
	_ = json.Unmarshal(body, &tk)
	if resp.StatusCode != http.StatusOK || tk.IDToken == "" {
		msg := tk.Error
		if tk.Description != "" {
			msg += "：" + tk.Description
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d %s", resp.StatusCode, clip(string(body), 200))
		}
		return nil, fmt.Errorf("换取令牌失败：%s", msg)
	}
	claims, err := c.Verify(ctx, p, cfg.ClientID, tk.IDToken, nonce)
	if err != nil {
		return nil, err
	}
	if p.UserinfoURL != "" && tk.AccessToken != "" {
		if info, err := c.userinfo(ctx, p.UserinfoURL, tk.AccessToken); err == nil && (info["sub"] == nil || info["sub"] == claims["sub"]) {
			for k, v := range info {
				if _, ok := claims[k]; !ok {
					claims[k] = v
				}
			}
		}
	}
	return claims, nil
}

func (c *Client) userinfo(ctx context.Context, u, access string) (Claims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo 返回 %d", resp.StatusCode)
	}
	var out Claims
	return out, json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
}

// Verify 校验 ID Token：签名、iss、aud、exp、nonce。
func (c *Client) Verify(ctx context.Context, p *Provider, clientID, raw, nonce string) (Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("ID Token 格式不对")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	hb, err := b64(parts[0])
	if err != nil || json.Unmarshal(hb, &hdr) != nil {
		return nil, errors.New("ID Token 头部不对")
	}
	sig, err := b64(parts[2])
	if err != nil {
		return nil, errors.New("ID Token 签名不对")
	}
	signed := []byte(parts[0] + "." + parts[1])
	keys, err := c.jwks(ctx, p.JWKSURL, false)
	if err != nil {
		return nil, err
	}
	key := pickKey(keys, hdr.Kid)
	if key == nil {
		// IdP 轮换了密钥：强制刷新一次
		if keys, err = c.jwks(ctx, p.JWKSURL, true); err != nil {
			return nil, err
		}
		if key = pickKey(keys, hdr.Kid); key == nil {
			return nil, errors.New("找不到签名 ID Token 的公钥")
		}
	}
	if err := verifySig(hdr.Alg, key, signed, sig); err != nil {
		return nil, err
	}
	pb, err := b64(parts[1])
	if err != nil {
		return nil, errors.New("ID Token 内容不对")
	}
	var claims Claims
	dec := json.NewDecoder(strings.NewReader(string(pb)))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return nil, errors.New("ID Token 内容不对")
	}
	if iss, _ := claims["iss"].(string); strings.TrimRight(iss, "/") != strings.TrimRight(p.Issuer, "/") {
		return nil, fmt.Errorf("ID Token 的签发方不对：%s", iss)
	}
	if !claims.audience(clientID) {
		return nil, errors.New("ID Token 不是签发给这个 Client ID 的")
	}
	now := time.Now().Unix()
	if exp, ok := claims.num("exp"); !ok || now > exp+60 {
		return nil, errors.New("ID Token 已过期")
	}
	if n, _ := claims["nonce"].(string); n != nonce {
		return nil, errors.New("ID Token 的 nonce 对不上，请重新登录")
	}
	return claims, nil
}

func pickKey(keys map[string]crypto.PublicKey, kid string) crypto.PublicKey {
	if k, ok := keys[kid]; ok {
		return k
	}
	if kid == "" && len(keys) == 1 {
		for _, k := range keys {
			return k
		}
	}
	return nil
}

func verifySig(alg string, key crypto.PublicKey, signed, sig []byte) error {
	var h crypto.Hash
	switch alg {
	case "RS256", "ES256":
		h = crypto.SHA256
	case "RS384", "ES384":
		h = crypto.SHA384
	case "RS512":
		h = crypto.SHA512
	default:
		return fmt.Errorf("不支持的签名算法 %s", alg)
	}
	var digest []byte
	switch h {
	case crypto.SHA256:
		d := sha256.Sum256(signed)
		digest = d[:]
	case crypto.SHA384:
		d := sha512.Sum384(signed)
		digest = d[:]
	default:
		d := sha512.Sum512(signed)
		digest = d[:]
	}
	switch k := key.(type) {
	case *rsa.PublicKey:
		if !strings.HasPrefix(alg, "RS") || rsa.VerifyPKCS1v15(k, h, digest, sig) != nil {
			return errors.New("ID Token 签名校验失败")
		}
	case *ecdsa.PublicKey:
		size := (k.Curve.Params().BitSize + 7) / 8
		if !strings.HasPrefix(alg, "ES") || len(sig) != 2*size {
			return errors.New("ID Token 签名校验失败")
		}
		r, s := new(big.Int).SetBytes(sig[:size]), new(big.Int).SetBytes(sig[size:])
		if !ecdsa.Verify(k, digest, r, s) {
			return errors.New("ID Token 签名校验失败")
		}
	default:
		return errors.New("不支持的公钥类型")
	}
	return nil
}

// ---- 声明 ----

type Claims map[string]any

func (c Claims) audience(clientID string) bool {
	switch a := c["aud"].(type) {
	case string:
		return a == clientID
	case []any:
		for _, x := range a {
			if s, _ := x.(string); s == clientID {
				return true
			}
		}
	}
	return false
}

func (c Claims) num(k string) (int64, bool) {
	switch v := c[k].(type) {
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			f, ferr := v.Float64()
			return int64(f), ferr == nil
		}
		return n, true
	case float64:
		return int64(v), true
	}
	return 0, false
}

// lookup 按点号路径取声明，例如 realm_access.roles。
func (c Claims) lookup(path string) any {
	if path == "" {
		return nil
	}
	if v, ok := c[path]; ok {
		return v
	}
	var cur any = map[string]any(c)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// String 取一个字符串声明；数组取第一项。
func (c Claims) String(path string) string {
	switch v := c.lookup(path).(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		if len(v) > 0 {
			s, _ := v[0].(string)
			return strings.TrimSpace(s)
		}
	case json.Number:
		return v.String()
	}
	return ""
}

// Strings 取一个字符串数组声明；单个字符串也当作一项，逗号分隔的也拆开。
func (c Claims) Strings(path string) []string {
	var out []string
	add := func(s string) {
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	switch v := c.lookup(path).(type) {
	case string:
		add(v)
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				add(s)
			}
		}
	}
	return out
}

// Identity 是按配置从声明里取出的用户资料。
type Identity struct {
	Subject     string   `json:"subject"`
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
	Department  string   `json:"department"`
	Groups      []string `json:"groups"`
}

func (cfg *Config) Identity(c Claims) Identity {
	id := Identity{
		Subject:     c.String("sub"),
		Username:    strings.ToLower(c.String(cfg.UsernameClaim)),
		DisplayName: c.String(cfg.NameClaim),
		Email:       c.String(cfg.EmailClaim),
		Department:  c.String(cfg.DepartmentClaim),
		Groups:      c.Strings(cfg.GroupsClaim),
	}
	if id.Groups == nil {
		id.Groups = []string{}
	}
	return id
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
