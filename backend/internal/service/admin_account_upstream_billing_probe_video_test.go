package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestVideoProbeAccountSaveDoesNotDependOnSupplierAvailability(t *testing.T) {
	for _, base := range []string{"https://suanliai.top/v1", "https://unadapted.example/v1"} {
		t.Run(base, func(t *testing.T) {
			enabled := true
			repo := &accountBillingSettingsAdminRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{}}
			admin := &adminServiceImpl{accountRepo: repo}
			created, err := admin.CreateAccount(context.Background(), &CreateAccountInput{
				Name: "video", Platform: PlatformVideo, Type: AccountTypeAPIKey,
				Credentials:  map[string]any{"base_url": base, "api_key": "fixture-key"},
				ProbeEnabled: &enabled, SkipDefaultGroupBind: true,
				Extra: map[string]any{UpstreamBillingProbeExtraKey: map[string]any{"status": "ok"}},
			})
			require.NoError(t, err)
			require.True(t, created.Extra[UpstreamBillingProbeEnabledExtraKey].(bool))
			require.NotContains(t, created.Extra, UpstreamBillingProbeExtraKey)
			loaded, err := repo.GetByID(context.Background(), created.ID)
			require.NoError(t, err)
			require.True(t, loaded.Extra[UpstreamBillingProbeEnabledExtraKey].(bool))

			upstream := &httpUpstreamRecorder{resp: videoProbeHTTPResponse(http.StatusServiceUnavailable, "fixture failure")}
			probe := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
			snapshot, err := probe.ProbeAccount(context.Background(), created.ID)
			require.NoError(t, err)
			require.Contains(t, []string{UpstreamBillingProbeStatusFailed, UpstreamBillingProbeStatusUnsupported}, snapshot.Status)
			require.Equal(t, UpstreamBillingProbeKindVideoModelPrices, snapshot.Kind)

			updated, err := admin.UpdateAccount(context.Background(), created.ID, &UpdateAccountInput{Name: "saved after price failure", ProbeEnabled: &enabled})
			require.NoError(t, err)
			require.Equal(t, "saved after price failure", updated.Name)
			require.True(t, updated.Extra[UpstreamBillingProbeEnabledExtraKey].(bool))
			require.Equal(t, false, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
			loaded, err = repo.GetByID(context.Background(), created.ID)
			require.NoError(t, err)
			require.Equal(t, updated.Extra, loaded.Extra)
			require.Equal(t, 1, repo.updateCalls)
		})
	}
}

func TestVideoProbeAccountUpdateRejectsRateSyncBeforeWrite(t *testing.T) {
	account := videoProbeFixtureAccount()
	repo := &accountBillingSettingsAdminRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}}
	admin := &adminServiceImpl{accountRepo: repo}
	enabled := true
	_, err := admin.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Name: "must not persist", RateSyncEnabled: &enabled})
	require.ErrorIs(t, err, ErrVideoBillingRateSyncUnsupported)
	status, _ := infraerrors.ToHTTP(err)
	require.Equal(t, http.StatusBadRequest, status)
	require.Zero(t, repo.updateCalls)
	require.NotEqual(t, "must not persist", account.Name)

	// An unrelated save disables a legacy rate-sync flag without moving rates.
	updated, err := admin.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{Name: "saved"})
	require.NoError(t, err)
	require.Equal(t, false, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
	require.Equal(t, 0.7, *updated.RateMultiplier)
	require.Equal(t, 0.4, updated.Extra[AccountCostMultiplierExtraKey])

	// Both normal and legacy extra-based probe flags remain supported.
	for _, enabled := range []bool{false, true} {
		updated, err = admin.UpdateAccount(context.Background(), account.ID, &UpdateAccountInput{
			Extra: map[string]any{UpstreamBillingProbeEnabledExtraKey: enabled},
		})
		require.NoError(t, err)
		require.Equal(t, enabled, updated.Extra[UpstreamBillingProbeEnabledExtraKey])
		require.Equal(t, false, updated.Extra[UpstreamBillingRateSyncEnabledExtraKey])
	}
}

func TestVideoProbeMixedPlatformBulkToggle(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			1: {ID: 1, Platform: PlatformVideo, Type: AccountTypeAPIKey},
			2: {ID: 2, Platform: PlatformGrok, Type: AccountTypeAPIKey},
		}}
		result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
			AccountIDs: []int64{1, 2}, ProbeEnabled: &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, 2, result.Success)
		require.Len(t, repo.bulkUpdates, 1)
		require.Equal(t, enabled, repo.bulkUpdates[0].Extra[UpstreamBillingProbeEnabledExtraKey])
		// Raw extra cannot inject a video rate-sync flag or a price snapshot.
		repo.bulkUpdates = nil
		_, err = (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
			AccountIDs: []int64{1}, Extra: map[string]any{
				UpstreamBillingRateSyncEnabledExtraKey: true,
				UpstreamBillingProbeExtraKey:           map[string]any{"kind": "video_model_prices", "status": "ok"},
			},
		})
		require.NoError(t, err)
		for _, update := range repo.bulkUpdates {
			require.NotContains(t, update.Extra, UpstreamBillingRateSyncEnabledExtraKey)
			require.NotContains(t, update.Extra, UpstreamBillingProbeExtraKey)
		}
	}
}
