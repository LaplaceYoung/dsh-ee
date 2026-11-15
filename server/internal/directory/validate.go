package directory

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// 企业目录的逐条校验（docs/plugin-signing.md §2.2）。这里只做「不联网」的校验：
// 格式、身份/摘要是否自洽、来源是不是允许的 npm 包名或 https tarball 地址、tarball 主机是否
// 落在白名单/公网策略里、条目内嵌的签名声明是否和身份/摘要一致、签名者是否受信任。
//
// 目录 v1 不下载 tarball、不安装、不自动升级：所以这里**不核对**内容摘要对应的实际内容，
// 下载/安装时再用现有流程（预检、安装计划、#134 的扩展验签）核对。

// Options 是校验时用到的策略。
type Options struct {
	// tarball 主机白名单（DSH_EE_DIRECTORY_TARBALL_HOSTS）。非空时只接受名单里的主机，
	// 名单里的主机可以是内网地址；为空时只接受公网 https 地址。
	TarballHosts []string
	// 受信任的签名公钥，用来核对条目里声明的签名者。nil 表示只做结构检查（命令行签名前用，
	// 那时还没有服务端的信任根）。
	TrustedKeys []extension.TrustedKey
}

// Issue 是一条校验结果。Field 是给人看的字段路径，Index 是条目下标（文档级为 -1）。
type Issue struct {
	// error / warning
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Field    string `json:"field"`
	Index    int    `json:"index"`
	Message  string `json:"message"`
}

// EntryResult 是单条校验的附加信息；Issues 里按 Index 对应。
type EntryResult struct {
	Index int `json:"index"`
	// none / declared
	SignatureStatus string `json:"signature_status"`
	SignatureKeyID  string `json:"signature_key_id,omitempty"`
	// active / retired / revoked / unknown；没配信任根或没带签名时为空
	SignatureKeyState string `json:"signature_key_state,omitempty"`
}

// Result 是整份目录的校验结果。
type Result struct {
	OK       bool          `json:"ok"`
	Errors   int           `json:"errors"`
	Warnings int           `json:"warnings"`
	Issues   []Issue       `json:"issues"`
	Entries  []EntryResult `json:"entries"`
}

func (r *Result) add(sev, code, field string, index int, msg string) {
	r.Issues = append(r.Issues, Issue{Severity: sev, Code: code, Field: field, Index: index, Message: msg})
	if sev == "error" {
		r.Errors++
	} else {
		r.Warnings++
	}
}

func (r *Result) errf(code, field string, index int, format string, args ...any) {
	r.add("error", code, field, index, fmt.Sprintf(format, args...))
}

func (r *Result) warnf(code, field string, index int, format string, args ...any) {
	r.add("warning", code, field, index, fmt.Sprintf(format, args...))
}

