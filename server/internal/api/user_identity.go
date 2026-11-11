package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/eejwt"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 按员工本人的身份调用企业系统（docs/plugins.md §7）。
//
// 连接原来只有一份管理员配的共享凭据，源系统分不清是谁在调。这里加一层「员工身份」，和共享凭据并存：
//   - ee_jwt：dsh-ee 为每次调用签一个 5 分钟有效的 JWT，对方用 /.well-known/jwks.json 验签就知道是谁，
//     dsh-ee 不用保存员工在那个系统里的密码；
//   - user_secret：员工在连接页面填自己的 token，按人加密存放。
//
// 权限仍由源系统判定。

const (
	userAuthEEJWT  = "ee_jwt"
	userAuthSecret = "user_secret"
)

// connUserAuth 是连接上的员工身份设置。
type connUserAuth struct {
	// ee_jwt / user_secret
	Mode string `json:"mode"`
	// 放在哪个请求头。空则 Authorization: Bearer <值>；填了就把值原样放进这个头
	Header string `json:"header,omitempty"`
	// ee_jwt：令牌的 aud，对方据此确认令牌是签给自己的。空则用连接标识
	Audience string `json:"audience,omitempty"`
	// ee_jwt：除了 id、用户名、姓名、邮箱，还带哪些员工信息（dept / roles / idp_sub），默认都不带
	Claims []string `json:"claims,omitempty"`
	// user_secret：告诉员工这个凭据去哪里拿
	Hint string `json:"hint,omitempty"`
}

// eeClaimChoices 是管理员可以按连接打开的员工信息。
var eeClaimChoices = []string{"dept", "roles", "idp_sub"}

// headerOf 是这份凭据占用的请求头名字（规范化后），用来检查两份凭据有没有撞在同一个头上。
func (a *connAuth) headerOf() string {
	switch {
	case a == nil || a.Type == "none":
		return ""
	case a.Type == "header":
		return http.CanonicalHeaderKey(a.Header)
	}
	return "Authorization"
}

func (u *connUserAuth) headerOf() string {
	if u.Header != "" {
		return http.CanonicalHeaderKey(u.Header)
	}
	return "Authorization"
}

// validateUserAuth 检查并规范化员工身份设置；没开时把字段清成 nil。
func validateUserAuth(sp *assetSpec) string {
	u := sp.UserAuth
	if u == nil || u.Mode == "" || u.Mode == "none" {
		sp.UserAuth = nil
		return ""
	}
	if u.Mode != userAuthEEJWT && u.Mode != userAuthSecret {
		return "员工身份只能是 ee_jwt 或 user_secret"
	}
	u.Header = strings.TrimSpace(u.Header)
	if u.Header != "" && !headerNameRe.MatchString(u.Header) {
		return "员工身份的请求头名字不对，例如 X-User-Token"
	}
	if u.headerOf() == sp.Auth.headerOf() {
		return "共享凭据和员工身份不能放在同一个请求头（" + u.headerOf() + "）里，请给其中一个换个请求头"
	}
	if u.Mode == userAuthEEJWT {
		u.Hint = ""
		u.Audience = strings.TrimSpace(u.Audience)
		if len(u.Audience) > 128 || strings.ContainsAny(u.Audience, " \t\r\n") {
			return "aud 最多 128 个字符，不能有空格"
		}
		u.Claims = dedupeStrings(u.Claims)
		for _, c := range u.Claims {
			if !slices.Contains(eeClaimChoices, c) {
				return "ee_jwt 能额外带的员工信息只有 " + strings.Join(eeClaimChoices, " / ")
			}
		}
		if len(u.Claims) == 0 {
			u.Claims = nil
		}
	} else {
		u.Audience, u.Claims = "", nil
		u.Hint = strings.TrimSpace(u.Hint)
		if !limitRunes(u.Hint, 300) {
			return "给员工的说明最多 300 个字符"
		}
	}
	return ""
}

