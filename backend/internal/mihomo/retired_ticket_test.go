package mihomo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyConfigOmitsRetiredTicketListeners(t *testing.T) {
	m := New(t.TempDir())
	t.Cleanup(m.Close)
	var old saved
	require.NoError(t, json.Unmarshal([]byte(`{"use_once":true,"collection_last_node":"a","nodes":[{"name":"a","type":"http","server":"127.0.0.1","port":8080}]}`), &old))
	raw, err := m.config(old)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "CODEX-COLLECT")
	require.NotContains(t, string(raw), "ASTRA")
	var result struct {
		Groups []struct { Type string `json:"type"` } `json:"proxy-groups"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.Equal(t, "load-balance", result.Groups[0].Type)
	require.Error(t, m.Submit("once_on", nil, false))
}
