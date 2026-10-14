package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type managedPluginRepo struct {
	PluginRepository
	rows []*PluginInstallation
	err  error
}

func (r managedPluginRepo) List(context.Context) ([]*PluginInstallation, error) { return r.rows, r.err }

func TestManagedPluginStartupRejectsUnreviewedActivePlugins(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []*PluginInstallation
		err  error
		ok   bool
	}{
		{name: "empty", ok: true},
		{name: "disabled", rows: []*PluginInstallation{{State: "disabled"}}, ok: true},
		{name: "enabled", rows: []*PluginInstallation{{State: PluginStateEnabled}}},
		{name: "starting", rows: []*PluginInstallation{{State: PluginStateStarting}}},
		{name: "binding", rows: []*PluginInstallation{{State: "error", Bindings: []PluginBinding{{Enabled: true}}}}},
		{name: "read-failed", err: errors.New("read failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &PluginManager{repo: managedPluginRepo{rows: tc.rows, err: tc.err}}
			err := m.ValidateManagedStartup(context.Background())
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
