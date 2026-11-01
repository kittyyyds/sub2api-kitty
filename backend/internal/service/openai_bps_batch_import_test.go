package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type bpsImportTestAdmin struct {
	mu       sync.Mutex
	saved    *CreateAccountInput
	existing int64
	assigned *int64
	groupErr error
}

func (a *bpsImportTestAdmin) GetGroup(context.Context, int64) (*Group, error) {
	return &Group{Platform: PlatformOpenAI, Status: StatusActive}, nil
}
func (a *bpsImportTestAdmin) ValidateAccountGroupBindings(context.Context, []int64) error {
	return a.groupErr
}
func (a *bpsImportTestAdmin) FindBPSImportAccount(context.Context, string) (int64, error) {
	return a.existing, nil
}
func (a *bpsImportTestAdmin) PrepareAccountImport(_ context.Context, _, _ string, p *int64, _ string) (*AccountImportTicket, error) {
	if a.assigned != nil {
		p = a.assigned
	}
	return &AccountImportTicket{ID: "ticket", ProxyID: p}, nil
}
func (a *bpsImportTestAdmin) CompleteAccountImport(context.Context, string, map[string]any) error {
	return nil
}
func (a *bpsImportTestAdmin) CancelAccountImport(context.Context, string) error { return nil }
func (a *bpsImportTestAdmin) CreateAccount(_ context.Context, in *CreateAccountInput) (*Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saved = in
	return &Account{ID: 77}, nil
}

type bpsImportTestClient struct {
	*reauthTestOAuthClient
	email     string
	workspace string
	nonce     string
	form      url.Values
	proxy     string
}

