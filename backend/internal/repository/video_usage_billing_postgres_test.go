//go:build video_postgres

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This opt-in suite uses a disposable schema and a dedicated local PostgreSQL
// connection. It does not depend on the Docker integration harness or public
// application tables. It exercises actual transaction and unique-index locks.
func newVideoUsagePostgresFixture(t *testing.T) (*sql.DB, *usageBillingRepository) {
	t.Helper()
	dsn := os.Getenv("VIDEO_USAGE_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "set VIDEO_USAGE_TEST_DATABASE_URL for -tags video_postgres")
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	schema := fmt.Sprintf("video_usage_test_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema+",pg_catalog")
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		_, cleanupErr := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close())
	})
	_, err = db.Exec(videoUsagePostgresSchema)
	require.NoError(t, err)
	for _, migration := range []string{"071_add_usage_billing_dedup.sql", "073_add_usage_billing_dedup_archive.sql"} {
		ddl, readErr := os.ReadFile("../../migrations/" + migration)
		require.NoError(t, readErr)
		_, err = db.Exec(string(ddl))
		require.NoError(t, err)
	}
	return db, &usageBillingRepository{db: db}
}

// Keep the usage receipt shape explicit so a new required insert field causes
// this fixture to fail instead of silently deriving a schema from the query.
const videoUsagePostgresSchema = `
CREATE TABLE users (id BIGINT PRIMARY KEY, balance NUMERIC(20,10) NOT NULL,
 updated_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ);
INSERT INTO users (id,balance) VALUES (1,10);
CREATE TABLE api_keys (id BIGINT PRIMARY KEY, quota NUMERIC(20,10) NOT NULL,
 quota_used NUMERIC(20,10) NOT NULL, status TEXT NOT NULL,
 updated_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ);
INSERT INTO api_keys (id,quota,quota_used,status) VALUES (166,100,0,'active');
CREATE TABLE usage_logs (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, api_key_id BIGINT NOT NULL,
 account_id BIGINT NOT NULL, request_id TEXT NOT NULL, model TEXT NOT NULL,
 requested_model TEXT, upstream_model TEXT, upstream_response_model TEXT,
 upstream_model_mismatch BOOLEAN, group_id BIGINT, subscription_id BIGINT,
 input_tokens INTEGER, output_tokens INTEGER, cache_creation_tokens INTEGER,
 cache_read_tokens INTEGER, cache_creation_5m_tokens INTEGER, cache_creation_1h_tokens INTEGER,
 image_output_tokens INTEGER, image_output_cost NUMERIC(20,10),
 image_input_tokens INTEGER, image_input_cost NUMERIC(20,10),
 input_cost NUMERIC(20,10), output_cost NUMERIC(20,10), cache_creation_cost NUMERIC(20,10),
 cache_read_cost NUMERIC(20,10), total_cost NUMERIC(20,10) NOT NULL,
 actual_cost NUMERIC(20,10) NOT NULL, rate_multiplier NUMERIC(20,10),
 account_rate_multiplier NUMERIC(20,10), billing_type SMALLINT, request_type SMALLINT,
 stream BOOLEAN, openai_ws_mode BOOLEAN, duration_ms INTEGER, first_token_ms INTEGER,
 user_agent TEXT, ip_address TEXT, image_count INTEGER, image_size TEXT,
 image_input_size TEXT, image_output_size TEXT, image_size_source TEXT,
 image_size_breakdown JSONB, video_count INTEGER, video_resolution TEXT,
 video_duration_seconds INTEGER, service_tier TEXT, reasoning_effort TEXT,
 requested_reasoning_effort TEXT, inbound_endpoint TEXT, upstream_endpoint TEXT,
 cache_ttl_overridden BOOLEAN, long_context_billing_applied BOOLEAN, channel_id BIGINT,
 model_mapping_chain TEXT, billing_tier TEXT, billing_mode TEXT,
 account_stats_cost NUMERIC(20,10), upstream_request_id TEXT, session_id TEXT,
 usage_audit JSONB, native_compaction_v2 BOOLEAN, created_at TIMESTAMPTZ NOT NULL,
 UNIQUE (request_id, api_key_id));`

func videoPostgresCommand() (*service.UsageBillingCommand, *service.UsageLog) {
	cmd, receipt := videoBillingReceiptFixture()
	cmd.APIKeyQuotaCost = cmd.BalanceCost
	cmd.RequestFingerprint = ""
	cmd.Normalize()
	return cmd, receipt
}

