//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// The explicit DSN points at a disposable database. This test never uses the
// application database configuration and owns only a uniquely named schema.
func TestChannelMonitorV2OutputTPSPostgres(t *testing.T) {
	dsn := os.Getenv("MONITOR_TPS_TEST_DSN")
	if dsn == "" {
		t.Skip("MONITOR_TPS_TEST_DSN is required for real PostgreSQL verification")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("monitor_tps_%d", time.Now().UnixNano())
	_, err = db.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	defer db.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
	_, err = db.Exec("SET search_path TO " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	tx, err := db.Begin()
	require.NoError(t, err)
	for _, file := range []string{"194_channel_monitor_v2.sql", "199_channel_monitor_v2_fixed_rollups.sql", "334_kitty_monitor_output_tps.sql"} {
		raw, e := migrations.FS.ReadFile(file)
		require.NoError(t, e)
		_, e = tx.Exec(string(raw))
		require.NoError(t, e, file)
	}
	require.NoError(t, tx.Commit())
	_, err = db.Exec(`CREATE TABLE groups(id bigint primary key, name text, platform text, deleted_at timestamptz);
CREATE TABLE accounts(id bigint primary key, platform text);
CREATE TABLE usage_logs(id bigserial primary key, request_id text, created_at timestamptz, group_id bigint, account_id bigint, user_id bigint, model text, requested_model text, request_type int, actual_cost numeric, input_tokens bigint default 0, output_tokens bigint default 0, cache_creation_tokens bigint default 0, cache_read_tokens bigint default 0, first_token_ms bigint, duration_ms bigint, stream boolean);
INSERT INTO groups(id,name,platform) VALUES(7,'Synthetic group','openai');
INSERT INTO accounts(id,platform) VALUES(2,'openai');`)
	require.NoError(t, err)
	start := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	epoch := start.Add(30 * time.Second)
	_, err = db.Exec(`UPDATE channel_monitor_v2_output_tps_epoch SET started_at=$1`, epoch)
	require.NoError(t, err)
	type sample struct {
		model           string
		when            time.Time
		stream          bool
		output          int
		first, duration any
		cost            int
		kind            int
	}
	now := epoch.Add(time.Second)
	samples := []sample{
		{"current", now, true, 100, 1000, 2000, 1, 0},
		{"current", now, true, 400, 2000, 12000, 1, 0},
		{"current", now, false, 99999, 100, 1000, 1, 0},
		{"current", now, true, 99999, nil, 1000, 1, 0},
		{"current", now, true, 99999, 2000, 1000, 1, 0},
		{"current", now, true, 0, 100, 1000, 1, 0},
		{"current", now, true, 99999, 100, 1000, 0, 0},
		{"current", start, true, 99999, 100, 1000, 1, 0},
		{"current", now, true, 99999, 100, 1000, 1, 4},
		{"current", now, true, 99999, 100, 1000, 1, 6},
		{"historical", start, true, 5000, 100, 1000, 1, 0},
	}
	for i, s := range samples {
		_, err = db.Exec(`INSERT INTO usage_logs(request_id,created_at,group_id,account_id,user_id,model,requested_model,request_type,actual_cost,output_tokens,first_token_ms,duration_ms,stream) VALUES($1,$2,7,2,3,$3,$3,$4,$5,$6,$7,$8,$9)`, fmt.Sprintf("synthetic-%d", i), s.when, s.model, s.kind, s.cost, s.output, s.first, s.duration, s.stream)
		require.NoError(t, err)
	}
	end := start.Add(2 * time.Minute)
	for _, q := range []string{channelMonitorV2UsageMetricsSQL, channelMonitorV2UserMetricsSQL} {
		_, err = db.Exec(fmt.Sprintf(q, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end)
		require.NoError(t, err)
	}
	for _, q := range []string{channelMonitorV2MetricsRollupSQL, channelMonitorV2UserMetricsRollupSQL} {
		_, err = db.Exec(q, "300 seconds", 300, start, end)
		require.NoError(t, err)
	}
	for _, table := range []string{"channel_monitor_v2_metrics_1m", "channel_monitor_v2_user_metrics_1m", "channel_monitor_v2_metrics_rollup", "channel_monitor_v2_user_metrics_rollup"} {
		var tokens, ms, count int64
		require.NoError(t, db.QueryRow("SELECT SUM(output_speed_tokens),SUM(output_speed_ms),SUM(output_speed_samples) FROM "+table+" WHERE model='current'").Scan(&tokens, &ms, &count))
		require.Equal(t, int64(500), tokens, table)
		require.Equal(t, int64(11000), ms, table)
		require.Equal(t, int64(2), count, table)
		require.NoError(t, db.QueryRow("SELECT SUM(output_speed_samples) FROM "+table+" WHERE model='historical'").Scan(&count))
		require.Zero(t, count, table)
	}
	cfg := service.ChannelMonitorV2Config{Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: true}}}
	repo := &channelMonitorV2Repository{db: db}
	facts, err := repo.loadFacts(context.Background(), service.ChannelMonitorV2Filter{Start: start, End: end}, cfg, false)
	require.NoError(t, err)
	require.Len(t, facts, 2)
	for _, f := range facts {
		a := newMetricAccumulator()
		a.addFact(f)
		m := a.metric(2, true)
		if f.Model == "historical" {
			require.Nil(t, m.OutputTPS)
		} else {
			require.NotNil(t, m.OutputTPS)
			require.InDelta(t, 500.0/11, *m.OutputTPS, .000001)
		}
	}
}
