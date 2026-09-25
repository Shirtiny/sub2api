-- Definitions only; upgrading never issues or enables a promotion.
CREATE TABLE cafe_campaigns (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(48) NOT NULL,
    name VARCHAR(100) NOT NULL,
    discount_percent INTEGER NOT NULL CHECK (discount_percent BETWEEN 1 AND 99),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (expires_at > starts_at),
    CHECK (code ~ '^CAFE-PUBLIC-[A-Z0-9][A-Z0-9-]{0,35}$')
);
CREATE UNIQUE INDEX cafecampaign_code ON cafe_campaigns(code);
CREATE TABLE cafe_campaign_uses (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES cafe_campaigns(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX cafecampaignuse_campaign_id_user_id ON cafe_campaign_uses(campaign_id, user_id);
CREATE UNIQUE INDEX cafecampaignuse_order_id ON cafe_campaign_uses(order_id);
