// Package eejwt 签发和校验 ee_jwt：dsh-ee 替员工调用企业系统时带的短期身份令牌。
//
// 企业系统用 dsh-ee 公开的 JWKS（/.well-known/jwks.json）验签，就知道这次调用是哪位员工发起的，
// 权限仍由它自己判定；dsh-ee 不用保存员工在那个系统里的密码。只用标准库，固定 RS256：
// 企业自研系统常用的验签库都支持，不给调用方挑算法的余地。
package eejwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

// Lifetime 是令牌的有效期。只用于一次调用，所以很短；验签方按 exp 拒绝过期的。
const Lifetime = 5 * time.Minute

// Key 是一把签名密钥。
type Key struct {
	ID      string
	Private *rsa.PrivateKey
}

// Generate 生成一把新的签名密钥。kid 取公钥指纹的前 16 个字符，同一把钥匙的 kid 不变。
func Generate() (*Key, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &Key{ID: keyID(&priv.PublicKey), Private: priv}, nil
}

func keyID(pub *rsa.PublicKey) string {
	sum := sha256.Sum256(x509.MarshalPKCS1PublicKey(pub))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}

// Marshal 把私钥编成 PKCS#8，交给调用方加密后存放。
func (k *Key) Marshal() ([]byte, error) { return x509.MarshalPKCS8PrivateKey(k.Private) }

// Parse 读回 Marshal 的结果。
func Parse(der []byte) (*Key, error) {
	v, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	priv, ok := v.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("不是 RSA 私钥")
	}
	return &Key{ID: keyID(&priv.PublicKey), Private: priv}, nil
}

// JWK 是 JWKS 里的一把公钥。
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS 是公开给验签方的公钥集合。
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// Public 返回这把钥匙的公钥，放进 JWKS。
func (k *Key) Public() JWK {
	pub := &k.Private.PublicKey
	return JWK{
		Kty: "RSA", Use: "sig", Alg: "RS256", Kid: k.ID,
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// Sign 用 k 签一个令牌。claims 里的时间用 Unix 秒。
func Sign(k *Key, claims map[string]any) (string, error) {
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": k.ID})
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k.Private, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Expect 是验签方对令牌的要求。
type Expect struct {
	// 签发方，就是 dsh-ee 的访问地址；空则不查
	Issuer string
	// 这个系统自己的标识。必须查：否则给别的系统签的令牌也能拿来调这个系统
	Audience string
	// 验签时的时间，零值用当前时间
	Now time.Time
}

// Verify 校验令牌并返回声明。企业系统那一侧（适配层、服务扩展 SDK）照这个流程做：
// 只认 RS256，按 kid 找公钥，验签，再查 exp / nbf / iss / aud。
func Verify(token string, set JWKS, want Expect) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("令牌格式不对")
	}
	var head struct{ Alg, Kid string }
	if raw, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil || json.Unmarshal(raw, &head) != nil {
		return nil, errors.New("令牌头解析失败")
	}
	if head.Alg != "RS256" {
		return nil, fmt.Errorf("不接受签名算法 %q", head.Alg)
	}
	i := slices.IndexFunc(set.Keys, func(k JWK) bool { return k.Kid == head.Kid && k.Kty == "RSA" })
	if i < 0 {
		return nil, fmt.Errorf("JWKS 里没有 kid 为 %q 的公钥", head.Kid)
	}
	n, err1 := base64.RawURLEncoding.DecodeString(set.Keys[i].N)
	e, err2 := base64.RawURLEncoding.DecodeString(set.Keys[i].E)
	if err1 != nil || err2 != nil || len(n) == 0 || len(e) == 0 || len(e) > 4 {
		return nil, errors.New("JWKS 里的公钥格式不对")
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("签名解析失败")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return nil, errors.New("签名不对")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("令牌内容解析失败")
	}
	claims := map[string]any{}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, errors.New("令牌内容解析失败")
	}
	now := want.Now
	if now.IsZero() {
		now = time.Now()
	}
	exp, ok := claims["exp"].(float64)
	if !ok || !now.Before(time.Unix(int64(exp), 0)) {
		return nil, errors.New("令牌已过期")
	}
	if nbf, ok := claims["nbf"].(float64); ok && now.Before(time.Unix(int64(nbf), 0).Add(-time.Minute)) {
		return nil, errors.New("令牌还没生效")
	}
	if want.Issuer != "" && claims["iss"] != want.Issuer {
		return nil, fmt.Errorf("签发方不对：%v", claims["iss"])
	}
	if want.Audience == "" {
		return nil, errors.New("验签时必须指定 audience")
	}
	if claims["aud"] != want.Audience {
		return nil, fmt.Errorf("这个令牌不是签给 %s 的", want.Audience)
	}
	return claims, nil
}
