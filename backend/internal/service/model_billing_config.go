package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// Version 2 rules never activate legacy unscoped charges.
const ModelBillingSchemaVersion = 2
const ModelBillingAlways = "always"
const ModelBillingContext = "context"

type ModelBillingConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Enabled       bool               `json:"enabled"`
	Rules         []ModelBillingRule `json:"rules"`
}
type ModelBillingRule struct {
	Model           string  `json:"model"`
	Multiplier      float64 `json:"multiplier"`
	GroupID         int64   `json:"group_id"`
	Enabled         bool    `json:"enabled"`
	Mode            string  `json:"mode"`
	ThresholdTokens int     `json:"threshold_tokens"`
}

func DefaultModelBillingConfig() ModelBillingConfig {
	return ModelBillingConfig{SchemaVersion: 2, Rules: []ModelBillingRule{}}
}

// Legacy data remains in the backup and audit history, never copied to all groups.
func (c *ModelBillingConfig) UnmarshalJSON(data []byte) error {
	type plain ModelBillingConfig
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.SchemaVersion == 0 {
		*c = DefaultModelBillingConfig()
		return nil
	}
	*c = ModelBillingConfig(v)
	return nil
}

var modelBillingPattern = regexp.MustCompile(`^[a-zA-Z0-9_./:-]+\*?$`)

func validateModelBillingConfig(c ModelBillingConfig) error {
	bad := func(m string) error { return infraerrors.BadRequest("MODEL_BILLING_INVALID", m) }
	if c.SchemaVersion != 2 {
		if !c.Enabled && len(c.Rules) == 0 {
			return nil
		}
		return bad("reload group-scoped billing settings")
	}
	if len(c.Rules) > 100 || c.Enabled && len(c.Rules) == 0 {
		return bad("select 1–100 group/model rules")
	}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		model := strings.ToLower(strings.TrimSpace(r.Model))
		key := fmt.Sprintf("%d:%s", r.GroupID, model)
		if r.GroupID <= 0 || !modelBillingPattern.MatchString(model) || len(model) > 200 || seen[key] {
			return bad("select a group and unique model; only trailing * is allowed")
		}
		if math.IsNaN(r.Multiplier) || math.IsInf(r.Multiplier, 0) || r.Multiplier < 1 || r.Multiplier > 1000 {
			return bad("multiplier must be 1–1000")
		}
		if r.Mode != ModelBillingAlways && r.Mode != ModelBillingContext {
			return bad("select always or context mode")
		}
		if r.Mode == ModelBillingContext && (r.ThresholdTokens < 1 || r.ThresholdTokens > 100_000_000) {
			return bad("context threshold must be 1–100000000 tokens")
		}
		seen[key] = true
	}
	return nil
}

// Select one rule before testing its threshold. Exact beats longest prefix.
func (c ModelBillingConfig) match(groupID int64, model string) *ModelBillingRule {
	if !c.Enabled || c.SchemaVersion != 2 || groupID <= 0 {
		return nil
	}
	model = strings.ToLower(strings.TrimSpace(model))
	var found *ModelBillingRule
	longest := 0
	for i := range c.Rules {
		r := &c.Rules[i]
		if !r.Enabled || r.GroupID != groupID {
			continue
		}
		pattern := strings.ToLower(strings.TrimSpace(r.Model))
		if pattern == model {
			return r
		}
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if len(prefix) > longest && strings.HasPrefix(model, prefix) {
				found = r
				longest = len(prefix)
			}
		}
	}
	return found
}
func modelBillingContextTokens(t UsageTokens) int64 {
	cw := t.CacheCreationTokens
	if cw <= 0 {
		five, hour := normalizeCacheCreationBreakdown(t)
		cw = five + hour
	}
	return int64(max(0, t.InputTokens)) + int64(max(0, t.CacheReadTokens)) + int64(max(0, cw))
}
func (c ModelBillingConfig) multiplier(groupID int64, model string, tokens UsageTokens) float64 {
	r := c.match(groupID, model)
	if r == nil || r.Mode == ModelBillingContext && modelBillingContextTokens(tokens) <= int64(r.ThresholdTokens) {
		return 1
	}
	return r.Multiplier
}
func applyModelBillingMultiplier(cost *CostBreakdown, cfg ModelBillingConfig, groupID int64, model string, tokens UsageTokens) {
	if cost == nil || cost.BillingMode != "" && cost.BillingMode != string(BillingModeToken) || cost.modelBillingMultiplier != 0 {
		return
	}
	m := cfg.multiplier(groupID, model, tokens)
	cost.ActualCost *= m
	cost.modelBillingMultiplier = m
	if r := cfg.match(groupID, model); r != nil && r.Mode == ModelBillingContext && m > 1 {
		cost.LongContextBillingApplied = true
	}
}
func modelBillingGroupID(in CostInput) int64 {
	if in.GroupID != nil {
		return *in.GroupID
	}
	if in.Group != nil {
		return in.Group.ID
	}
	return 0
}

