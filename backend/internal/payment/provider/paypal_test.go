//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

var _ payment.Provider = (*PayPal)(nil)
var _ payment.RefundQueryProvider = (*PayPal)(nil)

func TestNewPayPalValidatesAndDefaultsConfig(t *testing.T) {
	t.Parallel()

	_, err := NewPayPal("1", map[string]string{
		"clientSecret": "secret",
		"webhookId":    "WH-1",
	})
	require.ErrorContains(t, err, "clientId")

	_, err = NewPayPal("1", map[string]string{
		"clientId":     "client-id",
		"clientSecret": "secret",
		"webhookId":    "WH-1",
		"environment":  "staging",
	})
	require.ErrorContains(t, err, "sandbox or live")

	provider, err := NewPayPal("1", map[string]string{
		"clientId":     "client-id",
		"clientSecret": "secret",
		"webhookId":    "WH-1",
	})
	require.NoError(t, err)
	require.Equal(t, payPalSandboxAPIBase, provider.apiBase)
	require.Equal(t, payPalEnvironmentSandbox, provider.config["environment"])
	require.Equal(t, "USD", provider.config["currency"])
	require.Equal(t, payment.TypePayPal, provider.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypePayPal}, provider.SupportedTypes())
	require.Equal(t, map[string]string{
		"client_id":   "client-id",
		"currency":    "USD",
		"environment": "sandbox",
	}, provider.MerchantIdentityMetadata())

	live, err := NewPayPal("2", map[string]string{
		"clientId":     "client-id",
		"clientSecret": "secret",
		"webhookId":    "WH-2",
		"environment":  "LIVE",
		"currency":     "eur",
	})
	require.NoError(t, err)
	require.Equal(t, payPalLiveAPIBase, live.apiBase)
	require.Equal(t, "EUR", live.config["currency"])
}

