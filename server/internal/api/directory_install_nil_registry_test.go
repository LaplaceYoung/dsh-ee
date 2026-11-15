package api_test

// 数据目录不可写时 newRegistry 返回 nil，服务仍然启动：npm 相关接口应回 503 而不是
// nil 解引用 panic（net/http recover 后请求直接断连 / 500 并留栈）。这里从 HTTP 入口验证。

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// TestDirectoryInstallWithoutRegistry 验证 registry 为 nil（数据目录建不出来）时：
//   - 目录里的插件条目 install / 升级回 503 固定文案，不 panic；
//   - 扩展条目仍走自己的校验（这里来源地址为空 → 400），没有被 npm 源不可用拦掉。
func TestDirectoryInstallWithoutRegistry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if ok, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil || !ok {
		t.Fatalf("bootstrap: %v %v", ok, err)
	}
	admin, err := st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("读管理员：%v", err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 数据目录的父路径是一个普通文件 ⇒ os.MkdirAll 必然失败（对 root 也成立），
	// newRegistry 返回 nil，但 api.New 仍然返回可用服务。
	blocker := filepath.Join(t.TempDir(), "data-dir-blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := api.New(st, box, api.Options{DataDir: filepath.Join(blocker, "data"), Version: "0.2.0"}, logger)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	adminClient := newClient(t, ts.URL)
	code, body := adminClient.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)

	// 配一个上游 registry：没有它时插件安装会在到达 npm 源之前就以「离线模式」结束，
	// 测不到这次要修的 nil 解引用。
	code, body = adminClient.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, http.StatusOK, body)
	set := decodeAs[map[string]any](t, body)
	set["upstream"].(map[string]any)["registry"] = "custom"
	set["upstream"].(map[string]any)["customRegistry"] = "https://npm.example.invalid"
	code, body = adminClient.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "保存上游设置", code, http.StatusOK, body)

	// 直接登记两条条目（本用例只验证安装入口的判空，不重复测目录签名 / apply）。
	if _, err := st.ApplyDirectoryEntries(ctx, "acme-prod", strings.Repeat("d", 64), admin.ID, []store.DirectoryEntry{
		{
			Kind: directory.KindPlugin, Name: "dsh-plugin-nil", Version: "1.0.0",
			Channel: directory.ChannelStable, Digest: pluginSRI(t, "registry-nil"),
			SourceType: directory.SourceNPM,
		},
		{
			Kind: directory.KindExtension, Name: "acme/nil-ext", Version: "1.0.0",
			Channel: directory.ChannelStable, Digest: strings.Repeat("a", 64),
			SourceType: directory.SourceTarball, SourceURL: "",
		},
	}); err != nil {
		t.Fatalf("登记目录条目：%v", err)
	}
	entries, err := st.ListDirectoryEntries(ctx)
	if err != nil {
		t.Fatalf("读目录条目：%v", err)
	}
	ids := map[string]int64{}
	for _, e := range entries {
		ids[e.Name+"@"+e.Version] = e.ID
	}
	pluginID, ok := ids["dsh-plugin-nil@1.0.0"]
	if !ok {
		t.Fatalf("没有插件条目：%+v", ids)
	}
	extID, ok := ids["acme/nil-ext@1.0.0"]
	if !ok {
		t.Fatalf("没有扩展条目：%+v", ids)
	}

	// 1. 插件 install / upgrade：503 + 固定文案，响应里没有内部错误。
	for _, action := range []string{"install", "upgrade"} {
		code, body := adminClient.do("POST", "/api/admin/directory/entries/"+itoa(pluginID)+"/"+action, nil, nil)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("插件 %s 在 registry 为 nil 时应回 503，得到 %d：%s", action, code, body)
		}
		var got struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("插件 %s 的 503 响应不是 JSON：%v；%s", action, err, body)
		}
		const want = "npm 源暂不可用：服务端数据目录未初始化或不可写"
		if got.Error != want {
			t.Fatalf("插件 %s 的 503 文案应固定且不含内部错误，得到 %q", action, got.Error)
		}
	}

	// 2. 扩展条目不受影响：走自己的来源校验（空地址回 400），不是 npm 源不可用的 503。
	code, body = adminClient.do("POST", "/api/admin/directory/entries/"+itoa(extID)+"/install", nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("扩展条目应按来源校验回 400，得到 %d：%s", code, body)
	}
	if strings.Contains(string(body), "npm 源") {
		t.Fatalf("扩展条目不应被 npm 源不可用挡住：%s", body)
	}
}
