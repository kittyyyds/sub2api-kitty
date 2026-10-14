package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficialAccountDefaultsCreateAndUpdate(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{accounts: make(map[int64]*Account)}
			svc := &adminServiceImpl{accountRepo: repo}
			proxyID := int64(3)
			creds := map[string]any{"access_token": "fixture-token", "refresh_token": "fixture-refresh"}
			account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
				Name: "official-defaults", Platform: platform, Type: AccountTypeOAuth,
				Credentials: creds, ProxyID: &proxyID, Concurrency: 15, SkipDefaultGroupBind: true,
			})
			require.NoError(t, err)
			require.Equal(t, codexFingerprintOff, account.GetCodexFingerprintMode())
			require.False(t, account.IsTLSFingerprintEnabled())
			for _, key := range []string{"anti_degradation", "anti_degrade", "fingerprint_v2_seed", "client_installation_id", "tls_fingerprint_builtin", "account_traffic_policy"} {
				require.NotContains(t, account.Extra, key)
			}
			// A name-only edit must not reinstall the retired defaults.
			updated, err := svc.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Name: "renamed"})
			require.NoError(t, err)
			require.Equal(t, codexFingerprintOff, updated.GetCodexFingerprintMode())
			require.False(t, updated.IsTLSFingerprintEnabled())
			require.Equal(t, creds, updated.Credentials)
			require.Equal(t, &proxyID, updated.ProxyID)
			require.Equal(t, 15, updated.Concurrency)
		})
	}
}

type officialTestSlotCache struct {
	ConcurrencyCache
	active, lastLimit int
	lastID            int64
}

func (c *officialTestSlotCache) AcquireAccountSlot(_ context.Context, id int64, limit int, _ string) (bool, error) {
	c.lastID, c.lastLimit = id, limit
	if c.active >= limit {
		return false, nil
	}
	c.active++
	return true, nil
}
func (c *officialTestSlotCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.active--
	return nil
}

func TestOfficialAccountTestsShareBusinessCapacity(t *testing.T) {
	cache := &officialTestSlotCache{}
	concurrency := NewConcurrencyService(cache)
	svc := &AccountTestService{concurrencyService: concurrency}
	a := &Account{ID: 9, Concurrency: 1, Extra: map[string]any{
		"anti_degradation": true, "anti_degrade": map[string]any{"enabled": true, "mode": "mode1", "concurrency": 15},
	}}
	business, err := concurrency.AcquireAccountSlot(context.Background(), a.ID, a.Concurrency)
	require.NoError(t, err)
	require.True(t, business.Acquired)
	_, err = svc.acquireTestAccountSlot(context.Background(), a)
	require.ErrorIs(t, err, ErrAccountTestBusy)
	require.Equal(t, 1, cache.lastLimit)
	business.ReleaseFunc()
	release, err := svc.acquireTestAccountSlot(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, a.ID, cache.lastID)
	release()
	require.Zero(t, cache.active)
}
