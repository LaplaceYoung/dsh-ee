package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业目录 v1（docs/plugin-signing.md §2.2、task-57）：目录整体验签 + 逐条校验 + 登记。
// 覆盖：签名有效 / 篡改 / 未知 key / 条目非法 / 重复 / 权限 / apply 幂等 / 敏感变化确认 /
// tarball 白名单（SSRF）/ 只登记不安装 / 审计。

type dirPlan struct {
	OK       bool `json:"ok"`
	Errors   int  `json:"errors"`
	Warnings int  `json:"warnings"`
	Document struct {
		Format         int    `json:"format"`
		DirectoryID    string `json:"directory_id"`
		DocumentDigest string `json:"document_digest"`
		Signature      *struct {
			Status   string `json:"status"`
			KeyID    string `json:"key_id"`
			KeyState string `json:"key_state"`
			Message  string `json:"message"`
		} `json:"signature"`
	} `json:"document"`
	Entries []struct {
		Index       int    `json:"index"`
		ID          string `json:"id"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Version     string `json:"version"`
		AutoUpgrade bool   `json:"auto_upgrade"`
		Issues      []struct {
			Severity string `json:"severity"`
			Code     string `json:"code"`
		} `json:"issues"`
		Signature *struct {
			Status   string `json:"status"`
			KeyID    string `json:"key_id"`
			KeyState string `json:"key_state"`
		} `json:"signature"`
	} `json:"entries"`
	Issues []struct {
		Severity string `json:"severity"`
		Code     string `json:"code"`
	} `json:"issues"`
	Diff *struct {
		Created   int `json:"created"`
		Updated   int `json:"updated"`
		Unchanged int `json:"unchanged"`
		Removed   int `json:"removed"`
	} `json:"diff"`
	Sensitive []string `json:"sensitive"`
}

type dirApplyResp struct {
	OK             bool   `json:"ok"`
	DirectoryID    string `json:"directory_id"`
	DocumentDigest string `json:"document_digest"`
	Result         struct {
		Created   int `json:"created"`
		Updated   int `json:"updated"`
		Unchanged int `json:"unchanged"`
		Removed   int `json:"removed"`
	} `json:"result"`
	Confirmed []string               `json:"confirmed"`
	Entries   []store.DirectoryEntry `json:"entries"`
}

type dirListResp struct {
	Directories  []store.DirectorySummary `json:"directories"`
	Entries      []store.DirectoryEntry   `json:"entries"`
	Count        int                      `json:"count"`
	TarballHosts []string                 `json:"tarball_hosts"`
}

func dirPEM(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	raw, err := extension.MarshalPublicKeyPEM(pub)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// signedDirJSON 用私钥签一份目录，返回贴进接口的 JSON 文本。
func signedDirJSON(t *testing.T, doc *directory.Document, priv ed25519.PrivateKey) string {
	t.Helper()
	if err := directory.SignDocument(doc, priv); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func unsignedDirJSON(t *testing.T, doc *directory.Document) string {
	t.Helper()
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func pluginSRI(t *testing.T, seed string) string {
	t.Helper()
	sum := sha512.Sum512([]byte(seed))
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

// dirEntrySig 造一个扩展条目的内嵌签名声明（目录里没有内容，只核对声明）。
func dirEntrySig(name, version, digest, keyID string) *directory.EntrySignature {
	return &directory.EntrySignature{
		Version: 1, Alg: directory.SignatureAlg, KeyID: keyID,
		Identity: name + "@" + version, Digest: digest,
		Signature: base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
	}
}

func testDirectory(t *testing.T, entries []directory.Entry) *directory.Document {
	t.Helper()
	return &directory.Document{
		Format:      directory.FormatVersion,
		DirectoryID: "acme-prod",
		Title:       extension.Text{ZH: "ACME 企业扩展目录", EN: "ACME extension directory"},
		GeneratedAt: "2026-11-13T08:00:00Z",
		Issuer:      directory.Issuer{Name: "ACME 平台组"},
		Entries:     entries,
	}
}

var (
	extDigest1 = strings.Repeat("a", 64)
	extDigest2 = strings.Repeat("b", 64)
)

// testDirectoryEntries 是三条合法条目：声明式扩展、带签名声明的扩展、npm 插件。
func testDirectoryEntries(t *testing.T, withSig bool, sigKeyID string) []directory.Entry {
	pluginDigest := pluginSRI(t, "oa")
	entries := []directory.Entry{
		{
			Kind: directory.KindExtension, Name: "acme/keyring-itsm", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest:    extDigest1,
			Source:    directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/keyring-1.0.0.zip"},
			Publisher: &directory.Publisher{Username: "alice", Department: "平台组"},
			Notes:     "审核过的工单扩展",
		},
		{
			Kind: directory.KindExtension, Name: "acme/audit-log", Version: "2.0.0", Channel: directory.ChannelBeta,
			Digest: extDigest2,
			Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/audit-2.0.0.zip"},
		},
		{
			Kind: directory.KindPlugin, Name: "@acme/oa-plugin", Version: "2.0.0-beta.1", Channel: directory.ChannelBeta,
			Digest: pluginDigest,
			Source: directory.Source{Type: directory.SourceNPM, Registry: "https://npm.acme.internal"},
		},
	}
	if withSig {
		entries[1].Signature = dirEntrySig("acme/audit-log", "2.0.0", extDigest2, sigKeyID)
	}
	return entries
}

func hasIssueCode(issues []struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
}, code string) bool {
	for _, is := range issues {
		if is.Code == code {
			return true
		}
	}
	return false
}

func TestEnterpriseDirectory(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, logger)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	dirPub, dirPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	extPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entrySigKeyID := extension.KeyID(extPub)

	check := func(t *testing.T, document string) dirPlan {
		t.Helper()
		code, body := admin.do("POST", "/api/admin/directory/check", map[string]any{"document": document}, nil)
		expect(t, "目录 check", code, 200, body)
		return decodeAs[dirPlan](t, body)
	}
	apply := func(t *testing.T, document string, confirm bool) (int, []byte) {
		t.Helper()
		return admin.do("POST", "/api/admin/directory/apply", map[string]any{"document": document, "confirm": confirm}, nil)
	}
	list := func(t *testing.T) dirListResp {
		t.Helper()
		code, body := admin.do("GET", "/api/admin/directory", nil, nil)
		expect(t, "目录列表", code, 200, body)
		return decodeAs[dirListResp](t, body)
	}

	// 1. 没有签名：不能登记（企业目录要求整体签名）
	doc := testDirectory(t, testDirectoryEntries(t, true, entrySigKeyID))
	p := check(t, unsignedDirJSON(t, doc))
	if p.OK || p.Document.Signature == nil || p.Document.Signature.Status != directory.SignatureUnsigned || !hasIssueCode(p.Issues, "signature.required") {
		t.Fatalf("没签名应不通过：%+v", p)
	}

	// 2. 加目录签名公钥（purpose=directory），签过名的目录通过
	code, body = admin.do("POST", "/api/admin/signing-keys", map[string]any{"public_key": dirPEM(t, dirPub), "purpose": "directory", "note": "企业目录签名"}, nil)
	expect(t, "加目录签名公钥", code, 201, body)
	if got := decodeAs[store.TrustedSigningKey](t, body); got.Purpose != "directory" || got.KeyID != extension.KeyID(dirPub) {
		t.Fatalf("公钥用途不对：%+v", got)
	}
	signed := signedDirJSON(t, doc, dirPriv)
	p = check(t, signed)
	if !p.OK || p.Document.Signature == nil || p.Document.Signature.Status != directory.SignatureValid || p.Document.Signature.KeyState != extension.KeyStateActive || p.Document.DirectoryID != "acme-prod" || p.Document.DocumentDigest == "" {
		t.Fatalf("签过名的目录应通过：%+v", p)
	}
	if len(p.Entries) != 3 || p.Entries[2].Kind != directory.KindPlugin || p.Entries[1].Signature == nil || p.Entries[1].Signature.Status != directory.EntrySignatureDeclared || p.Entries[1].Signature.KeyState != "unknown" {
		t.Fatalf("条目信息不对：%+v", p.Entries)
	}
	if !hasIssueCode(p.Issues, "entry.signature.untrusted") {
		t.Fatalf("条目签名者还没加信任根，应提示 untrusted：%+v", p.Issues)
	}

	// 3. 篡改内容：签名对不上
	tampered := doc
	tampered.Entries = append([]directory.Entry{}, doc.Entries...)
	tampered.Entries[0].Notes = "偷偷改的说明"
	p = check(t, unsignedDirJSON(t, tampered))
	if p.OK || p.Document.Signature == nil || p.Document.Signature.Status != directory.SignatureInvalid || !hasIssueCode(p.Issues, "signature.invalid") {
		t.Fatalf("篡改后应 invalid：%+v", p)
	}

	// 4. 未知签名 key：不受信任
	unknown := testDirectory(t, testDirectoryEntries(t, false, ""))
	p = check(t, signedDirJSON(t, unknown, otherPriv))
	if p.OK || p.Document.Signature == nil || p.Document.Signature.Status != directory.SignatureInvalid || !strings.Contains(p.Document.Signature.Message, "不受信任") {
		t.Fatalf("未知 key 应不通过：%+v", p)
	}

	// 5. 非法条目 + 重复条目：逐条报错
	invalid := testDirectory(t, []directory.Entry{
		{ // 扩展名没有作者
			Kind: directory.KindExtension, Name: "keyring", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/x.zip"},
		},
		{ // npm 包名大写 + 摘要不是 sha512 SRI
			Kind: directory.KindPlugin, Name: "@Acme/OA", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // http tarball
			Kind: directory.KindExtension, Name: "acme/http", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "http://cdn.example.com/x.zip"},
		},
		{ // 本机地址（SSRF）
			Kind: directory.KindExtension, Name: "acme/local", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://127.0.0.1/x.zip"},
		},
		{ // 重复条目
			Kind: directory.KindExtension, Name: "acme/keyring-itsm", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/keyring-1.0.0.zip"},
		},
		{ // 和上面完全相同，触发 duplicate
			Kind: directory.KindExtension, Name: "acme/keyring-itsm", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/keyring-1.0.0.zip"},
		},
	})
	p = check(t, signedDirJSON(t, invalid, dirPriv))
	if p.OK || p.Errors == 0 {
		t.Fatalf("非法条目应不通过：%+v", p)
	}
	for _, want := range []string{"entry.name", "entry.digest", "entry.source.url", "entry.duplicate"} {
		if !hasIssueCode(p.Issues, want) {
			t.Fatalf("应报 %s：%+v", want, p.Issues)
		}
	}
	// 非法目录 apply 400，且什么都没有登记
	code, body = apply(t, signedDirJSON(t, invalid, dirPriv), true)
	expect(t, "非法目录不能登记", code, 400, body)
	if got := list(t); got.Count != 0 || len(got.Entries) != 0 {
		t.Fatalf("非法目录不应登记：%+v", got)
	}

	// 6. 权限：普通员工不能看/check/apply；connections.manage 或 settings.manage 可以
	code, body = admin.do("GET", "/api/admin/roles", nil, nil)
	expect(t, "角色列表", code, 200, body)
	var memberID int64
	for _, r := range decodeAs[struct{ Roles []store.Role }](t, body).Roles {
		if r.Code == rbac.RoleMember {
			memberID = r.ID
		}
	}
	if memberID == 0 {
		t.Fatal("找不到普通员工角色")
	}
	newRole := func(code, name, perm string) int64 {
		t.Helper()
		code2, body := admin.do("POST", "/api/admin/roles", map[string]any{"code": code, "name": name, "permissions": []string{perm}}, nil)
		expect(t, "建角色 "+code, code2, 201, body)
		return decodeAs[store.Role](t, body).ID
	}
	newUser := func(name string, roles []int64) *client {
		t.Helper()
		code, body := admin.do("POST", "/api/admin/users", map[string]any{
			"username": name, "password": "directory-pass-1", "department": "研发效能部", "role_ids": roles,
		}, nil)
		expect(t, "建用户 "+name, code, 201, body)
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": name, "password": "directory-pass-1"}, nil)
		expect(t, name+" 登录", code, 200, body)
		return c
	}
	settingsRole := newRole("directory-settings", "企业目录（设置）", rbac.SettingsManage)
	connRole := newRole("directory-conn", "企业目录（连接）", rbac.ConnectionsManage)
	settingsUser := newUser("directory-settings", []int64{memberID, settingsRole})
	connUser := newUser("directory-conn", []int64{memberID, connRole})
	memberUser := newUser("directory-member", []int64{memberID})
	for what, c := range map[string]*client{"settings.manage": settingsUser, "connections.manage": connUser} {
		code, body := c.do("GET", "/api/admin/directory", nil, nil)
		expect(t, what+" 能看目录", code, 200, body)
		code, body = c.do("POST", "/api/admin/directory/check", map[string]any{"document": signed}, nil)
		expect(t, what+" 能 check", code, 200, body)
	}
	code, body = memberUser.do("GET", "/api/admin/directory", nil, nil)
	expect(t, "员工不能看目录", code, 403, body)
	code, body = memberUser.do("POST", "/api/admin/directory/check", map[string]any{"document": signed}, nil)
	expect(t, "员工不能 check 目录", code, 403, body)
	code, body = memberUser.do("POST", "/api/admin/directory/apply", map[string]any{"document": signed}, nil)
	expect(t, "员工不能登记目录", code, 403, body)

	// 7. 登记：只登记条目，不安装、不建连接
	code, body = apply(t, signed, false)
	expect(t, "登记目录", code, 200, body)
	applied := decodeAs[dirApplyResp](t, body)
	if !applied.OK || applied.Result.Created != 3 || len(applied.Entries) != 3 || applied.DocumentDigest == "" {
		t.Fatalf("登记结果不对：%+v", applied)
	}
	got := list(t)
	if got.Count != 3 || len(got.Directories) != 1 || got.Directories[0].EntriesCount != 3 || got.Directories[0].DocumentDigest != applied.DocumentDigest {
		t.Fatalf("列表不对：%+v", got)
	}
	sawUnknownSigner := false
	for _, row := range got.Entries {
		if row.DirectoryID != "acme-prod" {
			t.Fatalf("登记的行目录不对：%+v", row)
		}
		if row.Name == "acme/audit-log" {
			if row.SignatureKeyID != entrySigKeyID || row.SignatureKeyState != "unknown" {
				t.Fatalf("条目内嵌签名的声明没登记对：%+v", row)
			}
			sawUnknownSigner = true
		}
	}
	if !sawUnknownSigner {
		t.Fatalf("没找到带签名声明的条目：%+v", got.Entries)
	}
	var assets int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM assets").Scan(&assets); err != nil || assets != 0 {
		t.Fatalf("目录只登记不安装：assets=%d err=%v", assets, err)
	}
	var auditN int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs WHERE action = 'directory.apply'").Scan(&auditN); err != nil || auditN != 1 {
		t.Fatalf("apply 审计：%d %v", auditN, err)
	}

	// 8. 再 apply 同一份：幂等（不新增、不改行 id）
	ids := map[int64]bool{}
	for _, row := range got.Entries {
		ids[row.ID] = true
	}
	code, body = apply(t, signed, false)
	expect(t, "重复登记", code, 200, body)
	if again := decodeAs[dirApplyResp](t, body); again.Result.Created != 0 || again.Result.Updated != 0 || again.Result.Unchanged != 3 || again.Result.Removed != 0 {
		t.Fatalf("重复 apply 应幂等：%+v", again.Result)
	}
	got = list(t)
	if got.Count != 3 {
		t.Fatalf("重复 apply 不应产生新行：%+v", got)
	}
	for _, row := range got.Entries {
		if !ids[row.ID] {
			t.Fatalf("重复 apply 不应换行 id：%+v", row)
		}
	}

	// 9. 目录变化：同版本换内容 + 移除条目，需要 confirm
	changedEntries := testDirectoryEntries(t, true, entrySigKeyID)
	changedEntries[0].Digest = strings.Repeat("c", 64)
	changedEntries[0].Notes = "换了内容"
	changedEntries = changedEntries[:2] // 去掉 npm 插件（会触发 removed）
	changedDoc := testDirectory(t, changedEntries)
	changedJSON := signedDirJSON(t, changedDoc, dirPriv)
	p = check(t, changedJSON)
	if !p.OK || p.Diff == nil || p.Diff.Updated != 1 || p.Diff.Removed != 1 || len(p.Sensitive) < 2 {
		t.Fatalf("差异预检不对：%+v", p.Diff)
	}
	code, body = apply(t, changedJSON, false)
	expect(t, "敏感变化要确认", code, 409, body)
	code, body = apply(t, changedJSON, true)
	expect(t, "确认后登记", code, 200, body)
	if upd := decodeAs[dirApplyResp](t, body); upd.Result.Created != 0 || upd.Result.Updated != 1 || upd.Result.Removed != 1 || upd.Result.Unchanged != 1 || len(upd.Confirmed) == 0 {
		t.Fatalf("覆盖登记结果不对：%+v", upd.Result)
	}
	got = list(t)
	if got.Count != 2 {
		t.Fatalf("覆盖后的登记表：%+v", got)
	}
	for _, row := range got.Entries {
		if row.Name == "acme/keyring-itsm" && (row.Digest != strings.Repeat("c", 64) || row.Notes != "换了内容") {
			t.Fatalf("同版本换内容没覆盖：%+v", row)
		}
	}

	// 10. 条目内嵌签名：加扩展签名公钥后能看出签名者状态
	code, body = admin.do("POST", "/api/admin/signing-keys", map[string]any{"public_key": dirPEM(t, extPub), "purpose": "extension", "note": "扩展作者钥匙"}, nil)
	expect(t, "加扩展签名公钥", code, 201, body)
	extKeyID := extension.KeyID(extPub)
	docWithSig := testDirectory(t, testDirectoryEntries(t, true, extKeyID))
	p = check(t, signedDirJSON(t, docWithSig, dirPriv))
	if !p.OK || p.Entries[1].Signature == nil || p.Entries[1].Signature.KeyState != extension.KeyStateActive || hasIssueCode(p.Issues, "entry.signature.untrusted") {
		t.Fatalf("条目签名者受信任时应显示 active：%+v", p.Entries[1].Signature)
	}

	// 11. tarball 白名单（SSRF）：第二个实例只接受白名单主机，名单里的内网地址放行
	srv2 := api.New(st, box, api.Options{DataDir: t.TempDir(), DirectoryTarballHosts: []string{"storage.acme.internal"}}, logger)
	ts2 := httptest.NewServer(srv2.Handler())
	defer ts2.Close()
	admin2 := newClient(t, ts2.URL)
	code, body = admin2.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "第二个实例登录", code, 200, body)
	allowedDoc := testDirectory(t, []directory.Entry{{
		Kind: directory.KindExtension, Name: "acme/internal", Version: "1.0.0", Channel: directory.ChannelStable,
		Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://storage.acme.internal/oa-1.0.0.zip"},
	}})
	code, body = admin2.do("POST", "/api/admin/directory/check", map[string]any{"document": signedDirJSON(t, allowedDoc, dirPriv)}, nil)
	expect(t, "白名单地址 check", code, 200, body)
	if p := decodeAs[dirPlan](t, body); !p.OK {
		t.Fatalf("白名单里的内网地址应通过：%+v", p.Issues)
	}
	blockedDoc := testDirectory(t, []directory.Entry{{
		Kind: directory.KindExtension, Name: "acme/public", Version: "1.0.0", Channel: directory.ChannelStable,
		Digest: extDigest1, Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/oa.zip"},
	}})
	code, body = admin2.do("POST", "/api/admin/directory/check", map[string]any{"document": signedDirJSON(t, blockedDoc, dirPriv)}, nil)
	expect(t, "白名单外地址 check", code, 200, body)
	if p := decodeAs[dirPlan](t, body); p.OK {
		t.Fatalf("白名单外的地址应被拒绝：%+v", p)
	}

	// 12. 目录签名 key 换下 / 撤销：换下仍认（warning），撤销后不能登记
	code, body = admin.do("POST", "/api/admin/signing-keys/"+extension.KeyID(dirPub)+"/retire", nil, nil)
	expect(t, "换下目录签名 key", code, 200, body)
	p = check(t, signed)
	if !p.OK || !hasIssueCode(p.Issues, "signature.retired") || p.Document.Signature == nil || p.Document.Signature.KeyState != extension.KeyStateRetired {
		t.Fatalf("换下的目录 key 仍认历史签名：%+v", p)
	}
	code, body = admin.do("POST", "/api/admin/signing-keys/"+extension.KeyID(dirPub)+"/revoke", nil, nil)
	expect(t, "撤销目录签名 key", code, 200, body)
	p = check(t, signed)
	if p.OK || p.Document.Signature == nil || p.Document.Signature.Status != directory.SignatureRevoked || !hasIssueCode(p.Issues, "signature.revoked") {
		t.Fatalf("撤销后应不通过：%+v", p)
	}
	code, body = apply(t, signed, true)
	expect(t, "撤销后不能登记", code, 400, body)

	// 13. 文档格式错误：JSON 坏 400；format 不是 2 给结构化错误
	code, body = admin.do("POST", "/api/admin/directory/check", map[string]any{"document": "{"}, nil)
	expect(t, "坏 JSON", code, 400, body)
	format1 := testDirectory(t, testDirectoryEntries(t, false, ""))
	format1.Format = 1
	p = check(t, unsignedDirJSON(t, format1))
	if p.OK || !hasIssueCode(p.Issues, "format.unsupported") {
		t.Fatalf("format 1 应报错：%+v", p.Issues)
	}
	unknownField := `{"format":2,"directory_id":"acme-prod","issuer":{"name":"x"},"entries":[],"extra":1}`
	code, body = admin.do("POST", "/api/admin/directory/check", map[string]any{"document": unknownField}, nil)
	expect(t, "未知字段", code, 400, body)
	// 白名单策略会出现在列表里
	if hosts := list(t); len(hosts.TarballHosts) != 0 {
		t.Fatalf("默认没有 tarball 白名单：%+v", hosts.TarballHosts)
	}
}
