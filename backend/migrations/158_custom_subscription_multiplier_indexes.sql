-- Keep these index creations separate from 157 because 157 was already applied in some environments before the index follow-up.
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
    WHERE subscription_multiplier >= 2;
