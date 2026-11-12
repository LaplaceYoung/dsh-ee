package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/feishu"
	"github.com/hikariming/dsh-ee/server/internal/oidc"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// OIDC 登录：和 LDAP 并列，员工在登录页点「用企业统一登录」跳到 IdP，回来时按声明建号、同步资料与角色。
// 配置单独存一行设置（里面有 client secret），声明 → 角色映射沿用 LDAP 组映射的做法。

const (
	oidcSettingsKey = "oidc"
	oidcStateCookie = "dsh_ee_oidc"
	oidcCallback    = "/api/auth/oidc/callback"
	oidcStateTTL    = 10 * time.Minute
)

type oidcStored struct {
	oidc.Config
	ClientSecretEnc string `json:"clientSecretEnc,omitempty"`
	// 飞书回调的 Encrypt Key，同样加密保存
	FeishuEncryptKeyEnc string `json:"feishuEncryptKeyEnc,omitempty"`
}

func (s *Server) loadOIDC(ctx context.Context) (*oidc.Config, error) {
	st := oidcStored{Config: *oidc.Defaults()}
	if _, err := s.store.GetSettingJSON(ctx, oidcSettingsKey, &st); err != nil {
		return nil, err
	}
	if st.RoleMappings == nil {
		st.RoleMappings = []oidc.RoleMapping{}
	}
	cfg := st.Config
	if st.ClientSecretEnc != "" {
		raw, err := base64.StdEncoding.DecodeString(st.ClientSecretEnc)
		if err != nil {
			return nil, err
		}
		pw, err := s.box.Open(raw)
		if err != nil {
			return nil, fmt.Errorf("解密 OIDC client secret（主密钥是否换过？）: %w", err)
		}
		cfg.ClientSecret = string(pw)
	}
	if st.FeishuEncryptKeyEnc != "" {
		raw, err := base64.StdEncoding.DecodeString(st.FeishuEncryptKeyEnc)
		if err != nil {
			return nil, err
		}
		key, err := s.box.Open(raw)
		if err != nil {
			return nil, fmt.Errorf("解密飞书 Encrypt Key（主密钥是否换过？）: %w", err)
		}
		cfg.FeishuEncryptKey = string(key)
	}
	return &cfg, nil
}

func (s *Server) saveOIDC(ctx context.Context, cfg *oidc.Config, userID int64) error {
	st := oidcStored{Config: *cfg}
	if cfg.ClientSecret != "" {
		sealed, err := s.box.Seal([]byte(cfg.ClientSecret))
		if err != nil {
			return err
		}
		st.ClientSecretEnc = base64.StdEncoding.EncodeToString(sealed)
	}
	if cfg.FeishuEncryptKey != "" {
		sealed, err := s.box.Seal([]byte(cfg.FeishuEncryptKey))
		if err != nil {
			return err
		}
		st.FeishuEncryptKeyEnc = base64.StdEncoding.EncodeToString(sealed)
	}
	return s.store.PutSettingJSON(ctx, oidcSettingsKey, st, userID)
}

func (s *Server) oidcEnabled(ctx context.Context) (*oidc.Config, bool) {
	cfg, err := s.loadOIDC(ctx)
	if err != nil || !cfg.Enabled || cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, false
	}
	return cfg, true
}

func (s *Server) oidcHTTP() *oidc.Client {
	s.oidcOnce.Do(func() {
		hc := s.opts.OIDCClient
		if hc == nil {
			hc = &http.Client{Timeout: 15 * time.Second, Transport: s.opts.DemoSite.guard(&http.Transport{Proxy: http.ProxyFromEnvironment})}
		}
		s.oidcClient = oidc.NewClient(hc)
	})
	return s.oidcClient
}

func (s *Server) oidcRedirect(r *http.Request) string { return s.publicBase(r) + oidcCallback }

// ---- 管理接口 ----

type oidcView struct {
	oidc.Config
	ClientSecretSet bool `json:"clientSecretSet"`
	// 飞书 Encrypt Key 只告诉页面填没填，不回显
	FeishuEncryptKeySet bool   `json:"feishuEncryptKeySet"`
	RedirectURI         string `json:"redirectUri"`
}

// GET /api/admin/auth/oidc
func (s *Server) handleGetOIDC(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadOIDC(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, oidcView{Config: *cfg, ClientSecretSet: cfg.ClientSecret != "", FeishuEncryptKeySet: cfg.FeishuEncryptKey != "", RedirectURI: s.oidcRedirect(r)})
}

