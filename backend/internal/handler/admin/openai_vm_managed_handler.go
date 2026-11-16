package admin

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// OpenAIVMManagedHandler is the admin surface for accounts whose login and
// tokens live in a VM2 slot. Login bodies pass straight through to the bridge:
// they are never bound to a struct, logged, or returned.
type OpenAIVMManagedHandler struct {
	service *service.OpenAIVMManagedService
	batch   *service.OpenAIVMBatchService
	admin   service.AdminService
}

func NewOpenAIVMManagedHandler(s *service.OpenAIVMManagedService, batch *service.OpenAIVMBatchService, admin service.AdminService) *OpenAIVMManagedHandler {
	if s == nil {
		return nil
	}
	return &OpenAIVMManagedHandler{service: s, batch: batch, admin: admin}
}

// The pasted batch text holds one password and one 2FA secret per line.
const openAIVMManagedBatchBodyLimit = service.OpenAIVMBatchMaxInputBytes + 64<<10

type openAIVMBatchRequest struct {
	service.OpenAIVMBatchInput
	ConfirmMixedChannelRisk *bool `json:"confirm_mixed_channel_risk"`
}

func (h *OpenAIVMManagedHandler) batchInput(c *gin.Context) (service.OpenAIVMBatchInput, bool) {
	var in openAIVMBatchRequest
	if h.batch == nil {
		response.NotFound(c, "Batch import is not available")
		return in.OpenAIVMBatchInput, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, openAIVMManagedBatchBodyLimit)
	if c.ShouldBindJSON(&in) != nil {
		// Never echo the body: every line of it is a login.
		response.BadRequest(c, "Invalid batch import request")
		return in.OpenAIVMBatchInput, false
	}
	in.SkipMixedChannelCheck = in.ConfirmMixedChannelRisk != nil && *in.ConfirmMixedChannelRisk
	return in.OpenAIVMBatchInput, true
}

// BatchPreview reports what a batch would do. Nothing is created.
func (h *OpenAIVMManagedHandler) BatchPreview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	in, ok := h.batchInput(c)
	if !ok {
		return
	}
	preview, err := h.batch.Preview(c.Request.Context(), in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}

