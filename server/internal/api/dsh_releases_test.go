package api

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

type releaseFixture struct {
	s            *Server
	st           *store.Store
	upstream     *httptest.Server
	content      []byte
	manifestSHA  string
	manifestSize int64
	downloads    atomic.Int32
}

func newReleaseFixture(t *testing.T, mode string) *releaseFixture {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	name := "dsh_ee_test_" + hex.EncodeToString(suffix)
	st, err := store.Open(context.Background(), strings.Replace(base, ")/", ")/"+name, 1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB().ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		_ = st.Close()
	})
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	f := &releaseFixture{st: st, content: []byte("signed desktop zip")}
	fullSum := sha512.Sum512(f.content)
	f.manifestSHA = base64.StdEncoding.EncodeToString(fullSum[:])
	f.manifestSize = int64(len(f.content))
	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 每个平台一个 feed：macOS 是 nightly-mac.yml，Windows 是 nightly.yml。
		if rest, ok := strings.CutPrefix(r.URL.Path, "/feeds/"); ok {
			target, _, ok := strings.Cut(rest, "/")
			if !ok {
				http.NotFound(w, r)
				return
			}
			asset := "/bin/dsh.zip"
			if target == "win-x64" {
				asset = "/bin/dsh-win.exe"
			}
			w.Header().Set("Content-Type", "text/yaml")
			_, _ = fmt.Fprintf(w, "version: 1.2.3\nfiles:\n  - url: '%s%s'\n    sha512: '%s'\n    size: %d\nreleaseDate: '2026-11-02T12:00:00Z'\n", f.upstream.URL, asset, f.manifestSHA, f.manifestSize)
			return
		}
		switch r.URL.Path {
		case "/bin/dsh.zip", "/bin/dsh-win.exe", "/desktop/dsh.dmg":
			f.downloads.Add(1)
			_, _ = w.Write(f.content)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.upstream.Close)
	s := New(st, nil, Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for target := range s.releaseFeeds {
		feed := "nightly-mac.yml"
		if target == "win-x64" {
			feed = "nightly.yml"
		}
		s.releaseFeeds[target] = f.upstream.URL + "/feeds/" + target + "/" + feed
		// 只有 macOS 有官方 DMG，测试里也只给它准备下载地址
		if s.releaseDMGs[target] != "" {
			s.releaseDMGs[target] = f.upstream.URL + "/desktop/dsh.dmg"
		}
	}
	set := store.DefaultSettings()
	set.Distribution.AutoSync = mode
	if err := st.SaveSettings(context.Background(), set, 0); err != nil {
		t.Fatal(err)
	}
	f.s = s
	return f
}

func (f *releaseFixture) setMode(t *testing.T, mode string, offerDmg bool) {
	t.Helper()
	set, err := f.st.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	set.Distribution.AutoSync = mode
	set.Distribution.OfferDmg = offerDmg
	if err := f.st.SaveSettings(context.Background(), set, 0); err != nil {
		t.Fatal(err)
	}
	f.s.settings.v = nil
}

func TestDshReleaseNotifyDoesNotDownload(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	if err := f.s.syncDshReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.downloads.Load() != 0 {
		t.Fatalf("notify 模式不应下载安装包：%d", f.downloads.Load())
	}
	items, err := f.st.ListDshReleases(context.Background(), "")
	if err != nil || len(items) != 3 {
		t.Fatalf("应记录三个平台的 target：%d %v", len(items), err)
	}
	kinds := map[string]string{}
	for _, r := range items {
		if r.State != store.ReleaseAvailable || r.SHA512 == "" {
			t.Fatalf("清单记录不对：%+v", r)
		}
		kinds[r.Target] = r.Kind
	}
	if kinds["mac-arm64"] != "zip" || kinds["mac-x64"] != "zip" || kinds["win-x64"] != "exe" {
		t.Fatalf("安装包类型应按后缀区分：%v", kinds)
	}
}

func TestDshReleaseMirrorAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		badHash  bool
		badSize  bool
		wantFail bool
	}{
		{name: "成功"},
		{name: "哈希不一致", badHash: true, wantFail: true},
		{name: "大小不一致", badSize: true, wantFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReleaseFixture(t, "mirror")
			if tc.badHash {
				other := sha512.Sum512([]byte("other"))
				f.manifestSHA = base64.StdEncoding.EncodeToString(other[:])
			}
			if tc.badSize {
				f.manifestSize++
			}
			err := f.s.syncDshReleases(context.Background())
			if tc.wantFail && err == nil {
				t.Fatal("校验不一致应返回错误")
			}
			if !tc.wantFail && err != nil {
				t.Fatal(err)
			}
			items, listErr := f.st.ListDshReleases(context.Background(), "mac-arm64")
			if listErr != nil || len(items) != 1 {
				t.Fatalf("版本记录：%v %v", items, listErr)
			}
			r := items[0]
			if tc.wantFail {
				if r.State != store.ReleaseFailed || r.Error == "" {
					t.Fatalf("失败后状态：%+v", r)
				}
				part, _ := f.s.releasePartPath(r)
				if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("失败后应删除 .part：%v", err)
				}
				return
			}
			if r.State != store.ReleaseMirrored || r.LocalPath == "" || r.SHA512 != f.manifestSHA {
				t.Fatalf("镜像状态：%+v", r)
			}
			got, err := os.ReadFile(r.LocalPath)
			if err != nil || string(got) != string(f.content) {
				t.Fatalf("镜像内容：%q %v", got, err)
			}
		})
	}
}

