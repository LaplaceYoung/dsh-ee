package registry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// wantErrStatus 断言错误是带指定状态码的 registry.Error。
func wantErrStatus(t *testing.T, err error, status int) {
	t.Helper()
	var re *Error
	if !errors.As(err, &re) {
		t.Fatalf("应为 registry.Error，得到 %v", err)
	}
	if re.Status != status {
		t.Fatalf("状态码应为 %d，得到 %d（%s）", status, re.Status, re.Msg)
	}
}

func TestSyncGateGlobalLimit(t *testing.T) {
	g := newSyncGate()
	rel1, err := g.reserve(1, []SyncRoot{{Name: "a"}}, 2, 10)
	if err != nil {
		t.Fatalf("第一个任务应成功：%v", err)
	}
	rel2, err := g.reserve(2, []SyncRoot{{Name: "b"}}, 2, 10)
	if err != nil {
		t.Fatalf("第二个任务应成功：%v", err)
	}
	if _, err := g.reserve(3, []SyncRoot{{Name: "c"}}, 2, 10); err == nil {
		t.Fatal("超过全局上限应拒绝")
	} else {
		wantErrStatus(t, err, 429)
	}
	rel1()
	rel3, err := g.reserve(3, []SyncRoot{{Name: "c"}}, 2, 10)
	if err != nil {
		t.Fatalf("释放名额后应能新建任务：%v", err)
	}
	// release 幂等：重复调用不会把计数减成负数。
	rel2()
	rel2()
	rel3()
	if _, err := g.reserve(1, []SyncRoot{{Name: "d"}}, 2, 10); err != nil {
		t.Fatalf("全部释放后应能再建任务：%v", err)
	}
}

func TestSyncGatePerUserLimit(t *testing.T) {
	g := newSyncGate()
	rel1, err := g.reserve(7, []SyncRoot{{Name: "a"}}, 10, 1)
	if err != nil {
		t.Fatalf("第一个任务应成功：%v", err)
	}
	if _, err := g.reserve(7, []SyncRoot{{Name: "b"}}, 10, 1); err == nil {
		t.Fatal("同一用户超过每用户上限应拒绝")
	} else {
		wantErrStatus(t, err, 429)
	}
	// 系统任务（by=0）不计每用户额度。
	relSys, err := g.reserve(0, []SyncRoot{{Name: "c"}}, 10, 1)
	if err != nil {
		t.Fatalf("系统任务应成功：%v", err)
	}
	// 其他用户不受这个用户影响。
	rel2, err := g.reserve(8, []SyncRoot{{Name: "d"}}, 10, 1)
	if err != nil {
		t.Fatalf("另一个用户的任务应成功：%v", err)
	}
	rel1()
	relSys()
	rel2()
}

func TestSyncGateRootConflicts(t *testing.T) {
	g := newSyncGate()
	rel, err := g.reserve(1, []SyncRoot{{Name: "pkg-a"}}, 10, 10)
	if err != nil {
		t.Fatalf("第一个任务应成功：%v", err)
	}
	// 同名的 latest 在同步中，指定版本也冲突。
	if _, err := g.reserve(1, []SyncRoot{{Name: "pkg-a", Version: "1.0.0"}}, 10, 10); err == nil {
		t.Fatal("同名 latest 同步中时指定版本应冲突")
	} else {
		wantErrStatus(t, err, 409)
	}
	// 不同包（含企业作用域）可以并行。
	relB, err := g.reserve(1, []SyncRoot{{Name: "@company/pkg-b", Version: "1.0.0"}}, 10, 10)
	if err != nil {
		t.Fatalf("不同包应能同时同步：%v", err)
	}
	// 同名同版本重复请求冲突。
	if _, err := g.reserve(2, []SyncRoot{{Name: "@company/pkg-b", Version: "1.0.0"}}, 10, 10); err == nil {
		t.Fatal("同名同版本重复同步应被拒绝")
	} else {
		wantErrStatus(t, err, 409)
	}
	// 同名不同版本可以并行。
	relB2, err := g.reserve(2, []SyncRoot{{Name: "@company/pkg-b", Version: "2.0.0"}}, 10, 10)
	if err != nil {
		t.Fatalf("同名不同版本应能同时同步：%v", err)
	}
	// 指定版本在同步中时，latest 请求也冲突。
	if _, err := g.reserve(3, []SyncRoot{{Name: "@company/pkg-b"}}, 10, 10); err == nil {
		t.Fatal("指定版本同步中时 latest 应冲突")
	} else {
		wantErrStatus(t, err, 409)
	}
	rel()
	relB()
	relB2()
	relAfter, err := g.reserve(1, []SyncRoot{{Name: "pkg-a"}}, 10, 10)
	if err != nil {
		t.Fatalf("任务结束后同名同步应恢复：%v", err)
	}
	relAfter()
}

