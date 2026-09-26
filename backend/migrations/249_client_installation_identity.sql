-- Freeze the identity that was actually used before integrating the clients.
-- UUID seeds alone are not sufficient: old V2 included row ID and epoch.
WITH candidates AS (
 SELECT id, extra,
   COALESCE(extra->'client_identity',extra->'fingerprint_v2') AS profile,
   (extra#>>'{anti_degrade,enabled}'='true' AND extra#>>'{anti_degrade,mode}'='mode1'
    AND extra#>>'{anti_degrade,policy_version}' IN ('2','3')) IS TRUE AS mode1
 FROM accounts WHERE platform='openai' AND type='oauth'
   AND COALESCE(extra->>'client_installation_id','')=''
), materials AS (
 SELECT *, CASE
   WHEN profile->>'enabled'='true' AND profile->>'schema_version'='2'
    AND extra->>'fingerprint_v2_seed' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    AND extra->>'fingerprint_v2_seed'<>'00000000-0000-0000-0000-000000000000'
    AND COALESCE(profile->>'identity_epoch','1') ~ '^([1-9][0-9]{0,5}|1000000)$'
    THEN '["installation","'||id::text||'","'||(extra->>'fingerprint_v2_seed')||'","'||COALESCE(profile->>'identity_epoch','1')||'"]'
   WHEN extra->>'codex_fingerprint_seed' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    AND extra->>'codex_fingerprint_seed'<>'00000000-0000-0000-0000-000000000000'
    THEN (CASE WHEN mode1 THEN 'sub2api:mode1-install-id:v2:' ELSE 'sub2api:codex-install-id:v2:' END)||(extra->>'codex_fingerprint_seed')
   ELSE NULL END AS material
 FROM candidates
), digests AS (
 SELECT *, substring(sha256(convert_to(material,'UTF8')) FROM 1 FOR 16) AS digest FROM materials
), identities AS (
 SELECT id, CASE
   WHEN NOT mode1 AND COALESCE(profile->>'enabled','false')<>'true' AND btrim(COALESCE(extra->>'openai_device_id',''))<>'' THEN btrim(extra->>'openai_device_id')
   WHEN digest IS NOT NULL THEN encode(set_byte(set_byte(digest,6,(get_byte(digest,6)&15)|64),8,(get_byte(digest,8)&63)|128),'hex')::uuid::text
   ELSE gen_random_uuid()::text END AS installation
 FROM digests
)
UPDATE accounts AS a SET extra=jsonb_set(COALESCE(a.extra,'{}'::jsonb),'{client_installation_id}',to_jsonb(i.installation),true)
 || CASE WHEN a.extra#>>'{fingerprint_v2,enabled}'='true' AND a.extra#>>'{fingerprint_v2,ws_mode}' IN ('off','ctx_pool','passthrough')
    THEN jsonb_build_object('openai_oauth_responses_websockets_v2_mode',a.extra#>>'{fingerprint_v2,ws_mode}') ELSE '{}'::jsonb END
FROM identities AS i WHERE a.id=i.id;
