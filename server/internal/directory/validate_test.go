package directory

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// 目录条目的逐条校验（docs/plugin-signing.md §2.2）：格式、身份/摘要、来源、签名声明。
// 校验不联网、不安装；目录 v1 只登记来源。

func keyID16(seed byte) string {
	return strings.Repeat(string("0123456789abcdef"[seed%16]), 16)
}

func validDoc() *Document {
	return &Document{
		Format:      FormatVersion,
		DirectoryID: "acme-prod",
		Issuer:      Issuer{Name: "ACME 平台组", KeyID: keyID16(1)},
		Entries: []Entry{{
			Kind: KindExtension, Name: "acme/keyring-itsm", Version: "1.2.0", Channel: ChannelStable,
			Digest: strings.Repeat("a", 64),
			Source: Source{Type: SourceTarball, URL: "https://cdn.example.com/keyring-1.2.0.zip"},
		}},
	}
}

// issueCodes 收集结果里的 code（可选只看某种 severity）。
func issueCodes(r *Result, severity string) map[string]bool {
	out := map[string]bool{}
	for _, is := range r.Issues {
		if severity == "" || is.Severity == severity {
			out[is.Code] = true
		}
	}
	return out
}

func TestValidateValidDocument(t *testing.T) {
	doc := sampleDoc()
	r := Validate(doc, Options{})
	if !r.OK || r.Errors != 0 || len(r.Entries) != len(doc.Entries) {
		t.Fatalf("合法目录应通过：%+v", r)
	}
	if r.Entries[0].SignatureStatus != EntrySignatureNone || r.Entries[1].SignatureStatus != EntrySignatureNone {
		t.Fatalf("没带签名时状态是 none：%+v", r.Entries)
	}
	// 同名字不同版本、不同通道可以并列
	doc.Entries = append(doc.Entries, Entry{
		Kind: KindExtension, Name: "acme/keyring-itsm", Version: "1.3.0", Channel: ChannelBeta,
		Digest: strings.Repeat("b", 64),
		Source: Source{Type: SourceTarball, URL: "https://cdn.example.com/keyring-1.3.0.zip"},
	})
	if r := Validate(doc, Options{}); !r.OK {
		t.Fatalf("不同版本/通道应允许：%+v", r.Issues)
	}
}

func TestValidateDocumentProblems(t *testing.T) {
	cases := []struct {
		name string
		doc  func(*Document)
		code string
	}{
		{"format 不是 2", func(d *Document) { d.Format = 1 }, "format.unsupported"},
		{"目录 id 不合法", func(d *Document) { d.DirectoryID = "ACME Prod" }, "directory_id.invalid"},
		{"缺签发者", func(d *Document) { d.Issuer.Name = " " }, "issuer.name_required"},
		{"key id 不合法", func(d *Document) { d.Issuer.KeyID = "xyz" }, "issuer.key_id_invalid"},
		{"没有条目", func(d *Document) { d.Entries = nil }, "entries.empty"},
		{"条目太多", func(d *Document) {
			for i := 0; i < MaxEntries; i++ {
				d.Entries = append(d.Entries, d.Entries[0])
			}
		}, "entries.too_many"},
		{"签名 key 和 issuer 不一致", func(d *Document) {
			d.Signature = &Signature{KeyID: keyID16(2)}
		}, "signature.key_id_mismatch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := validDoc()
			c.doc(doc)
			if r := Validate(doc, Options{}); r.OK || !issueCodes(r, "error")[c.code] {
				t.Fatalf("应报 %s：%+v", c.code, r.Issues)
			}
		})
	}
	// 标题太长 / 生成时间不合法只是 warning，不阻断 apply
	doc := validDoc()
	doc.Title = extension.Text{ZH: strings.Repeat("长", 201)}
	doc.GeneratedAt = "昨天"
	r := Validate(doc, Options{})
	if !r.OK || r.Warnings != 2 {
		t.Fatalf("展示性问题应是 warning：%+v", r)
	}
}

