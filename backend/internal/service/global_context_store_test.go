//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestGlobalContextSnapshotRetainsVerifiedRulesOnDBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := NewGlobalContextRuleStore(db)
	mock.ExpectQuery("SELECT .* FROM global_context_pricing_rules ORDER BY").WillReturnRows(sqlmock.NewRows(strings.Split(globalContextColumns, ",")).AddRow(1, "gpt-6.1-sol", "openai", 500000, 2, 1.5, 2, 2, true, 1, time.Now()))
	require.NoError(t, s.Prime(context.Background()))
	r, err := s.Find(context.Background(), " GPT-6.1-SOL ")
	require.NoError(t, err)
	require.Equal(t, int64(1), r.ID)
	// A caller must not mutate the shared snapshot.
	r.InputMultiplier = 99
	s.refreshAt = time.Time{}
	mock.ExpectQuery("SELECT .* FROM global_context_pricing_rules ORDER BY").WillReturnError(errors.New("offline"))
	r, err = s.Find(context.Background(), "gpt-6.1-sol")
	require.NoError(t, err)
	require.Equal(t, 2., r.InputMultiplier)
	r, err = s.Find(context.Background(), "other")
	require.NoError(t, err)
	require.Nil(t, r)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGlobalContextSnapshotDoesNotBlockConcurrentBilling(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := NewGlobalContextRuleStore(db)
	s.snapshot = map[string]GlobalContextRule{"fixture": {ID: 1, InputMultiplier: 2}}
	// Another request is refreshing or saving. Reads must use the verified cache.
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	done := make(chan *GlobalContextRule, 1)
	go func() { r, _ := s.Find(context.Background(), "fixture"); done <- r }()
	select {
	case r := <-done:
		require.Equal(t, 2., r.InputMultiplier)
	case <-time.After(time.Second):
		t.Fatal("billing blocked behind DB refresh")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
