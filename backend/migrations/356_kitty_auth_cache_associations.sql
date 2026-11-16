-- Retain Custom's digest-only credentials and fallback groups while adding
-- Kitty's multi-group keys, media prices and public-group permissions.
-- Group rows contain configuration; changing updated_at alone is not a change.
CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND (to_jsonb(OLD) - 'updated_at') IS NOT DISTINCT FROM
                              (to_jsonb(NEW) - 'updated_at') THEN
        RETURN NEW;
    END IF;
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT api_key_cache_digest(k.key) FROM api_keys k
    WHERE k.deleted_at IS NULL AND k.key <> ''
      AND (k.group_id = OLD.id OR k.fallback_group_id = OLD.id
           OR EXISTS (SELECT 1 FROM api_key_groups kg WHERE kg.api_key_id = k.id AND kg.group_id = OLD.id));
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_api_key_auth_cache_invalidation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        RETURN OLD;
    END IF;
    IF ROW(OLD.key, OLD.status, OLD.deleted_at, OLD.user_id, OLD.group_id,
           OLD.fallback_group_id, OLD.ip_whitelist, OLD.ip_blacklist, OLD.expires_at,
           OLD.concurrency_limit, OLD.quota, OLD.rate_limit_5h, OLD.rate_limit_1d, OLD.rate_limit_7d)
       IS DISTINCT FROM
       ROW(NEW.key, NEW.status, NEW.deleted_at, NEW.user_id, NEW.group_id,
           NEW.fallback_group_id, NEW.ip_whitelist, NEW.ip_blacklist, NEW.expires_at,
           NEW.concurrency_limit, NEW.quota, NEW.rate_limit_5h, NEW.rate_limit_1d, NEW.rate_limit_7d) THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        IF NEW.deleted_at IS NULL AND NEW.key IS DISTINCT FROM OLD.key THEN
            PERFORM enqueue_auth_cache_invalidation(NEW.key);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_user_auth_cache_invalidation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND ROW(OLD.status, OLD.role, OLD.deleted_at, OLD.restrict_public_groups, OLD.concurrency, OLD.rpm_limit)
           IS NOT DISTINCT FROM
           ROW(NEW.status, NEW.role, NEW.deleted_at, NEW.restrict_public_groups, NEW.concurrency, NEW.rpm_limit) THEN
        RETURN NEW;
    END IF;
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT api_key_cache_digest(k.key) FROM api_keys k
    WHERE k.user_id = OLD.id AND k.deleted_at IS NULL AND k.key <> '';
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_key_group_auth_cache_invalidation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT api_key_cache_digest(k.key) FROM api_keys k
        WHERE k.id = OLD.api_key_id AND k.deleted_at IS NULL AND k.key <> '';
    END IF;
    IF TG_OP <> 'DELETE' THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT api_key_cache_digest(k.key) FROM api_keys k
        WHERE k.id = NEW.api_key_id AND k.deleted_at IS NULL AND k.key <> '';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_api_key_groups_auth_cache_invalidation ON api_key_groups;
CREATE TRIGGER trg_api_key_groups_auth_cache_invalidation
AFTER INSERT OR UPDATE OR DELETE ON api_key_groups
FOR EACH ROW EXECUTE FUNCTION enqueue_key_group_auth_cache_invalidation();

-- Any membership change alters the user's allowed-groups snapshot, including
-- public groups when restrict_public_groups is set. Invalidate all their keys.
CREATE OR REPLACE FUNCTION enqueue_allowed_group_auth_cache_invalidation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND ROW(OLD.user_id, OLD.group_id) IS NOT DISTINCT FROM ROW(NEW.user_id, NEW.group_id) THEN
        RETURN NEW;
    END IF;
    IF TG_OP <> 'INSERT' THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT api_key_cache_digest(k.key) FROM api_keys k
        WHERE k.user_id = OLD.user_id AND k.deleted_at IS NULL AND k.key <> '';
    END IF;
    IF TG_OP <> 'DELETE' THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT api_key_cache_digest(k.key) FROM api_keys k
        WHERE k.user_id = NEW.user_id AND k.deleted_at IS NULL AND k.key <> '';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
