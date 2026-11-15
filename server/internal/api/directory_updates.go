package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业目录的更新检查（docs/plugin-signing.md §8）：把登记过的目录条目和「已经装在 dsh-ee 里
// 的版本」对一遍，告诉管理员哪些条目有更新、哪些已经是最新、哪些还没装、哪些看不出来。
//
// 这是**只读**接口：不下载、不安装、不升级、不改登记表。真正的安装 / 升级仍走现有流程，
// 由管理员显式触发。auto_upgrade 本期也只在目录条目和设置里登记，接口只展示、不执行。
//
// 「已装版本」的口径（服务端能看到的部分，docs 里同样写明）：
//   - 扩展（kind=extension）：扩展装成的连接上、当前生效版本里的 extension.version；
//   - 插件（kind=dsh-plugin）：企业 npm 源里这个包**已上架**的最高版本（员工能装到的版本）。
//     员工 DSH 端真正装了哪个版本服务端看不到（只有下载记录），所以插件这一侧是近似，不是
//     终端清点；没有已上架版本但有包时给「未知」，连包都没有就是「未安装」。
const (
	// 目录目标版本比已装版本新：可以升级
	dirUpdatesAvailable = "available"
	// 已装版本等于或高于目录目标：不需要更新
	dirUpdatesCurrent = "current"
	// 还没有装（扩展没有连接；企业源里没有这个插件）
	dirUpdatesNotInstalled = "not_installed"
	// 有安装痕迹但看不出当前版本（连接/清单读不出来、企业源里没有已上架版本等）
	dirUpdatesUnknown = "unknown"
)

