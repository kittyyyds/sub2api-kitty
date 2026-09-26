-- Persist the model-directory evidence collected alongside active capability
-- probes. One /v1/models request is shared by every model on the same account;
-- these columns copy only the bounded verdict onto each protocol result.
ALTER TABLE model_capability_probe_results
    ADD COLUMN IF NOT EXISTS catalog_status VARCHAR(32) NOT NULL DEFAULT 'unchecked',
    ADD COLUMN IF NOT EXISTS catalog_declared BOOLEAN,
    ADD COLUMN IF NOT EXISTS catalog_http_status INTEGER,
    ADD COLUMN IF NOT EXISTS catalog_checked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS catalog_error VARCHAR(128);

COMMENT ON COLUMN model_capability_probe_results.catalog_status IS
    'Sanitized /v1/models check outcome; never stores a response body or credential';
