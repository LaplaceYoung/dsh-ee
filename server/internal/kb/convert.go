package kb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// 支持的格式：Markdown、纯文本、Word（.docx）、PDF（有文字层的）、Excel（.xlsx）、CSV、PPT（.pptx）。
var Formats = []string{".md", ".markdown", ".txt", ".docx", ".pdf", ".xlsx", ".csv", ".pptx"}

// ErrUnsupported 表示不支持这种文件。
var ErrUnsupported = errors.New("只支持 PDF、Word（.docx）、Excel（.xlsx）、CSV、PPT（.pptx）、Markdown 和纯文本文件")

// ErrNoText 表示文件里抽不出文字（多半是扫描件）。
var ErrNoText = errors.New("这个文件里没有可以提取的文字，可能是扫描件；扫描件暂不支持")

// Word、PPT 转换时输出的上限。maxDocxXML 那些管的是输入，管不住输出：横向合并的格子、补齐表格、纵向合并时抄上一格、
// 同一页幻灯片被反复引用，都能让 1 KB 的文件转出几个 GB；接口等转完才查正文有没有超过 8 MB，来不及。
const (
	// 正文最多多少字节（攒在表格里还没写出去的字也按这个数）：接口上限的两倍，正常文件碰不到
	maxOutput = 16 << 20
	// 一份文件里的表格一共最多多少格。每张表按「行数 × 最宽一行」算，因为写成 Markdown 时短的行要补齐
	maxTableCells = 2000000
)

var errTooBig = errors.New("转换出来的正文太大，请拆成几个文件再上传")

// Converted 是转换结果。
type Converted struct {
	Title    string
	Markdown string
	Mime     string
	Info     Info
}

// Info 是转换「体检」：跟员工能不能查到有关的事，上传后给维护者看，也记在文档上。
type Info struct {
	Format string `json:"format"` // md / txt / docx / pdf / xlsx / csv / pptx
	// PDF、PPT 的页数
	Pages int `json:"pages,omitempty"`
	// 没有收录的页（PDF 扫描页、乱码页）
	Skipped []SkippedPage `json:"skipped,omitempty"`
	// 去掉的页眉页脚有几种（PDF）
	HeaderFooter int `json:"header_footer,omitempty"`
	// 补回的自动编号个数（Word）
	Numbering int `json:"numbering,omitempty"`
	Tables    int `json:"tables,omitempty"`
	// Excel / CSV
	Sheets       int    `json:"sheets,omitempty"`
	HiddenSheets int    `json:"hidden_sheets,omitempty"`
	Rows         int    `json:"rows,omitempty"`
	Header       string `json:"header,omitempty"`   // first：第一行是表头；letters：用 A、B、C 当列名
	Encoding     string `json:"encoding,omitempty"` // CSV 不是 UTF-8 时的原编码
	// PPT 里带备注的页数
	Notes int `json:"notes,omitempty"`
}

// SkippedPage 是一页没有收录的原因。
type SkippedPage struct {
	Page   int    `json:"page"`
	Reason string `json:"reason"` // scanned：扫描页；garbled：文字是乱码
}

// Options 是转换时可以改的设置。
type Options struct {
	// Excel / CSV 的表头：""（自动判断）、"first"（第一行是表头）、"none"（第一行就是数据）
	Header string
}

// ctxCheckEvery 是解析循环里隔多少轮检查一次取消/超时。取 2 的幂，用位与算，开销可以忽略。
const ctxCheckEvery = 4096

// checkCtx 在长循环里定期检查请求有没有被取消、转换有没有超时。
// 转换是纯内存计算，文件里的数字（PDF 页数）和重复引用（PPT 同一页被引用几千次）都能让循环跑很久；
// 每次检查都调 ctx.Err() 太碎，所以按 ctxCheckEvery 轮查一次。
func checkCtx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		// 包装成中文，同时保留 errors.Is 能识别的原错误；接口直接把 err.Error() 回给用户。
		return fmt.Errorf("转换超时或已取消（文件太大或内容太复杂）：%w", err)
	}
	return nil
}

