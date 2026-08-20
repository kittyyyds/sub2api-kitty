ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS usage_audit JSONB;

COMMENT ON COLUMN usage_logs.usage_audit IS
    'Compact upstream usage evidence, local token estimates, deviations and confidence; excludes prompts and generated content';

CREATE INDEX IF NOT EXISTS idx_usage_logs_usage_audit_status_created
    ON usage_logs ((usage_audit->>'status'), created_at DESC)
    WHERE usage_audit IS NOT NULL;
