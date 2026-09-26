//go:build unit

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func addCompatibleTestReleases(client *updateServiceGitHubClientStub, releases []*GitHubRelease) {
	client.descriptors = map[string][]byte{}
	for _, r := range releases {
		if r == nil {
			continue
		}
		p, err := currentPackageContract(r.TagName)
		if err != nil {
			panic(err)
		}
		archiveName := "sub2api_" + strings.TrimPrefix(r.TagName, "v") + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
		p.Artifacts = map[string]string{archiveName: strings.Repeat("a", 64)}
		base := "https://github.com/" + githubRepo + "/releases/download/" + r.TagName + "/"
		r.Assets = append(r.Assets, GitHubAsset{Name: archiveName, BrowserDownloadURL: base + archiveName, Size: 20}, GitHubAsset{Name: "checksums.txt", BrowserDownloadURL: base + "checksums.txt", Size: 80})
		body, _ := json.Marshal(p)
		u := "https://github.com/" + githubRepo + "/releases/download/" + r.TagName + "/" + compatibilityAsset
		r.Assets = append(r.Assets, GitHubAsset{Name: compatibilityAsset, BrowserDownloadURL: u, Size: int64(len(body))})
		client.descriptors[u] = body
	}
}

func TestCompatibleUpdateRejectsPlainUpstreamBeforeDownload(t *testing.T) {
	r := &GitHubRelease{TagName: "v2.8.16", Assets: []GitHubAsset{{Name: "sub2api_linux_amd64.tar.gz", BrowserDownloadURL: "https://github.com/ranxi2001/sub2api/releases/download/v2.8.16/sub2api_linux_amd64.tar.gz"}}}
	client := &updateServiceGitHubClientStub{release: r}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.8.15-kitty.1", "source")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.False(t, info.HasUpdate)
	require.Contains(t, info.Warning, "兼容")
	require.Error(t, svc.applyReleaseAssets(context.Background(), packageAssets(r), "2.8.16"))
}

func TestCompatibleRollbackFiltersSchemaAndCapabilityLoss(t *testing.T) {
	releases := []*GitHubRelease{{TagName: "v2.8.15-kitty.3"}, {TagName: "v2.8.15-kitty.2"}, {TagName: "v2.8.15-kitty.1"}}
	client := &updateServiceGitHubClientStub{recentReleases: releases}
	addCompatibleTestReleases(client, releases)
	for i, r := range releases[:2] {
		u := r.Assets[len(r.Assets)-1].BrowserDownloadURL
		var p compatiblePackage
		require.NoError(t, json.Unmarshal(client.descriptors[u], &p))
		if i == 0 {
			delete(p.Migrations, "254_recharge_bonus_permanent.sql")
		} else {
			p.Capabilities = p.Capabilities[1:]
		}
		client.descriptors[u], _ = json.Marshal(p)
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.8.15-kitty.4", "source")
	versions, err := svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "2.8.15-kitty.1", versions[0].Version)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "2.8.15-kitty.3"), ErrRollbackVersionNotAllowed)
}

func TestCompatiblePackageChecksVersionAndMigrationChecksum(t *testing.T) {
	p, err := currentPackageContract("2.8.15-kitty.1")
	require.NoError(t, err)
	require.NoError(t, p.validate("v2.8.15-kitty.1"))
	require.Error(t, p.validate("2.8.15-kitty.2"))
	p.Migrations["254_recharge_bonus_permanent.sql"] = strings.Repeat("0", 64)
	require.ErrorContains(t, p.validate(p.Version), "254_recharge_bonus_permanent.sql")
}

func TestCompatiblePackageURLStaysInReleaseRepository(t *testing.T) {
	valid := "https://github.com/" + githubRepo + "/releases/download/v2.8.15-kitty.1/" + compatibilityAsset
	require.NoError(t, validatePackageAssetURL(valid))
	for _, invalid := range []string{strings.Replace(valid, "https:", "http:", 1), strings.Replace(valid, githubRepo, "ranxi2001/sub2api", 1), strings.Replace(valid, "github.com", "github.com.evil.example", 1), valid + "?redirect=other"} {
		require.Error(t, validatePackageAssetURL(invalid))
	}
}

