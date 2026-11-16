package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
)

// This rule checks eligibility only. An email answer never binds a QQ identity
// to a website account and never grants access to its balance or usage details.
type JoinReviewConfig struct {
	Enabled      bool    `json:"enabled"`
	MinSpent     float64 `json:"min_spent"`
	Question     string  `json:"question"`
	RejectReason string  `json:"reject_reason"`
}

type JoinVerifyInfo struct {
	Method        string `json:"method"`
	VerifyMessage string `json:"verify_message"`
	ReviewQAList  []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	} `json:"review_qa_list"`
}

func validateJoinReview(r *JoinReviewConfig) error {
	if r == nil {
		return nil
	}
	if !validRechargeThreshold(r.MinSpent) || (r.Enabled && r.MinSpent <= 0) {
		return errors.New("入群审核消费门槛须大于 0，最多两位小数，按后台累计消费金额判断")
	}
	for _, value := range []string{r.Question, r.RejectReason} {
		if len([]rune(value)) > 200 || strings.ContainsFunc(value, unicode.IsControl) {
			return errors.New("入群问题和拒绝原因最多 200 字，不能包含控制字符")
		}
	}
	if r.Enabled && (strings.TrimSpace(r.Question) == "" || strings.TrimSpace(r.RejectReason) == "") {
		return errors.New("开启入群审核前请填写对应的入群问题和拒绝原因")
	}
	return nil
}

func joinReviewPolicy(c Config, group string) string {
	raw, _ := json.Marshal([]any{c.AppID, c.Sandbox, group, c.groupPolicy(group).JoinReview})
	return codeHash(string(raw))
}

func joinReviewEmail(e Event, rule JoinReviewConfig) (string, bool) {
	answer := ""
	switch e.VerifyInfo.Method {
	case "verify_message":
		answer = e.VerifyInfo.VerifyMessage
	case "admin_review_qa":
		matches := 0
		for _, qa := range e.VerifyInfo.ReviewQAList {
			if strings.TrimSpace(qa.Question) == strings.TrimSpace(rule.Question) {
				answer = qa.Answer
				matches++
			}
		}
		if matches != 1 {
			return "", false
		}
	default:
		return "", false
	}
	answer = strings.TrimSpace(answer)
	if len(answer) > 254 || strings.ContainsFunc(answer, unicode.IsSpace) || strings.ContainsFunc(answer, unicode.IsControl) {
		return "", false
	}
	address, err := mail.ParseAddress(answer)
	if err != nil || address.Name != "" || address.Address != answer {
		return "", false
	}
	return strings.ToLower(answer), true
}

func (s *Service) joinReviewDecision(ctx context.Context, tx *sql.Tx, e Event, rule JoinReviewConfig) (string, string, error) {
	// Missing question metadata can be a platform/configuration problem. It is
	// not evidence that the applicant is ineligible; leave it for a human.
	if e.VerifyInfo.Method != "verify_message" {
		matches := 0
		if e.VerifyInfo.Method == "admin_review_qa" {
			for _, qa := range e.VerifyInfo.ReviewQAList {
				if strings.TrimSpace(qa.Question) == strings.TrimSpace(rule.Question) {
					matches++
				}
			}
		}
		if matches != 1 {
			return "", "manual_verification_unavailable", nil
		}
	}
	email, valid := joinReviewEmail(e, rule)
	if !valid {
		return "decline", "declined_bad_answer", nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM users WHERE lower(email)=$1 AND status='active' AND deleted_at IS NULL LIMIT 2`, email)
	if err != nil {
		return "", "", err
	}
	var uid int64
	count := 0
	for rows.Next() {
		if err = rows.Scan(&uid); err != nil {
			_ = rows.Close()
			return "", "", err
		}
		count++
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return "", "", err
	}
	if count == 0 {
		return "decline", "declined_ineligible", nil
	}
	if count != 1 {
		return "", "", errors.New("ambiguous account")
	}
	var qualified bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(actual_cost),0)>=$2::numeric FROM qq_bot_usage_metrics WHERE user_id=$1`, uid, decimal.NewFromFloat(rule.MinSpent).String()).Scan(&qualified)
	if err != nil {
		return "", "", err
	}
	if !qualified {
		return "decline", "declined_ineligible", nil
	}
	return "approve", "approved", nil
}

