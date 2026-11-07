package qqbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CommunityGuide deliberately contains only advertised rules. Never return
// Config/Public(), group OpenIDs, account identities or personal activity data.
type CommunityGuide struct {
	Enabled          bool               `json:"enabled"`
	ChatEnabled      bool               `json:"chat_enabled"`
	AutoEmailBinding bool               `json:"auto_email_binding"`
	JoinReview       *CommunityJoinRule `json:"join_review,omitempty"`
	Benefits         []CommunityBenefit `json:"benefits"`
	Commands         []CommunityCommand `json:"commands"`
}

type CommunityJoinRule struct {
	MinSpent float64 `json:"min_spent"`
	Question string  `json:"question"`
}

type CommunityBenefit struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Highlight string   `json:"highlight"`
	Lines     []string `json:"lines"`
	Command   string   `json:"command"`
}

type CommunityCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// One read-only snapshot: viewing the guide cannot settle tasks, grant rewards,
// consume lottery chances, generate binding codes or send email/QQ messages.
func (s *Service) CommunityGuide(ctx context.Context) (CommunityGuide, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return CommunityGuide{}, err
	}
	defer func() { _ = tx.Rollback() }()
	bot, err := loadCommunityConfig(ctx, tx)
	if err != nil {
		return CommunityGuide{}, err
	}
	checkin, err := loadCheckinConfig(ctx, tx, bot.CheckinReward)
	if err != nil {
		return CommunityGuide{}, err
	}
	lottery, err := loadLotteryConfig(ctx, tx)
	if err != nil {
		return CommunityGuide{}, err
	}
	ranking, err := loadRankingConfig(ctx, tx, bot.AppID)
	if err != nil {
		return CommunityGuide{}, err
	}
	guide := communityGuide(bot, checkin, lottery, ranking)
	return guide, tx.Commit()
}

// The public reader does not retrieve or decrypt credentials, prompts or the
// private knowledge base. Keep this SQL allowlist separate from admin config.
func loadCommunityConfig(ctx context.Context, db queryRower) (Config, error) {
	bot := DefaultConfig()
	var raw string
	err := db.QueryRowContext(ctx, `SELECT jsonb_build_object(
 'enabled',value::jsonb->'enabled','sandbox',value::jsonb->'sandbox',
 'app_id',value::jsonb->'app_id','allowed_groups',value::jsonb->'allowed_groups',
 'group_policies',value::jsonb->'group_policies','chat_disabled',value::jsonb->'chat_disabled',
 'checkin_reward',value::jsonb->'checkin_reward','invite_enabled',value::jsonb->'invite_enabled',
 'invite_reward',value::jsonb->'invite_reward','invite_daily_limit',value::jsonb->'invite_daily_limit',
 'invite_min_recharge_total',value::jsonb->'invite_min_recharge_total',
 'invite_min_recharge_30d',value::jsonb->'invite_min_recharge_30d',
 'invite_recharge_window_days',value::jsonb->'invite_recharge_window_days')::text
 FROM settings WHERE key=$1`, configKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return bot, nil
	}
	if err != nil {
		return bot, err
	}
	err = json.Unmarshal([]byte(raw), &bot)
	return bot, err
}

