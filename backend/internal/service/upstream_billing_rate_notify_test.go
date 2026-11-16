package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// Unlike the legacy probe stub, this repository enforces the production
// snapshot CAS and never adds cost synchronization behavior to the fixture.
type rateNotifyCASRepo struct {
	*upstreamBillingProbeAccountRepo
	beforeClaim func()
}

func (r *rateNotifyCASRepo) UpdateUpstreamBillingProbeSnapshot(_ context.Context, expected *Account, snapshot *UpstreamBillingProbeSnapshot, rate *float64) error {
	r.mu.Lock()
	live := r.accounts[expected.ID]
	if live == nil || NewAPIAccountFingerprint(live) != NewAPIAccountFingerprint(expected) ||
		!reflect.DeepEqual(decodeUpstreamBillingProbeSnapshot(live.Extra), decodeUpstreamBillingProbeSnapshot(expected.Extra)) ||
		upstreamBillingProbeEnabled(live) != upstreamBillingProbeEnabled(expected) {
		r.mu.Unlock()
		return ErrUpstreamBillingProbeIdentityChanged
	}
	raw, _ := json.Marshal(snapshot)
	var stored map[string]any
	_ = json.Unmarshal(raw, &stored)
	live.Extra[UpstreamBillingProbeExtraKey] = stored
	if rate != nil && upstreamBillingRateSyncEnabled(live) {
		v := *rate
		live.RateMultiplier = &v
	}
	callback := r.beforeClaim
	isClaim := snapshot.RateNotification != nil && snapshot.RateNotification.LeaseToken != ""
	r.mu.Unlock()
	if isClaim && callback != nil {
		callback()
	}
	return nil
}

type rateNotifySender struct {
	mu     sync.Mutex
	calls  int
	fail   bool
	bodies []string
}

func (s *rateNotifySender) SendEmail(_ context.Context, _, _, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.bodies = append(s.bodies, body)
	if s.fail {
		return errors.New("private SMTP diagnostic must not be persisted")
	}
	return nil
}

type rateNotifyHTTP struct {
	HTTPUpstream
	rate float64
	fail bool
}

