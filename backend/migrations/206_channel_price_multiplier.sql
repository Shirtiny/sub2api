-- Existing configured prices remain list prices at 1x. No price data is rewritten.
ALTER TABLE channel_model_pricing
    ADD COLUMN price_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1
    CHECK (price_multiplier > 0 AND price_multiplier <= 1000);

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN price_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1
    CHECK (price_multiplier > 0 AND price_multiplier <= 1000);
