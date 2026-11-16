package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// quota429CooldownThreshold is the default number of consecutive OpenAI 429s
// before the formal cooldown; gateway.openai_429_soft_threshold overrides it.
const quota429CooldownThreshold = 30

var (
	quota429Mu     sync.Mutex
	quota429Counts map[int64]int
)

func nextOpenAI429Cooldown(accountID int64) (count int, shouldCooldown bool) {
	return nextOpenAI429CooldownAt(accountID, quota429CooldownThreshold)
}

func nextOpenAI429CooldownAt(accountID int64, threshold int) (count int, shouldCooldown bool) {
	if threshold <= 0 {
		threshold = quota429CooldownThreshold
	}
	quota429Mu.Lock()
	defer quota429Mu.Unlock()

	if quota429Counts == nil {
		quota429Counts = map[int64]int{}
	}

	count = quota429Counts[accountID]
	if count < threshold {
		count++
		quota429Counts[accountID] = count
	}
	return count, count >= threshold
}

// openAI429SoftThreshold resolves gateway.openai_429_soft_threshold.
func (s *RateLimitService) openAI429SoftThreshold() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAI429SoftThreshold > 0 {
		return s.cfg.Gateway.OpenAI429SoftThreshold
	}
	return quota429CooldownThreshold
}

// ResetOpenAI429Counter clears the in-process consecutive OpenAI 429 count for an account.
func ResetOpenAI429Counter(accountID int64) {
	if accountID <= 0 {
		return
	}
	quota429Mu.Lock()
	defer quota429Mu.Unlock()
	if quota429Counts == nil {
		return
	}
	delete(quota429Counts, accountID)
}

func openAI429Count(accountID int64) int {
	quota429Mu.Lock()
	defer quota429Mu.Unlock()
	if quota429Counts == nil {
		return 0
	}
	return quota429Counts[accountID]
}

func openAI429CooldownDisabledForAccount(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Credentials != nil {
		switch v := account.Credentials["disable_429_cooling"].(type) {
		case bool:
			return v
		}
	}
	switch strings.ToLower(strings.TrimSpace(account.GetCredential("disable_429_cooling"))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *RateLimitService) shouldCooldownOpenAI429(ctx context.Context, account *Account, headers http.Header, body []byte) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}

	persistOpenAI429PlanType(ctx, s.accountRepo, account, body)
	s.persistOpenAICodexSnapshot(ctx, account, headers)

	if openAI429CooldownDisabledForAccount(account) {
		ResetOpenAI429Counter(account.ID)
		return false
	}

	threshold := s.openAI429SoftThreshold()
	count, shouldCooldown := nextOpenAI429CooldownAt(account.ID, threshold)
	soft429 := !shouldCooldown
	level := slog.LevelDebug
	if count >= threshold-1 {
		level = slog.LevelInfo
	}
	selectionAction := "keep_near_limit_eligible"
	if shouldCooldown {
		selectionAction = "apply_original_cooldown"
	}
	slog.Log(ctx, level, "openai_429_consecutive_count",
		"account_id", account.ID,
		"count", count,
		"threshold", threshold,
		"soft_429", soft429,
		"formal_cooldown", shouldCooldown,
		"should_cooldown", shouldCooldown,
		"selection_action", selectionAction,
	)
	return shouldCooldown
}
