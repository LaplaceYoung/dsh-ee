package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ---- 权限申请：连接、技能、知识库 ----
//
// 表名沿用最早的 connection_requests（迁移 0019 加了 kind、app_id、note）。
// kind 为 connection / skill 时 asset_id 指能力，为 knowledge 时指知识库。

const (
	RequestConnection = "connection"
	RequestSkill      = "skill"
	RequestKnowledge  = "knowledge"
)

// AccessRequest 是一条申请，带上目标与应用的名称，方便直接显示。
type AccessRequest struct {
	ID         int64      `json:"id"`
	Kind       string     `json:"kind"`
	UserID     int64      `json:"user_id"`
	Username   string     `json:"username"`
	UserName   string     `json:"user_name"`
	Department string     `json:"department"`
	AssetID    int64      `json:"asset_id"`
	Slug       string     `json:"slug"`
	Title      string     `json:"title"`
	AppID      int64      `json:"app_id"`
	AppSlug    string     `json:"app_slug"`
	AppTitle   string     `json:"app_title"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	Note       string     `json:"note"`
	HandledBy  *int64     `json:"handled_by"`
	HandledAt  *time.Time `json:"handled_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// 目标的标识和名称按 kind 从能力表或知识库表里取；目标被删了就是空的。
const accessRequestSelect = `SELECT r.id, r.kind, r.user_id, u.username, u.display_name, u.department, r.asset_id,
	COALESCE(CASE WHEN r.kind = 'knowledge' THEN b.slug ELSE a.slug END, ''),
	COALESCE(CASE WHEN r.kind = 'knowledge' THEN b.title ELSE a.title END, ''),
	r.app_id, COALESCE(p.slug, ''), COALESCE(p.title, ''), r.reason, r.status, r.note, r.handled_by, r.handled_at, r.created_at
	FROM connection_requests r
	JOIN users u ON u.id = r.user_id
	LEFT JOIN assets a ON r.kind <> 'knowledge' AND a.id = r.asset_id
	LEFT JOIN kb_bases b ON r.kind = 'knowledge' AND b.id = r.asset_id
	LEFT JOIN assets p ON p.id = r.app_id AND r.app_id > 0`

func scanAccessRequests(rows *sql.Rows) ([]*AccessRequest, error) {
	defer rows.Close()
	out := []*AccessRequest{}
	for rows.Next() {
		var c AccessRequest
		var by sql.NullInt64
		var at sql.NullTime
		if err := rows.Scan(&c.ID, &c.Kind, &c.UserID, &c.Username, &c.UserName, &c.Department, &c.AssetID, &c.Slug, &c.Title,
			&c.AppID, &c.AppSlug, &c.AppTitle, &c.Reason, &c.Status, &c.Note, &by, &at, &c.CreatedAt); err != nil {
			return nil, err
		}
		if by.Valid {
			c.HandledBy = &by.Int64
		}
		if at.Valid {
			c.HandledAt = &at.Time
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CreateAccessRequest 登记申请；同一人对同一目标已有待处理的申请时返回它，不重复建
// （带了应用而原来那条没带时，补上应用）。
func (s *Store) CreateAccessRequest(ctx context.Context, userID int64, kind string, targetID, appID int64, reason string) (id int64, existed bool, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT id FROM connection_requests WHERE user_id = ? AND kind = ? AND asset_id = ? AND status = 'pending' ORDER BY id DESC LIMIT 1",
		userID, kind, targetID).Scan(&id)
	if err == nil {
		if appID > 0 {
			_, err = s.db.ExecContext(ctx, "UPDATE connection_requests SET app_id = ? WHERE id = ? AND app_id = 0", appID, id)
		}
		return id, true, err
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO connection_requests (user_id, kind, asset_id, app_id, reason, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)",
		userID, kind, targetID, appID, truncateRunes(reason, 500), now())
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	return id, false, err
}

// UserAccessRequests 是这个人提交过的申请，新的在前。
func (s *Store) UserAccessRequests(ctx context.Context, userID int64, limit int) ([]*AccessRequest, error) {
	rows, err := s.db.QueryContext(ctx, accessRequestSelect+" WHERE r.user_id = ? ORDER BY r.id DESC LIMIT ?", userID, limit)
	if err != nil {
		return nil, err
	}
	return scanAccessRequests(rows)
}

// PendingAccessRequests 是待处理的申请，早的在前；kinds 为空表示全部类型。
func (s *Store) PendingAccessRequests(ctx context.Context, limit int, kinds ...string) ([]*AccessRequest, error) {
	q, args := accessRequestSelect+" WHERE r.status = 'pending'", []any{}
	if len(kinds) > 0 {
		q += " AND r.kind IN (?" + strings.Repeat(", ?", len(kinds)-1) + ")"
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY r.id LIMIT ?", append(args, limit)...)
	if err != nil {
		return nil, err
	}
	return scanAccessRequests(rows)
}

// CountPendingAccessRequests 数待处理的申请；kinds 为空表示全部类型。
func (s *Store) CountPendingAccessRequests(ctx context.Context, kinds ...string) (int, error) {
	q, args := "SELECT COUNT(*) FROM connection_requests WHERE status = 'pending'", []any{}
	if len(kinds) > 0 {
		q += " AND kind IN (?" + strings.Repeat(", ?", len(kinds)-1) + ")"
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	var n int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// GetAccessRequest 取一条申请；不存在返回 ErrNotFound。
func (s *Store) GetAccessRequest(ctx context.Context, id int64) (*AccessRequest, error) {
	rows, err := s.db.QueryContext(ctx, accessRequestSelect+" WHERE r.id = ?", id)
	if err != nil {
		return nil, err
	}
	out, err := scanAccessRequests(rows)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out[0], nil
}

// GetPendingAccessRequest 取一条待处理的申请；不存在或已处理返回 ErrNotFound。
func (s *Store) GetPendingAccessRequest(ctx context.Context, id int64) (*AccessRequest, error) {
	c, err := s.GetAccessRequest(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != "pending" {
		return nil, ErrNotFound
	}
	return c, nil
}

// ResolveAccessRequest 把待处理的申请标为已通过 / 已驳回；已处理过的返回 ErrNotFound（并发处理只有一次成功）。
func (s *Store) ResolveAccessRequest(ctx context.Context, id int64, status string, by int64, note string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE connection_requests SET status = ?, handled_by = ?, handled_at = ?, note = ? WHERE id = ? AND status = 'pending'",
		status, by, now(), truncateRunes(note, 500), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReopenAccessRequest 把已经通过、但授权失败的申请退回待处理，让审批人可以把授权重做一遍。
// 只退回 status = 'approved' 的记录：申请一旦被驳回就保持驳回，不会被这里意外打开。
// 返回 ErrNotFound 表示没有可退回的申请（例如期间被别人改过），调用方记日志按「回滚没做到」处理。
func (s *Store) ReopenAccessRequest(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE connection_requests SET status = 'pending', handled_by = NULL, handled_at = NULL, note = '' WHERE id = ? AND status = 'approved'",
		id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
