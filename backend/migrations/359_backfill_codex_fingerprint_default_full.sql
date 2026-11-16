-- Backfill Codex fingerprint seeds now that missing/invalid mode defaults to full.
-- Eligible: OpenAI oauth and setup-token whose mode is not an explicit off.
-- VM-bound/managed accounts keep their legacy default (off) unless an explicit
-- mode is configured, so they are skipped here; an explicit mode already gets a
-- seed from the account update path.
-- Idempotent: valid canonical seeds are preserved on rerun.
UPDATE accounts
SET extra = jsonb_set(
    COALESCE(extra, '{}'::jsonb),
    '{codex_fingerprint_seed}',
    to_jsonb(gen_random_uuid()::text),
    true
)
WHERE deleted_at IS NULL
  AND platform = 'openai'
  AND type IN ('oauth', 'setup-token')
  AND COALESCE(NULLIF(btrim(extra->>'codex_fingerprint_mode'), ''), 'full') <> 'off'
  AND COALESCE(extra->>'openai_vm_bound', '') <> 'true'
  AND COALESCE(extra->>'openai_vm_managed', '') <> 'true'
  AND (
      extra->>'codex_fingerprint_seed' IS NULL
      OR btrim(extra->>'codex_fingerprint_seed') = ''
      OR NOT (
          extra->>'codex_fingerprint_seed' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
          AND extra->>'codex_fingerprint_seed' <> '00000000-0000-0000-0000-000000000000'
      )
  );
