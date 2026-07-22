CREATE TABLE IF NOT EXISTS subscription_concurrency_entitlements (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    source_order_id BIGINT,
    concurrency INT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscription_concurrency_entitlements_concurrency_positive CHECK (concurrency > 0),
    CONSTRAINT subscription_concurrency_entitlements_window_valid CHECK (starts_at < expires_at)
);

CREATE INDEX IF NOT EXISTS idx_subscription_concurrency_entitlements_user_window
    ON subscription_concurrency_entitlements(user_id, starts_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_subscription_concurrency_entitlements_subscription
    ON subscription_concurrency_entitlements(subscription_id, starts_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_concurrency_entitlements_source_order
    ON subscription_concurrency_entitlements(source_order_id);

-- Migration 174 initialized existing plans to 1 because the field did not
-- exist before. Use the configured new-user default for those historical plans
-- so enabling this feature does not unexpectedly lower active users' limits.
UPDATE subscription_plans sp
SET concurrency = CASE
    WHEN s.value ~ '^[1-9][0-9]{0,9}$' THEN LEAST(s.value::bigint, 2147483647)::int
    ELSE 1
END,
    updated_at = NOW()
FROM settings s
WHERE s.key = 'default_concurrency'
  AND sp.concurrency = 1;

-- Preserve the best available current entitlement for active subscriptions
-- created before this feature existed. The latest completed/paid order is the
-- only reliable source for a historical plan snapshot.
INSERT INTO subscription_concurrency_entitlements (
    user_id, subscription_id, source_order_id, concurrency, starts_at, expires_at
)
SELECT us.user_id,
       us.id,
       latest_order.id,
       latest_order.concurrency,
       GREATEST(us.starts_at, COALESCE(latest_order.fulfillment_at, us.starts_at)),
       us.expires_at
FROM user_subscriptions us
JOIN LATERAL (
    SELECT po.id,
           sp.concurrency,
           COALESCE(po.completed_at, po.paid_at, po.created_at) AS fulfillment_at
    FROM payment_orders po
    JOIN subscription_plans sp ON sp.id = po.plan_id
    WHERE po.user_id = us.user_id
      AND po.subscription_group_id = us.group_id
      AND po.order_type = 'subscription'
      AND po.status IN ('PAID', 'RECHARGING', 'COMPLETED')
      AND sp.concurrency > 0
    ORDER BY COALESCE(po.completed_at, po.paid_at, po.created_at) DESC, po.id DESC
    LIMIT 1
) latest_order ON TRUE
WHERE us.deleted_at IS NULL
  AND us.status = 'active'
  AND us.expires_at > NOW()
  AND us.plan_concurrency IS NULL
  AND GREATEST(us.starts_at, COALESCE(latest_order.fulfillment_at, us.starts_at)) < us.expires_at
ON CONFLICT DO NOTHING;

-- Orders created before the snapshot column was introduced must still carry
-- the plan value when they are fulfilled after deployment. Do not apply a
-- custom multiplier to concurrency.
UPDATE payment_orders po
SET subscription_concurrency = sp.concurrency,
    updated_at = NOW()
FROM subscription_plans sp
WHERE po.plan_id = sp.id
  AND po.order_type = 'subscription'
  AND po.subscription_concurrency IS NULL
  AND po.status IN ('PENDING', 'PAID', 'RECHARGING', 'COMPLETED')
  AND sp.concurrency > 0;
