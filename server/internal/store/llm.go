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

// Provider 是企业在上游开通的模型服务（OpenAI 兼容接口）。APIKeyEnc 为密文，绝不返回给前端。
type Provider struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Vendor     string `json:"vendor"`
	BaseURL    string `json:"base_url"`
	APIKeyHint string `json:"api_key_hint"`
	// 同一对外模型挂在多个上游时，数字小的先用。
	Priority     int        `json:"priority"`
	Enabled      bool       `json:"enabled"`
	LastCheckAt  *time.Time `json:"last_check_at"`
	LastCheckOK  *bool      `json:"last_check_ok"`
	LastCheckMsg string     `json:"last_check_msg"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// 这个上游上启用的对外模型名（来自 models 表），给额度等页面列可选模型用。
	Models    []string `json:"models"`
	APIKeyEnc []byte   `json:"-"`
}

const providerCols = `p.id, p.name, p.vendor, p.base_url, p.api_key_hint, p.priority, p.enabled,
	p.last_check_at, p.last_check_ok, p.last_check_msg, p.created_at, p.updated_at, p.api_key_enc`

func scanProvider(row interface{ Scan(...any) error }) (*Provider, error) {
	var p Provider
	var checkAt sql.NullTime
	var checkOK sql.NullBool
	err := row.Scan(&p.ID, &p.Name, &p.Vendor, &p.BaseURL, &p.APIKeyHint, &p.Priority, &p.Enabled,
		&checkAt, &checkOK, &p.LastCheckMsg, &p.CreatedAt, &p.UpdatedAt, &p.APIKeyEnc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if checkAt.Valid {
		p.LastCheckAt = &checkAt.Time
	}
	if checkOK.Valid {
		p.LastCheckOK = &checkOK.Bool
	}
	p.Models = []string{}
	return &p, nil
}

// fillProviderModels 把各上游启用中的对外模型名填进 Models。
func (s *Store) fillProviderModels(ctx context.Context, ps []*Provider) error {
	if len(ps) == 0 {
		return nil
	}
	byID := map[int64]*Provider{}
	for _, p := range ps {
		byID[p.ID] = p
	}
	rows, err := s.db.QueryContext(ctx, "SELECT provider_id, name FROM models WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var name string
		if err := rows.Scan(&pid, &name); err != nil {
			return err
		}
		if p := byID[pid]; p != nil {
			p.Models = append(p.Models, name)
		}
	}
	return rows.Err()
}

func (s *Store) CreateProvider(ctx context.Context, p *Provider) error {
	t := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO providers (name, vendor, base_url, api_key_enc, api_key_hint, models, priority, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '[]', ?, ?, ?, ?)`,
		p.Name, p.Vendor, p.BaseURL, p.APIKeyEnc, p.APIKeyHint, p.Priority, p.Enabled, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	p.CreatedAt, p.UpdatedAt = t, t
	return err
}

func (s *Store) GetProvider(ctx context.Context, id int64) (*Provider, error) {
	p, err := scanProvider(s.db.QueryRowContext(ctx, "SELECT "+providerCols+" FROM providers p WHERE p.id = ?", id))
	if err != nil {
		return nil, err
	}
	return p, s.fillProviderModels(ctx, []*Provider{p})
}