func (c *bpsImportTestClient) ConnectionToken(_ context.Context, f url.Values, p, nonce, profile string) (*openai.TokenResponse, *openai.SIWCIdentity, error) {
	if profile != "bps" || nonce == "" {
		return nil, nil, errors.New("wrong profile or nonce")
	}
	c.nonce = nonce
	c.form = f
	c.proxy = p
	id := &openai.SIWCIdentity{Email: c.email}
	id.Subject = "synthetic-subject"
	return &openai.TokenResponse{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", IDToken: reauthTestJWT(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": c.workspace, "chatgpt_user_id": "synthetic-user"}}), ExpiresIn: 3600}, id, nil
}
func (*bpsImportTestClient) RevokeConnectionToken(context.Context, string, string, string) error {
	return nil
}
func bpsTestInput() BPSImportInput {
	return BPSImportInput{RequestID: uuid.NewString(), Entries: []BPSImportEntry{{Email: "user@example.com", Password: "synthetic-password", TOTPSecret: "JBSWY3DPEHPK3PXP"}}, GroupIDs: []int64{123}, Models: []string{"gpt-6-astra"}, Concurrency: 3, Priority: 50}
}
func bpsTestService(t *testing.T) (*OpenAIOAuthReauthService, *bpsImportTestClient) {
	t.Helper()
	s, _, _, _, client, _ := newReauthTestService("acct-1")
	t.Cleanup(s.oauth.Stop)
	c := &bpsImportTestClient{reauthTestOAuthClient: client, email: "user@example.com", workspace: "workspace-1"}
	s.oauth.oauthClient = c
	s.bpsImportRunner = func(_ context.Context, _, _, _, _, auth, _ string) (string, error) {
		u, _ := url.Parse(auth)
		return openai.ExcelRedirectURI + "?code=synthetic&state=" + url.QueryEscape(u.Query().Get("state")), nil
	}
	return s, c
}
func waitBPSJob(t *testing.T, s *OpenAIOAuthReauthService, id string) *BPSImportStatus {
	t.Helper()
	var out *BPSImportStatus
	require.Eventually(t, func() bool {
		out, _ = s.BPSImport("admin:1", id, false)
		return out != nil && (out.Status == "completed" || out.Status == "canceled")
	}, 3*time.Second, 5*time.Millisecond)
	return out
}
func TestBPSImportValidatesAndRedacts(t *testing.T) {
	for _, change := range []func(*BPSImportInput){
		func(in *BPSImportInput) { in.Entries = nil }, func(in *BPSImportInput) { in.Entries = append(in.Entries, in.Entries[0]) },
		func(in *BPSImportInput) { in.Entries[0].TOTPSecret = "123456" }, func(in *BPSImportInput) { in.Entries[0].Email = "Person <user@example.com>" },
		func(in *BPSImportInput) { in.Models = nil }, func(in *BPSImportInput) { in.Models = []string{"../../invalid"} }, func(in *BPSImportInput) { in.GroupIDs = nil },
		func(in *BPSImportInput) { in.Concurrency = 0 }, func(in *BPSImportInput) { in.Priority = -1 }, func(in *BPSImportInput) { in.RequestID = "bad" },
	} {
		in := bpsTestInput()
		change(&in)
		require.Error(t, validateBPSImport(&in))
	}
	in := bpsTestInput()
	in.Entries[0].Email = " USER@EXAMPLE.COM "
	in.Entries[0].Password = " padded "
	in.Models = append(in.Models, in.Models...)
	require.NoError(t, validateBPSImport(&in))
	require.Equal(t, "user@example.com", in.Entries[0].Email)
	require.Equal(t, " padded ", in.Entries[0].Password)
	require.Len(t, in.Models, 1)
	raw, _ := json.Marshal(in)
	redacted := RedactAuditBody(raw, "application/json")
	require.NotContains(t, string(redacted), " padded ")
	require.NotContains(t, string(redacted), "JBSWY3DPEHPK3PXP")
}
func TestBPSImportCallbackBoundary(t *testing.T) {
	good := openai.ExcelRedirectURI + "?code=synthetic&state=expected"
	code, err := validateBPSImportCallback(good, "expected")
	require.NoError(t, err)
	require.Equal(t, "synthetic", code)
	for _, bad := range []string{good + "&code=second", good + "#fragment", good + "&state=second", good + "&error=denied", good + "&client_id=codex", strings.Replace(good, "bps.openai.com", "evil.invalid", 1), strings.Replace(good, "state=expected", "state=wrong", 1)} {
		_, err = validateBPSImportCallback(bad, "expected")
		require.Error(t, err)
	}
}
func TestBPSImportCreatesNativeAccountAndPreservesSelections(t *testing.T) {
	s, c := bpsTestService(t)
	admin := &bpsImportTestAdmin{}
	in := bpsTestInput()
	in.Workspace = "workspace-1"
	status, err := s.StartBPSImport(context.Background(), "admin:1", in, admin)
	require.NoError(t, err)
	status = waitBPSJob(t, s, status.ID)
	require.Equal(t, "succeeded", status.Rows[0].Status)
	admin.mu.Lock()
	saved := admin.saved
	admin.mu.Unlock()
	require.NotNil(t, saved)
	require.True(t, saved.ExplicitBPSImport)
	require.Equal(t, in.GroupIDs, saved.GroupIDs)
	require.Equal(t, in.Concurrency, saved.Concurrency)
	require.Equal(t, in.Priority, saved.Priority)
	require.Equal(t, openai.ExcelClientID, saved.Credentials["client_id"])
	require.Equal(t, true, saved.Extra["openai_excel_bps"])
	require.Equal(t, "off", saved.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, openai.ExcelClientID, c.form.Get("client_id"))
	require.NotEmpty(t, c.form.Get("code_verifier"))
	require.NotEmpty(t, c.nonce)
	data, _ := json.Marshal(status)
	require.NotContains(t, string(data), "synthetic-password")
	require.NotContains(t, string(data), "synthetic-access")
	require.NotContains(t, string(data), "JBSWY3DPEHPK3PXP")
	s.bpsImportMu.Lock()
	require.Empty(t, s.bpsImports[status.ID].encrypted[0])
	s.bpsImportMu.Unlock()
}
func TestBPSImportIdempotencyIsolationAndCancellation(t *testing.T) {
	s, _ := bpsTestService(t)
	var calls atomic.Int32
	entered := make(chan struct{})
	s.bpsImportRunner = func(ctx context.Context, _, _, _, _, _, _ string) (string, error) {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	in := bpsTestInput()
	admin := &bpsImportTestAdmin{}
	first, err := s.StartBPSImport(ctx, "admin:1", in, admin)
	require.NoError(t, err)
	<-entered
	cancel()
	same, err := s.StartBPSImport(context.Background(), "admin:1", in, admin)
	require.NoError(t, err)
	require.Equal(t, first.ID, same.ID)
	require.Equal(t, int32(1), calls.Load())
	require.Empty(t, s.BPSImports("admin:2"))
	_, err = s.BPSImport("admin:2", first.ID, true)
	require.Error(t, err)
	in.Entries[0].Password = "changed"
	_, err = s.StartBPSImport(context.Background(), "admin:1", in, admin)
	require.Error(t, err)
	status, err := s.BPSImport("admin:1", first.ID, false)
	require.NoError(t, err)
	require.Equal(t, "running", status.Status, "disconnecting the request must not cancel background login")
	_, err = s.BPSImport("admin:1", first.ID, true)
	require.NoError(t, err)
	status = waitBPSJob(t, s, first.ID)
	require.Equal(t, "canceled", status.Status)
	require.Equal(t, "canceled", status.Rows[0].Status)
}
func TestBPSImportRejectsIdentityWorkspaceProxyAndSkipsExisting(t *testing.T) {
	for _, scenario := range []string{"email", "workspace", "proxy", "existing", "groups"} {
		t.Run(scenario, func(t *testing.T) {
			s, c := bpsTestService(t)
			admin := &bpsImportTestAdmin{}
			in := bpsTestInput()
			in.Workspace = "workspace-1"
			var calls atomic.Int32
			original := s.bpsImportRunner
			s.bpsImportRunner = func(ctx context.Context, a, b, c, d, e, f string) (string, error) {
				calls.Add(1)
				return original(ctx, a, b, c, d, e, f)
			}
			switch scenario {
			case "email":
				c.email = "other@example.com"
			case "workspace":
				c.workspace = "other"
			case "proxy":
				id := int64(99)
				admin.assigned = &id
			case "existing":
				admin.existing = 88
			case "groups":
				admin.groupErr = errors.New("outside scope")
			}
			status, err := s.StartBPSImport(context.Background(), "admin:1", in, admin)
			if scenario == "groups" {
				require.Error(t, err)
				require.Zero(t, calls.Load())
				return
			}
			require.NoError(t, err)
			status = waitBPSJob(t, s, status.ID)
			expected := "failed"
			if scenario == "existing" {
				expected = "skipped"
			}
			require.Equal(t, expected, status.Rows[0].Status)
			if scenario == "existing" || scenario == "proxy" {
				require.Zero(t, calls.Load())
			}
			admin.mu.Lock()
			require.Nil(t, admin.saved)
			admin.mu.Unlock()
		})
	}
}
func TestBPSImportExplicitSelectionsBypassOnlyNativeTemplate(t *testing.T) {
	cfg := DefaultOAuthAutoConfig()
	cfg.Enabled = true
	cfg.GroupIDs = []int64{9}
	cfg.Concurrency = 7
	raw, _ := json.Marshal(cfg)
	svc := &adminServiceImpl{settingService: NewSettingService(&accountOpsSettingsStub{raw: string(raw)}, nil), groupRepo: autoConfigGroups{group: &Group{ID: 9, Platform: PlatformOpenAI, Status: StatusActive}}}
	for _, native := range []bool{true, false} {
		in := &CreateAccountInput{ExplicitBPSImport: true, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 3, Priority: 50, GroupIDs: []int64{123}, Credentials: map[string]any{"client_id": "codex"}}
		if native {
			in.Credentials["client_id"] = openai.ExcelClientID
		}
		require.NoError(t, svc.ApplyOAuthAutoConfig(context.Background(), in))
		if native {
			require.Equal(t, []int64{123}, in.GroupIDs)
			require.Equal(t, 3, in.Concurrency)
		} else {
			require.Equal(t, []int64{9}, in.GroupIDs)
			require.Equal(t, 7, in.Concurrency)
		}
	}
}
