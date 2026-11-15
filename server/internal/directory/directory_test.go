package directory

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// 企业目录 format 2 的签名/验签（docs/plugin-signing.md §2.2）：目录整体 Ed25519 签名，
// 签的是除 signature 之外的 canonical JSON；重新缩进不影响验签，改内容/换身份/换目录都验不过。

func testDigest(seed string) string {
	sum := sha512.Sum512([]byte(seed))
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func sampleDoc() *Document {
	return &Document{
		Format:      FormatVersion,
		DirectoryID: "acme-prod",
		Title:       extension.Text{ZH: "ACME 企业扩展目录", EN: "ACME extension directory"},
		GeneratedAt: "2026-11-13T08:00:00Z",
		Issuer:      Issuer{Name: "ACME 平台组"},
		Entries: []Entry{
			{
				Kind: KindExtension, Name: "acme/keyring-itsm", Version: "1.2.0", Channel: ChannelStable,
				Digest:    strings.Repeat("a", 64),
				Source:    Source{Type: SourceTarball, URL: "https://cdn.example.com/keyring-1.2.0.zip"},
				Publisher: &Publisher{UserID: 7, Username: "alice", Department: "平台组"},
				Notes:     "审核过的工单扩展",
			},
			{
				Kind: KindPlugin, Name: "@acme/oa-plugin", Version: "2.0.0-beta.1", Channel: ChannelBeta,
				Digest: testDigest("oa"),
				Source: Source{Type: SourceNPM, Registry: "https://npm.example.com"},
			},
		},
	}
}

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func signDoc(t *testing.T, doc *Document, priv ed25519.PrivateKey) {
	t.Helper()
	if err := SignDocument(doc, priv); err != nil {
		t.Fatalf("签名：%v", err)
	}
}

func verify(doc *Document, pubs ...ed25519.PublicKey) *VerifyResult {
	return VerifyDocument(doc, extension.ActiveKeys(pubs))
}

func TestSignAndVerify(t *testing.T) {
	pub, priv := testKey(t)
	doc := sampleDoc()
	signDoc(t, doc, priv)

	if doc.Signature == nil || doc.Issuer.KeyID != extension.KeyID(pub) {
		t.Fatalf("签名要带 key id 并自动填 issuer.key_id：%+v", doc.Signature)
	}
	payload, err := CanonicalPayload(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Signature.Digest != PayloadDigest(payload) || doc.Signature.DirectoryID != doc.DirectoryID || doc.Signature.Alg != SignatureAlg {
		t.Fatalf("签名字段不对：%+v", doc.Signature)
	}
	res := verify(doc, pub)
	if res.Status != SignatureValid || res.KeyState != extension.KeyStateActive || res.KeyID != extension.KeyID(pub) || res.Digest != doc.Signature.Digest {
		t.Fatalf("验签结果：%+v", res)
	}

	// 重新缩进 / 改空白不影响验签：签名对 canonical payload，不是原始文件字节
	raw, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(append([]byte("\n  "), raw...))
	if err != nil {
		t.Fatal(err)
	}
	if res := verify(reparsed, pub); res.Status != SignatureValid {
		t.Fatalf("重新缩进后应仍有效：%+v", res)
	}
}

func TestVerifyRejectsTamper(t *testing.T) {
	pub, priv := testKey(t)
	cases := []struct {
		name   string
		tamper func(doc *Document)
	}{
		{"改条目说明", func(doc *Document) { doc.Entries[0].Notes = "被改过" }},
		{"改条目摘要", func(doc *Document) { doc.Entries[1].Digest = testDigest("changed") }},
		{"改签名者名称", func(doc *Document) { doc.Issuer.Name = "别人" }},
		{"改生成时间", func(doc *Document) { doc.GeneratedAt = "2027-01-01T00:00:00Z" }},
		{"加一条", func(doc *Document) {
			doc.Entries = append(doc.Entries, Entry{
				Kind: KindPlugin, Name: "@acme/evil", Version: "9.9.9", Channel: ChannelStable,
				Digest: testDigest("evil"), Source: Source{Type: SourceNPM},
			})
		}},
		{"改签名值", func(doc *Document) {
			raw, _ := base64.StdEncoding.DecodeString(doc.Signature.Signature)
			raw[0] ^= 0xff
			doc.Signature.Signature = base64.StdEncoding.EncodeToString(raw)
		}},
		{"改签名的 digest", func(doc *Document) { doc.Signature.Digest = strings.Repeat("b", 64) }},
		{"换目录 id", func(doc *Document) {
			// signature.directory_id 和文档对不上
			doc.DirectoryID = "acme-test"
		}},
		{"issuer 和签名 key 不一致", func(doc *Document) { doc.Issuer.KeyID = strings.Repeat("0", 16) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := sampleDoc()
			signDoc(t, doc, priv)
			c.tamper(doc)
			if res := verify(doc, pub); res.Status != SignatureInvalid {
				t.Fatalf("篡改后应 invalid：%+v", res)
			}
		})
	}
	// 去掉签名：unsigned，不能登记
	doc := sampleDoc()
	if res := verify(doc, pub); res.Status != SignatureUnsigned {
		t.Fatalf("没签名应 unsigned：%+v", res)
	}
}

func TestVerifyKeyStates(t *testing.T) {
	pub, priv := testKey(t)
	otherPub, _ := testKey(t)
	doc := sampleDoc()
	signDoc(t, doc, priv)

	// 未知签名者
	if res := verify(doc, otherPub); res.Status != SignatureInvalid || !strings.Contains(res.Message, "不受信任") {
		t.Fatalf("未知签名者应 invalid：%+v", res)
	}
	// 一把受信任钥匙都没有
	if res := VerifyDocument(doc, nil); res.Status != SignatureInvalid || !strings.Contains(res.Message, "没有配置") {
		t.Fatalf("没有信任根应 invalid：%+v", res)
	}
	// 换下：仍有效，带 key_state
	keys := []extension.TrustedKey{{Public: pub, State: extension.KeyStateRetired}}
	if res := VerifyDocument(doc, keys); res.Status != SignatureValid || res.KeyState != extension.KeyStateRetired {
		t.Fatalf("换下的钥匙仍认历史签名：%+v", res)
	}
	// 撤销：签名对得上，但状态是 revoked
	keys = []extension.TrustedKey{{Public: pub, State: extension.KeyStateRevoked}}
	if res := VerifyDocument(doc, keys); res.Status != SignatureRevoked || res.KeyState != extension.KeyStateRevoked {
		t.Fatalf("撤销的钥匙应 revoked：%+v", res)
	}
}

func TestSignDocumentRejectsWrongKey(t *testing.T) {
	_, priv := testKey(t)
	otherPub, _ := testKey(t)
	doc := sampleDoc()
	doc.Issuer.KeyID = extension.KeyID(otherPub)
	if err := SignDocument(doc, priv); err == nil || !strings.Contains(err.Error(), "不是同一把") {
		t.Fatalf("issuer.key_id 和私钥不一致时应拒绝签名：%v", err)
	}
	// 不是 Ed25519 私钥
	if err := SignDocument(sampleDoc(), ed25519.PrivateKey(make([]byte, 10))); err == nil {
		t.Fatal("长度不对的私钥应拒绝")
	}
}

func TestParseStrict(t *testing.T) {
	for name, raw := range map[string]string{
		"空":       "   ",
		"不是对象":    "[]",
		"未知字段":    `{"format":2,"directory_id":"acme","issuer":{"name":"x","key_id":"` + strings.Repeat("a", 16) + `"},"entries":[],"unknown":1}`,
		"后面还有内容":  `{"format":2,"directory_id":"acme","issuer":{"name":"x","key_id":"` + strings.Repeat("a", 16) + `"},"entries":[]} {}`,
		"不是 JSON": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatalf("应拒绝：%s", raw)
			}
		})
	}
	if _, err := Parse(bytes.Repeat([]byte(" "), MaxDocumentBytes+1)); err == nil {
		t.Fatal("超过大小上限应拒绝")
	}
}