// Validate 校验整份目录。doc 必须是 Parse 解析出来的；这里不改 doc、不联网。
func Validate(doc *Document, opt Options) *Result {
	r := &Result{Issues: []Issue{}, Entries: []EntryResult{}}
	if doc == nil {
		r.errf("document.empty", "", -1, "目录是空的")
		r.OK = false
		return r
	}
	if doc.Format != FormatVersion {
		r.errf("format.unsupported", "format", -1, "只支持 format %d 的目录，这份是 %d", FormatVersion, doc.Format)
	}
	if !directoryIDRe.MatchString(doc.DirectoryID) {
		r.errf("directory_id.invalid", "directory_id", -1, "directory_id %q 不对：只用小写字母、数字、点、下划线、连字符，最多 64 个字符", doc.DirectoryID)
	}
	if title := doc.Title; utf8.RuneCountInString(title.Any()) > 200 {
		r.warnf("title.too_long", "title", -1, "目录标题超过 200 个字符，只影响展示")
	}
	if msg := GeneratedAtProblem(doc.GeneratedAt); msg != "" {
		r.warnf("generated_at.invalid", "generated_at", -1, "%s", msg)
	}
	if strings.TrimSpace(doc.Issuer.Name) == "" {
		r.errf("issuer.name_required", "issuer.name", -1, "目录要写签发者名称（issuer.name）")
	} else if utf8.RuneCountInString(doc.Issuer.Name) > 128 {
		r.errf("issuer.name_too_long", "issuer.name", -1, "签发者名称最多 128 个字符")
	}
	// 没签名的目录没有 issuer.key_id 可写（签名时自动填）；写了就必须是合法的 key id
	if (doc.Signature != nil || doc.Issuer.KeyID != "") && !keyIDRe.MatchString(doc.Issuer.KeyID) {
		r.errf("issuer.key_id_invalid", "issuer.key_id", -1, "issuer.key_id 要是签名公钥的 sha256 前 16 位十六进制")
	}
	if doc.Signature != nil && doc.Signature.KeyID != "" && doc.Signature.KeyID != doc.Issuer.KeyID {
		r.errf("signature.key_id_mismatch", "signature.key_id", -1, "签名里的 key_id 和 issuer.key_id 不一致")
	}
	if len(doc.Entries) == 0 {
		r.errf("entries.empty", "entries", -1, "目录里没有条目")
	}
	if len(doc.Entries) > MaxEntries {
		r.errf("entries.too_many", "entries", -1, "目录最多 %d 条，这份有 %d 条", MaxEntries, len(doc.Entries))
	}

	seen := map[string]int{}
	for i := range doc.Entries {
		e := &doc.Entries[i]
		r.Entries = append(r.Entries, EntryResult{Index: i, SignatureStatus: EntrySignatureNone})
		validateEntry(r, i, e, opt)
		// 同 kind + name + version + channel 只能出现一次；不同版本、不同通道可以并列
		key := strings.Join([]string{e.Kind, e.Name, e.Version, e.Channel}, "\x00")
		if first, dup := seen[key]; dup {
			r.errf("entry.duplicate", fmt.Sprintf("entries[%d]", i), i,
				"和 entries[%d] 重复：同一个 %s 的 %s@%s（%s）只能出现一次", first, e.Kind, e.Name, e.Version, e.Channel)
		} else {
			seen[key] = i
		}
	}
	r.OK = r.Errors == 0
	return r
}

