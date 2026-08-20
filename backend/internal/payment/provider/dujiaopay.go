package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	dujiaoPayDefaultAPIBase      = "https://www.dujiaopay.com"
	dujiaoPayHTTPTimeout         = 15 * time.Second
	dujiaoPayMaxResponseSize     = 1 << 20
	dujiaoPayMaxErrorSummary     = 512
	dujiaoPayWebhookTolerance    = 5 * time.Minute
	dujiaoPayEventOrderPaid      = "order.paid"
	dujiaoPayOrderStatusPaid     = "paid"
	dujiaoPayOrderStatusExpired  = "expired"
	dujiaoPayOrderStatusCanceled = "canceled"
)

// DujiaoPay implements the non-custodial DujiaoPay stablecoin checkout API.
type DujiaoPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

func NewDujiaoPay(instanceID string, config map[string]string) (*DujiaoPay, error) {
	for _, key := range []string{"keyId", "apiSecret"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("dujiaopay config missing required key: %s", key)
		}
	}

	cfg := cloneStringMap(config)
	apiBase, err := normalizeDujiaoPayAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("dujiaopay config currency: %w", err)
	}
	cfg["currency"] = currency

	return &DujiaoPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: dujiaoPayHTTPTimeout},
	}, nil
}

func normalizeDujiaoPayAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = dujiaoPayDefaultAPIBase
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("dujiaopay apiBase must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "www.dujiaopay.com" && host != "dujiaopay.com" {
		return "", fmt.Errorf("dujiaopay apiBase host must be www.dujiaopay.com")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("dujiaopay apiBase must not include a path, query, or fragment")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (d *DujiaoPay) Name() string        { return "DujiaoPay" }
func (d *DujiaoPay) ProviderKey() string { return payment.TypeDujiaoPay }
func (d *DujiaoPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeDujiaoPay}
}

func (d *DujiaoPay) MerchantIdentityMetadata() map[string]string {
	if d == nil {
		return nil
	}
	return map[string]string{
		"key_id":   strings.TrimSpace(d.config["keyId"]),
		"currency": d.currency(),
	}
}

func (d *DujiaoPay) currency() string {
	if d == nil {
		return payment.DefaultPaymentCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(d.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
	}
	return currency
}

func (d *DujiaoPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return nil, fmt.Errorf("dujiaopay create payment: missing order id")
	}
	amount, err := normalizeDujiaoPayAmount(req.Amount, d.currency())
	if err != nil {
		return nil, fmt.Errorf("dujiaopay create payment: %w", err)
	}

	payload := dujiaoPayCreateOrderRequest{
		FiatCurrency:    d.currency(),
		FiatAmount:      amount,
		MerchantOrderID: orderID,
		Metadata: map[string]string{
			"out_trade_no": orderID,
		},
		SuccessURL: strings.TrimSpace(req.ReturnURL),
		CancelURL:  strings.TrimSpace(req.ReturnURL),
	}

	var response dujiaoPayOrderResponse
	if err := d.doJSON(ctx, http.MethodPost, "/v1/orders", payload, &response, orderID); err != nil {
		return nil, fmt.Errorf("dujiaopay create payment: %w", err)
	}
	order := response.value()
	if strings.TrimSpace(order.OrderID) == "" || strings.TrimSpace(order.CheckoutURL) == "" {
		return nil, fmt.Errorf("dujiaopay create payment: response missing order_id or checkout_url")
	}

	return &payment.CreatePaymentResponse{
		TradeNo:  order.OrderID,
		PayURL:   order.CheckoutURL,
		Currency: d.currency(),
	}, nil
}

func normalizeDujiaoPayAmount(raw, currency string) (string, error) {
	if _, err := payment.AmountToMinorUnit(raw, currency); err != nil {
		return "", err
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return "", fmt.Errorf("invalid amount: %s", raw)
	}
	return amount.StringFixed(int32(payment.CurrencyMaxFractionDigits(currency))), nil
}

func (d *DujiaoPay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	orderID := strings.TrimSpace(tradeNo)
	if orderID == "" {
		return nil, fmt.Errorf("dujiaopay query order: missing order id")
	}

	var response dujiaoPayOrderResponse
	path := "/v1/orders/" + url.PathEscape(orderID)
	if err := d.doJSON(ctx, http.MethodGet, path, nil, &response, ""); err != nil {
		return nil, fmt.Errorf("dujiaopay query order: %w", err)
	}
	order := response.value()
	if strings.TrimSpace(order.OrderID) == "" {
		return nil, fmt.Errorf("dujiaopay query order: response missing order_id")
	}

	status := dujiaoPayProviderStatus(order.Status)
	amount := 0.0
	if strings.TrimSpace(order.FiatAmount) != "" {
		parsed, err := decimal.NewFromString(strings.TrimSpace(order.FiatAmount))
		if err != nil || parsed.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("dujiaopay query order: invalid fiat_amount")
		}
		amount = parsed.InexactFloat64()
	} else if status == payment.ProviderStatusPaid {
		return nil, fmt.Errorf("dujiaopay query order: paid response missing fiat_amount")
	}

	currency := strings.ToUpper(strings.TrimSpace(order.FiatCurrency))
	if currency == "" {
		if status == payment.ProviderStatusPaid {
			return nil, fmt.Errorf("dujiaopay query order: paid response missing fiat_currency")
		}
		currency = d.currency()
	}
	metadata := d.orderMetadata(order, currency)

	return &payment.QueryOrderResponse{
		TradeNo:  order.OrderID,
		Status:   status,
		Amount:   amount,
		PaidAt:   strings.TrimSpace(order.PaidAt),
		Metadata: metadata,
	}, nil
}