// oidcInput：clientSecret 不传表示沿用已存的，传空串表示清除。
type oidcInput struct {
	oidc.Config
	ClientSecret *string `json:"clientSecret"`
	// 同 clientSecret：不传沿用已存的，传空串清除
	FeishuEncryptKey    *string `json:"feishuEncryptKey"`
	FeishuEncryptKeySet *bool   `json:"feishuEncryptKeySet"`
	// 只读字段，页面会原样带回来
	ClientSecretSet *bool   `json:"clientSecretSet"`
	RedirectURI     *string `json:"redirectUri"`
}

func (s *Server) resolveOIDCInput(ctx context.Context, in *oidcInput) (*oidc.Config, string, error) {
	cfg := in.Config
	if in.ClientSecret != nil {
		cfg.ClientSecret = strings.TrimSpace(*in.ClientSecret)
	} else {
		cur, err := s.loadOIDC(ctx)
		if err != nil {
			return nil, "", err
		}
		cfg.ClientSecret = cur.ClientSecret
	}
	if in.FeishuEncryptKey != nil {
		cfg.FeishuEncryptKey = strings.TrimSpace(*in.FeishuEncryptKey)
	} else {
		cur, err := s.loadOIDC(ctx)
		if err != nil {
			return nil, "", err
		}
		cfg.FeishuEncryptKey = cur.FeishuEncryptKey
	}
	if msg := cfg.Validate(); msg != "" {
		return nil, msg, nil
	}
	return &cfg, "", nil
}

// PUT /api/admin/auth/oidc
func (s *Server) handlePutOIDC(w http.ResponseWriter, r *http.Request) {
	var in oidcInput
	if !readJSON(w, r, &in) {
		return
	}
	cfg, msg, err := s.resolveOIDCInput(r.Context(), &in)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	cur, err := s.loadOIDC(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 和 LDAP 一样：声明 → 角色映射等于给一批人授权，要有「管理角色」权限且只能映射自己能授予的角色
	if !slices.Equal(cur.RoleMappings, cfg.RoleMappings) {
		if !currentAccess(r).Has(rbac.RolesManage) {
			writeErr(w, http.StatusForbidden, "修改组与角色的映射需要「管理角色与权限」权限")
			return
		}
		if msg, ok := s.checkGrantable(r, cfg.ManagedRoles()); !ok {
			writeErr(w, http.StatusForbidden, msg)
			return
		}
	}
	if err := s.saveOIDC(r.Context(), cfg, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditOIDCUpdate, cfg.Issuer, map[string]any{"enabled": cfg.Enabled, "mappings": len(cfg.RoleMappings), "secret_changed": in.ClientSecret != nil})
	s.handleGetOIDC(w, r)
}

// POST /api/admin/auth/oidc/test：用页面上还没保存的配置读一次发现配置与公钥。
func (s *Server) handleTestOIDC(w http.ResponseWriter, r *http.Request) {
	var in oidcInput
	if !readJSON(w, r, &in) {
		return
	}
	cfg, msg, err := s.resolveOIDCInput(r.Context(), &in)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if cfg.Issuer == "" {
		writeErr(w, http.StatusBadRequest, "请先填 Issuer")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	p, err := s.oidcHTTP().Discover(ctx, cfg.Issuer)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": p, "redirectUri": s.oidcRedirect(r)})
}

// ---- 登录流程 ----

type oidcPending struct {
	nonce, verifier, next string
	exp                   time.Time
}

type oidcStates struct {
	mu sync.Mutex
	m  map[string]oidcPending
}

func (o *oidcStates) put(state string, p oidcPending) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.m == nil {
		o.m = map[string]oidcPending{}
	}
	now := time.Now()
	for k, v := range o.m {
		if now.After(v.exp) {
			delete(o.m, k)
		}
	}
	o.m[state] = p
}

func (o *oidcStates) take(state string) (oidcPending, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	p, ok := o.m[state]
	delete(o.m, state)
	return p, ok && time.Now().Before(p.exp)
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// safeNext 只允许站内相对路径，防止被拿来做开放重定向。
func safeNext(raw string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && !strings.HasPrefix(raw, "/\\") {
		return raw
	}
	return "/go"
}

// loginError 回到登录页并带上原因（登录页显示）。
func loginError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/login?oidc_error="+url.QueryEscape(msg), http.StatusFound)
}

// GET /api/auth/oidc/start?next=…：跳到 IdP 登录。
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.oidcEnabled(r.Context())
	if !ok {
		loginError(w, r, "没有开启企业统一登录")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	p, err := s.oidcHTTP().Discover(ctx, cfg.Issuer)
	if err != nil {
		s.log.Warn("OIDC 发现配置读取失败", "err", err)
		loginError(w, r, "企业统一登录暂时不可用，请稍后再试或联系管理员")
		return
	}
	state, nonce, verifier := randString(24), randString(24), randString(48)
	s.oidcStates.put(state, oidcPending{nonce: nonce, verifier: verifier, next: safeNext(r.URL.Query().Get("next")), exp: time.Now().Add(oidcStateTTL)})
	// state 同时放进 cookie：回调时比对，防止别人把自己的登录结果塞进你的浏览器
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: state, Path: "/api/auth/oidc", MaxAge: int(oidcStateTTL.Seconds()), HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, oidc.AuthCodeURL(p, cfg, s.oidcRedirect(r), state, nonce, verifier), http.StatusFound)
}

