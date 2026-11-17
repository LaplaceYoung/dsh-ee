package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"time"
)

// tarEntry 是构造 tar.gz 用的原始条目：typ 覆盖目录这类不参与文件计数的条目，
// 用来验证「原始条目数上限」是在类型过滤之前就对每个 tr.Next() 生效的。
type tarEntry struct {
	name string
	body string
	typ  byte
}

// tgzEntries 按给定顺序写 tar 条目（包括目录等非 regular 类型）。
// 条目多到 maxFiles 量级时 body 为空，构造出来的 .tgz 只有几十 KB。
func tgzEntries(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: e.typ}
		if e.typ == tar.TypeDir {
			h.Mode = 0o755
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
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

// dupFlood 构造 1 条合法 package.json + n 条同名零长条目。
// #212 之后按真实路径去重：这些条目只算 1 个文件、0 字节，所以只靠去重计数
// 永远触发不了 maxFiles / maxUnpackedSize —— 但循环仍然要逐条解 header。
func dupFlood(n int, typ byte) []tarEntry {
	entries := make([]tarEntry, 0, n+1)
	entries = append(entries, tarEntry{name: "package/package.json", body: `{"name":"@acme/dup","version":"1.0.0"}`, typ: tar.TypeReg})
	for i := 0; i < n; i++ {
		name := "package/dup.js"
		if typ == tar.TypeDir {
			name = "package/dup/"
		}
		entries = append(entries, tarEntry{name: name, typ: typ})
	}
	return entries
}

// 同名重复条目不能绕过上限：按路径去重只用于清单/体积记账，原始条目数超了 maxFiles 就得拒收。
func TestInspectTarballRejectsRawEntryFlood(t *testing.T) {
	cases := []struct {
		name string
		typ  byte
	}{
		{"同名 regular 条目", tar.TypeReg},
		// 目录条目在旧实现里连 tarCounter 都不进，同样能靠数量打满 CPU。
		{"同名目录条目", tar.TypeDir},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := dupFlood(maxFiles+1, tc.typ) // + package.json = maxFiles+2 个原始条目
			b := tgzEntries(t, entries)
			start := time.Now()
			info, err := InspectTarball(bytes.NewReader(b))
			elapsed := time.Since(start)
			t.Logf("原始条目 %d 个（.tgz %d 字节）：err=%v 耗时 %v", len(entries), len(b), err, elapsed)
			if err == nil {
				t.Fatalf("原始条目 %d > maxFiles=%d 应拒收；按路径去重后 files=%d bytes=%d 放行了",
					len(entries), maxFiles, info.FileCount, info.UnpackedSize)
			}
			if !strings.Contains(err.Error(), "Too many files or files too large") {
				t.Fatalf("错误文案应与现有「文件太多」一致：%v", err)
			}
		})
	}
}

// 恰好 maxFiles 个原始条目不算超限；通过时清单/体积仍按去重口径记账。
func TestInspectTarballRawEntryLimitBoundary(t *testing.T) {
	entries := dupFlood(maxFiles-1, tar.TypeReg) // + package.json = 恰好 maxFiles
	b := tgzEntries(t, entries)
	info, err := InspectTarball(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("恰好 %d 个原始条目不该拒收：%v", maxFiles, err)
	}
	if info.FileCount != 2 {
		t.Fatalf("同名条目只应算一个文件：files=%d", info.FileCount)
	}
	wantBytes := int64(len(`{"name":"@acme/dup","version":"1.0.0"}`))
	if info.UnpackedSize != wantBytes {
		t.Fatalf("解压体积应按去重后的声明计：%d（应 %d）", info.UnpackedSize, wantBytes)
	}
}