func TestDshReleaseRejectsCrossHostRedirect(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "version: 1.2.3\n")
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/feed", http.StatusFound)
	}))
	defer redirect.Close()
	f.s.releaseFeeds["mac-arm64"] = redirect.URL + "/feed"
	_, err := f.s.fetchDshManifest(context.Background(), "mac-arm64")
	if err == nil || !strings.Contains(err.Error(), "拒绝重定向") {
		t.Fatalf("跨 host 重定向应被拒绝：%v", err)
	}
}

func TestDshReleaseRecover(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	r := &store.DshRelease{Target: "mac-arm64", Kind: "zip", Version: "1.2.3", ReleasedAt: time.Now(), File: "dsh.zip", Size: 10, SHA512: "aGFzaA==", UpstreamURL: f.upstream.URL + "/bin/dsh.zip", State: store.ReleaseDownloading}
	if err := f.st.CreateDshRelease(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	part, _ := f.s.releasePartPath(r)
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.s.recoverDshReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := f.st.DshReleaseByID(context.Background(), r.ID)
	if err != nil || got.State != store.ReleaseAvailable {
		t.Fatalf("重启后应恢复 available：%+v %v", got, err)
	}
	if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("重启后应删除 .part：%v", err)
	}
}

func TestDshReleaseCleanupKeepsRecommended(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	var oldest *store.DshRelease
	for i := 1; i <= 4; i++ {
		r := &store.DshRelease{Target: "mac-arm64", Kind: "zip", Version: fmt.Sprintf("1.0.%d", i), ReleasedAt: time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC), File: fmt.Sprintf("dsh-%d.zip", i), Size: 1, SHA512: "aGFzaA==", UpstreamURL: f.upstream.URL + "/bin/dsh.zip", State: store.ReleaseMirrored}
		r.LocalPath = filepath.Join(f.s.opts.DataDir, r.File)
		if err := os.WriteFile(r.LocalPath, []byte{byte(i)}, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := f.st.CreateDshRelease(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			oldest = r
		}
	}
	if err := f.st.SetDshReleaseRecommended(context.Background(), oldest.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.cleanupDshReleases(context.Background(), "mac-arm64", 2); err != nil {
		t.Fatal(err)
	}
	items, err := f.st.ListDshReleases(context.Background(), "mac-arm64")
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, r := range items {
		states[r.Version] = r.State
	}
	if states["1.0.1"] != store.ReleaseMirrored || states["1.0.3"] != store.ReleaseMirrored || states["1.0.4"] != store.ReleaseMirrored || states["1.0.2"] != store.ReleaseRemoved {
		t.Fatalf("清理结果：%v", states)
	}
	if _, err := os.Stat(oldest.LocalPath); err != nil {
		t.Fatalf("推荐版本文件必须保留：%v", err)
	}
}

func TestDshReleaseDMGHash(t *testing.T) {
	f := newReleaseFixture(t, "mirror")
	f.setMode(t, "mirror", true)
	if err := f.s.syncDshReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := f.st.DshReleaseByVersion(context.Background(), "mac-arm64", "dmg", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if r.State != store.ReleaseMirrored || r.Size != int64(len(f.content)) || r.SHA512 != f.manifestSHA {
		t.Fatalf("DMG 应自行计算大小和 sha512：%+v", r)
	}
}

func TestDshReleaseWindowsHasNoDmg(t *testing.T) {
	f := newReleaseFixture(t, "mirror")
	f.setMode(t, "mirror", true)
	if err := f.s.syncDshReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := f.st.ListDshReleases(context.Background(), "win-x64")
	if err != nil || len(items) != 1 {
		t.Fatalf("Windows 只应有一个安装包（没有 DMG）：%v %v", items, err)
	}
	r := items[0]
	if r.Kind != "exe" || r.State != store.ReleaseMirrored || r.SHA512 != f.manifestSHA {
		t.Fatalf("Windows 安装包：%+v", r)
	}
	// 来自官方清单的安装包（zip / exe）都算已校验，只有 DMG 是例外
	if ok, _ := publicDshRelease(r, "http://ee")["verified"].(bool); !ok {
		t.Fatal("exe 的哈希来自官方清单，应标记为已验证")
	}
}

func TestDshReleaseMirrorDoesNotRedownloadRemoved(t *testing.T) {
	f := newReleaseFixture(t, "mirror")
	ctx := context.Background()
	if err := f.s.syncDshReleases(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := f.st.DshReleaseByVersion(ctx, "mac-arm64", "zip", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ResetDshRelease(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	before := f.downloads.Load()
	if err := f.s.syncDshReleases(ctx); err != nil {
		t.Fatal(err)
	}
	if f.downloads.Load() != before {
		t.Fatalf("被移除的版本不应被自动镜像重新下载：%d -> %d", before, f.downloads.Load())
	}
	got, _ := f.st.DshReleaseByID(ctx, r.ID)
	if got.State != store.ReleaseRemoved {
		t.Fatalf("状态应保持 removed：%s", got.State)
	}
	// 管理员仍可手动重新下载。
	if err := f.s.downloadDshRelease(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = f.st.DshReleaseByID(ctx, r.ID)
	if got.State != store.ReleaseMirrored {
		t.Fatalf("手动下载后应为 mirrored：%s", got.State)
	}
}

func TestDshReleaseOversizeUpstreamFails(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	ctx := context.Background()
	// 清单/DB 声明的大小比上游实际返回的小 1 字节：必须在下完之前就拒绝，并把状态置为 failed。
	r := &store.DshRelease{
		Target: "mac-arm64", Kind: "zip", Version: "9.9.9", ReleasedAt: time.Now(), File: "dsh-9.9.9.zip",
		Size: f.manifestSize - 1, SHA512: f.manifestSHA,
		UpstreamURL: f.upstream.URL + "/bin/dsh.zip", State: store.ReleaseAvailable,
	}
	if err := f.st.CreateDshRelease(ctx, r); err != nil {
		t.Fatal(err)
	}
	err := f.s.downloadDshRelease(ctx, r.ID)
	if err == nil || !strings.Contains(err.Error(), "比清单声明的") {
		t.Fatalf("上游比清单大时应下载失败：%v", err)
	}
	got, err := f.st.DshReleaseByID(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != store.ReleaseFailed || !strings.Contains(got.Error, "比清单声明的") {
		t.Fatalf("下载失败后状态应为 failed 且记录原因：%+v", got)
	}
	part, err := f.s.releasePartPath(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失败后应删除 .part：%v", err)
	}
}

func TestDshReleaseDMGVersionedFileAndSeparateCleanup(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	m := &dshManifest{Version: "1.2.3", ReleaseDate: "2026-11-02T12:00:00Z"}
	m.Files = append(m.Files, struct {
		URL    string `yaml:"url"`
		SHA512 string `yaml:"sha512"`
		Size   int64  `yaml:"size"`
	}{f.upstream.URL + "/bin/dsh.zip", f.manifestSHA, f.manifestSize})
	a, err := f.s.dmgFromManifest("mac-arm64", m)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = "1.2.4"
	b, err := f.s.dmgFromManifest("mac-arm64", m)
	if err != nil {
		t.Fatal(err)
	}
	if a.File == b.File || !strings.Contains(a.File, "1.2.3") {
		t.Fatalf("DMG 本地文件名应带版本：%s %s", a.File, b.File)
	}
	// 1 个 zip + 2 个 dmg，各自保留 1 个：zip 不应被 dmg 挤掉。
	ctx := context.Background()
	mk := func(kind, version, file string, day int) {
		r := &store.DshRelease{Target: "mac-arm64", Kind: kind, Version: version, ReleasedAt: time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC), File: file, Size: 1, SHA512: "aGFzaA==", UpstreamURL: f.upstream.URL, State: store.ReleaseMirrored}
		r.LocalPath = filepath.Join(f.s.opts.DataDir, file)
		_ = os.WriteFile(r.LocalPath, []byte{1}, 0o600)
		if err := f.st.CreateDshRelease(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	mk("zip", "1.0.0", "a.zip", 1)
	mk("dmg", "1.0.1", "b.dmg", 2)
	mk("dmg", "1.0.2", "c.dmg", 3)
	if err := f.s.cleanupDshReleases(ctx, "mac-arm64", 1); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ kind, ver, want string }{{"zip", "1.0.0", store.ReleaseMirrored}, {"dmg", "1.0.1", store.ReleaseRemoved}, {"dmg", "1.0.2", store.ReleaseMirrored}} {
		r, _ := f.st.DshReleaseByVersion(ctx, "mac-arm64", v.kind, v.ver)
		if r.State != v.want {
			t.Errorf("%s %s 状态 %s，期望 %s", v.kind, v.ver, r.State, v.want)
		}
	}
}
