//go:build ttl_postgres

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This test accepts only its explicitly named synthetic database. It never
// connects to a production database, including for schema preparation.
func TestUsageLogTTLPostgres(t *testing.T) {
	dsn := os.Getenv("KITTY_TTL_TEST_DSN")
	if dsn == "" {
		t.Skip("set KITTY_TTL_TEST_DSN for the isolated PostgreSQL test")
	}
	require.Contains(t, dsn, "dbname=kitty_absorb_ttl_tests ")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	var name string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name))
	require.Equal(t, "kitty_absorb_ttl_tests", name)
	require.NoError(t, ApplyMigrations(ctx, db))
	var userID, keyID, accountID int64
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO users(email,password_hash,balance) VALUES('ttl@example.invalid','synthetic',100) RETURNING id").Scan(&userID))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,'synthetic-ttl-key','ttl') RETURNING id", userID).Scan(&keyID))
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO accounts(name,platform,type) VALUES('synthetic-ttl','openai','apikey') RETURNING id").Scan(&accountID))
	repo := newUsageLogRepositoryWithSQL(nil, db)
	newLog := func(id string) *service.UsageLog {
		return &service.UsageLog{UserID: userID, APIKeyID: keyID, AccountID: accountID, RequestID: id, Model: "synthetic-ttl", InputTokens: 3, OutputTokens: 5,
			CacheCreationTokens: 2399, CacheCreation30mTokens: 9, CacheCreation12hTokens: 2390, TotalCost: 0.02, ActualCost: 0.01, RateMultiplier: 0.5, CreatedAt: time.Now().UTC()}
	}
	// Transaction context exercises the single-row path, while concurrent calls
	// exercise the production batch queue and all positional INSERT fields.
	t.Run("single", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		log := newLog("ttl-single")
		inserted, err := newUsageLogRepositoryWithSQL(nil, tx).Create(ctx, log)
		require.NoError(t, err)
		require.True(t, inserted)
		var short, long int
		require.NoError(t, tx.QueryRowContext(ctx, "SELECT cache_creation_30m_tokens,cache_creation_12h_tokens FROM usage_logs WHERE id=$1", log.ID).Scan(&short, &long))
		require.Equal(t, 9, short)
		require.Equal(t, 2390, long)
		require.NoError(t, tx.Commit())
	})
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			log := newLog(fmt.Sprintf("ttl-batch-%d", i))
			inserted, err := repo.Create(ctx, log)
			if err == nil && !inserted { err = fmt.Errorf("initial insert was deduplicated") }
			if err == nil {
				var got *service.UsageLog
				got, err = repo.GetByID(ctx, log.ID)
				if err == nil && (got.CacheCreation30mTokens != 9 || got.CacheCreation12hTokens != 2390) { err = fmt.Errorf("TTL did not roundtrip") }
			}
			errs <- err
		}(i)
	}
	wg.Wait(); close(errs)
	for err := range errs { require.NoError(t, err) }
	// Request replay cannot create a second permanent receipt or change amounts.
	duplicate := newLog("ttl-batch-0")
	inserted, err := repo.Create(ctx, duplicate)
	require.NoError(t, err); require.False(t, inserted)
	var rows, short, long int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*),sum(cache_creation_30m_tokens),sum(cache_creation_12h_tokens) FROM kitty_permanent_usage WHERE user_id=$1", userID).Scan(&rows, &short, &long))
	require.Equal(t, 17, rows); require.Equal(t, 17*9, short); require.Equal(t, 17*2390, long)
	// Correcting only a TTL bucket updates its permanent receipt exactly once.
	_, err = db.ExecContext(ctx, "UPDATE usage_logs SET cache_creation_30m_tokens=10,cache_creation_12h_tokens=2389 WHERE request_id='ttl-single'")
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT cache_creation_30m_tokens,cache_creation_12h_tokens FROM kitty_permanent_usage WHERE request_key='request:ttl-single'").Scan(&short, &long))
	require.Equal(t, 10, short); require.Equal(t, 2389, long)
	var balance string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT balance::text FROM users WHERE id=$1", userID).Scan(&balance))
	require.Equal(t, "100.00000000", strings.TrimSpace(balance), "recording metadata must not move money")
}