func (h *rateNotifyHTTP) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if req.URL.Path == "/v1/usage" {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: upstreamBalanceProbeWalletBody()}, nil
	}
	if h.fail {
		return &http.Response{StatusCode: 403, Header: http.Header{"Server": {"cloudflare"}, "Cf-Ray": {"test"}}, Body: io.NopCloser(strings.NewReader(`<html>cf-chl-test</html>`))}, nil
	}
	body := fmt.Sprintf(`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":%[1]g,"resolved_rate_multiplier":%[1]g,"effective_rate_multiplier":%[1]g,"peak_rate_enabled":false,"observed_at":"2026-11-11T01:00:00Z"}`, h.rate)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func rateNotifyFixture(t *testing.T) (*UpstreamBillingProbeService, *rateNotifyCASRepo, *rateNotifySender, *rateNotifyHTTP, *time.Time) {
	t.Helper()
	rate := 0.37 // Local manual rate deliberately differs from the upstream.
	a := &Account{ID: 17, Name: "probe", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"base_url": "https://upstream.example/v1", "api_key": "private-api-key"},
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: true, UpstreamBillingRateSyncEnabledExtraKey: false}, RateMultiplier: &rate}
	r := &rateNotifyCASRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{17: a}}}
	settings := &upstreamBillingProbeSettingRepo{values: map[string]string{SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30,"rate_change_notify_enabled":true,"rate_change_notify_email":"notice@example.com","rate_change_notify_epoch":"epoch-1"}`}}
	h := &rateNotifyHTTP{rate: 1}
	svc := newUpstreamBillingProbeTestService(r, h, settings)
	now := time.Date(2026, 11, 11, 1, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	sender := &rateNotifySender{}
	svc.SetRateNotificationSender(sender)
	svc.accountRepo.(*rateNotifyAccountRepository).async = false
	return svc, r, sender, h, &now
}

func probeRateNotify(t *testing.T, svc *UpstreamBillingProbeService) *UpstreamBillingProbeSnapshot {
	t.Helper()
	snapshot, err := svc.ProbeAccount(context.Background(), 17)
	require.NoError(t, err)
	return snapshot
}

func TestRateReminderRealProbeIndependentOfSyncAndKeepsCadence(t *testing.T) {
	svc, repo, sender, httpStub, now := rateNotifyFixture(t)
	first := probeRateNotify(t, svc)
	require.Equal(t, 1.0, first.RateNotification.Baseline)
	require.Nil(t, first.RateNotification.Candidate)
	httpStub.rate = 0.6
	*now = now.Add(time.Minute)
	changed := probeRateNotify(t, svc)
	require.InDelta(t, float64(30*time.Minute), float64(changed.NextProbeAt.Sub(*now)), float64(5*time.Minute))
	*now = now.Add(4 * time.Minute)
	probeRateNotify(t, svc)
	require.Zero(t, sender.calls)
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	require.Equal(t, 1, sender.calls)
	*now = now.Add(30 * time.Minute)
	probeRateNotify(t, svc)
	require.Equal(t, 1, sender.calls)
	require.Equal(t, 0.37, *repo.accounts[17].RateMultiplier)
	require.False(t, upstreamBillingRateSyncEnabled(repo.accounts[17]))
	require.NotContains(t, sender.bodies[0], "private-api-key")
	require.Contains(t, sender.bodies[0], "不会启用或改变自动跟价")
}

func TestRateReminderRestartFailureRecoveryAndRetry(t *testing.T) {
	svc, repo, sender, httpStub, now := rateNotifyFixture(t)
	probeRateNotify(t, svc)
	httpStub.rate = 2
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	*now = now.Add(5 * time.Minute)
	httpStub.fail = true
	failed := probeRateNotify(t, svc)
	require.Equal(t, "cloudflare_blocked", failed.LastError)
	require.Zero(t, sender.calls)
	// A new service uses the persisted state, not its process memory.
	settings := svc.settingService.settingRepo
	restarted := newUpstreamBillingProbeTestService(repo, httpStub, settings)
	restarted.now = func() time.Time { return *now }
	restarted.SetRateNotificationSender(sender)
	restarted.accountRepo.(*rateNotifyAccountRepository).async = false
	httpStub.fail = false
	sender.fail = true
	attempt := probeRateNotify(t, restarted)
	require.Equal(t, 1, sender.calls)
	stored := decodeUpstreamBillingProbeSnapshot(repo.accounts[17].Extra)
	require.Equal(t, "email_send_failed", stored.RateNotification.LastError)
	require.NotContains(t, fmt.Sprint(stored), "private SMTP")
	require.Equal(t, attempt.NextProbeAt, stored.NextProbeAt)
	*now = now.Add(30 * time.Second)
	probeRateNotify(t, restarted)
	require.Equal(t, 1, sender.calls)
	*now = now.Add(30 * time.Second)
	sender.fail = false
	probeRateNotify(t, restarted)
	require.Equal(t, 2, sender.calls)
	*now = now.Add(time.Minute)
	httpStub.rate = 1
	recovered := probeRateNotify(t, restarted)
	require.Nil(t, recovered.RateNotification.Candidate)
	*now = now.Add(time.Minute)
	httpStub.rate = 3
	probeRateNotify(t, restarted)
	*now = now.Add(5 * time.Minute)
	probeRateNotify(t, restarted)
	require.Equal(t, 3, sender.calls)
}

func TestRateReminderSettingsDefaultEpochAndDisable(t *testing.T) {
	ctx := context.Background()
	repo := &upstreamBillingProbeSettingRepo{values: map[string]string{SettingKeyUpstreamBillingProbeSettings: `{"enabled":false,"interval_minutes":83}`}}
	settingsSvc := NewSettingService(repo, &config.Config{})
	legacy, err := settingsSvc.GetUpstreamBillingProbeSettings(ctx)
	require.NoError(t, err)
	require.False(t, legacy.RateChangeNotifyEnabled)
	require.Equal(t, 83, legacy.IntervalMinutes)
	require.False(t, legacy.Enabled)
	legacy.RateChangeNotifyEnabled = true
	legacy.RateChangeNotifyEmail = "bad\n@example.com"
	require.Error(t, settingsSvc.SetUpstreamBillingProbeSettings(ctx, legacy))
	legacy.RateChangeNotifyEmail = "notice@example.com"
	require.NoError(t, settingsSvc.SetUpstreamBillingProbeSettings(ctx, legacy))
	epoch := legacy.RateChangeNotifyEpoch
	require.NotEmpty(t, epoch)
	require.NoError(t, settingsSvc.SetUpstreamBillingProbeSettings(ctx, legacy))
	require.Equal(t, epoch, legacy.RateChangeNotifyEpoch)
	legacy.RateChangeNotifyEnabled = false
	require.NoError(t, settingsSvc.SetUpstreamBillingProbeSettings(ctx, legacy))
	require.NotEqual(t, epoch, legacy.RateChangeNotifyEpoch)
	svc, _, sender, h, now := rateNotifyFixture(t)
	probeRateNotify(t, svc)
	h.rate = 2
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	settings, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	settings.RateChangeNotifyEnabled = false
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	*now = now.Add(5 * time.Minute)
	snap := probeRateNotify(t, svc)
	require.Nil(t, snap.RateNotification)
	require.Zero(t, sender.calls)
}

func TestRateReminderCASAndDisableAfterClaim(t *testing.T) {
	svc, repo, sender, h, now := rateNotifyFixture(t)
	probeRateNotify(t, svc)
	h.rate = 2
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	// Disabling the account between the persisted claim and SendEmail blocks it.
	repo.beforeClaim = func() {
		repo.mu.Lock()
		repo.accounts[17].Extra[UpstreamBillingProbeEnabledExtraKey] = false
		repo.mu.Unlock()
	}
	*now = now.Add(5 * time.Minute)
	probeRateNotify(t, svc)
	require.Zero(t, sender.calls)
	// Two instances cannot both claim the same old snapshot.
	repo.beforeClaim = nil
	repo.accounts[17].Extra[UpstreamBillingProbeEnabledExtraKey] = true
	a, _ := repo.GetByID(context.Background(), 17)
	b, _ := repo.GetByID(context.Background(), 17)
	snapA := decodeUpstreamBillingProbeSnapshot(a.Extra)
	snapB := decodeUpstreamBillingProbeSnapshot(b.Extra)
	snapA.RateNotification.LeaseToken = "one"
	snapB.RateNotification.LeaseToken = "two"
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), a, snapA, nil))
	require.ErrorIs(t, repo.UpdateUpstreamBillingProbeSnapshot(context.Background(), b, snapB, nil), ErrUpstreamBillingProbeIdentityChanged)
}

type rateNotifyReadBarrier struct {
	AccountRepository
	reads atomic.Int32
	ready chan struct{}
}

func (r *rateNotifyReadBarrier) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.AccountRepository.GetByID(ctx, id)
	if n := r.reads.Add(1); n <= 2 {
		if n == 2 {
			close(r.ready)
		}
		<-r.ready
	}
	return a, err
}

func (r *rateNotifyReadBarrier) UpdateUpstreamBillingProbeSnapshot(ctx context.Context, a *Account, snapshot *UpstreamBillingProbeSnapshot, rate *float64) error {
	return r.AccountRepository.(upstreamBillingProbeSnapshotWriter).UpdateUpstreamBillingProbeSnapshot(ctx, a, snapshot, rate)
}

func TestRateReminderTwoInstancesSendOnceThroughSnapshotCAS(t *testing.T) {
	svc, repo, sender, h, now := rateNotifyFixture(t)
	probeRateNotify(t, svc)
	h.rate = 2
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	wrapped := svc.accountRepo.(*rateNotifyAccountRepository)
	wrapped.sender = nil // Persist confirmation before releasing two senders.
	*now = now.Add(5 * time.Minute)
	probeRateNotify(t, svc)
	barrier := &rateNotifyReadBarrier{AccountRepository: repo, ready: make(chan struct{})}
	first := *wrapped
	first.AccountRepository, first.sender = barrier, sender
	second := first
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); first.deliverRateNotification(context.Background(), 17) }()
	go func() { defer wg.Done(); second.deliverRateNotification(context.Background(), 17) }()
	wg.Wait()
	require.Equal(t, 1, sender.calls)
	require.NotNil(t, decodeUpstreamBillingProbeSnapshot(repo.accounts[17].Extra).RateNotification.SentAt)
}

type rateNotifyNewAPIRepo struct {
	*newAPIAuthRepoFake
	accounts *rateNotifyCASRepo
	writes   int
}

func (r *rateNotifyNewAPIRepo) WriteSnapshot(ctx context.Context, a *Account, b *NewAPIAccountBinding, snapshot *UpstreamBillingProbeSnapshot) error {
	current := r.bindings[a.ID]
	if current == nil || b.Profile.Revision != current.Profile.Revision || current.Fingerprint != NewAPIAccountFingerprint(a) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	r.writes++
	return r.accounts.UpdateUpstreamBillingProbeSnapshot(ctx, a, snapshot, nil)
}

func TestRateReminderRealBoundNewAPIProbeAndRevisionProtection(t *testing.T) {
	svc, accounts, sender, _, now := rateNotifyFixture(t)
	a := accounts.accounts[17]
	a.Credentials = map[string]any{"base_url": "https://example.com/v1", "api_key": "sk-abcd123456789wxyz"}
	binding := &NewAPIAccountBinding{AccountID: 17, TokenID: 3, Fingerprint: NewAPIAccountFingerprint(a), Profile: NewAPISiteAuthorization{ID: 2, Revision: 1, SiteURL: "https://example.com", UserID: 7, Ciphertext: "encrypted-management-secret"}}
	repo := &rateNotifyNewAPIRepo{newAPIAuthRepoFake: &newAPIAuthRepoFake{bindings: map[int64]*NewAPIAccountBinding{17: binding}}, accounts: accounts}
	h := &newAPIHTTP{bodies: map[string]string{"GET /api/status": `{"success":true,"data":{"quota_per_unit":1}}`, "GET /api/user/self": `{"success":true,"data":{"id":7,"quota":10}}`, "GET /api/pricing": `{"success":true,"group_ratio":{"vip":1}}`, "GET /api/token/?p=0&page_size=100": `{"success":true,"data":{"items":[{"id":3,"key":"abcd**********wxyz","group":"vip"}],"total":1,"page":1,"page_size":100}}`}}
	// Recreate production wiring after configuring the dedicated repository.
	svc = newUpstreamBillingProbeTestService(accounts, h, svc.settingService.settingRepo)
	svc.now = func() time.Time { return *now }
	svc.SetNewAPIAuthorization(repo, newAPIEncryptorFake{}, true)
	svc.SetRateNotificationSender(sender)
	svc.accountRepo.(*rateNotifyAccountRepository).async = false
	first := probeRateNotify(t, svc)
	require.Equal(t, "bound_group_pricing", first.RateSource)
	require.Equal(t, 1.0, first.RateNotification.Baseline)
	h.bodies["GET /api/pricing"] = `{"success":true,"group_ratio":{"vip":0.4}}`
	*now = now.Add(time.Minute)
	probeRateNotify(t, svc)
	*now = now.Add(5 * time.Minute)
	probeRateNotify(t, svc)
	require.Equal(t, 1, sender.calls)
	require.GreaterOrEqual(t, repo.writes, 5)
	require.Equal(t, 0.37, *a.RateMultiplier)
	require.False(t, upstreamBillingRateSyncEnabled(a))
	require.NotContains(t, sender.bodies[0], "management-secret")
	// Changing authorization revision invalidates old notification writes.
	loaded, _ := accounts.GetByID(context.Background(), 17)
	pending := decodeUpstreamBillingProbeSnapshot(loaded.Extra)
	binding.Profile.Revision++
	err := svc.accountRepo.(*rateNotifyAccountRepository).writeSnapshot(context.Background(), loaded, pending)
	require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
}
