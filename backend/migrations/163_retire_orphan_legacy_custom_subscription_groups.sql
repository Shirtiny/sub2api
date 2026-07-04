-- Retire orphaned legacy physical custom subscription groups after virtual custom entitlement rollout.
-- These groups no longer represent active entitlement. API keys are moved back to the source
-- subscription group so a future virtual custom subscription can reuse the stable source group.

UPDATE api_keys AS k
SET group_id = g.custom_source_group_id,
    updated_at = NOW()
FROM groups AS g
WHERE k.group_id = g.id
  AND k.deleted_at IS NULL
  AND g.deleted_at IS NULL
  AND g.is_custom_subscription_group = TRUE
  AND g.custom_source_group_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM user_subscriptions AS us
    WHERE us.group_id = g.id
      AND us.deleted_at IS NULL
      AND us.status = 'active'
      AND (us.expires_at IS NULL OR us.expires_at > NOW())
  );

UPDATE groups AS g
SET status = 'disabled',
    updated_at = NOW()
WHERE g.deleted_at IS NULL
  AND g.status = 'active'
  AND g.is_custom_subscription_group = TRUE
  AND g.custom_source_group_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM user_subscriptions AS us
    WHERE us.group_id = g.id
      AND us.deleted_at IS NULL
      AND us.status = 'active'
      AND (us.expires_at IS NULL OR us.expires_at > NOW())
  );
