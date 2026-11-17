package api

import (
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// origin 判定是「换地址是否需要重填 key」的唯一依据：scheme/host/port 变化才算换址，
// path/query/尾部斜杠不算；host 按 DNS 规则忽略大小写。
func TestProviderBaseURLOrigin(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com":         "https://api.deepseek.com",
		"https://API.DeepSeek.com:443/v1/": "https://api.deepseek.com:443",
		"http://127.0.0.1:4000/v1":         "http://127.0.0.1:4000",
		"https://example.com/v1?x=1":       "https://example.com",
		"http://user:pass@Example.COM/p":   "http://example.com",
	}
	for raw, want := range cases {
		got, ok := baseURLOrigin(raw)
		if !ok || got != want {
			t.Fatalf("baseURLOrigin(%q) = %q, %v；期望 %q", raw, got, ok, want)
		}
	}
	for _, bad := range []string{"", "://bad", "ftp://example.com", "https://", "example.com"} {
		if origin, ok := baseURLOrigin(bad); ok {
			t.Fatalf("baseURLOrigin(%q) 应判为无法判定，实际 %q", bad, origin)
		}
	}
}

// 只有记录里明确写了 api_key_origin 且与当前 base_url 的 origin 不一致时才阻断；
// 空串是加列前的旧记录，按原逻辑放行（管理员重填 key 时补写）。
func TestProviderKeyOriginMoved(t *testing.T) {
	cases := []struct {
		name  string
		p     *store.Provider
		moved bool
	}{
		{"nil", nil, false},
		{"旧记录为空", &store.Provider{BaseURL: "https://new.example", APIKeyOrigin: ""}, false},
		{"同 origin", &store.Provider{BaseURL: "https://api.example/v1", APIKeyOrigin: "https://api.example"}, false},
		{"同 origin 不同大小写", &store.Provider{BaseURL: "https://API.Example/v1", APIKeyOrigin: "https://api.example"}, false},
		{"换 host", &store.Provider{BaseURL: "https://new.example", APIKeyOrigin: "https://old.example"}, true},
		{"换 scheme", &store.Provider{BaseURL: "https://api.example", APIKeyOrigin: "http://api.example"}, true},
		{"换端口", &store.Provider{BaseURL: "https://api.example:8443", APIKeyOrigin: "https://api.example:443"}, true},
		{"base_url 无法解析", &store.Provider{BaseURL: "://broken", APIKeyOrigin: "https://old.example"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerKeyOriginMoved(tc.p); got != tc.moved {
				t.Fatalf("providerKeyOriginMoved = %v，期望 %v", got, tc.moved)
			}
		})
	}
}
