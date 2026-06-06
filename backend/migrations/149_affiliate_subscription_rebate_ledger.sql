-- Track subscription-day affiliate rebates separately from balance rebates.
-- Subscription rebates are accrued into the affiliate ledger first, can be frozen
-- with the same affiliate freeze window, and are transferred by the inviter later.

ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS subscription_group_id BIGINT NULL REFERENCES groups(id) ON DELETE SET NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS transferred_at TIMESTAMPTZ NULL;

COMMENT ON COLUMN user_affiliate_ledger.subscription_group_id IS 'Subscription group for subscription-day affiliate rebate ledgers; NULL for balance rebates';
COMMENT ON COLUMN user_affiliate_ledger.transferred_at IS 'When a subscription-day rebate was transferred into a user subscription; NULL means not transferred yet';
COMMENT ON COLUMN user_affiliate_ledger.action IS 'accrue|transfer|accrue_subscription|transfer_subscription';

CREATE INDEX IF NOT EXISTS idx_user_affiliate_ledger_subscription_rebate
    ON user_affiliate_ledger(user_id, subscription_group_id, action, frozen_until, transferred_at)
    WHERE action IN ('accrue_subscription', 'transfer_subscription');

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_ledger_subscription_order_uniq
    ON user_affiliate_ledger(user_id, source_user_id, source_order_id, subscription_group_id, action)
    WHERE action = 'accrue_subscription'
      AND source_order_id IS NOT NULL
      AND subscription_group_id IS NOT NULL;