func assertVideoPostgresState(t *testing.T, db *sql.DB, balance, quota float64, dedup, receipt int) {
	t.Helper()
	var gotBalance, gotQuota float64
	var gotDedup, gotReceipt int
	require.NoError(t, db.QueryRow(`SELECT balance FROM users WHERE id=1`).Scan(&gotBalance))
	require.NoError(t, db.QueryRow(`SELECT quota_used FROM api_keys WHERE id=166`).Scan(&gotQuota))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_billing_dedup`).Scan(&gotDedup))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&gotReceipt))
	require.Equal(t, balance, gotBalance)
	require.Equal(t, quota, gotQuota)
	require.Equal(t, dedup, gotDedup)
	require.Equal(t, receipt, gotReceipt)
	t.Logf("balance=%.2f quota_used=%.2f hot_dedup=%d receipts=%d", gotBalance, gotQuota, gotDedup, gotReceipt)
}

func TestVideoUsagePostgresConcurrentSettlementChargesOnce(t *testing.T) {
	db, repo := newVideoUsagePostgresFixture(t)
	const attempts = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan bool, attempts)
	errors := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cmd, receipt := videoPostgresCommand()
			result, err := repo.ApplyVideoUsage(context.Background(), cmd, receipt)
			if err != nil {
				errors <- err
				return
			}
			results <- result.Applied
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	applied := 0
	for result := range results {
		if result {
			applied++
		}
	}
	require.Equal(t, 1, applied)
	assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
	t.Logf("simultaneous_attempts=%d applied=%d", attempts, applied)
}

func TestVideoUsagePostgresReceiptFailureRollsBackThenRetries(t *testing.T) {
	db, repo := newVideoUsagePostgresFixture(t)
	_, err := db.Exec(`ALTER TABLE usage_logs ADD CONSTRAINT fixture_receipt_failure CHECK (actual_cost=0)`)
	require.NoError(t, err)
	cmd, receipt := videoPostgresCommand()
	result, err := repo.ApplyVideoUsage(context.Background(), cmd, receipt)
	require.ErrorContains(t, err, "fixture_receipt_failure")
	require.Nil(t, result)
	assertVideoPostgresState(t, db, 10, 0, 0, 0)
	_, err = db.Exec(`ALTER TABLE usage_logs DROP CONSTRAINT fixture_receipt_failure`)
	require.NoError(t, err)
	cmd, receipt = videoPostgresCommand()
	result, err = repo.ApplyVideoUsage(context.Background(), cmd, receipt)
	require.NoError(t, err)
	require.True(t, result.Applied)
	assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
}

func TestVideoUsagePostgresRepairsHotAndArchivedMissingReceipts(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprintf("archived_%v", archived), func(t *testing.T) {
			db, repo := newVideoUsagePostgresFixture(t)
			cmd, _ := videoPostgresCommand()
			chargedAt := time.Date(2026, 9, 20, 14, 38, 31, 0, time.UTC)
			table := "usage_billing_dedup"
			if archived {
				table += "_archive"
			}
			_, err := db.Exec(`INSERT INTO `+table+` (request_id,api_key_id,request_fingerprint,created_at) VALUES ($1,$2,$3,$4)`,
				cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint, chargedAt)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE users SET balance=6.25; UPDATE api_keys SET quota_used=3.75`)
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				cmd, receipt := videoPostgresCommand()
				result, applyErr := repo.ApplyVideoUsage(context.Background(), cmd, receipt)
				require.NoError(t, applyErr)
				require.False(t, result.Applied)
				require.True(t, chargedAt.Equal(receipt.CreatedAt))
			}
			var persisted time.Time
			require.NoError(t, db.QueryRow(`SELECT created_at FROM usage_logs`).Scan(&persisted))
			require.True(t, chargedAt.Equal(persisted))
			assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
			t.Logf("original_charge_timestamp=%s repaired_timestamp=%s", chargedAt.Format(time.RFC3339), persisted.Format(time.RFC3339))
		})
	}
}

func TestVideoUsagePostgresFailedPlaceholderChargesAndRepairsOnce(t *testing.T) {
	db, repo := newVideoUsagePostgresFixture(t)
	_, placeholder := videoPostgresCommand()
	placeholder.ActualCost = 0
	placeholder.CreatedAt = time.Now().Add(-time.Hour)
	inserted, err := (&usageLogRepository{sql: db}).createSingle(context.Background(), db, placeholder)
	require.NoError(t, err)
	require.True(t, inserted)
	for i := 0; i < 2; i++ {
		cmd, receipt := videoPostgresCommand()
		result, applyErr := repo.ApplyVideoUsage(context.Background(), cmd, receipt)
		require.NoError(t, applyErr)
		require.Equal(t, i == 0, result.Applied)
		require.True(t, receipt.CreatedAt.After(placeholder.CreatedAt))
	}
	var actual float64
	require.NoError(t, db.QueryRow(`SELECT actual_cost FROM usage_logs`).Scan(&actual))
	require.Equal(t, 3.75, actual)
	assertVideoPostgresState(t, db, 6.25, 3.75, 1, 1)
}

func TestVideoUsagePostgresHistoricalFingerprintConflictPreservesState(t *testing.T) {
	db, repo := newVideoUsagePostgresFixture(t)
	cmd, receipt := videoPostgresCommand()
	_, err := db.Exec(`INSERT INTO usage_billing_dedup_archive (request_id,api_key_id,request_fingerprint,created_at) VALUES ($1,$2,'different-original-price',NOW())`, cmd.RequestID, cmd.APIKeyID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users SET balance=6.25; UPDATE api_keys SET quota_used=3.75`)
	require.NoError(t, err)
	_, err = repo.ApplyVideoUsage(context.Background(), cmd, receipt)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	assertVideoPostgresState(t, db, 6.25, 3.75, 0, 0)
}