var (
	// directory_id：一个企业可以有多份目录，各自覆盖登记
	directoryIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	// key id：公钥的 sha256 前 16 位十六进制（和 extension.KeyID 同一口径）
	keyIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// validateEntry 校验一条。Issue 都带上条目下标和字段路径，前端按条目分组显示。
func validateEntry(r *Result, i int, e *Entry, opt Options) {
	at := func(field string) string { return fmt.Sprintf("entries[%d].%s", i, field) }
	switch e.Kind {
	case KindPlugin, KindExtension:
	case "":
		r.errf("entry.kind", at("kind"), i, "条目缺少 kind：要是 %s 或 %s", KindPlugin, KindExtension)
	default:
		r.errf("entry.kind", at("kind"), i, "不认识的 kind「%s」：要是 %s 或 %s", e.Kind, KindPlugin, KindExtension)
	}

	// 名字和版本的格式
	switch e.Kind {
	case KindExtension:
		if !extension.ValidID(e.Name) {
			r.errf("entry.name", at("name"), i, "扩展名 %q 不对：要写成「作者/名字」，只用小写字母、数字和连字符", e.Name)
		}
	case KindPlugin:
		if !extension.ValidPackageName(e.Name) {
			r.errf("entry.name", at("name"), i, "npm 包名 %q 不对：只用小写字母、数字、点、下划线、连字符，可带 @作用域", e.Name)
		} else if len(e.Name) > 214 {
			r.errf("entry.name", at("name"), i, "npm 包名最多 214 个字符")
		}
	}
	if !extension.ValidVersion(e.Version) {
		r.errf("entry.version", at("version"), i, "版本 %q 不是语义化版本，例如 1.2.0（beta 可以用 1.2.0-beta.1）", e.Version)
	}
	if e.Channel != ChannelStable && e.Channel != ChannelBeta {
		r.errf("entry.channel", at("channel"), i, "通道要是 %s 或 %s", ChannelStable, ChannelBeta)
	}
	if msg := digestProblem(e.Kind, e.Digest); msg != "" {
		r.errf("entry.digest", at("digest"), i, "%s", msg)
	}

	// 来源：只允许 npm 包名或固定 https tarball 地址
	switch e.Source.Type {
	case SourceNPM:
		if e.Source.URL != "" {
			r.errf("entry.source.url", at("source.url"), i, "npm 来源不要填 tarball 地址；包名就是条目里的 name")
		}
		if msg := registryProblem(e.Source.Registry); msg != "" {
			r.errf("entry.source.registry", at("source.registry"), i, "%s", msg)
		}
	case SourceTarball:
		if e.Source.Registry != "" {
			r.errf("entry.source.registry", at("source.registry"), i, "tarball 来源不需要 registry")
		}
		if msg := CheckTarballURL(e.Source.URL, opt.TarballHosts); msg != "" {
			r.errf("entry.source.url", at("source.url"), i, "%s", msg)
		}
	case "":
		r.errf("entry.source.type", at("source.type"), i, "条目缺少 source.type：要是 %s 或 %s", SourceNPM, SourceTarball)
	default:
		r.errf("entry.source.type", at("source.type"), i, "不认识的来源类型「%s」：只允许 npm 包名或固定 tarball 地址（不做 Git 安装）", e.Source.Type)
	}
	if e.Size < 0 {
		r.errf("entry.size", at("size"), i, "size 不能是负数")
	} else if e.Size > 0 && e.Size > MaxTarballSize(e.Kind) {
		r.errf("entry.size", at("size"), i, "声明大小 %d 字节超过 %s 的上限 %d 字节", e.Size, e.Kind, MaxTarballSize(e.Kind))
	}
	if utf8.RuneCountInString(e.Notes) > 1000 {
		r.warnf("entry.notes_too_long", at("notes"), i, "说明超过 1000 个字符，只保留展示用；建议写短一点")
	}
	if e.MinVersion != "" && !extension.ValidVersion(e.MinVersion) {
		r.errf("entry.min_version", at("min_version"), i, "min_version %q 不是语义化版本", e.MinVersion)
	}
	// auto_upgrade 是可选声明（format 2 additive）。本期只登记、只做只读更新检查，
	// 不会真的自动升级；这里给 warning 让管理员在目录校验结果里就看到这个边界。
	if e.AutoUpgrade {
		r.warnf("entry.auto_upgrade", at("auto_upgrade"), i, "auto_upgrade=true 只登记、不执行：本期 dsh-ee 不会自动升级，升级仍要管理员显式触发")
	}
	if p := e.Publisher; p != nil {
		if p.UserID < 0 {
			r.warnf("entry.publisher.user_id", at("publisher.user_id"), i, "publisher.user_id 不能是负数")
		}
		if utf8.RuneCountInString(p.Username) > 128 || utf8.RuneCountInString(p.Department) > 128 {
			r.warnf("entry.publisher.too_long", at("publisher"), i, "publisher 的用户名/部门超过 128 个字符")
		}
	}
	validateEntrySignature(r, i, e, opt)
}

// validateEntrySignature 校验条目内嵌的签名声明。目录里没有扩展内容，所以只核对：
// 形状、身份和摘要是否和条目一致、签名者是否在受信任公钥里。签名本身在安装时核对。
func validateEntrySignature(r *Result, i int, e *Entry, opt Options) {
	sig := e.Signature
	er := &r.Entries[i]
	if sig == nil {
		er.SignatureStatus = EntrySignatureNone
		return
	}
	er.SignatureStatus = EntrySignatureDeclared
	er.SignatureKeyID = sig.KeyID
	at := func(field string) string { return fmt.Sprintf("entries[%d].signature.%s", i, field) }
	switch {
	case sig.Version != SignatureVersion:
		r.errf("entry.signature.version", at("version"), i, "不认识的签名版本 %d：dsh-ee 现在只认 %d", sig.Version, SignatureVersion)
	case sig.Alg != SignatureAlg:
		r.errf("entry.signature.alg", at("alg"), i, "不认识的签名算法「%s」：dsh-ee 只认 %s", sig.Alg, SignatureAlg)
	case !keyIDRe.MatchString(sig.KeyID):
		r.errf("entry.signature.key_id", at("key_id"), i, "key_id 要是签名公钥的 sha256 前 16 位十六进制")
	case sig.Identity == "" || sig.Digest == "" || sig.Signature == "":
		r.errf("entry.signature.fields", at(""), i, "签名缺少 identity、digest 或 signature 字段")
	case !validBase64Signature(sig.Signature):
		r.errf("entry.signature.value", at("signature"), i, "签名不是合法的 base64 或长度不对")
	}
	if e.Kind == KindExtension {
		if want := e.Name + "@" + e.Version; sig.Identity != "" && sig.Identity != want {
			r.errf("entry.signature.identity", at("identity"), i, "签名是给 %s 的，这条是 %s", sig.Identity, want)
		}
		if sig.Digest != "" && sig.Digest != e.Digest {
			r.errf("entry.signature.digest", at("digest"), i, "签名对的摘要和条目的 digest 不一致")
		}
	} else if e.Kind == KindPlugin {
		r.warnf("entry.signature.unverified", at(""), i, "dsh-plugin 包的签名还没有定义格式：这里只登记声明，安装/审核时不据此验签")
	}
	if opt.TrustedKeys == nil || !keyIDRe.MatchString(sig.KeyID) {
		return
	}
	state := trustedKeyState(sig.KeyID, opt.TrustedKeys)
	er.SignatureKeyState = state
	switch state {
	case "unknown":
		r.warnf("entry.signature.untrusted", at("key_id"), i, "签名者 %s 不在受信任的签名公钥里：目录只能核对声明，安装时按签名策略处理", sig.KeyID)
	case extension.KeyStateRevoked:
		r.warnf("entry.signature.revoked", at("key_id"), i, "签名者 %s 的公钥已撤销：这个版本的签名不能再用来新装，建议从目录里去掉", sig.KeyID)
	}
}

// trustedKeyState 在受信任公钥里找 key id，返回 active / retired / revoked / unknown。
func trustedKeyState(keyID string, keys []extension.TrustedKey) string {
	for _, k := range keys {
		if len(k.Public) != 32 { // ed25519.PublicKeySize
			continue
		}
		if extension.KeyID(k.Public) != keyID {
			continue
		}
		if k.State == "" {
			return extension.KeyStateActive
		}
		return k.State
	}
	return "unknown"
}

// digestProblem 检查条目摘要的格式：扩展是 sha256 十六进制，插件是 npm 的 sha512 SRI。
func digestProblem(kind, digest string) string {
	digest = strings.TrimSpace(digest)
	switch {
	case digest == "":
		return "digest 必填：按条目类型写扩展的 sha256 或 npm 的 sha512 SRI"
	case kind == KindPlugin:
		rest, ok := strings.CutPrefix(digest, "sha512-")
		if !ok {
			return "dsh-plugin 条目的 digest 要写成 npm 的 sha512 SRI：sha512-<base64>"
		}
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil || len(raw) != 64 {
			return "digest 的 sha512 部分不是合法的 base64 或长度不对"
		}
		return ""
	case kind == KindExtension:
		if len(digest) != 64 || !isLowerHex(digest) {
			return "扩展条目的 digest 要是清单（声明式连同接口文档）的 sha256，64 位小写十六进制"
		}
		return ""
	}
	return ""
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validBase64Signature(sig string) bool {
	raw, err := base64.StdEncoding.DecodeString(sig)
	return err == nil && len(raw) == 64 // ed25519.SignatureSize
}

// registryProblem 检查 npm registry 地址：只接受 https、不带用户名密码。企业内网 registry
// （https://npm.acme.internal）是正常的，所以这里不套公网/内网限制——目录 v1 只登记地址，
// 真正抓包的是现有的企业源/上游流程。
func registryProblem(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > 512 {
		return "registry 地址太长（最多 512 个字符）"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "registry 不是合法的地址：" + err.Error()
	}
	if u.Scheme != "https" {
		return "registry 必须用 https://"
	}
	if u.Hostname() == "" {
		return "registry 缺少主机名"
	}
	if u.User != nil {
		return "registry 地址里不要带用户名密码"
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "registry 地址不要带查询参数或锚点"
	}
	return ""
}

// CheckTarballURL 检查 tarball 地址是不是允许的固定 https 地址，返回要显示的问题（空表示通过）。
//
// hosts 是 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单（支持 *.example.com）：
//   - 不配白名单：只接受公网 https 主机，拒绝 localhost、内网/链路本地/保留地址和常见内网域名；
//   - 配了白名单：只接受名单里的主机，但名单里的主机允许是内网地址（企业自己的对象存储）。
//
// 目录 v1 不会去下载这个地址；这里的限制是纵深防御，防止以后接下载/安装时把内网地址带进去。
func CheckTarballURL(raw string, hosts []string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "tarball 来源要填固定地址 URL"
	}
	if len(raw) > 2048 {
		return "tarball 地址太长（最多 2048 个字符）"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "tarball 地址不是合法的 URL：" + err.Error()
	}
	if u.Scheme != "https" {
		return "tarball 地址必须用 https://"
	}
	if u.User != nil {
		return "tarball 地址里不要带用户名密码"
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "tarball 地址缺少主机名"
	}
	if u.Path == "" || u.Path == "/" {
		return "tarball 地址要指向具体文件"
	}
	if len(hosts) > 0 {
		if !hostAllowlisted(host, hosts) {
			return fmt.Sprintf("tarball 主机 %s 不在 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单里", host)
		}
		return ""
	}
	if msg := publicHostProblem(host); msg != "" {
		return msg
	}
	return ""
}

// ParseTarballHosts 解析 tarball 主机白名单（DSH_EE_DIRECTORY_TARBALL_HOSTS）：逗号分隔的主机名，
// 可写 *.<后缀>。主机名统一小写；不允许把协议/路径/端口写进来。
func ParseTarballHosts(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		p := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(part), "."))
		if p == "" {
			continue
		}
		if len(p) > 253 {
			return nil, fmt.Errorf("主机名太长：%q", p)
		}
		base := strings.TrimPrefix(p, "*.")
		if base == p && strings.Contains(p, "*") {
			return nil, fmt.Errorf("%q 不合法：通配只能写成 *.example.com", p)
		}
		for _, label := range strings.Split(base, ".") {
			if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return nil, fmt.Errorf("%q 不是合法的主机名", p)
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
					return nil, fmt.Errorf("%q 不是合法的主机名：只允许字母、数字、连字符和点", p)
				}
			}
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) > 32 {
		return nil, fmt.Errorf("白名单最多 32 个主机")
	}
	return out, nil
}

