package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const providerHubStateSQL = `SELECT jsonb_build_object(
 'queried_at',clock_timestamp(),
 'started_at',(SELECT started_at FROM kitty_provider_operations_state WHERE singleton),
 'suppliers',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM kitty_suppliers s),'[]'),
 'wallets',COALESCE((SELECT jsonb_agg(to_jsonb(w) ORDER BY w.id) FROM (
  SELECT w.id,w.supplier_id,w.name,w.identity,w.currency,w.credit_currency,w.low_balance,w.sync_interval_minutes,
   w.auth_mode,w.username,w.remote_user_id,(w.secret_cipher<>'') AS has_secret,
   w.balance,w.balance_unit,w.balance_source,w.observed_at,w.attempted_at,w.sync_error,w.active,w.revision,
   w.observed_at>=clock_timestamp()-make_interval(mins=>CASE WHEN w.sync_interval_minutes>0 THEN w.sync_interval_minutes*2 ELSE 60 END) AS fresh,
   CASE WHEN sum((CASE WHEN p.kind='refund' THEN -1 ELSE 1 END)*(p.credits+p.bonus))>0
    AND sum((CASE WHEN p.kind='refund' THEN -1 ELSE 1 END)*p.cash*p.fx_rate)>=0 THEN
    sum((CASE WHEN p.kind='refund' THEN -1 ELSE 1 END)*p.cash*p.fx_rate)/sum((CASE WHEN p.kind='refund' THEN -1 ELSE 1 END)*(p.credits+p.bonus)) END AS credit_cost,
   count(p.id) AS purchases
  FROM kitty_supplier_wallets w LEFT JOIN kitty_supplier_purchases p ON p.wallet_id=w.id AND p.voided_at IS NULL
  GROUP BY w.id)w),'[]'),
 'bindings',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',b.id,'account_id',b.account_id,'wallet_id',b.wallet_id,'started_at',b.started_at,'account_name',a.name) ORDER BY b.account_id)
  FROM kitty_supplier_bindings b LEFT JOIN accounts a ON a.id=b.account_id WHERE b.ended_at IS NULL),'[]'),
 'purchases',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id DESC) FROM (SELECT * FROM kitty_supplier_purchases ORDER BY id DESC LIMIT 500)p),'[]'),
 'purchase_total',(SELECT count(*) FROM kitty_supplier_purchases),
 'rules',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM kitty_supplier_cost_rules r WHERE ended_at IS NULL),'[]'),
 'accounts',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM (SELECT id,name,type,platform,status FROM accounts WHERE deleted_at IS NULL ORDER BY id LIMIT 5000)a),'[]'),
 'account_total',(SELECT count(*) FROM accounts WHERE deleted_at IS NULL),
 'limits',jsonb_build_object('purchase_rows',500,'account_rows',5000))`

func (r *channelRepository) ReadProviderHub(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := r.db.QueryRowContext(ctx, providerHubStateSQL).Scan(&raw)
	return raw, err
}
func (r *channelRepository) ReadProviderHubPurchases(ctx context.Context, id int64, page, size int) (json.RawMessage, error) {
	var raw json.RawMessage
	e := r.db.QueryRowContext(ctx, `SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id DESC) FROM (SELECT * FROM kitty_supplier_purchases WHERE wallet_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3)p),'[]'),'total',(SELECT count(*) FROM kitty_supplier_purchases WHERE wallet_id=$1),'page',$4::integer,'page_size',$2::integer)`, id, size, (page-1)*size, page).Scan(&raw)
	return raw, e
}

