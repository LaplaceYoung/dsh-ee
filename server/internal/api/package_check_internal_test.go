package api

import (
	"net/http/httptest"
	"testing"
)

func TestReadPackageTarballWithoutRegistry(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest("POST", "/api/me/packages/check", nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	_, _, _, ok := s.readPackageTarball(rec, req)
	if ok {
		t.Fatal("registry 为 nil 时不应继续解析")
	}
	if rec.Code != 503 {
		t.Fatalf("应返回 503，得到 %d：%s", rec.Code, rec.Body.String())
	}
}
