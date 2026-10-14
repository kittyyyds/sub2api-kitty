package handler

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestModelPlazaConfiguredSupplierImagesBypassLegacyNameFilter(t *testing.T) {
	for _, name := range []string{"图片模型", "image生图1K"} {
		g := &service.PlazaGroup{Name: name, Platform: service.PlatformOpenAI, Models: []service.PlazaModel{{Name: "Banana-2-香蕉2", Platform: service.PlatformOpenAI, Pricing: &service.ChannelModelPricing{BillingMode: service.BillingModeImage, Image: &service.ImageModelConfig{Provider: "suanli", Description: "说明", Cost: .123, AppliedMarkup: .5}}}}}
		result := toModelPlazaGroupDTO(g, nil, nil)
		require.Len(t, result.Models, 1)
		require.Equal(t, "Banana-2-香蕉2", result.Models[0].Name)
		require.Contains(t, result.Models[0].Description, "说明")
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "applied_markup")
		require.NotContains(t, string(raw), "cost")
	}
}
