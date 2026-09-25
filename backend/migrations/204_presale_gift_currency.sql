-- Preserve legacy USD gift definitions and snapshots. New campaigns can use a
-- CNY face value, converted into USD ledger balance at the order's locked rate.
ALTER TABLE promotion_activities
 ADD COLUMN bonus_currency VARCHAR(3) NOT NULL DEFAULT 'USD',
 ADD CONSTRAINT promotion_activities_bonus_currency_valid CHECK (bonus_currency IN ('USD', 'CNY'));
ALTER TABLE payment_orders
 ADD COLUMN presale_balance_bonus_currency VARCHAR(3) NOT NULL DEFAULT 'USD',
 ADD COLUMN presale_balance_bonus_face_amount DECIMAL(20,8) NOT NULL DEFAULT 0;
UPDATE payment_orders SET presale_balance_bonus_face_amount = presale_balance_bonus_amount
 WHERE presale_balance_bonus_activity_id IS NOT NULL;
ALTER TABLE payment_orders ADD CONSTRAINT payment_orders_bonus_face_valid CHECK (
 presale_balance_bonus_currency IN ('USD', 'CNY') AND (
  (presale_balance_bonus_activity_id IS NULL AND presale_balance_bonus_face_amount = 0)
  OR (presale_balance_bonus_activity_id IS NOT NULL AND presale_balance_bonus_face_amount > 0
      AND presale_balance_bonus_face_amount <= 1000000)
 ));
