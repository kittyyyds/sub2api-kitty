package prism

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordedCall struct {
	method  string
	path    string
	headers http.Header
	body    string
}

func fakeResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewRequiresCookieAndTransport(t *testing.T) {
	_, err := New(Config{Do: func(*http.Request) (*http.Response, error) { return nil, nil }})
	require.Error(t, err)
	var prismErr *Error
	require.ErrorAs(t, err, &prismErr)
	require.Equal(t, ErrorUnauthorized, prismErr.Kind)

	_, err = New(Config{Cookie: "session=fixture"})
	require.Error(t, err)
	require.ErrorAs(t, err, &prismErr)
	require.Equal(t, ErrorTransport, prismErr.Kind)
}

func TestRunStartAndStatusUsesCookieAndDoesNotLogBody(t *testing.T) {
	var mu sync.Mutex
	var calls []recordedCall
	transport := func(req *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(req.Body)
		mu.Lock()
		calls = append(calls, recordedCall{method: req.Method, path: req.URL.Path, headers: req.Header.Clone(), body: string(raw)})
		count := len(calls)
		mu.Unlock()
		if count == 1 {
			return fakeResponse(http.StatusOK, `{"request_id":"req-1","turn_state":"state-1","status":"running"}`), nil
		}
		return fakeResponse(http.StatusOK, `{"status":"completed","response":{"payload":{"id":"resp-1","model":"prism-model","output":[{"type":"message","content":[{"type":"output_text","text":"fixture answer"}]}]}}}`), nil
	}

	client, err := New(Config{
		Cookie:       "session=fixture-secret",
		Do:           transport,
		SkipSandbox:  true,
		PollInterval: time.Millisecond,
		PollAttempts: 2,
	})
	require.NoError(t, err)
	result, err := client.Run(context.Background(), Request{
		Model: "prism-model",
		Input: []InputMessage{{Role: "user", Content: []ContentPart{{Type: "input_text", Text: "hello"}}}},
	})
	require.NoError(t, err)
	require.Equal(t, "resp-1", result.ResponseID)
	require.Equal(t, "fixture answer", result.Text)
	require.Equal(t, "prism-model", result.Model)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, calls, 2)
	require.Equal(t, "/api/llm/response_with_tools_start", calls[0].path)
	require.Equal(t, "/api/llm/response_with_tools_status", calls[1].path)
	for _, call := range calls {
		require.Equal(t, "session=fixture-secret", call.headers.Get("Cookie"))
		require.NotContains(t, call.body, "session=fixture-secret")
	}
}

