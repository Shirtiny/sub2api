CREATE TABLE IF NOT EXISTS cafe_coupons (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(48) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    membership_level INTEGER NOT NULL DEFAULT 0,
    coupon_type VARCHAR(20) NOT NULL,
    value DECIMAL(20,8) NOT NULL,
    period VARCHAR(20) NOT NULL DEFAULT 'month',
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'issued',
    order_id BIGINT NULL REFERENCES payment_orders(id),
    applied_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cafe_coupons_code_unique UNIQUE (code),
    CONSTRAINT cafe_coupons_user_period_unique UNIQUE (user_id, period, period_start),
    CONSTRAINT cafe_coupons_type_check CHECK (coupon_type IN ('cash', 'discount')),
    CONSTRAINT cafe_coupons_period_check CHECK (period IN ('day', 'week', 'month')),
    CONSTRAINT cafe_coupons_status_check CHECK (status IN ('issued', 'applied', 'void')),
    CONSTRAINT cafe_coupons_value_check CHECK (value > 0),
    CONSTRAINT cafe_coupons_membership_level_check CHECK (membership_level BETWEEN 0 AND 3),
    CONSTRAINT cafe_coupons_period_range_check CHECK (period_end > period_start)
);

CREATE INDEX IF NOT EXISTS idx_cafe_coupons_user_id ON cafe_coupons(user_id);
CREATE INDEX IF NOT EXISTS idx_cafe_coupons_status ON cafe_coupons(status);
CREATE INDEX IF NOT EXISTS idx_cafe_coupons_order_id ON cafe_coupons(order_id);
CREATE INDEX IF NOT EXISTS idx_cafe_coupons_period_end ON cafe_coupons(period_end);
