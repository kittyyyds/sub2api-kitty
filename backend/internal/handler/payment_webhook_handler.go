package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentWebhookHandler handles payment provider webhook callbacks.
type PaymentWebhookHandler struct {
	paymentService *service.PaymentService
	registry       *payment.Registry
}

// maxWebhookBodySize is the maximum allowed webhook request body size (1 MB).
const maxWebhookBodySize = 1 << 20

// webhookLogTruncateLen is the maximum length of raw body logged on verify failure.
const webhookLogTruncateLen = 200

// NewPaymentWebhookHandler creates a new PaymentWebhookHandler.
func NewPaymentWebhookHandler(paymentService *service.PaymentService, registry *payment.Registry) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentService: paymentService,
		registry:       registry,
	}
}

// EasyPayNotify handles EasyPay payment notifications.
// GET /api/v1/payment/webhook/easypay
// POST /api/v1/payment/webhook/easypay
func (h *PaymentWebhookHandler) EasyPayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeEasyPay)
}

// AlipayNotify handles Alipay payment notifications.
// POST /api/v1/payment/webhook/alipay
func (h *PaymentWebhookHandler) AlipayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeAlipay)
}

// WxpayNotify handles WeChat Pay payment notifications.
// POST /api/v1/payment/webhook/wxpay
func (h *PaymentWebhookHandler) WxpayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeWxpay)
}

// StripeWebhook handles Stripe webhook events.
// POST /api/v1/payment/webhook/stripe
func (h *PaymentWebhookHandler) StripeWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeStripe)
}

// AirwallexWebhook 处理空中云汇 Webhook 事件。
// POST /api/v1/payment/webhook/airwallex
func (h *PaymentWebhookHandler) AirwallexWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeAirwallex)
}

// DujiaoPayWebhook handles DujiaoPay stablecoin payment events.
// POST /api/v1/payment/webhook/dujiaopay
func (h *PaymentWebhookHandler) DujiaoPayWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeDujiaoPay)
}

// PayPalWebhook handles PayPal Checkout and Capture events.
// POST /api/v1/payment/webhook/paypal
func (h *PaymentWebhookHandler) PayPalWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypePayPal)
}

