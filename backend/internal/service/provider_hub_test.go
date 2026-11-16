package service

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type providerHubRemoteTestHTTP struct {
	HTTPUpstream
	respond func(*http.Request) (int, string)
}

func (f *providerHubRemoteTestHTTP) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	status, body := f.respond(req)
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

type providerHubTestRepo struct {
	ChannelRepository
	ProviderHubRepository
	wallet   ProviderHubWalletAuthorization
	rotated  string
	observed *time.Time
	balance  *float64
	reason   string
}

func (r *providerHubTestRepo) ProviderHubAuthorization(context.Context, int64) (*ProviderHubWalletAuthorization, error) {
	copy := r.wallet
	return &copy, nil
}
func (r *providerHubTestRepo) RotateProviderHubCredential(_ context.Context, w *ProviderHubWalletAuthorization, cipher string) error {
	r.rotated = cipher
	w.Cipher = cipher
	return nil
}
func (r *providerHubTestRepo) SaveProviderHubObservation(_ context.Context, _ *ProviderHubWalletAuthorization, b *float64, _, _, reason string, at *time.Time) error {
	r.balance = b
	r.reason = reason
	r.observed = at
	return nil
}

func providerHubRemoteFixture(t *testing.T, site string, respond func(*http.Request) (int, string)) (*ChannelService, *providerHubTestRepo) {
	t.Helper()
	repo := &providerHubTestRepo{wallet: ProviderHubWalletAuthorization{WalletID: 1, Revision: 1, SiteType: site, BaseURL: "https://supplier.example", AuthMode: "token", Cipher: "nap-fixture-only", CreditCurrency: "USD"}}
	p := newUpstreamBillingProbeTestService(&upstreamBillingProbeAccountRepo{}, &providerHubRemoteTestHTTP{respond: respond}, &upstreamBillingProbeSettingRepo{})
	p.newAPIFixedKey = true
	p.newAPIEncryptor = hubTestEncryptor{}
	return &ChannelService{repo: repo, providerProbe: p}, repo
}

type hubTestEncryptor struct{}

func (hubTestEncryptor) Encrypt(v string) (string, error) { return v, nil }
func (hubTestEncryptor) Decrypt(v string) (string, error) { return v, nil }

func TestProviderHubNewAPIConversionAndAuthorization(t *testing.T) {
	calls := 0
	s, repo := providerHubRemoteFixture(t, "newapi", func(r *http.Request) (int, string) {
		calls++
		require.Equal(t, "Bearer nap-fixture-only", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		if r.URL.Path == "/api/user/self" {
			return 200, `{"success":true,"data":{"quota":-150000}}`
		}
		require.Equal(t, "/api/status", r.URL.Path)
		return 200, `{"success":true,"data":{"quota_per_unit":500000}}`
	})
	require.NoError(t, s.SyncProviderHubWallet(context.Background(), 1))
	require.Equal(t, 2, calls)
	require.NotNil(t, repo.balance)
	require.InDelta(t, -0.3, *repo.balance, 1e-12)
}
func TestProviderHubRefreshAndSecretFreeKeys(t *testing.T) {
	calls, refreshed := 0, false
	s, repo := providerHubRemoteFixture(t, "sub2api", func(r *http.Request) (int, string) {
		calls++
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshed = true
			return 200, `{"code":0,"data":{"access_token":"new-fixture-only","refresh_token":"new-refresh-only"}}`
		case "/api/v1/keys":
			if !refreshed {
				return 401, `{}`
			}
			require.Equal(t, "Bearer new-fixture-only", r.Header.Get("Authorization"))
			return 200, `{"code":0,"data":{"items":[{"id":1,"name":"active","status":"active","key":"sk-fixture-only-secret"},{"id":2,"name":"disabled","status":"disabled","key":"sk-disabled-secret"}],"total":2}}`
		}
		t.Fatalf("unexpected request %s", r.URL.Path)
		return 500, `{}`
	})
	repo.wallet.Cipher = `{"access_token":"old-fixture-only","refresh_token":"old-refresh-only"}`
	keys, e := s.ProviderHubRemoteKeys(context.Background(), 1)
	require.NoError(t, e)
	require.Len(t, keys, 2)
	require.True(t, keys[0].Available)
	require.False(t, keys[1].Available)
	require.Equal(t, 3, calls)
	require.Contains(t, repo.rotated, "new-refresh-only")
	encoded, e := json.Marshal(keys)
	require.NoError(t, e)
	require.NotContains(t, string(encoded), "sk-fixture-only-secret")
	require.NotContains(t, string(encoded), "new-refresh-only")
}
func TestProviderHubRejectsMalformedInventoryAndDuplicatePurchaseInput(t *testing.T) {
	s, _ := providerHubRemoteFixture(t, "sub2api", func(*http.Request) (int, string) { return 200, `{"code":0,"data":{"unrelated":true}}` })
	_, e := s.ProviderHubRemoteKeys(context.Background(), 1)
	require.ErrorIs(t, e, ErrProviderRemote)
	for _, currency := range []string{"", "US", "U\nSD", "USDT"} {
		c := ProviderHubCommand{Action: "wallet", SupplierID: 1, Name: "wallet", Identity: "identity", Currency: currency, CreditCurrency: "USD"}
		require.ErrorIs(t, ValidateProviderHubCommand(&c), ErrProviderHubInput)
	}
	c := ProviderHubCommand{Action: "rule", ID: 1, Revision: 1, Model: "*", Method: "request", UnitPrice: 0, Currency: "USD"}
	require.NoError(t, ValidateProviderHubCommand(&c))
	require.NotNil(t, c.FXToUSD)
	require.Equal(t, 1.0, *c.FXToUSD)
	c.Model = "gpt-*"
	require.ErrorIs(t, ValidateProviderHubCommand(&c), ErrProviderHubInput)
}

func TestProviderHubCacheWrite12hPricePreservesOptionalZeroAndRejectsInvalid(t *testing.T) {
	for _, price := range []*float64{nil, float64Ptr(0), float64Ptr(3.5)} {
		c := ProviderHubCommand{Action: "rule", ID: 1, Revision: 1, Model: "*", Method: "tokens", CacheWritePrice: 2, CacheWrite12hPrice: price, Currency: "USD"}
		require.NoError(t, ValidateProviderHubCommand(&c))
		require.Equal(t, price, c.CacheWrite12hPrice)
	}
	for _, price := range []float64{-1, math.NaN(), math.Inf(1), 1e13} {
		c := ProviderHubCommand{Action: "rule", ID: 1, Revision: 1, Model: "*", Method: "tokens", CacheWrite12hPrice: &price, Currency: "USD"}
		require.ErrorIs(t, ValidateProviderHubCommand(&c), ErrProviderHubInput)
	}
}
