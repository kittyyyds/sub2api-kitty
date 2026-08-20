DO $$
DECLARE
    target_group_id BIGINT;
BEGIN
    SELECT id
      INTO target_group_id
      FROM groups
     WHERE name = 'Grok 增强版'
       AND platform = 'grok'
       AND deleted_at IS NULL;

    IF target_group_id IS NULL THEN
        RAISE EXCEPTION 'Grok enhancement group was not found';
    END IF;

    UPDATE groups
       SET grok_prompt_enabled = TRUE,
           grok_prompt_mode = 'prepend',
           grok_prompt_content = '',
           updated_at = NOW()
     WHERE id = target_group_id;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
      FROM api_keys AS k
     WHERE k.group_id = target_group_id
       AND k.deleted_at IS NULL
       AND k.key <> '';
END;
$$;
