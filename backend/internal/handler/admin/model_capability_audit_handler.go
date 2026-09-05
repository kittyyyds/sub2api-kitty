package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelCapabilityAuditHandler exposes the durable, aggregate model audit. The
// legacy single-account probe remains on AccountHandler for compatibility.
type ModelCapabilityAuditHandler struct {
	auditService *service.ModelCapabilityAuditService
}

func NewModelCapabilityAuditHandler(auditService *service.ModelCapabilityAuditService) *ModelCapabilityAuditHandler {
	return &ModelCapabilityAuditHandler{auditService: auditService}
}

type modelCapabilityAuditProbeRequest struct {
	AccountIDs []int64  `json:"account_ids"`
	ModelIDs   []string `json:"model_ids"`
	Protocols  []string `json:"protocols"`
	ActiveOnly *bool    `json:"active_only"`
	MaxItems   int      `json:"max_items"`
}

func (h *ModelCapabilityAuditHandler) unavailable(c *gin.Context) bool {
	if h == nil || h.auditService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Model capability audit service unavailable")
		return true
	}
	return false
}

// Probe runs a bounded batch and persists every protocol result.
func (h *ModelCapabilityAuditHandler) Probe(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req modelCapabilityAuditProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	activeOnly := true
	if req.ActiveOnly != nil {
		activeOnly = *req.ActiveOnly
	}
	run, err := h.auditService.Run(c.Request.Context(), service.ModelCapabilityAuditOptions{
		AccountIDs: req.AccountIDs, ModelIDs: req.ModelIDs, Protocols: req.Protocols,
		ActiveOnly: activeOnly, MaxItems: req.MaxItems,
	}, nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

// Summary combines passive usage-log evidence and latest active results.
func (h *ModelCapabilityAuditHandler) Summary(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	days, limit := 30, 200
	if raw := c.Query("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	result, err := h.auditService.Summary(c.Request.Context(), days, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Runs returns recent durable probe runs.
func (h *ModelCapabilityAuditHandler) Runs(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	result, err := h.auditService.ListRuns(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Run returns one run and its redacted protocol evidence.
func (h *ModelCapabilityAuditHandler) Run(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid run id")
		return
	}
	run, evidence, err := h.auditService.GetRun(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"run": run, "evidence": evidence})
}
