package handler

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/videoreference"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type VideoReferenceHandler struct {
	store           *videoreference.Store
	baseURL         string
	uploads         chan struct{}
	downloads       chan struct{}
	uploadTimeout   time.Duration
	downloadTimeout time.Duration
}

func referenceTimeout(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return 90 * time.Second
}

func NewVideoReferenceHandler() *VideoReferenceHandler {
	h := &VideoReferenceHandler{uploads: make(chan struct{}, 4), downloads: make(chan struct{}, 16)}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("VIDEO_REFERENCE_PUBLIC_BASE_URL")), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return h
	}
	dir := strings.TrimSpace(os.Getenv("VIDEO_REFERENCE_STORAGE_DIR"))
	if dir == "" {
		return h
	}
	if !filepath.IsAbs(dir) {
		return h
	}
	h.store, err = videoreference.New(dir)
	if err != nil {
		return h
	}
	h.baseURL = base
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_, done, allowed := managedinstance.Begin(context.Background())
			if allowed {
				h.store.Cleanup()
			}
			done()
		}
	}()
	return h
}

func (h *VideoReferenceHandler) Upload(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key == nil || key.UserID < 1 {
		c.JSON(401, gin.H{"error": gin.H{"message": "请使用本人视频分组的密钥"}})
		return
	}
	if key.Group == nil || key.Group.Platform != service.PlatformVideo {
		c.JSON(403, gin.H{"error": gin.H{"message": "请选择视频分组的密钥"}})
		return
	}
	if h.store == nil {
		c.JSON(503, gin.H{"error": gin.H{"message": "临时素材上传尚未启用，请使用公开图片或音频链接"}})
		return
	}
	select {
	case h.uploads <- struct{}{}:
		defer func() { <-h.uploads }()
	default:
		c.Header("Retry-After", "10")
		c.JSON(429, gin.H{"error": gin.H{"message": "素材上传繁忙，请稍后重试"}})
		return
	}
	controller := http.NewResponseController(c.Writer)
	timeout := referenceTimeout(h.uploadTimeout)
	if err := controller.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		c.JSON(503, gin.H{"error": gin.H{"message": "当前连接暂不支持素材上传，请稍后重试"}})
		return
	}
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, videoreference.MaxFileBytes+(64<<10))
	timer := time.AfterFunc(timeout, func() { _ = c.Request.Body.Close() })
	defer timer.Stop()
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(400, gin.H{"error": gin.H{"message": "请使用 multipart/form-data 上传 file 字段"}})
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		c.JSON(400, gin.H{"error": gin.H{"message": "每次请选择一个图片或音频文件"}})
		return
	}
	entry, err := h.store.Put(key.UserID, part)
	if err != nil {
		status, msg := 400, "素材上传失败，请检查文件后重试"
		if errors.Is(err, videoreference.ErrFormat) {
			msg = "支持 PNG/JPEG/GIF/WebP 图片及 MP3/WAV/OGG/FLAC/M4A 音频"
		}
		var maxErr *http.MaxBytesError
		if errors.Is(err, videoreference.ErrLimit) || errors.As(err, &maxErr) {
			status, msg = 413, "单个文件最多 25MB，每人临时素材最多 100MB；请稍后重试或使用公开链接"
		}
		c.JSON(status, gin.H{"error": gin.H{"message": msg}})
		return
	}
	if _, err := reader.NextPart(); err != io.EOF {
		_ = h.store.Delete(key.UserID, entry.ID)
		c.JSON(400, gin.H{"error": gin.H{"message": "每次只支持一个 file 字段"}})
		return
	}
	c.JSON(201, gin.H{"id": entry.ID, "url": h.baseURL + "/media/video-references/" + entry.ID, "expires_at": entry.ExpiresAt, "bytes": entry.Bytes, "mime_type": entry.MIME})
}

func (h *VideoReferenceHandler) Content(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Referrer-Policy", "no-referrer")
	if h.store == nil {
		c.Status(404)
		return
	}
	select {
	case h.downloads <- struct{}{}:
		defer func() { <-h.downloads }()
	default:
		c.Status(429)
		return
	}
	f, entry, err := h.store.Open(c.Param("id"))
	if err != nil {
		c.Status(404)
		return
	}
	defer f.Close()
	controller := http.NewResponseController(c.Writer)
	if err := controller.SetWriteDeadline(time.Now().Add(referenceTimeout(h.downloadTimeout))); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	c.Header("Content-Type", entry.MIME)
	c.Header("Content-Disposition", `attachment; filename="reference"`)
	http.ServeContent(c.Writer, c.Request, "reference", entry.ExpiresAt.Add(-videoreference.TTL), f)
}
