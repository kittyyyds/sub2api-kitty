package service

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestPricingAuditDistinguishesFreeFromMissing(t *testing.T) {
	zero := 0.0
	one := 1.0
	for _, tc := range []struct {
		name     string
		resolved *ResolvedPricing
		cfg      *ChannelModelPricing
		base     bool
		status   string
	}{
		{"nil", nil, nil, false, "missing"},
		{"unknown", &ResolvedPricing{Mode: BillingModeToken}, nil, false, "missing"},
		{"allocated_zero_not_free", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{}}, &ChannelModelPricing{}, false, "missing"},
		{"partial", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{InputPricePerToken: 1}}, &ChannelModelPricing{InputPrice: &one}, false, "missing"},
		{"explicit_zero", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{}}, &ChannelModelPricing{InputPrice: &zero, OutputPrice: &zero}, false, "free"},
		{"catalog_zero", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{}}, nil, true, "free"},
		{"paid_priority_not_free", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{OutputPricePerTokenPriority: 1}}, nil, true, "priced"},
		{"paid_image_not_free", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{ImageOutputPricePerToken: 1}}, nil, true, "priced"},
		{"priced", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{OutputPricePerToken: 1}}, nil, true, "priced"},
		{"negative", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{InputPricePerToken: -1}}, nil, true, "invalid"},
		{"nan", &ResolvedPricing{Mode: BillingModeToken, BasePricing: &ModelPricing{InputPricePerToken: math.NaN()}}, nil, true, "invalid"},
		{"request_missing", &ResolvedPricing{Mode: BillingModePerRequest}, &ChannelModelPricing{}, false, "missing"},
		{"request_free", &ResolvedPricing{Mode: BillingModePerRequest}, &ChannelModelPricing{PerRequestPrice: &zero}, false, "free"},
		{"tiers", &ResolvedPricing{Mode: BillingModeVideo, RequestTiers: []PricingInterval{{}}}, nil, false, "review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := pricingAuditStatus(tc.resolved, tc.cfg, tc.base)
			require.Equal(t, tc.status, status)
		})
	}
}
