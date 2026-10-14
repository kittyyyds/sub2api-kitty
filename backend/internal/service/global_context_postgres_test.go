//go:build unit && linux

package service

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestGlobalContextPostgresBillingAndRecovery(t *testing.T) {
	dsn := os.Getenv("KITTY_ISOLATED_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated fixture only")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "fixture", name)
	schema := fmt.Sprintf("context_fixture_%d", time.Now().UnixNano())
	_, err = db.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	isolated, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	defer isolated.Close()
	ddl, err := migrations.FS.ReadFile("270_kitty_global_context_pricing.sql")
	require.NoError(t, err)
	_, err = isolated.Exec(string(ddl))
	require.NoError(t, err)
	store := NewGlobalContextRuleStore(isolated)
	ctx := context.Background()
	require.NoError(t, store.Prime(ctx))
	bs := NewBillingService(&config.Config{}, nil)
	bs.globalContextRules = store
	bs.fallbackPrices["gpt-5.4"] = &ModelPricing{InputPricePerToken: 0.000001, OutputPricePerToken: 0.000002, LongContextInputThreshold: 10, LongContextInputMultiplier: 3, LongContextOutputMultiplier: 3}
	request := CostInput{Model: "gpt-5.4", Tokens: UsageTokens{InputTokens: 500001, OutputTokens: 10}, RateMultiplier: 0.2}
	base, err := bs.CalculateCostUnified(request)
	require.NoError(t, err)
	rule := GlobalContextRule{Model: "gpt-5.4", Platform: "openai", ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2, CacheReadMultiplier: 2, CacheWriteMultiplier: 2, Enabled: true}
	saved, err := store.Save(ctx, rule)
	require.NoError(t, err)
	_, err = store.Save(ctx, rule)
	require.ErrorIs(t, err, ErrGlobalContextConflict)
	cost, err := bs.CalculateCostUnified(request)
	require.NoError(t, err)
	require.InDelta(t, base.TotalCost*2/3, cost.TotalCost, 1e-9)
	require.InDelta(t, base.TotalCost, *cost.ContextBaseTotalCost, 1e-9)
	request.Tokens.InputTokens = 500000
	cost, err = bs.CalculateCostUnified(request)
	require.NoError(t, err)
	require.InDelta(t, 0.500020, cost.TotalCost, 1e-9)
	disabled := *saved
	disabled.Enabled = false
	updated, err := store.Save(ctx, disabled)
	require.NoError(t, err)
	request.Tokens.InputTokens = 500001
	cost, err = bs.CalculateCostUnified(request)
	require.NoError(t, err)
	require.InDelta(t, base.TotalCost, cost.TotalCost, 1e-9)
	require.ErrorIs(t, store.Delete(ctx, saved.ID, saved.Revision), ErrGlobalContextConflict)
	require.NoError(t, store.Delete(ctx, updated.ID, updated.Revision))
	cost, err = bs.CalculateCostUnified(request)
	require.NoError(t, err)
	require.InDelta(t, base.TotalCost, cost.TotalCost, 1e-9)
}
