package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业目录条目的安装 / 升级（docs/plugin-signing.md §8 的下一步）。directory.go 只负责把
// 签过名的目录登记成来源，装不装由管理员在这里明确触发：**不自动安装、不自动升级**。
//
//   - 扩展（kind=extension）：按条目登记的固定 tarball 地址下载 zip，核对身份 / 版本 / 摘要
//     （以及条目声明的签名者）后，复用网页安装同一条路径（安装计划、敏感项确认、签名策略、
//     连接校验）。tarball 地址在下载前按当前策略再过一遍，防止内网地址被带进来。
//   - 插件（kind=dsh-plugin）：只接受 npm 来源的固定包名 + 固定版本，交给现有 registry 预载
//     流程，入库状态是 pending，和普通预载一样要在审核页通过才对外可见——不绕过审核。
//
// 失败都在写库之前拦下：扩展不会留下装了一半的连接，插件不会在版本表里留下半份记录。

const (
	auditDirectoryInstall = "directory.install"
	auditDirectoryUpgrade = "directory.upgrade"

	// 目录安装/升级请求体：只有管理员参数、OAuth 客户端这类文字。
	maxDirInstallRequest = 1 << 20
	// 下载扩展 zip 的超时和上限：目录 v1 的扩展包上限 8 MiB（extension.ReadZip 再兜一次）。
	dirExtFetchTimeout = 30 * time.Second
)

// dirInstallReq 是安装 / 升级目录条目的请求体。和网页安装扩展一样，只是不能带包：
// 内容只从目录登记的来源取，请求里塞不进别的代码。
type dirInstallReq struct {
	// 升级时有需要管理员确认的敏感变化（凭据模式、新地址等）要传 true
	Confirm bool `json:"confirm"`
	// 只出计划、不执行：管理页打开安装 / 升级对话框时先拿计划给管理员看，
	// 确认后再用同一接口（不带 plan_only）执行；扩展的下载/核对照做，插件不会启动同步。
	PlanOnly bool `json:"plan_only"`
	// 新装时的连接标识；升级时忽略（沿用原来的）
	Slug string `json:"slug"`
	// 接口根地址；空则用接口文档里的 servers
	URL string `json:"url"`
	// shared 模式的共享凭据放在哪里
	Auth *connAuth `json:"auth"`
	// 共享凭据的明文（只写不读）
	Secret *string `json:"secret"`
	// 清单里 fill: admin 的参数值
	Admin map[string]json.RawMessage `json:"admin"`
	Scope *store.Scope               `json:"scope"`
	// 扩展用 oauth 凭据时，管理员在对方开放平台的客户端；secret 留空表示沿用原来的
	OAuthClientID     string `json:"oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret"`
}

// dirPluginPlan 是插件条目的安装计划：目标版本、企业源里现在是什么状态、要不要审核。
type dirPluginPlan struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Channel  string `json:"channel"`
	Digest   string `json:"digest"`
	Source   string `json:"source"`
	Registry string `json:"registry,omitempty"`
	// 企业源里已有的版本和状态（同一版本）
	ExistingVersion string `json:"existing_version,omitempty"`
	ExistingStatus  string `json:"existing_status,omitempty"`
	// 本次要从上游同步的版本；空表示不需要同步
	SyncVersion string `json:"sync_version,omitempty"`
	// 同步后还要在审核页通过才对外可见
	ReviewRequired bool `json:"review_required"`
	// 给人看的一句话：这次做什么、为什么没做
	Message string `json:"message,omitempty"`
}

// dirInstallPlan 是安装 / 升级的返回：计划 + 结果。失败时也尽量给全，管理员按它排查。
type dirInstallPlan struct {
	OK          bool                  `json:"ok"`
	Action      string                `json:"action"` // install / upgrade
	Kind        string                `json:"kind"`
	DirectoryID string                `json:"directory_id"`
	Entry       *store.DirectoryEntry `json:"entry"`
	// 扩展：完整的扩展安装计划（check / signature / sensitive / existing）
	Extension *extPlan `json:"extension,omitempty"`
	// 插件：同步 / 审核计划
	Plugin *dirPluginPlan `json:"plugin,omitempty"`
	// 需要管理员确认的变化
	Sensitive []string `json:"sensitive,omitempty"`
	// 结果：装成的连接、同步任务或当前状态
	Result map[string]any `json:"result,omitempty"`
	// 给人看的一句话（没做事的原因）
	Message string `json:"message,omitempty"`
	// 这是只出计划的预览（plan_only），什么都没改
	PlanOnly bool `json:"plan_only,omitempty"`
}

