package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// The source fork drove Tibo probing from its Codex ticket harvester loop
// (openAICodexTicketHarvestLoop, every 6s). Ticket harvesting and Cookie WS
// are dead code there, so only the Tibo tick is ported, as its own loop.
const defaultOpenAITiboTickInterval = 6 * time.Second

// openAITiboLoopWarmup delays the first tick after process start so the VM,
// IPv6 and plugin transports a probe shares with business requests are wired
// and reconciled before any probe can leave.
const openAITiboLoopWarmup = 30 * time.Second

func (s *OpenAIGatewayService) openAITiboTickInterval() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAITiboRoute.TickInterval > 0 {
		return s.cfg.Gateway.OpenAITiboRoute.TickInterval
	}
	return defaultOpenAITiboTickInterval
}

// ConfigureOpenAITiboProbeRuntime registers the probe loop on the node that
// owns background jobs (runtime.role=full and, during a managed handoff, the
// active lease owner). Gateway-only replicas never probe, so each account is
// probed by one process. Call it after the VM / IPv6 / plugin transports are
// attached to the gateway: a probe uses the same upstream chain as a request.
func ConfigureOpenAITiboProbeRuntime(s *OpenAIGatewayService, cfg *config.Config) {
	if s == nil || !cfg.RunsBackgroundJobs() {
		return
	}
	managedinstance.Register("openai-tibo-probe-loop", s.StartOpenAITiboProbeLoop, s.StopOpenAITiboProbeLoop)
}

// StartOpenAITiboProbeLoop starts background Tibo probing. It is a no-op while
// gateway.openai_tibo_route.enabled is false or on a gateway-only replica, so
// no synthetic probe is sent there.
func (s *OpenAIGatewayService) StartOpenAITiboProbeLoop() {
	if s == nil || s.accountRepo == nil || !s.openAITiboProbesRunHere() {
		return
	}
	s.openaiTiboLoopMu.Lock()
	defer s.openaiTiboLoopMu.Unlock()
	if s.openaiTiboLoopDone != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.openaiTiboLoopCancel = cancel
	s.openaiTiboLoopDone = done
	go func() {
		defer close(done)
		s.openAITiboProbeLoop(ctx)
	}()
	logger.L().Info("openai_tibo probe loop started")
}

// StopOpenAITiboProbeLoop stops the loop and waits for in-flight probes.
func (s *OpenAIGatewayService) StopOpenAITiboProbeLoop() {
	if s == nil {
		return
	}
	s.openaiTiboLoopMu.Lock()
	defer s.openaiTiboLoopMu.Unlock()
	cancel, done := s.openaiTiboLoopCancel, s.openaiTiboLoopDone
	s.openaiTiboLoopCancel, s.openaiTiboLoopDone = nil, nil
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.openaiTiboProbeWG.Wait()
}

func (s *OpenAIGatewayService) openAITiboProbeLoop(ctx context.Context) {
	timer := time.NewTimer(openAITiboLoopWarmup)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.probeOpenAITiboRoutes(ctx)
			timer.Reset(s.openAITiboTickInterval())
		}
	}
}

// IsOpenAITiboVerdictExtraKey reports the server-managed Tibo verdict keys.
func IsOpenAITiboVerdictExtraKey(key string) bool {
	return strings.HasPrefix(key, openAITiboVerdictExtraKeyPrefix)
}

// validOpenAIAstraSamples reads the persisted definitive probe observations of
// an astra verdict record: unknown, future, unstamped or foreign-model rows are
// dropped, then the last openAITiboRecentSampleLimit are kept oldest first.
func validOpenAIAstraSamples(raw any, now time.Time) []openAITiboRecentSample {
	model, _ := openAITiboExtraField(raw, "model").(string)
	if !isOpenAIGPT6AstraModel(model) {
		return nil
	}
	var rows []map[string]any
	switch samples := openAITiboExtraField(raw, "recent_samples").(type) {
	case []map[string]any:
		rows = samples
	case []any:
		for _, sample := range samples {
			if row, ok := sample.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
	default:
		encoded, err := json.Marshal(samples)
		if err != nil || json.Unmarshal(encoded, &rows) != nil {
			return nil
		}
	}
	result := make([]openAITiboRecentSample, 0, len(rows))
	for _, row := range rows {
		value, _ := row["verdict"].(string)
		if !openAITiboVerdict(value).definitive() {
			continue
		}
		stamp, _ := row["at"].(string)
		at, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil || at.IsZero() || at.After(now) {
			continue
		}
		result = append(result, openAITiboRecentSample{Verdict: openAITiboVerdict(value), At: at})
	}
	slices.SortStableFunc(result, func(a, b openAITiboRecentSample) int { return a.At.Compare(b.At) })
	if len(result) > openAITiboRecentSampleLimit {
		result = result[len(result)-openAITiboRecentSampleLimit:]
	}
	return result
}

// OpenAITiboRouteStatuses is the admin view of the account's Tibo routes
// (nil when Tibo does not apply). It never starts probes.
func (s *OpenAIGatewayService) OpenAITiboRouteStatuses(account *Account, now time.Time) []OpenAITiboRouteStatus {
	return s.openAITiboRouteStatuses(account, now)
}
