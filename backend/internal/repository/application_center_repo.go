package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type applicationCenterRepository struct {
	db *sql.DB
}

func NewApplicationCenterRepository(db *sql.DB) service.ApplicationRunRepository {
	return &applicationCenterRepository{db: db}
}

const applicationRunSelectColumns = `
	id::text, user_id, app_slug, app_revision, status,
	planner_api_key_id, media_api_key_id, session_id,
	COALESCE(current_step_key, ''), input, output,
	COALESCE(error_code, ''), COALESCE(error_message, ''),
	retry_count, version, created_at, updated_at, completed_at, cancelled_at`

const applicationStepSelectColumns = `
	id, run_id::text, step_key, sequence, executor, credential_role, status,
	request, output, COALESCE(gateway_request_id, ''),
	COALESCE(error_code, ''), COALESCE(error_message, ''),
	attempt, version, started_at, completed_at, created_at, updated_at`

func (r *applicationCenterRepository) Create(ctx context.Context, params service.CreateApplicationRunParams) (err error) {
	if params.Run == nil || params.InitialStep == nil {
		return service.ErrApplicationInvalidInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	run := params.Run
	if _, err = tx.ExecContext(ctx, `
INSERT INTO application_runs (
    id, user_id, app_slug, app_revision, status,
    planner_api_key_id, media_api_key_id, session_id, current_step_key,
    input, version, created_at, updated_at
) VALUES (
    $1::uuid, $2, $3, $4, $5,
    $6, $7, $8, NULLIF($9, ''),
    $10::jsonb, $11, $12, $13
)`, run.ID, run.UserID, run.AppSlug, run.AppRevision, run.Status,
		run.PlannerAPIKeyID, run.MediaAPIKeyID, run.SessionID, run.CurrentStepKey,
		string(run.Input), run.Version, run.CreatedAt, run.UpdatedAt); err != nil {
		return err
	}

	step := params.InitialStep
	if _, err = tx.ExecContext(ctx, `
INSERT INTO application_run_steps (
    run_id, step_key, sequence, executor, credential_role, status,
    request, attempt, version, created_at, updated_at
) VALUES (
    $1::uuid, $2, $3, $4, $5, $6,
    $7::jsonb, $8, $9, $10, $11
)`, run.ID, step.StepKey, step.Sequence, step.Executor, step.CredentialRole, step.Status,
		string(step.Request), step.Attempt, step.Version, step.CreatedAt, step.UpdatedAt); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) GetByID(ctx context.Context, userID int64, runID string) (*service.ApplicationRun, error) {
	run, err := scanApplicationRun(r.db.QueryRowContext(ctx, `
SELECT `+applicationRunSelectColumns+`
FROM application_runs
WHERE id = $1::uuid AND user_id = $2`, runID, userID))
	if err != nil {
		return nil, translateApplicationRunError(err)
	}
	steps, err := r.listSteps(ctx, []string{run.ID})
	if err != nil {
		return nil, err
	}
	run.Steps = steps[run.ID]
	if run.Steps == nil {
		run.Steps = make([]*service.ApplicationRunStep, 0)
	}
	return run, nil
}

func (r *applicationCenterRepository) List(
	ctx context.Context,
	userID int64,
	params pagination.PaginationParams,
	appSlug string,
) ([]*service.ApplicationRun, *pagination.PaginationResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	where := " WHERE user_id = $1"
	countArgs := []any{userID}
	if appSlug != "" {
		where += " AND app_slug = $2"
		countArgs = append(countArgs, appSlug)
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_runs"+where, countArgs...).Scan(&total); err != nil {
		return nil, nil, err
	}

	queryArgs := append([]any{}, countArgs...)
	limitPosition := len(queryArgs) + 1
	offsetPosition := len(queryArgs) + 2
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT `+applicationRunSelectColumns+`
FROM application_runs`+where+`
ORDER BY created_at DESC, id DESC
LIMIT $`+fmt.Sprint(limitPosition)+` OFFSET $`+fmt.Sprint(offsetPosition), queryArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]*service.ApplicationRun, 0, pageSize)
	runIDs := make([]string, 0, pageSize)
	for rows.Next() {
		run, scanErr := scanApplicationRun(rows)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		runs = append(runs, run)
		runIDs = append(runIDs, run.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	steps, err := r.listSteps(ctx, runIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, run := range runs {
		run.Steps = steps[run.ID]
		if run.Steps == nil {
			run.Steps = make([]*service.ApplicationRunStep, 0)
		}
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return runs, &pagination.PaginationResult{
		Total: total, Page: page, PageSize: pageSize, Pages: pages,
	}, nil
}

func (r *applicationCenterRepository) CompleteStep(ctx context.Context, params service.CompleteApplicationStepParams) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	runStatus, currentStep, runVersion, err := lockApplicationRun(ctx, tx, params.UserID, params.RunID)
	if err != nil {
		return err
	}
	if runStatus != service.ApplicationRunStatusRunning || currentStep != params.StepKey {
		return service.ErrApplicationInvalidTransition
	}
	stepID, stepStatus, stepVersion, err := lockApplicationStep(ctx, tx, params.RunID, params.StepKey)
	if err != nil {
		return err
	}
	if stepStatus != service.ApplicationRunStatusPending && stepStatus != service.ApplicationRunStatusRunning {
		return service.ErrApplicationInvalidTransition
	}

	now := time.Now()
	stepResult, err := tx.ExecContext(ctx, `
UPDATE application_run_steps
SET status = 'completed',
    output = NULLIF($2, '')::jsonb,
    gateway_request_id = COALESCE(NULLIF($3, ''), gateway_request_id),
    error_code = NULL,
    error_message = NULL,
    started_at = COALESCE(started_at, $4),
    completed_at = $4,
    updated_at = $4,
    version = version + 1
WHERE id = $1
  AND version = $5
  AND (NULLIF($3, '') IS NULL OR gateway_request_id IS NULL OR gateway_request_id = $3)`, stepID, applicationJSONArg(params.StepOutput),
		params.GatewayRequestID, now, stepVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(stepResult); err != nil {
		return err
	}

	if params.NextStep != nil {
		next := params.NextStep
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application_run_steps (
    run_id, step_key, sequence, executor, credential_role, status,
    request, attempt, version, created_at, updated_at
) VALUES (
    $1::uuid, $2, $3, $4, $5, $6,
    $7::jsonb, $8, $9, $10, $11
)`, params.RunID, next.StepKey, next.Sequence, next.Executor, next.CredentialRole,
			next.Status, string(next.Request), next.Attempt, next.Version, next.CreatedAt, next.UpdatedAt); err != nil {
			return err
		}
		runResult, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET current_step_key = $2,
    error_code = NULL,
    error_message = NULL,
    updated_at = $3,
    version = version + 1
WHERE id = $1::uuid AND user_id = $4 AND version = $5`,
			params.RunID, next.StepKey, now, params.UserID, runVersion)
		if err != nil {
			return err
		}
		if err := requireApplicationRowsAffected(runResult); err != nil {
			return err
		}
	} else {
		runResult, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET status = 'completed',
    current_step_key = NULL,
    output = NULLIF($2, '')::jsonb,
    error_code = NULL,
    error_message = NULL,
    completed_at = $3,
    updated_at = $3,
    version = version + 1
WHERE id = $1::uuid AND user_id = $4 AND version = $5`,
			params.RunID, applicationJSONArg(params.RunOutput), now, params.UserID, runVersion)
		if err != nil {
			return err
		}
		if err := requireApplicationRowsAffected(runResult); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) CheckpointStep(ctx context.Context, params service.CheckpointApplicationStepParams) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	runStatus, currentStep, runVersion, err := lockApplicationRun(ctx, tx, params.UserID, params.RunID)
	if err != nil {
		return err
	}
	if runStatus != service.ApplicationRunStatusRunning || currentStep != params.StepKey {
		return service.ErrApplicationInvalidTransition
	}
	stepID, stepStatus, stepVersion, err := lockApplicationStep(ctx, tx, params.RunID, params.StepKey)
	if err != nil {
		return err
	}
	if stepStatus != service.ApplicationRunStatusPending && stepStatus != service.ApplicationRunStatusRunning {
		return service.ErrApplicationInvalidTransition
	}

	now := time.Now()
	stepResult, err := tx.ExecContext(ctx, `
UPDATE application_run_steps
SET status = 'running',
    output = COALESCE(NULLIF($2, '')::jsonb, output),
    gateway_request_id = $3,
    error_code = NULL,
    error_message = NULL,
    started_at = COALESCE(started_at, $4),
    updated_at = $4,
    version = version + 1
WHERE id = $1
  AND version = $5
  AND (gateway_request_id IS NULL OR gateway_request_id = $3)`, stepID, applicationJSONArg(params.StepOutput),
		params.GatewayRequestID, now, stepVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(stepResult); err != nil {
		return err
	}
	runResult, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET updated_at = $3, version = version + 1
WHERE id = $1::uuid AND user_id = $2 AND version = $4`, params.RunID, params.UserID, now, runVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(runResult); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) FailStep(ctx context.Context, params service.FailApplicationStepParams) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	runStatus, currentStep, runVersion, err := lockApplicationRun(ctx, tx, params.UserID, params.RunID)
	if err != nil {
		return err
	}
	if runStatus != service.ApplicationRunStatusRunning || currentStep != params.StepKey {
		return service.ErrApplicationInvalidTransition
	}
	stepID, stepStatus, stepVersion, err := lockApplicationStep(ctx, tx, params.RunID, params.StepKey)
	if err != nil {
		return err
	}
	if stepStatus != service.ApplicationRunStatusPending && stepStatus != service.ApplicationRunStatusRunning {
		return service.ErrApplicationInvalidTransition
	}

	now := time.Now()
	stepResult, err := tx.ExecContext(ctx, `
UPDATE application_run_steps
SET status = 'failed',
    output = COALESCE(NULLIF($2, '')::jsonb, output),
    gateway_request_id = COALESCE(NULLIF($3, ''), gateway_request_id),
    error_code = NULLIF($4, ''),
    error_message = NULLIF($5, ''),
    started_at = COALESCE(started_at, $6),
    completed_at = $6,
    updated_at = $6,
    version = version + 1
WHERE id = $1
  AND version = $7
  AND (NULLIF($3, '') IS NULL OR gateway_request_id IS NULL OR gateway_request_id = $3)`, stepID, applicationJSONArg(params.StepOutput), params.GatewayRequestID,
		params.ErrorCode, params.ErrorMessage, now, stepVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(stepResult); err != nil {
		return err
	}
	runResult, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET status = 'failed',
    error_code = NULLIF($2, ''),
    error_message = NULLIF($3, ''),
    updated_at = $4,
    version = version + 1
WHERE id = $1::uuid AND user_id = $5 AND version = $6`,
		params.RunID, params.ErrorCode, params.ErrorMessage, now, params.UserID, runVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(runResult); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) Cancel(ctx context.Context, userID int64, runID string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	status, currentStep, version, err := lockApplicationRun(ctx, tx, userID, runID)
	if err != nil {
		return err
	}
	if status == service.ApplicationRunStatusCompleted || status == service.ApplicationRunStatusCancelled {
		return service.ErrApplicationInvalidTransition
	}
	now := time.Now()
	if currentStep != "" {
		if _, err := tx.ExecContext(ctx, `
UPDATE application_run_steps
SET status = 'cancelled', completed_at = COALESCE(completed_at, $3), updated_at = $3, version = version + 1
WHERE run_id = $1::uuid AND step_key = $2 AND status IN ('pending', 'running', 'failed')`,
			runID, currentStep, now); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET status = 'cancelled', current_step_key = NULL,
    error_code = NULL, error_message = NULL,
    cancelled_at = $3, updated_at = $3, version = version + 1
WHERE id = $1::uuid AND user_id = $2 AND version = $4`, runID, userID, now, version)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(result); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) Retry(ctx context.Context, userID int64, runID string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	status, currentStep, version, err := lockApplicationRun(ctx, tx, userID, runID)
	if err != nil {
		return err
	}
	if status != service.ApplicationRunStatusFailed || currentStep == "" {
		return service.ErrApplicationInvalidTransition
	}
	stepID, stepStatus, stepVersion, executor, stepOutput, gatewayRequestID, err :=
		lockApplicationStepForRetry(ctx, tx, runID, currentStep)
	if err != nil {
		return err
	}
	if stepStatus != service.ApplicationRunStatusFailed {
		return service.ErrApplicationInvalidTransition
	}
	retainedOutput := ""
	retainedGatewayRequestID := ""
	if service.IsRecoverableApplicationMediaCheckpoint(executor, stepOutput, gatewayRequestID) {
		retainedOutput = string(stepOutput)
		retainedGatewayRequestID = gatewayRequestID
	}
	now := time.Now()
	stepResult, err := tx.ExecContext(ctx, `
UPDATE application_run_steps
SET status = 'pending',
    output = NULLIF($2, '')::jsonb,
    gateway_request_id = NULLIF($3, ''),
    error_code = NULL, error_message = NULL,
    attempt = attempt + 1, started_at = NULL, completed_at = NULL,
    updated_at = $4, version = version + 1
WHERE id = $1 AND version = $5`, stepID, retainedOutput, retainedGatewayRequestID, now, stepVersion)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(stepResult); err != nil {
		return err
	}
	runResult, err := tx.ExecContext(ctx, `
UPDATE application_runs
SET status = 'running', error_code = NULL, error_message = NULL,
    completed_at = NULL, cancelled_at = NULL,
    retry_count = retry_count + 1, updated_at = $3, version = version + 1
WHERE id = $1::uuid AND user_id = $2 AND version = $4`, runID, userID, now, version)
	if err != nil {
		return err
	}
	if err := requireApplicationRowsAffected(runResult); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *applicationCenterRepository) listSteps(ctx context.Context, runIDs []string) (map[string][]*service.ApplicationRunStep, error) {
	out := make(map[string][]*service.ApplicationRunStep, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT `+applicationStepSelectColumns+`
FROM application_run_steps
WHERE run_id = ANY($1::uuid[])
ORDER BY run_id, sequence, id`, pq.Array(runIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		step, scanErr := scanApplicationStep(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out[step.RunID] = append(out[step.RunID], step)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type applicationScanner interface {
	Scan(dest ...any) error
}

func scanApplicationRun(scanner applicationScanner) (*service.ApplicationRun, error) {
	run := &service.ApplicationRun{}
	var input, output []byte
	var plannerAPIKeyID sql.NullInt64
	if err := scanner.Scan(
		&run.ID, &run.UserID, &run.AppSlug, &run.AppRevision, &run.Status,
		&plannerAPIKeyID, &run.MediaAPIKeyID, &run.SessionID, &run.CurrentStepKey,
		&input, &output, &run.ErrorCode, &run.ErrorMessage,
		&run.RetryCount, &run.Version, &run.CreatedAt, &run.UpdatedAt, &run.CompletedAt, &run.CancelledAt,
	); err != nil {
		return nil, err
	}
	if plannerAPIKeyID.Valid {
		run.PlannerAPIKeyID = &plannerAPIKeyID.Int64
	}
	run.Input = cloneApplicationJSONBytes(input)
	run.Output = cloneApplicationJSONBytes(output)
	return run, nil
}

func scanApplicationStep(scanner applicationScanner) (*service.ApplicationRunStep, error) {
	step := &service.ApplicationRunStep{}
	var request, output []byte
	if err := scanner.Scan(
		&step.ID, &step.RunID, &step.StepKey, &step.Sequence, &step.Executor, &step.CredentialRole, &step.Status,
		&request, &output, &step.GatewayRequestID, &step.ErrorCode, &step.ErrorMessage,
		&step.Attempt, &step.Version, &step.StartedAt, &step.CompletedAt, &step.CreatedAt, &step.UpdatedAt,
	); err != nil {
		return nil, err
	}
	step.Request = cloneApplicationJSONBytes(request)
	step.Output = cloneApplicationJSONBytes(output)
	return step, nil
}

func lockApplicationRun(ctx context.Context, tx *sql.Tx, userID int64, runID string) (status string, currentStep string, version int64, err error) {
	err = tx.QueryRowContext(ctx, `
SELECT status, COALESCE(current_step_key, ''), version
FROM application_runs
WHERE id = $1::uuid AND user_id = $2
FOR UPDATE`, runID, userID).Scan(&status, &currentStep, &version)
	if err != nil {
		err = translateApplicationRunError(err)
	}
	return
}

func lockApplicationStep(ctx context.Context, tx *sql.Tx, runID, stepKey string) (id int64, status string, version int64, err error) {
	err = tx.QueryRowContext(ctx, `
SELECT id, status, version
FROM application_run_steps
WHERE run_id = $1::uuid AND step_key = $2
FOR UPDATE`, runID, stepKey).Scan(&id, &status, &version)
	if errors.Is(err, sql.ErrNoRows) {
		err = service.ErrApplicationStepNotFound
	}
	return
}

func lockApplicationStepForRetry(
	ctx context.Context,
	tx *sql.Tx,
	runID, stepKey string,
) (
	id int64,
	status string,
	version int64,
	executor string,
	output []byte,
	gatewayRequestID string,
	err error,
) {
	err = tx.QueryRowContext(ctx, `
SELECT id, status, version, executor, output, COALESCE(gateway_request_id, '')
FROM application_run_steps
WHERE run_id = $1::uuid AND step_key = $2
FOR UPDATE`, runID, stepKey).Scan(
		&id,
		&status,
		&version,
		&executor,
		&output,
		&gatewayRequestID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		err = service.ErrApplicationStepNotFound
	}
	return
}

func translateApplicationRunError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrApplicationRunNotFound
	}
	return err
}

func requireApplicationRowsAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return service.ErrApplicationStaleWrite
	}
	return nil
}

func applicationJSONArg(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func cloneApplicationJSONBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}
