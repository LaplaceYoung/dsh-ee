package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- npm 源：包与版本 ----

const (
	OriginHosted   = "hosted"
	OriginUpstream = "upstream"

	StatusPending     = "pending"
	StatusPublished   = "published"
	StatusRejected    = "rejected"
	StatusUnpublished = "unpublished"
)

var ErrExists = errors.New("already exists")

type NpmPackage struct {
	Name          string            `json:"name"`
	Origin        string            `json:"origin"`
	IsPlugin      bool              `json:"is_plugin"`
	OwnerID       *int64            `json:"owner_id"`
	DistTags      map[string]string `json:"dist_tags"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Author        string            `json:"author"`
	Category      string            `json:"category"`
	Kind          string            `json:"kind"`
	Icon          string            `json:"icon"`
	Homepage      string            `json:"homepage"`
	Repository    string            `json:"repository"`
	Featured      bool              `json:"featured"`
	UpstreamScore *int              `json:"upstream_score"`
	UpstreamStars *int              `json:"upstream_stars"`
	DepCount      *int              `json:"dep_count"`
	ClosureSize   *int64            `json:"closure_size"`
	OfflineReady  *bool             `json:"offline_ready"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type NpmVersion struct {
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Manifest         json.RawMessage `json:"-"`
	Integrity        string          `json:"integrity"`
	Shasum           string          `json:"shasum"`
	Size             int64           `json:"size"`
	BlobKey          string          `json:"-"`
	IsPlugin         bool            `json:"is_plugin"`
	HasInstallScript bool            `json:"has_install_script"`
	Status           string          `json:"status"`
	Scan             json.RawMessage `json:"scan"`
	PublisherID      *int64          `json:"publisher_id"`
	ReviewNote       string          `json:"review_note"`
	ReviewedBy       *int64          `json:"reviewed_by"`
	ReviewedAt       *time.Time      `json:"reviewed_at"`
	Deprecated       *string         `json:"deprecated"`
	CreatedAt        time.Time       `json:"created_at"`
	PublishedAt      *time.Time      `json:"published_at"`
}

const packageCols = `name, origin, is_plugin, owner_id, dist_tags, title, description, author, category, kind, icon,
	homepage, repository, featured, upstream_score, upstream_stars, dep_count, closure_size, offline_ready, created_at, updated_at`

func scanPackage(row interface{ Scan(...any) error }) (*NpmPackage, error) {
	var p NpmPackage
	var owner sql.NullInt64
	var tags string
	var score, stars, deps sql.NullInt32
	var closure sql.NullInt64
	var offline sql.NullBool
	err := row.Scan(&p.Name, &p.Origin, &p.IsPlugin, &owner, &tags, &p.Title, &p.Description, &p.Author, &p.Category, &p.Kind, &p.Icon,
		&p.Homepage, &p.Repository, &p.Featured, &score, &stars, &deps, &closure, &offline, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner.Valid {
		p.OwnerID = &owner.Int64
	}
	p.DistTags = map[string]string{}
	if tags != "" {
		_ = json.Unmarshal([]byte(tags), &p.DistTags)
	}
	p.UpstreamScore = nullInt(score)
	p.UpstreamStars = nullInt(stars)
	p.DepCount = nullInt(deps)
	if closure.Valid {
		p.ClosureSize = &closure.Int64
	}
	if offline.Valid {
		p.OfflineReady = &offline.Bool
	}
	return &p, nil
}

func nullInt(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int32)
	return &i
}

const versionCols = `name, version, manifest, integrity, shasum, size, blob_key, is_plugin, has_install_script, status, scan,
	publisher_id, review_note, reviewed_by, reviewed_at, deprecated, created_at, published_at`

// 列表查询不取 manifest，省掉大文本。
const versionColsLite = `name, version, '' AS manifest, integrity, shasum, size, blob_key, is_plugin, has_install_script, status, scan,
	publisher_id, review_note, reviewed_by, reviewed_at, deprecated, created_at, published_at`

func scanVersion(row interface{ Scan(...any) error }) (*NpmVersion, error) {
	var v NpmVersion
	var manifest string
	var scan, deprecated sql.NullString
	var publisher, reviewer sql.NullInt64
	var reviewedAt, publishedAt sql.NullTime
	err := row.Scan(&v.Name, &v.Version, &manifest, &v.Integrity, &v.Shasum, &v.Size, &v.BlobKey, &v.IsPlugin, &v.HasInstallScript, &v.Status, &scan,
		&publisher, &v.ReviewNote, &reviewer, &reviewedAt, &deprecated, &v.CreatedAt, &publishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if manifest != "" {
		v.Manifest = json.RawMessage(manifest)
	}
	if scan.Valid && scan.String != "" {
		v.Scan = json.RawMessage(scan.String)
	}
	if publisher.Valid {
		v.PublisherID = &publisher.Int64
	}
	if reviewer.Valid {
		v.ReviewedBy = &reviewer.Int64
	}
	if reviewedAt.Valid {
		v.ReviewedAt = &reviewedAt.Time
	}
	if deprecated.Valid {
		v.Deprecated = &deprecated.String
	}
	if publishedAt.Valid {
		v.PublishedAt = &publishedAt.Time
	}
	return &v, nil
}

func (s *Store) GetPackage(ctx context.Context, name string) (*NpmPackage, error) {
	return scanPackage(s.db.QueryRowContext(ctx, "SELECT "+packageCols+" FROM npm_packages WHERE name = ?", name))
}

func (s *Store) GetVersion(ctx context.Context, name, version string) (*NpmVersion, error) {
	return scanVersion(s.db.QueryRowContext(ctx, "SELECT "+versionCols+" FROM npm_versions WHERE name = ? AND version = ?", name, version))
}

// ListVersions 返回一个包的版本（含清单），statuses 为空时返回全部。
func (s *Store) ListVersions(ctx context.Context, name string, statuses ...string) ([]*NpmVersion, error) {
	q := "SELECT " + versionCols + " FROM npm_versions WHERE name = ?"
	args := []any{name}
	if len(statuses) > 0 {
		q += " AND status IN (" + placeholders(len(statuses)) + ")"
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	return s.queryVersions(ctx, q+" ORDER BY created_at", args...)
}

func (s *Store) queryVersions(ctx context.Context, q string, args ...any) ([]*NpmVersion, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NpmVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VersionStatuses 返回一个包每个版本的审核状态（回源时用来过滤上游版本）。
func (s *Store) VersionStatuses(ctx context.Context, name string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT version, status FROM npm_versions WHERE name = ?", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var v, st string
		if err := rows.Scan(&v, &st); err != nil {
			return nil, err
		}
		out[v] = st
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func jsonText(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func insertPackage(ctx context.Context, e execer, p *NpmPackage) error {
	tags, err := jsonText(p.DistTags)
	if err != nil {
		return err
	}
	t := now()
	p.CreatedAt, p.UpdatedAt = t, t
	_, err = e.ExecContext(ctx,
		`INSERT INTO npm_packages (name, origin, is_plugin, owner_id, dist_tags, title, description, author, category, kind, icon,
		   homepage, repository, featured, upstream_score, upstream_stars, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.Origin, p.IsPlugin, p.OwnerID, tags, p.Title, p.Description, p.Author, p.Category, p.Kind, p.Icon,
		p.Homepage, p.Repository, p.Featured, p.UpstreamScore, p.UpstreamStars, t, t)
	if isDuplicate(err) {
		return ErrExists
	}
	return err
}

func insertVersion(ctx context.Context, e execer, v *NpmVersion) error {
	var scan any
	if len(v.Scan) > 0 {
		scan = string(v.Scan)
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	_, err := e.ExecContext(ctx,
		`INSERT INTO npm_versions (name, version, manifest, integrity, shasum, size, blob_key, is_plugin, has_install_script, status, scan,
		   publisher_id, deprecated, created_at, published_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.Name, v.Version, string(v.Manifest), v.Integrity, v.Shasum, v.Size, v.BlobKey, v.IsPlugin, v.HasInstallScript, v.Status, scan,
		v.PublisherID, v.Deprecated, v.CreatedAt, v.PublishedAt)
	if isDuplicate(err) {
		return ErrExists
	}
	return err
}

// CatalogPatch 是包的展示信息；nil 字段不改。
type CatalogPatch struct {
	Title, Description, Author, Category, Kind, Icon, Homepage, Repository *string
	Featured                                                               *bool
	IsPlugin                                                               *bool
	UpstreamScore, UpstreamStars                                           *int
}

func (p CatalogPatch) sets() ([]string, []any) {
	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	for _, f := range []struct {
		col string
		v   *string
	}{{"title", p.Title}, {"description", p.Description}, {"author", p.Author}, {"category", p.Category}, {"kind", p.Kind},
		{"icon", p.Icon}, {"homepage", p.Homepage}, {"repository", p.Repository}} {
		if f.v != nil {
			add(f.col, *f.v)
		}
	}
	if p.Featured != nil {
		add("featured", *p.Featured)
	}
	if p.IsPlugin != nil {
		add("is_plugin", *p.IsPlugin)
	}
	if p.UpstreamScore != nil {
		add("upstream_score", *p.UpstreamScore)
	}
	if p.UpstreamStars != nil {
		add("upstream_stars", *p.UpstreamStars)
	}
	return sets, args
}

func (s *Store) UpdatePackageCatalog(ctx context.Context, name string, p CatalogPatch) error {
	return updateCatalog(ctx, s.db, name, p)
}

// queryExecer 是 *sql.DB 与 *sql.Tx 共有的读写方法：零影响行时要再查一次记录在不在。
type queryExecer interface {
	execer
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func updateCatalog(ctx context.Context, e queryExecer, name string, p CatalogPatch) error {
	sets, args := p.sets()
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, now(), name)
	res, err := e.ExecContext(ctx, "UPDATE npm_packages SET "+strings.Join(sets, ", ")+" WHERE name = ?", args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var existing string
		err := e.QueryRowContext(ctx, "SELECT name FROM npm_packages WHERE name = ?", name).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// PublishHosted 写入企业作用域里新发布的一个版本：包不存在时创建（发布者成为负责人），
// 版本号已存在（包括被驳回、下架的）时返回 ErrExists——npm 不允许同一版本号重新发布。
func (s *Store) PublishHosted(ctx context.Context, pkg *NpmPackage, v *NpmVersion, tags map[string]string, catalog CatalogPatch) (created bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	existing, err := scanPackage(tx.QueryRowContext(ctx, "SELECT "+packageCols+" FROM npm_packages WHERE name = ? FOR UPDATE", pkg.Name))
	switch {
	case errors.Is(err, ErrNotFound):
		pkg.DistTags = tags
		if err := insertPackage(ctx, tx, pkg); err != nil {
			return false, err
		}
		created = true
	case err != nil:
		return false, err
	default:
		if existing.Origin != OriginHosted {
			return false, fmt.Errorf("%s 是从上游镜像的包，不能在这里发布", pkg.Name)
		}
		merged := existing.DistTags
		for k, val := range tags {
			merged[k] = val
		}
		text, err := jsonText(merged)
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE npm_packages SET dist_tags = ?, updated_at = ? WHERE name = ?", text, now(), pkg.Name); err != nil {
			return false, err
		}
	}
	if err := insertVersion(ctx, tx, v); err != nil {
		return false, err
	}
	if err := updateCatalog(ctx, tx, pkg.Name, catalog); err != nil {
		return false, err
	}
	return created, tx.Commit()
}

// EnsureUpstreamPackage 记录一个从上游镜像来的包（已存在时不动）。
func (s *Store) EnsureUpstreamPackage(ctx context.Context, p *NpmPackage) error {
	p.Origin = OriginUpstream
	if p.DistTags == nil {
		p.DistTags = map[string]string{}
	}
	err := insertPackage(ctx, s.db, p)
	if errors.Is(err, ErrExists) {
		return nil
	}
	return err
}

// InsertMirroredVersion 记录一个镜像下来的上游版本；已存在时返回 ErrExists。
func (s *Store) InsertMirroredVersion(ctx context.Context, v *NpmVersion) error {
	return insertVersion(ctx, s.db, v)
}

// SetPackageDistTags 整体替换 dist-tags。
func (s *Store) SetPackageDistTags(ctx context.Context, name string, tags map[string]string) error {
	text, err := jsonText(tags)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE npm_packages SET dist_tags = ?, updated_at = ? WHERE name = ?", text, now(), name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetVersionDeprecated(ctx context.Context, name, version string, msg *string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE npm_versions SET deprecated = ? WHERE name = ? AND version = ?", msg, name, version)
	return err
}

// SetVersionStatus 把处于 from 状态之一的版本改成 to，并记下审核人与意见。
// version 为空时作用于整个包。返回改动的行数。
// wasLive 为 true 时只动上过架的版本（published_at 只在真正上架时写入）。
func (s *Store) SetVersionStatus(ctx context.Context, name, version string, from []string, to string, reviewer int64, note string, wasLive bool) (int64, error) {
	t := now()
	q := `UPDATE npm_versions SET status = ?, reviewed_by = ?, reviewed_at = ?, review_note = ?,
	        published_at = CASE WHEN ? = 'published' AND published_at IS NULL THEN ? ELSE published_at END
	      WHERE name = ? AND status IN (` + placeholders(len(from)) + `)`
	args := []any{to, reviewer, t, note, to, t, name}
	for _, f := range from {
		args = append(args, f)
	}
	if version != "" {
		q += " AND version = ?"
		args = append(args, version)
	}
	if wasLive {
		q += " AND published_at IS NOT NULL"
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, _ = s.db.ExecContext(ctx, "UPDATE npm_packages SET updated_at = ? WHERE name = ?", t, name)
	}
	return n, nil
}

// SetClosureStats 记录预载根的依赖闭包统计。
func (s *Store) SetClosureStats(ctx context.Context, name string, deps int, size int64, offline bool) error {
	_, err := s.db.ExecContext(ctx, "UPDATE npm_packages SET dep_count = ?, closure_size = ?, offline_ready = ?, updated_at = ? WHERE name = ?",
		deps, size, offline, now(), name)
	return err
}

// PackageFilter 是能力列表的筛选。
type PackageFilter struct {
	// 只要插件或企业托管包（不含上游镜像来的普通依赖）
	CatalogOnly bool
	Names       []string
}

func (s *Store) ListPackages(ctx context.Context, f PackageFilter) ([]*NpmPackage, error) {
	q := "SELECT " + packageCols + " FROM npm_packages WHERE 1 = 1"
	var args []any
	if f.CatalogOnly {
		q += " AND (is_plugin = 1 OR origin = 'hosted')"
	}
	if f.Names != nil {
		if len(f.Names) == 0 {
			return nil, nil
		}
		q += " AND name IN (" + placeholders(len(f.Names)) + ")"
		for _, n := range f.Names {
			args = append(args, n)
		}
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY updated_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NpmPackage
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// VersionsOf 批量取多个包的版本（不含清单）。
func (s *Store) VersionsOf(ctx context.Context, names []string) (map[string][]*NpmVersion, error) {
	out := map[string][]*NpmVersion{}
	for start := 0; start < len(names); start += 500 {
		chunk := names[start:min(start+500, len(names))]
		args := make([]any, len(chunk))
		for i, n := range chunk {
			args[i] = n
		}
		vs, err := s.queryVersions(ctx, "SELECT "+versionColsLite+" FROM npm_versions WHERE name IN ("+placeholders(len(chunk))+") ORDER BY created_at", args...)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			out[v.Name] = append(out[v.Name], v)
		}
	}
	return out, nil
}

// PendingVersions 是审核队列（不含清单）。
func (s *Store) PendingVersions(ctx context.Context) ([]*NpmVersion, error) {
	return s.queryVersions(ctx, "SELECT "+versionColsLite+" FROM npm_versions WHERE status = 'pending' ORDER BY created_at")
}

// PublishedPluginVersions 是所有已上架的插件版本（生成 DSH 索引用，不含清单）。
func (s *Store) PublishedPluginVersions(ctx context.Context) ([]*NpmVersion, error) {
	return s.queryVersions(ctx, "SELECT "+versionColsLite+" FROM npm_versions WHERE status = 'published' AND is_plugin = 1 ORDER BY name, created_at")
}

// RegistryStats 是 npm 源页面上的统计。
type RegistryStats struct {
	HostedPackages   int   `json:"hosted_packages"`
	UpstreamPackages int   `json:"upstream_packages"`
	UpstreamPlugins  int   `json:"upstream_plugins"`
	Versions         int   `json:"versions"`
	Pending          int   `json:"pending"`
	StorageBytes     int64 `json:"storage_bytes"`
	Downloads30d     int   `json:"downloads_30d"`
}

func (s *Store) RegistryStats(ctx context.Context) (*RegistryStats, error) {
	var st RegistryStats
	err := s.db.QueryRowContext(ctx, `SELECT
		  COALESCE(SUM(CASE WHEN origin = 'hosted' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN origin = 'upstream' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN origin = 'upstream' AND is_plugin = 1 THEN 1 ELSE 0 END), 0)
		FROM npm_packages`).Scan(&st.HostedPackages, &st.UpstreamPackages, &st.UpstreamPlugins)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0), COALESCE(SUM(size), 0)
		FROM npm_versions`).Scan(&st.Versions, &st.Pending, &st.StorageBytes)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM npm_downloads WHERE created_at >= ?", now().Add(-30*24*time.Hour)).Scan(&st.Downloads30d)
	return &st, err
}

// ---- 下载日志 ----

// Download 是一次 tarball 下载；Client / OS / Arch 来自 User-Agent，认不出时为空。
type Download struct {
	Name, Version    string
	UserID, TokenID  *int64
	Client, OS, Arch string
}

func (s *Store) RecordDownload(ctx context.Context, d Download) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO npm_downloads (name, version, user_id, token_id, client, os, arch, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		d.Name, d.Version, d.UserID, d.TokenID, d.Client, d.OS, d.Arch, now())
	return err
}

// InstallCounts 统计每个包的下载次数（即企业内安装次数）。
func (s *Store) InstallCounts(ctx context.Context, names []string) (map[string]int, error) {
	out := map[string]int{}
	for start := 0; start < len(names); start += 500 {
		chunk := names[start:min(start+500, len(names))]
		args := make([]any, len(chunk))
		for i, n := range chunk {
			args[i] = n
		}
		rows, err := s.db.QueryContext(ctx, "SELECT name, COUNT(*) FROM npm_downloads WHERE name IN ("+placeholders(len(chunk))+") GROUP BY name", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var n string
			var c int
			if err := rows.Scan(&n, &c); err != nil {
				rows.Close()
				return nil, err
			}
			out[n] = c
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("读取包下载统计: %w", err)
		}
	}
	return out, nil
}

// ---- npm 令牌 ----

type NpmToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Username   string     `json:"username"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Access     string     `json:"access"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

const tokenCols = "t.id, t.user_id, u.username, t.name, t.prefix, t.access, t.last_used_at, t.revoked_at, t.created_at"

func scanToken(row interface{ Scan(...any) error }) (*NpmToken, error) {
	var t NpmToken
	var used, revoked sql.NullTime
	err := row.Scan(&t.ID, &t.UserID, &t.Username, &t.Name, &t.Prefix, &t.Access, &used, &revoked, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if used.Valid {
		t.LastUsedAt = &used.Time
	}
	if revoked.Valid {
		t.RevokedAt = &revoked.Time
	}
	return &t, nil
}

func (s *Store) CreateNpmToken(ctx context.Context, t *NpmToken, hash string) error {
	t.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, "INSERT INTO npm_tokens (user_id, name, prefix, token_hash, access, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.UserID, t.Name, t.Prefix, hash, t.Access, t.CreatedAt)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

// ActiveNpmToken 按哈希找未吊销、所属用户未禁用的令牌。
func (s *Store) ActiveNpmToken(ctx context.Context, hash string) (*NpmToken, *User, error) {
	t, err := scanToken(s.db.QueryRowContext(ctx,
		"SELECT "+tokenCols+" FROM npm_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ? AND t.revoked_at IS NULL AND u.disabled = 0", hash))
	if err != nil {
		return nil, nil, err
	}
	u, err := s.GetUser(ctx, t.UserID)
	if err != nil {
		return nil, nil, err
	}
	return t, u, nil
}

// ListNpmTokens 列出令牌；userID 为 0 时列出全部。
func (s *Store) ListNpmTokens(ctx context.Context, userID int64) ([]*NpmToken, error) {
	q := "SELECT " + tokenCols + " FROM npm_tokens t JOIN users u ON u.id = t.user_id WHERE t.revoked_at IS NULL"
	var args []any
	if userID != 0 {
		q += " AND t.user_id = ?"
		args = append(args, userID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY t.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NpmToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetNpmToken(ctx context.Context, id int64) (*NpmToken, error) {
	return scanToken(s.db.QueryRowContext(ctx, "SELECT "+tokenCols+" FROM npm_tokens t JOIN users u ON u.id = t.user_id WHERE t.id = ?", id))
}

func (s *Store) RevokeNpmToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE npm_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchNpmToken 更新最近使用时间；一分钟内只写一次，安装时上百个请求不至于都打到数据库。
func (s *Store) TouchNpmToken(ctx context.Context, id int64) error {
	t := now()
	_, err := s.db.ExecContext(ctx, "UPDATE npm_tokens SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)",
		t, id, t.Add(-time.Minute))
	return err
}

// ---- 预载任务 ----

type SyncJob struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Params     json.RawMessage `json:"params"`
	Total      int             `json:"total"`
	Done       int             `json:"done"`
	Failed     int             `json:"failed"`
	Bytes      int64           `json:"bytes"`
	Log        string          `json:"log"`
	CreatedBy  *int64          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

const jobCols = "id, kind, status, params, total, done, failed, bytes, log, created_by, created_at, finished_at"

func scanJob(row interface{ Scan(...any) error }) (*SyncJob, error) {
	var j SyncJob
	var params string
	var log sql.NullString
	var by sql.NullInt64
	var fin sql.NullTime
	err := row.Scan(&j.ID, &j.Kind, &j.Status, &params, &j.Total, &j.Done, &j.Failed, &j.Bytes, &log, &by, &j.CreatedAt, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.Params = json.RawMessage(params)
	j.Log = log.String
	if by.Valid {
		j.CreatedBy = &by.Int64
	}
	if fin.Valid {
		j.FinishedAt = &fin.Time
	}
	return &j, nil
}

func (s *Store) CreateJob(ctx context.Context, j *SyncJob) error {
	j.CreatedAt = now()
	if j.Status == "" {
		j.Status = "running"
	}
	res, err := s.db.ExecContext(ctx, "INSERT INTO sync_jobs (kind, status, params, total, log, created_by, created_at) VALUES (?, ?, ?, ?, '', ?, ?)",
		j.Kind, j.Status, string(j.Params), j.Total, j.CreatedBy, j.CreatedAt)
	if err != nil {
		return err
	}
	j.ID, err = res.LastInsertId()
	return err
}

// UpdateJob 写入进度与日志；status 非 running 时记结束时间。
func (s *Store) UpdateJob(ctx context.Context, j *SyncJob) error {
	var fin any
	if j.Status != "running" {
		t := now()
		j.FinishedAt = &t
		fin = t
	}
	_, err := s.db.ExecContext(ctx, "UPDATE sync_jobs SET status = ?, total = ?, done = ?, failed = ?, bytes = ?, log = ?, finished_at = ? WHERE id = ?",
		j.Status, j.Total, j.Done, j.Failed, j.Bytes, j.Log, fin, j.ID)
	return err
}

func (s *Store) GetJob(ctx context.Context, id int64) (*SyncJob, error) {
	return scanJob(s.db.QueryRowContext(ctx, "SELECT "+jobCols+" FROM sync_jobs WHERE id = ?", id))
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]*SyncJob, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+jobCols+" FROM sync_jobs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SyncJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// FailRunningJobs 在启动时把上次没跑完（进程退出）的任务标成中断。
func (s *Store) FailRunningJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_jobs SET status = 'interrupted', finished_at = ? WHERE status = 'running'", now())
	return err
}