// Convert 按文件扩展名把上传的文件转成 Markdown 正文。
//
// ctx 用来取消转换：客户端断开连接、或者上层设的超时到了，转换会尽快停下并返回错误。
// 转换本身不做网络 I/O，ctx 只在解析、拼正文的循环里定期检查。
func Convert(ctx context.Context, name string, data []byte, opt Options) (*Converted, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	ext := strings.ToLower(path.Ext(name))
	title := strings.TrimSpace(strings.TrimSuffix(path.Base(strings.ReplaceAll(name, "\\", "/")), path.Ext(name)))
	var out *Converted
	var err error
	switch ext {
	case ".md", ".markdown":
		out, err = textFile(data, "text/markdown")
		if out != nil {
			out.Info.Format = "md"
		}
	case ".txt":
		out, err = textFile(data, "text/plain")
		if out != nil {
			out.Info.Format = "txt"
		}
	case ".docx":
		out, err = convertDocx(ctx, data)
	case ".pdf":
		out, err = convertPDFEntry(ctx, data)
	case ".xlsx":
		out, err = convertXLSX(ctx, data, opt)
	case ".csv":
		out, err = convertCSV(ctx, data, opt)
	case ".pptx":
		out, err = convertPPTX(ctx, data)
	case ".doc":
		return nil, errors.New("旧版 Word（.doc）不支持，请在 Word 里另存为 .docx 后再上传")
	case ".xls":
		return nil, errors.New("旧版 Excel（.xls）不支持，请在 Excel 里另存为 .xlsx 后再上传")
	case ".ppt":
		return nil, errors.New("旧版 PowerPoint（.ppt）不支持，请在 PowerPoint 里另存为 .pptx 后再上传")
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	out.Markdown = tidy(out.Markdown)
	if strings.TrimSpace(out.Markdown) == "" {
		return nil, ErrNoText
	}
	if out.Title == "" {
		out.Title = title
	}
	return out, nil
}

func textFile(data []byte, mime string) (*Converted, error) {
	text, enc, err := decodeText(data)
	if err != nil {
		return nil, err
	}
	return &Converted{Markdown: text, Mime: mime, Info: Info{Encoding: enc}}, nil
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// tidy 统一换行、去掉行尾空白、把连续多个空行压成一个。
func tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	s = strings.Join(lines, "\n")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.Trim(s, "\n") + "\n"
}

// ---- Word ----

const maxDocxXML = 64 << 20

// convertDocx 读 word/document.xml：段落样式里的「标题 N」转成 #，自动编号补回正文，项目符号转成 -，表格转成 Markdown 表格。
func convertDocx(ctx context.Context, data []byte) (*Converted, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("不是有效的 .docx 文件")
	}
	var doc, styles, num []byte
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			doc, err = readZip(f)
		case "word/styles.xml":
			styles, err = readZip(f)
		case "word/numbering.xml":
			num, err = readZip(f)
		}
		if err != nil {
			return nil, err
		}
	}
	if doc == nil {
		return nil, errors.New("不是有效的 .docx 文件：缺少 word/document.xml")
	}
	st := docxStyles(styles)
	info := Info{Format: "docx"}
	md, title, err := docxBody(ctx, doc, st, parseNumbering(num), &info)
	if err != nil {
		return nil, err
	}
	return &Converted{Title: title, Markdown: md, Mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Info: info}, nil
}

func readZip(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxDocxXML {
		return nil, errors.New(".docx 里的内容太大")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxDocxXML))
}

var headingStyleRe = regexp.MustCompile(`(?i)^(heading|标题)\s*([1-6])$`)

// styleNum 是段落样式里带的自动编号（标题样式常常这样绑定「第一章」）。ilvl 为 -1 表示样式里没写级别。
type styleNum struct {
	numID string
	ilvl  int
}

type docxStyleInfo struct {
	headings map[string]int // 样式 id → 标题级别
	nums     map[string]styleNum
}

