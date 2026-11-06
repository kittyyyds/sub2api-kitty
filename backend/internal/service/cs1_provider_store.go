package service

import (
	"context"
	"encoding/json"
)

// CS1 providers use private, individually versioned settings. The repository
// saves their procurement wallet in the same transaction. No credentials are
// included in public settings or provider DTOs.
type CS1ProviderRepository interface {
	ReadCS1UserBalance(context.Context) (float64, error)
	ReadCS1Providers(context.Context) (json.RawMessage, error)
	SaveCS1Provider(context.Context, string, int64, json.RawMessage) (json.RawMessage, error)
	DeleteCS1Provider(context.Context, string, int64) error
}

func (s *ChannelService) CS1UserBalance(ctx context.Context) (float64, error) {
	r, e := s.cs1ProviderRepo()
	if e != nil {
		return 0, e
	}
	return r.ReadCS1UserBalance(ctx)
}

func (s *ChannelService) cs1ProviderRepo() (CS1ProviderRepository, error) {
	r, ok := s.repo.(CS1ProviderRepository)
	if !ok {
		return nil, ErrProviderHubUnavailable
	}
	return r, nil
}
func (s *ChannelService) LoadProviderRecords(ctx context.Context, out any) error {
	r, e := s.cs1ProviderRepo()
	if e != nil {
		return e
	}
	raw, e := r.ReadCS1Providers(ctx)
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}
func (s *ChannelService) SaveCS1Provider(ctx context.Context, id string, revision int64, record any, out any) error {
	r, e := s.cs1ProviderRepo()
	if e != nil {
		return e
	}
	raw, e := json.Marshal(record)
	if e != nil {
		return e
	}
	raw, e = r.SaveCS1Provider(ctx, id, revision, raw)
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}
func (s *ChannelService) DeleteCS1Provider(ctx context.Context, id string, revision int64) error {
	r, e := s.cs1ProviderRepo()
	if e != nil {
		return e
	}
	return r.DeleteCS1Provider(ctx, id, revision)
}
func (s *ChannelService) Encrypt(secret string) (string, error) {
	if s.providerProbe == nil || !s.providerProbe.newAPIFixedKey || s.providerProbe.newAPIEncryptor == nil {
		return "", ErrNewAPIEncryptionKey
	}
	return s.providerProbe.newAPIEncryptor.Encrypt(secret)
}
func (s *ChannelService) Decrypt(secret string) (string, error) {
	if s.providerProbe == nil || !s.providerProbe.newAPIFixedKey || s.providerProbe.newAPIEncryptor == nil {
		return "", ErrNewAPIEncryptionKey
	}
	return s.providerProbe.newAPIEncryptor.Decrypt(secret)
}

// The callback executes within the existing managed leader cycle, never from
// a detached goroutine or on a standby process.
func (s *UpstreamBillingProbeService) SetCS1ProviderRefresh(fn func(context.Context)) {
	s.cs1ProviderRefresh = fn
}
