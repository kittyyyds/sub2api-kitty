package vm2api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedCapacityIncludesManualSlotsWithoutMutatingVM2(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/panel/proxies":
			replyData(w, map[string]any{"config": map[string]int{"bind_limit": 5}, "proxies": []Proxy{
				{ID: "px-one", Host: "exit.test", Port: 1080, Enabled: true, BoundVMIDs: []string{"manual-slot", "s2a-slot"}},
				{ID: "px-disabled", Host: "disabled.test", Port: 1080, Enabled: false},
			}})
		case r.Method == "POST" && r.URL.Path == "/api/panel/proxies/px-one/reveal":
			replyData(w, map[string]string{"id": "px-one", "uri": "socks5://user:canary-password@exit.test:1080"})
		case r.Method == "POST" && r.URL.Path == "/api/panel/proxies/px-disabled/reveal":
			replyData(w, map[string]string{"id": "px-disabled", "uri": "socks5://user:canary-password@disabled.test:1080"})
		default:
			t.Errorf("Unexpected mutating or unrelated request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	})
	p, err := NewProvider(c, "capacity-test")
	require.NoError(t, err)
	caps, err := p.ManagedProxyCapacity(context.Background(), []string{
		"socks5h://user:canary-password@EXIT.test:1080",
		"socks5://user:canary-password@disabled.test:1080",
		"socks5://user:other-password@exit.test:1080",
	})
	require.NoError(t, err)
	require.Len(t, caps, 3)
	require.Equal(t, 2, caps[0].Used)
	require.Equal(t, 5, caps[0].Limit)
	require.True(t, caps[0].Available)
	require.False(t, caps[1].Available)
	require.Zero(t, caps[2].Used)
	require.True(t, caps[2].Available)
	raw, _ := json.Marshal(caps)
	for _, secret := range []string{"password", "exit.test", "user", "manual-slot"} {
		require.False(t, strings.Contains(string(raw), secret))
	}
}

func TestManagedCapacityRejectsUnknownLimit(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		replyData(w, map[string]any{"config": map[string]int{"bind_limit": 0}})
	})
	p, err := NewProvider(c, "capacity-test")
	require.NoError(t, err)
	_, err = p.ManagedProxyCapacity(context.Background(), []string{"socks5://exit.test:1080"})
	require.Error(t, err)
}
