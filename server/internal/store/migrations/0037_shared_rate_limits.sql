-- 公开协议面（/v1/* 与 /mcp）的每令牌限流从进程内固定窗口改成数据库共享固定窗口：
-- N 个实例连同一个库时共用同一份额度，不再因为多实例部署把额度放大 N 倍。
-- key 是令牌的 sha256（没有令牌时退回客户端 IP 的摘要），不存令牌明文；
-- 窗口按配置长度对齐（线上默认 1 分钟），依赖各实例时钟同步（部署时开 NTP）。
-- 旧窗口由后台按窗口边界清理，判定不依赖清理任务本身。
CREATE TABLE shared_rate_limits (
  rate_key     VARCHAR(191) NOT NULL,
  window_start DATETIME(3)  NOT NULL,
  count        INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (rate_key, window_start),
  KEY idx_shared_rate_limits_window (window_start)
);
