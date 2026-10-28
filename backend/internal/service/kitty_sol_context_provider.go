package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Restore only the explicitly approved customer policy. Other retired global
// rules remain inactive; no prices, thresholds or database rows are rewritten.
type solContextRuleReader struct{ store *GlobalContextRuleStore }

func (r solContextRuleReader) Find(ctx context.Context, model string) (*GlobalContextRule, error) {
	if strings.ToLower(strings.TrimSpace(model)) != "gpt-6.1-sol" {
		return nil, nil
	}
	return r.store.Find(ctx, "gpt-6.1-sol")
}

func provideSolContextRules(db *sql.DB) (globalContextRuleReader, error) {
	if db == nil {
		return nil, errors.New("6.1-sol context rules require the pricing database")
	}
	store := NewGlobalContextRuleStore(db)
	if err := store.Prime(context.Background()); err != nil {
		return nil, err
	}
	return solContextRuleReader{store: store}, nil
}
