-- Durable model capability and identity audit history.
-- The tables intentionally contain only model names and bounded diagnostic
-- metadata; request/response bodies and credentials are never persisted.
CREATE TABLE IF NOT EXISTS model_capability_probe_runs (
    id BIGSERIAL PRIMARY KEY,
    "trigger" VARCHAR(32) NOT NULL DEFAULT 'manual',
    status VARCHAR(32) NOT NULL DEFAULT 'running',
    requested_by BIGINT,
    total_count INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    verified_count INTEGER NOT NULL DEFAULT 0,
    mismatch_count INTEGER NOT NULL DEFAULT 0,
    unsupported_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    total_items INTEGER NOT NULL DEFAULT 0,
    completed_items INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    created_by BIGINT,
    error_message TEXT,
    error_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_capability_probe_runs_started_at
    ON model_capability_probe_runs (started_at DESC);

CREATE TABLE IF NOT EXISTS model_capability_probe_results (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES model_capability_probe_runs(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    account_name VARCHAR(200) NOT NULL DEFAULT '',
    platform VARCHAR(64) NOT NULL DEFAULT '',
    group_id BIGINT,
    channel_id BIGINT,
    requested_model VARCHAR(200) NOT NULL,
    mapped_model VARCHAR(200),
    response_model VARCHAR(200),
    upstream_model VARCHAR(200),
    protocol VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    capability VARCHAR(64),
    capability_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    identity_status VARCHAR(32) NOT NULL DEFAULT 'unobserved',
    confidence VARCHAR(16) NOT NULL DEFAULT 'low',
    evidence_source VARCHAR(32) NOT NULL DEFAULT 'active_probe',
    source VARCHAR(32) NOT NULL DEFAULT 'active',
    http_status INTEGER,
    latency_ms BIGINT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    tool_call_found BOOLEAN NOT NULL DEFAULT FALSE,
    tool_arguments_valid BOOLEAN NOT NULL DEFAULT FALSE,
    tool_name VARCHAR(200),
    evidence_code VARCHAR(128),
    evidence VARCHAR(128),
    error_message TEXT,
    response_summary VARCHAR(512),
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_capability_probe_results_lookup
    ON model_capability_probe_results (account_id, requested_model, protocol, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_model_capability_probe_results_run
    ON model_capability_probe_results (run_id, id);
CREATE INDEX IF NOT EXISTS idx_model_capability_probe_results_checked_at
    ON model_capability_probe_results (checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_model_capability_probe_results_identity
    ON model_capability_probe_results (identity_status, evidence_source);

COMMENT ON TABLE model_capability_probe_runs IS
    'Durable bounded model capability audit runs; no credentials or request bodies';
COMMENT ON TABLE model_capability_probe_results IS
    'Capability and model identity evidence from active probes or passive traffic';

-- The IF NOT EXISTS clauses keep this migration safe for installations that
-- briefly ran an earlier development version of the audit tables.
ALTER TABLE model_capability_probe_runs
    ADD COLUMN IF NOT EXISTS "trigger" VARCHAR(32) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS requested_by BIGINT,
    ADD COLUMN IF NOT EXISTS total_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS completed_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS verified_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mismatch_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS unsupported_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS requested_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS total_items INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS completed_items INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS created_by BIGINT,
    ADD COLUMN IF NOT EXISTS error_message TEXT,
    ADD COLUMN IF NOT EXISTS error_summary TEXT,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE model_capability_probe_results
    ADD COLUMN IF NOT EXISTS account_name VARCHAR(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS platform VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS channel_id BIGINT,
    ADD COLUMN IF NOT EXISTS response_model VARCHAR(200),
    ADD COLUMN IF NOT EXISTS upstream_model VARCHAR(200),
    ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS capability VARCHAR(64),
    ADD COLUMN IF NOT EXISTS capability_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS identity_status VARCHAR(32) NOT NULL DEFAULT 'unobserved',
    ADD COLUMN IF NOT EXISTS confidence VARCHAR(16) NOT NULL DEFAULT 'low',
    ADD COLUMN IF NOT EXISTS evidence_source VARCHAR(32) NOT NULL DEFAULT 'active_probe',
    ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS http_status INTEGER,
    ADD COLUMN IF NOT EXISTS latency_ms BIGINT,
    ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS success_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failure_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tool_name VARCHAR(200),
    ADD COLUMN IF NOT EXISTS evidence_code VARCHAR(128),
    ADD COLUMN IF NOT EXISTS evidence VARCHAR(128),
    ADD COLUMN IF NOT EXISTS error_message TEXT,
    ADD COLUMN IF NOT EXISTS response_summary VARCHAR(512),
    ADD COLUMN IF NOT EXISTS checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
