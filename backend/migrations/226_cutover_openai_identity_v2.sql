-- Prepare the independent v2 namespace without changing the still-running
-- previous application. New profiles remain opt-in. Legacy keys are retired
-- by the v2 runtime, then archived/removed by the verified release step AFTER
-- switching the application. This makes image rollback safe during migration.
UPDATE accounts
SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{fingerprint_v2_seed}',
  to_jsonb(gen_random_uuid()::text), true)
WHERE deleted_at IS NULL
  AND platform = 'openai'
  AND type = 'oauth'
  AND extra->'fingerprint_v2'->>'enabled' = 'true'
  AND extra->'fingerprint_v2'->>'schema_version' = '2'
  AND NOT COALESCE(
    extra->>'fingerprint_v2_seed' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    AND extra->>'fingerprint_v2_seed' <> '00000000-0000-0000-0000-000000000000', false);
