ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_invite_limit INTEGER NULL;

COMMENT ON COLUMN user_affiliates.aff_invite_limit IS '专属可邀请人数上限（NULL 表示沿用全局，0 表示不限制）';

CREATE INDEX IF NOT EXISTS idx_user_affiliates_invite_limit_custom
    ON user_affiliates (updated_at)
    WHERE aff_invite_limit IS NOT NULL;
