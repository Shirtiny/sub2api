-- User-initiated daily/weekly quota reset allowances.
-- A value represents remaining resets; zero keeps the feature disabled.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS reset_count INT NOT NULL DEFAULT 0;

-- Keep malformed legacy/manual values from becoming an unbounded allowance.
UPDATE user_subscriptions
SET reset_count = 0
WHERE reset_count IS NULL OR reset_count < 0 OR reset_count > 1000;

ALTER TABLE user_subscriptions
    ALTER COLUMN reset_count SET DEFAULT 0,
    ALTER COLUMN reset_count SET NOT NULL;

ALTER TABLE user_subscriptions
    DROP CONSTRAINT IF EXISTS user_subscriptions_reset_count_valid;

ALTER TABLE user_subscriptions
    ADD CONSTRAINT user_subscriptions_reset_count_valid
    CHECK (reset_count >= 0 AND reset_count <= 1000);
