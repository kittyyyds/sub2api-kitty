package vmbridge

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type capacityProvider struct {
	testProvider
	calls int
	err   error
}

func (p *capacityProvider) ManagedProxyCapacity(_ context.Context, urls []string) ([]ProxyCapacity, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return []ProxyCapacity{{Used: 2, Limit: 5, Available: true}}, nil
}

func TestCapacityEndpointRequiresAuthenticationAndNeverEchoesProxyCredentials(t *testing.T) {
	p := &capacityProvider{}
	m := newTestManager(t, p)
	call := func(auth, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/managed-capacity", strings.NewReader(body))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, r)
		return w
	}
	body := `{"proxies":["socks5://user:private-secret@exit.test:1080"]}`
	require.Equal(t, 401, call("", body).Code)
	require.Zero(t, p.calls)
	w := call("Bearer "+testSecret, body)
	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `[{"used":2,"limit":5,"available":true}]`, w.Body.String())
	require.Equal(t, 400, call("Bearer "+testSecret, `{"proxies":[]}`).Code)
	p.err = errors.New("private-secret")
	w = call("Bearer "+testSecret, body)
	require.Equal(t, 502, w.Code)
	require.NotContains(t, w.Body.String(), "private-secret")
}
