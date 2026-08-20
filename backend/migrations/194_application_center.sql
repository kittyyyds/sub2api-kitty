CREATE TABLE IF NOT EXISTS application_runs (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_slug VARCHAR(64) NOT NULL,
    app_revision INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'running',
    planner_api_key_id BIGINT NOT NULL REFERENCES api_keys(id),
    media_api_key_id BIGINT NOT NULL REFERENCES api_keys(id),
    session_id VARCHAR(80) NOT NULL,
    current_step_key VARCHAR(64),
    input JSONB NOT NULL DEFAULT '{}'::jsonb,
    output JSONB,
    error_code VARCHAR(64),
    error_message VARCHAR(2000),
    retry_count INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    CONSTRAINT application_runs_status_valid
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT application_runs_revision_positive CHECK (app_revision > 0),
    CONSTRAINT application_runs_retry_count_nonnegative CHECK (retry_count >= 0),
    CONSTRAINT application_runs_version_positive CHECK (version > 0),
    CONSTRAINT application_runs_session_unique UNIQUE (session_id)
);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_created
    ON application_runs (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_status_created
    ON application_runs (user_id, status, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_app_created
    ON application_runs (user_id, app_slug, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_application_runs_session
    ON application_runs (session_id);

CREATE TABLE IF NOT EXISTS application_run_steps (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES application_runs(id) ON DELETE CASCADE,
    step_key VARCHAR(64) NOT NULL,
    sequence INTEGER NOT NULL,
    executor VARCHAR(32) NOT NULL,
    credential_role VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    request JSONB NOT NULL DEFAULT '{}'::jsonb,
    output JSONB,
    gateway_request_id VARCHAR(256),
    error_code VARCHAR(64),
    error_message VARCHAR(2000),
    attempt INTEGER NOT NULL DEFAULT 1,
    version BIGINT NOT NULL DEFAULT 1,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT application_run_steps_status_valid
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT application_run_steps_executor_valid
        CHECK (executor IN (
            'chat_completions',
            'image_generation',
            'image_edit',
            'video_generation',
            'video_edit',
            'video_extension'
        )),
    CONSTRAINT application_run_steps_credential_role_valid
        CHECK (credential_role IN ('planner', 'media')),
    CONSTRAINT application_run_steps_sequence_positive CHECK (sequence > 0),
    CONSTRAINT application_run_steps_attempt_positive CHECK (attempt > 0),
    CONSTRAINT application_run_steps_version_positive CHECK (version > 0),
    CONSTRAINT application_run_steps_run_step_unique UNIQUE (run_id, step_key)
);

CREATE INDEX IF NOT EXISTS idx_application_run_steps_run_sequence
    ON application_run_steps (run_id, sequence, id);

CREATE INDEX IF NOT EXISTS idx_application_run_steps_run_status
    ON application_run_steps (run_id, status, sequence);
