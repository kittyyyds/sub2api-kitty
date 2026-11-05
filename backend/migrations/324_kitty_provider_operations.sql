-- Procurement records are independent of customer settlement and retained logs.
SET LOCAL lock_timeout = '250ms';
SET LOCAL statement_timeout = '15s';

CREATE TABLE kitty_suppliers (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 site_type TEXT NOT NULL CHECK(site_type IN ('sub2api','newapi','manual')),
 base_url TEXT NOT NULL DEFAULT '',
 notes TEXT NOT NULL DEFAULT '',
 active BOOLEAN NOT NULL DEFAULT TRUE,
 revision BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX kitty_suppliers_name ON kitty_suppliers(lower(name));
CREATE TABLE kitty_supplier_wallets (
 id BIGSERIAL PRIMARY KEY,
 supplier_id BIGINT NOT NULL REFERENCES kitty_suppliers(id),
 name TEXT NOT NULL,
 identity TEXT NOT NULL,
 currency TEXT NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 credit_currency TEXT NOT NULL CHECK(credit_currency ~ '^[A-Z]{3}$'),
 low_balance NUMERIC CHECK(low_balance>=0 AND low_balance<'Infinity'::numeric),
 sync_interval_minutes INTEGER NOT NULL DEFAULT 0 CHECK(sync_interval_minutes=0 OR sync_interval_minutes BETWEEN 5 AND 1440),
 auth_mode TEXT NOT NULL DEFAULT 'none' CHECK(auth_mode IN ('none','token','password')),
 username TEXT NOT NULL DEFAULT '',
 remote_user_id TEXT NOT NULL DEFAULT '',
 secret_cipher TEXT NOT NULL DEFAULT '',
 balance NUMERIC,
 balance_unit TEXT NOT NULL DEFAULT '',
 balance_source TEXT NOT NULL DEFAULT 'unknown',
 observed_at TIMESTAMPTZ,
 attempted_at TIMESTAMPTZ,
 sync_error TEXT NOT NULL DEFAULT '',
 active BOOLEAN NOT NULL DEFAULT TRUE,
 revision BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(supplier_id,identity)
);
CREATE TABLE kitty_supplier_bindings (
 id BIGSERIAL PRIMARY KEY,
 account_id BIGINT NOT NULL,
 wallet_id BIGINT NOT NULL REFERENCES kitty_supplier_wallets(id),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 ended_at TIMESTAMPTZ,
 CHECK(ended_at IS NULL OR ended_at>=started_at)
);
CREATE UNIQUE INDEX kitty_supplier_binding_current ON kitty_supplier_bindings(account_id) WHERE ended_at IS NULL;
CREATE INDEX kitty_supplier_binding_history ON kitty_supplier_bindings(account_id,started_at DESC);
CREATE INDEX kitty_supplier_binding_wallet ON kitty_supplier_bindings(wallet_id) WHERE ended_at IS NULL;
CREATE TABLE kitty_supplier_purchases (
 id BIGSERIAL PRIMARY KEY,
 wallet_id BIGINT NOT NULL REFERENCES kitty_supplier_wallets(id),
 operation_id TEXT NOT NULL UNIQUE,
 kind TEXT NOT NULL CHECK(kind IN ('topup','refund','opening')),
 cash NUMERIC NOT NULL CHECK(cash>=0 AND cash<'Infinity'::numeric),
 cash_currency TEXT NOT NULL CHECK(cash_currency ~ '^[A-Z]{3}$'),
 fx_rate NUMERIC NOT NULL CHECK(fx_rate>0 AND fx_rate<'Infinity'::numeric),
 credits NUMERIC NOT NULL CHECK(credits>=0 AND credits<'Infinity'::numeric),
 bonus NUMERIC NOT NULL DEFAULT 0 CHECK(bonus>=0 AND bonus<'Infinity'::numeric),
 reference TEXT NOT NULL DEFAULT '',
 note TEXT NOT NULL DEFAULT '',
 occurred_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 voided_at TIMESTAMPTZ,
 revision BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX kitty_supplier_purchase_wallet ON kitty_supplier_purchases(wallet_id,created_at);
CREATE TABLE kitty_supplier_credit_rates (
 id BIGSERIAL PRIMARY KEY,
 wallet_id BIGINT NOT NULL REFERENCES kitty_supplier_wallets(id),
 cash_basis NUMERIC NOT NULL,
 credit_basis NUMERIC NOT NULL,
 rate NUMERIC,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX kitty_supplier_credit_rate_history ON kitty_supplier_credit_rates(wallet_id,recorded_at DESC,id DESC);
CREATE FUNCTION kitty_supplier_credit_rate_trigger() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cash_total NUMERIC;credit_total NUMERIC;
BEGIN
 PERFORM id FROM kitty_supplier_wallets WHERE id=NEW.wallet_id FOR UPDATE;
 SELECT COALESCE(sum((CASE WHEN kind='refund' THEN -1 ELSE 1 END)*cash*fx_rate),0),
  COALESCE(sum((CASE WHEN kind='refund' THEN -1 ELSE 1 END)*(credits+bonus)),0)
 INTO cash_total,credit_total FROM kitty_supplier_purchases WHERE wallet_id=NEW.wallet_id AND voided_at IS NULL;
 INSERT INTO kitty_supplier_credit_rates(wallet_id,cash_basis,credit_basis,rate)
 VALUES(NEW.wallet_id,cash_total,credit_total,CASE WHEN credit_total>0 AND cash_total>=0 THEN cash_total/credit_total END);
 RETURN NEW;
END;
$$;
CREATE TRIGGER kitty_supplier_credit_rate_write AFTER INSERT OR UPDATE OF voided_at
 ON kitty_supplier_purchases FOR EACH ROW EXECUTE FUNCTION kitty_supplier_credit_rate_trigger();
CREATE TABLE kitty_supplier_cost_rules (
 id BIGSERIAL PRIMARY KEY,
 wallet_id BIGINT NOT NULL REFERENCES kitty_supplier_wallets(id),
 model TEXT NOT NULL,
 method TEXT NOT NULL CHECK(method IN ('standard_multiplier','account_credits','tokens','image','video_second','video_item','request')),
 multiplier NUMERIC NOT NULL DEFAULT 1 CHECK(multiplier>=0 AND multiplier<'Infinity'::numeric),
 input_price NUMERIC NOT NULL DEFAULT 0 CHECK(input_price>=0 AND input_price<'Infinity'::numeric),
 output_price NUMERIC NOT NULL DEFAULT 0 CHECK(output_price>=0 AND output_price<'Infinity'::numeric),
 cache_read_price NUMERIC NOT NULL DEFAULT 0 CHECK(cache_read_price>=0 AND cache_read_price<'Infinity'::numeric),
 cache_write_price NUMERIC NOT NULL DEFAULT 0 CHECK(cache_write_price>=0 AND cache_write_price<'Infinity'::numeric),
 unit_price NUMERIC NOT NULL DEFAULT 0 CHECK(unit_price>=0 AND unit_price<'Infinity'::numeric),
 currency TEXT NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 note TEXT NOT NULL DEFAULT '',
 fx_to_usd NUMERIC CHECK(fx_to_usd>0 AND fx_to_usd<'Infinity'::numeric),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 ended_at TIMESTAMPTZ,
 CHECK(ended_at IS NULL OR ended_at>=started_at)
);
CREATE UNIQUE INDEX kitty_supplier_rule_current ON kitty_supplier_cost_rules(wallet_id,model) WHERE ended_at IS NULL;
CREATE INDEX kitty_supplier_rule_history ON kitty_supplier_cost_rules(wallet_id,model,started_at DESC);

CREATE TABLE kitty_procurement_receipts (
 api_key_id BIGINT NOT NULL,
 request_key TEXT NOT NULL,
 source_log_id BIGINT NOT NULL,
 account_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 group_id BIGINT,
 model TEXT NOT NULL,
 upstream_model TEXT NOT NULL,
 upstream_request_id TEXT,
 supplier_id BIGINT,
 supplier_name TEXT,
 wallet_id BIGINT,
 wallet_name TEXT,
 rule_id BIGINT,
 rule_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
 credit_cost NUMERIC,
 estimated_cost NUMERIC,
 cost_currency TEXT,
 request_valuation NUMERIC,
 measurements JSONB NOT NULL,
 cost_status TEXT NOT NULL DEFAULT 'unknown',
 observed_at TIMESTAMPTZ NOT NULL,
 captured_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(api_key_id,request_key)
);
CREATE INDEX kitty_procurement_receipt_time ON kitty_procurement_receipts(observed_at);
CREATE INDEX kitty_procurement_receipt_supplier ON kitty_procurement_receipts(supplier_id,observed_at);
CREATE INDEX kitty_procurement_receipt_account ON kitty_procurement_receipts(account_id,observed_at);
CREATE TABLE kitty_procurement_adjustments (
 id BIGSERIAL PRIMARY KEY,
 api_key_id BIGINT NOT NULL,
 request_key TEXT NOT NULL,
 operation_id TEXT NOT NULL UNIQUE,
 amount NUMERIC NOT NULL CHECK(amount>=0 AND amount<'Infinity'::numeric),
 currency TEXT NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 reference TEXT NOT NULL CHECK(length(reference)>0),
 fx_to_usd NUMERIC CHECK(fx_to_usd>0 AND fx_to_usd<'Infinity'::numeric),
 note TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(api_key_id,request_key) REFERENCES kitty_procurement_receipts(api_key_id,request_key)
);
CREATE INDEX kitty_procurement_adjustment_receipt ON kitty_procurement_adjustments(api_key_id,request_key,id DESC);
CREATE TABLE kitty_provider_operations_state (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO kitty_provider_operations_state(singleton) VALUES(TRUE);
CREATE TABLE kitty_supplier_remote_operations (
 operation_id TEXT PRIMARY KEY,
 wallet_id BIGINT NOT NULL REFERENCES kitty_supplier_wallets(id),
 key_name TEXT NOT NULL,
 group_identity TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','succeeded','unconfirmed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 completed_at TIMESTAMPTZ
);

CREATE FUNCTION kitty_procurement_cost(rule JSONB, m JSONB, credit_rate NUMERIC) RETURNS NUMERIC LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE amount NUMERIC; method TEXT:=rule->>'method';
BEGIN
 IF (m->>'input')::numeric<0 OR (m->>'output')::numeric<0 OR (m->>'cache_read')::numeric<0 OR (m->>'cache_write')::numeric<0 THEN RETURN NULL; END IF;
 CASE method
  WHEN 'standard_multiplier' THEN
   IF (m->>'standard')::numeric<0 THEN RETURN NULL; END IF;
   amount:=(m->>'standard')::numeric*(rule->>'multiplier')::numeric;
  WHEN 'account_credits' THEN
   IF (m->>'account_credits')::numeric<0 THEN RETURN NULL; END IF;
   amount:=(m->>'account_credits')::numeric*credit_rate;
  WHEN 'tokens' THEN
   IF (m->>'images')::bigint>0 OR (m->>'videos')::bigint>0 THEN RETURN NULL; END IF;
   amount:=((m->>'input')::numeric*(rule->>'input_price')::numeric+
    (m->>'output')::numeric*(rule->>'output_price')::numeric+
    (m->>'cache_read')::numeric*(rule->>'cache_read_price')::numeric+
    (m->>'cache_write')::numeric*(rule->>'cache_write_price')::numeric)/1000000;
  WHEN 'image' THEN
   IF (m->>'images')::bigint<=0 THEN RETURN NULL; END IF;
   amount:=(m->>'images')::numeric*(rule->>'unit_price')::numeric;
  WHEN 'video_second' THEN
   IF (m->>'videos')::bigint<=0 OR COALESCE((m->>'seconds')::numeric,0)<=0 THEN RETURN NULL; END IF;
   amount:=(m->>'seconds')::numeric*(m->>'videos')::numeric*(rule->>'unit_price')::numeric;
  WHEN 'video_item' THEN
   IF (m->>'videos')::bigint<=0 THEN RETURN NULL; END IF;
   amount:=(m->>'videos')::numeric*(rule->>'unit_price')::numeric;
  WHEN 'request' THEN amount:=(rule->>'unit_price')::numeric;
  ELSE RETURN NULL;
 END CASE;
 IF amount>=0 AND amount<'Infinity'::numeric THEN RETURN round(amount,12); END IF;
 RETURN NULL;
END;
$$;

CREATE FUNCTION kitty_capture_procurement(u usage_logs) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE k TEXT; existing kitty_procurement_receipts%ROWTYPE; r kitty_supplier_cost_rules%ROWTYPE;
 w kitty_supplier_wallets%ROWTYPE; s kitty_suppliers%ROWTYPE;
 binding_id BIGINT; credit_rate_id BIGINT; rule_json JSONB:='{}'::jsonb; m JSONB; cr NUMERIC; ec NUMERIC; val NUMERIC;
BEGIN
 k:=CASE WHEN COALESCE(u.request_id,'')<>'' THEN 'request:'||u.request_id ELSE 'legacy-row:'||u.id::text END;
 m:=jsonb_build_object('input',u.input_tokens,'output',u.output_tokens,'cache_read',u.cache_read_tokens,'cache_write',u.cache_creation_tokens,
  'images',u.image_count,'videos',u.video_count,'seconds',u.video_duration_seconds,'standard',u.total_cost,
  'account_credits',COALESCE(u.account_stats_cost,u.total_cost)*COALESCE(u.account_rate_multiplier,1));
 val:=CASE WHEN u.actual_cost>=0 AND u.actual_cost<'Infinity'::numeric THEN u.actual_cost END;
 SELECT * INTO existing FROM kitty_procurement_receipts WHERE api_key_id=u.api_key_id AND request_key=k;
 IF FOUND THEN
  IF existing.source_log_id<>u.id OR existing.account_id<>u.account_id OR existing.user_id<>u.user_id THEN RETURN; END IF;
  -- Receipt corrections reuse the frozen procurement rule, never today's config.
  ec:=kitty_procurement_cost(existing.rule_snapshot,m,existing.credit_cost);
  UPDATE kitty_procurement_receipts SET measurements=m,request_valuation=val,estimated_cost=ec,
   cost_status=CASE WHEN ec IS NULL THEN 'unknown' ELSE 'estimated' END,
   upstream_request_id=COALESCE(u.upstream_request_id,upstream_request_id)
  WHERE api_key_id=u.api_key_id AND request_key=k;
  RETURN;
 END IF;
 SELECT wallet_id INTO binding_id FROM kitty_supplier_bindings WHERE account_id=u.account_id
  AND started_at<=u.created_at AND (ended_at IS NULL OR ended_at>u.created_at) ORDER BY started_at DESC,id DESC LIMIT 1;
 IF FOUND THEN
  SELECT * INTO w FROM kitty_supplier_wallets WHERE id=binding_id;
  SELECT * INTO s FROM kitty_suppliers WHERE id=w.supplier_id;
  SELECT * INTO r FROM kitty_supplier_cost_rules WHERE wallet_id=w.id AND model IN ('*',COALESCE(NULLIF(u.upstream_model,''),NULLIF(u.requested_model,''),u.model))
   AND started_at<=u.created_at AND (ended_at IS NULL OR ended_at>u.created_at)
   ORDER BY (model<>'*') DESC,started_at DESC,id DESC LIMIT 1;
  IF FOUND THEN
   rule_json:=to_jsonb(r)-'ended_at'-'note';
   IF r.method='account_credits' THEN
    -- Purchase changes materialize their own immutable reference version. The
    -- request path performs one indexed lookup, never an all-purchases scan.
    SELECT id,rate INTO credit_rate_id,cr FROM kitty_supplier_credit_rates
     WHERE wallet_id=w.id AND recorded_at<=u.created_at ORDER BY recorded_at DESC,id DESC LIMIT 1;
    rule_json:=rule_json||jsonb_build_object('credit_rate_version',credit_rate_id);
   END IF;
   ec:=kitty_procurement_cost(rule_json,m,cr);
  END IF;
 END IF;
 INSERT INTO kitty_procurement_receipts(api_key_id,request_key,source_log_id,account_id,user_id,group_id,model,upstream_model,upstream_request_id,
  supplier_id,supplier_name,wallet_id,wallet_name,rule_id,rule_snapshot,credit_cost,estimated_cost,cost_currency,request_valuation,measurements,cost_status,observed_at)
 VALUES(u.api_key_id,k,u.id,u.account_id,u.user_id,u.group_id,COALESCE(NULLIF(u.requested_model,''),u.model),COALESCE(NULLIF(u.upstream_model,''),NULLIF(u.requested_model,''),u.model),u.upstream_request_id,
  s.id,s.name,w.id,w.name,r.id,rule_json,cr,ec,r.currency,val,m,CASE WHEN ec IS NULL THEN 'unknown' ELSE 'estimated' END,u.created_at)
 ON CONFLICT DO NOTHING;
END;
$$;
CREATE FUNCTION kitty_procurement_capture_trigger() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM kitty_capture_procurement(NEW);
 RETURN NEW;
END;
$$;
CREATE TRIGGER kitty_procurement_write AFTER INSERT OR UPDATE OF input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,total_cost,actual_cost,account_stats_cost,account_rate_multiplier,image_count,video_count,video_duration_seconds,upstream_request_id
 ON usage_logs FOR EACH ROW EXECUTE FUNCTION kitty_procurement_capture_trigger();

COMMENT ON TABLE kitty_procurement_receipts IS 'Immutable procurement identity and cost-rule snapshots; quantity corrections reuse frozen rules. Estimated costs are not supplier invoice evidence. No customer balance mutations. Existing historical usage is not retroactively assigned.';