func TestValidateEntryProblems(t *testing.T) {
	sha := strings.Repeat("a", 64)
	sri := func(raw []byte) string { return "sha512-" + base64.StdEncoding.EncodeToString(raw) }
	digestOf := func(s string) string { sum := sha512.Sum512([]byte(s)); return sri(sum[:]) }
	cases := []struct {
		name     string
		mutate   func(*Entry)
		code     string
		severity string
	}{
		{"kind 不认识", func(e *Entry) { e.Kind = "git" }, "entry.kind", "error"},
		{"扩展名没有作者", func(e *Entry) { e.Name = "keyring" }, "entry.name", "error"},
		{"扩展名大写", func(e *Entry) { e.Name = "Acme/keyring" }, "entry.name", "error"},
		{"npm 包名大写", func(e *Entry) {
			*e = Entry{Kind: KindPlugin, Name: "@Acme/OA", Version: "1.0.0", Channel: ChannelStable, Digest: digestOf("x"), Source: Source{Type: SourceNPM}}
		}, "entry.name", "error"},
		{"版本不是 semver", func(e *Entry) { e.Version = "v1" }, "entry.version", "error"},
		{"通道不认识", func(e *Entry) { e.Channel = "nightly" }, "entry.channel", "error"},
		{"扩展摘要不是 sha256", func(e *Entry) { e.Digest = "abc" }, "entry.digest", "error"},
		{"扩展摘要大写", func(e *Entry) { e.Digest = strings.ToUpper(sha) }, "entry.digest", "error"},
		{"插件摘要不是 SRI", func(e *Entry) {
			*e = Entry{Kind: KindPlugin, Name: "@acme/oa", Version: "1.0.0", Channel: ChannelStable, Digest: sha, Source: Source{Type: SourceNPM}}
		}, "entry.digest", "error"},
		{"插件摘要长度不对", func(e *Entry) {
			*e = Entry{Kind: KindPlugin, Name: "@acme/oa", Version: "1.0.0", Channel: ChannelStable, Digest: sri([]byte("short")), Source: Source{Type: SourceNPM}}
		}, "entry.digest", "error"},
		{"来源类型 git", func(e *Entry) { e.Source = Source{Type: "git", URL: "https://git.example.com/r.git"} }, "entry.source.type", "error"},
		{"npm 来源带 tarball 地址", func(e *Entry) { e.Source = Source{Type: SourceNPM, URL: "https://x/y.tgz"} }, "entry.source.url", "error"},
		{"registry 不是 https", func(e *Entry) { e.Source = Source{Type: SourceNPM, Registry: "http://npm.example.com"} }, "entry.source.registry", "error"},
		{"tarball 不是 https", func(e *Entry) { e.Source = Source{Type: SourceTarball, URL: "http://cdn.example.com/x.zip"} }, "entry.source.url", "error"},
		{"tarball 是本机", func(e *Entry) { e.Source = Source{Type: SourceTarball, URL: "https://127.0.0.1/x.zip"} }, "entry.source.url", "error"},
		{"tarball 是内网域名", func(e *Entry) { e.Source = Source{Type: SourceTarball, URL: "https://npm.internal/x.zip"} }, "entry.source.url", "error"},
		{"tarball 带用户名密码", func(e *Entry) { e.Source = Source{Type: SourceTarball, URL: "https://u:p@cdn.example.com/x.zip"} }, "entry.source.url", "error"},
		{"size 是负数", func(e *Entry) { e.Size = -1 }, "entry.size", "error"},
		{"size 超过上限", func(e *Entry) { e.Size = MaxTarballSize(KindExtension) + 1 }, "entry.size", "error"},
		{"min_version 不合法", func(e *Entry) { e.MinVersion = "v2" }, "entry.min_version", "error"},
		{"说明太长", func(e *Entry) { e.Notes = strings.Repeat("x", 1001) }, "entry.notes_too_long", "warning"},
		{"发布者 id 是负数", func(e *Entry) { e.Publisher = &Publisher{UserID: -1} }, "entry.publisher.user_id", "warning"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := validDoc()
			c.mutate(&doc.Entries[0])
			r := Validate(doc, Options{})
			if c.severity == "error" && r.OK {
				t.Fatalf("应报错 %s：%+v", c.code, r.Issues)
			}
			if !issueCodes(r, c.severity)[c.code] {
				t.Fatalf("应报 %s/%s：%+v", c.severity, c.code, r.Issues)
			}
		})
	}
	// 同一身份重复出现是错误
	doc := validDoc()
	doc.Entries = append(doc.Entries, doc.Entries[0])
	if r := Validate(doc, Options{}); r.OK || !issueCodes(r, "error")["entry.duplicate"] {
		t.Fatalf("重复条目应报错：%+v", r.Issues)
	}
}

