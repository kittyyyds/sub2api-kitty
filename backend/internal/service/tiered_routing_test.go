package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveTierByRules(t *testing.T) {
	cfg := TieredRoutingSettings{Enabled: true, PremiumUserIDs: []int64{7}, MinBalanceForPremium: 100}
	require.Equal(t, TierPremium, resolveTierByRules(&User{ID: 7}, cfg))
	require.Equal(t, TierPremium, resolveTierByRules(&User{ID: 8, Balance: 150}, cfg))
	require.Equal(t, TierStandard, resolveTierByRules(&User{ID: 8, Balance: 50}, cfg))
	require.Equal(t, TierStandard, resolveTierByRules(nil, cfg))
}

func TestTieredRoutingDisabled(t *testing.T) {
	svc := NewTieredRoutingService(nil)
	require.Equal(t, TierStandard, svc.ResolveUserTier(&User{ID: 7}))
	require.Equal(t, TierStandard, svc.ResolvePool(&User{ID: 7, Balance: 9999}))
}

func TestAccountPool(t *testing.T) {
	require.Equal(t, TierStandard, AccountPool(nil))
	require.Equal(t, TierStandard, AccountPool(&Account{}))
	require.Equal(t, TierPremium, AccountPool(&Account{Extra: map[string]any{"pool": "premium"}}))
	require.Equal(t, TierPremium, AccountPool(&Account{Extra: map[string]any{"pool": " Premium "}}))
	require.Equal(t, TierStandard, AccountPool(&Account{Extra: map[string]any{"pool": "gold"}}))
	require.Equal(t, TierStandard, AccountPool(&Account{Extra: map[string]any{"pool": "premium", AccountTieredRoutingEnabledExtraKey: false}}))
	require.Equal(t, TierPremium, AccountPool(&Account{Extra: map[string]any{"pool": "premium", AccountTieredRoutingEnabledExtraKey: true}}))
	require.Equal(t, TierStandard, AccountPool(&Account{Extra: map[string]any{"pool": "standard", AccountTieredRoutingEnabledExtraKey: true}}))
	require.Equal(t, TierStandard, AccountPool(&Account{Extra: map[string]any{"pool": "premium", AccountTieredRoutingEnabledExtraKey: "false"}}))
}

func TestTierPoolAccountSwitchOverridesSavedPreferredPool(t *testing.T) {
	accounts := []Account{
		{ID: 1, Extra: map[string]any{"pool": "premium", AccountTieredRoutingEnabledExtraKey: false}},
		{ID: 2, Extra: map[string]any{"pool": "premium", AccountTieredRoutingEnabledExtraKey: true}},
		{ID: 3, Extra: map[string]any{"pool": "standard", AccountTieredRoutingEnabledExtraKey: true}},
	}
	require.Equal(t, []Account{accounts[1]}, FilterTierPoolAccounts(accounts, TierPremium))
	require.Equal(t, accounts, FilterTierPoolAccounts(accounts, TierStandard))
	accounts[1].Extra[AccountTieredRoutingEnabledExtraKey] = false
	require.Equal(t, accounts, FilterTierPoolAccounts(accounts, TierPremium))
	require.Equal(t, "premium", accounts[0].Extra[AccountPoolExtraKey])
}

func TestFilterTierPoolAccounts(t *testing.T) {
	accounts := []Account{
		{ID: 1, Extra: map[string]any{"pool": "premium"}},
		{ID: 2},
	}
	// standard 不过滤
	require.Len(t, FilterTierPoolAccounts(accounts, TierStandard), 2)
	// premium 命中池
	got := FilterTierPoolAccounts(accounts, TierPremium)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
	// 池空时回退全量
	require.Len(t, FilterTierPoolAccounts(accounts[1:], TierPremium), 1)
	// 空输入
	require.Empty(t, FilterTierPoolAccounts(nil, TierPremium))
}

func TestTierPoolContext(t *testing.T) {
	require.Equal(t, TierStandard, TierPoolFromContext(nil))
	require.Equal(t, TierStandard, TierPoolFromContext(context.Background()))
	ctx := WithTierPool(context.Background(), TierPremium)
	require.Equal(t, TierPremium, TierPoolFromContext(ctx))
}

func TestTieredRoutingSettingsRoundtrip(t *testing.T) {
	repo := &stubTieredRoutingSettingRepo{}
	svc := NewTieredRoutingService(repo)
	got := svc.GetSettings(context.Background())
	require.False(t, got.Enabled)

	updated, err := svc.UpdateSettings(context.Background(), TieredRoutingSettings{
		Enabled: true, PremiumUserIDs: []int64{1, 1, -5}, MinBalanceForPremium: -1,
	})
	require.NoError(t, err)
	require.True(t, updated.Enabled)
	require.Equal(t, []int64{1}, updated.PremiumUserIDs)
	require.Equal(t, 0.0, updated.MinBalanceForPremium)
}

type stubTieredRoutingSettingRepo struct {
	values map[string]string
}

func (s *stubTieredRoutingSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	return &Setting{Key: key, Value: s.values[key]}, nil
}

func (s *stubTieredRoutingSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *stubTieredRoutingSettingRepo) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *stubTieredRoutingSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	return nil, nil
}

func (s *stubTieredRoutingSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	return nil
}

func (s *stubTieredRoutingSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *stubTieredRoutingSettingRepo) Delete(_ context.Context, _ string) error { return nil }
