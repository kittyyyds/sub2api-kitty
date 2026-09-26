-- Add resilient execution metadata and a dedicated, redacted result store for
-- paired behavior audits. Historical migration 234 remains immutable.
ALTER TABLE model_capability_probe_runs
    ADD COLUMN IF NOT EXISTS request_key VARCHAR(64),
    ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Structured summaries must not be constrained by the original diagnostic
-- preview size. No request or response bodies are stored in this column.
ALTER TABLE model_capability_probe_results
    ALTER COLUMN response_summary TYPE TEXT;

CREATE TABLE IF NOT EXISTS model_behavior_audit_results (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES model_capability_probe_runs(id) ON DELETE CASCADE,
    target_account_id BIGINT NOT NULL,
    target_account_name VARCHAR(200) NOT NULL DEFAULT '',
    anchor_account_id BIGINT NOT NULL,
    anchor_account_name VARCHAR(200) NOT NULL DEFAULT '',
    case_key VARCHAR(64) NOT NULL,
    case_label VARCHAR(128) NOT NULL DEFAULT '',
    requested_model VARCHAR(200) NOT NULL,
    target_mapped_model VARCHAR(200) NOT NULL,
    anchor_mapped_model VARCHAR(200) NOT NULL,
    target_response_model VARCHAR(200),
    anchor_response_model VARCHAR(200),
    comparison_status VARCHAR(32) NOT NULL,
    target_status VARCHAR(32) NOT NULL,
    anchor_status VARCHAR(32) NOT NULL,
    target_correct BOOLEAN NOT NULL DEFAULT FALSE,
    anchor_correct BOOLEAN NOT NULL DEFAULT FALSE,
    target_http_status INTEGER,
    anchor_http_status INTEGER,
    target_latency_ms BIGINT,
    anchor_latency_ms BIGINT,
    target_output_hash VARCHAR(64),
    anchor_output_hash VARCHAR(64),
    expected_output_hash VARCHAR(64) NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, target_account_id, case_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_model_capability_probe_runs_running_request
    ON model_capability_probe_runs ("trigger", request_key)
    WHERE status = 'running' AND request_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_model_behavior_audit_results_run
    ON model_behavior_audit_results (run_id, id);

COMMENT ON TABLE model_behavior_audit_results IS
    'Redacted paired behavior evidence; stores hashes and outcomes, never prompts, answers, bodies, or credentials';