func hubExecOne(ctx context.Context, tx *sql.Tx, q string, args ...any) error {
	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return service.ErrProviderHubConflict
	}
	return nil
}
func hubLockWallet(ctx context.Context, tx *sql.Tx, id, revision int64) (string, error) {
	var currency string
	err := tx.QueryRowContext(ctx, `SELECT currency FROM kitty_supplier_wallets WHERE id=$1 AND revision=$2 AND active FOR UPDATE`, id, revision).Scan(&currency)
	if errors.Is(err, sql.ErrNoRows) {
		return "", service.ErrProviderHubConflict
	}
	return currency, err
}
func hubBumpWallet(ctx context.Context, tx *sql.Tx, id int64) error {
	_, e := tx.ExecContext(ctx, `UPDATE kitty_supplier_wallets SET revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id)
	return e
}
func (r *channelRepository) MutateProviderHub(ctx context.Context, c service.ProviderHubCommand) (json.RawMessage, error) {
	if err := service.ValidateProviderHubCommand(&c); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	id := c.ID
	switch c.Action {
	case "supplier":
		if id == 0 {
			err = tx.QueryRowContext(ctx, `INSERT INTO kitty_suppliers(name,site_type,base_url,notes) VALUES($1,$2,$3,$4) RETURNING id`, c.Name, c.SiteType, c.BaseURL, c.Notes).Scan(&id)
		} else {
			err = hubExecOne(ctx, tx, `UPDATE kitty_suppliers SET name=$1,notes=$2,revision=revision+1,updated_at=clock_timestamp(),site_type=$3,base_url=$4
    WHERE id=$5 AND revision=$6 AND active AND ((site_type=$3 AND base_url=$4) OR NOT EXISTS(SELECT 1 FROM kitty_supplier_wallets WHERE supplier_id=$5 AND secret_cipher<>''))`, c.Name, c.Notes, c.SiteType, c.BaseURL, id, c.Revision)
		}
	case "archive_supplier":
		err = hubExecOne(ctx, tx, `UPDATE kitty_suppliers SET active=FALSE,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND revision=$2 AND NOT EXISTS(SELECT 1 FROM kitty_supplier_wallets WHERE supplier_id=$1 AND active)`, id, c.Revision)
	case "wallet":
		if id == 0 {
			err = tx.QueryRowContext(ctx, `INSERT INTO kitty_supplier_wallets(supplier_id,name,identity,currency,credit_currency,low_balance,sync_interval_minutes)
   SELECT id,$2,$3,$4,$5,$6,$7 FROM kitty_suppliers WHERE id=$1 AND active RETURNING id`, c.SupplierID, c.Name, c.Identity, c.Currency, c.CreditCurrency, c.LowBalance, c.SyncIntervalMinutes).Scan(&id)
		} else {
			err = hubExecOne(ctx, tx, `UPDATE kitty_supplier_wallets SET name=$1,low_balance=$2,revision=revision+1,updated_at=clock_timestamp(),sync_interval_minutes=$9
   WHERE id=$3 AND revision=$4 AND active AND supplier_id=$5 AND identity=$6 AND currency=$7 AND credit_currency=$8`, c.Name, c.LowBalance, id, c.Revision, c.SupplierID, c.Identity, c.Currency, c.CreditCurrency, c.SyncIntervalMinutes)
		}
	case "archive_wallet":
		if _, err = hubLockWallet(ctx, tx, id, c.Revision); err == nil {
			err = hubExecOne(ctx, tx, `UPDATE kitty_supplier_wallets SET active=FALSE,revision=revision+1,updated_at=clock_timestamp(),secret_cipher='',auth_mode='none' WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM kitty_supplier_bindings WHERE wallet_id=$1 AND ended_at IS NULL)`, id)
			if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE kitty_supplier_cost_rules SET ended_at=clock_timestamp() WHERE wallet_id=$1 AND ended_at IS NULL`, id)
			}
		}
	case "bindings":
		if _, err = hubLockWallet(ctx, tx, id, c.Revision); err != nil {
			break
		}
		var count int
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE id=ANY($1) AND deleted_at IS NULL`, pq.Array(c.AccountIDs)).Scan(&count)
		if err != nil {
			break
		}
		if count != len(c.AccountIDs) {
			err = service.ErrProviderHubInput
			break
		}
		var conflicts int
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM kitty_supplier_bindings WHERE account_id=ANY($1) AND wallet_id<>$2 AND ended_at IS NULL`, pq.Array(c.AccountIDs), id).Scan(&conflicts)
		if err != nil {
			break
		}
		if conflicts > 0 && !c.Reassign {
			err = service.ErrProviderHubConflict
			break
		}
		// Account rows serialize competing assignments, including previously unbound accounts.
		rows, e := tx.QueryContext(ctx, `SELECT id FROM accounts WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(c.AccountIDs))
		if e != nil {
			err = e
			break
		}
		for rows.Next() {
			var account int64
			err = rows.Scan(&account)
			if err != nil {
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		_ = rows.Close()
		if err != nil {
			break
		}
		// Recheck after taking the account locks.
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM kitty_supplier_bindings WHERE account_id=ANY($1) AND wallet_id<>$2 AND ended_at IS NULL`, pq.Array(c.AccountIDs), id).Scan(&conflicts)
		if err != nil {
			break
		}
		if conflicts > 0 && !c.Reassign {
			err = service.ErrProviderHubConflict
			break
		}
		_, err = tx.ExecContext(ctx, `UPDATE kitty_supplier_bindings SET ended_at=clock_timestamp() WHERE account_id=ANY($1) AND wallet_id<>$2 AND ended_at IS NULL`, pq.Array(c.AccountIDs), id)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO kitty_supplier_bindings(account_id,wallet_id) SELECT a,$2 FROM unnest($1::bigint[])a WHERE NOT EXISTS(SELECT 1 FROM kitty_supplier_bindings WHERE account_id=a AND ended_at IS NULL)`, pq.Array(c.AccountIDs), id)
		}
		if err == nil {
			err = hubBumpWallet(ctx, tx, id)
		}
	case "unbind":
		// ID is a binding identity, not a business account ID.
		var wallet int64
		err = tx.QueryRowContext(ctx, `SELECT wallet_id FROM kitty_supplier_bindings WHERE id=$1 AND ended_at IS NULL`, id).Scan(&wallet)
		if err == nil {
			_, err = hubLockWallet(ctx, tx, wallet, c.Revision)
		}
		if err == nil {
			err = hubExecOne(ctx, tx, `UPDATE kitty_supplier_bindings SET ended_at=clock_timestamp() WHERE id=$1 AND ended_at IS NULL`, id)
		}
		if err == nil {
			err = hubBumpWallet(ctx, tx, wallet)
		}
	case "purchase":
		var currency string
		err = tx.QueryRowContext(ctx, `SELECT currency FROM kitty_supplier_wallets WHERE id=$1 AND active FOR UPDATE`, id).Scan(&currency)
		if err != nil {
			break
		}
		if c.Currency == currency && c.FXRate != 1 {
			err = service.ErrProviderHubInput
			break
		}
		var previous []byte
		err = tx.QueryRowContext(ctx, `SELECT jsonb_build_array(wallet_id,kind,cash,cash_currency,fx_rate,credits,bonus,reference,note,occurred_at) FROM kitty_supplier_purchases WHERE operation_id=$1`, c.OperationID).Scan(&previous)
		if err == nil {
			var same bool
			err = tx.QueryRowContext(ctx, `SELECT $1::jsonb=jsonb_build_array($2::bigint,$3::text,$4::numeric,$5::text,$6::numeric,$7::numeric,$8::numeric,$9::text,$10::text,$11::timestamptz)`, string(previous), id, c.Kind, c.Cash, c.Currency, c.FXRate, c.Credits, c.Bonus, c.Reference, c.Note, c.OccurredAt).Scan(&same)
			if err == nil && !same {
				err = service.ErrProviderHubConflict
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			break
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO kitty_supplier_purchases(wallet_id,operation_id,kind,cash,cash_currency,fx_rate,credits,bonus,reference,note,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, id, c.OperationID, c.Kind, c.Cash, c.Currency, c.FXRate, c.Credits, c.Bonus, c.Reference, c.Note, c.OccurredAt).Scan(&id)
	case "void_purchase":
		err = hubExecOne(ctx, tx, `UPDATE kitty_supplier_purchases SET voided_at=clock_timestamp(),revision=revision+1 WHERE id=$1 AND revision=$2 AND voided_at IS NULL`, id, c.Revision)
	case "rule":
		var currency string
		currency, err = hubLockWallet(ctx, tx, id, c.Revision)
		if err != nil {
			break
		}
		if c.Method == "account_credits" && c.Currency != currency {
			err = service.ErrProviderHubInput
			break
		}
		_, err = tx.ExecContext(ctx, `UPDATE kitty_supplier_cost_rules SET ended_at=clock_timestamp() WHERE wallet_id=$1 AND model=$2 AND ended_at IS NULL`, id, c.Model)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO kitty_supplier_cost_rules(wallet_id,model,method,multiplier,input_price,output_price,cache_read_price,cache_write_price,cache_write_12h_price,unit_price,currency,note,fx_to_usd) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id, c.Model, c.Method, c.Multiplier, c.InputPrice, c.OutputPrice, c.CacheReadPrice, c.CacheWritePrice, c.CacheWrite12hPrice, c.UnitPrice, c.Currency, c.Note, c.FXToUSD)
		}
		if err == nil {
			err = hubBumpWallet(ctx, tx, id)
		}
	case "close_rule":
		var wallet int64
		err = tx.QueryRowContext(ctx, `SELECT wallet_id FROM kitty_supplier_cost_rules WHERE id=$1 AND ended_at IS NULL`, id).Scan(&wallet)
		if err == nil {
			_, err = hubLockWallet(ctx, tx, wallet, c.Revision)
		}
		if err == nil {
			err = hubExecOne(ctx, tx, `UPDATE kitty_supplier_cost_rules SET ended_at=clock_timestamp() WHERE id=$1 AND ended_at IS NULL`, id)
		}
		if err == nil {
			err = hubBumpWallet(ctx, tx, wallet)
		}
	case "authorization":
		if _, err = hubLockWallet(ctx, tx, id, c.Revision); err != nil {
			break
		}
		var mode, username, userID, cipher string
		err = tx.QueryRowContext(ctx, `SELECT auth_mode,username,remote_user_id,secret_cipher FROM kitty_supplier_wallets WHERE id=$1`, id).Scan(&mode, &username, &userID, &cipher)
		if err != nil {
			break
		}
		if c.AuthMode != "none" && c.SecretCipher == "" && (cipher == "" || mode != c.AuthMode || username != c.Username || userID != c.RemoteUserID) {
			err = service.ErrProviderHubInput
			break
		}
		if c.AuthMode == "none" {
			c.SecretCipher = ""
		} else if c.SecretCipher == "" {
			c.SecretCipher = cipher
		}
		_, err = tx.ExecContext(ctx, `UPDATE kitty_supplier_wallets SET auth_mode=$2,username=$3,remote_user_id=$4,secret_cipher=$5,revision=revision+1,updated_at=clock_timestamp(),balance=NULL,observed_at=NULL,balance_source='unknown',sync_error='' WHERE id=$1`, id, c.AuthMode, c.Username, c.RemoteUserID, c.SecretCipher)
	case "adjustment":
		var matches bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kitty_procurement_receipts WHERE api_key_id=$1 AND request_key=$2)`, c.APIKeyID, c.RequestKey).Scan(&matches)
		if err != nil {
			break
		}
		if !matches {
			err = service.ErrProviderHubInput
			break
		}
		var previous int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM kitty_procurement_adjustments WHERE operation_id=$1 AND api_key_id=$2 AND request_key=$3 AND amount=$4 AND currency=$5 AND reference=$6 AND note=$7 AND fx_to_usd IS NOT DISTINCT FROM $8::numeric`, c.OperationID, c.APIKeyID, c.RequestKey, c.Amount, c.Currency, c.Reference, c.Note, c.FXToUSD).Scan(&previous)
		if err == nil {
			id = previous
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			break
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO kitty_procurement_adjustments(api_key_id,request_key,operation_id,amount,currency,reference,note,fx_to_usd) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, c.APIKeyID, c.RequestKey, c.OperationID, c.Amount, c.Currency, c.Reference, c.Note, c.FXToUSD).Scan(&id)
	default:
		err = service.ErrProviderHubInput
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isUniqueViolation(err) {
			return nil, service.ErrProviderHubConflict
		}
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"id": id, "committed": true})
}

func (r *channelRepository) ProviderHubAuthorization(ctx context.Context, id int64) (*service.ProviderHubWalletAuthorization, error) {
	out := &service.ProviderHubWalletAuthorization{WalletID: id, AccountIDs: []int64{}}
	err := r.db.QueryRowContext(ctx, `SELECT w.revision,s.id,s.site_type,s.base_url,w.currency,w.credit_currency,w.auth_mode,w.username,w.remote_user_id,w.secret_cipher
  FROM kitty_supplier_wallets w JOIN kitty_suppliers s ON s.id=w.supplier_id WHERE w.id=$1 AND w.active AND s.active`, id).Scan(&out.Revision, &out.SupplierID, &out.SiteType, &out.BaseURL, &out.Currency, &out.CreditCurrency, &out.AuthMode, &out.Username, &out.RemoteUserID, &out.Cipher)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT b.account_id FROM kitty_supplier_bindings b JOIN accounts a ON a.id=b.account_id WHERE b.wallet_id=$1 AND b.ended_at IS NULL AND a.deleted_at IS NULL ORDER BY b.account_id LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var account int64
		if err = rows.Scan(&account); err != nil {
			return nil, err
		}
		out.AccountIDs = append(out.AccountIDs, account)
	}
	return out, rows.Err()
}
func (r *channelRepository) SaveProviderHubObservation(ctx context.Context, w *service.ProviderHubWalletAuthorization, balance *float64, unit, source, reason string, observedAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE kitty_supplier_wallets SET balance=CASE WHEN $6='' THEN $3::numeric ELSE balance END,
  balance_unit=CASE WHEN $6='' THEN $4 ELSE balance_unit END,balance_source=CASE WHEN $6='' THEN $5 ELSE balance_source END,
  observed_at=CASE WHEN $6='' THEN COALESCE($7::timestamptz,clock_timestamp()) ELSE observed_at END,attempted_at=clock_timestamp(),sync_error=$6
  WHERE id=$1 AND revision=$2 AND active`, w.WalletID, w.Revision, balance, unit, source, reason, observedAt)
	if err != nil {
		return err
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return service.ErrProviderHubConflict
	}
	return nil
}