// writeDirectoryPlan 统一写回安装 / 升级的计划或结果。
func writeDirectoryPlan(w http.ResponseWriter, status int, out *dirInstallPlan, planOnly bool) {
	out.PlanOnly = planOnly
	writeJSON(w, status, out)
}

// directoryEntryInstallPerm 是某类目录条目安装 / 升级需要的权限点。
func directoryEntryInstallPerm(kind string) string {
	if kind == directory.KindPlugin {
		return rbac.PluginsManage
	}
	return rbac.ConnectionsManage
}

// dirInstallAction 把入口转成计划里的动作名。
func dirInstallAction(upgrade bool) string {
	if upgrade {
		return "upgrade"
	}
	return "install"
}

// dirEntryTarget 是审计目标：这份目录里的哪一条。
func dirEntryTarget(e *store.DirectoryEntry) string {
	return "directory:" + e.DirectoryID + "#" + itoa64(e.ID)
}

// dirEntryIdentity 是条目在日志 / 返回里的标识。
func dirEntryIdentity(e *store.DirectoryEntry) string {
	return entryIdentity(e.Kind, e.Name, e.Version, e.Channel)
}

// dirInstallPlanOf 组装返回体。
func dirInstallPlanOf(entry *store.DirectoryEntry, upgrade bool) *dirInstallPlan {
	return &dirInstallPlan{
		Action:      dirInstallAction(upgrade),
		Kind:        entry.Kind,
		DirectoryID: entry.DirectoryID,
		Entry:       entry,
		Sensitive:   []string{},
	}
}

// readDirInstallReq 读请求体；没有请求体时当成空请求（插件安装不需要额外参数）。
func readDirInstallReq(w http.ResponseWriter, r *http.Request) (*dirInstallReq, bool) {
	req := &dirInstallReq{}
	if r.ContentLength == 0 {
		return req, true
	}
	if !readJSONLimit(w, r, req, maxDirInstallRequest) {
		return nil, false
	}
	return req, true
}

// POST /api/admin/directory/entries/{id}/install：把一条登记过的目录条目装进企业。
// 权限按条目类型：扩展要 connections.manage，插件要 plugins.manage。
func (s *Server) handleDirectoryEntryInstall(w http.ResponseWriter, r *http.Request) {
	s.directoryEntryInstall(w, r, false)
}

// POST /api/admin/directory/entries/{id}/upgrade：把已装过的条目升级到目录登记的版本。
// 只做管理员明确触发的一次升级，不做自动升级。
func (s *Server) handleDirectoryEntryUpgrade(w http.ResponseWriter, r *http.Request) {
	s.directoryEntryInstall(w, r, true)
}

func (s *Server) directoryEntryInstall(w http.ResponseWriter, r *http.Request, upgrade bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, ok := readDirInstallReq(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	entry, err := s.store.GetDirectoryEntry(ctx, id)
	if err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "没有这条已登记的目录条目：先 apply 目录，或者刷新列表")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	// 权限按类型：装扩展是「管连接」，装插件是「管插件」。
	need := directoryEntryInstallPerm(entry.Kind)
	if !currentAccess(r).Has(need) {
		writeErr(w, http.StatusForbidden, "装这类目录条目需要 "+need+" 权限")
		return
	}
	switch entry.Kind {
	case directory.KindExtension:
		s.dirInstallExtension(ctx, w, r, entry, req, upgrade)
	case directory.KindPlugin:
		s.dirInstallPlugin(ctx, w, r, entry, req, upgrade)
	default:
		writeErr(w, http.StatusBadRequest, "不认识的条目类型「"+entry.Kind+"」：只支持 extension 或 dsh-plugin")
	}
}

// dirFailPlan 记一条「没装成」的计划并写回。
func dirFailPlan(w http.ResponseWriter, status int, entry *store.DirectoryEntry, upgrade bool, msg string, mutate func(*dirInstallPlan)) {
	plan := dirInstallPlanOf(entry, upgrade)
	plan.Message = msg
	if mutate != nil {
		mutate(plan)
	}
	writeJSON(w, status, map[string]any{"error": msg, "plan": plan})
}

// ---- 扩展 ----

