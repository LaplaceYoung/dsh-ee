-- API Key 绑定的上游 origin（scheme://host[:port]，如 https://api.deepseek.com）。
-- 守卫上线前，PATCH base_url 改 origin 且不传 api_key 时旧 key 会被保留并被发往新地址；
-- 这列让读取侧能识别「key 属于旧服务商」的历史记录：checkProvider 与网关等转发路径在
-- api_key_origin 与当前 base_url 的 origin 不一致时拒绝把 key 发出去。空串表示加列前的
-- 旧记录，读取侧不阻断（无法判定），管理员下次重填 api_key 时补写。
ALTER TABLE providers ADD COLUMN api_key_origin VARCHAR(255) NOT NULL DEFAULT '';