const providerOperationsCTE = `WITH selected AS (
 SELECT r.*,a.amount AS reconciled_cost,a.currency AS reconciled_currency,a.reference AS invoice_reference,a.id AS adjustment_id,
  CASE WHEN a.id IS NOT NULL THEN a.amount*a.fx_to_usd ELSE r.estimated_cost*(r.rule_snapshot->>'fx_to_usd')::numeric END AS usd_cost,
  CASE WHEN a.id IS NOT NULL THEN a.amount ELSE r.estimated_cost END AS effective_cost,
  CASE WHEN a.id IS NOT NULL THEN a.currency ELSE r.cost_currency END AS effective_currency,
  CASE WHEN a.id IS NOT NULL THEN 'reconciled' ELSE r.cost_status END AS evidence
 FROM kitty_procurement_receipts r LEFT JOIN LATERAL(SELECT * FROM kitty_procurement_adjustments a WHERE a.api_key_id=r.api_key_id AND a.request_key=r.request_key ORDER BY id DESC LIMIT 1)a ON TRUE
 WHERE r.observed_at>=$1 AND r.observed_at<$2
 AND ($3::bigint=0 OR r.supplier_id=$3) AND ($4::bigint=0 OR r.wallet_id=$4)
 AND ($5::bigint=0 OR r.account_id=$5) AND ($6::bigint=0 OR r.group_id=$6)
 AND ($7='' OR r.model=$7 OR r.upstream_model=$7)
)`
const providerOperationsMetrics = `jsonb_build_object('requests',count(*),'request_valuation',COALESCE(sum(request_valuation),0),
 'valuation_unknown',count(*) FILTER(WHERE request_valuation IS NULL),
 'priced',count(effective_cost),'reconciled',count(*) FILTER(WHERE evidence='reconciled'),
 'estimated',count(*) FILTER(WHERE evidence='estimated'),'unknown',count(*) FILTER(WHERE effective_cost IS NULL),
 'usd_priced',count(usd_cost),'usd_cost',COALESCE(sum(usd_cost),0),
 'comparable_count',count(*) FILTER(WHERE usd_cost IS NOT NULL AND request_valuation IS NOT NULL),
 'comparable_valuation',COALESCE(sum(request_valuation) FILTER(WHERE usd_cost IS NOT NULL AND request_valuation IS NOT NULL),0),
 'comparable_profit',COALESCE(sum(request_valuation-usd_cost) FILTER(WHERE usd_cost IS NOT NULL AND request_valuation IS NOT NULL),0))`

