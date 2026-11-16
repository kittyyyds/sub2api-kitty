package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.OpenAIVMBatchRepository = (*openAIVMRepository)(nil)

// NewOpenAIVMBatchRepository exposes the batch-import tables of the binding store.
func NewOpenAIVMBatchRepository(db *sql.DB) service.OpenAIVMBatchRepository {
	return &openAIVMRepository{db: db}
}

func (r *openAIVMRepository) CreateBatch(ctx context.Context, createdBy int64, runtime string, options json.RawMessage, items []service.OpenAIVMBatchItem) (*service.OpenAIVMBatchJob, error) {
	if len(options) == 0 {
		options = json.RawMessage(`{}`)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var creator any
	if createdBy > 0 {
		creator = createdBy
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO openai_vm_batch_jobs(created_by,status,runtime_type,total,options) VALUES($1,'running',$2,$3,$4::jsonb) RETURNING id`, creator, runtime, len(items), string(options)).Scan(&id); err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO openai_vm_batch_items(job_id,line_no,email,proxy_id,status,error_code) VALUES($1,$2,$3,$4,$5,$6)`, id, item.LineNo, item.Email, item.ProxyID, item.Status, item.ErrorCode); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetBatch(ctx, id)
}

func (r *openAIVMRepository) GetBatch(ctx context.Context, id int64) (*service.OpenAIVMBatchJob, error) {
	job := &service.OpenAIVMBatchJob{}
	err := r.db.QueryRowContext(ctx, `SELECT id,status,runtime_type,total,created_at,finished_at FROM openai_vm_batch_jobs WHERE id=$1`, id).Scan(&job.ID, &job.Status, &job.RuntimeType, &job.Total, &job.CreatedAt, &job.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,line_no,email,proxy_id,account_id,status,error_code,updated_at FROM openai_vm_batch_items WHERE job_id=$1 ORDER BY line_no`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	job.Items = []service.OpenAIVMBatchItem{}
	for rows.Next() {
		var item service.OpenAIVMBatchItem
		if err = rows.Scan(&item.ID, &item.LineNo, &item.Email, &item.ProxyID, &item.AccountID, &item.Status, &item.ErrorCode, &item.UpdatedAt); err != nil {
			return nil, err
		}
		job.Items = append(job.Items, item)
	}
	return job, rows.Err()
}

// SetBatchItem records one line's progress. A nil accountID keeps the one already stored.
func (r *openAIVMRepository) SetBatchItem(ctx context.Context, itemID int64, status, errorCode string, accountID *int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE openai_vm_batch_items SET status=$2,error_code=$3,account_id=COALESCE($4,account_id),updated_at=NOW() WHERE id=$1`, itemID, status, errorCode, accountID)
	return err
}

func (r *openAIVMRepository) FinishBatch(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE openai_vm_batch_jobs SET status=$2,finished_at=NOW(),updated_at=NOW() WHERE id=$1`, id, status)
	return err
}

// InterruptUnfinishedBatches runs at start-up: the passwords and 2FA secrets
// of unfinished lines were only in the previous process's memory.
func (r *openAIVMRepository) InterruptUnfinishedBatches(ctx context.Context) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE openai_vm_batch_items SET status='interrupted',updated_at=NOW() WHERE status IN ('queued','creating','logging_in') AND job_id IN (SELECT id FROM openai_vm_batch_jobs WHERE status IN ('queued','running'))`)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE openai_vm_batch_jobs SET status='interrupted',finished_at=NOW(),updated_at=NOW() WHERE status IN ('queued','running')`); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ManagedEmails maps the lower-cased login email of every VM2-managed account
// to its account ID. The email is the account's name until its first login and
// its credential email afterwards; both are checked.
func (r *openAIVMRepository) ManagedEmails(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,COALESCE(credentials->>'email','') FROM accounts WHERE deleted_at IS NULL AND platform='openai' AND type='oauth' AND COALESCE(extra->>'openai_vm_managed','')='true'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var name, email string
		if err = rows.Scan(&id, &name, &email); err != nil {
			return nil, err
		}
		for _, value := range []string{name, email} {
			if value = strings.ToLower(strings.TrimSpace(value)); strings.Contains(value, "@") {
				out[value] = id
			}
		}
	}
	return out, rows.Err()
}

func (r *openAIVMRepository) ManagedAccountsPerProxy(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT original_proxy_id,COUNT(*) FROM openai_vm_bindings WHERE mode='vm_managed' AND original_proxy_id IS NOT NULL GROUP BY original_proxy_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var count int
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}
