package registry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 能力形态与分类：与前端、dshfind 同一套 id。
var (
	Kinds      = []string{"skill", "connector", "knowledge", "plugin"}
	Categories = []string{"skin", "ui", "agent", "memory", "client", "channel", "tools", "fun", "resource"}
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
)

// Error 是带 HTTP 状态码的业务错误，消息直接给 npm 客户端或前端显示。
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func errf(status int, format string, args ...any) error {
	return &Error{Status: status, Msg: fmt.Sprintf(format, args...)}
}

// Service 实现 npm 源的业务规则。
type Service struct {
	store    *store.Store
	blobs    *Blobs
	up       *Upstream
	log      *slog.Logger
	settings func(context.Context) (*store.AppSettings, error)
	// owner 是本实例标识（cfg.Addr），写进新建同步任务的 sync_jobs.owner，
	// 供多实例启动时只清理本实例遗留的任务。
	owner string
	// 单个 tarball 的大小上限
	MaxTarball int64
	// 同时运行的同步/导入任务上限；0 表示用默认值（见 mirror.go）
	MaxSyncJobs        int
	MaxSyncJobsPerUser int
	// dshfind 插件目录（DSH 索引 format 1）的地址，上游预载从这里挑插件
	FeedURL string
	feed    feedCache
	// 同步任务准入（数量上限与 (name,version) 去重）
	gate syncGate
}

func NewService(st *store.Store, blobs *Blobs, up *Upstream, settings func(context.Context) (*store.AppSettings, error), log *slog.Logger, owner string) *Service {
	return &Service{
		store: st, blobs: blobs, up: up, log: log, settings: settings, owner: owner,
		MaxTarball:         100 << 20,
		MaxSyncJobs:        defaultMaxSyncJobs,
		MaxSyncJobsPerUser: defaultMaxSyncJobsPerUser,
		FeedURL:            "https://api.dshfind.com/market/dsh-index.json",
		gate:               newSyncGate(),
	}
}

func (s *Service) Blobs() *Blobs { return s.blobs }

// UpstreamBase 返回上游 registry 地址；离线模式或没配好时返回空串。
func UpstreamBase(set *store.AppSettings) string {
	if set.Upstream.Mode == "offline" {
		return ""
	}
	if set.Upstream.Registry == "custom" {
		return strings.TrimRight(set.Upstream.CustomRegistry, "/")
	}
	return UpstreamRegistries[set.Upstream.Registry]
}

// proxying 判断企业作用域之外的包是否按需回源。
func proxying(set *store.AppSettings) bool {
	return set.Registry.ProxyMode == "cache" && UpstreamBase(set) != ""
}

// pluginNeedsReview：白名单模式下，上游插件的版本要先在本地审核上架才对外可见。
func pluginNeedsReview(set *store.AppSettings) bool { return set.AdmissionMode != "open" }

// PluginNeedsReview 和同步时的审核门是同一个判断：企业目录安装插件时用它告诉管理员
// 「同步后还要等审核」，不要在两处各写一遍策略。
func PluginNeedsReview(set *store.AppSettings) bool { return pluginNeedsReview(set) }

// localOnly 判断一个包是否只能用本地数据：企业作用域，或者曾经在这里托管过（防依赖混淆：
// 即使管理员后来删掉了作用域，已托管的包名也永远不会去上游找）。
func localOnly(set *store.AppSettings, name string, pkg *store.NpmPackage) bool {
	return InScopes(name, set.CompanyScopes()) || (pkg != nil && pkg.Origin == store.OriginHosted)
}

func localVersion(v *store.NpmVersion) (Version, error) {
	var m map[string]any
	if err := json.Unmarshal(v.Manifest, &m); err != nil {
		return Version{}, fmt.Errorf("%s@%s 的清单损坏：%w", v.Name, v.Version, err)
	}
	out := Version{Version: v.Version, Manifest: m, Integrity: v.Integrity, Shasum: v.Shasum, Time: v.CreatedAt}
	if v.PublishedAt != nil {
		out.Time = *v.PublishedAt
	}
	if v.Deprecated != nil {
		out.Deprecated = *v.Deprecated
	}
	return out, nil
}

