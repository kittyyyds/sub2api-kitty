package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type astraTargetVerifyStub struct {
	*astraSetupUpstream
	calls   int
	err     error
	request *http.Request
}

func (p *astraTargetVerifyStub) VerifyAstraGatewayTarget(_ context.Context, req *http.Request, _ string, _ int64, _ int) error {
	p.calls++
	p.request = req
	return p.err
}

type astraVerifyAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *astraVerifyAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestAstraGatewayVerifyUsesRouteVerifier(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexGatewayPin = config.CodexGatewayPinConfig{Enabled: true, SourceAccountIDs: []int64{8}, TargetAccountIDs: []int64{7}}
	a := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"access_token": "tok-7", "chatgpt_account_id": "acct-7"}}
	provider := &astraTargetVerifyStub{astraSetupUpstream: &astraSetupUpstream{}}
	svc := &AccountTestService{cfg: cfg, accountRepo: &astraVerifyAccountRepo{account: a}, httpUpstream: provider, openaiGatewayService: &OpenAIGatewayService{}}
	result, err := svc.TestAstraGateway(t.Context(), "verify", 7, 1)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Empty(t, result.Answer)
	require.Empty(t, result.Expected)
	require.Equal(t, "Bearer tok-7", provider.request.Header.Get("Authorization"))
	require.Equal(t, "acct-7", provider.request.Header.Get("ChatGPT-Account-ID"))
	require.Nil(t, provider.request.Body)
	require.Equal(t, 1, provider.calls)
	provider.err = errors.New("target_probe_failed")
	result, err = svc.TestAstraGateway(t.Context(), "verify", 7, 1)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "target_probe_failed", result.Reason)
}
