ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_bonus_activity_id BIGINT,
    ADD COLUMN IF NOT EXISTS subscription_bonus_days INT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'payment_orders_subscription_bonus_days_valid'
          AND conrelid = 'payment_orders'::regclass
    ) THEN
        ALTER TABLE payment_orders
            ADD CONSTRAINT payment_orders_subscription_bonus_days_valid
            CHECK (subscription_bonus_days >= 0 AND subscription_bonus_days <= 36500);
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS promotion_activities (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    activity_type VARCHAR(50) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    max_uses_per_user INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT promotion_activities_window_valid CHECK (starts_at < ends_at),
    CONSTRAINT promotion_activities_max_uses_valid CHECK (max_uses_per_user > 0 AND max_uses_per_user <= 1000)
);

CREATE INDEX IF NOT EXISTS idx_promotion_activities_active_window
    ON promotion_activities(activity_type, enabled, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS promotion_activity_plans (
    id BIGSERIAL PRIMARY KEY,
    activity_id BIGINT NOT NULL REFERENCES promotion_activities(id),
    plan_id BIGINT NOT NULL REFERENCES subscription_plans(id),
    bonus_days INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT promotion_activity_plans_bonus_days_valid CHECK (bonus_days > 0 AND bonus_days <= 36500),
    CONSTRAINT promotion_activity_plans_activity_plan_unique UNIQUE (activity_id, plan_id)
);

CREATE INDEX IF NOT EXISTS idx_promotion_activity_plans_plan
    ON promotion_activity_plans(plan_id);

CREATE TABLE IF NOT EXISTS promotion_activity_participations (
    id BIGSERIAL PRIMARY KEY,
    activity_id BIGINT NOT NULL REFERENCES promotion_activities(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    order_id BIGINT NOT NULL REFERENCES payment_orders(id),
    plan_id BIGINT NOT NULL,
    bonus_days INT NOT NULL,
    status VARCHAR(20) NOT NULL,
    reserved_at TIMESTAMPTZ NOT NULL,
    granted_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    release_reason VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT promotion_activity_participations_order_unique UNIQUE (order_id),
    CONSTRAINT promotion_activity_participations_bonus_days_valid CHECK (bonus_days > 0 AND bonus_days <= 36500),
    CONSTRAINT promotion_activity_participations_status_valid CHECK (status IN ('reserved', 'granted', 'released'))
);

CREATE INDEX IF NOT EXISTS idx_promotion_activity_participations_activity_user_status
    ON promotion_activity_participations(activity_id, user_id, status);
CREATE INDEX IF NOT EXISTS idx_promotion_activity_participations_user_status
    ON promotion_activity_participations(user_id, status);
