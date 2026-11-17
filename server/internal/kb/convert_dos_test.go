package kb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// minimalPDF 造一个最小的 PDF：/Root/Pages/Count 由参数决定，正文只引用一页。
// 页数完全由文件里的这个数字控制，跟文件里实际有多少页无关——审计实验里 585 字节的文件写 1e12。
func minimalPDF(count int64) []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [3 0 R] /Count %d >>", count),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"<< /Length 44 >>\nstream\nBT /F1 12 Tf 72 720 Td (Hello) Tj ET\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs)+1)
	for i, o := range objs {
		offs[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objs)+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offs[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// convertWithin 等 Convert 在 d 内返回，超时判失败。
// 防的是「修前复现」变成真的跑几十天：如果页数上限哪天被去掉，测试会在几秒内失败，而不是挂住 CI。
func convertWithin(t *testing.T, d time.Duration, ctx context.Context, name string, data []byte, opt Options) (*Converted, error, time.Duration) {
	t.Helper()
	type result struct {
		out *Converted
		err error
	}
	ch := make(chan result, 1)
	start := time.Now()
	go func() {
		out, err := Convert(ctx, name, data, opt)
		ch <- result{out: out, err: err}
	}()
	select {
	case r := <-ch:
		return r.out, r.err, time.Since(start)
	case <-time.After(d):
		t.Fatalf("%s 在 %v 内没有返回：转换可能又变回无限/超长循环了", name, d)
		return nil, nil, 0
	}
}

// H-1：PDF 页数来自文件里的 /Root/Pages/Count。
// 修前：585 字节的文件写 1e12 页，Convert 按页循环、永不结束（实测约 7µs/页 ⇒ 1e12 页约 80 天，
// int64 上限约 290 万年），期间客户端断开也不会停。
// 修后：毫秒级返回「页数太多」，整份拒收（不截断，免得维护者以为文档收全了）。
func TestPDFPageCountCap(t *testing.T) {
	data := minimalPDF(1_000_000_000_000)
	if len(data) > 1024 {
		t.Fatalf("复现文件应该只有几百字节，实际 %d 字节", len(data))
	}
	out, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "poc.pdf", data, Options{})
	if out != nil {
		t.Fatalf("超限的 PDF 应该整份拒收，不能截断后返回正文")
	}
	if err == nil || !strings.Contains(err.Error(), "页数太多") || !strings.Contains(err.Error(), "2000") {
		t.Fatalf("应报「页数太多」并带上限：%v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("超限 PDF 应快速拒收，实际 %v", elapsed)
	}
}

// 上限边界：刚好 maxPDFPages 页照常转（不能误杀正常文档），多一页才拒收。
func TestPDFPageCountCapBoundary(t *testing.T) {
	out, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "at-cap.pdf", minimalPDF(maxPDFPages), Options{})
	if err != nil {
		t.Fatalf("上限之内的 PDF 不应该被拒：%v", err)
	}
	if out.Info.Format != "pdf" || out.Info.Pages != maxPDFPages {
		t.Fatalf("体检 = %+v", out.Info)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("上限之内的 PDF 转得太慢：%v", elapsed)
	}

	if _, err, _ := convertWithin(t, 5*time.Second, context.Background(), "over-cap.pdf", minimalPDF(maxPDFPages+1), Options{}); err == nil || !strings.Contains(err.Error(), "页数太多") {
		t.Fatalf("超过上限一页应拒收：%v", err)
	}
}

// M-1：同一页被 sldIdLst 引用很多次。修前每次引用都重新解压、再建一遍 XML 树：
// 4.5 KB 的文件（40 万个空节点 × 引用 200 次）实测 19.2 秒，引用 5000 次约 8 分钟。
// 修后同一页只解析一次，但「节点数 × 引用次数」超过预算时整份拒收。
func TestPPTXRepeatedSlideBudget(t *testing.T) {
	slide := slideOf(strings.Repeat(`<a:x/>`, 100_000))
	data := pptxOf(t, 200, slide)
	if len(data) > 8<<10 {
		t.Fatalf("复现文件应该只有几 KB，实际 %d 字节", len(data))
	}
	_, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "amp.pptx", data, Options{})
	if err == nil || !strings.Contains(err.Error(), "内容太多") {
		t.Fatalf("超出「节点数 × 引用次数」预算应拒收：%v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("预算超限应快速拒收，实际 %v", elapsed)
	}
}

// 去重后同一页引用很多次不应重复解析/排版：页数、正文仍按引用次数输出，但很快转完。
func TestPPTXRepeatedSlideCached(t *testing.T) {
	slide := slideOf(strings.Repeat(`<a:x/>`, 1000))
	data := pptxOf(t, 300, slide)
	out, err, elapsed := convertWithin(t, 5*time.Second, context.Background(), "cached.pptx", data, Options{})
	if err != nil {
		t.Fatalf("同一页引用 300 次应该能转：%v", err)
	}
	if out.Info.Pages != 300 || strings.Count(out.Markdown, "## 第 ") != 300 {
		t.Fatalf("页数 = %+v，正文里 %d 节", out.Info, strings.Count(out.Markdown, "## 第 "))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("去重后 300 次引用太慢：%v", elapsed)
	}
}

// 重复引用时体检计数保持原语义：表格按引用次数计（修前也是这样），不是按去重后的不同页计。
func TestPPTXRepeatedSlideInfoCounters(t *testing.T) {
	tbl := `<p:graphicFrame><a:graphic><a:graphicData><a:tbl><a:tr><a:tc><a:txBody><a:p><a:r><a:t>格</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	data := pptxOf(t, 3, slideOf(tbl))
	out, err, _ := convertWithin(t, 5*time.Second, context.Background(), "table.pptx", data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Info.Pages != 3 || out.Info.Tables != 3 || strings.Count(out.Markdown, "| 格 |") != 3 {
		t.Fatalf("重复引用的体检/正文 = %+v，%d 张表", out.Info, strings.Count(out.Markdown, "| 格 |"))
	}
}

// ctx 已经在入口取消：立刻返回，不做解析。
func TestConvertContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Convert(ctx, "a.pdf", minimalPDF(3), Options{})
	if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "转换") {
		t.Fatalf("取消后应返回可识别的中文错误：%v", err)
	}
}

// 上层设的转换超时到了：返回能 errors.Is(context.DeadlineExceeded) 的错误。
func TestConvertContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // 确保 deadline 已过
	_, err := Convert(ctx, "a.pdf", minimalPDF(3), Options{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时后应返回 deadline exceeded：%v", err)
	}
}

// countingCancelCtx 在第 cancelAfter 次 Err() 检查之后开始返回取消，
// 这样「循环中途取消」的测试是确定性的，不依赖 sleep/goroutine 的时序。
type countingCancelCtx struct {
	context.Context
	checks      int32
	cancelAfter int32
}

func (c *countingCancelCtx) Err() error {
	if atomic.AddInt32(&c.checks, 1) > c.cancelAfter {
		return context.Canceled
	}
	return nil
}

// 转换循环中途取消：PDF 在上限之内会按页循环，第 3 次检查（第 2 页左右）取消，应该很快停下。
func TestConvertContextCanceledMidLoop(t *testing.T) {
	ctx := &countingCancelCtx{Context: context.Background(), cancelAfter: 3}
	start := time.Now()
	_, err := Convert(ctx, "a.pdf", minimalPDF(maxPDFPages), Options{})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("循环中途取消应返回 context.Canceled：%v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("取消后转换停了太久：%v", elapsed)
	}
}
