-- 飞书审批卡动作：随机 action id 只存哈希、状态与对象，不把操作权限寄托在按钮上。

CREATE TABLE feishu_card_actions (
  id          BIGINT        NOT NULL AUTO_INCREMENT,
  action_hash VARCHAR(64)   NOT NULL,
  kind        VARCHAR(32)   NOT NULL,
  object_key  VARCHAR(96)   NOT NULL,
  asset_id    BIGINT        NOT NULL,
  sub_id      BIGINT        NOT NULL DEFAULT 0,
  message_id  VARCHAR(128)  NOT NULL,
  status      VARCHAR(16)   NOT NULL,
  event_id    VARCHAR(128)  NOT NULL DEFAULT '',
  result      VARCHAR(256)  NOT NULL DEFAULT '',
  expires_at  DATETIME(3)   NOT NULL,
  completed_at DATETIME(3)  NULL,
  created_at  DATETIME(3)   NOT NULL,
  updated_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_feishu_card_actions_hash (action_hash),
  KEY idx_feishu_card_actions_object (kind, object_key),
  KEY idx_feishu_card_actions_status_expiry (status, expires_at)
);
