-- Durable per-user/API-key daily usage facts.
--
-- Unlike usage_dashboard_* tables, these rows intentionally survive usage_logs
-- retention cleanup so users can query calendar-month cost and token totals.
-- There are no foreign keys because deleting a user or key must not erase
-- historical totals.

CREATE TABLE IF NOT EXISTS user_api_key_usage_daily (
    bucket_date DATE NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    billing_type SMALLINT NOT NULL DEFAULT 0,
    request_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, user_id, api_key_id, billing_type)
);

CREATE INDEX IF NOT EXISTS idx_user_api_key_usage_daily_user_date
    ON user_api_key_usage_daily (user_id, bucket_date);

CREATE INDEX IF NOT EXISTS idx_user_api_key_usage_daily_key_date
    ON user_api_key_usage_daily (api_key_id, bucket_date);

-- Seed every source row still available at migration time. Older purged history
-- cannot be reconstructed, but subsequent aggregation cycles keep these facts.
INSERT INTO user_api_key_usage_daily (
    bucket_date,
    user_id,
    api_key_id,
    billing_type,
    request_count,
    input_tokens,
    output_tokens,
    cache_creation_tokens,
    cache_read_tokens,
    total_cost,
    actual_cost,
    total_duration_ms,
    computed_at
)
SELECT
    (ul.created_at AT TIME ZONE CURRENT_SETTING('TimeZone'))::date,
    ul.user_id,
    ul.api_key_id,
    CASE WHEN ul.billing_type = 1 OR ul.subscription_id IS NOT NULL THEN 1 ELSE 0 END,
    COUNT(*),
    COALESCE(SUM(ul.input_tokens), 0),
    COALESCE(SUM(ul.output_tokens), 0),
    COALESCE(SUM(ul.cache_creation_tokens), 0),
    COALESCE(SUM(ul.cache_read_tokens), 0),
    COALESCE(SUM(ul.total_cost), 0),
    COALESCE(SUM(ul.actual_cost), 0),
    COALESCE(SUM(COALESCE(ul.duration_ms, 0)), 0),
    NOW()
FROM usage_logs ul
WHERE ul.created_at < date_trunc('day', NOW())
GROUP BY
    (ul.created_at AT TIME ZONE CURRENT_SETTING('TimeZone'))::date,
    ul.user_id,
    ul.api_key_id,
    CASE WHEN ul.billing_type = 1 OR ul.subscription_id IS NOT NULL THEN 1 ELSE 0 END
ON CONFLICT (bucket_date, user_id, api_key_id, billing_type)
DO UPDATE SET
    request_count = EXCLUDED.request_count,
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    cache_creation_tokens = EXCLUDED.cache_creation_tokens,
    cache_read_tokens = EXCLUDED.cache_read_tokens,
    total_cost = EXCLUDED.total_cost,
    actual_cost = EXCLUDED.actual_cost,
    total_duration_ms = EXCLUDED.total_duration_ms,
    computed_at = NOW()
WHERE EXCLUDED.request_count > user_api_key_usage_daily.request_count;
