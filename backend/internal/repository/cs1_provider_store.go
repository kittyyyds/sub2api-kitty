package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const cs1ProviderPrefix = "kitty_upstream_provider_v1:"

func (r *channelRepository) ReadCS1UserBalance(ctx context.Context) (float64, error) {
	var amount float64
	e := r.db.QueryRowContext(ctx, `SELECT COALESCE(sum(balance),0) FROM users WHERE role='user' AND deleted_at IS NULL`).Scan(&amount)
	return amount, e
}

func (r *channelRepository) ReadCS1Providers(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	e := r.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(value::jsonb ORDER BY updated_at DESC,key),'[]') FROM settings WHERE key LIKE 'kitty_upstream_provider_v1:%' AND NOT COALESCE((value::jsonb->>'deleted')::boolean,false)`).Scan(&raw)
	return raw, e
}

func (r *channelRepository) SaveCS1Provider(ctx context.Context, id string, revision int64, raw json.RawMessage) (json.RawMessage, error) {
	var v struct {
		Name, SiteType, BaseURL, Notes string
		WalletID                       int64
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil || len(id) != 24 || revision < 0 {
		return nil, service.ErrProviderHubInput
	}
	str := func(key string) string { x, _ := obj[key].(string); return x }
	v.Name = str("name")
	v.SiteType = str("site_type")
	v.BaseURL = str("base_url")
	v.Notes = str("notes")
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, cs1ProviderPrefix+id); e != nil {
		return nil, e
	}
	var prev string
	e = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, cs1ProviderPrefix+id).Scan(&prev)
	var old map[string]any
	var supplierID, walletID int64
	if errors.Is(e, sql.ErrNoRows) {
		if revision != 0 {
			return nil, service.ErrProviderHubConflict
		}
		e = tx.QueryRowContext(ctx, `INSERT INTO kitty_suppliers(name,site_type,base_url,notes) VALUES($1,$2,$3,$4) RETURNING id`, v.Name, v.SiteType, v.BaseURL, v.Notes).Scan(&supplierID)
		if e != nil {
			return nil, e
		}
		e = tx.QueryRowContext(ctx, `INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency) VALUES($1,$2,$3,'CNY','USD') RETURNING id`, supplierID, v.Name, id).Scan(&walletID)
		if e != nil {
			return nil, e
		}
	} else {
		if e != nil {
			return nil, e
		}
		if json.Unmarshal([]byte(prev), &old) != nil {
			return nil, service.ErrProviderHubUnavailable
		}
		oldRevision, _ := old["revision"].(float64)
		deleted, _ := old["deleted"].(bool)
		if deleted || int64(oldRevision) != revision {
			return nil, service.ErrProviderHubConflict
		}
		sid, _ := old["supplier_id"].(float64)
		wid, _ := old["wallet_id"].(float64)
		supplierID = int64(sid)
		walletID = int64(wid)
		// Imported accounts keep their explicit URL/credentials. Moving a saved
		// supplier to a new account does not silently change those accounts.
		_, e = tx.ExecContext(ctx, `UPDATE kitty_suppliers SET name=$2,site_type=$3,base_url=$4,notes=$5,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, supplierID, v.Name, v.SiteType, v.BaseURL, v.Notes)
		if e != nil {
			return nil, e
		}
	}
	obj["revision"] = revision + 1
	obj["supplier_id"] = supplierID
	obj["wallet_id"] = walletID
	raw, e = json.Marshal(obj)
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,clock_timestamp()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at`, cs1ProviderPrefix+id, string(raw))
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE kitty_supplier_wallets SET name=$2,balance=($3::jsonb->'metrics'->>'balance')::numeric,balance_unit=COALESCE($3::jsonb->'metrics'->>'currency','USD'),balance_source='provider_dashboard',observed_at=($3::jsonb->>'updated_at')::timestamptz,attempted_at=clock_timestamp(),sync_error=COALESCE($3::jsonb->>'last_error',''),revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, walletID, v.Name, string(raw))
	if e != nil {
		return nil, e
	}
	return raw, tx.Commit()
}

func (r *channelRepository) DeleteCS1Provider(ctx context.Context, id string, revision int64) error {
	// Recoverable tombstone. Procurement and imported accounts are retained;
	// the dashboard credential is removed from the active provider record.
	result, e := r.db.ExecContext(ctx, `UPDATE settings SET value=((value::jsonb-'secret_encrypted')||jsonb_build_object('deleted',true,'revision',$3::bigint+1))::text,updated_at=clock_timestamp() WHERE key=$1 AND (value::jsonb->>'revision')::bigint=$2 AND NOT COALESCE((value::jsonb->>'deleted')::boolean,false)`, cs1ProviderPrefix+id, revision, revision)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n != 1 {
		return service.ErrProviderHubConflict
	}
	return e
}