// Packument 返回对外可见的包文档。
func (s *Service) Packument(ctx context.Context, name string) (*Packument, error) {
	set, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	pkg, err := s.store.GetPackage(ctx, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	local, err := s.store.ListVersions(ctx, name, store.StatusPublished)
	if err != nil {
		return nil, err
	}
	var versions []Version
	for _, v := range local {
		lv, err := localVersion(v)
		if err != nil {
			s.log.Warn("跳过损坏的版本", "err", err)
			continue
		}
		versions = append(versions, lv)
	}
	tags := map[string]string{}
	if pkg != nil {
		tags = pkg.DistTags
	}

	if !localOnly(set, name, pkg) && proxying(set) {
		doc, err := s.up.Packument(ctx, UpstreamBase(set), name, 0)
		switch {
		case errors.Is(err, ErrUpstreamNotFound):
		case err != nil:
			if len(versions) == 0 {
				return nil, errf(502, "上游不可用：%v", err)
			}
			s.log.Warn("上游不可用，只返回本地已有版本", "pkg", name, "err", err)
		default:
			statuses, err := s.store.VersionStatuses(ctx, name)
			if err != nil {
				return nil, err
			}
			var upstream []Version
			for _, v := range doc.VersionNames() {
				if st, known := statuses[v]; known && st != store.StatusPublished {
					continue
				}
				uv := doc.Version(v)
				if uv == nil || (uv.Parsed.IsPlugin && pluginNeedsReview(set) && statuses[v] != store.StatusPublished) {
					continue
				}
				upstream = append(upstream, Version{Version: v, Manifest: uv.Manifest, Integrity: uv.Integrity, Shasum: uv.Shasum, Time: uv.Time, Deprecated: uv.Parsed.Deprecated})
			}
			versions = MergeVersions(upstream, versions)
			tags = doc.DistTags
		}
	}
	if len(versions) == 0 {
		return nil, ErrNotFound
	}
	return &Packument{Name: name, Versions: versions, DistTags: tags}, nil
}

// VersionFromFile 从 tarball 文件名里取版本号：<去掉作用域的包名>-<版本>.tgz。
func VersionFromFile(name, file string) (string, bool) {
	base := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		base = name[i+1:]
	}
	v, ok := strings.CutPrefix(file, base+"-")
	if !ok {
		return "", false
	}
	v, ok = strings.CutSuffix(v, ".tgz")
	return v, ok && v != ""
}

// Tarball 打开某个版本的 tarball。企业作用域之外、本地还没有的版本在回源模式下现拉现存。
func (s *Service) Tarball(ctx context.Context, name, version string) (*os.File, *store.NpmVersion, error) {
	v, err := s.store.GetVersion(ctx, name, version)
	if err == nil {
		if v.Status != store.StatusPublished {
			return nil, nil, ErrNotFound
		}
		f, err := s.blobs.Open(v.BlobKey)
		if err != nil {
			return nil, nil, fmt.Errorf("%s@%s 的 tarball 丢失：%w", name, version, err)
		}
		return f, v, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, nil, err
	}

	set, err := s.settings(ctx)
	if err != nil {
		return nil, nil, err
	}
	pkg, err := s.store.GetPackage(ctx, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, nil, err
	}
	if localOnly(set, name, pkg) || !proxying(set) {
		return nil, nil, ErrNotFound
	}
	base := UpstreamBase(set)
	doc, err := s.up.Packument(ctx, base, name, 0)
	if errors.Is(err, ErrUpstreamNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, errf(502, "上游不可用：%v", err)
	}
	uv := doc.Version(version)
	if uv == nil {
		// 可能是刚发布的新版本，缓存里还没有
		if doc, err = s.up.Packument(ctx, base, name, -1); err == nil {
			uv = doc.Version(version)
		}
	}
	if uv == nil || (uv.Parsed.IsPlugin && pluginNeedsReview(set)) {
		return nil, nil, ErrNotFound
	}
	nv, err := s.mirror(ctx, name, uv, store.StatusPublished, nil, false)
	if err != nil {
		return nil, nil, errf(502, "%v", err)
	}
	f, err := s.blobs.Open(nv.BlobKey)
	return f, nv, err
}