func TestRunClassifiesUnauthorizedWithoutEchoingUpstreamBody(t *testing.T) {
	const secret = "cookie-secret-that-must-not-leak"
	client, err := New(Config{
		Cookie: secret,
		Do: func(*http.Request) (*http.Response, error) {
			return fakeResponse(http.StatusUnauthorized, `{"error":"`+secret+`"}`), nil
		},
		SkipSandbox: true,
	})
	require.NoError(t, err)
	_, err = client.Run(context.Background(), Request{Model: "prism-model", Input: []InputMessage{{Role: "user"}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
	var prismErr *Error
	require.ErrorAs(t, err, &prismErr)
	require.Equal(t, ErrorUnauthorized, prismErr.Kind)
	require.Equal(t, http.StatusUnauthorized, prismErr.Status)
}

func TestRunRejectsInvalidRequestBeforeTransport(t *testing.T) {
	called := false
	client, err := New(Config{Cookie: "fixture", Do: func(*http.Request) (*http.Response, error) {
		called = true
		return fakeResponse(http.StatusOK, `{}`), nil
	}, SkipSandbox: true})
	require.NoError(t, err)
	_, err = client.Run(context.Background(), Request{})
	require.Error(t, err)
	require.False(t, called)
}

func TestRunTimesOutPolling(t *testing.T) {
	client, err := New(Config{
		Cookie: "fixture",
		Do: func(*http.Request) (*http.Response, error) {
			return fakeResponse(http.StatusOK, `{"request_id":"req","status":"running"}`), nil
		},
		SkipSandbox:  true,
		RequestTTL:   5 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		PollAttempts: 2,
	})
	require.NoError(t, err)
	_, err = client.Run(context.Background(), Request{Model: "prism-model", Input: []InputMessage{{Role: "user"}}})
	require.Error(t, err)
	var prismErr *Error
	require.ErrorAs(t, err, &prismErr)
	require.Equal(t, ErrorTimeout, prismErr.Kind)
	require.True(t, prismErr.RequestStarted)
}

func TestNewRejectsUnexpectedCredentialDestinations(t *testing.T) {
	for _, baseURL := range []string{"http://prism.openai.com", "https://attacker.invalid", "https://prism.openai.com@attacker.invalid", "https://prism.openai.com:443", "https://prism.openai.com/path"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := New(Config{BaseURL: baseURL, Cookie: "fixture", Do: func(*http.Request) (*http.Response, error) {
				t.Fatal("configuration validation must precede transport")
				return nil, nil
			}})
			require.Error(t, err)
		})
	}
	_, err := New(Config{Cookie: "fixture", ProjectID: "../another-project", Do: func(*http.Request) (*http.Response, error) { return nil, nil }})
	require.Error(t, err)
}

func textRequest() Request {
	return Request{Model: "requested-model", Input: []InputMessage{{Role: "user", Content: []ContentPart{{Type: "input_text", Text: "hello"}}}}}
}

const completedFixture = `{"status":"completed","response":{"payload":{"id":"resp-fixture","output":[{"type":"message","content":[{"type":"output_text","text":"fixture answer"}]}]}}}`

func TestFullBootstrapUsesUUIDQueriesAndNonJSONAcknowledgements(t *testing.T) {
	var projectID string
	var requests []string
	var starts int
	client, err := New(Config{
		Cookie:       "session=fixture",
		PollInterval: time.Millisecond,
		Do: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "prism.openai.com", req.URL.Host)
			require.Equal(t, "https", req.URL.Scheme)
			require.Equal(t, "session=fixture", req.Header.Get("Cookie"))
			require.NotContains(t, req.URL.Path, "?")
			requests = append(requests, req.Method+" "+req.URL.Path)
			body, readErr := io.ReadAll(req.Body)
			require.NoError(t, readErr)
			require.NotContains(t, string(body), "session=fixture")
			var payload map[string]any
			if len(body) > 0 {
				require.NoError(t, json.Unmarshal(body, &payload))
			}
			if strings.HasPrefix(req.URL.Path, "/s/sandboxes/proxy/") {
				require.Equal(t, "sandbox-fixture", req.Header.Get("X-Crixet-Sandbox-Token"))
				require.NotEmpty(t, req.URL.Query().Get("prism_cache_bust"))
			}
			switch req.URL.Path {
			case "/api/projects":
				projectID = payload["project_uuid"].(string)
				require.True(t, validUUID(projectID))
				require.Equal(t, byte('4'), projectID[14])
				return fakeResponse(201, ""), nil
			case "/api/y":
				require.Equal(t, projectID, payload["docId"])
				return fakeResponse(200, `{"token":"ys-fixture","url":"wss://prism.openai.com/y/doc"}`), nil
			case "/api/backend/1/new":
				return fakeResponse(200, `{"token":"sandbox-fixture"}`), nil
			case "/s/sandboxes/proxy/heartbeat":
				return fakeResponse(200, "pong"), nil
			case "/api/projects/" + projectID + "/sandbox/resources-token":
				require.Equal(t, "sandbox-fixture", payload["sandbox_token"])
				require.Nil(t, payload["sandbox_session_id"])
				return fakeResponse(200, `{"access_token":"resources-fixture"}`), nil
			case "/s/sandboxes/proxy/resources-token":
				require.Equal(t, "resources-fixture", payload["token"])
				require.Equal(t, DefaultBaseURL+"/y/d/"+projectID, payload["baseUrl"])
				return fakeResponse(204, ""), nil
			case "/s/sandboxes/proxy/token":
				require.Equal(t, "ys-fixture", payload["token"])
				require.Equal(t, projectID, payload["docId"])
				return fakeResponse(200, "ok"), nil
			case "/s/sandboxes/proxy/wait-for-sync":
				require.Equal(t, "10000", req.URL.Query().Get("wait_ms"))
				return fakeResponse(200, "synced"), nil
			case "/s/sandboxes/proxy/render":
				require.Equal(t, "async", req.URL.Query().Get("renderMode"))
				require.Equal(t, "main.tex", payload["mainDocument"])
				return fakeResponse(200, `{"statusPath":"render-status?waitMs=321&renderResultMode=stream-v1"}`), nil
			case "/s/sandboxes/proxy/render-status":
				require.Equal(t, "321", req.URL.Query().Get("waitMs"))
				return fakeResponse(200, "rendered"), nil
			case "/api/llm/response_with_tools_start":
				starts++
				input := payload["input"].([]any)
				require.Equal(t, "message", input[0].(map[string]any)["type"])
				metadata := payload["metadata"].(map[string]any)
				require.Equal(t, projectID, metadata["projectId"])
				require.Equal(t, "requested-model", metadata["model"])
				return fakeResponse(200, `{"request_id":"request-fixture","turn_state":"state-fixture","status":"running"}`), nil
			case "/api/llm/response_with_tools_status":
				require.Equal(t, "request-fixture", payload["request_id"])
				require.Equal(t, "state-fixture", payload["turn_state"])
				return fakeResponse(200, completedFixture), nil
			default:
				t.Fatalf("unexpected endpoint %s", req.URL.Path)
				return nil, nil
			}
		},
	})
	require.NoError(t, err)
	result, err := client.Run(context.Background(), textRequest())
	require.NoError(t, err)
	require.Equal(t, "fixture answer", result.Text)
	require.Empty(t, result.Model, "missing upstream model must not be filled from request")
	require.Nil(t, result.Usage)
	require.Equal(t, "unknown", result.UsageSource)
	require.Equal(t, []string{
		"POST /api/projects", "POST /api/y", "POST /api/backend/1/new", "GET /s/sandboxes/proxy/heartbeat",
		"POST /api/projects/" + projectID + "/sandbox/resources-token", "POST /s/sandboxes/proxy/resources-token",
		"POST /s/sandboxes/proxy/token", "GET /s/sandboxes/proxy/wait-for-sync", "POST /s/sandboxes/proxy/render",
		"GET /s/sandboxes/proxy/render-status", "POST /api/llm/response_with_tools_start", "POST /api/llm/response_with_tools_status",
	}, requests)
	require.Equal(t, 1, starts)
	require.Equal(t, projectID, client.Sandbox().ProjectID)
	_, err = client.Run(context.Background(), textRequest())
	require.NoError(t, err)
	require.Len(t, requests, 14, "cached sandbox must not be bootstrapped twice")
}

