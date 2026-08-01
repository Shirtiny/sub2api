CREATE INDEX IF NOT EXISTS promotionactivityparticipation_activity_id_created_at
    ON promotion_activity_participations(activity_id, created_at);

CREATE INDEX IF NOT EXISTS paymentorder_subscription_bonus_activity_id
    ON payment_orders(subscription_bonus_activity_id)
    WHERE subscription_bonus_activity_id IS NOT NULL;