// hostAllowlisted 按精确主机名或 *.后缀 匹配白名单。空白名单里的项在配置阶段已经校验过。
func hostAllowlisted(host string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(p), "."))
		if p == "" {
			continue
		}
		if suffix, ok := strings.CutPrefix(p, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) && len(host) > len(suffix)+1 {
				return true
			}
			continue
		}
		if host == p {
			return true
		}
	}
	return false
}

// PublicHostProblem 是 publicHostProblem 的导出形式：安装 / 下载扩展包时按实际解析到的 IP
// 再核一遍，防止域名解析到内网（企业目录的 tarball 地址虽然登记时校验过，下载前仍要再确认）。
func PublicHostProblem(host string) string { return publicHostProblem(host) }

// publicHostProblem 检查主机名是不是「公网才接受」的：拒绝 IP 字面量里的内网/回环/链路本地/
// 保留地址、localhost 和常见内网域名、单段主机名。
func publicHostProblem(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsLoopback():
			return "tarball 主机 " + host + " 是本机回环地址，不能用"
		case ip.IsPrivate():
			return "tarball 主机 " + host + " 是内网地址；内网地址要显式列进 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单"
		case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
			return "tarball 主机 " + host + " 是链路本地地址，不能用"
		case ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast():
			return "tarball 主机 " + host + " 不是单播公网地址，不能用"
		}
		return ""
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "tarball 主机 " + host + " 是本机地址，不能用"
	}
	for _, suffix := range []string{".local", ".internal", ".home.arpa", ".lan", ".corp", ".localdomain"} {
		if strings.HasSuffix(host, suffix) || host == strings.TrimPrefix(suffix, ".") {
			return "tarball 主机 " + host + " 是内网域名；内网地址要显式列进 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单"
		}
	}
	if !strings.Contains(host, ".") {
		return "tarball 主机 " + host + " 是单段主机名（通常是内网地址）；内网地址要显式列进 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单"
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "tarball 主机名 " + host + " 不合法"
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
				return "tarball 主机名 " + host + " 不合法"
			}
		}
	}
	return ""
}
