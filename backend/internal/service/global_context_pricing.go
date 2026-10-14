package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"math"
	"strings"
	"sync"
	"time"
)

// Global rules override only context ladders. They never replace group base prices,
// account cost statistics, service-tier multipliers or non-token billing modes.
type GlobalContextRule struct {
	ID                   int64     `json:"id"`
	Model                string    `json:"model"`
	Platform             string    `json:"platform"`
	ThresholdTokens      int       `json:"threshold_tokens"`
	InputMultiplier      float64   `json:"input_multiplier"`
	OutputMultiplier     float64   `json:"output_multiplier"`
	CacheReadMultiplier  float64   `json:"cache_read_multiplier"`
	CacheWriteMultiplier float64   `json:"cache_write_multiplier"`
	Enabled              bool      `json:"enabled"`
	Revision             int64     `json:"revision"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (r *GlobalContextRule) Validate() error {
	r.Model = strings.ToLower(strings.TrimSpace(r.Model))
	r.Platform = strings.ToLower(strings.TrimSpace(r.Platform))
	if r.Model == "" || len(r.Model) > 200 || strings.ContainsAny(r.Model, "*?\r\n\t ") || r.Platform == "" || len(r.Platform) > 40 {
		return errors.New("请选择平台并填写精确模型 ID（不支持通配符）")
	}
	if r.ThresholdTokens < 1 || r.ThresholdTokens > 100_000_000 {
		return errors.New("上下文阈值须为 1–100000000 个 token")
	}
	for _, m := range []float64{r.InputMultiplier, r.OutputMultiplier, r.CacheReadMultiplier, r.CacheWriteMultiplier} {
		if math.IsNaN(m) || math.IsInf(m, 0) || m < 0.01 || m > 100 {
			return errors.New("倍率须为 0.01–100 的有限数值")
		}
	}
	if r.ID < 0 || r.Revision < 0 {
		return errors.New("无效的规则版本")
	}
	return nil
}

type globalContextRuleReader interface {
	Find(context.Context, string) (*GlobalContextRule, error)
}
type GlobalContextRuleStore struct {
	db        *sql.DB
	mu        sync.RWMutex
	refreshMu sync.Mutex
	snapshot  map[string]GlobalContextRule
	refreshAt time.Time
}

// Prime must succeed before this instance accepts traffic. A transient refresh
// failure then uses the last verified snapshot instead of turning usage free.
func (s *GlobalContextRuleStore) Prime(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return s.refreshLocked(ctx)
}
func (s *GlobalContextRuleStore) refreshLocked(ctx context.Context) error {
	rules, err := s.List(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshAt = time.Now().Add(time.Second)
	if err != nil {
		return err
	}
	next := make(map[string]GlobalContextRule, len(rules))
	for _, r := range rules {
		if r.Enabled {
			next[r.Model] = *r
		}
	}
	s.snapshot = next
	return nil
}

func NewGlobalContextRuleStore(db *sql.DB) *GlobalContextRuleStore {
	return &GlobalContextRuleStore{db: db}
}

const globalContextColumns = `id,model,platform,threshold_tokens,input_multiplier,output_multiplier,cache_read_multiplier,cache_write_multiplier,enabled,revision,updated_at`

type globalContextScanner interface{ Scan(...any) error }

func scanGlobalContextRule(s globalContextScanner) (*GlobalContextRule, error) {
	r := &GlobalContextRule{}
	err := s.Scan(&r.ID, &r.Model, &r.Platform, &r.ThresholdTokens, &r.InputMultiplier, &r.OutputMultiplier, &r.CacheReadMultiplier, &r.CacheWriteMultiplier, &r.Enabled, &r.Revision, &r.UpdatedAt)
	return r, err
}
func boundedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 2*time.Second)
}

// Only one caller refreshes the snapshot. Concurrent billing keeps using the
// last verified snapshot instead of queuing behind a slow/unavailable database.
func (s *GlobalContextRuleStore) Find(ctx context.Context, model string) (*GlobalContextRule, error) {
	s.mu.RLock()
	empty, stale := s.snapshot == nil, time.Now().After(s.refreshAt)
	s.mu.RUnlock()
	if empty {
		if err := s.Prime(ctx); err != nil {
			return nil, err
		}
	} else if stale && s.refreshMu.TryLock() {
		err := s.refreshLocked(ctx)
		s.refreshMu.Unlock()
		if err != nil {
			logger.LegacyPrintf("service.context_pricing", "rule refresh failed; retaining last verified pricing snapshot")
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.snapshot[strings.ToLower(strings.TrimSpace(model))]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (s *GlobalContextRuleStore) List(ctx context.Context) ([]*GlobalContextRule, error) {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT `+globalContextColumns+` FROM global_context_pricing_rules ORDER BY platform,model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*GlobalContextRule, 0)
	for rows.Next() {
		r, e := scanGlobalContextRule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

var ErrGlobalContextConflict = errors.New("规则已被修改或已存在，请刷新后重试")

func (s *GlobalContextRuleStore) Save(ctx context.Context, r GlobalContextRule) (*GlobalContextRule, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if err := r.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	args := []any{r.Model, r.Platform, r.ThresholdTokens, r.InputMultiplier, r.OutputMultiplier, r.CacheReadMultiplier, r.CacheWriteMultiplier, r.Enabled}
	var row *sql.Row
	if r.ID == 0 {
		if r.Revision != 0 {
			return nil, ErrGlobalContextConflict
		}
		row = s.db.QueryRowContext(ctx, `INSERT INTO global_context_pricing_rules(model,platform,threshold_tokens,input_multiplier,output_multiplier,cache_read_multiplier,cache_write_multiplier,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(model) DO NOTHING RETURNING `+globalContextColumns, args...)
	} else {
		args = append(args, r.ID, r.Revision)
		row = s.db.QueryRowContext(ctx, `UPDATE global_context_pricing_rules SET platform=$2,threshold_tokens=$3,input_multiplier=$4,output_multiplier=$5,cache_read_multiplier=$6,cache_write_multiplier=$7,enabled=$8,revision=revision+1,updated_at=NOW() WHERE id=$9 AND revision=$10 AND model=$1 RETURNING `+globalContextColumns, args...)
	}
	out, err := scanGlobalContextRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGlobalContextConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && s.snapshot != nil {
		if out.Enabled {
			s.snapshot[out.Model] = *out
		} else {
			delete(s.snapshot, out.Model)
		}
	}
	return out, err
}
func (s *GlobalContextRuleStore) Delete(ctx context.Context, id, revision int64) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM global_context_pricing_rules WHERE id=$1 AND revision=$2`, id, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrGlobalContextConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for model, r := range s.snapshot {
		if r.ID == id {
			delete(s.snapshot, model)
		}
	}
	return nil
}

// Snapshot pins all catalog probe calls to the same rule revision.
type globalContextSnapshotKey struct{}
type globalContextSnapshot struct {
	model string
	rule  *GlobalContextRule
}

func (s *BillingService) globalContextRule(ctx context.Context, model string) (*GlobalContextRule, error) {
	if ctx != nil {
		if snap, ok := ctx.Value(globalContextSnapshotKey{}).(globalContextSnapshot); ok && snap.model == model {
			return snap.rule, nil
		}
	}
	if s.globalContextRules == nil {
		return nil, nil
	}
	r, err := s.globalContextRules.Find(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("global context rule unavailable: %w", err)
	}
	return r, nil
}
func (s *BillingService) GlobalContextRules() *GlobalContextRuleStore {
	store, _ := s.globalContextRules.(*GlobalContextRuleStore)
	return store
}
func applyGlobalContextRule(bd *CostBreakdown, r *GlobalContextRule, t UsageTokens, rate float64) {
	if r == nil || bd == nil {
		return
	}
	bd.ContextRuleID = r.ID
	bd.ContextRuleRevision = r.Revision
	cacheWrite := t.CacheCreationTokens
	if cacheWrite <= 0 {
		five, hour := normalizeCacheCreationBreakdown(t)
		cacheWrite = five + hour
	}
	if t.InputTokens+t.CacheReadTokens+cacheWrite <= r.ThresholdTokens {
		return
	}
	before := bd.ActualCost
	bd.InputCost *= r.InputMultiplier
	bd.ImageInputCost *= r.InputMultiplier
	bd.OutputCost *= r.OutputMultiplier
	bd.ImageOutputCost *= r.OutputMultiplier
	bd.CacheReadCost *= r.CacheReadMultiplier
	bd.CacheCreationCost *= r.CacheWriteMultiplier
	bd.TotalCost = bd.InputCost + bd.ImageInputCost + bd.OutputCost + bd.ImageOutputCost + bd.CacheReadCost + bd.CacheCreationCost
	bd.ActualCost = bd.TotalCost * math.Max(0, rate)
	bd.LongContextBillingApplied = bd.ActualCost > before
}

func (s *BillingService) globalContextRuleForCost(input CostInput) (*GlobalContextRule, error) {
	if input.SkipGlobalContextRules {
		return nil, nil
	}
	return s.globalContextRule(input.Ctx, input.Model)
}

// Configured model IDs only: never return credential values or contact upstreams.
func (s *GlobalContextRuleStore) ConfiguredModels(ctx context.Context, platform string) ([]string, error) {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT model FROM (
 SELECT mapping.key model FROM accounts a CROSS JOIN LATERAL jsonb_each_text(CASE WHEN jsonb_typeof(a.credentials->'model_mapping')='object' THEN a.credentials->'model_mapping' ELSE '{}'::jsonb END) mapping WHERE a.deleted_at IS NULL AND ($1='openai-compatible' OR a.platform=$1)
 UNION SELECT mapping.value FROM accounts a CROSS JOIN LATERAL jsonb_each_text(CASE WHEN jsonb_typeof(a.credentials->'model_mapping')='object' THEN a.credentials->'model_mapping' ELSE '{}'::jsonb END) mapping WHERE a.deleted_at IS NULL AND ($1='openai-compatible' OR a.platform=$1)
 UNION SELECT jsonb_array_elements_text(models) FROM channel_model_pricing WHERE ($1='openai-compatible' OR platform=$1) AND billing_mode='token'
 UNION SELECT jsonb_array_elements_text(CASE WHEN jsonb_typeof(card->'models')='array' THEN card->'models' ELSE '[]'::jsonb END) FROM groups g CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(g.model_pricing)='array' THEN g.model_pricing ELSE '[]'::jsonb END) card WHERE g.deleted_at IS NULL AND ($1='openai-compatible' OR g.platform=$1 OR card->>'platform'=$1) AND COALESCE(card->>'billing_mode','token')='token'
 ) names WHERE model<>'' AND model !~ '[*?[:space:]]' ORDER BY model LIMIT 5000`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var model string
		if err = rows.Scan(&model); err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, rows.Err()
}
