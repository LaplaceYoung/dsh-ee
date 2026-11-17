package kb

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"errors"
	"io"
)

// PDF 的压缩放大：PDF 里的 FlateDecode 流（zlib/deflate）最高能放大到约 1032:1，
// 一个几百 KB 的文件可以让一条内容流解压成几百 MB 甚至几 GB。ledongthuc/pdf 解压时没有上限，
// 单页解析过程中也拿不到中断点（kbConvertTimeout 只在页与页之间检查），等它返回再查正文大小已经太晚：
// 分配是按 GB 走的，进程会被 OOM 杀掉（runtime 的 fatal error，recover 兜不住）。
//
// 所以在把 PDF 交给库之前先自己扫一遍文件里的流：按 /Length（没有就找 endstream）定位每条流，
// 用 zlib 流式解压、只数解压出来的字节数（不攒内容，扫描本身内存是常数），超过预算就整份拒收。
// 扫描失败（不是 FlateDecode、链式过滤器、PDF 结构太怪）时退回正常路径，不误拒正常文件；
// 真正的硬上限由子进程隔离兜底（见 pdfworker.go）。
const (
	// 单条流解压后的上限。正常 PDF 的内容流（一页文字）通常几十 KB；
	// 一页几十 MB 的“文字”必然是构造出来的。
	maxPDFStreamBytes = 16 << 20
	// 一份 PDF 里所有流（不含图片、内嵌字体程序，见 pdfStreamSkippable）解压后的总上限。
	maxPDFStreamTotal = 64 << 20
)

var errPDFTooBig = errors.New("PDF 内容太多或文件太大，请拆成几个文件再上传")

// 预扫描自身的 CPU 预算：PDF 可能有成千上万条流，每条都往回找字典会退化成 O(n²)，
// 所以给单条字典的往回查找和整份文件的往回查找总量都设上限；超了就停止预扫描（交给子进程兜底）。
const (
	maxPDFDictScan    = 1 << 20
	maxPDFScanBackSum = 64 << 20
)

// checkPDFInflateBudget 预扫描 PDF 里的 FlateDecode 流，解压后超过预算的整份拒收。
// 只做“拒绝”，不做改写：解压不了/定位不了的流一律放过（交给库），避免误杀正常 PDF。
func checkPDFInflateBudget(ctx context.Context, data []byte) error {
	// 不是 PDF 头就交给库去报「不是有效的 PDF 文件」。
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil
	}
	total := 0
	scanBack := 0
	for pos := 0; pos < len(data); {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		i := bytes.Index(data[pos:], []byte("stream"))
		if i < 0 {
			break
		}
		kw := pos + i
		pos = kw + len("stream")
		// "endstream" 里的 "stream"、名字对象 /streams 之类不算关键字。
		if kw > 0 && !pdfSpace(data[kw-1]) && !pdfDelim(data[kw-1]) {
			continue
		}
		ds := pdfStreamDataStart(data, pos)
		if ds < 0 {
			continue
		}
		dictStart, dictEnd := pdfDictBeforeStream(data, kw, maxPDFDictScan)
		if dictEnd > dictStart {
			if scanBack += dictEnd - dictStart; scanBack > maxPDFScanBackSum {
				break
			}
		}
		dict := data[dictStart:dictEnd]
		end, next := pdfStreamBounds(data, ds, dict)
		if next <= kw {
			// 定位不到流的结尾：不冒险继续扫（可能已经在二进制流里），直接交给库。
			break
		}
		pos = next
		if pdfStreamSkippable(dict) {
			// 图片流、内嵌字体程序流：文字抽取不会解压它们（库只读页面内容流、ToUnicode、
			// 对象流和 xref 流），给它们算预算只会误杀正常文档。
			continue
		}
		n, ok := pdfInflateSize(data[ds:end], maxPDFStreamBytes)
		if !ok {
			continue
		}
		if n > maxPDFStreamBytes {
			return errPDFTooBig
		}
		total += n
		if total > maxPDFStreamTotal {
			return errPDFTooBig
		}
	}
	return nil
}

