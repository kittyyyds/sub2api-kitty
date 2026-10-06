//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadWebhookPayloadEasyPayAcceptsGETAndPOST(t *testing.T) {
	t.Parallel()
	const payload = "pid=1&trade_no=trade42&out_trade_no=sub2_42&type=alipay&name=%E5%85%85%E5%80%BC+credit&money=12.50&trade_status=TRADE_SUCCESS&sign=test-signature&sign_type=MD5&param=checkout%26source%3Dweb"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(method, "/notify", nil)
			if method == http.MethodGet {
				req.URL.RawQuery = payload
			} else {
				req.Body = io.NopCloser(strings.NewReader(payload))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
			}
			got, _, err := readWebhookPayload(req, payment.TypeEasyPay)
			require.NoError(t, err)
			require.Equal(t, payload, got, "signature verification must receive the original encoding")
			require.Equal(t, "sub2_42", extractOutTradeNo(got, payment.TypeEasyPay))
		})
	}
}

func TestEasyPayNotifyRejectsInvalidRequestsBeforeProviderLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const formType = "application/x-www-form-urlencoded"
	const payload = "out_trade_no=sub2_42&trade_no=trade42&sign=test-signature"
	tests := []struct {
		name        string
		method      string
		query       string
		body        string
		contentType string
		status      int
	}{
		{"GET identical duplicates", http.MethodGet, payload + "&out_trade_no=sub2_42", "", "", http.StatusBadRequest},
		{"GET conflicting duplicates", http.MethodGet, payload + "&out_trade_no=foreign", "", "", http.StatusBadRequest},
		{"GET encoded duplicate key", http.MethodGet, payload + "&%6fut_trade_no=foreign", "", "", http.StatusBadRequest},
		{"GET duplicate signature", http.MethodGet, payload + "&sign=test-signature", "", "", http.StatusBadRequest},
		{"GET duplicate empty values", http.MethodGet, payload + "&param=&param=", "", "", http.StatusBadRequest},
		{"GET invalid escaping", http.MethodGet, payload + "&param=%zz", "", "", http.StatusBadRequest},
		{"GET invalid separator", http.MethodGet, payload + ";param=bad", "", "", http.StatusBadRequest},
		{"GET empty key", http.MethodGet, payload + "&=bad", "", "", http.StatusBadRequest},
		{"GET no parameters", http.MethodGet, "", "", "", http.StatusBadRequest},
		{"GET repeated body parameters", http.MethodGet, payload, payload, formType, http.StatusBadRequest},
		{"GET conflicting body", http.MethodGet, payload, "out_trade_no=foreign", formType, http.StatusBadRequest},
		{"GET disjoint body", http.MethodGet, payload, "param=extra", formType, http.StatusBadRequest},
		{"POST identical duplicates", http.MethodPost, "", payload + "&trade_no=trade42", formType, http.StatusBadRequest},
		{"POST conflicting duplicates", http.MethodPost, "", payload + "&trade_no=other", formType, http.StatusBadRequest},
		{"POST encoded duplicate key", http.MethodPost, "", payload + "&%73ign=other", formType, http.StatusBadRequest},
		{"POST repeated query parameters", http.MethodPost, payload, payload, formType, http.StatusBadRequest},
		{"POST conflicting query", http.MethodPost, "out_trade_no=foreign", payload, formType, http.StatusBadRequest},
		{"POST disjoint query", http.MethodPost, "param=extra", payload, formType, http.StatusBadRequest},
		{"POST query only", http.MethodPost, payload, "", formType, http.StatusBadRequest},
		{"POST malformed query", http.MethodPost, "bad=%zz", payload, formType, http.StatusBadRequest},
		{"POST malformed body", http.MethodPost, "", payload + "&param=%zz", formType, http.StatusBadRequest},
		{"POST empty body", http.MethodPost, "", "", formType, http.StatusBadRequest},
		{"POST JSON body", http.MethodPost, "", payload, "application/json", http.StatusUnsupportedMediaType},
		{"POST multipart body", http.MethodPost, "", payload, "multipart/form-data; boundary=test", http.StatusUnsupportedMediaType},
		{"POST missing content type", http.MethodPost, "", payload, "", http.StatusUnsupportedMediaType},
		{"POST invalid content type", http.MethodPost, "", payload, formType + ";broken", http.StatusUnsupportedMediaType},
		{"unsupported method", http.MethodPut, "", payload, formType, http.StatusMethodNotAllowed},
		{"HEAD is not GET", http.MethodHead, payload, "", "", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/notify", strings.NewReader(tt.body))
			req.URL.RawQuery = tt.query
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			// A nil service guarantees rejected input cannot reach provider lookup.
			NewPaymentWebhookHandler(nil, nil).EasyPayNotify(c)
			require.Equal(t, tt.status, w.Code)
			require.Equal(t, "invalid notification", w.Body.String())
		})
	}
}

