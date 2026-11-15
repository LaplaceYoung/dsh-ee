package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 固定窗口：limit 次放行、第 limit+1 次起拒绝，计数封顶 limit+1，不同 key 互不影响。
func TestSharedRateLimitFixedWindow(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	window := time.Minute
	start := time.Date(2026, 11, 13, 10, 30, 0, 0, time.UTC)
	key := "token:" + strings.Repeat("a", 64)

	for i, wantRemaining := range []int{2, 1, 0} {
		got, err := st.TakeSharedRateLimit(ctx, key, start, window, 3)
		if err != nil {
			t.Fatalf("第 %d 次计数：%v", i+1, err)
		}
		if !got.Allowed {
			t.Fatalf("第 %d 次应放行：%+v", i+1, got)
		}
		if got.Limit != 3 || got.Remaining != wantRemaining {
			t.Fatalf("第 %d 次状态不对：%+v，期望 remaining=%d", i+1, got, wantRemaining)
		}
		if want := start.Add(window); !got.Reset.Equal(want) {
			t.Fatalf("Reset 应为窗口结束 %v，实际 %v", want, got.Reset)
		}
	}

	// 超限后不再放行，且计数封顶在 limit+1（攻击者没法把一行计数撑大）。
	for i := 0; i < 3; i++ {
		got, err := st.TakeSharedRateLimit(ctx, key, start, window, 3)
		if err != nil {
			t.Fatalf("超限后计数：%v", err)
		}
		if got.Allowed {
			t.Fatalf("超限后不应放行：%+v", got)
		}
		if got.Remaining != 0 {
			t.Fatalf("超限后 remaining 应为 0：%+v", got)
		}
	}
	var count int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT count FROM shared_rate_limits WHERE rate_key = ? AND window_start = ?", key, start).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("计数应封顶在 limit+1=4，实际 %d", count)
	}

	// 另一个 key 有自己的额度，不受影响。
	other, err := st.TakeSharedRateLimit(ctx, "token:"+strings.Repeat("b", 64), start, window, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !other.Allowed || other.Remaining != 2 {
		t.Fatalf("另一个 key 应独立计数：%+v", other)
	}
}

// 窗口翻页后重新计数；上一窗口的行保留到后台清理。
func TestSharedRateLimitWindowResets(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	window := time.Minute
	start := time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC)
	key := "token:" + strings.Repeat("c", 64)

	if got, err := st.TakeSharedRateLimit(ctx, key, start, window, 1); err != nil || !got.Allowed || got.Remaining != 0 {
		t.Fatalf("第一个窗口的第一次：%+v %v", got, err)
	}
	if got, err := st.TakeSharedRateLimit(ctx, key, start, window, 1); err != nil || got.Allowed {
		t.Fatalf("同一窗口第二次应超限：%+v %v", got, err)
	}

	// 下一个窗口从 0 重新计数，Reset 指向新窗口结束。
	next := start.Add(window)
	got, err := st.TakeSharedRateLimit(ctx, key, next, window, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.Remaining != 0 {
		t.Fatalf("窗口翻页后应重新放行：%+v", got)
	}
	if want := next.Add(window); !got.Reset.Equal(want) {
		t.Fatalf("新窗口 Reset 应为 %v，实际 %v", want, got.Reset)
	}
	var rows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM shared_rate_limits WHERE rate_key = ?", key).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("两个窗口应各留一行（等后台清理），实际 %d 行", rows)
	}
}

// 多实例：两个独立连接池（模拟两个进程）连同一个库，额度是合计的，不会各算一份。
func TestSharedRateLimitSharedAcrossInstances(t *testing.T) {
	st, dsn := openTokenTestStoreDSN(t)
	other, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("开第二个连接池：%v", err)
	}
	t.Cleanup(func() { other.Close() })
	ctx := context.Background()
	window := time.Minute
	start := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	key := "token:" + strings.Repeat("d", 64)

	// A 实例用掉 3 次，B 实例用掉 2 次：两边看到的是同一份额度（limit=5）。
	for _, inst := range []*Store{st, st, st, other, other} {
		got, err := inst.TakeSharedRateLimit(ctx, key, start, window, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Allowed {
			t.Fatalf("额度内应放行：%+v", got)
		}
	}
	// 第 6 次在任一实例上都应被拒绝：B 实例读到的 remaining 与 A 一致。
	for _, inst := range []*Store{st, other} {
		got, err := inst.TakeSharedRateLimit(ctx, key, start, window, 5)
		if err != nil {
			t.Fatal(err)
		}
		if got.Allowed || got.Remaining != 0 {
			t.Fatalf("跨实例的额度应合计：%+v", got)
		}
	}
}

// 后台清理只删已经结束的窗口，重复执行幂等；当前窗口的行保留。
func TestSharedRateLimitCleanup(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	window := time.Minute
	current := time.Now().UTC().Truncate(window)
	old := current.Add(-2 * window)

	for _, k := range []string{"token:old-a", "token:old-b"} {
		if _, err := st.TakeSharedRateLimit(ctx, k, old, window, 10); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.TakeSharedRateLimit(ctx, "token:current", current, window, 10); err != nil {
		t.Fatal(err)
	}

	n, err := st.DeleteOldSharedRateLimits(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应清理 2 个过期窗口，实际 %d", n)
	}
	// 幂等：再跑一次没有可删的。
	if n, err := st.DeleteOldSharedRateLimits(ctx, current); err != nil || n != 0 {
		t.Fatalf("重复清理应无副作用：n=%d err=%v", n, err)
	}
	var rows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM shared_rate_limits").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("当前窗口的行应保留，实际 %d 行", rows)
	}
}

// limit <= 0 视为不限流：直接放行，不落库、不报错。
func TestSharedRateLimitDisabled(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	got, err := st.TakeSharedRateLimit(ctx, "token:x", time.Now(), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.Limit != 0 || got.Remaining != 0 {
		t.Fatalf("关闭限流时应直接放行：%+v", got)
	}
	var rows int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM shared_rate_limits").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("关闭限流时不该写库，实际 %d 行", rows)
	}
}

// 数据库错误要原样返回给调用方（由 api 层决定降级策略），不能被吞掉。
func TestSharedRateLimitDatabaseError(t *testing.T) {
	st, dsn := openTokenTestStoreDSN(t)
	closed, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()

	ctx := context.Background()
	window := time.Minute
	start := time.Now().UTC().Truncate(window)
	if _, err := closed.TakeSharedRateLimit(ctx, "token:x", start, window, 10); err == nil {
		t.Fatal("连接池已关闭时应返回错误")
	}
	if _, err := closed.DeleteOldSharedRateLimits(ctx, start); err == nil {
		t.Fatal("连接池已关闭时清理应返回错误")
	}

	// 迁移缺失/表被删的错误同样要暴露出来。
	if _, err := st.DB().ExecContext(ctx, "DROP TABLE shared_rate_limits"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.TakeSharedRateLimit(ctx, "token:x", start, window, 10); err == nil {
		t.Fatal("表不存在时应返回错误")
	}
}
