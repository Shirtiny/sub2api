-- Bound legacy/intermediate snapshots and track when the latest retained sample
-- was written so hot fingerprints do not rewrite a large JSONB/TOAST value on
-- every occurrence.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS request_snapshot_at TIMESTAMPTZ;

UPDATE request_control_logs
SET request_snapshot = '{}'::jsonb,
    request_snapshot_at = NULL
WHERE octet_length(COALESCE(request_snapshot->>'body', '')) > 262144
   OR pg_column_size(request_snapshot) > 1048576;

UPDATE request_control_logs
SET request_snapshot_at = created_at
WHERE request_snapshot_at IS NULL
  AND request_snapshot <> '{}'::jsonb;

COMMENT ON COLUMN request_control_logs.request_snapshot_at IS
    'Timestamp of the latest retained request snapshot sample; refreshed at most once per 15 minutes';
