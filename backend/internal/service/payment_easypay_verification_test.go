//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type easyPayVerificationProvider struct {
	paymentFulfillmentTestProvider
	resp  *payment.QueryOrderResponse
	err   error
	calls int
}

func (p *easyPayVerificationProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	p.calls++
	return p.resp, p.err
}

func easyPayVerificationFixture(t *testing.T, status string) (*PaymentService, *dbent.PaymentOrder, *easyPayVerificationProvider, *mockUserRepo, *paymentOrderLifecycleRedeemRepo) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	u, err := client.User.Create().SetEmail("verification@example.test").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	o, err := client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName("test").SetAmount(100).SetPayAmount(100).SetFeeRate(0).SetRechargeCode("VERIFY-REDEEM").SetOutTradeNo("sub2_verify_test").SetPaymentType(payment.TypeEasyPay).SetPaymentTradeNo("platform-transaction").SetOrderType(payment.OrderTypeBalance).SetStatus(status).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("test.invalid").Save(ctx)
	require.NoError(t, err)
	users := &mockUserRepo{getByIDUser: &User{ID: u.ID, Email: u.Email}}
	users.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		users.getByIDUser.Balance += amount
		return nil
	}
	redeem := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{o.RechargeCode: {ID: 1, Code: o.RechargeCode, Type: RedeemTypeBalance, Value: 100, Status: StatusUnused}}}
	p := &easyPayVerificationProvider{paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeEasyPay, supportedTypes: []payment.PaymentType{payment.TypeEasyPay}}, resp: &payment.QueryOrderResponse{TradeNo: o.PaymentTradeNo, Status: payment.ProviderStatusPaid, Amount: 100, Metadata: map[string]string{"pid": "123", "out_trade_no": o.OutTradeNo}}}
	registry := payment.NewRegistry()
	registry.Register(p)
	s := &PaymentService{entClient: client, registry: registry, providersLoaded: true, userRepo: users, redeemService: NewRedeemService(redeem, users, nil, nil, nil, client, nil, nil)}
	return s, o, p, users, redeem
}

func TestEasyPayUnverifiedNotificationCannotCredit(t *testing.T) {
	cases := map[string]func(*easyPayVerificationProvider){
		"query_error":      func(p *easyPayVerificationProvider) { p.err = errors.New("private provider failure") },
		"nil_response":     func(p *easyPayVerificationProvider) { p.resp = nil },
		"unpaid":           func(p *easyPayVerificationProvider) { p.resp.Status = payment.ProviderStatusPending },
		"foreign_order":    func(p *easyPayVerificationProvider) { p.resp.Metadata["out_trade_no"] = "foreign" },
		"missing_merchant": func(p *easyPayVerificationProvider) { delete(p.resp.Metadata, "pid") },
		"missing_trade":    func(p *easyPayVerificationProvider) { p.resp.TradeNo = "" },
		"wrong_trade":      func(p *easyPayVerificationProvider) { p.resp.TradeNo = "another" },
		"wrong_amount":     func(p *easyPayVerificationProvider) { p.resp.Amount = 99.99 },
		"fractional_cent":  func(p *easyPayVerificationProvider) { p.resp.Amount = 100.001 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, o, p, u, r := easyPayVerificationFixture(t, OrderStatusPending)
			mutate(p)
			err := s.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 100, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123", "verified": "true"}}, payment.TypeEasyPay)
			require.Error(t, err)
			got, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, got.Status)
			require.Zero(t, u.getByIDUser.Balance)
			require.Empty(t, r.useCalls)
		})
	}
}

func TestEasyPayCouponNotificationStillRequiresProviderVerification(t *testing.T) {
	s, o, p, u, r := easyPayVerificationFixture(t, OrderStatusPending)
	ctx := context.Background()
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetProviderSnapshot(map[string]any{"recharge_coupon_id": 42}).ExecX(ctx)
	p.resp.Status = payment.ProviderStatusPending
	err := s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 100, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}, payment.TypeEasyPay)
	require.Error(t, err)
	require.Equal(t, OrderStatusPending, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Zero(t, u.getByIDUser.Balance)
	require.Empty(t, r.useCalls)
}

