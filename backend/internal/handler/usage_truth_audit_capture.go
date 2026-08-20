package handler

import (
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	usageAuditResponseCaptureLimit = 512 * 1024
	usageAuditResponseHeadLimit    = 64 * 1024
)

// usageAuditCaptureWriter keeps a bounded response snapshot for token auditing.
// Large responses retain both the beginning and the tail so terminal SSE usage
// events remain available without retaining the generated content in memory.
type usageAuditCaptureWriter struct {
	gin.ResponseWriter

	mu         sync.Mutex
	head       []byte
	tail       []byte
	totalBytes int64
}

func beginUsageAuditCapture(c *gin.Context) *usageAuditCaptureWriter {
	if c == nil || c.Writer == nil {
		return nil
	}
	w := &usageAuditCaptureWriter{ResponseWriter: c.Writer}
	c.Writer = w
	return w
}

func (w *usageAuditCaptureWriter) Unwrap() http.ResponseWriter {
	if w == nil {
		return nil
	}
	return w.ResponseWriter
}

func (w *usageAuditCaptureWriter) Write(data []byte) (int, error) {
	if w == nil || w.ResponseWriter == nil {
		return 0, nil
	}
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.capture(data[:n])
	}
	return n, err
}

func (w *usageAuditCaptureWriter) WriteString(data string) (int, error) {
	if w == nil || w.ResponseWriter == nil {
		return 0, nil
	}
	n, err := w.ResponseWriter.WriteString(data)
	if n > 0 {
		w.capture([]byte(data[:n]))
	}
	return n, err
}

func (w *usageAuditCaptureWriter) capture(data []byte) {
	if w == nil || len(data) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	w.totalBytes += int64(len(data))
	if remaining := usageAuditResponseHeadLimit - len(w.head); remaining > 0 {
		take := min(remaining, len(data))
		w.head = append(w.head, data[:take]...)
		data = data[take:]
	}
	if len(data) == 0 {
		return
	}

	tailLimit := usageAuditResponseCaptureLimit - usageAuditResponseHeadLimit
	if len(data) >= tailLimit {
		w.tail = append(w.tail[:0], data[len(data)-tailLimit:]...)
		return
	}
	if overflow := len(w.tail) + len(data) - tailLimit; overflow > 0 {
		copy(w.tail, w.tail[overflow:])
		w.tail = w.tail[:len(w.tail)-overflow]
	}
	w.tail = append(w.tail, data...)
}

func (w *usageAuditCaptureWriter) snapshot() ([]byte, bool) {
	if w == nil {
		return nil, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	truncated := w.totalBytes > usageAuditResponseCaptureLimit
	extra := 0
	if truncated && len(w.head) > 0 && len(w.tail) > 0 {
		extra = 1
	}
	out := make([]byte, 0, len(w.head)+len(w.tail)+extra)
	out = append(out, w.head...)
	if extra > 0 {
		out = append(out, '\n')
	}
	out = append(out, w.tail...)
	return out, truncated
}

func populateClaudeUsageAudit(result *service.ForwardResult, endpoint, fallbackModel string, requestBody []byte, capture *usageAuditCaptureWriter) {
	if result == nil || result.UsageAudit != nil {
		return
	}
	responseBody, truncated := capture.snapshot()
	result.UsageAudit = service.BuildClaudeUsageAudit(
		endpoint,
		firstUsageAuditModel(result.UpstreamModel, result.Model, fallbackModel),
		requestBody,
		responseBody,
		truncated,
		result.Usage,
	)
}

func populateOpenAIUsageAudit(result *service.OpenAIForwardResult, endpoint, fallbackModel string, requestBody []byte, capture *usageAuditCaptureWriter) {
	if result == nil || result.UsageAudit != nil {
		return
	}
	responseBody, truncated := capture.snapshot()
	result.UsageAudit = service.BuildOpenAIUsageAudit(
		endpoint,
		firstUsageAuditModel(result.UpstreamModel, result.BillingModel, result.Model, fallbackModel),
		requestBody,
		responseBody,
		truncated,
		result.Usage,
	)
}

func firstUsageAuditModel(models ...string) string {
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			return model
		}
	}
	return ""
}
