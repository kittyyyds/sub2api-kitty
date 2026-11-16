//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSavedOpenAILoginEncryptionScopeExpiryAndForget(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	s := &OpenAIOAuthService{}
	a := &Account{ID: 8, CreatedAt: time.Now(), Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"email": "owner@example.test", "chatgpt_account_id": "workspace-one"}}
	job := "00112233-4455-4677-8899-aabbccddeeff"
	store := openAISavedLoginStore()
	record := savedOpenAILoginRecord{Scope: pendingOpenAILoginScope(77, job), Input: OpenAIPasswordLoginInput{Email: "owner@example.test", Password: "synthetic-password", MFAMode: "secret", MFASecret: "JBSWY3DPEHPK3PXP"}, Authorized: true, WorkspaceID: "workspace-one", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	require.NoError(t, store.write(record))
	require.Error(t, s.RememberPasswordLogin(78, job, a), "task owners are isolated")
	wrong := *a
	wrong.Credentials = map[string]any{"email": "other@example.test", "chatgpt_account_id": "workspace-one"}
	require.Error(t, s.RememberPasswordLogin(77, job, &wrong))
	wrong.Credentials = map[string]any{"email": "owner@example.test", "chatgpt_account_id": "workspace-two"}
	require.Error(t, s.RememberPasswordLogin(77, job, &wrong))
	require.NoError(t, s.RememberPasswordLogin(77, job, a))
	raw, err := os.ReadFile(store.file(accountOpenAILoginScope(a)))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "synthetic-password")
	require.NotContains(t, string(raw), "JBSWY3DPEHPK3PXP")
	// A new service instance reads the persistent store, never the temporary task.
	summary, err := (&OpenAIOAuthService{}).SavedPasswordLogin(a)
	require.NoError(t, err)
	require.Equal(t, "owner@example.test", summary.Email)
	visible, _ := json.Marshal(summary)
	require.NotContains(t, string(visible), "synthetic-password")
	require.NotContains(t, string(visible), "JBSWY3DPEHPK3PXP")
	recycled := *a
	recycled.CreatedAt = a.CreatedAt.Add(time.Second)
	v, err := s.SavedPasswordLogin(&recycled)
	require.NoError(t, err)
	require.Nil(t, v)
	expired := record
	expired.Scope = "pending:expired"
	expired.ExpiresAt = time.Now().Add(-time.Second).Unix()
	require.NoError(t, store.write(expired))
	p, err := store.read(expired.Scope)
	require.NoError(t, err)
	require.Nil(t, p)
	require.NoError(t, s.ForgetPasswordLogin(a))
	v, err = s.SavedPasswordLogin(a)
	require.NoError(t, err)
	require.Nil(t, v)
	require.NoError(t, s.ForgetPasswordLogin(a))
}

func TestSavedOpenAILoginUsesCurrentProxyAndDoesNotExposeSecret(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	a := &Account{ID: 9, CreatedAt: time.Now(), Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"email": "owner@example.test", "chatgpt_account_id": "w"}}
	stale := int64(123)
	require.NoError(t, openAISavedLoginStore().write(savedOpenAILoginRecord{Scope: accountOpenAILoginScope(a), WorkspaceID: "w", Input: OpenAIPasswordLoginInput{Email: "owner@example.test", Password: "synthetic", MFAMode: "code", ProxyID: &stale}}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Input    OpenAIPasswordLoginInput `json:"input"`
			ProxyURL string                   `json:"proxy_url"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&b))
		require.Nil(t, b.Input.ProxyID, "must not reuse stale saved proxy")
		require.Empty(t, b.ProxyURL)
		require.Equal(t, "synthetic", b.Input.Password)
		require.False(t, b.Input.Remember)
		_, _ = w.Write([]byte(`{"id":"00112233-4455-4677-8899-aabbccddeeff","status":"running"}`))
	}))
	defer server.Close()
	s := &OpenAIOAuthService{passwordLogin: &openAIPasswordLoginClient{endpoint: server.URL, client: server.Client(), secret: "synthetic"}}
	_, err := s.StartSavedPasswordLogin(context.Background(), 77, a)
	require.NoError(t, err)
	current := int64(999)
	a.ProxyID = &current
	input, err := s.savedPasswordLoginInput(a)
	require.NoError(t, err)
	require.EqualValues(t, 999, *input.ProxyID)
	_, err = s.StartSavedPasswordLogin(context.Background(), 77, a)
	require.Error(t, err, "unavailable current proxy must fail without direct fallback")
	// Ciphertext modification, or loss of the key, is an error rather than a wrong login attempt.
	p := openAISavedLoginStore().file(accountOpenAILoginScope(a))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(p, data, 0600))
	_, err = s.SavedPasswordLogin(a)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "synthetic")
	require.NoError(t, os.Remove(filepath.Join(openAISavedLoginStore().dir, ".key")))
	_, err = s.SavedPasswordLogin(a)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "JBSWY"))
}
