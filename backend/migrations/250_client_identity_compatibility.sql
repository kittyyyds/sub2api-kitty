-- Pin the forwarding contract of imported V2 profiles, including disabled
-- profiles. Merely freezing installation IDs in 249 must not rehash existing
-- custom prompt cache keys. This marker never enables a profile or policy.
-- New/duplicated rows get "integrated" from the server-owned create path.
UPDATE accounts
SET extra = jsonb_set(extra, '{client_identity_compatibility}', '"identity_v2"'::jsonb, true)
WHERE platform = 'openai' AND type = 'oauth'
  AND COALESCE(extra->>'client_identity_compatibility', '') = ''
  AND (extra->'client_identity' IS NULL OR extra->'client_identity' = 'null'::jsonb)
  AND jsonb_typeof(extra->'fingerprint_v2') = 'object'
  AND extra#>>'{fingerprint_v2,schema_version}' = '2';
