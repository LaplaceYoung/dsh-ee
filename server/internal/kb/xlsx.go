package kb

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	// MaxSheetRows 是一个 Excel / CSV 文件最多收多少行数据
	MaxSheetRows = 50000
	maxSheetCols = 200
	// 一段表格最多几行、大约多少字；每段都重复表头，单独被搜到、读到时也知道每列是什么
	blockRows  = 100
	blockRunes = 1200
)

var errTooManyRows = fmt.Errorf("表格超过 %d 行，请拆成几个文件再上传", MaxSheetRows)

// sheet 是一个工作表去掉空行空列之后的内容。rowNo、colNo 是原来的行号（从 1 开始）和列号（从 0 开始），
// 用来写「第 2～101 行」和 A、B、C 列名，跟员工在 Excel 里看到的一致。
type sheet struct {
	name  string
	cells [][]string
	rowNo []int
	colNo []int
}

// convertXLSX 每个工作表一节，表格分段，每段开头重复表头；取单元格显示的值，合并单元格的值填到每一格，隐藏的工作表跳过。
func convertXLSX(ctx context.Context, data []byte, opt Options) (out *Converted, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("Excel 解析失败：%v", r)
		}
	}()
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 512 << 20, UnzipXMLSizeLimit: 128 << 20})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") || strings.Contains(err.Error(), "password") {
			return nil, errors.New("Excel 有密码保护，请去掉密码后再上传")
		}
		return nil, errors.New("不是有效的 .xlsx 文件")
	}
	defer f.Close()
	info := Info{Format: "xlsx"}
	var sheets []sheet
	total := 0
	for _, name := range f.GetSheetList() {
		if err := checkCtx(ctx); err != nil {
			return nil, err
		}
		if visible, err := f.GetSheetVisible(name); err == nil && !visible {
			info.HiddenSheets++
			continue
		}
		raw, err := xlsxRows(ctx, f, name, MaxSheetRows-total)
		if err != nil {
			return nil, err
		}
		sh := compactSheet(name, raw)
		if len(sh.cells) == 0 {
			continue
		}
		total += len(sh.cells)
		sheets = append(sheets, sh)
	}
	md := sheetsMarkdown(sheets, true, opt, &info)
	return &Converted{Markdown: md, Mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Info: info}, nil
}

// xlsxRows 读一个工作表显示的值，最多 limit 行（数的是原始行，空行也算，防止超大文件把内存吃光）。
func xlsxRows(ctx context.Context, f *excelize.File, name string, limit int) ([][]string, error) {
	rows, err := f.Rows(name)
	if err != nil {
		return nil, errors.New("Excel 工作表「" + name + "」读取失败")
	}
	defer rows.Close()
	var out [][]string
	nonEmpty := 0
	for n := 0; rows.Next(); n++ {
		if n&(ctxCheckEvery-1) == 0 {
			if err := checkCtx(ctx); err != nil {
				return nil, err
			}
		}
		cols, err := rows.Columns()
		if err != nil {
			return nil, errors.New("Excel 工作表「" + name + "」读取失败")
		}
		if len(cols) > maxSheetCols {
			cols = cols[:maxSheetCols]
		}
		out = append(out, cols)
		if !blankRow(cols) {
			nonEmpty++
			if nonEmpty > limit {
				return nil, errTooManyRows
			}
		}
	}
	// 合并单元格：左上角的值填到整个区域
	merged, _ := f.GetMergeCells(name)
	for _, m := range merged {
		c1, r1, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		c2, r2, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 != nil || err2 != nil || (r2-r1+1)*(c2-c1+1) > 100000 {
			continue
		}
		v := m.GetCellValue()
		for r := r1; r <= min(r2, len(out)); r++ {
			for c := c1; c <= min(c2, maxSheetCols); c++ {
				for len(out[r-1]) < c {
					out[r-1] = append(out[r-1], "")
				}
				out[r-1][c-1] = v
			}
		}
	}
	return out, nil
}

