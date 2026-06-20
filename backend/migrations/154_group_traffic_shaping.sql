-- 154_group_traffic_shaping.sql
-- 为 groups 表新增 4 个流量伪装行为开关字段。
--
-- 用途：把某些分组配置成"对外友好型"，让第三方聚合站（如 axonhub）感知到的 TTFT
--      更低、TPS 更稳、成功率更高，同时不影响其他分组的真实流量。
--
-- 默认全部 false：现有所有分组、新建分组的行为完全不变。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS traffic_shaping_enabled       BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS sse_early_keepalive_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS stream_smoothing_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS silent_retry_5xx_enabled      BOOLEAN NOT NULL DEFAULT FALSE;
