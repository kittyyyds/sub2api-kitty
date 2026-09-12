package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type profileEgressAccountRepo struct {
	AccountRepository
	account *Account
	writes  int
}

func (r *profileEgressAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *profileEgressAccountRepo) UpdateExtra(_ context.Context, _ int64, extra map[string]any) error {
	r.writes++
	if r.account.Extra == nil {
		r.account.Extra = map[string]any{}
	}
	for k, v := range extra {
		r.account.Extra[k] = v
	}
	return nil
}

type profileEgressProxyRepo struct {
	ProxyRepository
	proxy *Proxy
}

func (r *profileEgressProxyRepo) GetByID(context.Context, int64) (*Proxy, error) { return r.proxy, nil }

type profileEgressProber struct {
	urls    []string
	fail    bool
	onProbe func()
}

func (p *profileEgressProber) ProbeProxy(_ context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	p.urls = append(p.urls, proxyURL)
	if p.onProbe != nil {
		p.onProbe()
	}
	if p.fail {
		return nil, 0, errors.New("proxy http://secret:password@proxy.invalid failed")
	}
	return &ProxyExitInfo{IP: "203.0.113.1", Timezone: "America/New_York"}, 42, nil
}
func egressFixture() (*adminServiceImpl, *profileEgressAccountRepo, *profileEgressProxyRepo, *profileEgressProber) {
	id := int64(7)
	ar := &profileEgressAccountRepo{account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &id, Extra: map[string]any{"unrelated": "keep"}}}
	pr := &profileEgressProxyRepo{proxy: &Proxy{ID: 7, Name: "Residential", Protocol: "http", Host: "proxy.invalid", Port: 8080, Username: "secret", Password: "password", Status: StatusActive, UpdatedAt: time.Now()}}
	probe := &profileEgressProber{}
	return &adminServiceImpl{accountRepo: ar, proxyRepo: pr, proxyProber: probe}, ar, pr, probe
}
func TestClientProfileEgressUsesBoundProxyAndInvalidatesChangedBinding(t *testing.T) {
	s, ar, pr, p := egressFixture()
	got, err := s.ProbeOpenAIClientProfileEgress(context.Background(), 3, nil)
	require.NoError(t, err)
	require.Equal(t, []string{pr.proxy.URL()}, p.urls)
	require.Equal(t, "America/New_York", got.Timezone)
	require.False(t, got.TimezoneSent)
	require.Equal(t, "keep", ar.account.Extra["unrelated"])
	saved, err := s.GetOpenAIClientProfileEgress(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, got.IP, saved.IP)
	pr.proxy.Host = "changed.invalid"
	saved, err = s.GetOpenAIClientProfileEgress(context.Background(), 3)
	require.NoError(t, err)
	require.Nil(t, saved)
}
func TestClientProfileEgressFailureNeverFallsBackOrExposesProxyAuth(t *testing.T) {
	s, ar, _, p := egressFixture()
	p.fail = true
	_, err := s.ProbeOpenAIClientProfileEgress(context.Background(), 3, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "password")
	require.Len(t, p.urls, 1)
	require.NotEmpty(t, p.urls[0])
	require.Zero(t, ar.writes)
}
func TestClientProfileEgressRejectsWrongProxyAndMidProbeChange(t *testing.T) {
	s, ar, pr, p := egressFixture()
	wrong := int64(99)
	_, err := s.ProbeOpenAIClientProfileEgress(context.Background(), 3, &wrong)
	require.Error(t, err)
	require.Empty(t, p.urls)
	p.onProbe = func() { pr.proxy.Host = "changed.invalid" }
	_, err = s.ProbeOpenAIClientProfileEgress(context.Background(), 3, nil)
	require.Error(t, err)
	require.Zero(t, ar.writes)
}
func TestClientProfileEgressDirectAndMultipleProxySelection(t *testing.T) {
	s, ar, _, p := egressFixture()
	ar.account.ProxyID = nil
	got, err := s.ProbeOpenAIClientProfileEgress(context.Background(), 3, nil)
	require.NoError(t, err)
	require.Equal(t, "direct", got.Source)
	require.Equal(t, []string{""}, p.urls)
	ar.account.Extra["proxy_pool"] = []int64{7, 8}
	_, err = s.ProbeOpenAIClientProfileEgress(context.Background(), 3, nil)
	require.Error(t, err)
	require.Len(t, p.urls, 1)
}