func (s *Service) joinReviewRecord(ctx context.Context, j Job, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE qq_bot_jobs SET kind='join_review',trigger_reason='join_request',decision_reason=$2 WHERE id=$1`, j.ID, reason)
	return err
}

func (s *Service) reviewJoinRequest(ctx context.Context, c Config, j Job, e Event) error {
	if e.JoinReviewPolicy == "" {
		return nil
	}
	if rule := c.groupPolicy(j.GroupID).JoinReview; !c.Enabled || !c.allows(j.GroupID) || c.AppID != j.AppID || rule == nil || !rule.Enabled || e.JoinReviewPolicy != joinReviewPolicy(c, j.GroupID) {
		return s.joinReviewRecord(ctx, j, "cancelled_policy_changed")
	}
	if len(e.AutoApproved) != 0 && string(e.AutoApproved) != "null" {
		return s.joinReviewRecord(ctx, j, "already_auto_approved")
	}
	applied, err := time.Parse(time.RFC3339, e.ApplyAt)
	if e.Bot || !validGroupInfoID(e.GroupID) || !validGroupInfoID(e.MemberID) || e.MemberID != j.MemberID || e.GroupID != j.GroupID ||
		e.JoinRequestID == "" || len(e.JoinRequestID) > 2048 || err != nil || time.Since(applied) > 30*time.Minute || time.Until(applied) > time.Minute {
		return s.joinReviewRecord(ctx, j, "manual_invalid_or_stale_request")
	}
	// Fetch credentials before taking the final config lock. No model is called.
	requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	token, err := s.token(requestCtx, c)
	if err != nil {
		_ = s.joinReviewRecord(ctx, j, "manual_credentials_unavailable")
		return err
	}
	tx, err := s.db.BeginTx(requestCtx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := s.lockConfig(requestCtx, tx)
	if err != nil {
		return err
	}
	rule := current.groupPolicy(j.GroupID).JoinReview
	if !current.Enabled || current.AppID != j.AppID || current.Sandbox != c.Sandbox || !current.allows(j.GroupID) ||
		rule == nil || !rule.Enabled || e.JoinReviewPolicy != joinReviewPolicy(current, j.GroupID) {
		_ = tx.Rollback()
		return s.joinReviewRecord(ctx, j, "cancelled_policy_changed")
	}
	op, reason, err := s.joinReviewDecision(requestCtx, tx, e, *rule)
	if err != nil {
		_ = tx.Rollback()
		_ = s.joinReviewRecord(ctx, j, "manual_lookup_failed")
		return err
	}
	if op == "" {
		_ = tx.Rollback()
		return s.joinReviewRecord(ctx, j, reason)
	}
	receipt, err := s.approvedJoinReceipt(requestCtx, tx, current, e, op)
	if err != nil {
		return err
	}
	// Reserve at most one attempt durably. A timeout may mean QQ applied the
	// decision; never replay an ambiguous operation or omit the request ID.
	if _, err = tx.ExecContext(requestCtx, `SELECT pg_advisory_xact_lock(71427103)`); err != nil {
		return err
	}
	var recent int
	if err = tx.QueryRowContext(requestCtx, `SELECT COUNT(*) FROM qq_bot_jobs WHERE kind='join_review' AND reply_reserved_at>NOW()-INTERVAL '1 minute'`).Scan(&recent); err != nil {
		return err
	}
	if recent >= 55 {
		_ = tx.Rollback()
		return s.joinReviewRecord(ctx, j, "manual_rate_limited")
	}
	res, err := tx.ExecContext(requestCtx, `UPDATE qq_bot_jobs SET kind='join_review',trigger_reason='join_request',decision_reason=$2,reply_reserved_at=NOW() WHERE id=$1 AND status='running' AND reply_reserved_at IS NULL`, j.ID, "attempting_"+op)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return nil
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	body := map[string]any{"op": op, "join_request_id": e.JoinRequestID}
	if op == "decline" {
		body["reject_reason"] = strings.TrimSpace(rule.RejectReason)
		body["add_to_member_blacklist"] = false
	}
	path := "/v2/groups/" + url.PathEscape(j.GroupID) + "/approval_join_request/" + url.PathEscape(j.MemberID)
	data, sendErr := s.qqRequestMethodWithToken(requestCtx, current, token, http.MethodPost, path, body)
	// QQ normally returns {}. A nonzero application code is not success, even
	// when the HTTP status is 2xx. Raw responses and applicant emails stay private.
	if sendErr == nil && len(data) > 0 {
		var result struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(data, &result) != nil || result.Code != 0 {
			sendErr = errors.New("QQ approval result unavailable")
		}
	}
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	if sendErr != nil {
		code := "manual_result_unconfirmed"
		var qqErr *qqRequestError
		if errors.As(sendErr, &qqErr) {
			code = fmt.Sprintf("manual_qq_http_%d_code_%d", qqErr.StatusCode, qqErr.Code)
		}
		_ = s.joinReviewRecord(finalCtx, j, code)
		return sendErr
	}
	if receipt != nil {
		raw, _ := json.Marshal(map[string]any{"_kitty_approved": receipt})
		_, err = s.db.ExecContext(finalCtx, `UPDATE qq_bot_jobs SET kind='join_review',trigger_reason='join_request',decision_reason=$2,payload=$3::jsonb WHERE id=$1`, j.ID, reason, string(raw))
		return err
	}
	return s.joinReviewRecord(finalCtx, j, reason)
}
