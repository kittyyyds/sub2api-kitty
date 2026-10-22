package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrUpdateRestartRequired     = infraerrors.Conflict("UPDATE_RESTART_REQUIRED", "restart the service before another update or rollback")
	ErrNoUpdateAvailable         = infraerrors.Conflict("ALREADY_UP_TO_DATE", "no update available; current version is latest")
	ErrRollbackVersionNotAllowed = infraerrors.BadRequest("ROLLBACK_VERSION_NOT_ALLOWED", "version is not in the allowed rollback list")
)

const (
	updateCacheKey = "update_check_cache"
	updateCacheTTL = 1200 // 20 minutes
	// Security: allowed download domains for updates
	allowedDownloadHost = "github.com"
	allowedAssetHost    = "objects.githubusercontent.com"

	// Security: max download size (500MB)
	maxDownloadSize = 500 * 1024 * 1024

	// Rollback: expose at most the 3 most recent versions older than current
	maxRollbackVersions = 3
	// Fetch a few extra releases so filtering (current/newer/prerelease) still leaves enough candidates
	rollbackFetchPageSize = 15
)

// UpdateCache defines cache operations for update service
type UpdateCache interface {
	GetUpdateInfo(ctx context.Context) (string, error)
	SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error
}

// GitHubReleaseClient 获取 GitHub release 信息的接口
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
	DownloadFile(ctx context.Context, url, dest string, maxSize int64) error
	FetchChecksumFile(ctx context.Context, url string) ([]byte, error)
}

// UpdateService handles software updates
type UpdateService struct {
	mutationMu      sync.Mutex
	restartRequired bool
	cache           UpdateCache
	githubClient    GitHubReleaseClient
	currentVersion  string
	buildType       string // "source" for manual builds, "release" for CI builds
	executablePath  func() (string, error)
}

// NewUpdateService creates a new UpdateService
func NewUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, version, buildType string) *UpdateService {
	return &UpdateService{
		cache:          cache,
		githubClient:   githubClient,
		currentVersion: version,
		buildType:      buildType,
		executablePath: os.Executable,
	}
}

// UpdateInfo contains update information
type UpdateInfo struct {
	CurrentVersion string       `json:"current_version"`
	LatestVersion  string       `json:"latest_version"`
	HasUpdate      bool         `json:"has_update"`
	ReleaseInfo    *ReleaseInfo `json:"release_info,omitempty"`
	Cached         bool         `json:"cached"`
	Warning        string       `json:"warning,omitempty"`
	BuildType      string       `json:"build_type"` // "source" or "release"
}

// ReleaseInfo contains GitHub release details
type ReleaseInfo struct {
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets,omitempty"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
}

// GitHubRelease represents GitHub API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// RollbackVersion describes a release version the system can roll back to
type RollbackVersion struct {
	Version     string `json:"version"` // without "v" prefix, e.g. "0.1.146"
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckUpdate checks for available updates
func (s *UpdateService) CheckUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	if _, err := releaseRepository(); err != nil {
		return nil, err
	}
	// Try cache first
	if !force {
		if cached, err := s.getFromCache(ctx); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch from GitHub
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		// Return cached on error
		if cached, cacheErr := s.getFromCache(ctx); cacheErr == nil && cached != nil {
			cached.Warning = "Using cached data: " + err.Error()
			return cached, nil
		}
		return &UpdateInfo{
			CurrentVersion: s.currentVersion,
			LatestVersion:  s.currentVersion,
			HasUpdate:      false,
			Warning:        err.Error(),
			BuildType:      s.buildType,
		}, nil
	}

	// Cache result
	s.saveToCache(ctx, info)
	return info, nil
}

// PerformUpdate downloads and applies the update
// Uses atomic file replacement pattern for safe in-place updates
func (s *UpdateService) PerformUpdate(ctx context.Context) error {
	if err := s.checkRestartRequired(); err != nil {
		return err
	}

	// Mutation requires fresh release metadata; cache fallback is for display.
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		return err
	}

	if !info.HasUpdate {
		return ErrNoUpdateAvailable
	}

	if info.ReleaseInfo == nil {
		return fmt.Errorf("release metadata unavailable")
	}
	return s.applyReleaseAssets(ctx, info.ReleaseInfo.Assets, info.LatestVersion)
}

