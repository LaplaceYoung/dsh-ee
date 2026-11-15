package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// 专家 · 技能 · 连接器的管理策略（docs/expert-center.md §5）：受管分类与禁用策略。
// 两张表都很小（几十行），读资产时整表读出来挂到资产上，不做缓存。

// 分类分组，对应 DSH 面板的三个 Tab。
const (
	CategoryGroupApp        = "app"
	CategoryGroupSkill      = "skill"
	CategoryGroupConnection = "connection"
)

// CategoryGroupOf 是某类资产归属的分类分组。
func CategoryGroupOf(kind string) string {
	switch kind {
	case KindApp:
		return CategoryGroupApp
	case KindConnection:
		return CategoryGroupConnection
	}
	return CategoryGroupSkill
}

type Category struct {
	ID        int64     `json:"id"`
	Group     string    `json:"group"`
	Name      string    `json:"name"`
	Sort      int       `json:"sort"`
	Scope     Scope     `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	var c Category
	var scope string
	err := row.Scan(&c.ID, &c.Group, &c.Name, &c.Sort, &scope, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Scope = parseScope(scope)
	return &c, nil
}

const categoryCols = "id, grp, name, sort, scope, created_at, updated_at"

// ListCategories 按分组、排序、名字列出全部分类。
func (s *Store) ListCategories(ctx context.Context) ([]*Category, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+categoryCols+" FROM asset_categories ORDER BY grp, sort, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCategory(ctx context.Context, id int64) (*Category, error) {
	return scanCategory(s.db.QueryRowContext(ctx, "SELECT "+categoryCols+" FROM asset_categories WHERE id = ?", id))
}

// CreateCategory 新建分类；同一分组里重名返回 ErrDuplicate。
func (s *Store) CreateCategory(ctx context.Context, c *Category) error {
	scope, err := scopeText(c.Scope)
	if err != nil {
		return err
	}
	t := now()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO asset_categories (grp, name, sort, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		c.Group, c.Name, c.Sort, scope, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	c.Scope, c.CreatedAt, c.UpdatedAt = c.Scope.Normalize(), t, t
	return err
}

// UpdateCategory 改名、排序与范围（分组不能改）。改名时把归在这里的资产的分类文本一起改掉。
func (s *Store) UpdateCategory(ctx context.Context, c *Category) error {
	scope, err := scopeText(c.Scope)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx, "UPDATE asset_categories SET name = ?, sort = ?, scope = ?, updated_at = ? WHERE id = ?", c.Name, c.Sort, scope, t, c.ID)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := scanCategory(tx.QueryRowContext(ctx, "SELECT "+categoryCols+" FROM asset_categories WHERE id = ?", c.ID)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE assets SET category = ? WHERE category_id = ?", c.Name, c.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCategory 删分类；归在这里的资产变成没归类（分类文本也清空）。
func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE assets SET category_id = NULL, category = '' WHERE category_id = ?", id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM asset_categories WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// Block 是一条禁用策略：Scope 里的人用不了 AssetIDs 里的能力。
type Block struct {
	ID        int64     `json:"id"`
	Scope     Scope     `json:"scope"`
	AssetIDs  []int64   `json:"asset_ids"`
	Note      string    `json:"note"`
	UpdatedBy *int64    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const blockCols = "id, scope, asset_ids, note, updated_by, created_at, updated_at"

func scanBlock(row interface{ Scan(...any) error }) (*Block, error) {
	var b Block
	var scope, ids string
	var by sql.NullInt64
	err := row.Scan(&b.ID, &scope, &ids, &b.Note, &by, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.Scope = parseScope(scope)
	b.AssetIDs = []int64{}
	_ = json.Unmarshal([]byte(ids), &b.AssetIDs)
	if by.Valid {
		v := by.Int64
		b.UpdatedBy = &v
	}
	return &b, nil
}

func normalizeIDs(ids []int64) []int64 {
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func (s *Store) ListBlocks(ctx context.Context) ([]*Block, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+blockCols+" FROM asset_blocks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Block{}
	for rows.Next() {
		b, err := scanBlock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetBlock(ctx context.Context, id int64) (*Block, error) {
	return scanBlock(s.db.QueryRowContext(ctx, "SELECT "+blockCols+" FROM asset_blocks WHERE id = ?", id))
}

// SaveBlock 新建（ID 为 0）或替换一条禁用策略。
func (s *Store) SaveBlock(ctx context.Context, b *Block, by int64) error {
	b.Scope, b.AssetIDs = b.Scope.Normalize(), normalizeIDs(b.AssetIDs)
	scope, err := scopeText(b.Scope)
	if err != nil {
		return err
	}
	ids, err := json.Marshal(b.AssetIDs)
	if err != nil {
		return err
	}
	t := now()
	if b.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			"INSERT INTO asset_blocks (scope, asset_ids, note, updated_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			scope, string(ids), b.Note, nullID(by), t, t)
		if err != nil {
			return err
		}
		b.ID, err = res.LastInsertId()
		b.CreatedAt, b.UpdatedAt = t, t
		return err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE asset_blocks SET scope = ?, asset_ids = ?, note = ?, updated_by = ?, updated_at = ? WHERE id = ?",
		scope, string(ids), b.Note, nullID(by), t, b.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetBlock(ctx, b.ID); err != nil {
			return err
		}
	}
	b.UpdatedAt = t
	return nil
}

func (s *Store) DeleteBlock(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM asset_blocks WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// attachGates 给资产填上所在分类的范围与命中的禁用策略（Asset.Gate 用）。
func (s *Store) attachGates(ctx context.Context, assets []*Asset) error {
	if len(assets) == 0 {
		return nil
	}
	cats, err := s.ListCategories(ctx)
	if err != nil {
		return err
	}
	blocks, err := s.ListBlocks(ctx)
	if err != nil {
		return err
	}
	byID := make(map[int64]*Category, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	for _, a := range assets {
		a.CategoryScope, a.Blocks = nil, nil
		if a.CategoryID != nil {
			if c := byID[*a.CategoryID]; c != nil {
				sc := c.Scope
				a.CategoryScope = &sc
			}
		}
		for _, b := range blocks {
			if slices.Contains(b.AssetIDs, a.ID) {
				a.Blocks = append(a.Blocks, b.Scope)
			}
		}
	}
	return nil
}