func blankRow(r []string) bool {
	for _, c := range r {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// compactSheet 去掉全空的行和列，单元格里的换行换成空格、竖线转义。
func compactSheet(name string, raw [][]string) sheet {
	sh := sheet{name: name}
	width := 0
	for _, r := range raw {
		width = max(width, len(r))
	}
	used := make([]bool, width)
	for _, r := range raw {
		for j, c := range r {
			if strings.TrimSpace(c) != "" {
				used[j] = true
			}
		}
	}
	for j, u := range used {
		if u {
			sh.colNo = append(sh.colNo, j)
		}
	}
	for i, r := range raw {
		if blankRow(r) {
			continue
		}
		row := make([]string, len(sh.colNo))
		for k, j := range sh.colNo {
			if j < len(r) {
				row[k] = cellText(r[j])
			}
		}
		sh.cells = append(sh.cells, row)
		sh.rowNo = append(sh.rowNo, i+1)
	}
	return sh
}

func cellText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "|", "\\|").Replace(s)
	return s
}

var (
	numberLike = regexp.MustCompile(`^[-+]?[¥$€£]?\s*[\d,]+(\.\d+)?\s*%?$`)
	dateLike   = regexp.MustCompile(`^\d{1,4}[-/.年]\d{1,2}([-/.月]\d{1,4}日?)?(\s+\d{1,2}:\d{2}(:\d{2})?)?$`)
)

func dataLike(s string) bool {
	return numberLike.MatchString(s) || dateLike.MatchString(s)
}

// firstRowIsHeader 猜第一行是不是表头：大部分格子有字、没有数字和日期、不重复，而且下面还有数据。
func firstRowIsHeader(cells [][]string) bool {
	if len(cells) < 2 {
		return false
	}
	head := cells[0]
	filled := 0
	seen := map[string]bool{}
	for _, c := range head {
		if c == "" {
			continue
		}
		if dataLike(c) || seen[c] || utf8.RuneCountInString(c) > 40 {
			return false
		}
		seen[c] = true
		filled++
	}
	return filled*2 >= len(head)
}

// colName 是 Excel 的列名：0 → A，25 → Z，26 → AA。
func colName(j int) string {
	name := ""
	for j++; j > 0; j = (j - 1) / 26 {
		name = string(rune('A'+(j-1)%26)) + name
	}
	return name
}

// sheetsMarkdown 把几个工作表写成 Markdown：withHeading 为 true 时每个工作表一个 # 标题（CSV 只有一个表，不写）。
func sheetsMarkdown(sheets []sheet, withHeading bool, opt Options, info *Info) string {
	var b strings.Builder
	var kinds []string
	for _, sh := range sheets {
		info.Sheets++
		header := firstRowIsHeader(sh.cells)
		switch opt.Header {
		case "first":
			header = true
		case "none":
			header = false
		}
		var head []string
		rows, rowNo := sh.cells, sh.rowNo
		if header {
			head, rows, rowNo = sh.cells[0], sh.cells[1:], sh.rowNo[1:]
			for i, h := range head {
				if h == "" {
					head[i] = colName(sh.colNo[i]) + " 列"
				}
			}
			kinds = append(kinds, "first")
		} else {
			for _, j := range sh.colNo {
				head = append(head, colName(j))
			}
			kinds = append(kinds, "letters")
		}
		info.Rows += len(rows)
		info.Tables++
		level := "##"
		if withHeading {
			fmt.Fprintf(&b, "# %s\n\n", sh.name)
		} else {
			level = "#"
		}
		if len(rows) == 0 {
			b.WriteString(markdownTable([][]string{head}) + "\n")
			continue
		}
		headLen := 0
		for _, h := range head {
			headLen += utf8.RuneCountInString(h)
		}
		// 按行数和字数分段，每段开头重复表头
		for start := 0; start < len(rows); {
			end, size := start, headLen
			for end < len(rows) && end-start < blockRows {
				n := 0
				for _, c := range rows[end] {
					n += utf8.RuneCountInString(c) + 3
				}
				if end > start && size+n > blockRunes {
					break
				}
				size += n
				end++
			}
			// 剩下的行不多了就并进这一段，别留一段只有一两行的
			if rest := len(rows) - end; rest > 0 && rest <= 5 {
				end = len(rows)
			}
			fmt.Fprintf(&b, "%s 第 %d～%d 行\n\n", level, rowNo[start], rowNo[end-1])
			b.WriteString(markdownTable(append([][]string{head}, rows[start:end]...)) + "\n")
			start = end
		}
	}
	switch {
	case len(kinds) == 0:
	case !strings.Contains(strings.Join(kinds, ","), "letters"):
		info.Header = "first"
	case !strings.Contains(strings.Join(kinds, ","), "first"):
		info.Header = "letters"
	default:
		info.Header = "mixed"
	}
	return b.String()
}

