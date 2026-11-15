package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// SyncRoot 是一个要预载的包（通常是插件），连同它的依赖闭包一起镜像。
type SyncRoot struct {
	Name string `json:"name"`
	// 为空时取上游的 latest
	Version string `json:"version,omitempty"`
	// 期望的内容摘要（npm 的 sha512 SRI，如 sha512-<base64>）。非空时先核对上游元数据里
	// 声明的 integrity，对不上就不入库：企业目录登记了摘要，同步时要按它核对。
	Integrity string `json:"integrity,omitempty"`
	// 来自 dshfind 目录的展示信息（可选）
	Catalog *FeedPlugin `json:"catalog,omitempty"`
}

type SyncParams struct {
	Roots []SyncRoot `json:"roots"`
	// 根包（插件）入库后的状态：pending 待审核 / published 直接上架
	RootStatus string `json:"root_status"`
	// 不是插件的根包也要审核（员工申请引入时）
	ForceReview bool `json:"force_review,omitempty"`
	// 申请理由（员工申请引入时）
	Reason string `json:"reason,omitempty"`
}

// 依赖闭包下载的并发数。
const mirrorWorkers = 6

// 同步任务（预载/申请引入/检查更新/离线导入）会并发拉上游、写 tarball、解包扫描。
// 没有上限时重复点击或脚本刷接口就能同时堆起任意多个任务，把上游带宽、磁盘 IO 和
// 数据库连接一起占满。这里做两件事：
//  1. 限制同时运行的任务数（全局 + 每用户）；
//  2. 同一个包的同一个版本同一时间只在一个任务里同步，重复请求直接拒绝。
const (
	// 全局同时运行的同步/导入任务上限。
	defaultMaxSyncJobs = 3
	// 每个用户同时运行的任务上限；系统任务（by=0）不计。
	defaultMaxSyncJobsPerUser = 2
)

// syncGate 是同步任务的准入控制，进程内生效（与 README 里「预载任务在单个进程里」一致）。
// 由 NewService 初始化，不要直接使用零值。
type syncGate struct {
	mu      sync.Mutex
	running int
	byUser  map[int64]int
	active  map[string]int
}

func newSyncGate() syncGate {
	return syncGate{byUser: map[int64]int{}, active: map[string]int{}}
}

// syncRootKey 是同步去重键：版本为空表示「上游 latest」，按 name@* 记。
func syncRootKey(r SyncRoot) string {
	if r.Version == "" {
		return r.Name + "@*"
	}
	return r.Name + "@" + r.Version
}

// conflictLocked 判断一个根包是否与正在同步的任务重叠：版本为空（latest）与同名的任何
// 任务都冲突；指定版本只与同名同版本、或同名的 latest 任务冲突。
func (g *syncGate) conflictLocked(r SyncRoot) string {
	for key := range g.active {
		i := strings.LastIndexByte(key, '@')
		name, version := key[:i], key[i+1:]
		if name != r.Name {
			continue
		}
		if version == "*" {
			return name + "@latest"
		}
		if r.Version == "" || version == r.Version {
			return name + "@" + version
		}
	}
	return ""
}

