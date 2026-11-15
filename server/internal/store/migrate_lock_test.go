package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// openFreshTestStores 在同一个全新的随机库上开 n 个 Store：只建库，不执行迁移，
// 迁移由测试自己控制（要验证多个实例并发迁移）。DSH_EE_TEST_DSN 为空时跳过；
// 测试结束 DROP DATABASE 并关闭所有连接。
func openFreshTestStores(t *testing.T, n int) []*Store {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("生成随机库名: %v", err)
	}
	name := "dsh_ee_test_lock_" + hex.EncodeToString(suffix)
	dsn := strings.Replace(base, ")/", ")/"+name, 1)

	stores := make([]*Store, 0, n)
	for i := 0; i < n; i++ {
		st, err := Open(context.Background(), dsn)
		if err != nil {
			for _, prev := range stores {
				prev.Close()
			}
			t.Fatalf("打开测试库 %s: %v", name, err)
		}
		stores = append(stores, st)
	}
	t.Cleanup(func() {
		// DROP 用第一个连接执行（连接着的库也可以删），再关闭所有连接。
		if len(stores) > 0 {
			_, _ = stores[0].DB().ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		}
		for _, st := range stores {
			st.Close()
		}
	})
	return stores
}

func assertMigrateLockEmpty(t *testing.T, st *Store) {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM migration_lock").Scan(&n); err != nil {
		t.Fatalf("查 migration_lock: %v", err)
	}
	if n != 0 {
		t.Fatalf("迁移结束后 migration_lock 应为空，实际 %d 行", n)
	}
}

// 两个实例同时首次启动：只有一个执行 DDL，另一个等待；两个 Migrate 都返回 nil，
// 迁移版本每条只记一次，锁释放干净；之后顺序重跑也不重复执行。
func TestMigrateLockSerializes(t *testing.T) {
	stores := openFreshTestStores(t, 2)
	ctx := context.Background()

	start := make(chan struct{})
	errs := make([]error, len(stores))
	var wg sync.WaitGroup
	for i, st := range stores {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			<-start
			errs[i] = st.Migrate(ctx)
		}(i, st)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("实例 %d 并发 Migrate 失败: %v", i, err)
		}
	}

	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	var applied int
	if err := stores[0].DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("查 schema_migrations: %v", err)
	}
	if applied != len(files) {
		t.Fatalf("schema_migrations 应有 %d 行（内置迁移文件数），实际 %d 行", len(files), applied)
	}
	assertMigrateLockEmpty(t, stores[0])

	// 顺序再跑两遍全量迁移：已应用的版本不重复执行，行数不变，锁每次都释放。
	for round := 0; round < 2; round++ {
		for i, st := range stores {
			if err := st.Migrate(ctx); err != nil {
				t.Fatalf("第 %d 遍顺序 Migrate（实例 %d）失败: %v", round+2, i, err)
			}
		}
	}
	var appliedAgain int
	if err := stores[1].DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&appliedAgain); err != nil {
		t.Fatalf("查 schema_migrations: %v", err)
	}
	if appliedAgain != len(files) {
		t.Fatalf("重复 Migrate 后 schema_migrations 仍应是 %d 行，实际 %d 行", len(files), appliedAgain)
	}
	assertMigrateLockEmpty(t, stores[1])

	// 释放函数必须幂等：连调两次不报错，锁行为空。
	release, err := stores[0].acquireMigrateLock(ctx)
	if err != nil {
		t.Fatalf("抢迁移锁: %v", err)
	}
	release()
	release()
	assertMigrateLockEmpty(t, stores[0])
}

// 持有实例崩溃（锁行 acquired_at 超过陈旧阈值）：新实例接管陈旧锁并完成迁移。
func TestMigrateLockStealsStale(t *testing.T) {
	st := openFreshTestStores(t, 1)[0]
	ctx := context.Background()

	// 先走一次真实的 acquire/release，把 migration_lock 表建出来。
	release, err := st.acquireMigrateLock(ctx)
	if err != nil {
		t.Fatalf("抢迁移锁: %v", err)
	}
	release()

	// 模拟 10 分钟前崩溃的实例留下的锁行（超过默认 5 分钟陈旧阈值）。
	deadAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO migration_lock (id, owner, acquired_at) VALUES (1, 'dead-instance', ?)", deadAt); err != nil {
		t.Fatalf("写入陈旧锁行: %v", err)
	}

	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("陈旧锁应能被接管并完成迁移，Migrate 返回: %v", err)
	}
	assertMigrateLockEmpty(t, st)
}

// 另一个活着的实例正在迁移：等待超过 migrateLockWait 后报错，错误信息能指导运维。
func TestMigrateLockWaitTimeout(t *testing.T) {
	st := openFreshTestStores(t, 1)[0]
	ctx := context.Background()

	release, err := st.acquireMigrateLock(ctx)
	if err != nil {
		t.Fatalf("抢迁移锁: %v", err)
	}
	release()

	// 模拟另一个实例正在迁移：锁行刚写入，未超过陈旧阈值，不能被接管。
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO migration_lock (id, owner, acquired_at) VALUES (1, 'live-instance', ?)", now()); err != nil {
		t.Fatalf("写入测试锁行: %v", err)
	}

	oldWait := migrateLockWait
	migrateLockWait = 300 * time.Millisecond
	defer func() {
		migrateLockWait = oldWait
		// 清理锁行，避免影响同库后续断言（库本身在 t.Cleanup 里 DROP）。
		_, _ = st.DB().ExecContext(context.Background(), "DELETE FROM migration_lock WHERE id = 1")
	}()

	err = st.Migrate(ctx)
	if err == nil {
		t.Fatal("锁被另一个实例占着时 Migrate 应超时失败")
	}
	if !strings.Contains(err.Error(), "迁移锁超时") {
		t.Fatalf("超时错误应包含「迁移锁超时」，实际: %v", err)
	}
	// 错误信息要能指导运维：谁在占用、怎么办。
	for _, want := range []string{"另一个实例正在执行迁移", "陈旧锁可被接管"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("超时错误应包含 %q，实际: %v", want, err)
		}
	}
}
