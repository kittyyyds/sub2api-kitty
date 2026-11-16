package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

const privateMessageEvent = "C2C_MESSAGE_CREATE"
const privateJobGroup = "_kitty_private"

func isPrivateJob(j Job) bool {
	return j.EventType == privateMessageEvent && j.GroupID == privateJobGroup && j.MemberID != ""
}

// Enqueue is called only after the existing QQ signature/app-ID verification.
// C2C identity comes exclusively from author.user_openid, never member_openid,
// a requested user ID, arbitrary message content, or an unverified group ID.
func (s *Service) enqueuePrivateAccount(ctx context.Context, c Config, p Payload) error {
	if !c.Enabled {
		return nil
	}
	var raw struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
		Author    struct {
			OpenID string `json:"user_openid"`
		} `json:"author"`
	}
	if err := json.Unmarshal(p.Data, &raw); err != nil {
		return err
	}
	if raw.ID == "" || len(raw.ID) > 256 || raw.Author.OpenID == "" || len(raw.Author.OpenID) > 128 || len(raw.Content) > 2048 {
		return nil
	}
	cmd, _ := command(raw.Content)
	switch cmd {
	case "帮助", "个人信息", "我的密钥", "密钥用量", "余额提醒", "我的余额", "今日用量":
	default:
		return nil // private messages never enter AI chat, rewards or transfers
	}
	at, err := time.Parse(time.RFC3339, raw.Timestamp)
	if err != nil || time.Since(at) > passiveReplyWindow || at.After(time.Now().Add(time.Minute)) {
		return nil
	}
	e := Event{ID: raw.ID, Content: raw.Content, GroupID: privateJobGroup}
	e.Author.MemberID = raw.Author.OpenID
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,335))`, c.AppID+":"+raw.Author.OpenID); err != nil {
		return err
	}
	// Bound work, including unknown/unbound identities, without consuming model quota.
	var recent int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM qq_bot_jobs WHERE app_id=$1 AND member_id=$2 AND event_type=$3 AND created_at>NOW()-INTERVAL '1 minute'`, c.AppID, raw.Author.OpenID, privateMessageEvent).Scan(&recent); err != nil {
		return err
	}
	if recent >= 12 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO qq_bot_jobs(id,app_id,event_key,event_type,group_id,member_id,message_id,payload,kind,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'private_account',$9) ON CONFLICT(app_id,event_key) DO NOTHING`, randomID(), c.AppID, "private:"+raw.ID, privateMessageEvent, privateJobGroup, raw.Author.OpenID, raw.ID, string(body), at)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) sendPrivateAccount(ctx context.Context, c Config, j Job, text string) error {
	if !isPrivateJob(j) || j.MessageID == "" || time.Since(j.CreatedAt) >= passiveReplyWindow {
		return errReplyWindow
	}
	ctx, cancel := context.WithDeadline(ctx, j.CreatedAt.Add(passiveReplyWindow))
	defer cancel()
	_, err := s.qqRequest(ctx, c, "/v2/users/"+url.PathEscape(j.MemberID)+"/messages", map[string]any{"msg_type": 0, "content": limitText(text, 1800), "msg_id": j.MessageID, "msg_seq": 1})
	return err
}

func (s *Service) privateAccountMessage(ctx context.Context, c Config, j Job) error {
	var e Event
	if err := json.Unmarshal(j.Payload, &e); err != nil {
		return err
	}
	cmd, arg := command(e.Content)
	text := "私聊支持：个人信息、我的密钥 [页码]、密钥用量 ID [YYYY-MM-DD]、余额提醒 开启/关闭/状态。请先在群内完成网站账号绑定，再使用同一 QQ 私聊。"
	var err error
	switch cmd {
	case "个人信息":
		if arg != "" {
			text = "用法：个人信息。"
		} else {
			text, err = s.personalInfo(ctx, c, j, time.Now())
		}
	case "我的密钥":
		text, err = s.myAPIKeys(ctx, c, j, arg)
	case "密钥用量":
		text, err = s.myAPIKeyUsage(ctx, c, j, arg, time.Now())
	case "余额提醒":
		text, err = s.balanceSubscription(ctx, c, j, arg)
	case "我的余额":
		var uid int64
		_, uid, err = s.personalQueryIdentity(ctx, c, j)
		if errors.Is(err, sql.ErrNoRows) {
			text, err = bindFirst, nil
		} else if err == nil {
			var balance float64
			err = s.db.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL`, uid).Scan(&balance)
			text = formatPrivateBalance(balance)
		}
	case "今日用量":
		_, _, err = s.personalQueryIdentity(ctx, c, j)
		if errors.Is(err, sql.ErrNoRows) {
			text, err = bindFirst, nil
		} else if err == nil {
			text, err = s.todayUsage(ctx, c, j, time.Now())
		}
	}
	if err != nil {
		text = "账户查询暂时不可用，请稍后重试。"
	}
	return s.sendPrivateAccount(ctx, c, j, text)
}

