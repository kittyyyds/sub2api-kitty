CREATE TABLE IF NOT EXISTS token_reward_claims (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_type VARCHAR(16) NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    tier_key VARCHAR(64) NOT NULL,
    threshold_tokens BIGINT NOT NULL,
    usage_tokens BIGINT NOT NULL,
    reward_amount DECIMAL(20, 8) NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT token_reward_claims_period_type_valid CHECK (period_type IN ('weekly', 'monthly')),
    CONSTRAINT token_reward_claims_period_valid CHECK (period_end > period_start),
    CONSTRAINT token_reward_claims_threshold_positive CHECK (threshold_tokens > 0),
    CONSTRAINT token_reward_claims_usage_nonnegative CHECK (usage_tokens >= 0),
    CONSTRAINT token_reward_claims_reward_positive CHECK (reward_amount > 0),
    CONSTRAINT token_reward_claims_once_per_tier UNIQUE (user_id, period_start, period_type, tier_key)
);

CREATE INDEX IF NOT EXISTS idx_token_reward_claims_user_claimed
    ON token_reward_claims (user_id, claimed_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_token_reward_claims_admin_claimed
    ON token_reward_claims (claimed_at DESC, id DESC);

