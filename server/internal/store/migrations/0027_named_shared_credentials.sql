-- 多份命名共享凭据。旧连接的 assets.secret_enc 仍是缺省共享凭据；未选 name 时完全沿用旧行为。

CREATE TABLE connection_shared_credentials (
  asset_id    BIGINT        NOT NULL,
  name        VARCHAR(40)   NOT NULL,
  secret_enc  VARBINARY(4096) NOT NULL,
  secret_hint VARCHAR(16)   NOT NULL DEFAULT '',
  created_at  DATETIME(3)   NOT NULL,
  updated_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (asset_id, name),
  KEY idx_connection_shared_credentials_updated (asset_id, updated_at)
);

-- 审计记 selected shared credential；空代表旧默认凭据，不能反推 secret 内容。
ALTER TABLE connection_calls ADD COLUMN shared_name VARCHAR(40) NOT NULL DEFAULT '' AFTER auth_mode;
