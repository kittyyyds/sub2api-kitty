//go:build bonus_postgres

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// New installs omit retired data actions. Existing ledgers retain their actual
// historical execution record without forging, deleting, or rewriting any row.
func TestMergedPostgresFullMigrationLedger(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing_ledger_%t", existing), func(t *testing.T) {
			testMergedPostgresMigrationLedger(t, existing)
		})
	}
}

func testMergedPostgresMigrationLedger(t *testing.T, existing bool) {
	parsed, err := url.Parse(os.Getenv("RECHARGE_BONUS_TEST_DATABASE_URL"))
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	admin, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	defer func() { require.NoError(t, admin.Close()) }()
	name := fmt.Sprintf("kitty_merge_migrations_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name))
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
		_, err := admin.Exec("DROP DATABASE " + pq.QuoteIdentifier(name))
		require.NoError(t, err)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	files, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	const historicalName = "188_enable_grok_enhancement_group.sql"
	if existing {
		// Reproduce genuine historical execution using immutable original SQL,
		// then upgrade. This fixture is never used by new-install startup.
		beforeHistoricalSeed, oldFS := fstest.MapFS{}, fstest.MapFS{}
		for _, name := range files {
			content, err := migrations.FS.ReadFile(name)
			require.NoError(t, err)
			oldFS[name] = &fstest.MapFile{Data: content}
			if name < historicalName {
				beforeHistoricalSeed[name] = &fstest.MapFile{Data: content}
			}
		}
		historical, err := migrations.HistoryFS.ReadFile("history/" + historicalName)
		require.NoError(t, err)
		oldFS[historicalName] = &fstest.MapFile{Data: historical}
		require.NoError(t, applyMigrationsFS(ctx, db, beforeHistoricalSeed))
		_, err = db.ExecContext(ctx, `INSERT INTO groups(name,platform) VALUES ('Grok 增强版','grok')`)
		require.NoError(t, err)
		require.NoError(t, applyMigrationsFS(ctx, db, oldFS))
		// A user's later choice must not be reactivated by the new binary.
		_, err = db.ExecContext(ctx, `UPDATE groups SET grok_prompt_enabled=false WHERE name='Grok 增强版'`)
		require.NoError(t, err)
	}
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, ApplyMigrations(ctx, db), "re-entry must validate without repeating writes")
	require.NoError(t, validateManagedMigrations(ctx, db, migrations.FS))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	wantCount := len(files)
	if existing {
		wantCount++
		var checksum string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename=$1", historicalName).Scan(&checksum))
		require.Equal(t, managedHistoricalMigrations[historicalName], checksum)
		var enabled bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT grok_prompt_enabled FROM groups WHERE name='Grok 增强版'`).Scan(&enabled))
		require.False(t, enabled)
	} else {
		var groupCount int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE name='Grok 增强版'`).Scan(&groupCount))
		require.Zero(t, groupCount, "clean install must not invent a historical campaign group")
	}
	require.Equal(t, wantCount, count)
	for _, name := range files {
		content, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		var checksum string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename=$1", name).Scan(&checksum))
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(string(content))))), checksum, name)
	}
	t.Logf("validated %d active migrations; actual immutable ledger=%d; existing=%t", len(files), count, existing)
}

// Exercise the merged schema with real PostgreSQL: editing legacy rate/RPM
// overrides must preserve a denied-model-only row and the permanent bonus ledger.
func TestMergedPostgresModelRestrictionsPreservePermanentBonus(t *testing.T) {
	db, bonus := newRechargeBonusPostgresFixture(t, true)
	_, err := db.Exec(`CREATE TABLE groups (id BIGINT PRIMARY KEY); INSERT INTO groups VALUES (7)`)
	require.NoError(t, err)
	for _, migration := range []string{
		"047_add_user_group_rate_multipliers.sql",
		"127_add_user_group_rpm_override.sql",
		"245_user_group_denied_models.sql",
	} {
		runRechargeBonusSQL(t, db, "../../migrations/"+migration)
	}
	now := time.Now().UTC()
	grant := grantRechargeBonusTestOrder(t, db, bonus, 1, rechargeBonusTestRule(t, db, 50, 1.99, 0), now)
	require.Nil(t, grant.ExpiresAt)
	rates := NewUserGroupRateRepository(db)
	ctx := context.Background()
	require.NoError(t, rates.SyncGroupDeniedModels(ctx, 7, []service.GroupUserDeniedModelsInput{
		{UserID: 42, DeniedModels: []string{"gpt-6-luna", "gpt-6-*"}},
	}))
	// Every legacy cleanup path previously deleted rows whose only remaining
	// setting was denied_models. Keep all of them under the same real DB check.
	require.NoError(t, rates.SyncGroupRateMultipliers(ctx, 7, nil))
	require.NoError(t, rates.SyncUserGroupRates(ctx, 42, map[int64]*float64{}))
	require.NoError(t, rates.SyncUserGroupRates(ctx, 42, map[int64]*float64{7: nil}))
	require.NoError(t, rates.SyncGroupRPMOverrides(ctx, 7, nil))
	require.NoError(t, rates.SyncGroupRPMOverrides(ctx, 7, []service.GroupRPMOverrideInput{{UserID: 42}}))
	require.NoError(t, rates.ClearGroupRPMOverrides(ctx, 7))
	denied, err := rates.GetDeniedModelsByUserAndGroup(ctx, 42, 7)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-6-luna", "gpt-6-*"}, denied)

	rate, rpm := 0.5, 30
	require.NoError(t, rates.SyncGroupRateMultipliers(ctx, 7, []service.GroupRateMultiplierInput{{UserID: 42, RateMultiplier: rate}}))
	require.NoError(t, rates.SyncGroupRPMOverrides(ctx, 7, []service.GroupRPMOverrideInput{{UserID: 42, RPMOverride: &rpm}}))
	require.NoError(t, rates.ClearGroupDeniedModels(ctx, 7))
	gotRate, err := rates.GetByUserAndGroup(ctx, 42, 7)
	require.NoError(t, err)
	require.NotNil(t, gotRate)
	require.InDelta(t, rate, *gotRate, 1e-9)
	gotRPM, err := rates.GetRPMOverrideByUserAndGroup(ctx, 42, 7)
	require.NoError(t, err)
	require.NotNil(t, gotRPM)
	require.Equal(t, rpm, *gotRPM)
	assertRechargeBonusBalances(t, db, 101.99, 0)
	var remaining float64
	var permanent bool
	require.NoError(t, db.QueryRow(`SELECT remaining_amount, expires_at IS NULL FROM recharge_bonus_grants WHERE id=$1`, grant.GrantID).Scan(&remaining, &permanent))
	require.InDelta(t, 1.99, remaining, 1e-9)
	require.True(t, permanent)
}
