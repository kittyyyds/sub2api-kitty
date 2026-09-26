-- Independent video platform and durable, owner-scoped asynchronous billing.
CREATE TABLE IF NOT EXISTS video_tasks (
    id TEXT PRIMARY KEY,
    upstream_task_id TEXT,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    subscription_id BIGINT,
    pending JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'submitting'
        CHECK (status IN ('submitting','pending','unknown','done','failed','needs_review')),
    next_poll_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, api_key_id, upstream_task_id)
);
CREATE INDEX IF NOT EXISTS video_tasks_poll_idx ON video_tasks(next_poll_at)
    WHERE status = 'pending';

-- Unsettled tasks retain the identities required by the billing transaction.
-- Keep the predicate literal in every guard so these bounded indexes are usable.
CREATE INDEX IF NOT EXISTS video_tasks_unsettled_user_idx ON video_tasks(user_id)
    WHERE status IN ('submitting','pending','unknown','needs_review');
CREATE INDEX IF NOT EXISTS video_tasks_unsettled_group_idx ON video_tasks(group_id)
    WHERE status IN ('submitting','pending','unknown','needs_review');
CREATE INDEX IF NOT EXISTS video_tasks_unsettled_account_idx ON video_tasks(account_id)
    WHERE status IN ('submitting','pending','unknown','needs_review');
CREATE INDEX IF NOT EXISTS video_tasks_unsettled_key_idx ON video_tasks(api_key_id)
    WHERE status IN ('submitting','pending','unknown','needs_review');
CREATE INDEX IF NOT EXISTS video_tasks_unsettled_subscription_idx ON video_tasks(subscription_id)
    WHERE status IN ('submitting','pending','unknown','needs_review') AND subscription_id IS NOT NULL;

CREATE OR REPLACE FUNCTION validate_video_task_identities()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    task_user users%ROWTYPE;
    task_group groups%ROWTYPE;
    task_account accounts%ROWTYPE;
    task_key api_keys%ROWTYPE;
    task_subscription user_subscriptions%ROWTYPE;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF ROW(NEW.id,NEW.user_id,NEW.group_id,NEW.account_id,NEW.api_key_id,NEW.subscription_id)
           IS DISTINCT FROM ROW(OLD.id,OLD.user_id,OLD.group_id,OLD.account_id,OLD.api_key_id,OLD.subscription_id) THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task identity is immutable';
        END IF;
        RETURN NEW;
    END IF;

    -- Global lock order for submission: user -> group -> account -> key -> subscription.
    -- Parent mutation triggers run under the parent's row lock and never acquire
    -- another parent lock. At the application's READ COMMITTED isolation, a
    -- deletion/identity change that waited here observes the committed task.
    SELECT * INTO task_user FROM users WHERE id=NEW.user_id FOR UPDATE;
    IF NOT FOUND OR task_user.deleted_at IS NOT NULL OR task_user.status<>'active' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task requires an active user';
    END IF;
    SELECT * INTO task_group FROM groups WHERE id=NEW.group_id FOR UPDATE;
    IF NOT FOUND OR task_group.deleted_at IS NOT NULL OR task_group.status<>'active'
       OR task_group.platform<>'video' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task requires an active video group';
    END IF;
    SELECT * INTO task_account FROM accounts WHERE id=NEW.account_id FOR UPDATE;
    IF NOT FOUND OR task_account.deleted_at IS NOT NULL OR task_account.status<>'active'
       OR task_account.platform<>'video' OR task_account.type<>'apikey'
       OR btrim(COALESCE(task_account.credentials->>'base_url',''))=''
       OR btrim(COALESCE(task_account.credentials->>'api_key',''))=''
       OR COALESCE(task_account.credentials->>'grok_media_provider','') NOT IN ('','suanli_special') THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task requires a configured active video account';
    END IF;
    SELECT * INTO task_key FROM api_keys WHERE id=NEW.api_key_id FOR UPDATE;
    IF NOT FOUND OR task_key.deleted_at IS NOT NULL OR task_key.status<>'active'
       OR task_key.user_id<>NEW.user_id OR task_key.group_id IS DISTINCT FROM NEW.group_id THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task key ownership or group is invalid';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM account_groups WHERE account_id=NEW.account_id AND group_id=NEW.group_id) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task account is not bound to its group';
    END IF;
    IF NEW.subscription_id IS NOT NULL THEN
        SELECT * INTO task_subscription FROM user_subscriptions WHERE id=NEW.subscription_id FOR UPDATE;
        IF NOT FOUND OR task_subscription.deleted_at IS NOT NULL OR task_subscription.status<>'active'
           OR task_subscription.starts_at>now() OR task_subscription.expires_at<=now()
           OR task_subscription.user_id<>NEW.user_id OR task_subscription.group_id<>NEW.group_id
           OR task_group.subscription_type<>'subscription' THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task subscription ownership or group is invalid';
        END IF;
    ELSIF task_group.subscription_type='subscription' THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='video task subscription is required';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_video_tasks_identity_insert ON video_tasks;
CREATE TRIGGER trg_video_tasks_identity_insert BEFORE INSERT ON video_tasks
    FOR EACH ROW EXECUTE FUNCTION validate_video_task_identities();