// GET /api/auth/oidc/callback?code=…&state=…
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: "", Path: "/api/auth/oidc", MaxAge: -1, HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
	if e := q.Get("error"); e != "" {
		loginError(w, r, "企业统一登录没有完成："+truncate(strings.TrimSpace(e+" "+q.Get("error_description")), 200))
		return
	}
	state := q.Get("state")
	c, err := r.Cookie(oidcStateCookie)
	if state == "" || err != nil || c.Value != state {
		loginError(w, r, "登录请求已失效，请重新登录")
		return
	}
	pending, ok := s.oidcStates.take(state)
	if !ok {
		loginError(w, r, "登录请求已失效，请重新登录")
		return
	}
	cfg, ok := s.oidcEnabled(r.Context())
	if !ok {
		loginError(w, r, "没有开启企业统一登录")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cl := s.oidcHTTP()
	p, err := cl.Discover(ctx, cfg.Issuer)
	if err == nil {
		var claims oidc.Claims
		claims, err = cl.Exchange(ctx, p, cfg, s.oidcRedirect(r), q.Get("code"), pending.verifier, pending.nonce)
		if err == nil {
			s.finishOIDCLogin(w, r, cfg, cfg.Identity(claims), pending.next)
			return
		}
	}
	s.log.Warn("OIDC 登录失败", "err", err)
	s.auditAs(r, nil, auditLoginFailed, "", map[string]any{"via": "oidc", "reason": truncate(err.Error(), 200)})
	loginError(w, r, "企业统一登录失败："+truncate(err.Error(), 200))
}

