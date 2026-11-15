package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 审批人评审的是申请 A，期间负责人又提交了 B 并覆盖 pending_scope（B 同样进审核）。
// 用看到 A 时记下的 scope_requested_at 审批：必须 ErrConflict，B 不被消费、范围不变；
// 只有用当前那份申请的标识才能通过。这条挡住「批的和看的不一致」的 TOCTOU。
func TestScopeResolveStaleRequestConflict(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	a := scopeTestAsset(t, st, "stale-scope", Scope{Users: []int64{1}})

	// 审批人在队列里看到的申请 A：分享给用户 1、2
	seenAt, err := st.RequestAssetScope(ctx, a.ID, Scope{Users: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("申请 A：%v", err)
	}
	seen, err := st.GetAsset(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if seen.ScopeRequestedAt == nil || !seen.ScopeRequestedAt.Equal(seenAt) {
		t.Fatalf("申请后应有快照标识 scope_requested_at：%+v", seen.ScopeRequestedAt)
	}

	// 管理员点通过前，负责人换成 B：全员
	sleepTurn()
	newAt, err := st.RequestAssetScope(ctx, a.ID, Scope{All: true}, nil)
	if err != nil {
		t.Fatalf("申请 B：%v", err)
	}
	if seenAt.Equal(newAt) {
		t.Fatalf("换个申请后 scope_requested_at 应更新：%v", seenAt)
	}

	// 拿旧快照审批：冲突，不消费 B，范围不变
	if ok, err := st.ResolveAssetScope(ctx, a.ID, true, "", seenAt); !errors.Is(err, ErrConflict) || ok {
		t.Fatalf("用旧快照审批应返回 ErrConflict，得到 ok=%v err=%v", ok, err)
	}
	cur, err := st.GetAsset(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cur.Scope.OnlyUser(1) || cur.PendingScope == nil || !cur.PendingScope.All {
		t.Fatalf("冲突时范围和申请都不应变：scope=%+v pending=%+v", cur.Scope, cur.PendingScope)
	}
	if cur.ScopeRequestedAt == nil || !cur.ScopeRequestedAt.Equal(newAt) {
		t.Fatalf("冲突后当前申请的快照标识不应变：%+v", cur.ScopeRequestedAt)
	}

	// 旧快照驳回同样不能消费新申请
	if ok, err := st.ResolveAssetScope(ctx, a.ID, false, "先别放开", seenAt); !errors.Is(err, ErrConflict) || ok {
		t.Fatalf("用旧快照驳回应返回 ErrConflict，得到 ok=%v err=%v", ok, err)
	}
	cur, _ = st.GetAsset(ctx, a.ID)
	if cur.PendingScope == nil || cur.ScopeNote != "" {
		t.Fatalf("驳回冲突不应改动申请：pending=%+v note=%q", cur.PendingScope, cur.ScopeNote)
	}

	// 令牌缺失（零值）不能绕过校验
	if ok, err := st.ResolveAssetScope(ctx, a.ID, true, "", time.Time{}); !errors.Is(err, ErrConflict) || ok {
		t.Fatalf("缺令牌应返回 ErrConflict，得到 ok=%v err=%v", ok, err)
	}

	// 用当前那份申请的标识才通过，生效的是 B
	if ok, err := st.ResolveAssetScope(ctx, a.ID, true, "", newAt); err != nil || !ok {
		t.Fatalf("用当前快照审批：%v %v", ok, err)
	}
	cur, _ = st.GetAsset(ctx, a.ID)
	if !cur.Scope.All || cur.PendingScope != nil || cur.ScopeRequestedAt != nil {
		t.Fatalf("通过后应生效 B 并清空申请：scope=%+v pending=%+v at=%+v", cur.Scope, cur.PendingScope, cur.ScopeRequestedAt)
	}

	// 已经处理过：没有在审申请，返回 false 而不是冲突
	if ok, err := st.ResolveAssetScope(ctx, a.ID, true, "", newAt); err != nil || ok {
		t.Fatalf("没有在审申请：%v %v", ok, err)
	}
}
