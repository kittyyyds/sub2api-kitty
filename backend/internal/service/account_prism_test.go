package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func prismTestConfig() *config.Config {
	return &config.Config{Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: true}}
}

func prismTestAccount() *Account {
	return &Account{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "fake-oauth-access", "refresh_token": "fake-oauth-refresh"},
		Extra:       map[string]any{PrismEnabledExtraKey: true, PrismModelsExtraKey: []string{"gpt-6-astra"}}}
}

type prismSettingsTestRepo struct {
	*upstreamBillingProbeAccountRepo
	edits           []PrismCredentialsUpdate
	ordinaryUpdates int
}

func (r *prismSettingsTestRepo) Update(ctx context.Context, account *Account) error {
	r.ordinaryUpdates++
	return r.upstreamBillingProbeAccountRepo.Update(ctx, account)
}

func (r *prismSettingsTestRepo) UpdateWithPrismAccountSettings(ctx context.Context, account *Account, edit PrismCredentialsUpdate, _ *bool, _ *bool, _ *float64) error {
	r.edits = append(r.edits, edit)
	return r.upstreamBillingProbeAccountRepo.Update(ctx, account)
}

func TestPrismCookieEncryptionAndReadBoundary(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	cfg := prismTestConfig()
	a := prismTestAccount()
	input := map[string]any{PrismCookieKey: "session=synthetic-cookie-canary"}
	a.Credentials[PrismCookieKey] = input[PrismCookieKey]
	require.NoError(t, preparePrismAccountForSave(a, nil, input, cfg))
	require.NotContains(t, a.Credentials, PrismCookieKey)
	encrypted := a.GetPrismCookieCiphertext()
	require.NotContains(t, encrypted, "synthetic-cookie-canary")
	plain, err := DecryptPrismCookie(a, cfg)
	require.NoError(t, err)
	require.Equal(t, input[PrismCookieKey], plain)
	require.Equal(t, "fake-oauth-access", a.GetCredential("access_token"))
	require.Equal(t, "session=synthetic-cookie-canary", input[PrismCookieKey], "caller input must not be modified")
	other, err := encryptPrismCookie(plain, cfg)
	require.NoError(t, err)
	require.NotEqual(t, encrypted, other, "every write must use a fresh nonce")

	wrongKey := prismTestConfig()
	wrongKey.Totp.EncryptionKey = strings.Repeat("43", 32)
	_, err = DecryptPrismCookie(a, wrongKey)
	require.Error(t, err)
	require.NotContains(t, err.Error(), plain)
	a.Credentials[PrismCookieEncryptedKey] = encrypted[:len(encrypted)-3] + "AAA"
	_, err = DecryptPrismCookie(a, cfg)
	require.Error(t, err)
}

func TestPrismBindingValidation(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	for _, mutate := range []func(*Account){
		func(a *Account) { a.Platform = PlatformGrok },
		func(a *Account) { a.Type = AccountTypeAPIKey },
		func(a *Account) { id := int64(3); a.ParentAccountID = &id },
		func(a *Account) { a.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity },
		func(a *Account) { a.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken },
	} {
		a := prismTestAccount()
		mutate(a)
		require.False(t, a.IsPrismEnabled())
		require.Error(t, preparePrismAccountForSave(a, nil, map[string]any{PrismCookieKey: "session=fake"}, prismTestConfig()))
	}
	a := prismTestAccount()
	require.ErrorContains(t, preparePrismAccountForSave(a, nil, nil, prismTestConfig()), "Bind this GPT account")
	for _, incoming := range []map[string]any{
		{PrismCookieKey: "secret\r\nInjected: true"},
		{PrismCookieKey: 123},
		{PrismCookieEncryptedKey: "spoofed"},
		{PrismProjectIDKey: "shared-project"},
		{PrismBaseURLKey: "https://untrusted.invalid"},
	} {
		require.Error(t, validatePrismInputCredentials(incoming))
	}
	a.Extra[PrismCookieKey] = "secret"
	require.Error(t, preparePrismAccountForSave(a, nil, nil, prismTestConfig()))
	_, err := encryptPrismCookie("session=fake", &config.Config{})
	require.ErrorContains(t, err, "persistent configured encryption key")
}

