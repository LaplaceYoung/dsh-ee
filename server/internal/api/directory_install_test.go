package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业目录条目的安装 / 升级（task-65）：只对登记过的条目、管理员明确触发；
// 扩展复用安装计划（confirm / 敏感项 / 签名 / 验签），插件只走 npm 固定版本 + 现有同步/审核。

// dirExtManifest 造一个声明式扩展清单（id / 版本 / 请求地址可变）。
func dirExtManifest(id, version string, servers ...string) string {
	raw, _ := json.Marshal(servers)
	return fmt.Sprintf(`{
  "id": %q,
  "version": %q,
  "name": { "zh": "工单", "en": "Tickets" },
  "description": "目录安装测试用的工单扩展",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": %s,
  "credential": { "mode": "shared" },
  "tools": [
    { "name": "list_tickets", "readOnly": true },
    { "name": "create_ticket" }
  ]
}`, id, version, raw)
}

// dirExtZip 把一组文件打成扩展 zip。
func dirExtZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// signedDirExtZip 打一个带有效签名的扩展包，返回 zip 和摘要（目录条目的 digest 口径）。
func signedDirExtZip(t *testing.T, manifest, openapi string, priv ed25519.PrivateKey) ([]byte, string) {
	t.Helper()
	_, raw, err := extension.SignExtension([]byte(manifest), []byte(openapi), priv)
	if err != nil {
		t.Fatal(err)
	}
	return dirExtZip(t, map[string]string{
		extension.ManifestFile:  manifest,
		"openapi.yaml":          openapi,
		extension.SignatureFile: string(raw),
	}), extension.Digest([]byte(manifest), []byte(openapi))
}

// unsignedDirExtZip 打一个没签名的扩展包，返回 zip 和摘要。
func unsignedDirExtZip(t *testing.T, manifest, openapi string) ([]byte, string) {
	t.Helper()
	return dirExtZip(t, map[string]string{
		extension.ManifestFile: manifest,
		"openapi.yaml":         openapi,
	}), extension.Digest([]byte(manifest), []byte(openapi))
}

// dirPackageServer 是假的扩展下载站（https + 自签证书），按路径返回包，并记录下载次数。
type dirPackageServer struct {
	*httptest.Server
	mu    sync.Mutex
	files map[string][]byte
	hits  map[string]int
}

func newDirPackageServer(t *testing.T, files map[string][]byte) *dirPackageServer {
	t.Helper()
	s := &dirPackageServer{files: files, hits: map[string]int{}}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		body, ok := s.files[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *dirPackageServer) hitsFor(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// dirInstallView 是安装 / 升级接口的返回（成功、失败两种形状放一起，按字段取）。
type dirInstallView struct {
	Error string `json:"error"`
	OK    bool   `json:"ok"`
	// 成功时顶层字段
	Action      string                `json:"action"`
	Kind        string                `json:"kind"`
	DirectoryID string                `json:"directory_id"`
	Entry       *store.DirectoryEntry `json:"entry"`
	Extension   *struct {
		Check struct {
			OK       bool   `json:"ok"`
			Identity string `json:"identity"`
			Digest   string `json:"digest"`
			Errors   int    `json:"errors"`
		} `json:"check"`
		Sensitive []string `json:"sensitive"`
		Signature *struct {
			Status string `json:"status"`
			KeyID  string `json:"key_id"`
		} `json:"signature"`
		Existing *struct {
			ID      int64  `json:"id"`
			Slug    string `json:"slug"`
			Version string `json:"version"`
		} `json:"existing"`
	} `json:"extension"`
	Plugin *struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		Digest          string `json:"digest"`
		Source          string `json:"source"`
		ExistingVersion string `json:"existing_version"`
		ExistingStatus  string `json:"existing_status"`
		SyncVersion     string `json:"sync_version"`
		ReviewRequired  bool   `json:"review_required"`
		Message         string `json:"message"`
	} `json:"plugin"`
	Sensitive []string       `json:"sensitive"`
	Result    map[string]any `json:"result"`
	Message   string         `json:"message"`
	// 这是只出计划的预览
	PlanOnly bool `json:"plan_only"`
	// 失败时计划放在 plan 里
	Plan *dirInstallView `json:"plan"`
}

// dirInstallResultAsset 从返回里取装成的连接 id。
func dirInstallResultAsset(t *testing.T, v *dirInstallView) int64 {
	t.Helper()
	asset, ok := v.Result["asset"].(map[string]any)
	if !ok {
		t.Fatalf("返回里没有 asset：%+v", v.Result)
	}
	id, ok := asset["id"].(float64)
	if !ok {
		t.Fatalf("asset.id 不是数字：%+v", asset)
	}
	return int64(id)
}

// dirInstallResultJob 从返回里取同步任务 id。
func dirInstallResultJob(t *testing.T, v *dirInstallView) int64 {
	t.Helper()
	job, ok := v.Result["job"].(map[string]any)
	if !ok {
		t.Fatalf("返回里没有 job：%+v", v.Result)
	}
	id, ok := job["id"].(float64)
	if !ok {
		t.Fatalf("job.id 不是数字：%+v", job)
	}
	return int64(id)
}

// waitStoreJob 直接查库等同步任务结束（不依赖调用者的 upstream.manage 权限）。
func waitStoreJob(t *testing.T, st *store.Store, id int64) store.SyncJob {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(context.Background(), id)
		if err != nil {
			t.Fatalf("查任务 %d：%v", id, err)
		}
		if j.Status != "running" {
			return *j
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("任务 %d 超时", id)
	return store.SyncJob{}
}

// countRows 数一类审计 / 表里的行数，用来断言「失败不留半装」。
func countRows(t *testing.T, st *store.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("查 %q：%v", query, err)
	}
	return n
}

// tenantRoles 建测试用的三种身份：普通员工、管连接的、管插件的。
type dirTestUsers struct {
	member  *client
	conn    *client
	plugins *client
}

func newDirTestUsers(t *testing.T, admin *client, base string) dirTestUsers {
	t.Helper()
	code, body := admin.do("GET", "/api/admin/roles", nil, nil)
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
		u := newClient(t, base)
		c2, b2 := u.do("POST", "/api/auth/login", map[string]string{"username": name, "password": "directory-pass-1"}, nil)
		expect(t, name+" 登录", c2, 200, b2)
		return u
	}
	return dirTestUsers{
		member:  newUser("dir-install-member", []int64{memberID}),
		conn:    newUser("dir-install-conn", []int64{memberID, newRole("dir-install-conn", "目录安装（连接）", rbac.ConnectionsManage)}),
		plugins: newUser("dir-install-plugins", []int64{memberID, newRole("dir-install-plugins", "目录安装（插件）", rbac.PluginsManage)}),
	}
}

