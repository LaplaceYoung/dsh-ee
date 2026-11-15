package store

import (
	"context"
	"errors"
	"testing"
)

// ReopenAccessRequest 只退回 approved 的申请：网页/飞书审批抢占成功之后授权失败时用它把申请放回
// 待处理，让审批人可以重做授权；已经被驳回的申请不能因为一次回滚又被打开。
func TestAccessRequestReopenOnlyApproved(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)

	// 已驳回：不能退回，状态不动。
	rejectedID, existed, err := st.CreateAccessRequest(ctx, u.ID, RequestSkill, 900001, 0, "先驳回")
	if err != nil || existed {
		t.Fatalf("准备驳回的申请：existed=%v err=%v", existed, err)
	}
	if err := st.ResolveAccessRequest(ctx, rejectedID, "rejected", u.ID, "不给"); err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenAccessRequest(ctx, rejectedID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("驳回的申请不能退回，应返回 ErrNotFound：%v", err)
	}
	if q, err := st.GetAccessRequest(ctx, rejectedID); err != nil || q.Status != "rejected" || q.Note != "不给" {
		t.Fatalf("驳回状态不该被改动：%+v err=%v", q, err)
	}

	// 已通过：退回 pending，清掉审批人、处理时间和说明。
	approvedID, existed, err := st.CreateAccessRequest(ctx, u.ID, RequestSkill, 900002, 0, "先通过")
	if err != nil || existed {
		t.Fatalf("准备通过的申请：existed=%v err=%v", existed, err)
	}
	if err := st.ResolveAccessRequest(ctx, approvedID, "approved", u.ID, "授权失败要退回"); err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenAccessRequest(ctx, approvedID); err != nil {
		t.Fatalf("已通过的申请应能退回：%v", err)
	}
	q, err := st.GetAccessRequest(ctx, approvedID)
	if err != nil {
		t.Fatal(err)
	}
	if q.Status != "pending" || q.HandledBy != nil || q.HandledAt != nil || q.Note != "" {
		t.Fatalf("退回后应回到干净的待处理状态：%+v", q)
	}

	// 再退回一次：已经不是 approved，ErrNotFound（不会把 pending 又处理一遍）。
	if err := st.ReopenAccessRequest(ctx, approvedID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("重复退回应返回 ErrNotFound：%v", err)
	}
	// 退回的申请可以重新被审批：CAS 的条件仍是 pending。
	if err := st.ResolveAccessRequest(ctx, approvedID, "approved", u.ID, ""); err != nil {
		t.Fatalf("退回后应能重新审批：%v", err)
	}
}