func TestPrismFeatureGateDefaultsClosed(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "")
	require.False(t, PrismFeatureEnabled())
	a := prismTestAccount()
	require.ErrorContains(t, preparePrismAccountForSave(a, nil, map[string]any{PrismCookieKey: "session=fake"}, prismTestConfig()), "disabled")
}

func TestPrismMappedScopeAndExcelConflict(t *testing.T) {
	a := prismTestAccount()
	a.Credentials["model_mapping"] = map[string]any{"alias": "gpt-6-astra"}
	require.True(t, a.IsPrismEnabledForModel("alias"))
	require.False(t, a.IsPrismEnabledForModel("gpt-6-sol"))
	require.True(t, a.isPrismUpstreamModelEnabled("gpt-6-astra"))
	a.Extra["openai_excel_bps"] = true
	a.Extra["openai_excel_bps_models"] = []string{"gpt-6-sol"}
	require.NoError(t, validatePrismProtocolConflict(a))
	a.Extra["openai_excel_bps_models"] = []string{"gpt-6-astra"}
	require.ErrorContains(t, validatePrismProtocolConflict(a), "same model")
	delete(a.Extra, "openai_excel_bps_models")
	require.Error(t, validatePrismProtocolConflict(a))
	a.Extra[PrismModelsExtraKey] = []string{}
	require.NoError(t, validatePrismProtocolConflict(a))
	require.False(t, a.IsPrismEnabledForModel("alias"))
	delete(a.Extra, PrismModelsExtraKey)
	require.True(t, a.isPrismAllModelsEnabled())
	require.False(t, a.IsOpenAIResponsesWebSocketV2Enabled())
	require.Equal(t, OpenAIWSIngressModeOff, a.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	require.True(t, a.IsOpenAIWSForceHTTPEnabled())
}

func TestPrismAdminUpdateEncryptsAndPreservesBinding(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	a := prismTestAccount()
	delete(a.Extra, PrismEnabledExtraKey)
	repo := &prismSettingsTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}}
	svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{
		Credentials: map[string]any{PrismCookieKey: "session=synthetic-bound-session"},
		Extra:       map[string]any{PrismEnabledExtraKey: true, PrismModelsExtraKey: []string{"gpt-6-astra"}},
	})
	require.NoError(t, err)
	require.NotContains(t, updated.Credentials, PrismCookieKey)
	require.Equal(t, "fake-oauth-refresh", updated.GetCredential("refresh_token"))
	encrypted := updated.GetPrismCookieCiphertext()
	require.NotEmpty(t, encrypted)
	// Re-auth and Codex import merge the server snapshot before calling Update.
	mergedReauth := shallowCopyMap(updated.Credentials)
	mergedReauth["access_token"] = "fake-new-oauth"
	updated, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: mergedReauth})
	require.NoError(t, err)
	require.Equal(t, encrypted, updated.GetPrismCookieCiphertext())
	_, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: map[string]any{PrismCookieEncryptedKey: "replaced-cipher"}})
	require.Error(t, err)
	updated, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: map[string]any{"model_mapping": map[string]any{"public": "gpt-6-astra"}}})
	require.NoError(t, err)
	require.Equal(t, encrypted, updated.GetPrismCookieCiphertext())
	require.NoError(t, persistAccountCredentials(context.Background(), repo, updated, map[string]any{"access_token": "fresh"}))
	require.Equal(t, encrypted, repo.accounts[a.ID].GetPrismCookieCiphertext())
	_, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: map[string]any{PrismCookieKey: ""}})
	require.Error(t, err, "clearing the cookie while enabled must fail")
	require.Equal(t, encrypted, repo.accounts[a.ID].GetPrismCookieCiphertext(), "failed edit must not mutate stored snapshot")
	updated, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: map[string]any{PrismCookieKey: ""}, Extra: map[string]any{PrismEnabledExtraKey: false}})
	require.NoError(t, err)
	require.Empty(t, updated.GetPrismCookieCiphertext())
}

