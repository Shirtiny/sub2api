ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS inviter_bound_at TIMESTAMPTZ NULL;

UPDATE user_affiliates
SET inviter_bound_at = COALESCE(inviter_bound_at, updated_at, created_at)
WHERE inviter_id IS NOT NULL
  AND inviter_bound_at IS NULL;

COMMENT ON COLUMN user_affiliates.inviter_bound_at IS '邀请关系绑定时间，用于计算返利有效期';