// BatchStart records the batch and begins working through it in the background.
func (h *OpenAIVMManagedHandler) BatchStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	in, ok := h.batchInput(c)
	if !ok {
		return
	}
	var createdBy int64
	if subject, found := middleware.GetAuthSubjectFromContext(c); found {
		createdBy = subject.UserID
	}
	job, err := h.batch.Start(c.Request.Context(), createdBy, in)
	if err != nil {
		var mixed *service.MixedChannelError
		if errors.As(err, &mixed) {
			c.JSON(http.StatusConflict, gin.H{"error": "mixed_channel_warning", "message": mixed.Error()})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func (h *OpenAIVMManagedHandler) batchID(c *gin.Context) (int64, bool) {
	if h.batch == nil {
		response.NotFound(c, "Batch import is not available")
		return 0, false
	}
	id, err := strconv.ParseInt(c.Param("batch_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid batch ID")
		return 0, false
	}
	return id, true
}

func (h *OpenAIVMManagedHandler) BatchStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	job, err := h.batch.Status(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func (h *OpenAIVMManagedHandler) BatchCancel(c *gin.Context) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	job, err := h.batch.Cancel(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

// OpenAIVMManagedOperations lists the relayed operations by route suffix. The
// bridge and the runtime hold the same list; anything not here is a 404. In
// particular nothing returns a saved password, 2FA secret or token.
var OpenAIVMManagedOperations = map[string]string{
	"POST login/oauth-url":       "login/oauth-url",
	"POST login/oauth-code":      "login/oauth-code",
	"POST login/password":        "login/password",
	"GET login/password":         "login/password",
	"POST login/password/code":   "login/password/code",
	"POST login/password/cancel": "login/password/cancel",
	"POST login/password/forget": "login/password/forget",
	"POST login/import":          "login/import",
	"POST token/refresh":         "token/refresh",
	"POST usage/reset-credit":    "usage/reset-credit",
	"POST power/on":              "power/on",
	"POST power/off":             "power/off",
	"GET diagnose":               "diagnose",
	"POST recover":               "recover",
	"POST exit-probe":            "exit-probe",
}

// An account file can be a whole Codex auth.json; everything else is small.
const (
	openAIVMManagedBodyLimit   = 64 << 10
	openAIVMManagedImportLimit = 256 << 10
)

func (h *OpenAIVMManagedHandler) Create(c *gin.Context) {
	var in struct {
		service.OpenAIVMManagedCreateInput
		ConfirmMixedChannelRisk *bool `json:"confirm_mixed_channel_risk"`
	}
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "Invalid managed account request")
		return
	}
	input := in.OpenAIVMManagedCreateInput
	input.SkipMixedChannelCheck = in.ConfirmMixedChannelRisk != nil && *in.ConfirmMixedChannelRisk
	account, view, err := h.service.Create(c.Request.Context(), input)
	if err != nil && account == nil {
		var mixed *service.MixedChannelError
		if errors.As(err, &mixed) {
			c.JSON(http.StatusConflict, gin.H{"error": "mixed_channel_warning", "message": mixed.Error()})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	out := gin.H{"account_id": account.ID, "name": account.Name, "managed": view}
	if err != nil {
		// The account exists but its slot does not yet; the panel offers Retry.
		out["slot_error"] = service.OpenAIVMManagedErrorCode(err)
	}
	response.Success(c, out)
}

func (h *OpenAIVMManagedHandler) Status(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	view, err := h.service.Status(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *OpenAIVMManagedHandler) Retry(c *gin.Context) {
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	view, err := h.service.Retry(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *OpenAIVMManagedHandler) ChangeProxy(c *gin.Context) {
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	var in struct {
		ProxyID int64 `json:"proxy_id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ProxyID <= 0 {
		response.BadRequest(c, "A proxy is required")
		return
	}
	view, err := h.service.ChangeProxy(c.Request.Context(), id, in.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *OpenAIVMManagedHandler) Delete(c *gin.Context) {
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Managed account and its slot were deleted"})
}

func (h *OpenAIVMManagedHandler) RefreshUsage(c *gin.Context) {
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	if _, err := h.service.RefreshUsage(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.Status(c)
}

// SetSchedulable is the administrator's switch: it sets the account's own
// scheduling flag, which the state sync never turns back on by itself.
func (h *OpenAIVMManagedHandler) SetSchedulable(c *gin.Context) {
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	var in struct {
		Schedulable *bool `json:"schedulable"`
	}
	if c.ShouldBindJSON(&in) != nil || in.Schedulable == nil {
		response.BadRequest(c, "schedulable is required")
		return
	}
	if err := h.service.SetSchedulable(c.Request.Context(), id, *in.Schedulable); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.Status(c)
}

// Operation relays one whitelisted operation to the account's slot.
func (h *OpenAIVMManagedHandler) Operation(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := openAIVMAccountParam(c)
	if !ok {
		return
	}
	suffix := c.Param("operation")
	for len(suffix) > 0 && suffix[0] == '/' {
		suffix = suffix[1:]
	}
	operation, known := OpenAIVMManagedOperations[c.Request.Method+" "+suffix]
	if !known {
		response.NotFound(c, "Unknown managed account operation")
		return
	}
	var body []byte
	if c.Request.Method != http.MethodGet {
		limit := int64(openAIVMManagedBodyLimit)
		if operation == "login/import" {
			limit = openAIVMManagedImportLimit
		}
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
		if err != nil {
			response.BadRequest(c, "The request is too large or unreadable")
			return
		}
	}
	out, err := h.service.Call(c.Request.Context(), id, c.Request.Method, operation, body)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}
