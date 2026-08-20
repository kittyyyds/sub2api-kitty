package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	payPalSandboxAPIBase  = "https://api-m.sandbox.paypal.com"
	payPalLiveAPIBase     = "https://api-m.paypal.com"
	payPalDefaultCurrency = "USD"

	payPalEnvironmentSandbox = "sandbox"
	payPalEnvironmentLive    = "live"

	payPalHTTPTimeout      = 20 * time.Second
	payPalMaxResponseSize  = 1 << 20
	payPalMaxErrorSummary  = 512
	payPalTokenExpirySkew  = time.Minute
	payPalOrderDescription = 127
	payPalBrandNameLimit   = 127
	payPalRefundNoteLimit  = 255

	payPalEventOrderApproved    = "CHECKOUT.ORDER.APPROVED"
	payPalEventCaptureCompleted = "PAYMENT.CAPTURE.COMPLETED"
	payPalEventCaptureDenied    = "PAYMENT.CAPTURE.DENIED"

	payPalOrderStatusApproved  = "APPROVED"
	payPalOrderStatusCompleted = "COMPLETED"
	payPalOrderStatusVoided    = "VOIDED"

	payPalVerificationSuccess = "SUCCESS"
)

type PayPal struct {
	instanceID string
	config     map[string]string
	apiBase    string
	httpClient *http.Client
}

type payPalTokenState struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

var payPalAccessTokens sync.Map

func NewPayPal(instanceID string, config map[string]string) (*PayPal, error) {
	for _, key := range []string{"clientId", "clientSecret", "webhookId"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("paypal config missing required key: %s", key)
		}
	}

	cfg := cloneStringMap(config)
	environment := strings.ToLower(strings.TrimSpace(cfg["environment"]))
	if environment == "" {
		environment = payPalEnvironmentSandbox
	}
	apiBase := ""
	switch environment {
	case payPalEnvironmentSandbox:
		apiBase = payPalSandboxAPIBase
	case payPalEnvironmentLive:
		apiBase = payPalLiveAPIBase
	default:
		return nil, fmt.Errorf("paypal config environment must be sandbox or live")
	}
	cfg["environment"] = environment

	currencyRaw := strings.TrimSpace(cfg["currency"])
	if currencyRaw == "" {
		currencyRaw = payPalDefaultCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(currencyRaw)
	if err != nil {
		return nil, fmt.Errorf("paypal config currency: %w", err)
	}
	cfg["currency"] = currency

	if utf8.RuneCountInString(strings.TrimSpace(cfg["brandName"])) > payPalBrandNameLimit {
		return nil, fmt.Errorf("paypal config brandName must not exceed %d characters", payPalBrandNameLimit)
	}

	return &PayPal{
		instanceID: strings.TrimSpace(instanceID),
		config:     cfg,
		apiBase:    apiBase,
		httpClient: &http.Client{Timeout: payPalHTTPTimeout},
	}, nil
}

func (p *PayPal) Name() string        { return "PayPal" }
func (p *PayPal) ProviderKey() string { return payment.TypePayPal }
func (p *PayPal) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypePayPal}
}

func (p *PayPal) MerchantIdentityMetadata() map[string]string {
	if p == nil {
		return nil
	}
	return map[string]string{
		"client_id":   strings.TrimSpace(p.config["clientId"]),
		"currency":    p.currency(),
		"environment": p.environment(),
	}
}

