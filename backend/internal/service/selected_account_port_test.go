//go:build unit

package service

import (
	"context"
	"math"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestSelectedAccountPortRatePersistence(t *testing.T) {
	for _, rate := range []float64{0, 0.65, 2} {
		repo := &accountRepoStubForBulkUpdate{createID: 41}
		svc := &adminServiceImpl{accountRepo: repo}
		account, err := svc.CreateAccount(t.Context(), &CreateAccountInput{
			Name: "fixture", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			SkipDefaultGroupBind: true, GroupRateMultiplier: &rate,
			Credentials: map[string]any{"api_key": "fixture"}, Extra: map[string]any{"cost_multiplier": .5},
		})
		require.NoError(t, err)
		require.Equal(t, rate, *account.GroupRateMultiplier)
		require.Equal(t, .5, account.Extra["cost_multiplier"])
		edit := &updateAccountCredsRepoStub{account: account}
		updated, err := (&adminServiceImpl{accountRepo: edit}).UpdateAccount(t.Context(), account.ID, &UpdateAccountInput{GroupRateMultiplier: &rate})
		require.NoError(t, err)
		require.Equal(t, rate, *updated.GroupRateMultiplier)
		require.Equal(t, 1, edit.updateCalls)
		_, err = svc.BulkUpdateAccounts(t.Context(), &BulkUpdateAccountsInput{AccountIDs: []int64{41}, GroupRateMultiplier: &rate})
		require.NoError(t, err)
		require.Equal(t, rate, *repo.lastBulkUpdate.GroupRateMultiplier)
	}
}

func TestSelectedAccountPortRejectsInvalidRatesBeforeWrites(t *testing.T) {
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1)} {
		repo := &accountRepoStubForBulkUpdate{createID: 41}
		svc := &adminServiceImpl{accountRepo: repo}
		_, err := svc.CreateAccount(t.Context(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, SkipDefaultGroupBind: true, GroupRateMultiplier: &rate})
		require.Error(t, err)
		require.Nil(t, repo.createAccount)
		edit := &updateAccountCredsRepoStub{account: &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}
		_, err = (&adminServiceImpl{accountRepo: edit}).UpdateAccount(t.Context(), 41, &UpdateAccountInput{GroupRateMultiplier: &rate})
		require.Error(t, err)
		require.Zero(t, edit.updateCalls)
		_, err = svc.BulkUpdateAccounts(t.Context(), &BulkUpdateAccountsInput{AccountIDs: []int64{41}, GroupRateMultiplier: &rate})
		require.Error(t, err)
		require.Zero(t, repo.bulkUpdateCalls)
	}
}

type selectedModelLimitsRepo struct {
	updateAccountCredsRepoStub
	limits map[int64][]string
}

func (r *selectedModelLimitsRepo) SetGroupAllowedModels(_ context.Context, id int64, limits map[int64][]string) error {
	r.limits = limits
	return nil
}

func TestSelectedAccountPortGroupLimitsPersistAndValidateBeforeWrite(t *testing.T) {
	repo := &selectedModelLimitsRepo{updateAccountCredsRepoStub: updateAccountCredsRepoStub{
		account: &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, GroupIDs: []int64{11, 12}},
	}}
	svc := &adminServiceImpl{accountRepo: repo}
	limits := map[int64][]string{11: {"model-a"}, 12: {"model-b"}}
	_, err := svc.UpdateAccount(t.Context(), 8, &UpdateAccountInput{GroupAllowedModels: limits})
	require.NoError(t, err)
	require.Equal(t, limits, repo.limits)
	_, err = svc.UpdateAccount(t.Context(), 8, &UpdateAccountInput{GroupAllowedModels: map[int64][]string{99: {"model-a"}}})
	require.Error(t, err)
	require.Equal(t, 1, repo.updateCalls, "unbound model limits must reject before account write")
	_, err = svc.UpdateAccount(t.Context(), 8, &UpdateAccountInput{GroupAllowedModels: map[int64][]string{}})
	require.NoError(t, err)
	require.Empty(t, repo.limits, "empty map explicitly clears restrictions")
}

