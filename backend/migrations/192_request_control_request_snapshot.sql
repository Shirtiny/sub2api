-- Keep one row per request-control event so an administrator can correlate an
-- exact request ID/timestamp with its diagnostic request snapshot. Historical
-- rows deduplicated by migration 190 remain readable with event_count > 1.
DROP INDEX IF EXISTS uq_request_control_logs_dedupe;

CREATE INDEX IF NOT EXISTS idx_request_control_logs_fingerprints
    ON request_control_logs (user_id, protocol, request_headers_hash, request_body_hash, created_at DESC);

ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN request_control_logs.request_snapshot IS
    'Admin-only request context, redacted full headers, and bounded request body diagnostic snapshot';
