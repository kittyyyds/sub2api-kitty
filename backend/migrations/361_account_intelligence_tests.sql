CREATE TABLE IF NOT EXISTS account_intelligence_tests (
 id UUID PRIMARY KEY,
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 request_id UUID NOT NULL,
 status VARCHAR(20) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 finished_at TIMESTAMPTZ,
 payload JSONB NOT NULL,
 UNIQUE(account_id, request_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS account_intelligence_one_running ON account_intelligence_tests(account_id) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS account_intelligence_history ON account_intelligence_tests(account_id, created_at DESC, id DESC);
