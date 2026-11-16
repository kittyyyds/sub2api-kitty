package service

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
)

// RechargeActivitySummary totals the recharges behind the order list filters.
type RechargeActivitySummary struct {
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// Count and amount must come from the same statement snapshot. COALESCE also
// permits an empty result without a preceding COUNT query.
func rechargeSummaryAggregates(field string) []dbent.AggregateFunc {
	return []dbent.AggregateFunc{
		dbent.As(dbent.Count(), "count"),
		func(selector *sql.Selector) string {
			return sql.As("COALESCE("+dbent.Sum(field)(selector)+", 0)", "amount")
		},
	}
}

// AdminRechargeActivitySummary totals the recharges in the order list's time,
// channel and email filters; the status filter does not apply. It counts what
// the operations "today recharge" metric counts — ordinary users' completed balance
// orders by completion time plus used balance and adjustment codes, without the
// PAY- codes that credit those orders — so all channels over today equal that
// metric.
func (s *PaymentService) AdminRechargeActivitySummary(ctx context.Context, filter RechargeActivityFilter) (RechargeActivitySummary, error) {
	var summary RechargeActivitySummary
	email := strings.TrimSpace(filter.Email)

	if filter.Channel == "" || filter.Channel == "direct" {
		q := s.entClient.PaymentOrder.Query().Where(
			paymentorder.OrderTypeEQ("balance"),
			paymentorder.CompletedAtNotNil(),
			paymentorder.HasUserWith(user.RoleEQ(RoleUser)),
		)
		if email != "" {
			q = q.Where(paymentorder.UserEmailContainsFold(email))
		}
		if !filter.StartTime.IsZero() {
			q = q.Where(paymentorder.CompletedAtGTE(filter.StartTime))
		}
		if !filter.EndTime.IsZero() {
			q = q.Where(paymentorder.CompletedAtLT(filter.EndTime))
		}
		var rows []RechargeActivitySummary
		if err := q.Aggregate(rechargeSummaryAggregates(paymentorder.FieldAmount)...).Scan(ctx, &rows); err != nil {
			return summary, fmt.Errorf("summarize direct recharges: %w", err)
		}
		if len(rows) > 0 {
			summary.Amount += rows[0].Amount
			summary.Count += rows[0].Count
		}
	}

	if filter.Channel != "direct" {
		q := s.entClient.RedeemCode.Query().Where(
			redeemcode.StatusEQ(StatusUsed), redeemcode.ValueGT(0), redeemcode.UsedAtNotNil(),
			redeemcode.Not(redeemcode.CodeHasPrefix("PAY-")),
			redeemcode.HasUserWith(user.RoleEQ(RoleUser)),
		)
		switch filter.Channel {
		case "redeem":
			q = q.Where(redeemcode.TypeEQ(RedeemTypeBalance))
		case "manual":
			q = q.Where(redeemcode.TypeEQ(AdjustmentTypeAdminBalance))
		default:
			q = q.Where(redeemcode.TypeIn(RedeemTypeBalance, AdjustmentTypeAdminBalance))
		}
		if email != "" {
			q = q.Where(redeemcode.HasUserWith(user.EmailContainsFold(email)))
		}
		if !filter.StartTime.IsZero() {
			q = q.Where(redeemcode.UsedAtGTE(filter.StartTime))
		}
		if !filter.EndTime.IsZero() {
			q = q.Where(redeemcode.UsedAtLT(filter.EndTime))
		}
		var rows []RechargeActivitySummary
		if err := q.Aggregate(rechargeSummaryAggregates(redeemcode.FieldValue)...).Scan(ctx, &rows); err != nil {
			return summary, fmt.Errorf("summarize code recharges: %w", err)
		}
		if len(rows) > 0 {
			summary.Amount += rows[0].Amount
			summary.Count += rows[0].Count
		}
	}
	return summary, nil
}