// applyReleaseAssets downloads the platform archive from the given release assets,
// verifies its checksum, and atomically swaps the running binary.
// Shared by PerformUpdate (latest) and RollbackToVersion (specific older version).
func (s *UpdateService) applyReleaseAssets(ctx context.Context, releaseAssets []Asset, version string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.restartRequired {
		return ErrUpdateRestartRequired
	}
	contract, err := s.compatibleRelease(ctx, releaseAssets, version)
	if err != nil {
		return err
	}
	archive, checksums, err := s.compatiblePlatformAssets(releaseAssets, version, contract)
	if err != nil {
		return err
	}

	// Get current executable path
	exePath, err := s.executablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	if err := s.applyCompatibleArchiveAt(ctx, archive, checksums, contract, exePath); err != nil {
		return err
	}
	s.restartRequired = true
	return nil
}

func (s *UpdateService) applyCompatibleArchiveAt(ctx context.Context, archive, checksums Asset, contract *compatiblePackage, exePath string) error {
	exeDir := filepath.Dir(exePath)

	// Create temp directory in the SAME directory as executable
	// This ensures os.Rename is atomic (same filesystem)
	tempDir, err := os.MkdirTemp(exeDir, ".sub2api-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Download archive
	archivePath := filepath.Join(tempDir, archive.Name)
	if err := s.downloadFile(ctx, archive.DownloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := checkPackageFile(archivePath, contract.Artifacts[archive.Name]); err != nil {
		return err
	}

	if info, err := os.Stat(archivePath); err != nil || info.Size() != archive.Size {
		return fmt.Errorf("downloaded archive size mismatch")
	}
	if err := s.verifyChecksum(ctx, archivePath, checksums.DownloadURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Extract binary from archive
	newBinaryPath := filepath.Join(tempDir, "sub2api")
	if err := s.extractBinary(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Set executable permission before replacement
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return replaceCompatibleBinary(exePath, newBinaryPath, s.currentVersion, os.Rename)
}

// Rollback restores the previous version
func (s *UpdateService) Rollback() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.restartRequired {
		return ErrUpdateRestartRequired
	}
	exePath, err := s.executablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	if err := rollbackCompatibleBinaryAt(exePath); err != nil {
		return err
	}
	s.restartRequired = true
	return nil
}

func rollbackCompatibleBinaryAt(exePath string) error {
	backupFile := exePath + ".backup"
	if err := validateBackupContract(backupFile); err != nil {
		return err
	}
	// Copy first: a failed replacement leaves both the live binary and the
	// validated backup intact, and a successful rollback keeps its evidence.
	dir, err := os.MkdirTemp(filepath.Dir(exePath), ".sub2api-rollback-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	candidate := filepath.Join(dir, "sub2api")
	if err := copyUpdateBinary(backupFile, candidate); err != nil {
		return err
	}
	if err := os.Rename(candidate, exePath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	return nil
}

// ListRollbackVersions returns up to maxRollbackVersions release versions that are
// strictly older than the current version (the current version itself is excluded),
// newest first. Draft and prerelease entries are skipped.
func (s *UpdateService) ListRollbackVersions(ctx context.Context) ([]RollbackVersion, error) {
	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return nil, err
	}

	versions := make([]RollbackVersion, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, RollbackVersion{
			Version:     strings.TrimPrefix(r.TagName, "v"),
			PublishedAt: r.PublishedAt,
			HTMLURL:     r.HTMLURL,
		})
	}
	return versions, nil
}

// RollbackToVersion downloads and installs a specific older version.
// The target must be one of the versions returned by ListRollbackVersions;
// anything else (including the current version) is rejected.
func (s *UpdateService) RollbackToVersion(ctx context.Context, version string) error {
	if err := s.checkRestartRequired(); err != nil {
		return err
	}

	target := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if target == "" {
		return ErrRollbackVersionNotAllowed
	}

	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return err
	}

	var match *GitHubRelease
	for _, r := range releases {
		if strings.TrimPrefix(r.TagName, "v") == target {
			match = r
			break
		}
	}
	if match == nil {
		return ErrRollbackVersionNotAllowed
	}

	assets := make([]Asset, len(match.Assets))
	for i, a := range match.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	return s.applyReleaseAssets(ctx, assets, target)
}

// fetchRollbackCandidates fetches recent releases and keeps the newest
// maxRollbackVersions entries strictly older than the current version.
func (s *UpdateService) fetchRollbackCandidates(ctx context.Context) ([]*GitHubRelease, error) {
	repository, err := releaseRepository()
	if err != nil {
		return nil, err
	}
	releases, err := s.githubClient.FetchRecentReleases(ctx, repository, rollbackFetchPageSize)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(releases))
	candidates := make([]*GitHubRelease, 0, maxRollbackVersions)
	for _, r := range releases {
		if r == nil || r.Draft || r.Prerelease {
			continue
		}
		v := strings.TrimPrefix(r.TagName, "v")
		if v == "" || seen[v] {
			continue
		}
		// Only versions strictly older than current (also excludes current itself)
		if compareVersions(v, s.currentVersion) >= 0 {
			continue
		}
		assets := packageAssets(r)
		contract, err := s.compatibleRelease(ctx, assets, v)
		if err != nil {
			continue
		}
		if _, _, err := s.compatiblePlatformAssets(assets, v, contract); err != nil {
			continue
		}
		seen[v] = true
		candidates = append(candidates, r)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(
			strings.TrimPrefix(candidates[i].TagName, "v"),
			strings.TrimPrefix(candidates[j].TagName, "v"),
		) > 0
	})

	if len(candidates) > maxRollbackVersions {
		candidates = candidates[:maxRollbackVersions]
	}
	return candidates, nil
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*UpdateInfo, error) {
	repository, err := releaseRepository()
	if err != nil {
		return nil, err
	}
	release, err := s.githubClient.FetchLatestRelease(ctx, repository)
	if err != nil {
		return nil, err
	}
	if release == nil || release.Draft || release.Prerelease {
		return nil, fmt.Errorf("no published compatible release metadata")
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	assets := make([]Asset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			Size:        a.Size,
		}
	}

	info := &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      compareVersions(s.currentVersion, latestVersion) < 0,
		ReleaseInfo: &ReleaseInfo{
			Name:        release.Name,
			Body:        release.Body,
			PublishedAt: release.PublishedAt,
			HTMLURL:     release.HTMLURL,
			Assets:      assets,
		},
		Cached:    false,
		BuildType: s.buildType,
	}
	if info.HasUpdate {
		contract, err := s.compatibleRelease(ctx, assets, latestVersion)
		if err == nil {
			_, _, err = s.compatiblePlatformAssets(assets, latestVersion, contract)
		}
		if err != nil {
			info.HasUpdate = false
			info.Warning = "新版未通过本站兼容校验，保留当前版本：" + err.Error()
		}
	}
	return info, nil
}

