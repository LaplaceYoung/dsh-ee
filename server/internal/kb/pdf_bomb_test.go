package kb

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// PDF 的压缩放大：一条 FlateDecode 流解压后能到几百 MB / 几 GB（deflate 上限约 1032:1），
// 而 ledongthuc/pdf 解压没有上限，等它返回再查正文大小已经来不及（分配按 GB 走，进程会被 OOM 杀掉）。
// 这里造的都是「文件只有几十 KB、解压后很大」的 PDF：构造时用流式 zlib，测试进程自己不会先吃掉几百 MB。

// buildPDF 把一组间接对象（对象体，不含 obj/endobj 和编号）拼成带 xref 的最小 PDF。
func buildPDF(objs ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", i+1)
		b.Write(o)
		b.WriteString("\nendobj\n")
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// streamObj 造一个流对象：/Length 按 data 写，filters 是 /Filter 的值（如 "/Filter /FlateDecode"），
// dictExtra 拼在字典里（如 /Subtype /Image 那些）。
func streamObj(data []byte, filters, dictExtra string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<< /Length %d", len(data))
	if filters != "" {
		fmt.Fprintf(&b, " %s", filters)
	}
	if dictExtra != "" {
		fmt.Fprintf(&b, " %s", dictExtra)
	}
	b.WriteString(" >>\nstream\n")
	b.Write(data)
	b.WriteString("\nendstream")
	return b.Bytes()
}

// flateLiteral 流式压缩一段内容流：BT ... (size 个 A) Tj ET。compressContent 里不会真的攒下 size 字节。
func flateLiteral(size int, ascii85Chain bool) (data []byte, filters string) {
	var comp bytes.Buffer
	zw := zlib.NewWriter(&comp)
	_, _ = zw.Write([]byte("BT /F1 12 Tf 72 720 Td ("))
	chunk := bytes.Repeat([]byte("A"), 64<<10)
	for left := size; left > 0; {
		n := min(len(chunk), left)
		if _, err := zw.Write(chunk[:n]); err != nil {
			panic(err)
		}
		left -= n
	}
	_, _ = zw.Write([]byte(") Tj ET\n"))
	_ = zw.Close()
	if !ascii85Chain {
		return comp.Bytes(), "/Filter /FlateDecode"
	}
	var enc bytes.Buffer
	w := ascii85.NewEncoder(&enc)
	_, _ = w.Write(comp.Bytes())
	_ = w.Close()
	// 跟真实 PDF 一样以 "~>" 结尾（标准库编码器不加这个终止符，解码器也不认，预扫描要自己处理）。
	enc.WriteString("~>")
	return enc.Bytes(), "/Filter [/ASCII85Decode /FlateDecode]"
}

// pdfOnePage 造一页 PDF：/Contents 是给定的若干条流（多条时写成数组）。
func pdfOnePage(t *testing.T, contents []pdfContent) []byte {
	t.Helper()
	objs := make([][]byte, 0, 3+len(contents))
	refs := make([]string, len(contents))
	for i := range contents {
		refs[i] = fmt.Sprintf("%d 0 R", 4+i)
	}
	contentsRef := refs[0]
	if len(refs) > 1 {
		contentsRef = "[" + strings.Join(refs, " ") + "]"
	}
	objs = append(objs,
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %s /Resources << >> >>", contentsRef)),
	)
	for _, c := range contents {
		objs = append(objs, streamObj(c.data, c.filters, c.dictExtra))
	}
	return buildPDF(objs...)
}

type pdfContent struct {
	data      []byte
	filters   string
	dictExtra string
}

// 修前红：32 MB 的压缩流（文件 ~32 KB）。修前 Convert 会把它解压成 32 MB 字面量并返回成功
// （实测 allocdelta≈832 MB、0.8s）；修后应在预扫描里读到 16 MB 上限就立即拒收，
// 几乎不分配内存（只流式数字节）。
func TestPDFFlateBombRejected(t *testing.T) {
	data, filters := flateLiteral(32<<20, false)
	pdf := pdfOnePage(t, []pdfContent{{data: data, filters: filters}})
	if len(pdf) > 1<<20 {
		t.Fatalf("复现文件应该只有几十 KB，实际 %d 字节", len(pdf))
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "bomb.pdf", pdf, Options{})
	runtime.ReadMemStats(&after)
	allocMB := float64(after.TotalAlloc-before.TotalAlloc) / (1 << 20)
	if out != nil {
		t.Fatalf("压缩放大的 PDF 应该整份拒收，不能返回正文")
	}
	if err == nil || !strings.Contains(err.Error(), "内容太多") {
		t.Fatalf("应报「内容太多」：%v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("应该在预扫描阶段快速拒收，实际 %v", elapsed)
	}
	// 真的解压 32 MB 时 allocdelta 是几百 MB（实测 832 MB）；这里只允许扫描的常数级开销。
	if allocMB > 64 {
		t.Fatalf("拒收过程不应该解压整条流：allocdelta = %.1f MB", allocMB)
	}
}

// /Filter [/ASCII85Decode /FlateDecode] 链也能放大：库先做 ASCII85 再解压。
// 预扫描要按同样顺序试一遍，否则换个过滤器就绕过去了。
func TestPDFASCII85FlateBombRejected(t *testing.T) {
	data, filters := flateLiteral(32<<20, true)
	pdf := pdfOnePage(t, []pdfContent{{data: data, filters: filters}})
	out, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "bomb85.pdf", pdf, Options{})
	if out != nil {
		t.Fatalf("ASCII85+Flate 的压缩放大也应该拒收")
	}
	if err == nil || !strings.Contains(err.Error(), "内容太多") {
		t.Fatalf("应报「内容太多」：%v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("应该在预扫描阶段快速拒收，实际 %v", elapsed)
	}
}

// 正常 PDF 里的 FlateDecode 内容流不能因为预扫描被误拒：压缩过的页面文字要照常抽出来。
func TestPDFCompressedContentOK(t *testing.T) {
	var comp bytes.Buffer
	zw := zlib.NewWriter(&comp)
	_, _ = zw.Write([]byte("BT /F1 12 Tf 72 720 Td (Quarterly travel policy: hotel limit is 600 per night.) Tj ET\n"))
	_ = zw.Close()
	pdf := pdfOnePage(t, []pdfContent{{data: comp.Bytes(), filters: "/Filter /FlateDecode"}})
	out, err := Convert(context.Background(), "policy.pdf", pdf, Options{})
	if err != nil {
		t.Fatalf("正常 PDF 不应被拒：%v", err)
	}
	if !strings.Contains(out.Markdown, "hotel limit is 600") || out.Info.Format != "pdf" || out.Info.Pages != 1 {
		t.Fatalf("正常 PDF 没转出来：%+v，正文 %q", out.Info, out.Markdown)
	}
}

// 正文总量预算：每页的流都在上限之内、整份总解压量也在上限之内，但抽出来的正文超过 maxOutput 要拒收，
// 不能等接口在转换之后去查 8 MB。
func TestPDFOutputBudgetRejected(t *testing.T) {
	const pageText = 5 << 20 // 5 MB/页 × 4 页 = 20 MB > maxOutput（16 MB）
	var contents []pdfContent
	for i := 0; i < 4; i++ {
		d, f := flateLiteral(pageText, false)
		contents = append(contents, pdfContent{data: d, filters: f})
	}
	pdf := pdfOnePage(t, contents)
	_, err, elapsed := convertWithin(t, 30*time.Second, context.Background(), "big-text.pdf", pdf, Options{})
	if err == nil || !strings.Contains(err.Error(), "正文太大") {
		t.Fatalf("正文超过 maxOutput 应拒收：%v", err)
	}
	if elapsed > 20*time.Second {
		t.Fatalf("正文预算检查太慢：%v", elapsed)
	}
}

// 正常文档里的大图片流（FlateDecode、解压后 20 MB）不能被误拒：文字抽取根本不读图片流，
// 预扫描跳过它们，否则带 Flate 截图的正常 PDF 会被误杀。
func TestPDFFlateImageStreamOK(t *testing.T) {
	var comp bytes.Buffer
	zw := zlib.NewWriter(&comp)
	zero := make([]byte, 64<<10)
	for i := 0; i < 20<<20/(64<<10); i++ {
		_, _ = zw.Write(zero)
	}
	_ = zw.Close()
	img := streamObj(comp.Bytes(), "/Filter /FlateDecode", "/Type /XObject /Subtype /Image /Width 300 /Height 300 /ColorSpace /DeviceGray /BitsPerComponent 8")

	var text bytes.Buffer
	tw := zlib.NewWriter(&text)
	_, _ = tw.Write([]byte("BT /F1 12 Tf 72 720 Td (Scanned cover page with photo, policy text follows.) Tj ET\n"))
	_ = tw.Close()

	// 对象：1 目录、2 页树、3 页、4 文字内容流、5 字体、6 图片。
	pdf := buildPDF(
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 6 0 R >> >> >>"),
		streamObj(text.Bytes(), "/Filter /FlateDecode", ""),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		img,
	)
	out, err, elapsed := convertWithin(t, 10*time.Second, context.Background(), "photo.pdf", pdf, Options{})
	if err != nil {
		t.Fatalf("带 Flate 图片流的正常 PDF 不应被拒：%v", err)
	}
	if !strings.Contains(out.Markdown, "policy text follows") {
		t.Fatalf("文字没抽出来：%q", out.Markdown)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("转换太慢：%v", elapsed)
	}
}

// 单条流都没超上限，但整份加起来超了总预算（5 × 14 MB = 70 MB > 64 MB）也要拒收：
// 一条页面的 /Contents 可以是数组，逐条看都不大、加起来一样能把内存撑爆。
func TestPDFStreamTotalBudgetRejected(t *testing.T) {
	var contents []pdfContent
	for i := 0; i < 5; i++ {
		d, f := flateLiteral(14<<20, false)
		contents = append(contents, pdfContent{data: d, filters: f})
	}
	pdf := pdfOnePage(t, contents)
	out, err, elapsed := convertWithin(t, 10*time.Second, context.Background(), "total.pdf", pdf, Options{})
	if out != nil || err == nil || !strings.Contains(err.Error(), "内容太多") {
		t.Fatalf("总解压量超过预算应拒收：out=%v err=%v", out, err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("应该在预扫描阶段快速拒收，实际 %v", elapsed)
	}
}

// 间接 /Length（/Filter /FlateDecode 加 /Length 5 0 R）很常见：预扫描没有直接长度可用时
// 要能退回按 endstream 定位，正常文件不能因此被误拒。
func TestPDFIndirectLengthOK(t *testing.T) {
	var comp bytes.Buffer
	zw := zlib.NewWriter(&comp)
	_, _ = zw.Write([]byte("BT /F1 12 Tf 72 720 Td (Indirect length stream with normal text.) Tj ET\n"))
	_ = zw.Close()
	pdf := buildPDF(
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << >> >>"),
		append(append([]byte("<< /Length 5 0 R /Filter /FlateDecode >>\nstream\n"), comp.Bytes()...), []byte("\nendstream")...),
		[]byte(fmt.Sprintf("%d", comp.Len())),
	)
	out, err := Convert(context.Background(), "indirect.pdf", pdf, Options{})
	if err != nil {
		t.Fatalf("间接 /Length 的正常 PDF 不应被拒：%v", err)
	}
	if !strings.Contains(out.Markdown, "Indirect length stream") {
		t.Fatalf("文字没抽出来：%q", out.Markdown)
	}
}
