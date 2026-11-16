//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stubBillingPreviewGroups struct {
	groups []Group
	rates  map[int64]float64
}

func (s *stubBillingPreviewGroups) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return s.groups, nil
}

func (s *stubBillingPreviewGroups) GetUserGroupRates(context.Context, int64) (map[int64]float64, error) {
	return s.rates, nil
}

type stubBillingPreviewCatalog struct {
	groups []PlazaGroup
}

func (s *stubBillingPreviewCatalog) ListPlazaGroups(context.Context) ([]PlazaGroup, error) {
	return s.groups, nil
}

func billingPreviewFloat(value float64) *float64 {
	return &value
}

func newBillingPreviewTestService(groups []Group, rates map[int64]float64, plaza []PlazaGroup) *BillingPreviewService {
	billing := NewBillingService(&config.Config{}, nil)
	return &BillingPreviewService{
		groups:   &stubBillingPreviewGroups{groups: groups, rates: rates},
		catalog:  &stubBillingPreviewCatalog{groups: plaza},
		billing:  billing,
		resolver: NewModelPricingResolver(nil, billing),
		now: func() time.Time {
			return time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
		},
	}
}

func billingPreviewGroup() Group {
	return Group{
		ID:             21,
		Name:           "CC-Max",
		Platform:       PlatformAnthropic,
		Status:         StatusActive,
		RateMultiplier: 0.7,
		ModelPricing: []ChannelModelPricing{
			{
				Platform:    PlatformAnthropic,
				Models:      []string{"claude-sonnet-4"},
				BillingMode: BillingModeToken,
				InputPrice:  billingPreviewFloat(5e-6),
				OutputPrice: billingPreviewFloat(30e-6),
			},
			{
				Platform:        PlatformAnthropic,
				Models:          []string{"image-model"},
				BillingMode:     BillingModeImage,
				PerRequestPrice: billingPreviewFloat(0.1),
			},
		},
	}
}

func billingPreviewPlaza() []PlazaGroup {
	return []PlazaGroup{{
		ID:       21,
		Name:     "CC-Max",
		Platform: PlatformAnthropic,
		Models: []PlazaModel{
			{Name: "claude-sonnet-4", Platform: PlatformAnthropic},
			{Name: "image-model", Platform: PlatformAnthropic},
		},
	}}
}

func TestBillingPreviewCatalogUsesUserAndPeakRatesAndFiltersNonTokenModels(t *testing.T) {
	group := billingPreviewGroup()
	group.SubscriptionType = SubscriptionTypeSubscription
	group.PeakRateEnabled = true
	group.PeakStart = "10:00"
	group.PeakEnd = "14:00"
	group.PeakRateMultiplier = 1.5
	svc := newBillingPreviewTestService([]Group{group}, map[int64]float64{21: 0.8}, billingPreviewPlaza())

	catalog, err := svc.Catalog(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, "user", catalog[0].RateSource)
	require.InDelta(t, 0.8, catalog[0].RateMultiplier, 1e-12)
	require.InDelta(t, 1.5, catalog[0].PeakMultiplier, 1e-12)
	require.InDelta(t, 1.2, catalog[0].EffectiveRateMultiplier, 1e-12)
	require.Equal(t, []BillingCatalogModel{{Name: "claude-sonnet-4", Platform: PlatformAnthropic}}, catalog[0].Models)
}

func TestBillingPreviewUsesUnifiedBillingCost(t *testing.T) {
	svc := newBillingPreviewTestService([]Group{billingPreviewGroup()}, nil, billingPreviewPlaza())

	result, err := svc.Preview(context.Background(), 7, BillingPreviewInput{
		GroupID:       21,
		Model:         "claude-sonnet-4",
		ModelPlatform: PlatformAnthropic,
		InputTokens:   1_000_000,
		OutputTokens:  500_000,
	})
	require.NoError(t, err)
	require.Equal(t, string(BillingModeToken), result.BillingMode)
	require.InDelta(t, 20.0, result.StandardCost, 1e-10)
	require.InDelta(t, 14.0, result.ActualCost, 1e-10)
	require.InDelta(t, 5.0, result.Breakdown.InputCost, 1e-10)
	require.InDelta(t, 15.0, result.Breakdown.OutputCost, 1e-10)
	require.InDelta(t, 3.5, result.ChargedBreakdown.InputCost, 1e-10)
	require.InDelta(t, 10.5, result.ChargedBreakdown.OutputCost, 1e-10)
	require.InDelta(t, 0.7, result.EffectiveRateMultiplier, 1e-12)
}