func (s *UpdateService) downloadFile(ctx context.Context, downloadURL, dest string) error {
	return s.githubClient.DownloadFile(ctx, downloadURL, dest, maxDownloadSize)
}

func (s *UpdateService) getArchiveName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s_%s", osName, arch)
}

// validateDownloadURL checks if the URL is from an allowed domain
// SECURITY: This prevents SSRF and ensures downloads only come from trusted GitHub domains
func validateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be HTTPS
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}

	// Check against allowed hosts
	host := parsedURL.Host
	// GitHub release URLs can be from github.com or objects.githubusercontent.com
	if host != allowedDownloadHost &&
		!strings.HasSuffix(host, "."+allowedDownloadHost) &&
		host != allowedAssetHost &&
		!strings.HasSuffix(host, "."+allowedAssetHost) {
		return fmt.Errorf("download from untrusted host: %s", host)
	}

	return nil
}

func (s *UpdateService) verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	// Download checksums file
	checksumData, err := s.githubClient.FetchChecksumFile(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}
	if len(checksumData) > 1024*1024 {
		return fmt.Errorf("checksums file is too large")
	}

	// Calculate file hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash in checksums file
	fileName := filepath.Base(filePath)
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.TrimPrefix(parts[1], "*") == fileName {
			if found {
				return fmt.Errorf("ambiguous checksum for %s", fileName)
			}
			found = true
			if !strings.EqualFold(parts[0], actualHash) {
				return fmt.Errorf("checksum mismatch for %s", fileName)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}

	return fmt.Errorf("checksum not found for %s", fileName)
}

