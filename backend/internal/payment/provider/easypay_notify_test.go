package provider

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func easyPayTestNotification() map[string]string {
	return map[string]string{
		"pid": "pid-1", "trade_no": "gateway-123", "out_trade_no": "order-123",
		"type": "alipay", "name": "VIP会员", "money": "12.34",
		"trade_status": tradeStatusSuccess, "sign_type": signTypeMD5,
	}
}

func encodeEasyPayTestNotification(params map[string]string, key string) string {
	params["sign"] = easyPaySign(params, key)
	values := make(url.Values, len(params))
	for name, value := range params {
		values.Set(name, value)
	}
	return values.Encode()
}

func TestEasyPayNotificationProtocolCompatibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, money, subject, param, paymentType, tradeStatus string
		wantAmount                                            float64
	}{
		{"document integer amount", "1", "VIP会员", "", "alipay", tradeStatusSuccess, 1},
		{"one decimal and custom type", "12.3", "VIP + 会员", "opaque-ref", "usdt_trc20", tradeStatusSuccess, 12.3},
		{"punctuation and leading zeroes", "0012.30", "会员=年度 R&D", `{"customer":"会员","id":12}`, "wxpay", tradeStatusSuccess, 12.3},
		{"literal percent encoding is decoded once", "0.01", "%26trade_status%3DTRADE_SUCCESS", "a+b%26c", "alipay", tradeStatusSuccess, 0.01},
		{"non success status", "12.34", "VIP会员", "", "alipay", "TRADE_CLOSED", 12.34},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := newTestEasyPay(t, "https://gateway.invalid")
			params := easyPayTestNotification()
			params["money"], params["name"], params["param"] = tc.money, tc.subject, tc.param
			params["type"], params["trade_status"] = tc.paymentType, tc.tradeStatus
			raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
			notification, err := provider.VerifyNotification(context.Background(), raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := payment.ProviderStatusFailed
			if tc.tradeStatus == tradeStatusSuccess {
				wantStatus = payment.ProviderStatusSuccess
			}
			if notification.TradeNo != params["trade_no"] || notification.OrderID != params["out_trade_no"] ||
				notification.Status != wantStatus || notification.Amount != tc.wantAmount || notification.RawData != raw {
				t.Fatal("notification fields did not match signed values")
			}
			if notification.Metadata["pid"] != params["pid"] || notification.Metadata["out_trade_no"] != params["out_trade_no"] ||
				notification.Metadata["money"] != tc.money {
				t.Fatalf("incorrect verified metadata: %+v", notification.Metadata)
			}
		})
	}
}

func TestEasyPayNotificationRequiresPaymentIdentityAndSignature(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"pid", "trade_no", "out_trade_no", "type", "money", "trade_status", "sign"} {
		for _, missing := range []bool{true, false} {
			name := field + " empty"
			if missing {
				name = field + " missing"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				provider := newTestEasyPay(t, "https://gateway.invalid")
				params := easyPayTestNotification()
				if missing {
					delete(params, field)
				} else {
					params[field] = ""
				}
				raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
				if field == "sign" {
					values, _ := url.ParseQuery(raw)
					if missing {
						values.Del("sign")
					} else {
						values.Set("sign", "")
					}
					raw = values.Encode()
				}
				assertEasyPayNotificationRejected(t, provider, raw)
			})
		}
	}
}

func TestEasyPayNotificationOptionalDisplayFieldsKeepSignatureChecks(t *testing.T) {
	t.Parallel()
	for _, fields := range [][]string{{"name"}, {"sign_type"}, {"name", "sign_type"}} {
		for _, omit := range []bool{false, true} {
			t.Run(strings.Join(fields, "+")+map[bool]string{true: " omitted", false: " empty"}[omit], func(t *testing.T) {
				provider := newTestEasyPay(t, "https://gateway.invalid")
				params := easyPayTestNotification()
				for _, field := range fields {
					if omit {
						delete(params, field)
					} else {
						params[field] = ""
					}
				}
				raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
				n, err := provider.VerifyNotification(context.Background(), raw, nil)
				if err != nil || n == nil || n.Amount != 12.34 || n.Status != payment.ProviderStatusSuccess {
					t.Fatalf("compatible signed notification failed: %v", err)
				}
				values, _ := url.ParseQuery(raw)
				values.Set("money", "0.01")
				assertEasyPayNotificationRejected(t, provider, values.Encode())
				assertEasyPayNotificationRejected(t, provider, encodeEasyPayTestNotification(params, "wrong-key"))
			})
		}
	}
}

func TestEasyPayMissingFieldDiagnosticDoesNotLeakValues(t *testing.T) {
	provider := newTestEasyPay(t, "https://gateway.invalid")
	params := easyPayTestNotification()
	delete(params, "trade_no")
	params["name"] = "private-subject"
	raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
	_, err := provider.VerifyNotification(context.Background(), raw, nil)
	if err == nil || err.Error() != "easypay notify: missing required field (trade_no)" {
		t.Fatalf("expected field-name-only diagnostic, got %v", err)
	}
}

