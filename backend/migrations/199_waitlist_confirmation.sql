-- Persist confirmation delivery state so retries do not resend successful mail.
-- Do not change original application timestamps or existing migration history.
ALTER TABLE waitlist_entries
    ADD COLUMN IF NOT EXISTS confirmation_attempted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS confirmation_sent_at TIMESTAMPTZ;
