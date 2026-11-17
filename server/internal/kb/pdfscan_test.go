package kb

import (
	"bytes"
	"strings"
	"testing"
)

// /Length 解析：直接写出的整数才认；/Length1（字体程序）、间接引用、超长数字都不能误认，
// 认错了就会退回找 endstream（二进制流里恰好有 "endstream" 时会定位错，预扫描就可能漏掉大流）。
func TestPDFDirectLengthParsing(t *testing.T) {
	cases := []struct {
		dict string
		want int
		ok   bool
	}{
		{`<< /Length 19473 >>`, 19473, true},
		{`<< /Length 0 >>`, 0, true},
		{`<< /Length 2048 /Filter /FlateDecode >>`, 2048, true},
		{`<< /Length1 100 /Subtype /Type1C >>`, 0, false},
		{`<< /Length2 100 /Length3 200 >>`, 0, false},
		{`<< /Length 12 0 R >>`, 0, false},
		{`<< /Filter /FlateDecode >>`, 0, false},
	}
	for _, c := range cases {
		got, ok := pdfDirectLength([]byte(c.dict))
		if got != c.want || ok != c.ok {
			t.Fatalf("%q => (%d, %v)，应为 (%d, %v)", c.dict, got, ok, c.want, c.ok)
		}
	}
}

// 有直接 /Length 时按它定位流尾：流数据里就算出现 "endstream" 也不会被截断。
func TestPDFStreamBoundsPrefersLength(t *testing.T) {
	prefix := []byte("<< /Length 14 >>\nstream\n")
	body := []byte("endstream12345") // 14 字节，开头就是 endstream
	buf := append(append(append([]byte{}, prefix...), body...), []byte("\nendstream tail")...)
	kw := bytes.Index(buf, []byte("stream"))
	dictStart, dictEnd := pdfDictBeforeStream(buf, kw, maxPDFDictScan)
	if dictEnd <= dictStart {
		t.Fatal("没找到流字典")
	}
	ds := pdfStreamDataStart(buf, kw+len("stream"))
	end, next := pdfStreamBounds(buf, ds, buf[dictStart:dictEnd])
	if end != ds+len(body) {
		t.Fatalf("end = %d，应为 %d（按 /Length 算，而不是流数据里的 endstream）", end, ds+len(body))
	}
	if !bytes.HasPrefix(buf[next:], []byte(" tail")) {
		t.Fatalf("next 应跳过流后面真正的 endstream，实际指向 %q", buf[next:])
	}
}

// 间接 /Length（/Length 5 0 R）退回 endstream 定位也要能扫，不能因此把整个文件后面的流全漏掉。
func TestPDFStreamBoundsIndirectLength(t *testing.T) {
	buf := []byte("<< /Length 5 0 R >>\nstream\nhello world\nendstream\n<< /Length 5 >>\nstream\nhello\nendstream")
	kw := bytes.Index(buf, []byte("stream"))
	dictStart, dictEnd := pdfDictBeforeStream(buf, kw, maxPDFDictScan)
	ds := pdfStreamDataStart(buf, kw+len("stream"))
	end, next := pdfStreamBounds(buf, ds, buf[dictStart:dictEnd])
	// 没有直接 /Length 时按 endstream 定位，可能把 endstream 前面那个换行也算进流数据；
	// 对解压没有影响（zlib/ASCII85 解码会忽略流尾多余字节）。
	if strings.TrimSpace(string(buf[ds:end])) != "hello world" {
		t.Fatalf("流定位错了：%q", buf[ds:end])
	}
	if next >= len(buf) {
		t.Fatalf("应该继续扫描后面的流")
	}
}