// handleNotify is the shared logic for all provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	rawBody, status, err := readWebhookPayload(c.Request, providerKey)
	if err != nil {
		if providerKey != payment.TypeEasyPay {
			slog.Error("[Payment Webhook] failed to read body", "provider", providerKey, "error", err)
			c.String(http.StatusBadRequest, "failed to read body")
			return
		}
		slog.Warn("[Payment Webhook] invalid request", "provider", providerKey, "error", err, "method", c.Request.Method)
		c.String(status, "invalid notification")
		return
	}

	// Extract out_trade_no to look up the order's specific provider instance.
	// This is needed when multiple instances of the same provider exist (e.g. multiple EasyPay accounts).
	outTradeNo := extractOutTradeNo(rawBody, providerKey)

	providers, err := h.paymentService.GetWebhookProviders(c.Request.Context(), providerKey, outTradeNo)
	if err != nil {
		slog.Warn("[Payment Webhook] provider not found", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
		if providerKey == payment.TypeWxpay {
			c.String(http.StatusBadRequest, "verify failed")
			return
		}
		writeSuccessResponse(c, providerKey)
		return
	}

	headers := make(map[string]string)
	for k := range c.Request.Header {
		headers[strings.ToLower(k)] = c.GetHeader(k)
	}

	verifiedProvider, notification, err := verifyNotificationWithProviders(c.Request.Context(), providers, rawBody, headers)
	if err != nil {
		slog.Error("[Payment Webhook] verify failed", "provider", providerKey, "error", err, "method", c.Request.Method, "bodyLen", len(rawBody))
		// EasyPay form values may contain a complete signature or a legacy return
		// URL with a resume token. Even a truncated body can expose those values.
		if providerKey != payment.TypeEasyPay {
			truncatedBody := rawBody
			if len(truncatedBody) > webhookLogTruncateLen {
				truncatedBody = truncatedBody[:webhookLogTruncateLen] + "...(truncated)"
			}
			slog.Debug("[Payment Webhook] verify failed body", "provider", providerKey, "rawBody", truncatedBody)
		}
		c.String(http.StatusBadRequest, "verify failed")
		return
	}

	resolvedProviderKey := verifiedProvider.ProviderKey()

	// nil notification means irrelevant event (e.g. Stripe non-payment event); return success.
	if notification == nil {
		writeSuccessResponse(c, resolvedProviderKey)
		return
	}

	if err := h.paymentService.ConfirmNotificationUpstream(c.Request.Context(), verifiedProvider, notification); err != nil {
		slog.Error("[Payment Webhook] upstream confirmation failed", "provider", resolvedProviderKey, "outTradeNo", notification.OrderID, "tradeNo", notification.TradeNo, "error", err)
		c.String(http.StatusBadGateway, "confirm failed")
		return
	}

	if err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, resolvedProviderKey); err != nil {
		// Unknown order: ack with 2xx so the provider stops retrying. This
		// guards against foreign environments whose webhook endpoints are
		// (mis)configured to point at us — without a 2xx, the provider will
		// retry for days and spam our error logs. We still emit a WARN so the
		// event is discoverable in logs.
		if errors.Is(err, service.ErrOrderNotFound) {
			slog.Warn("[Payment Webhook] unknown order, acking to stop retries",
				"provider", resolvedProviderKey,
				"outTradeNo", notification.OrderID,
				"tradeNo", notification.TradeNo,
			)
			writeSuccessResponse(c, resolvedProviderKey)
			return
		}
		slog.Error("[Payment Webhook] handle notification failed", "provider", resolvedProviderKey, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	writeSuccessResponse(c, resolvedProviderKey)
}

// EasyPay accepts exactly one parameter source: GET query or POST form body.
// Validate before using out_trade_no for instance lookup; never merge sources,
// collapse duplicate keys, or verify a truncated payload. Provider validation
// remains responsible for its field allowlist, value constraints and signature.
func readWebhookPayload(req *http.Request, providerKey string) (string, int, error) {
	if providerKey != payment.TypeEasyPay {
		if req.Method == http.MethodGet {
			return req.URL.RawQuery, 0, nil
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, maxWebhookBodySize))
		return string(body), http.StatusBadRequest, err
	}

	var raw string
	switch req.Method {
	case http.MethodGet:
		if len(req.URL.RawQuery) > maxWebhookBodySize {
			return "", http.StatusRequestEntityTooLarge, fmt.Errorf("notification query exceeds size limit")
		}
		if req.Body != nil {
			body, err := io.ReadAll(io.LimitReader(req.Body, 1))
			if err != nil || len(body) != 0 {
				return "", http.StatusBadRequest, fmt.Errorf("GET notification must not contain a body")
			}
		}
		raw = req.URL.RawQuery
	case http.MethodPost:
		if req.URL.RawQuery != "" {
			return "", http.StatusBadRequest, fmt.Errorf("POST notification must not contain query parameters")
		}
		mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/x-www-form-urlencoded" || len(req.Header.Values("Content-Type")) != 1 {
			return "", http.StatusUnsupportedMediaType, fmt.Errorf("POST notification requires a form-urlencoded body")
		}
		if req.ContentLength > maxWebhookBodySize {
			return "", http.StatusRequestEntityTooLarge, fmt.Errorf("notification body exceeds size limit")
		}
		if req.Body == nil {
			return "", http.StatusBadRequest, fmt.Errorf("notification body is empty")
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, maxWebhookBodySize+1))
		if err != nil {
			return "", http.StatusBadRequest, fmt.Errorf("failed to read notification body")
		}
		if len(body) > maxWebhookBodySize {
			return "", http.StatusRequestEntityTooLarge, fmt.Errorf("notification body exceeds size limit")
		}
		raw = string(body)
	default:
		return "", http.StatusMethodNotAllowed, fmt.Errorf("notification requires GET or POST")
	}

	values, err := url.ParseQuery(raw)
	if err != nil || len(values) == 0 {
		return "", http.StatusBadRequest, fmt.Errorf("invalid notification form encoding")
	}
	for key, entries := range values {
		if key == "" || len(entries) != 1 {
			return "", http.StatusBadRequest, fmt.Errorf("notification contains an empty or duplicate parameter")
		}
	}
	return raw, 0, nil
}

