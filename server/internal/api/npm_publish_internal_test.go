package api

// npm publish 的内存与并发保护：请求体上限、超限 413、并发名额占满 429、失败也要释放名额。
// 正常发布路径见 registry_test.go 的 TestRegistry（需要真实数据库）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func testNpmPublishServer(maxTarball int64) *Server {
	return &Server{registry: &registry.Service{MaxTarball: maxTarball}}
}

func testNpmPublishCaller() *npmCaller {
	return &npmCaller{
		user:   &store.User{ID: 1, Username: "alice"},
		access: &rbac.Access{Perms: map[string]bool{rbac.RegistryPublish: true}},
	}
}

func testNpmPublishRequest(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/npm/@acme%2fbig", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// oversizedPublishBody 生成一个比请求体上限还长、但前缀是合法 JSON 的 publish 请求体：
// 只有上限截断发生在读完整个 base64 附件字符串之前才会有意义。
func oversizedPublishBody(limit int64) []byte {
	prefix := `{"_id":"@acme/big","name":"@acme/big","versions":{"1.0.0":{"name":"@acme/big","version":"1.0.0"}},"_attachments":{"big-1.0.0.tgz":{"content_type":"application/octet-stream","length":1,"data":"`
	suffix := `"}}}`
	n := int(limit) + 1024 - len(prefix) - len(suffix)
	if n <= 0 {
		panic("limit 太小")
	}
	var buf bytes.Buffer
	buf.Grow(len(prefix) + n + len(suffix))
	buf.WriteString(prefix)
	buf.Write(bytes.Repeat([]byte("A"), n))
	buf.WriteString(suffix)
	return buf.Bytes()
}

func TestNpmPublishRejectsOversizedBody(t *testing.T) {
	s := testNpmPublishServer(64) // 上限本身很小，正文会被 +4MB 的元数据余量兜底
	body := oversizedPublishBody(npmPublishBodyLimit(s.registry.MaxTarball))

	for _, tc := range []struct {
		name          string
		contentLength int64
	}{
		{"Content-Length 已声明超限", int64(len(body))},
		{"分块上传读满上限", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := testNpmPublishRequest(body)
			req.ContentLength = tc.contentLength
			rec := httptest.NewRecorder()
			s.handleNpmPublish(rec, req, testNpmPublishCaller(), "@acme/big")
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("超限的 publish 应返回 413，得到 %d：%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestNpmPublishConcurrencyLimit(t *testing.T) {
	old := packageUploadSlots
	packageUploadSlots = make(chan struct{}, 1)
	t.Cleanup(func() { packageUploadSlots = old })

	release, ok := beginPackageUpload()
	if !ok {
		t.Fatal("名额空闲时应能占上")
	}
	defer release()

	// npm publish 与网页上传共用名额：名额占满时直接 429，不读请求体。
	s := testNpmPublishServer(1 << 20)
	body, err := json.Marshal(map[string]any{"_id": "@acme/big", "name": "@acme/big"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleNpmPublish(rec, testNpmPublishRequest(body), testNpmPublishCaller(), "@acme/big")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("名额占满时应返回 429，得到 %d：%s", rec.Code, rec.Body.String())
	}
}

func TestNpmPublishReleasesUploadSlotOnFailure(t *testing.T) {
	old := packageUploadSlots
	packageUploadSlots = make(chan struct{}, 1)
	t.Cleanup(func() { packageUploadSlots = old })

	s := testNpmPublishServer(1 << 20)
	rec := httptest.NewRecorder()
	s.handleNpmPublish(rec, testNpmPublishRequest([]byte("{not-json")), testNpmPublishCaller(), "@acme/big")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("坏 JSON 应返回 400，得到 %d：%s", rec.Code, rec.Body.String())
	}
	// 解析失败也必须把名额还回来，否则几次坏请求之后所有上传都会 429。
	release, ok := beginPackageUpload()
	if !ok {
		t.Fatal("坏请求结束后上传名额没有释放")
	}
	release()
}
