package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const candyTestTimeout = 3 * time.Minute

// One bounded worker per process; a transaction lock also enforces the global
// two-job limit across replicas. Reads never enqueue jobs or contact upstreams.
type CandyTestService struct {
	db       *sql.DB
	groups   GroupRepository
	keys     APIKeyRepository
	client   *http.Client
	endpoint string
	ctx      context.Context
	cancel   context.CancelFunc
	once     sync.Once
	wg       sync.WaitGroup
}

func NewCandyTestService(db *sql.DB, groups GroupRepository, keys APIKeyRepository, cfg *config.Config) *CandyTestService {
	ctx, cancel := context.WithCancel(context.Background())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // The normal API is reached inside this app container.
	transport.MaxConnsPerHost = 2
	return &CandyTestService{db: db, groups: groups, keys: keys, ctx: ctx, cancel: cancel,
		endpoint: fmt.Sprintf("http://127.0.0.1:%d/v1/responses", cfg.Server.Port),
		client:   &http.Client{Transport: transport, Timeout: candyTestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func ProvideCandyTestService(db *sql.DB, groups GroupRepository, keys APIKeyRepository, cfg *config.Config) *CandyTestService {
	s := NewCandyTestService(db, groups, keys, cfg)
	s.Start()
	return s
}

func (s *CandyTestService) Start() {
	s.once.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					if err := s.tick(s.ctx); err != nil && s.ctx.Err() == nil {
						slog.Error("candy_test.worker", "error", err)
					}
				}
			}
		}()
	})
}

func (s *CandyTestService) Stop() { s.cancel(); s.wg.Wait(); s.client.CloseIdleConnections() }

func (s *CandyTestService) Save(ctx context.Context, input CandyTestConfig) (*CandyTestConfig, error) {
	c, err := normalizeCandyTestConfig(input)
	if err != nil {
		return nil, err
	}
	// A disabled/expired key must never prevent an administrator from stopping
	// its schedule or hiding existing results.
	if c.ID == 0 || c.Enabled {
		group, groupErr := s.groups.GetByID(ctx, c.GroupID)
		if groupErr != nil {
			return nil, groupErr
		}
		if group == nil || group.Platform != PlatformOpenAI || group.Status != StatusActive {
			return nil, infraerrors.BadRequest("CANDY_TEST_GROUP", "仅支持有效的 GPT / OpenAI 分组")
		}
		if _, err = s.testKey(ctx, c); err != nil {
			return nil, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Serialize configuration creation to keep the feature bounded.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(78140201)"); err != nil {
		return nil, err
	}
	if c.ID == 0 {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM kitty_candy_tests").Scan(&count); err != nil {
			return nil, err
		}
		if count >= 20 {
			return nil, infraerrors.BadRequest("CANDY_TEST_LIMIT", "最多配置 20 个测试分组")
		}
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM kitty_candy_tests WHERE group_id=$1)", c.GroupID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, infraerrors.Conflict("CANDY_TEST_EXISTS", "此分组已配置，请编辑已有测试")
		}
		data, _ := json.Marshal(c)
		err = tx.QueryRowContext(ctx, `INSERT INTO kitty_candy_tests(group_id,config,enabled,published,next_run_at)
			VALUES ($1,$2,$3,$4,CASE WHEN $3 THEN NOW()+($5 * INTERVAL '1 minute') ELSE NULL END) RETURNING id`,
			c.GroupID, string(data), c.Enabled, c.Published, c.IntervalMinutes).Scan(&c.ID)
	} else {
		var groupID int64
		if err = tx.QueryRowContext(ctx, "SELECT group_id FROM kitty_candy_tests WHERE id=$1 FOR UPDATE", c.ID).Scan(&groupID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, infraerrors.NotFound("CANDY_TEST_NOT_FOUND", "测试不存在")
			}
			return nil, err
		}
		if groupID != c.GroupID {
			return nil, infraerrors.BadRequest("CANDY_TEST_GROUP_CHANGE", "编辑时不能更换分组，请新建对应分组的测试")
		}
		data, _ := json.Marshal(c)
		_, err = tx.ExecContext(ctx, `UPDATE kitty_candy_tests SET config=$2,enabled=$3,published=$4,
			next_run_at=CASE WHEN $3 THEN NOW()+($5 * INTERVAL '1 minute') ELSE NULL END,updated_at=NOW() WHERE id=$1`,
			c.ID, string(data), c.Enabled, c.Published, c.IntervalMinutes)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE kitty_candy_test_runs SET status='cancelled',finished_at=NOW(),
				result='{"message":"配置已变更，未开始的旧任务已取消，没有发出模型请求"}'::jsonb
				WHERE test_id=$1 AND status='queued'`, c.ID)
		}
		if err == nil {
			err = pruneCandyHistory(ctx, tx, c.ID)
		}
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &c, nil
}

// RunNow is admin-only at the router and atomically deduplicates clicks.
func (s *CandyTestService) RunNow(ctx context.Context, id int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var snapshot []byte
	if err = tx.QueryRowContext(ctx, "SELECT config FROM kitty_candy_tests WHERE id=$1 FOR UPDATE", id).Scan(&snapshot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, infraerrors.NotFound("CANDY_TEST_NOT_FOUND", "测试不存在")
		}
		return 0, err
	}
	var runID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO kitty_candy_test_runs(test_id,status,snapshot) VALUES($1,'queued',$2)
		ON CONFLICT (test_id) WHERE status IN ('queued','running') DO NOTHING RETURNING id`, id, string(snapshot)).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, "SELECT id FROM kitty_candy_test_runs WHERE test_id=$1 AND status IN ('queued','running')", id).Scan(&runID)
	}
	if err != nil {
		return 0, err
	}
	// A manual run resets the scheduled deadline, avoiding an immediate second test.
	if _, err = tx.ExecContext(ctx, `UPDATE kitty_candy_tests SET next_run_at=CASE WHEN enabled
		THEN NOW()+((config->>'interval_minutes')::int * INTERVAL '1 minute') ELSE NULL END WHERE id=$1`, id); err != nil {
		return 0, err
	}
	return runID, tx.Commit()
}

