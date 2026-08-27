-- Migration 192 temporarily removed fingerprint aggregation. Restore the
-- bounded row cardinality while retaining the latest diagnostic snapshot per
-- fingerprint. The merge also makes this safe if a shared environment briefly
-- accepted more than one row for the same fingerprint.
WITH grouped AS (
    SELECT
        user_id,
        protocol,
        request_headers_hash,
        request_body_hash,
        (ARRAY_AGG(id ORDER BY created_at DESC, id DESC))[1] AS keeper_id,
        SUM(GREATEST(event_count, 1)) AS event_count,
        MIN(first_seen_at) AS first_seen_at,
        MAX(last_seen_at) AS last_seen_at,
        MAX(violation_count) AS violation_count,
        BOOL_OR(counted_violation) AS counted_violation,
        BOOL_OR(email_sent) AS email_sent,
        BOOL_OR(hit_email_sent) AS hit_email_sent,
        BOOL_OR(ban_email_sent) AS ban_email_sent,
        BOOL_OR(auto_banned) AS auto_banned
    FROM request_control_logs
    WHERE user_id IS NOT NULL
      AND request_headers_hash <> ''
      AND request_body_hash <> ''
    GROUP BY user_id, protocol, request_headers_hash, request_body_hash
    HAVING COUNT(*) > 1
)
UPDATE request_control_logs AS keeper
SET event_count = grouped.event_count,
    first_seen_at = grouped.first_seen_at,
    last_seen_at = grouped.last_seen_at,
    violation_count = grouped.violation_count,
    counted_violation = grouped.counted_violation,
    email_sent = grouped.email_sent,
    hit_email_sent = grouped.hit_email_sent,
    ban_email_sent = grouped.ban_email_sent,
    auto_banned = grouped.auto_banned
FROM grouped
WHERE keeper.id = grouped.keeper_id;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY user_id, protocol, request_headers_hash, request_body_hash
            ORDER BY created_at DESC, id DESC
        ) AS row_number
    FROM request_control_logs
    WHERE user_id IS NOT NULL
      AND request_headers_hash <> ''
      AND request_body_hash <> ''
)
DELETE FROM request_control_logs AS duplicate
USING ranked
WHERE duplicate.id = ranked.id
  AND ranked.row_number > 1;

DROP INDEX IF EXISTS idx_request_control_logs_fingerprints;

CREATE UNIQUE INDEX IF NOT EXISTS uq_request_control_logs_dedupe
    ON request_control_logs (user_id, protocol, request_headers_hash, request_body_hash)
    WHERE user_id IS NOT NULL AND request_headers_hash <> '' AND request_body_hash <> '';
