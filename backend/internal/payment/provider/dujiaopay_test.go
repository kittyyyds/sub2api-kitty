//go:build unit

package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestNewDujiaoPayValidatesAndDefaultsConfig(t *testing.T) {
	t.Parallel()

	_, err := NewDujiaoPay("1", map[string]string{"apiSecret": "secret"})
	require.ErrorContains(t, err, "keyId")

	_, err = NewDujiaoPay("1", map[string]string{
		"keyId":     "ak_test",
		"apiSecret": "secret",
		"apiBase":   "https://evil.example.com",
	})
	require.ErrorContains(t, err, "apiBase host")

	provider, err := NewDujiaoPay("1", map[string]string{
		"keyId":     "ak_test",
		"apiSecret": "secret",
	})
	require.NoError(t, err)
	require.Equal(t, dujiaoPayDefaultAPIBase, provider.config["apiBase"])
	require.Equal(t, payment.DefaultPaymentCurrency, provider.config["currency"])
	require.Equal(t, payment.TypeDujiaoPay, provider.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeDujiaoPay}, provider.SupportedTypes())
}

func TestCanonicalDujiaoPayQuerySortsKeysAndValues(t *testing.T) {
	t.Parallel()

	require.Equal(t, "a=hello%20world&a=z&b=2", canonicalDujiaoPayQuery("b=2&a=z&a=hello+world"))
}

func TestDujiaoPayCreateQueryAndCancel(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		verifyTestDujiaoPayRequest(t, r, body, "api-secret")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/orders":
			require.Equal(t, "sub2_order_1", r.Header.Get("Idempotency-Key"))
			var payload dujiaoPayCreateOrderRequest
			require.NoError(t, json.Unmarshal(body, &payload))
			require.Equal(t, "CNY", payload.FiatCurrency)
			require.Equal(t, "12.30", payload.FiatAmount)
			require.Equal(t, "sub2_order_1", payload.MerchantOrderID)
			require.Equal(t, "sub2_order_1", payload.Metadata["out_trade_no"])
			require.Equal(t, "https://merchant.example/payment/result", payload.SuccessURL)
			_, _ = w.Write([]byte(`{"order_id":"do_123","status":"awaiting_payment","checkout_token":"ct_once","checkout_url":"https://www.dujiaopay.com/checkout/ct_once"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/orders/do_123":
			_, _ = w.Write([]byte(`{"order_id":"do_123","status":"paid","fiat_currency":"CNY","fiat_amount":"12.30","merchant_order_id":"sub2_order_1","chain":"tron","token_id":"tron-usdt","tx_id":"tx_123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/orders/do_123/cancel":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestDujiaoPayProvider(t, server)
	created, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order_1",
		Amount:    "12.3",
		ReturnURL: "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "do_123", created.TradeNo)
	require.Equal(t, "https://www.dujiaopay.com/checkout/ct_once", created.PayURL)
	require.Equal(t, "CNY", created.Currency)

	queried, err := provider.QueryOrder(context.Background(), "do_123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	require.Equal(t, 12.3, queried.Amount)
	require.Equal(t, "CNY", queried.Metadata["currency"])
	require.Equal(t, "ak_test", queried.Metadata["key_id"])

	require.NoError(t, provider.CancelPayment(context.Background(), "do_123"))
	require.Equal(t, 3, requestCount)
}

func TestDujiaoPayVerifyPaidWebhook(t *testing.T) {
	t.Parallel()

	provider, err := NewDujiaoPay("1", map[string]string{
		"keyId":         "ak_test",
		"apiSecret":     "api-secret",
		"webhookSecret": "webhook-secret",
	})
	require.NoError(t, err)

	rawBody := `{"event_id":"evt_1","event_type":"order.paid","event_version":"v1","created_at":"2026-06-06T12:00:00Z","data":{"order_id":"do_123","status":"paid","chain":"tron","token_id":"tron-usdt","payable_amount":"12.3001","fiat_currency":"CNY","fiat_amount":"12.30","tx_id":"tx_123","confirmations":19}}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write([]byte(timestamp + "." + rawBody))
	headers := map[string]string{
		"djp-webhook-id":        "evt_1",
		"djp-webhook-timestamp": timestamp,
		"djp-webhook-signature": hex.EncodeToString(mac.Sum(nil)),
	}

	notification, err := provider.VerifyNotification(context.Background(), rawBody, headers)
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "do_123", notification.TradeNo)
	require.Empty(t, notification.OrderID)
	require.Equal(t, 12.3, notification.Amount)
	require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	require.Equal(t, "CNY", notification.Metadata["currency"])
	require.Equal(t, "ak_test", notification.Metadata["key_id"])

	headers["djp-webhook-signature"] = "00"
	_, err = provider.VerifyNotification(context.Background(), rawBody, headers)
	require.ErrorContains(t, err, "invalid webhook signature")
}

func TestDujiaoPayWebhookExtractsMerchantOrderIDFromMetadata(t *testing.T) {
	t.Parallel()

	provider, err := NewDujiaoPay("1", map[string]string{
		"keyId":         "ak_test",
		"apiSecret":     "api-secret",
		"webhookSecret": "webhook-secret",
	})
	require.NoError(t, err)

	rawBody := `{"event_id":"evt_2","event_type":"order.paid","data":{"order_id":"do_456","status":"paid","fiat_currency":"CNY","fiat_amount":"5.00","metadata":{"out_trade_no":"sub2_order_456"}}}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write([]byte(timestamp + "." + rawBody))

	notification, err := provider.VerifyNotification(context.Background(), rawBody, map[string]string{
		"DJP-Webhook-Timestamp": timestamp,
		"DJP-Webhook-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
	})
	require.NoError(t, err)
	require.Equal(t, "sub2_order_456", notification.OrderID)
}

func TestDujiaoPayWebhookRejectsStaleTimestamp(t *testing.T) {
	t.Parallel()

	rawBody := `{}`
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write([]byte(timestamp + "." + rawBody))
	err := verifyDujiaoPayWebhookSignature(rawBody, map[string]string{
		"djp-webhook-timestamp": timestamp,
		"djp-webhook-signature": hex.EncodeToString(mac.Sum(nil)),
	}, "webhook-secret", time.Now())
	require.ErrorContains(t, err, "outside tolerance")
}

func mustTestDujiaoPayProvider(t *testing.T, server *httptest.Server) *DujiaoPay {
	t.Helper()
	provider, err := NewDujiaoPay("1", map[string]string{
		"keyId":         "ak_test",
		"apiSecret":     "api-secret",
		"webhookSecret": "webhook-secret",
		"currency":      "CNY",
	})
	require.NoError(t, err)
	provider.config["apiBase"] = server.URL
	provider.httpClient = server.Client()
	return provider
}

func verifyTestDujiaoPayRequest(t *testing.T, r *http.Request, body []byte, secret string) {
	t.Helper()
	require.Equal(t, "ak_test", r.Header.Get("DJP-Key-ID"))
	timestamp, err := strconv.ParseInt(r.Header.Get("DJP-Timestamp"), 10, 64)
	require.NoError(t, err)
	require.NotEmpty(t, r.Header.Get("DJP-Nonce"))
	expected := signDujiaoPayRequest(
		secret,
		"ak_test",
		r.Method,
		r.URL.EscapedPath(),
		r.URL.RawQuery,
		body,
		timestamp,
		r.Header.Get("DJP-Nonce"),
	)
	require.Equal(t, expected["DJP-Signature"], r.Header.Get("DJP-Signature"))
}
