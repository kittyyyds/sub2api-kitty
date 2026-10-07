package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// This proof is only produced after an authenticated provider query. It is never
// accepted from callback metadata, and lives for this fulfillment request only.
type easyPayProofKey struct{}
type easyPayProof struct {
	service             *PaymentService
	merchant            string
	orderID             int64
	outTradeNo, tradeNo string
	amount              float64
}

var easyPayQuerySlots = make(chan struct{}, 8)
var easyPayTransitionMu sync.Mutex // SQLite/test fallback; production uses a DB lock.

func (s *PaymentService) isEasyPayOrder(ctx context.Context, o *dbent.PaymentOrder) (bool, error) {
	// An explicit original provider keeps unrelated channels independent of a
	// subsequently deleted instance. PaymentType alone may mean an EasyPay method.
	if snapshot := psOrderProviderSnapshot(o); snapshot != nil && snapshot.ProviderKey != "" {
		return snapshot.ProviderKey == payment.TypeEasyPay, nil
	}
	if key := strings.TrimSpace(psStringValue(o.ProviderKey)); key != "" {
		return key == payment.TypeEasyPay, nil
	}
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return false, err
	}
	if inst != nil {
		return inst.ProviderKey == payment.TypeEasyPay, nil
	}
	if strings.TrimSpace(psStringValue(o.ProviderInstanceID)) != "" {
		return false, fmt.Errorf("payment provider binding is unresolved")
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().All(ctx)
	if err != nil {
		return false, err
	}
	matches := psFilterLegacyOrderProviderInstances(payment.GetBasePaymentType(o.PaymentType), instances)
	if len(matches) > 1 {
		return false, fmt.Errorf("legacy payment provider is ambiguous")
	}
	if strings.EqualFold(expectedNotificationProviderKeyForOrder(s.registry, o, ""), payment.TypeEasyPay) {
		return true, nil
	}
	if s.registry != nil && s.registry.GetProviderKey(payment.PaymentType(o.PaymentType)) != "" {
		return false, nil
	}
	return false, fmt.Errorf("legacy payment provider is unresolved")
}

func easyPayOrderMerchant(o *dbent.PaymentOrder) string {
	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return snapshot.ProviderInstanceID + ":" + snapshot.MerchantID
	}
	return psStringValue(o.ProviderInstanceID)
}

func easyPayCents(amount float64) (int64, bool) {
	if !isValidProviderAmount(amount) || amount > 1e12 {
		return 0, false
	}
	cents := math.Round(amount * 100)
	if math.Abs(amount*100-cents) > 1e-5 {
		return 0, false
	}
	return int64(cents), true
}

func (s *PaymentService) verifyEasyPayOrder(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, amount float64) (context.Context, error) {
	isEasyPay, err := s.isEasyPayOrder(ctx, o)
	if err != nil {
		return ctx, fmt.Errorf("resolve payment provider: %w", err)
	}
	if !isEasyPay {
		return ctx, nil
	}
	if p, ok := ctx.Value(easyPayProofKey{}).(easyPayProof); ok && p.service == s && p.merchant == easyPayOrderMerchant(o) && p.orderID == o.ID && p.outTradeNo == o.OutTradeNo && p.tradeNo == tradeNo && p.amount == amount {
		return ctx, nil
	}
	if strings.TrimSpace(o.OutTradeNo) == "" {
		return ctx, fmt.Errorf("easypay payment identifiers missing")
	}
	prov, err := s.getOrderProvider(ctx, o)
	if err != nil {
		return ctx, fmt.Errorf("resolve easypay provider: %w", err)
	}
	if prov.ProviderKey() != payment.TypeEasyPay {
		return ctx, fmt.Errorf("easypay provider mismatch")
	}
	if identity := providerMerchantIdentityMetadata(prov); len(identity) > 0 {
		if err := validateProviderSnapshotMetadata(o, payment.TypeEasyPay, identity); err != nil {
			return ctx, err
		}
	}
	queryCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	select {
	case easyPayQuerySlots <- struct{}{}:
		defer func() { <-easyPayQuerySlots }()
	case <-queryCtx.Done():
		return ctx, fmt.Errorf("easypay verification unavailable")
	}
	resp, err := prov.QueryOrder(queryCtx, o.OutTradeNo)
	if err != nil {
		// Provider errors can include HTTP URLs with merchant credentials.
		return ctx, fmt.Errorf("easypay verification unavailable; payment remains unverified")
	}
	if resp != nil && tradeNo == "" {
		tradeNo = resp.TradeNo
	}
	if o.PaymentTradeNo != "" && o.PaymentTradeNo != tradeNo {
		return ctx, fmt.Errorf("easypay existing transaction binding mismatch")
	}
	if err := validateEasyPayPaidQuery(o, resp, tradeNo, amount); err != nil {
		return ctx, err
	}
	if _, err := s.easyPayPaidTransition(ctx, o, tradeNo, amount, func(client *dbent.Client) (int, error) {
		if o.PaymentTradeNo == "" && (o.Status == OrderStatusPaid || o.Status == OrderStatusFailed || o.Status == OrderStatusRecharging) {
			return client.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.PaymentTradeNoEQ(""), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed, OrderStatusRecharging)).SetPaymentTradeNo(tradeNo).Save(ctx)
		}
		return 0, nil
	}); err != nil {
		return ctx, err
	}
	if o.PaymentTradeNo == "" && o.Status != OrderStatusPending {
		o.PaymentTradeNo = tradeNo
	}
	return context.WithValue(ctx, easyPayProofKey{}, easyPayProof{s, easyPayOrderMerchant(o), o.ID, o.OutTradeNo, tradeNo, amount}), nil
}

