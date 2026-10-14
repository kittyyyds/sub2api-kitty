// kitty-plugin-install is a local maintenance command for the reviewed prompt
// package. Run inside the application container as its normal user. It has no
// network listener, performs no migrations, and never prints runtime secrets.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
)

func main() {
	apply := flag.Bool("apply", false, "install and enable the fixed signed prompt package with empty rules")
	pkg := flag.String("package", "", "path to the original .s2plugin package")
	flag.Parse()
	if err := run(*apply, *pkg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(apply bool, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.New("configuration unavailable")
	}
	db, err := sql.Open("postgres", cfg.Database.DSNWithTimezone(cfg.Timezone)+" statement_timeout=10000 connect_timeout=5")
	if err != nil {
		return errors.New("database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var adminID int64
	if err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE role IN ($1,$2) AND status=$3 AND deleted_at IS NULL ORDER BY role DESC,id LIMIT 1`, service.RoleAdmin, service.RoleSuperAdmin, service.StatusActive).Scan(&adminID); err != nil {
		return errors.New("active administrator unavailable")
	}
	var stepUp bool
	if err = db.QueryRowContext(ctx, `SELECT COALESCE((SELECT value='true' FROM settings WHERE key='step_up_enabled'),false)`).Scan(&stepUp); err != nil {
		return errors.New("cannot determine verification policy")
	}
	if stepUp {
		return errors.New("two-factor verified admin web session required; maintenance install refused")
	}
	repo := repository.NewPluginRepository(db)
	encryptor, err := repository.NewAESEncryptor(cfg)
	if err != nil {
		return errors.New("configuration encryption unavailable")
	}
	manager := service.NewPluginManager(repo, encryptor, cfg, service.PluginHostInfo{Version: "2.9.9-kitty.3", BuildType: "release"}, nil)
	const key = "local.sub2api.gateway-group-prompt"
	installed, err := repo.GetByKey(ctx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.New("cannot read plugin state")
	}
	if !apply {
		if installed != nil {
			if err := manager.ValidateManagedStartup(ctx); err != nil {
				return errors.New("managed restart validation failed")
			}
			saved, err := manager.GetConfig(ctx, installed.ID)
			if err != nil {
				return errors.New("stored plugin configuration unavailable")
			}
			var cfg struct {
				Rules    []json.RawMessage          `json:"rules"`
				Profiles map[string]json.RawMessage `json:"profiles"`
			}
			if json.Unmarshal(saved, &cfg) != nil {
				return errors.New("invalid stored configuration")
			}
			token, _, err := manager.CreateUIAssetToken(ctx, installed.ID, time.Minute)
			if err != nil {
				return errors.New("plugin UI verification failed")
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/api/v1/plugin-ui/"+token+"/index.html", nil)
			if err != nil {
				return errors.New("plugin UI request unavailable")
			}
			client := &http.Client{Timeout: 10 * time.Second}
			res, err := client.Do(req)
			if err != nil {
				return errors.New("plugin UI request failed")
			}
			defer res.Body.Close()
			body, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
			if err != nil || res.StatusCode != 200 || !strings.Contains(string(body), "提示词规则") {
				return errors.New("plugin configuration page unavailable")
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": true, "plugin_id": installed.ID, "state": installed.State, "rules_empty": len(cfg.Rules) == 0 && len(cfg.Profiles) == 0, "ui_status": res.StatusCode, "managed_restart_valid": true, "changes": false})
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": true, "already_installed": installed != nil, "changes": false})
	}
	if path == "" {
		return errors.New("original package path required")
	}
	raw, e := os.ReadFile(path)
	if e != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != "26b4e22bdf0279bfa7475930fe54423c54c3fca7e9003540a838341e56a9a80a" {
		return errors.New("original reviewed package hash required")
	}
	if installed == nil {
		f, e := os.Open(path)
		if e != nil {
			return errors.New("package file unavailable")
		}
		defer f.Close()
		installed, e = manager.Install(ctx, f, &adminID)
		if e != nil {
			return fmt.Errorf("package install failed: %w", e)
		}
		if _, e = manager.SaveConfig(ctx, installed.ID, json.RawMessage(`{"rules":[]}`)); e != nil {
			return fmt.Errorf("empty configuration rejected: %w", e)
		}
	} else {
		// Repeated invocation must never erase an administrator's rules.
		current, e := manager.GetConfig(ctx, installed.ID)
		if e != nil {
			return errors.New("existing configuration unavailable")
		}
		var value struct {
			Rules    []json.RawMessage          `json:"rules"`
			Profiles map[string]json.RawMessage `json:"profiles"`
		}
		if json.Unmarshal(current, &value) != nil || len(value.Rules) > 0 || len(value.Profiles) > 0 {
			return errors.New("existing rules found; refusing to overwrite or enable")
		}
	}
	defer manager.Stop()
	result, err := manager.Enable(ctx, installed.ID, false, 100)
	if err != nil {
		return fmt.Errorf("plugin enable failed: %w", err)
	}
	test, err := manager.Test(ctx, installed.ID)
	if err != nil || !test.Success {
		return errors.New("plugin configuration test failed")
	}
	body := []byte(`{"model":"maintenance-no-upstream-call","input":"noop"}`)
	out, err := manager.TransformGatewayRequest(ctx, 1, 1, "responses", "maintenance-no-upstream-call", body)
	if err != nil || string(out) != string(body) {
		return errors.New("empty configuration changed a synthetic local request")
	}
	// Only reveal the installed plugin's administration menu; preserve every
	// other system setting. This client does not run schema initialization.
	settings := repository.NewSettingRepository(ent.NewClient(ent.Driver(entsql.OpenDB("postgres", db))))
	if err := settings.Set(ctx, service.SettingKeyPluginManagementEnabled, "true"); err != nil {
		return errors.New("plugin enabled but menu could not be displayed")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"plugin_id": result.ID, "version": result.Version, "state": result.State, "signature_status": result.SignatureStatus, "compatible": result.Compatibility.Compatible, "healthy": result.RuntimeHealthy, "rules_empty": true, "noop_verified": true, "upstream_calls": 0})
}
