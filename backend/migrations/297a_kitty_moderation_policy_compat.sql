-- The upstream legacy-hit bootstrap also sees Kitty's session-only audit rows.
-- They must retain their original exemption from account-disable counters.
-- Remove only those imported counters; preserve every original audit record.
DELETE FROM content_moderation_user_hits h
USING content_moderation_logs l
WHERE h.user_id = l.user_id
  AND h.generation = 0
  AND h.request_id = 'legacy:' || l.id::text
  AND l.action IN ('security_policy_block', 'security_policy_session_block');
