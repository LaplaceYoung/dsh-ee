-- 作业级归属：多实例下启动时只把本实例（以及升级前无归属）的 running 任务标中断，
-- 不再误伤其它在线实例正在跑的任务。
ALTER TABLE sync_jobs ADD COLUMN owner VARCHAR(191) NOT NULL DEFAULT '';
ALTER TABLE sync_jobs ADD INDEX idx_sync_jobs_owner_status (owner, status);