func (p *PayPal) currency() string {
	if p == nil {
		return payPalDefaultCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(p.config["currency"])
	if err != nil || strings.TrimSpace(p.config["currency"]) == "" {
		return payPalDefaultCurrency
	}
	return currency
}

func (p *PayPal) environment() string {
	if p == nil || strings.TrimSpace(p.config["environment"]) == "" {
		return payPalEnvironmentSandbox
	}
	return strings.ToLower(strings.TrimSpace(p.config["environment"]))
}

func (p *PayPal) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return nil, fmt.Errorf("paypal create payment: missing order id")
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		return nil, fmt.Errorf("paypal create payment: missing return URL")
	}
	amount, err := normalizePayPalAmount(req.Amount, p.currency())
	if err != nil {
		return nil, fmt.Errorf("paypal create payment: %w", err)
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal auth: %w", err)
	}

	payload := payPalCreateOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []payPalPurchaseUnitRequest{{
			ReferenceID: orderID,
			CustomID:    orderID,
			InvoiceID:   orderID,
			Description: truncatePayPalText(strings.TrimSpace(req.Subject), payPalOrderDescription),
			Amount: payPalMoney{
				CurrencyCode: p.currency(),
				Value:        amount,
			},
		}},
		ApplicationContext: payPalApplicationContext{
			BrandName:   strings.TrimSpace(p.config["brandName"]),
			LandingPage: "NO_PREFERENCE",
			UserAction:  "PAY_NOW",
			ReturnURL:   returnURL,
			CancelURL:   returnURL,
		},
	}

	var order payPalOrder
	requestID := payPalRequestID("create", orderID, amount, p.currency())
	if err := p.doJSON(ctx, http.MethodPost, "/v2/checkout/orders", token, payload, &order, requestID); err != nil {
		return nil, fmt.Errorf("paypal create payment: %w", err)
	}
	if strings.TrimSpace(order.ID) == "" {
		return nil, fmt.Errorf("paypal create payment: response missing order id")
	}
	approvalURL := payPalApprovalURL(order.Links)
	if approvalURL == "" {
		return nil, fmt.Errorf("paypal create payment: response missing approval URL")
	}

	return &payment.CreatePaymentResponse{
		TradeNo:  order.ID,
		PayURL:   approvalURL,
		Currency: p.currency(),
	}, nil
}

func (p *PayPal) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("paypal query order: missing trade number")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal auth: %w", err)
	}

	order, err := p.getOrder(ctx, token, tradeNo)
	if err != nil {
		var httpErr *payPalHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			capture, captureErr := p.getCapture(ctx, token, tradeNo)
			if captureErr != nil {
				return nil, fmt.Errorf("paypal query capture: %w", captureErr)
			}
			return p.queryResponseFromCapture(capture)
		}
		return nil, fmt.Errorf("paypal query order: %w", err)
	}

	if strings.EqualFold(order.Status, payPalOrderStatusApproved) {
		order, err = p.captureOrder(ctx, token, order.ID)
		if err != nil {
			return nil, fmt.Errorf("paypal capture order: %w", err)
		}
	}
	return p.queryResponseFromOrder(order)
}

