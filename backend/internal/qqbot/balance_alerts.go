package qqbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"time"
)

type BalanceAlertValidator func(context.Context, int64, float64) (bool, error)

// Administrator-only runtime summary. No balances, message bodies, QQ access
// credentials or API keys are exposed in these delivery diagnostics.
type BalanceAlertRecord struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) balanceAlertRecords(ctx context.Context, appID string) ([]BalanceAlertRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,status,attempts,last_error,created_at FROM qq_bot_balance_alerts WHERE app_id=$1 ORDER BY created_at DESC LIMIT 30`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BalanceAlertRecord{}
	for rows.Next() {
		var r BalanceAlertRecord
		if err = rows.Scan(&r.ID, &r.UserID, &r.Status, &r.Attempts, &r.LastError, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) SetBalanceAlertValidator(v BalanceAlertValidator) { s.balanceAlertValidator = v }
func formatPrivateBalance(balance float64) string {
	return fmt.Sprintf("我的余额：USD %.6f", balance)
}

// Called after a real threshold crossing by the existing billing notification
// worker. No request, model, balance or email operation is replayed here.
func (s *Service) EnqueueBalanceAlert(ctx context.Context, userID int64, balance, threshold float64, siteName, rechargeURL string) error {
	if userID <= 0 || threshold <= 0 || math.IsNaN(balance) || math.IsInf(balance, 0) || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil
	}
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !c.Enabled || !c.BalanceAlertsEnabled {
		return nil
	}
	u, parseErr := url.Parse(rechargeURL)
	if parseErr != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		rechargeURL = ""
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO qq_bot_balance_alerts(id,app_id,member_id,user_id,subscription_revision,binding_created_at,balance,threshold,site_name,recharge_url)
 SELECT $1,b.app_id,b.member_id,b.user_id,p.revision,b.created_at,$4,$5,$6,$7 FROM qq_bot_bindings b
 JOIN qq_bot_balance_subscriptions p ON p.app_id=b.app_id AND p.member_id=b.member_id AND p.user_id=b.user_id AND p.binding_created_at=b.created_at
 JOIN users u ON u.id=b.user_id AND u.status='active' AND u.deleted_at IS NULL AND u.balance_notify_enabled
 WHERE b.app_id=$2 AND b.user_id=$3 AND p.enabled ON CONFLICT DO NOTHING`, randomID(), c.AppID, userID, balance, threshold, limitText(siteName, 100), rechargeURL)
	return err
}

type balanceAlert struct {
	ID, AppID, MemberID, Revision, LeaseToken, SiteName, RechargeURL string
	UserID                                                           int64
	BindingAt                                                        time.Time
	Balance, Threshold                                               float64
	Attempts                                                         int
}

func (s *Service) startBalanceAlerts(ctx context.Context) {
	if s.balanceAlertValidator == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				work, stop := context.WithTimeout(ctx, 20*time.Second)
				if err := s.deliverOneBalanceAlert(work); err != nil && ctx.Err() == nil {
					slog.Warn("qqbot balance alert worker failed", "error_class", "storage_or_configuration")
				}
				stop()
			}
		}
	}()
}

func (s *Service) claimBalanceAlert(ctx context.Context, c Config) (balanceAlert, error) {
	var a balanceAlert
	a.LeaseToken = randomID()
	err := s.db.QueryRowContext(ctx, `WITH next AS (
 SELECT id FROM qq_bot_balance_alerts WHERE app_id=$1 AND created_at>NOW()-INTERVAL '24 hours'
 AND ((status='pending' AND next_attempt_at<=NOW()) OR (status='leased' AND lease_until<NOW()))
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
 ) UPDATE qq_bot_balance_alerts a SET status='leased',lease_token=$2,lease_until=NOW()+INTERVAL '2 minutes',attempts=attempts+1
 FROM next WHERE a.id=next.id RETURNING a.id,a.app_id,a.member_id,a.user_id,a.subscription_revision,a.binding_created_at,a.balance,a.threshold,a.site_name,a.recharge_url,a.attempts`, c.AppID, a.LeaseToken).Scan(&a.ID, &a.AppID, &a.MemberID, &a.UserID, &a.Revision, &a.BindingAt, &a.Balance, &a.Threshold, &a.SiteName, &a.RechargeURL, &a.Attempts)
	return a, err
}

func balanceAlertRetry(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 7 {
		attempts = 7
	}
	d := time.Minute * time.Duration(1<<uint(attempts-1))
	if d > time.Hour {
		return time.Hour
	}
	return d
}

