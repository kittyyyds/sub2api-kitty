-- Retire Custom's input-audit modules and standalone supplier-balance page.
-- Payment/admin operation logs and upstream wallet/account snapshots are shared
-- by retained features and are deliberately outside this migration.
DROP TABLE IF EXISTS prompt_audit_events;
DROP TABLE IF EXISTS prompt_audit_jobs;
DROP TABLE IF EXISTS content_moderation_logs;

DELETE FROM settings
WHERE key IN (
    'risk_control_enabled',
    'content_moderation_config',
    'prompt_audit_config',
    'cyber_policy_user_allowlist',
    'cyber_session_block_enabled',
    'cyber_session_block_ttl_seconds',
    'cyber_session_identity_strict_enabled',
    'supplier_balance_alert_thresholds_v1'
)
OR key LIKE 'notification_email_template:content_moderation.%';
