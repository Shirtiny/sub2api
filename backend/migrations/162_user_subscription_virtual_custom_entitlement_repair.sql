-- Idempotent repair for environments that applied an early version of
-- 161_user_subscription_virtual_custom_entitlement.sql before the file was finalized.
-- Keep 161 immutable; this migration carries the schema guarantees needed by code.

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS custom_multiplier INT NULL;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS custom_source_plan_id BIGINT NULL;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS custom_source_group_id BIGINT NULL;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS custom_expires_at TIMESTAMPTZ NULL;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS custom_display_name VARCHAR(120) NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_subscriptions_custom_multiplier_range_chk'
    ) THEN
        ALTER TABLE user_subscriptions
            ADD CONSTRAINT user_subscriptions_custom_multiplier_range_chk
            CHECK (custom_multiplier IS NULL OR (custom_multiplier >= 1 AND custom_multiplier <= 100))
            NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_subscriptions_custom_entitlement_complete_chk'
    ) THEN
        ALTER TABLE user_subscriptions
            ADD CONSTRAINT user_subscriptions_custom_entitlement_complete_chk
            CHECK (
                (custom_multiplier IS NULL AND custom_source_plan_id IS NULL AND custom_source_group_id IS NULL AND custom_expires_at IS NULL AND custom_display_name IS NULL)
                OR
                (custom_multiplier IS NOT NULL AND custom_source_plan_id IS NOT NULL AND custom_source_group_id IS NOT NULL AND custom_expires_at IS NOT NULL)
            )
            NOT VALID;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_custom_source_plan
    ON user_subscriptions(user_id, custom_source_plan_id, status, expires_at)
    WHERE deleted_at IS NULL
      AND custom_multiplier IS NOT NULL
      AND custom_source_plan_id IS NOT NULL
      AND custom_expires_at IS NOT NULL;
