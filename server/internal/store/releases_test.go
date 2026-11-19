package store

import (
	"context"
	"errors"
	"testing"
)

func TestDshReleaseStore(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	first := &DshRelease{Target: "mac-arm64", Kind: "zip", Version: "1.0.0", ReleasedAt: now(), File: "dsh-1.zip", Size: 100, SHA512: "aGFzaA==", UpstreamURL: "https://download.deepseek.com/dsh-1.zip", State: ReleaseAvailable}
	if err := st.CreateDshRelease(ctx, first); err != nil {
		t.Fatal(err)
	}
	duplicate := *first
	duplicate.ID = 0
	if err := st.CreateDshRelease(ctx, &duplicate); err == nil {
		t.Fatal("相同 target、kind、version 应触发唯一键冲突")
	}
	second := &DshRelease{Target: first.Target, Kind: first.Kind, Version: "1.0.1", ReleasedAt: now(), File: "dsh-2.zip", Size: 200, SHA512: "aGFzaA==", UpstreamURL: "https://download.deepseek.com/dsh-2.zip", State: ReleaseMirrored, LocalPath: "/tmp/dsh-2.zip"}
	if err := st.CreateDshRelease(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDshReleaseRecommended(ctx, first.ID); !errors.Is(err, ErrReleaseNotMirrored) {
		t.Fatalf("未下载版本不能推荐：%v", err)
	}
	if err := st.UpdateDshReleaseState(ctx, first.ID, ReleaseMirrored, "/tmp/dsh-1.zip", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDshReleaseRecommended(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDshReleaseRecommended(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListDshReleases(ctx, first.Target)
	if err != nil || len(items) != 2 {
		t.Fatalf("版本列表：%d %v", len(items), err)
	}
	if !items[0].Recommended || items[0].ID != second.ID {
		t.Fatalf("推荐版本应排在最前：%+v", items[0])
	}
	old, err := st.DshReleaseByID(ctx, first.ID)
	if err != nil || old.Recommended {
		t.Fatalf("设置新推荐时应清除旧推荐：%+v %v", old, err)
	}
	if err := st.ResetDshRelease(ctx, second.ID); !errors.Is(err, ErrReleaseRecommended) {
		t.Fatalf("推荐版本不能删除：%v", err)
	}
	if err := st.ResetDshRelease(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	old, err = st.DshReleaseByID(ctx, first.ID)
	if err != nil || old.State != ReleaseRemoved || old.LocalPath != "" || old.MirroredAt != nil {
		t.Fatalf("删除本地文件后应变为 removed：%+v %v", old, err)
	}
}

// Windows 的安装包是 exe（kind = exe），推荐新版本时同样要清掉同种类的旧推荐：
// 否则两个版本都标着推荐，旧的删不掉（推荐版本不能删），也永远不会被清理。
func TestDshReleaseRecommendClearsOtherKinds(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	mk := func(kind, version, file string) *DshRelease {
		r := &DshRelease{Target: "win-x64", Kind: kind, Version: version, ReleasedAt: now(), File: file, Size: 1, SHA512: "aGFzaA==",
			UpstreamURL: "https://download.deepseek.com/" + file, State: ReleaseMirrored, LocalPath: "/tmp/" + file}
		if err := st.CreateDshRelease(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	v1, v2 := mk("exe", "1.0.0", "dsh-1.exe"), mk("exe", "1.1.0", "dsh-2.exe")
	for _, id := range []int64{v1.ID, v2.ID} {
		if err := st.SetDshReleaseRecommended(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	items, err := st.ListDshReleases(ctx, "win-x64")
	if err != nil {
		t.Fatal(err)
	}
	var recommended []string
	for _, it := range items {
		if it.Recommended {
			recommended = append(recommended, it.Version)
		}
	}
	if len(recommended) != 1 || recommended[0] != "1.1.0" {
		t.Fatalf("同一平台只能有一个推荐版本，应是 1.1.0：%v", recommended)
	}
	if err := st.ResetDshRelease(ctx, v1.ID); err != nil {
		t.Fatalf("不再推荐的旧版本应能删除：%v", err)
	}
	// 种类之间互不影响：推荐 mac 的 dmg 不会清掉 zip 的推荐
	zip := &DshRelease{Target: "mac-arm64", Kind: "zip", Version: "1.1.0", ReleasedAt: now(), File: "dsh.zip", Size: 1, SHA512: "aGFzaA==", UpstreamURL: "https://download.deepseek.com/dsh.zip", State: ReleaseMirrored, LocalPath: "/tmp/dsh.zip"}
	dmg := &DshRelease{Target: "mac-arm64", Kind: "dmg", Version: "1.1.0", ReleasedAt: now(), File: "dsh.dmg", Size: 1, SHA512: "aGFzaA==", UpstreamURL: "https://download.deepseek.com/dsh.dmg", State: ReleaseMirrored, LocalPath: "/tmp/dsh.dmg"}
	for _, r := range []*DshRelease{zip, dmg} {
		if err := st.CreateDshRelease(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := st.SetDshReleaseRecommended(ctx, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := st.DshReleaseByID(ctx, zip.ID); err != nil || !got.Recommended {
		t.Fatalf("推荐 dmg 不应清掉 zip 的推荐：%+v %v", got, err)
	}
}