func TestReadWebhookPayloadEasyPaySizeLimits(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, size := range []int{maxWebhookBodySize, maxWebhookBodySize + 1} {
			t.Run(fmt.Sprintf("%s/%d", method, size), func(t *testing.T) {
				payload := "param=" + strings.Repeat("a", size-len("param="))
				req := httptest.NewRequest(method, "/notify", nil)
				if method == http.MethodGet {
					req.URL.RawQuery = payload
				} else {
					req.Body = io.NopCloser(strings.NewReader(payload))
					req.ContentLength = -1 // Exercise the streaming limit without a size header.
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				got, status, err := readWebhookPayload(req, payment.TypeEasyPay)
				if size > maxWebhookBodySize {
					require.Error(t, err)
					require.Equal(t, http.StatusRequestEntityTooLarge, status)
					require.Empty(t, got, "never forward a truncated payload")
					return
				}
				require.NoError(t, err)
				require.Equal(t, payload, got)
			})
		}
	}

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method+" bounded streaming read", func(t *testing.T) {
			reader := &webhookCountingReader{reader: strings.NewReader(strings.Repeat("x", 3*maxWebhookBodySize))}
			req := httptest.NewRequest(method, "/notify?out_trade_no=sub2_42", nil)
			req.ContentLength = -1
			req.Body = io.NopCloser(reader)
			limit := 1
			if method == http.MethodPost {
				req.URL.RawQuery = ""
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				limit = maxWebhookBodySize + 1
			}
			got, _, err := readWebhookPayload(req, payment.TypeEasyPay)
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, limit, reader.bytesRead)
		})
	}
}

func TestReadWebhookPayloadEasyPayRejectsBodyReadError(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/notify", nil)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Body = io.NopCloser(webhookErrorReader{})
			got, status, err := readWebhookPayload(req, payment.TypeEasyPay)
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, http.StatusBadRequest, status)
		})
	}
}

func TestReadWebhookPayloadEasyPayRejectsAmbiguousContentType(t *testing.T) {
	t.Parallel()
	for _, extraType := range []string{"application/x-www-form-urlencoded", "application/json"} {
		req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader("out_trade_no=sub2_42"))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Type", extraType)
		got, status, err := readWebhookPayload(req, payment.TypeEasyPay)
		require.Error(t, err)
		require.Empty(t, got)
		require.Equal(t, http.StatusUnsupportedMediaType, status)
	}
}

func TestReadWebhookPayloadPreservesOtherProviders(t *testing.T) {
	t.Parallel()
	for _, providerKey := range []string{payment.TypeAlipay, payment.TypeWxpay, payment.TypeStripe, payment.TypeAirwallex, payment.TypeDujiaoPay, payment.TypePayPal} {
		t.Run(providerKey, func(t *testing.T) {
			t.Parallel()
			const query = "out_trade_no=first&out_trade_no=second"
			const body = `{"event":"payment","data":{"id":"test"}}`
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				req := httptest.NewRequest(method, "/notify?"+query, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				got, _, err := readWebhookPayload(req, providerKey)
				require.NoError(t, err)
				want := body
				if method == http.MethodGet {
					want = query
				}
				require.Equal(t, want, got)
			}
		})
	}
}

