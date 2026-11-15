package kb

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const policy = `# 差旅管理办法

## 第一章 总则

本办法适用于全体员工的国内出差。

## 第二章 住宿

一线城市（北京、上海、广州、深圳）住宿标准每晚不超过 600 元，
其他城市每晚不超过 400 元。

超出标准的部分由员工自理。

## 第三章 交通

乘坐高铁二等座，单程超过 6 小时可乘坐飞机经济舱。
`

func TestSplitAndOutline(t *testing.T) {
	chunks := Split(policy)
	if len(chunks) != 4 {
		t.Fatalf("段数 = %d, %+v", len(chunks), chunks)
	}
	c := chunks[2]
	if c.Heading != "差旅管理办法 > 第二章 住宿" || c.From != 7 || c.To != 12 {
		t.Fatalf("第三段 = %+v", c)
	}
	if !strings.Contains(c.Text, "600 元") {
		t.Fatalf("第三段内容 = %q", c.Text)
	}
	out := Outline(policy)
	if len(out) != 4 || out[1].Title != "第一章 总则" || out[1].Line != 3 {
		t.Fatalf("大纲 = %+v", out)
	}
}

func TestSplitLongParagraph(t *testing.T) {
	long := strings.Repeat("很长的一段文字没有空行。\n", 400)
	chunks := Split("# 标题\n" + long)
	if len(chunks) < 2 {
		t.Fatalf("超长段落没有断开：%d 段", len(chunks))
	}
}

func TestTokens(t *testing.T) {
	got := Tokens("住宿标准 A4纸 ＡＢＣ")
	want := []string{"住宿", "宿标", "标准", "a4", "纸", "abc"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Tokens = %v, want %v", got, want)
	}
}

func loadPolicy(ix *Index) {
	ix.Load(1, "r1", []*Doc{
		{ID: 10, Base: 1, Title: "差旅管理办法", Folder: "行政/差旅", Version: 2, Text: policy},
		{ID: 11, Base: 1, Title: "报销须知", Folder: "财务", Version: 1, Text: "# 报销须知\n\n住宿发票要写明入住日期。\n酒店发票抬头写公司全称。\n"},
	})
	ix.Load(2, "r1", []*Doc{
		{ID: 20, Base: 2, Title: "薪酬制度", Version: 1, Text: "# 薪酬制度\n\n住宿补贴每月 1000 元。\n"},
	})
}

func TestSearch(t *testing.T) {
	ix := NewIndex()
	loadPolicy(ix)
	ctx := context.Background()

	res, err := ix.Search(ctx, Query{Text: "住宿 标准", Bases: []int64{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 || res.Hits[0].Doc != 10 || res.Hits[0].From != 7 {
		t.Fatalf("第一条应是差旅办法的住宿章：%+v", res.Hits)
	}
	if res.Hits[0].Line != 7 || !strings.Contains(res.Hits[0].Snippet, "【住宿】") {
		t.Fatalf("命中行与片段 = %d %q", res.Hits[0].Line, res.Hits[0].Snippet)
	}
	if res.TermHits["住宿"] != 2 || res.TermHits["标准"] != 1 {
		t.Fatalf("各词命中数 = %v", res.TermHits)
	}
	// 只搜库 1：库 2 的「住宿补贴」不能出现
	for _, h := range res.Hits {
		if h.Base != 1 {
			t.Fatalf("搜到了范围外的库：%+v", h)
		}
	}

	// all：两个词都要有
	res, _ = ix.Search(ctx, Query{Text: "住宿 发票", Mode: "all", Bases: []int64{1, 2}})
	if len(res.Hits) != 1 || res.Hits[0].Doc != 11 {
		t.Fatalf("all 模式 = %+v", res.Hits)
	}

	// 跨行的词（PDF 折行）也能搜到；全角、大小写统一
	ix.Load(3, "r1", []*Doc{{ID: 30, Base: 3, Title: "X", Version: 1, Text: "员工差\n旅标准见附件 ＰＤＦ。\n"}})
	res, _ = ix.Search(ctx, Query{Text: "差旅标准 pdf", Mode: "all", Bases: []int64{3}})
	if len(res.Hits) != 1 {
		t.Fatalf("跨行匹配失败：%+v", res)
	}

	// 单个汉字退回逐段核对
	res, _ = ix.Search(ctx, Query{Text: "飞", Bases: []int64{1}})
	if len(res.Hits) != 1 || res.Hits[0].From != 14 {
		t.Fatalf("单字检索 = %+v", res.Hits)
	}

	// 目录过滤
	res, _ = ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1}, Folder: "财务"})
	if len(res.Hits) != 1 || res.Hits[0].Doc != 11 {
		t.Fatalf("目录过滤 = %+v", res.Hits)
	}
	res, _ = ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1}, Folder: "行政"})
	if len(res.Hits) != 1 || res.Hits[0].Doc != 10 {
		t.Fatalf("上级目录过滤 = %+v", res.Hits)
	}

	// 英文整词：art 不匹配 start
	ix.Load(4, "r1", []*Doc{{ID: 40, Base: 4, Title: "E", Version: 1, Text: "Start the engine.\n"}})
	res, _ = ix.Search(ctx, Query{Text: "art", Bases: []int64{4}})
	if len(res.Hits) != 0 {
		t.Fatalf("英文应按整词：%+v", res.Hits)
	}
}

