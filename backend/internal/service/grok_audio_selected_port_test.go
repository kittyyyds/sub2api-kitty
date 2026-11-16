package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// Two real loopback WebSockets exercise frame types and disconnect propagation;
// no third-party endpoint, credential or paid model is used.
func TestGrokSelectedPortVoiceLoopback(t *testing.T) {
	for _, mode := range []string{"realtime", "tts", "tts-empty"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					kind, body, err := conn.Read(ctx)
					if err != nil {
						return
					}
					if err = conn.Write(ctx, kind, body); err != nil {
						return
					}
				}
			}))
			defer echo.Close()
			type result struct {
				chars int64
				audio bool
				err   error
			}
			done := make(chan result, 1)
			relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				client, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- result{err: err}
					return
				}
				defer client.CloseNow()
				up, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(echo.URL, "http"), nil)
				if err != nil {
					done <- result{err: err}
					return
				}
				defer up.CloseNow()
				svc := &OpenAIGatewayService{}
				upstream := &GrokRealtimeUpstream{conn: &coderOpenAIWSClientConn{conn: up}}
				if mode == "realtime" {
					audio, err := svc.ProxyGrokRealtimeConn(ctx, nil, client, upstream)
					done <- result{audio: audio, err: err}
				} else {
					chars, err := svc.ProxyGrokTTSStreamConn(ctx, client, upstream)
					done <- result{chars: chars, err: err}
				}
			}))
			defer relay.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(relay.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			event := []byte(`{"type":"session.update","session":{}}`)
			if mode == "tts" {
				event = []byte(`{"type":"text.delta","delta":"你好🌟a"}`)
			}
			require.NoError(t, client.Write(ctx, coderws.MessageText, event))
			kind, body, err := client.Read(ctx)
			require.NoError(t, err)
			require.Equal(t, coderws.MessageText, kind)
			require.JSONEq(t, string(event), string(body))
			if mode == "realtime" {
				raw := []byte{0, 255, 128, 1, 0, 9}
				require.False(t, json.Valid(raw))
				require.NoError(t, client.Write(ctx, coderws.MessageBinary, raw))
				kind, body, err = client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, coderws.MessageBinary, kind)
				require.Equal(t, raw, body)
			}
			_ = client.CloseNow()
			select {
			case got := <-done:
				require.Error(t, got.err)
				if mode == "realtime" {
					require.True(t, got.audio)
				}
				if mode == "tts" {
					require.Equal(t, int64(4), got.chars)
				}
				if mode == "tts-empty" {
					require.Zero(t, got.chars)
				}
			case <-ctx.Done():
				t.Fatal("voice relay did not release after client disconnect")
			}
		})
	}
}
