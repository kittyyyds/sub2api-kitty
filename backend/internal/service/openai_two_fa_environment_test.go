package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTwoFALegacyEnvironmentAlwaysSelectsBackgroundProtocol(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			worker := reauthruntime.New(t.TempDir(), "2.10.7", "http://127.0.0.1", "fixture")
			defer worker.Stop()
			proxy := timezoneProxyFixture()
			svc := &OpenAIOAuthReauthService{accounts: &reauthTestAccountReader{proxy: proxy}, worker: worker}
			probe := timezoneProberFunc(func(context.Context, string) (*ProxyExitInfo, int64, error) {
				t.Fatal("retired account environment must not probe or select a browser")
				return nil, 0, nil
			})
			entry := AccountTokenGuardReloginAccount{Email: "a@example.invalid", Password: "fixture", MFASecret: "fixture"}
			login, err := svc.TwoFAImportLogin(context.Background(), entry, proxy.ID, &TwoFALoginEnvironment{Enabled: enabled, Mode: "proxy", Timezone: "Asia/Tokyo"}, probe)
			require.NoError(t, err)
			require.NotNil(t, login)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = login(ctx)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}
func TestTwoFAUnavailableSelectedProxyNeverFallsBack(t *testing.T) {
	worker := reauthruntime.New(t.TempDir(), "2.10.7", "http://127.0.0.1", "fixture")
	defer worker.Stop()
	svc := &OpenAIOAuthReauthService{accounts: &reauthTestAccountReader{}, worker: worker}
	login, err := svc.TwoFAImportLogin(context.Background(), AccountTokenGuardReloginAccount{Email: "a@example.invalid", Password: "fixture", MFASecret: "fixture"}, 7, nil, nil)
	require.Error(t, err)
	require.Nil(t, login)
	require.NotContains(t, err.Error(), "fixture")
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
