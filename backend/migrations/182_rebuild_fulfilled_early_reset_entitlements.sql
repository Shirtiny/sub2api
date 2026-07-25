-- Migration 179 treated PAID and RECHARGING orders as fulfilled even when the
-- subscription assignment had not completed. Rebuild only the pristine rows
-- created by that migration so an in-flight order cannot shift older terms.
--
-- Subscriptions with runtime-created or runtime-updated entitlement rows are
-- intentionally left untouched. Reconstructing those rows could undo an early
-- reset that happened after migration 179 was applied.

CREATE TEMP TABLE IF NOT EXISTS migration_182_early_reset_rebuild_subscriptions (
    subscription_id BIGINT PRIMARY KEY
) ON COMMIT DROP;

TRUNCATE migration_182_early_reset_rebuild_subscriptions;

INSERT INTO migration_182_early_reset_rebuild_subscriptions (subscription_id)
SELECT DISTINCT ere.subscription_id
FROM subscription_early_reset_entitlements ere
JOIN schema_migrations sm
  ON sm.filename = '179_subscription_early_reset_entitlements.sql'
 AND ere.created_at = sm.applied_at
WHERE NOT EXISTS (
    SELECT 1
    FROM subscription_early_reset_entitlements other
    WHERE other.subscription_id = ere.subscription_id
      AND (
          other.created_at <> sm.applied_at
          OR other.updated_at <> other.created_at
      )
);

DELETE FROM subscription_early_reset_entitlements ere
USING migration_182_early_reset_rebuild_subscriptions rebuild
WHERE ere.subscription_id = rebuild.subscription_id;

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
    FROM migration_182_early_reset_rebuild_subscriptions rebuild
    JOIN user_subscriptions us ON us.id = rebuild.subscription_id
    JOIN payment_orders po
      ON po.user_id = us.user_id
     AND po.subscription_group_id = us.group_id
    WHERE us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.expires_at > NOW()
      AND po.order_type = 'subscription'
      AND (
          po.status = 'COMPLETED'
          OR (
              po.status IN ('PAID', 'RECHARGING')
              AND EXISTS (
                  SELECT 1
                  FROM payment_audit_logs pal
                  WHERE pal.order_id = po.id::text
                    AND pal.action IN ('SUBSCRIPTION_ASSIGNED', 'SUBSCRIPTION_SUCCESS')
              )
          )
      )
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

-- Preserve migration 179's snapshot fallback when no fulfilled order history
-- can reconstruct the current active term.
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
FROM migration_182_early_reset_rebuild_subscriptions rebuild
JOIN user_subscriptions us ON us.id = rebuild.subscription_id
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
