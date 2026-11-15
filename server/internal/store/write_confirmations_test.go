package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// 写确认记录过期清理：只删过期一天以上的；刚过期还在宽限窗口内的和没过期的都保留。
// 需要数据库（DSH_EE_TEST_DSN），和 package 内其它 DB 测试一样。
func TestPurgeWriteConfirmations(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)

	newConfirm := func(ttl time.Duration) *WriteConfirmation {
		t.Helper()
		c := &WriteConfirmation{
			UserID: u.ID, Tool: "local:acme/legacy-oa/submit_leave",
			ArgsHash: secret.Hash("args"), Args: `{"days":2}`, Client: "test",
		}
		if err := st.CreateWriteConfirmation(ctx, c, secret.Hash(secret.Token()), ttl); err != nil {
			t.Fatal(err)
		}
		return c
	}
	expired := newConfirm(-25 * time.Hour) // 过期超过一天：应删
	grace := newConfirm(-23 * time.Hour)   // 刚过期、还在宽限窗口内：保留
	fresh := newConfirm(time.Hour)         // 未过期：保留

	if err := st.PurgeWriteConfirmations(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetWriteConfirmation(ctx, expired.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("过期超过一天的写确认应被清理：%v", err)
	}
	for _, c := range []*WriteConfirmation{grace, fresh} {
		if _, err := st.GetWriteConfirmation(ctx, c.ID); err != nil {
			t.Fatalf("写确认 %d 不应被清理：%v", c.ID, err)
		}
	}
}
