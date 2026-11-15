package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

// DirectoryEntry 是登记过的一条企业目录条目（migrations/0034_directory_entries.sql）。
// 它只是目录声明，不是安装结果：真正的安装仍走扩展/插件的安装流程。
type DirectoryEntry struct {
	ID          int64  `json:"id"`
	DirectoryID string `json:"directory_id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Digest      string `json:"digest"`
	SourceType  string `json:"source_type"`
	// npm 来源可选的 registry 地址
	SourceRegistry string `json:"source_registry,omitempty"`
	// tarball 来源的固定地址
	SourceURL string `json:"source_url,omitempty"`
	// 条目声明的 tarball 大小（字节），0 表示没声明
	Size int64 `json:"size,omitempty"`
	// 条目声明的 auto_upgrade（format 2 additive，缺省 false）。本期只登记 / 只读展示，
	// 不执行自动升级；部署总开关是设置 directory.autoUpgrade（默认 false）。
	AutoUpgrade       bool   `json:"auto_upgrade"`
	PublisherUserID   *int64 `json:"publisher_user_id,omitempty"`
	PublisherUsername string `json:"publisher_username,omitempty"`
	PublisherDept     string `json:"publisher_department,omitempty"`
	Notes             string `json:"notes,omitempty"`
	// 条目内嵌的签名声明：key id、公钥状态（active/retired/revoked/unknown/none）和原文
	SignatureKeyID    string `json:"signature_key_id,omitempty"`
	SignatureKeyState string `json:"signature_key_state,omitempty"`
	SignatureJSON     string `json:"signature_json,omitempty"`
	// 这份条目所属目录文档的 canonical payload 摘要
	DocumentDigest string    `json:"document_digest"`
	AppliedBy      *int64    `json:"applied_by,omitempty"`
	AppliedAt      time.Time `json:"applied_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const directoryEntryCols = "id, directory_id, kind, name, version, channel, digest, source_type, " +
	"source_registry, source_url, size, auto_upgrade, publisher_user_id, publisher_username, publisher_department, notes, " +
	"signature_key_id, signature_key_state, signature_json, document_digest, applied_by, applied_at, created_at, updated_at"

func scanDirectoryEntry(row interface{ Scan(...any) error }) (*DirectoryEntry, error) {
	var e DirectoryEntry
	var pubBy, appliedBy sql.NullInt64
	if err := row.Scan(&e.ID, &e.DirectoryID, &e.Kind, &e.Name, &e.Version, &e.Channel, &e.Digest,
		&e.SourceType, &e.SourceRegistry, &e.SourceURL, &e.Size, &e.AutoUpgrade, &pubBy, &e.PublisherUsername, &e.PublisherDept,
		&e.Notes, &e.SignatureKeyID, &e.SignatureKeyState, &e.SignatureJSON, &e.DocumentDigest,
		&appliedBy, &e.AppliedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if pubBy.Valid {
		id := pubBy.Int64
		e.PublisherUserID = &id
	}
	if appliedBy.Valid {
		id := appliedBy.Int64
		e.AppliedBy = &id
	}
	return &e, nil
}

// ListDirectoryEntries 列出所有登记过的目录条目。
func (s *Store) ListDirectoryEntries(ctx context.Context) ([]DirectoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+directoryEntryCols+" FROM directory_entries ORDER BY directory_id, kind, name, version, channel")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DirectoryEntry{}
	for rows.Next() {
		e, err := scanDirectoryEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetDirectoryEntry 按 id 取一条登记过的目录条目。安装/升级只认这里登记过的来源。
func (s *Store) GetDirectoryEntry(ctx context.Context, id int64) (*DirectoryEntry, error) {
	return scanDirectoryEntry(s.db.QueryRowContext(ctx,
		"SELECT "+directoryEntryCols+" FROM directory_entries WHERE id = ?", id))
}

// ListDirectoryEntriesOf 列出一份目录登记过的条目。
func (s *Store) ListDirectoryEntriesOf(ctx context.Context, directoryID string) ([]DirectoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+directoryEntryCols+" FROM directory_entries WHERE directory_id = ? ORDER BY kind, name, version, channel", directoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DirectoryEntry{}
	for rows.Next() {
		e, err := scanDirectoryEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// DirectoryApplyResult 是一次 apply 对登记表的改动统计。
type DirectoryApplyResult struct {
	// 新登记的条目数
	Created int `json:"created"`
	// 内容有变化、被覆盖的条目数
	Updated int `json:"updated"`
	// 完全没变的条目数
	Unchanged int `json:"unchanged"`
	// 目录里已经没有、被移除的条目数
	Removed int `json:"removed"`
}

// ApplyDirectoryEntries 把一份已校验的目录登记进库，同一个 directory_id 覆盖登记：
// 新的插入、有变化的更新、没变化的保留、目录里没有的删除。整件事在一个事务里。
//
// entries 的 DirectoryID 会统一改成 directoryID；这份目录文档的摘要写进每一行
// （documentDigest），方便审计「这批条目来自哪份签过名的目录」。
// 只有 document_digest / applied_by / applied_at 变化的行也会更新，但统计里算 unchanged——
// 这样「再 apply 一次同一份目录」是幂等的。
func (s *Store) ApplyDirectoryEntries(ctx context.Context, directoryID, documentDigest string, appliedBy int64, entries []DirectoryEntry) (*DirectoryApplyResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, "SELECT "+directoryEntryCols+" FROM directory_entries WHERE directory_id = ?", directoryID)
	if err != nil {
		return nil, err
	}
	existing := map[string]DirectoryEntry{}
	for rows.Next() {
		e, err := scanDirectoryEntry(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		existing[directoryEntryKey(e.Kind, e.Name, e.Version, e.Channel)] = *e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	t := now()
	res := &DirectoryApplyResult{}
	seen := map[string]bool{}
	for i := range entries {
		e := &entries[i]
		e.DirectoryID = directoryID
		e.DocumentDigest = documentDigest
		e.AppliedBy = &appliedBy
		e.AppliedAt = t
		key := directoryEntryKey(e.Kind, e.Name, e.Version, e.Channel)
		if seen[key] {
			// 校验层已经拦过重复；这里再兜一次，避免唯一键报错把整次 apply 弄失败
			continue
		}
		seen[key] = true
		old, ok := existing[key]
		if !ok {
			id, err := insertDirectoryEntry(ctx, tx, e, t)
			if isDuplicate(err) {
				return nil, ErrDuplicate
			}
			if err != nil {
				return nil, err
			}
			e.ID = id
			e.CreatedAt, e.UpdatedAt = t, t
			res.Created++
			continue
		}
		e.ID, e.CreatedAt, e.UpdatedAt = old.ID, old.CreatedAt, t
		if !directoryEntryChanged(old, *e) {
			// 条目本身没变：只刷新「这批条目来自哪份目录文档」和签名者状态（信任根可能变了）
			if _, err := tx.ExecContext(ctx, "UPDATE directory_entries SET signature_key_state = ?, document_digest = ?, applied_by = ?, applied_at = ?, updated_at = ? WHERE id = ?",
				e.SignatureKeyState, e.DocumentDigest, e.AppliedBy, e.AppliedAt, t, old.ID); err != nil {
				return nil, err
			}
			res.Unchanged++
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE directory_entries SET
			digest = ?, source_type = ?, source_registry = ?, source_url = ?, size = ?, auto_upgrade = ?,
			publisher_user_id = ?, publisher_username = ?, publisher_department = ?, notes = ?,
			signature_key_id = ?, signature_key_state = ?, signature_json = ?,
			document_digest = ?, applied_by = ?, applied_at = ?, updated_at = ?
			WHERE id = ?`,
			e.Digest, e.SourceType, e.SourceRegistry, e.SourceURL, e.Size, e.AutoUpgrade,
			e.PublisherUserID, e.PublisherUsername, e.PublisherDept, e.Notes,
			e.SignatureKeyID, e.SignatureKeyState, e.SignatureJSON,
			e.DocumentDigest, e.AppliedBy, e.AppliedAt, t, old.ID); err != nil {
			return nil, err
		}
		res.Updated++
	}
	// 目录里已经没有的条目：从登记表移除（只影响这份目录，不卸载任何东西）
	for key, old := range existing {
		if seen[key] {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM directory_entries WHERE id = ?", old.ID); err != nil {
			return nil, err
		}
		res.Removed++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func directoryEntryKey(kind, name, version, channel string) string {
	return kind + "\x00" + name + "\x00" + version + "\x00" + channel
}

// directoryEntryChanged 判断条目内容有没有变化。document_digest、applied_* 不算内容变化：
// 目录里加了一条别的条目也会让整份文档的摘要变化，那不该让所有旧条目都算「更新」。
func directoryEntryChanged(a, b DirectoryEntry) bool {
	return a.Digest != b.Digest ||
		a.SourceType != b.SourceType ||
		a.SourceRegistry != b.SourceRegistry ||
		a.SourceURL != b.SourceURL ||
		a.Size != b.Size ||
		a.AutoUpgrade != b.AutoUpgrade ||
		!int64PtrEqual(a.PublisherUserID, b.PublisherUserID) ||
		a.PublisherUsername != b.PublisherUsername ||
		a.PublisherDept != b.PublisherDept ||
		a.Notes != b.Notes ||
		a.SignatureKeyID != b.SignatureKeyID ||
		a.SignatureJSON != b.SignatureJSON
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func insertDirectoryEntry(ctx context.Context, tx *sql.Tx, e *DirectoryEntry, t time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO directory_entries
		(directory_id, kind, name, version, channel, digest, source_type, source_registry, source_url, size, auto_upgrade,
		 publisher_user_id, publisher_username, publisher_department, notes,
		 signature_key_id, signature_key_state, signature_json, document_digest, applied_by, applied_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.DirectoryID, e.Kind, e.Name, e.Version, e.Channel, e.Digest, e.SourceType, e.SourceRegistry, e.SourceURL, e.Size, e.AutoUpgrade,
		e.PublisherUserID, e.PublisherUsername, e.PublisherDept, e.Notes,
		e.SignatureKeyID, e.SignatureKeyState, e.SignatureJSON, e.DocumentDigest, e.AppliedBy, e.AppliedAt, t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DirectorySummaries 按 directory_id 汇总登记情况：条目数、最近一次 apply 的时间/操作人/文档摘要。
type DirectorySummary struct {
	DirectoryID    string    `json:"directory_id"`
	EntriesCount   int       `json:"entries_count"`
	DocumentDigest string    `json:"document_digest"`
	AppliedBy      *int64    `json:"applied_by,omitempty"`
	AppliedAt      time.Time `json:"applied_at"`
}

// ListDirectorySummaries 汇总每个目录的登记情况，按最近 apply 时间倒序。
func (s *Store) ListDirectorySummaries(ctx context.Context) ([]DirectorySummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT directory_id, COUNT(*), MAX(document_digest), MAX(applied_at)
		FROM directory_entries GROUP BY directory_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DirectorySummary{}
	for rows.Next() {
		var sum DirectorySummary
		var appliedAt sql.NullTime
		if err := rows.Scan(&sum.DirectoryID, &sum.EntriesCount, &sum.DocumentDigest, &appliedAt); err != nil {
			return nil, err
		}
		if appliedAt.Valid {
			sum.AppliedAt = appliedAt.Time
		}
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AppliedAt.After(out[j].AppliedAt) })
	// 操作人：取该目录最近 apply 的那一行（MySQL/TiDB 不支持在这个聚合里直接带出来，简单再查一次）
	for i := range out {
		var by sql.NullInt64
		if err := s.db.QueryRowContext(ctx, `SELECT applied_by FROM directory_entries
			WHERE directory_id = ? ORDER BY applied_at DESC LIMIT 1`, out[i].DirectoryID).Scan(&by); err == nil && by.Valid {
			id := by.Int64
			out[i].AppliedBy = &id
		}
	}
	return out, nil
}
