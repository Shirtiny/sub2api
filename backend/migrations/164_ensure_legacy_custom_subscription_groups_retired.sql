-- Ensure legacy physical custom subscription groups are fully retired after the virtual entitlement rollout.
-- 163 had a checksum incident during development; this idempotent follow-up preserves migration
-- immutability while guaranteeing environments that already applied an earlier 163 get the final state.

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
