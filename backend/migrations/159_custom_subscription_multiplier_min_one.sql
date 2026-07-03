-- Allow custom subscription multiplier 1x without mutating already-applied 157/158 migrations.
ALTER TABLE subscription_plans
    ALTER COLUMN custom_multiplier_min SET DEFAULT 1;

ALTER TABLE subscription_plans
    ALTER COLUMN custom_multiplier_max SET DEFAULT 1;
