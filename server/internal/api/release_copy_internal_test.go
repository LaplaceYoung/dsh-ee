package api

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

func TestCopyReleaseBody(t *testing.T) {
	tests := []struct {
		name     string
		wantSize int64
		content  string
		// limit > 0 时临时把 maxDshReleaseBytes 改小，用于覆盖「未知大小超上限」分支。
		limit   int64
		wantN   int64
		wantErr string
	}{
		{name: "已知大小-恰好", wantSize: 10, content: "0123456789", wantN: 10},
		{name: "已知大小-少一字节", wantSize: 10, content: "012345678", wantN: 9, wantErr: "安装包大小不一致：清单 10，实际 9"},
		// 上游内容远超清单大小时必须只写 wantSize+1 字节就停下，不能先写满盘再比大小。
		{name: "已知大小-多一字节", wantSize: 10, content: strings.Repeat("a", 1000), wantN: 11, wantErr: "上游返回的内容比清单声明的 10 字节多"},
		{name: "未知大小-未超上限", wantSize: 0, content: "0123456789", wantN: 10},
		{name: "未知大小-恰好等于上限", wantSize: 0, content: "01234567", limit: 8, wantN: 8},
		{name: "未知大小-超上限", wantSize: 0, content: strings.Repeat("a", 100), limit: 8, wantN: 9, wantErr: "上限"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.limit > 0 {
				old := maxDshReleaseBytes
				maxDshReleaseBytes = tc.limit
				t.Cleanup(func() { maxDshReleaseBytes = old })
			}
			var dst bytes.Buffer
			n, err := copyReleaseBody(&dst, strings.NewReader(tc.content), tc.wantSize)
			if n != tc.wantN {
				t.Fatalf("返回字节数 %d，期望 %d", n, tc.wantN)
			}
			if int64(dst.Len()) != tc.wantN {
				t.Fatalf("实际写入 %d 字节，期望 %d", dst.Len(), tc.wantN)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("不应报错：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误 %v，期望包含 %q", err, tc.wantErr)
			}
		})
	}
}

func TestDshReleaseClientHasTotalTimeout(t *testing.T) {
	s := New(nil, nil, Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if dshReleaseTimeout <= 0 || s.releaseHTTP.Timeout != dshReleaseTimeout {
		t.Fatalf("release 客户端必须有总超时：const=%v client=%v", dshReleaseTimeout, s.releaseHTTP.Timeout)
	}
}

// releaseTestServer 起一个返回固定内容的上游，并构造只用于下载逻辑的 Server
// （不碰数据库，因此不需要 DSH_EE_TEST_DSN）。返回的 logs 若不为 nil 会收到日志。
func releaseTestServer(t *testing.T, content []byte, logs *bytes.Buffer) (*Server, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	t.Cleanup(upstream.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if logs != nil {
		log = slog.New(slog.NewTextHandler(logs, nil))
	}
	s := New(nil, nil, Options{DataDir: t.TempDir()}, log)
	// 把该平台的官方清单地址指向上游，releaseURLAllowed 才会放行上游地址。
	s.releaseFeeds["mac-arm64"] = upstream.URL + "/feeds/mac-arm64/nightly-mac.yml"
	return s, upstream
}

func TestDshReleaseDownloadFileBounded(t *testing.T) {
	content := []byte("release bytes from upstream")
	sum := sha512.Sum512(content)

	t.Run("无哈希的DMG照旧镜像但告警", func(t *testing.T) {
		var logs bytes.Buffer
		s, upstream := releaseTestServer(t, content, &logs)
		r := &store.DshRelease{Target: "mac-arm64", Kind: "dmg", Version: "1.2.3", UpstreamURL: upstream.URL + "/desktop/dsh.dmg", File: "dsh.dmg", State: store.ReleaseAvailable}
		size, gotSum, local, err := s.downloadDshReleaseFile(context.Background(), r)
		if err != nil {
			t.Fatalf("DMG（无清单哈希）应照旧镜像：%v", err)
		}
		if size != int64(len(content)) || gotSum != base64.StdEncoding.EncodeToString(sum[:]) {
			t.Fatalf("应自行计算大小和 sha512：size=%d sum=%s", size, gotSum)
		}
		if _, err := os.Stat(local); err != nil {
			t.Fatalf("镜像文件应存在：%v", err)
		}
		if !strings.Contains(logs.String(), "安装包没有可校验的哈希") {
			t.Fatalf("跳过哈希校验时必须告警，实际日志：%s", logs.String())
		}
	})

	t.Run("上游比清单大时拒绝且不留下文件", func(t *testing.T) {
		s, upstream := releaseTestServer(t, content, nil)
		r := &store.DshRelease{Target: "mac-arm64", Kind: "zip", Version: "1.2.3", UpstreamURL: upstream.URL + "/bin/dsh.zip", File: "dsh.zip", Size: int64(len(content)) - 1, SHA512: base64.StdEncoding.EncodeToString(sum[:]), State: store.ReleaseAvailable}
		if _, _, _, err := s.downloadDshReleaseFile(context.Background(), r); err == nil || !strings.Contains(err.Error(), "比清单声明的") {
			t.Fatalf("上游比清单大时应报错：%v", err)
		}
		part, err := s.releasePartPath(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("失败后必须删除 .part：%v", err)
		}
		if _, err := os.Stat(strings.TrimSuffix(part, ".part")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("失败后不应留下正式文件：%v", err)
		}
	})
}
