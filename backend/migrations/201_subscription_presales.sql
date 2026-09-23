-- Opt-in only: upgrading never starts a sale or changes existing subscriptions.
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS presale_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS presale_visible boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS presale_badge varchar(40) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS presale_reset_cards integer NOT NULL DEFAULT 0 CHECK (presale_reset_cards BETWEEN 0 AND 1000);

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS presale_starts_at timestamptz,
    ADD COLUMN IF NOT EXISTS presale_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS presale_activated_at timestamptz,
    ADD COLUMN IF NOT EXISTS presale_subscription_id bigint,
    ADD COLUMN IF NOT EXISTS presale_renewal boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS presale_plan_name varchar(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS presale_reset_cards integer NOT NULL DEFAULT 0 CHECK (presale_reset_cards BETWEEN 0 AND 1000);

ALTER TABLE payment_orders ADD CONSTRAINT payment_presale_window_valid CHECK (
    (presale_starts_at IS NULL AND presale_expires_at IS NULL AND presale_activated_at IS NULL)
    OR (presale_starts_at IS NOT NULL AND presale_expires_at IS NOT NULL
        AND presale_expires_at > presale_starts_at AND order_type = 'subscription')
);
CREATE INDEX IF NOT EXISTS paymentorder_presale_starts_at_status ON payment_orders (presale_starts_at, status);