func validateEasyPayPaidQuery(o *dbent.PaymentOrder, resp *payment.QueryOrderResponse, tradeNo string, amount float64) error {
	if resp == nil || resp.Status != payment.ProviderStatusPaid {
		return fmt.Errorf("easypay platform has not confirmed payment")
	}
	if resp.Metadata["out_trade_no"] != o.OutTradeNo || strings.TrimSpace(resp.Metadata["pid"]) == "" {
		return fmt.Errorf("easypay queried order identity mismatch")
	}
	if err := validateProviderSnapshotMetadata(o, payment.TypeEasyPay, resp.Metadata); err != nil {
		return err
	}
	if strings.TrimSpace(resp.TradeNo) == "" || resp.TradeNo != tradeNo {
		return fmt.Errorf("easypay queried transaction mismatch")
	}
	if o.PaymentTradeNo != "" && o.PaymentTradeNo != resp.TradeNo {
		return fmt.Errorf("easypay existing transaction binding mismatch")
	}
	expected, expectedOK := easyPayCents(o.PayAmount)
	queried, queriedOK := easyPayCents(resp.Amount)
	notified, notifiedOK := easyPayCents(amount)
	// Some EasyPay channels add a few cents to identify a payment. Preserve the
	// existing overpayment limit without ever accepting an underpayment or a
	// callback amount different from the authenticated platform query. Balance
	// fulfillment continues to use o.Amount, not the extra cents in PayAmount.
	if !expectedOK || !queriedOK || !notifiedOK || queried < expected || notified != queried ||
		!isPaidAmountWithinProviderTolerance(o.PayAmount, resp.Amount, PaymentOrderCurrency(o)) {
		return fmt.Errorf("easypay verified amount mismatch")
	}
	return nil
}

// Serializing the duplicate check and state update across fixed app instances
// prevents a transaction number being claimed by two orders in the same DB.
// Cross-site binding also requires the platform's out_trade_no to match exactly.
func (s *PaymentService) easyPayPaidTransition(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, update func(*dbent.Client) (int, error)) (int, error) {
	if s.entClient.Driver().Dialect() != dialect.Postgres {
		easyPayTransitionMu.Lock()
		defer easyPayTransitionMu.Unlock()
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()
	if s.entClient.Driver().Dialect() == dialect.Postgres {
		digest := sha256.Sum256([]byte("easypay-paid:" + tradeNo))
		lockID := int64(binary.BigEndian.Uint64(digest[:8]))
		if _, err = client.ExecContext(ctx, "SET LOCAL lock_timeout = '2s'"); err != nil {
			return 0, err
		}
		if _, err = client.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
			return 0, err
		}
	}
	others, err := client.PaymentOrder.Query().Where(paymentorder.PaymentTradeNoEQ(tradeNo), paymentorder.IDNEQ(o.ID)).All(ctx)
	if err != nil {
		return 0, err
	}
	for _, other := range others {
		// Only an authoritative non-EasyPay identity can exclude an old owner.
		key := strings.TrimSpace(psStringValue(other.ProviderKey))
		if snapshot := psOrderProviderSnapshot(other); snapshot != nil && snapshot.ProviderKey != "" {
			key = snapshot.ProviderKey
		}
		if key != "" && key != payment.TypeEasyPay {
			continue
		}
		thisSnapshot, otherSnapshot := psOrderProviderSnapshot(o), psOrderProviderSnapshot(other)
		if thisSnapshot != nil && otherSnapshot != nil && thisSnapshot.MerchantID != "" && otherSnapshot.MerchantID != "" && thisSnapshot.MerchantID != otherSnapshot.MerchantID {
			continue
		}
		return 0, fmt.Errorf("easypay transaction is already bound to another order")
	}
	n, err := update(client)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