func (r *channelRepository) ReadProviderOperations(ctx context.Context, f service.ProviderOperationsFilter) (json.RawMessage, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	args := []any{f.From, f.Until, f.SupplierID, f.WalletID, f.AccountID, f.GroupID, f.Model}
	var summary, trend, breakdown, currencies, receipts json.RawMessage
	if err = tx.QueryRowContext(ctx, providerOperationsCTE+` SELECT `+providerOperationsMetrics+` FROM selected`, args...).Scan(&summary); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, providerOperationsCTE+` SELECT COALESCE(jsonb_agg(v ORDER BY day),'[]') FROM (SELECT to_char(observed_at AT TIME ZONE $8,'YYYY-MM-DD') AS day,`+providerOperationsMetrics+` AS metrics FROM selected GROUP BY 1)v`, append(args, f.Timezone)...).Scan(&trend); err != nil {
		return nil, err
	}
	dimension, label := `COALESCE(supplier_id,0)::text`, `COALESCE(max(supplier_name),'Unassigned')`
	switch f.Dimension {
	case "wallet":
		dimension, label = `COALESCE(wallet_id,0)::text`, `COALESCE(max(wallet_name),'Unassigned')`
	case "account":
		dimension, label = `account_id::text`, `account_id::text`
	case "model":
		dimension, label = `model`, `model`
	case "group":
		dimension, label = `COALESCE(group_id,0)::text`, `COALESCE(group_id,0)::text`
	}
	q := providerOperationsCTE + ` SELECT COALESCE(jsonb_agg(v ORDER BY requests DESC,key),'[]') FROM (SELECT ` + dimension + ` AS key,` + label + ` AS name,count(*) AS requests,` + providerOperationsMetrics + ` AS metrics FROM selected GROUP BY ` + dimension + ` ORDER BY count(*) DESC,` + dimension + ` LIMIT 200)v`
	if err = tx.QueryRowContext(ctx, q, args...).Scan(&breakdown); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, providerOperationsCTE+` SELECT COALESCE(jsonb_agg(v ORDER BY currency),'[]') FROM (SELECT effective_currency AS currency,count(*) AS records,sum(effective_cost) AS amount FROM selected WHERE effective_cost IS NOT NULL GROUP BY effective_currency)v`, args...).Scan(&currencies); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, providerOperationsCTE+` SELECT COALESCE(jsonb_agg(v ORDER BY observed_at DESC,source_log_id DESC),'[]') FROM (SELECT api_key_id,request_key,source_log_id,account_id,group_id,model,upstream_model,upstream_request_id,supplier_id,supplier_name,wallet_id,wallet_name,estimated_cost,cost_currency,rule_id,rule_snapshot,credit_cost,measurements,request_valuation,effective_cost,effective_currency,usd_cost,evidence,invoice_reference,adjustment_id,observed_at FROM selected ORDER BY observed_at DESC,source_log_id DESC LIMIT $8 OFFSET $9)v`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...).Scan(&receipts); err != nil {
		return nil, err
	}
	var started time.Time
	if err = tx.QueryRowContext(ctx, `SELECT started_at FROM kitty_provider_operations_state WHERE singleton`).Scan(&started); err != nil {
		return nil, err
	}
	// Site-wide cash and balances keep their own scope even when request filters are set.
	var cash, balances, monitoring, purchases json.RawMessage
	if err = tx.QueryRowContext(ctx, providerCashSQL, f.From, f.Until).Scan(&cash); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, providerBalancesSQL).Scan(&balances); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, providerMonitoringSQL, f.From, f.Until).Scan(&monitoring); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(v ORDER BY currency),'[]') FROM (SELECT cash_currency AS currency,
  sum(cash) FILTER(WHERE kind IN ('topup','opening')) AS paid,sum(cash) FILTER(WHERE kind='refund') AS refunded,count(*) AS records
  FROM kitty_supplier_purchases p JOIN kitty_supplier_wallets w ON w.id=p.wallet_id
  WHERE p.voided_at IS NULL AND p.occurred_at>=$1 AND p.occurred_at<$2 AND ($3::bigint=0 OR w.supplier_id=$3) AND ($4::bigint=0 OR w.id=$4)
  GROUP BY cash_currency)v`, f.From, f.Until, f.SupplierID, f.WalletID).Scan(&purchases); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"scope": f, "queried_at": time.Now().UTC(), "started_at": started, "summary": summary, "trend": trend, "breakdown": breakdown, "cost_currencies": currencies, "receipts": receipts, "cash": cash, "balances": balances, "monitoring": monitoring, "purchases": purchases, "breakdown_limit": 200, "cash_scope": "site_wide", "history": "captured_since_activation_only"})
}

var _ service.ProviderHubRepository = (*channelRepository)(nil)

func (r *channelRepository) RotateProviderHubCredential(ctx context.Context, w *service.ProviderHubWalletAuthorization, cipher string) error {
	result, e := r.db.ExecContext(ctx, `UPDATE kitty_supplier_wallets SET secret_cipher=$4 WHERE id=$1 AND revision=$2 AND secret_cipher=$3 AND active`, w.WalletID, w.Revision, w.Cipher, cipher)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return service.ErrProviderHubConflict
	}
	w.Cipher = cipher
	return nil
}
func (r *channelRepository) BeginProviderHubKeyCreation(ctx context.Context, id int64, operation, name, group string) (bool, error) {
	result, e := r.db.ExecContext(ctx, `INSERT INTO kitty_supplier_remote_operations(operation_id,wallet_id,key_name,group_identity) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, operation, id, name, group)
	if e != nil {
		return false, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return false, e
	}
	if n == 1 {
		return true, nil
	}
	var succeeded bool
	e = r.db.QueryRowContext(ctx, `SELECT state='succeeded' FROM kitty_supplier_remote_operations WHERE operation_id=$1 AND wallet_id=$2 AND key_name=$3 AND group_identity=$4`, operation, id, name, group).Scan(&succeeded)
	if e != nil || !succeeded {
		return false, service.ErrProviderHubConflict
	}
	return false, nil
}
func (r *channelRepository) FinishProviderHubKeyCreation(ctx context.Context, operation string, success bool) error {
	state := "unconfirmed"
	if success {
		state = "succeeded"
	}
	_, e := r.db.ExecContext(ctx, `UPDATE kitty_supplier_remote_operations SET state=$2,completed_at=clock_timestamp() WHERE operation_id=$1 AND state='pending'`, operation, state)
	return e
}