// installDirEntry 调 install / upgrade 并解析返回。
func installDirEntry(t *testing.T, c *client, id int64, upgrade bool, body any) (int, *dirInstallView, []byte) {
	t.Helper()
	action := "install"
	if upgrade {
		action = "upgrade"
	}
	code, raw := c.do("POST", fmt.Sprintf("/api/admin/directory/entries/%d/%s", id, action), body, nil)
	v := decodeAs[dirInstallView](t, raw)
	return code, &v, raw
}

// dirListView 是 GET /api/admin/directory 的条目 + 安装状态。
type dirEntryView struct {
	ID                  int64  `json:"id"`
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	Version             string `json:"version"`
	Installed           bool   `json:"installed"`
	InstalledVersion    string `json:"installed_version"`
	InstalledStatus     string `json:"installed_status"`
	InstalledConnection string `json:"installed_connection"`
	UpgradeAvailable    bool   `json:"upgrade_available"`
}

type dirListView struct {
	Directories  []store.DirectorySummary `json:"directories"`
	Entries      []dirEntryView           `json:"entries"`
	Count        int                      `json:"count"`
	TarballHosts []string                 `json:"tarball_hosts"`
}

// directoryList 读目录条目列表。
func directoryList(t *testing.T, c *client) dirListView {
	t.Helper()
	code, body := c.do("GET", "/api/admin/directory", nil, nil)
	expect(t, "目录列表", code, 200, body)
	return decodeAs[dirListView](t, body)
}

// liveExtensionVersion 读某个连接当前生效版本的扩展版本号。
func liveExtensionVersion(t *testing.T, st *store.Store, assetID int64) string {
	t.Helper()
	lv, err := st.LiveAssetVersion(context.Background(), assetID)
	if err != nil {
		t.Fatalf("读连接 %d 的生效版本：%v", assetID, err)
	}
	var spec struct {
		Extension *struct {
			Version string `json:"version"`
			Digest  string `json:"digest"`
		} `json:"extension"`
	}
	if err := json.Unmarshal(lv.Spec, &spec); err != nil {
		t.Fatalf("解析连接版本内容：%v; %s", err, lv.Spec)
	}
	if spec.Extension == nil {
		return ""
	}
	return spec.Extension.Version
}

// ---- 扩展条目 ----

