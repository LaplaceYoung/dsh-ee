package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/registry"
)

// testTgz 打一个最小的 npm pack 格式 tarball，给上传/预检的边界测试用。
func testTgz(t *testing.T, name, version string) []byte {
	t.Helper()
	pj, err := json.Marshal(map[string]string{"name": name, "version": version})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(pj)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(pj); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testUploadRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/me/packages/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	return req
}

func testUploadServer(maxTarball int64) *Server {
	return &Server{registry: &registry.Service{MaxTarball: maxTarball}}
}

func TestReadPackageTarballExactContentLength(t *testing.T) {
	tgz := testTgz(t, "@company/exact-size", "1.0.0")
	s := testUploadServer(1 << 20)
	for name, chunked := range map[string]bool{"有 Content-Length": false, "分块上传": true} {
		t.Run(name, func(t *testing.T) {
			req := testUploadRequest(t, tgz)
			if chunked {
				req.ContentLength = -1
			}
			rec := httptest.NewRecorder()
			raw, info, m, ok := s.readPackageTarball(rec, req)
			if !ok {
				t.Fatalf("读取失败：%d %s", rec.Code, rec.Body.String())
			}
			if !bytes.Equal(raw, tgz) {
				t.Fatalf("读到的 tarball 与上传内容不一致：%d != %d", len(raw), len(tgz))
			}
			if info.PackageJSON == nil {
				t.Fatal("没有解析出 package.json")
			}
			if m.Name != "@company/exact-size" || m.Version != "1.0.0" {
				t.Fatalf("清单解析不对：%s@%s", m.Name, m.Version)
			}
		})
	}
}

func TestPackageUploadTooLarge(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 2048)
	t.Run("声明的 Content-Length 超限", func(t *testing.T) {
		s := testUploadServer(1024)
		rec := httptest.NewRecorder()
		if _, _, _, ok := s.readPackageTarball(rec, testUploadRequest(t, body)); ok {
			t.Fatal("超限的请求不应继续解析")
		}
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("应返回 413，得到 %d：%s", rec.Code, rec.Body.String())
		}
		rec = httptest.NewRecorder()
		if _, _, _, ok := s.inspectPackageTarball(rec, testUploadRequest(t, body)); ok {
			t.Fatal("超限的预检请求不应继续解析")
		}
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("预检应返回 413，得到 %d：%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("分块上传按实际字节数超限", func(t *testing.T) {
		s := testUploadServer(1024)
		tgz := testTgz(t, "@company/chunked", "1.0.0")
		// gzip 流本身很小，后面跟着超过上限的尾随数据：解包完把剩余读完时才会发现超限。
		big := append(bytes.Clone(tgz), bytes.Repeat([]byte("x"), 2048)...)
		req := testUploadRequest(t, big)
		req.ContentLength = -1
		rec := httptest.NewRecorder()
		if _, _, _, ok := s.inspectPackageTarball(rec, req); ok {
			t.Fatal("超限的分块预检不应继续解析")
		}
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("应返回 413，得到 %d：%s", rec.Code, rec.Body.String())
		}
	})
}

func TestInspectPackageTarballCountsTrailingBytes(t *testing.T) {
	tgz := testTgz(t, "@company/trailing", "1.0.1")
	body := append(bytes.Clone(tgz), []byte("trailing-data")...)
	s := testUploadServer(1 << 20)
	rec := httptest.NewRecorder()
	size, info, m, ok := s.inspectPackageTarball(rec, testUploadRequest(t, body))
	if !ok {
		t.Fatalf("预检失败：%d %s", rec.Code, rec.Body.String())
	}
	if size != int64(len(body)) {
		t.Fatalf("预检拿到的大小应为 %d（含尾随数据），得到 %d", len(body), size)
	}
	if info.PackageJSON == nil || m.Name != "@company/trailing" || m.Version != "1.0.1" {
		t.Fatalf("清单解析不对：%+v", m)
	}
}

func TestPackageUploadConcurrencyLimit(t *testing.T) {
	old := packageUploadSlots
	packageUploadSlots = make(chan struct{}, 1)
	t.Cleanup(func() { packageUploadSlots = old })

	release, ok := beginPackageUpload()
	if !ok {
		t.Fatal("名额空闲时应能占上")
	}
	if _, ok := beginPackageUpload(); ok {
		t.Fatal("名额占满后不应再占上")
	}

	// 名额占满时，上传入口直接回 429，不再读请求体。
	s := testUploadServer(1 << 20)
	rec := httptest.NewRecorder()
	s.handleUploadPackage(rec, testUploadRequest(t, testTgz(t, "@company/busy", "1.0.0")))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("名额占满时应返回 429，得到 %d：%s", rec.Code, rec.Body.String())
	}

	release()
	releaseAgain, ok := beginPackageUpload()
	if !ok {
		t.Fatal("release 之后应能重新占上名额")
	}
	releaseAgain()
}

// testTgzMultiTop 打一个「干净 package/ + 带 postinstall 的 hidden/」的 tarball：
// npm 安装时 strip:1 会把 hidden/ 平铺到安装根，审核必须先拒收。
func testTgzMultiTop(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("package/package.json", `{"name":"@company/x","version":"1.0.0"}`)
	write("hidden/package.json", `{"name":"@company/x","version":"1.0.0","scripts":{"postinstall":"node pwn.js"}}`)
	write("hidden/pwn.js", "require('child_process').execFileSync('id');\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInspectPackageTarballRejectsMultipleTopDirs(t *testing.T) {
	tgz := testTgzMultiTop(t)
	s := testUploadServer(1 << 20)

	rec := httptest.NewRecorder()
	if _, _, _, ok := s.inspectPackageTarball(rec, testUploadRequest(t, tgz)); ok {
		t.Fatal("预检不该接受多顶层目录的 tarball")
	}
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("顶层目录")) {
		t.Fatalf("预检应返回 400 并说明顶层目录问题，得到 %d：%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	if _, _, _, ok := s.readPackageTarball(rec, testUploadRequest(t, tgz)); ok {
		t.Fatal("发布路径不该接受多顶层目录的 tarball")
	}
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("顶层目录")) {
		t.Fatalf("发布应返回 400 并说明顶层目录问题，得到 %d：%s", rec.Code, rec.Body.String())
	}
}
