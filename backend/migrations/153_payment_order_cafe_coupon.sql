ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS cafe_coupon_code VARCHAR(48),
    ADD COLUMN IF NOT EXISTS cafe_coupon_discount DECIMAL(20,2) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_payment_orders_cafe_coupon_code
    ON payment_orders(cafe_coupon_code)
    WHERE cafe_coupon_code IS NOT NULL AND cafe_coupon_code <> '';
