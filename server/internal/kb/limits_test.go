package kb

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

const pmlNS = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

func docxOf(t *testing.T, body string, numbering string) []byte {
	t.Helper()
	files := map[string]string{"word/document.xml": `<w:document ` + wns + `><w:body>` + body + `</w:body></w:document>`}
	if numbering != "" {
		files["word/numbering.xml"] = `<w:numbering ` + wns + `>` + numbering + `</w:numbering>`
	}
	return zipOf(t, files)
}

func slideOf(shapes string) string {
	return `<p:sld ` + pmlNS + `><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:sld>`
}

// pptxOf 是只有一页的 PPT，这一页在 sldIdLst 里被引用 refs 次。
func pptxOf(t *testing.T, refs int, slide string) []byte {
	t.Helper()
	return zipOf(t, map[string]string{
		"ppt/presentation.xml": `<p:presentation ` + pmlNS + `><p:sldIdLst>` + strings.Repeat(`<p:sldId id="256" r:id="rId2"/>`, refs) + `</p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml": slide,
	})
}

// 压缩后只有几百字节到几十 KB 的文件，不能转出几十 MB 的正文（把下面的行数再放大几倍就是几个 GB，服务会被系统杀掉）。
// 接口对正文大小的检查在转完之后，拦不住，得转换的时候自己停下。
func TestConvertOutputBounded(t *testing.T) {
	para := func(text string) string { return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>` }
	numbered := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`
	numbering := func(start int, format, text string) string {
		return `<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="` + strconv.Itoa(start) + `"/><w:numFmt w:val="` + format + `"/><w:lvlText w:val="` + text + `"/></w:lvl></w:abstractNum>` +
			`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`
	}
	pptTable := func(rows string) string {
		return `<p:graphicFrame><a:graphic><a:graphicData><a:tbl>` + rows + `</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	}
	cases := []struct {
		name, file string
		data       []byte
		// 不为空：应该拒收，报错里有这几个字；为空：应该照常转出来，而且正文很小
		wantErr string
	}{
		// 一行 300 个横向合并 64 列的格子，下面 400 行各一格：每行都补齐成 19200 列
		{"Word 横向合并", "a.docx", docxOf(t, `<w:tbl><w:tr>`+strings.Repeat(`<w:tc><w:tcPr><w:gridSpan w:val="64"/></w:tcPr></w:tc>`, 300)+`</w:tr>`+
			strings.Repeat(`<w:tr><w:tc/></w:tr>`, 400)+`</w:tbl>`, ""), ""},
		// 列数不超，行数很多：200 列 × 3 万行
		{"Word 表格补齐", "a.docx", docxOf(t, `<w:tbl><w:tr>`+strings.Repeat(`<w:tc/>`, 200)+`</w:tr>`+
			strings.Repeat(`<w:tr><w:tc/></w:tr>`, 30000)+`</w:tbl>`, ""), "正文太大"},
		// 一格 2 万字，下面 1000 行都是纵向合并，每行抄一遍
		{"Word 纵向合并", "a.docx", docxOf(t, `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr>`+para(strings.Repeat("x", 20000))+`</w:tc></w:tr>`+
			strings.Repeat(`<w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr></w:tc></w:tr>`, 1000)+`</w:tbl>`, ""), "正文太大"},
		// 100 万格的表外面套 6 层：每套一层整张表重写一遍，竖线再多转义一次
		{"Word 嵌套表格", "a.docx", docxOf(t, strings.Repeat(`<w:tbl><w:tr><w:tc>`, 6)+
			`<w:tbl>`+strings.Repeat(`<w:tr>`+strings.Repeat(`<w:tc/>`, 200)+`</w:tr>`, 5000)+`</w:tbl>`+
			strings.Repeat(`</w:tc></w:tr></w:tbl>`, 6), ""), "正文太大"},
		// 编号文字 2 万字，1000 个编号段落
		{"Word 编号文字", "a.docx", docxOf(t, strings.Repeat(numbered, 1000), numbering(1, "decimal", strings.Repeat("y", 20000)+"%1")), "正文太大"},
		// 字母编号从 5.2 亿开始：按 Word 的写法要重复 2000 万个字母
		{"Word 字母编号", "a.docx", docxOf(t, numbered, numbering(26*20000000, "upperLetter", "%1")), ""},
		// 同一页（2 万字）引用 1000 次
		{"PPT 重复引用", "a.pptx", pptxOf(t, 1000, slideOf(`<p:sp><p:txBody><a:p><a:r><a:t>`+strings.Repeat("z", 20000)+`</a:t></a:r></a:p></p:txBody></p:sp>`)), "正文太大"},
		// 只有一页、页内一段就超过上限：之前只在进入下一页前检查，单页写完直接返回
		{"PPT 单页超限", "a.pptx", pptxOf(t, 1, slideOf(`<p:sp><p:txBody><a:p><a:r><a:t>`+strings.Repeat("z", 20<<20)+`</a:t></a:r></a:p></p:txBody></p:sp>`)), "正文太大"},
		{"PPT 页数", "a.pptx", pptxOf(t, 5001, slideOf(`<p:sp><p:txBody><a:p><a:r><a:t>z</a:t></a:r></a:p></p:txBody></p:sp>`)), "幻灯片超过"},
		// 一行 2500 格，下面 2500 个空行
		{"PPT 表格补齐", "a.pptx", pptxOf(t, 1, slideOf(pptTable(`<a:tr>`+strings.Repeat(`<a:tc/>`, 2500)+`</a:tr>`+strings.Repeat(`<a:tr/>`, 2500)))), "正文太大"},
		// 60 万层嵌套的空元素：没有一个字，树却有几百 MB
		{"PPT 元素个数", "a.pptx", pptxOf(t, 1, slideOf(strings.Repeat(`<a:x>`, 600000)+strings.Repeat(`</a:x>`, 600000))), "元素超过"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Convert(c.file, c.data, Options{})
			size := 0
			if out != nil {
				size = len(out.Markdown)
			}
			switch {
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Fatalf("%d 字节的文件应该拒收（%s），结果 err = %v，正文 %.1f MB", len(c.data), c.wantErr, err, float64(size)/(1<<20))
			case c.wantErr == "" && (err != nil || size > 1<<20):
				t.Fatalf("%d 字节的文件应该转出很小的正文，结果 err = %v，正文 %.1f MB", len(c.data), err, float64(size)/(1<<20))
			}
		})
	}
}

