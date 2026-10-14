package handler

import (
	"testing"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

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
