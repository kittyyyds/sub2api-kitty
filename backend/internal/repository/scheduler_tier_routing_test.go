package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchedulerMetadataRedactsRetiredTicketSecrets(t *testing.T) {
	original := service.Account{ID: 17, Extra: map[string]any{
		"codex_turn_ticket:gpt-6-astra": "historical secret",
	}}
	metadata := buildSchedulerMetadataAccount(original)
	require.NotContains(t, metadata.Extra, "codex_turn_ticket:gpt-6-astra")
	require.Contains(t, original.Extra, "codex_turn_ticket:gpt-6-astra")
}
