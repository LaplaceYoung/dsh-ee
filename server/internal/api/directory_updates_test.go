package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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

// 企业目录的更新检查（task-74、docs/plugin-signing.md §8）：GET /api/admin/directory/updates
// 只读对比「目录登记的版本」与扩展连接 / 企业源里已上架的版本，不下载、不安装、不升级。
// 覆盖：有更新 / 无更新 / 未安装 / 版本比较（含 prerelease、企业源比目录新）/ 签名状态
// （按当前信任根重算）/ 权限 / auto_upgrade 声明 + directory.autoUpgrade 设置 / 只读。

type dirUpdatesResp struct {
	Settings struct {
		AutoUpgrade bool `json:"auto_upgrade"`
	} `json:"settings"`
	Summary struct {
		Total        int `json:"total"`
		Available    int `json:"available"`
		Current      int `json:"current"`
		NotInstalled int `json:"not_installed"`
		Unknown      int `json:"unknown"`
	} `json:"summary"`
	Entries []struct {
		EntryID             int64  `json:"entry_id"`
		DirectoryID         string `json:"directory_id"`
		Kind                string `json:"kind"`
		Name                string `json:"name"`
		Channel             string `json:"channel"`
		Target              string `json:"target"`
		Current             string `json:"current"`
		CurrentSource       string `json:"current_source"`
		Status              string `json:"status"`
		AutoUpgrade         bool   `json:"auto_upgrade"`
		Reason              string `json:"reason"`
		InstalledConnection *struct {
			ID      int64  `json:"id"`
			Slug    string `json:"slug"`
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"installed_connection"`
		Signature *struct {
			Status   string `json:"status"`
			KeyID    string `json:"key_id"`
			KeyState string `json:"key_state"`
			Verified bool   `json:"verified"`
		} `json:"signature"`
	} `json:"entries"`
}

func TestDirectoryUpdates(t *testing.T) {
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

	// 目录签名公钥（purpose=directory）和条目里声明的扩展签名公钥
	dirPub, dirPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	extPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entrySigKeyID := extension.KeyID(extPub)
	code, body = admin.do("POST", "/api/admin/signing-keys", map[string]any{
		"public_key": dirPEM(t, dirPub), "purpose": "directory", "note": "企业目录签名",
	}, nil)
	expect(t, "加目录签名公钥", code, 201, body)

	// 目录条目：扩展 3 条、插件 5 条，覆盖有更新 / 无更新 / 未安装 / 未知 / 版本比较。
	extKeyringDigest := strings.Repeat("a", 64)
	entries := []directory.Entry{
		{ // 已装连接是 1.0.0，目录 1.2.0：有更新；带签名声明 + auto_upgrade
			Kind: directory.KindExtension, Name: "acme/keyring-itsm", Version: "1.2.0", Channel: directory.ChannelStable,
			Digest:      extKeyringDigest,
			Source:      directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/keyring-1.2.0.zip"},
			Signature:   dirEntrySig("acme/keyring-itsm", "1.2.0", extKeyringDigest, entrySigKeyID),
			Notes:       "审核过的工单扩展",
			AutoUpgrade: true,
		},
		{ // 已装连接就是 2.0.0：无更新
			Kind: directory.KindExtension, Name: "acme/audit-log", Version: "2.0.0", Channel: directory.ChannelBeta,
			Digest: strings.Repeat("b", 64),
			Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/audit-2.0.0.zip"},
		},
		{ // 没装过：未安装
			Kind: directory.KindExtension, Name: "acme/not-installed", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: strings.Repeat("c", 64),
			Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/not-installed-1.0.0.zip"},
		},
		{ // 企业源里是 1.0.0，目录 2.0.0：有更新
			Kind: directory.KindPlugin, Name: "@acme/oa-plugin", Version: "2.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "oa"), Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // 企业源里就是这个版本：无更新
			Kind: directory.KindPlugin, Name: "@acme/report-plugin", Version: "1.5.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "report"), Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // 企业源里的 2.0.0 比目录的 1.0.0 新：无更新（不能降级）
			Kind: directory.KindPlugin, Name: "@acme/ahead-plugin", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "ahead"), Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // 企业源里没有这个包：未安装
			Kind: directory.KindPlugin, Name: "@acme/new-plugin", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "new"), Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // 企业源里有包，但只有待审核版本：未知
			Kind: directory.KindPlugin, Name: "@acme/legacy-plugin", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "legacy"), Source: directory.Source{Type: directory.SourceNPM},
		},
		{ // beta 版本比较：企业源里 2.0.0-beta.1 < 目录 2.0.0-beta.2：有更新
			Kind: directory.KindPlugin, Name: "@acme/beta-plugin", Version: "2.0.0-beta.2", Channel: directory.ChannelBeta,
			Digest: pluginSRI(t, "beta2"), Source: directory.Source{Type: directory.SourceNPM},
		},
	}
	doc := testDirectory(t, entries)
	signed := signedDirJSON(t, doc, dirPriv)

	// check 计划里就带 auto_upgrade 和 warning：本期只登记、不执行
	code, body = admin.do("POST", "/api/admin/directory/check", map[string]any{"document": signed}, nil)
	expect(t, "目录 check", code, 200, body)
	plan := decodeAs[dirPlan](t, body)
	if !plan.OK || len(plan.Entries) != len(entries) {
		t.Fatalf("目录应通过校验：%+v", plan)
	}
	sawAutoEntry, sawAutoWarning := false, false
	for _, e := range plan.Entries {
		if e.Name == "acme/keyring-itsm" && e.AutoUpgrade {
			sawAutoEntry = true
		}
	}
	for _, is := range plan.Issues {
		if is.Code == "entry.auto_upgrade" {
			sawAutoWarning = true
		}
	}
	if !sawAutoEntry || !sawAutoWarning {
		t.Fatalf("check 计划应展示 auto_upgrade 声明并提示不执行：%+v %+v", plan.Entries, plan.Issues)
	}

	// 登记目录（只登记来源，不安装）
	code, body = admin.do("POST", "/api/admin/directory/apply", map[string]any{"document": signed, "confirm": true}, nil)
	expect(t, "登记目录", code, 200, body)
	applied := decodeAs[dirApplyResp](t, body)
	if !applied.OK || applied.Result.Created != len(entries) {
		t.Fatalf("登记结果不对：%+v", applied)
	}
	if got := decodeAs[dirListResp](t, mustGet(t, admin, "/api/admin/directory")); got.Count != len(entries) {
		t.Fatalf("登记行数不对：%+v", got)
	}
	// auto_upgrade 落了库：列表里那条是 true，其余是 false
	list := decodeAs[dirListResp](t, mustGet(t, admin, "/api/admin/directory"))
	for _, row := range list.Entries {
		if row.Name == "acme/keyring-itsm" && !row.AutoUpgrade {
			t.Fatalf("auto_upgrade 应登记成 true：%+v", row)
		}
		if row.Name != "acme/keyring-itsm" && row.AutoUpgrade {
			t.Fatalf("没声明的条目 auto_upgrade 应为 false：%+v", row)
		}
	}

	// 造「已安装」状态：扩展连接 + 企业源里的已上架版本
	seedConnection := func(slug, extID, version string) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{
			"title": slug, "type": "openapi", "url": "https://itsm.example.com",
			"extension": map[string]any{"id": extID, "version": version, "digest": strings.Repeat("a", 64), "manifest": "{}"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.CreateAsset(ctx, &store.Asset{Kind: store.KindConnection, Slug: slug, Title: slug}, raw, store.StatusPublished, 0); err != nil {
			t.Fatalf("造连接 %s：%v", slug, err)
		}
	}
	seedConnection("keyring-itsm", "acme/keyring-itsm", "1.0.0")
	seedConnection("audit-log", "acme/audit-log", "2.0.0")

	publish := func(name, version, status string) {
		t.Helper()
		pkg := &store.NpmPackage{Name: name, Origin: store.OriginHosted, IsPlugin: true}
		v := &store.NpmVersion{Name: name, Version: version, Integrity: pluginSRI(t, name+"@"+version), Status: status, IsPlugin: true}
		if _, err := st.PublishHosted(ctx, pkg, v, map[string]string{"latest": version}, store.CatalogPatch{}); err != nil {
			t.Fatalf("发布 %s@%s：%v", name, version, err)
		}
	}
	publish("@acme/oa-plugin", "1.0.0", store.StatusPublished)
	publish("@acme/report-plugin", "1.5.0", store.StatusPublished)
	publish("@acme/ahead-plugin", "2.0.0", store.StatusPublished)
	publish("@acme/legacy-plugin", "1.0.0", store.StatusPending)
	publish("@acme/beta-plugin", "2.0.0-beta.1", store.StatusPublished)

	updates := func(t *testing.T) dirUpdatesResp {
		t.Helper()
		return decodeAs[dirUpdatesResp](t, mustGet(t, admin, "/api/admin/directory/updates"))
	}
	byName := func(v dirUpdatesResp, name string) int {
		t.Helper()
		for i, e := range v.Entries {
			if e.Name == name {
				return i
			}
		}
		t.Fatalf("更新检查里没有 %s：%+v", name, v.Entries)
		return -1
	}

	// 只读：先把 assets / npm 版本数记下来，调用前后不能变
	var assetsBefore, versionsBefore int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM assets").Scan(&assetsBefore); err != nil {
		t.Fatal(err)
	}
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM npm_versions").Scan(&versionsBefore); err != nil {
		t.Fatal(err)
	}

	v := updates(t)
	if v.Summary.Total != len(entries) || v.Summary.Available != 3 || v.Summary.Current != 3 || v.Summary.NotInstalled != 2 || v.Summary.Unknown != 1 {
		t.Fatalf("汇总不对：%+v entries=%+v", v.Summary, v.Entries)
	}

	// 有更新：扩展连接 1.0.0 → 目录 1.2.0，带已装连接
	it := v.Entries[byName(v, "acme/keyring-itsm")]
	if it.Status != "available" || it.Current != "1.0.0" || it.Target != "1.2.0" || it.CurrentSource != "connection" ||
		it.Kind != directory.KindExtension || it.Channel != directory.ChannelStable || !it.AutoUpgrade {
		t.Fatalf("扩展有更新结果不对：%+v", it)
	}
	if it.InstalledConnection == nil || it.InstalledConnection.Slug != "keyring-itsm" || it.InstalledConnection.Version != "1.0.0" || it.InstalledConnection.ID == 0 {
		t.Fatalf("应返回已装连接：%+v", it.InstalledConnection)
	}
	if it.Signature == nil || it.Signature.Status != directory.EntrySignatureDeclared || it.Signature.KeyID != entrySigKeyID {
		t.Fatalf("带签名声明的条目状态应是 declared：%+v", it.Signature)
	}
	// 无更新：连接就是目录版本；没带签名声明的条目状态是 none
	if it := v.Entries[byName(v, "acme/audit-log")]; it.Status != "current" || it.Current != "2.0.0" || it.InstalledConnection == nil {
		t.Fatalf("扩展无更新结果不对：%+v", it)
	} else if it.Signature == nil || it.Signature.Status != directory.EntrySignatureNone {
		t.Fatalf("没带签名声明的条目状态应是 none：%+v", it.Signature)
	}
	// 未安装：没有连接
	if it := v.Entries[byName(v, "acme/not-installed")]; it.Status != "not_installed" || it.Current != "" || it.InstalledConnection != nil {
		t.Fatalf("扩展未安装结果不对：%+v", it)
	}
	// 插件有更新：企业源 1.0.0 → 目录 2.0.0
	if it := v.Entries[byName(v, "@acme/oa-plugin")]; it.Status != "available" || it.Current != "1.0.0" || it.CurrentSource != "registry" || it.Target != "2.0.0" {
		t.Fatalf("插件有更新结果不对：%+v", it)
	}
	// 插件无更新：企业源就是目录版本
	if it := v.Entries[byName(v, "@acme/report-plugin")]; it.Status != "current" || it.Current != "1.5.0" {
		t.Fatalf("插件无更新结果不对：%+v", it)
	}
	// 版本比较：企业源 2.0.0 比目录 1.0.0 新 → 不需要更新
	if it := v.Entries[byName(v, "@acme/ahead-plugin")]; it.Status != "current" || it.Current != "2.0.0" || !strings.Contains(it.Reason, "不需要更新") {
		t.Fatalf("企业源更新时应是 current：%+v", it)
	}
	// 版本比较：prerelease 2.0.0-beta.2 > 2.0.0-beta.1 → 有更新
	if it := v.Entries[byName(v, "@acme/beta-plugin")]; it.Status != "available" || it.Current != "2.0.0-beta.1" {
		t.Fatalf("prerelease 比较不对：%+v", it)
	}
	// 未安装：企业源里没有包
	if it := v.Entries[byName(v, "@acme/new-plugin")]; it.Status != "not_installed" || it.Current != "" {
		t.Fatalf("插件未安装结果不对：%+v", it)
	}
	// 未知：有包但没有已上架版本
	if it := v.Entries[byName(v, "@acme/legacy-plugin")]; it.Status != "unknown" || it.Current != "" {
		t.Fatalf("插件未知结果不对：%+v", it)
	}
	again := updates(t) // 连续两次结果稳定
	if again.Summary != v.Summary || len(again.Entries) != len(v.Entries) {
		t.Fatalf("连续两次汇总不一致：%+v vs %+v", again.Summary, v.Summary)
	}

	// 签名状态：按当前信任根重算。先没加条目签名公钥 → unknown；加了 → active；撤销 → revoked
	if it := v.Entries[byName(v, "acme/keyring-itsm")]; it.Signature == nil || it.Signature.Status != directory.EntrySignatureDeclared ||
		it.Signature.KeyID != entrySigKeyID || it.Signature.KeyState != "unknown" || it.Signature.Verified {
		t.Fatalf("不受信任的签名者应显示 unknown / verified=false：%+v", it.Signature)
	}
	code, body = admin.do("POST", "/api/admin/signing-keys", map[string]any{
		"public_key": dirPEM(t, extPub), "purpose": "extension", "note": "扩展签名",
	}, nil)
	expect(t, "加扩展签名公钥", code, 201, body)
	trusted := updates(t)
	if it := trusted.Entries[byName(trusted, "acme/keyring-itsm")]; it.Signature == nil || it.Signature.KeyState != extension.KeyStateActive {
		t.Fatalf("受信任后签名者状态应是 active：%+v", it.Signature)
	}
	code, body = admin.do("POST", "/api/admin/signing-keys/"+entrySigKeyID+"/revoke", nil, nil)
	expect(t, "撤销扩展签名公钥", code, 200, body)
	revoked := updates(t)
	if it := revoked.Entries[byName(revoked, "acme/keyring-itsm")]; it.Signature == nil || it.Signature.KeyState != extension.KeyStateRevoked {
		t.Fatalf("撤销后签名者状态应是 revoked：%+v", it.Signature)
	}

	// auto_upgrade：设置默认 false；PUT 打开后只读接口返回 true，且仍然不执行任何升级
	if v := updates(t); v.Settings.AutoUpgrade {
		t.Fatalf("directory.autoUpgrade 默认应是 false：%+v", v.Settings)
	}
	code, body = admin.do("PUT", "/api/admin/settings", map[string]any{"directory": map[string]any{"autoUpgrade": true}}, nil)
	expect(t, "打开 directory.autoUpgrade", code, 200, body)
	if v := updates(t); !v.Settings.AutoUpgrade {
		t.Fatalf("directory.autoUpgrade 应持久化并返回：%+v", v.Settings)
	}
	// 设置的值要校验：不是布尔值就拒绝（本期只存 / 只读展示，类型也要对）
	code, body = admin.do("PUT", "/api/admin/settings", map[string]any{"directory": map[string]any{"autoUpgrade": "yes"}}, nil)
	expect(t, "非法 directory.autoUpgrade 应拒绝", code, 400, body)
	// 只有声明了 auto_upgrade 的条目是 true；而且没有任何自动安装 / 升级发生
	v = updates(t)
	for _, e := range v.Entries {
		if e.Name == "acme/keyring-itsm" && !e.AutoUpgrade {
			t.Fatalf("条目声明的 auto_upgrade 应返回：%+v", e)
		}
		if e.Name != "acme/keyring-itsm" && e.AutoUpgrade {
			t.Fatalf("没声明 auto_upgrade 的条目不应返回 true：%+v", e)
		}
	}

	// 权限：connections.manage / settings.manage 都能看；普通员工不能
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
		c, b := admin.do("POST", "/api/admin/roles", map[string]any{"code": code, "name": name, "permissions": []string{perm}}, nil)
		expect(t, "建角色 "+code, c, 201, b)
		return decodeAs[store.Role](t, b).ID
	}
	newUser := func(name string, roles []int64) *client {
		t.Helper()
		c, b := admin.do("POST", "/api/admin/users", map[string]any{
			"username": name, "password": "directory-pass-1", "department": "研发效能部", "role_ids": roles,
		}, nil)
		expect(t, "建用户 "+name, c, 201, b)
		u := newClient(t, ts.URL)
		c, b = u.do("POST", "/api/auth/login", map[string]string{"username": name, "password": "directory-pass-1"}, nil)
		expect(t, name+" 登录", c, 200, b)
		return u
	}
	settingsRole := newRole("dir-updates-settings", "目录更新（设置）", rbac.SettingsManage)
	connRole := newRole("dir-updates-conn", "目录更新（连接）", rbac.ConnectionsManage)
	settingsUser := newUser("dir-updates-settings", []int64{memberID, settingsRole})
	connUser := newUser("dir-updates-conn", []int64{memberID, connRole})
	memberUser := newUser("dir-updates-member", []int64{memberID})
	for what, c := range map[string]*client{"settings.manage": settingsUser, "connections.manage": connUser} {
		code, body := c.do("GET", "/api/admin/directory/updates", nil, nil)
		expect(t, what+" 能看更新检查", code, 200, body)
	}
	code, body = memberUser.do("GET", "/api/admin/directory/updates", nil, nil)
	expect(t, "普通员工不能看更新检查", code, 403, body)

	// 只读：整个过程没有新建连接 / 没有新版本 / 登记表没被改
	var assetsAfter, versionsAfter int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM assets").Scan(&assetsAfter); err != nil {
		t.Fatal(err)
	}
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM npm_versions").Scan(&versionsAfter); err != nil {
		t.Fatal(err)
	}
	if assetsAfter != assetsBefore || versionsAfter != versionsBefore {
		t.Fatalf("更新检查不应安装 / 发布任何东西：assets %d→%d，versions %d→%d", assetsBefore, assetsAfter, versionsBefore, versionsAfter)
	}
	var auditInstall int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs WHERE action IN ('directory.install','directory.upgrade','extension.install')").Scan(&auditInstall); err != nil {
		t.Fatal(err)
	}
	if auditInstall != 0 {
		t.Fatalf("更新检查不应写安装 / 升级审计：%d", auditInstall)
	}
	// 同一个目录再 apply 一次同一份文档：幂等，auto_upgrade 不会被清掉
	code, body = admin.do("POST", "/api/admin/directory/apply", map[string]any{"document": signed, "confirm": true}, nil)
	expect(t, "重复登记", code, 200, body)
	if again := decodeAs[dirApplyResp](t, body); again.Result.Unchanged != len(entries) || again.Result.Updated != 0 || again.Result.Created != 0 {
		t.Fatalf("重复 apply 应幂等：%+v", again.Result)
	}
	afterReapply := updates(t)
	if it := afterReapply.Entries[byName(afterReapply, "acme/keyring-itsm")]; !it.AutoUpgrade {
		t.Fatalf("重新 apply 后 auto_upgrade 应保留：%+v", it)
	}
}