func (s *Service) balanceSubscription(ctx context.Context, c Config, j Job, arg string) (string, error) {
	if !isPrivateJob(j) {
		return "请私聊机器人发送“余额提醒 开启”，通知只发给已确认的本人私聊。", nil
	}
	if arg != "开启" && arg != "关闭" && arg != "状态" && strings.TrimSpace(arg) != "" {
		return "用法：余额提醒 开启/关闭/状态。", nil
	}
	current, uid, err := s.personalQueryIdentity(ctx, c, j)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	if arg == "开启" && !current.BalanceAlertsEnabled {
		return "站点尚未开启 QQ 低余额私聊提醒，请联系管理员。", nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Serialize against binding replacement/deletion and verify the exact user again.
	var boundAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT created_at FROM qq_bot_bindings WHERE app_id=$1 AND member_id=$2 AND user_id=$3 FOR UPDATE`, c.AppID, j.MemberID, uid).Scan(&boundAt)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	if arg == "开启" || arg == "关闭" {
		_, err = tx.ExecContext(ctx, `INSERT INTO qq_bot_balance_subscriptions(app_id,member_id,user_id,binding_created_at,revision,enabled,verified_private_at) VALUES($1,$2,$3,$4,$5,$6,NOW()) ON CONFLICT(app_id,member_id) DO UPDATE SET user_id=EXCLUDED.user_id,binding_created_at=EXCLUDED.binding_created_at,revision=EXCLUDED.revision,enabled=EXCLUDED.enabled,verified_private_at=NOW(),updated_at=NOW()`, c.AppID, j.MemberID, uid, boundAt, randomID(), arg == "开启")
		if err != nil {
			return "", err
		}
		_, err = tx.ExecContext(ctx, `UPDATE qq_bot_balance_alerts SET status='cancelled',last_error='subscription_changed',finished_at=NOW(),lease_token='' WHERE app_id=$1 AND member_id=$2 AND status IN ('pending','leased')`, c.AppID, j.MemberID)
		if err != nil {
			return "", err
		}
		if err = tx.Commit(); err != nil {
			return "", err
		}
		if arg == "关闭" {
			return "已关闭低余额私聊提醒，待发提醒已取消。", nil
		}
		return "已订阅低余额私聊提醒。请在网站个人设置开启余额提醒并设置阈值；QQ 提醒沿用网站总开关和个人阈值。不会补发订阅前的提醒。", nil
	}
	var enabled bool
	err = tx.QueryRowContext(ctx, `SELECT enabled FROM qq_bot_balance_subscriptions WHERE app_id=$1 AND member_id=$2 AND user_id=$3 AND binding_created_at=$4`, c.AppID, j.MemberID, uid, boundAt).Scan(&enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if enabled && current.BalanceAlertsEnabled {
		return "低余额私聊：已订阅。提醒阈值和总开关跟随网站个人设置。", nil
	}
	return "低余额私聊：未开启。私聊发送“余额提醒 开启”订阅。", nil
}
