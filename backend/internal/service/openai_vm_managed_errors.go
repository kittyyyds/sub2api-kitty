package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// How long a managed account stays out of selection after one of its requests
// failed in a way the runtime will sort out by itself. The state sync (every
// minute) replaces this with what the slot actually reports.
const (
	openAIVMManagedAuthPause   = 2 * time.Minute
	openAIVMManagedBridgePause = 30 * time.Second
	// Reasons written by this file start with this prefix, so the state sync
	// can lift exactly its own pauses and leave every other subsystem's alone.
	openAIVMManagedPausePrefix = "vm_managed:"
)

// handleOpenAIVMManagedUpstreamError decides what one failed request means for
// a VM2-managed account. It never sets status "error" and never judges the
// credential: the runtime refreshes the token, re-logs in at most once a day,
// and marks the slot when a person is needed. The state sync copies that state
// here. All this does is keep the account out of selection for a moment so the
// same request is not sent into the same wall again.
func (s *RateLimitService) handleOpenAIVMManagedUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, _ []byte) bool {
	if s == nil || account == nil {
		return false
	}
	switch statusCode {
	case http.StatusUnauthorized:
		// 403 is deliberately absent: it is also how a content-policy refusal
		// arrives, and a client must not be able to bench accounts with a prompt.
		s.pauseOpenAIVMManaged(ctx, account, openAIVMManagedAuthPause, "auth")
		return true
	case http.StatusTooManyRequests:
		s.pauseOpenAIVMManaged(ctx, account, openAIVMManagedRetryAfter(headers, time.Minute), "rate_limited")
		return false
	case http.StatusConflict, http.StatusServiceUnavailable, 529:
		// The bridge says the binding is not ready, or the runtime says the
		// slot cannot take a request right now.
		s.pauseOpenAIVMManaged(ctx, account, openAIVMManagedRetryAfter(headers, openAIVMManagedBridgePause), "slot_unavailable")
		return true
	}
	return false
}

func openAIVMManagedRetryAfter(headers http.Header, fallback time.Duration) time.Duration {
	if headers != nil {
		if after, err := time.ParseDuration(headers.Get("Retry-After") + "s"); err == nil && after > 0 {
			if after > 10*time.Minute {
				after = 10 * time.Minute
			}
			return after
		}
	}
	return fallback
}

func (s *RateLimitService) pauseOpenAIVMManaged(ctx context.Context, account *Account, d time.Duration, why string) {
	until := time.Now().Add(d)
	s.notifyAccountSchedulingBlocked(account, until, "vm_managed_"+why)
	if s.accountRepo == nil {
		return
	}
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, openAIVMManagedPausePrefix+why); err != nil {
		slog.Warn("openai_vm_managed_pause_failed", "account_id", account.ID, "error", err)
	}
}

// pauseOpenAIVMManagedAccount is the gateway-side entry for failures that never
// produced an HTTP status (the bridge could not be reached).
func (s *OpenAIGatewayService) pauseOpenAIVMManagedAccount(ctx context.Context, account *Account, d time.Duration, why string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(d)
	s.BlockAccountScheduling(account, until, "vm_managed_"+why)
	if s.accountRepo == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, openAIVMManagedPausePrefix+why); err != nil {
		slog.Warn("openai_vm_managed_pause_failed", "account_id", account.ID, "error", err)
	}
}

// shouldFailoverOpenAIVMManagedResponse reports the answers that mean "this
// slot did not take the request": its login is gone (401), the bridge has the
// binding closed (409), or the runtime cannot schedule the slot (503). Nothing
// was sent to OpenAI in any of them, so another account may serve the request.
func shouldFailoverOpenAIVMManagedResponse(account *Account, statusCode int) bool {
	if !account.IsOpenAIVMManaged() {
		return false
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusConflict, http.StatusServiceUnavailable:
		return true
	}
	return false
}
