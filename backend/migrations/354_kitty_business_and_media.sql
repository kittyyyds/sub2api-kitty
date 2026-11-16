-- Kitty UI/business features on the Custom lineage. No source rows or settings are copied.

-- Additive only: preserve every previously applied migration and target-specific account/Bridge state.

SET LOCAL lock_timeout = '2s';

SET LOCAL statement_timeout = '90s';

SET LOCAL search_path = public;

ALTER TABLE public.account_groups ADD COLUMN IF NOT EXISTS allowed_models jsonb;

ALTER TABLE public.accounts ADD COLUMN IF NOT EXISTS group_rate_multiplier numeric(10,4) DEFAULT 1.0 NOT NULL;

ALTER TABLE public.api_keys ADD COLUMN IF NOT EXISTS key_hash character varying(64);

ALTER TABLE public.api_keys ADD COLUMN IF NOT EXISTS key_prefix character varying(16);

ALTER TABLE public.api_keys ADD COLUMN IF NOT EXISTS concurrency_limit bigint DEFAULT 0 NOT NULL;

ALTER TABLE public.channel_monitor_v2_config ADD COLUMN IF NOT EXISTS candy_probes jsonb DEFAULT '[]'::jsonb NOT NULL;

ALTER TABLE public.channel_monitor_v2_config ADD COLUMN IF NOT EXISTS candy_history_limit integer DEFAULT 100 NOT NULL;

ALTER TABLE public.channel_monitor_v2_config ADD COLUMN IF NOT EXISTS card_timeline_mode text DEFAULT 'windows'::text NOT NULL;

ALTER TABLE public.channel_monitor_v2_config ADD COLUMN IF NOT EXISTS group_order bigint[] DEFAULT '{}'::bigint[] NOT NULL;

ALTER TABLE public.channel_monitors ADD COLUMN IF NOT EXISTS request_path character varying(500) DEFAULT ''::character varying NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS grok_prompt_enabled boolean DEFAULT false NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS grok_prompt_mode character varying(16) DEFAULT 'prepend'::character varying NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS grok_prompt_content text DEFAULT ''::text NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS image_model_prices jsonb;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS security_policy_enabled boolean DEFAULT false NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS security_policy_mode character varying(20) DEFAULT 'block_session'::character varying NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS security_policy_email_enabled boolean DEFAULT true NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS stream_only boolean DEFAULT false NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS codex_cli_only boolean DEFAULT false NOT NULL;

ALTER TABLE public.groups ADD COLUMN IF NOT EXISTS block_subagents boolean DEFAULT false NOT NULL;

ALTER TABLE public.payment_orders ADD COLUMN IF NOT EXISTS subscription_renewal_mode character varying(16) DEFAULT 'extend'::character varying NOT NULL;

ALTER TABLE public.usage_logs ADD COLUMN IF NOT EXISTS usage_audit jsonb;

ALTER TABLE public.usage_logs ADD COLUMN IF NOT EXISTS queue_wait_ms integer;

ALTER TABLE public.usage_logs ADD COLUMN IF NOT EXISTS queue_timing jsonb;

ALTER TABLE public.user_group_rate_multipliers ADD COLUMN IF NOT EXISTS denied_models jsonb;

CREATE TABLE IF NOT EXISTS public.api_key_groups (
    api_key_id bigint NOT NULL,
    group_id bigint NOT NULL
);

CREATE TABLE IF NOT EXISTS public.application_run_steps (
    id bigint NOT NULL,
    run_id uuid NOT NULL,
    step_key character varying(64) NOT NULL,
    sequence integer NOT NULL,
    executor character varying(32) NOT NULL,
    credential_role character varying(16) NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    request jsonb DEFAULT '{}'::jsonb NOT NULL,
    output jsonb,
    gateway_request_id character varying(256),
    error_code character varying(64),
    error_message character varying(2000),
    attempt integer DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT application_run_steps_attempt_positive CHECK ((attempt > 0)),
    CONSTRAINT application_run_steps_credential_role_valid CHECK (((credential_role)::text = ANY (ARRAY[('planner'::character varying)::text, ('media'::character varying)::text]))),
    CONSTRAINT application_run_steps_executor_valid CHECK (((executor)::text = ANY (ARRAY[('chat_completions'::character varying)::text, ('image_generation'::character varying)::text, ('image_edit'::character varying)::text, ('video_generation'::character varying)::text, ('video_edit'::character varying)::text, ('video_extension'::character varying)::text]))),
    CONSTRAINT application_run_steps_sequence_positive CHECK ((sequence > 0)),
    CONSTRAINT application_run_steps_status_valid CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text, ('cancelled'::character varying)::text]))),
    CONSTRAINT application_run_steps_version_positive CHECK ((version > 0))
);

CREATE SEQUENCE IF NOT EXISTS public.application_run_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.application_run_steps_id_seq OWNED BY public.application_run_steps.id;

CREATE TABLE IF NOT EXISTS public.application_runs (
    id uuid NOT NULL,
    user_id bigint NOT NULL,
    app_slug character varying(64) NOT NULL,
    app_revision integer NOT NULL,
    status character varying(16) DEFAULT 'running'::character varying NOT NULL,
    planner_api_key_id bigint,
    media_api_key_id bigint NOT NULL,
    session_id character varying(80) NOT NULL,
    current_step_key character varying(64),
    input jsonb DEFAULT '{}'::jsonb NOT NULL,
    output jsonb,
    error_code character varying(64),
    error_message character varying(2000),
    retry_count integer DEFAULT 0 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    CONSTRAINT application_runs_retry_count_nonnegative CHECK ((retry_count >= 0)),
    CONSTRAINT application_runs_revision_positive CHECK ((app_revision > 0)),
    CONSTRAINT application_runs_status_valid CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text, ('cancelled'::character varying)::text]))),
    CONSTRAINT application_runs_version_positive CHECK ((version > 0))
);

CREATE TABLE IF NOT EXISTS public.batch_image_money_holds (
    batch_id character varying(64) NOT NULL,
    api_key_id bigint NOT NULL,
    user_id bigint NOT NULL,
    hold_amount numeric(20,8) NOT NULL,
    actual_amount numeric(20,8),
    state text NOT NULL,
    CONSTRAINT batch_image_money_holds_hold_amount_check CHECK ((hold_amount >= (0)::numeric)),
    CONSTRAINT batch_image_money_holds_state_check CHECK ((state = ANY (ARRAY['held'::text, 'captured'::text, 'released'::text])))
);

