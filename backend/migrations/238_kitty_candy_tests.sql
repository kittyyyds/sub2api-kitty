-- Separate, opt-in group question tests. No existing monitor or account settings change.
CREATE TABLE IF NOT EXISTS kitty_candy_tests (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    config JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    next_run_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id)
);

CREATE TABLE IF NOT EXISTS kitty_candy_test_runs (
    id BIGSERIAL PRIMARY KEY,
    test_id BIGINT NOT NULL REFERENCES kitty_candy_tests(id) ON DELETE CASCADE,
    status VARCHAR(24) NOT NULL CHECK (status IN ('queued','running','passed','failed','inconclusive','error','cancelled')),
    snapshot JSONB NOT NULL,
    result JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS kitty_candy_one_pending
    ON kitty_candy_test_runs(test_id) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS kitty_candy_history
    ON kitty_candy_test_runs(test_id, id DESC);
