-- Seed database-level defaults for invite rebate settings.
-- These rows make newly introduced per-membership affiliate settings visible before
-- the first admin save and keep DB state aligned with service defaults.

INSERT INTO settings (key, value, updated_at)
VALUES
    ('affiliate_enabled', 'false', NOW()),
    ('affiliate_rebate_rate', '0.00000000', NOW()),
    ('affiliate_rebate_rate_level0', '0.00000000', NOW()),
    ('affiliate_rebate_rate_level1', '5.00000000', NOW()),
    ('affiliate_rebate_rate_level2', '15.00000000', NOW()),
    ('affiliate_rebate_rate_level3', '25.00000000', NOW()),
    ('affiliate_rebate_freeze_hours', '0', NOW()),
    ('affiliate_rebate_duration_days', '0', NOW()),
    ('affiliate_rebate_per_invitee_cap', '0.00000000', NOW()),
    ('affiliate_rebate_per_invitee_cap_level0', '0.00000000', NOW()),
    ('affiliate_rebate_per_invitee_cap_level1', '100.00000000', NOW()),
    ('affiliate_rebate_per_invitee_cap_level2', '300.00000000', NOW()),
    ('affiliate_rebate_per_invitee_cap_level3', '1000.00000000', NOW()),
    ('affiliate_invite_limit', '0', NOW()),
    ('affiliate_invite_limit_level0', '0', NOW()),
    ('affiliate_invite_limit_level1', '1', NOW()),
    ('affiliate_invite_limit_level2', '3', NOW()),
    ('affiliate_invite_limit_level3', '5', NOW())
ON CONFLICT (key) DO NOTHING;

-- If a deployment loaded the new cap fields before this migration, the UI may have
-- persisted the placeholder zero values for all paid levels. Treat that all-zero
-- paid-level state as uninitialized and backfill the intended defaults. A deliberate
-- admin override to 0 after this migration remains untouched.
UPDATE settings
SET value = CASE key
    WHEN 'affiliate_rebate_per_invitee_cap_level1' THEN '100.00000000'
    WHEN 'affiliate_rebate_per_invitee_cap_level2' THEN '300.00000000'
    WHEN 'affiliate_rebate_per_invitee_cap_level3' THEN '1000.00000000'
    ELSE value
END,
updated_at = NOW()
WHERE key IN (
    'affiliate_rebate_per_invitee_cap_level1',
    'affiliate_rebate_per_invitee_cap_level2',
    'affiliate_rebate_per_invitee_cap_level3'
)
AND EXISTS (
    SELECT 1
    FROM settings s1
    JOIN settings s2 ON s2.key = 'affiliate_rebate_per_invitee_cap_level2'
    JOIN settings s3 ON s3.key = 'affiliate_rebate_per_invitee_cap_level3'
    WHERE s1.key = 'affiliate_rebate_per_invitee_cap_level1'
      AND COALESCE(NULLIF(TRIM(s1.value), ''), '0')::numeric = 0
      AND COALESCE(NULLIF(TRIM(s2.value), ''), '0')::numeric = 0
      AND COALESCE(NULLIF(TRIM(s3.value), ''), '0')::numeric = 0
);
