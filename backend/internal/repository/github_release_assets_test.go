package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const privateReleaseURL = "https://github.com/kittyyyds/sub2api-kittyyyds/releases/download/v2.8.15-kitty.1/checksums.txt"
const privateReleaseAPI = "https://api.github.com/repos/kittyyyds/sub2api-kittyyyds/releases/tags/v2.8.15-kitty.1"
const privateAssetAPI = "https://api.github.com/repos/kittyyyds/sub2api-kittyyyds/releases/assets/765"

func privateAssetMetadata() map[string]any {
	return map[string]any{"tag_name": "v2.8.15-kitty.1", "assets": []any{
		map[string]any{"id": 765, "name": "checksums.txt", "browser_download_url": privateReleaseURL,
			// This untrusted field must never be used as an authenticated URL.
			"url": "https://attacker.invalid/credential-collector"},
	}}
}

func releaseTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: -1, Request: req}
}

func privateReleaseClient(t *testing.T, metadata any, assetTransport githubReleaseRoundTripFunc) *githubReleaseClient {
	t.Helper()
	body, err := json.Marshal(metadata)
	require.NoError(t, err)
	client := newTestGitHubReleaseClient()
	client.updateGitHubToken = "synthetic-private-token"
	transport := githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == privateReleaseAPI {
			require.Equal(t, "Bearer synthetic-private-token", req.Header.Get("Authorization"))
			require.Equal(t, "application/vnd.github.v3+json", req.Header.Get("Accept"))
			return releaseTestResponse(req, http.StatusOK, string(body)), nil
		}
		return assetTransport(req)
	})
	client.httpClient.Transport, client.downloadHTTPClient.Transport = transport, transport
	return client
}

func TestGitHubPrivateReleaseAssetDownloadAndChecksum(t *testing.T) {
	for _, toFile := range []bool{false, true} {
		t.Run(fmt.Sprint("file=", toFile), func(t *testing.T) {
			var assetCalls, cdnCalls int
			client := privateReleaseClient(t, privateAssetMetadata(), func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case privateAssetAPI:
					assetCalls++
					require.Equal(t, "Bearer synthetic-private-token", req.Header.Get("Authorization"))
					require.Equal(t, "application/octet-stream", req.Header.Get("Accept"))
					resp := releaseTestResponse(req, http.StatusFound, "")
					resp.Header.Set("Location", "https://release-assets.githubusercontent.com/signed?token=synthetic-cdn-signature")
					return resp, nil
				case "https://release-assets.githubusercontent.com/signed?token=synthetic-cdn-signature":
					cdnCalls++
					require.Empty(t, req.Header.Get("Authorization"), "GitHub API credential must not reach the signed CDN")
					return releaseTestResponse(req, http.StatusOK, "synthetic-package-bytes"), nil
				default:
					t.Fatalf("unexpected request destination %s", req.URL.Redacted())
					return nil, fmt.Errorf("unexpected destination")
				}
			})
			// Header sanitization also runs after a custom/shared redirect hook.
			for _, httpClient := range []*http.Client{client.httpClient, client.downloadHTTPClient} {
				httpClient.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
					req.Header = http.Header{"Authorization": []string{"Bearer synthetic-private-token"}}
					return nil
				}
			}
			if toFile {
				dest := filepath.Join(t.TempDir(), "asset")
				require.NoError(t, client.DownloadFile(context.Background(), privateReleaseURL, dest, 1024))
				body, err := os.ReadFile(dest)
				require.NoError(t, err)
				require.Equal(t, "synthetic-package-bytes", string(body))
			} else {
				body, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
				require.NoError(t, err)
				require.Equal(t, "synthetic-package-bytes", string(body))
			}
			require.Equal(t, 1, assetCalls)
			require.Equal(t, 1, cdnCalls)
		})
	}
}

func TestGitHubPrivateReleaseRejectsAssetIdentityMismatch(t *testing.T) {
	cases := map[string]func(map[string]any){
		"tag":           func(m map[string]any) { m["tag_name"] = "other" },
		"missing":       func(m map[string]any) { m["assets"] = []any{} },
		"wrong name":    func(m map[string]any) { m["assets"].([]any)[0].(map[string]any)["name"] = "other" },
		"zero id":       func(m map[string]any) { m["assets"].([]any)[0].(map[string]any)["id"] = 0 },
		"negative id":   func(m map[string]any) { m["assets"].([]any)[0].(map[string]any)["id"] = -5 },
		"fractional id": func(m map[string]any) { m["assets"].([]any)[0].(map[string]any)["id"] = 1.5 },
		"wrong repository": func(m map[string]any) {
			m["assets"].([]any)[0].(map[string]any)["browser_download_url"] = strings.Replace(privateReleaseURL, "kittyyyds/", "other/", 1)
		},
		"duplicate": func(m map[string]any) { assets := m["assets"].([]any); m["assets"] = append(assets, assets[0]) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			metadata := privateAssetMetadata()
			mutate(metadata)
			client := privateReleaseClient(t, metadata, func(req *http.Request) (*http.Response, error) {
				t.Fatalf("invalid metadata must not authorize an asset request: %s", req.URL.Redacted())
				return nil, fmt.Errorf("unexpected asset request")
			})
			_, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
			require.Error(t, err)
		})
	}
}

func TestGitHubPrivateReleaseMetadataFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := newTestGitHubReleaseClient()
			client.updateGitHubToken = "synthetic-private-token"
			calls := 0
			client.httpClient.Transport = githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, privateReleaseAPI, req.URL.String())
				resp := releaseTestResponse(req, status, "")
				resp.Header.Set("Location", "https://attacker.invalid/metadata")
				return resp, nil
			})
			_, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
			require.Error(t, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestGitHubPrivateReleaseRejectsAmbiguousPaths(t *testing.T) {
	for _, raw := range []string{
		strings.Replace(privateReleaseURL, "https:", "http:", 1), privateReleaseURL + "?x=1", privateReleaseURL + "#fragment",
		strings.Replace(privateReleaseURL, "github.com/", "person@github.com/", 1),
		strings.Replace(privateReleaseURL, "checksums.txt", "../checksums.txt", 1),
		strings.Replace(privateReleaseURL, "checksums.txt", "%63hecksums.txt", 1),
		strings.Replace(privateReleaseURL, "v2.8.15-kitty.1", "v2.8.15%2Fkitty.1", 1),
	} {
		t.Run(raw, func(t *testing.T) {
			client := newTestGitHubReleaseClient()
			client.updateGitHubToken = "synthetic-private-token"
			client.httpClient.Transport = githubReleaseRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid URL must not send an API request")
				return nil, fmt.Errorf("unexpected request")
			})
			_, err := client.newReleaseAssetRequest(context.Background(), raw)
			require.Error(t, err)
		})
	}
}

func TestGitHubPublicReleaseKeepsAnonymousDownload(t *testing.T) {
	client := newTestGitHubReleaseClient()
	calls := 0
	client.httpClient.Transport = githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, privateReleaseURL, req.URL.String())
		require.Empty(t, req.Header.Get("Authorization"))
		return releaseTestResponse(req, http.StatusOK, "public"), nil
	})
	body, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
	require.NoError(t, err)
	require.Equal(t, "public", string(body))
	require.Equal(t, 1, calls)
}

func TestGitHubReleaseResponseBounds(t *testing.T) {
	for _, advertised := range []bool{false, true} {
		t.Run(fmt.Sprint("content-length=", advertised), func(t *testing.T) {
			client := newTestGitHubReleaseClient()
			client.httpClient.Transport = githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp := releaseTestResponse(req, http.StatusOK, strings.Repeat("x", int(maxGitHubChecksumSize+1)))
				if advertised {
					resp.ContentLength = maxGitHubChecksumSize + 1
				}
				return resp, nil
			})
			_, err := client.FetchChecksumFile(context.Background(), "https://objects.githubusercontent.com/checksums")
			require.ErrorContains(t, err, "maximum size")
		})
	}
	client := privateReleaseClient(t, privateAssetMetadata(), func(req *http.Request) (*http.Response, error) {
		return releaseTestResponse(req, http.StatusOK, "oversized-private-file"), nil
	})
	dest := filepath.Join(t.TempDir(), "partial")
	require.ErrorContains(t, client.DownloadFile(context.Background(), privateReleaseURL, dest, 5), "maximum size")
	_, err := os.Stat(dest)
	require.True(t, os.IsNotExist(err))
}

func TestGitHubAssetRedirectDowngradeAndLoopRejected(t *testing.T) {
	for _, location := range []string{"http://release-assets.githubusercontent.com/insecure", "https://user@release-assets.githubusercontent.com/asset", privateAssetAPI} {
		t.Run(location, func(t *testing.T) {
			calls := 0
			client := privateReleaseClient(t, privateAssetMetadata(), func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, privateAssetAPI, req.URL.String(), "downgraded or userinfo destination must never be contacted")
				if calls > 1 {
					require.Empty(t, req.Header.Get("Authorization"))
				}
				resp := releaseTestResponse(req, http.StatusFound, "")
				resp.Header.Set("Location", location)
				return resp, nil
			})
			_, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
			require.Error(t, err)
			require.LessOrEqual(t, calls, 10)
		})
	}
}

func TestGitHubPrivateReleaseMetadataBoundsAndInvalidRepository(t *testing.T) {
	client := newTestGitHubReleaseClient()
	client.updateGitHubToken = "synthetic-private-token"
	calls := 0
	client.httpClient.Transport = githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := releaseTestResponse(req, http.StatusOK, "")
		resp.ContentLength = maxGitHubReleaseMetadataSize + 1
		return resp, nil
	})
	_, err := client.FetchChecksumFile(context.Background(), privateReleaseURL)
	require.ErrorContains(t, err, "maximum size")
	_, err = client.FetchLatestRelease(context.Background(), "kittyyyds/sub2api-kittyyyds")
	require.ErrorContains(t, err, "maximum size")
	_, err = client.FetchRecentReleases(context.Background(), "kittyyyds/sub2api-kittyyyds", 15)
	require.ErrorContains(t, err, "maximum size")
	require.Equal(t, 3, calls)
	for _, repo := range []string{"../repo", "owner/repo?query=1", "owner/repo/extra", "owner/%2Frepo"} {
		_, err := client.FetchLatestRelease(context.Background(), repo)
		require.ErrorContains(t, err, "invalid GitHub repository")
		_, err = client.FetchRecentReleases(context.Background(), repo, 15)
		require.ErrorContains(t, err, "invalid GitHub repository")
	}
	require.Equal(t, 3, calls, "invalid repository paths never reach authenticated API transport")
}
