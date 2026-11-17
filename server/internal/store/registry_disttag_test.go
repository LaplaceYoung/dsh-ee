package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// seedDistTagPackage 建一个托管包 @acme/tag-race，latest=1.0.0，已有 1.0.0 版本。
func seedDistTagPackage(t *testing.T, st *Store) string {
	t.Helper()
	ctx := context.Background()
	const name = "@acme/tag-race"
	owner := int64(7)
	_, err := st.PublishHosted(ctx, &NpmPackage{Name: name, Origin: OriginHosted, OwnerID: &owner},
		&NpmVersion{Name: name, Version: "1.0.0", Manifest: json.RawMessage(`{}`), Status: StatusPublished},
		map[string]string{"latest": "1.0.0"}, CatalogPatch{})
	if err != nil {
		t.Fatalf("建托管包：%v", err)
	}
	return name
}

// 读改写语义：回调看到的是锁内读到的最新值，写完能读回；回调返错不落库。
func TestDistTagUpdatePackageTags(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	name := seedDistTagPackage(t, st)

	tags, err := st.UpdatePackageDistTags(ctx, name, func(tags map[string]string) (map[string]string, error) {
		if tags["latest"] != "1.0.0" {
			t.Errorf("回调应看到库里的当前值，实际 %v", tags)
		}
		tags["beta"] = "1.0.0"
		return tags, nil
	})
	if err != nil {
		t.Fatalf("设置 beta：%v", err)
	}
	if tags["latest"] != "1.0.0" || tags["beta"] != "1.0.0" {
		t.Fatalf("返回值不对：%v", tags)
	}

	// 第二次读改写必须看到上一次已经提交的 beta（读改写基于最新值，不是进程内缓存）
	tags, err = st.UpdatePackageDistTags(ctx, name, func(tags map[string]string) (map[string]string, error) {
		if tags["beta"] != "1.0.0" {
			t.Errorf("第二次回调应看到上一次写入的 beta，实际 %v", tags)
		}
		delete(tags, "beta")
		tags["stable"] = "1.0.0"
		return tags, nil
	})
	if err != nil {
		t.Fatalf("删除 beta：%v", err)
	}
	if tags["beta"] != "" || tags["stable"] != "1.0.0" || tags["latest"] != "1.0.0" {
		t.Fatalf("删除后标签不对：%v", tags)
	}
	pkg, err := st.GetPackage(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DistTags["beta"] != "" || pkg.DistTags["stable"] != "1.0.0" {
		t.Fatalf("库里标签不对：%v", pkg.DistTags)
	}

	// 回调返错：不写库，错误原样返回
	sentinel := errors.New("校验失败")
	if _, err := st.UpdatePackageDistTags(ctx, name, func(tags map[string]string) (map[string]string, error) {
		tags["latest"] = "9.9.9"
		return nil, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("回调错误应原样返回，实际 %v", err)
	}
	pkg, err = st.GetPackage(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DistTags["latest"] != "1.0.0" {
		t.Fatalf("回调返错不应落库：%v", pkg.DistTags)
	}

	// 包不存在
	if _, err := st.UpdatePackageDistTags(ctx, "@acme/nope", func(tags map[string]string) (map[string]string, error) {
		return tags, nil
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("包不存在应返回 ErrNotFound，实际 %v", err)
	}
}

// N 个「设置不同标签」的读改写 + 一个发布新版本并发：行锁串行化后，每个标签都要在，
// 发布的 latest=2.0.0 也不能被任何一个基于旧值的整表写回覆盖。
func TestDistTagConcurrentUpdatesKeepAll(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	name := seedDistTagPackage(t, st)

	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n+1)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := st.UpdatePackageDistTags(ctx, name, func(tags map[string]string) (map[string]string, error) {
				tags[fmt.Sprintf("tag-%d", i)] = "1.0.0"
				return tags, nil
			})
			errs <- err
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := st.PublishHosted(ctx, &NpmPackage{Name: name, Origin: OriginHosted},
			&NpmVersion{Name: name, Version: "2.0.0", Manifest: json.RawMessage(`{}`), Status: StatusPublished},
			map[string]string{"latest": "2.0.0"}, CatalogPatch{})
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发写 dist-tags 出错：%v", err)
		}
	}

	pkg, err := st.GetPackage(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DistTags["latest"] != "2.0.0" {
		t.Fatalf("发布写入的 latest=2.0.0 被覆盖：%v", pkg.DistTags)
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("tag-%d", i)
		if pkg.DistTags[key] != "1.0.0" {
			t.Fatalf("并发写的 %s 丢了：%v", key, pkg.DistTags)
		}
	}
}
