ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS custom_multiplier_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS custom_multiplier_min INT NOT NULL DEFAULT 1;

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS custom_multiplier_max INT NOT NULL DEFAULT 1;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS is_custom_subscription_group BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS custom_owner_user_id BIGINT NULL;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS custom_source_plan_id BIGINT NULL;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS custom_source_group_id BIGINT NULL;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS custom_multiplier INT NULL;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_multiplier INT NULL;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_source_group_id BIGINT NULL;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_source_price DECIMAL(20,2) NULL;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_source_original_price DECIMAL(20,2) NULL;

CREATE INDEX IF NOT EXISTS idx_groups_is_custom_subscription_group
    ON groups(is_custom_subscription_group)
    WHERE deleted_at IS NULL;


CREATE INDEX IF NOT EXISTS idx_groups_custom_owner_plan
    ON groups(custom_owner_user_id, custom_source_plan_id)
    WHERE deleted_at IS NULL
      AND is_custom_subscription_group = TRUE
      AND custom_owner_user_id IS NOT NULL
      AND custom_source_plan_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_payment_orders_custom_subscription_pending
    ON payment_orders(user_id, plan_id, status, expires_at)
    WHERE subscription_multiplier >= 1;