// Settlement, reservation and price schedules share the same pinned policy.
func (s *BillingService) CalculateCostUnified(in CostInput) (*CostBreakdown, error) {
	if in.Ctx == nil {
		in.Ctx = context.Background()
	}
	in.Ctx = withModelBillingConfig(in.Ctx, s.modelBillingSettings)
	cfg := s.modelBillingSettings.modelBillingConfigForUsage(in.Ctx)
	gid := modelBillingGroupID(in)
	var r *ModelBillingRule
	if !in.SkipGlobalContextRules {
		r = cfg.match(gid, in.Model)
	}
	original := in
	// A matching group/model policy owns the surcharge, including always mode.
	// Never stack a global rule or a catalog ladder on top of that policy.
	in.overrideContextPricing = r != nil
	cost, err := s.calculateCostUnifiedBase(in)
	if err != nil || cost == nil {
		return cost, err
	}
	if in.overrideContextPricing && (cost.BillingMode == "" || cost.BillingMode == string(BillingModeToken)) {
		original.SkipGlobalContextRules = true
		base, e := s.calculateCostUnifiedBase(original)
		if e != nil {
			return nil, e
		}
		cost.ContextBaseTotalCost = &base.TotalCost
	}
	if !in.SkipGlobalContextRules {
		applyModelBillingMultiplier(cost, cfg, gid, in.Model, in.Tokens)
	}
	return cost, nil
}

func costModelBillingMultiplier(cost *CostBreakdown) float64 {
	if cost == nil || cost.modelBillingMultiplier == 0 {
		return 1
	}
	return cost.modelBillingMultiplier
}

type modelBillingContextKey struct{}

func withModelBillingConfig(ctx context.Context, settings *SettingService) context.Context {
	if _, ok := ctx.Value(modelBillingContextKey{}).(ModelBillingConfig); ok {
		return ctx
	}
	return context.WithValue(ctx, modelBillingContextKey{}, settings.modelBillingConfigForUsage(ctx))
}

type modelBillingConfigCache struct {
	mu      sync.Mutex
	config  ModelBillingConfig
	expires time.Time
}

// Cache per service, including failures, to avoid querying settings for every token bill.
// A refresh failure preserves the last valid policy; a cold failure leaves surcharges off.
func (s *SettingService) modelBillingConfigForUsage(ctx context.Context) ModelBillingConfig {
	if cfg, ok := ctx.Value(modelBillingContextKey{}).(ModelBillingConfig); ok {
		return cfg
	}
	if s == nil || s.settingRepo == nil {
		return ModelBillingConfig{}
	}
	cache := &s.modelBillingCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if time.Now().Before(cache.expires) {
		return cache.config
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := s.settingRepo.GetValue(ctx, "smart_ops_oauth_auto_config")
	cfg := struct {
		ModelBilling ModelBillingConfig `json:"model_billing"`
	}{DefaultModelBillingConfig()}
	if errors.Is(err, ErrSettingNotFound) {
		err = nil
	}
	if err == nil && raw != "" {
		err = json.Unmarshal([]byte(raw), &cfg)
	}
	if err == nil {
		err = validateModelBillingConfig(cfg.ModelBilling)
	}
	if err == nil {
		cache.config = cfg.ModelBilling
	} else {
		logger.LegacyPrintf("service.billing", "Model billing configuration refresh failed; retaining the last valid policy")
	}
	cache.expires = time.Now().Add(15 * time.Second)
	return cache.config
}
