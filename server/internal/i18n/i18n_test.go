package i18n

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"zh":       Zh,
		"zh-CN":    Zh,
		"zh_CN":    Zh,
		"zh-Hans":  Zh,
		"ZH-cn":    Zh,
		"en":       En,
		"en-US":    En,
		"es-ES":    Es,
		"pt-BR":    Pt,
		"hi-IN":    Hi,
		"  en-GB ": En,
		"":         "",
		"fr":       "",
		"fr-FR":    "",
		"zhxx":     "",
		"english":  "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q，应为 %q", in, got, want)
		}
	}
	if got := Languages(); !reflect.DeepEqual(got, []string{Zh, En, Es, Pt, Hi}) {
		t.Errorf("规范语言码顺序应为 zh/en/es/pt/hi，得到 %v", got)
	}
}

// 语言选择优先级：用户偏好 → 组织默认 → zh。非法值当作没设置，继续往下找。
func TestResolveLanguagePriority(t *testing.T) {
	cases := []struct{ user, org, want string }{
		{En, "", En},      // 用户有偏好，用它
		{"en-US", Zh, En}, // 用户偏好是浏览器写法，归一后仍优先
		{"", Es, Es},      // 用户没填，跟组织默认
		{"", "", Zh},      // 都没有，回 zh
		{"", "fr", Zh},    // 组织默认非法，兜底 zh
		{"fr", Pt, Pt},    // 用户偏好非法，跟组织默认
		{"fr", "fr", Zh},  // 两层都非法，兜底 zh
		{"zh-CN", En, Zh}, // zh-CN 归一成 zh，不被组织默认覆盖
		{"hi", "en", Hi},  // 印地语用户看到印地语
	}
	for _, c := range cases {
		if got := Resolve(c.user, c.org); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q，应为 %q", c.user, c.org, got, c.want)
		}
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		tpl    string
		params map[string]string
		want   string
	}{
		{"{{a}} 与 {{b}}", map[string]string{"a": "A", "b": "B"}, "A 与 B"},
		{"{{ a }}", map[string]string{"a": "A"}, "A"},
		{"重复 {{a}} / {{a}}", map[string]string{"a": "x"}, "重复 x / x"},
		{"没有参数", nil, "没有参数"},
		{"缺少 {{missing}}", map[string]string{"a": "A"}, "缺少 {{missing}}"},
		{"{{}}", map[string]string{"": "空"}, "{{}}"},
		{"{{未闭合", map[string]string{"未闭合": "x"}, "{{未闭合"},
	}
	for _, c := range cases {
		if got := Render(c.tpl, c.params); got != c.want {
			t.Errorf("Render(%q) = %q，应为 %q", c.tpl, got, c.want)
		}
	}
	got := Placeholders("{{title}} · {{count}} 次 · {{title}}")
	if !reflect.DeepEqual(got, []string{"title", "count"}) {
		t.Errorf("Placeholders = %v，应为 [title count]", got)
	}
}
