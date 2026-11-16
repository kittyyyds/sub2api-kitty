package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestEasyPayQueryOrderStatusMapping(t *testing.T) {
	t.Parallel()

	const orderID = "order-123"
	tests := []struct {
		name        string
		body        string
		wantStatus  string
		wantTradeNo string
		wantAmount  float64
		wantError   bool
	}{
		{
			name:        "matching top level paid statuses",
			body:        `{"code":1,"pid":"pid-1","out_trade_no":"order-123","trade_status":"TRADE_SUCCESS","status":1,"money":"12.34","trade_no":"gateway-123"}`,
			wantStatus:  payment.ProviderStatusPaid,
			wantTradeNo: "gateway-123",
			wantAmount:  12.34,
		},
		{
			name:      "waiting trade status with paid numeric status is rejected",
			body:      `{"code":1,"trade_status":"WAITING","status":1,"money":"12.34","trade_no":"gateway-123"}`,
			wantError: true,
		},
		{
			name:      "empty trade status with paid numeric status is rejected",
			body:      `{"code":1,"trade_status":"","status":1,"money":"12.34"}`,
			wantError: true,
		},
		{
			name:        "matching nested paid statuses",
			body:        `{"code":1,"data":{"pid":"pid-1","out_trade_no":"order-123","trade_status":"TRADE_SUCCESS","status":1,"money":"9.99","trade_no":"data-456"}}`,
			wantStatus:  payment.ProviderStatusPaid,
			wantTradeNo: "data-456",
			wantAmount:  9.99,
		},
		{
			name:        "legacy numeric paid status remains compatible",
			body:        `{"code":1,"pid":"pid-1","out_trade_no":"order-123","status":1,"money":"3.21","trade_no":"gateway-123"}`,
			wantStatus:  payment.ProviderStatusPaid,
			wantTradeNo: "gateway-123",
			wantAmount:  3.21,
		},
		{
			name:        "legacy numeric non paid status is pending",
			body:        `{"code":1,"status":0,"money":"3.21"}`,
			wantStatus:  payment.ProviderStatusPending,
			wantTradeNo: "",
			wantAmount:  3.21,
		},
		{
			name:      "query failure is an error",
			body:      `{"code":0,"msg":"订单不存在"}`,
			wantError: true,
		},
		{
			name:      "missing fields are an error",
			body:      `{}`,
			wantError: true,
		},
		{
			name:       "pending response does not synthesize identity or trade number",
			body:       `{"code":1,"status":0}`,
			wantStatus: payment.ProviderStatusPending,
		},
		{
			name:      "nested paid trade status conflicts with top level unpaid status",
			body:      `{"code":"1","status":"0","data":{"pid":"pid-1","out_trade_no":"order-123","trade_status":"TRADE_SUCCESS","money":"12.34","trade_no":"gateway-123"}}`,
			wantError: true,
		},
		{
			name:        "all four paid statuses agree with string code and status",
			body:        `{"code":"1","status":"1","trade_status":"TRADE_SUCCESS","data":{"pid":"pid-1","out_trade_no":"order-123","status":1,"trade_status":"TRADE_SUCCESS","money":"12.34","trade_no":"gateway-123"}}`,
			wantStatus:  payment.ProviderStatusPaid,
			wantTradeNo: "gateway-123",
			wantAmount:  12.34,
		},
		{
			name:       "all four unpaid statuses agree",
			body:       `{"code":1,"status":0,"trade_status":"WAITING","data":{"status":"0","trade_status":"WAITING"}}`,
			wantStatus: payment.ProviderStatusPending,
		},
		{
			name:        "matching envelope and data identities",
			body:        `{"code":1,"status":1,"pid":"pid-1","out_trade_no":"order-123","trade_no":"gateway-123","money":"12.30","data":{"pid":"pid-1","out_trade_no":"order-123","trade_no":"gateway-123","money":"12.3"}}`,
			wantStatus:  payment.ProviderStatusPaid,
			wantTradeNo: "gateway-123",
			wantAmount:  12.30,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotQuery url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %q, want %q", r.Method, http.MethodGet)
				}
				if r.URL.Path != "/api.php" {
					t.Errorf("path = %q, want /api.php", r.URL.Path)
				}
				gotQuery = make(url.Values, len(r.URL.Query()))
				for key, values := range r.URL.Query() {
					gotQuery[key] = append([]string(nil), values...)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := newTestEasyPay(t, server.URL)
			resp, err := provider.QueryOrder(context.Background(), orderID)
			if tt.wantError {
				if err == nil || resp != nil {
					t.Fatalf("expected query failure, got response=%+v err=%v", resp, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryOrder returned error: %v", err)
			}
			if resp.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q (response=%+v)", resp.Status, tt.wantStatus, resp)
			}
			if resp.TradeNo != tt.wantTradeNo {
				t.Fatalf("trade_no = %q, want %q", resp.TradeNo, tt.wantTradeNo)
			}
			if resp.Amount != tt.wantAmount {
				t.Fatalf("amount = %v, want %v", resp.Amount, tt.wantAmount)
			}
			if resp.Status == payment.ProviderStatusPaid {
				if resp.Metadata["pid"] != "pid-1" || resp.Metadata["out_trade_no"] != orderID || resp.Metadata["money"] == "" {
					t.Fatalf("missing verified query metadata: %+v", resp.Metadata)
				}
			} else if !strings.Contains(tt.body, `"pid"`) && resp.Metadata["pid"] != "" {
				t.Fatalf("query synthesized merchant identity: %+v", resp.Metadata)
			}
			for key, want := range map[string]string{
				"act":          "order",
				"pid":          "pid-1",
				"key":          "pkey-1",
				"out_trade_no": orderID,
			} {
				if got := gotQuery.Get(key); got != want {
					t.Fatalf("query[%s] = %q, want %q (query=%v)", key, got, want, gotQuery)
				}
			}
		})
	}
}

