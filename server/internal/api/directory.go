package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 企业扩展目录 v1（docs/plugin-signing.md §2.2、roadmap-plugin-distribution.md §3）：
// 管理员把一份整体签过名的 dsh-ee-directory.json 贴进来，dsh-ee 用受信任的签名公钥验签、
// 逐条校验，然后把条目登记到 directory_entries 表。**只登记，不安装、不升级、不回滚**：
// 真正装扩展/插件仍走现有安装流程（安装计划、审核、#134 的扩展验签）。
//
// 目录是一份「可引用、可缓存、可审计的来源清单」，不是新的市场，也不等于审核通过。

const (
	auditDirectoryApply = "directory.apply"
	// 请求体上限：目录文档本身 2 MiB（directory.MaxDocumentBytes），JSON 字符串转义再留一倍。
	maxDirectoryRequest = 2 * directory.MaxDocumentBytes
)

// directoryDocReq 是 check / apply 的请求体：目录 JSON 原文 + apply 的确认开关。
type directoryDocReq struct {
	// 目录 JSON 原文（粘贴 dsh-ee-directory.json 的内容）
	Document string `json:"document"`
	// apply 时有敏感变化（移除已登记条目、同版本换内容/来源）要传 true；check 不用
	Confirm bool `json:"confirm"`
}

// directoryDocView 是目录头的展示信息 + 整体验签结果。
type directoryDocView struct {
	Format         int                     `json:"format"`
	DirectoryID    string                  `json:"directory_id"`
	Title          extension.Text          `json:"title,omitempty"`
	GeneratedAt    string                  `json:"generated_at,omitempty"`
	Issuer         directory.Issuer        `json:"issuer"`
	EntriesCount   int                     `json:"entries_count"`
	DocumentDigest string                  `json:"document_digest"`
	Signature      *directory.VerifyResult `json:"signature"`
}

// directoryEntryPlan 是单条条目的展示信息 + 它自己的校验结果。
type directoryEntryPlan struct {
	Index      int                  `json:"index"`
	ID         string               `json:"id"` // kind:name@version/channel，给人看
	Kind       string               `json:"kind"`
	Name       string               `json:"name"`
	Version    string               `json:"version"`
	Channel    string               `json:"channel"`
	Digest     string               `json:"digest"`
	Source     directory.Source     `json:"source"`
	Size       int64                `json:"size,omitempty"`
	Publisher  *directory.Publisher `json:"publisher,omitempty"`
	Notes      string               `json:"notes,omitempty"`
	MinVersion string               `json:"min_version,omitempty"`
	// 条目声明的 auto_upgrade（format 2 additive）；本期只登记，不执行自动升级
	AutoUpgrade bool                   `json:"auto_upgrade"`
	Signature   *directoryEntrySigView `json:"signature,omitempty"`
	Issues      []directory.Issue      `json:"issues"`
}

// directoryEntrySigView 是条目内嵌签名声明的状态。目录里没有内容，所以只能核对声明和签名者
// 是否受信任；verified 恒为 false，安装时再按 #134 的扩展验签核对真签名。
type directoryEntrySigView struct {
	Status   string `json:"status"`
	KeyID    string `json:"key_id,omitempty"`
	KeyState string `json:"key_state,omitempty"`
	Verified bool   `json:"verified"`
}

// directoryExistingView 是一份目录已经登记过的情况。
type directoryExistingView struct {
	EntriesCount   int       `json:"entries_count"`
	DocumentDigest string    `json:"document_digest,omitempty"`
	AppliedAt      time.Time `json:"applied_at,omitempty"`
	AppliedBy      *int64    `json:"applied_by,omitempty"`
}

// directoryDiff 是「这份目录 vs 已登记内容」的差异：apply 会改什么先给管理员看清楚。
type directoryDiff struct {
	Created           int      `json:"created"`
	Updated           int      `json:"updated"`
	Unchanged         int      `json:"unchanged"`
	Removed           int      `json:"removed"`
	CreatedIdentities []string `json:"created_identities,omitempty"`
	UpdatedIdentities []string `json:"updated_identities,omitempty"`
	RemovedIdentities []string `json:"removed_identities,omitempty"`
}

