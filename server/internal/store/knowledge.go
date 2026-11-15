package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// 知识库：库（可用范围 + 维护者）、文档（当前版本）、正文的各个版本、检索记录。

// Maintainers 是能上传、编辑、删除文档的人和角色。
type Maintainers struct {
	Users []int64 `json:"users"`
	Roles []int64 `json:"roles"`
}

func (m Maintainers) Normalize() Maintainers {
	out := Maintainers{Users: []int64{}, Roles: []int64{}}
	for _, u := range m.Users {
		if u > 0 && !slices.Contains(out.Users, u) {
			out.Users = append(out.Users, u)
		}
	}
	for _, r := range m.Roles {
		if r > 0 && !slices.Contains(out.Roles, r) {
			out.Roles = append(out.Roles, r)
		}
	}
	return out
}

// Allows 判断用户是不是维护者。
func (m Maintainers) Allows(userID int64, roleIDs []int64) bool {
	if slices.Contains(m.Users, userID) {
		return true
	}
	for _, r := range roleIDs {
		if slices.Contains(m.Roles, r) {
			return true
		}
	}
	return false
}

type KBase struct {
	ID          int64       `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Scope       Scope       `json:"scope"`
	Maintainers Maintainers `json:"maintainers"`
	Revision    string      `json:"revision"`
	CreatedBy   *int64      `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	// 列表查询时填充
	Docs int `json:"docs"`
}

