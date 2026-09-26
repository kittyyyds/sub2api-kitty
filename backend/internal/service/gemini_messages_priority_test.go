package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type geminiPriorityAccountRepo struct {
	AccountRepository
	grouped  []Account
	platform []Account
	byID     map[int64]*Account
}

func (r *geminiPriorityAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if account, ok := r.byID[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (r *geminiPriorityAccountRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	return append([]Account(nil), r.grouped...), nil
}

func (r *geminiPriorityAccountRepo) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	return append([]Account(nil), r.platform...), nil
}

func (r *geminiPriorityAccountRepo) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	return append([]Account(nil), r.platform...), nil
}

type geminiPrioritySettingRepo struct {
	SettingRepository
	value string
}

type geminiPriorityStickyCache struct {
	GatewayCache
	accountID int64
}

func (c *geminiPriorityStickyCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return c.accountID, nil
}

func (c *geminiPriorityStickyCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *geminiPriorityStickyCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (r *geminiPrioritySettingRepo) GetValue(context.Context, string) (string, error) {
	if r.value == "" {
		return "", errors.New("setting not found")
	}
	return r.value, nil
}

func TestGeminiMessagesCompatServiceListSchedulableAccountsAppliesEffectivePriority(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42)

	repo := &geminiPriorityAccountRepo{
		grouped: []Account{
			{ID: 1, Priority: 100, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 5}}},
			{ID: 2, Priority: 1, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 10}}},
		},
		platform: []Account{
			{ID: 3, Priority: 7, AccountGroups: []AccountGroup{{GroupID: groupID, Priority: 1}}},
		},
	}

	settingService := NewSettingService(&geminiPrioritySettingRepo{value: "true"}, nil)
	settingService.refreshAccountGroupPrioritySetting(true)
	defer settingService.refreshAccountGroupPrioritySetting(false)

	svc := &GeminiMessagesCompatService{
		accountRepo:    repo,
		settingService: settingService,
	}

	accounts, err := svc.listSchedulableAccountsOnce(ctx, &groupID, PlatformGemini, false)
	if err != nil {
		t.Fatalf("list grouped accounts: %v", err)
	}
	if got, want := accounts[0].Priority, 5; got != want {
		t.Fatalf("group priority = %d, want %d", got, want)
	}
	if got, want := accounts[1].Priority, 10; got != want {
		t.Fatalf("group priority = %d, want %d", got, want)
	}

	accounts, err = svc.listSchedulableAccountsOnce(ctx, nil, PlatformAntigravity, true)
	if err != nil {
		t.Fatalf("list fallback accounts: %v", err)
	}
	if got, want := accounts[0].Priority, 7; got != want {
		t.Fatalf("fallback priority = %d, want global priority %d", got, want)
	}

	settingService.refreshAccountGroupPrioritySetting(false)
	accounts, err = svc.listSchedulableAccountsOnce(ctx, &groupID, PlatformGemini, false)
	if err != nil {
		t.Fatalf("list grouped accounts with feature disabled: %v", err)
	}
	if got, want := accounts[0].Priority, 100; got != want {
		t.Fatalf("disabled feature priority = %d, want global priority %d", got, want)
	}
}

func TestGeminiMessagesCompatServiceStickyHitAppliesEffectivePriority(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42)
	account := &Account{
		ID:          7,
		Platform:    PlatformGemini,
		Status:      StatusActive,
		Schedulable: true,
		Priority:    100,
		AccountGroups: []AccountGroup{{
			GroupID:  groupID,
			Priority: 3,
		}},
	}
	repo := &geminiPriorityAccountRepo{byID: map[int64]*Account{account.ID: account}}
	cache := &geminiPriorityStickyCache{accountID: account.ID}
	settingService := NewSettingService(&geminiPrioritySettingRepo{value: "true"}, nil)
	settingService.refreshAccountGroupPrioritySetting(true)
	defer settingService.refreshAccountGroupPrioritySetting(false)

	svc := &GeminiMessagesCompatService{
		accountRepo:    repo,
		cache:          cache,
		settingService: settingService,
	}

	got := svc.tryStickySessionHit(ctx, &groupID, "session", "cache-key", "", nil, PlatformGemini, false)
	if got == nil {
		t.Fatal("sticky session should return the schedulable account")
	}
	if got.Priority != 3 {
		t.Fatalf("sticky hit priority = %d, want group priority 3", got.Priority)
	}

	settingService.refreshAccountGroupPrioritySetting(false)
	account.Priority = 100
	got = svc.tryStickySessionHit(ctx, &groupID, "session", "cache-key", "", nil, PlatformGemini, false)
	if got == nil {
		t.Fatal("sticky session should still return the schedulable account with feature disabled")
	}
	if got.Priority != 100 {
		t.Fatalf("disabled feature sticky priority = %d, want global priority 100", got.Priority)
	}
}