// dirUpdatesConn 是扩展条目对应的已装连接。只给展示，不做权限判断。
type dirUpdatesConn struct {
	ID      int64  `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// dirUpdatesItem 是一条目录条目的更新检查结果。
type dirUpdatesItem struct {
	EntryID     int64  `json:"entry_id"`
	DirectoryID string `json:"directory_id"`
	// 条目类型：extension 或 dsh-plugin
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Channel string `json:"channel"`
	// 目录里登记的版本（目标版本）
	Target string `json:"target"`
	// 当前已装版本；没装或看不出来时为空
	Current string `json:"current,omitempty"`
	// 当前版本从哪来：connection（扩展连接）或 registry（企业 npm 源）
	CurrentSource string `json:"current_source,omitempty"`
	// available / current / not_installed / unknown
	Status string `json:"status"`
	// 扩展：装成的连接
	InstalledConnection *dirUpdatesConn `json:"installed_connection,omitempty"`
	// 条目内嵌签名声明的状态（none / declared + 按当前信任根重算的 key_state）；
	// 目录里没有内容，所以 verified 恒为 false，安装时再核对真签名
	Signature *directoryEntrySigView `json:"signature"`
	// 条目声明的 auto_upgrade（本期只展示，不执行）
	AutoUpgrade bool `json:"auto_upgrade"`
	// 给人看的一句话：当前是什么情况、为什么是这个结论
	Reason string `json:"reason,omitempty"`
}

// dirUpdatesSummary 是各状态的条数，方便管理页直接显示。
type dirUpdatesSummary struct {
	Total        int `json:"total"`
	Available    int `json:"available"`
	Current      int `json:"current"`
	NotInstalled int `json:"not_installed"`
	Unknown      int `json:"unknown"`
}

// dirUpdatesSettings 是和更新检查相关的部署设置。本期只读不执行。
type dirUpdatesSettings struct {
	// 设置 directory.autoUpgrade（默认 false）。打开也不会自动升级，本期只登记 / 展示。
	AutoUpgrade bool `json:"auto_upgrade"`
}

// dirUpdatesView 是 GET /api/admin/directory/updates 的返回。
type dirUpdatesView struct {
	Settings dirUpdatesSettings `json:"settings"`
	Summary  dirUpdatesSummary  `json:"summary"`
	Entries  []dirUpdatesItem   `json:"entries"`
}

// dirInstalledExtension 是扩展在当前部署里装成的连接；一个扩展只取最早建的那个连接。
type dirInstalledExtension struct {
	AssetID int64
	Slug    string
	Title   string
	Version string
}

// directoryInstalledExtensions 一次取出所有扩展装成的连接（和 findExtension 同口径：
// 只看连接当前生效的已上架版本），按扩展 ID 建索引。同一种扩展有多个连接时取 id 最小的，
// 保证每次结果稳定；别的连接不在这里清点。
func (s *Server) directoryInstalledExtensions(ctx context.Context) (map[string]*dirInstalledExtension, error) {
	conns, err := s.store.ListAssets(ctx, store.KindConnection)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(conns))
	for _, a := range conns {
		ids = append(ids, a.ID)
	}
	live, err := s.store.LiveAssetVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := map[string]*dirInstalledExtension{}
	for _, a := range conns {
		lv := live[a.ID]
		if lv == nil {
			continue
		}
		sp := parseSpec(lv.Spec)
		if sp.Extension == nil || sp.Extension.ID == "" {
			continue
		}
		if _, seen := out[sp.Extension.ID]; seen {
			continue
		}
		out[sp.Extension.ID] = &dirInstalledExtension{AssetID: a.ID, Slug: a.Slug, Title: a.Title, Version: sp.Extension.Version}
	}
	return out, nil
}

// directoryTrustedKeyStates 把受信任的目录签名公钥换算成 key id → 状态（active / retired /
// revoked）。环境变量来源同样是 active。和验签用的是同一份合并结果，保证展示和校验一致。
func (s *Server) directoryTrustedKeyStates(ctx context.Context) (map[string]string, error) {
	keys, err := s.trustedDirectoryKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if len(k.Public) != 32 { // ed25519.PublicKeySize
			continue
		}
		state := k.State
		if state == "" {
			state = extension.KeyStateActive
		}
		out[extension.KeyID(k.Public)] = state
	}
	return out, nil
}

// directoryEntrySignatureView 按当前信任根给条目的签名声明造展示状态。
// 登记表里也存了 apply 时的 key_state，但信任根可能已经换下 / 撤销，所以这里重算一次。
func directoryEntrySignatureView(row store.DirectoryEntry, keyStates map[string]string) *directoryEntrySigView {
	if row.SignatureKeyID == "" {
		return &directoryEntrySigView{Status: directory.EntrySignatureNone}
	}
	state := keyStates[row.SignatureKeyID]
	if state == "" {
		state = "unknown"
	}
	return &directoryEntrySigView{Status: directory.EntrySignatureDeclared, KeyID: row.SignatureKeyID, KeyState: state}
}

// highestVersion 取版本列表里最高的一个（按 semver；非法版本排在最前，不会盖过一个合法版本）。
func highestVersion(versions []*store.NpmVersion) string {
	best := ""
	for _, v := range versions {
		if v == nil || v.Version == "" {
			continue
		}
		if best == "" || registry.CompareVersions(v.Version, best) > 0 {
			best = v.Version
		}
	}
	return best
}

// directoryUpdateOf 检查一条目录条目，返回状态和给人看的原因。只读，不联网。
func (s *Server) directoryUpdateOf(ctx context.Context, row store.DirectoryEntry, installed map[string]*dirInstalledExtension, keyStates map[string]string) (dirUpdatesItem, error) {
	item := dirUpdatesItem{
		EntryID: row.ID, DirectoryID: row.DirectoryID, Kind: row.Kind, Name: row.Name,
		Channel: row.Channel, Target: row.Version, AutoUpgrade: row.AutoUpgrade,
		Signature: directoryEntrySignatureView(row, keyStates),
	}
	setStatus := func(status, reason string) dirUpdatesItem {
		item.Status = status
		item.Reason = reason
		return item
	}
	switch row.Kind {
	case directory.KindExtension:
		inst := installed[row.Name]
		if inst == nil {
			return setStatus(dirUpdatesNotInstalled, "还没有装这个扩展的连接：扩展条目要先安装才会出现在这里"), nil
		}
		item.Current, item.CurrentSource = inst.Version, "connection"
		item.InstalledConnection = &dirUpdatesConn{ID: inst.AssetID, Slug: inst.Slug, Title: inst.Title, Version: inst.Version}
		if !extension.ValidVersion(inst.Version) {
			return setStatus(dirUpdatesUnknown, "已装连接上的扩展版本「"+inst.Version+"」不是语义化版本，无法和目录版本比较"), nil
		}
		if registry.CompareVersions(row.Version, inst.Version) > 0 {
			return setStatus(dirUpdatesAvailable, "连接里生效的是 "+inst.Version+"，目录登记的是 "+row.Version), nil
		}
		if registry.CompareVersions(row.Version, inst.Version) == 0 {
			return setStatus(dirUpdatesCurrent, "连接里生效的就是目录登记的版本"), nil
		}
		return setStatus(dirUpdatesCurrent, "连接里生效的 "+inst.Version+" 比目录登记的 "+row.Version+" 新，不需要更新"), nil

	case directory.KindPlugin:
		if _, err := s.store.GetPackage(ctx, row.Name); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return setStatus(dirUpdatesNotInstalled, "企业 npm 源里还没有这个包：目录条目还没同步 / 发布过"), nil
			}
			return item, err
		}
		versions, err := s.store.ListVersions(ctx, row.Name, store.StatusPublished)
		if err != nil {
			return item, err
		}
		current := highestVersion(versions)
		if current == "" {
			return setStatus(dirUpdatesUnknown, "企业源里有这个包，但还没有已上架的版本（可能还在等审核），看不出当前可装版本"), nil
		}
		item.Current, item.CurrentSource = current, "registry"
		if !extension.ValidVersion(current) {
			return setStatus(dirUpdatesUnknown, "企业源里已上架的版本「"+current+"」不是语义化版本，无法和目录版本比较"), nil
		}
		if registry.CompareVersions(row.Version, current) > 0 {
			return setStatus(dirUpdatesAvailable, "企业源里已上架的最高版本是 "+current+"，目录登记的是 "+row.Version+"；发布/审核通过后才装得到"), nil
		}
		if registry.CompareVersions(row.Version, current) == 0 {
			return setStatus(dirUpdatesCurrent, "企业源里已上架的最高版本就是目录登记的版本"), nil
		}
		return setStatus(dirUpdatesCurrent, "企业源里已上架的 "+current+" 比目录登记的 "+row.Version+" 新，不需要更新"), nil
	}
	return setStatus(dirUpdatesUnknown, "不认识的条目类型「"+row.Kind+"」：只有 extension 和 dsh-plugin 能检查更新"), nil
}

// GET /api/admin/directory/updates：把登记过的目录条目和已安装的插件/扩展版本对比一遍。
// 只读：不装、不升、不改登记表；auto_upgrade 本期只展示、不执行。
func (s *Server) handleDirectoryUpdates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.store.ListDirectoryEntries(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	set, err := s.appSettings(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	installed, err := s.directoryInstalledExtensions(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	keyStates, err := s.directoryTrustedKeyStates(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := dirUpdatesView{
		Settings: dirUpdatesSettings{AutoUpgrade: set.Directory.AutoUpgrade},
		Entries:  make([]dirUpdatesItem, 0, len(rows)),
	}
	for _, row := range rows {
		item, err := s.directoryUpdateOf(ctx, row, installed, keyStates)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		out.Summary.Total++
		switch item.Status {
		case dirUpdatesAvailable:
			out.Summary.Available++
		case dirUpdatesCurrent:
			out.Summary.Current++
		case dirUpdatesNotInstalled:
			out.Summary.NotInstalled++
		default:
			out.Summary.Unknown++
		}
		out.Entries = append(out.Entries, item)
	}
	writeJSON(w, http.StatusOK, out)
}
