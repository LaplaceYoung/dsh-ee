package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 员工自己的连接凭据（user_secret）。内容由调用方用主密钥加密后传进来，这里只管存取。

// UserCredentialInfo 是给员工自己看的：填过没有、末尾几位、什么时候填的。
type UserCredentialInfo struct {
	Hint      string    `json:"hint"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CredentialHolder 是给管理员看的：谁填了、什么时候填的。不含凭据内容，也不含末尾几位。
type CredentialHolder struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Department  string    `json:"department"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) SetUserCredential(ctx context.Context, userID, assetID int64, enc []byte, hint string) error {
	t := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_credentials (user_id, asset_id, secret_enc, hint, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE secret_enc = VALUES(secret_enc), hint = VALUES(hint), updated_at = VALUES(updated_at)`,
		userID, assetID, enc, hint, t, t)
	return err
}

// UserCredential 取一位员工在一个连接上的凭据（密文）；没填过返回 nil。
func (s *Store) UserCredential(ctx context.Context, userID, assetID int64) ([]byte, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, "SELECT secret_enc FROM user_credentials WHERE user_id = ? AND asset_id = ?", userID, assetID).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return enc, err
}

// UserCredentialInfos 列出一位员工填过凭据的连接，按连接 id。
func (s *Store) UserCredentialInfos(ctx context.Context, userID int64) (map[int64]UserCredentialInfo, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT asset_id, hint, updated_at FROM user_credentials WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]UserCredentialInfo{}
	for rows.Next() {
		var id int64
		var c UserCredentialInfo
		if err := rows.Scan(&id, &c.Hint, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// DeleteUserCredential 删掉一位员工在一个连接上的凭据；本来就没有时返回 false。
func (s *Store) DeleteUserCredential(ctx context.Context, userID, assetID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM user_credentials WHERE user_id = ? AND asset_id = ?", userID, assetID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteUserCredentials 删掉一位员工的全部凭据（停用、离职时），返回删了几份。
func (s *Store) DeleteUserCredentials(ctx context.Context, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM user_credentials WHERE user_id = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteAssetCredentials 删掉一个连接上所有员工的凭据（连接不再用 user_secret 时），返回删了几份。
func (s *Store) DeleteAssetCredentials(ctx context.Context, assetID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM user_credentials WHERE asset_id = ?", assetID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CredentialHolders 列出在一个连接上填了凭据的员工，最近填的在前。
func (s *Store) CredentialHolders(ctx context.Context, assetID int64) ([]CredentialHolder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.user_id, u.username, u.display_name, u.department, c.updated_at
		FROM user_credentials c JOIN users u ON u.id = c.user_id
		WHERE c.asset_id = ? ORDER BY c.updated_at DESC, c.user_id`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CredentialHolder{}
	for rows.Next() {
		var h CredentialHolder
		if err := rows.Scan(&h.UserID, &h.Username, &h.DisplayName, &h.Department, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
