package service

import (
 "testing"
 "github.com/Wei-Shaw/sub2api/internal/config"
 "github.com/stretchr/testify/require"
)

func TestKittySelectedPricesAndGroupMultiplier(t *testing.T) {
 for _,tc:=range []struct{model string; input,output float64}{
  {"gpt-5.6-luna",0.4,2.4},{"gpt-6-astra",10,50},
 } {
  t.Run(tc.model,func(t *testing.T){
   svc:=NewBillingService(&config.Config{},nil)
   price,err:=svc.GetModelPricing(tc.model); require.NoError(t,err)
   require.InDelta(t,tc.input,price.InputPricePerToken*1e6,1e-10)
   require.InDelta(t,tc.output,price.OutputPricePerToken*1e6,1e-10)
   // Below the long-context threshold: ten 100K-token requests represent 1M.
   cost,err:=svc.CalculateCost(tc.model,UsageTokens{InputTokens:100000,OutputTokens:100000},3)
   require.NoError(t,err)
   require.InDelta(t,(tc.input+tc.output)*0.1,cost.TotalCost,1e-10)
   require.InDelta(t,(tc.input+tc.output)*0.3,cost.ActualCost,1e-10)
   fast,err:=svc.CalculateCostWithServiceTier(tc.model,UsageTokens{InputTokens:100000,OutputTokens:100000},3,"priority")
   require.NoError(t,err); require.InDelta(t,cost.ActualCost*2,fast.ActualCost,1e-10)
  })
 }
}

func TestKittyRemovedVariantLockDoesNotRewriteSubagentModels(t *testing.T) {
 for _,lock:=range []string{"sol","terra","luna"} {
  account:=&Account{Platform:PlatformOpenAI,Type:AccountTypeOAuth,Extra:map[string]any{"openai_gpt5_variant_lock":lock}}
  for _,model:=range []string{"gpt-6-astra","gpt-5.6-sol","gpt-5.6-terra","gpt-5.6-luna"} {
   require.Equal(t,model,normalizeOpenAIModelForUpstream(account,model))
  }
 }
}