func TestCompatibleBackupRejectsMissingContractAndTamperedBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub2api.backup")
	body := []byte("test binary")
	require.NoError(t, os.WriteFile(path, body, 0600))
	require.Error(t, validateBackupContract(path))
	p, err := currentPackageContract("2.8.15-kitty.1")
	require.NoError(t, err)
	hash := sha256.Sum256(body)
	p.BinarySHA256 = hex.EncodeToString(hash[:])
	encoded, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+".compatibility.json", encoded, 0600))
	require.NoError(t, validateBackupContract(path))
	require.NoError(t, os.WriteFile(path, []byte("changed"), 0600))
	require.ErrorContains(t, validateBackupContract(path), "SHA256")
}

func TestCompatibleRevisionOrdering(t *testing.T) {
	require.Equal(t, -1, compareVersions("2.8.15-kitty.9", "2.8.15-kitty.10"))
	require.Equal(t, 1, compareVersions("2.8.16-kitty.1", "2.8.15-kitty.10"))
	require.Equal(t, 0, compareVersions("2.8.15-kitty.1", "2.8.15-kitty.1"))
}

func makeUpdateArchive(t *testing.T, contents ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, content := range contents {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sub2api", Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func compatibleArchiveFixture(t *testing.T, archive []byte) (*UpdateService, *updateServiceGitHubClientStub, string) {
	t.Helper()
	release := &GitHubRelease{TagName: "v2.8.15-kitty.2"}
	client := &updateServiceGitHubClientStub{release: release}
	addCompatibleTestReleases(client, []*GitHubRelease{release})
	asset := &release.Assets[0]
	asset.Size = int64(len(archive))
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	client.downloads = map[string][]byte{asset.BrowserDownloadURL: archive}
	client.descriptors[release.Assets[1].BrowserDownloadURL] = []byte(digest + "  " + asset.Name + "\n")
	release.Assets[1].Size = int64(len(client.descriptors[release.Assets[1].BrowserDownloadURL]))
	updateFixtureDescriptor(t, client, func(p *compatiblePackage) { p.Artifacts[asset.Name] = digest })
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "2.8.15-kitty.1", "source")
	exe := filepath.Join(t.TempDir(), "sub2api")
	require.NoError(t, os.WriteFile(exe, []byte("current binary"), 0755))
	svc.executablePath = func() (string, error) { return exe, nil }
	return svc, client, exe
}

func updateFixtureDescriptor(t *testing.T, client *updateServiceGitHubClientStub, mutate func(*compatiblePackage)) {
	t.Helper()
	a := &client.release.Assets[len(client.release.Assets)-1]
	var p compatiblePackage
	require.NoError(t, json.Unmarshal(client.descriptors[a.BrowserDownloadURL], &p))
	mutate(&p)
	body, err := json.Marshal(p)
	require.NoError(t, err)
	client.descriptors[a.BrowserDownloadURL] = body
	a.Size = int64(len(body))
}

func TestCompatibleUpdateInstallsAndPreservesValidatedBackup(t *testing.T) {
	svc, client, exe := compatibleArchiveFixture(t, makeUpdateArchive(t, "replacement binary"))
	require.NoError(t, os.WriteFile(exe+".backup", []byte("older binary"), 0700))
	require.NoError(t, os.WriteFile(exe+".backup.compatibility.json", []byte("older proof"), 0600))
	require.NoError(t, svc.PerformUpdate(context.Background()))
	body, err := os.ReadFile(exe)
	require.NoError(t, err)
	require.Equal(t, "replacement binary", string(body))
	body, err = os.ReadFile(exe + ".backup")
	require.NoError(t, err)
	require.Equal(t, "current binary", string(body))
	require.NoError(t, validateBackupContract(exe+".backup"))
	require.Equal(t, 1, client.downloadCalls)
	// This process still embeds the prior version/schema. It must not label a
	// second on-disk binary with that old contract before the requested restart.
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrUpdateRestartRequired)
	require.ErrorIs(t, svc.Rollback(), ErrUpdateRestartRequired)
	require.Equal(t, 1, client.downloadCalls)

	// Model a restarted process without invoking the executable under test.
	restarted := NewUpdateService(nil, client, "2.8.15-kitty.2", "source")
	restarted.executablePath = func() (string, error) { return exe, nil }
	require.NoError(t, restarted.Rollback())
	body, err = os.ReadFile(exe)
	require.NoError(t, err)
	require.Equal(t, "current binary", string(body))
	require.NoError(t, validateBackupContract(exe+".backup"))
}

