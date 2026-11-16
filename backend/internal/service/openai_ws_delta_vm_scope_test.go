package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Gateway-inferred WS continuation (delta_context_enabled) is limited to the
// Codex identity lane. VM-bound and VM2-managed accounts (and API-key relays)
// must keep sending the complete input on every turn, without an injected
// previous_response_id, even on a reused socket with delta enabled.
func TestOpenAIWSDeltaContextSkipsVMAndNonCodexLanes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Account)
		delta bool
	}{
		{"codex_oauth", func(*Account) {}, true},
		{"vm_bound", func(a *Account) { a.Extra["openai_vm_bound"] = true }, false},
		{"vm_managed", func(a *Account) { a.Extra[OpenAIVMManagedExtraKey] = true }, false},
		{"api_key", func(a *Account) {
			a.Type = AccountTypeAPIKey
			a.Credentials = map[string]any{"api_key": "sk-test"}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan []byte, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				for turn := 1; ; turn++ {
					_, raw, err := conn.ReadMessage()
					if err != nil {
						return
					}
					requests <- raw
					output := autoContinuationAnswer
					if turn == 1 {
						output = autoContinuationCall
					}
					if conn.WriteMessage(websocket.TextMessage, autoContinuationEvent(fmt.Sprintf("resp_%d", turn), output)) != nil {
						return
					}
				}
			}))
			defer server.Close()
			s, a, httpUpstream := httpBridgeFixture(t, server.URL)
			defer s.openaiWSPool.Close()
			s.cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			tc.setup(a)
			require.True(t, s.isOpenAIWSDeltaContextEnabled())
			require.Equal(t, tc.delta, s.openAIWSDeltaContextApplies(a))

			first, _ := httpBridgeContext("vm-scope")
			_, err := s.Forward(context.Background(), first, a, autoContinuationBody(true))
			require.NoError(t, err)
			<-requests
			second, _ := httpBridgeContext("vm-scope")
			_, err = s.Forward(context.Background(), second, a, autoContinuationBody(true, autoContinuationCall, autoContinuationTool))
			require.NoError(t, err)
			raw := <-requests
			require.Zero(t, httpUpstream.callCount)
			if tc.delta {
				require.Equal(t, "resp_1", gjson.GetBytes(raw, "previous_response_id").String())
				require.Len(t, gjson.GetBytes(raw, "input").Array(), 1)
				return
			}
			require.False(t, second.GetBool("openai_ws_auto_incremental"))
			require.Equal(t, "disabled", second.GetString("openai_ws_auto_incremental_reason"))
			require.False(t, gjson.GetBytes(raw, "previous_response_id").Exists(), "no inferred anchor outside the Codex identity lane")
			require.Len(t, gjson.GetBytes(raw, "input").Array(), 4, "full input is sent unchanged")
			poolValue, ok := s.openaiWSPool.accounts.Load(a.ID)
			if ok {
				accountPool := poolValue.(*openAIWSAccountPool)
				accountPool.mu.Lock()
				for _, conn := range accountPool.conns {
					require.Nil(t, conn.deltaCheckpoint.Load(), "no checkpoint is recorded for this lane")
				}
				accountPool.mu.Unlock()
			}
		})
	}
}
