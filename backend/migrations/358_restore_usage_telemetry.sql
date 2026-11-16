-- Restore retained UI telemetry. Additive and compatible with kitty.6 rollback.

-- Additive diagnostics storage; old binaries do not read or write this table.
-- No FK: usage_logs can be partitioned; retention also removes orphaned details.
CREATE TABLE IF NOT EXISTS request_timing_details (
    usage_log_id BIGINT NOT NULL,
    trace_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    detail JSONB NOT NULL,
    PRIMARY KEY (usage_log_id, trace_id)
);
CREATE INDEX IF NOT EXISTS request_timing_details_created_at_idx ON request_timing_details(created_at);

-- Diagnostic-only HTTP lifecycle records, including requests without usage.
-- No FK: retention is independent of billing and partitioned usage_logs.
CREATE TABLE IF NOT EXISTS request_connection_traces (
 trace_id UUID PRIMARY KEY,
 source VARCHAR(32) NOT NULL DEFAULT 'application',
 host VARCHAR(255) NOT NULL DEFAULT '',
 scheme VARCHAR(8) NOT NULL DEFAULT '',
 method VARCHAR(16) NOT NULL DEFAULT '',
 http_version VARCHAR(16) NOT NULL DEFAULT '',
 tls_version VARCHAR(32) NOT NULL DEFAULT '',
 upstream_status VARCHAR(64) NOT NULL DEFAULT '',
 network_error VARCHAR(64) NOT NULL DEFAULT '',

 started_at TIMESTAMPTZ NOT NULL,
 request_id VARCHAR(128) NOT NULL,
 client_request_id VARCHAR(128) NOT NULL,
 user_id BIGINT NOT NULL,
 api_key_id BIGINT NOT NULL,
 account_id BIGINT NOT NULL DEFAULT 0,
 group_id BIGINT NOT NULL DEFAULT 0,
 model VARCHAR(256) NOT NULL DEFAULT '',
 path VARCHAR(128) NOT NULL,
 client_ip VARCHAR(64) NOT NULL DEFAULT '',
 user_agent VARCHAR(512) NOT NULL DEFAULT '',
 stream BOOLEAN NOT NULL,
 detail JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS request_connection_traces_time_idx ON request_connection_traces(started_at DESC, trace_id);
CREATE INDEX IF NOT EXISTS request_connection_traces_user_time_idx ON request_connection_traces(user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS request_connection_traces_key_time_idx ON request_connection_traces(api_key_id, started_at DESC);
CREATE INDEX IF NOT EXISTS request_connection_traces_request_idx ON request_connection_traces(request_id);
CREATE INDEX IF NOT EXISTS request_connection_traces_client_request_idx ON request_connection_traces(client_request_id);
CREATE INDEX IF NOT EXISTS request_connection_traces_host_time_idx ON request_connection_traces(host, started_at DESC);

-- Application payload bytes, independent of token billing and diagnostic retention.
-- No cascading foreign keys: deleting a key must not erase historical metering.
CREATE TABLE IF NOT EXISTS user_traffic_events (
    event_id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    request_id VARCHAR(64) NOT NULL DEFAULT '',
    endpoint VARCHAR(256) NOT NULL,
    method VARCHAR(16) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    upload_bytes BIGINT NOT NULL CHECK (upload_bytes >= 0),
    download_bytes BIGINT NOT NULL CHECK (download_bytes >= 0),
    status_code INTEGER NOT NULL,
    outcome VARCHAR(32) NOT NULL,
    request_complete BOOLEAN NOT NULL,
    measurement_version SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS user_traffic_events_user_time_idx
    ON user_traffic_events (user_id, started_at DESC, event_id);
CREATE INDEX IF NOT EXISTS user_traffic_events_key_time_idx
    ON user_traffic_events (api_key_id, started_at DESC);
CREATE INDEX IF NOT EXISTS user_traffic_events_time_idx
    ON user_traffic_events (started_at DESC, event_id);

CREATE TABLE IF NOT EXISTS user_traffic_hourly (
    bucket_start TIMESTAMPTZ NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    upload_bytes BIGINT NOT NULL DEFAULT 0 CHECK (upload_bytes >= 0),
    download_bytes BIGINT NOT NULL DEFAULT 0 CHECK (download_bytes >= 0),
    requests BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, api_key_id, group_id, bucket_start)
);
CREATE INDEX IF NOT EXISTS user_traffic_hourly_time_idx ON user_traffic_hourly (bucket_start);
COMMENT ON TABLE user_traffic_events IS 'HTTP/SSE application read/write payload bytes; not wire traffic or proof of client receipt. Version 1 attributes a request to its start time.';

-- Match descending keyset pagination, including tied start timestamps.
-- Retain existing indexes for older binaries. Remove redundant ones only after
-- deployment-specific EXPLAIN/usage measurements and the compatibility window.
CREATE INDEX IF NOT EXISTS user_traffic_events_user_cursor_idx
    ON user_traffic_events (user_id, started_at DESC, event_id DESC);
CREATE INDEX IF NOT EXISTS user_traffic_events_key_cursor_idx
    ON user_traffic_events (api_key_id, started_at DESC, event_id DESC);
CREATE INDEX IF NOT EXISTS user_traffic_events_cursor_idx
    ON user_traffic_events (started_at DESC, event_id DESC);