func dedupeStrings(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// errNeedCredential：这位员工还没在这个连接上填自己的凭据。
var errNeedCredential = errors.New("需要员工本人的凭据")

// userAuthHeader 为这位员工的这一次调用拼出员工身份的请求头。base 是员工访问 dsh-ee 的地址，作 ee_jwt 的签发方。
func (s *Server) userAuthHeader(ctx context.Context, a *store.Asset, sp *assetSpec, v viewer, base string) (http.Header, error) {
	h := http.Header{}
	u := sp.UserAuth
	if u == nil {
		return h, nil
	}
	var value string
	switch u.Mode {
	case userAuthEEJWT:
		tok, err := s.signEEJWT(ctx, a, u, v, base)
		if err != nil {
			return nil, err
		}
		value = tok
	case userAuthSecret:
		enc, err := s.store.UserCredential(ctx, v.user.ID, a.ID)
		if err != nil {
			return nil, err
		}
		if len(enc) == 0 {
			return nil, errNeedCredential
		}
		plain, err := s.box.Open(enc)
		if err != nil {
			return nil, fmt.Errorf("解密员工凭据（主密钥是否换过？）：%w", err)
		}
		value = string(plain)
	}
	if u.Header == "" {
		h.Set("Authorization", "Bearer "+value)
	} else {
		h.Set(u.Header, value)
	}
	return h, nil
}

// ---- ee_jwt 的签名密钥 ----

const (
	eeKeysSetting = "ee_jwt_keys"
	// 轮换时新钥匙先公开、过这么久才开始签名：验签方多半缓存着 JWKS，要给它们时间取到新公钥
	eeKeyDelay = 10 * time.Minute
	// 被换下来的公钥继续公开多久
	eeKeyGrace = 24 * time.Hour
	// 内存里的密钥多久重新读一次库：多个副本靠这个在轮换后对齐（要比 eeKeyDelay 短）
	eeKeysTTL = time.Minute
)

// eeStoredKey 是存在 settings 里的一把密钥。私钥用主密钥加密，几个副本共用。
// 钥匙按 ActiveAt 排队：到了时间的最后一把用来签名，它前面的就算换下来了。
type eeStoredKey struct {
	KID       string    `json:"kid"`
	Private   string    `json:"private"`
	CreatedAt time.Time `json:"created_at"`
	ActiveAt  time.Time `json:"active_at"`
}

type eeStoredKeys struct {
	Keys []eeStoredKey `json:"keys"`
}

// eeKeyInfo 是给管理员看的一把钥匙：在用（active）、还没到时间（pending）、已换下来（retired）。
type eeKeyInfo struct {
	KID       string    `json:"kid"`
	CreatedAt time.Time `json:"created_at"`
	ActiveAt  time.Time `json:"active_at"`
	State     string    `json:"state"`
}

type eeLoadedKey struct {
	key      *eejwt.Key
	created  time.Time
	activeAt time.Time
}

type eeKeyring struct {
	mu   sync.Mutex
	at   time.Time
	keys []eeLoadedKey
}

// eeSigning 是此刻该用哪一把签名：到了时间的最后一把。
func eeSigning(keys []eeLoadedKey, now time.Time) int {
	at := -1
	for i, k := range keys {
		if !k.activeAt.After(now) {
			at = i
		}
	}
	return at
}

// eeKeys 返回当前用来签名的密钥、要公开的公钥和密钥列表。第一次用到时生成一把。
func (s *Server) eeKeys(ctx context.Context) (*eejwt.Key, eejwt.JWKS, []eeKeyInfo, error) {
	k := &s.eeKeyring
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.keys) == 0 || time.Since(k.at) >= eeKeysTTL {
		loaded, err := s.loadEEKeys(ctx)
		if err != nil {
			return nil, eejwt.JWKS{}, nil, err
		}
		k.keys, k.at = loaded, time.Now()
	}
	// 读完库再取时间：第一把钥匙是读库时现生成的，时间取早了它就还「没到时间」
	now := time.Now()
	cur := eeSigning(k.keys, now)
	if cur < 0 {
		return nil, eejwt.JWKS{}, nil, errors.New("没有可用的 ee_jwt 签名密钥")
	}
	jwks := eejwt.JWKS{Keys: []eejwt.JWK{}}
	infos := []eeKeyInfo{}
	for i, lk := range k.keys {
		state := "active"
		switch {
		case i > cur:
			state = "pending"
		case i < cur:
			state = "retired"
			// 换下来超过宽限期的不再公开
			if now.Sub(k.keys[i+1].activeAt) > eeKeyGrace {
				continue
			}
		}
		jwks.Keys = append(jwks.Keys, lk.key.Public())
		infos = append(infos, eeKeyInfo{KID: lk.key.ID, CreatedAt: lk.created, ActiveAt: lk.activeAt, State: state})
	}
	return k.keys[cur].key, jwks, infos, nil
}

