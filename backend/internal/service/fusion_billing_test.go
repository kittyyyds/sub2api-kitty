//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFusionReservationIncludesTokenSurchargeButNotSearchOrFinalMedia(t *testing.T) {
	svc := newInflightEstimateGateway(t, nil)
	svc.settingService = modelBillingSettings(t, ModelBillingConfig{SchemaVersion: 2, Enabled: true, Rules: []ModelBillingRule{{Model: "gpt-6-luna*", Multiplier: 10, GroupID: 71, Enabled: true, Mode: ModelBillingAlways}, {Model: "banana*", Multiplier: 100, GroupID: 71, Enabled: true, Mode: ModelBillingAlways}}})
	group := &Group{ID: 71, Platform: PlatformOpenAI, RateMultiplier: 2}
	key := &APIKey{User: &User{ID: 1}, Group: group, GroupID: &group.ID}
	req := InflightEstimateRequest{Model: "gpt-6-luna", BodyBytes: 4000, MaxTokens: 500, SearchCalls: 2}
	got, ok := svc.EstimateInflightReservation(t.Context(), key, req)
	require.True(t, ok)
	base, err := svc.billingService.CalculateCost(req.Model, UsageTokens{InputTokens: 1000, OutputTokens: 500}, 2)
	require.NoError(t, err)
	search := svc.billingService.CalculateSearchCost(2, nil, 2)
	require.InDelta(t, base.ActualCost*10+search.ActualCost, got, 1e-12)
	group.ImageModelPrices = map[string]map[string]float64{"banana": {"1K": .06, "2K": .08, "4K": .1}}
	for _, rate := range []float64{0, 7} {
		group.RateMultiplier = rate
		got, ok = svc.EstimateInflightReservation(t.Context(), key, InflightEstimateRequest{Model: "banana", Kind: InflightEstimateImage, Units: 2})
		require.True(t, ok)
		require.InDelta(t, .2, got, 1e-12)
	}
	group.ImageModelPrices["banana"] = map[string]float64{"1K": 0, "2K": 0, "4K": 0}
	got, ok = svc.EstimateInflightReservation(t.Context(), key, InflightEstimateRequest{Model: "banana", Kind: InflightEstimateImage})
	require.True(t, ok)
	require.Zero(t, got)
	group.RateMultiplier = 0
	group.ImageModelPrices["banana"] = map[string]float64{"4K": .1}
	got, ok = svc.EstimateInflightReservation(t.Context(), key, InflightEstimateRequest{Model: "banana", Kind: InflightEstimateImage})
	require.True(t, ok)
	require.InDelta(t, .1, got, 1e-12, "partial final cells must survive zero generic rates")
}

func TestFusionVideoReservationSurvivesHTTPAndRestartUntilSettlement(t *testing.T) {
	svc, repo, key, account, body := newVideoTaskFixture()
	cache := newMemInflightCache(2)
	svc.billingCacheService = newInflightSvc(t, cache, 60)
	key.Group.RateMultiplier = 0
	task, err := svc.BeginVideoTask(t.Context(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.NoError(t, err)
	require.NotNil(t, task.Pending.InflightReservation)
	require.Equal(t, 1, cache.count())
	id := task.Pending.InflightReservation.RequestID
	require.InDelta(t, 1.3, cache.res[id], 1e-12, "reserve the immutable final quote")
	require.Greater(t, time.Until(cache.exp[id]), 6*24*time.Hour)
	_, err = svc.billingCacheService.ReserveInflight(t.Context(), key.User, key.Group, nil, 1)
	require.ErrorIs(t, err, ErrInsufficientBalance)
	svc.MarkVideoSubmissionUnknown(t.Context(), task)
	require.Equal(t, 1, cache.count(), "unknown results retain the reservation")
	raw, err := json.Marshal(task.Pending)
	require.NoError(t, err)
	var restored GrokVideoPendingBilling
	require.NoError(t, json.Unmarshal(raw, &restored))
	recovered := *task
	recovered.Pending = restored
	recovered.UpstreamTaskID = "completed-after-restart"
	repo.billed, repo.usageRecorded = true, true
	require.NoError(t, svc.SettleVideoTask(t.Context(), &recovered, &OpenAIForwardResult{VideoCount: 1}, nil))
	require.Equal(t, "done", repo.state)
	require.Zero(t, cache.count())
}

func TestFusionVideoReservationReleasesOnlyConfirmedFailure(t *testing.T) {
	svc, repo, key, account, body := newVideoTaskFixture()
	cache := newMemInflightCache(2)
	svc.billingCacheService = newInflightSvc(t, cache, 60)
	repo.createErr = errors.New("database unavailable before submission")
	_, err := svc.BeginVideoTask(t.Context(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.Error(t, err)
	require.Zero(t, cache.count())
	repo.createErr = nil
	task, err := svc.BeginVideoTask(t.Context(), key, account, nil, "特价/sd-2.5", GrokMediaEndpointVideosGenerations, body, "application/json")
	require.NoError(t, err)
	require.NoError(t, svc.finishVideoTask(t.Context(), task, "failed", "supplier confirmed failure"))
	require.Zero(t, cache.count())
}

func TestFusionTransferredReservationDoesNotReleaseOnHandlerReturn(t *testing.T) {
	cache := newMemInflightCache(2)
	svc := newInflightSvc(t, cache, 60)
	r, err := svc.ReserveInflight(context.Background(), &User{ID: 1}, nil, nil, 1)
	require.NoError(t, err)
	ref, err := r.transferToVideo(24 * time.Hour)
	require.NoError(t, err)
	r.HandlerDone()
	r.HandlerDone()
	require.Equal(t, 1, cache.count())
	require.NoError(t, svc.releaseVideoInflight(t.Context(), 1, ref))
	require.Zero(t, cache.count())
}
