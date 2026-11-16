package service

// Stop shuts down the managed worker and releases any proxy leases. The former
// Start loop only queued Excel authorizations for Excel BPS and was removed.
func (s *OpenAIOAuthReauthService) Stop() {
	if s == nil {
		return
	}
	s.stopWorker()
	s.mihomoLeaseMu.Lock()
	leases := s.mihomoLeases
	s.mihomoLeases = make(map[int64]func())
	s.mihomoLeaseMu.Unlock()
	for _, release := range leases {
		release()
	}
}
