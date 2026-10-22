package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/accounttimer"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SchedulableTimers exposes an optional capability without expanding the broad
// AccountRepository interface (and every unrelated mock). Production uses sql.DB.
func (r *accountRepository) SchedulableTimers() accounttimer.Repository {
	db, ok := r.sql.(*sql.DB)
	if !ok || db == nil {
		return nil
	}
	return &accounttimer.Store{
		DB: db, Policy: accounttimer.ConfiguredPolicy(),
		Enqueue: func(ctx context.Context, tx *sql.Tx, id int64) error {
			return enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &id, nil, nil)
		},
		AfterCommit:  r.syncSchedulerAccountSnapshotDetached,
		CheckOpening: checkAccountTimerAdmission,
	}
}

func checkAccountTimerAdmission(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	var held bool
	err := tx.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM account_iq_lifecycle WHERE account_id=$1
   AND (passed_at IS NULL OR quarantined_at IS NOT NULL OR quarantine_pending))
 OR EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND extra ? 'openai_two_fa_admission'
   AND COALESCE(extra->'openai_two_fa_admission'->>'phase','') <> 'passed')`, id).Scan(&held)
	if err != nil {
		return "", err
	}
	if held {
		return "admission_pending", nil
	}
	return "", nil
}