// Word 表格最多收 200 列（跟 Excel 一样），多出来的不要；起始值超出 Word 范围的字母编号写成数字。
func TestDocxColumnCap(t *testing.T) {
	var row strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&row, `<w:tc><w:p><w:r><w:t>c%d</w:t></w:r></w:p></w:tc>`, i)
	}
	out, err := Convert("宽表.docx", docxOf(t, `<w:tbl><w:tr>`+row.String()+`</w:tr></w:tbl>`, ""), Options{})
	if err != nil {
		t.Fatal(err)
	}
	head := strings.SplitN(out.Markdown, "\n", 2)[0]
	if strings.Count(head, " | ") != 199 || !strings.HasSuffix(head, "| c199 |") {
		t.Fatalf("应该只收前 200 列：%d 列，结尾 %q", strings.Count(head, " | ")+1, head[max(0, len(head)-20):])
	}
	if got := formatNum(32767, "upperLetter"); len(got) != 1261 || formatNum(32768, "upperLetter") != "32768" {
		t.Fatalf("字母编号 32767 = %d 个字母，32768 = %s", len(got), formatNum(32768, "upperLetter"))
	}
}

// 正常的大文件不受影响：5000 行的表、63 列的表（Word 的上限）、表里套表、300 页的 PPT。
func TestConvertLargeDocs(t *testing.T) {
	cell := func(text string) string { return `<w:tc><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc>` }
	var body strings.Builder
	body.WriteString(`<w:tbl>`)
	for i := 1; i <= 5000; i++ {
		body.WriteString(`<w:tr>` + cell(fmt.Sprintf("第 %d 行", i)) + strings.Repeat(cell("差旅费报销标准按城市分档执行"), 5) + `</w:tr>`)
	}
	body.WriteString(`</w:tbl><w:p><w:r><w:t>宽表</w:t></w:r></w:p><w:tbl>`)
	for i := 0; i < 50; i++ {
		body.WriteString(`<w:tr>` + strings.Repeat(cell("格"), 63) + `</w:tr>`)
	}
	// 版式表格：整页内容放在一个单元格里，里面再有表格
	body.WriteString(`</w:tbl><w:p><w:r><w:t>套表</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:tbl><w:tr><w:tc><w:tbl>`)
	for i := 0; i < 500; i++ {
		body.WriteString(`<w:tr>` + strings.Repeat(cell("审批人签字"), 8) + `</w:tr>`)
	}
	body.WriteString(`</w:tbl></w:tc></w:tr></w:tbl></w:tc></w:tr></w:tbl>`)
	out, err := Convert("大表.docx", docxOf(t, body.String(), ""), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Info.Tables != 3 || strings.Count(out.Markdown, "差旅费报销标准按城市分档执行") != 25000 ||
		strings.Count(out.Markdown, "格") != 50*63 || strings.Count(out.Markdown, "审批人签字") != 4000 {
		t.Fatalf("大表格没转全：%+v，%d 字节", out.Info, len(out.Markdown))
	}

	sp := func(ph, text string) string {
		return `<p:sp><p:nvSpPr><p:nvPr><p:ph type="` + ph + `"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`
	}
	tc := `<a:tc><a:txBody><a:p><a:r><a:t>指标</a:t></a:r></a:p></a:txBody></a:tc>`
	tbl := `<p:graphicFrame><a:graphic><a:graphicData><a:tbl>` + strings.Repeat(`<a:tr>`+strings.Repeat(tc, 8)+`</a:tr>`, 20) + `</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	files := map[string]string{}
	var ids, rels strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&ids, `<p:sldId id="%d" r:id="rId%d"/>`, 255+i, i)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i, i)
		files[fmt.Sprintf("ppt/slides/slide%d.xml", i)] = slideOf(sp("title", fmt.Sprintf("第 %d 讲", i)) + strings.Repeat(sp("body", "本页要点：流程、责任人、时限"), 20) + tbl)
		files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i)] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			fmt.Sprintf(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide%d.xml"/></Relationships>`, i)
		files[fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i)] = `<p:notes ` + pmlNS + `><p:cSld><p:spTree>` + sp("body", "讲稿") + `</p:spTree></p:cSld></p:notes>`
	}
	files["ppt/presentation.xml"] = `<p:presentation ` + pmlNS + `><p:sldIdLst>` + ids.String() + `</p:sldIdLst></p:presentation>`
	files["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() + `</Relationships>`
	out, err = Convert("培训.pptx", zipOf(t, files), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Info.Pages != 300 || out.Info.Notes != 300 || out.Info.Tables != 300 || !strings.Contains(out.Markdown, "## 第 300 页 第 300 讲") {
		t.Fatalf("300 页的 PPT 没转全：%+v", out.Info)
	}
}