// reserve 申请一次任务名额并登记要同步的根包。成功返回的 release 必须调用一次。
// by 为 0 表示系统任务，不计每用户额度；maxJobs/maxPerUser <= 0 时用默认值。
func (g *syncGate) reserve(by int64, roots []SyncRoot, maxJobs, maxPerUser int) (func(), error) {
	if maxJobs <= 0 {
		maxJobs = defaultMaxSyncJobs
	}
	if maxPerUser <= 0 {
		maxPerUser = defaultMaxSyncJobsPerUser
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running >= maxJobs {
		return nil, errf(429, "正在同步的任务太多（最多 %d 个同时运行），请等当前任务结束后再试", maxJobs)
	}
	if by != 0 && g.byUser[by] >= maxPerUser {
		return nil, errf(429, "你正在同步的任务太多（最多 %d 个），请等当前任务结束后再试", maxPerUser)
	}
	conflicts := map[string]bool{}
	for _, r := range roots {
		if key := g.conflictLocked(r); key != "" {
			conflicts[key] = true
		}
	}
	if len(conflicts) > 0 {
		names := make([]string, 0, len(conflicts))
		for key := range conflicts {
			names = append(names, key)
		}
		slices.Sort(names)
		if len(names) > 5 {
			names = append(names[:5:5], "…")
		}
		return nil, errf(409, "%s 正在同步中，请等当前任务结束后再试", strings.Join(names, "、"))
	}
	g.running++
	if by != 0 {
		g.byUser[by]++
	}
	keys := make([]string, 0, len(roots))
	for _, r := range roots {
		key := syncRootKey(r)
		g.active[key]++
		keys = append(keys, key)
	}
	var once sync.Once
	return func() {
		once.Do(func() { g.release(by, keys) })
	}, nil
}

func (g *syncGate) release(by int64, keys []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running--
	if by != 0 {
		g.byUser[by]--
		if g.byUser[by] <= 0 {
			delete(g.byUser, by)
		}
	}
	for _, key := range keys {
		g.active[key]--
		if g.active[key] <= 0 {
			delete(g.active, key)
		}
	}
}

// dedupeSyncRoots 按 (name, version) 去掉请求里重复的根包，保留第一次出现的顺序。
func dedupeSyncRoots(roots []SyncRoot) []SyncRoot {
	seen := make(map[string]bool, len(roots))
	out := make([]SyncRoot, 0, len(roots))
	for _, r := range roots {
		key := syncRootKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// job 包装一个同步任务的进度与日志，定期写回数据库。
type job struct {
	mu    sync.Mutex
	st    *store.Store
	j     *store.SyncJob
	lines []string
	dirty time.Time
}

func (j *job) logf(format string, args ...any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	line := time.Now().UTC().Format("15:04:05 ") + fmt.Sprintf(format, args...)
	j.lines = append(j.lines, line)
	// 日志只留最近 400 行
	if len(j.lines) > 400 {
		j.lines = j.lines[len(j.lines)-400:]
	}
}

func (j *job) add(done, failed int, bytes int64) {
	j.mu.Lock()
	j.j.Done += done
	j.j.Failed += failed
	j.j.Bytes += bytes
	j.mu.Unlock()
	j.flush(false)
}

func (j *job) flush(force bool) {
	j.mu.Lock()
	if !force && time.Since(j.dirty) < time.Second {
		j.mu.Unlock()
		return
	}
	j.dirty = time.Now()
	j.j.Log = strings.Join(j.lines, "\n")
	snapshot := *j.j
	j.mu.Unlock()
	_ = j.st.UpdateJob(context.Background(), &snapshot)
	j.mu.Lock()
	j.j.FinishedAt = snapshot.FinishedAt
	j.mu.Unlock()
}

// StartSync 建一个任务并在后台执行，立即返回任务记录。
func (s *Service) StartSync(ctx context.Context, kind string, params SyncParams, by int64) (*store.SyncJob, error) {
	if len(params.Roots) == 0 {
		return nil, errf(400, "没有要同步的包")
	}
	set, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	if UpstreamBase(set) == "" {
		return nil, errf(409, "当前是离线模式或没有配置上游 registry，不能在线预载；请用离线导入")
	}
	for _, r := range params.Roots {
		if !ValidName(r.Name) {
			return nil, errf(400, "包名 %q 不合法", r.Name)
		}
		if InScopes(r.Name, set.CompanyScopes()) {
			return nil, errf(400, "%s 属于企业作用域，只能在企业内发布，不能从上游预载", r.Name)
		}
	}
	if params.RootStatus != store.StatusPublished {
		params.RootStatus = store.StatusPending
	}
	// 同一个请求里重复的包只同步一次。
	params.Roots = dedupeSyncRoots(params.Roots)
	release, err := s.gate.reserve(by, params.Roots, s.MaxSyncJobs, s.MaxSyncJobsPerUser)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(params)
	j := &store.SyncJob{Kind: kind, Params: raw, Total: len(params.Roots), Owner: s.owner}
	if by != 0 {
		j.CreatedBy = &by
	}
	if err := s.store.CreateJob(ctx, j); err != nil {
		release()
		return nil, err
	}
	go s.runSync(j, params, by, release)
	return j, nil
}

func (s *Service) runSync(j *store.SyncJob, params SyncParams, by int64, release func()) {
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	jb := &job{st: s.store, j: j}
	defer func() {
		if r := recover(); r != nil {
			jb.logf("任务异常退出：%v", r)
			jb.j.Status = "failed"
			jb.flush(true)
		}
	}()
	for _, root := range params.Roots {
		if err := s.syncRoot(ctx, jb, root, params, by); err != nil {
			jb.logf("✘ %s：%v", root.Name, err)
			jb.add(1, 1, 0)
			continue
		}
		jb.add(1, 0, 0)
	}
	jb.mu.Lock()
	jb.j.Status = "succeeded"
	if jb.j.Failed > 0 {
		jb.j.Status = "failed"
		if jb.j.Failed < jb.j.Total {
			jb.j.Status = "partial"
		}
	}
	jb.mu.Unlock()
	jb.logf("完成：%d 个成功，%d 个失败", jb.j.Done-jb.j.Failed, jb.j.Failed)
	jb.flush(true)
}

// syncRoot 镜像一个根包与它的依赖闭包，并记下闭包统计。
func (s *Service) syncRoot(ctx context.Context, jb *job, root SyncRoot, params SyncParams, by int64) error {
	set, err := s.settings(ctx)
	if err != nil {
		return err
	}
	base := UpstreamBase(set)
	if base == "" {
		return errors.New("上游不可用（离线模式）")
	}
	if pkg, err := s.store.GetPackage(ctx, root.Name); err == nil && pkg.Origin == store.OriginHosted {
		return fmt.Errorf("本地已有同名的企业托管包")
	}
	doc, err := s.up.Packument(ctx, base, root.Name, -1)
	if err != nil {
		return err
	}
	version := root.Version
	if version == "" {
		version = doc.DistTags["latest"]
	}
	uv := doc.Version(version)
	if uv == nil {
		return fmt.Errorf("上游没有版本 %q", version)
	}
	// 企业目录登记了内容摘要：上游声明的内容必须就是这一份。对不上时直接失败，
	// 连 tarball 都不下载，也不写任何版本记录（失败不留半装）。
	if root.Integrity != "" {
		if uv.Integrity == "" {
			return fmt.Errorf("上游没有给 %s@%s 提供 integrity，无法核对目录登记的摘要", root.Name, version)
		}
		if uv.Integrity != root.Integrity {
			return fmt.Errorf("%s@%s 的内容摘要（%s）和企业目录登记的不一致，拒绝同步", root.Name, version, uv.Integrity)
		}
	}
	status := store.StatusPublished
	if params.ForceReview || (uv.Parsed.IsPlugin && pluginNeedsReview(set)) {
		status = params.RootStatus
	}
	var publisher *int64
	if by != 0 {
		publisher = &by
	}
	nv, err := s.mirror(ctx, root.Name, uv, status, publisher, true)
	if err != nil {
		return err
	}
	_ = s.store.SetPackageDistTags(ctx, root.Name, doc.DistTags)
	if root.Catalog != nil {
		_ = s.store.UpdatePackageCatalog(ctx, root.Name, root.Catalog.patch())
	}
	jb.logf("· %s@%s（%s）", root.Name, version, nv.Status)

	// 依赖里的插件和根包过同一道审核门：否则员工申请引入一个普通包，它依赖的社区插件会不经审核直接上架
	depPlugin := store.StatusPublished
	if pluginNeedsReview(set) {
		depPlugin = params.RootStatus
	}
	c := &closure{s: s, ctx: ctx, base: base, scopes: set.CompanyScopes(), jb: jb, pluginStatus: depPlugin, seen: map[string]bool{root.Name + "@" + version: true}}
	c.sem = make(chan struct{}, mirrorWorkers)
	c.walk(uv.Parsed)
	c.wg.Wait()

	offline := !c.scripts && len(c.unresolved) == 0 && !nv.HasInstallScript
	if err := s.store.SetClosureStats(ctx, root.Name, c.count, c.size+nv.Size, offline); err != nil {
		return err
	}
	if len(c.unresolved) > 0 {
		jb.logf("  %s 有 %d 个依赖没能镜像：%s", root.Name, len(c.unresolved), strings.Join(c.unresolved, "、"))
	}
	jb.logf("✔ %s@%s：依赖 %d 个，共 %.1f MB%s", root.Name, version, c.count, float64(c.size+nv.Size)/(1<<20),
		map[bool]string{true: "，可离线安装", false: "，含安装脚本或外部依赖，离线可能装不上"}[offline])
	jb.add(0, 0, c.size+nv.Size)
	return nil
}

// closure 并发遍历依赖树：dependencies、optionalDependencies 与非可选的 peerDependencies。
type closure struct {
	s      *Service
	ctx    context.Context
	base   string
	scopes []string
	jb     *job
	// 依赖里遇到插件时入库的状态（普通依赖总是直接可用）
	pluginStatus string
	sem          chan struct{}
	wg           sync.WaitGroup

	mu         sync.Mutex
	seen       map[string]bool
	count      int
	size       int64
	scripts    bool
	unresolved []string
}

func (c *closure) walk(m *Manifest) {
	for _, deps := range []map[string]string{m.Dependencies, m.OptionalDependencies, m.RequiredPeers()} {
		for name, spec := range deps {
			c.wg.Add(1)
			go func() {
				defer c.wg.Done()
				c.sem <- struct{}{}
				next := c.visit(name, spec)
				<-c.sem
				if next != nil {
					c.walk(next)
				}
			}()
		}
	}
}

// visit 解析并镜像一个依赖，返回需要继续展开的清单（已经见过时返回 nil）。
func (c *closure) visit(name, spec string) *Manifest {
	if alias, ok := strings.CutPrefix(spec, "npm:"); ok {
		// npm:real-name@range
		i := strings.LastIndex(alias, "@")
		if i <= 0 {
			name, spec = alias, "*"
		} else {
			name, spec = alias[:i], alias[i+1:]
		}
	}
	if !ValidName(name) || nonRegistrySpec(spec) {
		c.fail(name + "@" + spec)
		return nil
	}
	// 企业作用域的依赖在本地托管，不去上游找
	if InScopes(name, c.scopes) {
		return nil
	}
	doc, err := c.s.up.Packument(c.ctx, c.base, name, 0)
	if err != nil {
		c.fail(name)
		c.jb.logf("  ✘ %s：%v", name, err)
		return nil
	}
	version := ResolveRange(spec, doc.VersionNames(), doc.DistTags)
	if version == "" {
		c.fail(name + "@" + spec)
		return nil
	}
	key := name + "@" + version
	c.mu.Lock()
	if c.seen[key] {
		c.mu.Unlock()
		return nil
	}
	c.seen[key] = true
	c.mu.Unlock()

	uv := doc.Version(version)
	if uv == nil {
		c.fail(key)
		return nil
	}
	status := store.StatusPublished
	if uv.Parsed.IsPlugin {
		status = c.pluginStatus
	}
	nv, err := c.s.mirror(c.ctx, name, uv, status, nil, false)
	if err != nil {
		c.fail(key)
		c.jb.logf("  ✘ %s：%v", key, err)
		return nil
	}
	if nv.IsPlugin && nv.Status != store.StatusPublished {
		c.jb.logf("  · 依赖 %s 是插件（%s），审核通过后才能安装", key, nv.Status)
	}
	if len(doc.DistTags) > 0 {
		if pkg, err := c.s.store.GetPackage(c.ctx, name); err == nil && pkg.Origin == store.OriginUpstream {
			_ = c.s.store.SetPackageDistTags(c.ctx, name, doc.DistTags)
		}
	}
	c.mu.Lock()
	c.count++
	c.size += nv.Size
	if nv.HasInstallScript {
		c.scripts = true
	}
	c.mu.Unlock()
	return uv.Parsed
}

func (c *closure) fail(what string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.unresolved) < 50 {
		c.unresolved = append(c.unresolved, what)
	}
}

// nonRegistrySpec 判断依赖是不是 git / URL / 本地路径这类不走 registry 的写法。
func nonRegistrySpec(spec string) bool {
	spec = strings.TrimSpace(spec)
	for _, p := range []string{"git", "github:", "gitlab:", "bitbucket:", "gist:", "file:", "link:", "workspace:", "http:", "https:"} {
		if strings.HasPrefix(spec, p) {
			return true
		}
	}
	// owner/repo 简写
	return strings.Contains(spec, "/") && !strings.HasPrefix(spec, "@")
}

// ---- 已预载插件的新版本 ----

// UpstreamLatest 并发查上游 registry 的 dist-tags.latest（走包信息缓存）；查不到的包不出现在结果里。
// dshfind 目录里的版本常常落后 npm 好几个版本，判断「上游有新版」要以这里为准。
func (s *Service) UpstreamLatest(ctx context.Context, names []string) map[string]string {
	out := map[string]string{}
	set, err := s.settings(ctx)
	if err != nil || UpstreamBase(set) == "" {
		return out
	}
	base := UpstreamBase(set)
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, mirrorWorkers)
	)
	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			doc, err := s.up.Packument(ctx, base, name, 0)
			if err != nil || doc.DistTags["latest"] == "" {
				return
			}
			mu.Lock()
			out[name] = doc.DistTags["latest"]
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// CheckUpdates 检查已镜像的上游插件有没有新版本，按 upstream.autoSync 设置同步。
func (s *Service) CheckUpdates(ctx context.Context, by int64) (*store.SyncJob, error) {
	set, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	if set.Upstream.AutoSync == "off" || UpstreamBase(set) == "" {
		return nil, nil
	}
	pkgs, err := s.store.ListPackages(ctx, store.PackageFilter{CatalogOnly: true})
	if err != nil {
		return nil, err
	}
	var roots []SyncRoot
	for _, p := range pkgs {
		if p.Origin != store.OriginUpstream || !p.IsPlugin {
			continue
		}
		doc, err := s.up.Packument(ctx, UpstreamBase(set), p.Name, -1)
		if err != nil {
			continue
		}
		latest := doc.DistTags["latest"]
		if latest == "" {
			continue
		}
		if _, err := s.store.GetVersion(ctx, p.Name, latest); errors.Is(err, store.ErrNotFound) {
			roots = append(roots, SyncRoot{Name: p.Name, Version: latest})
		}
	}
	if len(roots) == 0 {
		return nil, nil
	}
	status := store.StatusPending
	if set.Upstream.AutoSync == "auto" {
		status = store.StatusPublished
	}
	return s.StartSync(ctx, "update", SyncParams{Roots: roots, RootStatus: status}, by)
}
