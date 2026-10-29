// Task-scoped update of the already configured plugin. Never reads business
// accounts or changes bindings, prices, balances, authentication policy or scope.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"os"
	"time"
)

const pluginID = "local.kitty.oai-basispoints"
const packageSHA = "970af2672d269c832846cbe45102b1a312809c73ace46ba3e8ab259771db1dce"
const publisherID = "kitty-bps-sub-20261029-excel-v3"

func main() {
	apply := flag.Bool("apply", false, "install reviewed diagnostic package after operator disables old runtime")
	path := flag.String("package", "", "reviewed package path")
	flag.Parse()
	if err := run(*apply, *path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(apply bool, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.New("configuration unavailable")
	}
	if cfg.Plugins.AllowUnsigned || cfg.Plugins.TrustedPublishers[publisherID] == "" {
		return errors.New("reviewed publisher trust required; unsigned policy must stay disabled")
	}
	db, err := sql.Open("postgres", cfg.Database.DSNWithTimezone(cfg.Timezone)+" statement_timeout=10000 connect_timeout=5")
	if err != nil {
		return errors.New("database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	var stepUp bool
	if err = db.QueryRowContext(ctx, "SELECT COALESCE((SELECT value='true' FROM settings WHERE key='step_up_enabled'),false)").Scan(&stepUp); err != nil {
		return errors.New("verification policy unavailable")
	}
	if stepUp {
		return errors.New("verified admin session required; maintenance update refused")
	}
	repo := repository.NewPluginRepository(db)
	installed, err := repo.GetByKey(ctx, pluginID)
	if err != nil || installed == nil || installed.ID != 2 {
		return errors.New("expected plugin not found")
	}
	if installed.Version != "0.1.1" || installed.State != service.PluginStateDisabled {
		return errors.New("expected disabled version 0.1.1; refusing changed state")
	}
	for _, b := range installed.Bindings {
		if b.Enabled {
			return errors.New("plugin binding still enabled")
		}
	}
	encryptedBefore := installed.ConfigEncrypted
	if encryptedBefore == "" {
		return errors.New("configured plugin expected")
	}
	encryptor, err := repository.NewAESEncryptor(cfg)
	if err != nil {
		return errors.New("configuration encryption unavailable")
	}
	manager := service.NewPluginManager(repo, encryptor, cfg, service.PluginHostInfo{Version: "2.17.7-kitty.7", BuildType: "release"}, nil)
	defer manager.Stop()
	before, err := manager.GetConfig(ctx, installed.ID)
	if err != nil {
		return errors.New("plugin configuration unavailable")
	}
	var scope struct {
		AccountIDs []int64           `json:"account_ids"`
		Models     []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(before, &scope) != nil || len(scope.AccountIDs) != 1 || scope.AccountIDs[0] != 12637 || len(scope.Models) != 5 {
		return errors.New("scope changed; refusing update")
	}
	raw, err := os.ReadFile(path)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != packageSHA {
		return errors.New("reviewed package checksum mismatch")
	}
	if !apply {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": true, "plugin": pluginID, "config_preserved": true, "writes": false})
	}
	installed, err = manager.Install(ctx, bytes.NewReader(raw), installed.InstalledBy)
	if err != nil {
		return fmt.Errorf("signed update failed: %w", err)
	}
	if installed.ID != 2 || installed.Version != "0.1.2" || installed.State != service.PluginStateDisabled || installed.SignatureStatus != service.PluginSignatureTrusted || !installed.Compatibility.Compatible {
		return errors.New("unexpected installed state")
	}
	if installed.ConfigEncrypted != encryptedBefore {
		return errors.New("configuration changed unexpectedly; leave disabled")
	}
	after, err := manager.GetConfig(ctx, installed.ID)
	if err != nil || !bytes.Equal(before, after) {
		return errors.New("configuration verification failed; leave disabled")
	}
	test, err := manager.Test(ctx, installed.ID)
	if err != nil || test == nil || !test.Success {
		return errors.New("isolated self-test failed; leave disabled")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"updated": true, "id": installed.ID, "version": installed.Version, "state": installed.State, "signature": installed.SignatureStatus, "config_preserved": true, "account_scope": []int64{12637}, "models": 5, "business_accounts_read": 0, "upstream_calls": 0, "host_restart": false})
}
