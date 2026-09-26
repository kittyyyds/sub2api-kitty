package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplicationCenterRepositoryCompleteStepUsesLocksAndOptimisticVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewApplicationCenterRepository(db)

	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	now := time.Now().UTC()
	next := &service.ApplicationRunStep{
		StepKey: "media", Sequence: 2, Executor: service.ApplicationExecutorImageGeneration,
		CredentialRole: service.ApplicationCredentialMedia, Status: service.ApplicationRunStatusPending,
		Request: json.RawMessage(`{"model":"grok-imagine-image","prompt":"cat"}`), Attempt: 1, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, COALESCE(current_step_key, ''), version")).
		WithArgs(runID, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step_key", "version"}).
			AddRow(service.ApplicationRunStatusRunning, "planner", int64(4)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status, version")).
		WithArgs(runID, "planner").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version"}).
			AddRow(int64(9), service.ApplicationRunStatusPending, int64(2)))
	mock.ExpectExec("(?s)UPDATE application_run_steps.*WHERE id = \\$1 AND version = \\$5").
		WithArgs(int64(9), `{"final_prompt":"cat"}`, "gateway-1", sqlmock.AnyArg(), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?s)INSERT INTO application_run_steps").
		WithArgs(runID, "media", 2, service.ApplicationExecutorImageGeneration,
			service.ApplicationCredentialMedia, service.ApplicationRunStatusPending,
			string(next.Request), 1, int64(1), now, now).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("(?s)UPDATE application_runs.*current_step_key = \\$2.*version = \\$5").
		WithArgs(runID, "media", sqlmock.AnyArg(), int64(7), int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.CompleteStep(context.Background(), service.CompleteApplicationStepParams{
		UserID: 7, RunID: runID, StepKey: "planner",
		StepOutput: json.RawMessage(`{"final_prompt":"cat"}`), GatewayRequestID: "gateway-1", NextStep: next,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplicationCenterRepositoryCompleteStepRejectsStaleStepWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewApplicationCenterRepository(db)
	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, COALESCE(current_step_key, ''), version")).
		WithArgs(runID, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step_key", "version"}).
			AddRow(service.ApplicationRunStatusRunning, "media", int64(4)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status, version")).
		WithArgs(runID, "media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version"}).
			AddRow(int64(10), service.ApplicationRunStatusPending, int64(3)))
	mock.ExpectExec("(?s)UPDATE application_run_steps.*WHERE id = \\$1 AND version = \\$5").
		WithArgs(int64(10), `{"request_id":"req-1"}`, "req-1", sqlmock.AnyArg(), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.CompleteStep(context.Background(), service.CompleteApplicationStepParams{
		UserID: 7, RunID: runID, StepKey: "media",
		StepOutput: json.RawMessage(`{"request_id":"req-1"}`), GatewayRequestID: "req-1",
	})
	require.ErrorIs(t, err, service.ErrApplicationStaleWrite)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplicationCenterRepositoryCheckpointStepKeepsRunActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewApplicationCenterRepository(db)
	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, COALESCE(current_step_key, ''), version")).
		WithArgs(runID, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step_key", "version"}).
			AddRow(service.ApplicationRunStatusRunning, "media", int64(4)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status, version")).
		WithArgs(runID, "media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version"}).
			AddRow(int64(10), service.ApplicationRunStatusPending, int64(3)))
	mock.ExpectExec("(?s)UPDATE application_run_steps.*status = 'running'.*WHERE id = \\$1.*version = \\$5.*\\(gateway_request_id IS NULL OR gateway_request_id = \\$3\\)").
		WithArgs(int64(10), `{"request_id":"video-1","status":"processing"}`, "video-1", sqlmock.AnyArg(), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?s)UPDATE application_runs.*updated_at = \\$3.*version = \\$4").
		WithArgs(runID, int64(7), sqlmock.AnyArg(), int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.CheckpointStep(context.Background(), service.CheckpointApplicationStepParams{
		UserID: 7, RunID: runID, StepKey: "media",
		StepOutput:       json.RawMessage(`{"request_id":"video-1","status":"processing"}`),
		GatewayRequestID: "video-1",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplicationCenterRepositoryCheckpointStepRejectsDifferentPersistedRequestID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewApplicationCenterRepository(db)
	runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, COALESCE(current_step_key, ''), version")).
		WithArgs(runID, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step_key", "version"}).
			AddRow(service.ApplicationRunStatusRunning, "media", int64(5)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status, version")).
		WithArgs(runID, "media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version"}).
			AddRow(int64(10), service.ApplicationRunStatusRunning, int64(4)))
	mock.ExpectExec("(?s)UPDATE application_run_steps.*\\(gateway_request_id IS NULL OR gateway_request_id = \\$3\\)").
		WithArgs(int64(10), `{"request_id":"video-2","status":"processing"}`, "video-2", sqlmock.AnyArg(), int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = repo.CheckpointStep(context.Background(), service.CheckpointApplicationStepParams{
		UserID: 7, RunID: runID, StepKey: "media",
		StepOutput:       json.RawMessage(`{"request_id":"video-2","status":"processing"}`),
		GatewayRequestID: "video-2",
	})
	require.ErrorIs(t, err, service.ErrApplicationStaleWrite)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplicationCenterRepositoryRetryKeepsOnlyRecoverableMediaCheckpoint(t *testing.T) {
	tests := []struct {
		name                 string
		executor             string
		gatewayRequestID     string
		output               string
		wantOutput           string
		wantGatewayRequestID string
	}{
		{
			name:                 "processing video task remains resumable",
			executor:             service.ApplicationExecutorVideoGeneration,
			gatewayRequestID:     "video-1",
			output:               `{"request_id":"video-1","status":"processing"}`,
			wantOutput:           `{"request_id":"video-1","status":"processing"}`,
			wantGatewayRequestID: "video-1",
		},
		{
			name:             "failed video task is cleared before retry",
			executor:         service.ApplicationExecutorVideoGeneration,
			gatewayRequestID: "video-1",
			output:           `{"request_id":"video-1","status":"failed"}`,
		},
		{
			name:             "expired video task is cleared before retry",
			executor:         service.ApplicationExecutorVideoGeneration,
			gatewayRequestID: "video-1",
			output:           `{"request_id":"video-1","status":"expired"}`,
		},
		{
			name:                 "processing image task remains resumable",
			executor:             service.ApplicationExecutorImageGeneration,
			gatewayRequestID:     "image-1",
			output:               `{"task_id":"image-1","status":"processing"}`,
			wantOutput:           `{"task_id":"image-1","status":"processing"}`,
			wantGatewayRequestID: "image-1",
		},
		{
			name:             "completed image task is cleared before retry",
			executor:         service.ApplicationExecutorImageGeneration,
			gatewayRequestID: "image-1",
			output:           `{"task_id":"image-1","status":"completed"}`,
		},
		{
			name:             "failed image task is cleared before retry",
			executor:         service.ApplicationExecutorImageEdit,
			gatewayRequestID: "image-1",
			output:           `{"task_id":"image-1","status":"failed"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			repo := NewApplicationCenterRepository(db)
			runID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT status, COALESCE(current_step_key, ''), version")).
				WithArgs(runID, int64(7)).
				WillReturnRows(sqlmock.NewRows([]string{"status", "current_step_key", "version"}).
					AddRow(service.ApplicationRunStatusFailed, "media", int64(4)))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT id, status, version, executor, output, COALESCE(gateway_request_id, '')")).
				WithArgs(runID, "media").
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "status", "version", "executor", "output", "gateway_request_id",
				}).AddRow(
					int64(10), service.ApplicationRunStatusFailed, int64(3),
					tt.executor, []byte(tt.output), tt.gatewayRequestID,
				))
			mock.ExpectExec("(?s)UPDATE application_run_steps.*output = NULLIF\\(\\$2, ''\\)::jsonb.*gateway_request_id = NULLIF\\(\\$3, ''\\).*WHERE id = \\$1 AND version = \\$5").
				WithArgs(int64(10), tt.wantOutput, tt.wantGatewayRequestID, sqlmock.AnyArg(), int64(3)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("(?s)UPDATE application_runs.*status = 'running'.*retry_count = retry_count \\+ 1.*WHERE id = \\$1::uuid AND user_id = \\$2 AND version = \\$4").
				WithArgs(runID, int64(7), sqlmock.AnyArg(), int64(4)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			err = repo.Retry(context.Background(), 7, runID)
			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
