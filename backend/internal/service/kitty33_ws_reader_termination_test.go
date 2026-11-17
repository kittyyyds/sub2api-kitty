//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const kitty33ReaderImage = `{"type":"image_generation_call","id":"ig_reader","status":"completed","result":"c3ludGhldGljLWltYWdl","size":"1024x1024"}`
const kitty33ReaderUsage = `{"input_tokens":11,"output_tokens":3,"input_tokens_details":{"cached_tokens":2}}`

type kitty33WSBoundaryWriter struct {
	*httptest.ResponseRecorder
	onImage          func()
	failImage        bool
	failTerminal     bool
	imageAttempts    int
	terminalAttempts int
	partialTerminal  []byte
}

func (w *kitty33WSBoundaryWriter) Write(p []byte) (int, error) {
	if w.failTerminal && strings.Contains(string(p), `"type":"response.failed"`) {
		w.terminalAttempts++
		w.partialTerminal = append([]byte(nil), p[:len(p)/2]...)
		n, _ := w.ResponseRecorder.Write(w.partialTerminal)
		return n, io.ErrClosedPipe
	}
	if strings.Contains(string(p), `"type":"image_generation_call"`) {
		w.imageAttempts++
		if w.failImage {
			return 0, io.ErrClosedPipe
		}
		n, err := w.ResponseRecorder.Write(p)
		if w.onImage != nil {
			w.onImage()
		}
		return n, err
	}
	return w.ResponseRecorder.Write(p)
}

type kitty33WSDeadlineContext struct{ context.Context }

func (c kitty33WSDeadlineContext) Err() error {
	if context.Cause(c.Context) == context.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	return c.Context.Err()
}

