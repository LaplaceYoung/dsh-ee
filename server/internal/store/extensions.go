package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ExtensionInstallation 是一个已安装的声明式扩展。Manifest 与 OpenAPI 保留安装时的原文，Digest 可据此核对。
// Config 只含非 secret 安装参数；SecretEnc 是整个 secret 配置对象的密文，不能序列化到接口。
type ExtensionInstallation struct {
	AssetID   int64     `json:"asset_id"`
	ID        string    `json:"extension_id"`
	Version   string    `json:"version"`
	Digest    string    `json:"digest"`
	Kind      string    `json:"kind"`
	Manifest  string    `json:"manifest"`
	OpenAPI   string    `json:"openapi"`
	Config    string    `json:"config"`
	SecretEnc []byte    `json:"-"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) CreateExtensionInstallation(ctx context.Context, in *ExtensionInstallation) error {
	t := now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO extension_installations
		(asset_id, extension_id, version, digest, kind, manifest, openapi, config, secret_enc, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.AssetID, in.ID, in.Version, in.Digest, in.Kind, in.Manifest, in.OpenAPI, in.Config, in.SecretEnc, in.CreatedBy, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

// ExtensionSecretConfig 取安装时的 secret 配置密文；未配置过则返回 nil。仅供运行时解密，不能用于接口响应。
func (s *Store) ExtensionSecretConfig(ctx context.Context, assetID int64) ([]byte, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, "SELECT secret_enc FROM extension_installations WHERE asset_id = ?", assetID).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return enc, err
}

// ExtensionInstallationOf 返回安装元数据，管理页显示身份、版本和 digest。
func (s *Store) ExtensionInstallationOf(ctx context.Context, assetID int64) (*ExtensionInstallation, error) {
	var in ExtensionInstallation
	var createdBy sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT asset_id, extension_id, version, digest, kind, manifest, openapi, config, created_by, created_at, updated_at
		FROM extension_installations WHERE asset_id = ?`, assetID).
		Scan(&in.AssetID, &in.ID, &in.Version, &in.Digest, &in.Kind, &in.Manifest, &in.OpenAPI, &in.Config, &createdBy, &in.CreatedAt, &in.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if createdBy.Valid {
		id := createdBy.Int64
		in.CreatedBy = &id
	}
	return &in, nil
}
