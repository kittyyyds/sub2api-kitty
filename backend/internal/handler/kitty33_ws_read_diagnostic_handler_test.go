//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The producer is exercised by the real service reader tests. Here its immutable
// context value enters Kitty's actual monitoring middleware and queue, without
// introducing an HTTP -> WS production dispatch or a test hook in the program.
func TestKitty33WSReadDiagnosticKeepsOpsOutcome(t *testing.T) {
	for _, mode := range []string{"http_error", "stream_error", "recovered", "success", "skip", "cancel", "native_ws", "request_scoped", "later_turn"} {
		t.Run(mode, func(t *testing.T) {
			var rows []*service.OpsInsertErrorLogInput
			for _, diagnostic := range []bool{false, true} {
				setupOpsErrorLogTestQueue(t, 8)
				gin.SetMode(gin.TestMode)
				ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				router := gin.New()
				router.Use(OpsErrorLoggerMiddleware(ops))
				router.POST("/v1/responses", func(c *gin.Context) {
					service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)
					if diagnostic {
						c.Set("openai_ws_read_diagnostic", &service.OpsUpstreamErrorEvent{
							AtUnixMs: 1, AccountID: 7, AccountName: "synthetic-reader", Platform: service.PlatformOpenAI,
							Kind: "stream_read_diagnostic", Stage: "ws_response", Reason: "incomplete_after_progress",
							ReplayBoundary: &service.OpsReplayBoundary{UpstreamProgressObserved: true, ClientOutputStarted: true, UsageSource: service.OpsReplayUsageUpstream},
						})
					}
					if mode == "native_ws" {
						service.SetOpenAIClientTransport(c, service.OpenAIClientTransportWS)
					}
					if mode == "request_scoped" {
						service.MarkOpsRequestScopedError(c, "invalid_request_error")
					}
					if mode == "later_turn" {
						c.Set(service.OpsStreamTurnKey, 1)
					}
					if mode == "skip" {
						c.Set(service.OpsSkipPassthroughKey, true)
					}
					if mode == "cancel" {
						ctx, cancel := context.WithCancel(c.Request.Context())
						cancel()
						c.Request = c.Request.WithContext(ctx)
					}
					if mode == "success" {
						c.JSON(http.StatusOK, gin.H{"status": "completed"})
						return
					}
					c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
						AccountID: 7, UpstreamStatusCode: http.StatusBadGateway, Kind: "upstream_error", Message: "original outcome",
					}})
					if mode == "stream_error" || mode == "later_turn" {
						service.MarkOpsStreamFailure(c, "upstream_error", "server_error", "original outcome", http.StatusBadGateway)
						c.Header("Content-Type", "text/event-stream")
						c.String(http.StatusOK, "data: {\"type\":\"response.failed\"}\n\n")
						return
					}
					status := http.StatusBadGateway
					if mode == "recovered" {
						status = http.StatusOK
					}
					c.JSON(status, gin.H{"error": gin.H{"type": "upstream_error", "message": "original outcome"}})
				})
				router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
				if mode == "skip" || mode == "success" {
					require.Empty(t, opsErrorLogQueue, "diagnostic cannot create a row past the original monitoring decision")
					continue
				}
				require.Len(t, opsErrorLogQueue, 1, "no extra diagnostic row")
				entry := (<-opsErrorLogQueue).entry
				require.NotNil(t, entry.UpstreamErrorsJSON)
				events, err := service.ParseOpsUpstreamErrors(*entry.UpstreamErrorsJSON)
				require.NoError(t, err)
				wantDiagnostic := diagnostic && (mode == "http_error" || mode == "stream_error" || mode == "recovered")
				found := 0
				for _, event := range events {
					if event.Kind == "stream_read_diagnostic" {
						found++
						require.Zero(t, event.UpstreamStatusCode)
						require.NotNil(t, event.ReplayBoundary)
						require.False(t, event.ReplayBoundary.TerminalSeen)
						require.False(t, event.ReplayBoundary.RetryRequested)
					}
				}
				require.Equal(t, map[bool]int{false: 0, true: 1}[wantDiagnostic], found)
				before := *entry.UpstreamErrorsJSON
				require.NoError(t, service.SanitizeOpsUpstreamErrorsForQueue(entry))
				require.Equal(t, before, *entry.UpstreamErrorsJSON)
				copyOfEntry := *entry
				copyOfEntry.CreatedAt, copyOfEntry.UpstreamErrorsJSON = time.Time{}, nil
				rows = append(rows, &copyOfEntry)
			}
			if len(rows) > 0 {
				require.Len(t, rows, 2)
				before, err := json.Marshal(rows[0])
				require.NoError(t, err)
				after, err := json.Marshal(rows[1])
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after), "diagnostic cannot alter existing row classification, status, ownership or usage policy")
			}
		})
	}
}
