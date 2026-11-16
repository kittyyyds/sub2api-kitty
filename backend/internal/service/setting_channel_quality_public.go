package service

import "context"

// IsChannelQualityPublicEnabled only controls user-facing results and voting.
// Existing installations keep their display until an administrator disables it.
// A failed settings read must not accidentally expose a disabled result page.
func (s *SettingService) IsChannelQualityPublicEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyChannelQualityPublicEnabled})
	return err == nil && !isFalseSettingValue(values[SettingKeyChannelQualityPublicEnabled])
}