CREATE TABLE IF NOT EXISTS public.channel_monitor_v2_candy_results (
    id bigint NOT NULL,
    group_id bigint NOT NULL,
    config_key character varying(64) NOT NULL,
    model character varying(100) NOT NULL,
    reasoning_effort character varying(16) NOT NULL,
    slot timestamp with time zone NOT NULL,
    checked_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    verdict character varying(16) DEFAULT 'running'::character varying NOT NULL,
    latency_ms bigint DEFAULT 0 NOT NULL,
    answer_preview character varying(512) DEFAULT ''::character varying NOT NULL,
    reason character varying(32) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT channel_monitor_v2_candy_results_verdict_check CHECK (((verdict)::text = ANY (ARRAY[('running'::character varying)::text, ('correct'::character varying)::text, ('incorrect'::character varying)::text, ('error'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.channel_monitor_v2_candy_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.channel_monitor_v2_candy_results_id_seq OWNED BY public.channel_monitor_v2_candy_results.id;

CREATE TABLE IF NOT EXISTS public.channel_monitor_v3_categories (
    id bigint NOT NULL,
    name character varying(64) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.channel_monitor_v3_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.channel_monitor_v3_categories_id_seq OWNED BY public.channel_monitor_v3_categories.id;

CREATE TABLE IF NOT EXISTS public.channel_monitor_v3_components (
    id bigint NOT NULL,
    category_id bigint,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    group_id bigint NOT NULL,
    model character varying(100) DEFAULT ''::character varying NOT NULL,
    degraded_ttft_ms integer DEFAULT 0 NOT NULL,
    show_multiplier boolean DEFAULT true NOT NULL,
    visibility character varying(16) DEFAULT 'group'::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT channel_monitor_v3_components_degraded_ttft_ms_check CHECK (((degraded_ttft_ms = 0) OR ((degraded_ttft_ms >= 1000) AND (degraded_ttft_ms <= 120000)))),
    CONSTRAINT channel_monitor_v3_components_visibility_check CHECK (((visibility)::text = ANY (ARRAY[('group'::character varying)::text, ('public'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.channel_monitor_v3_components_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.channel_monitor_v3_components_id_seq OWNED BY public.channel_monitor_v3_components.id;

CREATE TABLE IF NOT EXISTS public.channel_monitor_v3_config (
    id smallint DEFAULT 1 NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    interval_minutes integer DEFAULT 5 NOT NULL,
    cells integer DEFAULT 90 NOT NULL,
    availability_range character varying(8) DEFAULT '7d'::character varying NOT NULL,
    down_error_rate numeric(5,4) DEFAULT 0.2 NOT NULL,
    degraded_error_rate numeric(5,4) DEFAULT 0.05 NOT NULL,
    degraded_ttft_ms integer DEFAULT 10000 NOT NULL,
    min_requests integer DEFAULT 1 NOT NULL,
    ignored_error_categories text[] DEFAULT ARRAY['authentication'::text, 'client_cancelled'::text, 'content_policy'::text, 'context_limit'::text, 'group_access'::text, 'model_unsupported'::text, 'not_found'::text, 'quota_or_balance'::text] NOT NULL,
    featured_component_id bigint,
    footer_note character varying(500) DEFAULT ''::character varying NOT NULL,
    updated_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    idle_probe jsonb DEFAULT '{"groups": [], "enabled": false}'::jsonb NOT NULL,
    CONSTRAINT channel_monitor_v3_config_availability_range_check CHECK (((availability_range)::text = ANY (ARRAY[('24h'::character varying)::text, ('7d'::character varying)::text, ('30d'::character varying)::text]))),
    CONSTRAINT channel_monitor_v3_config_cells_check CHECK (((cells >= 30) AND (cells <= 120))),
    CONSTRAINT channel_monitor_v3_config_degraded_error_rate_check CHECK (((degraded_error_rate > (0)::numeric) AND (degraded_error_rate <= (1)::numeric))),
    CONSTRAINT channel_monitor_v3_config_degraded_ttft_ms_check CHECK (((degraded_ttft_ms >= 1000) AND (degraded_ttft_ms <= 120000))),
    CONSTRAINT channel_monitor_v3_config_down_error_rate_check CHECK (((down_error_rate > (0)::numeric) AND (down_error_rate <= (1)::numeric))),
    CONSTRAINT channel_monitor_v3_config_id_check CHECK ((id = 1)),
    CONSTRAINT channel_monitor_v3_config_interval_minutes_check CHECK ((interval_minutes = ANY (ARRAY[1, 2, 3, 5, 10, 15, 30, 60]))),
    CONSTRAINT channel_monitor_v3_config_min_requests_check CHECK (((min_requests >= 1) AND (min_requests <= 1000)))
);

CREATE TABLE IF NOT EXISTS public.channel_monitor_v3_probe_results (
    id bigint NOT NULL,
    group_id bigint NOT NULL,
    model character varying(100) NOT NULL,
    slot_start timestamp with time zone NOT NULL,
    slot_minutes integer NOT NULL,
    outcome character varying(16) DEFAULT 'running'::character varying NOT NULL,
    latency_ms integer,
    error_category character varying(32) DEFAULT ''::character varying NOT NULL,
    error_message character varying(500) DEFAULT ''::character varying NOT NULL,
    account_id bigint,
    cost_usd numeric(20,10),
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT channel_monitor_v3_probe_results_outcome_check CHECK (((outcome)::text = ANY (ARRAY[('running'::character varying)::text, ('success'::character varying)::text, ('failure'::character varying)::text]))),
    CONSTRAINT channel_monitor_v3_probe_results_slot_minutes_check CHECK (((slot_minutes >= 1) AND (slot_minutes <= 60)))
);

CREATE SEQUENCE IF NOT EXISTS public.channel_monitor_v3_probe_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.channel_monitor_v3_probe_results_id_seq OWNED BY public.channel_monitor_v3_probe_results.id;

CREATE TABLE IF NOT EXISTS public.global_context_pricing_rules (
    id bigint NOT NULL,
    model text NOT NULL,
    platform text NOT NULL,
    threshold_tokens bigint NOT NULL,
    input_multiplier double precision NOT NULL,
    output_multiplier double precision NOT NULL,
    cache_read_multiplier double precision NOT NULL,
    cache_write_multiplier double precision NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT global_context_pricing_rules_cache_read_multiplier_check CHECK (((cache_read_multiplier >= (0.01)::double precision) AND (cache_read_multiplier <= (100)::double precision))),
    CONSTRAINT global_context_pricing_rules_cache_write_multiplier_check CHECK (((cache_write_multiplier >= (0.01)::double precision) AND (cache_write_multiplier <= (100)::double precision))),
    CONSTRAINT global_context_pricing_rules_input_multiplier_check CHECK (((input_multiplier >= (0.01)::double precision) AND (input_multiplier <= (100)::double precision))),
    CONSTRAINT global_context_pricing_rules_output_multiplier_check CHECK (((output_multiplier >= (0.01)::double precision) AND (output_multiplier <= (100)::double precision))),
    CONSTRAINT global_context_pricing_rules_threshold_tokens_check CHECK (((threshold_tokens > 0) AND (threshold_tokens <= 100000000)))
);

CREATE SEQUENCE IF NOT EXISTS public.global_context_pricing_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.global_context_pricing_rules_id_seq OWNED BY public.global_context_pricing_rules.id;

CREATE TABLE IF NOT EXISTS public.image_supplier_tasks (
    id text NOT NULL,
    user_id bigint NOT NULL,
    api_key_id bigint NOT NULL,
    account_id bigint NOT NULL,
    idempotency_hash text NOT NULL,
    request_hash text NOT NULL,
    snapshot jsonb NOT NULL,
    upstream_task_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'submitting'::text NOT NULL,
    result bytea,
    next_poll_at timestamp with time zone DEFAULT (now() + '00:06:00'::interval) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS public.kitty_media_identities (
    user_id bigint NOT NULL,
    subject text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT kitty_media_identities_subject_check CHECK ((subject ~ '^[A-Za-z0-9_-]{43}$'::text))
);

CREATE TABLE IF NOT EXISTS public.kitty_media_storage_orders (
    order_id character varying(128) NOT NULL,
    user_id bigint NOT NULL,
    asset_id uuid NOT NULL,
    days integer NOT NULL,
    amount numeric(20,8) NOT NULL,
    before_expiry timestamp with time zone NOT NULL,
    after_expiry timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT kitty_media_storage_orders_amount_check CHECK ((amount > (0)::numeric)),
    CONSTRAINT kitty_media_storage_orders_days_check CHECK (((days >= 1) AND (days <= 365)))
);

CREATE TABLE IF NOT EXISTS public.kitty_media_wallet_ledger (
    order_id character varying(128) NOT NULL,
    entry_type text NOT NULL,
    held_amount numeric(20,8) DEFAULT 0 NOT NULL,
    actual_amount numeric(20,8) NOT NULL,
    released_amount numeric(20,8) NOT NULL,
    expired_amount numeric(20,8) NOT NULL,
    evidence_hash character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT kitty_media_wallet_ledger_actual_amount_check CHECK ((actual_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_ledger_entry_type_check CHECK ((entry_type = ANY (ARRAY['hold'::text, 'capture'::text, 'release'::text]))),
    CONSTRAINT kitty_media_wallet_ledger_expired_amount_check CHECK ((expired_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_ledger_held_amount_check CHECK ((held_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_ledger_released_amount_check CHECK ((released_amount >= (0)::numeric))
);

CREATE TABLE IF NOT EXISTS public.kitty_media_wallet_orders (
    order_id character varying(128) NOT NULL,
    user_id bigint NOT NULL,
    fingerprint character varying(64) NOT NULL,
    currency character varying(3) DEFAULT 'USD'::character varying NOT NULL,
    hold_amount numeric(20,8) NOT NULL,
    actual_amount numeric(20,8) DEFAULT 0 NOT NULL,
    released_amount numeric(20,8) DEFAULT 0 NOT NULL,
    expired_amount numeric(20,8) DEFAULT 0 NOT NULL,
    billing_state text DEFAULT 'held'::text NOT NULL,
    submission_state text DEFAULT 'reserved'::text NOT NULL,
    upstream_id text DEFAULT ''::text NOT NULL,
    evidence_hash character varying(64) DEFAULT ''::character varying NOT NULL,
    settlement_hash character varying(64) DEFAULT ''::character varying NOT NULL,
    quote jsonb NOT NULL,
    price_contract jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT kitty_media_wallet_orders_actual_amount_check CHECK ((actual_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_orders_billing_state_check CHECK ((billing_state = ANY (ARRAY['held'::text, 'captured'::text, 'released'::text]))),
    CONSTRAINT kitty_media_wallet_orders_check CHECK ((((billing_state = 'held'::text) AND (actual_amount = (0)::numeric) AND (released_amount = (0)::numeric) AND (expired_amount = (0)::numeric)) OR ((billing_state <> 'held'::text) AND (((actual_amount + released_amount) + expired_amount) = hold_amount)))),
    CONSTRAINT kitty_media_wallet_orders_check1 CHECK (((billing_state <> 'released'::text) OR (actual_amount = (0)::numeric))),
    CONSTRAINT kitty_media_wallet_orders_currency_check CHECK (((currency)::text = 'USD'::text)),
    CONSTRAINT kitty_media_wallet_orders_expired_amount_check CHECK ((expired_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_orders_hold_amount_check CHECK (((hold_amount >= (0)::numeric) AND (hold_amount < (1000000)::numeric))),
    CONSTRAINT kitty_media_wallet_orders_released_amount_check CHECK ((released_amount >= (0)::numeric)),
    CONSTRAINT kitty_media_wallet_orders_submission_state_check CHECK ((submission_state = ANY (ARRAY['reserved'::text, 'submitting'::text, 'unknown'::text, 'accepted'::text, 'rejected'::text])))
);

CREATE TABLE IF NOT EXISTS public.kitty_permanent_usage (
    source_log_id bigint NOT NULL,
    api_key_id bigint NOT NULL,
    request_key text NOT NULL,
    user_id bigint NOT NULL,
    input_tokens bigint NOT NULL,
    output_tokens bigint NOT NULL,
    cache_creation_tokens bigint NOT NULL,
    cache_read_tokens bigint NOT NULL,
    actual_cost numeric,
    total_cost numeric,
    invalid_metrics boolean DEFAULT false NOT NULL,
    billing_type smallint NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    captured_at timestamp with time zone DEFAULT now() NOT NULL,
    cache_creation_30m_tokens bigint DEFAULT 0 NOT NULL,
    cache_creation_12h_tokens bigint DEFAULT 0 NOT NULL
);

CREATE TABLE IF NOT EXISTS public.kitty_permanent_usage_state (
    singleton boolean DEFAULT true NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    baseline_highwater bigint NOT NULL,
    baseline_cursor bigint DEFAULT 0 NOT NULL,
    baseline_complete boolean DEFAULT false NOT NULL,
    baseline_completed_at timestamp with time zone,
    CONSTRAINT kitty_permanent_usage_state_singleton_check CHECK (singleton)
);

CREATE TABLE IF NOT EXISTS public.kitty_procurement_adjustments (
    id bigint NOT NULL,
    api_key_id bigint NOT NULL,
    request_key text NOT NULL,
    operation_id text NOT NULL,
    amount numeric NOT NULL,
    currency text NOT NULL,
    reference text NOT NULL,
    fx_to_usd numeric,
    note text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kitty_procurement_adjustments_amount_check CHECK (((amount >= (0)::numeric) AND (amount < 'Infinity'::numeric))),
    CONSTRAINT kitty_procurement_adjustments_currency_check CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT kitty_procurement_adjustments_fx_to_usd_check CHECK (((fx_to_usd > (0)::numeric) AND (fx_to_usd < 'Infinity'::numeric))),
    CONSTRAINT kitty_procurement_adjustments_reference_check CHECK ((length(reference) > 0))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_procurement_adjustments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_procurement_adjustments_id_seq OWNED BY public.kitty_procurement_adjustments.id;

CREATE TABLE IF NOT EXISTS public.kitty_procurement_receipts (
    api_key_id bigint NOT NULL,
    request_key text NOT NULL,
    source_log_id bigint NOT NULL,
    account_id bigint NOT NULL,
    user_id bigint NOT NULL,
    group_id bigint,
    model text NOT NULL,
    upstream_model text NOT NULL,
    upstream_request_id text,
    supplier_id bigint,
    supplier_name text,
    wallet_id bigint,
    wallet_name text,
    rule_id bigint,
    rule_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    credit_cost numeric,
    estimated_cost numeric,
    cost_currency text,
    request_valuation numeric,
    measurements jsonb NOT NULL,
    cost_status text DEFAULT 'unknown'::text NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    captured_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE IF NOT EXISTS public.kitty_provider_operations_state (
    singleton boolean DEFAULT true NOT NULL,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kitty_provider_operations_state_singleton_check CHECK (singleton)
);

CREATE TABLE IF NOT EXISTS public.kitty_retained_media (
    id uuid NOT NULL,
    user_id bigint NOT NULL,
    source_key character varying(64) NOT NULL,
    sha256 character varying(64) NOT NULL,
    kind text NOT NULL,
    name character varying(200) NOT NULL,
    mime_type character varying(80) NOT NULL,
    bytes bigint NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    expires_at timestamp with time zone DEFAULT (statement_timestamp() + '24:00:00'::interval) NOT NULL,
    updated_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT kitty_retained_media_bytes_check CHECK (((bytes > 0) AND (bytes <= 100663296))),
    CONSTRAINT kitty_retained_media_kind_check CHECK ((kind = ANY (ARRAY['image'::text, 'video'::text]))),
    CONSTRAINT kitty_retained_media_status_check CHECK ((status = ANY (ARRAY['active'::text, 'deleting'::text, 'deleted'::text])))
);

CREATE TABLE IF NOT EXISTS public.kitty_supplier_bindings (
    id bigint NOT NULL,
    account_id bigint NOT NULL,
    wallet_id bigint NOT NULL,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    ended_at timestamp with time zone,
    CONSTRAINT kitty_supplier_bindings_check CHECK (((ended_at IS NULL) OR (ended_at >= started_at)))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_supplier_bindings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_supplier_bindings_id_seq OWNED BY public.kitty_supplier_bindings.id;

CREATE TABLE IF NOT EXISTS public.kitty_supplier_cost_rules (
    id bigint NOT NULL,
    wallet_id bigint NOT NULL,
    model text NOT NULL,
    method text NOT NULL,
    multiplier numeric DEFAULT 1 NOT NULL,
    input_price numeric DEFAULT 0 NOT NULL,
    output_price numeric DEFAULT 0 NOT NULL,
    cache_read_price numeric DEFAULT 0 NOT NULL,
    cache_write_price numeric DEFAULT 0 NOT NULL,
    unit_price numeric DEFAULT 0 NOT NULL,
    currency text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    fx_to_usd numeric,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    ended_at timestamp with time zone,
    cache_write_12h_price numeric,
    CONSTRAINT kitty_supplier_cost_rules_cache_read_price_check CHECK (((cache_read_price >= (0)::numeric) AND (cache_read_price < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_cache_write_12h_price_check CHECK (((cache_write_12h_price >= (0)::numeric) AND (cache_write_12h_price < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_cache_write_price_check CHECK (((cache_write_price >= (0)::numeric) AND (cache_write_price < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_check CHECK (((ended_at IS NULL) OR (ended_at >= started_at))),
    CONSTRAINT kitty_supplier_cost_rules_currency_check CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT kitty_supplier_cost_rules_fx_to_usd_check CHECK (((fx_to_usd > (0)::numeric) AND (fx_to_usd < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_input_price_check CHECK (((input_price >= (0)::numeric) AND (input_price < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_method_check CHECK ((method = ANY (ARRAY['standard_multiplier'::text, 'account_credits'::text, 'tokens'::text, 'image'::text, 'video_second'::text, 'video_item'::text, 'request'::text]))),
    CONSTRAINT kitty_supplier_cost_rules_multiplier_check CHECK (((multiplier >= (0)::numeric) AND (multiplier < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_output_price_check CHECK (((output_price >= (0)::numeric) AND (output_price < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_cost_rules_unit_price_check CHECK (((unit_price >= (0)::numeric) AND (unit_price < 'Infinity'::numeric)))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_supplier_cost_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_supplier_cost_rules_id_seq OWNED BY public.kitty_supplier_cost_rules.id;

CREATE TABLE IF NOT EXISTS public.kitty_supplier_credit_rates (
    id bigint NOT NULL,
    wallet_id bigint NOT NULL,
    cash_basis numeric NOT NULL,
    credit_basis numeric NOT NULL,
    rate numeric,
    recorded_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_supplier_credit_rates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_supplier_credit_rates_id_seq OWNED BY public.kitty_supplier_credit_rates.id;

CREATE TABLE IF NOT EXISTS public.kitty_supplier_purchases (
    id bigint NOT NULL,
    wallet_id bigint NOT NULL,
    operation_id text NOT NULL,
    kind text NOT NULL,
    cash numeric NOT NULL,
    cash_currency text NOT NULL,
    fx_rate numeric NOT NULL,
    credits numeric NOT NULL,
    bonus numeric DEFAULT 0 NOT NULL,
    reference text DEFAULT ''::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    voided_at timestamp with time zone,
    revision bigint DEFAULT 1 NOT NULL,
    CONSTRAINT kitty_supplier_purchases_bonus_check CHECK (((bonus >= (0)::numeric) AND (bonus < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_purchases_cash_check CHECK (((cash >= (0)::numeric) AND (cash < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_purchases_cash_currency_check CHECK ((cash_currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT kitty_supplier_purchases_credits_check CHECK (((credits >= (0)::numeric) AND (credits < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_purchases_fx_rate_check CHECK (((fx_rate > (0)::numeric) AND (fx_rate < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_purchases_kind_check CHECK ((kind = ANY (ARRAY['topup'::text, 'refund'::text, 'opening'::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_supplier_purchases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_supplier_purchases_id_seq OWNED BY public.kitty_supplier_purchases.id;

CREATE TABLE IF NOT EXISTS public.kitty_supplier_remote_operations (
    operation_id text NOT NULL,
    wallet_id bigint NOT NULL,
    key_name text NOT NULL,
    group_identity text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT kitty_supplier_remote_operations_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'succeeded'::text, 'unconfirmed'::text])))
);

CREATE TABLE IF NOT EXISTS public.kitty_supplier_wallets (
    id bigint NOT NULL,
    supplier_id bigint NOT NULL,
    name text NOT NULL,
    identity text NOT NULL,
    currency text NOT NULL,
    credit_currency text NOT NULL,
    low_balance numeric,
    sync_interval_minutes integer DEFAULT 0 NOT NULL,
    auth_mode text DEFAULT 'none'::text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    remote_user_id text DEFAULT ''::text NOT NULL,
    secret_cipher text DEFAULT ''::text NOT NULL,
    balance numeric,
    balance_unit text DEFAULT ''::text NOT NULL,
    balance_source text DEFAULT 'unknown'::text NOT NULL,
    observed_at timestamp with time zone,
    attempted_at timestamp with time zone,
    sync_error text DEFAULT ''::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kitty_supplier_wallets_auth_mode_check CHECK ((auth_mode = ANY (ARRAY['none'::text, 'token'::text, 'password'::text]))),
    CONSTRAINT kitty_supplier_wallets_credit_currency_check CHECK ((credit_currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT kitty_supplier_wallets_currency_check CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT kitty_supplier_wallets_low_balance_check CHECK (((low_balance >= (0)::numeric) AND (low_balance < 'Infinity'::numeric))),
    CONSTRAINT kitty_supplier_wallets_sync_interval_minutes_check CHECK (((sync_interval_minutes = 0) OR ((sync_interval_minutes >= 5) AND (sync_interval_minutes <= 1440))))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_supplier_wallets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_supplier_wallets_id_seq OWNED BY public.kitty_supplier_wallets.id;

CREATE TABLE IF NOT EXISTS public.kitty_suppliers (
    id bigint NOT NULL,
    name text NOT NULL,
    site_type text NOT NULL,
    base_url text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT kitty_suppliers_name_check CHECK (((length(name) >= 1) AND (length(name) <= 120))),
    CONSTRAINT kitty_suppliers_site_type_check CHECK ((site_type = ANY (ARRAY['sub2api'::text, 'newapi'::text, 'manual'::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_suppliers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_suppliers_id_seq OWNED BY public.kitty_suppliers.id;

CREATE TABLE IF NOT EXISTS public.kitty_video_price_changes (
    id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    group_id bigint NOT NULL,
    model text NOT NULL,
    old_price jsonb NOT NULL,
    new_price jsonb NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.kitty_video_price_changes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.kitty_video_price_changes_id_seq OWNED BY public.kitty_video_price_changes.id;

CREATE TABLE IF NOT EXISTS public.new_api_account_bindings (
    account_id bigint NOT NULL,
    authorization_id bigint NOT NULL,
    upstream_token_id bigint NOT NULL,
    account_fingerprint text NOT NULL,
    token_group text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT new_api_account_bindings_upstream_token_id_check CHECK ((upstream_token_id > 0))
);

CREATE TABLE IF NOT EXISTS public.new_api_site_authorizations (
    id bigint NOT NULL,
    site_url text NOT NULL,
    upstream_user_id bigint NOT NULL,
    access_token_ciphertext text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT new_api_site_authorizations_revision_check CHECK ((revision > 0)),
    CONSTRAINT new_api_site_authorizations_upstream_user_id_check CHECK ((upstream_user_id > 0))
);

CREATE SEQUENCE IF NOT EXISTS public.new_api_site_authorizations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.new_api_site_authorizations_id_seq OWNED BY public.new_api_site_authorizations.id;

CREATE TABLE IF NOT EXISTS public.qq_bot_balance_alerts (
    id text NOT NULL,
    app_id text NOT NULL,
    member_id text NOT NULL,
    user_id bigint NOT NULL,
    subscription_revision text NOT NULL,
    binding_created_at timestamp with time zone NOT NULL,
    balance numeric(20,8) NOT NULL,
    threshold numeric(20,8) NOT NULL,
    site_name text DEFAULT ''::text NOT NULL,
    recharge_url text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    lease_token text DEFAULT ''::text NOT NULL,
    lease_until timestamp with time zone,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT qq_bot_balance_alerts_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'leased'::text, 'sent'::text, 'cancelled'::text, 'expired'::text]))),
    CONSTRAINT qq_bot_balance_alerts_threshold_check CHECK ((threshold > (0)::numeric))
);

CREATE TABLE IF NOT EXISTS public.qq_bot_balance_subscriptions (
    app_id text NOT NULL,
    member_id text NOT NULL,
    user_id bigint NOT NULL,
    binding_created_at timestamp with time zone NOT NULL,
    revision text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    verified_private_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS public.recharge_bonus_batch_holds (
    id bigint NOT NULL,
    batch_id character varying(255) NOT NULL,
    user_id bigint NOT NULL,
    grant_id bigint NOT NULL,
    held_amount numeric(20,8) NOT NULL,
    consumed_amount numeric(20,8) DEFAULT 0 NOT NULL,
    restored_amount numeric(20,8) DEFAULT 0 NOT NULL,
    expired_amount numeric(20,8) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'held'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    settled_at timestamp with time zone,
    CONSTRAINT recharge_bonus_batch_holds_amount_positive CHECK ((held_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_batch_holds_parts_bounded CHECK ((((consumed_amount + restored_amount) + expired_amount) <= held_amount)),
    CONSTRAINT recharge_bonus_batch_holds_parts_nonnegative CHECK (((consumed_amount >= (0)::numeric) AND (restored_amount >= (0)::numeric) AND (expired_amount >= (0)::numeric))),
    CONSTRAINT recharge_bonus_batch_holds_status_valid CHECK (((status)::text = ANY (ARRAY[('held'::character varying)::text, ('settled'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.recharge_bonus_batch_holds_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.recharge_bonus_batch_holds_id_seq OWNED BY public.recharge_bonus_batch_holds.id;

CREATE TABLE IF NOT EXISTS public.recharge_bonus_grants (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    payment_order_id bigint NOT NULL,
    rule_id bigint,
    granted_amount numeric(20,8) NOT NULL,
    remaining_amount numeric(20,8) NOT NULL,
    revoked_amount numeric(20,8) DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recharge_bonus_grants_amounts_bounded CHECK (((remaining_amount + revoked_amount) <= granted_amount)),
    CONSTRAINT recharge_bonus_grants_granted_positive CHECK ((granted_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_grants_remaining_nonnegative CHECK ((remaining_amount >= (0)::numeric)),
    CONSTRAINT recharge_bonus_grants_revoked_nonnegative CHECK ((revoked_amount >= (0)::numeric)),
    CONSTRAINT recharge_bonus_grants_status_valid CHECK (((status)::text = ANY (ARRAY[('active'::character varying)::text, ('exhausted'::character varying)::text, ('expired'::character varying)::text, ('revoked'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.recharge_bonus_grants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.recharge_bonus_grants_id_seq OWNED BY public.recharge_bonus_grants.id;

CREATE TABLE IF NOT EXISTS public.recharge_bonus_ledger (
    id bigint NOT NULL,
    grant_id bigint NOT NULL,
    user_id bigint NOT NULL,
    request_id character varying(255) NOT NULL,
    entry_type character varying(20) NOT NULL,
    amount numeric(20,8) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recharge_bonus_ledger_amount_positive CHECK ((amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_ledger_type_valid CHECK (((entry_type)::text = ANY (ARRAY[('grant'::character varying)::text, ('consume'::character varying)::text, ('expire'::character varying)::text, ('revoke'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.recharge_bonus_ledger_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.recharge_bonus_ledger_id_seq OWNED BY public.recharge_bonus_ledger.id;

CREATE TABLE IF NOT EXISTS public.recharge_bonus_order_snapshots (
    payment_order_id bigint NOT NULL,
    rule_id bigint,
    recharge_amount numeric(20,8) NOT NULL,
    bonus_amount numeric(20,8) NOT NULL,
    validity_days integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recharge_bonus_order_snapshot_amount_positive CHECK ((recharge_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_order_snapshot_bonus_positive CHECK ((bonus_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_order_snapshot_validity_nonnegative CHECK ((validity_days >= 0))
);

CREATE TABLE IF NOT EXISTS public.recharge_bonus_rules (
    id bigint NOT NULL,
    recharge_amount numeric(20,8) NOT NULL,
    bonus_amount numeric(20,8) NOT NULL,
    validity_days integer NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    starts_at timestamp with time zone,
    ends_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recharge_bonus_rules_amount_positive CHECK ((recharge_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_rules_bonus_positive CHECK ((bonus_amount > (0)::numeric)),
    CONSTRAINT recharge_bonus_rules_validity_nonnegative CHECK ((validity_days >= 0)),
    CONSTRAINT recharge_bonus_rules_window_valid CHECK (((ends_at IS NULL) OR (starts_at IS NULL) OR (ends_at > starts_at)))
);

CREATE SEQUENCE IF NOT EXISTS public.recharge_bonus_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.recharge_bonus_rules_id_seq OWNED BY public.recharge_bonus_rules.id;

CREATE TABLE IF NOT EXISTS public.registration_ip_quotas (
    ip_hash character varying(64) NOT NULL,
    account_count smallint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT registration_ip_quotas_account_count_check CHECK (((account_count >= 1) AND (account_count <= 2))),
    CONSTRAINT registration_ip_quotas_ip_hash_check CHECK ((length((ip_hash)::text) = 64))
);

CREATE TABLE IF NOT EXISTS public.support_center_messages (
    id bigint NOT NULL,
    ticket_id bigint NOT NULL,
    author_role character varying(8) NOT NULL,
    author_user_id bigint,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT support_center_messages_author_role_check CHECK (((author_role)::text = ANY (ARRAY[('user'::character varying)::text, ('admin'::character varying)::text]))),
    CONSTRAINT support_center_messages_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 5000)))
);

CREATE SEQUENCE IF NOT EXISTS public.support_center_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.support_center_messages_id_seq OWNED BY public.support_center_messages.id;

CREATE TABLE IF NOT EXISTS public.support_center_tickets (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    category character varying(32) DEFAULT ''::character varying NOT NULL,
    title character varying(100) NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    message_count integer DEFAULT 0 NOT NULL,
    last_message_at timestamp with time zone DEFAULT now() NOT NULL,
    last_message_role character varying(8) DEFAULT 'user'::character varying NOT NULL,
    user_unread boolean DEFAULT false NOT NULL,
    admin_unread boolean DEFAULT true NOT NULL,
    closed_at timestamp with time zone,
    closed_by_role character varying(8) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT support_center_tickets_closed_by_role_check CHECK (((closed_by_role)::text = ANY (ARRAY[(''::character varying)::text, ('user'::character varying)::text, ('admin'::character varying)::text]))),
    CONSTRAINT support_center_tickets_last_message_role_check CHECK (((last_message_role)::text = ANY (ARRAY[('user'::character varying)::text, ('admin'::character varying)::text]))),
    CONSTRAINT support_center_tickets_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('processing'::character varying)::text, ('replied'::character varying)::text, ('closed'::character varying)::text])))
);

CREATE SEQUENCE IF NOT EXISTS public.support_center_tickets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.support_center_tickets_id_seq OWNED BY public.support_center_tickets.id;

CREATE TABLE IF NOT EXISTS public.usage_billing_events (
    request_id character varying(255) NOT NULL,
    api_key_id bigint NOT NULL,
    request_fingerprint character varying(64) NOT NULL,
    schema_version integer NOT NULL,
    payload jsonb NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    result jsonb,
    attempts integer DEFAULT 0 NOT NULL,
    failure_code text,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    staged_at timestamp with time zone DEFAULT now() NOT NULL,
    applied_at timestamp with time zone,
    completed_at timestamp with time zone,
    CONSTRAINT usage_billing_events_payload_check CHECK ((jsonb_typeof(payload) = 'object'::text)),
    CONSTRAINT usage_billing_events_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT usage_billing_events_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'applied'::text, 'complete'::text, 'legacy'::text, 'blocked'::text])))
);

CREATE TABLE IF NOT EXISTS public.usage_billing_quotes (
    request_id character varying(255) NOT NULL,
    api_key_id bigint NOT NULL,
    request_fingerprint character varying(64) NOT NULL,
    quoted_actual_cost numeric NOT NULL,
    canonical_actual_cost numeric(20,8) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    quote_encoding_version smallint DEFAULT 0 NOT NULL
);

CREATE TABLE IF NOT EXISTS public.usage_platform_quota_totals (
    quota_id bigint NOT NULL,
    daily_usage_usd numeric(20,8) DEFAULT 0 NOT NULL,
    weekly_usage_usd numeric(20,8) DEFAULT 0 NOT NULL,
    monthly_usage_usd numeric(20,8) DEFAULT 0 NOT NULL,
    legacy_daily_anchor timestamp with time zone NOT NULL,
    legacy_weekly_anchor timestamp with time zone NOT NULL,
    legacy_monthly_anchor timestamp with time zone NOT NULL,
    daily_window_start timestamp with time zone NOT NULL,
    weekly_window_start timestamp with time zone NOT NULL,
    monthly_window_start timestamp with time zone NOT NULL,
    CONSTRAINT usage_platform_quota_totals_daily_usage_usd_check CHECK ((daily_usage_usd >= (0)::numeric)),
    CONSTRAINT usage_platform_quota_totals_monthly_usage_usd_check CHECK ((monthly_usage_usd >= (0)::numeric)),
    CONSTRAINT usage_platform_quota_totals_weekly_usage_usd_check CHECK ((weekly_usage_usd >= (0)::numeric))
);

CREATE TABLE IF NOT EXISTS public.user_cleanup_guards (
    user_id bigint NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    is_protected boolean DEFAULT false NOT NULL,
    updated_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS public.user_cleanup_identity_archives (
    preview_id uuid NOT NULL,
    user_id bigint NOT NULL,
    identities jsonb DEFAULT '[]'::jsonb NOT NULL,
    channels jsonb DEFAULT '[]'::jsonb NOT NULL,
    adoption_decisions jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS public.user_cleanup_previews (
    id uuid NOT NULL,
    actor_user_id bigint NOT NULL,
    candidate_ids bigint[] NOT NULL,
    candidate_snapshot jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '00:10:00'::interval) NOT NULL,
    completed_at timestamp with time zone,
    result jsonb
);

CREATE TABLE IF NOT EXISTS public.video_tasks (
    id text NOT NULL,
    upstream_task_id text,
    user_id bigint NOT NULL,
    api_key_id bigint NOT NULL,
    group_id bigint NOT NULL,
    account_id bigint NOT NULL,
    subscription_id bigint,
    pending jsonb NOT NULL,
    status text DEFAULT 'submitting'::text NOT NULL,
    next_poll_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text,
    request_hash text,
    hold_amount numeric(20,8) DEFAULT 0 NOT NULL,
    billing_state text DEFAULT 'legacy'::text NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT video_tasks_status_check CHECK ((status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'done'::text, 'failed'::text, 'needs_review'::text])))
);

ALTER TABLE ONLY public.application_run_steps ALTER COLUMN id SET DEFAULT nextval('public.application_run_steps_id_seq'::regclass);

ALTER TABLE ONLY public.channel_monitor_v2_candy_results ALTER COLUMN id SET DEFAULT nextval('public.channel_monitor_v2_candy_results_id_seq'::regclass);

ALTER TABLE ONLY public.channel_monitor_v3_categories ALTER COLUMN id SET DEFAULT nextval('public.channel_monitor_v3_categories_id_seq'::regclass);

ALTER TABLE ONLY public.channel_monitor_v3_components ALTER COLUMN id SET DEFAULT nextval('public.channel_monitor_v3_components_id_seq'::regclass);

ALTER TABLE ONLY public.channel_monitor_v3_probe_results ALTER COLUMN id SET DEFAULT nextval('public.channel_monitor_v3_probe_results_id_seq'::regclass);

ALTER TABLE ONLY public.global_context_pricing_rules ALTER COLUMN id SET DEFAULT nextval('public.global_context_pricing_rules_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_procurement_adjustments ALTER COLUMN id SET DEFAULT nextval('public.kitty_procurement_adjustments_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_supplier_bindings ALTER COLUMN id SET DEFAULT nextval('public.kitty_supplier_bindings_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_supplier_cost_rules ALTER COLUMN id SET DEFAULT nextval('public.kitty_supplier_cost_rules_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_supplier_credit_rates ALTER COLUMN id SET DEFAULT nextval('public.kitty_supplier_credit_rates_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_supplier_purchases ALTER COLUMN id SET DEFAULT nextval('public.kitty_supplier_purchases_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_supplier_wallets ALTER COLUMN id SET DEFAULT nextval('public.kitty_supplier_wallets_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_suppliers ALTER COLUMN id SET DEFAULT nextval('public.kitty_suppliers_id_seq'::regclass);

ALTER TABLE ONLY public.kitty_video_price_changes ALTER COLUMN id SET DEFAULT nextval('public.kitty_video_price_changes_id_seq'::regclass);

ALTER TABLE ONLY public.new_api_site_authorizations ALTER COLUMN id SET DEFAULT nextval('public.new_api_site_authorizations_id_seq'::regclass);

ALTER TABLE ONLY public.recharge_bonus_batch_holds ALTER COLUMN id SET DEFAULT nextval('public.recharge_bonus_batch_holds_id_seq'::regclass);

ALTER TABLE ONLY public.recharge_bonus_grants ALTER COLUMN id SET DEFAULT nextval('public.recharge_bonus_grants_id_seq'::regclass);

ALTER TABLE ONLY public.recharge_bonus_ledger ALTER COLUMN id SET DEFAULT nextval('public.recharge_bonus_ledger_id_seq'::regclass);

ALTER TABLE ONLY public.recharge_bonus_rules ALTER COLUMN id SET DEFAULT nextval('public.recharge_bonus_rules_id_seq'::regclass);

ALTER TABLE ONLY public.support_center_messages ALTER COLUMN id SET DEFAULT nextval('public.support_center_messages_id_seq'::regclass);

ALTER TABLE ONLY public.support_center_tickets ALTER COLUMN id SET DEFAULT nextval('public.support_center_tickets_id_seq'::regclass);

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.api_key_groups'::regclass AND conname='api_key_groups_pkey') THEN
  ALTER TABLE ONLY public.api_key_groups
    ADD CONSTRAINT api_key_groups_pkey PRIMARY KEY (api_key_id, group_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_run_steps'::regclass AND conname='application_run_steps_pkey') THEN
  ALTER TABLE ONLY public.application_run_steps
    ADD CONSTRAINT application_run_steps_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_run_steps'::regclass AND conname='application_run_steps_run_step_unique') THEN
  ALTER TABLE ONLY public.application_run_steps
    ADD CONSTRAINT application_run_steps_run_step_unique UNIQUE (run_id, step_key);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_runs'::regclass AND conname='application_runs_pkey') THEN
  ALTER TABLE ONLY public.application_runs
    ADD CONSTRAINT application_runs_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_runs'::regclass AND conname='application_runs_session_unique') THEN
  ALTER TABLE ONLY public.application_runs
    ADD CONSTRAINT application_runs_session_unique UNIQUE (session_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.batch_image_money_holds'::regclass AND conname='batch_image_money_holds_pkey') THEN
  ALTER TABLE ONLY public.batch_image_money_holds
    ADD CONSTRAINT batch_image_money_holds_pkey PRIMARY KEY (batch_id, api_key_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v2_candy_results'::regclass AND conname='channel_monitor_v2_candy_results_group_id_config_key_slot_key') THEN
  ALTER TABLE ONLY public.channel_monitor_v2_candy_results
    ADD CONSTRAINT channel_monitor_v2_candy_results_group_id_config_key_slot_key UNIQUE (group_id, config_key, slot);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v2_candy_results'::regclass AND conname='channel_monitor_v2_candy_results_pkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v2_candy_results
    ADD CONSTRAINT channel_monitor_v2_candy_results_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_categories'::regclass AND conname='channel_monitor_v3_categories_pkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_categories
    ADD CONSTRAINT channel_monitor_v3_categories_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_components'::regclass AND conname='channel_monitor_v3_components_pkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_components
    ADD CONSTRAINT channel_monitor_v3_components_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_config'::regclass AND conname='channel_monitor_v3_config_pkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_config
    ADD CONSTRAINT channel_monitor_v3_config_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_probe_results'::regclass AND conname='channel_monitor_v3_probe_results_pkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_probe_results
    ADD CONSTRAINT channel_monitor_v3_probe_results_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.global_context_pricing_rules'::regclass AND conname='global_context_pricing_rules_model_key') THEN
  ALTER TABLE ONLY public.global_context_pricing_rules
    ADD CONSTRAINT global_context_pricing_rules_model_key UNIQUE (model);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.global_context_pricing_rules'::regclass AND conname='global_context_pricing_rules_pkey') THEN
  ALTER TABLE ONLY public.global_context_pricing_rules
    ADD CONSTRAINT global_context_pricing_rules_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.image_supplier_tasks'::regclass AND conname='image_supplier_tasks_api_key_id_idempotency_hash_key') THEN
  ALTER TABLE ONLY public.image_supplier_tasks
    ADD CONSTRAINT image_supplier_tasks_api_key_id_idempotency_hash_key UNIQUE (api_key_id, idempotency_hash);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.image_supplier_tasks'::regclass AND conname='image_supplier_tasks_pkey') THEN
  ALTER TABLE ONLY public.image_supplier_tasks
    ADD CONSTRAINT image_supplier_tasks_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_identities'::regclass AND conname='kitty_media_identities_pkey') THEN
  ALTER TABLE ONLY public.kitty_media_identities
    ADD CONSTRAINT kitty_media_identities_pkey PRIMARY KEY (user_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_identities'::regclass AND conname='kitty_media_identities_subject_key') THEN
  ALTER TABLE ONLY public.kitty_media_identities
    ADD CONSTRAINT kitty_media_identities_subject_key UNIQUE (subject);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_storage_orders'::regclass AND conname='kitty_media_storage_orders_pkey') THEN
  ALTER TABLE ONLY public.kitty_media_storage_orders
    ADD CONSTRAINT kitty_media_storage_orders_pkey PRIMARY KEY (order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_wallet_ledger'::regclass AND conname='kitty_media_wallet_ledger_pkey') THEN
  ALTER TABLE ONLY public.kitty_media_wallet_ledger
    ADD CONSTRAINT kitty_media_wallet_ledger_pkey PRIMARY KEY (order_id, entry_type);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_wallet_orders'::regclass AND conname='kitty_media_wallet_orders_pkey') THEN
  ALTER TABLE ONLY public.kitty_media_wallet_orders
    ADD CONSTRAINT kitty_media_wallet_orders_pkey PRIMARY KEY (order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_permanent_usage'::regclass AND conname='kitty_permanent_usage_pkey') THEN
  ALTER TABLE ONLY public.kitty_permanent_usage
    ADD CONSTRAINT kitty_permanent_usage_pkey PRIMARY KEY (api_key_id, request_key);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_permanent_usage_state'::regclass AND conname='kitty_permanent_usage_state_pkey') THEN
  ALTER TABLE ONLY public.kitty_permanent_usage_state
    ADD CONSTRAINT kitty_permanent_usage_state_pkey PRIMARY KEY (singleton);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_procurement_adjustments'::regclass AND conname='kitty_procurement_adjustments_operation_id_key') THEN
  ALTER TABLE ONLY public.kitty_procurement_adjustments
    ADD CONSTRAINT kitty_procurement_adjustments_operation_id_key UNIQUE (operation_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_procurement_adjustments'::regclass AND conname='kitty_procurement_adjustments_pkey') THEN
  ALTER TABLE ONLY public.kitty_procurement_adjustments
    ADD CONSTRAINT kitty_procurement_adjustments_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_procurement_receipts'::regclass AND conname='kitty_procurement_receipts_pkey') THEN
  ALTER TABLE ONLY public.kitty_procurement_receipts
    ADD CONSTRAINT kitty_procurement_receipts_pkey PRIMARY KEY (api_key_id, request_key);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_provider_operations_state'::regclass AND conname='kitty_provider_operations_state_pkey') THEN
  ALTER TABLE ONLY public.kitty_provider_operations_state
    ADD CONSTRAINT kitty_provider_operations_state_pkey PRIMARY KEY (singleton);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_retained_media'::regclass AND conname='kitty_retained_media_pkey') THEN
  ALTER TABLE ONLY public.kitty_retained_media
    ADD CONSTRAINT kitty_retained_media_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_retained_media'::regclass AND conname='kitty_retained_media_user_id_source_key_key') THEN
  ALTER TABLE ONLY public.kitty_retained_media
    ADD CONSTRAINT kitty_retained_media_user_id_source_key_key UNIQUE (user_id, source_key);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_bindings'::regclass AND conname='kitty_supplier_bindings_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_bindings
    ADD CONSTRAINT kitty_supplier_bindings_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_cost_rules'::regclass AND conname='kitty_supplier_cost_rules_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_cost_rules
    ADD CONSTRAINT kitty_supplier_cost_rules_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_credit_rates'::regclass AND conname='kitty_supplier_credit_rates_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_credit_rates
    ADD CONSTRAINT kitty_supplier_credit_rates_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_purchases'::regclass AND conname='kitty_supplier_purchases_operation_id_key') THEN
  ALTER TABLE ONLY public.kitty_supplier_purchases
    ADD CONSTRAINT kitty_supplier_purchases_operation_id_key UNIQUE (operation_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_purchases'::regclass AND conname='kitty_supplier_purchases_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_purchases
    ADD CONSTRAINT kitty_supplier_purchases_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_remote_operations'::regclass AND conname='kitty_supplier_remote_operations_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_remote_operations
    ADD CONSTRAINT kitty_supplier_remote_operations_pkey PRIMARY KEY (operation_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_wallets'::regclass AND conname='kitty_supplier_wallets_pkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_wallets
    ADD CONSTRAINT kitty_supplier_wallets_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_wallets'::regclass AND conname='kitty_supplier_wallets_supplier_id_identity_key') THEN
  ALTER TABLE ONLY public.kitty_supplier_wallets
    ADD CONSTRAINT kitty_supplier_wallets_supplier_id_identity_key UNIQUE (supplier_id, identity);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_suppliers'::regclass AND conname='kitty_suppliers_pkey') THEN
  ALTER TABLE ONLY public.kitty_suppliers
    ADD CONSTRAINT kitty_suppliers_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_video_price_changes'::regclass AND conname='kitty_video_price_changes_pkey') THEN
  ALTER TABLE ONLY public.kitty_video_price_changes
    ADD CONSTRAINT kitty_video_price_changes_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.new_api_account_bindings'::regclass AND conname='new_api_account_bindings_pkey') THEN
  ALTER TABLE ONLY public.new_api_account_bindings
    ADD CONSTRAINT new_api_account_bindings_pkey PRIMARY KEY (account_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.new_api_site_authorizations'::regclass AND conname='new_api_site_authorizations_pkey') THEN
  ALTER TABLE ONLY public.new_api_site_authorizations
    ADD CONSTRAINT new_api_site_authorizations_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.new_api_site_authorizations'::regclass AND conname='new_api_site_authorizations_site_url_upstream_user_id_key') THEN
  ALTER TABLE ONLY public.new_api_site_authorizations
    ADD CONSTRAINT new_api_site_authorizations_site_url_upstream_user_id_key UNIQUE (site_url, upstream_user_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_alerts'::regclass AND conname='qq_bot_balance_alerts_pkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_alerts
    ADD CONSTRAINT qq_bot_balance_alerts_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_subscriptions'::regclass AND conname='qq_bot_balance_subscriptions_pkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_subscriptions
    ADD CONSTRAINT qq_bot_balance_subscriptions_pkey PRIMARY KEY (app_id, member_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_batch_holds'::regclass AND conname='recharge_bonus_batch_holds_pkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_batch_holds
    ADD CONSTRAINT recharge_bonus_batch_holds_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_batch_holds'::regclass AND conname='recharge_bonus_batch_holds_unique') THEN
  ALTER TABLE ONLY public.recharge_bonus_batch_holds
    ADD CONSTRAINT recharge_bonus_batch_holds_unique UNIQUE (batch_id, grant_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_grants'::regclass AND conname='recharge_bonus_grants_payment_order_id_key') THEN
  ALTER TABLE ONLY public.recharge_bonus_grants
    ADD CONSTRAINT recharge_bonus_grants_payment_order_id_key UNIQUE (payment_order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_grants'::regclass AND conname='recharge_bonus_grants_pkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_grants
    ADD CONSTRAINT recharge_bonus_grants_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_ledger'::regclass AND conname='recharge_bonus_ledger_idempotent') THEN
  ALTER TABLE ONLY public.recharge_bonus_ledger
    ADD CONSTRAINT recharge_bonus_ledger_idempotent UNIQUE (grant_id, request_id, entry_type);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_ledger'::regclass AND conname='recharge_bonus_ledger_pkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_ledger
    ADD CONSTRAINT recharge_bonus_ledger_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_order_snapshots'::regclass AND conname='recharge_bonus_order_snapshots_pkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_order_snapshots
    ADD CONSTRAINT recharge_bonus_order_snapshots_pkey PRIMARY KEY (payment_order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_rules'::regclass AND conname='recharge_bonus_rules_pkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_rules
    ADD CONSTRAINT recharge_bonus_rules_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_rules'::regclass AND conname='recharge_bonus_rules_recharge_amount_key') THEN
  ALTER TABLE ONLY public.recharge_bonus_rules
    ADD CONSTRAINT recharge_bonus_rules_recharge_amount_key UNIQUE (recharge_amount);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.registration_ip_quotas'::regclass AND conname='registration_ip_quotas_pkey') THEN
  ALTER TABLE ONLY public.registration_ip_quotas
    ADD CONSTRAINT registration_ip_quotas_pkey PRIMARY KEY (ip_hash);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.support_center_messages'::regclass AND conname='support_center_messages_pkey') THEN
  ALTER TABLE ONLY public.support_center_messages
    ADD CONSTRAINT support_center_messages_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.support_center_tickets'::regclass AND conname='support_center_tickets_pkey') THEN
  ALTER TABLE ONLY public.support_center_tickets
    ADD CONSTRAINT support_center_tickets_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_probe_results'::regclass AND conname='uq_channel_monitor_v3_probe_slot') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_probe_results
    ADD CONSTRAINT uq_channel_monitor_v3_probe_slot UNIQUE (group_id, model, slot_start);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.usage_billing_events'::regclass AND conname='usage_billing_events_pkey') THEN
  ALTER TABLE ONLY public.usage_billing_events
    ADD CONSTRAINT usage_billing_events_pkey PRIMARY KEY (request_id, api_key_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.usage_billing_quotes'::regclass AND conname='usage_billing_quotes_pkey') THEN
  ALTER TABLE ONLY public.usage_billing_quotes
    ADD CONSTRAINT usage_billing_quotes_pkey PRIMARY KEY (request_id, api_key_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.usage_platform_quota_totals'::regclass AND conname='usage_platform_quota_totals_pkey') THEN
  ALTER TABLE ONLY public.usage_platform_quota_totals
    ADD CONSTRAINT usage_platform_quota_totals_pkey PRIMARY KEY (quota_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.user_cleanup_guards'::regclass AND conname='user_cleanup_guards_pkey') THEN
  ALTER TABLE ONLY public.user_cleanup_guards
    ADD CONSTRAINT user_cleanup_guards_pkey PRIMARY KEY (user_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.user_cleanup_identity_archives'::regclass AND conname='user_cleanup_identity_archives_pkey') THEN
  ALTER TABLE ONLY public.user_cleanup_identity_archives
    ADD CONSTRAINT user_cleanup_identity_archives_pkey PRIMARY KEY (preview_id, user_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.user_cleanup_previews'::regclass AND conname='user_cleanup_previews_pkey') THEN
  ALTER TABLE ONLY public.user_cleanup_previews
    ADD CONSTRAINT user_cleanup_previews_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.video_tasks'::regclass AND conname='video_tasks_pkey') THEN
  ALTER TABLE ONLY public.video_tasks
    ADD CONSTRAINT video_tasks_pkey PRIMARY KEY (id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.video_tasks'::regclass AND conname='video_tasks_user_id_api_key_id_upstream_task_id_key') THEN
  ALTER TABLE ONLY public.video_tasks
    ADD CONSTRAINT video_tasks_user_id_api_key_id_upstream_task_id_key UNIQUE (user_id, api_key_id, upstream_task_id);
 END IF;
END $kitty_schema$;

CREATE INDEX IF NOT EXISTS idx_api_key_groups_group_id ON public.api_key_groups USING btree (group_id, api_key_id);

CREATE INDEX IF NOT EXISTS idx_application_run_steps_grok_video_request ON public.application_run_steps USING btree (gateway_request_id, run_id) WHERE ((gateway_request_id IS NOT NULL) AND ((executor)::text = ANY (ARRAY[('video_generation'::character varying)::text, ('video_edit'::character varying)::text, ('video_extension'::character varying)::text])));

CREATE INDEX IF NOT EXISTS idx_application_run_steps_run_sequence ON public.application_run_steps USING btree (run_id, sequence, id);

CREATE INDEX IF NOT EXISTS idx_application_run_steps_run_status ON public.application_run_steps USING btree (run_id, status, sequence);

CREATE INDEX IF NOT EXISTS idx_application_runs_session ON public.application_runs USING btree (session_id);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_app_created ON public.application_runs USING btree (user_id, app_slug, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_created ON public.application_runs USING btree (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_application_runs_user_status_created ON public.application_runs USING btree (user_id, status, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_candy_history ON public.channel_monitor_v2_candy_results USING btree (group_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_candy_history_key ON public.channel_monitor_v2_candy_results USING btree (group_id, config_key, checked_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_monitor_v2_candy_running ON public.channel_monitor_v2_candy_results USING btree (group_id, config_key) WHERE ((verdict)::text = 'running'::text);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v3_components_order ON public.channel_monitor_v3_components USING btree (category_id, sort_order, id);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v3_probe_results_slot ON public.channel_monitor_v3_probe_results USING btree (slot_start);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_batch_holds_batch ON public.recharge_bonus_batch_holds USING btree (batch_id, status, id);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_grants_user_active_expiry ON public.recharge_bonus_grants USING btree (user_id, expires_at, id) WHERE ((remaining_amount > (0)::numeric) AND ((status)::text = 'active'::text));

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_ledger_user_created ON public.recharge_bonus_ledger USING btree (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_rules_active ON public.recharge_bonus_rules USING btree (enabled, recharge_amount, starts_at, ends_at);

CREATE INDEX IF NOT EXISTS idx_support_center_messages_author_time ON public.support_center_messages USING btree (author_user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_support_center_messages_ticket ON public.support_center_messages USING btree (ticket_id, id);

CREATE INDEX IF NOT EXISTS idx_support_center_tickets_status_activity ON public.support_center_tickets USING btree (status, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_support_center_tickets_user_activity ON public.support_center_tickets USING btree (user_id, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_cleanup_previews_actor_created ON public.user_cleanup_previews USING btree (actor_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS image_supplier_tasks_pending_idx ON public.image_supplier_tasks USING btree (next_poll_at) WHERE (status = ANY (ARRAY['pending'::text, 'ready'::text, 'submitting'::text]));

CREATE INDEX IF NOT EXISTS image_supplier_tasks_result_expiry_idx ON public.image_supplier_tasks USING btree (updated_at) WHERE ((result IS NOT NULL) AND (status = 'completed'::text));

CREATE INDEX IF NOT EXISTS kitty_media_storage_orders_owner ON public.kitty_media_storage_orders USING btree (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS kitty_media_wallet_pending_idx ON public.kitty_media_wallet_orders USING btree (updated_at) WHERE (billing_state = 'held'::text);

CREATE INDEX IF NOT EXISTS kitty_media_wallet_user_idx ON public.kitty_media_wallet_orders USING btree (user_id, created_at);

CREATE INDEX IF NOT EXISTS kitty_permanent_usage_user ON public.kitty_permanent_usage USING btree (user_id);

CREATE INDEX IF NOT EXISTS kitty_procurement_adjustment_receipt ON public.kitty_procurement_adjustments USING btree (api_key_id, request_key, id DESC);

CREATE INDEX IF NOT EXISTS kitty_procurement_receipt_account ON public.kitty_procurement_receipts USING btree (account_id, observed_at);

CREATE INDEX IF NOT EXISTS kitty_procurement_receipt_supplier ON public.kitty_procurement_receipts USING btree (supplier_id, observed_at);

CREATE INDEX IF NOT EXISTS kitty_procurement_receipt_time ON public.kitty_procurement_receipts USING btree (observed_at);

CREATE INDEX IF NOT EXISTS kitty_retained_media_expiry ON public.kitty_retained_media USING btree (expires_at) WHERE (status = 'active'::text);

CREATE INDEX IF NOT EXISTS kitty_retained_media_owner ON public.kitty_retained_media USING btree (user_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS kitty_supplier_binding_current ON public.kitty_supplier_bindings USING btree (account_id) WHERE (ended_at IS NULL);

CREATE INDEX IF NOT EXISTS kitty_supplier_binding_history ON public.kitty_supplier_bindings USING btree (account_id, started_at DESC);

CREATE INDEX IF NOT EXISTS kitty_supplier_binding_wallet ON public.kitty_supplier_bindings USING btree (wallet_id) WHERE (ended_at IS NULL);

CREATE INDEX IF NOT EXISTS kitty_supplier_credit_rate_history ON public.kitty_supplier_credit_rates USING btree (wallet_id, recorded_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS kitty_supplier_purchase_wallet ON public.kitty_supplier_purchases USING btree (wallet_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS kitty_supplier_rule_current ON public.kitty_supplier_cost_rules USING btree (wallet_id, model) WHERE (ended_at IS NULL);

CREATE INDEX IF NOT EXISTS kitty_supplier_rule_history ON public.kitty_supplier_cost_rules USING btree (wallet_id, model, started_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS kitty_suppliers_name ON public.kitty_suppliers USING btree (lower(name));

CREATE INDEX IF NOT EXISTS new_api_account_bindings_authorization_id_idx ON public.new_api_account_bindings USING btree (authorization_id);

CREATE UNIQUE INDEX IF NOT EXISTS qq_bot_balance_alerts_pending_member ON public.qq_bot_balance_alerts USING btree (app_id, member_id) WHERE (status = ANY (ARRAY['pending'::text, 'leased'::text]));

CREATE INDEX IF NOT EXISTS qq_bot_balance_alerts_queue ON public.qq_bot_balance_alerts USING btree (next_attempt_at, created_at) WHERE (status = ANY (ARRAY['pending'::text, 'leased'::text]));

CREATE INDEX IF NOT EXISTS usage_billing_events_pending ON public.usage_billing_events USING btree (next_attempt_at, staged_at) WHERE (state = ANY (ARRAY['pending'::text, 'applied'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_created_id_idx ON public.video_tasks USING btree (created_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS video_tasks_idempotency_key_idx ON public.video_tasks USING btree (api_key_id, idempotency_key) WHERE (idempotency_key IS NOT NULL);

CREATE INDEX IF NOT EXISTS video_tasks_poll_idx ON public.video_tasks USING btree (next_poll_at) WHERE (status = 'pending'::text);

CREATE INDEX IF NOT EXISTS video_tasks_prepaid_references_idx ON public.video_tasks USING btree (updated_at) WHERE (billing_state = ANY (ARRAY['held'::text, 'refund_pending'::text, 'captured'::text, 'refunded'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_unsettled_account_idx ON public.video_tasks USING btree (account_id) WHERE (status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'needs_review'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_unsettled_group_idx ON public.video_tasks USING btree (group_id) WHERE (status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'needs_review'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_unsettled_key_idx ON public.video_tasks USING btree (api_key_id) WHERE (status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'needs_review'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_unsettled_subscription_idx ON public.video_tasks USING btree (subscription_id) WHERE ((status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'needs_review'::text])) AND (subscription_id IS NOT NULL));

CREATE INDEX IF NOT EXISTS video_tasks_unsettled_user_idx ON public.video_tasks USING btree (user_id) WHERE (status = ANY (ARRAY['submitting'::text, 'pending'::text, 'unknown'::text, 'needs_review'::text]));

CREATE INDEX IF NOT EXISTS video_tasks_user_created_id_idx ON public.video_tasks USING btree (user_id, created_at DESC, id DESC);

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.api_key_groups'::regclass AND conname='api_key_groups_api_key_id_fkey') THEN
  ALTER TABLE ONLY public.api_key_groups
    ADD CONSTRAINT api_key_groups_api_key_id_fkey FOREIGN KEY (api_key_id) REFERENCES public.api_keys(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.api_key_groups'::regclass AND conname='api_key_groups_group_id_fkey') THEN
  ALTER TABLE ONLY public.api_key_groups
    ADD CONSTRAINT api_key_groups_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_run_steps'::regclass AND conname='application_run_steps_run_id_fkey') THEN
  ALTER TABLE ONLY public.application_run_steps
    ADD CONSTRAINT application_run_steps_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.application_runs(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_runs'::regclass AND conname='application_runs_media_api_key_id_fkey') THEN
  ALTER TABLE ONLY public.application_runs
    ADD CONSTRAINT application_runs_media_api_key_id_fkey FOREIGN KEY (media_api_key_id) REFERENCES public.api_keys(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_runs'::regclass AND conname='application_runs_planner_api_key_id_fkey') THEN
  ALTER TABLE ONLY public.application_runs
    ADD CONSTRAINT application_runs_planner_api_key_id_fkey FOREIGN KEY (planner_api_key_id) REFERENCES public.api_keys(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.application_runs'::regclass AND conname='application_runs_user_id_fkey') THEN
  ALTER TABLE ONLY public.application_runs
    ADD CONSTRAINT application_runs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v2_candy_results'::regclass AND conname='channel_monitor_v2_candy_results_group_id_fkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v2_candy_results
    ADD CONSTRAINT channel_monitor_v2_candy_results_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_components'::regclass AND conname='channel_monitor_v3_components_category_id_fkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_components
    ADD CONSTRAINT channel_monitor_v3_components_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.channel_monitor_v3_categories(id) ON DELETE SET NULL;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_components'::regclass AND conname='channel_monitor_v3_components_group_id_fkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_components
    ADD CONSTRAINT channel_monitor_v3_components_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_config'::regclass AND conname='channel_monitor_v3_config_featured_component_id_fkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_config
    ADD CONSTRAINT channel_monitor_v3_config_featured_component_id_fkey FOREIGN KEY (featured_component_id) REFERENCES public.channel_monitor_v3_components(id) ON DELETE SET NULL;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.channel_monitor_v3_probe_results'::regclass AND conname='channel_monitor_v3_probe_results_group_id_fkey') THEN
  ALTER TABLE ONLY public.channel_monitor_v3_probe_results
    ADD CONSTRAINT channel_monitor_v3_probe_results_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.groups(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_identities'::regclass AND conname='kitty_media_identities_user_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_identities
    ADD CONSTRAINT kitty_media_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE RESTRICT;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_storage_orders'::regclass AND conname='kitty_media_storage_orders_asset_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_storage_orders
    ADD CONSTRAINT kitty_media_storage_orders_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES public.kitty_retained_media(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_storage_orders'::regclass AND conname='kitty_media_storage_orders_order_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_storage_orders
    ADD CONSTRAINT kitty_media_storage_orders_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.kitty_media_wallet_orders(order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_storage_orders'::regclass AND conname='kitty_media_storage_orders_user_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_storage_orders
    ADD CONSTRAINT kitty_media_storage_orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_wallet_ledger'::regclass AND conname='kitty_media_wallet_ledger_order_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_wallet_ledger
    ADD CONSTRAINT kitty_media_wallet_ledger_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.kitty_media_wallet_orders(order_id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_media_wallet_orders'::regclass AND conname='kitty_media_wallet_orders_user_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_media_wallet_orders
    ADD CONSTRAINT kitty_media_wallet_orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_procurement_adjustments'::regclass AND conname='kitty_procurement_adjustments_api_key_id_request_key_fkey') THEN
  ALTER TABLE ONLY public.kitty_procurement_adjustments
    ADD CONSTRAINT kitty_procurement_adjustments_api_key_id_request_key_fkey FOREIGN KEY (api_key_id, request_key) REFERENCES public.kitty_procurement_receipts(api_key_id, request_key);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_retained_media'::regclass AND conname='kitty_retained_media_user_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_retained_media
    ADD CONSTRAINT kitty_retained_media_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_bindings'::regclass AND conname='kitty_supplier_bindings_wallet_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_bindings
    ADD CONSTRAINT kitty_supplier_bindings_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.kitty_supplier_wallets(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_cost_rules'::regclass AND conname='kitty_supplier_cost_rules_wallet_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_cost_rules
    ADD CONSTRAINT kitty_supplier_cost_rules_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.kitty_supplier_wallets(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_credit_rates'::regclass AND conname='kitty_supplier_credit_rates_wallet_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_credit_rates
    ADD CONSTRAINT kitty_supplier_credit_rates_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.kitty_supplier_wallets(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_purchases'::regclass AND conname='kitty_supplier_purchases_wallet_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_purchases
    ADD CONSTRAINT kitty_supplier_purchases_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.kitty_supplier_wallets(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_remote_operations'::regclass AND conname='kitty_supplier_remote_operations_wallet_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_remote_operations
    ADD CONSTRAINT kitty_supplier_remote_operations_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES public.kitty_supplier_wallets(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.kitty_supplier_wallets'::regclass AND conname='kitty_supplier_wallets_supplier_id_fkey') THEN
  ALTER TABLE ONLY public.kitty_supplier_wallets
    ADD CONSTRAINT kitty_supplier_wallets_supplier_id_fkey FOREIGN KEY (supplier_id) REFERENCES public.kitty_suppliers(id);
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.new_api_account_bindings'::regclass AND conname='new_api_account_bindings_account_id_fkey') THEN
  ALTER TABLE ONLY public.new_api_account_bindings
    ADD CONSTRAINT new_api_account_bindings_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.new_api_account_bindings'::regclass AND conname='new_api_account_bindings_authorization_id_fkey') THEN
  ALTER TABLE ONLY public.new_api_account_bindings
    ADD CONSTRAINT new_api_account_bindings_authorization_id_fkey FOREIGN KEY (authorization_id) REFERENCES public.new_api_site_authorizations(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_alerts'::regclass AND conname='qq_bot_balance_alerts_app_id_member_id_fkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_alerts
    ADD CONSTRAINT qq_bot_balance_alerts_app_id_member_id_fkey FOREIGN KEY (app_id, member_id) REFERENCES public.qq_bot_balance_subscriptions(app_id, member_id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_alerts'::regclass AND conname='qq_bot_balance_alerts_user_id_fkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_alerts
    ADD CONSTRAINT qq_bot_balance_alerts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_subscriptions'::regclass AND conname='qq_bot_balance_subscriptions_app_id_member_id_fkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_subscriptions
    ADD CONSTRAINT qq_bot_balance_subscriptions_app_id_member_id_fkey FOREIGN KEY (app_id, member_id) REFERENCES public.qq_bot_bindings(app_id, member_id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.qq_bot_balance_subscriptions'::regclass AND conname='qq_bot_balance_subscriptions_user_id_fkey') THEN
  ALTER TABLE ONLY public.qq_bot_balance_subscriptions
    ADD CONSTRAINT qq_bot_balance_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_batch_holds'::regclass AND conname='recharge_bonus_batch_holds_grant_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_batch_holds
    ADD CONSTRAINT recharge_bonus_batch_holds_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES public.recharge_bonus_grants(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_batch_holds'::regclass AND conname='recharge_bonus_batch_holds_user_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_batch_holds
    ADD CONSTRAINT recharge_bonus_batch_holds_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_grants'::regclass AND conname='recharge_bonus_grants_payment_order_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_grants
    ADD CONSTRAINT recharge_bonus_grants_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES public.payment_orders(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_grants'::regclass AND conname='recharge_bonus_grants_rule_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_grants
    ADD CONSTRAINT recharge_bonus_grants_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.recharge_bonus_rules(id) ON DELETE SET NULL;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_grants'::regclass AND conname='recharge_bonus_grants_user_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_grants
    ADD CONSTRAINT recharge_bonus_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_ledger'::regclass AND conname='recharge_bonus_ledger_grant_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_ledger
    ADD CONSTRAINT recharge_bonus_ledger_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES public.recharge_bonus_grants(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_ledger'::regclass AND conname='recharge_bonus_ledger_user_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_ledger
    ADD CONSTRAINT recharge_bonus_ledger_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_order_snapshots'::regclass AND conname='recharge_bonus_order_snapshots_payment_order_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_order_snapshots
    ADD CONSTRAINT recharge_bonus_order_snapshots_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES public.payment_orders(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.recharge_bonus_order_snapshots'::regclass AND conname='recharge_bonus_order_snapshots_rule_id_fkey') THEN
  ALTER TABLE ONLY public.recharge_bonus_order_snapshots
    ADD CONSTRAINT recharge_bonus_order_snapshots_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.recharge_bonus_rules(id) ON DELETE SET NULL;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.support_center_messages'::regclass AND conname='support_center_messages_author_user_id_fkey') THEN
  ALTER TABLE ONLY public.support_center_messages
    ADD CONSTRAINT support_center_messages_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.support_center_messages'::regclass AND conname='support_center_messages_ticket_id_fkey') THEN
  ALTER TABLE ONLY public.support_center_messages
    ADD CONSTRAINT support_center_messages_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.support_center_tickets(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.support_center_tickets'::regclass AND conname='support_center_tickets_user_id_fkey') THEN
  ALTER TABLE ONLY public.support_center_tickets
    ADD CONSTRAINT support_center_tickets_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.usage_platform_quota_totals'::regclass AND conname='usage_platform_quota_totals_quota_id_fkey') THEN
  ALTER TABLE ONLY public.usage_platform_quota_totals
    ADD CONSTRAINT usage_platform_quota_totals_quota_id_fkey FOREIGN KEY (quota_id) REFERENCES public.user_platform_quotas(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.user_cleanup_guards'::regclass AND conname='user_cleanup_guards_user_id_fkey') THEN
  ALTER TABLE ONLY public.user_cleanup_guards
    ADD CONSTRAINT user_cleanup_guards_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
 END IF;
END $kitty_schema$;

DO $kitty_schema$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.user_cleanup_identity_archives'::regclass AND conname='user_cleanup_identity_archives_preview_id_fkey') THEN
  ALTER TABLE ONLY public.user_cleanup_identity_archives
    ADD CONSTRAINT user_cleanup_identity_archives_preview_id_fkey FOREIGN KEY (preview_id) REFERENCES public.user_cleanup_previews(id);
 END IF;
END $kitty_schema$;

-- Feature-local functions; existing Custom auth, VM2, BPS and account policy triggers remain in place.

SET LOCAL check_function_bodies = false;

CREATE OR REPLACE FUNCTION public.invalidate_new_api_account_binding()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
 IF OLD.platform IS DISTINCT FROM NEW.platform
    OR OLD.type IS DISTINCT FROM NEW.type
    OR OLD.proxy_id IS DISTINCT FROM NEW.proxy_id
    OR OLD.credentials -> 'api_key' IS DISTINCT FROM NEW.credentials -> 'api_key'
    OR OLD.credentials -> 'base_url' IS DISTINCT FROM NEW.credentials -> 'base_url'
    OR OLD.credentials -> 'header_override_enabled' IS DISTINCT FROM NEW.credentials -> 'header_override_enabled'
    OR OLD.credentials -> 'header_overrides' IS DISTINCT FROM NEW.credentials -> 'header_overrides'
 THEN
  DELETE FROM new_api_account_bindings WHERE account_id = OLD.id;
  IF FOUND OR OLD.extra ->> 'upstream_billing_provider' = 'new_api' THEN
   NEW.extra := COALESCE(NEW.extra, '{}'::jsonb) - 'upstream_billing_provider' - 'upstream_billing_probe';
  END IF;
 END IF;
 RETURN NEW;
END;
$function$;

CREATE OR REPLACE FUNCTION public.kitty_backfill_permanent_usage(batch_size integer)
 RETURNS bigint
 LANGUAGE plpgsql
AS $function$
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
$function$;

CREATE OR REPLACE FUNCTION public.kitty_capture_permanent_usage(u usage_logs, overwrite_metrics boolean)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE k TEXT; invalid BOOLEAN; ac NUMERIC; tc NUMERIC;
BEGIN
 k:=CASE WHEN COALESCE(u.request_id,'')<>'' THEN 'request:'||u.request_id ELSE 'legacy-row:'||u.id::text END;
 ac:=CASE WHEN u.actual_cost>'-Infinity'::numeric AND u.actual_cost<'Infinity'::numeric THEN u.actual_cost END;
 tc:=CASE WHEN u.total_cost>'-Infinity'::numeric AND u.total_cost<'Infinity'::numeric THEN u.total_cost END;
 invalid:=(ac IS NULL OR tc IS NULL OR u.input_tokens<0 OR u.output_tokens<0 OR u.cache_creation_tokens<0 OR u.cache_read_tokens<0 OR u.cache_creation_30m_tokens<0 OR u.cache_creation_12h_tokens<0);
 INSERT INTO kitty_permanent_usage(source_log_id,api_key_id,request_key,user_id,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,cache_creation_30m_tokens,cache_creation_12h_tokens,actual_cost,total_cost,invalid_metrics,billing_type,observed_at)
 VALUES(u.id,u.api_key_id,k,u.user_id,GREATEST(u.input_tokens,0),GREATEST(u.output_tokens,0),GREATEST(u.cache_creation_tokens,0),GREATEST(u.cache_read_tokens,0),GREATEST(u.cache_creation_30m_tokens,0),GREATEST(u.cache_creation_12h_tokens,0),ac,tc,invalid,u.billing_type,u.created_at)
 ON CONFLICT DO NOTHING;
 -- A replay with a new log ID cannot overwrite a previously corrected receipt.
 -- Baseline backfill never replaces a live, newer trigger snapshot.
 IF overwrite_metrics THEN
  UPDATE kitty_permanent_usage SET input_tokens=GREATEST(u.input_tokens,0),output_tokens=GREATEST(u.output_tokens,0),
   cache_creation_tokens=GREATEST(u.cache_creation_tokens,0),cache_read_tokens=GREATEST(u.cache_read_tokens,0),
   cache_creation_30m_tokens=GREATEST(u.cache_creation_30m_tokens,0),cache_creation_12h_tokens=GREATEST(u.cache_creation_12h_tokens,0),
   actual_cost=ac,total_cost=tc,invalid_metrics=invalid,billing_type=u.billing_type,observed_at=u.created_at
  WHERE source_log_id=u.id AND api_key_id=u.api_key_id AND request_key=k AND user_id=u.user_id
  AND (input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,cache_creation_30m_tokens,cache_creation_12h_tokens,actual_cost,total_cost,invalid_metrics,billing_type,observed_at)
   IS DISTINCT FROM (GREATEST(u.input_tokens,0)::bigint,GREATEST(u.output_tokens,0)::bigint,GREATEST(u.cache_creation_tokens,0)::bigint,GREATEST(u.cache_read_tokens,0)::bigint,GREATEST(u.cache_creation_30m_tokens,0)::bigint,GREATEST(u.cache_creation_12h_tokens,0)::bigint,ac,tc,invalid,u.billing_type,u.created_at);
 END IF;
END;
$function$;

CREATE OR REPLACE FUNCTION public.kitty_capture_procurement(u usage_logs)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE k TEXT; existing kitty_procurement_receipts%ROWTYPE; r kitty_supplier_cost_rules%ROWTYPE;
 w kitty_supplier_wallets%ROWTYPE; s kitty_suppliers%ROWTYPE;
 binding_id BIGINT; credit_rate_id BIGINT; rule_json JSONB:='{}'::jsonb; m JSONB; cr NUMERIC; ec NUMERIC; val NUMERIC;
BEGIN
 k:=CASE WHEN COALESCE(u.request_id,'')<>'' THEN 'request:'||u.request_id ELSE 'legacy-row:'||u.id::text END;
 m:=jsonb_build_object('input',u.input_tokens,'output',u.output_tokens,'cache_read',u.cache_read_tokens,'cache_write',u.cache_creation_tokens,
  'cache_write_30m',u.cache_creation_30m_tokens,'cache_write_12h',u.cache_creation_12h_tokens,
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
$function$;

CREATE OR REPLACE FUNCTION public.kitty_permanent_usage_capture_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
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
$function$;

CREATE OR REPLACE FUNCTION public.kitty_procurement_capture_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
 PERFORM kitty_capture_procurement(NEW);
 RETURN NEW;
END;
$function$;

CREATE OR REPLACE FUNCTION public.kitty_procurement_cost(rule jsonb, m jsonb, credit_rate numeric)
 RETURNS numeric
 LANGUAGE plpgsql
 IMMUTABLE
AS $function$
DECLARE amount NUMERIC; method TEXT:=rule->>'method';
 cache_write NUMERIC; cache_30m NUMERIC; cache_12h NUMERIC; cache_amount NUMERIC;
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
   -- Split only complete, non-negative integral upstream measurements. Missing
   -- or inconsistent TTL details keep the historic aggregate cost unchanged.
   cache_write:=(m->>'cache_write')::numeric;
   cache_30m:=(m->>'cache_write_30m')::numeric;
   cache_12h:=(m->>'cache_write_12h')::numeric;
   cache_amount:=cache_write*(rule->>'cache_write_price')::numeric;
   IF cache_30m>=0 AND cache_12h>=0 AND cache_30m=trunc(cache_30m) AND cache_12h=trunc(cache_12h)
    AND cache_30m+cache_12h=cache_write AND (rule->>'cache_write_12h_price') IS NOT NULL THEN
    cache_amount:=cache_30m*(rule->>'cache_write_price')::numeric+cache_12h*(rule->>'cache_write_12h_price')::numeric;
   END IF;
   IF (m->>'images')::bigint>0 OR (m->>'videos')::bigint>0 THEN RETURN NULL; END IF;
   amount:=((m->>'input')::numeric*(rule->>'input_price')::numeric+
    (m->>'output')::numeric*(rule->>'output_price')::numeric+
    (m->>'cache_read')::numeric*(rule->>'cache_read_price')::numeric+
    cache_amount)/1000000;
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
$function$;

CREATE OR REPLACE FUNCTION public.kitty_supplier_credit_rate_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
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
$function$;

CREATE OR REPLACE FUNCTION public.protect_unsettled_video_identity()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
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
$function$;

CREATE OR REPLACE FUNCTION public.reject_usage_billing_event_identity_mutation()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF OLD.request_id IS DISTINCT FROM NEW.request_id
       OR OLD.api_key_id IS DISTINCT FROM NEW.api_key_id
       OR OLD.request_fingerprint IS DISTINCT FROM NEW.request_fingerprint
       OR OLD.schema_version IS DISTINCT FROM NEW.schema_version
       OR OLD.payload IS DISTINCT FROM NEW.payload
       OR OLD.observed_at IS DISTINCT FROM NEW.observed_at THEN
        RAISE EXCEPTION 'usage billing event identity is immutable';
    END IF;
    RETURN NEW;
END;
$function$;

CREATE OR REPLACE FUNCTION public.validate_video_task_identities()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
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
$function$;

SET LOCAL check_function_bodies = true;

DROP TRIGGER IF EXISTS new_api_account_binding_identity_guard ON public.accounts;

CREATE TRIGGER new_api_account_binding_identity_guard BEFORE UPDATE ON public.accounts FOR EACH ROW EXECUTE FUNCTION invalidate_new_api_account_binding();

DROP TRIGGER IF EXISTS trg_accounts_unsettled_video_identity ON public.accounts;

CREATE TRIGGER trg_accounts_unsettled_video_identity BEFORE DELETE OR UPDATE OF id, deleted_at, platform, type, credentials ON public.accounts FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

DROP TRIGGER IF EXISTS trg_api_keys_unsettled_video_identity ON public.api_keys;

CREATE TRIGGER trg_api_keys_unsettled_video_identity BEFORE DELETE OR UPDATE OF id, deleted_at, user_id, group_id ON public.api_keys FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

DROP TRIGGER IF EXISTS trg_groups_unsettled_video_identity ON public.groups;

CREATE TRIGGER trg_groups_unsettled_video_identity BEFORE DELETE OR UPDATE OF id, deleted_at, platform, subscription_type ON public.groups FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

DROP TRIGGER IF EXISTS trg_subscriptions_unsettled_video_identity ON public.user_subscriptions;

CREATE TRIGGER trg_subscriptions_unsettled_video_identity BEFORE DELETE OR UPDATE OF id, deleted_at, user_id, group_id ON public.user_subscriptions FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

DROP TRIGGER IF EXISTS trg_users_unsettled_video_identity ON public.users;

CREATE TRIGGER trg_users_unsettled_video_identity BEFORE DELETE OR UPDATE OF id, deleted_at ON public.users FOR EACH ROW EXECUTE FUNCTION protect_unsettled_video_identity();

DROP TRIGGER IF EXISTS trg_video_tasks_identity_insert ON public.video_tasks;

CREATE TRIGGER trg_video_tasks_identity_insert BEFORE INSERT ON public.video_tasks FOR EACH ROW EXECUTE FUNCTION validate_video_task_identities();

DROP TRIGGER IF EXISTS trg_video_tasks_identity_update ON public.video_tasks;

CREATE TRIGGER trg_video_tasks_identity_update BEFORE UPDATE OF id, user_id, group_id, account_id, api_key_id, subscription_id ON public.video_tasks FOR EACH ROW EXECUTE FUNCTION validate_video_task_identities();

DROP TRIGGER IF EXISTS usage_billing_event_identity_immutable ON public.usage_billing_events;

CREATE TRIGGER usage_billing_event_identity_immutable BEFORE UPDATE ON public.usage_billing_events FOR EACH ROW EXECUTE FUNCTION reject_usage_billing_event_identity_mutation();

DROP TRIGGER IF EXISTS kitty_permanent_usage_delete ON public.usage_logs;

CREATE TRIGGER kitty_permanent_usage_delete BEFORE DELETE ON public.usage_logs FOR EACH ROW EXECUTE FUNCTION kitty_permanent_usage_capture_trigger();

DROP TRIGGER IF EXISTS kitty_supplier_credit_rate_write ON public.kitty_supplier_purchases;

CREATE TRIGGER kitty_supplier_credit_rate_write AFTER INSERT OR UPDATE OF voided_at ON public.kitty_supplier_purchases FOR EACH ROW EXECUTE FUNCTION kitty_supplier_credit_rate_trigger();

DROP TRIGGER IF EXISTS kitty_permanent_usage_write ON public.usage_logs;

CREATE TRIGGER kitty_permanent_usage_write AFTER INSERT OR UPDATE OF input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cache_creation_30m_tokens, cache_creation_12h_tokens, actual_cost, total_cost, billing_type, created_at ON public.usage_logs FOR EACH ROW EXECUTE FUNCTION kitty_permanent_usage_capture_trigger();

DROP TRIGGER IF EXISTS kitty_procurement_write ON public.usage_logs;

CREATE TRIGGER kitty_procurement_write AFTER INSERT OR UPDATE OF input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, cache_creation_30m_tokens, cache_creation_12h_tokens, total_cost, actual_cost, account_stats_cost, account_rate_multiplier, image_count, video_count, video_duration_seconds, upstream_request_id ON public.usage_logs FOR EACH ROW EXECUTE FUNCTION kitty_procurement_capture_trigger();

CREATE OR REPLACE VIEW public.qq_bot_usage_metrics AS
 SELECT user_id,
    input_tokens,
    output_tokens,
    cache_creation_tokens,
    cache_read_tokens,
    actual_cost,
    billing_type,
    observed_at AS created_at
   FROM public.kitty_permanent_usage
  WHERE (NOT invalid_metrics);

CREATE OR REPLACE VIEW public.usage_platform_quota_effective AS
 SELECT q.id,
    q.user_id,
    q.platform,
    q.daily_limit_usd,
    q.weekly_limit_usd,
    q.monthly_limit_usd,
    q.deleted_at,
        CASE
            WHEN (p.quota_id IS NULL) THEN q.daily_usage_usd
            ELSE (
            CASE
                WHEN ((COALESCE(q.daily_window_start, p.legacy_daily_anchor) IS NULL) OR (COALESCE(q.daily_window_start, p.legacy_daily_anchor) >= p.daily_window_start)) THEN q.daily_usage_usd
                ELSE (0)::numeric
            END +
            CASE
                WHEN ((COALESCE(q.daily_window_start, p.legacy_daily_anchor) IS NULL) OR (p.daily_window_start >= COALESCE(q.daily_window_start, p.legacy_daily_anchor))) THEN p.daily_usage_usd
                ELSE (0)::numeric
            END)
        END AS daily_usage_usd,
        CASE
            WHEN (p.quota_id IS NULL) THEN q.weekly_usage_usd
            ELSE (
            CASE
                WHEN ((COALESCE(q.weekly_window_start, p.legacy_weekly_anchor) IS NULL) OR (COALESCE(q.weekly_window_start, p.legacy_weekly_anchor) >= p.weekly_window_start)) THEN q.weekly_usage_usd
                ELSE (0)::numeric
            END +
            CASE
                WHEN ((COALESCE(q.weekly_window_start, p.legacy_weekly_anchor) IS NULL) OR (p.weekly_window_start >= COALESCE(q.weekly_window_start, p.legacy_weekly_anchor))) THEN p.weekly_usage_usd
                ELSE (0)::numeric
            END)
        END AS weekly_usage_usd,
        CASE
            WHEN (p.quota_id IS NULL) THEN q.monthly_usage_usd
            ELSE (
            CASE
                WHEN ((COALESCE(q.monthly_window_start, p.legacy_monthly_anchor) IS NULL) OR (COALESCE(q.monthly_window_start, p.legacy_monthly_anchor) >= p.monthly_window_start)) THEN q.monthly_usage_usd
                ELSE (0)::numeric
            END +
            CASE
                WHEN ((COALESCE(q.monthly_window_start, p.legacy_monthly_anchor) IS NULL) OR (p.monthly_window_start >= COALESCE(q.monthly_window_start, p.legacy_monthly_anchor))) THEN p.monthly_usage_usd
                ELSE (0)::numeric
            END)
        END AS monthly_usage_usd,
    GREATEST(COALESCE(q.daily_window_start, p.legacy_daily_anchor), p.daily_window_start) AS daily_window_start,
    GREATEST(COALESCE(q.weekly_window_start, p.legacy_weekly_anchor), p.weekly_window_start) AS weekly_window_start,
    GREATEST(COALESCE(q.monthly_window_start, p.legacy_monthly_anchor), p.monthly_window_start) AS monthly_window_start
   FROM (public.user_platform_quotas q
     LEFT JOIN public.usage_platform_quota_totals p ON ((p.quota_id = q.id)));

INSERT INTO kitty_permanent_usage_state(singleton,baseline_highwater) SELECT TRUE,COALESCE(max(id),0) FROM usage_logs ON CONFLICT DO NOTHING;

INSERT INTO kitty_provider_operations_state(singleton) VALUES(TRUE) ON CONFLICT DO NOTHING;

INSERT INTO channel_monitor_v3_config(id) VALUES(1) ON CONFLICT DO NOTHING;