func TestValidateEntrySignature(t *testing.T) {
	pub, _ := testKey(t)
	keyID := extension.KeyID(pub)
	entryDigest := strings.Repeat("a", 64)
	validSig := func() *EntrySignature {
		raw := make([]byte, ed25519.SignatureSize)
		return &EntrySignature{
			Version: SignatureVersion, Alg: SignatureAlg, KeyID: keyID,
			Identity: "acme/keyring-itsm@1.2.0", Digest: entryDigest,
			Signature: base64.StdEncoding.EncodeToString(raw),
		}
	}
	cases := []struct {
		name     string
		mutate   func(*EntrySignature)
		code     string
		severity string
	}{
		{"签名版本不认识", func(s *EntrySignature) { s.Version = 9 }, "entry.signature.version", "error"},
		{"签名算法不认识", func(s *EntrySignature) { s.Alg = "rsa" }, "entry.signature.alg", "error"},
		{"key id 不合法", func(s *EntrySignature) { s.KeyID = "nope" }, "entry.signature.key_id", "error"},
		{"签名值不是 base64", func(s *EntrySignature) { s.Signature = "!!!" }, "entry.signature.value", "error"},
		{"身份和条目不一致", func(s *EntrySignature) { s.Identity = "acme/other@1.2.0" }, "entry.signature.identity", "error"},
		{"摘要和条目不一致", func(s *EntrySignature) { s.Digest = strings.Repeat("b", 64) }, "entry.signature.digest", "error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := validDoc()
			doc.Entries[0].Signature = validSig()
			c.mutate(doc.Entries[0].Signature)
			r := Validate(doc, Options{TrustedKeys: extension.ActiveKeys([]ed25519.PublicKey{pub})})
			if c.severity == "error" && r.OK {
				t.Fatalf("应报错：%+v", r.Issues)
			}
			if !issueCodes(r, c.severity)[c.code] {
				t.Fatalf("应报 %s：%+v", c.code, r.Issues)
			}
		})
	}

	// 签名者对得上受信任公钥：声明状态 declared + key_state active，不报 warning
	doc := validDoc()
	doc.Entries[0].Signature = validSig()
	r := Validate(doc, Options{TrustedKeys: extension.ActiveKeys([]ed25519.PublicKey{pub})})
	if !r.OK || r.Warnings != 0 || r.Entries[0].SignatureStatus != EntrySignatureDeclared || r.Entries[0].SignatureKeyState != extension.KeyStateActive {
		t.Fatalf("受信任签名者：%+v %+v", r, r.Entries)
	}
	// 没配信任根（命令行签名前）：只做结构检查，不报不受信任
	if r := Validate(doc, Options{}); !r.OK {
		t.Fatalf("没配信任根时不应因签名者报错：%+v", r.Issues)
	}
	// 未知签名者 / 撤销的签名者：warning，不阻断登记，但要让管理员看见
	r = Validate(doc, Options{TrustedKeys: []extension.TrustedKey{{Public: pub, State: extension.KeyStateRevoked}}})
	if !r.OK || !issueCodes(r, "warning")["entry.signature.revoked"] || r.Entries[0].SignatureKeyState != extension.KeyStateRevoked {
		t.Fatalf("撤销签名者应 warning：%+v %+v", r, r.Entries)
	}
	otherPub, _ := testKey(t)
	r = Validate(doc, Options{TrustedKeys: extension.ActiveKeys([]ed25519.PublicKey{otherPub})})
	if !r.OK || !issueCodes(r, "warning")["entry.signature.untrusted"] || r.Entries[0].SignatureKeyState != "unknown" {
		t.Fatalf("未知签名者应 warning：%+v %+v", r, r.Entries)
	}
	// dsh-plugin 条目的签名还没有格式：只登记声明，给 warning
	doc = validDoc()
	doc.Entries[0].Kind = KindPlugin
	doc.Entries[0].Name = "@acme/keyring"
	doc.Entries[0].Digest = testDigest("p")
	doc.Entries[0].Signature = &EntrySignature{Version: 1, Alg: SignatureAlg, KeyID: keyID, Identity: "x@1", Digest: "x", Signature: base64.StdEncoding.EncodeToString(make([]byte, 64))}
	r = Validate(doc, Options{TrustedKeys: extension.ActiveKeys([]ed25519.PublicKey{pub})})
	if !r.OK || !issueCodes(r, "warning")["entry.signature.unverified"] {
		t.Fatalf("插件签名应 warning：%+v", r.Issues)
	}
}

