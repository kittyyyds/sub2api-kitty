package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestManagedModelManifestComesFromTheSlot(t *testing.T) {
	var seen []http.Header
	svc, _, _, accounts, direct := managedRequestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET /v1/accounts/7/models", r.Method+" "+r.URL.Path)
		seen = append(seen, r.Header.Clone())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.5","object":"model"},{"id":"gpt-5.4","object":"model"}],"source":"model-policy"}`)
	})
	manifest, err := svc.FetchCodexModelsManifest(context.Background(), managedAccount(accounts), "0.137.0", "")
	require.NoError(t, err)
	var body struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(manifest.Body, &body), "the slot's plain list is turned into a Codex manifest")
	var slugs []string
	for _, model := range body.Models {
		slugs = append(slugs, model.Slug)
	}
	require.ElementsMatch(t, []string{"gpt-5.5", "gpt-5.4"}, slugs)
	require.Len(t, seen, 1)
	require.Equal(t, "Bearer "+vmTestSecret, seen[0].Get("Authorization"))
	require.Empty(t, seen[0].Get("Chatgpt-Account-Id"))
	require.Empty(t, direct.requests, "the manifest must not be fetched from OpenAI with an empty token")
}

func TestManagedModelManifestFailureDoesNotDisableTheAccount(t *testing.T) {
	svc, _, _, accounts, _ := managedRequestFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"detail":"Unauthorized"}`)
	})
	_, err := svc.FetchCodexModelsManifest(context.Background(), managedAccount(accounts), "0.137.0", "")
	require.Error(t, err)
	state := accounts.snapshot()
	require.Empty(t, accounts.errors)
	require.Equal(t, StatusActive, state.Status)
	// The model list comes from the bridge, not from the account's login: a
	// failure there says nothing about the account and does not pause it.
	require.Empty(t, accounts.pauses)
}

func TestManagedBridgeOutagePausesBrieflyButUnsupportedRequestsDoNot(t *testing.T) {
	svc, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) {})
	account := managedAccount(accounts)

	err := svc.handleOpenAIUpstreamTransportError(context.Background(), nil, account, errors.New("OpenAI VM bridge transport failed"), true)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover, "another account may take the request")
	require.Equal(t, []string{openAIVMManagedPausePrefix + "bridge_unreachable"}, accounts.pauses)
	state := accounts.snapshot()
	require.WithinDuration(t, time.Now().Add(openAIVMManagedBridgePause), *state.TempUnschedulableUntil, 5*time.Second)
	require.Empty(t, accounts.errors)

	// A request this account type cannot serve says nothing about its health.
	_ = svc.handleOpenAIUpstreamTransportError(context.Background(), nil, account, errOpenAIVMManagedUnsupported, true)
	require.Len(t, accounts.pauses, 1)

	// An ordinary account is not paused by this rule.
	plainRepo := &managedTestAccounts{a: &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}}
	plainSvc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: plainRepo}
	plain, _ := plainRepo.GetByID(context.Background(), 9)
	_ = plainSvc.handleOpenAIUpstreamTransportError(context.Background(), nil, plain, errors.New("temporary blip"), true)
	require.Empty(t, plainRepo.pauses)
}

func TestManagedAccountIsOutsideEveryTokenWorkflow(t *testing.T) {
	_, _, _, accounts, _ := managedRequestFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	account := managedAccount(accounts)

	// Re-authorization writes a fresh credential into Sub2API.
	reauth := &OpenAIOAuthReauthService{accounts: managedReauthAccounts{accounts}}
	_, err := reauth.accountFor(context.Background(), 7)
	require.ErrorIs(t, err, errOpenAIVMManagedNoToken)

	// Reset credits are spent by a person from the managed panel, never automatically.
	account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	accounts.mu.Lock()
	accounts.a.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	accounts.mu.Unlock()
	autoReset := &OpenAIQuotaAutoResetService{accountRepo: accounts}
	require.NoError(t, autoReset.evaluateAccount(context.Background(), 7))

	// A failed account test must not park the account in status "error".
	tests := &AccountTestService{accountRepo: accounts}
	require.NoError(t, tests.setAccountTestError(context.Background(), 7, "Authentication failed (401)"))
	require.Empty(t, accounts.errors)
	plainRepo := &managedTestAccounts{a: &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}}
	require.NoError(t, (&AccountTestService{accountRepo: plainRepo}).setAccountTestError(context.Background(), 9, "Authentication failed (401)"))
	require.Len(t, plainRepo.errors, 1, "ordinary accounts keep the old behaviour")

	released := managedAccount(accounts)
	delete(released.Extra, "openai_vm_bound")
	require.False(t, ipv6Eligible(released), "a managed account already has a fixed exit")
	require.True(t, ipv6Eligible(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
}

type managedReauthAccounts struct{ accounts *managedTestAccounts }

func (r managedReauthAccounts) GetAccount(ctx context.Context, id int64) (*Account, error) {
	return r.accounts.GetByID(ctx, id)
}
func (managedReauthAccounts) GetProxy(context.Context, int64) (*Proxy, error) { return nil, nil }

func TestManagedDeleteFromTheAccountListGoesThroughTheSlot(t *testing.T) {
	f := newManagedFixture(t)
	f.create(t)
	previous := openAIVMManagedDeleter
	t.Cleanup(func() { openAIVMManagedDeleter = previous })
	openAIVMManagedDeleter = f.svc.Delete

	admin := &adminServiceImpl{accountRepo: f.accounts}
	require.NoError(t, admin.DeleteAccount(context.Background(), 7))
	requests, _ := f.bridge.seen()
	require.Equal(t, "DELETE /v1/accounts/7", requests[len(requests)-1], "the slot is destroyed, not orphaned")
	require.Equal(t, 1, f.repo.released)
	require.Equal(t, 1, f.admin.deletes)

	// With the real admin service on both ends: the slot is destroyed once, the
	// row is deleted once, and the two services do not call each other in circles.
	r := newManagedFixture(t)
	r.create(t)
	real := &adminServiceImpl{accountRepo: r.accounts}
	r.svc.admin = real
	openAIVMManagedDeleter = r.svc.Delete
	require.NoError(t, real.DeleteAccount(context.Background(), 7))
	requests, _ = r.bridge.seen()
	deletes := 0
	for _, route := range requests {
		if route == "DELETE /v1/accounts/7" {
			deletes++
		}
	}
	require.Equal(t, 1, deletes)
	require.Equal(t, 1, r.repo.released)
	require.Equal(t, 1, r.accounts.deletes)

	// While the slot cannot be destroyed, the account must stay.
	g := newManagedFixture(t)
	g.create(t)
	openAIVMManagedDeleter = g.svc.Delete
	g.bridge.fail["DELETE /v1/accounts/7"] = http.StatusConflict
	require.Error(t, (&adminServiceImpl{accountRepo: g.accounts}).DeleteAccount(context.Background(), 7))
	require.Zero(t, g.admin.deletes)
	require.False(t, g.accounts.deleted)
	require.True(t, strings.HasPrefix(g.repo.b.Status, "delete_failed"))
}
