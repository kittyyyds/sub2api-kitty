package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchedulerMetadataPreservesAccountTierSwitch(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		original := service.Account{ID: 17, Extra: map[string]any{
			service.AccountPoolExtraKey:                 service.TierPremium,
			service.AccountTieredRoutingEnabledExtraKey: enabled,
			"codex_turn_ticket:gpt-6-astra":             "historical secret",
		}}
		metadata := buildSchedulerMetadataAccount(original)
		require.Equal(t, enabled, metadata.Extra[service.AccountTieredRoutingEnabledExtraKey])
		require.Equal(t, service.AccountPool(&original), service.AccountPool(&metadata))
		require.NotContains(t, metadata.Extra, "codex_turn_ticket:gpt-6-astra")
	}
}
