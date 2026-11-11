package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
)

// 解包检查的上限，防止压缩炸弹。
const (
	maxUnpackedSize = 512 << 20
	maxFiles        = 50000
	maxScanFileSize = 2 << 20
	maxPackageJSON  = 4 << 20
	// dsh.bundle.patch 内容检查：只留少量 YAML，避免恶意包塞满内存。
	maxPatchFiles = 64
	maxPatchBytes = 4 << 20
)

// TarballInfo 是解包检查的结果。
type TarballInfo struct {
	PackageJSON  []byte
	FileCount    int
	UnpackedSize int64
	// 包根目录下的相对路径（最多记 2 万个，用于检查 dsh.bundle.patch 等）
	Files   map[string]bool
	Secrets []SecretHit
	// 根目录下少量 .yml/.yaml 的内容（受 maxPatchFiles/maxPatchBytes 限制），
	// 用于真正解析 dsh.bundle.patch。
	YAML       map[string][]byte
	patchBytes int64
	// 包内 SKILL.md 的数量与路径（技能由插件运行时注入，能力中心看不到）。
	Skills     int
	SkillFiles []string
}

type SecretHit struct {
	File string `json:"file"`
	Kind string `json:"kind"`
}

// 疑似密钥。只报类型和文件，不把命中的内容存下来。
var secretPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"private-key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |ENCRYPTED |PGP )?PRIVATE KEY`)},
	{"aws-access-key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"github-token", regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b|\bgithub_pat_[A-Za-z0-9_]{60,}`)},
	{"npm-token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b|_authToken\s*=\s*[^\s$]{8,}`)},
	{"dsh-ee-key", regexp.MustCompile(`\b(?:sk-ee-|ee_npm_)[A-Za-z0-9_-]{20,}`)},
	{"llm-api-key", regexp.MustCompile(`\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{32,}`)},
	{"slack-token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`)},
}

// 不该出现在包里的文件。
var secretFiles = []struct {
	kind  string
	match func(base string) bool
}{
	{"dotenv", func(b string) bool {
		return b == ".env" || (strings.HasPrefix(b, ".env.") && !strings.HasSuffix(b, ".example") && !strings.HasSuffix(b, ".sample"))
	}},
	{"npmrc", func(b string) bool { return b == ".npmrc" }},
	{"private-key-file", func(b string) bool {
		return b == "id_rsa" || b == "id_ed25519" || strings.HasSuffix(b, ".pem") || strings.HasSuffix(b, ".p12")
	}},
}

var ErrNoPackageJSON = errors.New("tarball 里没有 package.json")

// InspectTarball 解开 .tgz：取出 package.json、统计文件、扫描疑似密钥。
// npm 打包时根目录是 package/，个别包（如 @types/*）用别的名字，所以按第一级目录认。
func InspectTarball(r io.Reader) (*TarballInfo, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("不是 gzip：%w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	info := &TarballInfo{Files: map[string]bool{}, YAML: map[string][]byte{}}
	root := ""
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解包失败：%w", err)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			continue
		}
		name := strings.TrimPrefix(path.Clean("/"+h.Name), "/")
		top, rel, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		if root == "" {
			root = top
		}
		if top != root {
			continue
		}
		info.FileCount++
		info.UnpackedSize += h.Size
		if info.FileCount > maxFiles || info.UnpackedSize > maxUnpackedSize {
			return nil, errors.New("解包后文件过多或过大")
		}
		if len(info.Files) < 20000 {
			info.Files[rel] = true
		}
		base := path.Base(rel)
		if strings.EqualFold(base, "SKILL.md") {
			info.Skills++
			if len(info.SkillFiles) < 20 {
				info.SkillFiles = append(info.SkillFiles, rel)
			}
		}
		for _, f := range secretFiles {
			if f.match(base) {
				info.Secrets = append(info.Secrets, SecretHit{File: rel, Kind: f.kind})
			}
		}
		switch {
		case rel == "package.json":
			if h.Size > maxPackageJSON {
				return nil, errors.New("package.json 过大")
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxPackageJSON))
			if err != nil {
				return nil, err
			}
			info.PackageJSON = b
			info.scan(rel, b)
		case h.Size <= maxScanFileSize && scannable(base):
			b, err := io.ReadAll(io.LimitReader(tr, maxScanFileSize))
			if err != nil {
				return nil, err
			}
			info.scan(rel, b)
			if isYAML(base) && len(info.YAML) < maxPatchFiles && info.patchBytes+int64(len(b)) <= maxPatchBytes {
				info.YAML[rel] = b
				info.patchBytes += int64(len(b))
			}
		}
	}
	if info.PackageJSON == nil {
		return nil, ErrNoPackageJSON
	}
	return info, nil
}

func (info *TarballInfo) scan(file string, b []byte) {
	if bytes.IndexByte(b, 0) >= 0 {
		return // 二进制
	}
	for _, p := range secretPatterns {
		if p.re.Match(b) {
			info.Secrets = append(info.Secrets, SecretHit{File: file, Kind: p.kind})
		}
	}
}