func TestEasyPayNotificationRejectsInvalidSignedFields(t *testing.T) {
	t.Parallel()
	for field, values := range map[string][]string{
		"pid":          {"other-merchant", "pid-1 ", "pid-1&x=y"},
		"trade_no":     {"gateway 123", "gateway=123", "gateway\x00", "gateway\n"},
		"out_trade_no": {"order 123", "order-123&pid=pid-1", "order-123\r"},
		"type":         {" alipay", "alipay&trade_status=TRADE_SUCCESS", "alipay=wxpay"},
		"trade_status": {"TRADE_SUCCESS ", " TRADE_SUCCESS", "TRADE_SUCCESS&x=y"},
		"name":         {" ", "VIP\n会员", "VIP\x00会员", string([]byte{0xff}), "VIP&notify_url=https://merchant.invalid/notify"},
		"param":        {"ref\x00", "ref&pid=pid-1", "a&b&c=d"},
		"money":        {"NaN", "Inf", "-Inf", "-1.00", "0", "0.00", "+1.00", "1e2", ".10", "1.", "1.001", "1..2", "1,00", " 1.00", "1.00 ", "１.００", "999999999999999999999999.99", "90071992547409.92", "90071992547409.90"},
		"sign_type":    {"RSA", "SHA256", "MD5 ", "\tMD5"},
		"notify_url":   {"https://merchant.invalid/notify", ""},
		"return_url":   {"https://merchant.invalid/return"},
		"clientip":     {"127.0.0.1"},
		"device":       {"mobile"},
		"cid":          {"1"},
		"unknown":      {""},
		"SIGN":         {"ignored-signature"},
	} {
		for _, value := range values {
			t.Run(field+" "+value, func(t *testing.T) {
				t.Parallel()
				provider := newTestEasyPay(t, "https://gateway.invalid")
				params := easyPayTestNotification()
				params[field] = value
				assertEasyPayNotificationRejected(t, provider, encodeEasyPayTestNotification(params, provider.config["pkey"]))
			})
		}
	}
}

func TestEasyPayNotificationRejectsDuplicateAndMalformedForm(t *testing.T) {
	t.Parallel()
	provider := newTestEasyPay(t, "https://gateway.invalid")
	params := easyPayTestNotification()
	params["param"] = ""
	raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
	for key, value := range params {
		t.Run("duplicate "+key, func(t *testing.T) {
			assertEasyPayNotificationRejected(t, provider, raw+"&"+url.QueryEscape(key)+"="+url.QueryEscape(value))
		})
	}
	for _, suffix := range []string{"&%70id=pid-1", "&pid=other", "&param=%ZZ", "&param=%", "&param=semi;colon", "&private-callback-field=value"} {
		assertEasyPayNotificationRejected(t, provider, raw+suffix)
	}
	assertEasyPayNotificationRejected(t, provider, "name="+strings.Repeat("x", maxEasypayResponseSize))
}

func TestEasyPayNotificationSignatureValidation(t *testing.T) {
	t.Parallel()
	provider := newTestEasyPay(t, "https://gateway.invalid")
	params := easyPayTestNotification()
	raw := encodeEasyPayTestNotification(params, provider.config["pkey"])
	values, _ := url.ParseQuery(raw)
	values.Set("sign", strings.ToUpper(params["sign"]))
	values.Set("sign_type", "md5")
	if _, err := provider.VerifyNotification(context.Background(), values.Encode(), nil); err != nil {
		t.Fatalf("case-insensitive MD5 hex rejected: %v", err)
	}
	for _, sign := range []string{"short-signature", strings.Repeat("g", 32), strings.Repeat("0", 32), params["sign"] + " ", easyPaySign(params, "wrong-key")} {
		values.Set("sign", sign)
		assertEasyPayNotificationRejected(t, provider, values.Encode())
	}
	values.Set("sign", params["sign"])
	values.Set("money", "0.01")
	assertEasyPayNotificationRejected(t, provider, values.Encode())
}

func TestEasyPayNotificationRejectsAmbiguousSignedPaymentRequest(t *testing.T) {
	t.Parallel()
	provider := newTestEasyPay(t, "https://gateway.invalid")
	// This is a real locally generated checkout signature, with no payment made.
	// MD5 canonicalization does not escape '&' in the return URL. Splitting its
	// value creates signed-looking payment-result fields without knowing the key.
	created, err := provider.createRedirectPayment(payment.CreatePaymentRequest{
		OrderID: "order-123", PaymentType: "alipay", Amount: "12.34", Subject: "VIP会员",
		ReturnURL: "https://merchant.invalid/return&trade_no=gateway-forged&trade_status=TRADE_SUCCESS",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := url.Parse(created.PayURL)
	if err != nil {
		t.Fatal(err)
	}
	forged := checkout.Query()
	forged.Set("return_url", "https://merchant.invalid/return")
	forged.Set("trade_no", "gateway-forged")
	forged.Set("trade_status", tradeStatusSuccess)
	assertSignatureMatches := func(values url.Values) {
		t.Helper()
		params := make(map[string]string, len(values))
		for key := range values {
			params[key] = values.Get(key)
		}
		if !easyPayVerifySign(params, provider.config["pkey"], params["sign"]) {
			t.Fatal("regression fixture must reuse a valid checkout signature")
		}
	}
	assertSignatureMatches(forged)
	assertEasyPayNotificationRejected(t, provider, forged.Encode())

	// Hiding request-only keys inside allowed values also preserves the same
	// signature. A key allowlist alone is therefore insufficient.
	forged.Set("name", forged.Get("name")+"&notify_url="+forged.Get("notify_url"))
	forged.Del("notify_url")
	forged.Set("pid", forged.Get("pid")+"&return_url="+forged.Get("return_url"))
	forged.Del("return_url")
	assertSignatureMatches(forged)
	assertEasyPayNotificationRejected(t, provider, forged.Encode())
}

func assertEasyPayNotificationRejected(t *testing.T, provider *EasyPay, raw string) {
	t.Helper()
	notification, err := provider.VerifyNotification(context.Background(), raw, nil)
	if err == nil || notification != nil {
		t.Fatal("invalid notification was accepted")
	}
	values, _ := url.ParseQuery(raw)
	for _, secret := range []string{raw, provider.config["pkey"], values.Get("sign"), "private-callback-field"} {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatal("notification error exposed callback data or secrets")
		}
	}
}
