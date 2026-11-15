package registry

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// ---- 造离线包用的辅助函数 ----

// newDBBundleService 造一个连着测试库的 Service；没有 DSH_EE_TEST_DSN 时由
// openRegistryTestStore 跳过。
func newDBBundleService(t *testing.T) *Service {
	t.Helper()
	st := openRegistryTestStore(t)
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st, blobs, nil, st.GetSettings, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// newOfflineBundleService 造一个不连数据库的 Service：规模上限在写库之前就会被拒绝，
// 测这些上限不需要 DSN。
func newOfflineBundleService(t *testing.T) *Service {
	t.Helper()
	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(nil, blobs, nil, func(context.Context) (*store.AppSettings, error) {
		return store.DefaultSettings(), nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// rawBundleVersion 造一个版本条目：blob 内容由调用方给，条目上的包名/版本由
// declaredName/declaredVersion 给，两者可以不同（用来构造被篡改的摆渡包）。
func rawBundleVersion(t *testing.T, declaredName, declaredVersion string, raw []byte) BundleVersion {
	t.Helper()
	sum := sha512.Sum512(raw)
	shasum := sha1.Sum(raw)
	manifest, err := json.Marshal(map[string]string{"name": declaredName, "version": declaredVersion})
	if err != nil {
		t.Fatal(err)
	}
	return BundleVersion{
		Version:   declaredVersion,
		Manifest:  manifest,
		Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:]),
		Shasum:    hex.EncodeToString(shasum[:]),
		Size:      int64(len(raw)),
		Blob:      hex.EncodeToString(sum[:]),
		CreatedAt: time.Now().UTC(),
	}
}

// bundleVersionEntry 造一个版本条目和对应的 tarball。contentJSON 是 tarball 里真实的
// package.json。
func bundleVersionEntry(t *testing.T, declaredName, declaredVersion, contentJSON string) (BundleVersion, []byte) {
	t.Helper()
	if !json.Valid([]byte(contentJSON)) {
		t.Fatalf("测试包的 package.json 不合法：%s", contentJSON)
	}
	raw := tgz(t, map[string]string{"package/package.json": contentJSON})
	return rawBundleVersion(t, declaredName, declaredVersion, raw), raw
}

// bundleManifestFor 造一个格式合法的清单。
func bundleManifestFor(pkgs ...BundlePackage) BundleManifest {
	return BundleManifest{Format: 1, CreatedAt: time.Now().UTC(), Source: "unit-test", Packages: pkgs}
}

// bundleTar 把清单和 tarball 打成离线包：第一项是 dsh-ee-bundle.json，之后是
// blobs/<sha512>。
func bundleTar(t *testing.T, man BundleManifest, tarballs map[string][]byte) []byte {
	t.Helper()
	body, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	return bundleTarRaw(t, body, tarballs)
}

func bundleTarRaw(t *testing.T, manifest []byte, tarballs map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write := func(name string, body []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	write(bundleManifestName, manifest)
	keys := make([]string, 0, len(tarballs))
	for key := range tarballs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		write("blobs/"+key, tarballs[key])
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// wantBundleError 断言导入被拒绝，并且错误信息里能看出是哪一项超限 / 哪个包不对。
func wantBundleError(t *testing.T, err error, subs ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("应该拒绝导入，却成功了")
	}
	for _, sub := range subs {
		if !strings.Contains(err.Error(), sub) {
			t.Fatalf("错误信息 %q 应包含 %q", err.Error(), sub)
		}
	}
}

// ---- 内容交叉核对（需要数据库） ----

func TestBundleImportContent(t *testing.T) {
	svc := newDBBundleService(t)
	ctx := context.Background()

	t.Run("清单与内容一致时成功入库", func(t *testing.T) {
		entry, raw := bundleVersionEntry(t, "bundle-ok-a", "1.0.0", `{"name":"bundle-ok-a","version":"1.0.0","license":"MIT"}`)
		man := bundleManifestFor(BundlePackage{Name: "bundle-ok-a", DistTags: map[string]string{"latest": "1.0.0"}, Versions: []BundleVersion{entry}})

		res, j, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, map[string][]byte{entry.Blob: raw})), 0)
		if err != nil {
			t.Fatalf("导入应成功：%v", err)
		}
		if res.Added != 1 || res.Existing != 0 || res.Pending != 0 {
			t.Fatalf("导入结果不对：%+v", res)
		}
		if j == nil || j.Status != "succeeded" {
			t.Fatalf("任务状态应为 succeeded：%+v", j)
		}
		v, err := svc.store.GetVersion(ctx, "bundle-ok-a", "1.0.0")
		if err != nil {
			t.Fatalf("版本应已入库：%v", err)
		}
		if v.Status != store.StatusPublished || v.IsPlugin {
			t.Fatalf("普通包应直接上架且不是插件：%+v", v)
		}
		if v.BlobKey != entry.Blob || v.Size != int64(len(raw)) {
			t.Fatalf("blob 或大小不对：key=%s size=%d（期望 %s / %d）", v.BlobKey, v.Size, entry.Blob, len(raw))
		}
		p, err := svc.store.GetPackage(ctx, "bundle-ok-a")
		if err != nil {
			t.Fatalf("包应已入库：%v", err)
		}
		if p.IsPlugin {
			t.Fatalf("普通包不该被标成插件：%+v", p)
		}
	})

	t.Run("清单与内容不一致时整体拒绝且不留数据", func(t *testing.T) {
		// 清单说 bundle-liar-a@1.0.0，tarball 里的 package.json 却是 bundle-swapped-b@9.9.9。
		entry, raw := bundleVersionEntry(t, "bundle-liar-a", "1.0.0", `{"name":"bundle-swapped-b","version":"9.9.9"}`)
		man := bundleManifestFor(BundlePackage{Name: "bundle-liar-a", Versions: []BundleVersion{entry}})

		res, j, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, map[string][]byte{entry.Blob: raw})), 0)
		if res != nil {
			t.Fatalf("被拒绝的导入不该有结果：%+v", res)
		}
		wantBundleError(t, err, "bundle-liar-a@1.0.0", "bundle-swapped-b@9.9.9")
		wantErrStatus(t, err, 400)
		if j == nil || j.Status != "failed" {
			t.Fatalf("任务状态应为 failed：%+v", j)
		}
		// 交叉核对在写库之前，被拒绝的导入不能留下包或版本。
		if _, err := svc.store.GetPackage(ctx, "bundle-liar-a"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("被拒绝的导入不应创建包：%v", err)
		}
		if _, err := svc.store.GetVersion(ctx, "bundle-liar-a", "1.0.0"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("被拒绝的导入不应创建版本：%v", err)
		}
	})

	t.Run("tarball 无法解析时整体拒绝", func(t *testing.T) {
		cases := []struct {
			name string
			raw  []byte
		}{
			{"不是 gzip", []byte("definitely not a gzip tarball")},
			{"没有 package.json", tgz(t, map[string]string{"package/index.js": "console.log(1)"})},
		}
		for i, tc := range cases {
			name := fmt.Sprintf("bundle-broken-%d", i)
			entry := rawBundleVersion(t, name, "1.0.0", tc.raw)
			man := bundleManifestFor(BundlePackage{Name: name, Versions: []BundleVersion{entry}})
			_, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, map[string][]byte{entry.Blob: tc.raw})), 0)
			wantBundleError(t, err, name+"@1.0.0", "拒绝导入")
			if _, err := svc.store.GetVersion(ctx, name, "1.0.0"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("%s：被拒绝的导入不应创建版本：%v", tc.name, err)
			}
		}
	})

	t.Run("is_plugin 以 tarball 里的 package.json 为准", func(t *testing.T) {
		// 清单把 is_plugin 标成 false，tarball 里却声明了 dsh.bundle：必须按真实内容
		// 判定为插件，白名单模式下进入待审核（默认设置 AdmissionMode=allowlist）。
		entry, raw := bundleVersionEntry(t, "bundle-lying-plugin", "1.0.0",
			`{"name":"bundle-lying-plugin","version":"1.0.0","license":"MIT","dsh":{"bundle":{"patch":"cordis.patch.yml"}}}`)
		entry.IsPlugin = false
		entry.HasInstallScript = false
		man := bundleManifestFor(BundlePackage{Name: "bundle-lying-plugin", IsPlugin: false, Versions: []BundleVersion{entry}})

		res, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, map[string][]byte{entry.Blob: raw})), 0)
		if err != nil {
			t.Fatalf("导入应成功（内容合法，只是清单标错了）：%v", err)
		}
		if res.Added != 1 || res.Pending != 1 {
			t.Fatalf("插件版本应进入待审核：%+v", res)
		}
		v, err := svc.store.GetVersion(ctx, "bundle-lying-plugin", "1.0.0")
		if err != nil {
			t.Fatalf("版本应已入库：%v", err)
		}
		if !v.IsPlugin {
			t.Fatalf("IsPlugin 应取自 tarball 里的 package.json：%+v", v)
		}
		if v.Status != store.StatusPending {
			t.Fatalf("白名单模式下插件应待审核，实际 %s", v.Status)
		}
		if len(v.Scan) == 0 {
			t.Fatal("插件版本应带自动检查结果（Scan）")
		}
		p, err := svc.store.GetPackage(ctx, "bundle-lying-plugin")
		if err != nil {
			t.Fatalf("包应已入库：%v", err)
		}
		if !p.IsPlugin {
			t.Fatalf("包的 is_plugin 也应以真实内容为准：%+v", p)
		}
	})
}