// fetchDirectoryExtension 按条目登记的固定 tarball 地址下载扩展 zip。
// 下载前按当前策略重新检查地址（纵深防御：登记后策略可能调整，登记表也可能被别的路径改过），
// 然后流式读、限大小，绝不解压到磁盘。
func (s *Server) fetchDirectoryExtension(ctx context.Context, entry *store.DirectoryEntry) ([]byte, error) {
	url := strings.TrimSpace(entry.SourceURL)
	if msg := directory.CheckTarballURL(url, s.opts.DirectoryTarballHosts); msg != "" {
		return nil, statusErr(http.StatusBadRequest, "条目来源地址不允许下载："+msg)
	}
	ctx, cancel := context.WithTimeout(ctx, dirExtFetchTimeout)
	defer cancel()
	// 没有白名单时只允许公网地址：登记校验按主机名做的，下载前再按 DNS 解析结果核一遍，
	// 防止域名解析到内网 / 云主机元数据地址（有白名单时以内网对象存储是正常用法，交给白名单）。
	if len(s.opts.DirectoryTarballHosts) == 0 {
		if u, err := neturl.Parse(url); err == nil && u.Hostname() != "" {
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
			if err != nil {
				return nil, statusErr(http.StatusBadGateway, "解析扩展包地址失败："+err.Error())
			}
			for _, addr := range ips {
				if msg := directory.PublicHostProblem(addr.IP.String()); msg != "" {
					return nil, statusErr(http.StatusBadRequest, "扩展包地址解析到不允许的地址："+msg+"（企业内网存储要显式列进 DSH_EE_DIRECTORY_TARBALL_HOSTS）")
				}
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, statusErr(http.StatusBadRequest, "条目来源地址不合法："+err.Error())
	}
	tr := s.upstream.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	// 不跟随跨主机重定向：那等于把「只从这个地址下载」绕过去了。
	hc := &http.Client{Transport: tr, Timeout: dirExtFetchTimeout, CheckRedirect: sameHostRedirect}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, statusErr(http.StatusBadGateway, "下载扩展包失败："+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusErr(http.StatusBadGateway, fmt.Sprintf("下载扩展包失败：%s 返回 HTTP %d", url, resp.StatusCode))
	}
	maxBytes := directory.MaxTarballSize(directory.KindExtension)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, statusErr(http.StatusBadGateway, "下载扩展包失败："+err.Error())
	}
	if int64(len(raw)) > maxBytes {
		return nil, statusErr(http.StatusBadRequest, fmt.Sprintf("扩展包超过上限 %d MB", maxBytes>>20))
	}
	if entry.Size > 0 && int64(len(raw)) != entry.Size {
		return nil, statusErr(http.StatusConflict, fmt.Sprintf("扩展包大小（%d 字节）和目录声明的 %d 字节不一致，内容可能被换过", len(raw), entry.Size))
	}
	return raw, nil
}

// checkDirectoryExtension 把下载到的包和登记的条目对上：身份 / 版本 / 摘要必须完全一致，
// 条目声明了签名者的，还要真的由那个签名者签过。返回读好的扩展文件和安装计划。
func (s *Server) checkDirectoryExtension(ctx context.Context, entry *store.DirectoryEntry, raw []byte) (map[string][]byte, *extPlan, error) {
	files, err := extension.ReadZip(raw)
	if err != nil {
		return nil, nil, statusErr(http.StatusBadRequest, "下载到的不是合法的扩展包："+err.Error())
	}
	p, err := s.planExtension(ctx, files, "")
	if err != nil {
		return nil, nil, err
	}
	if !p.Check.OK {
		return files, p, statusErr(http.StatusBadRequest, "扩展没有通过检查，改好后再装")
	}
	// 目录登记的是哪一份内容，就只装哪一份：身份（含版本）和摘要都要对得上。
	if want := entry.Name + "@" + entry.Version; p.Check.Identity != want {
		return files, p, statusErr(http.StatusConflict, fmt.Sprintf("目录登记的是 %s，下载到的包是 %s：版本对不上", want, p.Check.Identity))
	}
	if p.Check.Digest != entry.Digest {
		return files, p, statusErr(http.StatusConflict, "下载到的包内容和目录登记的摘要不一致，内容可能被换过")
	}
	// 目录里声明的签名者：登记时只能核对声明，现在有内容了，要求这个包确实由它签名。
	if entry.SignatureKeyID != "" {
		if p.Signature == nil || p.Signature.Status != extension.SignatureValid || p.Signature.KeyID != entry.SignatureKeyID {
			got := "没有可用的签名"
			status := extension.SignatureUnsigned
			if p.Signature != nil {
				status = p.Signature.Status
				got = "签名状态 " + status
				if p.Signature.KeyID != "" {
					got += "，签名者 " + p.Signature.KeyID
				}
			}
			return files, p, statusErr(http.StatusConflict, "目录声明这个版本由 "+entry.SignatureKeyID+" 签名，但下载到的包"+got+"：签名对不上，拒绝安装")
		}
	}
	return files, p, nil
}

// dirInstallExtension 安装 / 升级一条扩展条目：下载 → 核对 → 复用扩展安装计划 → 写库。
func (s *Server) dirInstallExtension(ctx context.Context, w http.ResponseWriter, r *http.Request, entry *store.DirectoryEntry, req *dirInstallReq, upgrade bool) {
	if entry.SourceType != directory.SourceTarball {
		dirFailPlan(w, http.StatusBadRequest, entry, upgrade, "扩展条目只支持从固定 tarball 地址安装；npm 来源的端侧扩展不在服务端安装", nil)
		return
	}
	raw, err := s.fetchDirectoryExtension(ctx, entry)
	if err != nil {
		var p *httpProblem
		if errors.As(err, &p) {
			dirFailPlan(w, p.status, entry, upgrade, p.msg, nil)
			return
		}
		s.internalErr(w, r, err)
		return
	}
	files, p, err := s.checkDirectoryExtension(ctx, entry, raw)
	if err != nil {
		var hp *httpProblem
		if errors.As(err, &hp) {
			dirFailPlan(w, hp.status, entry, upgrade, hp.msg, func(plan *dirInstallPlan) {
				plan.Extension = p
				if p != nil {
					plan.Sensitive = p.Sensitive
				}
			})
			return
		}
		s.internalErr(w, r, err)
		return
	}
	// 安装 / 升级的语义：install 只装没装过的，upgrade 只升已装过的。
	if upgrade {
		if p.Existing == nil {
			dirFailPlan(w, http.StatusConflict, entry, upgrade, "这个扩展还没装过，没有可升级的连接；请先用 install", func(plan *dirInstallPlan) { plan.Extension = p })
			return
		}
		if p.Existing.Version == entry.Version {
			dirFailPlan(w, http.StatusConflict, entry, upgrade, "已经装的就是 "+entry.Version+"，不需要升级", func(plan *dirInstallPlan) { plan.Extension = p })
			return
		}
		if registry.CompareVersions(entry.Version, p.Existing.Version) < 0 {
			dirFailPlan(w, http.StatusConflict, entry, upgrade, fmt.Sprintf("目录里的版本 %s 比已装的 %s 旧，不支持降级", entry.Version, p.Existing.Version), func(plan *dirInstallPlan) { plan.Extension = p })
			return
		}
	} else if p.Existing != nil {
		dirFailPlan(w, http.StatusConflict, entry, upgrade, "这个扩展已经装过了（连接 "+p.Existing.Slug+"）；升级请用 upgrade", func(plan *dirInstallPlan) { plan.Extension = p })
		return
	}
	// 只出计划：把检查结果、敏感项、要填的管理员参数 / 凭据交给网页，管理员确认后再执行。
	if req.PlanOnly {
		plan := dirInstallPlanOf(entry, upgrade)
		plan.OK = true
		plan.Extension = p
		plan.Sensitive = p.Sensitive
		plan.Message = "安装计划已生成；确认后才会下载并装成连接"
		writeDirectoryPlan(w, http.StatusOK, plan, true)
		return
	}
	if len(p.Sensitive) > 0 && !req.Confirm {
		dirFailPlan(w, http.StatusConflict, entry, upgrade, "这次升级有需要确认的变化："+strings.Join(p.Sensitive, "；"), func(plan *dirInstallPlan) {
			plan.Extension = p
			plan.Sensitive = p.Sensitive
		})
		return
	}
	installed := ""
	if p.Existing != nil {
		installed = p.Existing.Version
	}
	action := auditDirectoryInstall
	if upgrade {
		action = auditDirectoryUpgrade
	}
	in := &extInstallRun{
		req: extInstallReq{
			Slug: req.Slug, URL: req.URL, Auth: req.Auth, Secret: req.Secret, Admin: req.Admin,
			Scope: req.Scope, Confirm: true,
			OAuthClientID: req.OAuthClientID, OAuthClientSecret: req.OAuthClientSecret,
		},
		files: files, p: p,
		auditAction: action,
		auditTarget: dirEntryTarget(entry),
		auditExtra: map[string]any{
			"entry_id": entry.ID, "entry": dirEntryIdentity(entry), "kind": entry.Kind,
			"source": "tarball:" + entry.SourceURL, "entry_signature": entry.SignatureKeyID,
			"installed_version": installed,
		},
	}
	result, err := s.runExtInstall(ctx, r, in)
	if err != nil {
		var hp *httpProblem
		if errors.As(err, &hp) {
			dirFailPlan(w, hp.status, entry, upgrade, hp.msg, func(plan *dirInstallPlan) { plan.Extension = p })
			return
		}
		s.internalErr(w, r, err)
		return
	}
	plan := dirInstallPlanOf(entry, upgrade)
	plan.OK = true
	plan.Extension = p
	plan.Sensitive = p.Sensitive
	plan.Result = result
	status := http.StatusCreated
	if upgrade {
		status = http.StatusOK
	}
	writeDirectoryPlan(w, status, plan, false)
}

// ---- 插件 ----

// dirInstallPlugin 安装 / 升级一条插件条目：只认 npm 的固定包名 + 固定版本，
// 交给现有 registry 预载（pending 待审核），或核对企业源里已经有的那一份。
func (s *Server) dirInstallPlugin(ctx context.Context, w http.ResponseWriter, r *http.Request, entry *store.DirectoryEntry, req *dirInstallReq, upgrade bool) {
	// 插件条目最终要交给 npm 源预载。数据目录不可写时 newRegistry 返回 nil，服务仍会启动；
	// 这里先按 npm 源不可用回 503，避免下面 s.registry.StartSync 变成 nil 解引用。
	// 扩展条目走 dirInstallExtension，不经过这里，仍可安装 / 升级。
	if !s.registryReady(w) {
		return
	}
	planOnly := req.PlanOnly
	if entry.SourceType != directory.SourceNPM {
		dirFailPlan(w, http.StatusBadRequest, entry, upgrade, "插件条目只允许 npm 包名 + 固定版本，不支持 tarball / Git 安装", nil)
		return
	}
	plan := &dirPluginPlan{
		Name: entry.Name, Version: entry.Version, Channel: entry.Channel, Digest: entry.Digest,
		Source: entry.SourceType, Registry: entry.SourceRegistry,
	}
	fail := func(status int, msg string) {
		plan.Message = msg
		dirFailPlan(w, status, entry, upgrade, msg, func(p *dirInstallPlan) { p.Plugin = plan })
	}
	set, err := s.appSettings(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	base := registry.UpstreamBase(set)
	// 登记里写了来源 registry 的，只认当前企业上游：现有预载流程只从配置的上游取包，
	// 不能拿一个对不上的地址糊弄管理员。
	if entry.SourceRegistry != "" && entry.SourceRegistry != base {
		fail(http.StatusConflict, "条目登记的 registry 和当前企业上游不一致：请从企业上游同步，或先把上游配置成这个地址")
		return
	}
	// 企业源里已经有这个版本：核对摘要和状态，不再同步。
	v, err := s.store.GetVersion(ctx, entry.Name, entry.Version)
	switch {
	case err == nil:
		plan.ExistingVersion, plan.ExistingStatus = v.Version, v.Status
		if v.Integrity != entry.Digest {
			fail(http.StatusConflict, "企业源里这个版本的内容摘要和目录登记的不一致，拒绝安装（请发布同内容的版本，或更新目录）")
			return
		}
		switch v.Status {
		case store.StatusPublished:
			if upgrade {
				fail(http.StatusConflict, "企业源里已经是 "+entry.Version+" 了，没有可升级的版本")
				return
			}
			plan.Message = "这个版本已经在企业源里，无需再安装"
			plan.ReviewRequired = false
			out := dirInstallPlanOf(entry, upgrade)
			out.OK = true
			out.Plugin = plan
			out.Result = map[string]any{"status": "installed", "already": true, "version": v}
			writeDirectoryPlan(w, http.StatusOK, out, planOnly)
			return
		case store.StatusPending:
			plan.Message = "这个版本已经同步，等审核通过后才会对外可见"
			plan.ReviewRequired = true
			out := dirInstallPlanOf(entry, upgrade)
			out.OK = true
			out.Plugin = plan
			out.Result = map[string]any{"status": "pending", "review_required": true, "version": v}
			writeDirectoryPlan(w, http.StatusOK, out, planOnly)
			return
		case store.StatusRejected:
			fail(http.StatusConflict, "这个版本被审核驳回了，不能安装；换一个版本或重新发布")
			return
		default:
			fail(http.StatusConflict, "这个版本的状态是 "+v.Status+"，不能安装")
			return
		}
	case !errors.Is(err, store.ErrNotFound):
		s.internalErr(w, r, err)
		return
	}
	// 企业源里没有这个版本：升级要求这个包之前装过，而且不能往旧版本「升级」。
	if upgrade {
		pkg, perr := s.store.GetPackage(ctx, entry.Name)
		if perr != nil || pkg == nil {
			fail(http.StatusConflict, "企业源里还没有这个插件，没有可升级的版本；请先用 install")
			return
		}
		versions, verr := s.store.ListVersions(ctx, entry.Name)
		if verr != nil {
			s.internalErr(w, r, verr)
			return
		}
		latest := ""
		for _, lv := range versions {
			if lv.Status == store.StatusRejected {
				continue
			}
			if latest == "" || registry.CompareVersions(lv.Version, latest) > 0 {
				latest = lv.Version
			}
		}
		if latest != "" && registry.CompareVersions(entry.Version, latest) < 0 {
			fail(http.StatusConflict, fmt.Sprintf("目录里的版本 %s 比企业源里最新的 %s 旧，不支持降级", entry.Version, latest))
			return
		}
	}
	if base == "" {
		fail(http.StatusConflict, "当前是离线模式或没有配置上游 registry，不能在线安装插件；请用离线导入")
		return
	}
	// 企业作用域的包只在企业源发布，不去上游找（防依赖混淆）；要装新版本请先在企业源发布。
	if registry.InScopes(entry.Name, set.CompanyScopes()) {
		fail(http.StatusConflict, "企业作用域的包只能在企业源发布，不能从上游同步；请先把这个版本发布到企业源")
		return
	}
	plan.SyncVersion = entry.Version
	plan.ReviewRequired = registry.PluginNeedsReview(set)
	next := "按企业准入策略处理"
	if plan.ReviewRequired {
		next = "还要在审核页通过才对外可见"
	}
	if planOnly {
		plan.Message = "将同步 " + entry.Name + "@" + entry.Version + "；同步完成后" + next
		out := dirInstallPlanOf(entry, upgrade)
		out.OK = true
		out.Plugin = plan
		out.Result = map[string]any{"status": "planned", "review_required": plan.ReviewRequired}
		writeDirectoryPlan(w, http.StatusOK, out, true)
		return
	}
	by := int64(0)
	if u := currentUser(r); u != nil {
		by = u.ID
	}
	// 固定版本 + 固定摘要：同步时按目录登记的 integrity 核对上游内容，对不上就失败、不落库。
	// RootStatus 只给 pending：最多是待审核，绝不在这里直接上架。
	j, err := s.registry.StartSync(ctx, "directory", registry.SyncParams{
		Roots:      []registry.SyncRoot{{Name: entry.Name, Version: entry.Version, Integrity: entry.Digest}},
		RootStatus: store.StatusPending,
	}, by)
	if err != nil {
		s.registryErr(w, r, err)
		return
	}
	plan.Message = "已开始同步 " + entry.Name + "@" + entry.Version + "；同步完成后" + next
	action := auditDirectoryInstall
	if upgrade {
		action = auditDirectoryUpgrade
	}
	s.audit(r, action, dirEntryTarget(entry), map[string]any{
		"entry_id": entry.ID, "entry": dirEntryIdentity(entry), "kind": entry.Kind,
		"source": "npm:" + entry.Name + "@" + entry.Version, "digest": entry.Digest,
		"job": j.ID, "review_required": plan.ReviewRequired, "upgrade": upgrade,
	})
	out := dirInstallPlanOf(entry, upgrade)
	out.OK = true
	out.Plugin = plan
	out.Result = map[string]any{"status": "syncing", "job": j, "review_required": plan.ReviewRequired}
	writeDirectoryPlan(w, http.StatusAccepted, out, false)
}