func TestSearchRegexAndLimit(t *testing.T) {
	ix := NewIndex()
	loadPolicy(ix)
	ctx := context.Background()
	res, err := ix.Search(ctx, Query{Text: `\d+ 元`, Mode: "regex", Bases: []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || res.Hits[0].Line != 9 {
		t.Fatalf("正则 = %+v", res)
	}
	// 写错的正则按普通文字匹配，并说明一句
	ix.Load(5, "r1", []*Doc{{ID: 50, Base: 5, Title: "F", Version: 1, Text: "见附件(一)。\n"}})
	res, err = ix.Search(ctx, Query{Text: "(一", Mode: "regex", Bases: []int64{5}})
	if err != nil || res.Total != 1 || res.Note == "" {
		t.Fatalf("错误的正则应按文字匹配：%+v %v", res, err)
	}
	res, _ = ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1, 2}, Limit: 1})
	if len(res.Hits) != 1 || !res.Truncated || res.Total != 3 {
		t.Fatalf("limit = %+v", res)
	}
}

func TestVerifyQuote(t *testing.T) {
	cases := []struct {
		from, to int
		quote    string
		ok       bool
	}{
		{9, 9, "一线城市（北京、上海、广州、深圳）住宿标准每晚不超过 600 元", true},
		{9, 10, "住宿标准每晚不超过600元，其他城市每晚不超过400元", true}, // 跨行、空格与标点不同
		{9, 9, "一线城市…每晚不超过 600 元", true},
		{9, 9, "一线城市住宿标准每晚不超过 800 元", false},
		{16, 16, "住宿标准每晚不超过 600 元", false}, // 行号差太远
		{9, 9, "。", false},
	}
	for _, c := range cases {
		if got := VerifyQuote(policy, c.from, c.to, c.quote); got != c.ok {
			t.Errorf("VerifyQuote(%d-%d, %q) = %v", c.from, c.to, c.quote, got)
		}
	}
}