func TestUnexpectedYSweetURLStopsBeforeSendingTokens(t *testing.T) {
	calls := 0
	client, err := New(Config{Cookie: "fixture", ProjectID: "12345678-1234-4123-8123-123456789abc", Do: func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/api/y", req.URL.Path)
		return fakeResponse(200, `{"token":"secret-fixture","url":"wss://attacker.invalid/doc"}`), nil
	}})
	require.NoError(t, err)
	_, err = client.Run(context.Background(), textRequest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-fixture")
	require.Equal(t, 1, calls)
}

func TestUsageSourcesNeverTreatCumulativeTotalsAsRequestUsage(t *testing.T) {
	cases := []struct {
		name         string
		payloadUsage string
		progress     string
		source       string
		input        int
	}{
		{"empty", `{}`, "", "unknown", 0},
		{"missing", "", "", "unknown", 0},
		{"cumulative only", "", `{"total_token_usage":{"input_tokens":10000,"output_tokens":9000,"total_tokens":19000}}`, "unknown", 0},
		{"last turn observation", "", `{"total_token_usage":{"input_tokens":10000,"output_tokens":9000},"last_token_usage":{"input_tokens":11,"output_tokens":3}}`, "last_token_usage", 11},
		{"payload wins", `{"input_tokens":20,"output_tokens":6,"total_tokens":26,"input_tokens_details":{"cached_tokens":5},"output_tokens_details":{"reasoning_tokens":2}}`, `{"last_token_usage":{"input_tokens":11,"output_tokens":3}}`, "payload", 20},
		{"negative usage", `{"input_tokens":-20,"output_tokens":6}`, "", "unknown", 0},
		{"fractional usage", `{"input_tokens":1.1,"output_tokens":6}`, "", "unknown", 0},
		{"inconsistent usage", `{"input_tokens":20,"output_tokens":6,"total_tokens":99}`, "", "unknown", 0},
		{"cached exceeds input", `{"input_tokens":20,"output_tokens":6,"cached_input_tokens":21}`, "", "unknown", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(completedFixture), &body))
			payload, _ := nestedMap(body, "response", "payload")
			if tc.payloadUsage != "" {
				var usage map[string]any
				require.NoError(t, json.Unmarshal([]byte(tc.payloadUsage), &usage))
				payload["usage"] = usage
			}
			if tc.progress != "" {
				body["codex_live_progress"] = map[string]any{"eventPreviews": []any{map[string]any{"payload_type": "token_count", "raw": `{"payload":{"info":` + tc.progress + `}}`}}}
			}
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			client, err := New(Config{Cookie: "fixture", SkipSandbox: true, Do: func(*http.Request) (*http.Response, error) { return fakeResponse(200, string(encoded)), nil }})
			require.NoError(t, err)
			result, err := client.Run(context.Background(), textRequest())
			if tc.payloadUsage != "" && tc.source == "unknown" {
				require.Error(t, err, "an explicitly malformed payload usage must not be accepted")
				require.Nil(t, result.Usage)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.source, result.UsageSource)
			if tc.source == "unknown" {
				require.Nil(t, result.Usage)
			} else {
				require.NotNil(t, result.Usage)
				require.Equal(t, tc.input, result.Usage.InputTokens)
			}
		})
	}
}

