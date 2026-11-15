package feishu

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/i18n"
)

// 这是本 PR 最重要的防漂移门：每个飞书消息键都必须在五种语言里都有非空译文，
// 并且五种语言用的是同一组 {{参数}}——新增消息或改参数时漏翻一条就红。
func TestFeishuMessageCatalogComplete(t *testing.T) {
	langs := i18n.Languages()
	if len(langs) != 5 {
		t.Fatalf("应有 zh/en/es/pt/hi 五种语言，实际 %v", langs)
	}
	if len(MessageKeys()) == 0 {
		t.Fatal("消息表是空的")
	}
	for _, lang := range langs {
		if strings.TrimSpace(lang) == "" {
			t.Fatalf("语言码不能为空：%v", langs)
		}
	}
	for _, key := range MessageKeys() {
		table := messages[key]
		if table == nil {
			t.Fatalf("消息键 %s 没有语言表", key)
		}
		want := i18n.Placeholders(table[i18n.Zh])
		for _, lang := range langs {
			tpl := table[lang]
			if strings.TrimSpace(tpl) == "" {
				t.Fatalf("消息键 %s 缺少 %s 译文", key, lang)
			}
			if got := i18n.Placeholders(tpl); !reflect.DeepEqual(got, want) {
				t.Fatalf("消息键 %s 的 %s 占位符是 %v，中文源是 %v", key, lang, got, want)
			}
		}
		// 每种语言都必须真的能渲染出来，不能有残留的 {{参数}}。
		for _, lang := range langs {
			params := map[string]string{}
			for _, name := range want {
				params[name] = "x"
			}
			if out := Text(lang, key, params); strings.Contains(out, "{{") {
				t.Fatalf("消息键 %s 的 %s 渲染后仍有占位符：%q", key, lang, out)
			}
			if strings.TrimSpace(Text(lang, key, params)) == "" {
				t.Fatalf("消息键 %s 的 %s 渲染结果是空串", key, lang)
			}
		}
	}
}

// 语言选择优先级：用户有 language 用它 / 空则组织默认 / 都没有 zh。
// 飞书发送前用的就是这个 Resolve，所以这里同时是消息语言选择的单测。
func TestFeishuRecipientLanguagePriority(t *testing.T) {
	cases := []struct{ user, org, want string }{
		{"en", "zh", "en"},
		{"", "es", "es"},
		{"", "", "zh"},
		{"pt-BR", "hi", "pt"},
		{"fr", "hi", "hi"},
	}
	for _, c := range cases {
		if got := i18n.Resolve(c.user, c.org); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q，应为 %q", c.user, c.org, got, c.want)
		}
	}
}

// Text 的参数替换与回退：不认识的语言回退英文，不认识的键返回键名（便于暴露漏登记）。
func TestFeishuText(t *testing.T) {
	title := "企业审批"
	if got := Text("en", MsgCardHubTitle, map[string]string{"title": title}); got != title+" · Pending review" {
		t.Errorf("英文标题渲染不对：%q", got)
	}
	if got := Text("fr", MsgCardButtonApprove, nil); got != "Approve" {
		t.Errorf("未知语言应回退英文：%q", got)
	}
	if got := Text("hi", MsgToastDecisionApproved, map[string]string{"title": title}); !strings.Contains(got, title) || !strings.Contains(got, "स्वीकृत") {
		t.Errorf("印地语 toast 渲染不对：%q", got)
	}
	if got := Text("en", "feishu.unknown.key", nil); got != "feishu.unknown.key" {
		t.Errorf("未知消息键应返回键名：%q", got)
	}
	// 组织默认语言非法时由 i18n.Resolve 兜底 zh，消息表本身永远能渲染中文。
	if got := Text(i18n.Resolve("", ""), MsgKBGap, map[string]string{"title": "库", "query": "词", "count": "2", "id": "7"}); !strings.Contains(got, "知识库") {
		t.Errorf("兜底中文渲染不对：%q", got)
	}
}
