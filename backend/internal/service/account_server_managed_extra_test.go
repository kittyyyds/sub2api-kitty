package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerManagedExtraPreservesTiboVerdictWithoutMutatingInputs(t *testing.T) {
	key := openAITiboVerdictExtraKeyPrefix + "gpt-6-astra"
	current := map[string]any{key: "current", "old_setting": true}
	edit := map[string]any{key: "stale", "openai_vm_managed": true}
	merged := MergeServerManagedAccountExtra(edit, current)
	require.Equal(t, "current", merged[key])
	require.Equal(t, true, merged["openai_vm_managed"])
	require.NotContains(t, merged, "old_setting")
	require.Equal(t, "stale", edit[key])
	require.Equal(t, "current", current[key])
	require.NotContains(t, RedactServerManagedAccountExtra(merged), key)
	require.Contains(t, merged, key)
	require.Equal(t, current[key], MergeServerManagedAccountExtra(nil, current)[key])
	require.Nil(t, MergeServerManagedAccountExtra(nil, nil))
}

// The retired Excel BPS credential state is no longer server-managed: it is a
// retired extra key that full-account writes and exports drop.
func TestRetiredExcelBPSCredentialStateIsNotServerManaged(t *testing.T) {
	key := "openai_excel_bps_credential_state"
	require.False(t, isServerManagedAccountExtraKey(key))
	require.True(t, IsRetiredAccountExtraKey(key))
	require.NotContains(t, StripRetiredAccountExtra(map[string]any{key: map[string]any{"status": "unknown"}}), key)
}
