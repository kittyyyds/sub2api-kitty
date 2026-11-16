package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Monitor probes have no plan manager, quality scoring, isolation, showcase,
// or user balance effects. Their sole owners are channel monitor V2 and V3.
type MonitorProbeConfig struct {
	QuestionKind, Prompt, ReasoningEffort, ModelID string
	ParallelCount                                  int
}
type MonitorProbePlan struct {
	GroupID int64
	ModelID string
	Config  *MonitorProbeConfig
}
type MonitorProbeResult struct {
	Status, ResponseText, ErrorMessage string
	AccountID                          int64
	CostUSD                            *float64
}
type monitorGroupProvider interface {
	GetByID(context.Context, int64) (*Group, error)
}
type monitorGroupRouter interface {
	route(context.Context, *Group, string, map[int64]struct{}) (*monitorGroupRoute, error)
}
type MonitorGroupProbeService struct {
	groups     monitorGroupProvider
	router     monitorGroupRouter
	accounts   *AccountTestService
	runAccount func(context.Context, int64, string, *MonitorProbeConfig) (*ScheduledTestResult, error)
}

var errMonitorGroupAccountBusy = errors.New("monitor account concurrency unavailable")

func NewMonitorGroupProbeService(groups GroupRepository, gateway *GatewayService, openai *OpenAIGatewayService, concurrency *ConcurrencyService, accounts *AccountTestService) *MonitorGroupProbeService {
	return &MonitorGroupProbeService{groups: groups, router: &gatewayMonitorGroupRouter{gateway: gateway, openai: openai, concurrency: concurrency, slotWait: time.Minute}, accounts: accounts}
}
func (s *MonitorGroupProbeService) runSample(ctx context.Context, plan *MonitorProbePlan, group *Group) *MonitorProbeResult {
	// An unusable credential must not pin every scheduled probe to the same
	// account. Only failures before any answer may move to another account;
	// a wrong or partial answer is final and must never be cherry-picked away.
	excluded := map[int64]struct{}{}
	var result *MonitorProbeResult
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			if result == nil {
				result = &MonitorProbeResult{Status: "failed", ErrorMessage: ctx.Err().Error()}
			}
			return result
		}
		route, err := s.router.route(ctx, group, plan.ModelID, excluded)
		if err != nil {
			if result == nil {
				result = &MonitorProbeResult{Status: "failed", ErrorMessage: "no_available_account: " + err.Error()}
			}
			return result
		}
		if _, repeated := excluded[route.account.ID]; repeated {
			route.release()
			return result
		}
		result = s.runSelectedAccount(ctx, plan, route)
		if !monitorProbeCanFailover(result) {
			return result
		}
		excluded[route.account.ID] = struct{}{}
	}
	return result
}

func (s *MonitorGroupProbeService) runSelectedAccount(ctx context.Context, plan *MonitorProbePlan, route *monitorGroupRoute) *MonitorProbeResult {
	defer route.release()
	result := &MonitorProbeResult{Status: "failed", AccountID: route.account.ID}
	if s.runAccount != nil {
		row, err := s.runAccount(ctx, route.account.ID, route.model, plan.Config)
		if row != nil {
			result.Status = row.Status
			result.ResponseText = row.ResponseText
			result.ErrorMessage = row.ErrorMessage
		}
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = err.Error()
		}
		return result
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/internal/channel-monitor/probe", nil)
	c.Set(accountTestBackgroundKey, true)
	prompt := ""
	if plan.Config != nil {
		prompt = plan.Config.Prompt
		c.Request = c.Request.WithContext(withAccountTestReasoningEffort(ctx, plan.Config.ReasoningEffort))
	}
	err := s.accounts.TestAccountConnection(c, route.account.ID, route.model, prompt, AccountTestModeDefault)
	result.ResponseText, result.ErrorMessage = parseTestSSEOutput(recorder.Body.String())
	if err != nil && result.ErrorMessage == "" {
		result.ErrorMessage = err.Error()
	}
	if err == nil && result.ErrorMessage == "" {
		result.Status = "success"
	}
	// The official connection tester has no authoritative provider cost receipt.
	// Leave cost unknown rather than inventing zero or touching customer ledgers.
	return result
}

