ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS early_reset_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS early_reset_duration_days INT NOT NULL DEFAULT 1;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_early_reset_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS subscription_early_reset_duration_days INT NOT NULL DEFAULT 0;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS early_reset_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS early_reset_duration_days INT NOT NULL DEFAULT 0;
