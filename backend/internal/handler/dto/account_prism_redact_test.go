package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrismAccountCredentialRedaction(t *testing.T) {
	a := &service.Account{Credentials: map[string]any{
		service.PrismCookieKey:          "must-not-leak",
		service.PrismCookieEncryptedKey: "ciphertext-must-not-leak",
		service.PrismUserIDKey:          "user-test-id",
	}}
	out := AccountFromServiceShallow(a)
	require.NotContains(t, out.Credentials, service.PrismCookieKey)
	require.NotContains(t, out.Credentials, service.PrismCookieEncryptedKey)
	require.True(t, out.CredentialsStatus["has_prism_cookie_encrypted"])
	require.Equal(t, "user-test-id", out.Credentials[service.PrismUserIDKey])
	merged := service.MergePreservingSensitiveCreds(a.Credentials, map[string]any{"model_mapping": map[string]string{"public": "gpt-6-astra"}})
	require.Equal(t, "ciphertext-must-not-leak", merged[service.PrismCookieEncryptedKey])
}
