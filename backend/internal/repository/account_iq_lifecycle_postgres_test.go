package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
	"time"
)

func TestIQAutomationPostgres(t *testing.T) {
	dsn := os.Getenv("KITTY_ACCOUNT_AUTOMATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires dedicated isolated PostgreSQL fixture")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "kitty_account_automation_fixture", name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	require.NoError(t, ApplyMigrations(ctx, db))
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	plans := NewScheduledTestPlanRepository(db)
	results := NewScheduledTestResultRepository(db)
	var production, isolation int64
	suffix := fmt.Sprint(time.Now().UnixNano())
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,status) VALUES($1,'openai','active') RETURNING id`, "iq-production-"+suffix).Scan(&production))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,status) VALUES($1,'openai','active') RETURNING id`, "iq-isolation-"+suffix).Scan(&isolation))
	b := service.DefaultIQBootstrap()
	b.Enabled = true
	b.QuarantineEnabled = true
	b.QuarantineGroupID = isolation
	add := func(ticket string) *service.Account {
		a := &service.Account{Name: "iq-fixture-" + suffix, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Concurrency: 1, Schedulable: false, InitialIQBootstrap: &b, ImportTicket: ticket, Credentials: map[string]any{"access_token": "fixture-token"}}
		require.NoError(t, repo.CreateWithAccountGroups(ctx, a, []service.AccountGroup{{GroupID: production, Priority: 1}}))
		return a
	}
	a := add("")
	var planID int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT plan_id FROM account_iq_lifecycle WHERE account_id=$1`, a.ID).Scan(&planID))
	check := func(a *service.Account, n int, sched bool) {
		got, e := repo.GetByID(ctx, a.ID)
		require.NoError(t, e)
		require.Equal(t, n, got.Concurrency)
		require.Equal(t, sched, got.Schedulable)
	}
	check(a, 1, false)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET concurrency=99,schedulable=true WHERE id=$1`, a.ID)
	require.NoError(t, err)
	check(a, 1, false)
	step := 0
	sample := func(planID int64, status, answer, message string) *service.ScheduledTestResult {
		p, e := plans.GetByID(ctx, planID)
		require.NoError(t, e)
		until := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond)
		_, e = db.ExecContext(ctx, `UPDATE scheduled_test_plans SET running_until=$2 WHERE id=$1`, planID, until)
		require.NoError(t, e)
		step++
		at := time.Now().UTC().Add(-time.Hour).Add(time.Duration(step) * time.Second).Truncate(time.Microsecond)
		cfg := *p.PelicanConfig
		cfg.ModelID = p.ModelID
		return &service.ScheduledTestResult{PlanID: planID, Status: status, ResponseText: answer, ErrorMessage: message, StartedAt: at, FinishedAt: at.Add(time.Millisecond), PelicanConfig: &cfg, PlanUpdatedAt: p.UpdatedAt, LeaseUntil: &until}
	}
	save := func(r *service.ScheduledTestResult) { _, e := results.Create(ctx, r); require.NoError(t, e) }
	for i := 0; i < 5; i++ {
		save(sample(planID, "success", "21", ""))
	}
	check(a, 1, false)
	save(sample(planID, "skipped", "", "waiting"))
	state, err := readIQLifecycle(ctx, db, a.ID)
	require.NoError(t, err)
	require.Equal(t, 5, state.CorrectStreak)
	cfg := service.DefaultOAuthAutoConfig()
	cfg.UpgradeEnabled = true
	cfg.UpgradeGroupIDs = []int64{production, isolation}
	cfg.Revision = "fixture-" + suffix
	cfg.UpdatedAt = time.Now().Add(-time.Hour)
	raw, _ := json.Marshal(cfg)
	_, err = db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, service.SettingKeyOAuthAutoConfig, string(raw))
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		require.NoError(t, repo.RecordConcurrencyResult(ctx, service.AccountConcurrencyResult{AccountID: a.ID, StartedAt: time.Now(), Success: true}, cfg))
	}
	check(a, 1, false)
	sixth := sample(planID, "success", "21", "")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := results.Create(ctx, sixth); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	check(a, 2, true)
	var passed int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM account_iq_lifecycle_events WHERE account_id=$1 AND kind='passed'`, a.ID).Scan(&passed))
	require.Equal(t, 1, passed)
	state, err = readIQLifecycle(ctx, db, a.ID)
	require.NoError(t, err)
	require.Len(t, state.Samples, 6)
	require.NoError(t, repo.RecordConcurrencyResult(ctx, service.AccountConcurrencyResult{AccountID: a.ID, StartedAt: state.PassedAt.Add(-time.Second), Success: true}, cfg))
	check(a, 2, true)
	for i := 0; i < 20; i++ {
		require.NoError(t, repo.RecordConcurrencyResult(ctx, service.AccountConcurrencyResult{AccountID: a.ID, StartedAt: time.Now(), Success: true}, cfg))
	}
	check(a, 3, true)
	for i := 0; i < 6; i++ {
		save(sample(planID, "failed", "29", "answer_mismatch: expected 21"))
	}
	check(a, 3, true)
	var membership []byte
	require.NoError(t, db.QueryRowContext(ctx, `SELECT json_agg(group_id) FROM account_groups WHERE account_id=$1`, a.ID).Scan(&membership))
	require.JSONEq(t, fmt.Sprintf("[%d]", isolation), string(membership))
	for i := 0; i < 6; i++ {
		save(sample(planID, "success", "21", ""))
	}
	check(a, 3, true)
	for i := 0; i < 20; i++ {
		require.NoError(t, repo.RecordConcurrencyResult(ctx, service.AccountConcurrencyResult{AccountID: a.ID, StartedAt: time.Now().Add(time.Minute), Success: true}, cfg))
	}
	check(a, 3, true)
	state, err = readIQLifecycle(ctx, db, a.ID)
	require.NoError(t, err)
	require.NotNil(t, state.QuarantinedAt)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	conflict, err := restoreQualityGroups(ctx, tx, a.ID, &qualityState{})
	require.NoError(t, err)
	require.True(t, conflict)
	require.NoError(t, tx.Rollback())
	// First-stage quarantine stays closed even after later correct answers.
	pending := add("")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT plan_id FROM account_iq_lifecycle WHERE account_id=$1`, pending.ID).Scan(&planID))
	for i := 0; i < 6; i++ {
		save(sample(planID, "failed", "29", "answer_mismatch: expected 21"))
	}
	for i := 0; i < 6; i++ {
		save(sample(planID, "success", "21", ""))
	}
	check(pending, 1, false)
	// Invalid targets hold scheduling, preserve evidence and permit explicit repair.
	broken := add("")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT plan_id FROM account_iq_lifecycle WHERE account_id=$1`, broken.ID).Scan(&planID))
	for i := 0; i < 6; i++ {
		save(sample(planID, "success", "21", ""))
	}
	check(broken, 2, true)
	_, err = db.ExecContext(ctx, `UPDATE groups SET status='inactive' WHERE id=$1`, isolation)
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		save(sample(planID, "failed", "29", "answer_mismatch: expected 21"))
	}
	check(broken, 2, false)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET schedulable=true WHERE id=$1`, broken.ID)
	require.NoError(t, err)
	check(broken, 2, false)
	for i := 0; i < 6; i++ {
		save(sample(planID, "success", "21", ""))
	}
	check(broken, 2, false)
	state, err = readIQLifecycle(ctx, db, broken.ID)
	require.NoError(t, err)
	require.True(t, state.QuarantinePending)
	require.Len(t, state.Samples, 6)
	require.Equal(t, "29", state.Samples[0].Answer)
	require.Error(t, plans.(*scheduledTestPlanRepository).SetIQQuarantineTarget(ctx, planID, isolation))
	_, err = db.ExecContext(ctx, `UPDATE groups SET status='active' WHERE id=$1`, isolation)
	require.NoError(t, err)
	require.NoError(t, plans.(*scheduledTestPlanRepository).SetIQQuarantineTarget(ctx, planID, isolation))
	save(sample(planID, "success", "21", ""))
	check(broken, 2, false)
	state, err = readIQLifecycle(ctx, db, broken.ID)
	require.NoError(t, err)
	require.False(t, state.QuarantinePending)
	require.NotNil(t, state.QuarantinedAt)
	summaries, err := plans.(service.IQIntervalRepository).IQSummaries(ctx, []int64{broken.ID, a.ID, pending.ID})
	require.NoError(t, err)
	require.Len(t, summaries, 3)
	// Disabled/reconfigured plans cannot accept stale in-flight successes.
	fresh := add("")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT plan_id FROM account_iq_lifecycle WHERE account_id=$1`, fresh.ID).Scan(&planID))
	old := sample(planID, "success", "21", "")
	_, err = db.ExecContext(ctx, `UPDATE scheduled_test_plans SET enabled=false,updated_at=clock_timestamp() WHERE id=$1`, planID)
	require.NoError(t, err)
	save(old)
	state, err = readIQLifecycle(ctx, db, fresh.ID)
	require.NoError(t, err)
	require.Zero(t, state.CorrectStreak)
	check(fresh, 1, false)
	// A verified receipt binds the login to exactly one created account.
	ticket, err := repo.BeginAccountImport(ctx, "openai", "twofa", nil, "fixture@example.test")
	require.NoError(t, err)
	fp := service.ImportCredentialFingerprint(map[string]any{"access_token": "fixture-token"})
	require.NoError(t, repo.ReadyAccountImport(ctx, ticket.ID, fp))
	_, err = repo.ResolveAccountImport(ctx, ticket.ID, "wrong")
	require.Error(t, err)
	imported := add(ticket.ID)
	resolved, err := repo.ResolveAccountImport(ctx, ticket.ID, fp)
	require.NoError(t, err)
	require.Equal(t, imported.ID, *resolved.AccountID)
	duplicate := *imported
	duplicate.ID = 0
	require.Error(t, repo.Create(ctx, &duplicate))
	check(imported, 1, false)
}
