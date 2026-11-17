package api

import "testing"

// TestSameOIDCIssuer 覆盖「换没换 IdP」的判定：只有写法差异算同一个 IdP，
// origin / path / query 任何一处不同都算换了（旧 client secret 会被发到新地址）。
func TestSameOIDCIssuer(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"完全相同", "https://sso.acme.com/realms/acme", "https://sso.acme.com/realms/acme", true},
		{"尾部斜杠归一", "https://sso.acme.com/realms/acme/", "https://sso.acme.com/realms/acme", true},
		{"多个尾部斜杠归一", "https://sso.acme.com/realms/acme///", "https://sso.acme.com/realms/acme", true},
		{"首尾空白归一", " https://sso.acme.com/realms/acme ", "https://sso.acme.com/realms/acme", true},
		{"host 大小写归一", "https://SSO.Acme.com/realms/acme", "https://sso.acme.com/realms/acme", true},
		{"默认端口等价", "https://sso.acme.com:443/realms/acme", "https://sso.acme.com/realms/acme", true},
		{"http 默认端口等价", "http://sso.acme.com:80/x", "http://sso.acme.com/x", true},
		{"换端口", "https://sso.acme.com:8443/realms/acme", "https://sso.acme.com/realms/acme", false},
		{"换 scheme", "http://sso.acme.com/realms/acme", "https://sso.acme.com/realms/acme", false},
		{"换主机", "https://evil.example/realms/acme", "https://sso.acme.com/realms/acme", false},
		{"同主机换 path", "https://sso.acme.com/realms/other", "https://sso.acme.com/realms/acme", false},
		{"同主机加 path 前缀", "https://sso.acme.com/proxy/realms/acme", "https://sso.acme.com/realms/acme", false},
		{"path 大小写不同", "https://sso.acme.com/Realms/Acme", "https://sso.acme.com/realms/acme", false},
		{"query 不同", "https://sso.acme.com/realms/acme?x=1", "https://sso.acme.com/realms/acme", false},
		{"空 issuer 与空 issuer", "", "", true},
		{"空 issuer 与有值", "", "https://sso.acme.com", false},
		{"解析不出的写法与正常地址", "not a url", "https://sso.acme.com", false},
		{"解析不出的同一写法", "not a url", "not a url", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameOIDCIssuer(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameOIDCIssuer(%q, %q) = %v，期望 %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestFeishuAPIBaseOriginRule 记录飞书 API 根地址的比较规则：只比 origin
// （scheme + host + 端口），path 不同不算换接收方（base 是请求前缀）。
func TestFeishuAPIBaseOriginRule(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool // true 表示同 origin，允许沿用已存 app secret
	}{
		{"完全相同", "https://open.feishu.cn", "https://open.feishu.cn", true},
		{"尾部斜杠归一", "https://open.feishu.cn/", "https://open.feishu.cn", true},
		{"host 大小写归一", "https://OPEN.FEISHU.CN", "https://open.feishu.cn", true},
		{"默认端口等价", "https://open.feishu.cn:443", "https://open.feishu.cn", true},
		{"同 origin 的 path 变化", "https://open.feishu.cn/feishu-proxy", "https://open.feishu.cn", true},
		{"换主机", "https://open.feishu.cn.evil.example", "https://open.feishu.cn", false},
		{"换端口", "https://open.feishu.cn:8443", "https://open.feishu.cn", false},
		{"换 scheme", "http://open.feishu.cn", "https://open.feishu.cn", false},
		{"空地址与有值", "", "https://open.feishu.cn", false},
		{"空地址与空地址", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameOrigin(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameOrigin(%q, %q) = %v，期望 %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