// convertCSV 先认编码（Excel 导出的中文 CSV 多是 GBK）、再认分隔符，然后跟 Excel 的一个工作表一样处理。
func convertCSV(ctx context.Context, data []byte, opt Options) (*Converted, error) {
	info := Info{Format: "csv"}
	text, enc, err := decodeText(data)
	if err != nil {
		return nil, err
	}
	info.Encoding = enc
	delim := csvDelimiter(text)
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	var raw [][]string
	nonEmpty := 0
	for n := 0; ; n++ {
		if n&(ctxCheckEvery-1) == 0 {
			if err := checkCtx(ctx); err != nil {
				return nil, err
			}
		}
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("CSV 格式不对：" + err.Error())
		}
		if len(rec) > maxSheetCols {
			rec = rec[:maxSheetCols]
		}
		raw = append(raw, rec)
		if !blankRow(rec) {
			nonEmpty++
			if nonEmpty > MaxSheetRows {
				return nil, errTooManyRows
			}
		}
	}
	sh := compactSheet("", raw)
	var sheets []sheet
	if len(sh.cells) > 0 {
		sheets = append(sheets, sh)
	}
	md := sheetsMarkdown(sheets, false, opt, &info)
	return &Converted{Markdown: md, Mime: "text/csv", Info: info}, nil
}

// decodeText 把文本文件转成 UTF-8：认 BOM（UTF-8 / UTF-16），不是 UTF-8 的按 GB18030（兼容 GBK、GB2312）解。
// 返回原来的编码（UTF-8 时为空）。
func decodeText(data []byte) (string, string, error) {
	switch {
	case bytes.HasPrefix(data, []byte("\xef\xbb\xbf")):
		data = data[3:]
	case bytes.HasPrefix(data, []byte("\xff\xfe")), bytes.HasPrefix(data, []byte("\xfe\xff")):
		out, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder(), data)
		if err != nil {
			return "", "", errors.New("文件编码无法识别，请另存为 UTF-8 后再上传")
		}
		return string(out), "UTF-16", nil
	}
	if utf8.Valid(data) {
		return string(data), "", nil
	}
	out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
	if err != nil || !utf8.Valid(out) {
		return "", "", errors.New("文件编码无法识别，请另存为 UTF-8 后再上传")
	}
	return string(out), "GBK", nil
}

// csvDelimiter 看前几行，逗号、制表符、分号里哪个能把每行分成一样多的列。
func csvDelimiter(text string) rune {
	sample := text
	if len(sample) > 64<<10 {
		sample = sample[:64<<10]
	}
	best, bestScore := ',', -1
	for _, d := range []rune{',', '\t', ';'} {
		r := csv.NewReader(strings.NewReader(sample))
		r.Comma, r.LazyQuotes, r.FieldsPerRecord = d, true, -1
		counts := map[int]int{}
		lines := 0
		for lines < 20 {
			rec, err := r.Read()
			if err != nil {
				break
			}
			lines++
			counts[len(rec)]++
		}
		// 得分：最常见的列数（大于 1）出现了几行
		for n, c := range counts {
			if n > 1 && c > bestScore {
				best, bestScore = d, c
			}
		}
	}
	return best
}