func (s *Store) ListProviders(ctx context.Context, enabledOnly bool) ([]*Provider, error) {
	q := "SELECT " + providerCols + " FROM providers p"
	if enabledOnly {
		q += " WHERE p.enabled = 1"
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY p.priority, p.id")
	if err != nil {
		return nil, err
	}
	var out []*Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.fillProviderModels(ctx, out)
}

type ProviderPatch struct {
	Vendor     *string
	BaseURL    *string
	APIKeyEnc  []byte
	APIKeyHint *string
	Priority   *int
	Enabled    *bool
}

func (s *Store) UpdateProvider(ctx context.Context, id int64, p ProviderPatch) error {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Vendor != nil {
		add("vendor", *p.Vendor)
	}
	if p.BaseURL != nil {
		add("base_url", *p.BaseURL)
	}
	if p.APIKeyEnc != nil {
		add("api_key_enc", p.APIKeyEnc)
	}
	if p.APIKeyHint != nil {
		add("api_key_hint", *p.APIKeyHint)
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.Enabled != nil {
		add("enabled", *p.Enabled)
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, "UPDATE providers SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetProviderCheck(ctx context.Context, id int64, ok bool, msg string) error {
	if len([]rune(msg)) > 250 {
		msg = string([]rune(msg)[:250])
	}
	_, err := s.db.ExecContext(ctx, "UPDATE providers SET last_check_at = ?, last_check_ok = ?, last_check_msg = ? WHERE id = ?", now(), ok, msg, id)
	return err
}

// DeleteProvider 连同它的模型一起删除。
func (s *Store) DeleteProvider(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM models WHERE provider_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- 模型 ----

type Model struct {
	ID            int64     `json:"id"`
	ProviderID    int64     `json:"provider_id"`
	ProviderName  string    `json:"provider_name"`
	Name          string    `json:"name"`
	UpstreamModel string    `json:"upstream_model"`
	DisplayName   string    `json:"display_name"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const modelCols = "m.id, m.provider_id, p.name, m.name, m.upstream_model, m.display_name, m.enabled, m.created_at, m.updated_at"

func scanModel(row interface{ Scan(...any) error }) (*Model, error) {
	var m Model
	err := row.Scan(&m.ID, &m.ProviderID, &m.ProviderName, &m.Name, &m.UpstreamModel, &m.DisplayName, &m.Enabled, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

func (s *Store) ListModels(ctx context.Context) ([]*Model, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+modelCols+" FROM models m JOIN providers p ON p.id = m.provider_id ORDER BY m.name, p.priority, p.id, m.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetModel(ctx context.Context, id int64) (*Model, error) {
	return scanModel(s.db.QueryRowContext(ctx, "SELECT "+modelCols+" FROM models m JOIN providers p ON p.id = m.provider_id WHERE m.id = ?", id))
}

func (s *Store) CreateModel(ctx context.Context, m *Model) error {
	t := now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO models (provider_id, name, upstream_model, display_name, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ProviderID, m.Name, m.UpstreamModel, m.DisplayName, m.Enabled, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	m.ID, err = res.LastInsertId()
	m.CreatedAt, m.UpdatedAt = t, t
	return err
}

type ModelPatch struct {
	Name          *string
	UpstreamModel *string
	DisplayName   *string
	Enabled       *bool
}

func (s *Store) UpdateModel(ctx context.Context, id int64, p ModelPatch) error {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.UpstreamModel != nil {
		add("upstream_model", *p.UpstreamModel)
	}
	if p.DisplayName != nil {
		add("display_name", *p.DisplayName)
	}
	if p.Enabled != nil {
		add("enabled", *p.Enabled)
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, "UPDATE models SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteModel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM models WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SyncProviderModels 让上游的模型列表等于 names（对外名与上游名相同）：缺的补上，多的删除。
// 给「新建上游时顺手填模型」和旧接口用；精细的别名配置走模型接口。
func (s *Store) SyncProviderModels(ctx context.Context, providerID int64, names []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT name FROM models WHERE provider_id = ?", providerID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		existing[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取已有模型: %w", err)
	}
	want := map[string]bool{}
	t := now()
	for _, n := range names {
		want[n] = true
		if !existing[n] {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO models (provider_id, name, upstream_model, display_name, enabled, created_at, updated_at) VALUES (?, ?, ?, '', 1, ?, ?)",
				providerID, n, n, t, t); err != nil {
				return err
			}
		}
	}
	for n := range existing {
		if !want[n] {
			if _, err := tx.ExecContext(ctx, "DELETE FROM models WHERE provider_id = ? AND name = ?", providerID, n); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Candidate 是网关为一个对外模型名找到的一条可用线路。
type Candidate struct {
	Provider      *Provider
	UpstreamModel string
}

// ModelCandidates 按上游优先级返回对外模型名对应的全部可用线路（模型与上游都已启用）。
func (s *Store) ModelCandidates(ctx context.Context, name string) ([]Candidate, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+providerCols+", m.upstream_model FROM models m JOIN providers p ON p.id = m.provider_id "+
			"WHERE m.name = ? AND m.enabled = 1 AND p.enabled = 1 ORDER BY p.priority, p.id, m.id", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var p Provider
		var checkAt sql.NullTime
		var checkOK sql.NullBool
		var upstream string
		if err := rows.Scan(&p.ID, &p.Name, &p.Vendor, &p.BaseURL, &p.APIKeyHint, &p.Priority, &p.Enabled,
			&checkAt, &checkOK, &p.LastCheckMsg, &p.CreatedAt, &p.UpdatedAt, &p.APIKeyEnc, &upstream); err != nil {
			return nil, err
		}
		out = append(out, Candidate{Provider: &p, UpstreamModel: upstream})
	}
	return out, rows.Err()
}

// PublicModel 是对员工可见的模型。
type PublicModel struct {
	Name        string
	DisplayName string
	OwnedBy     string
}

// PublicModels 列出启用中的对外模型名（去重，取优先级最高的上游作 owned_by）。
func (s *Store) PublicModels(ctx context.Context) ([]PublicModel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.name, m.display_name, p.name FROM models m JOIN providers p ON p.id = m.provider_id
		 WHERE m.enabled = 1 AND p.enabled = 1 ORDER BY m.name, p.priority, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublicModel
	seen := map[string]bool{}
	for rows.Next() {
		var m PublicModel
		if err := rows.Scan(&m.Name, &m.DisplayName, &m.OwnedBy); err != nil {
			return nil, err
		}
		if !seen[m.Name] {
			seen[m.Name] = true
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// ImportLegacyProviderModels 把 0004 之前存在 providers.models（JSON）里的模型导入 models 表；
// 只处理还没有任何模型行的上游，可重复执行。
func (s *Store) ImportLegacyProviderModels(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.models FROM providers p WHERE NOT EXISTS (SELECT 1 FROM models m WHERE m.provider_id = p.id)`)
	if err != nil {
		return err
	}
	type legacy struct {
		id     int64
		models []string
	}
	var todo []legacy
	for rows.Next() {
		var id int64
		var raw sql.NullString
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var ms []string
		if raw.Valid && raw.String != "" {
			_ = json.Unmarshal([]byte(raw.String), &ms)
		}
		if len(ms) > 0 {
			todo = append(todo, legacy{id, ms})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, l := range todo {
		if err := s.SyncProviderModels(ctx, l.id, l.models); err != nil {
			return err
		}
	}
	return nil
}
