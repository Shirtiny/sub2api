-- Normalize the administrative base value. Effective concurrency is resolved
-- from active subscriptions or current balance; this value is not a floor.
-- Do not change plan/order grants, balances, quota counters or subscription dates.
UPDATE users SET concurrency = 2, updated_at = NOW()
WHERE deleted_at IS NULL AND concurrency <> 2;

ALTER TABLE users ALTER COLUMN concurrency SET DEFAULT 2;

INSERT INTO settings (key, value, updated_at)
VALUES ('default_concurrency', '2', NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