func TestSelectedAccountPortGroupModelScheduling(t *testing.T) {
	a := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"model-a": "model-a", "model-b": "model-b", "upstream-a": "upstream-a"}},
		GroupIDs:    []int64{11, 12}, AccountGroups: []AccountGroup{{GroupID: 11, AllowedModels: []string{"model-a"}}, {GroupID: 12, AllowedModels: []string{"model-b"}}},
	}
	g1, g2 := int64(11), int64(12)
	ctx := context.WithValue(t.Context(), ctxkey.Group, &Group{ID: g1})
	gateway := &GatewayService{}
	require.True(t, gateway.isModelSupportedByAccountInGroup(ctx, a, &g1, "model-a"))
	require.False(t, gateway.isModelSupportedByAccountInGroup(ctx, a, &g2, "model-a"), "selected fallback group overrides stale context")
	require.True(t, gateway.isModelSupportedByAccountInGroup(ctx, a, &g2, "model-b"))
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{}}
	allowed, reason := scheduler.isAccountRequestCompatibleReason(ctx, a, OpenAIAccountScheduleRequest{GroupID: &g2, RequestedModel: "model-a"})
	require.False(t, allowed)
	require.Equal(t, "group_model_not_allowed", reason)
	require.Nil(t, scheduler.service.recheckSelectedOpenAIAccountFromDBBeforeProfit(ctx, a, &g2, PlatformOpenAI, "model-a", false, ""), "legacy/sticky recheck must apply limits too")
	composite := WithCompositeRouteDecision(ctx, CompositeRouteDecision{Matched: true, PublicModel: "model-a", UpstreamModel: "upstream-a", TargetPlatform: PlatformOpenAI})
	require.True(t, gateway.isModelSupportedByAccountInGroup(composite, a, &g1, "upstream-a"))
	require.False(t, gateway.isModelSupportedByAccountInGroup(composite, a, &g2, "upstream-a"))
}

func TestSelectedAccountPortVideoCatalogReplacement(t *testing.T) {
	a := modelReplacementAccount()
	a.Platform = PlatformVideo
	require.True(t, a.IsModelSupported("removed-model"))
	repo := &upstreamModelReplacementRepo{rows: 1}
	upstream := &httpUpstreamRecorder{resp: modelReplacementResponse(http.StatusOK, `{"data":[{"id":"video-new"}]}`)}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	catalog, err := svc.ReplaceModelMappingWithUpstream(t.Context(), a)
	require.NoError(t, err)
	require.Equal(t, []string{"video-new"}, catalog.Models)
	require.Len(t, upstream.requests, 1, "video sync must not query language capability registry")
	require.True(t, a.IsModelSupported("video-new"))
	require.False(t, a.IsModelSupported("removed-model"))
	require.Equal(t, "original-key", a.GetCredential("api_key"))
	require.Equal(t, map[string]any{"model_mapping": map[string]any{"video-new": "video-new"}}, repo.updates.Credentials)
	for _, replace := range []bool{false, true} {
		upstream.resp = modelReplacementResponse(http.StatusNotFound, `{}`)
		_, err = svc.syncUpstreamModelCatalog(t.Context(), a, replace)
		require.Error(t, err, "failed video discovery must not silently return configured models")
	}
}

func TestSelectedAccountPortBothSchedulersRespectGroupLimitsBeforeStrictTier(t *testing.T) {
	for _, advanced := range []string{"false", "true"} {
		t.Run(advanced, func(t *testing.T) {
			group, repo := strictFixture()
			for i := 0; i < 2; i++ {
				repo.accounts[i].AccountGroups[0].AllowedModels = []string{"other-model"}
			}
			svc := &OpenAIGatewayService{accountRepo: repo, cache: &schedulerTestGatewayCache{}, cfg: &config.Config{}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(advanced)}
			selected, _, err := svc.SelectAccountWithScheduler(strictContext(group), &group.ID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, int64(3), selected.Account.ID)
			if selected.ReleaseFunc != nil {
				selected.ReleaseFunc()
			}
		})
	}
}
