//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func easyPayTenYuanFixture(t *testing.T) (*PaymentService, *dbent.PaymentOrder, *easyPayVerificationProvider, *mockUserRepo, *paymentOrderLifecycleRedeemRepo) {
	t.Helper()
	s, o, p, u, r := easyPayVerificationFixture(t, OrderStatusPending)
	var err error
	o, err = s.entClient.PaymentOrder.UpdateOne(o).SetAmount(10).SetPayAmount(10).Save(context.Background())
	require.NoError(t, err)
	r.codesByCode[o.RechargeCode].Value = 10
	p.resp.Amount = 10.01
	return s, o, p, u, r
}

func TestEasyPayOverpayCreditsPrincipalOnceAcrossEntryPoints(t *testing.T) {
	for _, entry := range []string{"callback", "reconcile", "user_verify"} {
		for _, paid := range []float64{10, 10.01, 10.05} {
			t.Run(entry+"/"+payment.FormatAmountForCurrency(paid, "CNY"), func(t *testing.T) {
				s, o, p, u, r := easyPayTenYuanFixture(t)
				p.resp.Amount = paid
				for range 2 {
					switch entry {
					case "callback":
						require.NoError(t, s.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: paid, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}, payment.TypeEasyPay))
					case "reconcile":
						current, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
						require.NoError(t, err)
						require.Equal(t, checkPaidResultAlreadyPaid, s.reconcilePaid(context.Background(), current))
					case "user_verify":
						got, err := s.VerifyOrderByOutTradeNo(context.Background(), o.OutTradeNo, o.UserID)
						require.NoError(t, err)
						require.Equal(t, OrderStatusCompleted, got.Status)
					}
				}
				got, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
				require.NoError(t, err)
				require.Equal(t, OrderStatusCompleted, got.Status)
				require.Equal(t, paid, got.PayAmount)
				require.Equal(t, 10.0, got.Amount)
				require.Equal(t, 10.0, u.getByIDUser.Balance)
				require.Len(t, r.useCalls, 1)
			})
		}
	}
}

func TestEasyPayOverpayStillRejectsUntrustedOrOutOfRangePayments(t *testing.T) {
	for _, tc := range []struct {
		name              string
		queried, notified float64
	}{
		{"underpayment", 9.99, 9.99},
		{"beyond_five_cents", 10.06, 10.06},
		{"callback_differs_from_query", 10.01, 10.02},
		{"callback_hides_underpayment", 9.99, 10.01},
		{"fractional_cent", 10.001, 10.001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, o, p, u, r := easyPayTenYuanFixture(t)
			p.resp.Amount = tc.queried
			err := s.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: tc.notified, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}, payment.TypeEasyPay)
			require.Error(t, err)
			got, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, got.Status)
			require.Zero(t, u.getByIDUser.Balance)
			require.Empty(t, r.useCalls)
		})
	}
}

func TestEasyPayExpiredManuallyCompensatedOrderIsNotCreditedAgain(t *testing.T) {
	s, o, _, u, r := easyPayTenYuanFixture(t)
	ctx := context.Background()
	var err error
	o, err = s.entClient.PaymentOrder.UpdateOne(o).SetStatus(OrderStatusExpired).SetExpiresAt(time.Now().Add(-time.Hour)).SetUpdatedAt(time.Now().Add(-30 * time.Minute)).Save(ctx)
	require.NoError(t, err)
	u.getByIDUser.Balance = 10 // Existing administrator compensation, outside payment fulfillment.
	recovered, err := s.ReconcilePendingPaymentOrders(ctx)
	require.NoError(t, err)
	require.Zero(t, recovered)
	require.NoError(t, s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 10.01, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}, payment.TypeEasyPay))
	_, err = s.VerifyOrderByOutTradeNo(ctx, o.OutTradeNo, o.UserID)
	require.NoError(t, err)
	got, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, got.Status)
	require.Equal(t, 10.0, u.getByIDUser.Balance)
	require.Empty(t, r.useCalls)
}

func TestEasyPayRepeatedVerificationPreservesFirstAudit(t *testing.T) {
	s, o, p, _, _ := easyPayTenYuanFixture(t)
	ctx := context.Background()
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, s.entClient)
	p.resp.Amount = 10.06
	for range 3 {
		require.Empty(t, s.reconcilePaid(ctx, o))
	}
	logs, err := s.GetOrderAuditLogs(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "PAYMENT_VERIFICATION_PENDING", logs[0].Action)
	require.Contains(t, logs[0].Detail, "verified amount mismatch")
}