// This tests the actual existing WS-to-SSE adapter without changing Kitty's
// HTTP ingress routing. Queue integration and real HTTP dispatch are exercised
// separately by the handler tests; no production test-only hook is introduced.
func TestKitty33WSReaderTerminationAndDiagnostic(t *testing.T) {
	cases := []struct {
		name, output, usage, ending      string
		source                           OpsReplayUsageSource
		stream, diagnostic, clientOutput bool
	}{
		{"image_eof", "image", kitty33ReaderUsage, "eof", OpsReplayUsageUpstream, true, true, true},
		{"image_absent_usage", "image", "", "eof", OpsReplayUsageAbsent, true, true, true},
		{"image_zero_usage", "image", `{"input_tokens":0,"output_tokens":0}`, "eof", OpsReplayUsageUpstream, true, true, true},
		{"image_empty_usage", "image", `{}`, "eof", OpsReplayUsageUpstream, true, true, true},
		{"image_null_usage", "image", `null`, "eof", OpsReplayUsageAbsent, true, true, true},
		{"image_array_usage", "image", `[]`, "eof", OpsReplayUsageAbsent, true, true, true},
		{"image_actual_then_zero", "image", kitty33ReaderUsage, "usage_then_zero", OpsReplayUsageUpstream, true, true, true},
		{"text_eof", "text", "", "eof", OpsReplayUsageAbsent, true, true, true},
		{"usage_only_eof", "", kitty33ReaderUsage, "eof", OpsReplayUsageUpstream, true, true, false},
		{"buffered_image_eof", "image", kitty33ReaderUsage, "eof", OpsReplayUsageUpstream, false, true, false},
		{"buffered_tool_eof", "tool", kitty33ReaderUsage, "eof", OpsReplayUsageUpstream, false, true, false},
		{"terminal_missing_payload", "tool", "", "missing_payload", OpsReplayUsageAbsent, false, true, false},
		{"image_completed", "image", kitty33ReaderUsage, "completed", "", true, false, true},
		{"image_failed", "image", kitty33ReaderUsage, "failed", "", true, false, true},
		{"image_cancel", "image", kitty33ReaderUsage, "cancel", "", true, false, true},
		{"image_deadline", "image", kitty33ReaderUsage, "deadline", "", true, false, true},
		{"image_write_failure", "image", kitty33ReaderUsage, "write_failure", "", true, false, false},
		{"terminal_short_write", "image", kitty33ReaderUsage, "terminal_short_write", OpsReplayUsageUpstream, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var readCtx context.Context = ctx
			var expire func()
			if tc.ending == "deadline" {
				deadlineCtx, finish := context.WithCancelCause(ctx)
				defer finish(nil)
				readCtx = kitty33WSDeadlineContext{deadlineCtx}
				expire = func() { finish(context.DeadlineExceeded) }
			}
			writer := &kitty33WSBoundaryWriter{ResponseRecorder: httptest.NewRecorder(),
				failImage: tc.ending == "write_failure", failTerminal: tc.ending == "terminal_short_write"}
			if tc.ending == "cancel" {
				writer.onImage = cancel
			} else if expire != nil {
				writer.onImage = expire
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(readCtx)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			conn := &openAIWSCaptureConn{}
			if tc.ending != "missing_payload" {
				conn.events = append(conn.events, []byte(`{"type":"response.created","response":{"id":"resp_reader","object":"response","model":"gpt-5.1","status":"in_progress"}}`))
				if tc.usage != "" {
					conn.events = append(conn.events, []byte(`{"type":"response.in_progress","response":{"id":"resp_reader","usage":`+tc.usage+`}}`))
				}
			}
			switch tc.output {
			case "image":
				conn.events = append(conn.events, []byte(`{"type":"response.output_item.done","output_index":0,"item":`+kitty33ReaderImage+`}`))
			case "text":
				conn.events = append(conn.events, []byte(`{"type":"response.output_text.delta","delta":"synthetic-reader-text"}`))
			case "tool":
				conn.events = append(conn.events, []byte(`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_reader","call_id":"call_reader","name":"lookup","arguments":"{}"}}`))
			}
			if tc.ending == "usage_then_zero" {
				conn.events = append(conn.events, []byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":0,"output_tokens":0}}}`))
			}
			if tc.ending == "completed" || tc.ending == "failed" {
				conn.events = append(conn.events, []byte(`{"type":"response.`+tc.ending+`","response":{"id":"resp_reader","model":"gpt-5.1","status":"`+tc.ending+`","output":[],"usage":`+kitty33ReaderUsage+`}}`))
			} else if tc.ending == "missing_payload" {
				conn.events = append(conn.events, []byte(`{"type":"response.completed"}`))
			}
			dialer := &wsSSETestDialer{conn: conn}
			upstream := &httpUpstreamRecorder{}
			svc := wsSSETestService(t, dialer, upstream)
			account := wsSSETestAccount()
			proxyID := int64(41)
			account.ProxyID = &proxyID
			account.Proxy = &Proxy{ID: proxyID, Name: "synthetic proxy", Protocol: "http", Host: "127.0.0.1", Port: 1,
				Username: "do-not-persist-user", Password: "do-not-persist-password"}
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(wsSSETestRequest), &body))
			body["stream"] = tc.stream
			result, err := svc.forwardOpenAIWSV2(readCtx, c, account, body, "", "", "test-oauth-token",
				OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, tc.stream,
				"gpt-5.1", "gpt-5.1", time.Now(), 1, "", nil)
			require.Equal(t, int32(1), dialer.calls.Load())
			require.Len(t, conn.writes, 1, "exactly one logical upstream execution after progress")
			require.Empty(t, upstream.requests, "no HTTP fallback")
			require.NotNil(t, result)
			if tc.ending == "completed" {
				require.NoError(t, err)
				require.True(t, result.SucceededForScheduling())
			} else if tc.ending == "failed" {
				// An authoritative protocol terminal may be returned as a failed
				// result; do not change Kitty's pre-existing Go error contract.
				require.False(t, result.SucceededForScheduling())
			} else {
				require.Error(t, err)
				require.False(t, result.SucceededForScheduling())
			}
			if tc.diagnostic && tc.ending != "missing_payload" && tc.ending != "terminal_short_write" {
				code, _, classified := OpenAIUpstreamStreamReadErrorDetails(err)
				require.True(t, classified, "retain Kitty's existing typed stream-read classification")
				require.Equal(t, OpenAIUpstreamStreamReadErrorCode, code)
			}
			wantTokens := tc.usage == kitty33ReaderUsage
			if wantTokens {
				require.Equal(t, 11, result.Usage.InputTokens)
				require.Equal(t, 3, result.Usage.OutputTokens)
				require.Equal(t, 2, result.Usage.CacheReadInputTokens)
			} else {
				require.Zero(t, result.Usage.InputTokens)
				require.Zero(t, result.Usage.OutputTokens)
			}
			wantImages := 0
			if tc.output == "image" {
				wantImages = 1
			}
			require.Equal(t, wantImages, result.ImageCount)
			wire := writer.Body.String()
			var terminals []gjson.Result
			for _, line := range strings.Split(wire, "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				frame := strings.TrimPrefix(line, "data: ")
				if !json.Valid([]byte(frame)) && tc.ending == "terminal_short_write" {
					require.True(t, strings.HasSuffix(wire, string(writer.partialTerminal)))
					continue
				}
				require.True(t, json.Valid([]byte(frame)))
				event := gjson.Parse(frame)
				if isOpenAIWSTerminalEvent(event.Get("type").String()) {
					terminals = append(terminals, event)
				}
			}
			departed := tc.ending == "cancel" || tc.ending == "deadline" || tc.ending == "write_failure" || tc.ending == "terminal_short_write"
			if departed {
				require.Empty(t, terminals, "failed writes and cancellation do not manufacture a delivered terminal")
				require.False(t, IsResponseCommitted(c))
				require.Empty(t, GetOpsStreamErrors(c))
				if tc.ending == "terminal_short_write" {
					require.Equal(t, 1, writer.terminalAttempts, "no suffix retry after a partial write")
				}
			} else if tc.stream && (tc.output == "image" || tc.ending == "completed" || tc.ending == "failed") {
				require.Len(t, terminals, 1)
				require.Equal(t, "resp_reader", terminals[0].Get("response.id").String())
				if tc.ending != "completed" && tc.ending != "failed" {
					require.Equal(t, "response.failed", terminals[0].Get("type").String())
					require.Equal(t, http.StatusOK, writer.Code, "wire 200 remains factual after SSE output")
					require.True(t, IsResponseCommitted(c))
				}
			}
			if tc.output == "image" && tc.stream {
				require.Equal(t, 1, writer.imageAttempts, "image is not replayed or duplicated")
			}
			entry := &OpsInsertErrorLogInput{ErrorMessage: "existing failure"}
			AttachOpenAIWSReadDiagnostic(c, entry)
			require.Equal(t, tc.diagnostic, entry.upstreamReadDiagnostic != nil)
			if tc.diagnostic {
				boundary := entry.upstreamReadDiagnostic.ReplayBoundary
				require.Equal(t, tc.source, boundary.UsageSource)
				require.Equal(t, tc.clientOutput, boundary.ClientOutputStarted)
				require.True(t, boundary.UpstreamProgressObserved)
				require.Equal(t, tc.ending == "missing_payload", boundary.TerminalSeen, "locally synthesized failure is not an upstream terminal")
				require.False(t, boundary.RetryRequested)
				require.Equal(t, account.ID, entry.upstreamReadDiagnostic.AccountID)
				require.Equal(t, proxyID, *entry.upstreamReadDiagnostic.ProxyID)
				require.NoError(t, SanitizeOpsUpstreamErrorsForQueue(entry))
				before := *entry.UpstreamErrorsJSON
				require.NoError(t, SanitizeOpsUpstreamErrorsForQueue(entry))
				require.Equal(t, before, *entry.UpstreamErrorsJSON)
				for _, private := range []string{"test-oauth-token", "synthetic-reader-text", "c3ludGhldGljLWltYWdl", "resp_reader", "do-not-persist"} {
					require.NotContains(t, before, private)
				}
			}
			if tc.ending == "deadline" {
				require.True(t, errors.Is(readCtx.Err(), context.DeadlineExceeded))
			}
			// Settle the actual adapter result once through the real Kitty usage
			// service. JSON preparation cannot create another billing command.
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			meter := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
				&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			price, groupID := 0.25, int64(33)
			key := &APIKey{ID: 333, GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1, ImagePrice1K: &price}}
			require.NoError(t, meter.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result,
				APIKey: key, User: &User{ID: 334}, Account: account}))
			require.Equal(t, 1, usageRepo.calls)
			require.Equal(t, 1, billingRepo.calls)
			require.Equal(t, wantImages, usageRepo.lastLog.ImageCount)
			require.Equal(t, wantImages, billingRepo.lastCmd.ImageCount)
			require.Equal(t, result.Usage.InputTokens, usageRepo.lastLog.InputTokens+usageRepo.lastLog.CacheReadTokens)
			require.Equal(t, usageRepo.lastLog.RequestID, billingRepo.lastCmd.RequestID)
			if result.RequestID != "" {
				require.Equal(t, result.RequestID, billingRepo.lastCmd.RequestID)
			} else {
				require.True(t, strings.HasPrefix(billingRepo.lastCmd.RequestID, "generated:"), "only the existing local billing identifier is generated when upstream omitted its response ID")
			}
			require.Equal(t, account.ID, billingRepo.lastCmd.AccountID)
			if !wantTokens && wantImages == 0 {
				require.Zero(t, billingRepo.lastCmd.BalanceCost, "no estimated cost for missing usage")
			}
		})
	}
}