func TestBillingPreviewFallsBackToGroupModelsListWithoutChannels(t *testing.T) {
	group := billingPreviewGroup()
	group.Platform = PlatformComposite
	group.ModelPricing = nil
	group.ModelAllowlist = GroupModelAllowlist{
		Enabled: true,
		Models:  []string{"claude-sonnet-4", " claude-sonnet-4 ", "gpt-5.4"},
	}
	svc := newBillingPreviewTestService([]Group{group}, nil, nil)

	catalog, err := svc.Catalog(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, []BillingCatalogModel{
		{Name: "claude-sonnet-4", Platform: PlatformAnthropic},
		{Name: "gpt-5.4", Platform: PlatformOpenAI},
	}, catalog[0].Models)

	result, err := svc.Preview(context.Background(), 7, BillingPreviewInput{
		GroupID:       group.ID,
		Model:         "gpt-5.4",
		ModelPlatform: PlatformOpenAI,
		InputTokens:   1_000_000,
	})
	require.NoError(t, err)
	require.Equal(t, string(BillingModeToken), result.BillingMode)
	require.Greater(t, result.StandardCost, 0.0)
	require.InDelta(t, result.StandardCost*group.RateMultiplier, result.ActualCost, 1e-10)
}

func TestBillingPreviewRejectsInvalidOrUnavailableSelections(t *testing.T) {
	svc := newBillingPreviewTestService([]Group{billingPreviewGroup()}, nil, billingPreviewPlaza())

	_, err := svc.Preview(context.Background(), 7, BillingPreviewInput{
		GroupID:       21,
		Model:         "claude-sonnet-4",
		ModelPlatform: PlatformAnthropic,
		InputTokens:   -1,
	})
	require.ErrorIs(t, err, ErrBillingPreviewInvalidInput)

	_, err = svc.Preview(context.Background(), 7, BillingPreviewInput{
		GroupID:       999,
		Model:         "claude-sonnet-4",
		ModelPlatform: PlatformAnthropic,
	})
	require.ErrorIs(t, err, ErrBillingPreviewSelectionDenied)

	_, err = svc.Preview(context.Background(), 7, BillingPreviewInput{
		GroupID:       21,
		Model:         "image-model",
		ModelPlatform: PlatformAnthropic,
	})
	require.ErrorIs(t, err, ErrBillingPreviewSelectionDenied)
}

func TestBillingPreviewOpenAICacheWriteLifetimes(t *testing.T) {
	group := billingPreviewGroup()
	group.Platform = PlatformOpenAI
	group.ModelPricing = []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6-luna"}, BillingMode: BillingModeToken,
		InputPrice: billingPreviewFloat(1e-6), OutputPrice: billingPreviewFloat(2e-6), CacheWritePrice: billingPreviewFloat(2e-6), CacheWrite12hPrice: billingPreviewFloat(4e-6)}}
	plaza := []PlazaGroup{{ID: group.ID, Platform: PlatformOpenAI, Models: []PlazaModel{{Name: "gpt-6-luna", Platform: PlatformOpenAI}}}}
	svc := newBillingPreviewTestService([]Group{group}, nil, plaza)
	input := BillingPreviewInput{GroupID: group.ID, Model: "gpt-6-luna", ModelPlatform: PlatformOpenAI,
		CacheCreationTokens: 3000, CacheCreation30mTokens: 1000, CacheCreation12hTokens: 2000}
	result, err := svc.Preview(context.Background(), 7, input)
	require.NoError(t, err)
	require.InDelta(t, 0.010, result.StandardCost, 1e-12)
	require.InDelta(t, 0.007, result.ActualCost, 1e-12)
	input.CacheCreation12hTokens = -1
	_, err = svc.Preview(context.Background(), 7, input)
	require.ErrorIs(t, err, ErrBillingPreviewInvalidInput)
	input.CacheCreation12hTokens = 0
	input.CacheCreation30mTokens = MaxBillingPreviewTokens + 1
	_, err = svc.Preview(context.Background(), 7, input)
	require.ErrorIs(t, err, ErrBillingPreviewInvalidInput)
}
