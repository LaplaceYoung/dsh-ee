package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// minimalPDF 造一个最小的 PDF：/Root/Pages/Count 由参数决定，正文只引用一页。
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

// H-1 端到端：KB 上传 591 字节、/Root/Pages/Count=1e12 的 PDF。
// 修前 Convert 会按页循环永不返回（实测约 7µs/页 ⇒ 约 80 天），上传请求也跟着挂住；
// 修后应该毫秒级拒收「页数太多」，不建文档，正常 PDF 不受影响。
func TestKnowledgePDFPageCap(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	// 修前这个请求不会返回：给客户端设超时，让测试失败而不是挂住整个包。
	admin.hc.Timeout = 15 * time.Second
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("POST", "/api/kb", map[string]any{"slug": "dos", "title": "DoS 测试"}, nil)
	expect(t, "建库", code, 201, body)
	kbID := decodeAs[struct{ ID int64 }](t, body).ID
	docs := "/api/kb/" + itoa(kbID) + "/docs"

	// 正常 PDF 仍然能上传：页数上限不能误杀正常文档。
	code, body = admin.upload(docs, nil, map[string][]byte{"正常.pdf": minimalPDF(2)})
	expect(t, "正常 PDF 上传", code, 201, body)
	if strings.Contains(string(body), "页数太多") {
		t.Fatalf("正常 PDF 被误杀：%s", body)
	}

	start := time.Now()
	code, body = admin.upload(docs, nil, map[string][]byte{"poc.pdf": minimalPDF(1_000_000_000_000)})
	elapsed := time.Since(start)
	// 唯一一个文件转换失败时接口回 400，错误在 results[].error 里。
	expect(t, "页数超限的 PDF 上传", code, 400, body)
	if !strings.Contains(string(body), "页数太多") {
		t.Fatalf("应返回「页数太多」：%s", body)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("超限 PDF 应快速拒收，实际 %v", elapsed)
	}
	code, body = admin.do("GET", "/api/kb/"+itoa(kbID), nil, nil)
	expect(t, "读库", code, 200, body)
	if n := decodeAs[struct {
		KB struct{ Docs int } `json:"kb"`
	}](t, body).KB.Docs; n != 1 {
		t.Fatalf("超限文件不应建文档：库里有 %d 篇", n)
	}
}
