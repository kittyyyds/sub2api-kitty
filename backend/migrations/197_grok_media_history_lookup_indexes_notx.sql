CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_application_run_steps_grok_video_request
ON application_run_steps (gateway_request_id, run_id)
WHERE gateway_request_id IS NOT NULL
  AND executor IN ('video_generation', 'video_edit', 'video_extension');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_grok_video_history_owner
ON usage_logs (session_id, user_id, api_key_id, group_id, account_id)
WHERE session_id IS NOT NULL
  AND video_count > 0
  AND account_id > 0;
