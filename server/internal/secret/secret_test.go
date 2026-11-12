package secret

import (
	"testing"
	"unicode/utf8"
)

func TestHintKeepsUTF8(t *testing.T) {
	for _, s := range []string{"密码", "abc密码", "密码abc", "a🔑b🔑c🔑"} {
		hint := Hint(s)
		if !utf8.ValidString(hint) {
			t.Fatalf("Hint(%q) 不是合法 UTF-8：%q", s, hint)
		}
	}
	if got := Hint("abcdef"); got != "…cdef" {
		t.Fatalf("Hint(abcdef) = %q", got)
	}
	if got := Hint("密码"); got != "****" {
		t.Fatalf("短密钥应完全隐藏，得到 %q", got)
	}
}
