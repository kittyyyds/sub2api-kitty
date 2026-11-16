package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMediaDownloadIdleAndClientCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, _, release := mediaDownloadContext(parent, time.Second, time.Second)
	defer release()
	cancel()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	idle, _, done := mediaDownloadContext(context.Background(), time.Second, 20*time.Millisecond)
	defer done()
	select {
	case <-idle.Done():
		require.Error(t, idle.Err())
	case <-time.After(time.Second):
		t.Fatal("stalled media transfer was not cancelled")
	}
}

func TestMediaDownloadPreservesRangeAndDetectsTruncation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	r := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: 4,
		Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-3/10"}, "Etag": {`"version1"`}, "Accept-Ranges": {"bytes"}},
		Body:   io.NopCloser(strings.NewReader("test"))}
	require.NoError(t, writeGrokMediaContentResponse(c, r))
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Equal(t, `"version1"`, w.Header().Get("ETag"))
	require.Equal(t, "bytes 0-3/10", w.Header().Get("Content-Range"))
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	r.Body = io.NopCloser(strings.NewReader("bad"))
	require.ErrorIs(t, writeGrokMediaContentResponse(c, r), io.ErrUnexpectedEOF)
}