func TestEasyPayCouponCannotReuseAnotherOrdersTransaction(t *testing.T) {
	s, o, _, u, _ := easyPayVerificationFixture(t, OrderStatusPending)
	ctx := context.Background()
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetProviderSnapshot(map[string]any{"recharge_coupon_id": 42}).SaveX(ctx)
	s.entClient.PaymentOrder.Create().SetUserID(o.UserID).SetUserEmail(o.UserEmail).SetUserName("test").SetAmount(100).SetPayAmount(100).SetRechargeCode("OTHER").SetOutTradeNo("sub2_other_coupon").SetPaymentType(payment.TypeEasyPay).SetPaymentTradeNo(o.PaymentTradeNo).SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now()).SetClientIP("127.0.0.1").SetSrcHost("test.invalid").SaveX(ctx)
	err := s.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 100, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}, payment.TypeEasyPay)
	require.ErrorContains(t, err, "already bound")
	require.Equal(t, OrderStatusPending, s.entClient.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Zero(t, u.getByIDUser.Balance)
}

func TestEasyPayVerifiedPaymentCreditsOnceAndUsesOneQuery(t *testing.T) {
	s, o, p, u, r := easyPayVerificationFixture(t, OrderStatusPending)
	n := &payment.PaymentNotification{OrderID: o.OutTradeNo, TradeNo: o.PaymentTradeNo, Amount: 100, Status: payment.NotificationStatusSuccess, Metadata: map[string]string{"pid": "123"}}
	require.NoError(t, s.HandlePaymentNotification(context.Background(), n, payment.TypeEasyPay))
	require.Equal(t, 1, p.calls)
	require.NoError(t, s.HandlePaymentNotification(context.Background(), n, payment.TypeEasyPay))
	require.Equal(t, 100.0, u.getByIDUser.Balance)
	require.Len(t, r.useCalls, 1)
}

func TestEasyPayPaidRetryCannotTrustOldStatus(t *testing.T) {
	for _, status := range []string{OrderStatusPaid, OrderStatusFailed, OrderStatusRecharging} {
		t.Run(status, func(t *testing.T) {
			s, o, p, u, r := easyPayVerificationFixture(t, status)
			p.resp.Status = payment.ProviderStatusPending
			require.Error(t, s.ExecuteBalanceFulfillment(context.Background(), o.ID))
			require.Zero(t, u.getByIDUser.Balance)
			require.Empty(t, r.useCalls)
			_, err := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetOrderType(payment.OrderTypeSubscription).SetSubscriptionGroupID(1).SetSubscriptionDays(30).Save(context.Background())
			require.NoError(t, err)
			require.Error(t, s.ExecuteSubscriptionFulfillment(context.Background(), o.ID))
			require.Equal(t, 2, p.calls)
		})
	}
}

func TestEasyPayReconcileUsesVerifiedEvidenceWithoutDoubleQuery(t *testing.T) {
	s, o, p, u, _ := easyPayVerificationFixture(t, OrderStatusPending)
	require.Equal(t, checkPaidResultAlreadyPaid, s.reconcilePaid(context.Background(), o))
	require.Equal(t, 1, p.calls)
	require.Equal(t, 100.0, u.getByIDUser.Balance)
}