func TestStartUnknownOutcomeIsNotRepeated(t *testing.T) {
	for _, failure := range []string{"transport", "malformed", "poll timeout"} {
		t.Run(failure, func(t *testing.T) {
			starts := 0
			client, err := New(Config{Cookie: "fixture", SkipSandbox: true, PollInterval: time.Millisecond, PollAttempts: 1, Do: func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "_start") {
					starts++
				}
				switch failure {
				case "transport":
					return nil, errors.New("secret transport detail must be hidden")
				case "malformed":
					return fakeResponse(200, "bad json"), nil
				default:
					return fakeResponse(200, `{"request_id":"request-fixture","status":"running"}`), nil
				}
			}})
			require.NoError(t, err)
			_, err = client.Run(context.Background(), textRequest())
			require.Error(t, err)
			var e *Error
			require.ErrorAs(t, err, &e)
			require.True(t, e.RequestStarted)
			require.NotContains(t, err.Error(), "secret transport detail")
			client.InvalidateSandbox()
			_, err = client.Run(context.Background(), textRequest())
			require.ErrorAs(t, err, &e)
			require.Equal(t, ErrorOutcomeUnknown, e.Kind)
			require.Equal(t, 1, starts, "resetting cached sandbox must not implicitly restart unknown turn")
		})
	}
}

func TestConcurrentRunWaitsAndCancelledWaiterNeverStarts(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	client, err := New(Config{Cookie: "fixture", SkipSandbox: true, Do: func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return fakeResponse(200, completedFixture), nil
	}})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, e := client.Run(context.Background(), textRequest()); done <- e }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = client.Run(ctx, textRequest())
	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, "queue", e.Op)
	require.False(t, e.RequestStarted)
	close(release)
	require.NoError(t, <-done)
}

func TestBootstrapRejectsStructuredFailureWithoutLeakingResponse(t *testing.T) {
	for _, response := range []string{`{"error":{"message":"secret fixture error"}}`, `{"status":"failed"}`, `{"status":"pending"}`} {
		client, err := New(Config{Cookie: "fixture", Do: func(*http.Request) (*http.Response, error) { return fakeResponse(200, response), nil }})
		require.NoError(t, err)
		_, err = client.bootstrap(context.Background(), http.MethodGet, "/s/sandboxes/proxy/wait-for-sync", nil, nil, "sync")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret fixture error")
	}
}

func TestTransportRefusesAbsoluteOrNetworkPaths(t *testing.T) {
	client, err := New(Config{Cookie: "fixture", Do: func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe paths must never reach transport")
		return nil, nil
	}})
	require.NoError(t, err)
	for _, requestPath := range []string{"https://attacker.invalid/", "//attacker.invalid/", "relative", "/valid#fragment"} {
		_, err = client.exchange(context.Background(), http.MethodGet, requestPath, nil, nil, "fixture")
		require.Error(t, err)
	}
}

