package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/stretchr/testify/require"
)

func TestFusionManagedStandbyIsNotReadyEvenWhenDependenciesAreHealthy(t *testing.T) {
	role := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(role, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "fusion-readiness")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", role)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() { _ = os.Setenv("KITTY_MANAGED_INSTANCE", "false"); _ = managedinstance.ConfigureFromEnv() })
	managedinstance.Current().MarkReady()
	app := NewLifecycle().Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for _, role := range []string{"standby", "active", "draining"} {
		require.NoError(t, managedinstance.Current().SetRole(role))
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, httptest.NewRequest("GET", "/readyz", nil))
		require.Equal(t, 503, rr.Code, "no active lease: readiness must stay withdrawn")
		rr = httptest.NewRecorder()
		app.ServeHTTP(rr, httptest.NewRequest("GET", "/health", nil))
		require.Equal(t, 200, rr.Code)
	}
}