// docxStyles 找出标题样式（样式 id → 级别）和样式上的编号。中文 Word 的样式 id 常是数字，要看样式名和大纲级别。
func docxStyles(styles []byte) docxStyleInfo {
	out := docxStyleInfo{headings: map[string]int{"Title": 1}, nums: map[string]styleNum{}}
	for i := 1; i <= 6; i++ {
		out.headings[fmt.Sprintf("Heading%d", i)] = i
	}
	if styles == nil {
		return out
	}
	type val struct {
		Val string `xml:"val,attr"`
	}
	var s struct {
		Styles []struct {
			ID      string `xml:"styleId,attr"`
			Name    val    `xml:"name"`
			BasedOn *val   `xml:"basedOn"`
			PPr     struct {
				Outline *struct {
					Val int `xml:"val,attr"`
				} `xml:"outlineLvl"`
				NumPr *struct {
					Ilvl  *val `xml:"ilvl"`
					NumID *val `xml:"numId"`
				} `xml:"numPr"`
			} `xml:"pPr"`
		} `xml:"style"`
	}
	if xml.Unmarshal(styles, &s) != nil {
		return out
	}
	based := map[string]string{}
	for _, st := range s.Styles {
		if m := headingStyleRe.FindStringSubmatch(strings.TrimSpace(st.Name.Val)); m != nil {
			out.headings[st.ID] = int(m[2][0] - '0')
		} else if st.PPr.Outline != nil && st.PPr.Outline.Val >= 0 && st.PPr.Outline.Val < 6 {
			out.headings[st.ID] = st.PPr.Outline.Val + 1
		}
		if np := st.PPr.NumPr; np != nil && np.NumID != nil {
			sn := styleNum{numID: np.NumID.Val, ilvl: -1}
			if np.Ilvl != nil {
				sn.ilvl, _ = strconv.Atoi(np.Ilvl.Val)
			}
			out.nums[st.ID] = sn
		}
		if st.BasedOn != nil {
			based[st.ID] = st.BasedOn.Val
		}
	}
	// 编号可以从基础样式继承
	for id := range based {
		if _, ok := out.nums[id]; ok {
			continue
		}
		for p, depth := based[id], 0; p != "" && depth < 10; p, depth = based[p], depth+1 {
			if sn, ok := out.nums[p]; ok {
				out.nums[id] = sn
				break
			}
		}
	}
	return out
}

type docxPara struct {
	style   string
	outline int // 段落自己的大纲级别，-1 表示没有
	// 段落自己的编号（numPr）；numID 为空表示没写，用样式上的
	numID  string
	ilvl   int
	hasNum bool // 段落写了 numPr
	text   strings.Builder
}

