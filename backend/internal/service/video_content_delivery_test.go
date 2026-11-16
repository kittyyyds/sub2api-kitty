//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type videoDeliveryRoundTrip func(*http.Request) (*http.Response, error)

func (f videoDeliveryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnifiedVideoContentFollowsRedirectWithoutCredentialsAndPreservesRange(t *testing.T) {
	c, rec := grokMediaContentTestContext("GET", "https://site.example/content", map[string]string{
		"Authorization": "Bearer private", "Cookie": "private=yes", "Range": "bytes=0-4", "If-Range": `"v1"`,
	})
	client := *unifiedVideoContentClient
	calls := 0
	client.Transport = videoDeliveryRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("Origin"))
		require.Equal(t, "bytes=0-4", r.Header.Get("Range"))
		require.Equal(t, `"v1"`, r.Header.Get("If-Range"))
		if calls == 1 {
			return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"https://media.example/video.mp4"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-4/100"}, "Etag": {`"v1"`}}, ContentLength: 5, Body: io.NopCloser(strings.NewReader("video")), Request: r}, nil
	})
	require.NoError(t, streamUnifiedVideoContent(c, "https://supplier.example/content?signature=fixture", &client))
	require.Equal(t, 2, calls)
	require.Equal(t, 206, rec.Code)
	require.Equal(t, "video", rec.Body.String())
	require.Equal(t, "bytes 0-4/100", rec.Header().Get("Content-Range"))
	require.Equal(t, `"v1"`, rec.Header().Get("Etag"))
	require.Empty(t, rec.Header().Get("Location"))
}

func TestUnifiedVideoContentRejectsUnsafeRedirectsAndURLs(t *testing.T) {
	for _, target := range []string{"http://media.example/a", "https://127.0.0.1/a", "https://10.0.0.2/a", "https://169.254.169.254/a", "https://[::1]/a", "https://[::ffff:127.0.0.1]/a", "https://100.64.0.1/a", "https://user:pass@media.example/a", "https://media.example:444/a", "https://metadata.google.internal/a"} {
		t.Run(target, func(t *testing.T) {
			require.Error(t, validateUnifiedVideoContentURL(target))
			c, rec := grokMediaContentTestContext("GET", "https://site.example/content", nil)
			client := *unifiedVideoContentClient
			calls := 0
			client.Transport = videoDeliveryRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 307, Header: http.Header{"Location": {target}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})
			require.Error(t, streamUnifiedVideoContent(c, "https://supplier.example/content", &client))
			require.Equal(t, 1, calls)
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestUnifiedVideoContentRejectsErrorDocumentsAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		kind, body string
		length     int64
	}{{"application/json", "{}", 2}, {"text/html", "error", 5}, {"video/mp4", "short", 100}} {
		c, _ := grokMediaContentTestContext("GET", "https://site.example/content", nil)
		client := *unifiedVideoContentClient
		client.Transport = videoDeliveryRoundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {tc.kind}}, Body: io.NopCloser(strings.NewReader(tc.body)), ContentLength: tc.length, Request: r}, nil
		})
		require.Error(t, streamUnifiedVideoContent(c, "https://supplier.example/video", &client))
	}
}

func TestUnifiedVideoContentPreservesRangeRejection(t *testing.T) {
	c, rec := grokMediaContentTestContext("GET", "https://site.example/content", nil)
	client := *unifiedVideoContentClient
	client.Transport = videoDeliveryRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 416, Header: http.Header{"Content-Range": {"bytes */5"}}, Body: io.NopCloser(strings.NewReader("private error")), Request: r}, nil
	})
	require.NoError(t, streamUnifiedVideoContent(c, "https://supplier.example/video", &client))
	require.Equal(t, 416, rec.Code)
	require.Equal(t, "bytes */5", rec.Header().Get("Content-Range"))
	require.Empty(t, rec.Body.String())
}