// auto_upgrade 是 format 2 的 additive 字段（本期只登记 / 只做只读更新检查，不执行自动升级）：
// 旧目录（没有这个字段）的签名 / 验签不受影响；显式 false 与不写等价；写成 true 才进
// canonical payload，改它会让签名对不上。
func TestAutoUpgradeAdditive(t *testing.T) {
	pub, priv := testKey(t)

	// 旧目录：不写 auto_upgrade，照常签名 / 验签，序列化结果里也不出现这个字段
	old := sampleDoc()
	signDoc(t, old, priv)
	oldJSON, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(oldJSON, []byte("auto_upgrade")) {
		t.Fatalf("没声明 auto_upgrade 时不应写进 JSON：%s", oldJSON)
	}
	reparsed, err := Parse(oldJSON)
	if err != nil {
		t.Fatalf("旧目录应能解析：%v", err)
	}
	if res := verify(reparsed, pub); res.Status != SignatureValid {
		t.Fatalf("旧目录验签应有效：%+v", res)
	}

	// 显式写 false 与不写等价：canonical payload 一样，旧签名仍然对得上
	withFalseJSON := bytes.Replace(oldJSON, []byte(`"digest":`), []byte(`"auto_upgrade":false,"digest":`), 1)
	withFalse, err := Parse(withFalseJSON)
	if err != nil {
		t.Fatalf("带 auto_upgrade:false 的目录应能解析：%v", err)
	}
	if withFalse.Entries[0].AutoUpgrade {
		t.Fatal("auto_upgrade:false 应解析成 false")
	}
	p1, _ := CanonicalPayload(reparsed)
	p2, _ := CanonicalPayload(withFalse)
	if !bytes.Equal(p1, p2) {
		t.Fatalf("auto_upgrade:false 不应改变 canonical payload：\n%s\n%s", p1, p2)
	}
	if res := verify(withFalse, pub); res.Status != SignatureValid {
		t.Fatalf("加了 auto_upgrade:false 后旧签名仍应有效：%+v", res)
	}

	// 声明 true：进 canonical payload，签名有效；把声明改掉签名就不过
	withTrue := sampleDoc()
	withTrue.Entries[0].AutoUpgrade = true
	signDoc(t, withTrue, priv)
	trueJSON, err := json.Marshal(withTrue)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(trueJSON, []byte(`"auto_upgrade":true`)) {
		t.Fatalf("声明 true 应写进 JSON：%s", trueJSON)
	}
	parsedTrue, err := Parse(trueJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !parsedTrue.Entries[0].AutoUpgrade {
		t.Fatal("auto_upgrade:true 应解析成 true")
	}
	if res := verify(parsedTrue, pub); res.Status != SignatureValid {
		t.Fatalf("声明 auto_upgrade:true 后验签应有效：%+v", res)
	}
	parsedTrue.Entries[0].AutoUpgrade = false
	if res := verify(parsedTrue, pub); res.Status != SignatureInvalid {
		t.Fatalf("改掉 auto_upgrade 声明后签名应对不上：%+v", res)
	}

	// 类型不对要拒绝，不能默默当成 false
	badJSON := bytes.Replace(oldJSON, []byte(`"digest":`), []byte(`"auto_upgrade":"yes","digest":`), 1)
	if _, err := Parse(badJSON); err == nil {
		t.Fatal("auto_upgrade 不是布尔值时应拒绝解析")
	}
}
