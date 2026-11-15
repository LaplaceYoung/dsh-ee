-- 企业目录条目 v1.1（format 2 additive）：条目可以声明 auto_upgrade（可选，缺省 false）。
-- 本期只登记 / 校验这个声明（GET /api/admin/directory/updates 做只读更新检查，
-- 设置 directory.auto_upgrade 默认 false），**不执行**自动升级：真正升级仍要管理员显式触发。
-- 只增列、不改已有列，旧目录（没有这个字段）apply 后这一列是默认值 0。

ALTER TABLE directory_entries
  ADD COLUMN auto_upgrade TINYINT(1) NOT NULL DEFAULT 0 AFTER size;
