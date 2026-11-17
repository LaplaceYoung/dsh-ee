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
	// 顶层目录最多记几个。正常的 npm 包只有一个（package/）；多顶层直接拒收，
	// 记几个只是为了在错误信息和检查项里点名。
	maxTopDirs = 8
	// 解压后字节流的上限：正文总量（maxUnpackedSize）之外，tar 框架本身还有开销 ——
	// 每个条目 512 字节 header、正文至多 511 字节对齐 padding、PAX 长名等额外记录，
	// 末尾还有 1024 字节 EOF 块，按每条目 2 KB + 1 KB 留余量，正常包不会被误伤。
	// 没有这层记账时，「大量超长 PAX 头 / 零长 header」不增加声明体积，却能一直解压。
	maxUnpackedStream = maxUnpackedSize + maxFiles*2048 + 1024
)

// TarballInfo 是解包检查的结果。
type TarballInfo struct {
	PackageJSON  []byte
	FileCount    int
	UnpackedSize int64
	// tarball 里的顶层目录（tar 路径的第一段，通常只有一个 package/）。
	// 多于一个时 InspectTarball 返回 ErrMultipleTopDirs，但仍带回已扫描的 info 供调用方诊断。
	TopDirs []string
	// 包内 regular 文件的相对路径（最多记 2 万个，用于检查 dsh.bundle.patch 等）：
	// 包根目录下的文件用去掉顶层目录的路径；多顶层时保留顶层目录前缀，避免撞名。
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

var (
	ErrNoPackageJSON = errors.New("No package.json in the tarball")
	// ErrMultipleTopDirs：tarball 里有不止一个顶层目录。npm 包只有一个包根（通常叫 package/），
	// 安装时 strip:1 会把每个顶层目录都平铺到安装根 —— 多出来的目录能让 install 脚本
	// 绕过审核执行，所以整包拒收，不能只信第一个目录。
	ErrMultipleTopDirs = errors.New("tarball 里有多个顶层目录")
	// errTooLarge：原始条目数（含同名重复、目录/链接等非文件条目）或解压字节数超限。
	// 按路径去重只用于清单与体积记账，不能当成条目数上限。
	errTooLarge = errors.New("Too many files or files too large after unpacking")
)

// tarLimits 是解包循环的硬上限。生产路径固定用 defaultTarLimits；
// 测试可以注入小上限，免得为了验证超限拒收去构造几百 MB 数据。
type tarLimits struct {
	// rawEntries：tr.Next() 返回的原始条目数上限（在类型过滤与路径去重之前计数）。
	rawEntries int
	// streamBytes：gzip 解压后的字节流上限（含 tar 框架开销）。
	streamBytes int64
}

func defaultTarLimits() tarLimits {
	return tarLimits{rawEntries: maxFiles, streamBytes: maxUnpackedStream}
}

// unpackedLimitReader 给 gzip 解压后的字节流记账，超过 limit 后持续返回 errTooLarge。
// 声明体积（tarCounter）管不住「大量零长 header / 超长 PAX 头」：它们不增加声明体积，
// 但每一条都要解压、解析、分配内存，只看声明体积就等于让 CPU 白跑。
type unpackedLimitReader struct {
	r     io.Reader
	limit int64
	n     int64
	err   error
}

func (l *unpackedLimitReader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.limit {
		l.err = errTooLarge
		return n, l.err
	}
	return n, err
}

// exceeded 报告解压字节是否超限。错误可能被 gzip / tar 包了一层，用它兜底判定。
func (l *unpackedLimitReader) exceeded() bool { return l.n > l.limit }

// unpackErr 把解压流超限统一成 errTooLarge，其余解包错误保持原样加前缀。
func unpackErr(lim *unpackedLimitReader, err error) error {
	if lim.exceeded() || errors.Is(err, errTooLarge) {
		return errTooLarge
	}
	return fmt.Errorf("Unpacking failed: %w", err)
}

// tarCounter 按 tar 里的真实路径去重后累计文件数与解压体积：同名条目后者覆盖前者，
// 只算一个文件；体积取最大的那个声明，避免重复条目把上限绕过去。
type tarCounter struct {
	sizes map[string]int64
	files int
	bytes int64
}

// add 返回该路径是否第一次出现（同名条目后者覆盖前者，但只算一个文件）。
func (c *tarCounter) add(name string, size int64) bool {
	if c.sizes == nil {
		c.sizes = map[string]int64{}
	}
	if prev, ok := c.sizes[name]; ok {
		if size > prev {
			c.bytes += size - prev
			c.sizes[name] = size
		}
		return false
	}
	c.sizes[name] = size
	c.files++
	c.bytes += size
	return true
}

func (c *tarCounter) tooLarge() bool { return c.files > maxFiles || c.bytes > maxUnpackedSize }

// InspectTarball 解开 .tgz：取出 package.json、统计文件、扫描疑似密钥。
// npm 打包时根目录是 package/，个别包（如 @types/*）用别的名字，所以按第一级目录认。
//
// 出现第二个顶层目录时返回 info 与 ErrMultipleTopDirs（info 已覆盖所有顶层目录的统计与扫描，
// 供调用方诊断）；调用方必须先判错并拒收，绝不能拿这个 info 当发布依据。
func InspectTarball(r io.Reader) (*TarballInfo, error) {
	return inspectTarball(r, defaultTarLimits())
}

// inspectTarball 是 InspectTarball 的实现；lim 只在测试里注入小上限。
func inspectTarball(r io.Reader, lim tarLimits) (*TarballInfo, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("Not gzip: %w", err)
	}
	defer gz.Close()
	// 解压字节记账要在 tar 之前：header、PAX 扩展头、正文全部经它读取。
	unpacked := &unpackedLimitReader{r: gz, limit: lim.streamBytes}
	tr := tar.NewReader(unpacked)
	info := &TarballInfo{Files: map[string]bool{}, YAML: map[string][]byte{}}
	root := ""
	var counter tarCounter
	rawEntries := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, unpackErr(unpacked, err)
		}
		// 原始条目数在类型过滤之前计：同名重复条目、目录/链接等非 regular 条目
		// 同样要逐条解压解析，去重后的文件数管不住它们。
		rawEntries++
		if rawEntries > lim.rawEntries {
			return nil, errTooLarge
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
			info.TopDirs = append(info.TopDirs, top)
		} else if top != root && len(info.TopDirs) < maxTopDirs && !slices.Contains(info.TopDirs, top) {
			// 先记下第二个及以后的顶层目录，统计/扫描完再统一拒收：解压总量与密钥扫描
			// 必须覆盖全部 regular 条目，否则多出来的顶层目录就是压缩炸弹与密钥的盲区。
			info.TopDirs = append(info.TopDirs, top)
		}
		// 统计覆盖所有顶层目录；tar 允许同名条目重复出现（后一个覆盖前一个），按真实路径去重。
		counter.add(name, h.Size)
		if counter.tooLarge() {
			return nil, errTooLarge
		}
		inRoot := top == root
		file := rel
		if !inRoot {
			file = name
		}
		if len(info.Files) < 20000 {
			info.Files[file] = true
		}
		base := path.Base(rel)
		if inRoot && strings.EqualFold(base, "SKILL.md") {
			info.Skills++
			if len(info.SkillFiles) < 20 {
				info.SkillFiles = append(info.SkillFiles, rel)
			}
		}
		for _, f := range secretFiles {
			if f.match(base) {
				info.Secrets = append(info.Secrets, SecretHit{File: file, Kind: f.kind})
			}
		}
		switch {
		case inRoot && rel == "package.json":
			if h.Size > maxPackageJSON {
				return nil, errors.New("package.json is too large")
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxPackageJSON))
			if err != nil {
				return nil, unpackErr(unpacked, err)
			}
			info.PackageJSON = b
			info.scan(file, b)
		case h.Size <= maxScanFileSize && scannable(base):
			b, err := io.ReadAll(io.LimitReader(tr, maxScanFileSize))
			if err != nil {
				return nil, unpackErr(unpacked, err)
			}
			info.scan(file, b)
			if inRoot && isYAML(base) && len(info.YAML) < maxPatchFiles && info.patchBytes+int64(len(b)) <= maxPatchBytes {
				info.YAML[rel] = b
				info.patchBytes += int64(len(b))
			}
		}
	}
	info.FileCount, info.UnpackedSize = counter.files, counter.bytes
	if len(info.TopDirs) > 1 {
		tops := make([]string, 0, len(info.TopDirs))
		for _, t := range info.TopDirs {
			tops = append(tops, truncateRunes(t, 64))
		}
		return info, fmt.Errorf("%w（%s）：npm 包只能有一个顶层目录", ErrMultipleTopDirs, strings.Join(tops, "、"))
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
	if item := checkCordisDependency(m); item != nil {
		add(item.Key, item.Level, item.Params)
	}
	if item := checkCordisPeerRange(m); item != nil {
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
