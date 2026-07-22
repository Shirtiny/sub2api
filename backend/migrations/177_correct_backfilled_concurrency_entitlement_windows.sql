-- Migration 175 could only reconstruct one historical concurrency grant per
-- active subscription. Its first version started that grant at the latest
-- order fulfillment time but ended it at the whole accumulated subscription
-- expiry, which made a renewal's concurrency active too early. Restrict this
-- correction to rows created by that migration itself so runtime grants are
-- never rewritten.

WITH corrected AS (
    SELECT sce.id,
           CASE
               WHEN us.custom_source_plan_id = po.plan_id
                    AND us.custom_expires_at IS NOT NULL
                   THEN GREATEST(
                       us.starts_at,
                       COALESCE(po.completed_at, po.paid_at, po.created_at)
                   )
               ELSE GREATEST(
                   us.starts_at,
                   sce.expires_at - (po.subscription_days * INTERVAL '1 day')
               )
           END AS corrected_starts_at,
           CASE
               WHEN us.custom_source_plan_id = po.plan_id
                    AND us.custom_expires_at IS NOT NULL
                   THEN LEAST(sce.expires_at, us.custom_expires_at)
               ELSE sce.expires_at
           END AS corrected_expires_at
    FROM subscription_concurrency_entitlements sce
    JOIN schema_migrations sm
      ON sm.filename = '175_subscription_concurrency_entitlements.sql'
     AND sce.created_at = sm.applied_at
    JOIN payment_orders po ON po.id = sce.source_order_id
    JOIN user_subscriptions us ON us.id = sce.subscription_id
    WHERE po.subscription_days IS NOT NULL
      AND po.subscription_days > 0
)
DELETE FROM subscription_concurrency_entitlements sce
USING corrected
WHERE sce.id = corrected.id
  AND corrected.corrected_starts_at >= corrected.corrected_expires_at;

WITH corrected AS (
    SELECT sce.id,
           CASE
               WHEN us.custom_source_plan_id = po.plan_id
                    AND us.custom_expires_at IS NOT NULL
                   THEN GREATEST(
                       us.starts_at,
                       COALESCE(po.completed_at, po.paid_at, po.created_at)
                   )
               ELSE GREATEST(
                   us.starts_at,
                   sce.expires_at - (po.subscription_days * INTERVAL '1 day')
               )
           END AS corrected_starts_at,
           CASE
               WHEN us.custom_source_plan_id = po.plan_id
                    AND us.custom_expires_at IS NOT NULL
                   THEN LEAST(sce.expires_at, us.custom_expires_at)
               ELSE sce.expires_at
           END AS corrected_expires_at
    FROM subscription_concurrency_entitlements sce
    JOIN schema_migrations sm
      ON sm.filename = '175_subscription_concurrency_entitlements.sql'
     AND sce.created_at = sm.applied_at
    JOIN payment_orders po ON po.id = sce.source_order_id
    JOIN user_subscriptions us ON us.id = sce.subscription_id
    WHERE po.subscription_days IS NOT NULL
      AND po.subscription_days > 0
)
UPDATE subscription_concurrency_entitlements sce
SET starts_at = corrected.corrected_starts_at,
    expires_at = corrected.corrected_expires_at,
    updated_at = NOW()
FROM corrected
WHERE sce.id = corrected.id
  AND corrected.corrected_starts_at < corrected.corrected_expires_at;
