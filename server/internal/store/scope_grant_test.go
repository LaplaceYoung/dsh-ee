package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// 并发批准同一个能力：每个事务都要锁行读改写，谁的授权都不能被后写者覆盖。
func TestGrantAssetScopeUserConcurrent(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	a := &Asset{Kind: KindSkill, Slug: "concurrent-skill", Title: "并发技能", Scope: Scope{Departments: []string{"研发部"}}, Delivery: DeliveryOptional}
	if err := st.CreateAsset(ctx, a, json.RawMessage(`{"title":"并发技能"}`), StatusPublished, 0); err != nil {
		t.Fatal(err)
	}

	const n = 12
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = int64(1000 + i)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, uid := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, err := st.GrantAssetScopeUser(ctx, a.ID, uid)
			if err != nil {
				errs <- fmt.Errorf("追加用户 %d: %w", uid, err)
				return
			}
			if !added {
				errs <- fmt.Errorf("追加用户 %d 应返回 added=true", uid)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	got, err := st.GetAsset(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Scope.Users) != n {
		t.Fatalf("并发追加后应有 %d 个用户，实际 %v（有授权被覆盖丢失）", n, got.Scope.Users)
	}
	for _, uid := range ids {
		if !slices.Contains(got.Scope.Users, uid) {
			t.Fatalf("用户 %d 的授权被并发覆盖丢了：%v", uid, got.Scope.Users)
		}
	}
	if !slices.Equal(got.Scope.Departments, []string{"研发部"}) {
		t.Fatalf("并发追加不应改动原有的部门范围：%+v", got.Scope)
	}

	// 幂等：已经包含时不再写库，返回 added=false
	if added, err := st.GrantAssetScopeUser(ctx, a.ID, ids[0]); err != nil || added {
		t.Fatalf("重复追加应返回 added=false / nil：added=%v err=%v", added, err)
	}
	// 全员可见时也不需要逐个追加
	all := &Asset{Kind: KindSkill, Slug: "all-skill", Title: "全员技能", Scope: Scope{All: true}, Delivery: DeliveryAuto}
	if err := st.CreateAsset(ctx, all, json.RawMessage(`{"title":"全员技能"}`), StatusPublished, 0); err != nil {
		t.Fatal(err)
	}
	if added, err := st.GrantAssetScopeUser(ctx, all.ID, 42); err != nil || added {
		t.Fatalf("范围是所有人时应返回 added=false / nil：added=%v err=%v", added, err)
	}
	// 非法用户 id 不会写进范围，也不返回「加上了」
	if added, err := st.GrantAssetScopeUser(ctx, a.ID, 0); err != nil || added {
		t.Fatalf("非法用户 id 应返回 added=false / nil：added=%v err=%v", added, err)
	}
	// 对象不存在 → ErrNotFound（审批层据此回 404）
	if _, err := st.GrantAssetScopeUser(ctx, 1<<40, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的资产应返回 ErrNotFound：%v", err)
	}
}

// 知识库同理：并发加人不能互相覆盖，重复/全员/不存在的行为与资产一致。
func TestGrantKBaseScopeUserConcurrent(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	b := &KBase{Slug: "concurrent-kb", Title: "并发知识库", Scope: Scope{Roles: []int64{7}}}
	if err := st.CreateKBase(ctx, b, 0); err != nil {
		t.Fatal(err)
	}

	const n = 12
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = int64(2000 + i)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, uid := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, err := st.GrantKBaseScopeUser(ctx, b.ID, uid)
			if err != nil {
				errs <- fmt.Errorf("追加用户 %d: %w", uid, err)
				return
			}
			if !added {
				errs <- fmt.Errorf("追加用户 %d 应返回 added=true", uid)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	got, err := st.GetKBase(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Scope.Users) != n {
		t.Fatalf("并发追加后应有 %d 个用户，实际 %v（有授权被覆盖丢失）", n, got.Scope.Users)
	}
	for _, uid := range ids {
		if !slices.Contains(got.Scope.Users, uid) {
			t.Fatalf("用户 %d 的授权被并发覆盖丢了：%v", uid, got.Scope.Users)
		}
	}
	if !slices.Equal(got.Scope.Roles, []int64{7}) {
		t.Fatalf("并发追加不应改动原有的角色范围：%+v", got.Scope)
	}

	if added, err := st.GrantKBaseScopeUser(ctx, b.ID, ids[0]); err != nil || added {
		t.Fatalf("重复追加应返回 added=false / nil：added=%v err=%v", added, err)
	}
	all := &KBase{Slug: "all-kb", Title: "全员知识库", Scope: Scope{All: true}}
	if err := st.CreateKBase(ctx, all, 0); err != nil {
		t.Fatal(err)
	}
	if added, err := st.GrantKBaseScopeUser(ctx, all.ID, 42); err != nil || added {
		t.Fatalf("范围是所有人时应返回 added=false / nil：added=%v err=%v", added, err)
	}
	if _, err := st.GrantKBaseScopeUser(ctx, 1<<40, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的知识库应返回 ErrNotFound：%v", err)
	}
}