func TestCompatibleUpdateRejectsMismatchedAssetBindingsBeforeDownload(t *testing.T) {
	for _, kind := range []string{"archive_tag", "descriptor_tag", "archive_filename", "archive_lookalike", "duplicate_checksums", "missing_archive_hash", "traversal", "encoded_path"} {
		t.Run(kind, func(t *testing.T) {
			svc, client, exe := compatibleArchiveFixture(t, makeUpdateArchive(t, "replacement"))
			switch kind {
			case "archive_tag":
				client.release.Assets[0].BrowserDownloadURL = strings.Replace(client.release.Assets[0].BrowserDownloadURL, "/v2.8.15-kitty.2/", "/v2.8.15-kitty.1/", 1)
			case "descriptor_tag":
				client.release.Assets[2].BrowserDownloadURL = strings.Replace(client.release.Assets[2].BrowserDownloadURL, "/v2.8.15-kitty.2/", "/v2.8.15-kitty.1/", 1)
			case "archive_filename":
				client.release.Assets[0].BrowserDownloadURL += ".unrelated"
			case "archive_lookalike":
				client.release.Assets[0].Name = "not-" + client.release.Assets[0].Name
			case "duplicate_checksums":
				client.release.Assets = append(client.release.Assets, client.release.Assets[1])
			case "missing_archive_hash":
				updateFixtureDescriptor(t, client, func(p *compatiblePackage) { p.Artifacts = nil })
			case "traversal":
				client.release.Assets[0].BrowserDownloadURL += "/../other"
			case "encoded_path":
				client.release.Assets[0].BrowserDownloadURL = strings.Replace(client.release.Assets[0].BrowserDownloadURL, "/sub2api_", "/%73ub2api_", 1)
			}
			require.Error(t, svc.PerformUpdate(context.Background()))
			require.Zero(t, client.downloadCalls)
			body, err := os.ReadFile(exe)
			require.NoError(t, err)
			require.Equal(t, "current binary", string(body))
		})
	}
}

func TestCompatibleUpdateVerificationFailurePreservesExistingFiles(t *testing.T) {
	for _, kind := range []string{"descriptor_hash", "checksum_hash", "duplicate_checksum", "archive_size", "duplicate_binary", "empty_binary", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			archive := makeUpdateArchive(t, "replacement")
			if kind == "duplicate_binary" {
				archive = makeUpdateArchive(t, "first", "second")
			}
			if kind == "empty_binary" {
				archive = makeUpdateArchive(t, "")
			}
			svc, client, exe := compatibleArchiveFixture(t, archive)
			require.NoError(t, os.WriteFile(exe+".backup", []byte("older binary"), 0700))
			require.NoError(t, os.WriteFile(exe+".backup.compatibility.json", []byte("older proof"), 0600))
			ctx := context.Background()
			switch kind {
			case "descriptor_hash":
				updateFixtureDescriptor(t, client, func(p *compatiblePackage) { p.Artifacts[client.release.Assets[0].Name] = strings.Repeat("b", 64) })
			case "checksum_hash":
				client.descriptors[client.release.Assets[1].BrowserDownloadURL] = []byte(strings.Repeat("c", 64) + "  " + client.release.Assets[0].Name + "\n")
			case "duplicate_checksum":
				u := client.release.Assets[1].BrowserDownloadURL
				client.descriptors[u] = append(client.descriptors[u], client.descriptors[u]...)
			case "archive_size":
				client.release.Assets[0].Size++
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			require.Error(t, svc.PerformUpdate(ctx))
			for suffix, want := range map[string]string{"": "current binary", ".backup": "older binary", ".backup.compatibility.json": "older proof"} {
				body, err := os.ReadFile(exe + suffix)
				require.NoError(t, err)
				require.Equal(t, want, string(body), suffix)
			}
			require.False(t, svc.restartRequired)
		})
	}
}

