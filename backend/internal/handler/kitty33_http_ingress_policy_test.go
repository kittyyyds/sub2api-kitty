//go:build unit

package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// The author's new public-handler fixtures assume HTTP -> WS. Kitty deliberately
// keeps HTTP ingress on HTTP. Exercise that real entry policy separately from
// the existing adapter's reader/termination contract in the service tests.
func TestKitty33HTTPIngressPreservesNativeTransport(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "buffered", true: "stream"}[stream], func(t *testing.T) {
			upstream := &oauthReliabilityModelHandlerUpstream{
				status: http.StatusOK, body: "data: " + oauthReliabilityHandlerModelSuccess + "\n\n",
			}
			h, usage := oauthReliabilityModelHandler(t, upstream, false, true)
			c, recorder := oauthReliabilityModelRequest(t, "responses", stream, context.Background())
			h.Responses(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "model available on the next credential")
			accounts, models := upstream.calls()
			require.Equal(t, []int64{1}, accounts, "one selected HTTP execution")
			require.Equal(t, []string{"gpt-5.6-sol"}, models)
			require.Equal(t, "http_sse", c.GetString("openai_ws_transport_decision"))
			require.Equal(t, "client_protocol_http", c.GetString("openai_ws_transport_reason"))
			require.Len(t, usage.created, 1)
			require.False(t, (<-usage.created).OpenAIWSMode)
			_, present := c.Get("openai_ws_read_diagnostic")
			require.False(t, present, "native HTTP does not invent a WS reader observation")
		})
	}
}
