-- Approval is per email; never change the global registration setting.
-- Audit IDs deliberately survive account deletion: a consumed grant must not reopen.
ALTER TABLE waitlist_entries
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by BIGINT,
    ADD COLUMN IF NOT EXISTS granted_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS approval_notice_attempted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approval_notice_sent_at TIMESTAMPTZ;
