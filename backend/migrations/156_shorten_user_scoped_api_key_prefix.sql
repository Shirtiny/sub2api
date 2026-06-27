-- Shorten user-scoped cafepass API key prefixes for UI/log display.
--
-- The full key is authenticated by hash. key_prefix is only a non-secret
-- identifier for UI, logs, search, and support workflows.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

UPDATE api_keys
SET key_prefix = regexp_replace(key_prefix, '^(cafepass-[0-9]+-.{8}).+$', '\1')
WHERE key_prefix ~ '^cafepass-[0-9]+-.{9,}$';

UPDATE deleted_api_key_audits
SET key_prefix = regexp_replace(key_prefix, '^(cafepass-[0-9]+-.{8}).+$', '\1')
WHERE key_prefix ~ '^cafepass-[0-9]+-.{9,}$';

UPDATE ops_error_logs
SET attempted_key_prefix = regexp_replace(attempted_key_prefix, '^(cafepass-[0-9]+-.{8}).+$', '\1')
WHERE attempted_key_prefix ~ '^cafepass-[0-9]+-.{9,}$';

UPDATE ops_error_logs
SET api_key_prefix = regexp_replace(api_key_prefix, '^(cafepass-[0-9]+-.{8}).+$', '\1')
WHERE api_key_prefix ~ '^cafepass-[0-9]+-.{9,}$';
