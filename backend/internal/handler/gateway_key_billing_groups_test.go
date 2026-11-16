package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMultiGroupKeyBillingDeclarationHasPerGroupRatesWithoutTopLevelScalar(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	groups := []keyBillingGroup{
		{GroupID: 1, GroupName: "Economy", keyBillingInfoResponse: keyBillingInfoResponse{EffectiveRateMultiplier: 0.18}},
		{GroupID: 2, GroupName: "Astra", keyBillingInfoResponse: keyBillingInfoResponse{EffectiveRateMultiplier: 0.3}},
	}
	declaration := buildMultiGroupKeyBillingInfo(groups, now)
	body, err := json.Marshal(declaration)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, float64(2), got["schema_version"])
	require.Equal(t, "multi_group", got["billing_mode"])
	require.Equal(t, 0.18, got["min_rate_multiplier"])
	require.Equal(t, 0.3, got["max_rate_multiplier"])
	require.NotContains(t, got, "resolved_rate_multiplier")
	require.NotContains(t, got, "effective_rate_multiplier")
	require.Len(t, got["groups"], 2)
}

func TestVideoKeyBillingUsesIndependentRateAndMixedScope(t *testing.T) {
	now := time.Now()
	key := &service.APIKey{Group: &service.Group{Platform: service.PlatformVideo, RateMultiplier: .3,
		VideoRateIndependent: true, VideoRateMultiplier: .8, PeakRateEnabled: true, PeakRateMultiplier: 2}}
	video := buildKeyBillingInfo(key, .2, now)
	require.Equal(t, "video", video.BillingScope)
	require.Equal(t, .8, video.EffectiveRateMultiplier)
	require.Nil(t, video.UserRateMultiplier)
	require.False(t, video.PeakRateEnabled)
	key.Group.VideoRateIndependent = false
	shared := buildKeyBillingInfo(key, .2, now)
	require.Equal(t, .2, shared.EffectiveRateMultiplier)
	require.Equal(t, .2, *shared.UserRateMultiplier)
	groups := []keyBillingGroup{{GroupID: 1, keyBillingInfoResponse: video}, {GroupID: 2, keyBillingInfoResponse: shared}}
	require.Equal(t, "video", buildMultiGroupKeyBillingInfo(groups, now).BillingScope)
	groups = append(groups, keyBillingGroup{GroupID: 3, keyBillingInfoResponse: keyBillingInfoResponse{BillingScope: "token", EffectiveRateMultiplier: .5}})
	require.Equal(t, "mixed", buildMultiGroupKeyBillingInfo(groups, now).BillingScope)
}
