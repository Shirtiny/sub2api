-- Store user API keys as non-reversible hashes.
--
-- Existing plaintext values are migrated with database-side SHA-256 because
-- SQL migrations do not have access to the application HMAC secret. New keys
-- store both an HMAC-SHA256 key_hash and a secret-independent lookup hash.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS key_hash_alg VARCHAR(20) NOT NULL DEFAULT 'sha256',
    ADD COLUMN IF NOT EXISTS key_lookup_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS key_prefix VARCHAR(32) NOT NULL DEFAULT '';

WITH rows_to_hash AS (
    SELECT
        id,
        (
            key ~ '^__hashed__[0-9]+__[0-9a-f]{12}$'
            OR key ~ '^__hashed__[0-9a-f]{32}$'
        ) AND (
            COALESCE(key_hash, '') <> ''
            OR COALESCE(key_lookup_hash, '') <> ''
        ) AS generated_tombstone
    FROM api_keys
    WHERE deleted_at IS NULL
      AND (
          key_hash IS NULL OR key_hash = ''
          OR key_hash_alg IS NULL OR key_hash_alg = ''
          OR key_lookup_hash IS NULL OR key_lookup_hash = ''
          OR key_prefix = ''
      )
)
UPDATE api_keys AS k
SET
    key_hash = CASE
        WHEN COALESCE(k.key_hash, '') = '' AND NOT rows_to_hash.generated_tombstone THEN encode(sha256(k.key::bytea), 'hex')
        ELSE k.key_hash
    END,
    key_hash_alg = CASE
        WHEN COALESCE(k.key_hash_alg, '') = '' THEN 'sha256'
        ELSE k.key_hash_alg
    END,
    key_lookup_hash = CASE
        WHEN COALESCE(k.key_lookup_hash, '') = '' AND NOT rows_to_hash.generated_tombstone THEN encode(sha256(k.key::bytea), 'hex')
        ELSE k.key_lookup_hash
    END,
    key_prefix = CASE
        WHEN COALESCE(k.key_prefix, '') = '' AND NOT rows_to_hash.generated_tombstone THEN LEFT(k.key, 16)
        ELSE k.key_prefix
    END
FROM rows_to_hash
WHERE k.id = rows_to_hash.id;

UPDATE api_keys
SET
    key = CONCAT('__hashed__', id, '__', LEFT(COALESCE(key_lookup_hash, key_hash), 12)),
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND COALESCE(key_lookup_hash, key_hash) IS NOT NULL
  AND COALESCE(key_lookup_hash, key_hash) <> ''
  AND key <> CONCAT('__hashed__', id, '__', LEFT(COALESCE(key_lookup_hash, key_hash), 12));

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash
    ON api_keys(key_hash)
    WHERE deleted_at IS NULL AND key_hash IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_lookup_hash_active
    ON api_keys(key_lookup_hash)
    WHERE deleted_at IS NULL AND key_lookup_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix
    ON api_keys(key_prefix)
    WHERE deleted_at IS NULL AND key_prefix <> '';

ALTER TABLE deleted_api_key_audits
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS key_hash_alg VARCHAR(20) NOT NULL DEFAULT 'sha256',
    ADD COLUMN IF NOT EXISTS key_lookup_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS key_prefix VARCHAR(32) NOT NULL DEFAULT '';

WITH audit_rows_to_hash AS (
    SELECT
        id,
        key ~ '^__redacted__[0-9]*__[0-9a-f]{12}$'
            AND (
                COALESCE(key_hash, '') <> ''
                OR COALESCE(key_lookup_hash, '') <> ''
            ) AS generated_redaction
    FROM deleted_api_key_audits
    WHERE (
          key_hash IS NULL OR key_hash = ''
          OR key_hash_alg IS NULL OR key_hash_alg = ''
          OR key_lookup_hash IS NULL OR key_lookup_hash = ''
          OR key_prefix = ''
      )
)
UPDATE deleted_api_key_audits AS a
SET
    key_hash = CASE
        WHEN COALESCE(a.key_hash, '') = '' AND NOT audit_rows_to_hash.generated_redaction THEN encode(sha256(a.key::bytea), 'hex')
        ELSE a.key_hash
    END,
    key_hash_alg = CASE
        WHEN COALESCE(a.key_hash_alg, '') = '' THEN 'sha256'
        ELSE a.key_hash_alg
    END,
    key_lookup_hash = CASE
        WHEN COALESCE(a.key_lookup_hash, '') = '' AND NOT audit_rows_to_hash.generated_redaction THEN encode(sha256(a.key::bytea), 'hex')
        ELSE a.key_lookup_hash
    END,
    key_prefix = CASE
        WHEN COALESCE(a.key_prefix, '') = '' AND NOT audit_rows_to_hash.generated_redaction THEN LEFT(a.key, 16)
        ELSE a.key_prefix
    END
FROM audit_rows_to_hash
WHERE a.id = audit_rows_to_hash.id;

UPDATE deleted_api_key_audits
SET key = CONCAT('__redacted__', api_key_id, '__', LEFT(COALESCE(key_lookup_hash, key_hash), 12))
WHERE COALESCE(key_lookup_hash, key_hash) IS NOT NULL
  AND COALESCE(key_lookup_hash, key_hash) <> ''
  AND key <> CONCAT('__redacted__', api_key_id, '__', LEFT(COALESCE(key_lookup_hash, key_hash), 12));

CREATE INDEX IF NOT EXISTS deletedapikeyaudit_key_hash
    ON deleted_api_key_audits (key_hash);

CREATE INDEX IF NOT EXISTS deletedapikeyaudit_key_lookup_hash
    ON deleted_api_key_audits (key_lookup_hash);

CREATE INDEX IF NOT EXISTS deletedapikeyaudit_key_prefix
    ON deleted_api_key_audits (key_prefix);
