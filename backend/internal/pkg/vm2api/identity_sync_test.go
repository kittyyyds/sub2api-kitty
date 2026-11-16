package vm2api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSyncAccessTokenUsesDedicatedEndpointAndNeverImportFallback(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/panel/vms/slot-a/bridge-identity" {
			t.Errorf("unexpected identity route: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 2 || body["access_token"] != "at-current" || body["chatgpt_account_id"] != "chatgpt-a" {
			t.Error("identity payload changed scope")
		}
		http.NotFound(w, r)
	})
	_, err := c.SyncAccessToken(context.Background(), "slot-a", AccessTokenCredential{AccessToken: "at-current", ChatGPTAccountID: "chatgpt-a"})
	if !IsNotFound(err) || calls != 1 {
		t.Fatalf("unsupported sync must fail without fallback: calls=%d err=%v", calls, err)
	}
}