func TestDirectoryExtensionInstall(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dirPub, dirPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	extPub, extPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	extKeyID := extension.KeyID(extPub)

	const itsm = "https://itsm.example.com"
	manV1 := dirExtManifest("acme/itsm", "1.0.0", itsm)
	manV2 := dirExtManifest("acme/itsm", "2.0.0", itsm, "https://itsm2.example.com")
	manV3 := dirExtManifest("acme/itsm", "3.0.0", itsm)
	manV5 := dirExtManifest("acme/itsm", "5.0.0", itsm)
	manV6 := dirExtManifest("acme/itsm", "6.0.0", itsm)
	zipV1, digV1 := signedDirExtZip(t, manV1, extOpenAPI, extPriv)
	zipV2, digV2 := signedDirExtZip(t, manV2, extOpenAPI, extPriv)
	zipV3, digV3 := signedDirExtZip(t, manV3, extOpenAPI, extPriv)
	zipV5, digV5 := unsignedDirExtZip(t, manV5, extOpenAPI)
	zipV6, digV6 := unsignedDirExtZip(t, manV6, extOpenAPI)
	packs := newDirPackageServer(t, map[string][]byte{
		"/v1.zip": zipV1, "/v2.zip": zipV2, "/v3.zip": zipV3, "/v5.zip": zipV5, "/v6.zip": zipV6,
	})

	// 主实例：信任目录签名钥匙和扩展签名钥匙，允许从 127.0.0.1 下载扩展包。
	srv := api.New(st, box, api.Options{
		DataDir:               t.TempDir(),
		Upstream:              packs.Client(),
		DirectoryTarballHosts: []string{"127.0.0.1"},
		Version:               "0.2.0",
		ExtensionTrustedKeys:  []ed25519.PublicKey{dirPub, extPub},
	}, logger)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 登记目录：三条正常版本（1.0.0 / 2.0.0 / 5.0.0 / 6.0.0 里有签名的、没签名的），
	// 一条同版本号但内容摘要不对（3.0.0），一条包版本和登记版本对不上（4.0.0 → 3.0.0 的包）。
	entries := []directory.Entry{
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "1.0.0", Channel: directory.ChannelStable, Digest: digV1,
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v1.zip"}, Signature: dirEntrySig("acme/itsm", "1.0.0", digV1, extKeyID)},
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "2.0.0", Channel: directory.ChannelStable, Digest: digV2,
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v2.zip"}, Signature: dirEntrySig("acme/itsm", "2.0.0", digV2, extKeyID)},
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "3.0.0", Channel: directory.ChannelStable, Digest: strings.Repeat("c", 64),
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v3.zip"}},
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "4.0.0", Channel: directory.ChannelStable, Digest: digV3,
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v3.zip"}},
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "5.0.0", Channel: directory.ChannelStable, Digest: digV5,
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v5.zip"}},
		{Kind: directory.KindExtension, Name: "acme/itsm", Version: "6.0.0", Channel: directory.ChannelStable, Digest: digV6,
			Source: directory.Source{Type: directory.SourceTarball, URL: packs.URL + "/v6.zip"}, Signature: dirEntrySig("acme/itsm", "6.0.0", digV6, extKeyID)},
	}
	document := signedDirJSON(t, testDirectory(t, entries), dirPriv)
	code, body = admin.do("POST", "/api/admin/directory/apply", map[string]any{"document": document}, nil)
	expect(t, "登记目录", code, 200, body)
	rows := map[string]store.DirectoryEntry{}
	for _, e := range decodeAs[dirApplyResp](t, body).Entries {
		rows[e.Version] = e
	}
	if len(rows) != len(entries) {
		t.Fatalf("登记条数不对：%d", len(rows))
	}

	users := newDirTestUsers(t, admin, ts.URL)
	assets := func() int { return countRows(t, st, "SELECT COUNT(*) FROM assets") }

	// 0. 列表：还没装，三种状态都显示未安装
	if lv := directoryList(t, admin); lv.Count != len(entries) {
		t.Fatalf("列表条数不对：%+v", lv)
	}
	for _, row := range directoryList(t, admin).Entries {
		if row.Installed || row.UpgradeAvailable {
			t.Fatalf("还没安装时不该显示已安装/可升级：%+v", row)
		}
	}

	// 1. 权限按条目类型：普通员工不行；管插件的不能装扩展（扩展要 connections.manage）。
	code, _, _ = installDirEntry(t, users.member, rows["1.0.0"].ID, false, nil)
	expect(t, "员工不能安装目录条目", code, 403, nil)
	code, v, raw := installDirEntry(t, users.plugins, rows["1.0.0"].ID, false, nil)
	expect(t, "管插件的不能装扩展", code, 403, raw)
	if v.Error == "" {
		t.Fatalf("403 要有原因：%s", raw)
	}

	// 2. 只出计划：不写库、不建连接，计划里带上签名和检查结果
	code, preview, raw := installDirEntry(t, users.conn, rows["1.0.0"].ID, false, map[string]any{"plan_only": true})
	expect(t, "只出安装计划", code, 200, raw)
	if !preview.OK || !preview.PlanOnly || preview.Extension == nil || preview.Extension.Check.Identity != "acme/itsm@1.0.0" ||
		preview.Extension.Signature == nil || preview.Extension.Signature.Status != extension.SignatureValid {
		t.Fatalf("安装计划不对：%s", raw)
	}
	if assets() != 0 || preview.Result != nil {
		t.Fatalf("只出计划不能建连接：%s", raw)
	}

	// 3. 安装扩展：下载、身份/摘要核对、安装计划、连接创建
	code, installed, raw := installDirEntry(t, users.conn, rows["1.0.0"].ID, false, map[string]any{"secret": "svc-token"})
	expect(t, "安装目录里的扩展", code, 201, raw)
	if !installed.OK || installed.Action != "install" || installed.Kind != directory.KindExtension ||
		installed.Extension == nil || installed.Extension.Check.Identity != "acme/itsm@1.0.0" ||
		installed.Extension.Signature == nil || installed.Extension.Signature.Status != extension.SignatureValid {
		t.Fatalf("安装结果不对：%s", raw)
	}
	assetID := dirInstallResultAsset(t, installed)
	if got := liveExtensionVersion(t, st, assetID); got != "1.0.0" {
		t.Fatalf("装上的扩展版本是 %q", got)
	}
	if n, _ := installed.Result["tools"].(float64); n != 2 {
		t.Fatalf("工具数不对：%+v", installed.Result["tools"])
	}
	if assets() != 1 {
		t.Fatalf("安装扩展应只建一个连接：%d", assets())
	}
	if n := countRows(t, st, "SELECT COUNT(*) FROM audit_logs WHERE action = 'directory.install'"); n != 1 {
		t.Fatalf("应有一条 directory.install 审计：%d", n)
	}

	// 4. 重复安装：409，什么都不改
	code, again, raw := installDirEntry(t, users.conn, rows["1.0.0"].ID, false, nil)
	expect(t, "重复安装", code, 409, raw)
	if again.Error == "" || assets() != 1 {
		t.Fatalf("重复安装不该建新连接：%s", raw)
	}

	// 5. 装了 1.0.0 时用 install 装 2.0.0：要引导到 upgrade
	code, _, raw = installDirEntry(t, users.conn, rows["2.0.0"].ID, false, nil)
	expect(t, "已装过要改用 upgrade", code, 409, raw)
	if !strings.Contains(string(raw), "upgrade") {
		t.Fatalf("应提示用 upgrade：%s", raw)
	}

	// 6. 升级 2.0.0 前先「只出计划」：能看到敏感项，但不改已装的版本
	code, upgradePreview, raw := installDirEntry(t, users.conn, rows["2.0.0"].ID, true, map[string]any{"plan_only": true})
	expect(t, "只出升级计划", code, 200, raw)
	if !upgradePreview.PlanOnly || len(upgradePreview.Sensitive) == 0 {
		t.Fatalf("升级计划应带敏感项：%s", raw)
	}
	if assets() != 1 || liveExtensionVersion(t, st, assetID) != "1.0.0" {
		t.Fatalf("只出计划不能改已装的连接：%s", raw)
	}

	// 7. 升级 2.0.0：多了可请求的地址，属于敏感变化，要确认；确认后版本升上去
	code, conflict, raw := installDirEntry(t, users.conn, rows["2.0.0"].ID, true, nil)
	expect(t, "敏感变化要确认", code, 409, raw)
	if len(conflict.Plan.Sensitive) == 0 || !strings.Contains(strings.Join(conflict.Plan.Sensitive, "；"), "itsm2.example.com") {
		t.Fatalf("应报新的请求地址：%s", raw)
	}
	code, upgraded, raw := installDirEntry(t, users.conn, rows["2.0.0"].ID, true, map[string]any{"confirm": true})
	expect(t, "确认后升级", code, 200, raw)
	if !upgraded.OK || upgraded.Action != "upgrade" || upgraded.Result["upgraded_from"] != "1.0.0" {
		t.Fatalf("升级结果不对：%s", raw)
	}
	if got := liveExtensionVersion(t, st, assetID); got != "2.0.0" {
		t.Fatalf("升级后生效版本是 %q", got)
	}
	if assets() != 1 {
		t.Fatalf("升级不该新建连接：%d", assets())
	}
	if n := countRows(t, st, "SELECT COUNT(*) FROM audit_logs WHERE action = 'directory.upgrade'"); n != 1 {
		t.Fatalf("应有一条 directory.upgrade 审计：%d", n)
	}

	// 8. 重复升级 / 降级都不行
	code, _, raw = installDirEntry(t, users.conn, rows["2.0.0"].ID, true, map[string]any{"confirm": true})
	expect(t, "重复升级", code, 409, raw)
	code, _, raw = installDirEntry(t, users.conn, rows["1.0.0"].ID, true, map[string]any{"confirm": true})
	expect(t, "不能降级", code, 409, raw)
	if !strings.Contains(string(raw), "旧") {
		t.Fatalf("应说明不支持降级：%s", raw)
	}

	// 9. 摘要对不上 / 版本对不上 / 声明签名者对不上：都拒绝，连接不变
	code, _, raw = installDirEntry(t, users.conn, rows["3.0.0"].ID, false, nil)
	expect(t, "摘要不一致", code, 409, raw)
	if !strings.Contains(string(raw), "摘要") {
		t.Fatalf("应提示摘要不一致：%s", raw)
	}
	code, _, raw = installDirEntry(t, users.conn, rows["4.0.0"].ID, false, nil)
	expect(t, "版本不一致", code, 409, raw)
	if !strings.Contains(string(raw), "版本对不上") {
		t.Fatalf("应提示版本对不上：%s", raw)
	}
	code, _, raw = installDirEntry(t, users.conn, rows["6.0.0"].ID, false, nil)
	expect(t, "目录声明了签名者但包没签名", code, 409, raw)
	if !strings.Contains(string(raw), "签名") {
		t.Fatalf("应提示签名对不上：%s", raw)
	}
	if assets() != 1 || liveExtensionVersion(t, st, assetID) != "2.0.0" {
		t.Fatalf("失败的安装不能改已装的连接：assets=%d", assets())
	}

	// 10. 列表：已装版本 / 连接 / 可升级都显示出来
	views := directoryList(t, admin).Entries
	byVersion := map[string]dirEntryView{}
	for _, v := range views {
		byVersion[v.Version] = v
	}
	if cur := byVersion["2.0.0"]; !cur.Installed || cur.InstalledVersion != "2.0.0" || cur.InstalledConnection != "itsm" || cur.UpgradeAvailable {
		t.Fatalf("当前版本的安装状态不对：%+v", cur)
	}
	if old := byVersion["1.0.0"]; old.Installed || old.UpgradeAvailable || old.InstalledVersion != "2.0.0" {
		t.Fatalf("旧版本的安装状态不对：%+v", old)
	}
	if next := byVersion["5.0.0"]; next.Installed || !next.UpgradeAvailable {
		t.Fatalf("更新的版本应显示可升级：%+v", next)
	}

	// 11. SSRF：换个没有 tarball 白名单的实例（同一个库），地址是回环地址，下载前就应拦下
	ssrfSrv := api.New(st, box, api.Options{
		DataDir:              t.TempDir(),
		Upstream:             packs.Client(),
		ExtensionTrustedKeys: []ed25519.PublicKey{dirPub, extPub},
	}, logger)
	ssrfTS := httptest.NewServer(ssrfSrv.Handler())
	defer ssrfTS.Close()
	ssrfAdmin := newClient(t, ssrfTS.URL)
	code, body = ssrfAdmin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "第二个实例登录", code, 200, body)
	before := packs.hitsFor("/v1.zip")
	code, _, raw = installDirEntry(t, ssrfAdmin, rows["1.0.0"].ID, false, nil)
	expect(t, "没有白名单时回环地址要拒绝", code, 400, raw)
	if !strings.Contains(string(raw), "回环") && !strings.Contains(string(raw), "内网") {
		t.Fatalf("应提示地址不允许：%s", raw)
	}
	if after := packs.hitsFor("/v1.zip"); after != before {
		t.Fatalf("SSRF 应在发请求前拦下：下载次数 %d → %d", before, after)
	}

	// 12. 强制签名：未签名的包在计划阶段就被拦下，不落库
	requireSrv := api.New(st, box, api.Options{
		DataDir:                   t.TempDir(),
		Upstream:                  packs.Client(),
		DirectoryTarballHosts:     []string{"127.0.0.1"},
		Version:                   "0.2.0",
		ExtensionTrustedKeys:      []ed25519.PublicKey{dirPub, extPub},
		ExtensionRequireSignature: true,
	}, logger)
	requireTS := httptest.NewServer(requireSrv.Handler())
	defer requireTS.Close()
	requireAdmin := newClient(t, requireTS.URL)
	code, body = requireAdmin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "第三个实例登录", code, 200, body)
	code, _, raw = installDirEntry(t, requireAdmin, rows["5.0.0"].ID, false, nil)
	expect(t, "强制签名时未签名包不能装", code, 400, raw)
	if !strings.Contains(string(raw), "签名") || assets() != 1 {
		t.Fatalf("应报签名问题且不落库：%s", raw)
	}

	// 13. 非法 / 不存在的条目
	code, body = admin.do("POST", "/api/admin/directory/entries/999999/install", map[string]any{}, nil)
	expect(t, "不存在的条目", code, 404, body)
	code, body = admin.do("POST", "/api/admin/directory/entries/abc/install", map[string]any{}, nil)
	expect(t, "非法条目 id", code, 400, body)
}

