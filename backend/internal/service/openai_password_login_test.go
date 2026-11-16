//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestOpenAIPasswordLoginOwnerAndProxyBinding(t *testing.T) {
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-worker-secret", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		calls = append(calls, body)
		w.Write([]byte(`{"id":"00112233-4455-4677-8899-aabbccddeeff","status":"running"}`))
	}))
	defer server.Close()
	s := &OpenAIOAuthService{passwordLogin: &openAIPasswordLoginClient{endpoint: server.URL, secret: "synthetic-worker-secret", client: server.Client()}}
	job, err := s.StartPasswordLogin(context.Background(), 77, OpenAIPasswordLoginInput{Email: "owner@example.test", Password: "synthetic-password", MFAMode: "code"})
	require.NoError(t, err)
	require.NotEmpty(t, job.ID)
	require.EqualValues(t, 77, calls[0]["owner_id"])
	require.Equal(t, "", calls[0]["proxy_url"])
	id := int64(9)
	_, err = s.StartPasswordLogin(context.Background(), 77, OpenAIPasswordLoginInput{ProxyID: &id})
	require.Error(t, err)
	require.Len(t, calls, 1, "missing configured proxy never launches a direct login")
	_, err = s.PasswordLoginAction(context.Background(), 88, job.ID, "code", "123456")
	require.NoError(t, err)
	require.EqualValues(t, 88, calls[1]["owner_id"])
	_, err = s.PasswordLoginAction(context.Background(), 0, job.ID, "status", "")
	require.Error(t, err)
	require.Len(t, calls, 2)
}

func TestOpenAIPasswordLoginErrorsNeverEchoProviderOrSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"code":"password_rejected","message":"secret-canary https://bad.invalid"}`))
	}))
	defer server.Close()
	client := &openAIPasswordLoginClient{endpoint: server.URL, secret: "test", client: server.Client()}
	var result OpenAIPasswordLoginJob
	err := client.call(context.Background(), map[string]any{}, &result)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-canary")
	require.NotContains(t, err.Error(), "bad.invalid")
}

func TestOpenAIPasswordLoginResultMustBeNormalCodexIdentity(t *testing.T) {
	encode := func(v any) string { data, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(data) }
	var audience = []string{openai.ClientID}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "header." + encode(map[string]any{"email": "owner@example.test", "aud": audience, "exp": time.Now().Unix() + 3600, "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "workspace", "chatgpt_plan_type": "plus"}}) + ".signature"
		json.NewEncoder(w).Encode(map[string]any{"proxy_id": 7, "credential": map[string]any{"access_token": "synthetic-access", "refresh_token": "synthetic-refresh", "id_token": id, "email": "owner@example.test", "chatgpt_account_id": "workspace", "expires_at": time.Now().Add(time.Hour).UnixMilli()}})
	}))
	defer server.Close()
	s := &OpenAIOAuthService{passwordLogin: &openAIPasswordLoginClient{endpoint: server.URL, secret: "test", client: server.Client()}}
	result, err := s.TakePasswordLoginResult(context.Background(), 77, "00112233-4455-4677-8899-aabbccddeeff")
	require.NoError(t, err)
	require.Equal(t, openai.ClientID, result.TokenInfo.ClientID)
	require.Equal(t, "plus", result.TokenInfo.PlanType)
	require.EqualValues(t, 7, *result.ProxyID)
	require.False(t, strings.Contains(fmt.Sprint(result.TokenInfo), "vm_managed"))
	audience = []string{openai.ExcelClientID}
	_, err = s.TakePasswordLoginResult(context.Background(), 77, "00112233-4455-4677-8899-aabbccddeeff")
	require.Error(t, err, "retired Excel grants cannot become normal Codex accounts")
}
