-- Strengthen monetary affiliate rebate idempotency at the ledger layer.
-- Payment fulfillment already claims an audit row per order; this index protects
-- future/manual accrual paths from issuing the same order rebate twice.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_ledger_accrue_source_order_uniq
ON user_affiliate_ledger (user_id, source_user_id, source_order_id, action)
WHERE action = 'accrue' AND source_order_id IS NOT NULL;
