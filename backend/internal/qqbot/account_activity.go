package qqbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The adapter reads the same user slots as the gateway. nil means untracked,
// not zero; unlimited users deliberately do not acquire a concurrency slot.
type UserConcurrencyReader func(context.Context, int64, int) (*int, error)

func (s *Service) SetUserConcurrencyReader(reader UserConcurrencyReader) { s.userConcurrency = reader }

func (s *Service) personalQueryIdentity(ctx context.Context, c Config, j Job) (Config, int64, error) {
	if j.AppID == "" || j.MemberID == "" || j.AppID != c.AppID {
		return c, 0, ErrDisabled
	}
	current, err := s.Config(ctx)
	if err != nil {
		return c, 0, err
	}
	if !current.Enabled || current.AppID != j.AppID || (!isPrivateJob(j) && !current.allows(j.GroupID)) {
		return c, 0, ErrDisabled
	}
	uid, err := boundUser(ctx, s.db, current.AppID, j.MemberID)
	return current, uid, err
}

func unknownMetric(n *int) string {
	if n == nil {
		return "暂不可用"
	}
	return strconv.Itoa(*n)
}

func (s *Service) personalInfo(ctx context.Context, c Config, j Job, at time.Time) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	_, uid, err := s.personalQueryIdentity(ctx, c, j)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	var name string
	var balance float64
	var limit int
	err = s.db.QueryRowContext(ctx, `SELECT username,balance,concurrency FROM users WHERE id=$1 AND deleted_at IS NULL AND status='active'`, uid).Scan(&name, &balance, &limit)
	if err != nil {
		return "", err
	}
	var concurrent, rpm *int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if limit <= 0 || s.userConcurrency == nil {
			return
		}
		metricCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		if n, e := s.userConcurrency(metricCtx, uid, limit); e == nil {
			concurrent = n
		}
	}()
	go func() {
		defer wg.Done()
		metricCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		var n int
		if e := s.db.QueryRowContext(metricCtx, `SELECT COUNT(*) FROM usage_logs WHERE user_id=$1 AND created_at >= $2 AND created_at < $3`, uid, at.Add(-time.Minute), at).Scan(&n); e == nil {
			rpm = &n
		}
	}()
	wg.Wait()
	if strings.TrimSpace(name) == "" {
		name = "未设置"
	}
	quota, active := strconv.Itoa(limit), unknownMetric(concurrent)
	if limit <= 0 {
		quota, active = "不限", "未跟踪（不限并发）"
	}
	return fmt.Sprintf("👤 **个人信息**\n昵称：%s\n用户 ID：%d\n账户余额：USD %.6f\n总并发：%s\n当前并发：%s\n当前 RPM（近60秒）：%s\n\nRPM 仅统计已写入用量记录的请求，不含尚未落库的请求。", replyPlainLabel(limitText(name, 60)), uid, balance, quota, active, unknownMetric(rpm)), nil
}

func (s *Service) myAPIKeys(ctx context.Context, c Config, j Job, arg string) (string, error) {
	page, ok := commandPageNumber(arg)
	if !ok {
		return "用法：我的密钥 [页码]。", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, uid, err := s.personalQueryIdentity(ctx, c, j)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	// Only the redacted representation leaves PostgreSQL. Never load the full
	// credential into the bot or send it to a model, log or chat history.
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,status,CASE WHEN LENGTH(key)>12 THEN LEFT(key,4)||'…'||RIGHT(key,4) ELSE '已隐藏' END FROM api_keys WHERE user_id=$1 AND deleted_at IS NULL ORDER BY id DESC LIMIT 6 OFFSET $2`, uid, (page-1)*5)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	text := fmt.Sprintf("🔑 **我的密钥 · 第%d页**", page)
	n := 0
	for rows.Next() {
		var id int64
		var name, status, masked string
		if err = rows.Scan(&id, &name, &status, &masked); err != nil {
			return "", err
		}
		n++
		if n > 5 {
			text += fmt.Sprintf("\n\n下一页：我的密钥 %d", page+1)
			break
		}
		text += fmt.Sprintf("\n\nID %d · %s\n%s · %s", id, replyPlainLabel(limitText(name, 60)), replyPlainLabel(masked), replyPlainLabel(status))
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	if n == 0 {
		text += "\n本页没有密钥。"
	}
	return text + "\n\n查询用量：密钥用量 ID [YYYY-MM-DD]（北京时间）", nil
}

func keyUsageArgs(arg string, now time.Time) (int64, time.Time, error) {
	args := strings.Fields(arg)
	if len(args) < 1 || len(args) > 2 {
		return 0, time.Time{}, errors.New("invalid arguments")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, time.Time{}, errors.New("invalid key id")
	}
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	if len(args) == 2 {
		requested, e := time.ParseInLocation("2006-01-02", args[1], zone)
		if e != nil || requested.After(start) || requested.Before(start.AddDate(0, 0, -90)) {
			return 0, time.Time{}, errors.New("invalid date")
		}
		start = requested
	}
	return id, start, nil
}

func (s *Service) myAPIKeyUsage(ctx context.Context, c Config, j Job, arg string, now time.Time) (string, error) {
	id, start, err := keyUsageArgs(arg, now)
	if err != nil {
		return "用法：密钥用量 ID [YYYY-MM-DD]；日期限今天或过去90天，按北京时间统计。", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, uid, err := s.personalQueryIdentity(ctx, c, j)
	if errors.Is(err, sql.ErrNoRows) {
		return bindFirst, nil
	}
	if err != nil {
		return "", err
	}
	var name string
	// Both IDs are required, including for administrators using the bot.
	err = s.db.QueryRowContext(ctx, `SELECT name FROM api_keys WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, uid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "未找到属于你的有效密钥，请先发送“我的密钥”。", nil
	}
	if err != nil {
		return "", err
	}
	end := start.AddDate(0, 0, 1)
	if end.After(now) {
		end = now
	}
	var count, input, output, write, read int64
	var cost float64
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(cache_creation_tokens),0),COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id=$1 AND api_key_id=$2 AND created_at >= $3 AND created_at < $4`, uid, id, start, end).Scan(&count, &input, &output, &write, &read, &cost)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("📊 **密钥用量**\nID %d · %s\n%s（北京时间）\n请求：%d 次\n费用：USD %.6f\n输入：%s\n输出：%s\n缓存读取：%s\n缓存写入：%s\n\n仅统计网站当前留存的用量明细。", id, replyPlainLabel(limitText(name, 60)), start.Format("2006-01-02"), count, cost, formatUsageTokens(input), formatUsageTokens(output), formatUsageTokens(read), formatUsageTokens(write)), nil
}
