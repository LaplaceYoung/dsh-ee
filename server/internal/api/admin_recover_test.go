package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// TestAdminRecover 覆盖 `dsh-ee admin recover`（应急全权账号）的两条关键路径。
// Recover 在 api 层、只依赖 *store.Store，所以这里不起 HTTP 服务，直接调函数。
func TestAdminRecover(t *testing.T) {
	t.Run("审计正常写入", func(t *testing.T) {
		st := openTestStore(t)
		ctx := context.Background()

		user, pass, expires, err := api.Recover(ctx, st)
		if err != nil {
			t.Fatalf("审计可写时 Recover 不应失败：%v", err)
		}
		if !strings.HasPrefix(user, "recover-") {
			t.Fatalf("用户名前缀不对：%q", user)
		}
		if d := time.Until(expires); d < 55*time.Minute || d > 65*time.Minute {
			t.Fatalf("到期时间应约为 1 小时后，实际 %v", d)
		}
		u := mustRecoverUser(t, st, user)
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(pass)); err != nil {
			t.Fatalf("返回的密码与库里的密码哈希对不上：%v", err)
		}
		if ok, err := st.IsRecoveryAccount(ctx, u.ID); err != nil || !ok {
			t.Fatalf("应存在未过期的 recovery_accounts 记录：ok=%v err=%v", ok, err)
		}
		logs, err := st.ListAudit(ctx, store.AuditFilter{Action: "admin.recover", Limit: 10})
		if err != nil {
			t.Fatalf("查审计失败：%v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("应恰好有 1 条 admin.recover 审计，实际 %d 条", len(logs))
		}
		if a := logs[0]; a.Target != user || a.ActorName != user || a.Via != "cli" {
			t.Fatalf("审计内容不对：target=%q actor=%q via=%q", a.Target, a.ActorName, a.Via)
		}
		if logs[0].Detail["expires_at"] == nil {
			t.Fatalf("审计 detail 里应记下 expires_at，实际 %v", logs[0].Detail)
		}
	})

	t.Run("审计表不可写", func(t *testing.T) {
		st := openTestStore(t)
		ctx := context.Background()
		// 构造审计写入失败：把 audit_logs 改名，InsertAudit 会报 table doesn't exist。
		if _, err := st.DB().ExecContext(ctx, "RENAME TABLE audit_logs TO audit_logs_bak"); err != nil {
			t.Fatalf("改名 audit_logs 以构造审计失败场景：%v", err)
		}
		t.Cleanup(func() {
			if _, err := st.DB().ExecContext(context.Background(), "RENAME TABLE audit_logs_bak TO audit_logs"); err != nil {
				t.Errorf("恢复 audit_logs 表名失败：%v", err)
			}
		})

		user, pass, expires, err := api.Recover(ctx, st)
		if err == nil {
			t.Fatal("审计写入失败时 Recover 必须返回错误，不能静默吞掉")
		}
		for _, want := range []string{"已创建", "audit_logs", "人工"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("错误信息应能让操作人看懂并处置（缺少 %q）：%v", want, err)
			}
		}
		// 关键：账号已经建好了，凭据和到期时间不能因为报错就丢掉（否则管理员会以为没拿到入口）。
		if user == "" || pass == "" || expires.IsZero() {
			t.Fatalf("审计失败时账号已创建，必须照常带回凭据；实际 username=%q password=%q expires=%v", user, pass, expires)
		}
		u := mustRecoverUser(t, st, user)
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(pass)); err != nil {
			t.Fatalf("返回的密码与库里的密码哈希对不上：%v", err)
		}
		if ok, err := st.IsRecoveryAccount(ctx, u.ID); err != nil || !ok {
			t.Fatalf("账号应已完整创建为恢复账号：ok=%v err=%v", ok, err)
		}
	})
}

func mustRecoverUser(t *testing.T, st *store.Store, username string) *store.User {
	t.Helper()
	u, err := st.GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatalf("库里找不到返回的账号 %q：%v", username, err)
	}
	return u
}
