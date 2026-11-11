package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// tgz 按固定顺序写入：package/ 下的文件在前（第一个条目决定包根目录），其余按名字排序。
func tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		pa, pb := strings.HasPrefix(a, "package/"), strings.HasPrefix(b, "package/")
		if pa != pb {
			if pa {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := files[name]
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestNames(t *testing.T) {
	for name, want := range map[string]bool{
		"left-pad": true, "@acme/x": true, "@acme/x.y_z~1": true,
		"JSONStream": false, "@acme": false, "../x": false, "@a/b/c": false, "a b": false, "": false,
	} {
		if got := ValidPublishName(name); got != want {
			t.Errorf("ValidPublishName(%q) = %v", name, got)
		}
	}
	if !ValidName("JSONStream") || ValidName("@acme/..") || ValidName("a/../b") {
		t.Error("ValidName 对历史包名或路径穿越的判断不对")
	}
	if Scope("@acme/x") != "@acme" || Scope("x") != "" || !InScopes("@acme/x", []string{"@acme"}) || InScopes("@acmex/y", []string{"@acme"}) {
		t.Error("作用域判断不对")
	}
	if TarballFile("@acme/x", "1.0.0") != "x-1.0.0.tgz" {
		t.Error("TarballFile")
	}
	if v, ok := VersionFromFile("@acme/x-y", "x-y-1.2.3-beta.1.tgz"); !ok || v != "1.2.3-beta.1" {
		t.Errorf("VersionFromFile = %q %v", v, ok)
	}
	for v, want := range map[string]bool{"1.0.0": true, "1.0.0-rc.1+build.5": true, "v1.0.0": false, "1.0": false, "01.0.0": false, "1.0.0-01": false} {
		if ValidVersion(v) != want {
			t.Errorf("ValidVersion(%q) 应为 %v", v, want)
		}
	}
	for tag, want := range map[string]bool{"latest": true, "beta": true, "next-2": true, "1.0.0": false, "^1": false, "": false, "a b": false} {
		if validTag(tag) != want {
			t.Errorf("validTag(%q) 应为 %v", tag, want)
		}
	}
}

func TestResolveRange(t *testing.T) {
	vs := []string{"1.0.0", "1.2.0", "1.3.0-beta.1", "2.0.0", "2.1.0"}
	tags := map[string]string{"latest": "2.0.0", "next": "2.1.0"}
	for spec, want := range map[string]string{
		"^1.0.0": "1.2.0", "~1.0.0": "1.0.0", "*": "2.0.0", "": "2.0.0", "latest": "2.0.0", "next": "2.1.0",
		">=2": "2.0.0", "^2.0.1": "2.1.0", "^3": "", "1.2.0 || 2.1.0": "2.1.0", "bogus tag": "",
	} {
		if got := ResolveRange(spec, vs, tags); got != want {
			t.Errorf("ResolveRange(%q) = %q，期望 %q", spec, got, want)
		}
	}
	// npm 的预发布规则（真实数据：@deepseek-ai/dsh-agent 的依赖写法）
	agent := []string{"0.0.1-rc.1", "0.2.1-alpha.5", "0.2.1-rc.1", "0.2.1-rc.5", "0.2.2-alpha.2"}
	agentTags := map[string]string{"latest": "0.0.1-rc.1", "alpha": "0.2.2-alpha.2", "next": "0.2.1-rc.5"}
	for spec, want := range map[string]string{
		"^0.2.1-rc.1": "0.2.1-rc.5", "^0.2.1-alpha.1": "0.2.1-rc.5", ">=0.2.2-alpha.0": "0.2.2-alpha.2", "^0.2.0": "", "0.2.2-alpha.2 || ^0.2.1-rc.2": "0.2.2-alpha.2",
	} {
		if got := ResolveRange(spec, agent, agentTags); got != want {
			t.Errorf("预发布：ResolveRange(%q) = %q，期望 %q", spec, got, want)
		}
	}
	for spec, want := range map[string]bool{"github:a/b": true, "git+https://x/y.git": true, "file:../x": true, "a/b": true, "^1.0.0": false, "@scope/x": false} {
		if nonRegistrySpec(spec) != want {
			t.Errorf("nonRegistrySpec(%q) 应为 %v", spec, want)
		}
	}
}

func TestInspectAndCheck(t *testing.T) {
	pkg := `{"name":"@acme/x","version":"1.0.0","dsh":{"bundle":{"patch":"cordis.patch.yml"}},
		"scripts":{"postinstall":"node fetch-binary.js"},"dependencies":{"left-pad":"^1","@acme/y":"^1"}}`
	b := tgz(t, map[string]string{
		"package/package.json":     pkg,
		"package/cordis.patch.yml": "x: 1",
		"package/dist/index.js":    "const key = 'AKIAABCDEFGHIJKLMNOP';",
		"package/.env":             "TOKEN=1",
		"package/logo.png":         "\x89PNG\x00AKIAABCDEFGHIJKLMNOP",
		"other/package.json":       `{"name":"ignored"}`,
	})
	info, err := InspectTarball(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := ParseManifest(info.PackageJSON)
	if m.Name != "@acme/x" || !m.IsPlugin || m.BundlePatch != "cordis.patch.yml" {
		t.Fatalf("清单解析不对：%+v", m)
	}
	items := Check(m, info, int64(len(b)), []string{"@acme"})
	got := map[string]ScanItem{}
	for _, it := range items {
		got[it.Key] = it
	}
	if got["bundle"].Level != LevelPass || got["install-scripts"].Level != LevelWarn || got["license"].Level != LevelWarn {
		t.Fatalf("检查结果不对：%+v", items)
	}
	if s := got["secrets"]; s.Level != LevelFail || s.Params["count"] != 2 {
		t.Fatalf("应该发现 .env 与 AWS key（二进制文件不扫）：%+v", s)
	}
	if d := got["dependencies"]; d.Params["count"] != 1 {
		t.Fatalf("企业作用域的依赖不算社区依赖：%+v", d)
	}

	missing := tgz(t, map[string]string{"package/package.json": `{"name":"a","version":"1.0.0","dsh":{"bundle":{"patch":"../evil.yml"}}}`})
	info, _ = InspectTarball(bytes.NewReader(missing))
	m, _ = ParseManifest(info.PackageJSON)
	for _, it := range Check(m, info, 1, nil) {
		if it.Key == "bundle" && (it.Level != LevelFail || it.Params["reason"] != "unsafe") {
			t.Fatalf("越界的 bundle.patch 应判失败：%+v", it)
		}
	}
	if _, err := InspectTarball(bytes.NewReader(tgz(t, map[string]string{"package/index.js": "x"}))); err != ErrNoPackageJSON {
		t.Fatalf("没有 package.json：%v", err)
	}
	if _, err := InspectTarball(strings.NewReader("not gzip")); err == nil {
		t.Fatal("非 gzip 应报错")
	}
}

func TestPackumentEncode(t *testing.T) {
	p := &Packument{
		Name: "@acme/x",
		Versions: []Version{
			{Version: "1.0.0", Manifest: map[string]any{"name": "@acme/x", "version": "1.0.0", "scripts": map[string]any{"install": "node-gyp rebuild"}, "_resolved": "https://registry.npmjs.org/x", "dist": map[string]any{"tarball": "https://registry.npmjs.org/x.tgz", "integrity": "sha512-a", "signatures": []any{"x"}, "attestations": map[string]any{"url": "https://registry.npmjs.org/-/npm/v1/attestations/x"}, "unpackedSize": 10.0}}, Integrity: "sha512-a", Shasum: "b", Time: time.Unix(1700000000, 0)},
			{Version: "2.0.0-beta.1", Manifest: map[string]any{"description": "d"}, Integrity: "sha512-c", Shasum: "d", Time: time.Unix(1700001000, 0)},
		},
		DistTags: map[string]string{"latest": "9.9.9", "beta": "2.0.0-beta.1"},
	}
	full, err := p.Encode("https://dsh.example/npm", false)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DistTags map[string]string         `json:"dist-tags"`
		Versions map[string]map[string]any `json:"versions"`
		Time     map[string]string         `json:"time"`
	}
	_ = json.Unmarshal(full, &doc)
	if doc.DistTags["latest"] != "1.0.0" || doc.DistTags["beta"] != "2.0.0-beta.1" {
		t.Fatalf("指向不存在版本的 latest 应回落到最高正式版：%v", doc.DistTags)
	}
	v1 := doc.Versions["1.0.0"]
	dist := v1["dist"].(map[string]any)
	if dist["tarball"] != "https://dsh.example/npm/@acme/x/-/x-1.0.0.tgz" || dist["signatures"] == nil || dist["attestations"] != nil || dist["unpackedSize"] != 10.0 || v1["_resolved"] != nil {
		t.Fatalf("dist 改写不对：%v / %v", dist, v1)
	}
	if doc.Time["created"] == "" || doc.Time["1.0.0"] == "" {
		t.Fatalf("time 字段缺失：%v", doc.Time)
	}
	corgi, _ := p.Encode("https://dsh.example/npm", true)
	_ = json.Unmarshal(corgi, &doc)
	if doc.Versions["1.0.0"]["hasInstallScript"] != true || doc.Versions["1.0.0"]["scripts"] != nil || doc.Versions["2.0.0-beta.1"]["description"] != nil {
		t.Fatalf("精简文档字段不对：%v", doc.Versions)
	}
}

func TestAssertIndex(t *testing.T) {
	ok := &Index{Format: 1, Title: map[string]string{"zh": "x"}, Description: map[string]string{"en": "y"},
		Plugins: []IndexPlugin{{Name: "@acme/x", Versions: []indexVersion{{Version: "1.0.0", PublishTimestamp: 1, Source: map[string]string{"type": "npm"}}}}}}
	if err := AssertIndex(ok); err != nil {
		t.Fatal(err)
	}
	dup := *ok
	dup.Plugins = append(dup.Plugins, ok.Plugins[0])
	if AssertIndex(&dup) == nil {
		t.Fatal("重名插件应被拒")
	}
	badCat := *ok
	badCat.Plugins = []IndexPlugin{{Name: "x", Metadata: &indexMeta{Category: "nope"}, Versions: ok.Plugins[0].Versions}}
	if AssertIndex(&badCat) == nil {
		t.Fatal("未知分类应被拒")
	}
}

func TestIndexPluginDropsCrossOriginHTTPIcon(t *testing.T) {
	versions := []*store.NpmVersion{{Version: "1.0.0", CreatedAt: time.Unix(1, 0)}}
	p := &store.NpmPackage{Name: "@acme/icon", Icon: "http://other.example/icon.png"}
	ip := indexPlugin(p, versions, 0, "https://portal.example")
	if ip == nil || ip.Icon != "" {
		t.Fatalf("跨源 http 图标应省略：%+v", ip)
	}
	p.Icon = "https://cdn.example/icon.png"
	ip = indexPlugin(p, versions, 0, "https://portal.example")
	if ip == nil || ip.Icon != "https://cdn.example/icon.png" {
		t.Fatalf("https 图标应保留：%+v", ip)
	}
}

func TestRepoAndURL(t *testing.T) {
	for in, want := range map[string]string{
		"git+https://github.com/a/b.git": "https://github.com/a/b",
		"github:a/b":                     "https://github.com/a/b",
		"a/b":                            "https://github.com/a/b",
		"git://github.com/a/b.git":       "https://github.com/a/b",
	} {
		if got := repoURL(in); got != want {
			t.Errorf("repoURL(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"https://x.com/a": "https://x.com/a", "javascript:alert(1)": "", "https://u:p@x.com": "", "https://x.com/?q=1": ""} {
		if got := httpURL(in); got != want {
			t.Errorf("httpURL(%q) = %q", in, got)
		}
	}
	if iconURL("https://avatars.githubusercontent.com/x?s=96") == "" || iconURL("https://u:p@x.com/a.png") != "" {
		t.Error("iconURL 应允许 query、拒绝凭据")
	}
	for _, tc := range []struct{ raw, base, want string }{
		{"https://cdn.example/x.png", "https://portal.example", "https://cdn.example/x.png"},
		{"http://portal.example/icon.png", "http://portal.example", "http://portal.example/icon.png"},
		{"http://portal.example/icon.png", "https://portal.example", ""},
		{"http://other.example/icon.png", "https://portal.example", ""},
		{"http://portal.example/icon.png", "", ""},
	} {
		if got := iconURLFor(tc.raw, tc.base); got != tc.want {
			t.Errorf("iconURLFor(%q, %q) = %q, want %q", tc.raw, tc.base, got, tc.want)
		}
	}
}

func TestParseUserAgent(t *testing.T) {
	for ua, want := range map[string]Client{
		"npm/10.8.2 node/v22.11.0 linux x64 workspaces/false": {"npm", "linux", "x64"},
		"pnpm/9.12.3 npm/? node/v20.18.0 darwin arm64":        {"pnpm", "darwin", "arm64"},
		"yarn/1.22.22 npm/? node/v20.18.0 win32 x64":          {"yarn", "win32", "x64"},
		"pnpm/9.0.0 npm/? node/v20.0.0 linux loong64":         {"pnpm", "linux", "loong64"},
		"Bun/1.1.30":                      {"bun", "", ""},
		"curl/8.5.0":                      {"other", "", ""},
		"npm/10 node/v22 haiku x64":       {"npm", "", "x64"},
		"npm/10 node/v22":                 {"npm", "", ""},
		"":                                {},
		"Mozilla/5.0 (X11; Linux x86_64)": {"other", "", ""},
	} {
		if got := ParseUserAgent(ua); got != want {
			t.Errorf("ParseUserAgent(%q) = %+v，期望 %+v", ua, got, want)
		}
	}
}

func TestReadTarball(t *testing.T) {
	b := tgz(t, map[string]string{
		"package/package.json": `{"name":"@acme/x","version":"1.0.0","scripts":{"postinstall":"node a.js"}}`,
		"package/a.js":         "fetch('https://evil.example/x')\n",
		"package/dist/key.js":  "const k = 'AKIAABCDEFGHIJKLMNOP';\nfetch('https://evil.example/y')\n",
		"package/.env":         "TOKEN=abc\n",
		"package/logo.png":     "\x89PNG\x00AKIA",
		"other/package.json":   `{"name":"ignored"}`,
	})
	scanned := map[string]bool{}
	c, err := ReadTarball(bytes.NewReader(b), "a.js", 1000, func(file string, _ []byte) { scanned[file] = true })
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 5 || len(c.Files) != 5 {
		t.Fatalf("应只统计包根目录下的 5 个文件：%+v", c.Files)
	}
	if c.File == nil || !strings.Contains(c.File.Text, "fetch") || c.File.Withheld != "" {
		t.Fatalf("应读出 a.js：%+v", c.File)
	}
	if !scanned["a.js"] || !scanned["package.json"] {
		t.Fatalf("普通文本文件都该扫描：%v", scanned)
	}
	if scanned[".env"] || scanned["dist/key.js"] || scanned["logo.png"] {
		t.Fatalf("密钥文件、含密钥的源码、二进制不该交给扫描：%v", scanned)
	}
	for _, file := range []string{".env", "dist/key.js", "logo.png", "missing.js"} {
		c, err := ReadTarball(bytes.NewReader(b), file, 1000, nil)
		if err != nil {
			t.Fatal(err)
		}
		if c.File == nil || c.File.Text != "" || c.File.Withheld == "" {
			t.Fatalf("%s 不该给内容：%+v", file, c.File)
		}
	}
	// 超长文件按字节截断且仍是合法 UTF-8
	long := tgz(t, map[string]string{"package/package.json": `{"name":"a","version":"1.0.0"}`, "package/big.txt": strings.Repeat("好", 1000)})
	c, err = ReadTarball(bytes.NewReader(long), "big.txt", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c.File.Truncated || len(c.File.Text) > 100 || !utf8.ValidString(c.File.Text) {
		t.Fatalf("截断：%+v", c.File)
	}
	if _, err := ReadTarball(bytes.NewReader(tgz(t, map[string]string{"package/a.js": "x"})), "", 10, nil); err != ErrNoPackageJSON {
		t.Fatalf("没有 package.json 应报错：%v", err)
	}
}

// integrity 和上游签名时的不一致（内容换过）就不带签名。
func TestPackumentSignatureMismatch(t *testing.T) {
	p := &Packument{Name: "x", Versions: []Version{{Version: "1.0.0", Manifest: map[string]any{"dist": map[string]any{"integrity": "sha512-old", "signatures": []any{"x"}}}, Integrity: "sha512-new", Shasum: "b", Time: time.Unix(1700000000, 0)}}}
	full, err := p.Encode("https://dsh.example/npm", false)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Versions map[string]map[string]any `json:"versions"`
	}
	_ = json.Unmarshal(full, &doc)
	if dist := doc.Versions["1.0.0"]["dist"].(map[string]any); dist["signatures"] != nil {
		t.Fatalf("integrity 不一致时不应带签名：%v", dist)
	}
}

// 索引版本要带上 package.json 的 engines.dsh，DSH 读端才能按兼容性过滤版本；
// 非法/workspace 范围省略，避免一条坏值让整份索引被拒收。
func TestIndexPluginIncludesEnginesDsh(t *testing.T) {
	versions := []*store.NpmVersion{
		{Version: "3.0.0", CreatedAt: time.Unix(3, 0), Manifest: json.RawMessage(`{"engines":{"dsh":"latest"}}`)},
		{Version: "2.0.0", CreatedAt: time.Unix(2, 0), Manifest: json.RawMessage(`{"engines":{"dsh":">=0.2.0-0 <0.3.0"}}`)},
		{Version: "1.0.0", CreatedAt: time.Unix(1, 0), Manifest: json.RawMessage(`{"engines":{"dsh":"workspace:*"}}`)},
		{Version: "0.9.0", CreatedAt: time.Unix(1, 0)},
	}
	ip := indexPlugin(&store.NpmPackage{Name: "dsh-x"}, versions, 0, "")
	if ip == nil || len(ip.Versions) != 4 {
		t.Fatalf("应保留 4 个合法版本：%+v", ip)
	}
	byVersion := map[string]indexVersion{}
	for _, v := range ip.Versions {
		byVersion[v.Version] = v
	}
	if e := byVersion["2.0.0"].Engines; e == nil || e.Dsh != ">=0.2.0-0 <0.3.0" {
		t.Fatalf("2.0.0 应带 engines.dsh：%+v", byVersion["2.0.0"])
	}
	for _, v := range []string{"3.0.0", "1.0.0", "0.9.0"} {
		if byVersion[v].Engines != nil {
			t.Errorf("%s 不应带 engines：%+v", v, byVersion[v].Engines)
		}
	}
	body, err := json.Marshal(byVersion["2.0.0"])
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Engines indexEngines `json:"engines"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Engines.Dsh != ">=0.2.0-0 <0.3.0" {
		t.Fatalf("索引 JSON 应包含 engines.dsh：%s", body)
	}
}