func makeDocx(t *testing.T, body string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>%s</w:body></w:document>`, body)
	s, _ := zw.Create("word/styles.xml")
	fmt.Fprint(s, `<?xml version="1.0"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:styleId="1"><w:name w:val="heading 1"/></w:style><w:style w:styleId="2"><w:name w:val="标题 2"/></w:style></w:styles>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestConvertDocx(t *testing.T) {
	body := `<w:p><w:pPr><w:pStyle w:val="1"/></w:pPr><w:r><w:t>报销制度</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="2"/></w:pPr><w:r><w:t>一、适用范围</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t xml:space="preserve">适用于全体 </w:t></w:r><w:r><w:t>员工。</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/></w:numPr></w:pPr><w:r><w:t>发票要真实</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>城市</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>标准</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>北京</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>600|元</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	out, err := Convert(context.Background(), "报销.docx", makeDocx(t, body), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "# 报销制度\n\n## 一、适用范围\n\n适用于全体 员工。\n\n- 发票要真实\n| 城市 | 标准 |\n| --- | --- |\n| 北京 | 600\\|元 |\n"
	if out.Markdown != want {
		t.Fatalf("docx 转换 =\n%s\nwant\n%s", out.Markdown, want)
	}
	if out.Title != "报销制度" {
		t.Fatalf("标题 = %q", out.Title)
	}
	if _, err := Convert(context.Background(), "x.docx", []byte("not zip"), Options{}); err == nil {
		t.Fatal("坏文件应报错")
	}
}

// makePDF 手写一个最小的 PDF（标准字体 Helvetica），两行文字间隔较大以测试断段。
func makePDF(lines ...string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 12 Tf\n")
	y := 750
	for i, l := range lines {
		gap := 14
		if i == 2 {
			gap = 40
		}
		if i > 0 {
			y -= gap
		}
		fmt.Fprintf(&content, "1 0 0 1 72 %d Tm (%s) Tj\n", y, l)
	}
	content.WriteString("ET\n")
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func TestConvertPDF(t *testing.T) {
	out, err := Convert(context.Background(), "handbook.pdf", makePDF("Travel policy", "Hotel limit is 600 per night.", "Flights need approval."), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "## 第 1 页\n\nTravel policy\nHotel limit is 600 per night.\n\nFlights need approval.\n"
	if out.Markdown != want {
		t.Fatalf("pdf 转换 = %q", out.Markdown)
	}
	if out.Title != "handbook" {
		t.Fatalf("标题 = %q", out.Title)
	}
	if _, err := Convert(context.Background(), "scan.pdf", makePDF(), Options{}); err != ErrNoText {
		t.Fatalf("没有文字的 PDF 应提示扫描件：%v", err)
	}
	if _, err := Convert(context.Background(), "bad.pdf", []byte("%PDF-1.4 broken"), Options{}); err == nil {
		t.Fatal("坏 PDF 应报错")
	}
}

func TestConvertText(t *testing.T) {
	out, err := Convert(context.Background(), "说明.md", []byte("\xef\xbb\xbf# 说明\r\n\r\n\r\n\r\n正文  \r\n"), Options{})
	if err != nil || out.Markdown != "# 说明\n\n正文\n" || out.Title != "说明" {
		t.Fatalf("md = %+v %v", out, err)
	}
	if _, err := Convert(context.Background(), "a.exe", []byte("x"), Options{}); err != ErrUnsupported {
		t.Fatalf("不支持的格式：%v", err)
	}
	if _, err := Convert(context.Background(), "a.txt", []byte{0xff, 0xfe}, Options{}); err == nil {
		t.Fatal("非 UTF-8 应报错")
	}
}

// 走索引的正则检索，结果要和逐段扫完全一样。
func TestSearchRegexIndexed(t *testing.T) {
	ix := NewIndex()
	var docs []*Doc
	texts := []string{
		policy,
		"# 报销须知\n\n住宿发票要写明入住日期。\n错误码 ERR404 表示找不到单据。\n",
		"# 年假\n\n第十二条 年假可以跨年使用，最多顺延 3 个月。\n第十三条 病假需要医院证明。\n",
		"# IT\n\nVPN 连不上时先重启客户端；Wi-Fi 密码见前台。\nThe VPN client v2.3 is required.\n",
	}
	for i, tx := range texts {
		docs = append(docs, &Doc{ID: int64(100 + i), Base: 9, Title: fmt.Sprintf("D%d", i), Version: 1, Text: tx})
	}
	ix.Load(9, "r1", docs)
	ctx := context.Background()
	patterns := []string{
		`住宿`, `差旅|年假`, `第十.条`, `第\d+条`, `ERR\d+`, `err404`, `(年假)?跨年`, `VPN.*客户端`, `v\d\.\d`,
		`(住宿|交通).{0,10}标准`, `年假可以`, `病假需要医院`, `a?`, `^#`, `元$`, `[高飞]`, `Wi-Fi`, `client v2`, `顺延\s*3`, `不存在的词`,
	}
	for _, p := range patterns {
		got, err := ix.Search(ctx, Query{Text: p, Mode: "regex", Bases: []int64{9}, Limit: 100})
		if err != nil {
			t.Fatalf("%s：%v", p, err)
		}
		re := regexp.MustCompile("(?i)" + p)
		want := 0
		for _, d := range docs {
			for _, c := range Split(d.Text) {
				for _, l := range Lines(c.Text) {
					if re.MatchString(l) {
						want++
					}
				}
			}
		}
		if got.Total != want {
			t.Errorf("%s：走索引命中 %d 行，逐行扫 %d 行", p, got.Total, want)
		}
	}
}

