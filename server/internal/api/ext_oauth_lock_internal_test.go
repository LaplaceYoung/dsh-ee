package api

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// oauthLocks 用固定分片代替「每个 (员工, 连接) 一个永不过期 mutex」的 map。
// 这里守住三件事：key→分片映射稳定、分片数有上界、同一 key 的刷新仍然串行。

// TestOAuthLockShards 验证同一个 (userID, assetID) 永远拿到同一把锁，
// 以及 64 个分片确实都被均匀用到（不是退化成一个全局大锁）。
func TestOAuthLockShards(t *testing.T) {
	keys := []connKey{
		{asset: 1, user: 1},
		{asset: 1, user: 2},
		{asset: 2, user: 1},
		{asset: 42, user: 7},
		{asset: 9999, user: 10000},
	}
	for _, k := range keys {
		want := oauthLock(k.user, k.asset)
		if want != &oauthLocks[oauthLockShardIndex(k.user, k.asset)] {
			t.Fatalf("(%d,%d) 没有落在 hash 算出的分片上", k.user, k.asset)
		}
		for i := 0; i < 100; i++ {
			if got := oauthLock(k.user, k.asset); got != want {
				t.Fatalf("(%d,%d) 两次拿到不同的锁，hash 不稳定", k.user, k.asset)
			}
		}
	}

	// 分片数在编译期固定：无论见过多少个 key，mutex 总数都不会超过 64。
	if len(oauthLocks) != oauthLockShardCount {
		t.Fatalf("分片数应为 %d，实际 %d", oauthLockShardCount, len(oauthLocks))
	}

	// 64×64 个小 id（贴近自增主键）应铺满全部分片。
	seen := map[uint64]struct{}{}
	for user := int64(1); user <= 64; user++ {
		for asset := int64(1); asset <= 64; asset++ {
			seen[oauthLockShardIndex(user, asset)] = struct{}{}
		}
	}
	if len(seen) != oauthLockShardCount {
		t.Fatalf("4096 个 key 只用到 %d/%d 个分片，hash 分布太差", len(seen), oauthLockShardCount)
	}

	// 不同 key 撞到同一分片是允许的（只多等一会儿），此时确实共用一把锁。
	var collided bool
	for user := int64(1); user <= 64 && !collided; user++ {
		for asset := int64(1); asset <= 64; asset++ {
			if user == 7 && asset == 11 {
				continue
			}
			if oauthLock(user, asset) == oauthLock(7, 11) {
				collided = true
				break
			}
		}
	}
	if !collided {
		t.Fatal("64 个分片扫描 4096 个 key 应能碰到一次分片冲突")
	}
}

// TestOAuthLockSerialPerKey 验证同一 (user, asset) 的临界区最多只有一个 goroutine：
// 这正是「很多系统 refresh token 只能用一次」所依赖的排队语义。
func TestOAuthLockSerialPerKey(t *testing.T) {
	const goroutines, rounds = 8, 25
	var inCritical, maxInCritical atomic.Int32
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				mu := oauthLock(7, 11)
				mu.Lock()
				n := inCritical.Add(1)
				for {
					old := maxInCritical.Load()
					if n <= old || maxInCritical.CompareAndSwap(old, n) {
						break
					}
				}
				// 持锁一小段时间，保证并发进入时一定能被观察到。
				time.Sleep(200 * time.Microsecond)
				inCritical.Add(-1)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if got := maxInCritical.Load(); got != 1 {
		t.Fatalf("同一 (user, asset) 的临界区最多应有 1 个 goroutine，实际 %d", got)
	}
}

// TestOAuthLockDifferentShardsParallel 验证落在他分片上的两个 key 仍可并行，
// 即分片没有退化为一把全局锁。
func TestOAuthLockDifferentShardsParallel(t *testing.T) {
	var a, b connKey
	found := false
	for user := int64(1); user <= 64 && !found; user++ {
		for asset := int64(1); asset <= 64; asset++ {
			if oauthLockShardIndex(user, asset) != oauthLockShardIndex(1, 1) {
				a, b, found = connKey{asset: 1, user: 1}, connKey{asset: asset, user: user}, true
				break
			}
		}
	}
	if !found {
		t.Fatal("找不到落在不同分片的两个 key")
	}
	muA, muB := oauthLock(a.user, a.asset), oauthLock(b.user, b.asset)
	if muA == muB {
		t.Fatalf("(%d,%d) 与 (%d,%d) 分片索引不同却拿到同一把锁", a.user, a.asset, b.user, b.asset)
	}

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	for _, mu := range []*sync.Mutex{muA, muB} {
		go func(mu *sync.Mutex) {
			mu.Lock()
			defer mu.Unlock()
			entered <- struct{}{}
			<-release
		}(mu)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("两个不同分片的锁应能同时持有，第 %d 个 goroutine 一直没进来", i+1)
		}
	}
}
