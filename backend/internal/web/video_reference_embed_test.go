//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVideoReferenceBypassesEmbeddedHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ServeEmbeddedFrontend())
	r.GET("/media/video-references/:id", func(c *gin.Context) { c.Data(200, "image/png", []byte("fixture-media")) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/media/video-references/fixture", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Body.String() != "fixture-media" { t.Fatalf("embedded frontend intercepted media: status=%d type=%s", w.Code, w.Header().Get("Content-Type")) }
}
