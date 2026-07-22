-- Migration 175 used the latest paid/recharging order as the best historical
-- plan snapshot. Those states do not prove that subscription fulfillment has
-- completed, so reserving source_order_id can block the real fulfillment
-- insert. Delete only rows tied to orders without a subscription assignment
-- audit; completed assignments keep their idempotency key.
DELETE FROM subscription_concurrency_entitlements sce
USING payment_orders po
WHERE sce.source_order_id = po.id
  AND po.status IN ('PENDING', 'PAID', 'RECHARGING')
  AND NOT EXISTS (
      SELECT 1
      FROM payment_audit_logs pal
      WHERE pal.order_id = po.id::text
        AND pal.action IN ('SUBSCRIPTION_ASSIGNED', 'SUBSCRIPTION_SUCCESS')
  );
