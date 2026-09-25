-- Additive activity support only. No promotion is enabled and no money is granted.
ALTER TABLE promotion_activity_plans ADD COLUMN bonus_balance DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE promotion_activity_participations
    ADD COLUMN bonus_balance DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN balance_reclaimed_at TIMESTAMPTZ;
ALTER TABLE payment_orders
    ADD COLUMN presale_balance_bonus_activity_id BIGINT,
    ADD COLUMN presale_balance_bonus_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN presale_balance_bonus_rate DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD CONSTRAINT payment_orders_balance_bonus_valid CHECK (
        (presale_balance_bonus_activity_id IS NULL AND presale_balance_bonus_amount = 0 AND presale_balance_bonus_rate = 0)
        OR (presale_balance_bonus_activity_id IS NOT NULL AND presale_balance_bonus_activity_id > 0 AND presale_starts_at IS NOT NULL
            AND presale_balance_bonus_amount > 0 AND presale_balance_bonus_amount <= 1000000
            AND presale_balance_bonus_rate > 0 AND presale_balance_bonus_rate < 1000000000000));
ALTER TABLE promotion_activity_plans DROP CONSTRAINT promotion_activity_plans_bonus_days_valid;
ALTER TABLE promotion_activity_plans ADD CONSTRAINT promotion_activity_plans_benefit_valid CHECK (
    (bonus_days > 0 AND bonus_days <= 36500 AND bonus_balance = 0)
    OR (bonus_days = 0 AND bonus_balance > 0 AND bonus_balance <= 1000000));
ALTER TABLE promotion_activity_participations DROP CONSTRAINT promotion_activity_participations_bonus_days_valid;
ALTER TABLE promotion_activity_participations ADD CONSTRAINT promotion_activity_participations_benefit_valid CHECK (
    (bonus_days > 0 AND bonus_days <= 36500 AND bonus_balance = 0 AND balance_reclaimed_at IS NULL)
    OR (bonus_days = 0 AND bonus_balance > 0 AND bonus_balance <= 1000000));