// directoryPlan 是 check 的返回，也是 apply 内部复用的结果。
type directoryPlan struct {
	OK       bool                   `json:"ok"`
	Errors   int                    `json:"errors"`
	Warnings int                    `json:"warnings"`
	Document directoryDocView       `json:"document"`
	Entries  []directoryEntryPlan   `json:"entries"`
	Issues   []directory.Issue      `json:"issues"`
	Existing *directoryExistingView `json:"existing,omitempty"`
	Diff     *directoryDiff         `json:"diff,omitempty"`
	// apply 前要管理员确认的敏感变化（移除条目、同版本换内容/换来源）
	Sensitive []string `json:"sensitive"`
}

// entryIdentity 是条目在展示/差异比较里用的标识。
func entryIdentity(kind, name, version, channel string) string {
	return kind + ":" + name + "@" + version + "/" + channel
}

// docEntryIdentity 返回目录条目的标识。
func docEntryIdentity(e directory.Entry) string {
	return entryIdentity(e.Kind, e.Name, e.Version, e.Channel)
}

// directoryPlanOf 解析后的目录 + 信任根 → 校验结果与将要登记的条目。
func (s *Server) directoryPlanOf(ctx context.Context, doc *directory.Document) (*directoryPlan, []store.DirectoryEntry, error) {
	trusted, err := s.trustedDirectoryKeys(ctx)
	if err != nil {
		return nil, nil, err
	}
	verify := directory.VerifyDocument(doc, trusted)
	validate := directory.Validate(doc, directory.Options{
		TarballHosts: s.opts.DirectoryTarballHosts,
		TrustedKeys:  trusted,
	})
	issues := make([]directory.Issue, 0, len(validate.Issues)+1)
	issues = append(issues, validate.Issues...)
	errorsN, warningsN := validate.Errors, validate.Warnings

	// 目录整体签名是硬门槛：unsigned / invalid / revoked 都不能登记；retired 仍认但提示。
	addSignatureIssue := func(sev, code, msg string) {
		issues = append(issues, directory.Issue{Severity: sev, Code: code, Field: "signature", Index: -1, Message: msg})
		if sev == "error" {
			errorsN++
		} else {
			warningsN++
		}
	}
	switch verify.Status {
	case directory.SignatureValid:
		if verify.KeyState == extension.KeyStateRetired {
			addSignatureIssue("warning", "signature.retired", verify.Message)
		}
	case directory.SignatureUnsigned:
		addSignatureIssue("error", "signature.required", verify.Message)
	case directory.SignatureRevoked:
		addSignatureIssue("error", "signature.revoked", verify.Message)
	default:
		addSignatureIssue("error", "signature.invalid", verify.Message)
	}

	plan := &directoryPlan{
		OK:       errorsN == 0,
		Errors:   errorsN,
		Warnings: warningsN,
		Issues:   issues,
		Entries:  make([]directoryEntryPlan, 0, len(doc.Entries)),
		Document: directoryDocView{
			Format:         doc.Format,
			DirectoryID:    doc.DirectoryID,
			Title:          doc.Title,
			GeneratedAt:    doc.GeneratedAt,
			Issuer:         doc.Issuer,
			EntriesCount:   len(doc.Entries),
			DocumentDigest: verify.Digest,
			Signature:      verify,
		},
		Sensitive: []string{},
	}
	byIndex := map[int][]directory.Issue{}
	for _, is := range issues {
		if is.Index >= 0 {
			byIndex[is.Index] = append(byIndex[is.Index], is)
		}
	}
	entries := make([]store.DirectoryEntry, 0, len(doc.Entries))
	for i := range doc.Entries {
		e := doc.Entries[i]
		er := validate.Entries[i]
		view := directoryEntryPlan{
			Index: i, ID: docEntryIdentity(e), Kind: e.Kind, Name: e.Name, Version: e.Version, Channel: e.Channel,
			Digest: e.Digest, Source: e.Source, Size: e.Size, Publisher: e.Publisher, Notes: e.Notes, MinVersion: e.MinVersion,
			AutoUpgrade: e.AutoUpgrade, Issues: byIndex[i],
		}
		if view.Issues == nil {
			view.Issues = []directory.Issue{}
		}
		if e.Signature != nil {
			view.Signature = &directoryEntrySigView{Status: er.SignatureStatus, KeyID: er.SignatureKeyID, KeyState: er.SignatureKeyState}
		}
		plan.Entries = append(plan.Entries, view)
		entries = append(entries, directoryStoreEntry(doc.DirectoryID, e, er))
	}
	if doc.DirectoryID != "" && len(doc.DirectoryID) <= 64 {
		existing, err := s.store.ListDirectoryEntriesOf(ctx, doc.DirectoryID)
		if err != nil {
			return nil, nil, err
		}
		if len(existing) > 0 {
			diff, sensitive := diffDirectoryEntries(existing, doc.Entries)
			if len(sensitive) > 0 {
				plan.Sensitive = sensitive
			}
			plan.Diff = diff
			latest := existing[0]
			for _, row := range existing[1:] {
				if row.AppliedAt.After(latest.AppliedAt) {
					latest = row
				}
			}
			plan.Existing = &directoryExistingView{EntriesCount: len(existing), DocumentDigest: latest.DocumentDigest, AppliedAt: latest.AppliedAt, AppliedBy: latest.AppliedBy}
		} else {
			plan.Diff = &directoryDiff{}
		}
	}
	return plan, entries, nil
}