func (r *channelRepository) ClaimProviderHubSync(ctx context.Context, limit int) ([]int64, error) {
	if limit < 1 || limit > 5 {
		return nil, service.ErrProviderHubInput
	}
	rows, e := r.db.QueryContext(ctx, `WITH due AS(SELECT w.id FROM kitty_supplier_wallets w JOIN kitty_suppliers s ON s.id=w.supplier_id WHERE w.active AND s.active AND w.sync_interval_minutes>0 AND (w.attempted_at IS NULL OR w.attempted_at<clock_timestamp()-make_interval(mins=>w.sync_interval_minutes)) ORDER BY w.attempted_at NULLS FIRST,w.id FOR UPDATE OF w SKIP LOCKED LIMIT $1)
  UPDATE kitty_supplier_wallets w SET attempted_at=clock_timestamp() FROM due WHERE w.id=due.id RETURNING w.id`, limit)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	out := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *channelRepository) LockProviderHubImport(ctx context.Context, id int64) (func(), error) {
	conn, e := r.db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	var acquired bool
	e = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(824137,$1::integer)`, id).Scan(&acquired)
	if e != nil || !acquired {
		_ = conn.Close()
		if e != nil {
			return nil, e
		}
		return nil, service.ErrProviderHubConflict
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(824137,$1::integer)`, id)
		_ = conn.Close()
	}, nil
}
