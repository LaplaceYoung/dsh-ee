-- 按员工本人的身份调用企业系统（docs/plugins.md §7）。
-- user_credentials：员工自己填的连接凭据（user_secret），按「员工 + 连接」一份，主密钥加密存放。
-- 管理员只能看到谁填了、什么时候填的，看不到内容。
CREATE TABLE user_credentials (
  user_id    BIGINT          NOT NULL,
  asset_id   BIGINT          NOT NULL,
  secret_enc VARBINARY(4096) NOT NULL,
  hint       VARCHAR(16)     NOT NULL DEFAULT '',
  created_at DATETIME(3)     NOT NULL,
  updated_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (user_id, asset_id),
  KEY idx_user_credentials_asset (asset_id)
);

-- 每次调用记下用的是哪种员工身份（空 = 只有共享凭据 / ee_jwt / user_secret），审计时能分清
ALTER TABLE connection_calls ADD COLUMN auth_mode VARCHAR(16) NOT NULL DEFAULT '';
