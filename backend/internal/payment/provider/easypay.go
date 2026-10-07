// Package provider contains concrete payment provider implementations.
package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
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
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// EasyPay constants.
const (
	easypayCodeSuccess      = 1
	easypayHTTPTimeout      = 10 * time.Second
	easypayQueryMaxAttempts = 3
	easypayQueryRetryDelay  = 200 * time.Millisecond
	maxEasypayResponseSize  = 1 << 20 // 1MB
	maxEasypayErrorSummary  = 512
	tradeStatusSuccess      = "TRADE_SUCCESS"
	signTypeMD5             = "MD5"
	paymentModePopup        = "popup"
	deviceMobile            = "mobile"
)

// EasyPay implements payment.Provider for the EasyPay aggregation platform.
type EasyPay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

type easyPayCustomMethod struct {
	Type         string `json:"type"`
	UpstreamType string `json:"upstreamType"`
	DisplayName  string `json:"displayName"`
}

// NewEasyPay creates a new EasyPay provider.
// config keys: pid, pkey, apiBase, notifyUrl, returnUrl, cid, cidAlipay, cidWxpay
func NewEasyPay(instanceID string, config map[string]string) (*EasyPay, error) {
	for _, k := range []string{"pid", "pkey", "apiBase", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("easypay config missing required key: %s", k)
		}
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = v
	}
	cfg["apiBase"] = normalizeEasyPayAPIBase(cfg["apiBase"])
	return &EasyPay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: easypayHTTPTimeout},
	}, nil
}

func normalizeEasyPayAPIBase(apiBase string) string {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		parsed.Path = trimEasyPayEndpointPath(parsed.Path)
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(trimEasyPayEndpointPath(base), "/")
}

func trimEasyPayEndpointPath(path string) string {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	lower := strings.ToLower(path)
	for _, endpoint := range []string{"/submit.php", "/mapi.php", "/api.php"} {
		if strings.HasSuffix(lower, endpoint) {
			return strings.TrimRight(path[:len(path)-len(endpoint)], "/")
		}
	}
	return path
}

func (e *EasyPay) apiBase() string {
	if e == nil {
		return ""
	}
	return normalizeEasyPayAPIBase(e.config["apiBase"])
}

func (e *EasyPay) Name() string        { return "EasyPay" }
func (e *EasyPay) ProviderKey() string { return payment.TypeEasyPay }
func (e *EasyPay) SupportedTypes() []payment.PaymentType {
	types := []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
	for _, method := range e.customMethods() {
		if method.Type != "" {
			types = append(types, method.Type)
		}
	}
	return types
}

func (e *EasyPay) MerchantIdentityMetadata() map[string]string {
	if e == nil {
		return nil
	}
	pid := strings.TrimSpace(e.config["pid"])
	if pid == "" {
		return nil
	}
	return map[string]string{"pid": pid}
}

func (e *EasyPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	// Payment mode determined by instance config, not payment type.
	// "popup" → hosted page (submit.php); "qrcode"/default → API call (mapi.php).
	mode := e.config["paymentMode"]
	if mode == paymentModePopup {
		return e.createRedirectPayment(req)
	}
	return e.createAPIPayment(ctx, req)
}

// createRedirectPayment builds a submit.php URL for browser redirect.
// No server-side API call — the user is redirected to EasyPay's hosted page.
// TradeNo is empty; it arrives via the notify callback after payment.
func (e *EasyPay) createRedirectPayment(req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	paymentType := e.upstreamPaymentType(req.PaymentType)
	params := map[string]string{
		"pid": e.config["pid"], "type": paymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount,
	}
	if cid := e.resolveCID(paymentType); cid != "" {
		params["cid"] = cid
	}
	if req.IsMobile {
		params["device"] = deviceMobile
	}
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	payURL := e.apiBase() + "/submit.php?" + q.Encode()
	return &payment.CreatePaymentResponse{PayURL: payURL}, nil
}

