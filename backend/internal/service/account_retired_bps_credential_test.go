//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRetiredBPSCredentialsAreNeverSchedulable(t *testing.T) {
	excel := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"client_id": openai.ExcelClientID}}
	plugin := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "bps"}}
	codex := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"client_id": openai.ClientID},
		Extra: map[string]any{"openai_excel_bps": true}}
	apiKey := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"auth_mode": "bps"}}

	for _, account := range []*Account{excel, plugin} {
		require.True(t, account.IsRetiredBPSCredential())
		require.False(t, account.IsModelSupported("gpt-5.1"))
		require.False(t, account.IsModelSupported(""))
		require.Error(t, rejectRetiredBPSCredentialWebSocket(account))
	}
	// A stale Excel BPS switch on a Codex credential no longer changes routing.
	require.False(t, codex.IsRetiredBPSCredential())
	require.True(t, codex.IsModelSupported("gpt-5.1"))
	require.NoError(t, rejectRetiredBPSCredentialWebSocket(codex))
	require.False(t, apiKey.IsRetiredBPSCredential())
	require.False(t, (*Account)(nil).IsRetiredBPSCredential())
}

func TestRetiredBPSCredentialErrorIsAClientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	require.ErrorIs(t, writeRetiredBPSCredentialError(c), errRetiredBPSCredential)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), retiredBPSCredentialErrorCode)
	require.ErrorIs(t, writeRetiredBPSCredentialError(nil), errRetiredBPSCredential)
}
