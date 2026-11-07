package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// The editable text is an introduction. Identity and delivery claims are
// generated separately from the actual binding / SMTP result.
type WelcomeConfig struct {
	Enabled          bool   `json:"enabled"`
	AutoEmailBinding bool   `json:"auto_email_binding"`
	ReviewedText     string `json:"reviewed_text"`
	OtherText        string `json:"other_text"`
}

func validateWelcome(w *WelcomeConfig) error {
	if w == nil {
		return nil
	}
	for _, text := range []string{w.ReviewedText, w.OtherText} {
		if len([]rune(text)) > 500 || strings.ContainsAny(text, "<>") || strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
			return errors.New("欢迎正文最多 500 字，不能包含标签或控制字符")
		}
	}
	return nil
}

func welcomePolicy(c Config, group string) string {
	raw, _ := json.Marshal([]any{c.AppID, c.Sandbox, group, c.groupPolicy(group).Welcome, c.groupPolicy(group).JoinReview})
	return codeHash(string(raw))
}

// This small, short-lived receipt is generated only after a successful QQ
// approval. It contains no answer, address, spending amount or binding token.
type approvedJoin struct {
	UserID    int64     `json:"user_id"`
	EmailHash string    `json:"email_hash"`
	Policy    string    `json:"policy"`
	Applied   time.Time `json:"applied"`
	Consumed  bool      `json:"consumed,omitempty"`
}

func (s *Service) approvedJoinReceipt(ctx context.Context, tx *sql.Tx, c Config, e Event, op string) (*approvedJoin, error) {
	w := c.groupPolicy(e.GroupID).Welcome
	if op != "approve" || e.ApplySource != "self_apply" || w == nil || !w.Enabled {
		return nil, nil
	}
	email, ok := joinReviewEmail(e, *c.groupPolicy(e.GroupID).JoinReview)
	if !ok {
		return nil, errors.New("approved answer unavailable")
	}
	p := &approvedJoin{Policy: welcomePolicy(c, e.GroupID)}
	var canonical string
	if err := tx.QueryRowContext(ctx, `SELECT id,email FROM users WHERE lower(email)=$1 AND status='active' AND deleted_at IS NULL`, email).Scan(&p.UserID, &canonical); err != nil {
		return nil, err
	}
	p.EmailHash = codeHash(canonical)
	p.Applied, _ = time.Parse(time.RFC3339, e.ApplyAt)
	return p, nil
}