// 审核页的 ReadTarball 走同一套解包循环，同样不能被同名重复条目绕过。
func TestReadTarballRejectsRawEntryFlood(t *testing.T) {
	entries := dupFlood(maxFiles+1, tar.TypeReg)
	b := tgzEntries(t, entries)
	start := time.Now()
	c, err := ReadTarball(bytes.NewReader(b), "", 1000, nil)
	elapsed := time.Since(start)
	t.Logf("原始条目 %d 个（.tgz %d 字节）：err=%v 耗时 %v", len(entries), len(b), err, elapsed)
	if err == nil {
		t.Fatalf("原始条目 %d > maxFiles=%d 应拒收；按路径去重后 total=%d 放行了", len(entries), maxFiles, c.Total)
	}
	if !strings.Contains(err.Error(), "Too many files or files too large") {
		t.Fatalf("错误文案应与现有「文件太多」一致：%v", err)
	}
}

// 解压字节上限：声明体积很小，但 gzip 解压后的字节流超过上限时也要拒收 ——
// 「零长 header / 超长 PAX 长名头」不增加声明体积，却能一直解压、解析。
// 这里注入 32 KB 上限；同一个包在生产上限（数百 MB）下远不到顶，不能误伤。
func TestInspectTarballLimitsUnpackedStream(t *testing.T) {
	const pkg = `{"name":"@acme/big","version":"1.0.0"}`
	cases := []struct {
		name    string
		entries []tarEntry
	}{
		{
			// 声明 64 KB 正文，远低于 maxUnpackedSize；但解压流（header + 正文）超过 32 KB。
			name: "正文超过解压上限",
			entries: []tarEntry{
				{name: "package/package.json", body: pkg, typ: tar.TypeReg},
				{name: "package/big.js", body: strings.Repeat("a", 64<<10), typ: tar.TypeReg},
			},
		},
		{
			// 声明 0 字节：只有 1 条超长路径，解压出来的 PAX 扩展头本身已经超过 32 KB。
			name: "PAX 长名头超过解压上限",
			entries: []tarEntry{
				{name: "package/package.json", body: pkg, typ: tar.TypeReg},
				{name: "package/" + strings.Repeat("a", 64<<10), typ: tar.TypeReg},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tgzEntries(t, tc.entries)
			lim := defaultTarLimits()
			lim.streamBytes = 32 << 10
			start := time.Now()
			_, err := inspectTarball(bytes.NewReader(b), lim)
			elapsed := time.Since(start)
			t.Logf(".tgz %d 字节，注入上限 %d：err=%v 耗时 %v", len(b), lim.streamBytes, err, elapsed)
			if err == nil || !strings.Contains(err.Error(), "Too many files or files too large") {
				t.Fatalf("解压字节流超限应拒收：%v", err)
			}
			// 生产上限下同一个包必须照常通过，否则就是误伤正常包。
			info, err := InspectTarball(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("生产上限下正常包被误伤：%v", err)
			}
			if info.FileCount != len(tc.entries) {
				t.Fatalf("正常包统计不对：files=%d（应 %d）", info.FileCount, len(tc.entries))
			}
		})
	}
}

// ReadTarball 的解包循环同样要受解压字节上限约束：这里让审核扫描去读正文，触发上限。
func TestReadTarballLimitsUnpackedStream(t *testing.T) {
	const pkg = `{"name":"@acme/big","version":"1.0.0"}`
	b := tgzEntries(t, []tarEntry{
		{name: "package/package.json", body: pkg, typ: tar.TypeReg},
		{name: "package/big.js", body: strings.Repeat("a", 64<<10), typ: tar.TypeReg},
	})
	lim := defaultTarLimits()
	lim.streamBytes = 32 << 10
	scanned := 0
	start := time.Now()
	_, err := readTarball(bytes.NewReader(b), "", 1000, func(string, []byte) { scanned++ }, lim)
	elapsed := time.Since(start)
	t.Logf(".tgz %d 字节，注入上限 %d：err=%v 耗时 %v", len(b), lim.streamBytes, err, elapsed)
	if err == nil || !strings.Contains(err.Error(), "Too many files or files too large") {
		t.Fatalf("解压字节流超限应拒收：%v", err)
	}
	// 生产上限下照常读取。
	c, err := ReadTarball(bytes.NewReader(b), "", 1000, nil)
	if err != nil || c.Total != 2 {
		t.Fatalf("生产上限下正常包被误伤：%v / %+v", err, c)
	}
}
