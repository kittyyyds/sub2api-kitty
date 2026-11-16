-- VM2-managed OpenAI accounts. The runtime logs the account in and holds its
-- tokens; Sub2API keeps the account's identity, routing and accounting only.
-- A managed binding reuses the dedicated-slot route (managed proxy, guards);
-- `mode` says who owns the credential.
ALTER TABLE openai_vm_bindings
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'sub_token' CHECK (mode IN ('sub_token','vm_managed')),
    -- What the administrator sees: available, awaiting_login, needs_relogin, ...
    ADD COLUMN managed_status TEXT NOT NULL DEFAULT '',
    -- Last slot snapshot reported by the bridge. Never a token, password or 2FA secret.
    ADD COLUMN managed_detail JSONB NOT NULL DEFAULT '{}',
    -- Set once, at the first successful login; scheduling is the administrator's from then on.
    ADD COLUMN activated_at TIMESTAMPTZ,
    ADD COLUMN synced_at TIMESTAMPTZ;

-- Batch import progress. Deliberately no column can hold a password or a 2FA
-- secret: those exist only in memory until the login request has been sent.
CREATE TABLE openai_vm_batch_jobs (
    id BIGSERIAL PRIMARY KEY,
    created_by BIGINT,
    status TEXT NOT NULL DEFAULT 'queued',
    runtime_type TEXT NOT NULL CHECK (runtime_type IN ('docker','kvm')),
    total INT NOT NULL DEFAULT 0,
    options JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);
CREATE TABLE openai_vm_batch_items (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES openai_vm_batch_jobs(id) ON DELETE CASCADE,
    line_no INT NOT NULL,
    email TEXT NOT NULL,
    proxy_id BIGINT,
    account_id BIGINT,
    status TEXT NOT NULL DEFAULT 'queued',
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, line_no)
);

-- Same guard as 308, plus the credential owner: an account is marked
-- openai_vm_managed exactly when its binding is vm_managed. Without this an
-- ordinary edit could drop the mark and send a token-less account down the
-- token paths, or mark a sub_token account and strand its real credential.
-- Managed accounts have no token for a shadow to borrow, so they have none.
CREATE OR REPLACE FUNCTION guard_openai_vm_account() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE managed BIGINT; binding_mode TEXT; owner_id BIGINT; parent_proxy BIGINT; parent_fallback BIGINT;
BEGIN
    IF TG_OP='INSERT' AND NEW.parent_account_id IS NOT NULL THEN
        -- Serialize shadow creation with lifecycle changes on its parent. Read
        -- the current route after the lock, never a caller's stale snapshot.
        SELECT proxy_id,proxy_fallback_origin_id INTO parent_proxy,parent_fallback
        FROM accounts WHERE id=NEW.parent_account_id FOR UPDATE;
        NEW.proxy_id := parent_proxy;
        NEW.proxy_fallback_origin_id := parent_fallback;
        NEW.extra := COALESCE(NEW.extra,'{}'::jsonb)-'openai_vm_bound'-'openai_vm_managed';
    END IF;
    owner_id := COALESCE(NEW.parent_account_id, NEW.id);
    SELECT managed_proxy_id,mode INTO managed,binding_mode FROM openai_vm_bindings WHERE account_id=owner_id;
    IF managed IS NOT NULL THEN
        IF TG_OP='INSERT' THEN
            IF binding_mode='vm_managed' THEN
                RAISE EXCEPTION 'A VM2-managed OpenAI account cannot have shadow accounts';
            END IF;
            NEW.proxy_id := managed;
            NEW.extra := COALESCE(NEW.extra,'{}'::jsonb) || '{"openai_vm_bound":true}'::jsonb;
        ELSIF NEW.proxy_id IS DISTINCT FROM managed
            OR NEW.deleted_at IS NOT NULL OR NEW.platform <> 'openai' OR NEW.type <> 'oauth'
            OR NEW.parent_account_id IS DISTINCT FROM OLD.parent_account_id
            OR NEW.proxy_fallback_origin_id IS NOT NULL
            OR NEW.extra->>'openai_vm_bound' IS DISTINCT FROM 'true'
            OR (binding_mode='vm_managed') IS DISTINCT FROM (COALESCE(NEW.extra->>'openai_vm_managed','')='true') THEN
            RAISE EXCEPTION 'Detach the OpenAI VM slot before changing or deleting its account route';
        END IF;
    ELSIF TG_OP='UPDATE' AND OLD.parent_account_id IS DISTINCT FROM NEW.parent_account_id
        AND EXISTS(SELECT 1 FROM openai_vm_bindings WHERE account_id=COALESCE(OLD.parent_account_id,OLD.id)) THEN
        RAISE EXCEPTION 'Detach the OpenAI VM slot before changing account ownership';
    END IF;
    RETURN NEW;
END $$;
