package kittyrisk

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var errKittyRiskDisabled = errors.New("Kitty 加强审核或证据联动已关闭，请管理员手动开启")

func (r *contentModerationArchiveRuntime) enabled(ctx context.Context) bool {
	return r != nil && r.options.Enabled != nil && r.options.Enabled(ctx)
}

func (s *ContentModerationService) applyReleaseGates(cfg *ContentModerationConfig) {
	if cfg == nil {
		return
	}
	if s == nil || s.releaseID == "" || cfg.ReviewRelease != s.releaseID {
		cfg.Enabled = false
	}
	if s == nil || s.releaseID == "" || cfg.EvidenceRelease != s.releaseID {
		cfg.EvidenceEnabled = false
	}
}

func (s *ContentModerationService) reviewEnabled(ctx context.Context) bool {
	return s.releaseGates(ctx).review
}

func (s *ContentModerationService) evidenceEnabled(ctx context.Context) bool {
	return s.releaseGates(ctx).evidence
}

type releaseGateSnapshot struct {
	release          string
	review, evidence bool
	expires          time.Time
}

// The disabled gateway path reads no provider secrets and normally performs
// no database query. Local admin saves publish immediately; other processes
// and out-of-band setting edits are observed within one second.
func (s *ContentModerationService) releaseGates(ctx context.Context) releaseGateSnapshot {
	if s == nil || s.settingRepo == nil || s.releaseID == "" {
		return releaseGateSnapshot{}
	}
	now := time.Now()
	if cached := s.gates.Load(); cached != nil && cached.release == s.releaseID && now.Before(cached.expires) {
		return *cached
	}
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	if cached := s.gates.Load(); cached != nil && cached.release == s.releaseID && now.Before(cached.expires) {
		return *cached
	}
	// Parse only switches and release stamps. A missing decryption key must
	// never cause disabled requests to parse/hydrate the reviewer credentials.
	var flags struct {
		Enabled         bool   `json:"enabled"`
		EvidenceEnabled bool   `json:"evidence_enabled"`
		ReviewRelease   string `json:"review_release"`
		EvidenceRelease string `json:"evidence_release"`
		Mode            string `json:"mode"`
	}
	snapshot := releaseGateSnapshot{release: s.releaseID, expires: now.Add(time.Second)}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyContentModerationConfig)
	if err == nil && json.Unmarshal([]byte(raw), &flags) == nil {
		snapshot.review = flags.Enabled && flags.ReviewRelease == s.releaseID && flags.Mode != ContentModerationModeOff
		snapshot.evidence = flags.EvidenceEnabled && flags.EvidenceRelease == s.releaseID
	}
	s.gates.Store(&snapshot)
	return snapshot
}

func (s *ContentModerationService) publishReleaseGates(cfg *ContentModerationConfig) {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	snapshot := &releaseGateSnapshot{release: s.releaseID, expires: time.Now().Add(time.Second)}
	if cfg != nil {
		snapshot.review = cfg.Enabled && cfg.ReviewRelease == s.releaseID && cfg.Mode != ContentModerationModeOff
		snapshot.evidence = cfg.EvidenceEnabled && cfg.EvidenceRelease == s.releaseID
	}
	s.gates.Store(snapshot)
}

func (s *ContentModerationService) EvidenceEnabled(ctx context.Context) bool {
	return s.evidenceEnabled(ctx)
}
func (s *ContentModerationService) ReviewEnabled(ctx context.Context) bool {
	return s.reviewEnabled(ctx)
}

// Check the current setting at the last network boundary, including health,
// manual provider tests, concurrent reviewers, and background shadow work.
type gatedReviewTransport struct {
	service *ContentModerationService
	base    http.RoundTripper
}

func (t gatedReviewTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.service.reviewEnabled(req.Context()) {
		return nil, errKittyRiskDisabled
	}
	return t.base.RoundTrip(req)
}

// NewManagedService uses independent settings, tables, Redis namespaces and
// archive directories; it never migrates or disables the site's legacy audit.
func NewManagedService(db *sql.DB, redisClient *redis.Client, settings SettingRepository, groups GroupRepository,
	users UserRepository, proxies ProxyRepository, invalidator APIKeyAuthCacheInvalidator,
	email *EmailService, keys APIKeyRepository, releaseID, dataDir string) (*ContentModerationService, error) {
	repo := NewContentModerationRepository(db)
	s := NewContentModerationService(settings, repo, NewContentModerationHashCache(redisClient), groups, users, proxies, invalidator, email)
	s.releaseID = strings.TrimSpace(releaseID)
	s.apiKeyRepo = keys
	if s.releaseID == "" {
		return nil, errors.New("Kitty risk release identity is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Persist the disabled state for a new release. Only explicit PUT flags
	// stamp that release as accepted; ordinary restarts keep a manual choice.
	raw, err := settings.GetValue(ctx, SettingKeyContentModerationConfig)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	var saved map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &saved) != nil || saved == nil {
		saved = map[string]json.RawMessage{}
	}
	for _, item := range []struct{ flag, release string }{{"enabled", "review_release"}, {"evidence_enabled", "evidence_release"}} {
		var accepted string
		_ = json.Unmarshal(saved[item.release], &accepted)
		if accepted != s.releaseID {
			saved[item.flag] = json.RawMessage("false")
			saved[item.release] = json.RawMessage(`""`)
		}
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	if string(encoded) != raw {
		if err = settings.Set(ctx, SettingKeyContentModerationConfig, string(encoded)); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	dir := filepath.Join(dataDir, "kitty-risk")
	keyPath := filepath.Join(dir, "keyring.json")
	s.ConfigureContentModerationCredentialKeyRing(keyPath)
	runtime, err := newContentModerationArchiveRuntime(repo.(ContentModerationArchiveRepository), ContentModerationArchiveRuntimeOptions{
		KeyRingPath: keyPath, RetryDir: filepath.Join(dir, "retry"), EmergencyDir: filepath.Join(dir, "emergency"),
		ChunkBytes: 256 * 1024, DiskMinFreeBytes: 64 * 1024 * 1024, RetryInitial: 5 * time.Second, RetryMax: 5 * time.Minute,
		Enabled: s.evidenceEnabled,
	})
	if err != nil {
		return nil, err
	}
	s.archiveRuntime = runtime
	// Generate a local key only on first installation; never replace a key
	// needed by previously encrypted provider settings or archived requests.
	if _, statErr := os.Stat(keyPath); errors.Is(statErr, os.ErrNotExist) && !strings.Contains(raw, "ciphertext") {
		var protected bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM kitty_risk_logs WHERE archive_key_id <> '')").Scan(&protected); err != nil {
			runtime.Close()
			return nil, err
		}
		if !protected {
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				runtime.Close()
				return nil, err
			}
			ring := ContentModerationArchiveKeyRing{CurrentKeyID: "kitty-risk-v1", Keys: map[string]string{"kitty-risk-v1": base64.StdEncoding.EncodeToString(key)}}
			if err := writePrivateModerationJSONAtomic(keyPath, ring); err != nil {
				runtime.Close()
				return nil, err
			}
			runtime.degraded.Store(false)
		}
	}
	runtime.SetDispositionProcessor(s.retryCyberPolicyDisposition)
	return s, nil
}