func TestEasyPayRejectsHistoricalTransactionReuse(t *testing.T) {
	s, o, p, u, _ := easyPayVerificationFixture(t, OrderStatusPending)
	_, err := s.entClient.PaymentOrder.Create().SetUserID(o.UserID).SetUserEmail(o.UserEmail).SetUserName("test").SetAmount(100).SetPayAmount(100).SetFeeRate(0).SetRechargeCode("OLD-REDEEM").SetOutTradeNo("sub2_old").SetPaymentType(payment.TypeEasyPay).SetPaymentTradeNo(o.PaymentTradeNo).SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusCompleted).SetExpiresAt(time.Now()).SetClientIP("127.0.0.1").SetSrcHost("test.invalid").Save(context.Background())
	require.NoError(t, err)
	require.Empty(t, s.reconcilePaid(context.Background(), o))
	require.Equal(t, 1, p.calls)
	require.Zero(t, u.getByIDUser.Balance)
	got, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, got.Status)
}

func TestEasyPayProofCannotAuthorizeAnotherService(t *testing.T) {
	s, o, p, _, _ := easyPayVerificationFixture(t, OrderStatusPending)
	ctx, err := s.verifyEasyPayOrder(context.Background(), o, o.PaymentTradeNo, o.PayAmount)
	require.NoError(t, err)
	other := &PaymentService{entClient: s.entClient, registry: s.registry, providersLoaded: true}
	p.resp.Status = payment.ProviderStatusPending
	_, err = other.verifyEasyPayOrder(ctx, o, o.PaymentTradeNo, o.PayAmount)
	require.Error(t, err)
}

func TestEasyPayLegacyAmbiguousProviderCannotFulfill(t *testing.T) {
	s, o, _, u, _ := easyPayVerificationFixture(t, OrderStatusPaid)
	ctx := context.Background()
	_, err := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPaymentType(payment.TypeAlipay).Save(ctx)
	require.NoError(t, err)
	s.registry = payment.NewRegistry()
	for _, name := range []string{"first", "second"} {
		_, err = s.entClient.PaymentProviderInstance.Create().SetName(name).SetProviderKey(payment.TypeEasyPay).SetConfig("{}").SetSupportedTypes("alipay").Save(ctx)
		require.NoError(t, err)
	}
	require.ErrorContains(t, s.ExecuteBalanceFulfillment(ctx, o.ID), "ambiguous")
	require.Zero(t, u.getByIDUser.Balance)
}

func TestEasyPayLegacyMissingProviderCannotFulfill(t *testing.T) {
	s, o, _, u, _ := easyPayVerificationFixture(t, OrderStatusPaid)
	_, err := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPaymentType(payment.TypeAlipay).Save(context.Background())
	require.NoError(t, err)
	s.registry = payment.NewRegistry()
	require.ErrorContains(t, s.ExecuteBalanceFulfillment(context.Background(), o.ID), "unresolved")
	require.Zero(t, u.getByIDUser.Balance)
}

func TestEasyPayLegacyMissingTransactionRecoveredFromPlatform(t *testing.T) {
	s, o, p, u, _ := easyPayVerificationFixture(t, OrderStatusPaid)
	_, err := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPaymentTradeNo("").Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.ExecuteBalanceFulfillment(context.Background(), o.ID))
	require.Equal(t, 100.0, u.getByIDUser.Balance)
	require.Equal(t, 1, p.calls)
	got, err := s.entClient.PaymentOrder.Get(context.Background(), o.ID)
	require.NoError(t, err)
	require.Equal(t, p.resp.TradeNo, got.PaymentTradeNo)
}

func TestEasyPayGuardPreservesKnownOtherProviderFulfillment(t *testing.T) {
	s, o, p, u, _ := easyPayVerificationFixture(t, OrderStatusPaid)
	_, err := s.entClient.PaymentOrder.UpdateOneID(o.ID).SetProviderKey(payment.TypeStripe).SetProviderInstanceID("999999").Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.ExecuteBalanceFulfillment(context.Background(), o.ID))
	require.Equal(t, 100.0, u.getByIDUser.Balance)
	require.Zero(t, p.calls)
}

func TestPaymentOrderNumberFitsWeChatLimit(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := generateOutTradeNo()
		require.LessOrEqual(t, len(id), 32)
		require.NotContains(t, seen, id)
		seen[id] = true
	}
}