func TestCheckTarballURL(t *testing.T) {
	ok := func(url string, hosts []string) bool { return CheckTarballURL(url, hosts) == "" }
	if !ok("https://cdn.example.com/pkgs/oa-1.0.0.tgz", nil) {
		t.Fatal("公网 https 应通过")
	}
	if !ok("https://cdn.example.com/oa-1.0.0.tgz?X-Signature=abc&expires=1", nil) {
		t.Fatal("带签名查询参数的固定地址应通过")
	}
	blocked := []string{
		"http://cdn.example.com/x.tgz",
		"ftp://cdn.example.com/x.tgz",
		"https://user:pass@cdn.example.com/x.tgz",
		"https://127.0.0.1/x.tgz",
		"https://10.0.0.8/x.tgz",
		"https://192.168.1.1/x.tgz",
		"https://169.254.169.254/x.tgz",
		"https://[::1]/x.tgz",
		"https://localhost/x.tgz",
		"https://foo.localhost/x.tgz",
		"https://npm.internal/x.tgz",
		"https://nas.local/x.tgz",
		"https://intranet/x.tgz",
		"https://cdn.example.com/",
		"https://cdn.example.com",
		"not-a-url",
	}
	for _, u := range blocked {
		if msg := CheckTarballURL(u, nil); msg == "" {
			t.Fatalf("应拒绝 %s", u)
		}
	}
	// 白名单：只接受名单里的主机，名单里的内网地址放行
	hosts := []string{"storage.acme.internal", "*.cdn.acme.com"}
	for _, u := range []string{"https://storage.acme.internal/oa.zip", "https://x.cdn.acme.com/oa.zip"} {
		if msg := CheckTarballURL(u, hosts); msg != "" {
			t.Fatalf("白名单里的地址应通过：%s（%s）", u, msg)
		}
	}
	for _, u := range []string{"https://cdn.example.com/oa.zip", "https://acme.internal/oa.zip", "https://cdn.acme.com/oa.zip"} {
		if CheckTarballURL(u, hosts) == "" {
			t.Fatalf("白名单外的地址应拒绝：%s", u)
		}
	}
	// 大小写/末尾点：主机名按小写匹配
	if !ok("https://STORAGE.ACME.INTERNAL./oa.zip", hosts) {
		t.Fatal("主机名大小写/末尾点应跟白名单匹配")
	}
}

func TestRegistryProblem(t *testing.T) {
	if registryProblem("https://npm.acme.internal") != "" {
		t.Fatal("企业内网 registry 应允许")
	}
	if registryProblem("") != "" {
		t.Fatal("不填 registry 应允许（用默认企业源）")
	}
	for _, raw := range []string{"http://npm.example.com", "https://u:p@npm.example.com", "https://npm.example.com/?a=1", "npm"} {
		if registryProblem(raw) == "" {
			t.Fatalf("应拒绝 registry %q", raw)
		}
	}
}

// auto_upgrade=true 是合法声明，但本期不执行自动升级：校验通过 + 给一条 warning，
// 让管理员在目录校验结果里就看到这个边界。
func TestValidateAutoUpgrade(t *testing.T) {
	doc := validDoc()
	if r := Validate(doc, Options{}); !r.OK || issueCodes(r, "warning")["entry.auto_upgrade"] {
		t.Fatalf("没声明 auto_upgrade 时不该有相关提示：%+v", r)
	}
	doc.Entries[0].AutoUpgrade = true
	r := Validate(doc, Options{})
	if !r.OK {
		t.Fatalf("auto_upgrade 是合法字段，不应报错：%+v", r.Issues)
	}
	if !issueCodes(r, "warning")["entry.auto_upgrade"] || issueCodes(r, "error")["entry.auto_upgrade"] {
		t.Fatalf("auto_upgrade=true 应只给 warning：%+v", r.Issues)
	}
}
