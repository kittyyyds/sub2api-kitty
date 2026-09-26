//go:build unit

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepo     string
	descriptors    map[string][]byte
	latestErr      error
	downloadErr    error
	downloads      map[string][]byte
	downloadCalls  int
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepo = repo
	return s.release, s.latestErr
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(_ context.Context, source, dest string, limit int64) error {
	s.downloadCalls++
	if s.downloadErr != nil {
		return s.downloadErr
	}
	body, ok := s.downloads[source]
	if !ok {
		return errors.New("fixture download unavailable")
	}
	if int64(len(body)) > limit {
		return errors.New("fixture download too large")
	}
	return os.WriteFile(dest, body, 0600)
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(_ context.Context, source string) ([]byte, error) {
	if body, ok := s.descriptors[source]; ok {
		return body, nil
	}
	return nil, errors.New("fixture checksum unavailable")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	githubClient := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v2.8.15-kitty.132",
			Name:    "v2.8.15-kitty.132",
		},
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		githubClient,
		"2.8.15-kitty.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
	require.Equal(t, githubRepo, githubClient.latestRepo)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	client := &updateServiceGitHubClientStub{recentReleases: releases}
	addCompatibleTestReleases(client, releases)
	return NewUpdateService(
		&updateServiceCacheStub{},
		client,
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.8.15-kitty.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v2.8.15-kitty.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v2.8.15-kitty.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v2.8.15-kitty.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v2.8.15-kitty.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v2.8.15-kitty.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v2.8.15-kitty.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v2.8.15-kitty.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v2.8.15-kitty.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("2.8.15-kitty.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "2.8.15-kitty.146", versions[0].Version)
	require.Equal(t, "2.8.15-kitty.144", versions[1].Version)
	require.Equal(t, "2.8.15-kitty.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.8.15-kitty.144"},
		{TagName: "v2.8.15-kitty.146"},
		{TagName: "v2.8.15-kitty.145"},
	}
	svc := newRollbackTestService("2.8.15-kitty.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "2.8.15-kitty.146", versions[0].Version)
	require.Equal(t, "2.8.15-kitty.145", versions[1].Version)
	require.Equal(t, "2.8.15-kitty.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.8.15-kitty.147"},
		{TagName: "v2.8.15-kitty.148"},
	}
	svc := newRollbackTestService("2.8.15-kitty.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"2.8.15-kitty.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v2.8.15-kitty.148"},
		{TagName: "v2.8.15-kitty.147"},
		{TagName: "v2.8.15-kitty.146"},
		{TagName: "v2.8.15-kitty.145"},
		{TagName: "v2.8.15-kitty.144"},
		{TagName: "v2.8.15-kitty.143"},
		{TagName: "v2.8.15-kitty.142"},
	}
	svc := newRollbackTestService("2.8.15-kitty.147", releases)

	for _, target := range []string{
		"",                  // empty
		"2.8.15-kitty.147",  // current version
		"v2.8.15-kitty.147", // current version with prefix
		"2.8.15-kitty.148",  // newer than current
		"2.8.15-kitty.142",  // older than the 3 most recent
		"9.9.9",             // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	releases := []*GitHubRelease{{TagName: "v2.8.15-kitty.147"}, {TagName: "v2.8.15-kitty.146"}}
	svc := newRollbackTestService("2.8.15-kitty.147", releases)
	client := svc.githubClient.(*updateServiceGitHubClientStub)
	client.downloadErr = errors.New("fixture download stopped")
	exe := filepath.Join(t.TempDir(), "sub2api")
	require.NoError(t, os.WriteFile(exe, []byte("current"), 0700))
	svc.executablePath = func() (string, error) { return exe, nil }
	err := svc.RollbackToVersion(context.Background(), "v2.8.15-kitty.146")
	require.ErrorContains(t, err, "fixture download stopped")
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	body, err := os.ReadFile(exe)
	require.NoError(t, err)
	require.Equal(t, "current", string(body))
}