DROP TRIGGER IF EXISTS trg_video_tasks_identity_update ON video_tasks;
CREATE TRIGGER trg_video_tasks_identity_update
    BEFORE UPDATE OF id,user_id,group_id,account_id,api_key_id,subscription_id ON video_tasks
    FOR EACH ROW EXECUTE FUNCTION validate_video_task_identities();

CREATE OR REPLACE FUNCTION protect_unsettled_video_identity()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    identity_changed BOOLEAN;
    has_unsettled BOOLEAN;
BEGIN
    identity_changed := TG_OP='DELETE';
    IF TG_OP='UPDATE' THEN
        identity_changed := NEW.id IS DISTINCT FROM OLD.id
            OR (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL);
        IF TG_TABLE_NAME='api_keys' THEN
            identity_changed := identity_changed OR NEW.user_id IS DISTINCT FROM OLD.user_id
                OR NEW.group_id IS DISTINCT FROM OLD.group_id;
        ELSIF TG_TABLE_NAME='groups' THEN
            identity_changed := identity_changed OR NEW.platform IS DISTINCT FROM OLD.platform
                OR NEW.subscription_type IS DISTINCT FROM OLD.subscription_type;
        ELSIF TG_TABLE_NAME='accounts' THEN
            identity_changed := identity_changed OR NEW.platform IS DISTINCT FROM OLD.platform
                OR NEW.type IS DISTINCT FROM OLD.type
                OR (NEW.credentials->>'base_url') IS DISTINCT FROM (OLD.credentials->>'base_url')
                OR COALESCE(NEW.credentials->>'grok_media_provider','')
                    IS DISTINCT FROM COALESCE(OLD.credentials->>'grok_media_provider','');
        ELSIF TG_TABLE_NAME='user_subscriptions' THEN
            identity_changed := identity_changed OR NEW.user_id IS DISTINCT FROM OLD.user_id
                OR NEW.group_id IS DISTINCT FROM OLD.group_id;
        END IF;
    END IF;
    IF NOT identity_changed THEN RETURN NEW; END IF;

    -- Each branch is static SQL: no dynamic identifier/value interpolation.
    -- Key strings, prices, status flags and token rotation are deliberately not
    -- identity fields; accepted tasks can still finish after admission is disabled.
    IF TG_TABLE_NAME='users' THEN
        SELECT EXISTS (SELECT 1 FROM video_tasks WHERE user_id=OLD.id
            AND status IN ('submitting','pending','unknown','needs_review')) INTO has_unsettled;
    ELSIF TG_TABLE_NAME='groups' THEN
        SELECT EXISTS (SELECT 1 FROM video_tasks WHERE group_id=OLD.id
            AND status IN ('submitting','pending','unknown','needs_review')) INTO has_unsettled;
    ELSIF TG_TABLE_NAME='accounts' THEN
        SELECT EXISTS (SELECT 1 FROM video_tasks WHERE account_id=OLD.id
            AND status IN ('submitting','pending','unknown','needs_review')) INTO has_unsettled;
    ELSIF TG_TABLE_NAME='api_keys' THEN
        SELECT EXISTS (SELECT 1 FROM video_tasks WHERE api_key_id=OLD.id
            AND status IN ('submitting','pending','unknown','needs_review')) INTO has_unsettled;
    ELSIF TG_TABLE_NAME='user_subscriptions' THEN
        SELECT EXISTS (SELECT 1 FROM video_tasks WHERE subscription_id=OLD.id
            AND status IN ('submitting','pending','unknown','needs_review')) INTO has_unsettled;
    ELSE
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='unexpected video task identity table';
    END IF;
    IF has_unsettled THEN
        RAISE EXCEPTION USING ERRCODE='23514',
            MESSAGE='identity has unsettled video tasks; finish or reconcile them before deletion or rebinding';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_users_unsettled_video_identity ON users;
CREATE TRIGGER trg_users_unsettled_video_identity BEFORE UPDATE OF id,deleted_at OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();
DROP TRIGGER IF EXISTS trg_groups_unsettled_video_identity ON groups;
CREATE TRIGGER trg_groups_unsettled_video_identity BEFORE UPDATE OF id,deleted_at,platform,subscription_type OR DELETE ON groups
    FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();
DROP TRIGGER IF EXISTS trg_accounts_unsettled_video_identity ON accounts;
CREATE TRIGGER trg_accounts_unsettled_video_identity BEFORE UPDATE OF id,deleted_at,platform,type,credentials OR DELETE ON accounts
    FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();
DROP TRIGGER IF EXISTS trg_api_keys_unsettled_video_identity ON api_keys;
CREATE TRIGGER trg_api_keys_unsettled_video_identity BEFORE UPDATE OF id,deleted_at,user_id,group_id OR DELETE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();
DROP TRIGGER IF EXISTS trg_subscriptions_unsettled_video_identity ON user_subscriptions;
CREATE TRIGGER trg_subscriptions_unsettled_video_identity BEFORE UPDATE OF id,deleted_at,user_id,group_id OR DELETE ON user_subscriptions
    FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic','openai','gemini','antigravity','grok',
        'kimi','zhipu','deepseek','minimax','opencode_go','video'));
ALTER TABLE composite_model_routes DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
ALTER TABLE composite_model_routes ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic','openai','gemini','antigravity','grok',
        'kimi','zhipu','deepseek','minimax','opencode_go','video'));