func communityGuide(bot Config, checkin CheckinConfig, lottery LotteryConfig, ranking RankingConfig) CommunityGuide {
	g := CommunityGuide{Enabled: bot.Enabled && !bot.Sandbox && len(bot.AllowedGroups) > 0,
		Benefits: []CommunityBenefit{}, Commands: []CommunityCommand{}}
	if !g.Enabled {
		return g
	}
	activities := false
	for _, group := range bot.AllowedGroups {
		p := bot.groupPolicy(group)
		activities = activities || !p.DisableActivities
		g.ChatEnabled = g.ChatEnabled || (!bot.ChatDisabled && !p.DisableChat)
		// With several groups the public join link may not identify which policy
		// applies. Do not present another group's threshold as a universal rule.
		if len(bot.AllowedGroups) == 1 {
			if r := p.JoinReview; r != nil && r.Enabled {
				g.JoinReview = &CommunityJoinRule{MinSpent: r.MinSpent, Question: r.Question}
				g.AutoEmailBinding = p.Welcome != nil && p.Welcome.Enabled && p.Welcome.AutoEmailBinding
			}
		}
	}
	g.Commands = append(g.Commands,
		CommunityCommand{"帮助", "查看机器人支持的指令"},
		CommunityCommand{"我的余额", "查询已绑定网站账号的余额"},
		CommunityCommand{"今日用量", "查询本人当天的模型使用情况"},
		CommunityCommand{"价格 deepseek-flash", "查询模型当前价格"},
		CommunityCommand{"渠道状态", "查看站点公开的渠道状态"},
	)
	if !activities {
		return g
	}
	if checkin.Enabled {
		amount := fmt.Sprintf("%g", checkin.MinAmount)
		if checkin.MinAmount != checkin.MaxAmount {
			amount += fmt.Sprintf("–%g", checkin.MaxAmount)
		}
		g.Benefits = append(g.Benefits, CommunityBenefit{ID: "checkin", Title: "每日签到", Highlight: amount + " 站内余额",
			Lines: []string{"每天每个网站账号可签到一次，按北京时间计算。", "充值条件：" + checkin.describe(), "随机金额按当前活动概率发放，并非每次都获得最高奖励。"}, Command: "签到"})
		g.Commands = append(g.Commands, CommunityCommand{"签到", "领取今日签到奖励"})
		if r := checkin.streakReward(); r.enabled() {
			g.Benefits = append(g.Benefits, CommunityBenefit{ID: "streak", Title: "连续签到", Highlight: r.description(),
				Lines: []string{fmt.Sprintf("连续签到每满 %d 天获得一次奖励。", checkin.streakDays()), "保持每天签到；中断会影响连续天数。"}, Command: "签到"})
		}
	}
	for _, set := range []struct {
		tasks    []GrowthTask
		periodic bool
	}{{checkin.Growth.tasks(), false}, {checkin.Growth.PeriodicTasks, true}} {
		for _, task := range set.tasks {
			if !task.Enabled || !task.reward().enabled() {
				continue
			}
			title := "成长奖励"
			if task.Kind == "binding" {
				title = "完成账号绑定"
			} else if task.Kind == "usage_count" {
				title = "API 调用成长奖励"
			} else if task.Kind == "checkin_total" || task.Kind == "checkin_streak" {
				title = "签到成长奖励"
			}
			frequency := "每个网站账号限领一次，重新绑定不会重复发放。"
			prefix := "task:"
			if set.periodic {
				frequency = "周期任务，每累计达标一轮可再次获得奖励。"
				prefix = "periodic:"
			}
			g.Benefits = append(g.Benefits, CommunityBenefit{ID: prefix + task.ID, Title: title, Highlight: task.reward().description(),
				Lines: []string{"目标：" + siteGrowthTarget(task), frequency}, Command: "成长任务"})
		}
	}
	if len(checkin.Growth.tasks()) > 0 || len(checkin.Growth.PeriodicTasks) > 0 {
		g.Commands = append(g.Commands, CommunityCommand{"成长任务", "查看任务门槛、进度和奖励"})
	}
	if lottery.Enabled {
		lines := []string{fmt.Sprintf("累计有效充值每满 %g 站内额度，赠 %d 次抽奖机会；活动赠额不算充值。", lottery.RechargeStep, lottery.ChancesPerStep), "每抽消耗 1 次机会；支持 1–10 连抽，机会不足时整批不抽。"}
		for _, p := range lottery.Prizes {
			lines = append(lines, fmt.Sprintf("%s：站内余额 %g，概率 %g%%。", p.Name, p.Amount, float64(p.ProbabilityBPS)/100))
		}
		if lottery.GuaranteeEnabled {
			lines = append(lines, fmt.Sprintf("累计每抽 %d 次触发一次保底，普通中奖不重置进度。", lottery.GuaranteeEvery))
			for _, p := range lottery.GuaranteePrizes {
				lines = append(lines, fmt.Sprintf("保底 %s：站内余额 %g，概率 %g%%。", p.Name, p.Amount, float64(p.ProbabilityBPS)/100))
			}
		} else {
			lines = append(lines, "当前未开启保底，抽奖可能获得谢谢参与。")
		}
		g.Benefits = append(g.Benefits, CommunityBenefit{ID: "lottery", Title: "幸运抽奖", Highlight: "签到、成长和充值可攒机会", Lines: lines, Command: "抽奖"})
		g.Commands = append(g.Commands, CommunityCommand{"抽奖", "使用 1 次机会参与抽奖"}, CommunityCommand{"抽奖次数", "查询剩余抽奖机会"}, CommunityCommand{"我的奖励", "查看已获得的奖励"})
	}
	if ranking.Enabled {
		g.Commands = append(g.Commands, CommunityCommand{"今日排行榜", "查看今日 Token 榜和消费榜"})
		if ranking.RewardsEnabled {
			lines := []string{"按北京时间统计，次日自动结算；两个榜单独立奖励，可同时获奖。", "奖励规则生效日期：" + ranking.EffectiveDay}
			for _, board := range []struct {
				name   string
				prizes []RankingPrize
			}{{"Token 消耗榜", ranking.TokenRewards}, {"消费金额榜", ranking.AmountRewards}} {
				for _, p := range board.prizes {
					if p.Reward.enabled() {
						lines = append(lines, fmt.Sprintf("%s第 %d 名：%s。", board.name, p.Rank, p.Reward.description()))
					}
				}
			}
			g.Benefits = append(g.Benefits, CommunityBenefit{ID: "rankings", Title: "每日双榜奖励", Highlight: "Token 榜 · 消费榜", Lines: lines, Command: "今日排行榜"})
		}
	}
	if bot.InviteEnabled && bot.InviteReward > 0 && bot.InviteDailyLimit > 0 {
		g.Benefits = append(g.Benefits, CommunityBenefit{ID: "invite", Title: "邀请奖励", Highlight: fmt.Sprintf("%g 站内余额", bot.InviteReward), Lines: strings.Split(siteInviteRules(bot), "\n")[1:], Command: "邀请奖励"})
	}
	return g
}
