// ee-jwt-adapter 是企业系统那一侧的适配层样例：验证 dsh-ee 签的 ee_jwt，知道是哪位员工在调用。
//
// 真实系统里，把 verify 放进自己的鉴权中间件，验过之后换成系统自己的会话或用户对象即可。
// 只用标准库，可以整段抄走。用法：
//
//	ee-jwt-adapter -issuer https://ee.acme.com -audience oa -addr :8090
//
// 协议见 docs/ee-jwt.md。
package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type jwk struct{ Kty, Kid, N, E string }

// keys 缓存 dsh-ee 的验签公钥。遇到没见过的 kid（dsh-ee 轮换了密钥）就重新取一次，最多每分钟一次。
type keys struct {
	url string
	mu  sync.Mutex
	set map[string]*rsa.PublicKey
	at  time.Time
}

func (k *keys) get(kid string) (*rsa.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if pub := k.set[kid]; pub != nil {
		return pub, nil
	}
	if time.Since(k.at) < time.Minute {
		return nil, errors.New("不认识这把签名钥匙")
	}
	k.at = time.Now()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(k.url)
	if err != nil {
		return nil, fmt.Errorf("取 JWKS：%w", err)
	}
	defer resp.Body.Close()
	var doc struct{ Keys []jwk }
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("JWKS 格式不对：%w", err)
	}
	k.set = map[string]*rsa.PublicKey{}
	for _, j := range doc.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(j.N)
		e, err2 := base64.RawURLEncoding.DecodeString(j.E)
		if j.Kty == "RSA" && err1 == nil && err2 == nil {
			k.set[j.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		}
	}
	if pub := k.set[kid]; pub != nil {
		return pub, nil
	}
	return nil, errors.New("不认识这把签名钥匙")
}

// verify 校验令牌，返回里面的声明。四件事缺一不可：只认 RS256、验签、没过期、iss 和 aud 对得上。
func verify(token string, ks *keys, issuer, audience string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("令牌格式不对")
	}
	var head struct{ Alg, Kid string }
	if raw, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil || json.Unmarshal(raw, &head) != nil || head.Alg != "RS256" {
		return nil, errors.New("只接受 RS256 签名的令牌")
	}
	pub, err := ks.get(head.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("签名解析失败")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		return nil, errors.New("签名不对")
	}
	claims := map[string]any{}
	if raw, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil || json.Unmarshal(raw, &claims) != nil {
		return nil, errors.New("令牌内容解析失败")
	}
	if exp, ok := claims["exp"].(float64); !ok || time.Now().After(time.Unix(int64(exp), 0)) {
		return nil, errors.New("令牌已过期")
	}
	if claims["iss"] != issuer {
		return nil, errors.New("不是这台 dsh-ee 签发的")
	}
	if claims["aud"] != audience {
		return nil, errors.New("这个令牌不是签给本系统的")
	}
	return claims, nil
}

func main() {
	issuer := flag.String("issuer", "", "dsh-ee 的访问地址，如 https://ee.acme.com")
	audience := flag.String("audience", "", "本系统的标识：连接里填的 aud，没填就是连接标识")
	header := flag.String("header", "", "令牌放在哪个请求头；空则 Authorization: Bearer")
	addr := flag.String("addr", "127.0.0.1:8090", "监听地址")
	flag.Parse()
	if *issuer == "" || *audience == "" {
		log.Fatal("要指定 -issuer 和 -audience")
	}
	ks := &keys{url: strings.TrimRight(*issuer, "/") + "/.well-known/jwks.json"}

	// 示例接口：返回「我」是谁。真实系统在这里按 preferred_username / email 找到自己的用户，再走原有的权限判断。
	http.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if *header != "" {
			token = r.Header.Get(*header)
		}
		claims, err := verify(token, ks, strings.TrimRight(*issuer, "/"), *audience)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"username": claims["preferred_username"], "name": claims["name"], "email": claims["email"],
			"dept": claims["dept"], "roles": claims["roles"],
		})
	})
	// 这个样例自己的接口文档，接入 dsh-ee 时直接贴进「OpenAPI 连接」
	http.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"openapi":"3.0.0","info":{"title":"ee_jwt 适配层样例","version":"1"},"servers":[{"url":"http://%s"}],"paths":{"/me":{"get":{"operationId":"whoami","summary":"返回当前调用的员工是谁"}}}}`, r.Host)
	})
	log.Printf("ee-jwt-adapter 监听 %s，认 %s 签给 %s 的令牌", *addr, *issuer, *audience)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
