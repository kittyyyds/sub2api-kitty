//go:build unit

package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/videoreference"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type referenceDeadlineRecorder struct{ *httptest.ResponseRecorder }

func (r *referenceDeadlineRecorder) SetReadDeadline(time.Time) error  { return nil }
func (r *referenceDeadlineRecorder) SetWriteDeadline(time.Time) error { return nil }
func newReferenceRecorder() *referenceDeadlineRecorder {
	return &referenceDeadlineRecorder{httptest.NewRecorder()}
}

func referenceMultipart(t *testing.T, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	file, err := writer.CreateFormFile("file", "reference.png")
	require.NoError(t, err)
	_, err = file.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf, writer.FormDataContentType()
}

func TestVideoReferenceUploadAuthenticationAndContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := videoreference.New(t.TempDir())
	require.NoError(t, err)
	h := &VideoReferenceHandler{store: store, baseURL: "https://media.example", uploads: make(chan struct{}, 4), downloads: make(chan struct{}, 16)}
	for _, tc := range []struct {
		name, platform string
		auth           bool
		status         int
	}{{"missing", "", false, 401}, {"text", "openai", true, 403}, {"video", "video", true, 201}} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/upload", func(c *gin.Context) {
				if tc.auth {
					c.Set("api_key", &service.APIKey{UserID: 17, Group: &service.Group{Platform: tc.platform}})
				}
				h.Upload(c)
			})
			r.GET("/media/:id", h.Content)
			buf, contentType := referenceMultipart(t, []byte("\x89PNG\r\n\x1a\nfixture"))
			req := httptest.NewRequest("POST", "/upload", buf)
			req.Header.Set("Content-Type", contentType)
			w := newReferenceRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status != 201 {
				return
			}
			var result struct {
				ID, URL   string
				ExpiresAt time.Time `json:"expires_at"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, "https://media.example/media/video-references/"+result.ID, result.URL)
			require.WithinDuration(t, time.Now().Add(24*time.Hour), result.ExpiresAt, time.Minute)
			req = httptest.NewRequest("GET", "/media/"+result.ID, nil)
			req.Header.Set("Range", "bytes=0-3")
			w = newReferenceRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, 206, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			require.Len(t, w.Body.Bytes(), 4)
		})
	}
}

func TestVideoReferenceFailedMultipartRemovesFile(t *testing.T) {
	store, err := videoreference.New(t.TempDir())
	require.NoError(t, err)
	h := &VideoReferenceHandler{store: store, uploads: make(chan struct{}, 4)}
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	for i := 0; i < 2; i++ {
		file, err := writer.CreateFormFile("file", "test.png")
		require.NoError(t, err)
		_, err = io.WriteString(file, "\x89PNG\r\n\x1a\nfixture")
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	r := gin.New()
	r.POST("/upload", func(c *gin.Context) {
		c.Set("api_key", &service.APIKey{UserID: 1, Group: &service.Group{Platform: "video"}})
		h.Upload(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/upload", buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := newReferenceRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
}

func TestVideoReferenceUnsupportedDeadlineRejectsUpload(t *testing.T) {
	store, err := videoreference.New(t.TempDir())
	require.NoError(t, err)
	h := &VideoReferenceHandler{store: store, uploads: make(chan struct{}, 4)}
	r := gin.New()
	r.POST("/upload", func(c *gin.Context) {
		c.Set("api_key", &service.APIKey{UserID: 1, Group: &service.Group{Platform: "video"}})
		h.Upload(c)
	})
	body, ctype := referenceMultipart(t, []byte("\x89PNG\r\n\x1a\nfixture"))
	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 503, w.Code)
	require.Empty(t, h.uploads)
}

func TestVideoReferenceSlowUploadReleasesSlot(t *testing.T) {
	store, err := videoreference.New(t.TempDir())
	require.NoError(t, err)
	h := &VideoReferenceHandler{store: store, uploads: make(chan struct{}, 4), uploadTimeout: 50 * time.Millisecond}
	done := make(chan struct{}, 1)
	r := gin.New()
	r.POST("/upload", func(c *gin.Context) {
		c.Set("api_key", &service.APIKey{UserID: 1, Group: &service.Group{Platform: "video"}})
		h.Upload(c)
		done <- struct{}{}
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = io.WriteString(conn, "POST /upload HTTP/1.1\r\nHost: localhost\r\nContent-Type: multipart/form-data; boundary=x\r\nContent-Length: 10000\r\n\r\n--x\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\n\x89PNG\r\n\x1a\n")
	require.NoError(t, err)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("slow upload kept handler slot")
	}
	require.Empty(t, h.uploads)
	_, err = store.Put(1, bytes.NewReader([]byte("\x89PNG\r\n\x1a\nfixture")))
	require.NoError(t, err, "timed-out upload leaked quota or mutex")
}

func TestVideoReferenceSlowDownloadReleasesSlot(t *testing.T) {
	store, err := videoreference.New(t.TempDir())
	require.NoError(t, err)
	data := make([]byte, videoreference.MaxFileBytes)
	copy(data, "\x89PNG\r\n\x1a\n")
	entry, err := store.Put(1, bytes.NewReader(data))
	require.NoError(t, err)
	h := &VideoReferenceHandler{store: store, downloads: make(chan struct{}, 1), downloadTimeout: 50 * time.Millisecond}
	done := make(chan struct{}, 1)
	r := gin.New()
	r.GET("/media/:id", func(c *gin.Context) { h.Content(c); done <- struct{}{} })
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = io.WriteString(conn, "GET /media/"+entry.ID+" HTTP/1.1\r\nHost: localhost\r\n\r\n")
	require.NoError(t, err)
	// Never read the 25MiB response: the peer's TCP receive window fills.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("slow download kept handler slot")
	}
	require.Empty(t, h.downloads)
}
