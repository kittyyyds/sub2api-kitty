package repository

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.OpenAIVMManagedRepository = (*openAIVMRepository)(nil)

func (r *openAIVMRepository) ReserveManaged(ctx context.Context, id int64, runtime string, p *service.Proxy) (*service.OpenAIVMBinding, error) {
	return r.reserve(ctx, id, runtime, service.OpenAIVMModeManaged, p)
}

// SetManaged records what the bridge last reported. A nil detail keeps the
// previous snapshot, so a failed read does not blank the administrator's view.
func (r *openAIVMRepository) SetManaged(ctx context.Context, id int64, status string, detail json.RawMessage) error {
	var raw any
	if len(detail) > 0 {
		raw = string(detail)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE openai_vm_bindings SET managed_status=$2,managed_detail=COALESCE($3::jsonb,managed_detail),synced_at=NOW(),updated_at=NOW() WHERE account_id=$1 AND mode='vm_managed'`, id, status, raw)
	return err
}

func (r *openAIVMRepository) MarkActivated(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE openai_vm_bindings SET activated_at=NOW(),updated_at=NOW() WHERE account_id=$1 AND mode='vm_managed' AND activated_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *openAIVMRepository) ListManaged(ctx context.Context) ([]*service.OpenAIVMBinding, error) {
	rows, err := r.db.QueryContext(ctx, openAIVMBindingSelect+` WHERE mode='vm_managed' ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.OpenAIVMBinding
	for rows.Next() {
		b, err := scanOpenAIVMBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetOriginalProxy follows a proxy change the bridge has already applied: the
// "original" proxy of a managed binding is the exit its slot uses.
func (r *openAIVMRepository) SetOriginalProxy(ctx context.Context, id, proxyID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE openai_vm_bindings SET original_proxy_id=$2,updated_at=NOW() WHERE account_id=$1 AND mode='vm_managed'`, id, proxyID)
	return err
}

// ReleaseManaged removes a managed binding once the bridge has destroyed its
// slot, so the account row can be deleted. The account keeps its managed mark
// and is left unschedulable: with neither a slot nor a token it can serve
// nothing, and the mark keeps it out of every token path until it is gone.
func (r *openAIVMRepository) ReleaseManaged(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Same parent-first lock order as Reserve and Restore.
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return err
	}
	b, err := scanOpenAIVMBinding(tx.QueryRowContext(ctx, openAIVMBindingSelect+` WHERE account_id=$1 AND mode='vm_managed' FOR UPDATE`, id))
	if err != nil || b == nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM openai_vm_bindings WHERE account_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET proxy_id=$2,proxy_fallback_origin_id=$3,schedulable=FALSE,extra=COALESCE(extra,'{}'::jsonb)-'openai_vm_bound',updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id, b.OriginalProxyID, b.OriginalFallbackID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE proxies SET deleted_at=NOW(),status='disabled',updated_at=NOW() WHERE id=$1`, b.ManagedProxyID); err != nil {
		return err
	}
	if err = enqueueOpenAIVMSnapshots(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}