// docxBody 流式解析正文，只关心段落、文字、换行、制表符、编号和表格。
func docxBody(ctx context.Context, doc []byte, st docxStyleInfo, num *numbering, info *Info) (string, string, error) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var out strings.Builder
	var title string
	var para *docxPara
	// 表格可以嵌套，按栈处理；只输出最外层，嵌套表格的文字并进单元格
	type table struct {
		rows [][]string
		row  []string
		cell strings.Builder
		// 当前单元格：横向合并几列；纵向合并（1 开始合并、2 接着上一行）
		span, vmerge int
		// 最宽一行有几格；补齐以后一共几格（已经记进 cells）
		width, cells int
	}
	var tables []*table
	// 所有表格一共多少格；往单元格里写了多少字节（抄下来的合并格也算，嵌套表格每套一层整张重写一遍、再算一遍）
	cells, tableBytes := 0, 0
	inText, inNumPr := false, false
	writePara := func(p *docxPara) {
		// 先算编号：空段落在 Word 里也会占掉一个号
		numID, ilvl := p.numID, p.ilvl
		if numID == "" {
			if sn, ok := st.nums[p.style]; ok {
				numID, ilvl = sn.numID, sn.ilvl
			}
		}
		if ilvl < 0 {
			ilvl = num.levelFor(numID, p.style)
		}
		var label string
		bullet := p.hasNum
		if numID != "" {
			var ok bool
			label, bullet, ok = num.next(numID, ilvl)
			// 找不到编号定义时按项目符号处理；numId 为 0 表示去掉了编号
			bullet = bullet || !ok && p.hasNum && numID != "0"
		}
		text := strings.TrimSpace(p.text.String())
		if text == "" {
			return
		}
		if label != "" {
			info.Numbering++
		}
		prefixed := text
		if label != "" {
			prefixed = label + num.suffix(numID, ilvl) + text
		}
		if len(tables) > 0 {
			t := tables[len(tables)-1]
			if t.cell.Len() > 0 {
				t.cell.WriteString(" ")
			}
			t.cell.WriteString(prefixed)
			tableBytes += len(prefixed)
			return
		}
		level := st.headings[p.style]
		if p.outline >= 0 && p.outline < 6 {
			level = p.outline + 1
		}
		switch {
		case level > 0:
			if title == "" && level == 1 {
				title = text
			}
			out.WriteString(strings.Repeat("#", level) + " " + strings.ReplaceAll(prefixed, "\n", " ") + "\n\n")
		case bullet:
			out.WriteString("- " + text + "\n")
		default:
			out.WriteString(prefixed + "\n\n")
		}
	}
	// 一份 .docx 的 XML 最多 64 MB，token 数可以到几百万：隔一段查一次请求还在不在。
	tokens := 0
	for {
		if tokens++; tokens&(ctxCheckEvery-1) == 0 {
			if err := checkCtx(ctx); err != nil {
				return "", "", err
			}
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", errors.New(".docx 内容解析失败：" + err.Error())
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				para = &docxPara{outline: -1, ilvl: -1}
			case "pStyle":
				if para != nil {
					para.style = attr(t, "val")
				}
			case "outlineLvl":
				if para != nil {
					fmt.Sscanf(attr(t, "val"), "%d", &para.outline)
				}
			case "numPr":
				inNumPr = true
				if para != nil {
					para.hasNum = true
				}
			case "numId":
				if para != nil && inNumPr {
					para.numID = attr(t, "val")
				}
			case "ilvl":
				if para != nil && inNumPr {
					para.ilvl, _ = strconv.Atoi(attr(t, "val"))
				}
			case "gridSpan":
				if n := len(tables); n > 0 {
					if v, err := strconv.Atoi(attr(t, "val")); err == nil && v > 1 && v <= 64 {
						tables[n-1].span = v
					}
				}
			case "vMerge":
				if n := len(tables); n > 0 {
					if attr(t, "val") == "restart" {
						tables[n-1].vmerge = 1
					} else {
						tables[n-1].vmerge = 2
					}
				}
			case "t":
				inText = true
			case "tab":
				if para != nil {
					para.text.WriteString("\t")
				}
			case "br", "cr":
				if para != nil {
					para.text.WriteString("\n")
				}
			case "tbl":
				tables = append(tables, &table{})
			case "tc":
				if n := len(tables); n > 0 {
					tables[n-1].cell.Reset()
					tables[n-1].span, tables[n-1].vmerge = 1, 0
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "numPr":
				inNumPr = false
			case "p":
				if para != nil {
					writePara(para)
				}
				para = nil
			case "tc":
				if n := len(tables); n > 0 {
					tb := tables[n-1]
					text := strings.ReplaceAll(strings.TrimSpace(tb.cell.String()), "|", "\\|")
					// 纵向合并的格子：把上面那格的内容抄下来，这一行单独看也是完整的
					if col := len(tb.row); tb.vmerge == 2 && text == "" && len(tb.rows) > 0 && col < len(tb.rows[len(tb.rows)-1]) {
						text = tb.rows[len(tb.rows)-1][col]
						tableBytes += len(text)
					}
					// 跟 Excel 一样最多收 maxSheetCols 列（Word 自己最多 63 列），多出来的不要
					if len(tb.row) < maxSheetCols {
						tb.row = append(tb.row, text)
					}
					// 横向合并：补上空格子，列才对得齐
					for i := 1; i < tb.span && len(tb.row) < maxSheetCols; i++ {
						tb.row = append(tb.row, "")
					}
					tb.cell.Reset()
				}
			case "tr":
				if n := len(tables); n > 0 {
					tb := tables[n-1]
					tb.rows = append(tb.rows, tb.row)
					tb.width = max(tb.width, len(tb.row))
					tb.row = nil
					total := len(tb.rows) * tb.width
					cells += total - tb.cells
					tb.cells = total
				}
			case "tbl":
				n := len(tables)
				if n == 0 {
					break
				}
				tb := tables[n-1]
				tables = tables[:n-1]
				md := markdownTable(tb.rows)
				if len(tables) > 0 {
					// 嵌套表格：压成一行文字放进外层单元格
					outer := tables[len(tables)-1]
					outer.cell.WriteString(strings.ReplaceAll(md, "\n", " "))
					tableBytes += len(md)
				} else {
					out.WriteString(md + "\n")
					if md != "" {
						info.Tables++
					}
				}
			}
			if cells > maxTableCells || tableBytes > maxOutput || out.Len() > maxOutput {
				return "", "", errTooBig
			}
		case xml.CharData:
			if inText && para != nil {
				para.text.Write(t)
			}
		}
	}
	return out.String(), title, nil
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func markdownTable(rows [][]string) string {
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	if width == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rows {
		cells := make([]string, width)
		for j := range cells {
			if j < len(r) {
				cells[j] = strings.ReplaceAll(r[j], "\n", " ")
			}
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
		}
	}
	return b.String()
}

// ---- PDF ----

// maxPDFPages 是一份 PDF 最多收多少页。这个数字来自文件里的 /Root/Pages/Count，完全由上传的文件决定；
// 恶意构造的 PDF 写 1e12 页时，按「页数」循环等于让一次请求占满 CPU 几十天（不存在的页 r.Page 返回空页，
// 原逻辑会 continue 到永远）。超过上限整份拒收，不截断：静默丢页会让维护者以为文档收全了。
const maxPDFPages = 2000

