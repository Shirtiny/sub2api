-- Preserve the frequency and time range hidden by request fingerprint
-- deduplication. A row still represents one stable request shape, while these
-- fields show how many events it aggregates and when that shape first/last
-- appeared.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS event_count BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS first_seen_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ;

UPDATE request_control_logs
SET first_seen_at = COALESCE(first_seen_at, created_at),
    last_seen_at = COALESCE(last_seen_at, created_at)
WHERE first_seen_at IS NULL OR last_seen_at IS NULL;

ALTER TABLE request_control_logs
    ALTER COLUMN first_seen_at SET DEFAULT NOW(),
    ALTER COLUMN first_seen_at SET NOT NULL,
    ALTER COLUMN last_seen_at SET DEFAULT NOW(),
    ALTER COLUMN last_seen_at SET NOT NULL;

COMMENT ON COLUMN request_control_logs.event_count IS
    'Number of request events aggregated into this deduplicated row';
COMMENT ON COLUMN request_control_logs.first_seen_at IS
    'Earliest event timestamp represented by this deduplicated row';
COMMENT ON COLUMN request_control_logs.last_seen_at IS
    'Latest event timestamp represented by this deduplicated row';
