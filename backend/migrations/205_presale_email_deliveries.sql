-- Admin-initiated notices only. Creating receipts does not enable sales or send mail.
CREATE TABLE presale_email_deliveries (
    campaign_key VARCHAR(20) NOT NULL,
    recipient_hash CHAR(64) NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    admin_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('sending', 'sent', 'skipped', 'uncertain')),
    preview_version CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (campaign_key, recipient_hash)
);
