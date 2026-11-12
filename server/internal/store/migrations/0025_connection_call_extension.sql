-- 扩展装进来的连接（docs/plugins.md §5.1、§7.6）：每次调用记下扩展的版本和清单摘要，
-- 形如 acme/oa@1.2.0#3f2a9c01b7de；不是扩展装的连接为空。
ALTER TABLE connection_calls ADD COLUMN extension VARCHAR(160) NOT NULL DEFAULT '';
