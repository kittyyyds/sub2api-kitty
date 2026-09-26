ALTER TABLE token_reward_claims
    ADD COLUMN IF NOT EXISTS cycle_start TIMESTAMPTZ;

UPDATE token_reward_claims
SET cycle_start = period_start
WHERE cycle_start IS NULL;

ALTER TABLE token_reward_claims
    ALTER COLUMN cycle_start SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_token_reward_claims_once_per_cycle_tier
    ON token_reward_claims (user_id, cycle_start, period_type, tier_key);
