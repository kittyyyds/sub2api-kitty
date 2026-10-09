-- Additive: legacy tasks retain their original completion billing.
ALTER TABLE video_tasks ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE video_tasks ADD COLUMN IF NOT EXISTS request_hash TEXT;
ALTER TABLE video_tasks ADD COLUMN IF NOT EXISTS hold_amount NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE video_tasks ADD COLUMN IF NOT EXISTS billing_state TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE video_tasks ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS video_tasks_idempotency_key_idx
    ON video_tasks(api_key_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS video_tasks_prepaid_references_idx
    ON video_tasks(updated_at) WHERE billing_state IN ('held','refund_pending','captured','refunded');

-- Persist the audit in the same transaction as each group's new price.
CREATE TABLE IF NOT EXISTS kitty_video_price_changes (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    group_id BIGINT NOT NULL,
    model TEXT NOT NULL,
    old_price JSONB NOT NULL,
    new_price JSONB NOT NULL
);
