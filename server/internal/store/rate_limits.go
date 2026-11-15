package store

import (
	"context"
	"time"
)

// ---- 公开协议面的共享固定窗口限流（shared_rate_limits）----
//
// /v1/* 与 /mcp 的每令牌限流从进程内 map 改到数据库：N 个实例连同一个库时共用同一份额度，
// 不会因为多实例部署而成倍放水。窗口按 window 对齐（线上默认 1 分钟），依赖各实例时钟同步。
//
// 与 embed_rates 同一套模式：一条 upsert 原子建窗口/计数，再读回计数判断放行。
// 计数封顶 limit+1，超限后不再增长，攻击者没法把一行计数撑大。

// SharedRateLimit 是一次共享限流判定的结果，直接映射到 X-RateLimit-* 响应头。
type SharedRateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	Allowed   bool
}

// TakeSharedRateLimit 在一个固定窗口里消费一次额度，并返回窗口状态。
// windowStart 是窗口起点，由调用方按 window 对齐（多实例必须算出同一个起点），
// 这样跨实例计数才落在同一行；Reset = windowStart + window。
// limit <= 0 视为不限流（配置层关闭时不会调用到这里，这里只做兜底）。
func (s *Store) TakeSharedRateLimit(ctx context.Context, key string, windowStart time.Time, window time.Duration, limit int) (SharedRateLimit, error) {
	if window <= 0 {
		window = time.Minute
	}
	windowStart = windowStart.UTC().Truncate(time.Millisecond)
	reset := windowStart.Add(window)
	if limit <= 0 {
		return SharedRateLimit{Limit: 0, Remaining: 0, Reset: reset, Allowed: true}, nil
	}
	// 一条 upsert 同时完成「建窗口 / 计数 +1（封顶 limit+1）」：多实例并发时由行锁串行化，
	// 读回计数再判断是否放行。窗口结束前这一行一直存在。
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO shared_rate_limits (rate_key, window_start, count) VALUES (?, ?, 1)
		 ON DUPLICATE KEY UPDATE count = IF(count <= ?, count + 1, count)`,
		key, windowStart, limit); err != nil {
		return SharedRateLimit{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count FROM shared_rate_limits WHERE rate_key = ? AND window_start = ?`,
		key, windowStart).Scan(&count); err != nil {
		return SharedRateLimit{}, err
	}
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return SharedRateLimit{
		Limit:     limit,
		Remaining: remaining,
		Reset:     reset,
		Allowed:   count <= limit,
	}, nil
}

// DeleteOldSharedRateLimits 清理已经结束的限流窗口（只保留 windowStart 及以后的窗口）。
func (s *Store) DeleteOldSharedRateLimits(ctx context.Context, windowStart time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM shared_rate_limits WHERE window_start < ?", windowStart.UTC().Truncate(time.Millisecond))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
