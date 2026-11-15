package api_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 可用范围是整段覆盖的：管理端/负责人拿「打开页面时的旧快照」保存，会把期间
// 新审批进来的授权悄悄删掉。PATCH 带 updated_at 做乐观并发，过期就 409 要求刷新。
func TestHubScopePatchCAS(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 两个员工，用来模拟「期间新审批进来的授权」
	ids := map[string]int64{}
	clients := map[string]*client{}
	for _, name := range []string{"bob", "carol", "dave"} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": name, "password": name + "-password-1", "department": "研发效能部"}, nil)
		expect(t, "建用户", code, 201, body)
		ids[name] = decodeAs[struct{ ID int64 }](t, body).ID
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": name, "password": name + "-password-1"}, nil)
		expect(t, "登录", code, 200, body)
		clients[name] = c
	}
	dave := clients["dave"]

	// ---------- 能力 PATCH：缺令牌 400 / 旧令牌 409 / 刷新后成功且保留新授权 ----------
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "cas-app", "scope": map[string]any{"users": []int64{}},
		"spec": map[string]any{"title": "CAS", "description": "并发保存", "persona": "你是助手。"}}, nil)
	expect(t, "建应用", code, 201, body)
	created := decodeAs[struct{ Asset store.Asset }](t, body)
	appID, token0 := created.Asset.ID, created.Asset.UpdatedAt
	appPath := fmt.Sprintf("/api/hub/%d", appID)

	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{ids["bob"]}}}, nil)
	expect(t, "缺 updated_at 改范围", code, 400, body)
	if !strings.Contains(string(body), "updated_at") {
		t.Fatalf("400 文案应说明缺 updated_at：%s", body)
	}

	// 第一次保存（比如管理员 A）成功
	time.Sleep(2 * time.Millisecond) // updated_at 是毫秒精度，隔开两次写保证版本不同
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{ids["bob"]}}, "updated_at": token0}, nil)
	expect(t, "第一次保存范围", code, 200, body)

	// 管理员 B 还拿着旧快照（token0）保存：不能覆盖，返回 409
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{ids["carol"]}}, "updated_at": token0}, nil)
	expect(t, "旧快照保存范围", code, 409, body)
	if !strings.Contains(string(body), "范围已被其他人修改，请刷新后重试") {
		t.Fatalf("409 文案不对：%s", body)
	}

	// 旧快照那次没有落库，管理员 A 新加的 bob 还在
	code, body = admin.do("GET", appPath, nil, nil)
	expect(t, "读详情", code, 200, body)
	detail := decodeAs[struct{ Asset store.Asset }](t, body)
	if !slicesContain(detail.Asset.Scope.Users, ids["bob"]) || slicesContain(detail.Asset.Scope.Users, ids["carol"]) {
		t.Fatalf("冲突后范围应保持 bob、不能出现 carol：%+v", detail.Asset.Scope.Users)
	}

	// 刷新后基于新范围保存：bob 与 carol 都在
	fresh := detail.Asset.UpdatedAt.UTC().Format(time.RFC3339Nano)
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{ids["bob"], ids["carol"]}}, "updated_at": fresh}, nil)
	expect(t, "刷新后保存范围", code, 200, body)
	code, body = admin.do("GET", appPath, nil, nil)
	expect(t, "读详情", code, 200, body)
	detail = decodeAs[struct{ Asset store.Asset }](t, body)
	if !slicesContain(detail.Asset.Scope.Users, ids["bob"]) || !slicesContain(detail.Asset.Scope.Users, ids["carol"]) {
		t.Fatalf("刷新后保存应保留新授权：%+v", detail.Asset.Scope.Users)
	}

	// ---------- 负责人的「申请扩大范围」也带快照，过期同样 409 ----------
	code, body = dave.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "dave-app", "scope": map[string]any{"users": []int64{ids["dave"]}},
		"spec": map[string]any{"title": "自己用", "description": "d", "persona": "p"}}, nil)
	expect(t, "员工建自用应用", code, 201, body)
	own := decodeAs[struct{ Asset store.Asset }](t, body)
	ownPath := fmt.Sprintf("/api/hub/%d", own.Asset.ID)
	code, body = dave.do("GET", ownPath, nil, nil)
	expect(t, "读自己的应用", code, 200, body)
	ownToken := decodeAs[struct{ Asset store.Asset }](t, body).Asset.UpdatedAt
	code, body = dave.do("PATCH", ownPath, map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}}, "updated_at": ownToken}, nil)
	expect(t, "申请扩大范围", code, 200, body)
	if decodeAs[store.Asset](t, body).PendingScope == nil {
		t.Fatalf("扩大范围应进审核：%s", body)
	}

	// ---------- KB PATCH：同样要求 updated_at ----------
	code, body = admin.do("POST", "/api/kb", map[string]any{"slug": "cas-kb", "title": "CAS 知识库"}, nil)
	expect(t, "建知识库", code, 201, body)
	kbID := decodeAs[struct{ ID int64 }](t, body).ID
	kbPath := "/api/kb/" + itoa(kbID)
	code, body = admin.do("GET", kbPath, nil, nil)
	expect(t, "读知识库", code, 200, body)
	kbToken := scopeToken(t, admin, kbPath)

	code, body = admin.do("PATCH", kbPath, map[string]any{"maintainers": map[string]any{"users": []int64{ids["bob"]}}}, nil)
	expect(t, "缺 updated_at 改维护者", code, 400, body)
	time.Sleep(2 * time.Millisecond)
	code, body = admin.do("PATCH", kbPath, map[string]any{"maintainers": map[string]any{"users": []int64{ids["bob"]}}, "updated_at": kbToken}, nil)
	expect(t, "设维护者", code, 200, body)
	code, body = admin.do("PATCH", kbPath, map[string]any{"maintainers": map[string]any{"users": []int64{ids["carol"]}}, "updated_at": kbToken}, nil)
	expect(t, "旧快照改维护者", code, 409, body)
	if !strings.Contains(string(body), "范围或维护者已被其他人修改，请刷新后重试") {
		t.Fatalf("KB 409 文案不对：%s", body)
	}
	kbToken = scopeToken(t, admin, kbPath)
	code, body = admin.do("PATCH", kbPath, map[string]any{"maintainers": map[string]any{"users": []int64{ids["bob"], ids["carol"]}}, "updated_at": kbToken}, nil)
	expect(t, "刷新后改维护者", code, 200, body)

	// ---------- 插件策略正在设置 ----------
	if err := st.EnsureUpstreamPackage(ctx, &store.NpmPackage{Name: "dsh-cas", IsPlugin: true, Title: "CAS",
		DistTags: map[string]string{"latest": "1.0.0"}}); err != nil {
		t.Fatalf("建包：%v", err)
	}
	policyPath := "/api/admin/plugin-policies/dsh-cas"
	code, body = admin.do("PUT", policyPath, map[string]any{"scope": map[string]any{"users": []int64{ids["bob"]}}, "delivery": "optional"}, nil)
	expect(t, "首次设置插件策略", code, 200, body)
	first := decodeAs[store.PluginPolicy](t, body)
	time.Sleep(2 * time.Millisecond)
	code, body = admin.do("PUT", policyPath, map[string]any{"scope": map[string]any{"users": []int64{ids["bob"], ids["carol"]}},
		"delivery": "auto", "updated_at": first.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil)
	expect(t, "更新插件策略", code, 200, body)
	code, body = admin.do("PUT", policyPath, map[string]any{"scope": map[string]any{"all": true}, "delivery": "optional",
		"updated_at": first.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil)
	expect(t, "旧快照改插件策略", code, 409, body)
	if !strings.Contains(string(body), "插件策略已被其他人修改，请刷新后重试") {
		t.Fatalf("插件策略 409 文案不对：%s", body)
	}
}

// slicesContain 是测试里的小工具（避免为一次包含判断引入泛型断言噪音）。
func slicesContain(xs []int64, want int64) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// 把范围改成不需要审核时，在审的范围申请会一并撤销；撤销和保存共享同一个 updated_at
// 令牌，所以旧令牌既不能保存范围也不能清掉之后并发提交的新申请。
func TestHubClearScopeRequestCAS(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "dave", "password": "dave-password-1", "department": "研发效能部"}, nil)
	expect(t, "建用户", code, 201, body)
	daveID := decodeAs[struct{ ID int64 }](t, body).ID
	dave := newClient(t, ts.URL)
	code, body = dave.do("POST", "/api/auth/login", map[string]string{"username": "dave", "password": "dave-password-1"}, nil)
	expect(t, "登录", code, 200, body)

	// 负责人建一个自用应用，然后申请扩大范围（默认审核模式下会进审核）
	code, body = dave.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "clear-cas-app",
		"scope": map[string]any{"users": []int64{daveID}},
		"spec":  map[string]any{"title": "清空申请", "description": "d", "persona": "p"}}, nil)
	expect(t, "建自用应用", code, 201, body)
	appID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID
	appPath := "/api/hub/" + itoa(appID)
	tokenBefore := scopeToken(t, dave, appPath)
	time.Sleep(2 * time.Millisecond) // updated_at 毫秒精度：让申请写入的版本与旧令牌不同

	code, body = dave.do("PATCH", appPath, map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}}, "updated_at": tokenBefore}, nil)
	expect(t, "申请扩大范围", code, 200, body)
	if decodeAs[store.Asset](t, body).PendingScope == nil {
		t.Fatalf("扩大范围应进审核：%s", body)
	}
	tokenWithPending := scopeToken(t, dave, appPath)

	// 管理员拿申请前的旧令牌直接保存范围：必须 409，且不能清掉在审的申请
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{daveID}}, "updated_at": tokenBefore}, nil)
	expect(t, "旧令牌保存范围（顺带清空申请）", code, 409, body)
	if !strings.Contains(string(body), "范围已被其他人修改，请刷新后重试") {
		t.Fatalf("409 文案不对：%s", body)
	}
	code, body = admin.do("GET", appPath, nil, nil)
	expect(t, "读详情", code, 200, body)
	if decodeAs[struct{ Asset store.Asset }](t, body).Asset.PendingScope == nil {
		t.Fatal("409 时在审的申请不能被清掉")
	}

	// 刷新令牌后保存：范围生效，在审的申请被同一个 UPDATE 清掉
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{daveID}}, "updated_at": tokenWithPending}, nil)
	expect(t, "刷新令牌保存范围并清空申请", code, 200, body)
	if pending := decodeAs[store.Asset](t, body).PendingScope; pending != nil {
		t.Fatalf("保存成无需审核的范围后申请应被撤销：%+v", pending)
	}
	code, body = admin.do("GET", appPath, nil, nil)
	expect(t, "读详情", code, 200, body)
	if pending := decodeAs[struct{ Asset store.Asset }](t, body).Asset.PendingScope; pending != nil {
		t.Fatalf("详情里申请应已清空：%+v", pending)
	}

	// 保存之后又有人提交了新的申请：拿保存前的旧令牌再保存必须 409，新申请不能被清掉
	beforeSecond := scopeToken(t, dave, appPath)
	time.Sleep(2 * time.Millisecond) // 同上：保证第二次申请写入的版本与旧令牌不同
	code, body = dave.do("PATCH", appPath, map[string]any{"scope": map[string]any{"departments": []string{"研发效能部"}}, "updated_at": beforeSecond}, nil)
	expect(t, "第二次申请扩大范围", code, 200, body)
	code, body = admin.do("PATCH", appPath, map[string]any{"scope": map[string]any{"users": []int64{daveID}}, "updated_at": beforeSecond}, nil)
	expect(t, "旧令牌清空新申请", code, 409, body)
	code, body = admin.do("GET", appPath, nil, nil)
	expect(t, "读详情", code, 200, body)
	if pending := decodeAs[struct{ Asset store.Asset }](t, body).Asset.PendingScope; pending == nil {
		t.Fatal("并发提交的新申请不能被旧令牌清掉")
	}

	// 插件策略：第一次设置可以不带令牌，但已有策略时再「首次设置」必须 409 而不是覆盖
	if err := st.EnsureUpstreamPackage(ctx, &store.NpmPackage{Name: "dsh-clear", IsPlugin: true, Title: "Clear",
		DistTags: map[string]string{"latest": "1.0.0"}}); err != nil {
		t.Fatalf("建包：%v", err)
	}
	policyPath := "/api/admin/plugin-policies/dsh-clear"
	code, body = admin.do("PUT", policyPath, map[string]any{"scope": map[string]any{"users": []int64{daveID}}, "delivery": "optional"}, nil)
	expect(t, "首次设置插件策略", code, 200, body)
	code, body = admin.do("PUT", policyPath, map[string]any{"scope": map[string]any{"all": true}, "delivery": "auto"}, nil)
	expect(t, "已有策略时再次首次设置", code, 409, body)
	if !strings.Contains(string(body), "插件策略已被其他人修改，请刷新后重试") {
		t.Fatalf("插件策略 409 文案不对：%s", body)
	}
}
