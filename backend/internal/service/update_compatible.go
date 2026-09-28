package service

// The upstream download/checksum/atomic-replacement flow is retained. A small
// package contract prevents an ordinary upstream binary from replacing the
// permanent-credit and media/account behavior carried by this distribution.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/migrations"
)

const compatibilityAsset = "kitty-compatibility.json"
const compatibilityFamily = "kitty-ranxi-console"

// Inject with -X github.com/Wei-Shaw/sub2api/internal/service.releaseSiteID=kittyapi
// for KittyAPI. The default preserves existing KittyYYDS source builds; neither
// environment settings nor an incoming release may change this identity.
var releaseSiteID = "kittyyyds"

func releaseRepository() (string, error) {
	switch releaseSiteID {
	case "kittyyyds":
		return "kittyyyds/sub2api-kittyyyds", nil
	case "kittyapi":
		return "kittyyyds/sub2api-kitty", nil
	default:
		return "", fmt.Errorf("unknown compiled release site identity; updates are disabled")
	}
}

var compatibleVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-kitty\.([1-9][0-9]*)$`)

func isCompatibleVersion(version string) bool {
	parts := compatibleVersionPattern.FindStringSubmatch(version)
	if parts == nil {
		return false
	}
	for _, part := range parts[1:] {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

var requiredUpdateCapabilities = []string{
	"permanent-recharge-ledger-v1", "final-media-pricing-v1", "canvas-media-tasks-v1",
	"account-request-policy-v1", "kitty-console-v1", "user-group-denied-models-v1",
}

type compatiblePackage struct {
	Schema       int               `json:"schema"`
	Family       string            `json:"family"`
	SiteID       string            `json:"site_id"`
	Version      string            `json:"version"`
	Capabilities []string          `json:"capabilities"`
	Migrations   map[string]string `json:"migrations"`
	Artifacts    map[string]string `json:"artifacts"`
	BinarySHA256 string            `json:"binary_sha256,omitempty"`
	ReleaseTag   string            `json:"-"`
}

func currentPackageContract(version string) (*compatiblePackage, error) {
	if _, err := releaseRepository(); err != nil {
		return nil, err
	}
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return nil, err
	}
	rows := make(map[string]string, len(files))
	for _, name := range files {
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(body))))
		rows[name] = hex.EncodeToString(sum[:])
	}
	return &compatiblePackage{Schema: 1, Family: compatibilityFamily, SiteID: releaseSiteID, Version: version,
		Capabilities: append([]string(nil), requiredUpdateCapabilities...), Migrations: rows}, nil
}

func (p *compatiblePackage) validate(expectedVersion string) error {
	if p == nil || p.Schema != 1 || p.Family != compatibilityFamily ||
		!isCompatibleVersion(p.Version) || !isCompatibleVersion(expectedVersion) ||
		strings.TrimPrefix(p.Version, "v") != strings.TrimPrefix(expectedVersion, "v") {
		return fmt.Errorf("release is not a compatible Kitty package")
	}
	if _, err := releaseRepository(); err != nil {
		return err
	}
	// Only the verified pre-binding KittyYYDS release may lack a site label.
	// Existing migration/capability/hash checks still apply to that release.
	legacyYYDS := releaseSiteID == "kittyyyds" && p.SiteID == "" &&
		strings.TrimPrefix(p.Version, "v") == "2.8.18-kitty.4"
	if p.SiteID != releaseSiteID && !legacyYYDS {
		return fmt.Errorf("release site identity does not match this site")
	}
	have := make(map[string]bool, len(p.Capabilities))
	for _, capability := range p.Capabilities {
		have[capability] = true
	}
	for _, capability := range requiredUpdateCapabilities {
		if !have[capability] {
			return fmt.Errorf("release lacks required capability: %s", capability)
		}
	}
	current, err := currentPackageContract(expectedVersion)
	if err != nil {
		return err
	}
	for name, checksum := range current.Migrations {
		if p.Migrations[name] != checksum {
			return fmt.Errorf("release cannot preserve current database migration: %s", name)
		}
	}
	return nil
}

func packageAssets(release *GitHubRelease) []Asset {
	if release == nil {
		return nil
	}
	assets := make([]Asset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = Asset{Name: a.Name, DownloadURL: a.BrowserDownloadURL, Size: a.Size}
	}
	return assets
}

func validatePackageAssetURL(raw string) error {
	repository, err := releaseRepository()
	if err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !strings.HasPrefix(u.Path, "/"+repository+"/releases/download/") {
		return fmt.Errorf("compatible package must come from the configured release repository")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"+repository+"/releases/download/"), "/")
	if len(parts) != 2 || !isCompatibleVersion(parts[0]) || parts[1] == "" ||
		strings.ContainsAny(parts[1], `\\`) || parts[1] == "." || parts[1] == ".." {
		return fmt.Errorf("invalid compatible release asset path")
	}
	return nil
}

func validatePackageReleaseAsset(asset Asset, version, tag string) (string, error) {
	if err := validatePackageAssetURL(asset.DownloadURL); err != nil {
		return "", err
	}
	u, _ := url.Parse(asset.DownloadURL)
	repository, _ := releaseRepository() // already checked by validatePackageAssetURL
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"+repository+"/releases/download/"), "/")
	if parts[1] != asset.Name || strings.TrimPrefix(parts[0], "v") != strings.TrimPrefix(version, "v") ||
		(tag != "" && parts[0] != tag) {
		return "", fmt.Errorf("release asset does not match its version, tag or filename")
	}
	return parts[0], nil
}

func (s *UpdateService) compatiblePlatformAssets(assets []Asset, version string, contract *compatiblePackage) (Asset, Asset, error) {
	var archive, checksums Asset
	wanted := "sub2api_" + strings.TrimPrefix(version, "v") + "_" + s.getArchiveName() + ".tar.gz"
	legacy := "sub2api_" + s.getArchiveName() + ".tar.gz"
	for _, asset := range assets {
		if asset.Name == wanted || asset.Name == legacy {
			if archive.Name != "" {
				return Asset{}, Asset{}, fmt.Errorf("ambiguous platform archives")
			}
			archive = asset
		}
		if asset.Name == "checksums.txt" {
			if checksums.Name != "" {
				return Asset{}, Asset{}, fmt.Errorf("ambiguous release checksums")
			}
			checksums = asset
		}
	}
	if archive.Name == "" {
		return Asset{}, Asset{}, fmt.Errorf("no compatible release found for %s", s.getArchiveName())
	}
	wantSHA, err := hex.DecodeString(contract.Artifacts[archive.Name])
	if err != nil || len(wantSHA) != sha256.Size || checksums.Name == "" || archive.Size <= 0 || archive.Size > maxDownloadSize || checksums.Size <= 0 || checksums.Size > 1024*1024 {
		return Asset{}, Asset{}, fmt.Errorf("compatible archive checksums and valid sizes are required")
	}
	for _, asset := range []Asset{archive, checksums} {
		if _, err := validatePackageReleaseAsset(asset, version, contract.ReleaseTag); err != nil {
			return Asset{}, Asset{}, err
		}
	}
	return archive, checksums, nil
}

func (s *UpdateService) compatibleRelease(ctx context.Context, assets []Asset, version string) (*compatiblePackage, error) {
	var descriptor Asset
	for _, asset := range assets {
		if asset.Name != compatibilityAsset {
			continue
		}
		if descriptor.Name != "" || asset.Size <= 0 || asset.Size > 1024*1024 {
			return nil, fmt.Errorf("invalid package compatibility descriptor")
		}
		descriptor = asset
	}
	if descriptor.Name == "" {
		return nil, fmt.Errorf("this release has no Kitty compatibility descriptor; the existing site is preserved")
	}
	tag, err := validatePackageReleaseAsset(descriptor, version, "")
	if err != nil {
		return nil, err
	}
	body, err := s.githubClient.FetchChecksumFile(ctx, descriptor.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("read package compatibility: %w", err)
	}
	if len(body) > 1024*1024 || int64(len(body)) != descriptor.Size {
		return nil, fmt.Errorf("package compatibility descriptor size mismatch")
	}
	var p compatiblePackage
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("invalid package compatibility descriptor")
	}
	if err := p.validate(version); err != nil {
		return nil, err
	}
	p.ReleaseTag = tag
	return &p, nil
}

func checkPackageFile(path, want string) error {
	decoded, err := hex.DecodeString(want)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("missing package SHA256")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want) {
		return fmt.Errorf("package SHA256 mismatch")
	}
	return nil
}

func writeBackupContract(exePath, contractPath, version string) error {
	p, err := currentPackageContract(version)
	if err != nil {
		return err
	}
	if err := p.validate(version); err != nil {
		return err
	}
	f, err := os.Open(exePath)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	_ = f.Close()
	if err != nil {
		return err
	}
	p.BinarySHA256 = hex.EncodeToString(h.Sum(nil))
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(contractPath, body, 0600)
}

// Stage the previous executable without removing the live path. Only the final
// same-directory rename changes the executable; failures restore the old backup
// pair, so an unsuccessful update cannot destroy an existing rollback target.
func replaceCompatibleBinary(exePath, candidatePath, version string, rename func(string, string) error) error {
	stage, err := os.MkdirTemp(filepath.Dir(exePath), ".sub2api-swap-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	previous := filepath.Join(stage, "previous")
	if err := copyUpdateBinary(exePath, previous); err != nil {
		return err
	}
	proof := previous + ".compatibility.json"
	if err := writeBackupContract(previous, proof, version); err != nil {
		return err
	}
	backup := exePath + ".backup"
	type movedFile struct{ from, to string }
	var moved []movedFile
	var installed []string
	restore := func(cause error) error {
		var failures []string
		for i := len(installed) - 1; i >= 0; i-- {
			if err := os.Remove(installed[i]); err != nil && !os.IsNotExist(err) {
				failures = append(failures, err.Error())
			}
		}
		for i := len(moved) - 1; i >= 0; i-- {
			if err := rename(moved[i].to, moved[i].from); err != nil {
				failures = append(failures, err.Error())
			}
		}
		if len(failures) > 0 {
			cleanup = false
			return fmt.Errorf("update failed: %w; preserve recovery files in %s: %s", cause, stage, strings.Join(failures, "; "))
		}
		return cause
	}
	for i, target := range []string{backup, backup + ".compatibility.json"} {
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return restore(fmt.Errorf("existing backup path is not a regular file: %s", target))
		}
		stash := filepath.Join(stage, fmt.Sprintf("old-%d", i))
		if err := rename(target, stash); err != nil {
			return restore(err)
		}
		moved = append(moved, movedFile{target, stash})
	}
	for _, pair := range []movedFile{{previous, backup}, {proof, backup + ".compatibility.json"}} {
		if err := rename(pair.from, pair.to); err != nil {
			return restore(err)
		}
		installed = append(installed, pair.to)
	}
	if err := rename(candidatePath, exePath); err != nil {
		return restore(fmt.Errorf("replace failed: %w", err))
	}
	return nil
}

func copyUpdateBinary(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxDownloadSize {
		return fmt.Errorf("update source is not a bounded regular binary")
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, maxDownloadSize+1))
	if copyErr == nil && n != info.Size() {
		copyErr = fmt.Errorf("update source changed while copying")
	}
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validateBackupContract(backupPath string) error {
	f, err := os.Open(backupPath + ".compatibility.json")
	if err != nil {
		return fmt.Errorf("backup has no compatibility proof; choose a compatible release instead")
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return fmt.Errorf("invalid backup compatibility proof")
	}
	var p compatiblePackage
	if json.Unmarshal(body, &p) != nil {
		return fmt.Errorf("invalid backup compatibility proof")
	}
	if err := p.validate(p.Version); err != nil {
		return err
	}
	return checkPackageFile(backupPath, p.BinarySHA256)
}