func TestEasyPayQueryOrderRetriesTransientHTTPFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"pid":"pid-1","out_trade_no":"order-retry","status":1,"money":"10.00","trade_no":"gateway-retry"}`))
	}))
	defer server.Close()

	provider := newTestEasyPay(t, server.URL)
	resp, err := provider.QueryOrder(context.Background(), "order-retry")
	if err != nil {
		t.Fatalf("QueryOrder returned error: %v", err)
	}
	if resp.Status != payment.ProviderStatusPaid {
		t.Fatalf("status = %q, want %q", resp.Status, payment.ProviderStatusPaid)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestEasyPayQueryOrderNumericMerchantAndExactMoney(t *testing.T) {
	t.Parallel()
	// Larger than the exact integer range of float64: preserve the original pid.
	const pid = "9007199254740993"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"status":1,"pid":` + pid + `,"out_trade_no":"order-123","trade_no":"gateway-123","money":"0012.30","buyer":"","param":"","addtime":"2026-10-07"}`))
	}))
	defer server.Close()
	provider := newTestEasyPay(t, server.URL)
	provider.config["pid"] = pid
	resp, err := provider.QueryOrder(context.Background(), "order-123")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != payment.ProviderStatusPaid || resp.Amount != 12.30 || resp.Metadata["pid"] != pid ||
		resp.Metadata["out_trade_no"] != "order-123" || resp.Metadata["money"] != "0012.30" {
		t.Fatalf("incorrect verified query result: %+v", resp)
	}
}

func TestEasyPayQueryOrderRejectsConflictingStatuses(t *testing.T) {
	t.Parallel()
	fields := []struct {
		key    string
		nested bool
	}{
		{"trade_status", false}, {"status", false},
		{"trade_status", true}, {"status", true},
	}
	// Cover both directions for every pair of the four possible status fields.
	// A populated paid order must not hide conflicting evidence anywhere else.
	for paidIndex, paidField := range fields {
		for unpaidIndex, unpaidField := range fields {
			if paidIndex == unpaidIndex {
				continue
			}
			t.Run(fmt.Sprintf("paid_%d_unpaid_%d", paidIndex, unpaidIndex), func(t *testing.T) {
				t.Parallel()
				data := map[string]any{}
				resp := map[string]any{
					"code": 1, "pid": "pid-1", "out_trade_no": "order-123",
					"trade_no": "gateway-123", "money": "12.34", "data": data,
				}
				for _, entry := range []struct {
					key    string
					nested bool
					paid   bool
				}{
					{paidField.key, paidField.nested, true},
					{unpaidField.key, unpaidField.nested, false},
				} {
					var value any = "WAITING"
					if entry.key == "status" {
						value = 0
						if entry.paid {
							value = 1
						}
					} else if entry.paid {
						value = tradeStatusSuccess
					}
					if entry.nested {
						data[entry.key] = value
					} else {
						resp[entry.key] = value
					}
				}
				body, err := json.Marshal(resp)
				if err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write(body)
				}))
				defer server.Close()
				provider := newTestEasyPay(t, server.URL)
				result, err := provider.QueryOrder(context.Background(), "order-123")
				if err == nil || result != nil || !strings.Contains(err.Error(), "conflicting payment status") {
					t.Fatalf("conflicting statuses were not rejected: response=%+v err=%v", result, err)
				}
			})
		}
	}
}

func TestEasyPayQueryOrderDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, sameHost := range []bool{false, true} {
			t.Run(fmt.Sprintf("status_%d_same_host_%t", status, sameHost), func(t *testing.T) {
				t.Parallel()
				var gatewayCalls, redirectedCalls, customRedirectCalls atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					redirectedCalls.Add(1)
					_, _ = w.Write([]byte(`{"code":1,"status":1,"pid":"pid-1","out_trade_no":"order-123","trade_no":"gateway-123","money":"12.34"}`))
				}))
				defer target.Close()
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api.php" {
						redirectedCalls.Add(1)
						w.WriteHeader(http.StatusOK)
						return
					}
					gatewayCalls.Add(1)
					location := target.URL + "/capture?" + r.URL.RawQuery
					if sameHost {
						location = "/capture?" + r.URL.RawQuery
					}
					w.Header().Set("Location", location)
					w.WriteHeader(status)
				}))
				defer gateway.Close()
				provider := newTestEasyPay(t, gateway.URL)
				provider.httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
					customRedirectCalls.Add(1)
					return nil
				}
				result, err := provider.QueryOrder(context.Background(), "order-123")
				if err == nil || result != nil {
					t.Fatal("redirected query was accepted")
				}
				if gatewayCalls.Load() != 1 || redirectedCalls.Load() != 0 || customRedirectCalls.Load() != 0 {
					t.Fatalf("query followed or retried redirect: gateway=%d target=%d callback=%d", gatewayCalls.Load(), redirectedCalls.Load(), customRedirectCalls.Load())
				}
				if strings.Contains(err.Error(), provider.config["pkey"]) || strings.Contains(err.Error(), "capture") {
					t.Fatal("redirect failure exposed sensitive URL")
				}
				// The shared client must keep its original policy for other calls.
				if provider.httpClient.CheckRedirect(nil, nil) != nil || customRedirectCalls.Load() != 1 {
					t.Fatal("query mutated the shared HTTP client")
				}
			})
		}
	}
}