// pdfStreamDataStart 返回 stream 关键字之后数据开始的位置（按规范跳过 CRLF/LF/CR），不合法时返回 -1。
func pdfStreamDataStart(data []byte, pos int) int {
	if pos >= len(data) {
		return -1
	}
	switch data[pos] {
	case '\r':
		if pos+1 < len(data) && data[pos+1] == '\n' {
			return pos + 2
		}
		return pos + 1
	case '\n':
		return pos + 1
	}
	return -1
}

// pdfDictBeforeStream 从 stream 关键字往回找与它配对的字典。按最朴素的括号配对处理
// （字典的字符串值里恰好出现 "<<" / ">>" 的情况很少见）；找不到时返回 0,0。
func pdfDictBeforeStream(data []byte, kw, limit int) (int, int) {
	j := kw
	for j > 0 && pdfSpace(data[j-1]) {
		j--
	}
	if j < 2 || data[j-1] != '>' || data[j-2] != '>' {
		return 0, 0
	}
	min := j - limit
	if min < 0 {
		min = 0
	}
	depth := 1
	for i := j - 2; i >= 2 && i >= min; i-- {
		if data[i-1] == '>' && data[i-2] == '>' {
			depth++
			i--
			continue
		}
		if data[i-1] == '<' && data[i-2] == '<' {
			depth--
			i--
			if depth == 0 {
				return i - 1, j
			}
		}
	}
	return 0, 0
}

// pdfStreamBounds 返回流数据的结束位置和扫完这条流之后应该继续扫的位置。
// 优先用字典里直接写出的 /Length（精确，二进制数据里的 "endstream" 不会误伤）；
// /Length 是间接引用或不可用时退回找 endstream。
func pdfStreamBounds(data []byte, ds int, dict []byte) (end, next int) {
	if n, ok := pdfDirectLength(dict); ok && n >= 0 && ds+n <= len(data) {
		end = ds + n
		j := end
		for j < len(data) && pdfSpace(data[j]) {
			j++
		}
		if bytes.HasPrefix(data[j:], []byte("endstream")) {
			return end, j + len("endstream")
		}
	}
	i := bytes.Index(data[ds:], []byte("endstream"))
	if i < 0 {
		return len(data), len(data)
	}
	end = ds + i
	return end, end + len("endstream")
}

// pdfDirectLength 从流字典里取直接写出的 /Length。间接引用（/Length 5 0 R）不解析，返回 false
// 让调用方退回 endstream 定位：解析间接对象需要全文件搜索，恶意文件里放大次数不值得。
func pdfDirectLength(dict []byte) (int, bool) {
	key := []byte("/Length")
	for off := 0; ; {
		i := bytes.Index(dict[off:], key)
		if i < 0 {
			return 0, false
		}
		i += off
		off = i + len(key)
		p := off
		// 键名后面必须跟空白再跟值；/Length1、/Length2 这些是字体程序的键，跳过。
		if p >= len(dict) || !pdfSpace(dict[p]) {
			continue
		}
		for p < len(dict) && pdfSpace(dict[p]) {
			p++
		}
		start := p
		n := 0
		for p < len(dict) && dict[p] >= '0' && dict[p] <= '9' && p-start < 10 {
			n = n*10 + int(dict[p]-'0')
			p++
		}
		if p == start {
			continue
		}
		// 后面还有对象号/代号再跟 "R"，说明是间接引用，不认。
		q := p
		for q < len(dict) && pdfSpace(dict[q]) {
			q++
		}
		if q < len(dict) && dict[q] >= '0' && dict[q] <= '9' {
			continue
		}
		return n, true
	}
}