// mirror 下载并记录一个上游版本；已存在时直接返回本地记录。inspect 为 true 时解包做检查（插件与预载根）。
func (s *Service) mirror(ctx context.Context, name string, uv *UpstreamVersion, status string, publisher *int64, inspect bool) (*store.NpmVersion, error) {
	if v, err := s.store.GetVersion(ctx, name, uv.Version); err == nil {
		return v, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	d, err := s.up.FetchTarball(ctx, s.blobs, uv.Tarball, uv.Integrity, uv.Shasum, s.MaxTarball)
	if err != nil {
		return nil, err
	}
	var scan json.RawMessage
	if inspect || uv.Parsed.IsPlugin {
		f, err := s.blobs.Open(d.Key)
		if err != nil {
			return nil, err
		}
		info, err := InspectTarball(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s@%s：%w", name, uv.Version, err)
		}
		inner, err := ParseManifest(info.PackageJSON)
		if err != nil {
			return nil, fmt.Errorf("%s@%s 的 package.json 不是合法 JSON", name, uv.Version)
		}
		// 元数据与 tarball 里的清单对不上（manifest confusion）：拒收
		if inner.Name != name || inner.Version != uv.Version {
			return nil, fmt.Errorf("%s@%s 的 tarball 里写的是 %s@%s，与上游元数据不一致", name, uv.Version, inner.Name, inner.Version)
		}
		set, err := s.settings(ctx)
		if err != nil {
			return nil, err
		}
		items := Check(inner, info, d.Size, set.CompanyScopes())
		scan, _ = json.Marshal(items)
		// 自动检查有 LevelFail 的包不能因为上游预载传了 publish=true 就直接上架；
		// 与 Service.Publish 的 open 模式闸门保持一致，先进入待审核。
		if status == store.StatusPublished && slices.ContainsFunc(items, func(i ScanItem) bool { return i.Level == LevelFail }) {
			status = store.StatusPending
		}
	}

	manifest, err := json.Marshal(trimManifest(uv.Manifest))
	if err != nil {
		return nil, err
	}
	nv := &store.NpmVersion{
		Name: name, Version: uv.Version, Manifest: manifest,
		Integrity: d.Integrity, Shasum: d.Shasum, Size: d.Size, BlobKey: d.Key,
		IsPlugin: uv.Parsed.IsPlugin, HasInstallScript: len(uv.Parsed.InstallScripts()) > 0,
		Status: status, Scan: scan, PublisherID: publisher, CreatedAt: uv.Time,
	}
	if nv.CreatedAt.IsZero() {
		nv.CreatedAt = time.Now().UTC()
	}
	if status == store.StatusPublished {
		t := time.Now().UTC().Truncate(time.Millisecond)
		nv.PublishedAt = &t
	}
	if uv.Parsed.Deprecated != "" {
		nv.Deprecated = &uv.Parsed.Deprecated
	}
	pkg := &store.NpmPackage{Name: name, IsPlugin: uv.Parsed.IsPlugin, Title: uv.Parsed.Title(),
		Description: truncateRunes(plainText(uv.Parsed.Description), 1000), Author: truncateRunes(uv.Parsed.Author, 250),
		Homepage: httpURL(uv.Parsed.Homepage), Repository: httpURL(repoURL(uv.Parsed.RepoURL))}
	if err := s.store.EnsureUpstreamPackage(ctx, pkg); err != nil {
		return nil, err
	}
	if uv.Parsed.IsPlugin {
		yes := true
		_ = s.store.UpdatePackageCatalog(ctx, name, store.CatalogPatch{IsPlugin: &yes})
	}
	if err := s.store.InsertMirroredVersion(ctx, nv); err != nil {
		if errors.Is(err, store.ErrExists) {
			return s.store.GetVersion(ctx, name, uv.Version)
		}
		return nil, err
	}
	return nv, nil
}

// trimManifest 去掉不该存的大字段：readme 在版本文档里没有用，上游个别包会塞进去几 MB。
func trimManifest(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch k {
		case "readme", "readmeFilename", "_attachments":
			continue
		}
		out[k] = v
	}
	return out
}

// ---- 发布 ----

