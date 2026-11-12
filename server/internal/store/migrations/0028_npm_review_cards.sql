-- NPM 插件审批复合对象最长 343 字节：合法包名 214、分隔符 @ 1、semver 128。
-- 大字段只为保存复核对象，不能用无界联合索引；对象查询仍先用 kind 和前 64 个字符缩小范围。

ALTER TABLE feishu_card_actions MODIFY object_key VARCHAR(343) NOT NULL;

ALTER TABLE feishu_card_actions DROP INDEX idx_feishu_card_actions_object;

ALTER TABLE feishu_card_actions ADD KEY idx_feishu_card_actions_object_prefix (kind, object_key(64));
