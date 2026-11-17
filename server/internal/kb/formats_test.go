package kb

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const wns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func TestDocxNumbering(t *testing.T) {
	// 标题样式绑定「第一章」，正文段落用「第一条」，下一级「（一）」；另一个列表用 1. 2.，中间重新开始编号
	numbering := `<w:numbering ` + wns + `>
<w:abstractNum w:abstractNumId="0">
  <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="chineseCounting"/><w:lvlText w:val="第%1章"/><w:pStyle w:val="Heading2"/></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="1">
  <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="chineseCounting"/><w:lvlText w:val="第%1条"/></w:lvl>
  <w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="chineseCounting"/><w:lvlText w:val="（%2）"/><w:suff w:val="nothing"/></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="2">
  <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
  <w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="3">
  <w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/></w:lvl>
</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
<w:num w:numId="3"><w:abstractNumId w:val="2"/></w:num>
<w:num w:numId="4"><w:abstractNumId w:val="2"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride></w:num>
<w:num w:numId="5"><w:abstractNumId w:val="3"/></w:num>
</w:numbering>`
	styles := `<w:styles ` + wns + `>
<w:style w:styleId="Heading2"><w:name w:val="heading 2"/><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
<w:style w:styleId="Clause"><w:name w:val="条款"/><w:basedOn w:val="ClauseBase"/></w:style>
<w:style w:styleId="ClauseBase"><w:name w:val="条款基础"/><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr></w:style>
</w:styles>`
	p := func(style, num, text string) string {
		ppr := ""
		if style != "" {
			ppr += `<w:pStyle w:val="` + style + `"/>`
		}
		ppr += num
		return `<w:p><w:pPr>` + ppr + `</w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
	}
	np := func(id, lvl int) string {
		return fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, lvl, id)
	}
	body := p("Heading2", "", "总则") +
		p("Clause", "", "为规范差旅，制定本办法。") +
		p("Clause", "", "本办法适用于全体员工。") +
		p("", np(2, 1), "正式员工；") +
		p("", np(2, 1), "实习生。") +
		p("Heading2", "", "住宿") +
		p("Clause", "", "一线城市每晚不超过 600 元。") +
		p("Clause", np(0, 0), "（本段去掉了编号）") +
		p("", np(3, 0), "先申请") +
		p("", np(3, 1), "填写表单") +
		p("", np(3, 0), "再报销") +
		p("", np(4, 0), "重新开始") +
		p("", np(5, 0), "项目符号")
	for i := 0; i < 9; i++ {
		body += p("Clause", "", fmt.Sprintf("条款 %d", i+4))
	}
	doc := `<w:document ` + wns + `><w:body>` + body + `</w:body></w:document>`
	out, err := Convert(context.Background(), "差旅.docx", zipOf(t, map[string]string{"word/document.xml": doc, "word/styles.xml": styles, "word/numbering.xml": numbering}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## 第一章 总则\n", "第一条 为规范差旅", "第二条 本办法适用", "（一）正式员工；", "（二）实习生。",
		"## 第二章 住宿\n", "第三条 一线城市", "\n（本段去掉了编号）\n",
		"1. 先申请", "1.1 填写表单", "2. 再报销", "1. 重新开始", "- 项目符号", "第十二条 条款 12",
	} {
		if !strings.Contains(out.Markdown, want) {
			t.Fatalf("缺少 %q：\n%s", want, out.Markdown)
		}
	}
	if out.Info.Numbering != 20 || out.Info.Format != "docx" {
		t.Fatalf("体检 = %+v", out.Info)
	}
}

func TestDocxMergedCells(t *testing.T) {
	cell := func(pr, text string) string {
		return `<w:tc><w:tcPr>` + pr + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc>`
	}
	tbl := `<w:tbl>` +
		`<w:tr>` + cell("", "部门") + cell("", "城市") + cell("", "标准") + `</w:tr>` +
		`<w:tr>` + cell(`<w:vMerge w:val="restart"/>`, "销售部") + cell("", "北京") + cell("", "600") + `</w:tr>` +
		`<w:tr>` + cell(`<w:vMerge/>`, "") + cell("", "上海") + cell("", "550") + `</w:tr>` +
		`<w:tr>` + cell(`<w:gridSpan w:val="2"/>`, "其他城市") + cell("", "400") + `</w:tr>` +
		`</w:tbl>`
	doc := `<w:document ` + wns + `><w:body>` + tbl + `</w:body></w:document>`
	out, err := Convert(context.Background(), "标准.docx", zipOf(t, map[string]string{"word/document.xml": doc}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "| 部门 | 城市 | 标准 |\n| --- | --- | --- |\n| 销售部 | 北京 | 600 |\n| 销售部 | 上海 | 550 |\n| 其他城市 |  | 400 |\n"
	if out.Markdown != want || out.Info.Tables != 1 {
		t.Fatalf("合并单元格 =\n%s\nwant\n%s", out.Markdown, want)
	}
}

func TestChineseNum(t *testing.T) {
	for v, want := range map[int]string{1: "一", 10: "十", 11: "十一", 20: "二十", 21: "二十一", 100: "一百", 105: "一百零五", 110: "一百一十", 1001: "一千零一"} {
		if got := formatNum(v, "chineseCounting"); got != want {
			t.Errorf("%d → %s，应是 %s", v, got, want)
		}
	}
	for v, want := range map[int]string{1: "壹", 10: "壹拾", 12: "壹拾贰"} {
		if got := formatNum(v, "chineseLegalSimplified"); got != want {
			t.Errorf("大写 %d → %s，应是 %s", v, got, want)
		}
	}
	if formatNum(4, "upperRoman") != "IV" || formatNum(28, "lowerLetter") != "bb" || formatNum(3, "ideographTraditional") != "丙" || formatNum(2, "decimalEnclosedCircle") != "②" {
		t.Fatal("其他编号格式不对")
	}
}

func TestConvertXLSX(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "价目表")
	rows := [][]any{{"产品", "地区", "单价", "折扣"}}
	for i := 0; i < 250; i++ {
		rows = append(rows, []any{fmt.Sprintf("型号-%03d", i), "华东", 100 + i, 0.15})
	}
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetSheetRow("价目表", cell, &r)
	}
	pct, _ := f.NewStyle(&excelize.Style{NumFmt: 10}) // 0.00%
	_ = f.SetCellStyle("价目表", "D2", "D251", pct)
	_ = f.MergeCell("价目表", "B2", "B4") // 合并的地区：值填到每一格
	// 第二个表没有表头，第一行就是数据；中间有空行空列
	_, _ = f.NewSheet("通讯录")
	_ = f.SetCellValue("通讯录", "A1", "张三")
	_ = f.SetCellValue("通讯录", "C1", "13800000000")
	_ = f.SetCellValue("通讯录", "A3", "李四")
	_ = f.SetCellValue("通讯录", "C3", "13900000000")
	_, _ = f.NewSheet("草稿")
	_ = f.SetCellValue("草稿", "A1", "不该出现")
	_ = f.SetSheetVisible("草稿", false)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := Convert(context.Background(), "价目表.xlsx", buf.Bytes(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	md := out.Markdown
	for _, want := range []string{
		"# 价目表\n", "## 第 2～", "| 产品 | 地区 | 单价 | 折扣 |", "| 型号-000 | 华东 | 100 | 15.00% |", "| 型号-002 | 华东 | 102 | 15.00% |",
		"# 通讯录\n", "## 第 1～3 行", "| A | C |", "| 张三 | 13800000000 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("缺少 %q：\n%s", want, md[:min(len(md), 3000)])
		}
	}
	if strings.Contains(md, "不该出现") {
		t.Fatal("隐藏的工作表不该收")
	}
	// 每段都带表头：表头出现的次数 = 段数
	if heads, blocks := strings.Count(md, "| 产品 | 地区 | 单价 | 折扣 |"), strings.Count(md, "## 第 "); heads < 3 || heads != blocks-1 {
		t.Fatalf("每段都要带表头：表头 %d 次，段 %d 个", heads, blocks)
	}
	// 每一段都不会被切段切开（切段后每段开头都是标题）
	for _, c := range Split(md) {
		if strings.HasPrefix(c.Text, "|") {
			t.Fatalf("有一段没有标题，表格被切开了：%s", c.Text[:min(len(c.Text), 200)])
		}
	}
	if out.Info.Sheets != 2 || out.Info.HiddenSheets != 1 || out.Info.Rows != 252 || out.Info.Header != "mixed" {
		t.Fatalf("体检 = %+v", out.Info)
	}
	// 维护者指定「第一行不是表头」
	out, _ = Convert(context.Background(), "价目表.xlsx", buf.Bytes(), Options{Header: "none"})
	if !strings.Contains(out.Markdown, "| A | B | C | D |") || out.Info.Header != "letters" {
		t.Fatalf("指定没有表头：%+v", out.Info)
	}
}

func TestConvertCSV(t *testing.T) {
	src := "姓名;部门;分机\n王五;财务部;8001\n赵六;人事部;8002\n"
	gbk, _ := simplifiedchinese.GBK.NewEncoder().String(src)
	out, err := Convert(context.Background(), "通讯录.csv", []byte(gbk), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "# 第 2～3 行\n\n| 姓名 | 部门 | 分机 |\n| --- | --- | --- |\n| 王五 | 财务部 | 8001 |\n| 赵六 | 人事部 | 8002 |\n"
	if out.Markdown != want || out.Info.Encoding != "GBK" || out.Info.Header != "first" {
		t.Fatalf("csv =\n%s\n%+v", out.Markdown, out.Info)
	}
	// 引号里的逗号、换行
	out, _ = Convert(context.Background(), "a.csv", []byte("问题,答案\n\"年假, 能跨年吗\",\"可以\n最多 3 个月\"\n"), Options{})
	if !strings.Contains(out.Markdown, "| 年假, 能跨年吗 | 可以 最多 3 个月 |") {
		t.Fatalf("引号 = %s", out.Markdown)
	}
	big := strings.Repeat("a,b\n", MaxSheetRows+1)
	if _, err := Convert(context.Background(), "big.csv", []byte(big), Options{}); err == nil || !strings.Contains(err.Error(), "拆成") {
		t.Fatalf("超过上限应报错：%v", err)
	}
}

func TestConvertText_GBK(t *testing.T) {
	gbk, _ := simplifiedchinese.GBK.NewEncoder().String("差旅说明\n住宿 600 元\n")
	out, err := Convert(context.Background(), "说明.txt", []byte(gbk), Options{})
	if err != nil || !strings.Contains(out.Markdown, "住宿 600 元") || out.Info.Encoding != "GBK" {
		t.Fatalf("GBK 文本 = %+v %v", out, err)
	}
}

func TestConvertPPTX(t *testing.T) {
	const pns = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	sp := func(ph string, x, y int, paras ...string) string {
		nv := `<p:nvSpPr><p:cNvPr id="1" name="s"/><p:cNvSpPr/><p:nvPr>`
		if ph != "" {
			nv += `<p:ph type="` + ph + `"/>`
		}
		nv += `</p:nvPr></p:nvSpPr>`
		var body strings.Builder
		for _, t := range paras {
			body.WriteString(`<a:p><a:r><a:t>` + t + `</a:t></a:r></a:p>`)
		}
		return fmt.Sprintf(`<p:sp>%s<p:spPr><a:xfrm><a:off x="%d" y="%d"/></a:xfrm></p:spPr><p:txBody>%s</p:txBody></p:sp>`, nv, x, y, body.String())
	}
	slide := func(show string, shapes ...string) string {
		return `<p:sld ` + pns + show + `><p:cSld><p:spTree>` + strings.Join(shapes, "") + `</p:spTree></p:cSld></p:sld>`
	}
	tbl := `<p:graphicFrame><p:xfrm><a:off x="0" y="5000"/></p:xfrm><a:graphic><a:graphicData><a:tbl>` +
		`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>季度</a:t></a:r></a:p></a:txBody></a:tc><a:tc><a:txBody><a:p><a:r><a:t>目标</a:t></a:r></a:p></a:txBody></a:tc></a:tr>` +
		`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>Q1</a:t></a:r></a:p></a:txBody></a:tc><a:tc><a:txBody><a:p><a:r><a:t>100 万</a:t></a:r></a:p></a:txBody></a:tc></a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	files := map[string]string{
		"ppt/presentation.xml": `<p:presentation ` + pns + `><p:sldIdLst><p:sldId id="256" r:id="rId2"/><p:sldId id="257" r:id="rId3"/><p:sldId id="258" r:id="rId4"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>` +
			`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide2.xml"/>` +
			`<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide3.xml"/></Relationships>`,
		// 右边的框写在前面，排序后左边的在前
		"ppt/slides/slide1.xml": slide("", sp("", 6000, 2000, "右栏"), sp("title", 0, 0, "年度目标"), sp("", 0, 2000, "左栏第一点", "左栏第二点"), sp("sldNum", 0, 9000, "1"), tbl),
		"ppt/slides/slide2.xml": slide(` show="0"`, sp("title", 0, 0, "隐藏页")),
		"ppt/slides/slide3.xml": slide("", sp("", 0, 1000, "没有标题的一页")),
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide1.xml"/></Relationships>`,
		"ppt/notesSlides/notesSlide1.xml": `<p:notes ` + pns + `><p:cSld><p:spTree>` + sp("body", 0, 0, "讲到这里停一下") + `</p:spTree></p:cSld></p:notes>`,
	}
	out, err := Convert(context.Background(), "年会.pptx", zipOf(t, files), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "## 第 1 页 年度目标\n\n左栏第一点\n左栏第二点\n\n右栏\n\n| 季度 | 目标 |\n| --- | --- |\n| Q1 | 100 万 |\n\n备注：讲到这里停一下\n\n## 第 3 页\n\n没有标题的一页\n"
	if out.Markdown != want {
		t.Fatalf("pptx =\n%s\nwant\n%s", out.Markdown, want)
	}
	if out.Info.Pages != 2 || out.Info.Notes != 1 || out.Info.Tables != 1 {
		t.Fatalf("体检 = %+v", out.Info)
	}
}

func TestOldFormats(t *testing.T) {
	for _, name := range []string{"a.doc", "a.xls", "a.ppt"} {
		_, err := Convert(context.Background(), name, []byte("x"), Options{})
		if err == nil || !strings.Contains(err.Error(), "另存为") {
			t.Fatalf("%s：%v", name, err)
		}
	}
}

func TestSnippetTableHeader(t *testing.T) {
	ix := NewIndex()
	md := "# 价目表\n\n## 第 2～4 行\n\n| 产品 | 单价 |\n| --- | --- |\n| 甲型 | 100 |\n| 乙型 | 200 |\n"
	ix.Load(1, "r1", []*Doc{{ID: 1, Base: 1, Title: "价目表", Version: 1, Text: md}})
	res, _ := ix.Search(context.Background(), Query{Text: "乙型", Bases: []int64{1}})
	if len(res.Hits) != 1 || !strings.HasPrefix(res.Hits[0].Snippet, "| 产品 | 单价 | … ") || res.Hits[0].Line != 8 {
		t.Fatalf("表格行的片段应带表头：%+v", res.Hits)
	}
	res, _ = ix.Search(context.Background(), Query{Text: "产品", Bases: []int64{1}})
	if strings.Count(res.Hits[0].Snippet, "产品") != 1 {
		t.Fatalf("命中表头本身时不用再带：%+v", res.Hits)
	}
}

type pdfSpec struct {
	lines   []string
	image   bool // 整页一张图（扫描页）
	garbled bool // 用一个 ToUnicode 映射到私用区的字体（乱码页）
}

// makePDFPages 手写一个多页 PDF。
func makePDFPages(pages ...pdfSpec) []byte {
	var objs []string
	add := func(o string) int {
		objs = append(objs, o)
		return len(objs)
	}
	add("<< /Type /Catalog /Pages 2 0 R >>")
	add("") // 页树，最后填
	font := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /X def 1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfrange <20> <7E> <E000> endbfrange endcmap CMapName currentdict /CMap defineresource pop end end"
	toUni := add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap))
	badFont := add(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode %d 0 R >>", toUni))
	img := add("<< /Type /XObject /Subtype /Image /Width 300 /Height 300 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 0 >>\nstream\n\nendstream")
	var kids []string
	for _, p := range pages {
		var c strings.Builder
		if p.image {
			c.WriteString("q 300 0 0 300 0 0 cm /Im1 Do Q\n")
		}
		f := "/F1"
		if p.garbled {
			f = "/F2"
		}
		if len(p.lines) > 0 {
			fmt.Fprintf(&c, "BT %s 12 Tf\n", f)
			for i, l := range p.lines {
				fmt.Fprintf(&c, "1 0 0 1 72 %d Tm (%s) Tj\n", 750-i*16, l)
			}
			c.WriteString("ET\n")
		}
		content := add(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", c.Len(), c.String()))
		pg := add(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> /XObject << /Im1 %d 0 R >> >> /Contents %d 0 R >>", font, badFont, img, content))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))
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

func TestConvertPDFPages(t *testing.T) {
	body := func(n int, text string) pdfSpec {
		return pdfSpec{lines: []string{"Galaxy Tech Internal", text, fmt.Sprintf("Section %c continues here.", 'A'+n), fmt.Sprintf("Page %d of 5", n)}}
	}
	data := makePDFPages(
		body(1, "Travel policy: hotel limit is 600 per night."),
		pdfSpec{image: true},
		body(3, "Flights need approval from the manager."),
		pdfSpec{garbled: true, lines: []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ ABCDEFGHIJKLMNOP"}},
		body(5, "Reimburse within 30 days."),
	)
	out, err := Convert(context.Background(), "policy.pdf", data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	md := out.Markdown
	for _, want := range []string{
		"## 第 1 页\n\nTravel policy", "## 第 2 页\n\n> ⚠ 第 2 页是扫描页", "## 第 4 页\n\n> ⚠ 第 4 页抽出来的文字是乱码", "## 第 5 页\n\nReimburse",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("缺少 %q：\n%s", want, md)
		}
	}
	if strings.Contains(md, "Galaxy Tech Internal") || strings.Contains(md, "of 5") {
		t.Fatalf("页眉页脚应去掉：\n%s", md)
	}
	if !strings.Contains(md, "Section B continues here.") {
		t.Fatalf("正文不能当成页眉页脚去掉：\n%s", md)
	}
	want := []SkippedPage{{2, "scanned"}, {4, "garbled"}}
	if out.Info.Pages != 5 || out.Info.HeaderFooter != 2 || fmt.Sprint(out.Info.Skipped) != fmt.Sprint(want) {
		t.Fatalf("体检 = %+v", out.Info)
	}
	// 未收录的页在阅读视图里是提示块，检索时也搜不到乱码
	ix := NewIndex()
	ix.Load(1, "r", []*Doc{{ID: 1, Base: 1, Title: "p", Version: 1, Text: md}})
	if res, _ := ix.Search(context.Background(), Query{Text: "hotel", Bases: []int64{1}}); res.Total != 1 {
		t.Fatalf("文字页应能搜到：%+v", res)
	}
	if res, _ := ix.Search(context.Background(), Query{Text: "扫描页", Bases: []int64{1}}); res.Total != 0 {
		t.Fatalf("占位提示不该被搜到：%+v", res)
	}

	// 全是扫描页：整份拒收，告诉人怎么办
	if _, err := Convert(context.Background(), "scan.pdf", makePDFPages(pdfSpec{image: true}, pdfSpec{image: true}), Options{}); err == nil || !strings.Contains(err.Error(), "扫描件") {
		t.Fatalf("扫描件 = %v", err)
	}
	if _, err := Convert(context.Background(), "bad.pdf", makePDFPages(pdfSpec{garbled: true, lines: []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ"}}), Options{}); err == nil || !strings.Contains(err.Error(), "乱码") {
		t.Fatalf("乱码 = %v", err)
	}
	// 页数少于 3 页时不判断页眉页脚
	out, _ = Convert(context.Background(), "two.pdf", makePDFPages(body(1, "First page body text here."), body(2, "Second page body text.")), Options{})
	if !strings.Contains(out.Markdown, "Galaxy Tech Internal") || out.Info.HeaderFooter != 0 {
		t.Fatalf("两页时不该去页眉：\n%s", out.Markdown)
	}
}