// PublishDoc 是 npm publish 发来的请求体。
type PublishDoc struct {
	ID          string                     `json:"_id"`
	Name        string                     `json:"name"`
	DistTags    map[string]string          `json:"dist-tags"`
	Versions    map[string]json.RawMessage `json:"versions"`
	Attachments map[string]struct {
		ContentType string `json:"content_type"`
		Data        string `json:"data"`
		Length      int64  `json:"length"`
	} `json:"_attachments"`
	Readme string `json:"readme"`
}

// Publisher 是发布者的身份与权限。
// ConnectName 是 dsh-ee 生成的 DSH 接入插件在企业作用域下的包名。
const ConnectName = "dsh-connect"

type Publisher struct {
	UserID   int64
	Username string
	// 有能力管理权限（可以替别人发新版本、发布后直接上架以外的事交给审核）
	Manager bool
	// 跳过审核直接上架：只给 dsh-ee 自己生成的包（如 DSH 接入组合包）用。UserID 为 0 表示系统发布。
	AutoPublish bool
}

type PublishResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Created bool   `json:"created"`
}

// Publish 发布 npm 客户端发来的文档；tarball 在 _attachments 里、base64 编码。
func (s *Service) Publish(ctx context.Context, p Publisher, doc *PublishDoc) (*PublishResult, error) {
	if len(doc.Versions) != 1 || len(doc.Attachments) != 1 {
		return nil, errf(400, "一次只能发布一个版本（versions 与 _attachments 各一个）")
	}
	// 校验都通过之后才解码 base64：包名/版本/归属不合法的请求不带 100 MB 的附件做无用功。
	return s.publish(ctx, p, doc, nil, func() ([]byte, error) {
		var att string
		for _, a := range doc.Attachments {
			att = a.Data
		}
		raw, err := base64.StdEncoding.DecodeString(att)
		if err != nil {
			return nil, errf(400, "tarball 不是合法的 base64")
		}
		return raw, nil
	})
}

// PublishTarball 发布网页上传的 tarball：请求体已经是解码后的 .tgz，直接用它发布，
// 省掉 base64 编码之后再解码产生的两份大内存（单包上限 100 MB）。
func (s *Service) PublishTarball(ctx context.Context, p Publisher, doc *PublishDoc, raw []byte) (*PublishResult, error) {
	if len(doc.Versions) != 1 || len(doc.Attachments) != 0 {
		return nil, errf(400, "一次只能发布一个版本（versions 一个，tarball 在请求体里）")
	}
	if len(raw) == 0 {
		return nil, errf(400, "tarball 为空")
	}
	return s.publish(ctx, p, doc, raw, nil)
}