func TestPayloadPreservesTextAndReasoningWhitespace(t *testing.T) {
	response := `{"status":"completed","response":{"payload":{"id":"response-fixture","model":"reported-model","status":"completed","usage":{"input_tokens":10,"output_tokens":4,"cached_input_tokens":0},"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"  First\n"},{"type":"summary_text","text":"second  "}]},{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"  code:\n"},{"type":"output_text","text":"    line one\n"},{"type":"output_text","text":"    line two  \n"}]}]}}}`
	client, err := New(Config{Cookie: "fixture", SkipSandbox: true, Do: func(*http.Request) (*http.Response, error) { return fakeResponse(200, response), nil }})
	require.NoError(t, err)
	result, err := client.Run(context.Background(), textRequest())
	require.NoError(t, err)
	require.Equal(t, "  code:\n    line one\n    line two  \n", result.Text)
	require.Equal(t, "  First\nsecond  ", result.ReasoningText)
	require.Equal(t, "reported-model", result.Model)
	require.Equal(t, "payload", result.UsageSource)
}

func TestCompletedEnvelopeCannotHidePartialFailureOrUnsupportedOutput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"payload incomplete", func(p map[string]any) { p["status"] = "incomplete" }},
		{"payload failed", func(p map[string]any) { p["status"] = "failed" }},
		{"payload cancelled", func(p map[string]any) { p["status"] = "cancelled" }},
		{"payload error", func(p map[string]any) { p["error"] = map[string]any{"message": "private fixture detail"} }},
		{"incomplete details", func(p map[string]any) { p["incomplete_details"] = map[string]any{"reason": "max_output_tokens"} }},
		{"partial message", func(p map[string]any) { p["output"].([]any)[0].(map[string]any)["status"] = "in_progress" }},
		{"non assistant message", func(p map[string]any) { p["output"].([]any)[0].(map[string]any)["role"] = "user" }},
		{"function call after text", func(p map[string]any) {
			p["output"] = append(p["output"].([]any), map[string]any{"type": "function_call", "name": "fixture_function"})
		}},
		{"custom tool after text", func(p map[string]any) {
			p["output"] = append(p["output"].([]any), map[string]any{"type": "custom_tool_call", "name": "fixture_tool"})
		}},
		{"hosted tool after text", func(p map[string]any) {
			p["output"] = append(p["output"].([]any), map[string]any{"type": "web_search_call", "status": "completed"})
		}},
		{"unknown item", func(p map[string]any) {
			p["output"] = append(p["output"].([]any), map[string]any{"type": "future_output_type"})
		}},
		{"malformed item", func(p map[string]any) { p["output"] = append(p["output"].([]any), "unexpected-string") }},
		{"image content", func(p map[string]any) {
			p["output"].([]any)[0].(map[string]any)["content"] = []any{map[string]any{"type": "output_image", "data": "fixture"}}
		}},
		{"refusal content", func(p map[string]any) {
			p["output"].([]any)[0].(map[string]any)["content"] = []any{map[string]any{"type": "refusal", "refusal": "fixture"}}
		}},
		{"malformed text", func(p map[string]any) {
			p["output"].([]any)[0].(map[string]any)["content"] = []any{map[string]any{"type": "output_text", "text": 123}}
		}},
		{"invalid usage", func(p map[string]any) { p["usage"] = map[string]any{"input_tokens": 1} }},
		{"missing cache metadata", func(p map[string]any) { p["usage"] = map[string]any{"input_tokens": 10, "output_tokens": 2} }},
		{"null cache metadata", func(p map[string]any) {
			p["usage"] = map[string]any{"input_tokens": 10, "output_tokens": 2, "cached_input_tokens": nil}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(completedFixture), &body))
			payload, _ := nestedMap(body, "response", "payload")
			payload["usage"] = map[string]any{"input_tokens": 10, "output_tokens": 2, "cached_input_tokens": 0}
			tc.mutate(payload)
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			client, err := New(Config{Cookie: "fixture", SkipSandbox: true, Do: func(*http.Request) (*http.Response, error) { return fakeResponse(200, string(encoded)), nil }})
			require.NoError(t, err)
			result, err := client.Run(context.Background(), textRequest())
			var protocolErr *Error
			require.ErrorAs(t, err, &protocolErr)
			require.Equal(t, ErrorProtocol, protocolErr.Kind)
			require.True(t, protocolErr.RequestStarted)
			require.Empty(t, result.Text, "partial text must not escape a rejected completion")
			require.Nil(t, result.Usage, "rejected completion must not produce a billable result")
			require.NotContains(t, err.Error(), "private fixture detail")
		})
	}
}
