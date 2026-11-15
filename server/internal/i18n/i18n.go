// Package i18n 是服务端最小的多语言设施：语言码归一、接收者语言优先级、{{参数}} 模板渲染。
//
// 规范语言码是 zh / en / es / pt / hi 五个短码：数据库（users.language、settings 的
// defaultLanguage）和接口统一存短码；zh-CN、zh_CN、en-US 这类写法只在入口处出现，一律
// 归一成短码后再保存。
//
// 文案表不在这里：飞书面向收件人的消息在 internal/feishu/messages.go。
package i18n

import (
	"slices"
	"strings"
)

const (
	Zh = "zh"
	En = "en"
	Es = "es"
	Pt = "pt"
	Hi = "hi"
)

// supported 是规范语言码，顺序固定（中文优先），测试按它核对五语文案。
var supported = []string{Zh, En, Es, Pt, Hi}

// Languages 返回全部规范语言码（副本，调用方可以随便改）。
func Languages() []string { return slices.Clone(supported) }

// Default 是最终兜底语言：文案的源语言是中文，任何一层语言都缺失时用 zh。
const Default = Zh

// Normalize 把各种写法归一成规范短码；不认识的语言返回空串。
// 接受「语言」或「语言-地区/文字」两种写法：zh-CN、zh-Hans、zh_TW 都归一成 zh，
// en-US / en-GB 都是 en，es-419 是 es。带多余子标签的脏值（如 zh-CN-extra-bad）拒绝，
// 避免把非法值当合法偏好存进库。
func Normalize(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	c = strings.ReplaceAll(c, "_", "-")
	if c == "" {
		return ""
	}
	parts := strings.Split(c, "-")
	if len(parts) > 2 || !slices.Contains(supported, parts[0]) {
		return ""
	}
	if len(parts) == 2 {
		sub := parts[1]
		if len(sub) < 2 || len(sub) > 4 {
			return ""
		}
		for _, r := range sub {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				return ""
			}
		}
	}
	return parts[0]
}

// Resolve 按「用户语言偏好 → 组织默认语言 → zh」选接收者渲染语言。
// 非法或为空的值当作没设置，继续往下一层找，永远返回一个规范短码。
func Resolve(userLang, orgLang string) string {
	if v := Normalize(userLang); v != "" {
		return v
	}
	if v := Normalize(orgLang); v != "" {
		return v
	}
	return Default
}

// Render 把模板里的 {{name}} 换成 params[name]。没有对应参数时保留占位符原样，
// 便于测试和排查漏传参数；params 为 nil 时原样返回。
func Render(template string, params map[string]string) string {
	if len(params) == 0 || !strings.Contains(template, "{{") {
		return template
	}
	var b strings.Builder
	rest := template
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			b.WriteString(rest)
			break
		}
		end := strings.Index(rest[start+2:], "}}")
		if end < 0 {
			b.WriteString(rest)
			break
		}
		end += start + 2
		b.WriteString(rest[:start])
		name := strings.TrimSpace(rest[start+2 : end])
		if v, ok := params[name]; ok && name != "" {
			b.WriteString(v)
		} else {
			b.WriteString(rest[start : end+2])
		}
		rest = rest[end+2:]
	}
	return b.String()
}

// Placeholders 返回模板里用到的参数名（去重并按出现顺序排序），
// 用于核对同一消息键的五种语言用的是同一组参数。
func Placeholders(template string) []string {
	var out []string
	seen := map[string]bool{}
	rest := template
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start+2:], "}}")
		if end < 0 {
			break
		}
		end += start + 2
		name := strings.TrimSpace(rest[start+2 : end])
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		rest = rest[end+2:]
	}
	return out
}