// publish 发布企业作用域里的一个版本：校验 → 存 tarball → 解包检查 → 入库（待审核或直接上架）。
// raw 非空时直接用它；否则调用 decode 拿到 tarball 字节（npm 发布是 base64 附件）。
func (s *Service) publish(ctx context.Context, p Publisher, doc *PublishDoc, raw []byte, decode func() ([]byte, error)) (*PublishResult, error) {
	set, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	name := doc.Name
	scopes := set.CompanyScopes()
	if !ValidPublishName(name) {
		return nil, errf(400, "包名 %q 不合法（小写字母、数字与 - . _ ~）", name)
	}
	if !InScopes(name, scopes) {
		return nil, errf(403, "只能发布到企业作用域（%s），包名要写成 %s/<名字>", strings.Join(scopes, "、"), set.Scope)
	}
	// 接入插件由 dsh-ee 自己生成并直接上架，每台电脑都装。员工抢先发一个同名的包就成了它的维护人，
	// 之后还能改它的标签。
	if strings.HasSuffix(name, "/"+ConnectName) && !p.AutoPublish {
		return nil, errf(403, "%s 这个包名留给 dsh-ee 生成的 DSH 接入插件，请换一个名字", name)
	}
	var version string
	var meta json.RawMessage
	for v, m := range doc.Versions {
		version, meta = v, m
	}
	if !ValidVersion(version) {
		return nil, errf(400, "版本号 %q 不是合法的 semver", version)
	}

	pkg, err := s.store.GetPackage(ctx, name)
	switch {
	case errors.Is(err, store.ErrNotFound):
		pkg = nil
	case err != nil:
		return nil, err
	case pkg.Origin != store.OriginHosted:
		return nil, errf(409, "%s 已经作为上游包镜像在本地，不能在企业作用域里发布同名包", name)
	case !ownedBy(pkg, p) && !p.Manager && !p.AutoPublish:
		return nil, errf(403, "%s 由其他人维护，你不能发布新版本", name)
	}
	if _, err := s.store.GetVersion(ctx, name, version); err == nil {
		return nil, errf(409, "%s@%s 已经存在（被驳回或下架的版本号也不能重复使用），请升级版本号后再发布", name, version)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	if raw == nil {
		if raw, err = decode(); err != nil {
			return nil, err
		}
	}
	if int64(len(raw)) > s.MaxTarball {
		return nil, errf(413, "tarball 超过 %d MB", s.MaxTarball>>20)
	}
	d, err := s.blobs.Put(bytes.NewReader(raw), s.MaxTarball)
	if err != nil {
		return nil, err
	}
	var declared struct {
		Dist struct {
			Integrity string `json:"integrity"`
			Shasum    string `json:"shasum"`
		} `json:"dist"`
		GitHead     string `json:"gitHead"`
		NodeVersion string `json:"_nodeVersion"`
		NpmVersion  string `json:"_npmVersion"`
	}
	_ = json.Unmarshal(meta, &declared)
	if declared.Dist.Integrity != "" || declared.Dist.Shasum != "" {
		if err := CheckIntegrity(d, declared.Dist.Integrity, declared.Dist.Shasum); err != nil {
			return nil, errf(400, "tarball 与声明的摘要不一致：%v", err)
		}
	}
	info, err := InspectTarball(bytes.NewReader(raw))
	if err != nil {
		return nil, errf(400, "tarball 无法解包：%v", err)
	}
	var inner map[string]any
	if err := json.Unmarshal(info.PackageJSON, &inner); err != nil {
		return nil, errf(400, "tarball 里的 package.json 不是合法 JSON")
	}
	m := manifestFromMap(inner)
	if m.Name != name || m.Version != version {
		return nil, errf(400, "tarball 里的 package.json 是 %s@%s，与发布信息 %s@%s 不一致", m.Name, m.Version, name, version)
	}

	// 对外的清单以 tarball 里的 package.json 为准（防止发布元数据与实际内容不一致），
	// 只补上 npm 客户端附带的几个说明字段
	stored := trimManifest(inner)
	for k, v := range map[string]string{"gitHead": declared.GitHead, "_nodeVersion": declared.NodeVersion, "_npmVersion": declared.NpmVersion} {
		if v != "" {
			stored[k] = v
		}
	}
	stored["_npmUser"] = map[string]string{"name": p.Username}
	if r := doc.Readme; r != "" && len(r) <= 256<<10 {
		stored["readme"] = r
	}
	manifest, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}

	items := Check(m, info, d.Size, scopes)
	scan, _ := json.Marshal(items)
	status := store.StatusPending
	if p.AutoPublish || set.AdmissionMode == "open" && !slices.ContainsFunc(items, func(i ScanItem) bool { return i.Level == LevelFail }) {
		status = store.StatusPublished
	}
	var uid *int64
	if p.UserID > 0 {
		id := p.UserID
		uid = &id
	}
	nv := &store.NpmVersion{
		Name: name, Version: version, Manifest: manifest,
		Integrity: d.Integrity, Shasum: d.Shasum, Size: d.Size, BlobKey: d.Key,
		IsPlugin: m.IsPlugin, HasInstallScript: len(m.InstallScripts()) > 0,
		Status: status, Scan: scan, PublisherID: uid,
	}
	if status == store.StatusPublished {
		t := time.Now().UTC().Truncate(time.Millisecond)
		nv.PublishedAt = &t
	}
	tags := map[string]string{}
	for tag, v := range doc.DistTags {
		if v == version && validTag(tag) {
			tags[tag] = v
		}
	}
	if len(tags) == 0 {
		tags["latest"] = version
	}
	catalog := catalogFromManifest(m)
	if pkg != nil && pkg.IsPlugin {
		yes := true
		catalog.IsPlugin = &yes
	}
	created, err := s.store.PublishHosted(ctx, &store.NpmPackage{Name: name, Origin: store.OriginHosted, OwnerID: uid}, nv, tags, catalog)
	if errors.Is(err, store.ErrExists) {
		return nil, errf(409, "%s@%s 正在被同时发布，请稍后重试", name, version)
	}
	if err != nil {
		return nil, err
	}
	return &PublishResult{Name: name, Version: version, Status: status, Created: created}, nil
}