func TestPayPalCreatePaymentUsesHostedApproval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/oauth2/token":
			assertPayPalTokenRequest(t, r)
			writePayPalJSON(w, `{"access_token":"access-create","expires_in":3600}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders":
			require.Equal(t, "Bearer access-create", r.Header.Get("Authorization"))
			require.NotEmpty(t, r.Header.Get("PayPal-Request-Id"))
			var payload payPalCreateOrderRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "CAPTURE", payload.Intent)
			require.Len(t, payload.PurchaseUnits, 1)
			unit := payload.PurchaseUnits[0]
			require.Equal(t, "sub2_create_1", unit.ReferenceID)
			require.Equal(t, "sub2_create_1", unit.CustomID)
			require.Equal(t, "sub2_create_1", unit.InvoiceID)
			require.Equal(t, "USD", unit.Amount.CurrencyCode)
			require.Equal(t, "12.30", unit.Amount.Value)
			require.Equal(t, "https://merchant.example/payment/result", payload.ApplicationContext.ReturnURL)
			require.Equal(t, "https://merchant.example/payment/result", payload.ApplicationContext.CancelURL)
			require.Equal(t, "Kitty API", payload.ApplicationContext.BrandName)
			writePayPalJSON(w, `{"id":"ORDER-CREATE-1","status":"CREATED","links":[{"href":"https://www.sandbox.paypal.com/checkoutnow?token=ORDER-CREATE-1","rel":"approve"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "create")
	created, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_create_1",
		Amount:    "12.3",
		Subject:   "Balance recharge",
		ReturnURL: "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "ORDER-CREATE-1", created.TradeNo)
	require.Equal(t, "https://www.sandbox.paypal.com/checkoutnow?token=ORDER-CREATE-1", created.PayURL)
	require.Equal(t, "USD", created.Currency)
}

func TestPayPalQueryApprovedOrderCapturesAndReturnsCaptureID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			writePayPalJSON(w, `{"access_token":"access-query","expires_in":3600}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/checkout/orders/ORDER-QUERY-1":
			writePayPalJSON(w, `{"id":"ORDER-QUERY-1","status":"APPROVED","purchase_units":[{"custom_id":"sub2_query_1","invoice_id":"sub2_query_1","amount":{"currency_code":"USD","value":"9.50"}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders/ORDER-QUERY-1/capture":
			require.NotEmpty(t, r.Header.Get("PayPal-Request-Id"))
			writePayPalJSON(w, completedPayPalOrderJSON("ORDER-QUERY-1", "CAPTURE-QUERY-1", "sub2_query_1", "9.50"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "query-approved")
	queried, err := provider.QueryOrder(context.Background(), "ORDER-QUERY-1")
	require.NoError(t, err)
	require.Equal(t, "CAPTURE-QUERY-1", queried.TradeNo)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	require.Equal(t, 9.5, queried.Amount)
	require.Equal(t, "USD", queried.Metadata["currency"])
	require.Equal(t, "COMPLETED", queried.Metadata["status"])
	require.Equal(t, "ORDER-QUERY-1", queried.Metadata["paypal_order_id"])
}

func TestPayPalCaptureConflictRequeriesCompletedOrder(t *testing.T) {
	orderQueries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			writePayPalJSON(w, `{"access_token":"access-conflict","expires_in":3600}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/checkout/orders/ORDER-CONFLICT":
			orderQueries++
			if orderQueries == 1 {
				writePayPalJSON(w, `{"id":"ORDER-CONFLICT","status":"APPROVED","purchase_units":[{"custom_id":"sub2_conflict","amount":{"currency_code":"USD","value":"7.00"}}]}`)
				return
			}
			writePayPalJSON(w, completedPayPalOrderJSON("ORDER-CONFLICT", "CAPTURE-CONFLICT", "sub2_conflict", "7.00"))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders/ORDER-CONFLICT/capture":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"ORDER_ALREADY_CAPTURED"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "capture-conflict")
	queried, err := provider.QueryOrder(context.Background(), "ORDER-CONFLICT")
	require.NoError(t, err)
	require.Equal(t, "CAPTURE-CONFLICT", queried.TradeNo)
	require.Equal(t, payment.ProviderStatusPaid, queried.Status)
	require.Equal(t, 2, orderQueries)
}

func TestPayPalQueryFallsBackFromOrderToCapture(t *testing.T) {
	t.Run("completed capture", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/oauth2/token":
				writePayPalJSON(w, `{"access_token":"access-capture","expires_in":3600}`)
			case r.Method == http.MethodGet && r.URL.Path == "/v2/checkout/orders/CAPTURE-FALLBACK":
				http.NotFound(w, r)
			case r.Method == http.MethodGet && r.URL.Path == "/v2/payments/captures/CAPTURE-FALLBACK":
				writePayPalJSON(w, `{"id":"CAPTURE-FALLBACK","status":"COMPLETED","amount":{"currency_code":"USD","value":"6.25"},"custom_id":"sub2_fallback","create_time":"2026-07-20T08:00:00Z"}`)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		provider := mustTestPayPalProvider(t, server, "capture-fallback")
		queried, err := provider.QueryOrder(context.Background(), "CAPTURE-FALLBACK")
		require.NoError(t, err)
		require.Equal(t, payment.ProviderStatusPaid, queried.Status)
		require.Equal(t, 6.25, queried.Amount)
		require.Equal(t, "2026-07-20T08:00:00Z", queried.PaidAt)
	})

	t.Run("capture error is reported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/oauth2/token":
				writePayPalJSON(w, `{"access_token":"access-capture-error","expires_in":3600}`)
			case strings.HasPrefix(r.URL.Path, "/v2/checkout/orders/"):
				http.NotFound(w, r)
			case strings.HasPrefix(r.URL.Path, "/v2/payments/captures/"):
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"name":"INTERNAL_SERVER_ERROR","message":"capture lookup failed"}`)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		provider := mustTestPayPalProvider(t, server, "capture-error")
		_, err := provider.QueryOrder(context.Background(), "CAPTURE-ERROR")
		require.ErrorContains(t, err, "paypal query capture")
		require.ErrorContains(t, err, "capture lookup failed")
	})
}

func TestPayPalApprovedWebhookVerifiesAndCaptures(t *testing.T) {
	rawBody := `{"id":"WH-EVT-APPROVED","event_type":"CHECKOUT.ORDER.APPROVED","summary":"approved event must survive verification","resource":{"id":"ORDER-WEBHOOK-1","status":"APPROVED","purchase_units":[{"custom_id":"sub2_webhook_1","invoice_id":"sub2_webhook_1","amount":{"currency_code":"USD","value":"11.00"}}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			writePayPalJSON(w, `{"access_token":"access-webhook","expires_in":3600}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notifications/verify-webhook-signature":
			var payload payPalVerifyWebhookRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "WH-CONFIGURED", payload.WebhookID)
			require.Contains(t, string(payload.WebhookEvent), "approved event must survive verification")
			require.Contains(t, string(payload.WebhookEvent), "ORDER-WEBHOOK-1")
			writePayPalJSON(w, `{"verification_status":"SUCCESS"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders/ORDER-WEBHOOK-1/capture":
			writePayPalJSON(w, completedPayPalOrderJSON("ORDER-WEBHOOK-1", "CAPTURE-WEBHOOK-1", "sub2_webhook_1", "11.00"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "approved-webhook")
	notification, err := provider.VerifyNotification(context.Background(), rawBody, testPayPalWebhookHeaders())
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "CAPTURE-WEBHOOK-1", notification.TradeNo)
	require.Equal(t, "sub2_webhook_1", notification.OrderID)
	require.Equal(t, 11.0, notification.Amount)
	require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	require.Equal(t, "WH-EVT-APPROVED", notification.Metadata["event_id"])
}

func TestPayPalCompletedCaptureWebhook(t *testing.T) {
	rawBody := `{"id":"WH-EVT-CAPTURE","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE-WEBHOOK-2","status":"COMPLETED","amount":{"currency_code":"USD","value":"13.75"},"custom_id":"sub2_webhook_2","invoice_id":"sub2_webhook_2","create_time":"2026-07-20T09:00:00Z","supplementary_data":{"related_ids":{"order_id":"ORDER-WEBHOOK-2"}}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			writePayPalJSON(w, `{"access_token":"access-capture-webhook","expires_in":3600}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notifications/verify-webhook-signature":
			writePayPalJSON(w, `{"verification_status":"SUCCESS"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "capture-webhook")
	notification, err := provider.VerifyNotification(context.Background(), rawBody, testPayPalWebhookHeaders())
	require.NoError(t, err)
	require.NotNil(t, notification)
	require.Equal(t, "CAPTURE-WEBHOOK-2", notification.TradeNo)
	require.Equal(t, "sub2_webhook_2", notification.OrderID)
	require.Equal(t, 13.75, notification.Amount)
	require.Equal(t, "ORDER-WEBHOOK-2", notification.Metadata["paypal_order_id"])
	require.Equal(t, "USD", notification.Metadata["currency"])
}

func TestPayPalRefundAndQueryRefund(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			writePayPalJSON(w, `{"access_token":"access-refund","expires_in":3600}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/payments/captures/CAPTURE-REFUND/refund":
			require.NotEmpty(t, r.Header.Get("PayPal-Request-Id"))
			var payload payPalRefundRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, payPalMoney{CurrencyCode: "USD", Value: "4.25"}, payload.Amount)
			require.Equal(t, "customer request", payload.NoteToPayer)
			writePayPalJSON(w, `{"id":"REFUND-1","status":"PENDING"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/payments/refunds/REFUND-1":
			writePayPalJSON(w, `{"id":"REFUND-1","status":"COMPLETED"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := mustTestPayPalProvider(t, server, "refund")
	refunded, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "CAPTURE-REFUND",
		OrderID: "sub2_refund_1",
		Amount:  "4.25",
		Reason:  "customer request",
	})
	require.NoError(t, err)
	require.Equal(t, "REFUND-1", refunded.RefundID)
	require.Equal(t, payment.ProviderStatusPending, refunded.Status)

	queried, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{RefundID: "REFUND-1"})
	require.NoError(t, err)
	require.Equal(t, "REFUND-1", queried.RefundID)
	require.Equal(t, payment.ProviderStatusSuccess, queried.Status)
}

func mustTestPayPalProvider(t *testing.T, server *httptest.Server, suffix string) *PayPal {
	t.Helper()
	provider, err := NewPayPal("paypal-test-"+suffix, map[string]string{
		"clientId":     "client-id",
		"clientSecret": "client-secret",
		"webhookId":    "WH-CONFIGURED",
		"environment":  "sandbox",
		"currency":     "USD",
		"brandName":    "Kitty API",
	})
	require.NoError(t, err)
	provider.apiBase = server.URL
	provider.httpClient = server.Client()
	payPalAccessTokens.Delete(provider.tokenCacheKey())
	t.Cleanup(func() { payPalAccessTokens.Delete(provider.tokenCacheKey()) })
	return provider
}

func assertPayPalTokenRequest(t *testing.T, r *http.Request) {
	t.Helper()
	clientID, secret, ok := r.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "client-id", clientID)
	require.Equal(t, "client-secret", secret)
	require.Equal(t, "grant_type=client_credentials", readTestRequestBody(t, r))
}

func readTestRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(body)
}

func writePayPalJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func completedPayPalOrderJSON(orderID, captureID, localOrderID, amount string) string {
	return `{"id":"` + orderID + `","status":"COMPLETED","purchase_units":[{"custom_id":"` + localOrderID + `","invoice_id":"` + localOrderID + `","amount":{"currency_code":"USD","value":"` + amount + `"},"payments":{"captures":[{"id":"` + captureID + `","status":"COMPLETED","amount":{"currency_code":"USD","value":"` + amount + `"},"create_time":"2026-07-20T08:00:00Z"}]}}]}`
}

func testPayPalWebhookHeaders() map[string]string {
	return map[string]string{
		"PAYPAL-AUTH-ALGO":         "SHA256withRSA",
		"PAYPAL-CERT-URL":          "https://api-m.sandbox.paypal.com/certs/test.pem",
		"PAYPAL-TRANSMISSION-ID":   "transmission-id",
		"PAYPAL-TRANSMISSION-SIG":  "signature",
		"PAYPAL-TRANSMISSION-TIME": "2026-07-20T08:00:00Z",
	}
}
