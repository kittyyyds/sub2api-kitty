-- Only new explicit interval plans participate; existing cron plans stay unchanged.
CREATE UNIQUE INDEX IF NOT EXISTS scheduled_iq_interval_account_unique ON scheduled_test_plans(account_id)
 WHERE pelican_config->>'schedule_mode'='completion_interval';
CREATE INDEX IF NOT EXISTS scheduled_iq_interval_due ON scheduled_test_plans(next_run_at)
 WHERE enabled=true AND pelican_config->>'schedule_mode'='completion_interval';
