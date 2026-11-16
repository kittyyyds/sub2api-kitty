package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAndLookupImageModelPricesKeepsModelsIndependent(t *testing.T) {
	t.Parallel()
	raw := map[string]map[string]float64{
		"grok-imagine":               {"1k": 0.011, "2K": 0.012},
		"grok-imagine-image":         {"1K": 0.021, "4k": 0.024},
		"grok-imagine-image-quality": {"2K": 0.072},
		"xai/grok-imagine-image-2.0": {"1K": 0, "8K": 0.99, "4K": -1},
	}

	normalized := NormalizeImageModelPrices(raw)
	require.Equal(t, map[string]map[string]float64{
		"grok-imagine":               {ImageBillingSize1K: 0.011, ImageBillingSize2K: 0.012},
		"grok-imagine-image":         {ImageBillingSize1K: 0.021, ImageBillingSize4K: 0.024},
		"grok-imagine-image-quality": {ImageBillingSize2K: 0.072},
		"grok-imagine-image-2.0":     {ImageBillingSize1K: 0},
	}, normalized)

	price := LookupImageModelPrice(normalized, "grok-imagine", "1K")
	require.NotNil(t, price)
	require.InDelta(t, 0.011, *price, 1e-12)
	require.Nil(t, LookupImageModelPrice(normalized, "grok-imagine-image", "2K"))
	require.Nil(t, LookupImageModelPrice(normalized, "grok-imagine-image-quality", "1K"))

	free := LookupImageModelPrice(normalized, "models/grok-imagine-image-2.0", "1K")
	require.NotNil(t, free)
	require.Zero(t, *free)
}

func TestImageModelUSDPriceOverridesFlatPriceAndMissingTierFallsBack(t *testing.T) {
	t.Parallel()
	flat1K := 0.4
	flat2K := 0.5
	service := &BillingService{}
	config := &ImagePriceConfig{
		Price1K: &flat1K,
		Price2K: &flat2K,
		ModelPrices: map[string]map[string]float64{
			"grok-imagine":       {ImageBillingSize1K: 0.01},
			"grok-imagine-image": {ImageBillingSize1K: 0.02},
		},
	}

	imagine := service.CalculateImageCost("grok-imagine", "1K", 2, config, 0.15)
	standard := service.CalculateImageCost("grok-imagine-image", "1K", 2, config, 0.15)
	fallback := service.CalculateImageCost("grok-imagine", "2K", 1, config, 0.15)

	require.InDelta(t, 0.02, imagine.TotalCost, 1e-12)
	require.InDelta(t, 0.02, imagine.ActualCost, 1e-12)
	require.True(t, imagine.FinalPriceApplied)
	require.InDelta(t, 0.04, standard.TotalCost, 1e-12)
	require.InDelta(t, 0.04, standard.ActualCost, 1e-12)
	require.True(t, standard.FinalPriceApplied)
	require.InDelta(t, flat2K, fallback.TotalCost, 1e-12)
	require.InDelta(t, flat2K*0.15, fallback.ActualCost, 1e-12)
	require.False(t, fallback.FinalPriceApplied)
}

func TestOpenAIImageModelPricesAreIndependentFinalUSDCustomerPrices(t *testing.T) {
	t.Parallel()
	service := &BillingService{}
	config := &ImagePriceConfig{
		ModelPrices: NormalizeImageModelPrices(map[string]map[string]float64{
			"gpt-image-1":            {ImageBillingSize1K: 0.011},
			"gpt-image-1.5":          {ImageBillingSize2K: 0.022},
			"openai/gpt-image-2":     {ImageBillingSize4K: 0.01},
			"gpt-image-2.5":          {ImageBillingSize1K: 0.03},
			"gpt-image-2.5-flare":    {ImageBillingSize1K: 0.04, ImageBillingSize4K: 0},
			"gpt-image-2.5-sunburst": {ImageBillingSize1K: 0.05, ImageBillingSize2K: 0.08},
		}),
	}

	tests := []struct {
		model string
		tier  string
		want  float64
	}{
		{model: "gpt-image-1", tier: ImageBillingSize1K, want: 0.011},
		{model: "gpt-image-1.5", tier: ImageBillingSize2K, want: 0.022},
		{model: "models/gpt-image-2", tier: ImageBillingSize4K, want: 0.01},
		{model: "gpt-image-2.5", tier: ImageBillingSize1K, want: 0.03},
		{model: "gpt-image-2.5-flare", tier: ImageBillingSize1K, want: 0.04},
		{model: "gpt-image-2.5-flare", tier: ImageBillingSize4K, want: 0},
		{model: "gpt-image-2.5-sunburst", tier: ImageBillingSize1K, want: 0.05},
		{model: "gpt-image-2.5-sunburst", tier: ImageBillingSize2K, want: 0.08},
	}

	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.tier, func(t *testing.T) {
			cost := service.CalculateImageCost(tt.model, tt.tier, 1, config, 0.15)
			require.InDelta(t, tt.want, cost.TotalCost, 1e-12)
			require.InDelta(t, tt.want, cost.ActualCost, 1e-12)
			require.True(t, cost.FinalPriceApplied)
		})
	}
}
