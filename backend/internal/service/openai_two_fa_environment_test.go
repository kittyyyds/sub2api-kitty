package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTwoFAEnvironmentSnapshotsProxyBeforeLogin(t *testing.T) {
	proxy := timezoneProxyFixture()
	proxy.Protocol = "socks5"
	original := proxy.URL()
	reader := &reauthTestAccountReader{proxy: proxy}
	svc := &OpenAIOAuthReauthService{accounts: reader}
	environment := &TwoFALoginEnvironment{Enabled: true, Mode: "proxy"}
	probeCalls := 0
	prober := timezoneProberFunc(func(_ context.Context, u string) (*ProxyExitInfo, int64, error) {
		probeCalls++
		require.Equal(t, original, u)
		return timezoneResult("America/Los_Angeles"), 1, nil
	})
	entry := AccountTokenGuardReloginAccount{Email: "fixture@example.invalid", Password: "fixture", MFASecret: "fixture"}
	login, err := svc.TwoFAImportLogin(context.Background(), entry, 7, environment, prober)
	require.NoError(t, err)
	proxy.Host = "changed.invalid"
	environment.Mode = "manual"
	environment.Timezone = "Asia/Tokyo"
	id := strings.Repeat("b", 48)
	posted := false
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, original, body["proxy"])
			require.Equal(t, "America/Los_Angeles", body["timezone"])
			posted = true
			json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
		if r.Method == "DELETE" {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{"status":"failed","stage":"navigation","code":"security_verification_required","http_status":403}`))
	}))
	defer worker.Close()
	t.Setenv("KITTY_LOGIN_BROWSER_URL", worker.URL)
	t.Setenv("KITTY_LOGIN_BROWSER_TOKEN", strings.Repeat("fixture", 8))
	_, err = login(context.Background())
	require.Error(t, err)
	require.Equal(t, 1, probeCalls)
	require.True(t, posted)
}

func TestTwoFAEnvironmentProbeFailureStopsBeforeCredentials(t *testing.T) {
	svc := &OpenAIOAuthReauthService{accounts: &reauthTestAccountReader{proxy: timezoneProxyFixture()}}
	prober := timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) {
		return nil, 0, errors.New("private proxy password")
	})
	login, err := svc.TwoFAImportLogin(context.Background(), AccountTokenGuardReloginAccount{Email: "a@example.invalid", Password: "fixture", MFASecret: "fixture"}, 7, &TwoFALoginEnvironment{Enabled: true, Mode: "proxy"}, prober)
	require.NoError(t, err)
	_, err = login(context.Background())
	require.Error(t, err)
	p := reauthruntime.LoginFailure(err)
	require.Equal(t, "timezone_probe_failed", p.Code)
	require.NotContains(t, err.Error(), "private")
}

func TestTwoFATerminalDiagnosticsDoNotBlockBatchAndRemainBounded(t *testing.T) {
	svc := NewAccountTokenGuardService(nil, nil, nil, nil, nil)
	defer svc.clearTwoFALogins()
	entry := AccountTokenGuardReloginAccount{Email: "a@example.invalid", Password: "fixture", MFASecret: "fixture"}
	for i := 0; i < 260; i++ {
		job, err := svc.StartTwoFAProxyLogin(context.Background(), entry, func(ctx context.Context) (map[string]any, error) {
			reauthruntime.ReportLoginProgress(ctx, reauthruntime.LoginProgress{Status: "waiting_interaction", Stage: "security_verification", HTTPStatus: 403})
			return nil, &reauthruntime.LoginError{Progress: reauthruntime.LoginProgress{Status: "failed", Stage: "security_verification", Code: "login_timeout", HTTPStatus: 403}}
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool { j, ok := svc.TwoFALogin(job.ID); return ok && j.Status == "failed" }, time.Second, time.Millisecond)
	}
	svc.loginMu.Lock()
	count := len(svc.logins)
	svc.loginMu.Unlock()
	require.Equal(t, 256, count)
}
