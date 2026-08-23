-- Store the expected policy outcome and stable request fingerprints.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS expected_action VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS expected_reason VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS expected_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS expected_status_code INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS request_headers_hash CHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_body_hash CHAR(64) NOT NULL DEFAULT '';

-- Existing rows predate fingerprints. The partial index leaves them intact;
-- Repeated fingerprints update the single aggregate row rather than creating
-- another record; the service keeps violation_count and the latest timestamp.
CREATE UNIQUE INDEX IF NOT EXISTS uq_request_control_logs_dedupe
    ON request_control_logs (user_id, protocol, request_headers_hash, request_body_hash)
    WHERE user_id IS NOT NULL AND request_headers_hash <> '' AND request_body_hash <> '';

CREATE INDEX IF NOT EXISTS idx_request_control_logs_expected
    ON request_control_logs(expected_blocked, created_at DESC, id DESC);

-- Keep rolling hit timestamps independently of the deduplicated observation
-- row. This preserves the 5-minute spacing and violation-window semantics.
CREATE TABLE IF NOT EXISTS request_control_violation_states (
    user_id      BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    hit_times    JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_hit_at  TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Preserve the counted history that was recorded before this state table
-- existed. The query is intentionally idempotent and does not overwrite a
-- state created by a concurrent application startup.
WITH recent_hits AS (
    SELECT user_id, created_at,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at DESC) AS rn
    FROM request_control_logs
    WHERE user_id IS NOT NULL AND blocked = TRUE AND counted_violation = TRUE
), counted_state AS (
    SELECT user_id,
           jsonb_agg((FLOOR(EXTRACT(EPOCH FROM created_at) * 1000)::BIGINT) ORDER BY created_at) AS hit_times
    FROM recent_hits
    WHERE rn <= 10000
    GROUP BY user_id
), last_hits AS (
    SELECT user_id, MAX(created_at) AS last_hit_at
    FROM request_control_logs
    WHERE user_id IS NOT NULL AND blocked = TRUE
    GROUP BY user_id
)
INSERT INTO request_control_violation_states (user_id, hit_times, last_hit_at)
SELECT
    last_hits.user_id,
    COALESCE(counted_state.hit_times, '[]'::jsonb),
    last_hits.last_hit_at
FROM last_hits
LEFT JOIN counted_state ON counted_state.user_id = last_hits.user_id
ON CONFLICT (user_id) DO NOTHING;
