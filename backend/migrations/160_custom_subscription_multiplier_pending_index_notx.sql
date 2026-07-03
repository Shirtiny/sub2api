-- Rebuild the pending custom subscription order helper index for 1x custom orders.
DROP INDEX CONCURRENTLY IF EXISTS idx_payment_orders_custom_subscription_pending;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_orders_custom_subscription_pending
    ON payment_orders(user_id, plan_id, status, expires_at)
    WHERE subscription_multiplier >= 1;