func (p *PayPal) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	var event payPalWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("paypal parse webhook: %w", err)
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.EventType) == "" || len(event.Resource) == 0 {
		return nil, fmt.Errorf("paypal webhook is missing required event fields")
	}

	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal webhook auth: %w", err)
	}
	if err := p.verifyWebhookSignature(ctx, token, json.RawMessage(rawBody), headers); err != nil {
		return nil, err
	}

	switch strings.ToUpper(strings.TrimSpace(event.EventType)) {
	case payPalEventOrderApproved:
		var approved payPalOrder
		if err := json.Unmarshal(event.Resource, &approved); err != nil {
			return nil, fmt.Errorf("paypal parse approved order webhook: %w", err)
		}
		if strings.TrimSpace(approved.ID) == "" {
			return nil, fmt.Errorf("paypal approved order webhook missing order id")
		}
		captured, err := p.captureOrder(ctx, token, approved.ID)
		if err != nil {
			return nil, fmt.Errorf("paypal capture approved webhook order: %w", err)
		}
		resp, err := p.queryResponseFromOrder(captured)
		if err != nil {
			return nil, err
		}
		orderID := payPalOrderLocalID(captured)
		if orderID == "" {
			return nil, fmt.Errorf("paypal approved order webhook missing merchant order id")
		}
		resp.Metadata["event_id"] = event.ID
		return &payment.PaymentNotification{
			TradeNo:  resp.TradeNo,
			OrderID:  orderID,
			Amount:   resp.Amount,
			Status:   payment.NotificationStatusSuccess,
			RawData:  rawBody,
			Metadata: resp.Metadata,
		}, nil

	case payPalEventCaptureCompleted:
		var capture payPalCapture
		if err := json.Unmarshal(event.Resource, &capture); err != nil {
			return nil, fmt.Errorf("paypal parse capture webhook: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(capture.Status), payPalOrderStatusCompleted) {
			return nil, fmt.Errorf("paypal completed capture webhook has status %s", capture.Status)
		}
		orderID := payPalCaptureLocalID(capture)
		payPalOrderID := strings.TrimSpace(capture.SupplementaryData.RelatedIDs.OrderID)
		if orderID == "" && payPalOrderID != "" {
			order, queryErr := p.getOrder(ctx, token, payPalOrderID)
			if queryErr != nil {
				return nil, fmt.Errorf("paypal resolve capture merchant order: %w", queryErr)
			}
			orderID = payPalOrderLocalID(order)
		}
		if orderID == "" {
			return nil, fmt.Errorf("paypal capture webhook missing merchant order id")
		}
		resp, err := p.queryResponseFromCapture(capture)
		if err != nil {
			return nil, err
		}
		resp.Metadata["event_id"] = event.ID
		if payPalOrderID != "" {
			resp.Metadata["paypal_order_id"] = payPalOrderID
		}
		return &payment.PaymentNotification{
			TradeNo:  resp.TradeNo,
			OrderID:  orderID,
			Amount:   resp.Amount,
			Status:   payment.NotificationStatusSuccess,
			RawData:  rawBody,
			Metadata: resp.Metadata,
		}, nil

	case payPalEventCaptureDenied:
		return nil, nil
	default:
		return nil, nil
	}
}

func (p *PayPal) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	captureID := strings.TrimSpace(req.TradeNo)
	if captureID == "" {
		return nil, fmt.Errorf("paypal refund: missing capture id")
	}
	amount, err := normalizePayPalAmount(req.Amount, p.currency())
	if err != nil {
		return nil, fmt.Errorf("paypal refund: %w", err)
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal auth: %w", err)
	}
	payload := payPalRefundRequest{
		Amount:      payPalMoney{CurrencyCode: p.currency(), Value: amount},
		NoteToPayer: truncatePayPalText(strings.TrimSpace(req.Reason), payPalRefundNoteLimit),
	}
	var refund payPalRefund
	path := "/v2/payments/captures/" + url.PathEscape(captureID) + "/refund"
	requestID := payPalRequestID("refund", req.OrderID, captureID, amount, p.currency())
	if err := p.doJSON(ctx, http.MethodPost, path, token, payload, &refund, requestID); err != nil {
		return nil, fmt.Errorf("paypal refund: %w", err)
	}
	return payPalRefundResponse(refund)
}

func (p *PayPal) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	refundID := strings.TrimSpace(req.RefundID)
	if refundID == "" {
		return nil, fmt.Errorf("paypal query refund: missing refund id")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal auth: %w", err)
	}
	var refund payPalRefund
	if err := p.doJSON(ctx, http.MethodGet, "/v2/payments/refunds/"+url.PathEscape(refundID), token, nil, &refund, ""); err != nil {
		return nil, fmt.Errorf("paypal query refund: %w", err)
	}
	return payPalRefundResponse(refund)
}

func (p *PayPal) getOrder(ctx context.Context, token, orderID string) (payPalOrder, error) {
	var order payPalOrder
	err := p.doJSON(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(strings.TrimSpace(orderID)), token, nil, &order, "")
	return order, err
}

func (p *PayPal) getCapture(ctx context.Context, token, captureID string) (payPalCapture, error) {
	var capture payPalCapture
	err := p.doJSON(ctx, http.MethodGet, "/v2/payments/captures/"+url.PathEscape(strings.TrimSpace(captureID)), token, nil, &capture, "")
	return capture, err
}

