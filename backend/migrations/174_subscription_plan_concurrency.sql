ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS concurrency INT NOT NULL DEFAULT 1;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS plan_concurrency INT,
    ADD COLUMN IF NOT EXISTS plan_concurrency_expires_at TIMESTAMPTZ;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_concurrency INT;
