package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCanvasVideoQuotePerRequestResolutionTier(t *testing.T) {
	unit := 6.35
	model := "request-tier-fixture"
	group := &service.Group{Platform: service.PlatformVideo, RateMultiplier: 20,
		ModelPricing: []service.ChannelModelPricing{{Models: []string{model}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &unit,
			Video: &service.VideoModelConfig{
				ResolutionPrices: map[string]float64{"480p": 6.35, "720p": 8.35},
				Spec:             service.UnifiedVideoSpec{Supported: true, MinSeconds: 4, MaxSeconds: 30, Resolutions: []string{"480p", "720p"}, Ratios: []string{"16:9"}},
			}}}}
	for _, seconds := range []int{4, 30} {
		quote, err := quoteVideoInput(context.Background(), &service.APIKey{Group: group}, videoQuoteInput{
			UnifiedVideoInput: service.UnifiedVideoInput{Model: model, Seconds: seconds, Resolution: "720p", AspectRatio: "16:9"},
		})
		require.NoError(t, err)
		require.Equal(t, 8.35, quote.EstimatedCost)
		require.Equal(t, 8.35, quote.UnitPrice)
		require.Equal(t, "条", quote.Unit)
		require.True(t, quote.FinalUnitPrice)
	}
}

func TestCanvasVideoQuoteFinalPriceAndDuration(t *testing.T) {
	unit := .75
	model := "官转稳定sd2.0(933原生过真人720P)"
	group := &service.Group{Platform: service.PlatformVideo, RateMultiplier: 20,
		ModelPricing: []service.ChannelModelPricing{{Models: []string{model}, BillingMode: service.BillingModeVideo, PerRequestPrice: &unit}}}
	quote, err := canvasVideoQuote(group, model, 5, 1)
	require.NoError(t, err)
	require.Equal(t, 3.75, quote.EstimatedCost)
	require.Equal(t, "秒", quote.Unit)
	group.ModelPricing[0].BillingMode = service.BillingModePerRequest
	quote, err = canvasVideoQuote(group, model, 15, 1)
	require.NoError(t, err)
	require.Equal(t, .75, quote.EstimatedCost)
	require.Equal(t, "条", quote.Unit)
	_, err = canvasVideoQuote(group, model, 30, 1)
	require.Error(t, err)
	_, err = canvasVideoQuote(group, "unpriced", 5, 1)
	require.Error(t, err)
}