func TestPrismBindingOnlyEditsPreserveAccountConfiguration(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	for _, tc := range []struct {
		name     string
		incoming map[string]any
		unlink   bool
	}{
		{"cookie", map[string]any{PrismCookieKey: "session=replacement-fixture"}, false},
		{"user-id", map[string]any{PrismUserIDKey: "saved-user"}, false},
		{"unlink", map[string]any{PrismCookieKey: ""}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := prismTestAccount()
			a.Credentials["expires_at"] = "2026-10-01T00:00:00Z"
			a.Credentials["model_mapping"] = map[string]any{"alias": "gpt-6-astra"}
			a.Credentials["custom"] = "preserve-me"
			a.Credentials[PrismUserIDKey] = "saved-user"
			ciphertext, err := encryptPrismCookie("session=original-fixture", prismTestConfig())
			require.NoError(t, err)
			a.Credentials[PrismCookieEncryptedKey] = ciphertext
			original, err := cloneAccountJSONMap(a.Credentials)
			require.NoError(t, err)
			repo := &prismSettingsTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}}
			svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
			input := &UpdateAccountInput{Credentials: tc.incoming}
			if tc.unlink {
				input.Extra = map[string]any{PrismEnabledExtraKey: false}
			}
			updated, err := svc.UpdateAccount(context.Background(), a.ID, input)
			require.NoError(t, err)
			require.Len(t, repo.edits, 1)
			require.Zero(t, repo.ordinaryUpdates)
			require.Equal(t, original, repo.edits[0].Original)
			require.Equal(t, original, a.Credentials, "the editor must not mutate its read baseline")
			for _, key := range []string{"access_token", "refresh_token", "expires_at", "model_mapping", "custom"} {
				require.Equal(t, original[key], updated.Credentials[key], key)
			}
			for key := range tc.incoming {
				require.Contains(t, repo.edits[0].ExplicitKeys, key)
			}
			if _, supplied := tc.incoming[PrismCookieKey]; supplied {
				require.Contains(t, repo.edits[0].ExplicitKeys, PrismCookieEncryptedKey)
			}
			if tc.unlink {
				require.NotContains(t, updated.Credentials, PrismCookieEncryptedKey)
			} else if tc.name == "cookie" {
				plain, err := DecryptPrismCookie(updated, prismTestConfig())
				require.NoError(t, err)
				require.Equal(t, tc.incoming[PrismCookieKey], plain)
			} else {
				require.Equal(t, ciphertext, updated.GetPrismCookieCiphertext())
			}
			updated.Credentials["model_mapping"].(map[string]any)["alias"] = "changed-after-save"
			require.Equal(t, "gpt-6-astra", repo.edits[0].Original["model_mapping"].(map[string]any)["alias"], "baseline must not alias nested desired values")
		})
	}
}

func TestPrismAdminUpdateKeepsExplicitUnlinkAndTokenIntent(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	a := prismTestAccount()
	a.Extra = map[string]any{}
	repo := &prismSettingsTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}}
	svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
	_, err := svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{
		Credentials: map[string]any{PrismCookieKey: "", "access_token": "fake-oauth-access"},
	})
	require.NoError(t, err)
	require.Len(t, repo.edits, 1)
	require.Zero(t, repo.ordinaryUpdates)
	require.ElementsMatch(t, []string{PrismCookieKey, PrismCookieEncryptedKey, "access_token"}, repo.edits[0].ExplicitKeys)
	require.NotContains(t, repo.edits[0].Original, PrismCookieEncryptedKey)
	require.Equal(t, "fake-oauth-access", repo.edits[0].Original["access_token"])
}

