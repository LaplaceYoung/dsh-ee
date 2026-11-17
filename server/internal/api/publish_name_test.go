package api_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// TestPublishRejectsTraversalName 覆盖网页上传这条唯一绕过 npm 路由包名校验的发布入口：
// npm PUT /npm/<name> 在 handleNpm 里先用 ValidName 挡了一道，网页上传则直接走
// registry.PublishTarball → ValidPublishName。修复前两侧口径不一致，连续点的包会
// 入库、占住版本号，之后 npm GET 与网页审核又用 ValidName 一律 400，成了既不可读
// 也不可审的僵尸包。这里断言发布端和读端同样拒绝，且版本号没有被占用。
func TestPublishRejectsTraversalName(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	up := newFakeRegistry(t)
	ts := newRegistryServer(t, st, up)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 对照：正常包名的网页上传不受影响（默认审核模式 → 待审核 201）。
	okTgz := makeTgz(t, pluginPkg("@company/name-ok", "1.0.0", nil), bundleFile)
	code, body = admin.doRaw("POST", "/api/me/packages", okTgz, "application/octet-stream")
	expect(t, "合法包名可以发布", code, 201, body)

	for _, name := range []string{"@company/a..b", "@company/a...b", "@company/a..", "@company/a..b..c"} {
		tgz := makeTgz(t, pluginPkg(name, "1.0.0", nil), bundleFile)
		code, body = admin.doRaw("POST", "/api/me/packages", tgz, "application/octet-stream")
		expect(t, "连续点的包名应被拒绝："+name, code, 400, body)
		if _, err := st.GetVersion(ctx, name, "1.0.0"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s@1.0.0 不应入库（否则版本号被占用）：%v", name, err)
		}
	}
}
