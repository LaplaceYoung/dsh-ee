-- 用量细项：推理（思考）token 与命中前缀缓存的 prompt token。
-- 只加列、不改已有字段，老数据与没给细项的上游按 0 记；total_tokens 仍是上游返回的
-- 总量（推理 token 已含在 completion_tokens、缓存命中已含在 prompt_tokens 里，不重复计）。
ALTER TABLE usage_logs
  ADD COLUMN reasoning_tokens INT NOT NULL DEFAULT 0,
  ADD COLUMN cached_tokens    INT NOT NULL DEFAULT 0;
