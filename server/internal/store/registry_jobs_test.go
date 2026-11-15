package store

import (
	"context"
	"testing"
)

// 多实例下启动时只清理本实例遗留的 running 任务（sync_jobs.owner = 本实例地址），
// 不能误伤其它在线实例正在跑的任务；升级前旧版本创建的任务没有归属（owner 为空串），
// 无法判断属于哪个实例，按原语义一并标中断。
func TestFailRunningJobsByOwner(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()

	create := func(owner string) *SyncJob {
		t.Helper()
		j := &SyncJob{Kind: "mirror", Params: []byte(`{}`), Total: 1, Owner: owner}
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatalf("创建任务（owner=%q）：%v", owner, err)
		}
		return j
	}
	current := create("inst-a")
	other := create("inst-b")
	legacy := create("")

	// owner 要经 jobCols/scanJob 原样读回，列表接口也一样。
	jobs, err := st.ListJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ListJobs：%v", err)
	}
	owners := map[int64]string{}
	for _, j := range jobs {
		owners[j.ID] = j.Owner
	}
	for id, want := range map[int64]string{
		current.ID: "inst-a",
		other.ID:   "inst-b",
		legacy.ID:  "",
	} {
		if got := owners[id]; got != want {
			t.Fatalf("任务 %d 的 owner = %q，期望 %q", id, got, want)
		}
	}

	if err := st.FailRunningJobs(ctx, "inst-a"); err != nil {
		t.Fatalf("FailRunningJobs(inst-a)：%v", err)
	}

	for _, tc := range []struct {
		name string
		id   int64
		want string
	}{
		{"本实例遗留的任务", current.ID, "interrupted"},
		{"其它在线实例的任务", other.ID, "running"},
		{"升级前无归属的任务", legacy.ID, "interrupted"},
	} {
		j, err := st.GetJob(ctx, tc.id)
		if err != nil {
			t.Fatalf("%s：查任务 %d：%v", tc.name, tc.id, err)
		}
		if j.Status != tc.want {
			t.Fatalf("%s：status = %q，期望 %q", tc.name, j.Status, tc.want)
		}
		if j.Status != "running" && (j.FinishedAt == nil || j.FinishedAt.IsZero()) {
			t.Fatalf("%s：标中断后应记 finished_at", tc.name)
		}
	}
}
