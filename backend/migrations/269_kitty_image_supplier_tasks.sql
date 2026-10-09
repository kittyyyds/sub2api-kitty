-- Independent task journal. Never store API credentials or reference uploads.
CREATE TABLE IF NOT EXISTS image_supplier_tasks (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    idempotency_hash TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    snapshot JSONB NOT NULL,
    upstream_task_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'submitting',
    result BYTEA,
    next_poll_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '6 minutes',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (api_key_id, idempotency_hash)
);
CREATE INDEX IF NOT EXISTS image_supplier_tasks_pending_idx
ON image_supplier_tasks(next_poll_at) WHERE status IN ('pending','ready','submitting');
CREATE INDEX IF NOT EXISTS image_supplier_tasks_result_expiry_idx
ON image_supplier_tasks(updated_at) WHERE result IS NOT NULL AND status='completed';
