package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *accountRepository) GetProxyBindingPolicy(ctx context.Context) (service.ProxyBindingPolicy, error) {
	var p service.ProxyBindingPolicy
	db, ok := r.sql.(planRowQuerier)
	if !ok {
		return p, errors.New("IP 规则仓储未就绪")
	}
	var raw []byte
	if e := db.QueryRowContext(ctx, `SELECT config FROM proxy_binding_policy WHERE id=true`).Scan(&raw); e != nil {
		return p, e
	}
	e := json.Unmarshal(raw, &p)
	return p, e
}
func (r *accountRepository) SaveProxyBindingPolicy(ctx context.Context, p service.ProxyBindingPolicy) error {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return errors.New("IP 规则需要事务")
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71942021)`); e != nil {
		return e
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE proxy_binding_policy SET config=$1,updated_at=NOW() WHERE id=true`, string(raw)); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *accountRepository) RememberProxyBindingExit(ctx context.Context, id int64, ip string, version time.Time) error {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return errors.New("IP 出口记录需要事务")
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71942021)`); e != nil {
		return e
	}
	if ip == "" {
		_, e = tx.ExecContext(ctx, `UPDATE proxy_binding_exits SET checked_at='-infinity' WHERE proxy_id=$1 AND proxy_revision=$2`, id, version)
		if e != nil {
			return e
		}
		return tx.Commit()
	}
	var valid bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM proxies WHERE id=$1 AND updated_at=$2 AND deleted_at IS NULL)`, id, version).Scan(&valid); e != nil {
		return e
	}
	if !valid {
		return tx.Commit()
	}
	// Keep historical IP buckets when an endpoint changes; current membership is recalculated.
	_, e = tx.ExecContext(ctx, `INSERT INTO proxy_binding_exits(proxy_id,exit_ip,proxy_revision) SELECT id,$2,updated_at FROM proxies WHERE id=$1 AND updated_at=$3 AND deleted_at IS NULL ON CONFLICT(proxy_id) DO UPDATE SET exit_ip=excluded.exit_ip,proxy_revision=excluded.proxy_revision,checked_at=NOW()`, id, ip, version)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO proxy_binding_history(proxy_key,identity_key,platform,account_type,source,first_bound_at,last_bound_at) SELECT 'ip:'||$2,identity_key,platform,account_type,source,first_bound_at,last_bound_at FROM proxy_binding_history WHERE proxy_key='proxy:'||$1::text ON CONFLICT DO NOTHING`, id, ip)
	if e != nil {
		return e
	}
	// An address rotation also counts the accounts currently using the new exit.
	_, e = tx.ExecContext(ctx, `INSERT INTO proxy_binding_history(proxy_key,identity_key,platform,account_type,source) SELECT 'ip:'||$2,COALESCE(o.identity_key,md5(a.platform||'|'||COALESCE(NULLIF(lower(trim(a.credentials->>'email')),''),'account:'||a.id::text))),a.platform,a.type,COALESCE(o.source,'unknown') FROM accounts a LEFT JOIN account_import_origins o ON o.account_id=a.id WHERE a.proxy_id=$1 AND a.deleted_at IS NULL ON CONFLICT DO NOTHING`, id, ip)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (r *accountRepository) ProxyBindingSummaries(ctx context.Context) ([]service.ProxyBindingSummary, error) {
	rows, e := r.sql.QueryContext(ctx, `SELECT p.id,kitty_proxy_history_count(kitty_proxy_key(p.id),c.config),(SELECT count(*) FROM accounts a WHERE a.deleted_at IS NULL AND a.proxy_id IS NOT NULL AND kitty_proxy_key(a.proxy_id)=kitty_proxy_key(p.id)),(SELECT count(*) FROM account_import_tickets t WHERE t.state IN ('pending','ready') AND t.expires_at>NOW() AND kitty_proxy_key(t.proxy_id)=kitty_proxy_key(p.id)),(c.config->>'history_limit')::int,COALESCE((SELECT available FROM proxy_binding_allowance WHERE proxy_key=kitty_proxy_key(p.id)),false),COALESCE(x.proxy_revision=p.updated_at AND x.checked_at>NOW()-interval '10 minutes',false),COALESCE(x.exit_ip,'') FROM proxies p CROSS JOIN proxy_binding_policy c LEFT JOIN proxy_binding_exits x ON x.proxy_id=p.id WHERE p.deleted_at IS NULL ORDER BY p.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []service.ProxyBindingSummary{}
	for rows.Next() {
		var s service.ProxyBindingSummary
		if e = rows.Scan(&s.ProxyID, &s.History, &s.Current, &s.Reserved, &s.Limit, &s.ExtraAllowed, &s.ExitVerified, &s.ExitIP); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *accountRepository) AllowOneProxyBinding(ctx context.Context, id int64) error {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return errors.New("IP 放行需要事务")
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71942021)`); e != nil {
		return e
	}
	var available bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM proxies WHERE id=$1 AND status IN ('active','inactive') AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at>NOW()))`, id).Scan(&available)
	if e != nil {
		return e
	}
	if !available {
		return errors.New("代理本身不可用，请先检查代理状态")
	}
	if _, e = tx.ExecContext(ctx, `UPDATE proxies SET status='active',updated_at=clock_timestamp() WHERE id=$1 AND status='inactive'`, id); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO proxy_binding_allowance(proxy_key,available,granted_at) VALUES(kitty_proxy_key($1),true,NOW()) ON CONFLICT(proxy_key) DO UPDATE SET available=true,granted_at=CASE WHEN proxy_binding_allowance.available THEN proxy_binding_allowance.granted_at ELSE NOW() END`, id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
