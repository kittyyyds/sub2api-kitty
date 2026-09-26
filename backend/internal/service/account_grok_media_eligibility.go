package service

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// ValidateGrokMediaEligibilityExtra validates the optional per-account media
// routing override. A nil value removes the override and restores automatic
// provider-observation routing.
func ValidateGrokMediaEligibilityExtra(platform string, extra map[string]any) error {
	if platform != PlatformGrok || extra == nil {
		return nil
	}
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return nil
	}
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest("GROK_MEDIA_ELIGIBILITY_INVALID", "grok_media_eligible must be a boolean or null")
	}
	return nil
}

func normalizeGrokMediaEligibilityExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformGrok {
		return extra, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(platform, extra); err != nil {
		return nil, err
	}
	if extra == nil {
		return nil, nil
	}
	normalized := shallowCopyMap(extra)
	if normalized[GrokMediaEligibleExtraKey] == nil {
		delete(normalized, GrokMediaEligibleExtraKey)
	}
	return normalized, nil
}

func normalizeGrokMediaEligibilityUpdateExtra(account *Account, input *UpdateAccountInput, normalized map[string]any) (map[string]any, error) {
	if account == nil || account.Platform != PlatformGrok {
		return normalized, nil
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_INPUT", "account update input is required")
	}
	if err := ValidateGrokMediaEligibilityExtra(account.Platform, input.Extra); err != nil {
		return nil, err
	}
	if normalized == nil {
		normalized = make(map[string]any)
	} else {
		normalized = shallowCopyMap(normalized)
	}
	raw, provided := input.Extra[GrokMediaEligibleExtraKey]
	if provided {
		if raw == nil {
			delete(normalized, GrokMediaEligibleExtraKey)
		}
		return normalized, nil
	}
	if current, ok := account.Extra[GrokMediaEligibleExtraKey].(bool); ok {
		normalized[GrokMediaEligibleExtraKey] = current
	}
	return normalized, nil
}

// grokPaidSubscriptionTierEvidence returns explicit provider subscription
// evidence for an OAuth account. The access-token tier is authoritative when
// present so a stale persisted credential or billing snapshot cannot downgrade
// an account that is currently paid. Other credential/extra fields are legacy
// observations and are consulted only when the token has no usable tier.
//
// The second return value is true only for an explicit Free/X Basic tier. An
// unknown or empty value is deliberately not evidence either way; callers
// should continue with the billing snapshot probe in that case.
func grokPaidSubscriptionTierEvidence(account *Account) (tier string, free bool) {
	if account == nil || !account.IsGrokOAuth() {
		return "", false
	}

	if jwtTier := strings.TrimSpace(xai.SubscriptionTierFromJWT(account.GetCredential("access_token"))); jwtTier != "" {
		normalized := xai.NormalizeSubscriptionTier(jwtTier)
		switch {
		case isGrokFreeSubscriptionTier(normalized):
			return "", true
		case !isGrokUnknownSubscriptionTier(normalized):
			return normalized, false
		}
	}

	var paidTier string
	freeObserved := false
	for _, raw := range []string{
		account.GetCredential("subscription_tier"),
		account.GetCredential("plan_type"),
		account.GetExtraString("subscription_tier"),
		account.GetExtraString("plan_type"),
	} {
		normalized := xai.NormalizeSubscriptionTier(raw)
		if normalized == "" || isGrokUnknownSubscriptionTier(normalized) {
			continue
		}
		if isGrokFreeSubscriptionTier(normalized) {
			freeObserved = true
			continue
		}
		if paidTier == "" && !isGrokUnknownSubscriptionTier(normalized) {
			paidTier = normalized
		}
	}
	if paidTier != "" {
		return paidTier, false
	}
	return "", freeObserved
}