func (p *PayPal) captureOrder(ctx context.Context, token, orderID string) (payPalOrder, error) {
	orderID = strings.TrimSpace(orderID)
	var order payPalOrder
	path := "/v2/checkout/orders/" + url.PathEscape(orderID) + "/capture"
	err := p.doJSON(ctx, http.MethodPost, path, token, map[string]any{}, &order, payPalRequestID("capture", orderID))
	if err == nil {
		return order, nil
	}

	var httpErr *payPalHTTPError
	if !errors.As(err, &httpErr) || (httpErr.StatusCode != http.StatusConflict && httpErr.StatusCode != http.StatusUnprocessableEntity) {
		return payPalOrder{}, err
	}
	current, queryErr := p.getOrder(ctx, token, orderID)
	if queryErr == nil && strings.EqualFold(strings.TrimSpace(current.Status), payPalOrderStatusCompleted) {
		return current, nil
	}
	return payPalOrder{}, err
}

func (p *PayPal) queryResponseFromOrder(order payPalOrder) (*payment.QueryOrderResponse, error) {
	status := payPalProviderStatus(order.Status)
	if status == payment.ProviderStatusPaid {
		capture, ok := payPalCompletedCapture(order)
		if !ok {
			return nil, fmt.Errorf("paypal completed order is missing a completed capture")
		}
		resp, err := p.queryResponseFromCapture(capture)
		if err != nil {
			return nil, err
		}
		resp.Metadata["paypal_order_id"] = strings.TrimSpace(order.ID)
		return resp, nil
	}

	amount := 0.0
	currency := p.currency()
	if money, ok := payPalOrderMoney(order); ok {
		parsed, err := parsePayPalMoney(money, false)
		if err != nil {
			return nil, fmt.Errorf("paypal query order: %w", err)
		}
		amount = parsed
		if strings.TrimSpace(money.CurrencyCode) != "" {
			currency = strings.ToUpper(strings.TrimSpace(money.CurrencyCode))
		}
	}
	return &payment.QueryOrderResponse{
		TradeNo:  strings.TrimSpace(order.ID),
		Status:   status,
		Amount:   amount,
		Metadata: p.paymentMetadata(currency, order.Status),
	}, nil
}

func (p *PayPal) queryResponseFromCapture(capture payPalCapture) (*payment.QueryOrderResponse, error) {
	if strings.TrimSpace(capture.ID) == "" {
		return nil, fmt.Errorf("paypal capture response missing capture id")
	}
	status := payPalProviderStatus(capture.Status)
	amount, err := parsePayPalMoney(capture.Amount, status == payment.ProviderStatusPaid)
	if err != nil {
		return nil, fmt.Errorf("paypal capture amount: %w", err)
	}
	currency := strings.ToUpper(strings.TrimSpace(capture.Amount.CurrencyCode))
	if currency == "" {
		currency = p.currency()
	}
	return &payment.QueryOrderResponse{
		TradeNo:  strings.TrimSpace(capture.ID),
		Status:   status,
		Amount:   amount,
		PaidAt:   strings.TrimSpace(capture.CreateTime),
		Metadata: p.paymentMetadata(currency, capture.Status),
	}, nil
}

func (p *PayPal) paymentMetadata(currency, status string) map[string]string {
	return map[string]string{
		"client_id":   strings.TrimSpace(p.config["clientId"]),
		"currency":    strings.ToUpper(strings.TrimSpace(currency)),
		"environment": p.environment(),
		"status":      strings.ToUpper(strings.TrimSpace(status)),
	}
}