// directoryStoreEntry 把目录条目转成登记表的行。
func directoryStoreEntry(directoryID string, e directory.Entry, er directory.EntryResult) store.DirectoryEntry {
	row := store.DirectoryEntry{
		DirectoryID:       directoryID,
		Kind:              e.Kind,
		Name:              e.Name,
		Version:           e.Version,
		Channel:           e.Channel,
		Digest:            e.Digest,
		SourceType:        e.Source.Type,
		SourceRegistry:    e.Source.Registry,
		SourceURL:         e.Source.URL,
		Size:              e.Size,
		AutoUpgrade:       e.AutoUpgrade,
		Notes:             e.Notes,
		SignatureKeyID:    er.SignatureKeyID,
		SignatureKeyState: er.SignatureKeyState,
	}
	if e.Publisher != nil {
		row.PublisherUsername = e.Publisher.Username
		row.PublisherDept = e.Publisher.Department
		if e.Publisher.UserID > 0 {
			id := e.Publisher.UserID
			row.PublisherUserID = &id
		}
	}
	if e.Signature != nil {
		if raw, err := json.Marshal(e.Signature); err == nil {
			row.SignatureJSON = string(raw)
		}
	}
	return row
}

// diffDirectoryEntries 比较「已登记」和「这次目录里的」条目，给 check 展示、给 apply 判断
// 要不要 confirm。返回的 sensitive 是需要管理员确认的变化：移除已登记条目、同版本换内容/换来源。
func diffDirectoryEntries(existing []store.DirectoryEntry, docEntries []directory.Entry) (*directoryDiff, []string) {
	diff := &directoryDiff{}
	sensitive := []string{}
	old := map[string]store.DirectoryEntry{}
	for _, row := range existing {
		old[entryIdentity(row.Kind, row.Name, row.Version, row.Channel)] = row
	}
	seen := map[string]bool{}
	for _, e := range docEntries {
		id := docEntryIdentity(e)
		if seen[id] {
			continue
		}
		seen[id] = true
		row, ok := old[id]
		if !ok {
			diff.Created++
			diff.CreatedIdentities = append(diff.CreatedIdentities, id)
			continue
		}
		if directoryEntrySame(row, e) {
			diff.Unchanged++
			continue
		}
		diff.Updated++
		diff.UpdatedIdentities = append(diff.UpdatedIdentities, id)
		if row.Digest != e.Digest {
			sensitive = append(sensitive, "同版本换内容："+id+" 的摘要从 "+shortDigest(row.Digest)+" 变成 "+shortDigest(e.Digest))
		}
		if row.SourceType != e.Source.Type || row.SourceRegistry != e.Source.Registry || row.SourceURL != e.Source.URL {
			sensitive = append(sensitive, "换来源："+id+" 的来源变了")
		}
	}
	removed := make([]string, 0, len(old))
	for id := range old {
		if !seen[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	for _, id := range removed {
		diff.Removed++
		diff.RemovedIdentities = append(diff.RemovedIdentities, id)
		sensitive = append(sensitive, "移除已登记条目："+id)
	}
	return diff, sensitive
}

// directoryEntrySame 比较已登记条目和目录条目是否一致（不含 document_digest/applied_*，
// 目录里加一条别的条目会改变整份文档摘要，但旧条目本身没变）。
func directoryEntrySame(row store.DirectoryEntry, e directory.Entry) bool {
	publisherUser := int64(0)
	publisherName, publisherDept := "", ""
	if e.Publisher != nil {
		publisherUser, publisherName, publisherDept = e.Publisher.UserID, e.Publisher.Username, e.Publisher.Department
	}
	rowPublisherUser := int64(0)
	if row.PublisherUserID != nil {
		rowPublisherUser = *row.PublisherUserID
	}
	// 签名者状态（active/retired/revoked/unknown）由信任根决定，不是条目内容：换钥匙后
	// 重新 apply 同一份目录不应算作条目变化。
	sigKeyID, sigJSON := "", ""
	if e.Signature != nil {
		sigKeyID = e.Signature.KeyID
		if raw, err := json.Marshal(e.Signature); err == nil {
			sigJSON = string(raw)
		}
	}
	return row.Kind == e.Kind && row.Name == e.Name && row.Version == e.Version && row.Channel == e.Channel &&
		row.Digest == e.Digest && row.SourceType == e.Source.Type && row.SourceRegistry == e.Source.Registry &&
		row.SourceURL == e.Source.URL && row.Size == e.Size && row.AutoUpgrade == e.AutoUpgrade &&
		rowPublisherUser == publisherUser && row.PublisherUsername == publisherName && row.PublisherDept == publisherDept &&
		row.Notes == e.Notes && row.SignatureKeyID == sigKeyID && row.SignatureJSON == sigJSON
}

func shortDigest(digest string) string {
	if len(digest) > 16 {
		return digest[:16] + "…"
	}
	return digest
}

// readDirectoryDoc 读请求里的目录文档：只支持粘贴 JSON 文本，上传由前端读文件后放进来。
func readDirectoryDoc(w http.ResponseWriter, r *http.Request) (*directory.Document, *directoryDocReq, bool) {
	var req directoryDocReq
	if !readJSONLimit(w, r, &req, maxDirectoryRequest) {
		return nil, nil, false
	}
	doc, err := directory.Parse([]byte(req.Document))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return nil, nil, false
	}
	return doc, &req, true
}

// POST /api/admin/directory/check：验签 + 逐条校验，返回计划；不落库、不安装。
func (s *Server) handleDirectoryCheck(w http.ResponseWriter, r *http.Request) {
	doc, _, ok := readDirectoryDoc(w, r)
	if !ok {
		return
	}
	plan, _, err := s.directoryPlanOf(r.Context(), doc)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// POST /api/admin/directory/apply：校验通过（有敏感变化时还要 confirm）后登记到 directory_entries。
// 只登记，不自动安装；同一个 directory_id 覆盖登记（目录里删掉/改掉的条目同步）。
func (s *Server) handleDirectoryApply(w http.ResponseWriter, r *http.Request) {
	doc, req, ok := readDirectoryDoc(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	plan, entries, err := s.directoryPlanOf(ctx, doc)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !plan.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "目录没有通过校验，改好后再登记", "plan": plan})
		return
	}
	if len(plan.Sensitive) > 0 && !req.Confirm {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "这次登记有需要确认的变化：" + strings.Join(plan.Sensitive, "；"),
			"plan":  plan,
		})
		return
	}
	user := currentUser(r)
	var appliedBy int64
	if user != nil {
		appliedBy = user.ID
	}
	res, err := s.store.ApplyDirectoryEntries(ctx, doc.DirectoryID, plan.Document.DocumentDigest, appliedBy, entries)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "这份目录的条目正在被另一个请求登记，稍后再试")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	stored, err := s.store.ListDirectoryEntriesOf(ctx, doc.DirectoryID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	detail := map[string]any{
		"directory_id":    doc.DirectoryID,
		"document_digest": plan.Document.DocumentDigest,
		"signature":       signatureAuditDir(plan.Document.Signature),
		"created":         res.Created,
		"updated":         res.Updated,
		"unchanged":       res.Unchanged,
		"removed":         res.Removed,
		"confirmed":       plan.Sensitive,
	}
	s.audit(r, auditDirectoryApply, "directory:"+doc.DirectoryID, detail)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"directory_id":    doc.DirectoryID,
		"document_digest": plan.Document.DocumentDigest,
		"signature":       plan.Document.Signature,
		"result":          res,
		"confirmed":       plan.Sensitive,
		"entries":         stored,
	})
}