// loadEEKeys 从库里读出全部钥匙并解开；一把都没有时生成第一把（立即生效）。
func (s *Server) loadEEKeys(ctx context.Context) ([]eeLoadedKey, error) {
	var stored eeStoredKeys
	if _, err := s.store.GetSettingJSON(ctx, eeKeysSetting, &stored); err != nil {
		return nil, err
	}
	if len(stored.Keys) == 0 {
		// 几个副本可能同时走到这里，放进事务里：后到的会看到先到的那一把，不再另生成
		err := s.store.UpdateSettingJSON(ctx, eeKeysSetting, 0, func(old []byte) ([]byte, error) {
			stored = eeStoredKeys{}
			if len(old) > 0 {
				if err := json.Unmarshal(old, &stored); err != nil {
					return nil, err
				}
			}
			if len(stored.Keys) > 0 {
				return nil, nil
			}
			// 第一把立即生效。生效时间往前放一点：几个副本的时钟有先后，慢的那台不能觉得它还没到时间
			nk, err := s.newEEKey(time.Now().Add(-5 * time.Minute))
			if err != nil {
				return nil, err
			}
			stored.Keys = append(stored.Keys, *nk)
			return json.Marshal(stored)
		})
		if err != nil {
			return nil, fmt.Errorf("生成 ee_jwt 签名密钥：%w", err)
		}
	}
	out := make([]eeLoadedKey, 0, len(stored.Keys))
	for _, sk := range stored.Keys {
		sealed, err := base64.StdEncoding.DecodeString(sk.Private)
		if err != nil {
			return nil, fmt.Errorf("ee_jwt 密钥 %s 损坏：%w", sk.KID, err)
		}
		der, err := s.box.Open(sealed)
		if err != nil {
			return nil, fmt.Errorf("解密 ee_jwt 密钥 %s（主密钥是否换过？）：%w", sk.KID, err)
		}
		key, err := eejwt.Parse(der)
		if err != nil {
			return nil, fmt.Errorf("ee_jwt 密钥 %s 损坏：%w", sk.KID, err)
		}
		out = append(out, eeLoadedKey{key: key, created: sk.CreatedAt, activeAt: sk.ActiveAt})
	}
	slices.SortStableFunc(out, func(a, b eeLoadedKey) int { return a.activeAt.Compare(b.activeAt) })
	return out, nil
}

func (s *Server) newEEKey(activeAt time.Time) (*eeStoredKey, error) {
	key, err := eejwt.Generate()
	if err != nil {
		return nil, err
	}
	der, err := key.Marshal()
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.Seal(der)
	if err != nil {
		return nil, err
	}
	return &eeStoredKey{KID: key.ID, Private: base64.StdEncoding.EncodeToString(sealed), CreatedAt: time.Now().UTC().Truncate(time.Second), ActiveAt: activeAt.UTC().Truncate(time.Millisecond)}, nil
}