// createAPIPayment calls mapi.php to get payurl/qrcode (existing behavior).
func (e *EasyPay) createAPIPayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	notifyURL, returnURL := e.resolveURLs(req)
	paymentType := e.upstreamPaymentType(req.PaymentType)
	params := map[string]string{
		"pid": e.config["pid"], "type": paymentType,
		"out_trade_no": req.OrderID, "notify_url": notifyURL,
		"return_url": returnURL, "name": req.Subject,
		"money": req.Amount, "clientip": req.ClientIP,
	}
	if cid := e.resolveCID(paymentType); cid != "" {
		params["cid"] = cid
	}
	if req.IsMobile {
		params["device"] = deviceMobile
	}
	params["sign"] = easyPaySign(params, e.config["pkey"])
	params["sign_type"] = signTypeMD5

	body, err := e.post(ctx, e.apiBase()+"/mapi.php", params)
	if err != nil {
		return nil, fmt.Errorf("easypay create: %w", err)
	}
	var resp struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		PayURL2 string `json:"payurl2"` // H5 mobile payment URL
		QRCode  string `json:"qrcode"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("easypay parse: %w", err)
	}
	if resp.Code != easypayCodeSuccess {
		return nil, fmt.Errorf("easypay error: %s", resp.Msg)
	}
	payURL := resp.PayURL
	if req.IsMobile && resp.PayURL2 != "" {
		payURL = resp.PayURL2
	}
	base := e.apiBase()
	return &payment.CreatePaymentResponse{
		TradeNo: resp.TradeNo,
		PayURL:  resolveEasyPayReturnedRef(base, payURL),
		QRCode:  resolveEasyPayReturnedRef(base, resp.QRCode),
	}, nil
}

// resolveEasyPayReturnedRef absolutizes a payurl/payurl2/qrcode reference that
// mapi.php returned, resolving it against the instance's configured apiBase.
//
// EasyPay-compatible upstreams disagree on this field: some answer with a full
// checkout URL, others with a site-root-relative path such as
// "/api/pay/toapp/<order>". createRedirectPayment already builds an absolute URL
// itself (apiBase + "/submit.php?..."), but the mapi.php branch stored whatever
// came back verbatim, and nothing downstream repairs it —
// sanitizeCreatePaymentResponseDetails only strips NUL bytes before the value is
// persisted to pay_url/qr_code. The frontend then feeds qr_code straight into
// QRCode.toCanvas (PaymentQRCodeView.renderQR), so a relative path becomes a QR
// whose payload is a bare path: WeChat renders it as text, and pay_url resolves
// against the gateway's own domain and 404s.
//
// That is precisely the outcome the Alipay provider already refuses to produce —
// "Setting it as QRCode would let the frontend render an unscannable image"
// (createPagePayTrade) — so EasyPay is brought in line with the same rule.
//
// Only references beginning with "/" are resolved. Anything carrying a scheme is
// already actionable and is returned untouched, which covers both absolute
// https:// checkout URLs and app deep links (weixin://, wxp://, alipays://). A
// reference without a leading slash is left alone as well: QR payloads are
// frequently opaque tokens rather than paths, and rewriting "OrderToken123" into
// "<apiBase>/OrderToken123" would break a payload that works today — strictly
// worse than the bug this fixes.
func resolveEasyPayReturnedRef(apiBase, ref string) string {
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, "/") {
		return ref
	}
	base, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ref
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != "" {
		return ref
	}
	return base.ResolveReference(parsed).String()
}

// resolveURLs returns (notifyURL, returnURL) preferring request values,
// falling back to instance config.
func (e *EasyPay) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = e.config["notifyUrl"]
	}
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = e.config["returnUrl"]
	}
	return notifyURL, returnURL
}

func (e *EasyPay) customMethods() []easyPayCustomMethod {
	if e == nil {
		return nil
	}
	raw := strings.TrimSpace(e.config["customMethods"])
	if raw == "" {
		return nil
	}
	var methods []easyPayCustomMethod
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return nil
	}
	result := make([]easyPayCustomMethod, 0, len(methods))
	for _, method := range methods {
		method.Type = strings.TrimSpace(method.Type)
		method.UpstreamType = strings.TrimSpace(method.UpstreamType)
		method.DisplayName = strings.TrimSpace(method.DisplayName)
		if method.Type == "" || method.UpstreamType == "" {
			continue
		}
		result = append(result, method)
	}
	return result
}

func (e *EasyPay) upstreamPaymentType(paymentType string) string {
	paymentType = strings.TrimSpace(paymentType)
	for _, method := range e.customMethods() {
		if paymentType == method.Type {
			return method.UpstreamType
		}
	}
	return paymentType
}

func (e *EasyPay) QueryOrder(ctx context.Context, orderID string) (*payment.QueryOrderResponse, error) {
	if !validEasyPayIdentifier(orderID) {
		return nil, fmt.Errorf("easypay query: invalid out_trade_no")
	}
	params := map[string]string{
		"act": "order", "pid": e.config["pid"],
		"key": e.config["pkey"], "out_trade_no": orderID,
	}
	body, err := e.queryOrder(ctx, e.apiBase()+"/api.php", params)
	if err != nil {
		// Transport errors can contain the request URL, including the merchant key.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("easypay query: %w", ctx.Err())
		}
		return nil, fmt.Errorf("easypay query: request failed")
	}
	resp, err := parseEasyPayQueryObject(body)
	if err != nil {
		return nil, err
	}
	// Compare the exact code without float conversion or integer truncation.
	code, err := easyPayQueryText(resp["code"], true)
	if err != nil || code != "1" {
		return nil, fmt.Errorf("easypay query: unsuccessful response code")
	}
	var data map[string]json.RawMessage
	if raw, ok := resp["data"]; ok {
		data, err = parseEasyPayQueryObject(raw)
		if err != nil {
			return nil, err
		}
	}

	// Both response layouts and status representations remain supported, but
	// every supplied status must agree before any response can authorize payment.
	var statuses []bool
	for _, key := range []string{"trade_status", "status"} {
		for _, fields := range []map[string]json.RawMessage{resp, data} {
			if raw, ok := fields[key]; ok {
				value, err := easyPayQueryText(raw, key == "status")
				if err != nil || (key == "status" && value != "0" && value != "1") ||
					(key == "trade_status" && value != "" && !validEasyPayIdentifier(value)) {
					return nil, fmt.Errorf("easypay query: invalid payment status")
				}
				statuses = append(statuses, value == tradeStatusSuccess || (key == "status" && value == "1"))
			}
		}
	}
	if len(statuses) == 0 {
		return nil, fmt.Errorf("easypay query: missing payment status")
	}
	for _, paid := range statuses[1:] {
		if paid != statuses[0] {
			return nil, fmt.Errorf("easypay query: conflicting payment status")
		}
	}
	status := payment.ProviderStatusPending
	if statuses[0] {
		status = payment.ProviderStatusPaid
	}

	// Never assemble conflicting identities from the envelope and nested order.
	verified := make(map[string]string)
	var amount float64
	for _, key := range []string{"pid", "out_trade_no", "trade_no", "money"} {
		for _, fields := range []map[string]json.RawMessage{resp, data} {
			if raw, ok := fields[key]; ok {
				value, err := easyPayQueryText(raw, key == "pid")
				if err != nil {
					return nil, fmt.Errorf("easypay query: invalid %s", key)
				}
				if key == "money" {
					parsed, err := parseEasyPayAmount(value)
					if err != nil {
						return nil, fmt.Errorf("easypay query: invalid money")
					}
					if _, exists := verified[key]; exists && amount != parsed {
						return nil, fmt.Errorf("easypay query: conflicting money")
					}
					amount = parsed
				} else {
					if !validEasyPayIdentifier(value) {
						return nil, fmt.Errorf("easypay query: invalid %s", key)
					}
					if previous, exists := verified[key]; exists && previous != value {
						return nil, fmt.Errorf("easypay query: conflicting %s", key)
					}
				}
				if _, exists := verified[key]; !exists {
					verified[key] = value
				}
			}
		}
	}
	if pid, ok := verified["pid"]; ok && pid != strings.TrimSpace(e.config["pid"]) {
		return nil, fmt.Errorf("easypay query: pid mismatch")
	}
	if returnedOrder, ok := verified["out_trade_no"]; ok && returnedOrder != orderID {
		return nil, fmt.Errorf("easypay query: out_trade_no mismatch")
	}
	if status == payment.ProviderStatusPaid {
		for _, key := range []string{"pid", "out_trade_no", "trade_no", "money"} {
			if _, ok := verified[key]; !ok {
				return nil, fmt.Errorf("easypay query: paid response missing %s", key)
			}
		}
	}
	return &payment.QueryOrderResponse{
		TradeNo:  verified["trade_no"],
		Status:   status,
		Amount:   amount,
		Metadata: verified,
	}, nil
}

// Decode each object explicitly: encoding/json's usual last-key-wins behavior
// would hide duplicate identity/status fields. Unknown query fields are allowed
// because documented replies also include timestamps, buyer, name, etc.
func parseEasyPayQueryObject(body []byte) (map[string]json.RawMessage, error) {
	invalid := fmt.Errorf("easypay query: invalid response object")
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := token.(string)
		if !ok {
			return nil, invalid
		}
		if _, exists := fields[key]; exists {
			return nil, invalid
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, invalid
		}
		fields[key] = raw
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, invalid
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, invalid
	}
	return fields, nil
}

// PHP gateways may encode integer fields as either JSON integers or strings.
// Do not accept null, booleans, fractions, exponents, or lossy float coercions.
func easyPayQueryText(raw json.RawMessage, allowInteger bool) (string, error) {
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return value, nil
		}
	} else if allowInteger && len(raw) > 0 {
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				return "", fmt.Errorf("invalid field type")
			}
		}
		return string(raw), nil
	}
	return "", fmt.Errorf("invalid field type")
}

func (e *EasyPay) queryOrder(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < easypayQueryMaxAttempts; attempt++ {
		body, status, err := e.getRaw(ctx, endpoint, params)
		if err == nil && status >= http.StatusOK && status < http.StatusMultipleChoices {
			return body, nil
		}
		if err == nil {
			err = fmt.Errorf("HTTP %d", status)
		}
		lastErr = err
		if attempt+1 >= easypayQueryMaxAttempts || !isRetryableEasyPayQuery(status, err) {
			break
		}

		timer := time.NewTimer(easypayQueryRetryDelay * time.Duration(1<<attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func isRetryableEasyPayQuery(status int, err error) bool {
	return err != nil && (status == 0 || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError)
}

func (e *EasyPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	params, err := parseEasyPayNotification(rawBody)
	if err != nil {
		return nil, err
	}
	if params["pid"] != strings.TrimSpace(e.config["pid"]) {
		return nil, fmt.Errorf("easypay notify: pid mismatch")
	}
	if !easyPayVerifySign(params, e.config["pkey"], strings.ToLower(params["sign"])) {
		return nil, fmt.Errorf("easypay notify: invalid signature")
	}
	status := payment.ProviderStatusFailed
	if params["trade_status"] == tradeStatusSuccess {
		status = payment.ProviderStatusSuccess
	}
	amount, err := parseEasyPayAmount(params["money"])
	if err != nil {
		return nil, fmt.Errorf("easypay notify: invalid money")
	}
	return &payment.PaymentNotification{
		TradeNo: params["trade_no"], OrderID: params["out_trade_no"],
		Amount: amount, Status: status, RawData: rawBody,
		Metadata: map[string]string{"pid": params["pid"], "out_trade_no": params["out_trade_no"], "money": params["money"]},
	}, nil
}

func parseEasyPayNotification(rawBody string) (map[string]string, error) {
	if len(rawBody) > maxEasypayResponseSize {
		return nil, fmt.Errorf("easypay notify: invalid form")
	}
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		// ParseQuery errors can contain untrusted callback fragments.
		return nil, fmt.Errorf("easypay notify: invalid form")
	}
	params := make(map[string]string, len(values))
	for key, entries := range values {
		switch key {
		case "pid", "trade_no", "out_trade_no", "type", "name", "money", "trade_status", "sign", "sign_type", "param":
		default:
			return nil, fmt.Errorf("easypay notify: unexpected field")
		}
		if len(entries) != 1 {
			return nil, fmt.Errorf("easypay notify: duplicate field")
		}
		value := entries[0] // ParseQuery decodes exactly once, before signing.
		if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
			return nil, fmt.Errorf("easypay notify: invalid field")
		}
		// EasyPay signs unescaped key=value pairs. A value containing another
		// &key=value boundary can turn a signed payment request into a callback.
		// Ordinary ampersands, equals signs, Unicode and URL-encoded text remain
		// compatible; ambiguous embedded assignments must fail closed.
		if i := strings.IndexByte(value, '&'); i >= 0 && strings.Contains(value[i:], "=") {
			return nil, fmt.Errorf("easypay notify: ambiguous field")
		}
		params[key] = value
	}
	// The subject is display metadata. Some compatible gateways omit it from
	// notifications. Do not invent a value: verify the exact fields received.
	// sign_type defaults to MD5 in EasyPay; any explicit other algorithm fails.
	for _, key := range []string{"pid", "trade_no", "out_trade_no", "type", "money", "trade_status", "sign"} {
		if strings.TrimSpace(params[key]) == "" {
			// key comes from this fixed list, never from untrusted field values.
			return nil, fmt.Errorf("easypay notify: missing required field (%s)", key)
		}
	}
	if name := params["name"]; name != "" && strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("easypay notify: invalid name")
	}
	for _, key := range []string{"pid", "trade_no", "out_trade_no", "type", "trade_status"} {
		if !validEasyPayIdentifier(params[key]) {
			return nil, fmt.Errorf("easypay notify: invalid %s", key)
		}
	}
	if params["sign_type"] != "" && !strings.EqualFold(params["sign_type"], signTypeMD5) {
		return nil, fmt.Errorf("easypay notify: unsupported sign_type")
	}
	if sign, err := hex.DecodeString(params["sign"]); err != nil || len(sign) != md5.Size {
		return nil, fmt.Errorf("easypay notify: invalid signature format")
	}
	return params, nil
}

// Opaque merchant IDs and custom payment types are used by compatible gateways;
// retain letters, digits, '_' and '-' rather than requiring numeric IDs only.
func validEasyPayIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') &&
			(ch < '0' || ch > '9') && ch != '_' && ch != '-' {
			return false
		}
	}
	return true
}

// Validate decimal yuan before conversion to the shared float64 response type.
// Retaining integer cents and checking a decimal round trip prevents accepting
// exponents, non-finite values, fractional cents or silently rounded amounts.
func parseEasyPayAmount(value string) (float64, error) {
	invalid := fmt.Errorf("invalid decimal amount")
	whole, fraction, hasPoint := strings.Cut(value, ".")
	if whole == "" || (hasPoint && (len(fraction) < 1 || len(fraction) > 2)) {
		return 0, invalid
	}
	for _, ch := range whole + fraction {
		if ch < '0' || ch > '9' {
			return 0, invalid
		}
	}
	centString := strings.TrimLeft(whole+fraction+strings.Repeat("0", 2-len(fraction)), "0")
	cents, err := strconv.ParseUint(centString, 10, 64)
	if err != nil || cents == 0 || cents > 1<<53-1 {
		return 0, invalid
	}
	amount := float64(cents) / 100
	if strconv.FormatFloat(amount, 'f', 2, 64) != fmt.Sprintf("%d.%02d", cents/100, cents%100) {
		return 0, invalid
	}
	return amount, nil
}

func (e *EasyPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	attempts := e.refundAttempts(req)
	if len(attempts) == 0 {
		return nil, fmt.Errorf("easypay refund missing order identifier")
	}
	var firstErr error
	for i, attempt := range attempts {
		body, status, err := e.postRaw(ctx, e.apiBase()+"/api.php?act=refund", attempt.params)
		if err != nil {
			return nil, fmt.Errorf("easypay refund request: %w", err)
		}
		if err := parseEasyPayRefundResponse(status, body); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if i+1 < len(attempts) && isEasyPayRefundOrderNotFound(err) {
				continue
			}
			return nil, err
		}
		return &payment.RefundResponse{RefundID: attempt.refundID, Status: payment.ProviderStatusSuccess}, nil
	}
	return nil, firstErr
}

type easyPayRefundAttempt struct {
	params   map[string]string
	refundID string
}

func (e *EasyPay) refundAttempts(req payment.RefundRequest) []easyPayRefundAttempt {
	base := map[string]string{
		"pid": e.config["pid"], "key": e.config["pkey"], "money": req.Amount,
	}
	var attempts []easyPayRefundAttempt
	if orderID := strings.TrimSpace(req.OrderID); orderID != "" {
		params := cloneStringMap(base)
		params["out_trade_no"] = orderID
		attempts = append(attempts, easyPayRefundAttempt{params: params, refundID: orderID})
	}
	if tradeNo := strings.TrimSpace(req.TradeNo); tradeNo != "" {
		params := cloneStringMap(base)
		params["trade_no"] = tradeNo
		attempts = append(attempts, easyPayRefundAttempt{params: params, refundID: tradeNo})
	}
	return attempts
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isEasyPayRefundOrderNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "订单编号不存在") ||
		strings.Contains(msg, "订单不存在") ||
		strings.Contains(lower, "order not found") ||
		strings.Contains(lower, "not exist")
}

func parseEasyPayRefundResponse(status int, body []byte) error {
	summary := summarizeEasyPayResponse(body)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return fmt.Errorf("easypay refund HTTP %d: %s", status, summary)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Errorf("easypay refund empty response (HTTP %d): %s", status, summary)
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		(strings.HasPrefix(lower, "<") && strings.Contains(lower, "html")) {
		return fmt.Errorf("easypay refund non-JSON response (HTTP %d): %s", status, summary)
	}

	var resp struct {
		Code any    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("easypay refund non-JSON response (HTTP %d): %s", status, summary)
	}
	if !easyPayResponseCodeIsSuccess(resp.Code) {
		msg := strings.TrimSpace(resp.Msg)
		if msg == "" {
			msg = summary
		}
		return fmt.Errorf("easypay refund failed (HTTP %d): %s", status, msg)
	}
	return nil
}

func easyPayResponseCodeIsSuccess(code any) bool {
	switch v := code.(type) {
	case float64:
		return int(v) == easypayCodeSuccess
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return err == nil && n == easypayCodeSuccess
	default:
		return false
	}
}

func summarizeEasyPayResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > maxEasypayErrorSummary {
		truncated := summary[:maxEasypayErrorSummary]
		for len(truncated) > 0 && !utf8.ValidString(truncated) {
			truncated = truncated[:len(truncated)-1]
		}
		return truncated + "..."
	}
	return summary
}

func (e *EasyPay) resolveCID(paymentType string) string {
	if strings.HasPrefix(paymentType, "alipay") {
		if v := e.config["cidAlipay"]; v != "" {
			return v
		}
		return e.config["cid"]
	}
	if v := e.config["cidWxpay"]; v != "" {
		return v
	}
	return e.config["cid"]
}

func (e *EasyPay) post(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	body, _, err := e.postRaw(ctx, endpoint, params)
	return body, err
}

func (e *EasyPay) postRaw(ctx context.Context, endpoint string, params map[string]string) ([]byte, int, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return e.doRaw(req)
}

func (e *EasyPay) getRaw(ctx context.Context, endpoint string, params map[string]string) ([]byte, int, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, 0, err
	}
	query := requestURL.Query()
	for k, v := range params {
		query.Set(k, v)
	}
	requestURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	return e.doRaw(req)
}

func (e *EasyPay) doRaw(req *http.Request) ([]byte, int, error) {
	client := e.httpClient
	if client == nil {
		client = &http.Client{Timeout: easypayHTTPTimeout}
	}
	if req.Method == http.MethodGet {
		// Query credentials are in the URL. Never follow redirects (including
		// same-host redirects), which can forward the key via URL or Referer.
		// Copy the client so concurrent calls and other operations are unaffected.
		queryClient := *client
		queryClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &queryClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, sanitizeEasyPayTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEasypayResponseSize+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(body) > maxEasypayResponseSize {
		return nil, resp.StatusCode, fmt.Errorf("easypay response exceeds size limit")
	}
	return body, resp.StatusCode, nil
}

func sanitizeEasyPayTransportError(err error) error {
	if requestErr, ok := err.(*url.Error); ok {
		return fmt.Errorf("%s request failed: %v", requestErr.Op, requestErr.Err)
	}
	return err
}

func easyPaySign(params map[string]string, pkey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			_ = buf.WriteByte('&')
		}
		_, _ = buf.WriteString(k + "=" + params[k])
	}
	_, _ = buf.WriteString(pkey)
	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

func easyPayVerifySign(params map[string]string, pkey string, sign string) bool {
	return hmac.Equal([]byte(easyPaySign(params, pkey)), []byte(sign))
}