func TestCompatibleReplacementFailureRestoresPriorBackupPair(t *testing.T) {
	for _, step := range []int{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprintf("rename_%d", step), func(t *testing.T) {
			dir := t.TempDir()
			exe, candidate := filepath.Join(dir, "sub2api"), filepath.Join(dir, "candidate")
			for name, body := range map[string]string{exe: "live", candidate: "new", exe + ".backup": "old", exe + ".backup.compatibility.json": "old proof"} {
				require.NoError(t, os.WriteFile(name, []byte(body), 0700))
			}
			calls := 0
			err := replaceCompatibleBinary(exe, candidate, "2.8.15-kitty.1", func(from, to string) error {
				calls++
				if calls == step {
					return errors.New("injected rename failure")
				}
				return os.Rename(from, to)
			})
			require.ErrorContains(t, err, "injected rename failure")
			for name, want := range map[string]string{exe: "live", candidate: "new", exe + ".backup": "old", exe + ".backup.compatibility.json": "old proof"} {
				body, err := os.ReadFile(name)
				require.NoError(t, err)
				require.Equal(t, want, string(body))
			}
		})
	}
}

func TestCompatibleRollbackTamperingDoesNotReplaceLiveFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sub2api")
	require.NoError(t, os.WriteFile(exe, []byte("live"), 0700))
	require.NoError(t, os.WriteFile(exe+".backup", []byte("old"), 0700))
	require.NoError(t, writeBackupContract(exe+".backup", exe+".backup.compatibility.json", "2.8.15-kitty.1"))
	require.NoError(t, os.WriteFile(exe+".backup", []byte("tampered"), 0700))
	require.ErrorContains(t, rollbackCompatibleBinaryAt(exe), "SHA256")
	body, err := os.ReadFile(exe)
	require.NoError(t, err)
	require.Equal(t, "live", string(body))
}

func TestCompatibleUpdateCacheCannotAuthorizeOfflineMutation(t *testing.T) {
	svc, client, exe := compatibleArchiveFixture(t, makeUpdateArchive(t, "replacement"))
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	client.latestErr = errors.New("release metadata unavailable")
	info, err = svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.Cached)
	require.ErrorContains(t, svc.PerformUpdate(context.Background()), "release metadata unavailable")
	require.Zero(t, client.downloadCalls)
	body, err := os.ReadFile(exe)
	require.NoError(t, err)
	require.Equal(t, "current binary", string(body))

	cache := svc.cache.(*updateServiceCacheStub)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(cache.data), &stored))
	stored["timestamp"] = time.Now().Add(time.Hour).Unix()
	data, err := json.Marshal(stored)
	require.NoError(t, err)
	cache.data = string(data)
	_, err = svc.getFromCache(context.Background())
	require.Error(t, err)
}

func TestCompatiblePackageRejectsLegacyAndMalformedVersions(t *testing.T) {
	for _, version := range []string{"0.2.8-kittyyyds.3", "2.8.15", "2.8.15-kitty.0", "2.8.15-kitty.01", "2.8.15-kitty.1-extra", "999999999999999999999999.8.15-kitty.1", "2.8.15-kitty.999999999999999999999999"} {
		p, err := currentPackageContract(version)
		require.NoError(t, err)
		require.Error(t, p.validate(version), version)
	}
}
