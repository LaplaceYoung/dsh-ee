-- v0.2 第 1 批：声明式扩展安装。元数据与资产关联，删除连接时一起清理。

CREATE TABLE extension_installations (
  asset_id    BIGINT        NOT NULL,
  extension_id VARCHAR(128) NOT NULL,
  version     VARCHAR(32)   NOT NULL,
  digest      VARCHAR(64)   NOT NULL,
  kind        VARCHAR(16)   NOT NULL,
  manifest    MEDIUMTEXT    NOT NULL,
  openapi     MEDIUMTEXT    NOT NULL,
  -- 非 secret 安装配置；secret 配置整体加密后放 secret_enc，不进接口响应
  config      TEXT          NOT NULL,
  secret_enc  VARBINARY(4096) NULL,
  created_by  BIGINT        NULL,
  created_at  DATETIME(3)   NOT NULL,
  updated_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (asset_id),
  UNIQUE KEY uk_extension_installations_id (extension_id)
);

-- 扩展调用可追溯到具体清单版本与内容摘要。
ALTER TABLE connection_calls ADD COLUMN extension_id VARCHAR(128) NOT NULL DEFAULT '' AFTER auth_mode;
ALTER TABLE connection_calls ADD COLUMN extension_version VARCHAR(32) NOT NULL DEFAULT '' AFTER extension_id;
ALTER TABLE connection_calls ADD COLUMN extension_digest VARCHAR(64) NOT NULL DEFAULT '' AFTER extension_version;