func (s *Server) finishOIDCLogin(w http.ResponseWriter, r *http.Request, cfg *oidc.Config, id oidc.Identity, next string) {
	// 飞书本体和其它 OIDC 身份源共用登录流程；给 open_id 加来源前缀，后续组织同步不会误伤其它 IdP 的 sub。
	if cfg.Preset == "feishu" && !strings.HasPrefix(id.Subject, "feishu:") {
		id.Subject = feishu.ExternalSubject(id.Subject)
	}
	u, err := s.provisionOIDC(r.Context(), cfg, id)
	switch {
	case errors.Is(err, errNotProvisioned), errors.Is(err, errBadCredentials), errors.Is(err, errOIDCConflict), errors.Is(err, errOIDCIdentity), errors.Is(err, errOIDCNoSubject):
		s.auditAs(r, nil, auditLoginFailed, id.Username, map[string]any{"via": "oidc", "reason": err.Error()})
		loginError(w, r, err.Error())
		return
	case err != nil:
		s.log.Error("OIDC 建号失败", "err", err)
		loginError(w, r, "登录失败，请联系管理员")
		return
	}
	if err := s.store.TouchLogin(r.Context(), u.ID); err != nil {
		s.log.Warn("记录登录时间失败", "err", err)
	}
	s.auditAs(r, u, auditLogin, u.Username, map[string]any{"via": "oidc", "source": u.AuthSource})
	token := secret.Token()
	if err := s.store.CreateSession(r.Context(), secret.Hash(token), u.ID, s.opts.SessionTTL); err != nil {
		s.internalErr(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(s.opts.SessionTTL.Seconds()), HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, next, http.StatusFound)
}

var (
	errOIDCConflict  = errors.New("dsh-ee 里已有同名的本地或 LDAP 账号，不能用企业统一登录；请联系管理员")
	errOIDCIdentity  = errors.New("这个用户名在 dsh-ee 里已绑定另一个企业身份；如果是你的账号在统一登录里重建过，请联系管理员解除绑定")
	errOIDCNoSubject = errors.New("企业统一登录没有返回用户标识（sub），不能登录；请联系管理员")
)

// oidcUsername 把声明里的用户名变成 dsh-ee 的用户名；邮箱形式的取 @ 前面一段。
func oidcUsername(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !usernameRe.MatchString(name) {
		if i := strings.Index(name, "@"); i > 0 {
			name = name[:i]
		}
	}
	if !usernameRe.MatchString(name) {
		return ""
	}
	return name
}

// provisionOIDC 找到或建好账号，同步资料与映射的角色。规则与 LDAP 一致：
// 本地账号、LDAP 账号与机器人不能借 OIDC 登录；没开自动建号时只有预先建好的 OIDC 账号能登录。
func (s *Server) provisionOIDC(ctx context.Context, cfg *oidc.Config, id oidc.Identity) (*store.User, error) {
	if id.Subject == "" {
		return nil, errOIDCNoSubject
	}
	name := oidcUsername(id.Username)
	if name == "" {
		s.log.Warn("OIDC 声明里没有可用的用户名", "claim", cfg.UsernameClaim, "value", id.Username)
		return nil, errNotProvisioned
	}
	// 先按 sub 认人：用户名声明会变（改名、域名迁移），sub 不变就不应该新建第二个账号。
	var u *store.User
	if bySub, err := s.store.GetUserByExternalDN(ctx, store.AuthOIDC, id.Subject); err != nil {
		if !notFound(err) {
			return nil, err
		}
	} else if bySub != nil {
		u = bySub
		if u.Username != name {
			s.log.Info("OIDC 用户名声明已变化，沿用已绑定 sub 的账号", "user", u.Username, "claim", name)
		}
	}
	if u == nil {
		var err error
		u, err = s.store.GetUserByUsername(ctx, name)
		if err != nil && !notFound(err) {
			return nil, err
		}
	}
	if u != nil && (u.AccountType != store.AccountHuman || u.AuthSource != store.AuthOIDC) {
		return nil, errOIDCConflict
	}
	if u == nil {
		if !cfg.AutoCreate {
			return nil, errNotProvisioned
		}
		zero := int64(0)
		u = &store.User{Username: name, DisplayName: id.DisplayName, Department: id.Department, Email: id.Email,
			AuthSource: store.AuthOIDC, ExternalDN: id.Subject, MonthlyTokenLimit: &zero}
		if err := s.store.CreateUser(ctx, u); err != nil {
			if !errors.Is(err, store.ErrDuplicate) {
				return nil, err
			}
			if u, err = s.store.GetUserByUsername(ctx, name); err != nil {
				return nil, err
			}
		} else {
			roles := s.mappedRoles(ctx, cfg.ManagedRoles(), cfg.MatchRoles(id.Groups), nil)
			if member, err := s.store.GetRoleByCode(ctx, rbac.RoleMember); err == nil && !slices.Contains(cfg.ManagedRoles(), member.ID) {
				roles = dedupe(append(roles, member.ID))
			}
			if err := s.store.SetUserRoles(ctx, u.ID, roles); err != nil {
				return nil, err
			}
			s.log.Info("OIDC 用户首次登录，已创建账号", "user", name, "roles", roles)
			return s.store.GetUser(ctx, u.ID)
		}
	}
	// 账号按用户名找，认的却是 sub：用户名声明会变也会重名（邮箱前缀在另一个域重名、工号回收再分配），
	// 只按用户名认，后来的人会直接登进前一个人的账号。预先建好的账号还没有 sub，第一次登录时绑定。
	if u.ExternalDN != "" && u.ExternalDN != id.Subject {
		s.log.Warn("OIDC 用户名已绑定另一个 sub，拒绝登录", "user", name)
		return nil, errOIDCIdentity
	}
	if u.Disabled {
		return nil, errBadCredentials
	}
	var p store.UserPatch
	changed := false
	set := func(dst **string, cur, next string) {
		if next != "" && next != cur {
			*dst = &next
			changed = true
		}
	}
	set(&p.DisplayName, u.DisplayName, id.DisplayName)
	set(&p.Department, u.Department, id.Department)
	set(&p.Email, u.Email, id.Email)
	set(&p.ExternalDN, u.ExternalDN, id.Subject)
	if changed {
		if err := s.store.UpdateUser(ctx, u.ID, p); err != nil {
			return nil, err
		}
	}
	if len(cfg.RoleMappings) > 0 {
		a, err := s.store.UserAccess(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		next := s.mappedRoles(ctx, cfg.ManagedRoles(), cfg.MatchRoles(id.Groups), a.RoleIDs)
		cur := dedupe(append([]int64{}, a.RoleIDs...))
		slices.Sort(cur)
		slices.Sort(next)
		if !slices.Equal(cur, next) {
			if err := s.store.SetUserRoles(ctx, u.ID, next); err != nil {
				return nil, err
			}
		}
	}
	return s.store.GetUser(ctx, u.ID)
}

// mappedRoles：保留不由映射管理的角色，映射管理的按这次的组重新决定；已删除的角色跳过。
func (s *Server) mappedRoles(ctx context.Context, managed, matched, current []int64) []int64 {
	exists := map[int64]bool{}
	if roles, err := s.store.ListRoles(ctx); err == nil {
		for _, r := range roles {
			exists[r.ID] = true
		}
	}
	out := []int64{}
	for _, id := range current {
		if !slices.Contains(managed, id) {
			out = append(out, id)
		}
	}
	for _, id := range matched {
		if exists[id] {
			out = append(out, id)
		}
	}
	return dedupe(out)
}
