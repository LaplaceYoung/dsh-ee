package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// 数据中台的说明（第一版 M15）：上传生成的表与数据集。表的数据在单独的库里，见 internal/datahub。

type DataColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type DataTable struct {
	Name        string       `json:"name"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Columns     []DataColumn `json:"columns"`
	RowCount    int64        `json:"row_count"`
	CreatedBy   int64        `json:"created_by"`
	UpdatedBy   int64        `json:"updated_by"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// DataParam 是数据集的一个参数。Auto 的由系统按当前员工填（me_department 等），不能改。
type DataParam struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Default string `json:"default,omitempty"`
	Auto    bool   `json:"auto,omitempty"`
}

type DataSet struct {
	ID          int64       `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	SQL         string      `json:"sql"`
	Params      []DataParam `json:"params"`
	Scope       Scope       `json:"scope"`
	CreatedBy   int64       `json:"created_by"`
	UpdatedBy   int64       `json:"updated_by"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (s *Store) ListDataTables(ctx context.Context) ([]*DataTable, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, title, description, columns, row_count, created_by, updated_by, created_at, updated_at FROM data_tables ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*DataTable{}
	for rows.Next() {
		t, err := scanDataTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetDataTable(ctx context.Context, name string) (*DataTable, error) {
	return scanDataTable(s.db.QueryRowContext(ctx, "SELECT name, title, description, columns, row_count, created_by, updated_by, created_at, updated_at FROM data_tables WHERE name = ?", name))
}

func scanDataTable(row interface{ Scan(...any) error }) (*DataTable, error) {
	var t DataTable
	var cols []byte
	err := row.Scan(&t.Name, &t.Title, &t.Description, &cols, &t.RowCount, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cols, &t.Columns)
	return &t, nil
}

// SaveDataTable 新建或覆盖一张表的说明。
func (s *Store) SaveDataTable(ctx context.Context, t *DataTable, by int64) error {
	cols, _ := json.Marshal(t.Columns)
	n := now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO data_tables (name, title, description, columns, row_count, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE title = VALUES(title), description = VALUES(description), columns = VALUES(columns), row_count = VALUES(row_count), updated_by = VALUES(updated_by), updated_at = VALUES(updated_at)`,
		t.Name, t.Title, t.Description, string(cols), t.RowCount, by, by, n, n)
	return err
}

func (s *Store) DeleteDataTable(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM data_tables WHERE name = ?", name)
	return err
}

const dataSetColumns = "id, slug, title, description, sql_text, params, scope, created_by, updated_by, created_at, updated_at"

func scanDataSet(row interface{ Scan(...any) error }) (*DataSet, error) {
	var d DataSet
	var params, scope []byte
	err := row.Scan(&d.ID, &d.Slug, &d.Title, &d.Description, &d.SQL, &params, &scope, &d.CreatedBy, &d.UpdatedBy, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(params, &d.Params)
	_ = json.Unmarshal(scope, &d.Scope)
	if d.Params == nil {
		d.Params = []DataParam{}
	}
	return &d, nil
}

func (s *Store) ListDataSets(ctx context.Context) ([]*DataSet, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+dataSetColumns+" FROM data_sets ORDER BY slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*DataSet{}
	for rows.Next() {
		d, err := scanDataSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDataSet(ctx context.Context, slug string) (*DataSet, error) {
	return scanDataSet(s.db.QueryRowContext(ctx, "SELECT "+dataSetColumns+" FROM data_sets WHERE slug = ?", slug))
}

func (s *Store) CreateDataSet(ctx context.Context, d *DataSet, by int64) error {
	params, _ := json.Marshal(d.Params)
	scope, _ := json.Marshal(d.Scope.Normalize())
	n := now()
	res, err := s.db.ExecContext(ctx, "INSERT INTO data_sets (slug, title, description, sql_text, params, scope, created_by, updated_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		d.Slug, d.Title, d.Description, d.SQL, string(params), string(scope), by, by, n, n)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	d.CreatedAt, d.UpdatedAt, d.CreatedBy, d.UpdatedBy = n, n, by, by
	return nil
}

// UpdateDataSet 覆盖保存数据集。expected 非空时做乐观并发检查：范围与 SQL 都是整段覆盖的，
// 用页面上的旧快照保存会丢掉别人后来的改动；0 行受影响 → ErrNotFound / ErrConflict。
func (s *Store) UpdateDataSet(ctx context.Context, d *DataSet, by int64, expected *time.Time) error {
	params, _ := json.Marshal(d.Params)
	scope, _ := json.Marshal(d.Scope.Normalize())
	n := now()
	q := "UPDATE data_sets SET title = ?, description = ?, sql_text = ?, params = ?, scope = ?, updated_by = ?, updated_at = ? WHERE slug = ?"
	args := []any{d.Title, d.Description, d.SQL, string(params), string(scope), by, n, d.Slug}
	if expected != nil {
		q += " AND updated_at = ?"
		args = append(args, *expected)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		if expected != nil {
			return s.casMiss(ctx, "data_sets", "slug", d.Slug, *expected)
		}
		return ErrNotFound
	}
	d.UpdatedAt = n
	return nil
}

func (s *Store) DeleteDataSet(ctx context.Context, slug string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM data_sets WHERE slug = ?", slug)
	return err
}