// ---- 规模上限（不需要数据库） ----

func TestBundleManifestLimits(t *testing.T) {
	svc := newOfflineBundleService(t)
	ctx := context.Background()

	t.Run("清单超过上限", func(t *testing.T) {
		// 合法 JSON 后面补空白，把清单灌到超过 maxBundleManifest：读取时就该被拒。
		raw := append([]byte(`{"format":1,"packages":[]}`), bytes.Repeat([]byte(" "), maxBundleManifest)...)
		_, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTarRaw(t, raw, nil)), 0)
		wantBundleError(t, err, "清单", "超过上限")
		wantErrStatus(t, err, 400)
	})

	t.Run("包数超过上限", func(t *testing.T) {
		pkgs := make([]BundlePackage, maxBundlePackages+1)
		for i := range pkgs {
			pkgs[i] = BundlePackage{Name: fmt.Sprintf("limit-pkg-%d", i)}
		}
		_, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, bundleManifestFor(pkgs...), nil)), 0)
		wantBundleError(t, err, "个包", "超过上限")
		wantErrStatus(t, err, 400)
	})

	t.Run("版本数超过上限", func(t *testing.T) {
		versions := make([]BundleVersion, maxBundleVersions+1)
		for i := range versions {
			versions[i] = BundleVersion{Version: "1.0.0"}
		}
		man := bundleManifestFor(BundlePackage{Name: "limit-versions", Versions: versions})
		_, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, nil)), 0)
		wantBundleError(t, err, "个版本", "超过上限")
		wantErrStatus(t, err, 400)
	})

	t.Run("声明总大小超过上限", func(t *testing.T) {
		man := bundleManifestFor(BundlePackage{Name: "limit-bytes", Versions: []BundleVersion{
			{Version: "1.0.0", Size: maxBundleTotalBytes + 1},
		}})
		_, _, err := svc.ImportBundle(ctx, bytes.NewReader(bundleTar(t, man, nil)), 0)
		wantBundleError(t, err, "总大小", "超过上限")
		wantErrStatus(t, err, 400)
	})

	t.Run("刚好到上限的清单可以通过校验", func(t *testing.T) {
		// 500 个包 × 10 个版本 = 版本数正好到上限。
		pkgs := make([]BundlePackage, maxBundlePackages)
		for i := range pkgs {
			versions := make([]BundleVersion, maxBundleVersions/maxBundlePackages)
			pkgs[i] = BundlePackage{Name: fmt.Sprintf("boundary-pkg-%d", i), Versions: versions}
		}
		man := bundleManifestFor(pkgs...)
		if err := checkBundleManifest(&man); err != nil {
			t.Fatalf("正好到上限应通过：%v", err)
		}
		man.Packages[len(man.Packages)-1].Versions = append(man.Packages[len(man.Packages)-1].Versions, BundleVersion{})
		if err := checkBundleManifest(&man); err == nil {
			t.Fatal("多一个版本就应超限")
		}

		// 声明总大小正好到上限可以，多一字节就不行。
		bytesMan := bundleManifestFor(BundlePackage{Name: "boundary-bytes", Versions: []BundleVersion{{Version: "1.0.0", Size: maxBundleTotalBytes}}})
		if err := checkBundleManifest(&bytesMan); err != nil {
			t.Fatalf("声明总大小正好到上限应通过：%v", err)
		}
		bytesMan.Packages[0].Versions[0].Size = maxBundleTotalBytes + 1
		if err := checkBundleManifest(&bytesMan); err == nil {
			t.Fatal("声明总大小多一字节就应超限")
		}
	})
}