func (p *PayPal) verifyWebhookSignature(ctx context.Context, token string, rawEvent json.RawMessage, headers map[string]string) error {
	payload := payPalVerifyWebhookRequest{
		AuthAlgo:         payPalHeader(headers, "paypal-auth-algo"),
		CertURL:          payPalHeader(headers, "paypal-cert-url"),
		TransmissionID:   payPalHeader(headers, "paypal-transmission-id"),
		TransmissionSig:  payPalHeader(headers, "paypal-transmission-sig"),
		TransmissionTime: payPalHeader(headers, "paypal-transmission-time"),
		WebhookID:        strings.TrimSpace(p.config["webhookId"]),
		WebhookEvent:     rawEvent,
	}
	if payload.AuthAlgo == "" || payload.CertURL == "" || payload.TransmissionID == "" || payload.TransmissionSig == "" || payload.TransmissionTime == "" {
		return fmt.Errorf("paypal webhook signature headers are incomplete")
	}
	var response payPalVerifyWebhookResponse
	if err := p.doJSON(ctx, http.MethodPost, "/v1/notifications/verify-webhook-signature", token, payload, &response, ""); err != nil {
		return fmt.Errorf("paypal verify webhook signature: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(response.VerificationStatus), payPalVerificationSuccess) {
		return fmt.Errorf("paypal webhook signature verification failed")
	}
	return nil
}

func (p *PayPal) accessToken(ctx context.Context) (string, error) {
	stateAny, _ := payPalAccessTokens.LoadOrStore(p.tokenCacheKey(), &payPalTokenState{})
	state := stateAny.(*payPalTokenState)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.token != "" && time.Now().Before(state.expiresAt) {
		return state.token, nil
	}
	token, expiresIn, err := p.fetchAccessToken(ctx)
	if err != nil {
		return "", err
	}
	ttl := time.Duration(expiresIn) * time.Second
	if ttl <= payPalTokenExpirySkew {
		ttl /= 2
	} else {
		ttl -= payPalTokenExpirySkew
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	state.token = token
	state.expiresAt = time.Now().Add(ttl)
	return token, nil
}

func (p *PayPal) fetchAccessToken(ctx context.Context) (string, int, error) {
	target := strings.TrimRight(p.apiBase, "/") + "/v1/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", 0, err
	}
	req.SetBasicAuth(strings.TrimSpace(p.config["clientId"]), strings.TrimSpace(p.config["clientSecret"]))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en_US")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	body, status, err := p.do(req)
	if err != nil {
		return "", 0, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", 0, parsePayPalHTTPError(status, body)
	}
	var response payPalAccessTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", 0, fmt.Errorf("token response missing access_token")
	}
	return response.AccessToken, response.ExpiresIn, nil
}

func (p *PayPal) tokenCacheKey() string {
	return strings.Join([]string{
		strings.TrimSpace(p.apiBase),
		strings.TrimSpace(p.instanceID),
		strings.TrimSpace(p.config["clientId"]),
		p.environment(),
	}, "|")
}

func (p *PayPal) doJSON(ctx context.Context, method, path, token string, payload, out any, requestID string) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.apiBase, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(requestID) != "" {
		req.Header.Set("PayPal-Request-Id", strings.TrimSpace(requestID))
	}
	if method == http.MethodPost {
		req.Header.Set("Prefer", "return=representation")
	}

	responseBody, status, err := p.do(req)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return parsePayPalHTTPError(status, responseBody)
	}
	if out == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func (p *PayPal) do(req *http.Request) ([]byte, int, error) {
	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: payPalHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, payPalMaxResponseSize))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func normalizePayPalAmount(raw, currency string) (string, error) {
	if _, err := payment.AmountToMinorUnit(raw, currency); err != nil {
		return "", err
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return "", fmt.Errorf("invalid amount: %s", raw)
	}
	return amount.StringFixed(int32(payment.CurrencyMaxFractionDigits(currency))), nil
}

