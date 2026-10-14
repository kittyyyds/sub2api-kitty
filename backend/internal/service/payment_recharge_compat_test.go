package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type pricingBonusRepository struct {
	RechargeBonusRepository
	rule  *RechargeBonusRule
	err   error
	reads int
}

func (r *pricingBonusRepository) MatchRule(context.Context, float64, time.Time) (*RechargeBonusRule, error) {
	r.reads++
	return r.rule, r.err
}

func TestBalanceRechargeExactPackageNeverStacks(t *testing.T) {
	for _, mode := range []string{RechargeBonusModeBonus, RechargeBonusModeDiscount} {
		t.Run(mode, func(t *testing.T) {
			repo := &pricingBonusRepository{rule: &RechargeBonusRule{ID: 1, RechargeAmount: 100, BonusAmount: 25, ValidityDays: 0}}
			svc := &PaymentService{rechargeBonusRepo: repo}
			cfg := &PaymentConfig{BalanceRechargeMultiplier: 2, RechargeBonusMode: mode, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 50, BonusPercent: 30}}}
			quote, snapshot, err := svc.quoteBalanceRecharge(context.Background(), cfg, 100, "CNY")
			require.NoError(t, err)
			require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 200}, quote)
			require.Equal(t, 225.0, quote.Credited+snapshot.BonusAmount)
			require.Equal(t, 1, repo.reads)
			repo.rule.BonusAmount = 99
			require.Equal(t, 25.0, snapshot.BonusAmount, "the order retains its selected rule")
			require.Len(t, cfg.RechargeBonusTiers, 1, "pricing must not mutate shared settings")
		})
	}
}

func TestBalanceRechargeUnmatchedPackageUsesUpstreamTier(t *testing.T) {
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 2, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 50, BonusPercent: 30}}}
	svc := &PaymentService{rechargeBonusRepo: &pricingBonusRepository{}}
	quote, rule, err := svc.quoteBalanceRecharge(context.Background(), cfg, 100, "CNY")
	require.NoError(t, err)
	require.Nil(t, rule)
	require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 260, Bonus: 60, Percent: 30}, quote)
	cfg.RechargeBonusMode = RechargeBonusModeDiscount
	quote, rule, err = svc.quoteBalanceRecharge(context.Background(), cfg, 100, "CNY")
	require.NoError(t, err)
	require.Nil(t, rule)
	require.Equal(t, rechargeBonusQuote{PayBase: 70, Credited: 200, Bonus: 60, Percent: 30}, quote)
}

func TestBalanceRechargeRuleLookupFailureStopsOrder(t *testing.T) {
	svc := &PaymentService{rechargeBonusRepo: &pricingBonusRepository{err: errors.New("database unavailable")}}
	_, _, err := svc.quoteBalanceRecharge(context.Background(), &PaymentConfig{}, 100, "CNY")
	require.ErrorContains(t, err, "match recharge bonus rule")
}
