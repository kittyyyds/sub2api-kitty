package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
)

func originalBPSFixture(t *testing.T) *PluginInstallation {
	t.Helper()
	var manifest PluginManifest
	require.NoError(t, json.Unmarshal(originalBPSManifest, &manifest))
	return &PluginInstallation{ID: 5, PluginKey: manifest.ID, Version: manifest.Version,
		Manifest: manifest, SignatureStatus: PluginSignatureTrusted, State: PluginStateEnabled,
		BinarySHA256: manifest.Files[manifest.Runtimes[manifest.RuntimeKey()].Path],
		Bindings:     []PluginBinding{{Enabled: true, Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, RolloutPercent: 100}}}
}

func TestOriginalBPSCompatibilityIsExact(t *testing.T) {
	row := originalBPSFixture(t)
	require.True(t, EvaluatePluginCompatibility(row.Manifest, PluginHostInfo{Version: "2.19.0-kitty.2"}).Tested)
	for _, version := range []string{"2.19.0-kitty.1", "2.19.0", "2.20.0-kitty.1"} {
		require.False(t, EvaluatePluginCompatibility(row.Manifest, PluginHostInfo{Version: version}).Compatible)
	}
	row.Manifest.Files[row.Manifest.Runtimes[row.Manifest.RuntimeKey()].Path] = "changed"
	require.False(t, EvaluatePluginCompatibility(row.Manifest, PluginHostInfo{Version: "2.19.0-kitty.2"}).Compatible)
}

func TestOriginalBPSManagedValidationRejectsTampering(t *testing.T) {
	for _, change := range []func(*PluginInstallation){
		func(r *PluginInstallation) { r.SignatureStatus = "unsigned" },
		func(r *PluginInstallation) { r.BinarySHA256 = "changed" },
		func(r *PluginInstallation) { r.Version = "0.6.13" },
		func(r *PluginInstallation) { r.Bindings[0].Platform = "anthropic" },
		func(r *PluginInstallation) { r.Manifest.Requires.TransportAPI = 2 },
	} {
		row := originalBPSFixture(t)
		require.True(t, isReviewedOriginalBPSManaged(row))
		m := &PluginManager{repo: managedPluginRepo{rows: []*PluginInstallation{row}}}
		require.NoError(t, m.ValidateManagedStartup(context.Background()))
		change(row)
		require.Error(t, m.ValidateManagedStartup(context.Background()))
	}
}

type originalBPSLease struct {
	mu    sync.Mutex
	owner string
	lost  bool
}

func (s *originalBPSLease) Acquire(_ context.Context, id string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner != "" {
		return false, nil
	}
	s.owner = id
	return true, nil
}
func (s *originalBPSLease) Renew(_ context.Context, id string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.lost && s.owner == id, nil
}
func (s *originalBPSLease) Release(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner == id {
		s.owner = ""
	}
	return nil
}

func originalBPSController(t *testing.T) (*managedinstance.Controller, *originalBPSLease) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "role")
	require.NoError(t, os.WriteFile(path, []byte("standby"), 0600))
	t.Setenv("KITTY_MANAGED_INSTANCE", "true")
	t.Setenv("KITTY_INSTANCE_ID", "original-bps-test")
	t.Setenv("KITTY_INSTANCE_ROLE_FILE", path)
	require.NoError(t, managedinstance.ConfigureFromEnv())
	t.Cleanup(func() {
		os.Setenv("KITTY_MANAGED_INSTANCE", "false")
		require.NoError(t, managedinstance.ConfigureFromEnv())
	})
	lease := &originalBPSLease{}
	managedinstance.ConfigureLeaseStore(lease)
	return managedinstance.Current(), lease
}

func TestOriginalBPSStandbyCannotStartOrResolveIdentity(t *testing.T) {
	c, _ := originalBPSController(t)
	row := originalBPSFixture(t)
	m := NewPluginManager(&pluginTokenRepository{installation: row}, pluginTokenEncryptor{}, testPluginConfig(t.TempDir(), false), PluginHostInfo{Version: "2.19.0-kitty.2"}, nil)
	require.False(t, m.managedPluginAllowed(row))
	_, err := m.newRuntime(context.Background(), row)
	require.ErrorContains(t, err, "租约")
	require.NoError(t, m.reconcileCapability(context.Background(), []*PluginInstallation{row}, PluginCapabilityOpenAIOAuthOutbound))
	require.Nil(t, m.route.Load().runtime)
	require.NotEmpty(t, m.route.Load().unavailable)
	host := newPluginHostServiceServer(row.PluginKey, nil, nil, PluginAccountScope{})
	_, err = host.ResolveOutboundIdentity(context.Background(), &pluginv1.ResolveOutboundIdentityRequest{AccountId: 42})
	require.ErrorContains(t, err, "租约")
	require.NoError(t, c.SetRole("active"))
	c.Step(context.Background())
	require.True(t, m.managedPluginAllowed(row))
	c.Drain()
	require.False(t, m.managedPluginAllowed(row))
	require.False(t, managedPluginHostAccess(row.PluginKey))
	require.True(t, managedPluginHostAccess("local.sub2api.gateway-group-prompt"))
}

func TestOriginalBPSRuntimeDrainDoesNotCutStream(t *testing.T) {
	r := &pluginRuntime{done: make(chan struct{})}
	require.True(t, r.beginRequest())
	ended := make(chan struct{})
	go func() { r.drainUntilComplete(); close(ended) }()
	require.Eventually(t, func() bool { return r.draining.Load() }, time.Second, time.Millisecond)
	require.False(t, r.beginRequest())
	select {
	case <-ended:
		t.Fatal("cut active response")
	default:
	}
	r.finishRequest()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("did not finish draining")
	}
}

func TestOriginalBPSLeaseLossCancelsRuntime(t *testing.T) {
	c, lease := originalBPSController(t)
	require.NoError(t, c.SetRole("active"))
	c.Step(context.Background())
	ctx, ok := c.LeadershipContext()
	require.True(t, ok)
	r := &pluginRuntime{done: make(chan struct{})}
	r.watchLeadership(ctx)
	lease.mu.Lock()
	lease.lost = true
	lease.mu.Unlock()
	c.Step(context.Background())
	select {
	case <-r.leadershipDone:
	case <-time.After(time.Second):
		t.Fatal("runtime survived lease loss")
	}
}
