package service

import (
	"context"
	"fmt"
	"time"
)

// Exact site packages take precedence over upstream percentage tiers. Their
// credit is granted by the existing idempotent grant ledger after payment.
// An order without an exact package uses only the upstream pricing snapshot.
func (s *PaymentService) quoteBalanceRecharge(ctx context.Context, cfg *PaymentConfig, amount float64, currency string) (rechargeBonusQuote, *RechargeBonusRule, error) {
	var rule *RechargeBonusRule
	if s.rechargeBonusRepo != nil {
		matched, err := s.rechargeBonusRepo.MatchRule(ctx, amount, time.Now().UTC())
		if err != nil {
			return rechargeBonusQuote{}, nil, fmt.Errorf("match recharge bonus rule: %w", err)
		}
		if matched != nil {
			copyRule := *matched
			rule = &copyRule
		}
	}
	if rule != nil && cfg != nil {
		copyConfig := *cfg
		copyConfig.RechargeBonusTiers = nil
		return quoteRechargeBonus(&copyConfig, amount, currency), rule, nil
	}
	return quoteRechargeBonus(cfg, amount, currency), nil, nil
}