func (s *Service) welcomeRecord(ctx context.Context, j Job, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE qq_bot_jobs SET kind='welcome',trigger_reason='member_joined',decision_reason=$2 WHERE id=$1`, j.ID, reason)
	return err
}

const welcomeBindingGuide = "如需使用签到和账户服务，请在群内 @机器人 发送：绑定 你的注册邮箱，再按邮件提示确认；也可在网站个人资料生成绑定码并按提示确认。请勿公开验证码或确认链接。"

func (s *Service) welcomeMember(ctx context.Context, c Config, j Job, e Event) error {
	ctx, cancelWelcome := context.WithTimeout(ctx, 35*time.Second)
	defer cancelWelcome()
	w := c.groupPolicy(j.GroupID).Welcome
	if w == nil || !w.Enabled || e.WelcomePolicy == "" {
		return nil
	}
	at := eventTimestamp(e.Timestamp)
	if e.Bot || !validGroupInfoID(j.MemberID) || !validGroupInfoID(j.GroupID) || at.IsZero() || time.Since(at) > 5*time.Minute || time.Until(at) > time.Minute {
		return s.welcomeRecord(ctx, j, "skipped_stale_event")
	}
	// Reserve the whole welcome attempt before either SMTP or QQ. An unknown
	// result is never retried automatically after a restart or repeated event.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := s.lockConfig(ctx, tx)
	if err != nil {
		return err
	}
	if !current.Enabled || !current.allows(j.GroupID) || current.AppID != j.AppID || e.WelcomePolicy != welcomePolicy(current, j.GroupID) {
		_ = tx.Rollback()
		return s.welcomeRecord(ctx, j, "cancelled_policy_changed")
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(71427104)`); err != nil {
		return err
	}
	var groupCount, botCount, dailyCount int
	if err = tx.QueryRowContext(ctx, `SELECT
 COUNT(*) FILTER(WHERE group_id=$2 AND reply_reserved_at>NOW()-INTERVAL '1 minute'),
 COUNT(*) FILTER(WHERE reply_reserved_at>NOW()-INTERVAL '1 minute'),
 COUNT(*) FILTER(WHERE group_id=$2)
 FROM qq_bot_jobs WHERE app_id=$1 AND kind='welcome' AND reply_reserved_at>NOW()-INTERVAL '24 hours'`, j.AppID, j.GroupID).Scan(&groupCount, &botCount, &dailyCount); err != nil {
		return err
	}
	if groupCount >= 10 || botCount >= 20 || dailyCount >= 800 {
		_ = tx.Rollback()
		return s.welcomeRecord(ctx, j, "skipped_rate_limited")
	}
	res, err := tx.ExecContext(ctx, `UPDATE qq_bot_jobs SET kind='welcome',trigger_reason='member_joined',decision_reason='attempting',reply_reserved_at=NOW() WHERE id=$1 AND status='running' AND reply_reserved_at IS NULL`, j.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil
	}
	// Only the latest request for this member may authorize automatic mail.
	// A newer invitation/manual request cannot reuse an earlier email answer.
	var reviewID string
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT id,payload FROM qq_bot_jobs WHERE app_id=$1 AND group_id=$2 AND member_id=$3 AND event_type='GROUP_JOIN_REQUEST'
 AND created_at>NOW()-INTERVAL '30 minutes' AND created_at<=$4 ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, j.AppID, j.GroupID, j.MemberID, j.CreatedAt).Scan(&reviewID, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var receipt struct {
		Approved *approvedJoin `json:"_kitty_approved"`
	}
	_ = json.Unmarshal(raw, &receipt)
	p := receipt.Approved
	var email string
	var uid int64
	if p != nil && !p.Consumed && p.Policy == welcomePolicy(current, j.GroupID) && !p.Applied.After(at) && at.Sub(p.Applied) <= 30*time.Minute {
		err = tx.QueryRowContext(ctx, `SELECT id,email FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL`, p.UserID).Scan(&uid, &email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err != nil || codeHash(email) != p.EmailHash {
			uid, email = 0, ""
		}
		if _, err = tx.ExecContext(ctx, `UPDATE qq_bot_jobs SET payload=jsonb_set(payload,'{_kitty_approved,consumed}','true') WHERE id=$1`, reviewID); err != nil {
			return err
		}
	}
	var memberUID int64
	err = tx.QueryRowContext(ctx, `SELECT b.user_id FROM qq_bot_bindings b JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 AND b.member_id=$2 AND u.status='active' AND u.deleted_at IS NULL`, j.AppID, j.MemberID).Scan(&memberUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var memberBound, accountBound bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qq_bot_bindings WHERE app_id=$1 AND member_id=$2), EXISTS(SELECT 1 FROM qq_bot_bindings WHERE app_id=$1 AND user_id=$3)`, j.AppID, j.MemberID, uid).Scan(&memberBound, &accountBound); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	c = current
	w = c.groupPolicy(j.GroupID).Welcome
	intro, state, reason := w.OtherText, welcomeBindingGuide, "guide"
	if strings.TrimSpace(intro) == "" {
		intro = "欢迎加入 Kitty 交流群！"
	}
	if uid > 0 {
		intro = w.ReviewedText
		if strings.TrimSpace(intro) == "" {
			intro = "欢迎加入 Kitty 已消费用户交流群！进群消费资格审核已通过。"
		}
	}
	switch {
	case memberBound && memberUID > 0 && (uid == 0 || memberUID == uid):
		state, reason = "您已完成网站账号绑定，继续使用现有绑定即可。", "bound"
	case memberBound || accountBound:
		state, reason = "检测到已有绑定或绑定冲突，已保留原绑定，请联系管理员核对。", "conflict"
	case uid > 0 && w.AutoEmailBinding:
		e.Author.MemberID = j.MemberID
		_, pending := s.prepareEmailBinding(ctx, c, e, email, uid)
		if pending == nil {
			state, reason = "本次未能发送绑定确认邮件，可能存在发送间隔、额度或绑定状态限制。"+welcomeBindingGuide, "mail_not_sent"
		} else if err = s.deliverBindingEmail(ctx, e, pending); err != nil {
			state, reason = "绑定确认邮件发送失败，请稍后重试或联系管理员。"+welcomeBindingGuide, "mail_failed"
		} else {
			state, reason = "已向您入群申请填写的邮箱发送绑定确认邮件。请在 10 分钟内打开邮件，核对 QQ 身份并点击确认；确认后才会完成绑定，无需再生成绑定码或重复填写邮箱。", "mail_sent"
		}
	}
	// Re-read authorization after SMTP. A disabled or changed policy must not
	// produce a late unsolicited group message.
	latest, err := s.Config(ctx)
	if err != nil {
		return err
	}
	if !latest.Enabled || !latest.allows(j.GroupID) || latest.AppID != j.AppID || e.WelcomePolicy != welcomePolicy(latest, j.GroupID) {
		return s.welcomeRecord(ctx, j, "cancelled_after_"+reason)
	}
	text := fmt.Sprintf("<qqbot-at-user id=\"%s\" />\n%s\n\n%s", j.MemberID, strings.TrimSpace(intro), state)
	data, err := s.qqRequest(ctx, latest, "/v2/groups/"+url.PathEscape(j.GroupID)+"/messages", map[string]any{"msg_type": 2, "markdown": map[string]string{"content": text}})
	if err == nil {
		var result struct {
			Code int    `json:"code"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(data, &result) != nil || result.Code != 0 || result.ID == "" {
			err = errors.New("QQ welcome result unavailable")
		}
	}
	final, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err != nil {
		_ = s.welcomeRecord(final, j, "delivery_failed_"+reason)
		return errors.New("QQ 入群欢迎未确认发送，请检查主动群消息权限；不会自动补发")
	}
	return s.welcomeRecord(final, j, "sent_"+reason)
}
