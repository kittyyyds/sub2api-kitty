package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/vmbridge"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The operation list is an allow-list shared with the bridge: every entry must
// be a route the bridge relays, and nothing may hand back a saved login.
func TestOpenAIVMManagedOperationsAreBridgeRoutesAndNeverReveal(t *testing.T) {
	require.NotEmpty(t, OpenAIVMManagedOperations)
	for route, operation := range OpenAIVMManagedOperations {
		method, suffix, ok := strings.Cut(route, " ")
		require.Truef(t, ok, route)
		require.Equalf(t, suffix, operation, "%s: the route suffix is the bridge path", route)
		require.Truef(t, vmbridge.ManagedRouteKnown(method, operation), "%s is not relayed by the bridge", route)
		for _, banned := range []string{"reveal", "export", "credential", "secret"} {
			require.NotContainsf(t, operation, banned, "%s must not be offered", route)
		}
	}
	// Scheduling, concurrency and the proxy have their own handlers with their
	// own checks; they are deliberately not generic operations.
	for _, own := range []string{"POST schedulable", "PUT policy", "PUT proxy", "POST usage/refresh"} {
		require.NotContains(t, OpenAIVMManagedOperations, own)
	}
}

func TestOpenAIVMManagedOperationRefusesBeforeReachingTheSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// A service with no bridge behind it: any request that got past the
	// handler's own checks would panic instead of answering.
	h := NewOpenAIVMManagedHandler(&service.OpenAIVMManagedService{}, nil, nil)
	require.NotNil(t, h)
	router := gin.New()
	router.GET("/accounts/:id/vm-managed/op/*operation", h.Operation)
	router.POST("/accounts/:id/vm-managed/op/*operation", h.Operation)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
		return recorder
	}
	for name, tc := range map[string]struct {
		method, path, body string
		status             int
	}{
		"saved login reveal":     {http.MethodPost, "/accounts/7/vm-managed/op/login/password/reveal", "{}", http.StatusNotFound},
		"unknown operation":      {http.MethodPost, "/accounts/7/vm-managed/op/anything", "{}", http.StatusNotFound},
		"path escape":            {http.MethodPost, "/accounts/7/vm-managed/op/../../7", "{}", http.StatusNotFound},
		"write through a read":   {http.MethodGet, "/accounts/7/vm-managed/op/login/oauth-url", "", http.StatusNotFound},
		"read through a write":   {http.MethodPost, "/accounts/7/vm-managed/op/diagnose", "{}", http.StatusNotFound},
		"generic proxy change":   {http.MethodPost, "/accounts/7/vm-managed/op/proxy", "{}", http.StatusNotFound},
		"bad account id":         {http.MethodPost, "/accounts/abc/vm-managed/op/login/password", "{}", http.StatusBadRequest},
		"negative account id":    {http.MethodPost, "/accounts/-3/vm-managed/op/login/password", "{}", http.StatusBadRequest},
		"oversized login":        {http.MethodPost, "/accounts/7/vm-managed/op/login/password", strings.Repeat("x", openAIVMManagedBodyLimit+1), http.StatusBadRequest},
		"oversized account file": {http.MethodPost, "/accounts/7/vm-managed/op/login/import", strings.Repeat("x", openAIVMManagedImportLimit+1), http.StatusBadRequest},
	} {
		recorder := do(tc.method, tc.path, tc.body)
		require.Equalf(t, tc.status, recorder.Code, "%s: %s", name, recorder.Body.String())
		require.NotContainsf(t, recorder.Body.String(), "xxxx", "%s echoed the request", name)
	}
	require.Nil(t, NewOpenAIVMManagedHandler(nil, nil, nil), "no service, no routes")
}
