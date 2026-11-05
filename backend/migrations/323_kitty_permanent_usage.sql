-- Compact, permanent request metrics; no customer balance mutations or FK
-- cascades. Receipt updates (including video repair) remain reflected. Clearing
-- usage_logs must not clear these metrics. Cash refunds are a different ledger.
-- Give up quickly instead of queueing production writes behind a migration.
SET LOCAL lock_timeout = '250ms';
SET LOCAL statement_timeout = '15s';
CREATE TABLE kitty_permanent_usage (
 source_log_id BIGINT NOT NULL,
 api_key_id BIGINT NOT NULL,
 request_key TEXT NOT NULL,
 user_id BIGINT NOT NULL,
 input_tokens BIGINT NOT NULL,
 output_tokens BIGINT NOT NULL,
 cache_creation_tokens BIGINT NOT NULL,
 cache_read_tokens BIGINT NOT NULL,
 actual_cost NUMERIC,
 total_cost NUMERIC,
 invalid_metrics BOOLEAN NOT NULL DEFAULT FALSE,
 billing_type SMALLINT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(api_key_id,request_key)
);
CREATE INDEX kitty_permanent_usage_user ON kitty_permanent_usage(user_id);
CREATE TABLE kitty_permanent_usage_state (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton),
 started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 baseline_highwater BIGINT NOT NULL,
 baseline_cursor BIGINT NOT NULL DEFAULT 0,
 baseline_complete BOOLEAN NOT NULL DEFAULT FALSE,
 baseline_completed_at TIMESTAMPTZ
);

CREATE FUNCTION kitty_capture_permanent_usage(u usage_logs, overwrite_metrics BOOLEAN) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE k TEXT; invalid BOOLEAN; ac NUMERIC; tc NUMERIC;
BEGIN
 k:=CASE WHEN COALESCE(u.request_id,'')<>'' THEN 'request:'||u.request_id ELSE 'legacy-row:'||u.id::text END;
 ac:=CASE WHEN u.actual_cost>'-Infinity'::numeric AND u.actual_cost<'Infinity'::numeric THEN u.actual_cost END;
 tc:=CASE WHEN u.total_cost>'-Infinity'::numeric AND u.total_cost<'Infinity'::numeric THEN u.total_cost END;
 invalid:=(ac IS NULL OR tc IS NULL OR u.input_tokens<0 OR u.output_tokens<0 OR u.cache_creation_tokens<0 OR u.cache_read_tokens<0);
 INSERT INTO kitty_permanent_usage(source_log_id,api_key_id,request_key,user_id,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,actual_cost,total_cost,invalid_metrics,billing_type,observed_at)
 VALUES(u.id,u.api_key_id,k,u.user_id,GREATEST(u.input_tokens,0),GREATEST(u.output_tokens,0),GREATEST(u.cache_creation_tokens,0),GREATEST(u.cache_read_tokens,0),ac,tc,invalid,u.billing_type,u.created_at)
 ON CONFLICT DO NOTHING;
 -- A replay with a new log ID cannot overwrite a previously corrected receipt.
 -- Baseline backfill never replaces a live, newer trigger snapshot.
 IF overwrite_metrics THEN
  UPDATE kitty_permanent_usage SET input_tokens=GREATEST(u.input_tokens,0),output_tokens=GREATEST(u.output_tokens,0),
   cache_creation_tokens=GREATEST(u.cache_creation_tokens,0),cache_read_tokens=GREATEST(u.cache_read_tokens,0),
   actual_cost=ac,total_cost=tc,invalid_metrics=invalid,billing_type=u.billing_type,observed_at=u.created_at
  WHERE source_log_id=u.id AND api_key_id=u.api_key_id AND request_key=k AND user_id=u.user_id
  AND (input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,actual_cost,total_cost,invalid_metrics,billing_type,observed_at)
   IS DISTINCT FROM (GREATEST(u.input_tokens,0)::bigint,GREATEST(u.output_tokens,0)::bigint,GREATEST(u.cache_creation_tokens,0)::bigint,GREATEST(u.cache_read_tokens,0)::bigint,ac,tc,invalid,u.billing_type,u.created_at);
 END IF;
END;
$$;

CREATE FUNCTION kitty_permanent_usage_capture_trigger() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  -- Preserve rows deleted during the bounded historical backfill. This also
  -- covers FK cascades; no subtraction for retention or key/user deletion.
  PERFORM kitty_capture_permanent_usage(OLD,FALSE);
  RETURN OLD;
 END IF;
 PERFORM kitty_capture_permanent_usage(NEW,TRUE);
 RETURN NEW;
END;
$$;

-- No full-table backfill inside migration/startup. CREATE TRIGGER and the
-- high-water mark commit together, so new writes cannot fall through a gap.
CREATE TRIGGER kitty_permanent_usage_write AFTER INSERT OR UPDATE OF input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,actual_cost,total_cost,billing_type,created_at
 ON usage_logs FOR EACH ROW EXECUTE FUNCTION kitty_permanent_usage_capture_trigger();
CREATE TRIGGER kitty_permanent_usage_delete BEFORE DELETE ON usage_logs
 FOR EACH ROW EXECUTE FUNCTION kitty_permanent_usage_capture_trigger();
INSERT INTO kitty_permanent_usage_state(singleton,baseline_highwater) SELECT TRUE,COALESCE(max(id),0) FROM usage_logs;

CREATE FUNCTION kitty_backfill_permanent_usage(batch_size INTEGER) RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE s kitty_permanent_usage_state%ROWTYPE; u usage_logs%ROWTYPE; processed BIGINT:=0; last_id BIGINT;
BEGIN
 IF batch_size<1 OR batch_size>1000 THEN RAISE EXCEPTION 'invalid permanent usage batch size'; END IF;
 SELECT * INTO s FROM kitty_permanent_usage_state WHERE singleton FOR UPDATE;
 IF s.baseline_complete THEN RETURN 0; END IF;
 last_id:=s.baseline_cursor;
 FOR u IN SELECT * FROM usage_logs WHERE id>s.baseline_cursor AND id<=s.baseline_highwater ORDER BY id LIMIT batch_size LOOP
  PERFORM kitty_capture_permanent_usage(u,FALSE);
  last_id:=u.id;processed:=processed+1;
 END LOOP;
 UPDATE kitty_permanent_usage_state SET baseline_cursor=CASE WHEN processed<batch_size THEN s.baseline_highwater ELSE last_id END,
  baseline_complete=(processed<batch_size OR last_id>=s.baseline_highwater),
  baseline_completed_at=CASE WHEN processed<batch_size OR last_id>=s.baseline_highwater THEN NOW() END WHERE singleton;
 RETURN processed;
END;
$$;

COMMENT ON TABLE kitty_permanent_usage IS 'Permanent compact receipt metrics, independent of detail retention; amounts are recorded usage valuation, not cash income or verified net debits. History already removed before capture cannot be recovered.';