// extractOutTradeNo parses the webhook body to find the out_trade_no.
// This allows looking up the correct provider instance before verification.
func extractOutTradeNo(rawBody, providerKey string) string {
	switch providerKey {
	case payment.TypeEasyPay, payment.TypeAlipay:
		values, err := url.ParseQuery(rawBody)
		if err == nil {
			return values.Get("out_trade_no")
		}
	case payment.TypeAirwallex:
		var payload struct {
			Data struct {
				Object struct {
					MerchantOrderID string `json:"merchant_order_id"`
				} `json:"object"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil {
			return strings.TrimSpace(payload.Data.Object.MerchantOrderID)
		}
	case payment.TypeDujiaoPay:
		var payload struct {
			Data struct {
				MerchantOrderID string                     `json:"merchant_order_id"`
				Metadata        map[string]json.RawMessage `json:"metadata"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil {
			if orderID := strings.TrimSpace(payload.Data.MerchantOrderID); orderID != "" {
				return orderID
			}
			for _, key := range []string{"out_trade_no", "order_id"} {
				var orderID string
				if raw, ok := payload.Data.Metadata[key]; ok && json.Unmarshal(raw, &orderID) == nil {
					if orderID = strings.TrimSpace(orderID); orderID != "" {
						return orderID
					}
				}
			}
		}
	case payment.TypePayPal:
		var payload struct {
			Resource struct {
				CustomID      string `json:"custom_id"`
				InvoiceID     string `json:"invoice_id"`
				PurchaseUnits []struct {
					CustomID    string `json:"custom_id"`
					InvoiceID   string `json:"invoice_id"`
					ReferenceID string `json:"reference_id"`
				} `json:"purchase_units"`
			} `json:"resource"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil {
			for _, candidate := range []string{payload.Resource.CustomID, payload.Resource.InvoiceID} {
				if candidate = strings.TrimSpace(candidate); candidate != "" {
					return candidate
				}
			}
			for _, unit := range payload.Resource.PurchaseUnits {
				for _, candidate := range []string{unit.CustomID, unit.InvoiceID, unit.ReferenceID} {
					if candidate = strings.TrimSpace(candidate); candidate != "" {
						return candidate
					}
				}
			}
		}
	}
	// For other providers (Stripe, Alipay direct, WxPay direct), the registry
	// typically has only one instance, so no instance lookup is needed.
	return ""
}

// verifyNotificationWithProviders returns the provider instance that verified
// the notification so later steps (upstream confirmation) use the same
// merchant configuration.
func verifyNotificationWithProviders(ctx context.Context, providers []payment.Provider, rawBody string, headers map[string]string) (payment.Provider, *payment.PaymentNotification, error) {
	var lastErr error
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		notification, err := provider.VerifyNotification(ctx, rawBody, headers)
		if err != nil {
			lastErr = err
			continue
		}
		return provider, notification, nil
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("no webhook provider could verify notification")
}

// wxpaySuccessResponse is the JSON response expected by WeChat Pay webhook.
type wxpaySuccessResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WeChat Pay webhook success response constants.
const (
	wxpaySuccessCode    = "SUCCESS"
	wxpaySuccessMessage = "成功"
)

// writeSuccessResponse 返回各支付服务商要求的成功响应。
// 微信支付需要 JSON {"code":"SUCCESS","message":"成功"}；
// Stripe 和空中云汇接受空 200，其它服务商接受纯文本 "success"。
func writeSuccessResponse(c *gin.Context, providerKey string) {
	switch providerKey {
	case payment.TypeWxpay:
		c.JSON(http.StatusOK, wxpaySuccessResponse{Code: wxpaySuccessCode, Message: wxpaySuccessMessage})
	case payment.TypeStripe, payment.TypeAirwallex, payment.TypeDujiaoPay, payment.TypePayPal:
		c.String(http.StatusOK, "")
	default:
		c.String(http.StatusOK, "success")
	}
}