// pdfInflateSize 量一条流的解压后大小，最多读到 limit+1 字节（只数不攒）。
// ok=false 表示它不是 zlib 流（也不是 ASCII85Decode+FlateDecode 链），交给库处理。
func pdfInflateSize(raw []byte, limit int) (n int, ok bool) {
	if n, ok = pdfInflateReader(bytes.NewReader(raw), limit); ok {
		return n, true
	}
	// /Filter [/ASCII85Decode /FlateDecode]：库会先做 ASCII85 再解压，这里同样试一遍。
	// 先用跟库一样的过滤（PDF 的 ASCII85 流常以 "~>" 结尾，标准库解码器不认），再解码。
	dec := ascii85.NewDecoder(&pdfASCII85Reader{r: bytes.NewReader(raw)})
	if n, ok = pdfInflateReader(dec, limit); ok {
		return n, true
	}
	return 0, false
}

// pdfASCII85Reader 跟 ledongthuc/pdf 的 alphaReader 行为一致：只留 ASCII85 字符，
// 遇到 "~" 结束（PDF 的 ASCII85 数据以 "~>" 终止，标准库的 ascii85 解码器不认这个终止符）。
type pdfASCII85Reader struct {
	r   io.Reader
	eod bool
}

func (a *pdfASCII85Reader) Read(p []byte) (int, error) {
	if a.eod {
		return 0, io.EOF
	}
	n, err := a.r.Read(p)
	out := 0
	for i := 0; i < n; i++ {
		c := p[i]
		if c == '~' {
			a.eod = true
			return out, io.EOF
		}
		if c >= '!' && c <= 'u' || c == 'z' {
			p[out] = c
			out++
		}
	}
	return out, err
}

func pdfInflateReader(r io.Reader, limit int) (int, bool) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return 0, false
	}
	defer zr.Close()
	n, err := io.CopyN(io.Discard, zr, int64(limit)+1)
	switch {
	case err == nil:
		return int(n), true // 读满 limit+1，说明超限（调用方只看 n > limit）
	case errors.Is(err, io.EOF):
		return int(n), true
	default:
		if n > 0 {
			return int(n), true // 解压到一半坏了：库那边同样解不出来，会被 recover 兜住
		}
		return 0, false
	}
}

// pdfStreamSkippable 判断这条流会不会被文字抽取解压。图片和字体程序不会：
// 库只读页面 /Contents（含 ToUnicode CMap）、对象流（/Type /ObjStm）和 xref 流（/Type /XRef）。
func pdfStreamSkippable(dict []byte) bool {
	for _, k := range [][2]string{
		{"Subtype", "Image"}, {"Subtype", "Type1C"}, {"Subtype", "CIDFontType0C"},
		{"Subtype", "OpenType"}, {"Subtype", "TrueType"},
	} {
		if pdfNameValue(dict, k[0], k[1]) {
			return true
		}
	}
	// Type1 / TrueType 字体程序：/Length1 是字体文件的长度，这两个键只在字体流里出现。
	return bytes.Contains(dict, []byte("/Length1")) || bytes.Contains(dict, []byte("/Length2"))
}

// pdfNameValue 判断字典里 key 的值是不是名字 val（如 /Subtype /Image）。
func pdfNameValue(dict []byte, key, val string) bool {
	pat := []byte("/" + key)
	for off := 0; ; {
		i := bytes.Index(dict[off:], pat)
		if i < 0 {
			return false
		}
		i += off
		off = i + len(pat)
		// 键名要完整：/SubtypeX 不算 /Subtype。
		if off < len(dict) && !pdfSpace(dict[off]) && !pdfDelim(dict[off]) {
			continue
		}
		p := off
		for p < len(dict) && pdfSpace(dict[p]) {
			p++
		}
		if p < len(dict) && dict[p] == '/' && bytes.HasPrefix(dict[p+1:], []byte(val)) {
			e := p + 1 + len(val)
			if e >= len(dict) || pdfSpace(dict[e]) || pdfDelim(dict[e]) {
				return true
			}
		}
	}
}

// pdfSpace/pdfDelim 跟 PDF 规范里的空白、分隔符一致。
func pdfSpace(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

func pdfDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}
