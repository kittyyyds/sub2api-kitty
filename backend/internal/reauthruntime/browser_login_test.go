package reauthruntime

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowserLoginBindingCleanupAndSafeFailure(t *testing.T) {
	for _, outcome := range []string{"success", "mismatch", "error", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			id := strings.Repeat("a", 48)
			deleted := false
			polls := 0
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer "+strings.Repeat("fixture", 8), r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" && r.URL.Path == "/sessions" {
					var input map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
					require.Equal(t, "socks5://fixture:pass@proxy.invalid:1080", input["proxy"])
					require.Equal(t, "Asia/Tokyo", input["timezone"])
					json.NewEncoder(w).Encode(map[string]string{"id": id})
					return
				}
				require.Equal(t, "/sessions/"+id, r.URL.Path)
				if r.Method == "DELETE" {
					deleted = true
					w.Write([]byte(`{}`))
					return
				}
				polls++
				if outcome == "cancel" {
					cancel()
					w.Write([]byte(`{"status":"running","stage":"navigation"}`))
					return
				}
				if outcome == "error" {
					w.Write([]byte(`{"status":"failed","stage":"secret-password","code":"secret-totp","timezone":"secret"}`))
					return
				}
				email := "fixture@example.invalid"
				if outcome == "mismatch" {
					email = "other@example.invalid"
				}
				json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "stage": "succeeded", "credential": map[string]string{"access_token": "fixture-a", "refresh_token": "fixture-r", "id_token": "fixture-i", "email": email}})
			}))
			defer server.Close()
			t.Setenv("KITTY_LOGIN_BROWSER_URL", server.URL)
			t.Setenv("KITTY_LOGIN_BROWSER_TOKEN", strings.Repeat("fixture", 8))
			result, err := BrowserPasswordLogin(ctx, "fixture@example.invalid", "secret-password", "secret-totp", "socks5://fixture:pass@proxy.invalid:1080", "Asia/Tokyo")
			require.True(t, deleted)
			require.GreaterOrEqual(t, polls, 1)
			if outcome == "success" {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "secret")
				p := LoginFailure(err)
				if outcome == "mismatch" {
					require.Equal(t, "account_identity_mismatch", p.Code)
				}
				if outcome == "cancel" {
					require.Equal(t, "login_canceled", p.Code)
				}
			}
		})
	}
}

func TestBrowserLoginDoesNotForwardCredentialsAcrossRedirect(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	t.Setenv("KITTY_LOGIN_BROWSER_URL", source.URL)
	t.Setenv("KITTY_LOGIN_BROWSER_TOKEN", strings.Repeat("fixture", 8))
	_, err := BrowserPasswordLogin(context.Background(), "a@example.invalid", "fixture", "fixture", "http://fixture.invalid:1", "UTC")
	require.Error(t, err)
	require.False(t, called)
}

func TestBrowserDiagnosticAllowlist(t *testing.T) {
	p := SafeLoginProgress(LoginProgress{Status: "failure-secret", Stage: "password=secret", Code: "token=secret", Timezone: "session-secret", HTTPStatus: 999})
	require.Equal(t, "login_failed", p.Code)
	require.Equal(t, "failed", p.Stage)
	require.Empty(t, p.Timezone)
	require.Zero(t, p.HTTPStatus)
	require.Equal(t, "login_failed", SafeLoginProgress(LoginProgress{Status: "failed", Stage: "password"}).Code)
}
