-- Opt-in only: no seed rules, no changes to existing prices or balances.
CREATE TABLE IF NOT EXISTS global_context_pricing_rules (
 id BIGSERIAL PRIMARY KEY, model TEXT NOT NULL UNIQUE, platform TEXT NOT NULL,
 threshold_tokens BIGINT NOT NULL CHECK(threshold_tokens > 0 AND threshold_tokens <= 100000000),
 input_multiplier DOUBLE PRECISION NOT NULL CHECK(input_multiplier BETWEEN 0.01 AND 100),
 output_multiplier DOUBLE PRECISION NOT NULL CHECK(output_multiplier BETWEEN 0.01 AND 100),
 cache_read_multiplier DOUBLE PRECISION NOT NULL CHECK(cache_read_multiplier BETWEEN 0.01 AND 100),
 cache_write_multiplier DOUBLE PRECISION NOT NULL CHECK(cache_write_multiplier BETWEEN 0.01 AND 100),
 enabled BOOLEAN NOT NULL DEFAULT FALSE, revision BIGINT NOT NULL DEFAULT 1,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
