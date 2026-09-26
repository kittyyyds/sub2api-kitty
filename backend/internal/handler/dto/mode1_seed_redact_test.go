package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountIdentitySeedsStayServerManaged(t *testing.T) {
	a := &service.Account{ID: 1, Extra: map[string]any{
		"codex_fingerprint_seed":        "official-seed-secret",
		"codex_fingerprint_mode":        "device",
		"fingerprint_v2_seed":           "retired-v2-seed-secret",
		"client_installation_id":        "retired-installation-secret",
		"client_identity_compatibility": map[string]any{"client_installation_id": "retired-compatibility-secret"},
		"anti_degrade":                  map[string]any{"enabled": true, "prev": map[string]any{"fingerprint_v2_seed": "retired-snapshot-secret"}},
		"ordinary":                      "kept",
	}}
	out := AccountFromService(a)
	for _, key := range []string{"codex_fingerprint_seed", "fingerprint_v2_seed", "client_installation_id", "client_identity_compatibility", "anti_degrade"} {
		require.NotContains(t, out.Extra, key)
		require.Contains(t, a.Extra, key, "DTO redaction must not change the stored account")
	}
	require.Equal(t, "device", out.Extra["codex_fingerprint_mode"])
	require.Equal(t, "kept", out.Extra["ordinary"])

	for _, dto := range []any{out, AccountListItemFromAccount(out)} {
		raw, err := json.Marshal(dto)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(raw, &fields))
		for _, key := range []string{"anti_degradation", "protection_scope", "protection_mode", "proxy_pool"} {
			require.NotContains(t, fields, key)
		}
		for _, secret := range []string{"official-seed-secret", "retired-v2-seed-secret", "retired-installation-secret", "retired-compatibility-secret", "retired-snapshot-secret"} {
			require.NotContains(t, string(raw), secret)
		}
	}
}
