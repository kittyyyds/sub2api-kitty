//go:build unit

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func withReleaseSite(t *testing.T, site string) {
	t.Helper()
	previous := releaseSiteID
	releaseSiteID = site
	t.Cleanup(func() { releaseSiteID = previous })
}

func TestCompatibleSiteRoutesLatestRollbackAndAssetsToOwnRepository(t *testing.T) {
	for site, repository := range map[string]string{"kittyapi": "kittyyyds/sub2api-kitty", "kittyyyds": "kittyyyds/sub2api-kittyyyds"} {
		t.Run(site, func(t *testing.T) {
			withReleaseSite(t, site)
			release := &GitHubRelease{TagName: "v2.9.1-kitty.2"}
			client := &updateServiceGitHubClientStub{release: release, recentReleases: []*GitHubRelease{release}}
			addCompatibleTestReleases(client, client.recentReleases)
			svc := NewUpdateService(nil, client, "2.9.1-kitty.1", "release")
			info, err := svc.CheckUpdate(context.Background(), true)
			require.NoError(t, err)
			require.True(t, info.HasUpdate)
			require.Equal(t, repository, client.latestRepo)
			svc.currentVersion = "2.9.1-kitty.3"
			versions, err := svc.ListRollbackVersions(context.Background())
			require.NoError(t, err)
			require.Len(t, versions, 1)
			require.Equal(t, repository, client.recentRepo)
			other := "kittyyyds/sub2api-kitty"
			if site == "kittyapi" {
				other = "kittyyyds/sub2api-kittyyyds"
			}
			require.Error(t, validatePackageAssetURL("https://github.com/"+other+"/releases/download/v2.9.1-kitty.2/"+compatibilityAsset))
		})
	}
}

func TestCompatibleSiteRejectsCrossSiteAndUnlabeledUpdatesBeforeBinaryDownload(t *testing.T) {
	for _, site := range []string{"kittyapi", "kittyyyds"} {
		for _, label := range []string{"kittyapi", "kittyyyds", "", "unknown"} {
			if label == site {
				continue
			}
			t.Run(site+"/"+label, func(t *testing.T) {
				withReleaseSite(t, site)
				svc, client, exe := compatibleArchiveFixture(t, makeUpdateArchive(t, "replacement"))
				updateFixtureDescriptor(t, client, func(p *compatiblePackage) { p.SiteID = label })
				require.ErrorContains(t, svc.applyReleaseAssets(context.Background(), packageAssets(client.release), "2.8.15-kitty.2"), "site identity")
				require.Zero(t, client.downloadCalls)
				body, err := os.ReadFile(exe)
				require.NoError(t, err)
				require.Equal(t, "current binary", string(body))
			})
		}
	}
}

func TestCompatibleSiteLegacyYYDSAllowlistRetainsAllGuards(t *testing.T) {
	withReleaseSite(t, "kittyyyds")
	p, err := currentPackageContract("2.8.18-kitty.4")
	require.NoError(t, err)
	p.SiteID = ""
	require.NoError(t, p.validate(p.Version))
	p.Version = "2.8.18-kitty.5"
	require.ErrorContains(t, p.validate(p.Version), "site identity")
	p.Version = "2.8.18-kitty.4"
	p.Capabilities = nil
	require.ErrorContains(t, p.validate(p.Version), "capability")
	p.Capabilities = append([]string(nil), requiredUpdateCapabilities...)
	delete(p.Migrations, "254_recharge_bonus_permanent.sql")
	require.ErrorContains(t, p.validate(p.Version), "migration")
	withReleaseSite(t, "kittyapi")
	require.ErrorContains(t, p.validate(p.Version), "site identity")
}

func TestCompatibleSiteBackupCannotCrossSites(t *testing.T) {
	withReleaseSite(t, "kittyyyds")
	path := filepath.Join(t.TempDir(), "sub2api.backup")
	require.NoError(t, os.WriteFile(path, []byte("existing YYDS binary"), 0600))
	require.NoError(t, writeBackupContract(path, path+".compatibility.json", "2.9.1-kitty.1"))
	body, err := os.ReadFile(path + ".compatibility.json")
	require.NoError(t, err)
	var p compatiblePackage
	require.NoError(t, json.Unmarshal(body, &p))
	require.Equal(t, "kittyyyds", p.SiteID)
	require.NoError(t, validateBackupContract(path))
	withReleaseSite(t, "kittyapi")
	require.ErrorContains(t, validateBackupContract(path), "site identity")
	p.SiteID = ""
	p.Version = "2.8.18-kitty.4"
	body, err = json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+".compatibility.json", body, 0600))
	require.ErrorContains(t, validateBackupContract(path), "site identity")
}

func TestCompatibleSiteUnknownBuildFailsClosed(t *testing.T) {
	withReleaseSite(t, "invalid")
	client := &updateServiceGitHubClientStub{}
	svc := NewUpdateService(nil, client, "2.9.1-kitty.1", "release")
	_, err := svc.CheckUpdate(context.Background(), true)
	require.ErrorContains(t, err, "unknown compiled release site")
	require.ErrorContains(t, svc.PerformUpdate(context.Background()), "unknown compiled release site")
	_, err = svc.ListRollbackVersions(context.Background())
	require.ErrorContains(t, err, "unknown compiled release site")
	_, err = currentPackageContract("2.9.1-kitty.1")
	require.ErrorContains(t, err, "unknown compiled release site")
	require.Empty(t, client.latestRepo)
	require.Empty(t, client.recentRepo)
	require.Zero(t, client.downloadCalls)
}

func TestCompatibleSiteCacheDoesNotCrossSitesEvenWithoutUpdate(t *testing.T) {
	withReleaseSite(t, "kittyyyds")
	cache := &updateServiceCacheStub{}
	svc := NewUpdateService(cache, nil, "2.9.1-kitty.1", "release")
	svc.saveToCache(context.Background(), &UpdateInfo{LatestVersion: "2.9.1-kitty.1", ReleaseInfo: &ReleaseInfo{}})
	_, err := svc.getFromCache(context.Background())
	require.NoError(t, err)
	withReleaseSite(t, "kittyapi")
	_, err = svc.getFromCache(context.Background())
	require.ErrorContains(t, err, "another or unknown site")
}
