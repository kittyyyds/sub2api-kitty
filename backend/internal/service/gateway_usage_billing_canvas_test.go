//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type canvasUsageBillingRepo struct {
	UsageBillingRepository
	result *UsageBillingApplyResult
}

func (r *canvasUsageBillingRepo) Apply(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	return r.result, nil
}

type canvasUsageQuotaInvalidator struct {
	APIKeyQuotaUpdater
	keys []string
}

func (r *canvasUsageQuotaInvalidator) InvalidateAuthCacheByKey(_ context.Context, key string) {
	r.keys = append(r.keys, key)
}

func TestCanvasUsageBillingInvalidatesExhaustedKey(t *testing.T) {
	for _, tc := range []struct {
		name      string
		key       *APIKey
		applied   bool
		exhausted bool
		want      []string
	}{
		{name: "hash-only", key: &APIKey{ID: 6, KeyHash: strings.Repeat("a", 64)}, applied: true, exhausted: true, want: []string{"legacy-sha256:" + strings.Repeat("a", 64)}},
		{name: "plaintext", key: &APIKey{ID: 6, Key: "sk-fixture"}, applied: true, exhausted: true, want: []string{"sk-fixture"}},
		{name: "missing-handle", key: &APIKey{ID: 6}, applied: true, exhausted: true},
		{name: "not-exhausted", key: &APIKey{ID: 6, KeyHash: strings.Repeat("a", 64)}, applied: true},
		{name: "duplicate", key: &APIKey{ID: 6, KeyHash: strings.Repeat("a", 64)}, exhausted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalidator := &canvasUsageQuotaInvalidator{}
			repo := &canvasUsageBillingRepo{result: &UsageBillingApplyResult{Applied: tc.applied, APIKeyQuotaExhausted: tc.exhausted}}
			applied, err := applyUsageBilling(context.Background(), "canvas-fixture", &UsageLog{},
				&postUsageBillingParams{Cost: &CostBreakdown{}, User: &User{ID: 1}, APIKey: tc.key, Account: &Account{ID: 205}, APIKeyService: invalidator},
				&billingDeps{deferredService: &DeferredService{}}, repo)
			require.NoError(t, err)
			require.Equal(t, tc.applied, applied)
			require.Equal(t, tc.want, invalidator.keys)
		})
	}
}
