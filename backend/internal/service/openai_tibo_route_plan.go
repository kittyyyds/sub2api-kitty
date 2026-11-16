package service

import "time"

// HTTP is the only Tibo route left in the source fork (the Cookie WS hop and
// per-session route pins were retired there), so this port keeps only the
// scheduler tiering and the passive model check of a finished business
// response. Requests never wait for a probe.

// openAITiboSchedulerTier ranks an account for scheduling: 0 has a healthy
// route, 1 has no verdict or an unknown one (including accounts Tibo does
// not apply to), 2 is degraded. It never starts probes.
func (s *OpenAIGatewayService) openAITiboSchedulerTier(account *Account, now time.Time) int {
	if account == nil || !s.openAITiboRouteApplies(account) {
		return 1
	}
	switch s.openAITiboEffective(account.ID, openAITiboRouteHTTP, now) {
	case openAITiboHealthy:
		return 0
	case openAITiboDegraded:
		return 2
	default:
		return 1
	}
}

// observeOpenAITiboForwardResult runs the passive model check after a
// successful HTTP business response: a gpt-6-astra request answered by
// another model is hard evidence (verdict degraded, confirmation probe
// queued). The exact Tibo probe payload sent as a business request is the
// verdict of the probe itself and is not evidence. Verdict state lives in the
// process that probes, so gateway-only replicas do not record evidence.
func (s *OpenAIGatewayService) observeOpenAITiboForwardResult(account *Account, body []byte, result *OpenAIForwardResult) {
	if s == nil || result == nil || result.ClientDisconnect || result.OpenAIWSMode ||
		!s.openAITiboProbesRunHere() || !s.openAITiboRouteApplies(account) {
		return
	}
	if openAITiboIsProbePayloadRaw(body) {
		return
	}
	s.observeOpenAITiboResponseModel(account, openAITiboRouteHTTP, result.UpstreamModel, result.UpstreamResponseModel)
}
