-- 按「订阅 × 自然日」持久化用量汇总，供管理端订阅统计的每天/每周/整周期使用率使用。
--
-- 背景：per-user/per-subscription 的历史用量此前无处可查。
--   * usage_logs 是唯一持有 subscription_id + actual_cost 的表，但它受
--     dashboard_aggregation.retention.usage_logs_days 裁剪，线上实际只剩数天。
--   * usage_dashboard_daily 只有全站汇总，没有 user/subscription 维度。
--   * usage_dashboard_daily_users / hourly_users 只是活跃用户集合，不含金额。
--   * billing_usage_entries 结构合适但线上为空，该写入路径未启用。
--
-- 因此新建独立汇总表，由 DashboardAggregationService 的增量循环顺带写入，
-- 保留期与 usage_logs 解耦（见 dashboard_aggregation.retention.subscription_daily_days）。
--
-- 刻意不加 user_subscriptions/users/groups 外键：这张表的价值就在于订阅被撤销或
-- 删除后历史仍可回溯，级联删除会毁掉它。与 usage_dashboard_* 系列汇总表同样处理。
--
-- limit 三列是「写入当天的有效限额快照」（已含 custom_multiplier 倍率），
-- 这样套餐改价后历史使用率的分母不会跟着漂移。

CREATE TABLE IF NOT EXISTS subscription_usage_daily (
    subscription_id   BIGINT         NOT NULL,
    bucket_date       DATE           NOT NULL,
    user_id           BIGINT         NOT NULL,
    group_id          BIGINT,
    cost_usd          NUMERIC(20, 10) NOT NULL DEFAULT 0,
    request_count     BIGINT         NOT NULL DEFAULT 0,
    daily_limit_usd   NUMERIC(20, 8),
    weekly_limit_usd  NUMERIC(20, 8),
    monthly_limit_usd NUMERIC(20, 8),
    computed_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subscription_id, bucket_date)
);

CREATE INDEX IF NOT EXISTS idx_subscription_usage_daily_bucket_date
    ON subscription_usage_daily (bucket_date);

CREATE INDEX IF NOT EXISTS idx_subscription_usage_daily_user_date
    ON subscription_usage_daily (user_id, bucket_date);

CREATE INDEX IF NOT EXISTS idx_subscription_usage_daily_group_date
    ON subscription_usage_daily (group_id, bucket_date)
    WHERE group_id IS NOT NULL;