// rotateEEKey 换一把新的签名密钥：新公钥马上公开，过 eeKeyDelay 才开始用它签名；
// 旧钥匙在那之前照常签名，之后公钥再公开 eeKeyGrace。已经有一把在排队时不再生成，返回那一把。
func (s *Server) rotateEEKey(ctx context.Context, by int64) (string, error) {
	delay := eeKeyDelay
	if s.opts.EEKeyDelay > 0 {
		delay = s.opts.EEKeyDelay
	}
	var kid string
	err := s.store.UpdateSettingJSON(ctx, eeKeysSetting, by, func(old []byte) ([]byte, error) {
		var stored eeStoredKeys
		if len(old) > 0 {
			if err := json.Unmarshal(old, &stored); err != nil {
				return nil, err
			}
		}
		now := time.Now()
		slices.SortStableFunc(stored.Keys, func(a, b eeStoredKey) int { return a.ActiveAt.Compare(b.ActiveAt) })
		if n := len(stored.Keys); n > 0 && stored.Keys[n-1].ActiveAt.After(now) {
			kid = stored.Keys[n-1].KID
			return nil, nil
		}
		// 顺手清掉早就不公开的：换下来超过宽限期的
		kept := stored.Keys[:0]
		for i, sk := range stored.Keys {
			if i+1 < len(stored.Keys) && now.Sub(stored.Keys[i+1].ActiveAt) > eeKeyGrace {
				continue
			}
			kept = append(kept, sk)
		}
		nk, err := s.newEEKey(now.Add(delay))
		if err != nil {
			return nil, err
		}
		kid = nk.KID
		return json.Marshal(eeStoredKeys{Keys: append(kept, *nk)})
	})
	if err != nil {
		return "", err
	}
	s.eeKeyring.mu.Lock()
	s.eeKeyring.keys = nil
	s.eeKeyring.mu.Unlock()
	return kid, nil
}

// signEEJWT 为这位员工对这个连接的一次调用签一个令牌。
// 默认只带 id、用户名、姓名、邮箱；部门、角色、身份提供方里的标识由管理员按连接打开。
func (s *Server) signEEJWT(ctx context.Context, a *store.Asset, u *connUserAuth, v viewer, base string) (string, error) {
	key, _, _, err := s.eeKeys(ctx)
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", errors.New("还不知道 dsh-ee 的访问地址，签不了 ee_jwt：请在系统设置里填写「访问地址」")
	}
	aud := u.Audience
	if aud == "" {
		aud = a.Slug
	}
	now := time.Now()
	claims := map[string]any{
		"iss": base, "aud": aud, "sub": strconv.FormatInt(v.user.ID, 10),
		"preferred_username": v.user.Username, "name": displayOf(v.user),
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(eejwt.Lifetime).Unix(), "jti": secret.Token(),
	}
	if v.user.Email != "" {
		claims["email"] = v.user.Email
	}
	for _, c := range u.Claims {
		switch c {
		case "dept":
			claims["dept"] = v.user.Department
		case "roles":
			roles := []string{}
			if v.access != nil {
				roles = append(roles, v.access.RoleCodes...)
			}
			claims["roles"] = roles
		case "idp_sub":
			// 企业目录 / 统一登录里的标识（LDAP 的 DN、OIDC 的 sub），对方系统可以直接按它认人
			if v.user.ExternalDN != "" {
				claims["idp_sub"] = v.user.ExternalDN
			}
		}
	}
	return eejwt.Sign(key, claims)
}

// GET /.well-known/jwks.json：ee_jwt 的验签公钥，不需要登录。
func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	_, jwks, _, err := s.eeKeys(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, jwks)
}

// GET /api/admin/ee-jwt：给管理员看签发方、JWKS 地址和密钥列表（不含私钥）。
func (s *Server) handleEEJWTInfo(w http.ResponseWriter, r *http.Request) {
	_, _, infos, err := s.eeKeys(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	base, _ := s.connectBase(r.Context(), r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": base, "jwks_url": base + "/.well-known/jwks.json", "keys": infos,
		"lifetime_seconds": int(eejwt.Lifetime.Seconds()), "claims": eeClaimChoices,
		"rotate_delay_seconds": int(eeKeyDelay.Seconds()),
	})
}

// POST /api/admin/ee-jwt/rotate：换一把签名密钥。
func (s *Server) handleEEJWTRotate(w http.ResponseWriter, r *http.Request) {
	kid, err := s.rotateEEKey(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditEEJWTRotate, kid, nil)
	s.handleEEJWTInfo(w, r)
}

// ---- 员工自己的凭据（user_secret）----

