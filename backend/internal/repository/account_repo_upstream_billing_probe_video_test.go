package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestVideoProbeLockAndMergePreservesSwitchAndIsolatesRateSync(t *testing.T) {
	for _, tc := range []struct {
		name         string
		identity     bool
		explicit     *bool
		wantEnabled  bool
		wantSnapshot bool
	}{
		{name: "ordinary edit", identity: true, wantEnabled: true, wantSnapshot: true},
		{name: "explicit enable", identity: true, explicit: probeBoolPtr(true), wantEnabled: true, wantSnapshot: true},
		{name: "explicit disable", identity: true, explicit: probeBoolPtr(false)},
		{name: "new identity", identity: false, wantEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			mock.ExpectQuery(`(?s)SELECT.*FOR NO KEY UPDATE`).
				WithArgs(int64(81), service.PlatformVideo, service.AccountTypeAPIKey, `{}`, nil).
				WillReturnRows(sqlmock.NewRows([]string{"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "rate_sync_enabled", "snapshot", "ollama_session", "ollama_auto", "ollama_snapshot", "opencode_group_unchanged", "opencode_auto", "opencode_snapshot", "current_extra"}).
					AddRow(tc.identity, false, true, []byte(`true`), []byte(`true`), []byte(`{"kind":"video_model_prices","status":"ok","video_pricing":{"models":{}}}`), nil, nil, nil, false, nil, nil, nil))
			account := &service.Account{ID: 81, Platform: service.PlatformVideo, Type: service.AccountTypeAPIKey}
			extra, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, tc.explicit, nil)
			require.NoError(t, err)
			require.Equal(t, tc.wantEnabled, extra[service.UpstreamBillingProbeEnabledExtraKey])
			require.Equal(t, false, extra[service.UpstreamBillingRateSyncEnabledExtraKey])
			if tc.wantSnapshot {
				require.Equal(t, "video_model_prices", extra[service.UpstreamBillingProbeExtraKey].(map[string]any)["kind"])
			} else {
				require.NotContains(t, extra, service.UpstreamBillingProbeExtraKey)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
	_, err := lockAndMergeAccountProbeExtra(context.Background(), nil, &service.Account{Platform: service.PlatformVideo}, nil, probeBoolPtr(true))
	require.ErrorIs(t, err, service.ErrVideoBillingRateSyncUnsupported)
}

func TestVideoProbeSnapshotCASNeverWritesMultipliers(t *testing.T) {
	for _, affected := range []int64{1, 0} {
		for _, platform := range []string{service.PlatformVideo, service.PlatformOpenAI} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			mock.ExpectBegin()
			tx, err := client.Tx(context.Background())
			require.NoError(t, err)
			account := &service.Account{
				ID: 81, Platform: platform, Type: service.AccountTypeAPIKey,
				Extra: map[string]any{service.UpstreamBillingProbeEnabledExtraKey: true, service.UpstreamBillingRateSyncEnabledExtraKey: true},
			}
			// Even an accidental rate payload must not cross the repository guard
			// for video accounts or a video-kind snapshot on another account.
			snapshot := &service.UpstreamBillingProbeSnapshot{
				Kind: service.UpstreamBillingProbeKindVideoModelPrices, Status: service.UpstreamBillingProbeStatusOK,
				LastAttemptAt: time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC),
				Data:          map[string]any{"billing_scope": "token", "resolved_rate_multiplier": 0.2, "peak_rate_enabled": false},
				VideoPricing:  &service.VideoSupplierCatalog{Version: "test", Models: map[string]service.VideoSupplierPrice{}},
			}
			payload, err := json.Marshal(map[string]any{service.UpstreamBillingProbeExtraKey: snapshot})
			require.NoError(t, err)
			mock.ExpectExec(`(?s)UPDATE accounts.*AND platform = \$3.*AND type = \$4.*AND credentials = \$5::jsonb.*AND proxy_id IS NOT DISTINCT FROM \$6.*`+regexp.QuoteMeta("COALESCE(extra -> 'upstream_billing_probe', 'null'::jsonb) = $7::jsonb")+`.*`+regexp.QuoteMeta("COALESCE(extra -> 'upstream_billing_probe_enabled', 'null'::jsonb) = $8::jsonb")+`.*`+regexp.QuoteMeta("COALESCE(extra -> 'upstream_billing_rate_sync_enabled', 'null'::jsonb) = $9::jsonb")).
				WithArgs(string(payload), int64(81), platform, service.AccountTypeAPIKey, "null", nil, "null", "true", "true", nil).
				WillReturnResult(sqlmock.NewResult(0, affected))
			if affected == 1 {
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
					WithArgs(service.SchedulerOutboxEventAccountChanged, int64(81), nil, nil, sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}
			rate := 0.2
			repo := newAccountRepositoryWithSQL(client, db, nil)
			err = repo.UpdateUpstreamBillingProbeSnapshot(dbent.NewTxContext(context.Background(), tx), account, snapshot, &rate)
			if affected == 1 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)
			}
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		}
	}
}