// convertPDF 按页抽文字层：同一行的文字拼起来，行距明显变大的地方断段。每页前面加「## 第 N 页」，方便引用时说清在哪一页。
// 每页单独判断：扫描页（几乎没字、有大图）和乱码页（字体缺少编码映射）不收文字，正文里留一句说明；
// 每页都重复的页眉页脚去掉。全部页都收不了时整份拒收。
//
// 压缩放大（一条 FlateDecode 流解压成几 GB）在交给库解析前由 checkPDFInflateBudget 拦掉；
// 正文总量边抽边记账，超过 maxOutput 立即停（页内单行超大时 GetTextByRow 会一次分配很多内存，
// 那部分只能靠 pdfworker.go 的子进程隔离兜底）。
func convertPDF(ctx context.Context, data []byte) (out *Converted, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("PDF 解析失败：%v", r)
		}
	}()
	if err := checkPDFInflateBudget(ctx, data); err != nil {
		return nil, err
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") {
			return nil, errors.New("PDF 有密码保护，请去掉密码后再上传")
		}
		return nil, errors.New("不是有效的 PDF 文件：" + err.Error())
	}
	// 只取一次页数：页数来自文件，每次 r.NumPage() 都会重新解析页树；循环边界也用这个变量。
	pages := r.NumPage()
	if pages > maxPDFPages {
		return nil, fmt.Errorf("PDF 页数太多：共 %d 页，最多 %d 页，请拆成几个文件再上传", pages, maxPDFPages)
	}
	info := Info{Format: "pdf", Pages: pages}
	type page struct {
		n     int
		lines []string // 空字符串是段落之间的空行
		skip  string   // scanned / garbled
	}
	var pageList []*page
	outBytes := 0
	for i := 1; i <= pages; i++ {
		if err := checkCtx(ctx); err != nil {
			return nil, err
		}
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text := ""
		if rows, err := p.GetTextByRow(); err == nil {
			if text, err = pdfPageText(ctx, rows); err != nil {
				return nil, err
			}
		}
		// 边抽边记账：GetTextByRow 一次返回整页文字，等全部页抽完、接口再去查大小就晚了。
		if outBytes += len(text); outBytes > maxOutput {
			return nil, errTooBig
		}
		pg := &page{n: i}
		switch {
		// 先看长度：长文本不可能是「几乎没字」的扫描页，短文本才需要拼一遍数字数。
		case len(text) < 256 && utf8.RuneCountInString(strings.Join(strings.Fields(text), "")) < 20 && pdfHasImage(p):
			pg.skip = "scanned"
		case strings.TrimSpace(text) == "":
			continue // 空白页
		case garbled(text):
			pg.skip = "garbled"
		default:
			pg.lines = strings.Split(text, "\n")
		}
		pageList = append(pageList, pg)
	}
	var textPages []*page
	for _, pg := range pageList {
		if pg.skip == "" {
			textPages = append(textPages, pg)
		} else {
			info.Skipped = append(info.Skipped, SkippedPage{Page: pg.n, Reason: pg.skip})
		}
	}
	if len(textPages) == 0 {
		switch {
		case len(info.Skipped) > 0 && info.Skipped[0].Reason == "scanned":
			return nil, errors.New("这份 PDF 是扫描件（没有文字层），暂不支持；有电子版的话请传电子版")
		case len(info.Skipped) > 0:
			return nil, errors.New("这份 PDF 抽出来的文字是乱码（字体缺少编码信息），请换一份电子版，或另存为 Word 再上传")
		}
		return nil, ErrNoText
	}
	// 页眉页脚：每页最上面、最下面两行，在一半以上的页里都出现（数字不算）就去掉
	if len(textPages) >= 3 {
		removed := map[string]bool{}
		for _, top := range []bool{true, false} {
			count := map[string]int{}
			for _, pg := range textPages {
				seen := map[string]bool{}
				for _, k := range edgeLines(pg.lines, top) {
					if key := edgeKey(pg.lines[k]); key != "" && !seen[key] {
						seen[key] = true
						count[key]++
					}
				}
			}
			for _, pg := range textPages {
				for _, k := range edgeLines(pg.lines, top) {
					if key := edgeKey(pg.lines[k]); key != "" && count[key]*2 > len(textPages) && count[key] >= 3 {
						pg.lines[k] = "\x00"
						removed[key] = true
					}
				}
			}
		}
		info.HeaderFooter = len(removed)
	}
	var b strings.Builder
	for _, pg := range pageList {
		if err := checkCtx(ctx); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "## 第 %d 页\n\n", pg.n)
		switch pg.skip {
		case "scanned":
			fmt.Fprintf(&b, "> ⚠ 第 %d 页是扫描页，没有收录文字，请看原件。\n\n", pg.n)
			continue
		case "garbled":
			fmt.Fprintf(&b, "> ⚠ 第 %d 页抽出来的文字是乱码，没有收录，请看原件。\n\n", pg.n)
			continue
		}
		var kept []string
		for _, l := range pg.lines {
			if l != "\x00" {
				kept = append(kept, l)
			}
		}
		b.WriteString(strings.Trim(strings.Join(kept, "\n"), "\n") + "\n\n")
		if b.Len() > maxOutput {
			return nil, errTooBig
		}
	}
	return &Converted{Markdown: b.String(), Mime: "application/pdf", Info: info}, nil
}