func (s *CandyTestService) List(ctx context.Context, admin bool) ([]CandyTestCard, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.config,t.next_run_at,
		(SELECT COUNT(*) FROM kitty_candy_test_runs r WHERE r.test_id=t.id) FROM kitty_candy_tests t
		JOIN groups g ON g.id=t.group_id WHERE ($1 OR (t.published AND g.status='active' AND g.deleted_at IS NULL)) ORDER BY t.id LIMIT 20`, admin)
	if err != nil {
		return nil, err
	}
	cards := make([]CandyTestCard, 0)
	for rows.Next() {
		var id int64
		var data []byte
		var next *time.Time
		var count int
		if err = rows.Scan(&id, &data, &next, &count); err != nil {
			rows.Close()
			return nil, err
		}
		var c CandyTestConfig
		if err = json.Unmarshal(data, &c); err != nil {
			rows.Close()
			return nil, err
		}
		c.ID = id
		card := CandyTestCard{ID: id, Label: c.Label, Model: c.Model, ReasoningEffort: c.ReasoningEffort, Enabled: c.Enabled, IntervalMinutes: c.IntervalMinutes, NextRunAt: next, History: []CandyTestRun{}}
		card.GroupID = c.GroupID
		if admin {
			card.Config = &c
		}
		card.HistoryCount = min(count, 12)
		cards = append(cards, card)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range cards {
		history, err := s.history(ctx, cards[i].ID, 1)
		if err != nil {
			return nil, err
		}
		cards[i].History = history
	}
	return cards, nil
}

func (s *CandyTestService) History(ctx context.Context, id int64, admin bool) ([]CandyTestRun, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kitty_candy_tests t JOIN groups g ON g.id=t.group_id
		WHERE t.id=$1 AND ($2 OR (t.published AND g.status='active' AND g.deleted_at IS NULL)))`, id, admin).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, infraerrors.NotFound("CANDY_TEST_NOT_FOUND", "测试不存在或未公开")
	}
	return s.history(ctx, id, 12)
}