func (s *Service) finishBalanceAlert(ctx context.Context, a balanceAlert, status, code string, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE qq_bot_balance_alerts SET status=$3,last_error=$4,next_attempt_at=$5,lease_token='',lease_until=NULL,finished_at=CASE WHEN $3 IN ('sent','cancelled','expired') THEN NOW() ELSE NULL END WHERE id=$1 AND status='leased' AND lease_token=$2`, a.ID, a.LeaseToken, status, code, next)
	return err
}

func (s *Service) deliverOneBalanceAlert(ctx context.Context) error {
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !c.Enabled || !c.BalanceAlertsEnabled {
		_, err = s.db.ExecContext(ctx, `UPDATE qq_bot_balance_alerts SET status='cancelled',last_error='bot_or_notifications_disabled',finished_at=NOW(),lease_token='' WHERE app_id=$1 AND status IN ('pending','leased')`, c.AppID)
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE qq_bot_balance_alerts SET status='expired',last_error='delivery_window_expired',finished_at=NOW(),lease_token='' WHERE status IN ('pending','leased') AND created_at<=NOW()-INTERVAL '24 hours'`)
	if err != nil {
		return err
	}
	if c.NotificationQuietHours.active(time.Now()) {
		return nil
	}
	a, err := s.claimBalanceAlert(ctx, c)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Hold the existing config lock and the exact binding/subscription rows
	// across the bounded send so rebinding/unsubscribe cannot redirect delivery.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := s.lockConfig(ctx, tx)
	if err != nil {
		return err
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT p.enabled FROM qq_bot_balance_subscriptions p
 JOIN qq_bot_bindings b ON b.app_id=p.app_id AND b.member_id=p.member_id AND b.user_id=p.user_id AND b.created_at=p.binding_created_at
 JOIN users u ON u.id=p.user_id AND u.status='active' AND u.deleted_at IS NULL AND u.balance_notify_enabled
 WHERE p.app_id=$1 AND p.member_id=$2 AND p.user_id=$3 AND p.revision=$4 AND p.binding_created_at=$5 FOR SHARE OF p,b`, a.AppID, a.MemberID, a.UserID, a.Revision, a.BindingAt).Scan(&eligible)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !current.Enabled || !current.BalanceAlertsEnabled || current.AppID != a.AppID || !eligible || s.balanceAlertValidator == nil {
		_ = tx.Rollback()
		return s.finishBalanceAlert(ctx, a, "cancelled", "binding_or_subscription_changed", time.Now())
	}
	valid, err := s.balanceAlertValidator(ctx, a.UserID, a.Threshold)
	if err != nil {
		return err
	}
	if !valid {
		_ = tx.Rollback()
		return s.finishBalanceAlert(ctx, a, "cancelled", "balance_policy_changed_or_recovered", time.Now())
	}
	text := fmt.Sprintf("%s · 余额提醒\n触发时余额：USD %.6f\n提醒阈值：USD %.6f\n\n可在网站充值；私聊发送“余额提醒 关闭”可退订。", strings.TrimSpace(a.SiteName), a.Balance, a.Threshold)
	if a.RechargeURL != "" {
		text += "\n" + a.RechargeURL
	}
	sendCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	_, sendErr := s.qqRequest(sendCtx, current, "/v2/users/"+url.PathEscape(a.MemberID)+"/messages", map[string]any{"msg_type": 0, "content": limitText(text, 1800)})
	stop()
	// At least once: a successful QQ send followed by a failed database commit
	// can be delivered again after lease expiry. Never replay a model request.
	if err = tx.Commit(); err != nil {
		return err
	}
	if sendErr != nil {
		code := "qq_delivery_failed"
		var qe *qqRequestError
		if errors.As(sendErr, &qe) {
			code = fmt.Sprintf("qq_http_%d_code_%d", qe.StatusCode, qe.Code)
		}
		return s.finishBalanceAlert(ctx, a, "pending", code, time.Now().Add(balanceAlertRetry(a.Attempts)))
	}
	return s.finishBalanceAlert(ctx, a, "sent", "", time.Now())
}

func resetBalanceAlerts(ctx context.Context, tx *sql.Tx, old, current Config) error {
	if !old.BalanceAlertsEnabled && !current.BalanceAlertsEnabled {
		return nil
	}
	if old.AppID == current.AppID && old.Enabled == current.Enabled && old.BalanceAlertsEnabled == current.BalanceAlertsEnabled && old.Sandbox == current.Sandbox {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE qq_bot_balance_alerts SET status='cancelled',last_error='configuration_changed',finished_at=NOW(),lease_token='' WHERE app_id IN ($1,$2) AND status IN ('pending','leased')`, old.AppID, current.AppID)
	return err
}