// Return only stable categories to monitoring clients. Raw provider errors
// remain in the existing redacted account-test log, never in the public page.
func monitorProbeErrorCategory(message string) string {
	text := strings.ToLower(message)
	if channelMonitorV2ContainsAny(text, "insufficient account balance", "insufficient_balance", "insufficient_quota", "credit balance is too low") {
		return "quota_or_balance"
	}
	if strings.Contains(text, "upstream_not_ready") {
		return "account_pool_unavailable"
	}
	category := channelMonitorV3ProbeCategory(message)
	if category == "other" {
		return "probe_failed"
	}
	return category
}

func monitorProbeCanFailover(result *MonitorProbeResult) bool {
	if result == nil || result.Status == "success" || strings.TrimSpace(result.ResponseText) != "" || strings.HasPrefix(result.ErrorMessage, "answer_mismatch:") {
		return false
	}
	switch monitorProbeErrorCategory(result.ErrorMessage) {
	case "quota_or_balance", "authentication", "upstream_forbidden", "rate_or_capacity", "upstream_5xx", "transport_or_stream", "timeout", "account_pool_unavailable", "model_unsupported", "not_found":
		return true
	}
	return false
}
func normalizeMonitorReasoningEffort(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return s
	}
	return ""
}

type gatewayMonitorGroupRouter struct {
	gateway     *GatewayService
	openai      *OpenAIGatewayService
	concurrency *ConcurrencyService
	slotWait    time.Duration
}

type monitorGroupRoute struct {
	account *Account
	model   string
	release func()
}

func (r *gatewayMonitorGroupRouter) route(ctx context.Context, group *Group, model string, excluded map[int64]struct{}) (*monitorGroupRoute, error) {
	platform, upstreamModel := group.Platform, model
	if platform == PlatformComposite {
		decision, matched, err := r.gateway.resolveCompositeRouteDecision(ctx, group, model, CompositeRouteEndpointAny)
		if err != nil {
			return nil, err
		}
		if !matched {
			return nil, fmt.Errorf("%w supporting model: %s (composite target platform unknown)", ErrNoAvailableAccounts, model)
		}
		ctx = WithCompositeRouteDecision(ctx, decision)
		platform = decision.TargetPlatform
		if rewritten := strings.TrimSpace(decision.UpstreamModel); rewritten != "" {
			upstreamModel = rewritten
		}
	}
	var selection *AccountSelectionResult
	var err error
	if isOpenAIResponsesGatewayPlatform(platform) {
		selection, _, err = r.openai.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", upstreamModel, excluded,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, NormalizeOpenAICompatiblePlatform(platform))
	} else {
		selection, err = r.gateway.SelectAccountWithLoadAwareness(ctx, &group.ID, "", upstreamModel, excluded, "", 0)
	}
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil {
		return nil, ErrNoAvailableAccounts
	}
	release, err := r.holdSlot(ctx, selection)
	if err != nil {
		return nil, err
	}
	return &monitorGroupRoute{account: selection.Account, model: upstreamModel, release: release}, nil
}

func (r *gatewayMonitorGroupRouter) holdSlot(ctx context.Context, selection *AccountSelectionResult) (func(), error) {
	if selection.Acquired || selection.WaitPlan == nil || r.concurrency == nil {
		return releaseOnce(selection.ReleaseFunc), nil
	}
	plan := selection.WaitPlan
	wait := r.slotWait
	if plan.Timeout > 0 && plan.Timeout < wait {
		wait = plan.Timeout
	}
	deadline := time.Now().Add(wait)
	for {
		acquired, err := r.concurrency.AcquireAccountSlot(ctx, plan.AccountID, plan.MaxConcurrency)
		if err == nil && acquired != nil && acquired.Acquired {
			return releaseOnce(acquired.ReleaseFunc), nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errMonitorGroupAccountBusy
		}
		if remaining > 500*time.Millisecond {
			remaining = 500 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(remaining):
		}
	}
}

func releaseOnce(release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(release) }
}

func isOpenAIResponsesGatewayPlatform(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo:
		return true
	default:
		return false
	}
}
