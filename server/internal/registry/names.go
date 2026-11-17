// Package registry 是 dsh-ee 内置的 npm 源：企业作用域的托管包、上游包的回源与镜像、
// 发布审核，以及从已上架插件生成 DSH 插件索引。SQL 都在 store，这里只有协议与业务规则。
package registry

import (
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// 字符集层：新发布的包名，与 npm 现行规则、以及 DSH 索引读端的 PACKAGE_NAME 一致（全小写）。
// 只负责「读端之外再收紧」的部分，长度与连续点由 validBaseName 统一把关。
var publishName = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*$`)

// 字符集层：读取时放宽到历史包名（如 JSONStream），即允许大写和前导下划线；
// 空白与 URL 特殊字符仍然不接受。
var lookupName = regexp.MustCompile(`^(?:@[A-Za-z0-9][A-Za-z0-9._~-]*/)?[A-Za-z0-9_][A-Za-z0-9._~-]*$`)

const maxNameLength = 214

// npm 的保留名。npm 官方 validate-npm-package-name 的 exclusionList（v7.0.2）和
// npm publish 实际走的 @npmcli/package-json fixName 都硬拒绝这两个名字：
// 大小写不敏感，且只比较整名——所以 "@acme/node_modules" 这类 scoped 名是允许的。
// 核心模块名（http、fs 等）在 npm 侧只是 warning、历史包仍可读，不在这里拒绝。
// 来源：https://github.com/npm/validate-npm-package-name/blob/v7.0.2/lib/index.js
var reservedNames = []string{"node_modules", "favicon.ico"}

// isReservedName 判断是否是 npm 保留名（不区分大小写，按 npm 的规则只比较整名，
// 因此 scoped 包的 name 部分不做保留名判断）。
func isReservedName(name string) bool {
	return slices.Contains(reservedNames, strings.ToLower(name))
}

// validBaseName 是读、写两端共用的包名基线：长度上限 + 拒绝连续点（".."）+ 拒绝 npm 保留名。
//
// 连续点必须在发布端也拒绝：网页上传只走 ValidPublishName，而 npm 读、审核、上游同步
// 与索引都在用 ValidName。两边口径不一致时，@company/a..b 能发布入库、占住版本号，
// 之后 GET / 审核却永远 400 —— 既不可读也不可审。npm 存量包名里也不该有连续点
// （上游同步本来就用 ValidName 把它挡在外面），所以选「两侧一律拒绝」。
func validBaseName(name string) bool {
	return len(name) <= maxNameLength && !strings.Contains(name, "..") && !isReservedName(name)
}

// ValidPublishName 判断能否用这个名字发布新包：共享基线 + npm 新包字符集（全小写）。
func ValidPublishName(name string) bool {
	return validBaseName(name) && publishName.MatchString(name)
}

// ValidName 判断是否是合法的（含历史写法的）包名：共享基线 + 读端放宽的历史字符集。
// 发布端可以比它更严（全小写、无前导下划线），但绝不能更松。
func ValidName(name string) bool {
	return validBaseName(name) && lookupName.MatchString(name)
}

// Scope 返回 "@scope"；无作用域的包返回空串。
func Scope(name string) string {
	if !strings.HasPrefix(name, "@") {
		return ""
	}
	scope, _, _ := strings.Cut(name, "/")
	return scope
}

// TarballFile 是 npm 约定的 tarball 文件名：去掉作用域的包名-版本.tgz。
func TarballFile(name, version string) string {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}
	return base + "-" + version + ".tgz"
}

var scopeName = regexp.MustCompile(`^@[a-z0-9][a-z0-9._~-]*$`)

// ValidScope 校验 "@acme" 形式的作用域。
func ValidScope(s string) bool { return len(s) <= 64 && scopeName.MatchString(s) }

// InScopes 判断包名是否属于给定作用域之一。
func InScopes(name string, scopes []string) bool {
	sc := Scope(name)
	return sc != "" && slices.Contains(scopes, sc)
}

// ---- 版本号 ----

// 与 DSH 索引读端逐字一致的 semver 正则。
var strictSemver = func() *regexp.Regexp {
	num := `(?:0|[1-9]\d*)`
	pre := `(?:` + num + `|\d*[a-zA-Z-][0-9a-zA-Z-]*)`
	return regexp.MustCompile(`^` + num + `\.` + num + `\.` + num + `(?:-` + pre + `(?:\.` + pre + `)*)?(?:\+[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*)?$`)
}()

// ValidVersion 是发布时的版本号校验：严格 semver（与 DSH 索引读端一致，不接受 v 前缀和缺位）。
func ValidVersion(v string) bool {
	return len(v) <= 128 && strictSemver.MatchString(v)
}

// compareVersions 按 semver 比较，非法版本排在最前。
func compareVersions(a, b string) int {
	va, ea := semver.NewVersion(a)
	vb, eb := semver.NewVersion(b)
	switch {
	case ea != nil && eb != nil:
		return strings.Compare(a, b)
	case ea != nil:
		return -1
	case eb != nil:
		return 1
	}
	return va.Compare(vb)
}

// CompareVersions 按 semver 比较 a 与 b（-1 / 0 / 1），非法版本排在最前。
func CompareVersions(a, b string) int { return compareVersions(a, b) }

// SortVersions 按 semver 升序排序。
func SortVersions(vs []string) {
	slices.SortFunc(vs, compareVersions)
}

// isPrerelease 判断是否是预发布版本。
func isPrerelease(v string) bool {
	sv, err := semver.NewVersion(v)
	return err == nil && sv.Prerelease() != ""
}

// latestOf 选默认的 latest：最高的正式版，没有正式版时取最高的预发布版。
func latestOf(versions []string) string {
	best := ""
	for _, v := range versions {
		if isPrerelease(v) {
			continue
		}
		if best == "" || compareVersions(v, best) > 0 {
			best = v
		}
	}
	if best != "" {
		return best
	}
	for _, v := range versions {
		if best == "" || compareVersions(v, best) > 0 {
			best = v
		}
	}
	return best
}

// ResolveRange 按 npm 的规则从可选版本里挑一个：spec 是 dist-tag 时直接取标签；
// latest 标签满足范围时优先用它；否则取满足范围的最高版本。找不到时返回空串。
func ResolveRange(spec string, versions []string, distTags map[string]string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "*"
	}
	if v, ok := distTags[spec]; ok && slices.Contains(versions, v) {
		return v
	}
	r, ok := parseRange(spec)
	if !ok {
		return ""
	}
	if latest, ok := distTags["latest"]; ok && slices.Contains(versions, latest) {
		if lv, err := semver.NewVersion(latest); err == nil && r.satisfies(lv) {
			return latest
		}
	}
	best := ""
	var bestV *semver.Version
	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil || !r.satisfies(sv) {
			continue
		}
		if bestV == nil || sv.GreaterThan(bestV) {
			best, bestV = v, sv
		}
	}
	return best
}

// npmRange 是按 || 拆开的范围。每一段单独判断，以套用 npm 的预发布规则。
type npmRange []rangeBranch

type rangeBranch struct {
	c *semver.Constraints
	// 这一段里写到的预发布版本：npm 只允许和它们同一个 major.minor.patch 的预发布版本命中
	pre []*semver.Version
}

var versionInRange = regexp.MustCompile(`\d+\.\d+\.\d+-[0-9A-Za-z.-]+`)

func parseRange(spec string) (npmRange, bool) {
	var out npmRange
	for _, part := range strings.Split(spec, "||") {
		part = strings.TrimSpace(part)
		if part == "" {
			part = "*"
		}
		c, err := semver.NewConstraint(part)
		if err != nil {
			return nil, false
		}
		b := rangeBranch{c: c}
		for _, m := range versionInRange.FindAllString(part, -1) {
			if v, err := semver.NewVersion(m); err == nil && v.Prerelease() != "" {
				b.pre = append(b.pre, v)
			}
		}
		out = append(out, b)
	}
	return out, true
}

// satisfies 与 npm semver（不开 includePrerelease）一致：预发布版本只有在同一段里有
// 相同 major.minor.patch 的预发布比较项时才算命中。^0.2.1-rc.1 命中 0.2.1-rc.5 和 0.2.3，
// 但不命中 0.2.2-alpha.2。
func (r npmRange) satisfies(v *semver.Version) bool {
	for _, b := range r {
		if !b.c.Check(v) {
			continue
		}
		if v.Prerelease() == "" {
			return true
		}
		for _, p := range b.pre {
			if p.Major() == v.Major() && p.Minor() == v.Minor() && p.Patch() == v.Patch() {
				return true
			}
		}
	}
	return false
}