func isYAML(base string) bool {
	switch strings.ToLower(path.Ext(base)) {
	case ".yml", ".yaml":
		return true
	}
	return false
}

// 只扫文本类文件；压缩过的 min.js 也扫，密钥常被打包进去。
func scannable(base string) bool {
	ext := strings.ToLower(path.Ext(base))
	switch ext {
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts", ".jsx", ".tsx", ".json", ".yml", ".yaml", ".toml", ".ini", ".cfg", ".conf",
		".env", ".sh", ".ps1", ".py", ".md", ".txt", ".html", ".vue", ".properties", ".xml":
		return true
	}
	return ext == "" || strings.HasPrefix(base, ".env")
}

// ---- 自动检查 ----

// ScanItem 是一条自动检查结果。Key 与 Params 交给前端按语言渲染。
type ScanItem struct {
	Key    string         `json:"key"`
	Level  string         `json:"level"` // pass / warn / fail
	Params map[string]any `json:"params,omitempty"`
}

const (
	LevelPass = "pass"
	LevelWarn = "warn"
	LevelFail = "fail"
)

// Check 对一个版本做发布前检查。tarball 与清单不一致之类的硬错误由调用方直接拒绝，
// 这里给出的是审核员要看的项目。
func Check(m *Manifest, info *TarballInfo, size int64, companyScopes []string) []ScanItem {
	var items []ScanItem
	add := func(key, level string, params map[string]any) {
		items = append(items, ScanItem{Key: key, Level: level, Params: params})
	}

	switch {
	case !m.IsPlugin:
		add("bundle", LevelWarn, map[string]any{"reason": "missing"})
	case m.BundlePatch == "":
		add("bundle", LevelFail, map[string]any{"reason": "no-patch"})
	case !safeRelPath(m.BundlePatch):
		add("bundle", LevelFail, map[string]any{"reason": "unsafe", "path": m.BundlePatch})
	case !info.Files[strings.TrimPrefix(m.BundlePatch, "./")]:
		add("bundle", LevelFail, map[string]any{"reason": "file-missing", "path": m.BundlePatch})
	default:
		add("bundle", LevelPass, map[string]any{"path": m.BundlePatch})
	}

	if item := checkBundlePatch(m, info); item != nil {
		add(item.Key, item.Level, item.Params)
	}

	if info.Skills > 0 {
		files := info.SkillFiles
		if len(files) > 8 {
			files = append(files[:8:8], "…")
		}
		add("skills", LevelWarn, map[string]any{"count": info.Skills, "files": strings.Join(files, "、")})
	} else if m.IsPlugin {
		add("skills", LevelPass, nil)
	}

	if s := m.InstallScripts(); len(s) > 0 {
		add("install-scripts", LevelWarn, map[string]any{"scripts": strings.Join(s, ", ")})
	} else {
		add("install-scripts", LevelPass, nil)
	}

	if len(info.Secrets) > 0 {
		var hits []string
		for _, h := range info.Secrets {
			hits = append(hits, h.File+"（"+h.Kind+"）")
		}
		if len(hits) > 10 {
			hits = append(hits[:10], fmt.Sprintf("… +%d", len(hits)-10))
		}
		add("secrets", LevelFail, map[string]any{"count": len(info.Secrets), "files": strings.Join(hits, "、")})
	} else {
		add("secrets", LevelPass, nil)
	}

	if strings.TrimSpace(m.License) == "" {
		add("license", LevelWarn, nil)
	} else {
		add("license", LevelPass, map[string]any{"license": m.License})
	}

	if m.Engines["dsh"] != "" {
		add("engines", LevelPass, map[string]any{"range": m.Engines["dsh"]})
	} else if m.IsPlugin {
		add("engines", LevelWarn, nil)
	}

	var external []string
	for _, deps := range []map[string]string{m.Dependencies, m.OptionalDependencies, m.RequiredPeers()} {
		for name := range deps {
			if !InScopes(name, companyScopes) && !slices.Contains(external, name) {
				external = append(external, name)
			}
		}
	}
	slices.Sort(external)
	if len(external) > 0 {
		shown := external
		if len(shown) > 8 {
			shown = append(slices.Clone(shown[:8]), "…")
		}
		add("dependencies", LevelPass, map[string]any{"count": len(external), "names": strings.Join(shown, ", ")})
	}

	level := LevelPass
	if size > 20<<20 {
		level = LevelWarn
	}
	add("size", level, map[string]any{"size": size, "unpacked": info.UnpackedSize, "files": info.FileCount})
	return items
}

// safeRelPath 与 DSH 桌面端的 safeBundlePatch 同一口径：非空、相对、不含 . / .. / 反斜杠 / 冒号。
func safeRelPath(p string) bool {
	if p == "" || len(p) > 512 || strings.ContainsAny(p, "\x00\\") {
		return false
	}
	p = strings.TrimPrefix(p, "./")
	if p == "" || strings.HasPrefix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, ":") {
			return false
		}
	}
	return true
}
