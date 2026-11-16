package service

import (
	"errors"
	"net/http"

	coderws "github.com/coder/websocket"
)

// GatewayPromptWSCloseStatus keeps input failures distinct from transient faults.
func GatewayPromptWSCloseStatus(err error) coderws.StatusCode {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return coderws.StatusMessageTooBig
	}
	if errors.Is(err, ErrGatewayPromptInvalidRequest) {
		return coderws.StatusPolicyViolation
	}
	return coderws.StatusInternalError
}

func GatewayPromptWSCloseReason(err error) string {
	switch GatewayPromptWSCloseStatus(err) {
	case coderws.StatusMessageTooBig:
		return "Request body exceeds 32 MiB prompt limit; compact or start a new session"
	case coderws.StatusPolicyViolation:
		return "Invalid JSON request object; fix the request before retrying"
	default:
		return "Prompt plugin temporarily unavailable"
	}
}
