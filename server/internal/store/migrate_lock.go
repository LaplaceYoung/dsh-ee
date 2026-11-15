package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 启动迁移互斥：多个实例同时首次启动时，只允许一个实例执行 DDL，其它实例等它做完。
//
// 锁是 migration_lock 表里固定 id=1 的单行：INSERT 成功即持有；并发 INSERT 的输家拿到
// MySQL 1062（主键冲突）后转去轮询。持有实例崩溃时锁行会留在表里，acquired_at 超过
// migrateLockStale 之后别的实例用带 WHERE 条件的 UPDATE 抢占（CAS，只有一个能赢）。
//
// 故意只用 INSERT/UPDATE/DELETE + 主键行锁：GET_LOCK/RELEASE_LOCK 这类命名锁是
// MySQL 专有、TiDB 不支持（版本间行为也不一致），而 store 包要在 OceanBase（MySQL 模式）
// 与 TiDB 上都只有一种行为。
var (
	// migrateLockWait 是等待另一个实例完成迁移的上限，超过就报错让调用方重启重试。
	migrateLockWait = 60 * time.Second
	// migrateLockStale 是锁行的陈旧阈值：持有实例崩溃后，acquired_at 早于
	// now-migrateLockStale 的锁行可以被别的实例接管。
	migrateLockStale = 5 * time.Minute
)

// migrateLockPoll 是等待期间的重试间隔。
const migrateLockPoll = 500 * time.Millisecond

const (
	// migrateLockID 固定为 1：整张表只有一行，记录当前执行迁移的实例。
	migrateLockID = 1
	// migrateLockOwnerMax 与 owner 列宽（VARCHAR(191)）一致。
	migrateLockOwnerMax = 191
	// migrateLockReleaseTimeout 是释放锁的超时：数据库不可用也不能把启动/停机拖住。
	migrateLockReleaseTimeout = 5 * time.Second

	migrateLockTableDDL = `CREATE TABLE IF NOT EXISTS migration_lock (
		id          TINYINT      NOT NULL,
		owner       VARCHAR(191) NOT NULL,
		acquired_at DATETIME(3)  NOT NULL,
		PRIMARY KEY (id)
	)`

	mysqlErrTableExists  = 1050
	mysqlErrDuplicateKey = 1062
)

// acquireMigrateLock 抢数据库启动迁移锁。拿到锁返回释放函数；没拿到（在 migrateLockWait
// 内都等不到）返回错误，调用方应让本次启动失败，运维重启后重试。
//
// 释放函数是幂等的：重复调用只是再执行一次删不到行的 DELETE，不返回错误。释放失败也
// 不阻塞（只忽略），因为锁行会在 migrateLockStale 之后变成陈旧锁，由别的实例接管，
// 不会永久卡死迁移。
func (s *Store) acquireMigrateLock(ctx context.Context) (release func(), err error) {
	if _, err := s.db.ExecContext(ctx, migrateLockTableDDL); err != nil {
		// 并发建表时后到的实例即使写了 IF NOT EXISTS 也可能收到 1050（表已存在），
		// 表已经在了就是成功。
		if !isMySQLErr(err, mysqlErrTableExists) {
			return nil, fmt.Errorf("创建 migration_lock: %w", err)
		}
	}

	owner := migrateLockOwner()
	deadline := time.Now().Add(migrateLockWait)
	for {
		// 先试 INSERT：主键 id=1 保证同一时刻只有一个实例能插进去。
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO migration_lock (id, owner, acquired_at) VALUES (?, ?, ?)",
			migrateLockID, owner, now())
		if err == nil {
			return s.migrateLockReleaser(owner), nil
		}
		// 上下文取消时返回 ctx.Err()，而不是包装成数据库错误。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if !isMySQLErr(err, mysqlErrDuplicateKey) {
			return nil, fmt.Errorf("抢数据库迁移锁: %w", err)
		}

		// 行被别的实例占着：如果它已经超过陈旧阈值（持有实例崩溃留下的），接管。
		stolen, stealErr := s.stealStaleMigrateLock(ctx, owner)
		if stealErr != nil {
			return nil, stealErr
		}
		if stolen {
			return s.migrateLockReleaser(owner), nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf(
				"等待数据库迁移锁超时：另一个实例正在执行迁移（最多等待 %s）；持有实例崩溃后最多 %s 后陈旧锁可被接管",
				formatLockDuration(migrateLockWait), formatLockDuration(migrateLockStale))
		}
		wait := migrateLockPoll
		if remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// stealStaleMigrateLock 尝试接管陈旧锁行：只有 acquired_at < now-migrateLockStale 时
// UPDATE 才会命中，并发的抢占者里只有一个能把 RowsAffected 变成 1。
func (s *Store) stealStaleMigrateLock(ctx context.Context, owner string) (bool, error) {
	current := now()
	cutoff := current.Add(-migrateLockStale)
	res, err := s.db.ExecContext(ctx,
		"UPDATE migration_lock SET owner = ?, acquired_at = ? WHERE id = ? AND acquired_at < ?",
		owner, current, migrateLockID, cutoff)
	if err != nil {
		return false, fmt.Errorf("接管陈旧迁移锁: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("接管陈旧迁移锁: %w", err)
	}
	return n == 1, nil
}

// migrateLockReleaser 返回释放函数：DELETE 带 owner 条件，重复调用删不到行也不报错。
func (s *Store) migrateLockReleaser(owner string) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), migrateLockReleaseTimeout)
		defer cancel()
		// 失败忽略：锁行会在 migrateLockStale 之后过期，由接管逻辑代替释放。
		_, _ = s.db.ExecContext(ctx,
			"DELETE FROM migration_lock WHERE id = ? AND owner = ?", migrateLockID, owner)
	}
}

// migrateLockOwner 生成本实例的锁持有者标识：主机名/PID/4 字节随机。
// 用本包内的 helper 而不是 internal/lease，避免 lease -> store -> lease 的循环依赖。
func migrateLockOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	b := make([]byte, 4)
	var suffix string
	if _, err := rand.Read(b); err == nil {
		suffix = hex.EncodeToString(b)
	} else {
		// 随机数不可用不该挡住迁移：用纳秒时间戳兜底，只用于区分同机实例。
		suffix = fmt.Sprintf("%x", time.Now().UnixNano())
	}
	owner := fmt.Sprintf("%s/%d/%s", host, os.Getpid(), suffix)
	if len(owner) > migrateLockOwnerMax {
		owner = owner[:migrateLockOwnerMax]
	}
	return owner
}

// isMySQLErr 判断 err 是不是指定的 MySQL 错误号（OceanBase MySQL 模式与 TiDB 共用这套错误号）。
func isMySQLErr(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

// formatLockDuration 把时长写成错误信息里易读的形式：60s、5 分钟、300ms。
// 60s 不写成 1 分钟，保持和默认配置（migrateLockWait=60s）一致的读法。
func formatLockDuration(d time.Duration) string {
	switch {
	case d >= 2*time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%d 分钟", int64(d/time.Minute))
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", int64(d/time.Second))
	default:
		return d.String()
	}
}
