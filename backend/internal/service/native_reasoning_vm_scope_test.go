package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type nativeReplayCountingStore struct {
	inner *nativeReplayTestStore
	calls int
}

func (s *nativeReplayCountingStore) GetNativeReasoningReplay(ctx context.Context, owner string, keys []string, maxBytes int) (map[string]NativeReasoningReplayValue, error) {
	s.calls++
	return s.inner.GetNativeReasoningReplay(ctx, owner, keys, maxBytes)
}

func (s *nativeReplayCountingStore) PutNativeReasoningReplay(ctx context.Context, value NativeReasoningReplayWrite, limits NativeReasoningReplayLimits) error {
	s.calls++
	return s.inner.PutNativeReasoningReplay(ctx, value, limits)
}

func (s *nativeReplayCountingStore) InvalidateNativeReasoningReplay(ctx context.Context, owner string, refs []NativeReasoningReplayReference) error {
	s.calls++
	return s.inner.InvalidateNativeReasoningReplay(ctx, owner, refs)
}

// Native reasoning replay belongs to the Codex identity lane only. VM-bound and
// VM2-managed accounts reach upstream through the vm2api bridge, and Excel
// credentials are not a Codex CLI identity: with replay switched on for the
// user, those requests neither touch the replay store nor change the bytes
// sent upstream compared with a gateway that has no replay runtime at all.
func TestNativeReasoningReplayExcludesVMAndExcelAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := codexSynthEnabledConfig()
	cfg.JWT.Secret = "synthetic-native-replay-vm-scope"
	replayOn := nativeReplayPolicyReaderFunc(func(context.Context) (NativeReasoningReplayRuntimeConfig, error) {
		return NativeReasoningReplayRuntimeConfig{Mode: NativeReasoningReplayModeReplay, UserIDs: []int64{101}}, nil
	})
	newContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Request.Header.Set("session_id", "vm-scope-session")
		c.Set("api_key", &APIKey{ID: 202, UserID: 101})
		return c, rec
	}
	body := nativeCaptureJSON(map[string]any{"model": "gpt-6-astra", "messages": []any{map[string]any{"role": "user", "content": "Synthetic VM scope."}}, "stream": false})

	vmProxyID := int64(77)
	vmAccount := func(managed bool) *Account {
		account := newCodexSynthTestAccount(9911, AccountTypeOAuth)
		account.Extra["openai_vm_bound"] = true
		if managed {
			account.Extra[OpenAIVMManagedExtraKey] = true
		}
		account.ProxyID = &vmProxyID
		account.Proxy = &Proxy{ID: vmProxyID, Protocol: "http", Host: "127.0.0.1", Port: 1, Username: "9911", Password: "vm-secret", Status: StatusActive}
		return account
	}
	excel := newCodexSynthTestAccount(9912, AccountTypeOAuth)
	excel.Credentials["client_id"] = openai.ExcelClientID

	// The gate itself: no request-scoped replay state for these accounts.
	for name, account := range map[string]*Account{"vm_bound": vmAccount(false), "vm_managed": vmAccount(true), "excel_oauth": excel} {
		store := &nativeReplayCountingStore{inner: newNativeReplayTestStore()}
		svc := &OpenAIGatewayService{cfg: cfg}
		svc.SetNativeReasoningReplayRuntime(replayOn, NewNativeReasoningReplay(store, newNativeReasoningReplayCipher(cfg), DefaultNativeReasoningReplayLimits()))
		c, _ := newContext()
		authorization := svc.resolveNativeReasoningChatAuthorization(c, "gpt-6-astra")
		require.Nil(t, svc.beginNativeReasoningChat(c, account, body, authorization), name)
	}
	// Positive control: the same request on a direct Codex OAuth account is eligible.
	{
		svc := &OpenAIGatewayService{cfg: cfg}
		svc.SetNativeReasoningReplayRuntime(replayOn, NewNativeReasoningReplay(newNativeReplayTestStore(), newNativeReasoningReplayCipher(cfg), DefaultNativeReasoningReplayLimits()))
		c, _ := newContext()
		authorization := svc.resolveNativeReasoningChatAuthorization(c, "gpt-6-astra")
		require.NotNil(t, svc.beginNativeReasoningChat(c, newCodexSynthTestAccount(9913, AccountTypeOAuth), body, authorization))
	}

	// End to end through the production Chat bridge for VM routes.
	for _, managed := range []bool{false, true} {
		forward := func(withRuntime bool) ([]byte, *nativeReplayCountingStore) {
			wire, _ := nativeCaptureResponseFixture()
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(wire))}}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			store := &nativeReplayCountingStore{inner: newNativeReplayTestStore()}
			if withRuntime {
				svc.SetNativeReasoningReplayRuntime(replayOn, NewNativeReasoningReplay(store, newNativeReasoningReplayCipher(cfg), DefaultNativeReasoningReplayLimits()))
			}
			c, _ := newContext()
			_, err := svc.ForwardAsChatCompletions(context.Background(), c, vmAccount(managed), body, "", "")
			require.NoError(t, err)
			require.Len(t, upstream.bodies, 1)
			return upstream.bodies[0], store
		}
		plain, _ := forward(false)
		replayed, store := forward(true)
		require.Equal(t, string(plain), string(replayed), "VM route body is unchanged by the replay runtime (managed=%v)", managed)
		require.Zero(t, store.calls, "VM route never reads or writes replay state (managed=%v)", managed)
	}
}
