package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SharedCredential 是一份命名共享凭据。只有尾号和元数据能离开存储层；enc 仅供运行时解密。
type SharedCredential struct {
	AssetID    int64     `json:"asset_id"`
	Name       string    `json:"name"`
	SecretEnc  []byte    `json:"-"`
	SecretHint string    `json:"secret_hint"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func scanSharedCredential(row interface{ Scan(...any) error }, encrypted bool) (*SharedCredential, error) {
	var c SharedCredential
	var err error
	if encrypted {
		err = row.Scan(&c.AssetID, &c.Name, &c.SecretEnc, &c.SecretHint, &c.CreatedAt, &c.UpdatedAt)
	} else {
		err = row.Scan(&c.AssetID, &c.Name, &c.SecretHint, &c.CreatedAt, &c.UpdatedAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) SaveSharedCredential(ctx context.Context, c *SharedCredential) error {
	now := now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO connection_shared_credentials (asset_id, name, secret_enc, secret_hint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE secret_enc = VALUES(secret_enc), secret_hint = VALUES(secret_hint), updated_at = VALUES(updated_at)`,
		c.AssetID, c.Name, c.SecretEnc, c.SecretHint, now, now)
	return err
}

func (s *Store) SharedCredentialSecret(ctx context.Context, assetID int64, name string) (*SharedCredential, error) {
	return scanSharedCredential(s.db.QueryRowContext(ctx, "SELECT asset_id, name, secret_enc, secret_hint, created_at, updated_at FROM connection_shared_credentials WHERE asset_id = ? AND name = ?", assetID, name), true)
}

// ListSharedCredentials 只返回尾号与时间，不取密文。
func (s *Store) ListSharedCredentials(ctx context.Context, assetID int64) ([]SharedCredential, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT asset_id, name, secret_hint, created_at, updated_at FROM connection_shared_credentials WHERE asset_id = ? ORDER BY name", assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SharedCredential{}
	for rows.Next() {
		c, err := scanSharedCredential(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSharedCredential(ctx context.Context, assetID int64, name string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM connection_shared_credentials WHERE asset_id = ? AND name = ?", assetID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
