-- Persist request-control hit accounting and notification outcomes.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS violation_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS counted_violation BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS email_sent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS auto_banned BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_request_control_logs_violation_state
    ON request_control_logs(user_id, blocked, created_at DESC, counted_violation);