type KDoc struct {
	ID         int64     `json:"id"`
	BaseID     int64     `json:"kb_id"`
	Title      string    `json:"title"`
	Folder     string    `json:"folder"`
	SourceName string    `json:"source_name"`
	Mime       string    `json:"mime"`
	BlobKey    string    `json:"-"`
	Size       int64     `json:"size"`
	Version    int       `json:"version"`
	Lines      int       `json:"lines"`
	UpdatedBy  *int64    `json:"updated_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HasOrigin  bool      `json:"has_original"`
	// 转换体检（JSON，结构见 api 包的 kdocConvert）；网页新建的文档为空
	ConvertInfo json.RawMessage `json:"convert_info,omitempty"`
}

type KDocText struct {
	DocID     int64     `json:"doc_id"`
	Version   int       `json:"version"`
	Markdown  string    `json:"markdown,omitempty"`
	Note      string    `json:"note"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func newRevision() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const kbaseCols = "b.id, b.slug, b.title, b.description, b.scope, b.maintainers, b.revision, b.created_by, b.created_at, b.updated_at"

func scanKBase(row interface{ Scan(...any) error }, extra ...any) (*KBase, error) {
	var b KBase
	var scope, maint string
	var by sql.NullInt64
	err := row.Scan(append([]any{&b.ID, &b.Slug, &b.Title, &b.Description, &scope, &maint, &b.Revision, &by, &b.CreatedAt, &b.UpdatedAt}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.Scope = parseScope(scope)
	_ = json.Unmarshal([]byte(maint), &b.Maintainers)
	b.Maintainers = b.Maintainers.Normalize()
	if by.Valid {
		b.CreatedBy = &by.Int64
	}
	return &b, nil
}

// CreateKBase 新建知识库；slug 已存在时返回 ErrDuplicate。
func (s *Store) CreateKBase(ctx context.Context, b *KBase, by int64) error {
	scope, err := scopeText(b.Scope)
	if err != nil {
		return err
	}
	maint, err := jsonText(b.Maintainers.Normalize())
	if err != nil {
		return err
	}
	t := now()
	b.Revision = newRevision()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO kb_bases (slug, title, description, scope, maintainers, revision, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, b.Slug, b.Title, b.Description, scope, maint, b.Revision, nullID(by), t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if b.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	b.CreatedAt, b.UpdatedAt = t, t
	if by > 0 {
		b.CreatedBy = &by
	}
	return nil
}

func (s *Store) GetKBase(ctx context.Context, id int64) (*KBase, error) {
	var n int
	b, err := scanKBase(s.db.QueryRowContext(ctx, "SELECT "+kbaseCols+", (SELECT COUNT(*) FROM kb_docs d WHERE d.base_id = b.id) FROM kb_bases b WHERE b.id = ?", id), &n)
	if err != nil {
		return nil, err
	}
	b.Docs = n
	return b, nil
}

// GetKBaseBySlug 按标识取知识库（带文档数）。
func (s *Store) GetKBaseBySlug(ctx context.Context, slug string) (*KBase, error) {
	var n int
	b, err := scanKBase(s.db.QueryRowContext(ctx, "SELECT "+kbaseCols+", (SELECT COUNT(*) FROM kb_docs d WHERE d.base_id = b.id) FROM kb_bases b WHERE b.slug = ?", slug), &n)
	if err != nil {
		return nil, err
	}
	b.Docs = n
	return b, nil
}

// ListKBases 列出全部知识库（带文档数），按标题排序。
func (s *Store) ListKBases(ctx context.Context) ([]*KBase, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+kbaseCols+", (SELECT COUNT(*) FROM kb_docs d WHERE d.base_id = b.id) FROM kb_bases b ORDER BY b.title, b.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KBase{}
	for rows.Next() {
		var n int
		b, err := scanKBase(rows, &n)
		if err != nil {
			return nil, err
		}
		b.Docs = n
		out = append(out, b)
	}
	return out, rows.Err()
}

// KBaseRevisions 是全部库此刻的修订号，内存索引据此判断哪些库要重建。
func (s *Store) KBaseRevisions(ctx context.Context) (map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, revision FROM kb_bases")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var rev string
		if err := rows.Scan(&id, &rev); err != nil {
			return nil, err
		}
		out[id] = rev
	}
	return out, rows.Err()
}

type KBasePatch struct {
	Title       *string
	Description *string
	Scope       *Scope
	Maintainers *Maintainers
}

func (s *Store) UpdateKBase(ctx context.Context, id int64, p KBasePatch) error {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	if p.Title != nil {
		sets, args = append(sets, "title = ?"), append(args, *p.Title)
	}
	if p.Description != nil {
		sets, args = append(sets, "description = ?"), append(args, *p.Description)
	}
	if p.Scope != nil {
		sc, err := scopeText(*p.Scope)
		if err != nil {
			return err
		}
		sets, args = append(sets, "scope = ?"), append(args, sc)
	}
	if p.Maintainers != nil {
		m, err := jsonText(p.Maintainers.Normalize())
		if err != nil {
			return err
		}
		sets, args = append(sets, "maintainers = ?"), append(args, m)
	}
	res, err := s.db.ExecContext(ctx, "UPDATE kb_bases SET "+strings.Join(sets, ", ")+" WHERE id = ?", append(args, id)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetKBase(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// GrantKBaseScopeUser 把用户追加进知识库的可用范围，返回是否真的加了。
// 与 GrantAssetScopeUser 相同：事务里锁行读改写，并发审批不会互相覆盖；已包含时返回 added=false。
func (s *Store) GrantKBaseScopeUser(ctx context.Context, kbID, userID int64) (bool, error) {
	return s.grantScopeUser(ctx, "kb_bases", kbID, userID)
}

// DeleteKBase 删除库和它的全部文档与正文；原件留在 blob 存储里（按内容寻址，可能被别处引用）。
func (s *Store) DeleteKBase(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE t FROM kb_doc_texts t JOIN kb_docs d ON d.id = t.doc_id WHERE d.base_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM kb_gap_ignores WHERE base_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM kb_docs WHERE base_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM connection_requests WHERE kind = 'knowledge' AND asset_id = ? AND status = 'pending'", id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM kb_bases WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func bumpRevision(ctx context.Context, e execer, baseID int64, t time.Time) error {
	_, err := e.ExecContext(ctx, "UPDATE kb_bases SET revision = ?, updated_at = ? WHERE id = ?", newRevision(), t, baseID)
	return err
}

const kdocCols = "id, base_id, title, folder, source_name, mime, blob_key, size, version, line_count, updated_by, created_at, updated_at, convert_info"

func scanKDoc(row interface{ Scan(...any) error }) (*KDoc, error) {
	var d KDoc
	var by sql.NullInt64
	var info sql.NullString
	err := row.Scan(&d.ID, &d.BaseID, &d.Title, &d.Folder, &d.SourceName, &d.Mime, &d.BlobKey, &d.Size, &d.Version, &d.Lines, &by, &d.CreatedAt, &d.UpdatedAt, &info)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if by.Valid {
		d.UpdatedBy = &by.Int64
	}
	if info.Valid && info.String != "" {
		d.ConvertInfo = json.RawMessage(info.String)
	}
	d.HasOrigin = d.BlobKey != ""
	return &d, nil
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func countLines(md string) int {
	return strings.Count(strings.TrimRight(md, "\n"), "\n") + 1
}

// CreateKDoc 新建文档与它的第一版正文。
func (s *Store) CreateKDoc(ctx context.Context, d *KDoc, markdown, note string, by int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	d.Version, d.Lines = 1, countLines(markdown)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO kb_docs (base_id, title, folder, source_name, mime, blob_key, size, version, line_count, updated_by, created_at, updated_at, convert_info)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.BaseID, d.Title, d.Folder, d.SourceName, d.Mime, d.BlobKey, d.Size, d.Version, d.Lines, nullID(by), t, t, nullJSON(d.ConvertInfo))
	if err != nil {
		return err
	}
	if d.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO kb_doc_texts (doc_id, version, markdown, note, created_by, created_at) VALUES (?, 1, ?, ?, ?, ?)",
		d.ID, markdown, note, nullID(by), t); err != nil {
		return err
	}
	if err := bumpRevision(ctx, tx, d.BaseID, t); err != nil {
		return err
	}
	d.CreatedAt, d.UpdatedAt = t, t
	d.HasOrigin = d.BlobKey != ""
	if by > 0 {
		d.UpdatedBy = &by
	}
	return tx.Commit()
}

func (s *Store) GetKDoc(ctx context.Context, id int64) (*KDoc, error) {
	return scanKDoc(s.db.QueryRowContext(ctx, "SELECT "+kdocCols+" FROM kb_docs WHERE id = ?", id))
}

// ListKDocs 列出库里的文档，按目录、标题排序。
func (s *Store) ListKDocs(ctx context.Context, baseID int64) ([]*KDoc, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+kdocCols+" FROM kb_docs WHERE base_id = ? ORDER BY folder, title, id", baseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KDoc{}
	for rows.Next() {
		d, err := scanKDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// KDocPatch 修改文档；Markdown 不为空时出一个新版本，Origin 不为空时换成新上传的原件。
type KDocPatch struct {
	Title    *string
	Folder   *string
	Markdown *string
	Note     string
	// 重新上传了文件：换原件
	Origin *KDoc
	// 换转换体检（重新上传、重新转换、网页上改了正文时）
	ConvertInfo json.RawMessage
}

// UpdateKDoc 修改文档，返回修改后的文档。
func (s *Store) UpdateKDoc(ctx context.Context, id int64, p KDocPatch, by int64) (*KDoc, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	d, err := scanKDoc(tx.QueryRowContext(ctx, "SELECT "+kdocCols+" FROM kb_docs WHERE id = ? FOR UPDATE", id))
	if err != nil {
		return nil, err
	}
	t := now()
	if p.Title != nil {
		d.Title = *p.Title
	}
	if p.Folder != nil {
		d.Folder = *p.Folder
	}
	if p.Origin != nil {
		d.SourceName, d.Mime, d.BlobKey, d.Size = p.Origin.SourceName, p.Origin.Mime, p.Origin.BlobKey, p.Origin.Size
	}
	if p.ConvertInfo != nil {
		d.ConvertInfo = p.ConvertInfo
	}
	if p.Markdown != nil {
		d.Version++
		d.Lines = countLines(*p.Markdown)
		if _, err := tx.ExecContext(ctx, "INSERT INTO kb_doc_texts (doc_id, version, markdown, note, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)",
			d.ID, d.Version, *p.Markdown, p.Note, nullID(by), t); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE kb_docs SET title = ?, folder = ?, source_name = ?, mime = ?, blob_key = ?, size = ?, version = ?, line_count = ?, updated_by = ?, updated_at = ?, convert_info = ? WHERE id = ?`,
		d.Title, d.Folder, d.SourceName, d.Mime, d.BlobKey, d.Size, d.Version, d.Lines, nullID(by), t, nullJSON(d.ConvertInfo), d.ID); err != nil {
		return nil, err
	}
	if err := bumpRevision(ctx, tx, d.BaseID, t); err != nil {
		return nil, err
	}
	d.UpdatedAt = t
	d.HasOrigin = d.BlobKey != ""
	if by > 0 {
		d.UpdatedBy = &by
	}
	return d, tx.Commit()
}

func (s *Store) DeleteKDoc(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var base int64
	if err := tx.QueryRowContext(ctx, "SELECT base_id FROM kb_docs WHERE id = ? FOR UPDATE", id).Scan(&base); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM kb_doc_texts WHERE doc_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM kb_docs WHERE id = ?", id); err != nil {
		return err
	}
	if err := bumpRevision(ctx, tx, base, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// KDocText 取某个版本的正文；version 为 0 取当前版本。
func (s *Store) KDocText(ctx context.Context, docID int64, version int) (*KDocText, error) {
	q := "SELECT t.doc_id, t.version, t.markdown, t.note, t.created_by, t.created_at FROM kb_doc_texts t"
	var row *sql.Row
	if version > 0 {
		row = s.db.QueryRowContext(ctx, q+" WHERE t.doc_id = ? AND t.version = ?", docID, version)
	} else {
		row = s.db.QueryRowContext(ctx, q+" JOIN kb_docs d ON d.id = t.doc_id AND d.version = t.version WHERE t.doc_id = ?", docID)
	}
	var t KDocText
	var by sql.NullInt64
	err := row.Scan(&t.DocID, &t.Version, &t.Markdown, &t.Note, &by, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if by.Valid {
		t.CreatedBy = &by.Int64
	}
	return &t, nil
}

// KDocVersions 列出一篇文档的全部版本（不含正文），新的在前。
func (s *Store) KDocVersions(ctx context.Context, docID int64) ([]*KDocText, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT doc_id, version, note, created_by, created_at FROM kb_doc_texts WHERE doc_id = ? ORDER BY version DESC", docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KDocText{}
	for rows.Next() {
		var t KDocText
		var by sql.NullInt64
		if err := rows.Scan(&t.DocID, &t.Version, &t.Note, &by, &t.CreatedAt); err != nil {
			return nil, err
		}
		if by.Valid {
			t.CreatedBy = &by.Int64
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// KBaseTexts 是一个库全部文档的当前正文（建索引用）。
func (s *Store) KBaseTexts(ctx context.Context, baseID int64) ([]*KDoc, map[int64]string, error) {
	docs, err := s.ListKDocs(ctx, baseID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT t.doc_id, t.markdown FROM kb_doc_texts t JOIN kb_docs d ON d.id = t.doc_id AND d.version = t.version WHERE d.base_id = ?", baseID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	texts := map[int64]string{}
	for rows.Next() {
		var id int64
		var md string
		if err := rows.Scan(&id, &md); err != nil {
			return nil, nil, err
		}
		texts[id] = md
	}
	return docs, texts, rows.Err()
}

// ---- 检索记录 ----

type KQuery struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Tool      string    `json:"tool"`
	Query     string    `json:"query"`
	Bases     []int64   `json:"kb_ids"`
	Docs      []int64   `json:"docs"`
	Hits      int       `json:"hits"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) RecordKQuery(ctx context.Context, q KQuery) error {
	bases, _ := jsonText(nonNilIDs(q.Bases))
	docs, _ := jsonText(nonNilIDs(q.Docs))
	if r := []rune(q.Query); len(r) > 500 {
		q.Query = string(r[:500])
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO kb_queries (user_id, tool, query, bases, docs, hits, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		q.UserID, q.Tool, q.Query, bases, docs, q.Hits, now())
	return err
}

// KQueryFilter 筛选检索记录：UserID 不为 0 只看这个人的；Base 不为 0 只看查过这个库的。
type KQueryFilter struct {
	UserID int64
	Base   int64
	Tool   string
	Since  time.Time
	Limit  int
}

// RecentKQueries 新的在前。按库筛选在内存里做（bases 是 JSON），所以先多取一些。
func (s *Store) RecentKQueries(ctx context.Context, f KQueryFilter) ([]*KQuery, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	q := "SELECT id, user_id, tool, query, bases, docs, hits, created_at FROM kb_queries WHERE created_at >= ?"
	args := []any{f.Since}
	if f.UserID > 0 {
		q, args = q+" AND user_id = ?", append(args, f.UserID)
	}
	if f.Tool != "" {
		q, args = q+" AND tool = ?", append(args, f.Tool)
	}
	fetch := f.Limit
	if f.Base > 0 {
		fetch = f.Limit * 20
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id DESC LIMIT ?", append(args, fetch)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KQuery{}
	for rows.Next() {
		var k KQuery
		var bases, docs string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Tool, &k.Query, &bases, &docs, &k.Hits, &k.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(bases), &k.Bases)
		_ = json.Unmarshal([]byte(docs), &k.Docs)
		if f.Base > 0 && !slices.Contains(k.Bases, f.Base) {
			continue
		}
		out = append(out, &k)
		if len(out) == f.Limit {
			break
		}
	}
	return out, rows.Err()
}

// KBGapIgnores 是每个库被忽略的检索词（库 → 归一化后的检索词）。
func (s *Store) KBGapIgnores(ctx context.Context) (map[int64]map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT base_id, query_key FROM kb_gap_ignores")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]bool{}
	for rows.Next() {
		var base int64
		var key string
		if err := rows.Scan(&base, &key); err != nil {
			return nil, err
		}
		if out[base] == nil {
			out[base] = map[string]bool{}
		}
		out[base][key] = true
	}
	return out, rows.Err()
}

// IgnoreKBGap 把一个检索词从这个库的「待补文档」里拿掉；重复忽略不报错。
func (s *Store) IgnoreKBGap(ctx context.Context, baseID int64, key string, by int64) error {
	_, err := s.db.ExecContext(ctx, "INSERT IGNORE INTO kb_gap_ignores (base_id, query_key, created_by, created_at) VALUES (?, ?, ?, ?)", baseID, key, nullID(by), now())
	return err
}

// KDocBrief 是列表里显示的一篇文档：编号、标题、更新时间。
type KDocBrief struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RecentKDocs 是一个库最近更新的 n 篇文档。
func (s *Store) RecentKDocs(ctx context.Context, baseID int64, n int) ([]*KDocBrief, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, title, updated_at FROM kb_docs WHERE base_id = ? ORDER BY updated_at DESC, id DESC LIMIT ?", baseID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KDocBrief
	for rows.Next() {
		var d KDocBrief
		if err := rows.Scan(&d.ID, &d.Title, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}
