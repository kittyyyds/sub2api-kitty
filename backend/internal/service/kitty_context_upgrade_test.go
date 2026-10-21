//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Explicit customer rules must retain first priority even when the upstream
// release exempts a subscription model from its native long-context surcharge.
func TestKittyContextUpgradeRuleOverridesSubscriptionExemption(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, n := range []int{499999, 500000, 500001} {
			for _, tier := range []string{"", "priority"} {
				for _, enabled := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%d/%s/native=%t", accountType, n, tier, enabled), func(t *testing.T) {
						logs := &openAIRecordUsageLogRepoStub{inserted: true}
						users := &openAIRecordUsageUserRepoStub{}
						svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
						svc.billingService.globalContextRules = contextRulesStub{rule: &GlobalContextRule{
							ID: 7, Revision: 2, Model: "gpt-6.1-sol", Platform: PlatformOpenAI, Enabled: true,
							ThresholdTokens: 500000, InputMultiplier: 2, OutputMultiplier: 2,
							CacheReadMultiplier: 2, CacheWriteMultiplier: 2,
						}}
						key := openAIRecordUsageAPIKeyWithGroup(svc, 1, enabled)
						account := &Account{ID: 3, Platform: PlatformOpenAI, Type: accountType,
							Extra: map[string]any{openAILongContextBillingEnabledKey: enabled}}
						result := &OpenAIForwardResult{RequestID: t.Name(), Model: "gpt-6.1-sol", ServiceTier: &tier,
							Duration: time.Second, Usage: OpenAIUsage{InputTokens: n, OutputTokens: 2000,
								CacheReadInputTokens: 80000, CacheCreationInputTokens: 20000}}
						factor := 1.0
						if n > 500000 {
							factor = 2
						}
						if tier == "priority" {
							factor *= 2
						}
						want := (float64(n-100000)*2e-6 + 80000*0.1e-6 + 20000*2.5e-6 + 2000*10e-6) * factor
						quote, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, key,
							[]string{"gpt-6.1-sol"}, "gpt-6.1-sol", 1.1, 1.1, 1.1, 1.1,
							UsageTokens{InputTokens: n - 100000, OutputTokens: 2000, CacheReadTokens: 80000, CacheCreationTokens: 20000},
							tier, &enabled, openAIGPT61SolLongContextExempt(account, result), time.Time{})
						require.NoError(t, err)
						require.InDelta(t, want*1.1, quote.ActualCost, 1e-10)
						err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
							Result: result, APIKey: key, User: &User{ID: 2}, Account: account,
						})
						require.NoError(t, err)
						require.NotNil(t, logs.lastLog)
						require.InDelta(t, want, logs.lastLog.TotalCost, 1e-10)
						require.InDelta(t, quote.ActualCost, logs.lastLog.ActualCost, 1e-10)
						require.Equal(t, 1, users.deductCalls)
						require.InDelta(t, quote.ActualCost, users.lastAmount, 1e-10)
					})
				}
			}
		}
	}
}
