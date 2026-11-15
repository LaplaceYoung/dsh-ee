package api

import (
	"strings"
	"testing"
)

// nextCase 是 next 过滤的表驱动用例。合法时原样返回；不合法时 safeNext 回退到
// 默认目标 /go，webNext 回退到空串。
type nextCase struct {
	name  string
	raw   string
	valid bool
}

// nextCases 覆盖审计确认的绕过：浏览器解析 URL 时会吞掉 TAB/换行，
// 「/\t/evil.example」会变成协议相对的「//evil.example」，跳到任意外部站点。
func nextCases() []nextCase {
	return []nextCase{
		{name: "空串", raw: "", valid: false},
		{name: "普通站内路径", raw: "/ok", valid: true},
		{name: "带查询与片段", raw: "/ok?x=1#h", valid: true},
		{name: "默认目标本身", raw: "/go", valid: true},
		{name: "协议相对", raw: "//evil.example", valid: false},
		{name: "反斜杠开头", raw: "/\\evil.example", valid: false},
		{name: "制表符", raw: "/\t/evil.example", valid: false},
		{name: "换行", raw: "/\n/evil.example", valid: false},
		{name: "回车", raw: "/\r/evil.example", valid: false},
		{name: "NUL", raw: "/\x00/evil.example", valid: false},
		{name: "DEL", raw: "/\x7f/evil.example", valid: false},
		{name: "路径中间的反斜杠", raw: "/a\\b", valid: false},
		{name: "绝对地址", raw: "http://evil.example", valid: false},
		{name: "没有斜杠", raw: "evil", valid: false},
		{name: "API 路径", raw: "/api/admin/users", valid: false},
		{name: "超长路径", raw: "/" + strings.Repeat("a", 3000), valid: false},
	}
}

// assertNextIsLocal 断言过滤结果本身不会成为开放重定向：非空时必须以单个 / 开头，
// 不能以 // 或 /\ 开头，也不能含 ASCII 控制字符（浏览器会吞掉 TAB/换行）。
func assertNextIsLocal(t *testing.T, fn, got string) {
	t.Helper()
	if got == "" {
		return
	}
	if !strings.HasPrefix(got, "/") || strings.HasPrefix(got, "//") || strings.HasPrefix(got, "/\\") {
		t.Fatalf("%s 返回了可跳站的地址：%q", fn, got)
	}
	if strings.ContainsFunc(got, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		t.Fatalf("%s 返回了控制字符：%q", fn, got)
	}
}

// TestSafeNext：OIDC 登录入口（/api/auth/oidc/start?next=…）的 next 过滤。
// 非法值必须回退到默认目标 /go，而不是原样写进 Location。
func TestSafeNext(t *testing.T) {
	for _, tc := range nextCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := "/go"
			if tc.valid {
				want = tc.raw
			}
			got := safeNext(tc.raw)
			if got != want {
				t.Fatalf("safeNext(%q) = %q，期望 %q", tc.raw, got, want)
			}
			assertNextIsLocal(t, "safeNext", got)
		})
	}
}

// TestWebNext：与 safeNext 用同一套规则，非法值返回空串（调用方自行决定回退目标）。
// 两个入口的行为必须一致，避免只修好其中一个。
func TestWebNext(t *testing.T) {
	for _, tc := range nextCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := ""
			if tc.valid {
				want = tc.raw
			}
			got := webNext(tc.raw)
			if got != want {
				t.Fatalf("webNext(%q) = %q，期望 %q", tc.raw, got, want)
			}
			assertNextIsLocal(t, "webNext", got)
			// 一致性：safeNext 只在 webNext 拒绝时回退到 /go，绝不比 webNext 更宽松。
			if got != "" {
				if safeNext(tc.raw) != got {
					t.Fatalf("safeNext(%q) = %q，与 webNext 的 %q 不一致", tc.raw, safeNext(tc.raw), got)
				}
			} else if safeNext(tc.raw) != "/go" {
				t.Fatalf("webNext 拒绝 %q 时 safeNext 应回退到 /go，得到 %q", tc.raw, safeNext(tc.raw))
			}
		})
	}
}
