-- Keep hit and automatic-ban notification outcomes distinct in the admin log.
ALTER TABLE request_control_logs
    ADD COLUMN IF NOT EXISTS hit_email_sent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ban_email_sent BOOLEAN NOT NULL DEFAULT FALSE;
