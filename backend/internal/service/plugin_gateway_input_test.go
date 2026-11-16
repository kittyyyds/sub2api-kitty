package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestGatewayPromptInputClassification(t *testing.T) {
	m := &PluginManager{}
	m.transformRoute.Store(&pluginRoute{rolloutPercent: 100})
	for _, tc := range []struct {
		name   string
		body   []byte
		status coderws.StatusCode
	}{
		{"oversize", bytes.Repeat([]byte{'a'}, GatewayPromptMaxBody+1), coderws.StatusMessageTooBig},
		{"bad-json", []byte(`{"broken"`), coderws.StatusPolicyViolation},
		{"array", []byte(`[]`), coderws.StatusPolicyViolation},
		{"null", []byte(`null`), coderws.StatusPolicyViolation},
		{"runtime-unavailable", []byte(`{"model":"m","input":"hello"}`), coderws.StatusInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.TransformGatewayRequest(context.Background(), 2, 1, "responses", "m", tc.body)
			require.Error(t, err)
			err = fmt.Errorf("wrapped: %w", err)
			require.Equal(t, tc.status, GatewayPromptWSCloseStatus(err))
			require.LessOrEqual(t, len(GatewayPromptWSCloseReason(err)), 123)
			if tc.status == coderws.StatusPolicyViolation {
				require.ErrorIs(t, err, ErrGatewayPromptInvalidRequest)
			}
			if tc.status == coderws.StatusMessageTooBig {
				var maxErr *http.MaxBytesError
				require.True(t, errors.As(err, &maxErr))
				require.Equal(t, int64(GatewayPromptMaxBody), maxErr.Limit)
			}
		})
	}
	// Duplicate fields are also a client input error, not plugin downtime.
	err := validateGatewayPromptMutation([]byte(`{"model":"m","model":"hidden"}`), []byte(`{"model":"m"}`), "instructions")
	require.ErrorIs(t, err, ErrGatewayPromptInvalidRequest)
}
