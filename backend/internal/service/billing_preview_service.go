package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const MaxBillingPreviewTokens = 1_000_000_000

var (
	ErrBillingPreviewInvalidInput       = errors.New("invalid billing preview input")
	ErrBillingPreviewSelectionDenied    = errors.New("billing preview selection is unavailable")
	ErrBillingPreviewPricingUnavailable = errors.New("billing preview pricing is unavailable")
)

type BillingCatalogModel struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type BillingCatalogGroup struct {
	ID                      int64                 `json:"id"`
	Name                    string                `json:"name"`
	Platform                string                `json:"platform"`
	RateMultiplier          float64               `json:"rate_multiplier"`
	PeakMultiplier          float64               `json:"peak_multiplier"`
	EffectiveRateMultiplier float64               `json:"effective_rate_multiplier"`
	RateSource              string                `json:"rate_source"`
	Models                  []BillingCatalogModel `json:"models"`
}

type BillingPreviewInput struct {
	GroupID               int64  `json:"group_id"`
	Model                 string `json:"model"`
	ModelPlatform         string `json:"model_platform"`
	InputTokens           int    `json:"input_tokens"`
	OutputTokens          int    `json:"output_tokens"`
	CacheCreationTokens   int    `json:"cache_creation_tokens"`
	CacheReadTokens       int    `json:"cache_read_tokens"`
	CacheCreation5mTokens int    `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int    `json:"cache_creation_1h_tokens"`
	ServiceTier           string `json:"service_tier"`
}

type BillingPreviewBreakdown struct {
	InputCost         float64 `json:"input_cost"`
	OutputCost        float64 `json:"output_cost"`
	CacheCreationCost float64 `json:"cache_creation_cost"`
	CacheReadCost     float64 `json:"cache_read_cost"`
}

type BillingPreviewResult struct {
	GroupID                 int64                   `json:"group_id"`
	GroupName               string                  `json:"group_name"`
	Model                   string                  `json:"model"`
	ModelPlatform           string                  `json:"model_platform"`
	PricingSource           string                  `json:"pricing_source"`
	BillingMode             string                  `json:"billing_mode"`
	RateMultiplier          float64                 `json:"rate_multiplier"`
	PeakMultiplier          float64                 `json:"peak_multiplier"`
	EffectiveRateMultiplier float64                 `json:"effective_rate_multiplier"`
	StandardCost            float64                 `json:"standard_cost"`
	ActualCost              float64                 `json:"actual_cost"`
	Breakdown               BillingPreviewBreakdown `json:"breakdown"`
	ChargedBreakdown        BillingPreviewBreakdown `json:"charged_breakdown"`
	LongContextApplied      bool                    `json:"long_context_applied"`
}

type billingPreviewGroupProvider interface {
	GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error)
	GetUserGroupRates(ctx context.Context, userID int64) (map[int64]float64, error)
}

type billingPreviewCatalogProvider interface {
	ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error)
}

type billingPreviewModelEntry struct {
	public   BillingCatalogModel
	resolved *ResolvedPricing
}

type billingPreviewGroupEntry struct {
	public BillingCatalogGroup
	group  Group
	models []billingPreviewModelEntry
}

// BillingPreviewService keeps estimates on the same pricing path as usage billing.
type BillingPreviewService struct {
	groups   billingPreviewGroupProvider
	catalog  billingPreviewCatalogProvider
	billing  *BillingService
	resolver *ModelPricingResolver
	now      func() time.Time
}

func NewBillingPreviewService(
	apiKeyService *APIKeyService,
	modelPlazaService *ModelPlazaService,
	billingService *BillingService,
	resolver *ModelPricingResolver,
) *BillingPreviewService {
	return &BillingPreviewService{
		groups:   apiKeyService,
		catalog:  modelPlazaService,
		billing:  billingService,
		resolver: resolver,
		now:      timezone.Now,
	}
}

func (s *BillingPreviewService) Catalog(ctx context.Context, userID int64) ([]BillingCatalogGroup, error) {
	entries, err := s.loadCatalog(ctx, userID, s.currentTime())
	if err != nil {
		return nil, err
	}
	out := make([]BillingCatalogGroup, 0, len(entries))
	for i := range entries {
		out = append(out, entries[i].public)
	}
	return out, nil
}

func (s *BillingPreviewService) Preview(ctx context.Context, userID int64, input BillingPreviewInput) (*BillingPreviewResult, error) {
	if err := validateBillingPreviewInput(input); err != nil {
		return nil, err
	}

	entries, err := s.loadCatalog(ctx, userID, s.currentTime())
	if err != nil {
		return nil, err
	}
	entry, modelEntry := findBillingPreviewSelection(entries, input)
	if entry == nil || modelEntry == nil {
		return nil, ErrBillingPreviewSelectionDenied
	}

	groupID := entry.group.ID
	cost, err := s.billing.CalculateCostUnified(CostInput{
		Ctx:     ctx,
		Model:   modelEntry.public.Name,
		GroupID: &groupID,
		Group:   &entry.group,
		Tokens: UsageTokens{
			InputTokens:           input.InputTokens,
			OutputTokens:          input.OutputTokens,
			CacheCreationTokens:   input.CacheCreationTokens,
			CacheReadTokens:       input.CacheReadTokens,
			CacheCreation5mTokens: input.CacheCreation5mTokens,
			CacheCreation1hTokens: input.CacheCreation1hTokens,
		},
		RateMultiplier: entry.public.EffectiveRateMultiplier,
		ServiceTier:    normalizeBillingServiceTier(input.ServiceTier),
		Resolver:       s.resolver,
		Resolved:       modelEntry.resolved,
	})
	if err != nil {
		if errors.Is(err, ErrModelPricingUnavailable) {
			return nil, fmt.Errorf("%w: %v", ErrBillingPreviewPricingUnavailable, err)
		}
		return nil, err
	}

	breakdown := BillingPreviewBreakdown{
		InputCost:         cost.InputCost + cost.ImageInputCost,
		OutputCost:        cost.OutputCost + cost.ImageOutputCost,
		CacheCreationCost: cost.CacheCreationCost,
		CacheReadCost:     cost.CacheReadCost,
	}
	return &BillingPreviewResult{
		GroupID:                 entry.public.ID,
		GroupName:               entry.public.Name,
		Model:                   modelEntry.public.Name,
		ModelPlatform:           modelEntry.public.Platform,
		PricingSource:           modelEntry.resolved.Source,
		BillingMode:             cost.BillingMode,
		RateMultiplier:          entry.public.RateMultiplier,
		PeakMultiplier:          entry.public.PeakMultiplier,
		EffectiveRateMultiplier: entry.public.EffectiveRateMultiplier,
		StandardCost:            cost.TotalCost,
		ActualCost:              cost.ActualCost,
		Breakdown:               breakdown,
		ChargedBreakdown:        scaleBillingPreviewBreakdown(breakdown, entry.public.EffectiveRateMultiplier),
		LongContextApplied:      cost.LongContextBillingApplied,
	}, nil
}

func scaleBillingPreviewBreakdown(breakdown BillingPreviewBreakdown, multiplier float64) BillingPreviewBreakdown {
	return BillingPreviewBreakdown{
		InputCost:         breakdown.InputCost * multiplier,
		OutputCost:        breakdown.OutputCost * multiplier,
		CacheCreationCost: breakdown.CacheCreationCost * multiplier,
		CacheReadCost:     breakdown.CacheReadCost * multiplier,
	}
}

func (s *BillingPreviewService) loadCatalog(ctx context.Context, userID int64, at time.Time) ([]billingPreviewGroupEntry, error) {
	if s == nil || s.groups == nil || s.catalog == nil || s.billing == nil || s.resolver == nil {
		return nil, errors.New("billing preview service is not configured")
	}

	groups, err := s.groups.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list billing preview groups: %w", err)
	}
	rates, err := s.groups.GetUserGroupRates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list billing preview rates: %w", err)
	}
	plazaGroups, err := s.catalog.ListPlazaGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billing preview models: %w", err)
	}

	plazaByID := make(map[int64]PlazaGroup, len(plazaGroups))
	for i := range plazaGroups {
		plazaByID[plazaGroups[i].ID] = plazaGroups[i]
	}

	entries := make([]billingPreviewGroupEntry, 0, len(groups))
	for i := range groups {
		group := groups[i]
		models := billingPreviewCatalogModels(group, plazaByID)
		if len(models) == 0 {
			continue
		}

		baseRate := group.RateMultiplier
		rateSource := "group"
		if userRate, exists := rates[group.ID]; exists {
			baseRate = userRate
			rateSource = "user"
		}
		if baseRate < 0 {
			baseRate = 0
		}
		peakMultiplier := group.PeakMultiplierAt(at)

		entry := billingPreviewGroupEntry{
			group: group,
			public: BillingCatalogGroup{
				ID:                      group.ID,
				Name:                    group.Name,
				Platform:                group.Platform,
				RateMultiplier:          baseRate,
				PeakMultiplier:          peakMultiplier,
				EffectiveRateMultiplier: baseRate * peakMultiplier,
				RateSource:              rateSource,
				Models:                  make([]BillingCatalogModel, 0, len(models)),
			},
			models: make([]billingPreviewModelEntry, 0, len(models)),
		}

		seen := make(map[string]struct{}, len(models))
		for j := range models {
			model := models[j]
			resolved := s.resolver.Resolve(ctx, PricingInput{
				Model:   model.Name,
				GroupID: &group.ID,
				Group:   &group,
			})
			if resolved == nil || resolved.Mode != BillingModeToken ||
				(resolved.BasePricing == nil && len(resolved.Intervals) == 0) {
				continue
			}

			key := strings.ToLower(strings.TrimSpace(model.Platform)) + "\x00" + strings.ToLower(strings.TrimSpace(model.Name))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			publicModel := BillingCatalogModel{Name: model.Name, Platform: model.Platform}
			entry.public.Models = append(entry.public.Models, publicModel)
			entry.models = append(entry.models, billingPreviewModelEntry{public: publicModel, resolved: resolved})
		}
		if len(entry.models) > 0 {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func billingPreviewCatalogModels(group Group, plazaByID map[int64]PlazaGroup) []PlazaModel {
	if plaza, ok := plazaByID[group.ID]; ok && len(plaza.Models) > 0 {
		return plaza.Models
	}

	models := make([]PlazaModel, 0, len(group.ModelAllowlist.Models))
	for _, configuredModel := range group.ModelAllowlist.Models {
		name := strings.TrimSpace(configuredModel)
		if name == "" {
			continue
		}

		platform := group.Platform
		if platform == PlatformComposite {
			if detected, ok := DetectModelPlatform(name); ok {
				platform = detected
			}
		}
		models = append(models, PlazaModel{Name: name, Platform: platform})
	}
	return models
}

func (s *BillingPreviewService) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return timezone.Now()
}

func validateBillingPreviewInput(input BillingPreviewInput) error {
	if input.GroupID <= 0 || strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.ModelPlatform) == "" {
		return fmt.Errorf("%w: group and model are required", ErrBillingPreviewInvalidInput)
	}
	values := []int{
		input.InputTokens,
		input.OutputTokens,
		input.CacheCreationTokens,
		input.CacheReadTokens,
		input.CacheCreation5mTokens,
		input.CacheCreation1hTokens,
	}
	for _, value := range values {
		if value < 0 || value > MaxBillingPreviewTokens {
			return fmt.Errorf("%w: token values must be between 0 and %d", ErrBillingPreviewInvalidInput, MaxBillingPreviewTokens)
		}
	}
	switch normalizeBillingServiceTier(input.ServiceTier) {
	case "", "priority", "flex":
		return nil
	default:
		return fmt.Errorf("%w: unsupported service tier", ErrBillingPreviewInvalidInput)
	}
}

func findBillingPreviewSelection(entries []billingPreviewGroupEntry, input BillingPreviewInput) (*billingPreviewGroupEntry, *billingPreviewModelEntry) {
	for i := range entries {
		if entries[i].group.ID != input.GroupID {
			continue
		}
		for j := range entries[i].models {
			model := &entries[i].models[j]
			if model.public.Name == input.Model && model.public.Platform == input.ModelPlatform {
				return &entries[i], model
			}
		}
		return &entries[i], nil
	}
	return nil, nil
}