func TestPrismAdminUpdateLeavesOrdinaryAndBPSRoutingUnchanged(t *testing.T) {
	for _, bps := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "bps"}[bps], func(t *testing.T) {
			a := prismTestAccount()
			a.Extra = map[string]any{}
			if bps {
				a.Extra["openai_excel_bps"] = true
			}
			repo := &prismSettingsTestRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}}
			svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
			updated, err := svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Name: "renamed"})
			require.NoError(t, err)
			require.Equal(t, "renamed", updated.Name)
			require.Empty(t, repo.edits)
			require.Equal(t, 1, repo.ordinaryUpdates)
			require.Equal(t, bps, updated.IsExcelBPSEnabled())
		})
	}
}

func TestPrismRefreshAfterCookieUnlinkPreservesUserIDWithoutPanic(t *testing.T) {
	a := prismTestAccount()
	delete(a.Extra, PrismEnabledExtraKey)
	a.Credentials = map[string]any{PrismUserIDKey: "synthetic-user"}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}
	require.NoError(t, persistAccountCredentials(context.Background(), repo, a, nil))
	require.Equal(t, "synthetic-user", repo.accounts[a.ID].GetPrismUserID())
	require.Empty(t, repo.accounts[a.ID].GetPrismCookieCiphertext())
}

func TestPrismAdminCreateEncryptsBeforeRepositoryWrite(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{}}
	svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
	a, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "local-prism-test", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "fake-oauth", PrismCookieKey: "session=synthetic-create-canary"},
		Extra:       map[string]any{PrismEnabledExtraKey: true}, SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)
	require.NotContains(t, repo.accounts[a.ID].Credentials, PrismCookieKey)
	require.NotEmpty(t, repo.accounts[a.ID].GetPrismCookieCiphertext())
	require.Equal(t, "fake-oauth", repo.accounts[a.ID].GetCredential("access_token"))
	require.Error(t, persistAccountCredentials(context.Background(), repo, a, map[string]any{PrismCookieKey: "raw-not-allowed"}))
}

func TestPrismBulkAndExtraCannotBypassBinding(t *testing.T) {
	t.Setenv("SUB2API_PRISM_ENABLED", "true")
	a := prismTestAccount()
	require.NoError(t, preparePrismAccountForSave(a, nil, map[string]any{PrismCookieKey: "session=fake"}, prismTestConfig()))
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{a.ID: a}}
	svc := &adminServiceImpl{cfg: prismTestConfig(), accountRepo: repo}
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{a.ID}, Credentials: map[string]any{PrismCookieKey: "not-for-bulk"}})
	require.Error(t, err)
	require.Error(t, svc.UpdateAccountExtra(context.Background(), a.ID, map[string]any{PrismEnabledExtraKey: true}))
	require.Error(t, svc.UpdateAccountExtra(context.Background(), a.ID, map[string]any{"openai_excel_bps": true}))
	_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{a.ID}, Extra: map[string]any{"openai_excel_bps": true}})
	require.Error(t, err)
	require.False(t, repo.accounts[a.ID].IsExcelBPSEnabled())
	legacy := NewAccountService(repo, nil)
	_, err = legacy.Create(context.Background(), CreateAccountRequest{Credentials: map[string]any{PrismCookieKey: "cannot-persist-raw"}})
	require.Error(t, err)
	name := "renamed"
	_, err = legacy.Update(context.Background(), a.ID, UpdateAccountRequest{Name: &name})
	require.Error(t, err, "legacy service cannot bypass the encrypted binding editor")
}

func TestPrismInvalidCredentialShapesDoNotPanic(t *testing.T) {
	for _, value := range []any{nil, true, 42, []string{"bad"}, map[string]any{"nested": true}} {
		for _, key := range []string{PrismCookieKey, PrismCookieEncryptedKey, PrismBaseURLKey, PrismProjectIDKey} {
			require.NotPanics(t, func() { _ = validatePrismInputCredentials(map[string]any{key: value}) })
		}
		require.NotPanics(t, func() { _, _ = prismCredentialsForUpdate(map[string]any{PrismCookieEncryptedKey: value}, nil) })
	}
}
