package eejwt

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	k, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := Generate()
	now := time.Now()
	claims := func(patch map[string]any) map[string]any {
		c := map[string]any{"iss": "https://ee.example", "aud": "oa", "sub": "42", "preferred_username": "zhou.hang",
			"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(Lifetime).Unix(), "jti": "x"}
		for key, v := range patch {
			c[key] = v
		}
		return c
	}
	set := JWKS{Keys: []JWK{other.Public(), k.Public()}}
	want := Expect{Issuer: "https://ee.example", Audience: "oa"}

	tok, err := Sign(k, claims(nil))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Verify(tok, set, want)
	if err != nil || got["sub"] != "42" || got["preferred_username"] != "zhou.hang" {
		t.Fatalf("正常的令牌应通过：%v %v", got, err)
	}

	// 私钥存取一遍，kid 不变，签出来的照样能验
	der, _ := k.Marshal()
	back, err := Parse(der)
	if err != nil || back.ID != k.ID {
		t.Fatalf("私钥读回来 kid 应不变：%v", err)
	}
	if tok2, _ := Sign(back, claims(nil)); tok2 == "" {
		t.Fatal("读回来的私钥应能签名")
	} else if _, err := Verify(tok2, set, want); err != nil {
		t.Fatalf("读回来的私钥签的令牌应通过：%v", err)
	}

	reject := func(what, token string, s JWKS, w Expect, msg string) {
		t.Helper()
		if _, err := Verify(token, s, w); err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("%s：应被拒绝且原因包含「%s」，实际 %v", what, msg, err)
		}
	}
	sign := func(patch map[string]any) string { s, _ := Sign(k, claims(patch)); return s }
	reject("给别的系统签的", tok, set, Expect{Issuer: want.Issuer, Audience: "crm"}, "不是签给")
	reject("没指定 audience", tok, set, Expect{Issuer: want.Issuer}, "必须指定 audience")
	reject("签发方不对", tok, set, Expect{Issuer: "https://evil.example", Audience: "oa"}, "签发方")
	reject("过期", sign(map[string]any{"exp": now.Add(-time.Second).Unix()}), set, want, "过期")
	reject("没有 exp", sign(map[string]any{"exp": nil}), set, want, "过期")
	reject("还没生效", sign(map[string]any{"nbf": now.Add(10 * time.Minute).Unix()}), set, want, "还没生效")
	reject("公钥不在 JWKS 里", tok, JWKS{Keys: []JWK{other.Public()}}, want, "没有 kid")
	reject("格式不对", "a.b", set, want, "格式")

	// 用另一把私钥签、却写着这把钥匙的 kid
	forged, _ := Sign(&Key{ID: k.ID, Private: other.Private}, claims(nil))
	reject("签名不对", forged, set, want, "签名不对")

	// 改了内容：把 sub 换掉，签名对不上
	parts := strings.Split(tok, ".")
	body, _ := json.Marshal(claims(map[string]any{"sub": "1"}))
	reject("内容被改", parts[0]+"."+base64.RawURLEncoding.EncodeToString(body)+"."+parts[2], set, want, "签名不对")

	// alg 换成 none 或 HS256 一律不接受
	for _, alg := range []string{"none", "HS256"} {
		head, _ := json.Marshal(map[string]string{"alg": alg, "kid": k.ID})
		reject("算法 "+alg, base64.RawURLEncoding.EncodeToString(head)+"."+parts[1]+"."+parts[2], set, want, "不接受签名算法")
	}
}