// pdfHasImage 判断页面上有没有像扫描图那样的大图（宽 × 高至少 4 万像素）。
func pdfHasImage(p pdf.Page) bool {
	xo := p.Resources().Key("XObject")
	for _, name := range xo.Keys() {
		img := xo.Key(name)
		if img.Key("Subtype").Name() == "Image" && img.Key("Width").Int64()*img.Key("Height").Int64() >= 40000 {
			return true
		}
	}
	return false
}

// garbled 判断抽出来的字是不是乱码：替换符、私用区、控制字符超过三成。
// 字体缺少 ToUnicode 映射时，抽出来的就是这些字符。
func garbled(text string) bool {
	total, bad := 0, 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if r == utf8.RuneError || r >= 0xE000 && r <= 0xF8FF || r >= 0xF0000 || unicode.IsControl(r) {
			bad++
		}
	}
	return total > 0 && bad*10 > total*3
}

// edgeLines 是一页最上面（top）或最下面两行有字的行的下标。
func edgeLines(lines []string, top bool) []int {
	var out []int
	for i := range lines {
		k := i
		if !top {
			k = len(lines) - 1 - i
		}
		if strings.TrimSpace(lines[k]) != "" && lines[k] != "\x00" {
			out = append(out, k)
			if len(out) == 2 {
				break
			}
		}
	}
	return out
}

var digitRuns = regexp.MustCompile(`[0-9０-９]+`)

// edgeKey 是比较页眉页脚用的：数字换成 #（「第 3 页 / 共 20 页」每页都不一样），去掉空白。
func edgeKey(line string) string {
	return digitRuns.ReplaceAllString(strings.Join(strings.Fields(line), ""), "#")
}

func pdfPageText(ctx context.Context, rows pdf.Rows) (string, error) {
	type line struct {
		y    int64
		text string
	}
	var lines []line
	for i, row := range rows {
		// 一页里也可能有几十万行（构造出来的行坐标），循环中间要能停。
		if i%ctxCheckEvery == 0 {
			if err := checkCtx(ctx); err != nil {
				return "", err
			}
		}
		var b strings.Builder
		var prev rune
		for _, t := range row.Content {
			s := t.S
			if s == "" {
				continue
			}
			first, _ := utf8.DecodeRuneInString(s)
			// 两段英文、数字之间补一个空格；中文直接相连
			if b.Len() > 0 && isWordRune(prev) && isWordRune(first) {
				b.WriteByte(' ')
			}
			b.WriteString(s)
			prev, _ = utf8.DecodeLastRuneInString(s)
		}
		if t := strings.TrimSpace(b.String()); t != "" {
			lines = append(lines, line{row.Position, t})
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	// 典型行距：相邻两行纵坐标差的中位数
	var gaps []int64
	for i := 1; i < len(lines); i++ {
		if g := lines[i-1].y - lines[i].y; g > 0 {
			gaps = append(gaps, g)
		}
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	typical := int64(0)
	if len(gaps) > 0 {
		typical = gaps[(len(gaps)-1)/2]
	}
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			if g := lines[i-1].y - l.y; typical > 0 && g*10 > typical*16 {
				b.WriteString("\n\n")
			} else {
				b.WriteString("\n")
			}
		}
		b.WriteString(l.text)
	}
	return b.String(), nil
}

func isWordRune(r rune) bool {
	return r < 0x2E80 && (unicode.IsLetter(r) || unicode.IsDigit(r))
}