// catalogFromManifest 从清单派生能力市场的展示信息。
func catalogFromManifest(m *Manifest) store.CatalogPatch {
	title := truncateRunes(m.Title(), 250)
	desc := truncateRunes(plainText(m.Description), 1000)
	author := truncateRunes(m.Author, 250)
	home := httpURL(m.Homepage)
	repo := httpURL(repoURL(m.RepoURL))
	p := store.CatalogPatch{Title: &title, Description: &desc, Author: &author, Homepage: &home, Repository: &repo}
	if slices.Contains(Categories, m.EE.Category) {
		p.Category = &m.EE.Category
	}
	if slices.Contains(Kinds, m.EE.Kind) {
		p.Kind = &m.EE.Kind
	}
	if icon := iconURL(m.EE.Icon); icon != "" {
		p.Icon = &icon
	}
	if m.IsPlugin {
		yes := true
		p.IsPlugin = &yes
	}
	return p
}

// httpURL 只保留 http(s) 地址（与 DSH 索引读端的仓库地址同一口径：无凭据、无 query / fragment）。
func httpURL(raw string) string {
	u := webURL(raw)
	if u == nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.String()
}

// iconURL 是图标地址：允许 query（GitHub 头像是 …?s=96），不允许凭据与 fragment。
func iconURL(raw string) string { return iconURLFor(raw, "") }

// iconURLFor 按 DSH 插件市场索引读端的规则保留图标：
// 只允许 https，或与索引同源的 http；否则整个索引会被 DSH 拒收。
func iconURLFor(raw, base string) string {
	u := webURL(raw)
	if u == nil || u.Fragment != "" {
		return ""
	}
	if u.Scheme == "https" {
		return u.String()
	}
	if u.Scheme == "http" {
		b, err := url.Parse(base)
		if err == nil && b.Scheme == "http" && b.Host == u.Host {
			return u.String()
		}
	}
	return ""
}

func webURL(raw string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(u.String()) > 500 {
		return nil
	}
	return u
}

// repoURL 把 git+https://…/x.git、github:owner/repo 之类的写法转成网页地址。
func repoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, "github:"); ok {
		raw = "https://github.com/" + rest
	} else if !strings.Contains(raw, ":") && strings.Count(raw, "/") == 1 {
		raw = "https://github.com/" + raw
	}
	raw = strings.TrimPrefix(raw, "git+")
	if rest, ok := strings.CutPrefix(raw, "git://"); ok {
		raw = "https://" + rest
	}
	if rest, ok := strings.CutPrefix(raw, "ssh://git@"); ok {
		raw = "https://" + rest
	}
	return strings.TrimSuffix(raw, ".git")
}

var tagChars = func(r rune) bool {
	return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
}

// validTag：dist-tag 不能是合法的版本范围（npm 的规则），这里再限制成简单字符。
func validTag(tag string) bool {
	if tag == "" || len(tag) > 64 || strings.IndexFunc(tag, tagChars) >= 0 {
		return false
	}
	return ResolveRange(tag, []string{"0.0.0"}, nil) == "" && !ValidVersion(tag)
}

// ---- 已托管包的维护：dist-tag 与弃用 ----