func parsePayPalMoney(money payPalMoney, required bool) (float64, error) {
	raw := strings.TrimSpace(money.Value)
	if raw == "" {
		if required {
			return 0, fmt.Errorf("missing amount")
		}
		return 0, nil
	}
	amount, err := decimal.NewFromString(raw)
	if err != nil || amount.IsNegative() || (required && amount.IsZero()) {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	if required && strings.TrimSpace(money.CurrencyCode) == "" {
		return 0, fmt.Errorf("missing currency")
	}
	return amount.InexactFloat64(), nil
}

func payPalProviderStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case payPalOrderStatusCompleted:
		return payment.ProviderStatusPaid
	case payPalOrderStatusVoided, "DENIED", "DECLINED", "FAILED", "CANCELLED":
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func payPalRefundResponse(refund payPalRefund) (*payment.RefundResponse, error) {
	if strings.TrimSpace(refund.ID) == "" {
		return nil, fmt.Errorf("paypal refund response missing refund id")
	}
	status := payment.ProviderStatusPending
	switch strings.ToUpper(strings.TrimSpace(refund.Status)) {
	case "COMPLETED":
		status = payment.ProviderStatusSuccess
	case "PENDING":
		status = payment.ProviderStatusPending
	case "FAILED", "CANCELLED":
		status = payment.ProviderStatusFailed
	default:
		return nil, fmt.Errorf("paypal refund returned unknown status: %s", refund.Status)
	}
	response := &payment.RefundResponse{RefundID: strings.TrimSpace(refund.ID), Status: status}
	if status == payment.ProviderStatusFailed {
		return response, fmt.Errorf("paypal refund failed: status %s", refund.Status)
	}
	return response, nil
}

func payPalApprovalURL(links []payPalLink) string {
	for _, rel := range []string{"approve", "payer-action"} {
		for _, link := range links {
			if strings.EqualFold(strings.TrimSpace(link.Rel), rel) && strings.TrimSpace(link.Href) != "" {
				return strings.TrimSpace(link.Href)
			}
		}
	}
	return ""
}

func payPalCompletedCapture(order payPalOrder) (payPalCapture, bool) {
	for _, unit := range order.PurchaseUnits {
		for _, capture := range unit.Payments.Captures {
			if strings.EqualFold(strings.TrimSpace(capture.Status), payPalOrderStatusCompleted) {
				if capture.CustomID == "" {
					capture.CustomID = unit.CustomID
				}
				if capture.InvoiceID == "" {
					capture.InvoiceID = unit.InvoiceID
				}
				return capture, true
			}
		}
	}
	return payPalCapture{}, false
}

func payPalOrderMoney(order payPalOrder) (payPalMoney, bool) {
	for _, unit := range order.PurchaseUnits {
		if strings.TrimSpace(unit.Amount.Value) != "" || strings.TrimSpace(unit.Amount.CurrencyCode) != "" {
			return unit.Amount, true
		}
	}
	return payPalMoney{}, false
}

func payPalOrderLocalID(order payPalOrder) string {
	for _, unit := range order.PurchaseUnits {
		if value := strings.TrimSpace(unit.CustomID); value != "" {
			return value
		}
		if value := strings.TrimSpace(unit.InvoiceID); value != "" {
			return value
		}
		if value := strings.TrimSpace(unit.ReferenceID); value != "" {
			return value
		}
	}
	return ""
}

func payPalCaptureLocalID(capture payPalCapture) string {
	if value := strings.TrimSpace(capture.CustomID); value != "" {
		return value
	}
	return strings.TrimSpace(capture.InvoiceID)
}

func payPalHeader(headers map[string]string, key string) string {
	for candidate, value := range headers {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func payPalRequestID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func truncatePayPalText(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

type payPalHTTPError struct {
	StatusCode int
	Name       string
	Message    string
	DebugID    string
	Details    []payPalErrorDetail
	Summary    string
}

func (e *payPalHTTPError) Error() string {
	parts := []string{fmt.Sprintf("HTTP %d", e.StatusCode)}
	if e.Name != "" {
		parts = append(parts, e.Name)
	}
	if len(e.Details) > 0 && strings.TrimSpace(e.Details[0].Issue) != "" {
		parts = append(parts, strings.TrimSpace(e.Details[0].Issue))
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	} else if e.Summary != "" {
		parts = append(parts, e.Summary)
	}
	if e.DebugID != "" {
		parts = append(parts, "debug_id="+e.DebugID)
	}
	return strings.Join(parts, ": ")
}

func parsePayPalHTTPError(status int, body []byte) error {
	var payload struct {
		Name             string              `json:"name"`
		Message          string              `json:"message"`
		DebugID          string              `json:"debug_id"`
		Details          []payPalErrorDetail `json:"details"`
		Error            string              `json:"error"`
		ErrorDescription string              `json:"error_description"`
	}
	_ = json.Unmarshal(body, &payload)
	name := strings.TrimSpace(payload.Name)
	message := strings.TrimSpace(payload.Message)
	if name == "" {
		name = strings.TrimSpace(payload.Error)
	}
	if message == "" {
		message = strings.TrimSpace(payload.ErrorDescription)
	}
	summary := strings.TrimSpace(string(body))
	if len(summary) > payPalMaxErrorSummary {
		summary = summary[:payPalMaxErrorSummary] + "...(truncated)"
	}
	return &payPalHTTPError{
		StatusCode: status,
		Name:       name,
		Message:    message,
		DebugID:    strings.TrimSpace(payload.DebugID),
		Details:    payload.Details,
		Summary:    summary,
	}
}

type payPalAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type payPalCreateOrderRequest struct {
	Intent             string                      `json:"intent"`
	PurchaseUnits      []payPalPurchaseUnitRequest `json:"purchase_units"`
	ApplicationContext payPalApplicationContext    `json:"application_context"`
}

type payPalPurchaseUnitRequest struct {
	ReferenceID string      `json:"reference_id,omitempty"`
	CustomID    string      `json:"custom_id,omitempty"`
	InvoiceID   string      `json:"invoice_id,omitempty"`
	Description string      `json:"description,omitempty"`
	Amount      payPalMoney `json:"amount"`
}

type payPalApplicationContext struct {
	BrandName   string `json:"brand_name,omitempty"`
	LandingPage string `json:"landing_page,omitempty"`
	UserAction  string `json:"user_action,omitempty"`
	ReturnURL   string `json:"return_url"`
	CancelURL   string `json:"cancel_url"`
}

type payPalMoney struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type payPalOrder struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	PurchaseUnits []payPalPurchaseUnit `json:"purchase_units"`
	Links         []payPalLink         `json:"links"`
}

type payPalPurchaseUnit struct {
	ReferenceID string         `json:"reference_id"`
	CustomID    string         `json:"custom_id"`
	InvoiceID   string         `json:"invoice_id"`
	Amount      payPalMoney    `json:"amount"`
	Payments    payPalPayments `json:"payments"`
}

type payPalPayments struct {
	Captures []payPalCapture `json:"captures"`
}

type payPalCapture struct {
	ID                string                  `json:"id"`
	Status            string                  `json:"status"`
	Amount            payPalMoney             `json:"amount"`
	CustomID          string                  `json:"custom_id"`
	InvoiceID         string                  `json:"invoice_id"`
	CreateTime        string                  `json:"create_time"`
	SupplementaryData payPalSupplementaryData `json:"supplementary_data"`
}

type payPalSupplementaryData struct {
	RelatedIDs payPalRelatedIDs `json:"related_ids"`
}

type payPalRelatedIDs struct {
	OrderID string `json:"order_id"`
}

type payPalLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

type payPalWebhookEvent struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	Resource     json.RawMessage `json:"resource"`
}

type payPalVerifyWebhookRequest struct {
	AuthAlgo         string          `json:"auth_algo"`
	CertURL          string          `json:"cert_url"`
	TransmissionID   string          `json:"transmission_id"`
	TransmissionSig  string          `json:"transmission_sig"`
	TransmissionTime string          `json:"transmission_time"`
	WebhookID        string          `json:"webhook_id"`
	WebhookEvent     json.RawMessage `json:"webhook_event"`
}

type payPalVerifyWebhookResponse struct {
	VerificationStatus string `json:"verification_status"`
}

type payPalRefundRequest struct {
	Amount      payPalMoney `json:"amount"`
	NoteToPayer string      `json:"note_to_payer,omitempty"`
}

type payPalRefund struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type payPalErrorDetail struct {
	Issue       string `json:"issue"`
	Description string `json:"description"`
}