type webhookCountingReader struct {
	reader    io.Reader
	bytesRead int
}

func (r *webhookCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}

type webhookErrorReader struct{}

func (webhookErrorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestWriteSuccessResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		providerKey     string
		wantCode        int
		wantContentType string
		wantBody        string
		checkJSON       bool
		wantJSONCode    string
		wantJSONMessage string
	}{
		{
			name:            "wxpay returns JSON with code SUCCESS",
			providerKey:     "wxpay",
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			checkJSON:       true,
			wantJSONCode:    "SUCCESS",
			wantJSONMessage: "成功",
		},
		{
			name:            "stripe returns empty 200",
			providerKey:     "stripe",
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "",
		},
		{
			name:            "airwallex returns empty 200",
			providerKey:     payment.TypeAirwallex,
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "",
		},
		{
			name:            "dujiaopay returns empty 200",
			providerKey:     payment.TypeDujiaoPay,
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "",
		},
		{
			name:            "paypal returns empty 200",
			providerKey:     payment.TypePayPal,
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "",
		},
		{
			name:            "easypay returns plain text success",
			providerKey:     "easypay",
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "success",
		},
		{
			name:            "alipay returns plain text success",
			providerKey:     "alipay",
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "success",
		},
		{
			name:            "unknown provider returns plain text success",
			providerKey:     "unknown_provider",
			wantCode:        http.StatusOK,
			wantContentType: "text/plain",
			wantBody:        "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			writeSuccessResponse(c, tt.providerKey)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), tt.wantContentType)

			if tt.checkJSON {
				var resp wxpaySuccessResponse
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err, "response body should be valid JSON")
				assert.Equal(t, tt.wantJSONCode, resp.Code)
				assert.Equal(t, tt.wantJSONMessage, resp.Message)
			} else {
				assert.Equal(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

// TestUnknownOrderWebhookAcksWithSuccess exercises the response contract that
// handleNotify relies on when HandlePaymentNotification returns ErrOrderNotFound:
// we still need to emit the provider-specific 2xx so the provider stops
// retrying. We can't easily drive handleNotify end-to-end without mocking the
// concrete *service.PaymentService, so this test locks down the two ingredients
// the fix depends on:
//  1. errors.Is recognises the sentinel through fmt.Errorf %w wrapping (which
//     is how service layer wraps it with the out_trade_no context).
//  2. writeSuccessResponse produces the provider-specific body for Stripe
//     (empty 200) — matching what handleNotify calls on the ack path.
//
// If either contract breaks, the Stripe "unknown order → 500 loop" regresses.
func TestUnknownOrderWebhookAcksWithSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1) Sentinel recognition through wrapping.
	wrapped := fmt.Errorf("%w: out_trade_no=sub2_missing_42", service.ErrOrderNotFound)
	require.True(t, errors.Is(wrapped, service.ErrOrderNotFound),
		"handleNotify uses errors.Is on the wrapped service error; regression here "+
			"would mean unknown-order webhooks go back to returning 500 and looping forever")

	// A distinct error must NOT match — otherwise a DB failure would be silently
	// swallowed as an ack.
	other := errors.New("lookup order failed: connection refused")
	require.False(t, errors.Is(other, service.ErrOrderNotFound))

	// 2) Provider-specific success body is what handleNotify emits on the
	// ack path. Asserted again here because this is the shape Stripe expects
	// to consider the webhook acknowledged.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeSuccessResponse(c, payment.TypeStripe)
	require.Equal(t, http.StatusOK, w.Code,
		"Stripe requires 2xx to stop retrying; anything else restarts the retry loop")
	require.Empty(t, w.Body.String(), "Stripe expects an empty body on the ack path")
}

func TestWebhookConstants(t *testing.T) {
	t.Run("maxWebhookBodySize is 1MB", func(t *testing.T) {
		assert.Equal(t, int64(1<<20), int64(maxWebhookBodySize))
	})

	t.Run("webhookLogTruncateLen is 200", func(t *testing.T) {
		assert.Equal(t, 200, webhookLogTruncateLen)
	})
}

func TestExtractOutTradeNo(t *testing.T) {
	tests := []struct {
		name        string
		providerKey string
		rawBody     string
		want        string
	}{
		{
			name:        "easypay query payload",
			providerKey: "easypay",
			rawBody:     "out_trade_no=sub2_123&trade_status=TRADE_SUCCESS",
			want:        "sub2_123",
		},
		{
			name:        "alipay query payload",
			providerKey: "alipay",
			rawBody:     "notify_time=2026-04-20+12%3A00%3A00&out_trade_no=sub2_456",
			want:        "sub2_456",
		},
		{
			name:        "unknown provider",
			providerKey: "wxpay",
			rawBody:     "{}",
			want:        "",
		},
		{
			name:        "airwallex payment intent payload",
			providerKey: payment.TypeAirwallex,
			rawBody:     `{"name":"payment_intent.succeeded","data":{"object":{"merchant_order_id":"sub2_awx_123"}}}`,
			want:        "sub2_awx_123",
		},
		{
			name:        "dujiaopay metadata payload",
			providerKey: payment.TypeDujiaoPay,
			rawBody:     `{"event_type":"order.paid","data":{"order_id":"do_123","metadata":{"out_trade_no":"sub2_djp_123"}}}`,
			want:        "sub2_djp_123",
		},
		{
			name:        "dujiaopay payload without merchant order id",
			providerKey: payment.TypeDujiaoPay,
			rawBody:     `{"event_type":"order.paid","data":{"order_id":"do_456"}}`,
			want:        "",
		},
		{
			name:        "paypal approved order purchase unit",
			providerKey: payment.TypePayPal,
			rawBody:     `{"event_type":"CHECKOUT.ORDER.APPROVED","resource":{"id":"ORDER-1","purchase_units":[{"custom_id":"sub2_paypal_1","invoice_id":"sub2_paypal_1"}]}}`,
			want:        "sub2_paypal_1",
		},
		{
			name:        "paypal completed capture resource",
			providerKey: payment.TypePayPal,
			rawBody:     `{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE-1","custom_id":"sub2_paypal_2"}}`,
			want:        "sub2_paypal_2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractOutTradeNo(tt.rawBody, tt.providerKey))
		})
	}
}