func TestEasyPayQueryOrderRejectsMalformedResponses(t *testing.T) {
	t.Parallel()
	const valid = `{"code":1,"status":1,"pid":"pid-1","out_trade_no":"order-123","trade_no":"gateway-123","money":"12.34"}`
	cases := map[string]string{
		"invalid JSON":          `{"code":1,`,
		"array":                 `[]`,
		"null":                  `null`,
		"trailing JSON":         valid + `{}`,
		"trailing garbage":      valid + `private-callback-data`,
		"duplicate code":        strings.Replace(valid, `"code":1`, `"code":0,"code":1`, 1),
		"duplicate status":      strings.Replace(valid, `"status":1`, `"status":0,"status":1`, 1),
		"escaped duplicate pid": strings.Replace(valid, `"pid":"pid-1"`, `"p\u0069d":"other","pid":"pid-1"`, 1),
		"nested duplicate":      `{"code":1,"data":` + strings.Replace(valid, `"money":"12.34"`, `"money":"0.01","money":"12.34"`, 1) + `}`,
		"null data":             `{"code":1,"data":null}`,
		"array data":            `{"code":1,"data":[]}`,
		"missing status":        `{"code":1}`,
		"upstream message":      `{"code":0,"status":1,"msg":"pkey-1 private-callback-data"}`,
		"null trade status":     strings.TrimSuffix(valid, "}") + `,"trade_status":null}`,
		"invalid trade status":  strings.TrimSuffix(valid, "}") + `,"trade_status":"TRADE_SUCCESS\n"}`,
		"non string status":     strings.TrimSuffix(valid, "}") + `,"trade_status":true}`,
		// A truncated read must not accept a valid JSON prefix and silently drop
		// trailing content beyond the response-size limit.
		"oversized JSON prefix": valid + strings.Repeat(" ", maxEasypayResponseSize-len(valid)) + `private-callback-data`,
	}
	for _, key := range []string{"code", "pid", "out_trade_no", "trade_no", "money", "status"} {
		for _, replacement := range []string{"", "null", `""`, `" "`, `true`, `{}`, `[]`} {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(valid), &fields); err != nil {
				t.Fatal(err)
			}
			if replacement == "" {
				delete(fields, key)
			} else {
				fields[key] = json.RawMessage(replacement)
			}
			body, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			cases[key+" replaced by "+replacement] = string(body)
		}
	}
	for key, replacements := range map[string][]string{
		"code":         {`0`, `-1`, `2`, `1.9`, `1.0000000000000001`, `1e0`, `"01"`, `"+1"`, `" 1 "`},
		"pid":          {`"other-merchant"`, `"pid-1 "`, `"pid-1&key=private"`, `1.1`, `1e3`},
		"out_trade_no": {`"wrong-order"`, `"order-123\n"`, `"order-123&x=y"`, `123`},
		"trade_no":     {`"gateway 123"`, `"gateway\u0000"`, `"gateway&x=y"`, `123`},
		"money":        {`"NaN"`, `"Inf"`, `"-1"`, `"0"`, `"0.00"`, `"1e2"`, `"+1"`, `".10"`, `"1."`, `"1.234"`, `"12.34 "`, `"9999999999999999999999.99"`, `12.34`},
		"status":       {`2`, `-1`, `1.1`, `"paid"`},
	} {
		for _, replacement := range replacements {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(valid), &fields); err != nil {
				t.Fatal(err)
			}
			fields[key] = json.RawMessage(replacement)
			body, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			cases[key+" invalid "+replacement] = string(body)
		}
	}
	for _, key := range []string{"pid", "out_trade_no", "trade_no", "money"} {
		value := `"different"`
		if key == "money" {
			value = `"0.01"`
		}
		cases["conflicting nested "+key] = strings.TrimSuffix(valid, "}") + fmt.Sprintf(`,"data":{%q:%s}}`, key, value)
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			provider := newTestEasyPay(t, server.URL)
			resp, err := provider.QueryOrder(context.Background(), "order-123")
			if err == nil || resp != nil {
				t.Fatalf("malformed query accepted: response=%+v err=%v", resp, err)
			}
			if strings.Contains(err.Error(), body) || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), provider.config["pkey"]) {
				t.Fatal("query error exposed response or secret")
			}
			if calls.Load() != 1 {
				t.Fatalf("malformed response was retried: %d calls", calls.Load())
			}
		})
	}
}

func TestEasyPayQueryOrderRejectsInvalidOrderBeforeRequest(t *testing.T) {
	t.Parallel()
	provider := newTestEasyPay(t, "https://gateway.invalid")
	var calls atomic.Int32
	provider.httpClient = &http.Client{Transport: easyPayQueryRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, fmt.Errorf("unexpected request")
	})}
	for _, orderID := range []string{"", " ", "order-123 ", "order-123&pid=other", "order\x00", "order\n123"} {
		resp, err := provider.QueryOrder(context.Background(), orderID)
		if err == nil || resp != nil {
			t.Fatal("invalid requested order was accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("query sent an invalid order to the gateway")
	}
}

type easyPayQueryRoundTripFunc func(*http.Request) (*http.Response, error)

func (f easyPayQueryRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestEasyPayQueryOrderDoesNotExposeTransportSecrets(t *testing.T) {
	t.Parallel()
	provider := newTestEasyPay(t, "https://gateway.invalid")
	provider.httpClient = &http.Client{Transport: easyPayQueryRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("failed to request %s; private-callback-data", req.URL.String())
	})}
	resp, err := provider.QueryOrder(context.Background(), "order-123")
	if err == nil || resp != nil {
		t.Fatal("expected transport failure")
	}
	for _, secret := range []string{provider.config["pkey"], "private-callback-data", "gateway.invalid", "order-123"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("query error exposed transport details")
		}
	}
}