// myCredential 是员工在一个连接上看到的员工身份状态。
type myCredential struct {
	Mode string `json:"mode"`
	// user_secret：管理员写的说明、填过没有、末尾几位、什么时候填的
	Hint      string     `json:"hint,omitempty"`
	Filled    bool       `json:"filled"`
	Tail      string     `json:"tail,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func credentialView(u *connUserAuth, info map[int64]store.UserCredentialInfo, assetID int64) *myCredential {
	if u == nil {
		return nil
	}
	c := &myCredential{Mode: u.Mode, Hint: u.Hint}
	if u.Mode == userAuthEEJWT {
		// 身份令牌由 dsh-ee 现签，员工什么都不用填
		c.Filled = true
		return c
	}
	if got, ok := info[assetID]; ok {
		c.Filled, c.Tail = true, got.Hint
		at := got.UpdatedAt
		c.UpdatedAt = &at
	}
	return c
}

// loadSecretConnection 取出员工要填凭据的连接：要在自己的可用范围内，而且这个连接用的是 user_secret。
func (s *Server) loadSecretConnection(w http.ResponseWriter, r *http.Request) (*store.Asset, bool) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return nil, false
	}
	// 管连接的管理员不在可用范围里也能填：同步工具清单时用的是他自己的凭据
	if v := viewerOf(r); a.Status != store.AssetActive || a.LiveVersion == 0 || (!available(v, a) && !v.manages(a.Kind)) {
		writeErr(w, http.StatusNotFound, "连接不存在")
		return nil, false
	}
	lv, err := s.store.LiveAssetVersion(r.Context(), a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	if u := parseSpec(lv.Spec).UserAuth; u == nil || u.Mode != userAuthSecret {
		writeErr(w, http.StatusBadRequest, "这个连接不需要你填自己的凭据")
		return nil, false
	}
	return a, true
}

// PUT /api/me/connections/{id}/credential {secret}：填或更新自己在这个连接上的凭据。
func (s *Server) handleSetMyCredential(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadSecretConnection(w, r)
	if !ok {
		return
	}
	var req struct {
		Secret string `json:"secret"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	plain := strings.TrimSpace(req.Secret)
	if plain == "" || len(plain) > 2048 || strings.ContainsAny(plain, "\r\n") {
		writeErr(w, http.StatusBadRequest, "凭据不能为空，最多 2048 个字符，不能换行")
		return
	}
	sealed, err := s.box.Seal([]byte(plain))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	u := currentUser(r)
	if err := s.store.SetUserCredential(r.Context(), u.ID, a.ID, sealed, secret.Hint(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.dropUserConnClient(a.ID, u.ID)
	s.audit(r, auditCredentialSet, a.Slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/me/connections/{id}/credential：删掉自己在这个连接上的凭据。
func (s *Server) handleDeleteMyCredential(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	u := currentUser(r)
	gone, err := s.store.DeleteUserCredential(r.Context(), u.ID, a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if gone {
		s.dropUserConnClient(a.ID, u.ID)
		s.audit(r, auditCredentialDelete, a.Slug, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/admin/connections/{id}/credentials：哪些员工在这个连接上填了自己的凭据（看不到内容）。
func (s *Server) handleConnCredentials(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	holders, err := s.store.CredentialHolders(r.Context(), a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holders": holders})
}

// DELETE /api/admin/connections/{id}/credentials/{user}：收回一位员工在这个连接上的凭据，他要重新填才能再用。
func (s *Server) handleRevokeConnCredential(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("user"), 10, 64)
	if err != nil || uid <= 0 {
		writeErr(w, http.StatusBadRequest, "用户 id 不对")
		return
	}
	gone, err := s.store.DeleteUserCredential(r.Context(), uid, a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !gone {
		writeErr(w, http.StatusNotFound, "这位员工没有在这个连接上填过凭据")
		return
	}
	s.dropUserConnClient(a.ID, uid)
	s.audit(r, auditCredentialRevoke, a.Slug, map[string]any{"user_id": uid})
	w.WriteHeader(http.StatusNoContent)
}

// purgeUserCredentials 在员工被停用时删掉他按人存的凭据（docs/plugins.md §7.6）。
func (s *Server) purgeUserCredentials(ctx context.Context, userID int64) {
	n, err := s.store.DeleteUserCredentials(ctx, userID)
	if err != nil {
		s.log.Warn("删除停用员工的连接凭据失败", "user", userID, "err", err)
		return
	}
	if n > 0 {
		s.conns.mu.Lock()
		for k := range s.conns.m {
			if k.user == userID {
				delete(s.conns.m, k)
			}
		}
		s.conns.mu.Unlock()
		s.log.Info("已删除停用员工的连接凭据", "user", userID, "count", n)
	}
}