func TestVerifyNotificationWithProvidersReturnsMatchedProvider(t *testing.T) {
	firstErr := errors.New("wrong provider")
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: firstErr,
		},
		webhookHandlerProviderStub{
			key: payment.TypeWxpay,
			notification: &payment.PaymentNotification{
				OrderID: "sub2_42",
				TradeNo: "trade-42",
				Status:  payment.NotificationStatusSuccess,
			},
		},
	}

	providerKey, notification, err := verifyNotificationWithProviders(context.Background(), providers, "{}", map[string]string{"wechatpay-signature": "sig"})
	require.NoError(t, err)
	require.Equal(t, payment.TypeWxpay, providerKey)
	require.NotNil(t, notification)
	require.Equal(t, "sub2_42", notification.OrderID)
}

func TestVerifyNotificationWithProvidersFailsWhenAllProvidersReject(t *testing.T) {
	providers := []payment.Provider{
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: errors.New("verify failed a"),
		},
		webhookHandlerProviderStub{
			key:       payment.TypeWxpay,
			verifyErr: errors.New("verify failed b"),
		},
	}

	_, _, err := verifyNotificationWithProviders(context.Background(), providers, "{}", nil)
	require.Error(t, err)
}

type webhookHandlerProviderStub struct {
	key          string
	notification *payment.PaymentNotification
	verifyErr    error
}

func (p webhookHandlerProviderStub) Name() string        { return p.key }
func (p webhookHandlerProviderStub) ProviderKey() string { return p.key }
func (p webhookHandlerProviderStub) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.PaymentType(p.key)}
}
func (p webhookHandlerProviderStub) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p webhookHandlerProviderStub) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	if p.verifyErr != nil {
		return nil, p.verifyErr
	}
	return p.notification, nil
}
func (p webhookHandlerProviderStub) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
}