// fakeIntegrity 取假上游里某个版本的 tarball 摘要（目录条目的 digest 口径）。
func fakeIntegrity(t *testing.T, up *fakeRegistry, name, version string) string {
	t.Helper()
	up.mu.Lock()
	tgz := up.tgz[name+"/"+registry.TarballFile(name, version)]
	up.mu.Unlock()
	if tgz == nil {
		t.Fatalf("假上游里没有 %s@%s 的 tarball", name, version)
	}
	integrity, _ := digests(tgz)
	return integrity
}

// ---- 插件条目 ----

func TestDirectoryPluginInstall(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dirPub, dirPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	up := newFakeRegistry(t)
	for _, v := range []string{"2.0.0", "2.1.0", "2.2.0", "2.3.0"} {
		up.add(t, pluginPkg("dsh-plugin-dir", v, map[string]any{"description": "目录安装测试 " + v}), bundleFile)
	}
	up.add(t, pluginPkg("dsh-plugin-other", "1.0.0", nil), bundleFile)

	srv := api.New(st, box, api.Options{
		DataDir:              t.TempDir(),
		NpmUpstream:          up.Client(),
		ExtensionTrustedKeys: []ed25519.PublicKey{dirPub},
	}, logger)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 设置：企业作用域 @acme，上游指向假 registry
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"] = "@acme"
	set["upstream"].(map[string]any)["registry"] = "custom"
	set["upstream"].(map[string]any)["customRegistry"] = up.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "保存设置", code, 200, body)

	// 登记插件条目：正常版本、摘要不对、上游没有的版本、registry 不一致、企业作用域、tarball 来源。
	entries := []directory.Entry{
		{Kind: directory.KindPlugin, Name: "dsh-plugin-dir", Version: "2.0.0", Channel: directory.ChannelStable,
			Digest: fakeIntegrity(t, up, "dsh-plugin-dir", "2.0.0"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-dir", Version: "2.1.0", Channel: directory.ChannelStable,
			Digest: fakeIntegrity(t, up, "dsh-plugin-dir", "2.1.0"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-dir", Version: "2.2.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "wrong-content"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-dir", Version: "2.3.0", Channel: directory.ChannelStable,
			Digest: fakeIntegrity(t, up, "dsh-plugin-dir", "2.3.0"), Source: directory.Source{Type: directory.SourceNPM, Registry: "https://npm.other.example"}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-dir", Version: "9.9.9", Channel: directory.ChannelBeta,
			Digest: pluginSRI(t, "missing"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-other", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: fakeIntegrity(t, up, "dsh-plugin-other", "1.0.0"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "@acme/oa-plugin", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "scoped"), Source: directory.Source{Type: directory.SourceNPM}},
		{Kind: directory.KindPlugin, Name: "dsh-plugin-tarball", Version: "1.0.0", Channel: directory.ChannelStable,
			Digest: pluginSRI(t, "tarball"), Source: directory.Source{Type: directory.SourceTarball, URL: "https://cdn.example.com/tarball-1.0.0.tgz"}},
	}
	document := signedDirJSON(t, testDirectory(t, entries), dirPriv)
	code, body = admin.do("POST", "/api/admin/directory/apply", map[string]any{"document": document}, nil)
	expect(t, "登记插件目录", code, 200, body)
	rows := map[string]store.DirectoryEntry{}
	for _, e := range decodeAs[dirApplyResp](t, body).Entries {
		rows[e.Name+"@"+e.Version] = e
	}
	users := newDirTestUsers(t, admin, ts.URL)

	// 0. 列表权限：管插件的也能看目录；员工不行
	if code, body := users.plugins.do("GET", "/api/admin/directory", nil, nil); code != 200 {
		t.Fatalf("管插件的应能看目录：%d %s", code, body)
	}
	if code, _ := users.member.do("GET", "/api/admin/directory", nil, nil); code != 403 {
		t.Fatalf("员工不能看目录：%d", code)
	}

	// 1. 权限：员工不行；只有管连接的也不行（插件要 plugins.manage）
	code, _, _ = installDirEntry(t, users.member, rows["dsh-plugin-dir@2.0.0"].ID, false, nil)
	expect(t, "员工不能安装插件条目", code, 403, nil)
	code, _, raw := installDirEntry(t, users.conn, rows["dsh-plugin-dir@2.0.0"].ID, false, nil)
	expect(t, "管连接的不能装插件", code, 403, raw)

	// 2. 只出计划：不启动同步、不写版本
	code, preview, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.0.0"].ID, false, map[string]any{"plan_only": true})
	expect(t, "只出插件安装计划", code, 200, raw)
	if !preview.OK || !preview.PlanOnly || preview.Plugin == nil || preview.Plugin.SyncVersion != "2.0.0" || !preview.Plugin.ReviewRequired || preview.Result["status"] != "planned" {
		t.Fatalf("插件安装计划不对：%s", raw)
	}
	if n := countRows(t, st, "SELECT COUNT(*) FROM sync_jobs"); n != 0 {
		t.Fatalf("只出计划不能启动同步任务：%d", n)
	}
	if _, err := st.GetVersion(ctx, "dsh-plugin-dir", "2.0.0"); err == nil {
		t.Fatal("只出计划不能写版本")
	}

	// 3. 安装插件：走现有预载，版本是 pending（不绕过审核）
	code, installed, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.0.0"].ID, false, nil)
	expect(t, "安装目录里的插件", code, 202, raw)
	if !installed.OK || installed.Kind != directory.KindPlugin || installed.Plugin == nil ||
		installed.Plugin.SyncVersion != "2.0.0" || !installed.Plugin.ReviewRequired {
		t.Fatalf("插件安装计划不对：%s", raw)
	}
	job := waitStoreJob(t, st, dirInstallResultJob(t, installed))
	if job.Status != "succeeded" {
		t.Fatalf("预载任务失败：%+v", job)
	}
	pv, err := st.GetVersion(ctx, "dsh-plugin-dir", "2.0.0")
	if err != nil || pv.Status != store.StatusPending {
		t.Fatalf("插件版本应是 pending（待审核）：%+v %v", pv, err)
	}
	if n := countRows(t, st, "SELECT COUNT(*) FROM audit_logs WHERE action = 'directory.install'"); n != 1 {
		t.Fatalf("应有一条 directory.install 审计：%d", n)
	}

	// 4. 列表：这个版本已同步但待审核（未上架），新的 2.1.0 显示可升级
	views := map[string]dirEntryView{}
	for _, v := range directoryList(t, admin).Entries {
		views[v.Name+"@"+v.Version] = v
	}
	if row := views["dsh-plugin-dir@2.0.0"]; row.Installed || row.InstalledStatus != store.StatusPending || row.InstalledVersion != "2.0.0" {
		t.Fatalf("待审核版本的安装状态不对：%+v", row)
	}
	if row := views["dsh-plugin-dir@2.1.0"]; row.Installed || !row.UpgradeAvailable || row.InstalledVersion != "2.0.0" {
		t.Fatalf("新版本应显示可升级：%+v", row)
	}

	// 5. 再次安装：已同步、待审核，不是错误也不重复建任务
	code, again, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.0.0"].ID, false, nil)
	expect(t, "重复安装（待审核）", code, 200, raw)
	if !again.OK || again.Result["status"] != "pending" {
		t.Fatalf("重复安装应返回待审核状态：%s", raw)
	}

	// 6. 审核通过后列表显示已安装；再装：已经是这个版本
	code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "dsh-plugin-dir", "version": "2.0.0", "action": "approve"}, nil)
	expect(t, "审核通过", code, 200, body)
	code, already, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.0.0"].ID, false, nil)
	expect(t, "已上架后再安装", code, 200, raw)
	if !already.OK || already.Result["status"] != "installed" || already.Result["already"] != true {
		t.Fatalf("应返回已安装：%s", raw)
	}

	// 7. 升级到 2.1.0 前先只出计划：不启动任务；确认后再同步 → 待审核 → 审核 → 再升级是重复
	code, upgradePreview, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.1.0"].ID, true, map[string]any{"plan_only": true})
	expect(t, "只出插件升级计划", code, 200, raw)
	if !upgradePreview.PlanOnly || upgradePreview.Plugin == nil || upgradePreview.Plugin.SyncVersion != "2.1.0" || upgradePreview.Result["status"] != "planned" {
		t.Fatalf("插件升级计划不对：%s", raw)
	}
	jobsBefore := countRows(t, st, "SELECT COUNT(*) FROM sync_jobs")
	code, upResp, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.1.0"].ID, true, nil)
	expect(t, "升级插件", code, 202, raw)
	if n := countRows(t, st, "SELECT COUNT(*) FROM sync_jobs"); n != jobsBefore+1 {
		t.Fatalf("确认升级后应启动一个同步任务：%d → %d", jobsBefore, n)
	}
	if upResp.Plugin == nil || upResp.Plugin.SyncVersion != "2.1.0" || !upResp.Plugin.ReviewRequired {
		t.Fatalf("插件升级计划不对：%s", raw)
	}
	if j := waitStoreJob(t, st, dirInstallResultJob(t, upResp)); j.Status != "succeeded" {
		t.Fatalf("升级预载失败：%+v", j)
	}
	if v, err := st.GetVersion(ctx, "dsh-plugin-dir", "2.1.0"); err != nil || v.Status != store.StatusPending {
		t.Fatalf("升级版本应是 pending：%+v %v", v, err)
	}
	code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "dsh-plugin-dir", "version": "2.1.0", "action": "approve"}, nil)
	expect(t, "审核升级版本", code, 200, body)
	code, _, raw = installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.1.0"].ID, true, nil)
	expect(t, "重复升级", code, 409, raw)
	if !strings.Contains(string(raw), "没有可升级") {
		t.Fatalf("应说明已经是最新版：%s", raw)
	}
	// 对已上架的版本再 install：幂等
	code, already, raw = installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.1.0"].ID, false, nil)
	expect(t, "已上架的版本再安装", code, 200, raw)
	if already.Result["status"] != "installed" {
		t.Fatalf("应返回已安装：%s", raw)
	}

	// 8. 没装过的包不能「升级」
	code, _, raw = installDirEntry(t, users.plugins, rows["dsh-plugin-other@1.0.0"].ID, true, nil)
	expect(t, "没装过不能升级", code, 409, raw)
	if !strings.Contains(string(raw), "还没有") {
		t.Fatalf("应提示先安装：%s", raw)
	}

	// 9. 摘要不对：任务失败，版本不入库（失败不留半装）
	code, mismatch, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.2.0"].ID, false, nil)
	expect(t, "摘要不一致的插件照样起任务", code, 202, raw)
	failed := waitStoreJob(t, st, dirInstallResultJob(t, mismatch))
	if failed.Status != "failed" || !strings.Contains(failed.Log, "digest") {
		t.Fatalf("摘要不一致应让同步失败：%+v", failed)
	}
	if _, err := st.GetVersion(ctx, "dsh-plugin-dir", "2.2.0"); err == nil {
		t.Fatal("摘要不一致的版本不能入库")
	}

	// 10. 上游没有这个版本：同步失败，不入库
	code, missing, raw := installDirEntry(t, users.plugins, rows["dsh-plugin-dir@9.9.9"].ID, false, nil)
	expect(t, "上游没有的版本", code, 202, raw)
	missJob := waitStoreJob(t, st, dirInstallResultJob(t, missing))
	if missJob.Status != "failed" || !strings.Contains(missJob.Log, "Upstream has no version") {
		t.Fatalf("上游没有的版本应同步失败：%+v", missJob)
	}

	// 11. 来源 / 作用域不支持的条目直接拒绝
	code, _, raw = installDirEntry(t, users.plugins, rows["@acme/oa-plugin@1.0.0"].ID, false, nil)
	expect(t, "企业作用域不能从上游同步", code, 409, raw)
	if !strings.Contains(string(raw), "企业作用域") {
		t.Fatalf("应提示企业作用域：%s", raw)
	}
	code, _, raw = installDirEntry(t, users.plugins, rows["dsh-plugin-tarball@1.0.0"].ID, false, nil)
	expect(t, "插件不收 tarball 来源", code, 400, raw)
	if !strings.Contains(string(raw), "npm") {
		t.Fatalf("应提示只允许 npm：%s", raw)
	}
	code, _, raw = installDirEntry(t, users.plugins, rows["dsh-plugin-dir@2.3.0"].ID, false, nil)
	expect(t, "条目 registry 和上游不一致", code, 409, raw)
	if !strings.Contains(string(raw), "registry") {
		t.Fatalf("应提示 registry 不一致：%s", raw)
	}

	// 12. 审计：install（首次同步）、upgrade（2.1.0）各一条
	if n := countRows(t, st, "SELECT COUNT(*) FROM audit_logs WHERE action = 'directory.upgrade'"); n != 1 {
		t.Fatalf("应有一条 directory.upgrade 审计：%d", n)
	}
}