func TestDedupeSyncRoots(t *testing.T) {
	got := dedupeSyncRoots([]SyncRoot{
		{Name: "a"},
		{Name: "b", Version: "1.0.0"},
		{Name: "a"},
		{Name: "b", Version: "1.0.0"},
		{Name: "b", Version: "2.0.0"},
	})
	if len(got) != 3 {
		t.Fatalf("去重后应剩 3 个根包，得到 %d：%+v", len(got), got)
	}
	if got[0].Name != "a" || got[1].Name != "b" || got[1].Version != "1.0.0" || got[2].Version != "2.0.0" {
		t.Fatalf("去重结果顺序或内容不对：%+v", got)
	}
}

// ---- 数据库集成测试（CI 用 TiDB；本机没 DSH_EE_TEST_DSN 时跳过） ----

// openRegistryTestStore 建一个一次性库跑集成测试，结束后删除。
func openRegistryTestStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	dbName := "dsh_ee_registry_test_" + hex.EncodeToString(suffix)
	dsn := strings.Replace(base, ")/", ")/"+dbName, 1)
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.DB().ExecContext(context.Background(), "DROP DATABASE `"+dbName+"`")
		st.Close()
	})
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

// rewriteTransport 把发往真实上游的请求转到测试用的 httptest 服务。
type rewriteTransport struct{ target *url.URL }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme, u.Host = t.target.Scheme, t.target.Host
	clone := req.Clone(req.Context())
	clone.URL = &u
	clone.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// waitSyncGateIdle 等所有后台任务结束、名额释放。
func waitSyncGateIdle(t *testing.T, svc *Service, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		svc.gate.mu.Lock()
		running := svc.gate.running
		svc.gate.mu.Unlock()
		if running == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("同步任务没有在 %s 内结束（还剩 %d 个）", timeout, running)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartSyncAdmission(t *testing.T) {
	st := openRegistryTestStore(t)
	releaseUpstream := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseUpstream) }) }
	t.Cleanup(unblock)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 占住所有包元数据请求，让同步任务保持 running，直到测试放行。
		select {
		case <-releaseUpstream:
			http.Error(w, "upstream released", http.StatusBadGateway)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(upstream.Close)

	blobs, err := NewBlobs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	up, err := NewUpstream(&http.Client{Transport: rewriteTransport{target: target}}, t.TempDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, blobs, up, st.GetSettings, slog.New(slog.NewTextHandler(io.Discard, nil)), "test-instance")
	svc.MaxSyncJobs = 2
	svc.MaxSyncJobsPerUser = 1
	ctx := context.Background()

	// 同一次请求里重复的根包只同步一次。
	j, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-a"}, {Name: "pkg-a"}}}, 1)
	if err != nil {
		t.Fatalf("第一次同步失败：%v", err)
	}
	if j.Total != 1 {
		t.Fatalf("重复根包应去重成 1 个，得到 %d", j.Total)
	}

	// 同一个包正在同步时再次请求 → 409（latest 请求与指定版本请求都算）。
	if _, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-a", Version: "1.0.0"}}}, 2); err == nil {
		t.Fatal("重复同步应被拒绝")
	} else {
		wantErrStatus(t, err, 409)
	}
	if _, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-a"}}}, 2); err == nil {
		t.Fatal("重复同步应被拒绝")
	} else {
		wantErrStatus(t, err, 409)
	}

	// 每用户上限 1：同一个用户第二个任务 429。
	if _, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-b"}}}, 1); err == nil {
		t.Fatal("超过每用户任务上限应被拒绝")
	} else {
		wantErrStatus(t, err, 429)
	}

	// 另一个用户建第二个任务成功；全局上限 2 满了之后 429。
	if _, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-b"}}}, 2); err != nil {
		t.Fatalf("第二个任务应成功：%v", err)
	}
	if _, err := svc.StartSync(ctx, "preload", SyncParams{Roots: []SyncRoot{{Name: "pkg-c"}}}, 4); err == nil {
		t.Fatal("超过全局任务上限应被拒绝")
	} else {
		wantErrStatus(t, err, 429)
	}

	// 名额占满时离线导入也被拒。
	if _, _, err := svc.ImportBundle(ctx, strings.NewReader(""), 3); err == nil {
		t.Fatal("名额占满时导入应被拒绝")
	} else {
		wantErrStatus(t, err, 429)
	}

	// 放行上游，等任务结束、名额释放。
	unblock()
	waitSyncGateIdle(t, svc, 10*time.Second)

	// 名额释放后导入能走到解析离线包（空请求体 → 400，而不是 429）。
	if _, _, err := svc.ImportBundle(ctx, strings.NewReader(""), 3); err == nil {
		t.Fatal("空请求体应报离线包格式错误")
	} else {
		wantErrStatus(t, err, 400)
	}
}