// signatureAuditDir 是目录签名写进审计的形状：状态 + key id，不含签名内容。
func signatureAuditDir(v *directory.VerifyResult) string {
	if v == nil {
		return ""
	}
	if v.KeyID == "" {
		return v.Status
	}
	return v.Status + ":" + v.KeyID
}

// directoryEntryView 是登记条目 + 安装状态，给管理页展示「是否已安装 / 可升级」。
// 只给展示用，真正的安装 / 升级仍由 install / upgrade 接口再校验一次。
type directoryEntryView struct {
	store.DirectoryEntry
	// 已装版本：扩展是连接上生效的版本；插件是企业源里这个版本（或本地最新版本）
	InstalledVersion string `json:"installed_version,omitempty"`
	// 插件：企业源里那个版本的状态（published / pending / rejected / unpublished）
	InstalledStatus string `json:"installed_status,omitempty"`
	// 扩展：装成的连接标识
	InstalledConnection string `json:"installed_connection,omitempty"`
	// 目录登记的版本已经在企业里（扩展：连接上就是这一版；插件：企业源里就是这个版本）
	Installed bool `json:"installed"`
	// 目录里的版本比企业里现有的新，可以升级
	UpgradeAvailable bool `json:"upgrade_available"`
}

// directoryEntryViews 给每条登记条目补上安装状态。扩展按连接查（一种扩展只有一个连接），
// 插件按企业源里已有的版本查；查询失败时不挡住列表，只少给状态。
func (s *Server) directoryEntryViews(ctx context.Context, entries []store.DirectoryEntry) []directoryEntryView {
	out := make([]directoryEntryView, 0, len(entries))
	for _, e := range entries {
		v := directoryEntryView{DirectoryEntry: e}
		switch e.Kind {
		case directory.KindExtension:
			a, sp, err := s.findExtension(ctx, e.Name)
			if err != nil || a == nil || sp == nil || sp.Extension == nil {
				break
			}
			v.InstalledVersion = sp.Extension.Version
			v.InstalledConnection = a.Slug
			v.Installed = v.InstalledVersion == e.Version
			v.UpgradeAvailable = registry.CompareVersions(e.Version, v.InstalledVersion) > 0
		case directory.KindPlugin:
			if local, err := s.store.GetVersion(ctx, e.Name, e.Version); err == nil {
				// 插件「安装了」= 企业源里这个版本已上架；同步了但还在审核的显示为待审核
				v.InstalledVersion, v.InstalledStatus = local.Version, local.Status
				v.Installed = local.Status == store.StatusPublished
				break
			}
			// 企业源里没有这个版本：拿本地最新版本给管理员做「可升级」判断
			versions, err := s.store.ListVersions(ctx, e.Name)
			if err != nil {
				break
			}
			latest := ""
			for _, lv := range versions {
				if lv.Status == store.StatusRejected {
					continue
				}
				if latest == "" || registry.CompareVersions(lv.Version, latest) > 0 {
					latest, v.InstalledStatus = lv.Version, lv.Status
				}
			}
			if latest != "" {
				v.InstalledVersion = latest
				v.UpgradeAvailable = registry.CompareVersions(e.Version, latest) > 0
			}
		}
		out = append(out, v)
	}
	return out
}

// GET /api/admin/directory：登记过的目录与条目。只读。
// 权限：管连接的（看扩展 / 登记目录）或管插件的（看插件条目）都能看，写操作仍按类型校验。
func (s *Server) handleListDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	entries, err := s.store.ListDirectoryEntries(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	summaries, err := s.store.ListDirectorySummaries(ctx)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	views := s.directoryEntryViews(ctx, entries)
	// 没配白名单时给空数组而不是 null：网页直接读 .length，null 会让页面崩掉。
	hosts := s.opts.DirectoryTarballHosts
	if hosts == nil {
		hosts = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"directories": summaries,
		"entries":     views,
		"count":       len(views),
		// 当前 tarball 策略：空表示只接受公网 https 地址；有值表示只接受名单里的主机。
		// 安装扩展条目时会按同一策略再核对一次，这里给管理员/网页展示用。
		"tarball_hosts": hosts,
	})
}
