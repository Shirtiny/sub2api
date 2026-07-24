ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_byte_ms INT;
