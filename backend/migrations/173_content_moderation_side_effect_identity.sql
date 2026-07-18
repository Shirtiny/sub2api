-- Persist explicit idempotency evidence for content-moderation side effects.
-- Historical rows intentionally remain side_effects_applied = FALSE because
-- legacy violation_count values cannot distinguish real hits from the repeated
-- keyword-email storm that this migration follows.

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS input_hash VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS side_effects_applied BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_content_moderation_logs_user_applied_created_at
    ON content_moderation_logs(user_id, created_at DESC)
    WHERE side_effects_applied = TRUE;
