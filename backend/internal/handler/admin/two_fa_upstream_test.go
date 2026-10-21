package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamTwoFASettings struct {
	service.SettingRepository
	raw string
}

func (s upstreamTwoFASettings) GetValue(context.Context, string) (string, error) { return s.raw, nil }

type upstreamTwoFAImports struct {
	service.AdminService
	ticket    *service.AccountImportTicket
	completed chan string
	canceled  chan string
}

func (s *upstreamTwoFAImports) PrepareAccountImport(_ context.Context, platform, source string, proxy *int64, identity string) (*service.AccountImportTicket, error) {
	if platform != "openai" || source != "twofa" || proxy != nil || identity != "fixture@example.invalid" {
		return nil, fmt.Errorf("unexpected import metadata")
	}
	return s.ticket, nil
}
func (s *upstreamTwoFAImports) CompleteAccountImport(_ context.Context, id string, credential map[string]any) error {
	if credential["access_token"] != "fixture-access" {
		return fmt.Errorf("invalid result")
	}
	s.completed <- id
	return nil
}
func (s *upstreamTwoFAImports) CancelAccountImport(_ context.Context, id string) error {
	s.canceled <- id
	return nil
}

// The real HTTP handler must select the upstream service even when automation
// reserves a proxy. No local login runtime or real account is used by this test.
func TestTwoFAImportRestoresUpstreamProviderWithoutChangingAccountBinding(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("service_failure_%v", fail), func(t *testing.T) {
			payloads := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Error("invalid payload")
				}
				payloads <- payload
				if fail {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, `{"credential":{"access_token":"fixture-access","refresh_token":"fixture-refresh","id_token":"fixture-id"}}`)
			}))
			defer server.Close()
			raw, err := json.Marshal(map[string]any{"relogin_endpoint": server.URL})
			require.NoError(t, err)
			proxyID := int64(17)
			imports := &upstreamTwoFAImports{ticket: &service.AccountImportTicket{ID: "fixture-ticket", ProxyID: &proxyID}, completed: make(chan string, 1), canceled: make(chan string, 4)}
			svc := service.NewAccountTokenGuardService(upstreamTwoFASettings{raw: string(raw)}, nil, nil, imports, nil)
			// A nil worker would reject the old proxy-login branch before HTTP 202.
			reauth := service.NewOpenAIOAuthReauthService(nil, nil, nil, nil, &credentialHandlerEncryptor{ready: true}, false, nil, nil)
			h := NewAccountTokenGuardHandler(svc, reauth, nil)
			r := gin.New()
			r.POST("/login", h.StartTwoFALogin)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"fixture@example.invalid","password":"fixture-password","mfa_secret":"fixture-totp","credential_target":"operations","login_environment":{"enabled":true,"mode":"proxy","timezone":"Asia/Tokyo"}}`))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
			var response struct {
				Data service.OpenAITwoFALoginJob `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			job := response.Data
			defer svc.DeleteTwoFALogin(job.ID)
			require.Equal(t, "configured_service", job.LoginBackend)
			require.Equal(t, &proxyID, job.ProxyID)
			require.Equal(t, "fixture-ticket", job.ImportTicket)
			var result *service.OpenAITwoFALoginJob
			require.Eventually(t, func() bool { result, _ = svc.TwoFALogin(job.ID); return result != nil && result.Status != "running" }, 3*time.Second, 5*time.Millisecond)
			payload := <-payloads
			require.Equal(t, map[string]any{"action": "start", "auth_mode": "password_2fa", "email": "fixture@example.invalid", "password": "fixture-password", "mfa_secret": "fixture-totp"}, payload)
			require.Equal(t, "configured_service", result.LoginBackend)
			require.Equal(t, &proxyID, result.ProxyID)
			require.Empty(t, result.Timezone)
			require.False(t, result.BrowserAvailable)
			if fail {
				require.Equal(t, "failed", result.Status)
				require.Nil(t, result.Credential)
				require.Equal(t, "fixture-ticket", <-imports.canceled)
				require.Empty(t, imports.completed)
			} else {
				require.Equal(t, "succeeded", result.Status)
				require.Equal(t, "fixture-ticket", <-imports.completed)
				require.Empty(t, imports.canceled)
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "fixture-password")
			require.NotContains(t, string(encoded), "fixture-totp")
		})
	}
}
