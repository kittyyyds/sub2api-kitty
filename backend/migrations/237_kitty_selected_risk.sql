-- Selected risk module: additive, separate from existing audit; default OFF.
-- 风控中心内容审计配置与记录

INSERT INTO settings (key, value, updated_at)
VALUES ('kitty_risk_enabled', 'false', NOW())
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS kitty_risk_logs (
    id                  BIGSERIAL PRIMARY KEY,
    request_id          VARCHAR(128) NOT NULL DEFAULT '',
    user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email          VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id          BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    api_key_name        VARCHAR(100) NOT NULL DEFAULT '',
    group_id            BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    group_name          VARCHAR(255) NOT NULL DEFAULT '',
    endpoint            VARCHAR(128) NOT NULL DEFAULT '',
    provider            VARCHAR(64) NOT NULL DEFAULT '',
    model               VARCHAR(255) NOT NULL DEFAULT '',
    mode                VARCHAR(32) NOT NULL DEFAULT '',
    action              VARCHAR(32) NOT NULL DEFAULT '',
    flagged             BOOLEAN NOT NULL DEFAULT FALSE,
    highest_category    VARCHAR(64) NOT NULL DEFAULT '',
    highest_score       DECIMAL(8, 6) NOT NULL DEFAULT 0,
    category_scores     JSONB NOT NULL DEFAULT '{}'::jsonb,
    threshold_snapshot  JSONB NOT NULL DEFAULT '{}'::jsonb,
    input_excerpt       TEXT NOT NULL DEFAULT '',
    upstream_latency_ms INT,
    error               TEXT NOT NULL DEFAULT '',
    violation_count     INT NOT NULL DEFAULT 0,
    auto_banned         BOOLEAN NOT NULL DEFAULT FALSE,
    email_sent          BOOLEAN NOT NULL DEFAULT FALSE,
    queue_delay_ms      INT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE kitty_risk_logs ADD COLUMN IF NOT EXISTS violation_count INT NOT NULL DEFAULT 0;
ALTER TABLE kitty_risk_logs ADD COLUMN IF NOT EXISTS auto_banned BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE kitty_risk_logs ADD COLUMN IF NOT EXISTS email_sent BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE kitty_risk_logs ADD COLUMN IF NOT EXISTS queue_delay_ms INT;
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_created_at ON kitty_risk_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_group_created_at ON kitty_risk_logs(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_flagged_created_at ON kitty_risk_logs(flagged, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_user_created_at ON kitty_risk_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_api_key_created_at ON kitty_risk_logs(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_endpoint_created_at ON kitty_risk_logs(endpoint, created_at DESC);

-- 风控中心：记录关键词拦截命中的具体关键词

ALTER TABLE kitty_risk_logs ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(255) NOT NULL DEFAULT '';

-- Unified Risk Control archive metadata. Ciphertext is deliberately isolated
-- from the list table so ordinary administration queries cannot load it.
ALTER TABLE kitty_risk_logs
    ADD COLUMN IF NOT EXISTS protocol VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS transport VARCHAR(32) NOT NULL DEFAULT 'http',
    ADD COLUMN IF NOT EXISTS request_stage VARCHAR(64) NOT NULL DEFAULT 'http',
    ADD COLUMN IF NOT EXISTS request_target TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS input_hash VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archive_id UUID,
    ADD COLUMN IF NOT EXISTS archive_version INTEGER,
    ADD COLUMN IF NOT EXISTS archive_key_id VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archive_plaintext_sha256 BYTEA,
    ADD COLUMN IF NOT EXISTS archive_plaintext_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS archive_status VARCHAR(32) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS archive_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archive_content_lost BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS archive_deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS disposition_status VARCHAR(32) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS disposition_target VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS disposition_transitioned BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS email_delivery_status VARCHAR(16) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS email_delivery_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS legacy_source_job_id BIGINT;

UPDATE kitty_risk_logs
SET email_delivery_status = 'sent',
    email_delivery_claimed_at = COALESCE(email_delivery_claimed_at, created_at, NOW())
WHERE email_sent = TRUE AND email_delivery_claimed_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'kitty_risk_logs'::regclass
          AND conname = 'kitty_risk_logs_email_delivery_status_check'
    ) THEN
        ALTER TABLE kitty_risk_logs
            ADD CONSTRAINT kitty_risk_logs_email_delivery_status_check
            CHECK (email_delivery_status IN ('none', 'claimed', 'sent', 'failed'));
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_kitty_risk_logs_archive_id
    ON kitty_risk_logs(archive_id) WHERE archive_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_kitty_risk_logs_legacy_source_job
    ON kitty_risk_logs(legacy_source_job_id) WHERE legacy_source_job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_archive_status
    ON kitty_risk_logs(archive_status, created_at DESC)
    WHERE archive_status <> 'none';

CREATE TABLE IF NOT EXISTS kitty_risk_log_chunks (
    log_id BIGINT NOT NULL REFERENCES kitty_risk_logs(id) ON DELETE CASCADE,
    archive_id UUID NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_total INTEGER NOT NULL,
    nonce BYTEA NOT NULL,
    ciphertext BYTEA NOT NULL,
    plaintext_bytes INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (log_id, chunk_index),
    UNIQUE (archive_id, chunk_index),
    CHECK (chunk_index >= 0),
    CHECK (chunk_total > 0),
    CHECK (chunk_index < chunk_total),
    CHECK (octet_length(nonce) = 24),
    CHECK (plaintext_bytes >= 0)
);

CREATE INDEX IF NOT EXISTS idx_kitty_risk_log_chunks_archive
    ON kitty_risk_log_chunks(archive_id, chunk_index);

-- Sensitive archive operations are append-only and intentionally separate
-- from the general audit-log retention policy.
CREATE TABLE IF NOT EXISTS kitty_risk_archive_access_audits (
    id BIGSERIAL PRIMARY KEY,
    log_id BIGINT NOT NULL REFERENCES kitty_risk_logs(id),
    archive_id UUID,
    actor_user_id BIGINT REFERENCES users(id),
    action VARCHAR(32) NOT NULL,
    request_id VARCHAR(255) NOT NULL DEFAULT '',
    result VARCHAR(32) NOT NULL,
    bytes_served BIGINT NOT NULL DEFAULT 0,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kitty_risk_archive_audits_log
    ON kitty_risk_archive_access_audits(log_id, created_at DESC);

-- Retry state is idempotent by operation key and retained until success. A
-- worker may lease a row; capped exponential backoff changes next_attempt_at
-- but never drops the operation after a finite number of attempts.
CREATE TABLE IF NOT EXISTS kitty_risk_retry_operations (
    id BIGSERIAL PRIMARY KEY,
    operation_key VARCHAR(255) NOT NULL UNIQUE,
    operation_type VARCHAR(32) NOT NULL,
    log_id BIGINT REFERENCES kitty_risk_logs(id),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    leased_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kitty_risk_retry_due
    ON kitty_risk_retry_operations(next_attempt_at, id)
    WHERE completed_at IS NULL;

-- Online preparation and atomic cutover support for retiring Prompt Audit.
-- The staging payload contains metadata and encrypted archive chunks only.
ALTER TABLE kitty_risk_logs
    ADD COLUMN IF NOT EXISTS legacy_status VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS legacy_event_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS legacy_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kitty_risk_logs
    ALTER COLUMN user_email TYPE VARCHAR(320),
    ALTER COLUMN api_key_name TYPE VARCHAR(255);


-- YuFeng XGuard policy provenance and fragment-cache replay linkage.
ALTER TABLE kitty_risk_logs
    ADD COLUMN IF NOT EXISTS cache_hit BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS decision_source VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_log_id BIGINT,
    ADD COLUMN IF NOT EXISTS replay_of_input_hash VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fragment_role VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fragment_kind VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS context_class VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fragment_path VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_namespace VARCHAR(192) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS policy_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_profile VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS evidence_policy_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS keyword_tier VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS keyword_rule_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS evidence_mode VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS evidence_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS parser_status VARCHAR(32) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_source_log_id
    ON kitty_risk_logs(source_log_id) WHERE source_log_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_replay_hash
    ON kitty_risk_logs(replay_of_input_hash) WHERE replay_of_input_hash <> '';
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_context_profile
    ON kitty_risk_logs(context_class, model_profile, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_decision_source
    ON kitty_risk_logs(decision_source, created_at DESC);

-- Bounded, redacted keyword windows actually supplied to the second layer.
ALTER TABLE kitty_risk_logs
    ADD COLUMN IF NOT EXISTS evidence_windows JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE kitty_risk_logs
    DROP CONSTRAINT IF EXISTS kitty_risk_logs_evidence_windows_array;
ALTER TABLE kitty_risk_logs
    ADD CONSTRAINT kitty_risk_logs_evidence_windows_array
    CHECK (jsonb_typeof(evidence_windows) = 'array') NOT VALID;

-- DeepSeek reviewer provenance. Store only structured, redacted outcomes;
-- request credentials and full provider responses are forbidden.
ALTER TABLE kitty_risk_logs
    ADD COLUMN IF NOT EXISTS deepseek_confidence DECIMAL(6, 5),
    ADD COLUMN IF NOT EXISTS deepseek_category VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deepseek_reason VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS review_outcome VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewer_disagreement BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS review_attempts JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Preserve the absence of a DeepSeek review for historical rows. The ALTERs
-- also repair databases that ran an earlier pre-release draft of this migration.
ALTER TABLE kitty_risk_logs
    ALTER COLUMN deepseek_confidence DROP DEFAULT,
    ALTER COLUMN deepseek_confidence DROP NOT NULL;

UPDATE kitty_risk_logs
SET deepseek_confidence = NULL
WHERE deepseek_confidence = 0
  AND deepseek_category = ''
  AND deepseek_reason = ''
  AND review_outcome = ''
  AND review_attempts = '[]'::jsonb;

ALTER TABLE kitty_risk_logs
    DROP CONSTRAINT IF EXISTS kitty_risk_logs_deepseek_confidence_range;
ALTER TABLE kitty_risk_logs
    ADD CONSTRAINT kitty_risk_logs_deepseek_confidence_range
    CHECK (deepseek_confidence >= 0 AND deepseek_confidence <= 1) NOT VALID;

ALTER TABLE kitty_risk_logs
    DROP CONSTRAINT IF EXISTS kitty_risk_logs_review_attempts_array;
ALTER TABLE kitty_risk_logs
    ADD CONSTRAINT kitty_risk_logs_review_attempts_array
    CHECK (jsonb_typeof(review_attempts) = 'array') NOT VALID;

CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_review_outcome
    ON kitty_risk_logs(review_outcome, created_at DESC)
    WHERE review_outcome <> '';

-- This independent table is new on both Kitty sites.
CREATE INDEX IF NOT EXISTS idx_kitty_risk_logs_action_created_at
    ON kitty_risk_logs(action, created_at DESC);