// hostedForWrite 取一个可以由 p 修改的托管包。
func (s *Service) hostedForWrite(ctx context.Context, p Publisher, name string) (*store.NpmPackage, error) {
	pkg, err := s.store.GetPackage(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if pkg.Origin != store.OriginHosted {
		return nil, errf(403, "%s 不是企业托管的包", name)
	}
	if !ownedBy(pkg, p) && !p.Manager {
		return nil, errf(403, "%s 由其他人维护", name)
	}
	return pkg, nil
}

// ownedBy 报告包是不是 p 维护的。没有维护人的包（后台任务发布的接入插件）不属于任何员工，
// 只有管理员能动：按「没人维护就谁都能发」处理的话，每台电脑都装的接入插件人人可改。
func ownedBy(pkg *store.NpmPackage, p Publisher) bool {
	return pkg.OwnerID != nil && *pkg.OwnerID == p.UserID
}

// SetDistTag 设置或删除（version 为空）一个 dist-tag。
func (s *Service) SetDistTag(ctx context.Context, p Publisher, name, tag, version string) (map[string]string, error) {
	pkg, err := s.hostedForWrite(ctx, p, name)
	if err != nil {
		return nil, err
	}
	if !validTag(tag) {
		return nil, errf(400, "标签 %q 不合法", tag)
	}
	if version == "" {
		if tag == "latest" {
			return nil, errf(400, "不能删除 latest 标签")
		}
		delete(pkg.DistTags, tag)
	} else {
		if _, err := s.store.GetVersion(ctx, name, version); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errf(404, "%s@%s 不存在", name, version)
			}
			return nil, err
		}
		pkg.DistTags[tag] = version
	}
	if err := s.store.SetPackageDistTags(ctx, name, pkg.DistTags); err != nil {
		return nil, err
	}
	return pkg.DistTags, nil
}

// Deprecate 按 npm deprecate 发来的文档更新各版本的弃用说明。
func (s *Service) Deprecate(ctx context.Context, p Publisher, name string, versions map[string]json.RawMessage) (int, error) {
	if _, err := s.hostedForWrite(ctx, p, name); err != nil {
		return 0, err
	}
	existing, err := s.store.ListVersions(ctx, name)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, v := range existing {
		raw, ok := versions[v.Version]
		if !ok {
			continue
		}
		var body struct {
			Deprecated *string `json:"deprecated"`
		}
		if json.Unmarshal(raw, &body) != nil {
			continue
		}
		var want *string
		if body.Deprecated != nil && *body.Deprecated != "" {
			msg := truncateRunes(*body.Deprecated, 1000)
			want = &msg
		}
		if (want == nil) == (v.Deprecated == nil) && (want == nil || *want == *v.Deprecated) {
			continue
		}
		if err := s.store.SetVersionDeprecated(ctx, name, v.Version, want); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// ---- 审核 ----

// Review 处理审核动作。version 为空时作用于整个包（下架 / 恢复）。
func (s *Service) Review(ctx context.Context, reviewer int64, name, version, action, note string) (int64, error) {
	var from []string
	var to string
	switch action {
	case "approve":
		from, to = []string{store.StatusPending}, store.StatusPublished
	case "reject":
		from, to = []string{store.StatusPending}, store.StatusRejected
	case "unpublish":
		from, to = []string{store.StatusPublished, store.StatusPending}, store.StatusUnpublished
	case "restore":
		from, to = []string{store.StatusUnpublished, store.StatusRejected}, store.StatusPublished
	default:
		return 0, errf(400, "未知的审核动作 %q", action)
	}
	if (action == "reject" || action == "unpublish") && strings.TrimSpace(note) == "" {
		return 0, errf(400, "驳回与下架需要填写原因")
	}
	if version == "" && (action == "approve" || action == "reject") {
		return 0, errf(400, "审核要指定版本")
	}
	// 整包恢复只恢复上过架的版本。否则被驳回的、还没审就随整包下架的版本会跟着一起上架，
	// latest 也会指到没人审过的版本上；这些版本要逐个恢复，那是对这一个版本的决定。
	wasLive := action == "restore" && version == ""
	n, err := s.store.SetVersionStatus(ctx, name, version, from, to, reviewer, truncateRunes(note, 1000), wasLive)
	if err != nil {
		return 0, err
	}
	if n == 0 && wasLive {
		return 0, errf(409, "这个包没有上过架的版本可以恢复；被驳回或没审过的版本请逐个恢复")
	}
	if n == 0 {
		return 0, errf(409, "没有处于可%s状态的版本", map[string]string{"approve": "通过", "reject": "驳回", "unpublish": "下架", "restore": "恢复"}[action])
	}
	return n, nil
}

// Open 打开任意状态版本的 tarball（审核时下载查看用，调用方负责权限）。
func (s *Service) Open(v *store.NpmVersion) (io.ReadCloser, error) { return s.blobs.Open(v.BlobKey) }