// Return only public test ownership for authorization, never private config.
func (s *CandyTestService) PublishedGroupID(ctx context.Context, id int64) (int64, error) {
	var groupID int64
	err := s.db.QueryRowContext(ctx, `SELECT t.group_id FROM kitty_candy_tests t JOIN groups g ON g.id=t.group_id
		WHERE t.id=$1 AND t.published AND g.status='active' AND g.deleted_at IS NULL`, id).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, infraerrors.NotFound("CANDY_TEST_NOT_FOUND", "测试不存在或未公开")
	}
	return groupID, err
}

func (s *CandyTestService) history(ctx context.Context, id int64, limit int) ([]CandyTestRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,status,snapshot,result,created_at,finished_at FROM kitty_candy_test_runs WHERE test_id=$1 ORDER BY id DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]CandyTestRun, 0)
	for rows.Next() {
		var run CandyTestRun
		var snapshot, raw []byte
		var status string
		var created time.Time
		var finished *time.Time
		var runID int64
		if err = rows.Scan(&runID, &status, &snapshot, &raw, &created, &finished); err != nil {
			return nil, err
		}
		var config CandyTestConfig
		if err = json.Unmarshal(snapshot, &config); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &run); err != nil {
			return nil, err
		}
		run.ID = runID
		run.TestID = id
		run.Status = status
		run.CreatedAt = created
		run.FinishedAt = finished
		run.Question = config.Question
		if len(config.ExpectedAnswers) > 0 {
			run.StandardAnswer = config.ExpectedAnswers[0]
		}
		run.Model = config.Model
		run.ReasoningEffort = config.ReasoningEffort
		// Re-evaluate complete legacy answers against their original answer key.
		// Reads never write records, queue work, or call the model. Errors and
		// incomplete or ambiguous legacy responses remain untouched.
		if (run.Status == "passed" || run.Status == "failed") && strings.TrimSpace(run.Output) != "" {
			judgedStatus, judgedAnswer := judgeCandyAnswer(run.Output, config.ExpectedAnswers)
			if judgedStatus != run.Status || judgedAnswer != run.Answer {
				run.Status, run.Answer = judgedStatus, judgedAnswer
				run.Message = "已按保存的回答原文重新核对，未重新调用模型"
			}
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *CandyTestService) tick(ctx context.Context) error {
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	tx, err := s.db.BeginTx(dbCtx, nil)
	if err != nil {
		cancel()
		return err
	}
	defer tx.Rollback()
	defer cancel()
	if _, err = tx.ExecContext(dbCtx, "SELECT pg_advisory_xact_lock(78140202)"); err != nil {
		return err
	}
	// Never retry an interrupted request: it may already have consumed tokens.
	if _, err = tx.ExecContext(dbCtx, `UPDATE kitty_candy_test_runs SET status='error',finished_at=NOW(),
		result='{"message":"测试进程中断或超过期限；本轮未自动重试"}'::jsonb
		WHERE status='running' AND started_at<NOW()-INTERVAL '5 minutes'`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(dbCtx, `WITH due AS (
		SELECT t.id,t.config FROM kitty_candy_tests t JOIN groups g ON g.id=t.group_id
		WHERE t.enabled AND t.next_run_at<=NOW() AND g.status='active' AND g.deleted_at IS NULL
		FOR UPDATE OF t SKIP LOCKED
	), queued AS (
		INSERT INTO kitty_candy_test_runs(test_id,status,snapshot) SELECT id,'queued',config FROM due
		ON CONFLICT (test_id) WHERE status IN ('queued','running') DO NOTHING
	)
	UPDATE kitty_candy_tests SET next_run_at=NOW()+((config->>'interval_minutes')::int * INTERVAL '1 minute')
	WHERE id IN (SELECT id FROM due)`); err != nil {
		return err
	}
	var running int
	if err = tx.QueryRowContext(dbCtx, "SELECT COUNT(*) FROM kitty_candy_test_runs WHERE status='running'").Scan(&running); err != nil {
		return err
	}
	if running >= 2 {
		return tx.Commit()
	}
	var id, testID int64
	var snapshot []byte
	err = tx.QueryRowContext(dbCtx, `UPDATE kitty_candy_test_runs SET status='running',started_at=NOW()
		WHERE id=(SELECT id FROM kitty_candy_test_runs WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id,test_id,snapshot`).Scan(&id, &testID, &snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	cancel()
	var config CandyTestConfig
	if err = json.Unmarshal(snapshot, &config); err != nil {
		return err
	}
	config.ID = testID
	runCtx, stop := context.WithTimeout(ctx, candyTestTimeout)
	run := s.executeSafely(runCtx, config)
	stop()
	// Persist outcome after cancellation too, using a short independent context.
	writeCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	data, _ := json.Marshal(run)
	if _, err = s.db.ExecContext(writeCtx, `UPDATE kitty_candy_test_runs SET status=$2,result=$3,finished_at=NOW()
		WHERE id=$1 AND status='running'`, id, run.Status, string(data)); err != nil {
		return err
	}
	return pruneCandyHistory(writeCtx, s.db, testID)
}

type candyHistoryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func pruneCandyHistory(ctx context.Context, db candyHistoryExecer, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM kitty_candy_test_runs WHERE test_id=$1 AND status NOT IN ('queued','running')
		AND id NOT IN (SELECT id FROM kitty_candy_test_runs WHERE test_id=$1 ORDER BY id DESC LIMIT 12)`, id)
	return err
}

func (s *CandyTestService) executeSafely(ctx context.Context, c CandyTestConfig) (run CandyTestRun) {
	start := time.Now()
	defer func() {
		run.LatencyMS = time.Since(start).Milliseconds()
		if recover() != nil {
			run.Status = "error"
			run.Message = "测试执行异常；本轮未自动重试"
		}
	}()
	return s.execute(ctx, c)
}

type CandyTestKeyOption struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

// Only administrator-owned keys can be used, preventing tests from charging a
// customer's balance. The picker exposes names and IDs, never key material.
func (s *CandyTestService) KeyOptions(ctx context.Context) ([]CandyTestKeyOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k.id AS id,k.name AS name,g.id AS group_id,g.name AS group_name FROM api_keys k
		JOIN users u ON u.id=k.user_id JOIN groups g ON g.id=k.group_id
		WHERE u.role='admin' AND u.status='active' AND u.deleted_at IS NULL
		AND k.status='active' AND k.deleted_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>NOW())
		AND g.platform='openai' AND g.status='active' AND g.deleted_at IS NULL ORDER BY g.id,k.id LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CandyTestKeyOption, 0)
	for rows.Next() {
		var item CandyTestKeyOption
		if err = rows.Scan(&item.ID, &item.Name, &item.GroupID, &item.GroupName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *CandyTestService) testKey(ctx context.Context, c CandyTestConfig) (*APIKey, error) {
	k, err := s.keys.GetByID(ctx, c.APIKeyID)
	if err != nil {
		return nil, infraerrors.BadRequest("CANDY_TEST_KEY", "测试 Key 不存在或已删除")
	}
	if k == nil || k.GroupID == nil || *k.GroupID != c.GroupID || !k.IsActive() || k.IsExpired() || k.IsQuotaExhausted() {
		return nil, infraerrors.BadRequest("CANDY_TEST_KEY", "测试 Key 已停用、到期、额度不足或分组已变更")
	}
	var admin bool
	if err = s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='admin' AND status='active' AND deleted_at IS NULL)", k.UserID).Scan(&admin); err != nil {
		return nil, err
	}
	if !admin {
		return nil, infraerrors.BadRequest("CANDY_TEST_KEY_OWNER", "请选择管理员账号创建的测试 Key，避免扣除客户余额")
	}
	return k, nil
}
