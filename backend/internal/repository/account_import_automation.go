package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"time"
)

func (r *accountRepository) BeginAccountImport(ctx context.Context, platform, source string, proxyID *int64, identity string) (*service.AccountImportTicket, error) {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return nil, errors.New("导入分配需要事务")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71942021)`); err != nil {
		return nil, err
	}
	var raw []byte
	var policy service.ProxyBindingPolicy
	if err = tx.QueryRowContext(ctx, `SELECT config FROM proxy_binding_policy WHERE id=true`).Scan(&raw); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		return nil, err
	}
	t := &service.AccountImportTicket{ID: uuid.NewString(), Platform: platform, Source: source, ProxyID: proxyID}
	if identity == "" {
		identity = "pending:" + t.ID
	}
	// Existing identities always retain their original egress on re-login.
	var existing *int64
	prior := tx.QueryRowContext(ctx, `SELECT a.proxy_id FROM accounts a WHERE a.deleted_at IS NULL AND a.platform=$1 AND a.type='oauth' AND lower(trim(a.credentials->>'email'))=$2 ORDER BY a.id LIMIT 1`, platform, identity).Scan(&existing)
	if prior != nil && prior != sql.ErrNoRows {
		return nil, prior
	}
	if prior == nil {
		t.ProxyID = existing
	} else if policy.Matches(platform, "oauth", source) && policy.AutoAssign {
		var id int64
		err = tx.QueryRowContext(ctx, `SELECT p.id FROM proxies p JOIN proxy_binding_exits x ON x.proxy_id=p.id CROSS JOIN proxy_binding_policy c WHERE p.status='active' AND p.deleted_at IS NULL AND (p.expires_at IS NULL OR p.expires_at>NOW()) AND x.proxy_revision=p.updated_at AND x.checked_at>NOW()-interval '10 minutes' AND kitty_proxy_history_count(kitty_proxy_key(p.id),c.config)<(c.config->>'history_limit')::int AND NOT EXISTS(SELECT 1 FROM accounts a WHERE a.deleted_at IS NULL AND a.proxy_id IS NOT NULL AND kitty_proxy_key(a.proxy_id)=kitty_proxy_key(p.id)) AND NOT EXISTS(SELECT 1 FROM account_import_tickets t WHERE t.state IN ('pending','ready') AND t.expires_at>NOW() AND kitty_proxy_key(t.proxy_id)=kitty_proxy_key(p.id)) ORDER BY kitty_proxy_history_count(kitty_proxy_key(p.id),c.config),p.id LIMIT 1`).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("没有可分配的 IP：须出口已验证、当前空闲、历史未满且未被预留；未切换直连")
		}
		if err != nil {
			return nil, err
		}
		t.ProxyID = &id
		t.AutoAssigned = true
	}
	if policy.Matches(platform, "oauth", source) && t.ProxyID != nil && prior == sql.ErrNoRows {
		var allowed bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM proxies p CROSS JOIN proxy_binding_policy c WHERE p.id=$1 AND p.status='active' AND p.deleted_at IS NULL AND (p.expires_at IS NULL OR p.expires_at>NOW()) AND (kitty_proxy_history_count(kitty_proxy_key(p.id),c.config)<(c.config->>'history_limit')::int OR (NOT $2 AND EXISTS(SELECT 1 FROM proxy_binding_allowance WHERE proxy_key=kitty_proxy_key(p.id) AND available))) AND NOT EXISTS(SELECT 1 FROM account_import_tickets t WHERE t.state IN ('pending','ready') AND t.expires_at>NOW() AND kitty_proxy_key(t.proxy_id)=kitty_proxy_key(p.id)))`, *t.ProxyID, t.AutoAssigned).Scan(&allowed)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, errors.New("所选 IP 已达历史上限、被预留或不可用")
		}
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO account_import_tickets(id,platform,source,proxy_id,identity_key,auto_assigned) VALUES($1,$2,$3,$4,md5($2||'|'||$5),$6) RETURNING identity_key,state,expires_at`, t.ID, platform, source, t.ProxyID, identity, t.AutoAssigned).Scan(&t.IdentityKey, &t.State, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return t, nil
}
func (r *accountRepository) ReadyAccountImport(ctx context.Context, id, fp string) error {
	result, err := r.sql.ExecContext(ctx, `UPDATE account_import_tickets SET fingerprint=$2,state='ready',updated_at=NOW() WHERE id=$1 AND state='pending' AND expires_at>NOW()`, id, fp)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("登录预留已失效，请重新登录")
	}
	return nil
}
func (r *accountRepository) ReleaseAccountImport(ctx context.Context, id string) error {
	_, err := r.sql.ExecContext(ctx, `UPDATE account_import_tickets SET state='released',updated_at=NOW() WHERE id=$1 AND state IN ('pending','ready')`, id)
	return err
}
func (r *accountRepository) ResolveAccountImport(ctx context.Context, id, fp string) (*service.AccountImportTicket, error) {
	db, ok := r.sql.(planRowQuerier)
	if !ok {
		return nil, errors.New("账号导入仓储不支持查询")
	}
	t := &service.AccountImportTicket{ID: id}
	err := db.QueryRowContext(ctx, `SELECT platform,source,proxy_id,identity_key,state,account_id,auto_assigned,expires_at FROM account_import_tickets WHERE id=$1 AND fingerprint=$2 AND $2<>'' AND (state='consumed' OR (state='ready' AND expires_at>NOW()))`, id, fp).Scan(&t.Platform, &t.Source, &t.ProxyID, &t.IdentityKey, &t.State, &t.AccountID, &t.AutoAssigned, &t.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("2FA 导入票据已失效或与登录凭据不匹配")
	}
	return t, err
}
func lockAccountImport(ctx context.Context, client *dbent.Client, a *service.Account) error {
	if a.ImportTicket == "" {
		return nil
	}
	if _, err := client.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71942021)`); err != nil {
		return err
	}
	// Close the race with another import while holding the normal transaction lock.
	if batch, _ := a.Extra["openai_bps_batch_import"].(bool); batch {
		rows, err := client.QueryContext(ctx, `SELECT id FROM accounts WHERE deleted_at IS NULL AND platform='openai' AND type='oauth' AND lower(trim(credentials->>'email'))=lower(trim($1)) LIMIT 1`, a.GetCredential("email"))
		if err != nil {
			return err
		}
		exists := rows.Next()
		readErr := rows.Err()
		rows.Close()
		if readErr != nil {
			return readErr
		}
		if exists {
			return errors.New("该邮箱已有账号，请刷新账号列表")
		}
	}
	if _, err := client.ExecContext(ctx, `SELECT set_config('kitty.import_ticket',$1,true)`, a.ImportTicket); err != nil {
		return err
	}
	rows, err := client.QueryContext(ctx, `SELECT state,expires_at,platform,proxy_id,fingerprint FROM account_import_tickets WHERE id=$1 FOR UPDATE`, a.ImportTicket)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return errors.New("导入票据不存在")
	}
	var state, platform, fp string
	var expiry time.Time
	var proxyID *int64
	if err = rows.Scan(&state, &expiry, &platform, &proxyID, &fp); err != nil {
		return err
	}
	if state != "ready" || !expiry.After(time.Now()) || platform != a.Platform || fp != service.ImportCredentialFingerprint(a.Credentials) || (proxyID == nil) != (a.ProxyID == nil) || (proxyID != nil && *proxyID != *a.ProxyID) {
		return errors.New("导入预留已失效或已被消费，请刷新账号列表")
	}
	return rows.Err()
}
func consumeAccountImport(ctx context.Context, client *dbent.Client, a *service.Account) error {
	if a.ImportTicket == "" {
		return nil
	}
	_, err := client.ExecContext(ctx, `INSERT INTO account_import_origins(account_id,platform,source,identity_key,ticket_id) SELECT $2,platform,source,identity_key,id FROM account_import_tickets WHERE id=$1 AND state='ready'`, a.ImportTicket, a.ID)
	if err != nil {
		return fmt.Errorf("记录导入来源失败: %w", err)
	}
	_, err = client.ExecContext(ctx, `UPDATE account_import_tickets SET state='consumed',account_id=$2,updated_at=NOW() WHERE id=$1`, a.ImportTicket, a.ID)
	return err
}

// Only ordinary OAuth receipts may be finalized by the administrator create flow.
// The two-factor source can only be attested by the server login worker.
func (r *accountRepository) FinalizeOAuthImport(ctx context.Context, id, fp, identity string) error {
	if fp == "" {
		return errors.New("OAuth 登录缺少访问凭据")
	}
	_, err := r.sql.ExecContext(ctx, `UPDATE account_import_tickets SET fingerprint=$2,identity_key=md5(platform||'|'||$3),state='ready',updated_at=NOW() WHERE id=$1 AND source='oauth' AND state='pending' AND expires_at>NOW()`, id, fp, identity)
	return err
}

func (r *accountRepository) FindBPSImportAccount(ctx context.Context, email string) (int64, error) {
	db, ok := r.sql.(planRowQuerier)
	if !ok {
		return 0, errors.New("identity lookup unavailable")
	}
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE deleted_at IS NULL AND platform='openai' AND type='oauth' AND lower(trim(credentials->>'email'))=lower(trim($1)) ORDER BY id LIMIT 1`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
