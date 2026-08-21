-- Cafe coupons snapshot the member level at issuance, so the database must accept
-- the same LV.0-LV.5 range as the application.
ALTER TABLE cafe_coupons
    DROP CONSTRAINT IF EXISTS cafe_coupons_membership_level_check;

ALTER TABLE cafe_coupons
    ADD CONSTRAINT cafe_coupons_membership_level_check
    CHECK (membership_level BETWEEN 0 AND 5);
