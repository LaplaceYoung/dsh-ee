package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

var ErrInUse = errors.New("in use")

// ---- 权限点 ----

type Permission struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Group       string    `json:"group"`
	Description string    `json:"description"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
}

// SyncPermissions 把代码内置的权限点写入数据库：已存在的更新名称与说明；
// 新出现的插入并授予 DefaultRoles；数据库里标为内置、但代码里已没有的降为自建（可删）。
func (s *Store) SyncPermissions(ctx context.Context, catalog []rbac.Permission) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existing := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT code FROM permissions")
	if err != nil {
		return err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		existing[c] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取已有权限点: %w", err)
	}

	inCatalog := map[string]bool{}
	t := now()
	for _, p := range catalog {
		inCatalog[p.Code] = true
		if existing[p.Code] {
			if _, err := tx.ExecContext(ctx,
				"UPDATE permissions SET name = ?, group_name = ?, description = ?, builtin = 1 WHERE code = ?",
				p.Name, p.Group, p.Description, p.Code); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO permissions (code, name, group_name, description, builtin, created_at) VALUES (?, ?, ?, ?, 1, ?)",
			p.Code, p.Name, p.Group, p.Description, t); err != nil {
			return err
		}
		for _, roleCode := range p.DefaultRoles {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO role_permissions (role_id, permission_code)
				 SELECT id, ? FROM roles WHERE code = ?`, p.Code, roleCode); err != nil {
				return err
			}
		}
	}
	for code := range existing {
		if !inCatalog[code] {
			if _, err := tx.ExecContext(ctx, "UPDATE permissions SET builtin = 0 WHERE code = ? AND builtin = 1", code); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) ListPermissions(ctx context.Context) ([]*Permission, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT code, name, group_name, description, builtin, created_at FROM permissions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Code, &p.Name, &p.Group, &p.Description, &p.Builtin, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 内置的按代码目录顺序，自建的排在后面按 code 排
	order := map[string]int{}
	for i, p := range rbac.Catalog {
		order[p.Code] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, iok := order[out[i].Code]
		oj, jok := order[out[j].Code]
		if iok != jok {
			return iok
		}
		if iok {
			return oi < oj
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func (s *Store) PermissionExists(ctx context.Context, code string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM permissions WHERE code = ?", code).Scan(&n)
	return n > 0, err
}

func (s *Store) CreatePermission(ctx context.Context, p *Permission) error {
	p.CreatedAt = now()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO permissions (code, name, group_name, description, builtin, created_at) VALUES (?, ?, ?, ?, 0, ?)",
		p.Code, p.Name, p.Group, p.Description, p.CreatedAt)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

// DeletePermission 只删自建权限点；仍被菜单引用时拒绝，避免页面意外对所有人开放或无人可见。
func (s *Store) DeletePermission(ctx context.Context, code string) error {
	var builtin bool
	err := s.db.QueryRowContext(ctx, "SELECT builtin FROM permissions WHERE code = ?", code).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if builtin {
		return fmt.Errorf("%w: 内置权限点不能删除", ErrInUse)
	}
	var refs int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM menus WHERE permission = ?", code).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("%w: 还有 %d 个菜单使用这个权限点", ErrInUse, refs)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_permissions WHERE permission_code = ?", code); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM permissions WHERE code = ?", code); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- 角色 ----

type Role struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSuper     bool      `json:"is_super"`
	Builtin     bool      `json:"builtin"`
	Permissions []string  `json:"permissions"`
	UserCount   int       `json:"user_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RoleRef 是挂在用户上的角色摘要。
type RoleRef struct {
	ID      int64  `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	IsSuper bool   `json:"is_super"`
}

const roleCols = "r.id, r.code, r.name, r.description, r.is_super, r.builtin, r.created_at, r.updated_at"

func scanRole(row interface{ Scan(...any) error }) (*Role, error) {
	var r Role
	err := row.Scan(&r.ID, &r.Code, &r.Name, &r.Description, &r.IsSuper, &r.Builtin, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *Store) ListRoles(ctx context.Context) ([]*Role, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+roleCols+", (SELECT COUNT(*) FROM user_roles ur WHERE ur.role_id = r.id) FROM roles r ORDER BY r.is_super DESC, r.builtin DESC, r.id")
	if err != nil {
		return nil, err
	}
	var out []*Role
	byID := map[int64]*Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Description, &r.IsSuper, &r.Builtin, &r.CreatedAt, &r.UpdatedAt, &r.UserCount); err != nil {
			rows.Close()
			return nil, err
		}
		r.Permissions = []string{}
		out = append(out, &r)
		byID[r.ID] = &r
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	prow, err := s.db.QueryContext(ctx, "SELECT role_id, permission_code FROM role_permissions ORDER BY permission_code")
	if err != nil {
		return nil, err
	}
	defer prow.Close()
	for prow.Next() {
		var id int64
		var code string
		if err := prow.Scan(&id, &code); err != nil {
			return nil, err
		}
		if r := byID[id]; r != nil {
			r.Permissions = append(r.Permissions, code)
		}
	}
	return out, prow.Err()
}

func (s *Store) GetRole(ctx context.Context, id int64) (*Role, error) {
	r, err := scanRole(s.db.QueryRowContext(ctx, "SELECT "+roleCols+" FROM roles r WHERE r.id = ?", id))
	if err != nil {
		return nil, err
	}
	r.Permissions, err = s.rolePermissions(ctx, s.db, id)
	return r, err
}

func (s *Store) GetRoleByCode(ctx context.Context, code string) (*Role, error) {
	r, err := scanRole(s.db.QueryRowContext(ctx, "SELECT "+roleCols+" FROM roles r WHERE r.code = ?", code))
	if err != nil {
		return nil, err
	}
	r.Permissions, err = s.rolePermissions(ctx, s.db, r.ID)
	return r, err
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) rolePermissions(ctx context.Context, q querier, roleID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT permission_code FROM role_permissions WHERE role_id = ? ORDER BY permission_code", roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateRole(ctx context.Context, r *Role) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO roles (code, name, description, is_super, builtin, created_at, updated_at) VALUES (?, ?, ?, 0, 0, ?, ?)",
		r.Code, r.Name, r.Description, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if err := setRolePermissions(ctx, tx, r.ID, r.Permissions); err != nil {
		return err
	}
	r.CreatedAt, r.UpdatedAt = t, t
	return tx.Commit()
}

type RolePatch struct {
	Name        *string
	Description *string
	Permissions []string // nil 表示不修改
}

func (s *Store) UpdateRole(ctx context.Context, id int64, p RolePatch) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	if p.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *p.Name)
	}
	if p.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *p.Description)
	}
	args = append(args, id)
	res, err := tx.ExecContext(ctx, "UPDATE roles SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if p.Permissions != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id = ?", id); err != nil {
			return err
		}
		if err := setRolePermissions(ctx, tx, id, p.Permissions); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func setRolePermissions(ctx context.Context, tx *sql.Tx, roleID int64, codes []string) error {
	for _, c := range codes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO role_permissions (role_id, permission_code) VALUES (?, ?)", roleID, c); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRole 删除自建角色及其关联；持有该角色的用户随之失去这些权限。
func (s *Store) DeleteRole(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var builtin bool
	err = tx.QueryRowContext(ctx, "SELECT builtin FROM roles WHERE id = ?", id).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if builtin {
		return fmt.Errorf("%w: 内置角色不能删除", ErrInUse)
	}
	for _, q := range []string{
		"DELETE FROM role_permissions WHERE role_id = ?",
		"DELETE FROM user_roles WHERE role_id = ?",
		"DELETE FROM roles WHERE id = ?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- 用户与角色 ----

// UserAccess 汇总用户全部角色的权限。
func (s *Store) UserAccess(ctx context.Context, userID int64) (*rbac.Access, error) {
	a := &rbac.Access{Perms: map[string]bool{}}
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.code, r.is_super FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ? ORDER BY r.id`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var code string
		var super bool
		if err := rows.Scan(&id, &code, &super); err != nil {
			rows.Close()
			return nil, err
		}
		a.RoleIDs = append(a.RoleIDs, id)
		a.RoleCodes = append(a.RoleCodes, code)
		a.Super = a.Super || (super && !s.superOff.Load())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	prow, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT rp.permission_code FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id WHERE ur.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer prow.Close()
	for prow.Next() {
		var c string
		if err := prow.Scan(&c); err != nil {
			return nil, err
		}
		a.Perms[c] = true
	}
	return a, prow.Err()
}

// UserRoleRefs 返回每个用户的角色摘要，userIDs 为空时返回全部。
func (s *Store) UserRoleRefs(ctx context.Context) (map[int64][]RoleRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ur.user_id, r.id, r.code, r.name, r.is_super FROM user_roles ur JOIN roles r ON r.id = ur.role_id ORDER BY r.is_super DESC, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]RoleRef{}
	for rows.Next() {
		var uid int64
		var r RoleRef
		if err := rows.Scan(&uid, &r.ID, &r.Code, &r.Name, &r.IsSuper); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], r)
	}
	return out, rows.Err()
}

func (s *Store) SetUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_roles WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, id := range roleIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)", userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PermissionHolders 统计每个权限点有几个启用中的人（经由不是超级管理员的角色）持有。
func (s *Store) PermissionHolders(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT rp.permission_code, COUNT(DISTINCT u.id) FROM role_permissions rp
		 JOIN user_roles ur ON ur.role_id = rp.role_id JOIN users u ON u.id = ur.user_id
		 WHERE u.disabled = 0 GROUP BY rp.permission_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] = n
	}
	return out, rows.Err()
}

// CountSuperUsers 统计启用中的超级管理员人数，excludeUserID 不计入（用于「改完之后还剩几个」的判断）。
func (s *Store) CountSuperUsers(ctx context.Context, excludeUserID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT u.id) FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id JOIN roles r ON r.id = ur.role_id
		 WHERE r.is_super = 1 AND u.disabled = 0 AND u.id <> ?`, excludeUserID).Scan(&n)
	return n, err
}

// ---- 菜单 ----

type Menu struct {
	ID         int64     `json:"id"`
	ParentID   *int64    `json:"parent_id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Path       string    `json:"path"`
	PageKey    string    `json:"page_key"`
	Icon       string    `json:"icon"`
	Permission string    `json:"permission"`
	Sort       int       `json:"sort"`
	Visible    bool      `json:"visible"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const menuCols = "id, parent_id, kind, title, path, page_key, icon, permission, sort, visible, updated_at"

func scanMenu(row interface{ Scan(...any) error }) (*Menu, error) {
	var m Menu
	var parent sql.NullInt64
	var path, pageKey, icon, perm sql.NullString
	err := row.Scan(&m.ID, &parent, &m.Kind, &m.Title, &path, &pageKey, &icon, &perm, &m.Sort, &m.Visible, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if parent.Valid {
		v := parent.Int64
		m.ParentID = &v
	}
	m.Path, m.PageKey, m.Icon, m.Permission = path.String, pageKey.String, icon.String, perm.String
	return &m, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) ListMenus(ctx context.Context) ([]*Menu, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+menuCols+" FROM menus ORDER BY sort, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Menu
	for rows.Next() {
		m, err := scanMenu(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMenu(ctx context.Context, id int64) (*Menu, error) {
	return scanMenu(s.db.QueryRowContext(ctx, "SELECT "+menuCols+" FROM menus WHERE id = ?", id))
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertMenu(ctx context.Context, e execer, m *Menu) error {
	t := now()
	res, err := e.ExecContext(ctx,
		`INSERT INTO menus (parent_id, kind, title, path, page_key, icon, permission, sort, visible, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ParentID, m.Kind, m.Title, nullStr(m.Path), nullStr(m.PageKey), nullStr(m.Icon), nullStr(m.Permission), m.Sort, m.Visible, t, t)
	if err != nil {
		return err
	}
	m.ID, err = res.LastInsertId()
	m.UpdatedAt = t
	return err
}

func (s *Store) CreateMenu(ctx context.Context, m *Menu) error { return insertMenu(ctx, s.db, m) }

func (s *Store) UpdateMenu(ctx context.Context, m *Menu) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE menus SET parent_id = ?, kind = ?, title = ?, path = ?, page_key = ?, icon = ?, permission = ?, sort = ?, visible = ?, updated_at = ?
		 WHERE id = ?`,
		m.ParentID, m.Kind, m.Title, nullStr(m.Path), nullStr(m.PageKey), nullStr(m.Icon), nullStr(m.Permission), m.Sort, m.Visible, now(), m.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMenu 删除菜单；分组下还有子项时拒绝。
func (s *Store) DeleteMenu(ctx context.Context, id int64) error {
	var children int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM menus WHERE parent_id = ?", id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return fmt.Errorf("%w: 分组下还有 %d 个菜单，先移走或删除它们", ErrInUse, children)
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM menus WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type MenuOrder struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parent_id"`
	Sort     int    `json:"sort"`
}

func (s *Store) ReorderMenus(ctx context.Context, items []MenuOrder) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	for _, it := range items {
		if _, err := tx.ExecContext(ctx, "UPDATE menus SET parent_id = ?, sort = ?, updated_at = ? WHERE id = ?", it.ParentID, it.Sort, t, it.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetMenus 清空菜单并写入默认值。
func (s *Store) ResetMenus(ctx context.Context, defaults []rbac.Menu) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM menus"); err != nil {
		return err
	}
	var add func(items []rbac.Menu, parent *int64) error
	add = func(items []rbac.Menu, parent *int64) error {
		for i, d := range items {
			m := &Menu{ParentID: parent, Kind: d.Kind, Title: d.Title, Path: d.Path, PageKey: d.PageKey, Icon: d.Icon, Permission: d.Permission, Sort: (i + 1) * 10, Visible: !d.Hidden}
			if err := insertMenu(ctx, tx, m); err != nil {
				return err
			}
			if len(d.Children) > 0 {
				id := m.ID
				if err := add(d.Children, &id); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := add(defaults, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CountMenus(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM menus").Scan(&n)
	return n, err
}
