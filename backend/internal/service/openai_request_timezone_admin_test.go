package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func timezoneAdminFixture() (*adminServiceImpl, *upstreamBillingProbeAccountRepo) {
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
			Credentials: map[string]any{"model_mapping": map[string]any{"alias": "gpt-6-sol"}},
			Extra: map[string]any{"pool": "premium", "tiered_routing_enabled": true,
				"quota_daily_limit": float64(50), "quota_daily_used": float64(12),
				"openai_oauth_responses_websockets_v2_mode": "passthrough",
				codexFingerprintModeExtraKey:                "session", codexFingerprintSeedExtraKey: testCodexFingerprintSeed}},
		2: {ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"custom": "keep"}},
	}}
	return &adminServiceImpl{accountRepo: repo}, repo
}

func TestRequestTimezoneAdminUpdatePreservesCustomSettings(t *testing.T) {
	svc, repo := timezoneAdminFixture()
	before := mergeMap(nil, repo.accounts[1].Extra)
	credentials := mergeMap(nil, repo.accounts[1].Credentials)
	updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "America/New_York",
		openAIRequestTimezoneSyncSearchExtraKey: false,
	}})
	require.NoError(t, err)
	for key, value := range before {
		require.Equal(t, value, updated.Extra[key], key)
	}
	require.Equal(t, credentials, updated.Credentials)
	require.False(t, updated.OpenAIRequestTimezoneEnabled())

	// A normal full edit may intentionally remove quota limits, but must keep the
	// separate timezone policy and the server-managed usage/fingerprint fields.
	updated, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		"pool": "standard", "tiered_routing_enabled": false,
	}})
	require.NoError(t, err)
	require.False(t, updated.OpenAIRequestTimezoneEnabled())
	require.Equal(t, DefaultOpenAIRequestTimezone, updated.OpenAIRequestTimezone())
	require.NotContains(t, updated.Extra, "quota_daily_limit")
	require.Equal(t, float64(12), updated.Extra["quota_daily_used"])
	require.Equal(t, testCodexFingerprintSeed, updated.Extra[codexFingerprintSeedExtraKey])
}

func TestRequestTimezoneAdminRejectsInvalidBeforeWriting(t *testing.T) {
	for _, mode := range []string{"create", "update", "extra", "bulk"} {
		t.Run(mode, func(t *testing.T) {
			svc, repo := timezoneAdminFixture()
			invalid := map[string]any{openAIRequestTimezoneEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "Local"}
			var err error
			switch mode {
			case "create":
				_, err = svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: invalid, SkipDefaultGroupBind: true})
			case "update":
				_, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: invalid})
			case "extra":
				err = svc.UpdateAccountExtra(context.Background(), 1, invalid)
			case "bulk":
				_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Extra: invalid})
			}
			require.Error(t, err)
			require.Len(t, repo.accounts, 2)
			require.Empty(t, repo.updates)
			require.Empty(t, repo.bulkUpdates)
			require.NotContains(t, repo.accounts[1].Extra, openAIRequestTimezoneExtraKey)
		})
	}
}

func TestRequestTimezoneAdminBulkRejectsMixedPlatformsWithoutPartialWrites(t *testing.T) {
	svc, repo := timezoneAdminFixture()
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2}, Extra: map[string]any{openAIRequestTimezoneEnabledExtraKey: true},
	})
	require.Error(t, err)
	require.Empty(t, repo.bulkUpdates)
	require.Empty(t, repo.updates)
}
