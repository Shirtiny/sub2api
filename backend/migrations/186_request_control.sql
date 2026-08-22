-- Request control configuration is stored in settings; this table keeps only
-- the bounded, redacted observations needed by the admin risk-control page.
CREATE TABLE IF NOT EXISTS request_control_logs (
    id                  BIGSERIAL PRIMARY KEY,
    request_id          VARCHAR(128) NOT NULL DEFAULT '',
    user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email          VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id          BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name        VARCHAR(100) NOT NULL DEFAULT '',
    group_id            BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name          VARCHAR(255) NOT NULL DEFAULT '',
    endpoint            VARCHAR(128) NOT NULL DEFAULT '',
    provider            VARCHAR(64) NOT NULL DEFAULT '',
    protocol            VARCHAR(64) NOT NULL DEFAULT '',
    model               VARCHAR(255) NOT NULL DEFAULT '',
    action              VARCHAR(32) NOT NULL DEFAULT '',
    reason              VARCHAR(128) NOT NULL DEFAULT '',
    allowed             BOOLEAN NOT NULL DEFAULT FALSE,
    blocked             BOOLEAN NOT NULL DEFAULT FALSE,
    observed            BOOLEAN NOT NULL DEFAULT FALSE,
    client_kind         VARCHAR(64) NOT NULL DEFAULT '',
    user_agent          VARCHAR(512) NOT NULL DEFAULT '',
    originator          VARCHAR(128) NOT NULL DEFAULT '',
    tls_fingerprint     VARCHAR(128) NOT NULL DEFAULT '',
    tls_match           BOOLEAN,
    header_match        BOOLEAN,
    body_match          BOOLEAN,
    details             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_request_control_logs_created_at
    ON request_control_logs(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_control_logs_group_created_at
    ON request_control_logs(group_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_control_logs_user_created_at
    ON request_control_logs(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_control_logs_action_created_at
    ON request_control_logs(action, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_control_logs_protocol_created_at
    ON request_control_logs(protocol, created_at DESC, id DESC);
