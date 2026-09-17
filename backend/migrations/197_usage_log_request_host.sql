-- Preserve the client-facing hostname independently from API endpoint paths.
-- Historical logs remain NULL: the original ingress cannot be inferred reliably.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_host VARCHAR(253);

COMMENT ON COLUMN usage_logs.request_host IS 'Normalized client-facing ingress hostname; NULL for historical rows';
