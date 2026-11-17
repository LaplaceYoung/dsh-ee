package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
)

// tgzOrdered 按给定顺序写 tar 条目：tgz 用 map 装不下「同名重复条目」，这里显式给顺序。
func tgzOrdered(t *testing.T, entries [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e[0], Mode: 0o644, Size: int64(len(e[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// 顶层目录判定与审计复现：同一 tarball 里既有干净的 package/，又有带 postinstall 的
// hidden/。旧实现只统计/扫描第一个顶层目录：审核看到的是干净清单、解压上限也只算第一层，
// 而 npm 安装时 strip:1 会把 hidden/ 平铺到安装根执行 postinstall。
func TestInspectTarballRejectsMultipleTopDirs(t *testing.T) {
	files := map[string]string{
		"package/package.json": `{"name":"@acme/x","version":"1.0.0"}`,
		"package/index.js":     "module.exports = 1;\n",
		"hidden/package.json":  `{"name":"@acme/x","version":"1.0.0","scripts":{"postinstall":"node pwn.js"}}`,
		"hidden/pwn.js":        "require('child_process').execFileSync('id');\n",
		"hidden/.env":          "TOKEN=1\n",
	}
	b := tgz(t, files)
	info, err := InspectTarball(bytes.NewReader(b))
	if !errors.Is(err, ErrMultipleTopDirs) {
		var hasInstall bool
		if info != nil {
			m, _ := ParseManifest(info.PackageJSON)
			hasInstall = m != nil && len(m.InstallScripts()) > 0
		}
		t.Fatalf("多顶层目录的 tarball 应被拒收：err=%v HasInstallScript=%v FileCount=%+v",
			err, hasInstall, info)
	}
	if !strings.Contains(err.Error(), "hidden") || !strings.Contains(err.Error(), "package") {
		t.Fatalf("错误信息应点名两个顶层目录：%v", err)
	}
	if info == nil {
		t.Fatal("拒收时也要带回已扫描的 info，供调用方诊断")
	}
	if got := strings.Join(info.TopDirs, ","); got != "package,hidden" {
		t.Fatalf("顶层目录记录不对：%q", got)
	}
	// 统计口径：所有 regular 条目（含第二个顶层目录）都要计数。
	var wantSize int64
	for _, body := range files {
		wantSize += int64(len(body))
	}
	if info.FileCount != len(files) || info.UnpackedSize != wantSize {
		t.Fatalf("统计应覆盖全部 regular 条目：files=%d（应 %d）size=%d（应 %d）",
			info.FileCount, len(files), info.UnpackedSize, wantSize)
	}
	// package.json 仍取包根那份（干净、无 install 脚本）；恶意的是 hidden/package.json。
	m, err := ParseManifest(info.PackageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.InstallScripts()) != 0 {
		t.Fatalf("包根清单不该混入 hidden/package.json：%v", m.InstallScripts())
	}
	// 密钥扫描也要覆盖第二个顶层目录。
	foundEnv := false
	for _, h := range info.Secrets {
		if h.File == "hidden/.env" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Fatalf("hidden/.env 应被扫描到：%+v", info.Secrets)
	}
}

// 单个顶层目录（不论是否叫 package/）仍按原样通过——npm 包里 @types/* 等会用别的名字。
func TestInspectTarballAllowsSingleTopDir(t *testing.T) {
	t.Run("package 根", func(t *testing.T) {
		b := tgz(t, map[string]string{
			"package/package.json": `{"name":"@acme/x","version":"1.0.0"}`,
			"package/index.js":     "x\n",
		})
		info, err := InspectTarball(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(info.TopDirs, ","); got != "package" {
			t.Fatalf("TopDirs = %q", got)
		}
		if !info.Files["index.js"] || info.FileCount != 2 {
			t.Fatalf("包根相对路径不对：%+v / %d", info.Files, info.FileCount)
		}
	})
	t.Run("任意单顶层", func(t *testing.T) {
		b := tgz(t, map[string]string{
			"my-pkg/package.json": `{"name":"@acme/y","version":"2.0.0","scripts":{"postinstall":"node x.js"}}`,
			"my-pkg/index.js":     "x\n",
		})
		info, err := InspectTarball(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("单个非 package/ 顶层目录应照常接受：%v", err)
		}
		if got := strings.Join(info.TopDirs, ","); got != "my-pkg" {
			t.Fatalf("TopDirs = %q", got)
		}
		m, err := ParseManifest(info.PackageJSON)
		if err != nil || len(m.InstallScripts()) == 0 {
			t.Fatalf("应照常解析出 install 脚本：%v / %+v", err, m)
		}
		if !info.Files["index.js"] || info.FileCount != 2 {
			t.Fatalf("包根相对路径不对：%+v / %d", info.Files, info.FileCount)
		}
	})
}

// 同名条目只算一个文件；解压体积按最大声明计，避免重复条目绕过上限。
func TestInspectTarballCountsUniquePaths(t *testing.T) {
	pkg := `{"name":"a","version":"1.0.0"}`
	b := tgzOrdered(t, [][2]string{
		{"package/package.json", pkg},
		{"package/dup.js", "x"},
		{"package/dup.js", "yyy"},
	})
	info, err := InspectTarball(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if info.FileCount != 2 {
		t.Fatalf("同名条目只应算一个文件：%d", info.FileCount)
	}
	if want := int64(len(pkg) + len("yyy")); info.UnpackedSize != want {
		t.Fatalf("解压体积应取最大声明：%d（应 %d）", info.UnpackedSize, want)
	}
}

// 第二个顶层目录里的超大文件必须计入解压上限，否则多顶层就是压缩炸弹盲区。
func TestInspectTarballCountsHiddenTopTowardBombLimit(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	pkg := `{"name":"a","version":"1.0.0"}`
	if err := tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(pkg)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(pkg)); err != nil {
		t.Fatal(err)
	}
	// 只写 header、不写正文：InspectTarball 在校验总量时会先报超限，不会去读正文。
	if err := tw.WriteHeader(&tar.Header{Name: "hidden/big.bin", Mode: 0o644, Size: maxUnpackedSize + 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	// 故意不 Close tar writer（它会把缺的正文补零写出来），只刷 gzip。
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectTarball(bytes.NewReader(buf.Bytes())); err == nil || !strings.Contains(err.Error(), "Too many files or files too large") {
		t.Fatalf("第二个顶层目录里的超大文件应触发解压上限：%v", err)
	}
}

// 审核页的文件清单同样不能只列第一个顶层目录：多顶层要报错，且拒收前扫描覆盖全部条目。
func TestReadTarballRejectsMultipleTopDirs(t *testing.T) {
	b := tgz(t, map[string]string{
		"package/package.json": `{"name":"@acme/x","version":"1.0.0"}`,
		"package/a.js":         "fetch('https://evil.example/x')\n",
		"hidden/package.json":  `{"name":"@acme/x","version":"1.0.0","scripts":{"postinstall":"node pwn.js"}}`,
		"hidden/pwn.js":        "require('child_process').execFileSync('id');\n",
	})
	scanned := map[string]bool{}
	c, err := ReadTarball(bytes.NewReader(b), "", 1000, func(file string, _ []byte) { scanned[file] = true })
	if !errors.Is(err, ErrMultipleTopDirs) {
		t.Fatalf("多顶层目录的 tarball 应被拒收：err=%v contents=%+v", err, c)
	}
	if c != nil {
		t.Fatalf("拒收时不该返回只含第一个顶层目录的清单：%+v", c.Files)
	}
	if !scanned["hidden/pwn.js"] {
		t.Fatalf("拒收前的扫描要覆盖全部顶层目录：%v", scanned)
	}

	ok := tgz(t, map[string]string{
		"package/package.json": `{"name":"@acme/x","version":"1.0.0"}`,
		"package/a.js":         "x\n",
	})
	c, err = ReadTarball(bytes.NewReader(ok), "", 1000, nil)
	if err != nil || c.Total != 2 {
		t.Fatalf("单顶层正常包应照常读取：%v / %+v", err, c)
	}
}
