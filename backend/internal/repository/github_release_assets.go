package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxGitHubReleaseMetadataSize int64 = 16 * 1024 * 1024
	maxGitHubChecksumSize        int64 = 1024 * 1024
)

var githubReleasePathSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

func validGitHubRepository(repo string) bool {
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && githubReleasePathSegment.MatchString(parts[0]) && githubReleasePathSegment.MatchString(parts[1])
}

func readGitHubResponse(resp *http.Response, maximum int64) ([]byte, error) {
	if resp.ContentLength > maximum {
		return nil, fmt.Errorf("GitHub response exceeds maximum size of %d bytes", maximum)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, fmt.Errorf("GitHub response exceeds maximum size of %d bytes", maximum)
	}
	return body, nil
}

// An asset redirect normally leads to a short-lived, signed CDN URL. Never
// forward the API credential, even to an api.github.com redirect or after a
// caller-provided hook changes the destination or headers.
func githubAssetHTTPClient(base *http.Client) *http.Client {
	client := cloneHTTPClient(base)
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		defer func() { req.Header.Del("Authorization") }()
		req.Header.Del("Authorization")
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if previous != nil {
			if err := previous(req, via); err != nil {
				return err
			}
		}
		if len(via) != 0 && via[0].URL.Scheme == "https" && (req.URL.Scheme != "https" || req.URL.User != nil) {
			return fmt.Errorf("unsafe GitHub asset redirect")
		}
		return nil
	}
	return client
}

// The update service first validates its configured repository, release tag and
// asset URL. For private repositories, resolve that browser URL against fresh
// authenticated release metadata; never send the token to github.com itself or
// trust an API URL supplied by an asset response.
func (c *githubReleaseClient) newReleaseAssetRequest(ctx context.Context, raw string) (*http.Request, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if c.updateGitHubToken == "" || u.Host != "github.com" || !strings.Contains(u.Path, "/releases/download/") {
		return http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" ||
		!validGitHubRepository(parts[0]+"/"+parts[1]) || !githubReleasePathSegment.MatchString(parts[4]) || !githubReleasePathSegment.MatchString(parts[5]) {
		return nil, fmt.Errorf("invalid authenticated GitHub release asset URL")
	}
	repo, tag, name := parts[0]+"/"+parts[1], parts[4], parts[5]
	metadataURL := "https://api.github.com/repos/" + repo + "/releases/tags/" + url.PathEscape(tag)
	metadataReq, err := c.newAPIRequest(ctx, metadataURL)
	if err != nil {
		return nil, err
	}
	// Metadata must come from this exact API endpoint. Repository moves must be
	// reviewed rather than using a redirected response as an asset authority.
	metadataClient := cloneHTTPClient(c.httpClient)
	metadataClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := metadataClient.Do(metadataReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub release asset lookup returned %d", resp.StatusCode)
	}
	body, err := readGitHubResponse(resp, maxGitHubReleaseMetadataSize)
	if err != nil {
		return nil, err
	}
	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("invalid GitHub release asset metadata")
	}
	if release.TagName != tag {
		return nil, fmt.Errorf("GitHub release asset tag mismatch")
	}
	var assetID int64
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if assetID != 0 || asset.ID <= 0 || asset.BrowserDownloadURL != raw {
			return nil, fmt.Errorf("GitHub release asset identity mismatch")
		}
		assetID = asset.ID
	}
	if assetID == 0 {
		return nil, fmt.Errorf("GitHub release asset is missing")
	}
	req, err := c.newAPIRequest(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, assetID))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}
