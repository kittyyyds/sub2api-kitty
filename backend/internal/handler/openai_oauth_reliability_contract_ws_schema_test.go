//go:build unit

package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Kitty keeps HTTP ingress on its existing HTTP transport. Native WS and
// prewarm rejection/settlement are exercised by the real WS handler and
// service contracts, rather than changing routing to satisfy a fork fixture.
func TestKitty32HTTPStrictSchemaPreservesIngressPolicy(t *testing.T) {
	for _, route := range []string{"responses", "passthrough", "chat", "messages"} {
		for _, stream := range []bool{false, true} {
			t.Run(route+map[bool]string{false: "/json", true: "/stream"}[stream], func(t *testing.T) {
				u := &oauthReplayOpsUpstream{wire: `data: {"type":"response.failed","response":{"id":"resp_schema_http","status":"failed","error":{"type":"invalid_request_error","code":"invalid_json_schema","param":"text.format.schema","message":"uniqueItems is unsupported in this strict schema"}}}` + "\n\n"}
				h, usage := oauthReliabilityModelHandler(t, u, route == "passthrough", false)
				c, rec := oauthReliabilityModelRequest(t, route, stream, context.Background())
				switch route {
				case "chat":
					h.ChatCompletions(c)
				case "messages":
					h.Messages(c)
				default:
					h.Responses(c)
				}
				require.Equal(t, []int64{1}, u.accountIDs, "deterministic schema failure must not switch account")
				for len(usage.created) > 0 {
					row := <-usage.created
					require.Zero(t, row.InputTokens)
					require.Zero(t, row.OutputTokens)
					require.Zero(t, row.ActualCost, "an optional zero-usage audit row cannot create a charge")
				}
				require.Contains(t, rec.Body.String(), "invalid_json_schema")
				require.Contains(t, rec.Body.String(), "text.format.schema")
				require.Contains(t, rec.Body.String(), "uniqueItems")
				if rec.Code != http.StatusOK {
					require.Equal(t, http.StatusBadRequest, rec.Code)
				} else {
					require.True(t, stream)
					require.Equal(t, http.StatusBadRequest, c.GetInt(service.OpsUpstreamStatusCodeKey))
				}
				if !strings.Contains(rec.Body.String(), "data:") {
					require.True(t, gjson.Valid(rec.Body.String()))
				}
			})
		}
	}
}