func TestRegexNeed(t *testing.T) {
	ix := NewIndex()
	ix.Load(1, "r1", []*Doc{
		{ID: 1, Base: 1, Title: "A", Version: 1, Text: "差旅标准\n"},
		{ID: 2, Base: 1, Title: "B", Version: 1, Text: "年假规定\n"},
		{ID: 3, Base: 1, Title: "C", Version: 1, Text: "其他内容\n"},
	})
	bi := ix.bases[1]
	for _, c := range []struct {
		expr   string
		narrow bool
		n      int
	}{
		{"差旅", true, 1},
		{"差旅|年假", true, 2},
		{"差旅.*标准", true, 1},
		{"第\\d+条", false, 0}, // 只有单字，推不出词
		{"(差旅)?标准", true, 1}, // 「标准」一定要有
		{"差旅|.", false, 0},
	} {
		ids, ok := bi.match(regexNeed("(?i)" + c.expr))
		if ok != c.narrow || ok && len(ids) != c.n {
			t.Errorf("%s：narrow=%v ids=%v", c.expr, ok, ids)
		}
	}
}

func TestSearchTitleAndDocs(t *testing.T) {
	ix := NewIndex()
	loadPolicy(ix)
	ctx := context.Background()
	// 整个标题出现在检索文字里
	res, _ := ix.Search(ctx, Query{Text: "差旅管理办法 飞机", Bases: []int64{1, 2}})
	if len(res.Docs) != 1 || res.Docs[0].Doc != 10 || res.Docs[0].Lines != 16 {
		t.Fatalf("标题匹配 = %+v", res.Docs)
	}
	// 检索词都在标题里
	res, _ = ix.Search(ctx, Query{Text: "薪酬", Bases: []int64{1, 2}})
	if len(res.Docs) != 1 || res.Docs[0].Doc != 20 {
		t.Fatalf("标题匹配 = %+v", res.Docs)
	}
	res, _ = ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1, 2}})
	if len(res.Docs) != 0 {
		t.Fatalf("标题里没有「住宿」：%+v", res.Docs)
	}
	// 限定文档
	res, err := ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1, 2}, Docs: []int64{11, 20}})
	if err != nil || res.Total != 2 {
		t.Fatalf("限定文档 = %+v %v", res, err)
	}
	for _, h := range res.Hits {
		if h.Doc == 10 {
			t.Fatalf("搜到了范围外的文档：%+v", h)
		}
	}
	res, err = ix.Search(ctx, Query{Text: `\d+ 元`, Mode: "regex", Bases: []int64{1, 2}, Docs: []int64{20}})
	if err != nil || res.Total != 1 {
		t.Fatalf("正则限定文档 = %+v %v", res, err)
	}
	// 文档不在所选的库里
	if _, err := ix.Search(ctx, Query{Text: "住宿", Bases: []int64{1}, Docs: []int64{20}}); err == nil {
		t.Fatal("库外的文档应报错")
	}
}

// 约 10 万段的库上跑正则检索。
func BenchmarkSearchRegex(b *testing.B) {
	ix := NewIndex()
	words := []string{"差旅", "住宿", "报销", "年假", "病假", "合同", "客户", "发票", "审批", "预算", "采购", "培训", "考勤", "加班", "设备"}
	var docs []*Doc
	for i := 0; i < 20000; i++ {
		var tb strings.Builder
		for s := 0; s < 5; s++ {
			fmt.Fprintf(&tb, "## 第%d节\n\n", s+1)
			for k := 0; k < 4; k++ {
				fmt.Fprintf(&tb, "第%d条 %s和%s的规定，金额不超过 %d 元。\n", s*4+k+1, words[(i+s+k)%len(words)], words[(i*7+k)%len(words)], (i+k)%900+100)
			}
			tb.WriteString("\n")
		}
		docs = append(docs, &Doc{ID: int64(i + 1), Base: 1, Title: fmt.Sprintf("制度%d", i), Version: 1, Text: tb.String()})
	}
	docs[12345].Text += "\n错误码 ERR404 表示找不到单据。\n"
	ix.Load(1, "r1", docs)
	ctx := context.Background()
	for _, p := range []string{`ERR\d+.*单据`, `年假|病假`, `第\d+条`} {
		b.Run(p, func(b *testing.B) {
			for b.Loop() {
				if _, err := ix.Search(ctx, Query{Text: p, Mode: "regex", Bases: []int64{1}, Limit: 30}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