func (d *DujiaoPay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if err := verifyDujiaoPayWebhookSignature(rawBody, headers, d.config["webhookSecret"], time.Now()); err != nil {
		return nil, err
	}

	var event dujiaoPayWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("dujiaopay parse webhook: %w", err)
	}
	if strings.TrimSpace(event.EventType) != dujiaoPayEventOrderPaid {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(event.Data.Status), dujiaoPayOrderStatusPaid) {
		return nil, fmt.Errorf("dujiaopay paid webhook has non-paid status: %s", event.Data.Status)
	}
	if strings.TrimSpace(event.Data.OrderID) == "" {
		return nil, fmt.Errorf("dujiaopay paid webhook missing order_id")
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(event.Data.FiatAmount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("dujiaopay paid webhook has invalid fiat_amount")
	}
	currency := strings.ToUpper(strings.TrimSpace(event.Data.FiatCurrency))
	if currency == "" {
		return nil, fmt.Errorf("dujiaopay paid webhook missing fiat_currency")
	}

	outTradeNo := strings.TrimSpace(event.Data.MerchantOrderID)
	if outTradeNo == "" {
		outTradeNo = dujiaoPayMetadataString(event.Data.Metadata, "out_trade_no")
	}
	if outTradeNo == "" {
		outTradeNo = dujiaoPayMetadataString(event.Data.Metadata, "order_id")
	}

	metadata := map[string]string{
		"key_id":   strings.TrimSpace(d.config["keyId"]),
		"currency": currency,
		"status":   strings.ToLower(strings.TrimSpace(event.Data.Status)),
		"event_id": strings.TrimSpace(event.EventID),
		"chain":    strings.TrimSpace(event.Data.Chain),
		"token_id": strings.TrimSpace(event.Data.TokenID),
		"tx_id":    strings.TrimSpace(event.Data.TxID),
	}

	return &payment.PaymentNotification{
		TradeNo:  strings.TrimSpace(event.Data.OrderID),
		OrderID:  outTradeNo,
		Amount:   amount.InexactFloat64(),
		Status:   payment.NotificationStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (d *DujiaoPay) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("dujiaopay does not issue on-chain refunds; process the refund externally")
}

func (d *DujiaoPay) CancelPayment(ctx context.Context, tradeNo string) error {
	orderID := strings.TrimSpace(tradeNo)
	if orderID == "" {
		return nil
	}
	path := "/v1/orders/" + url.PathEscape(orderID) + "/cancel"
	if err := d.doJSON(ctx, http.MethodPost, path, nil, nil, ""); err != nil {
		return fmt.Errorf("dujiaopay cancel payment: %w", err)
	}
	return nil
}

func (d *DujiaoPay) orderMetadata(order dujiaoPayOrder, currency string) map[string]string {
	metadata := map[string]string{
		"key_id":   strings.TrimSpace(d.config["keyId"]),
		"currency": strings.ToUpper(strings.TrimSpace(currency)),
		"status":   strings.ToLower(strings.TrimSpace(order.Status)),
		"chain":    strings.TrimSpace(order.Chain),
		"token_id": strings.TrimSpace(order.TokenID),
		"tx_id":    strings.TrimSpace(order.TxID),
	}
	return metadata
}

func dujiaoPayProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case dujiaoPayOrderStatusPaid:
		return payment.ProviderStatusPaid
	case dujiaoPayOrderStatusExpired, dujiaoPayOrderStatusCanceled, "cancelled":
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func (d *DujiaoPay) doJSON(ctx context.Context, method, path string, payload, out any, idempotencyKey string) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	target := strings.TrimRight(d.config["apiBase"], "/") + path
	parsed, err := url.Parse(target)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	timestamp := time.Now().Unix()
	nonce := uuid.NewString()
	signedHeaders := signDujiaoPayRequest(
		d.config["apiSecret"],
		d.config["keyId"],
		method,
		parsed.EscapedPath(),
		parsed.RawQuery,
		body,
		timestamp,
		nonce,
	)
	for key, value := range signedHeaders {
		req.Header.Set(key, value)
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey))
	}

	client := d.httpClient
	if client == nil {
		client = &http.Client{Timeout: dujiaoPayHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, dujiaoPayMaxResponseSize))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeDujiaoPayResponse(responseBody))
	}
	if out == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func signDujiaoPayRequest(secret, keyID, method, path, rawQuery string, body []byte, unix int64, nonce string) map[string]string {
	canonical := strings.Join([]string{
		strings.ToUpper(method),
		path,
		canonicalDujiaoPayQuery(rawQuery),
		sha256Hex(body),
		strconv.FormatInt(unix, 10),
		nonce,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return map[string]string{
		"DJP-Key-ID":    keyID,
		"DJP-Timestamp": strconv.FormatInt(unix, 10),
		"DJP-Nonce":     nonce,
		"DJP-Signature": hex.EncodeToString(mac.Sum(nil)),
	}
}

func canonicalDujiaoPayQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(values))
	for _, key := range keys {
		items := append([]string(nil), values[key]...)
		sort.Strings(items)
		for _, value := range items {
			parts = append(parts, dujiaoPayQueryEscape(key)+"="+dujiaoPayQueryEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func dujiaoPayQueryEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func verifyDujiaoPayWebhookSignature(rawBody string, headers map[string]string, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("dujiaopay webhookSecret not configured")
	}
	timestamp := dujiaoPayHeader(headers, "djp-webhook-timestamp")
	signature := strings.ToLower(dujiaoPayHeader(headers, "djp-webhook-signature"))
	signature = strings.TrimPrefix(signature, "sha256=")
	if timestamp == "" || signature == "" {
		return fmt.Errorf("dujiaopay notification missing webhook timestamp or signature header")
	}

	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || unix <= 0 {
		return fmt.Errorf("dujiaopay invalid webhook timestamp")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if diff := now.Sub(time.Unix(unix, 0)).Abs(); diff > dujiaoPayWebhookTolerance {
		return fmt.Errorf("dujiaopay webhook timestamp outside tolerance")
	}

	provided, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("dujiaopay invalid webhook signature encoding")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + rawBody))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return fmt.Errorf("dujiaopay invalid webhook signature")
	}
	return nil
}

func dujiaoPayHeader(headers map[string]string, key string) string {
	for candidate, value := range headers {
		if strings.EqualFold(candidate, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dujiaoPayMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func summarizeDujiaoPayResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > dujiaoPayMaxErrorSummary {
		return summary[:dujiaoPayMaxErrorSummary] + "..."
	}
	return summary
}

type dujiaoPayCreateOrderRequest struct {
	FiatCurrency    string            `json:"fiat_currency"`
	FiatAmount      string            `json:"fiat_amount"`
	MerchantOrderID string            `json:"merchant_order_id"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	SuccessURL      string            `json:"success_url,omitempty"`
	CancelURL       string            `json:"cancel_url,omitempty"`
}

type dujiaoPayOrder struct {
	OrderID         string         `json:"order_id"`
	Chain           string         `json:"chain"`
	TokenID         string         `json:"token_id"`
	PayAddress      string         `json:"pay_address"`
	PayableAmount   string         `json:"payable_amount"`
	FiatCurrency    string         `json:"fiat_currency"`
	FiatAmount      string         `json:"fiat_amount"`
	MerchantOrderID string         `json:"merchant_order_id"`
	Metadata        map[string]any `json:"metadata"`
	Status          string         `json:"status"`
	ExpiresAt       string         `json:"expires_at"`
	PaidAt          string         `json:"paid_at"`
	CheckoutToken   string         `json:"checkout_token"`
	CheckoutURL     string         `json:"checkout_url"`
	TxID            string         `json:"tx_id"`
}

type dujiaoPayOrderResponse struct {
	dujiaoPayOrder
	Data  *dujiaoPayOrder `json:"data"`
	Order *dujiaoPayOrder `json:"order"`
}

func (r dujiaoPayOrderResponse) value() dujiaoPayOrder {
	if strings.TrimSpace(r.dujiaoPayOrder.OrderID) != "" {
		return r.dujiaoPayOrder
	}
	if r.Data != nil {
		return *r.Data
	}
	if r.Order != nil {
		return *r.Order
	}
	return r.dujiaoPayOrder
}

type dujiaoPayWebhookEvent struct {
	EventID      string                    `json:"event_id"`
	EventType    string                    `json:"event_type"`
	EventVersion string                    `json:"event_version"`
	CreatedAt    string                    `json:"created_at"`
	Data         dujiaoPayWebhookOrderData `json:"data"`
}

type dujiaoPayWebhookOrderData struct {
	OrderID         string         `json:"order_id"`
	MerchantOrderID string         `json:"merchant_order_id"`
	Metadata        map[string]any `json:"metadata"`
	Status          string         `json:"status"`
	Chain           string         `json:"chain"`
	TokenID         string         `json:"token_id"`
	PayableAmount   string         `json:"payable_amount"`
	FiatCurrency    string         `json:"fiat_currency"`
	FiatAmount      string         `json:"fiat_amount"`
	TxID            string         `json:"tx_id"`
	Confirmations   int            `json:"confirmations"`
}
