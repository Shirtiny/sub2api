-- Store bounded, redacted request metadata for on-demand admin inspection.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS request_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS request_body_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