func (s *UpdateService) extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f

	// Handle gzip compression
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() { _ = gzr.Close() }()
		reader = gzr
	}

	// Handle tar archive
	if strings.Contains(archivePath, ".tar") {
		tr := tar.NewReader(io.LimitReader(reader, maxDownloadSize*2+1))
		found := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			// SECURITY: Prevent Zip Slip / Path Traversal attack
			// Only allow files with safe base names, no directory traversal
			baseName := filepath.Base(hdr.Name)

			// Check for path traversal attempts
			if strings.Contains(hdr.Name, "..") {
				return fmt.Errorf("path traversal attempt detected: %s", hdr.Name)
			}

			// Validate the entry is a regular file
			if hdr.Typeflag != tar.TypeReg {
				continue // Skip directories and special files
			}

			// Only extract the specific binary we need
			if baseName == "sub2api" || baseName == "sub2api.exe" {
				if found {
					return fmt.Errorf("multiple binaries in archive")
				}
				found = true
				// Additional security: limit file size (max 500MB)
				const maxBinarySize = 500 * 1024 * 1024
				if hdr.Size <= 0 || hdr.Size > maxBinarySize {
					return fmt.Errorf("binary too large: %d bytes (max %d)", hdr.Size, maxBinarySize)
				}

				out, err := os.Create(destPath)
				if err != nil {
					return err
				}

				// Use LimitReader to prevent decompression bombs
				limited := io.LimitReader(tr, maxBinarySize)
				n, err := io.Copy(out, limited)
				if err != nil || n != hdr.Size {
					_ = out.Close()
					return fmt.Errorf("incomplete archive binary")
				}
				if err := out.Sync(); err != nil {
					_ = out.Close()
					return err
				}
				if err := out.Close(); err != nil {
					return err
				}
			}
		}
		if found {
			return nil
		}
		return fmt.Errorf("binary not found in archive")
	}

	// Direct copy for non-tar files (with size limit)
	const maxBinarySize = 500 * 1024 * 1024
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}

	limited := io.LimitReader(reader, maxBinarySize)
	if _, err := io.Copy(out, limited); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (s *UpdateService) getFromCache(ctx context.Context) (*UpdateInfo, error) {
	if s.cache == nil {
		return nil, fmt.Errorf("update cache unavailable")
	}
	data, err := s.cache.GetUpdateInfo(ctx)
	if err != nil {
		return nil, err
	}

	var cached struct {
		SiteID      string       `json:"site_id"`
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
	}
	if cached.SiteID != releaseSiteID {
		return nil, fmt.Errorf("cached release belongs to another or unknown site")
	}

	age := time.Now().Unix() - cached.Timestamp
	if age < 0 || age > updateCacheTTL || !isCompatibleVersion(cached.Latest) {
		return nil, fmt.Errorf("cache expired")
	}
	if cached.ReleaseInfo == nil {
		return nil, fmt.Errorf("cached release metadata missing")
	}
	if compareVersions(s.currentVersion, cached.Latest) < 0 {
		contract, err := s.compatibleRelease(ctx, cached.ReleaseInfo.Assets, cached.Latest)
		if err != nil {
			return nil, err
		}
		if _, _, err := s.compatiblePlatformAssets(cached.ReleaseInfo.Assets, cached.Latest, contract); err != nil {
			return nil, err
		}
	}

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  cached.Latest,
		HasUpdate:      compareVersions(s.currentVersion, cached.Latest) < 0,
		ReleaseInfo:    cached.ReleaseInfo,
		Cached:         true,
		BuildType:      s.buildType,
	}, nil
}

func (s *UpdateService) saveToCache(ctx context.Context, info *UpdateInfo) {
	if s.cache == nil {
		return
	}
	cacheData := struct {
		SiteID      string       `json:"site_id"`
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
	}{
		SiteID:      releaseSiteID,
		Latest:      info.LatestVersion,
		ReleaseInfo: info.ReleaseInfo,
		Timestamp:   time.Now().Unix(),
	}

	data, _ := json.Marshal(cacheData)
	_ = s.cache.SetUpdateInfo(ctx, string(data), time.Duration(updateCacheTTL)*time.Second)
}

// compareVersions compares two semantic versions
func compareVersions(current, latest string) int {
	currentParts := parseVersion(current)
	latestParts := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if currentParts[i] < latestParts[i] {
			return -1
		}
		if currentParts[i] > latestParts[i] {
			return 1
		}
	}
	// Local compatibility releases can carry fixes without changing the upstream
	// version. Only this distribution's numeric revision extends upstream order.
	currentRevision, currentKitty := compatibleRevision(current)
	latestRevision, latestKitty := compatibleRevision(latest)
	if currentKitty && latestKitty {
		if currentRevision < latestRevision {
			return -1
		}
		if currentRevision > latestRevision {
			return 1
		}
	}
	return 0
}

func compatibleRevision(version string) (int, bool) {
	_, suffix, ok := strings.Cut(version, "-kitty.")
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(suffix)
	return value, err == nil && value > 0
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexByte(v, '-'); idx != -1 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	result := [3]int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		if parsed, err := strconv.Atoi(parts[i]); err == nil {
			result[i] = parsed
		}
	}
	return result
}

func (s *UpdateService) checkRestartRequired() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.restartRequired {
		return ErrUpdateRestartRequired
	}
	return nil
}
