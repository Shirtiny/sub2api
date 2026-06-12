-- Store the exact keyword that triggered a keyword pre-block.

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(255) NOT NULL DEFAULT '';

