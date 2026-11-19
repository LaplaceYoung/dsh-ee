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

// 审批人评审的是申请 A，点通过前负责人换成了更大的申请 B（同样进审核）。
// 用 A 的 scope_requested_at 审批必须 409 且不生效；用审核队列里的当前标识才能通过。
// 不带 scope_requested_at 的旧客户端按契约收紧返回 400。
func TestHubScopeReviewStaleToken(t *testing.T) {
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
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "alice", "password": "alice-password-1", "department": "销售部"}, nil)
	expect(t, "建用户", code, 201, body)
	aliceID := decodeAs[struct{ ID int64 }](t, body).ID
	alice := newClient(t, ts.URL)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	// 自己用的应用直接发布
	code, body = alice.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "toctou-app", "scope": map[string]any{"users": []int64{aliceID}},
		"spec": map[string]any{"title": "TOCTOU 应用", "description": "复现范围审批 TOCTOU", "persona": "你是销售助理。"}}, nil)
	expect(t, "自己用的应用", code, 201, body)
	appID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID
	path := fmt.Sprintf("/api/hub/%d", appID)

	// 申请 A：分享给销售部 —— 审核人看到的就是这份
	code, body = alice.do("PATCH", path, map[string]any{"scope": map[string]any{"departments": []string{"销售部"}}, "updated_at": scopeToken(t, alice, path)}, nil)
	expect(t, "申请 A", code, 200, body)
	tokenA := decodeAs[store.Asset](t, body).ScopeRequestedAt

	// 点通过前，负责人换成 B：全员（同样进审核）
	code, body = alice.do("PATCH", path, map[string]any{"scope": map[string]any{"all": true}, "updated_at": scopeToken(t, alice, path)}, nil)
	expect(t, "申请 B", code, 200, body)
	tokenB := decodeAs[store.Asset](t, body).ScopeRequestedAt
	if tokenA == nil || tokenB == nil || tokenA.Equal(*tokenB) {
		t.Fatalf("两次申请的 scope_requested_at 应不同：A=%v B=%v", tokenA, tokenB)
	}

	// 旧客户端不带令牌：400，明确要求刷新
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": appID, "action": "approve"}, nil)
	expect(t, "不带快照标识", code, 400, body)
	if text := string(body); !strings.Contains(text, "scope_requested_at") || !strings.Contains(text, "刷新") {
		t.Fatalf("400 文案应说明要带 scope_requested_at 并刷新：%s", body)
	}

	// 用 A 的标识审批：409，B 不被消费、范围不变
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": appID, "action": "approve", "scope_requested_at": tokenA}, nil)
	expect(t, "旧快照审批", code, 409, body)
	if text := decodeAs[struct{ Error string }](t, body).Error; !strings.Contains(text, "已被更新") || !strings.Contains(text, "刷新") {
		t.Fatalf("409 文案应提示申请已更新、刷新后重审：%s", body)
	}
	det := decodeAs[struct{ Asset store.Asset }](t, mustGet(t, admin, path))
	if det.Asset.Scope.All || det.Asset.PendingScope == nil || !det.Asset.PendingScope.All {
		t.Fatalf("旧标识不能消费新申请：scope=%+v pending=%+v", det.Asset.Scope, det.Asset.PendingScope)
	}
	if det.Asset.ScopeRequestedAt == nil || !det.Asset.ScopeRequestedAt.Equal(*tokenB) {
		t.Fatalf("当前在审申请的标识应还是 B：%+v tokenB=%v", det.Asset.ScopeRequestedAt, tokenB)
	}

	// 用审核队列里当前那份申请的标识：204，B 生效
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": appID, "action": "approve", "scope_requested_at": tokenB}, nil)
	expect(t, "当前快照审批", code, 204, body)
	det = decodeAs[struct{ Asset store.Asset }](t, mustGet(t, admin, path))
	if !det.Asset.Scope.All || det.Asset.PendingScope != nil {
		t.Fatalf("通过后应生效 B 并清空申请：scope=%+v pending=%+v", det.Asset.Scope, det.Asset.PendingScope)
	}

	// 审计取证：review.approve 的 detail 记的是实际生效的 requested_at
	logs, err := st.ListAudit(ctx, store.AuditFilter{Action: "review.approve", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	audited := false
	for _, l := range logs {
		if !strings.Contains(l.Target, "#scope") {
			continue
		}
		raw, _ := l.Detail["requested_at"].(string)
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || !at.Equal(*tokenB) {
			t.Fatalf("审计 detail.requested_at 应是实际生效的 %v，得到 %q（%v）", tokenB, raw, err)
		}
		audited = true
	}
	if !audited {
		t.Fatalf("范围审批审计里应记录 requested_at：%+v", logs)
	}
}

// 范围申请审核中，负责人又提交了新版本：新版本不能因为「现在还只有自己能用」就免审直接上线，
// 否则审核人看的是旧内容、批的是范围，一通过就把没人审过的新内容推给了全员。
func TestHubNewVersionDuringScopeRequestNeedsReview(t *testing.T) {
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
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "alice", "password": "alice-password-1", "department": "销售部"}, nil)
	expect(t, "建用户", code, 201, body)
	aliceID := decodeAs[struct{ ID int64 }](t, body).ID
	alice := newClient(t, ts.URL)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	spec := func(persona string) map[string]any {
		return map[string]any{"title": "销售助理", "description": "帮销售查客户", "persona": persona}
	}
	code, body = alice.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "sales-helper", "scope": map[string]any{"users": []int64{aliceID}}, "spec": spec("你是销售助理。")}, nil)
	expect(t, "自己用的应用直接发布", code, 201, body)
	appID := decodeAs[struct{ Asset store.Asset }](t, body).Asset.ID
	path := fmt.Sprintf("/api/hub/%d", appID)

	// 申请分享给全员：进审核
	code, body = alice.do("PATCH", path, map[string]any{"scope": map[string]any{"all": true}, "updated_at": scopeToken(t, alice, path)}, nil)
	expect(t, "申请全员", code, 200, body)
	token := decodeAs[store.Asset](t, body).ScopeRequestedAt

	// 审核期间提交新版本：要跟着进审核，不能直接上线
	code, body = alice.do("POST", path+"/versions", map[string]any{"spec": spec("你是销售助理。【未经审核的新内容】")}, nil)
	expect(t, "审核期间提交新版本", code, 201, body)
	if status := decodeAs[struct{ Status string }](t, body).Status; status != "pending" {
		t.Fatalf("范围申请审核中，新版本应进审核而不是 %s", status)
	}

	// 审核人批准范围：全员看到的仍是审过的旧内容
	code, body = admin.do("POST", "/api/admin/hub/scope-review", map[string]any{"asset_id": appID, "action": "approve", "scope_requested_at": token}, nil)
	expect(t, "批准范围", code, 204, body)
	det := decodeAs[struct {
		Asset struct {
			Scope store.Scope
			Spec  struct{ Persona string }
		}
	}](t, mustGet(t, admin, path))
	if !det.Asset.Scope.All || strings.Contains(det.Asset.Spec.Persona, "未经审核") {
		t.Fatalf("批准范围后线上应仍是审过的内容：scope=%+v persona=%q", det.Asset.Scope, det.Asset.Spec.Persona)
	}
}
