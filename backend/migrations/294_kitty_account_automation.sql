-- Additive state. Existing accounts are deliberately not enrolled.
CREATE TABLE IF NOT EXISTS account_iq_lifecycle (
 account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 plan_id BIGINT UNIQUE REFERENCES scheduled_test_plans(id) ON DELETE SET NULL,
 config JSONB NOT NULL,
 correct_streak INTEGER NOT NULL DEFAULT 0,
 answer29_streak INTEGER NOT NULL DEFAULT 0,
 passed_at TIMESTAMPTZ,
 quarantined_at TIMESTAMPTZ,
 quarantine_pending BOOLEAN NOT NULL DEFAULT false,
 original_groups JSONB NOT NULL DEFAULT '[]',
 evidence_samples JSONB NOT NULL DEFAULT '[]',
 fingerprint TEXT NOT NULL DEFAULT '',
 last_started_at TIMESTAMPTZ,
 last_result_id BIGINT,
 action_error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS account_iq_lifecycle_events (
 id BIGSERIAL PRIMARY KEY, account_id BIGINT NOT NULL, plan_id BIGINT,
 kind TEXT NOT NULL, evidence JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_iq_lifecycle_events_account ON account_iq_lifecycle_events(account_id,id DESC);

-- Any credentials/health recovery must respect the initial admission hold.
CREATE OR REPLACE FUNCTION kitty_iq_admission_hold() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM account_iq_lifecycle WHERE account_id=NEW.id AND passed_at IS NULL) THEN
  NEW.schedulable := false;
  NEW.concurrency := 1;
 END IF;
 IF EXISTS(SELECT 1 FROM account_iq_lifecycle WHERE account_id=NEW.id AND quarantine_pending) THEN
  NEW.schedulable := false;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS kitty_iq_admission_hold ON accounts;
CREATE TRIGGER kitty_iq_admission_hold BEFORE UPDATE OF schedulable,concurrency ON accounts
 FOR EACH ROW EXECUTE FUNCTION kitty_iq_admission_hold();

-- Verified import receipts carry no passwords, MFA secrets or bearer tokens.
CREATE TABLE IF NOT EXISTS account_import_tickets (
 id TEXT PRIMARY KEY, platform TEXT NOT NULL, source TEXT NOT NULL,
 proxy_id BIGINT, identity_key TEXT NOT NULL, fingerprint TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','ready','consumed','released')),
 account_id BIGINT, auto_assigned BOOLEAN NOT NULL DEFAULT false,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW()+interval '30 minutes',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_import_tickets_reservations ON account_import_tickets(proxy_id,expires_at) WHERE state IN ('pending','ready');
CREATE TABLE IF NOT EXISTS account_import_origins (
 account_id BIGINT PRIMARY KEY, platform TEXT NOT NULL, source TEXT NOT NULL,
 identity_key TEXT NOT NULL, ticket_id TEXT UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS proxy_binding_policy (
 id BOOLEAN PRIMARY KEY DEFAULT true CHECK(id), config JSONB NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO proxy_binding_policy(id,config) VALUES(true,'{"enabled":false,"only_twofa":true,"platforms":["openai"],"history_limit":5,"auto_assign":false}') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS proxy_binding_exits (
 proxy_id BIGINT PRIMARY KEY, exit_ip TEXT NOT NULL, proxy_revision TIMESTAMPTZ NOT NULL, checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS proxy_binding_history (
 proxy_key TEXT NOT NULL,identity_key TEXT NOT NULL,platform TEXT NOT NULL,account_type TEXT NOT NULL,source TEXT NOT NULL,
 first_bound_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),last_bound_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(proxy_key,identity_key,platform,account_type,source)
);
CREATE TABLE IF NOT EXISTS proxy_binding_allowance (
 proxy_key TEXT PRIMARY KEY,available BOOLEAN NOT NULL DEFAULT false,granted_at TIMESTAMPTZ,consumed_at TIMESTAMPTZ
);
CREATE OR REPLACE FUNCTION kitty_proxy_key(pid BIGINT) RETURNS TEXT LANGUAGE sql STABLE AS $$
 SELECT COALESCE((SELECT 'ip:'||exit_ip FROM proxy_binding_exits WHERE proxy_id=pid),'proxy:'||pid::text)
$$;
CREATE OR REPLACE FUNCTION kitty_proxy_scope(p TEXT,t TEXT,s TEXT,c JSONB) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $$
 SELECT COALESCE((c->>'enabled')::boolean,false) AND t='oauth' AND (c->'platforms') ? p AND (NOT COALESCE((c->>'only_twofa')::boolean,true) OR s='twofa')
$$;
CREATE OR REPLACE FUNCTION kitty_proxy_history_count(k TEXT,c JSONB) RETURNS BIGINT LANGUAGE sql STABLE AS $$
 SELECT count(DISTINCT identity_key) FROM proxy_binding_history WHERE proxy_key=k AND account_type='oauth' AND (c->'platforms') ? platform AND (NOT COALESCE((c->>'only_twofa')::boolean,true) OR source='twofa')
$$;
-- Known current bindings are the baseline; unknown old import sources stay unknown.
INSERT INTO proxy_binding_history(proxy_key,identity_key,platform,account_type,source)
 SELECT kitty_proxy_key(a.proxy_id),COALESCE(o.identity_key,md5(a.platform||'|'||COALESCE(NULLIF(lower(trim(a.credentials->>'email')),''),'account:'||a.id::text))),a.platform,a.type,COALESCE(o.source,'unknown')
 FROM accounts a LEFT JOIN account_import_origins o ON o.account_id=a.id WHERE a.deleted_at IS NULL AND a.proxy_id IS NOT NULL
 ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION kitty_check_proxy_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c JSONB; k TEXT; ident TEXT; src TEXT; ticket TEXT; automatic BOOLEAN:=false; n BIGINT; limit_n INT; allowance BOOLEAN;
BEGIN
 IF NEW.proxy_id IS NULL OR NEW.deleted_at IS NOT NULL THEN RETURN NEW; END IF;
 IF TG_OP='UPDATE' AND OLD.proxy_id IS NOT DISTINCT FROM NEW.proxy_id AND OLD.platform=NEW.platform AND OLD.type=NEW.type THEN RETURN NEW; END IF;
 PERFORM pg_advisory_xact_lock(71942021);
 SELECT config INTO c FROM proxy_binding_policy WHERE id=true;
 k:=kitty_proxy_key(NEW.proxy_id);
 SELECT source,identity_key INTO src,ident FROM account_import_origins WHERE account_id=NEW.id;
 ticket:=current_setting('kitty.import_ticket',true);
 IF src IS NULL AND ticket IS NOT NULL AND ticket<>'' THEN
  SELECT source,identity_key,auto_assigned INTO src,ident,automatic FROM account_import_tickets WHERE id=ticket AND state='ready' AND platform=NEW.platform AND proxy_id=NEW.proxy_id AND expires_at>NOW();
 END IF;
 src:=COALESCE(src,'unknown');
 ident:=COALESCE(ident,md5(NEW.platform||'|'||COALESCE(NULLIF(lower(trim(NEW.credentials->>'email')),''),'account:'||NEW.id::text)));
 IF kitty_proxy_scope(NEW.platform,NEW.type,src,c) THEN
  IF NOT EXISTS(SELECT 1 FROM proxies WHERE id=NEW.proxy_id AND deleted_at IS NULL AND status='active' AND (expires_at IS NULL OR expires_at>NOW())) THEN
   RAISE EXCEPTION '所选代理已停用、过期或删除';
  END IF;
  IF EXISTS(SELECT 1 FROM account_import_tickets t WHERE t.id IS DISTINCT FROM ticket AND t.state IN ('pending','ready') AND t.expires_at>NOW() AND kitty_proxy_key(t.proxy_id)=k) THEN
   RAISE EXCEPTION '此 IP 已被另一导入任务预留';
  END IF;
  IF automatic AND EXISTS(SELECT 1 FROM accounts a WHERE a.id<>NEW.id AND a.deleted_at IS NULL AND a.proxy_id IS NOT NULL AND kitty_proxy_key(a.proxy_id)=k) THEN
   RAISE EXCEPTION '自动分配的 IP 已被其他账号占用，请重试';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM proxy_binding_history h WHERE h.proxy_key=k AND h.identity_key=ident AND kitty_proxy_scope(h.platform,h.account_type,h.source,c)) THEN
   n:=kitty_proxy_history_count(k,c);limit_n:=COALESCE((c->>'history_limit')::int,5);
   IF n>=limit_n THEN
    SELECT available INTO allowance FROM proxy_binding_allowance WHERE proxy_key=k;
    IF automatic OR NOT COALESCE(allowance,false) THEN RAISE EXCEPTION '此 IP 的规则内历史绑定已达上限，需要管理员确认额外放行一个账号'; END IF;
    UPDATE proxy_binding_allowance SET available=false,consumed_at=NOW() WHERE proxy_key=k;
   END IF;
  END IF;
 END IF;
 INSERT INTO proxy_binding_history(proxy_key,identity_key,platform,account_type,source) VALUES(k,ident,NEW.platform,NEW.type,src)
 ON CONFLICT(proxy_key,identity_key,platform,account_type,source) DO UPDATE SET last_bound_at=NOW();
 RETURN NEW;
END $$;
CREATE TRIGGER kitty_proxy_binding BEFORE INSERT OR UPDATE OF proxy_id,platform,type ON accounts FOR EACH ROW EXECUTE FUNCTION kitty_check_proxy_binding();
