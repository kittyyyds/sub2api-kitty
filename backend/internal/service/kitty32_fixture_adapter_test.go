package service

import (
	"context"
	"encoding/json"
	coderws "github.com/coder/websocket"
)

// Exercise the real frame write in both passthrough and pooled loopback tests.
type seededWSObservedConn struct{ *stagedPassthroughConn }

func (c *seededWSObservedConn) WriteJSON(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, raw)
}
