CREATE TABLE IF NOT EXISTS subscription_early_reset_entitlements (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    source_order_id BIGINT,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    duration_days INT NOT NULL DEFAULT 0,
    custom_term BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscription_early_reset_entitlements_duration_valid
        CHECK (duration_days >= 0 AND duration_days <= 36500),
    CONSTRAINT subscription_early_reset_entitlements_policy_valid
        CHECK ((enabled AND duration_days > 0) OR (NOT enabled AND duration_days = 0)),
    CONSTRAINT subscription_early_reset_entitlements_window_valid CHECK (starts_at < expires_at)
);

CREATE INDEX IF NOT EXISTS idx_subscription_early_reset_entitlements_subscription_window
    ON subscription_early_reset_entitlements(subscription_id, custom_term, starts_at, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_early_reset_entitlements_source_order
    ON subscription_early_reset_entitlements(source_order_id);

-- Reconstruct term boundaries for orders fulfilled after the subscription's
-- current start. Working backwards from each base/custom final expiry preserves
-- the policy order even when several plans in one group used different values.
WITH fulfilled_orders AS (
    SELECT po.id AS source_order_id,
           po.user_id,
           us.id AS subscription_id,
           po.subscription_early_reset_enabled AS enabled,
           CASE
               WHEN po.subscription_early_reset_enabled
                   THEN po.subscription_early_reset_duration_days
               ELSE 0
           END AS duration_days,
           po.subscription_source_group_id IS NOT NULL AS custom_term,
           po.subscription_days,
           us.starts_at AS subscription_starts_at,
           CASE
               WHEN po.subscription_source_group_id IS NOT NULL
                   THEN us.custom_expires_at
               ELSE us.expires_at
           END AS final_expires_at,
           COALESCE(po.completed_at, po.paid_at, po.created_at) AS fulfilled_at
    FROM payment_orders po
    JOIN user_subscriptions us
      ON us.user_id = po.user_id
     AND us.group_id = po.subscription_group_id
    WHERE us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.expires_at > NOW()
      AND po.order_type = 'subscription'
      AND po.status IN ('PAID', 'RECHARGING', 'COMPLETED')
      AND po.subscription_days > 0
      AND COALESCE(po.completed_at, po.paid_at, po.created_at) >= us.starts_at
), ordered_terms AS (
    SELECT fulfilled_orders.*,
           COALESCE(
               SUM(subscription_days) OVER (
                   PARTITION BY subscription_id, custom_term
                   ORDER BY fulfilled_at DESC, source_order_id DESC
                   ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
               ),
               0
           ) AS later_days
    FROM fulfilled_orders
    WHERE final_expires_at IS NOT NULL
), reconstructed_terms AS (
    SELECT source_order_id,
           user_id,
           subscription_id,
           enabled,
           duration_days,
           custom_term,
           final_expires_at - ((later_days + subscription_days) * INTERVAL '1 day') AS starts_at,
           final_expires_at - (later_days * INTERVAL '1 day') AS expires_at,
           subscription_starts_at
    FROM ordered_terms
)
INSERT INTO subscription_early_reset_entitlements (
    user_id, subscription_id, source_order_id, enabled, duration_days,
    custom_term, starts_at, expires_at
)
SELECT user_id,
       subscription_id,
       source_order_id,
       enabled,
       duration_days,
       custom_term,
       GREATEST(starts_at, subscription_starts_at),
       expires_at
FROM reconstructed_terms
WHERE GREATEST(starts_at, subscription_starts_at) < expires_at
ON CONFLICT (source_order_id) DO NOTHING;

-- Fall back to the currently visible snapshot for subscriptions whose order
-- history cannot be reconstructed.
INSERT INTO subscription_early_reset_entitlements (
    user_id, subscription_id, enabled, duration_days, custom_term, starts_at, expires_at
)
SELECT us.user_id,
       us.id,
       us.early_reset_enabled,
       CASE WHEN us.early_reset_enabled THEN us.early_reset_duration_days ELSE 0 END,
       us.custom_expires_at IS NOT NULL AND us.custom_expires_at > NOW(),
       us.starts_at,
       CASE
           WHEN us.custom_expires_at IS NOT NULL AND us.custom_expires_at > NOW()
               THEN us.custom_expires_at
           ELSE us.expires_at
       END
FROM user_subscriptions us
WHERE us.deleted_at IS NULL
  AND us.status = 'active'
  AND us.expires_at > NOW()
  AND us.starts_at < CASE
      WHEN us.custom_expires_at IS NOT NULL AND us.custom_expires_at > NOW()
          THEN us.custom_expires_at
      ELSE us.expires_at
  END
  AND NOT EXISTS (
      SELECT 1
      FROM subscription_early_reset_entitlements ere
      WHERE ere.subscription_id = us.id
  );
