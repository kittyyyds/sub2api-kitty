package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func readIQLifecycle(ctx context.Context, q planRowQuerier, accountID int64) (*service.IQLifecycleState, error) {
	var s service.IQLifecycleState
	var cfg, samples []byte
	err := q.QueryRowContext(ctx, `SELECT config,correct_streak,answer29_streak,passed_at,quarantined_at,action_error,fingerprint,last_started_at,evidence_samples,quarantine_pending FROM account_iq_lifecycle WHERE account_id=$1`, accountID).Scan(&cfg, &s.CorrectStreak, &s.Answer29Streak, &s.PassedAt, &s.QuarantinedAt, &s.ActionError, &s.Fingerprint, &s.LastStartedAt, &samples, &s.QuarantinePending)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(samples, &s.Samples); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(cfg, &s.Config); err != nil {
		return nil, err
	}
	return &s, nil
}

// Plan, account, lifecycle, result and scheduler events commit as one unit.
func (r *scheduledTestResultRepository) createIQLifecycleResult(ctx context.Context, result *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var accountID int64
	var enabled bool
	var revision time.Time
	var lease *time.Time
	var cfgRaw []byte
	var model string
	err = tx.QueryRowContext(ctx, `SELECT account_id,enabled,updated_at,running_until,model_id,pelican_config FROM scheduled_test_plans WHERE id=$1 FOR UPDATE`, result.PlanID).Scan(&accountID, &enabled, &revision, &lease, &model, &cfgRaw)
	if err != nil {
		return nil, err
	}
	var cfg service.PelicanTestConfig
	if err = json.Unmarshal(cfgRaw, &cfg); err != nil {
		return nil, err
	}
	var concurrency int
	var status string
	var schedulable bool
	var expires *time.Time
	var autoPause bool
	err = tx.QueryRowContext(ctx, `SELECT concurrency,status,schedulable,expires_at,auto_pause_on_expired FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, accountID).Scan(&concurrency, &status, &schedulable, &expires, &autoPause)
	if err != nil {
		return nil, err
	}
	out, err := insertScheduledTestResult(ctx, tx, result)
	if err != nil {
		return nil, err
	}
	state, err := readIQLifecycle(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	var owns bool
	if state != nil {
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(plan_id=$2,false) FROM account_iq_lifecycle WHERE account_id=$1`, accountID, result.PlanID).Scan(&owns)
		if err != nil {
			return nil, err
		}
	}
	valid := result.PelicanConfig != nil && enabled && owns && !result.PlanUpdatedAt.IsZero() && result.PlanUpdatedAt.Equal(revision) && result.LeaseUntil != nil && lease != nil && lease.Equal(*result.LeaseUntil) && lease.After(time.Now()) && cfg.ScheduleMode == service.IQCompletionInterval && cfg.QuestionKind == "candy" && cfg.TestChannel == "account" && service.IQConfigFingerprint(model, &cfg) == service.IQConfigFingerprint(result.PelicanConfig.ModelID, result.PelicanConfig)
	if valid && service.IQFinalAnswer(result) != "waiting" && (state.LastStartedAt == nil || result.StartedAt.After(*state.LastStartedAt)) {
		next := service.AdvanceIQLifecycle(*state, result)
		kind := ""
		answer := service.IQFinalAnswer(result)
		streak := next.CorrectStreak
		if answer == "29" {
			streak = next.Answer29Streak
		}
		if streak <= 1 && !next.QuarantinePending {
			next.Samples = nil
		}
		if !next.QuarantinePending && (answer == "21" || answer == "29") {
			if len(next.Samples) < streak {
				next.Samples = append(next.Samples, service.IQEvidenceSample{ResultID: out.ID, StartedAt: result.StartedAt, Answer: result.ResponseText})
			}
		} else if !next.QuarantinePending {
			next.Samples = nil
		}
		samples, _ := json.Marshal(next.Samples)
		if next.QuarantinedAt == nil && next.Config.QuarantineEnabled && (next.Answer29Streak >= 6 || next.QuarantinePending) {
			var targetValid bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND platform='openai' AND status='active' AND deleted_at IS NULL)`, next.Config.QuarantineGroupID).Scan(&targetValid)
			if err != nil {
				return nil, err
			}
			if !targetValid {
				next.QuarantinePending = true
				next.ActionError = "隔离目标分组不可用，已关闭客户调度并冻结升档；请在本账号测试设置中重新选择隔离分组"
				kind = "quarantine_failed"
			} else {
				// Lock the group against concurrent deletion before replacing memberships.
				var gid int64
				err = tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE id=$1 AND platform='openai' AND status='active' AND deleted_at IS NULL FOR SHARE`, next.Config.QuarantineGroupID).Scan(&gid)
				if err != nil {
					return nil, err
				}
				before, err := qualityGroups(ctx, tx, accountID)
				if err != nil {
					return nil, err
				}
				if _, err = tx.ExecContext(ctx, `UPDATE account_iq_lifecycle SET original_groups=$2 WHERE account_id=$1`, accountID, string(before)); err != nil {
					return nil, err
				}
				if _, err = tx.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1`, accountID); err != nil {
					return nil, err
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id,priority,created_at) SELECT id,$2,priority,NOW() FROM accounts WHERE id=$1`, accountID, gid); err != nil {
					return nil, err
				}
				if _, err = tx.ExecContext(ctx, `UPDATE accounts SET updated_at=clock_timestamp() WHERE id=$1`, accountID); err != nil {
					return nil, err
				}
				if err = qualityGroupsChanged(ctx, tx, accountID, before, []qualityGroup{{GroupID: gid}}); err != nil {
					return nil, err
				}
				now := time.Now().UTC()
				next.QuarantinedAt = &now
				next.QuarantinePending = false
				next.ActionError = ""
				kind = "quarantined"
			}
		}
		if next.PassedAt == nil && next.QuarantinedAt == nil && !next.QuarantinePending && next.CorrectStreak >= next.Config.CorrectRequired {
			if status != service.StatusActive || (autoPause && expires != nil && !expires.After(time.Now())) {
				next.ActionError = "糖果验证已达标，账号当前状态或有效期不允许开放调度"
			} else {
				now := time.Now().UTC()
				next.PassedAt = &now
				next.ActionError = ""
				kind = "passed"
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE account_iq_lifecycle SET correct_streak=$2,answer29_streak=$3,passed_at=$4,quarantined_at=$5,action_error=$6,fingerprint=$7,last_started_at=$8,last_result_id=$9,evidence_samples=$10,quarantine_pending=$11,updated_at=NOW() WHERE account_id=$1`, accountID, next.CorrectStreak, next.Answer29Streak, next.PassedAt, next.QuarantinedAt, next.ActionError, next.Fingerprint, next.LastStartedAt, out.ID, string(samples), next.QuarantinePending)
		if err != nil {
			return nil, err
		}
		if kind == "quarantine_failed" {
			if _, err = tx.ExecContext(ctx, `UPDATE accounts SET schedulable=false,updated_at=clock_timestamp() WHERE id=$1`, accountID); err != nil {
				return nil, err
			}
			if err = enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
				return nil, err
			}
		}
		if kind == "passed" {
			// State is persisted first so the database admission hold allows this update.
			_, err = tx.ExecContext(ctx, `UPDATE accounts SET concurrency=$2,schedulable=true,extra=COALESCE(extra,'{}'::jsonb)-'auto_config_concurrency',updated_at=clock_timestamp() WHERE id=$1`, accountID, next.Config.PassedConcurrency)
			if err != nil {
				return nil, err
			}
			if err = enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
				return nil, err
			}
		}
		if kind != "" {
			evidence, _ := json.Marshal(map[string]any{"samples": json.RawMessage(samples), "config": next.Config, "fingerprint": next.Fingerprint, "result_id": out.ID, "correct_streak": next.CorrectStreak, "answer29_streak": next.Answer29Streak, "previous_concurrency": concurrency, "previous_schedulable": schedulable, "target_group_id": next.Config.QuarantineGroupID, "error": next.ActionError})
			_, err = tx.ExecContext(ctx, `INSERT INTO account_iq_lifecycle_events(account_id,plan_id,kind,evidence) VALUES($1,$2,$3,$4)`, accountID, result.PlanID, kind, string(evidence))
			if err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// Target repair never reopens customer scheduling or restores former groups.
func (r *scheduledTestPlanRepository) SetIQQuarantineTarget(ctx context.Context, planID, groupID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accountID, id int64
	if err = tx.QueryRowContext(ctx, `SELECT account_id FROM scheduled_test_plans WHERE id=$1 FOR UPDATE`, planID).Scan(&accountID); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, accountID).Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE id=$1 AND platform='openai' AND status='active' AND deleted_at IS NULL FOR SHARE`, groupID).Scan(&id); err != nil {
		return fmt.Errorf("请选择启用中的 OpenAI 隔离分组")
	}
	result, err := tx.ExecContext(ctx, `UPDATE account_iq_lifecycle SET config=jsonb_set(jsonb_set(config,'{quarantine_group_id}',to_jsonb($2::bigint)),'{quarantine_enabled}','true'),updated_at=NOW() WHERE account_id=$1 AND plan_id=$3 AND quarantined_at IS NULL`, accountID, groupID, planID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("此账号未加入自动验证或已完成隔离，不能自动更换目标")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO account_iq_lifecycle_events(account_id,plan_id,kind,evidence) VALUES($1,$2,'target_changed',jsonb_build_object('target_group_id',$3::bigint))`, accountID, planID, groupID); err != nil {
		return err
	}
	return tx.Commit()
}
