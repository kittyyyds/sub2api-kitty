package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
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
	AccountIDs   []int64  `json:"account_ids"`
	ModelIDs     []string `json:"model_ids"`
	Protocols    []string `json:"protocols"`
	ActiveOnly   *bool    `json:"active_only"`
	MaxItems     int      `json:"max_items"`
	PreviewToken string   `json:"preview_token"`
}

type modelBehaviorAuditRequest struct {
	TargetAccountIDs []int64 `json:"target_account_ids"`
	ModelID          string  `json:"model_id"`
	CaseCount        int     `json:"case_count"`
	ReasoningEffort  string  `json:"reasoning_effort"`
	PreviewToken     string  `json:"preview_token"`
}

func (req modelBehaviorAuditRequest) options() service.ModelBehaviorAuditOptions {
	return service.ModelBehaviorAuditOptions{
		TargetAccountIDs: req.TargetAccountIDs,
		ModelID:          req.ModelID,
		CaseCount:        req.CaseCount,
		ReasoningEffort:  req.ReasoningEffort,
		PreviewToken:     req.PreviewToken,
	}
}

func (req modelCapabilityAuditProbeRequest) options() service.ModelCapabilityAuditOptions {
	activeOnly := true
	if req.ActiveOnly != nil {
		activeOnly = *req.ActiveOnly
	}
	return service.ModelCapabilityAuditOptions{
		AccountIDs: req.AccountIDs, ModelIDs: req.ModelIDs, Protocols: req.Protocols,
		ActiveOnly: activeOnly, MaxItems: req.MaxItems, PreviewToken: req.PreviewToken,
	}
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
	var requestedBy *int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		id := subject.UserID
		requestedBy = &id
	}
	run, err := h.auditService.Start(c.Request.Context(), req.options(), requestedBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

// Estimate returns the exact bounded target/request count without contacting
// any upstream. The UI uses it for an explicit cost confirmation.
func (h *ModelCapabilityAuditHandler) Estimate(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req modelCapabilityAuditProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	estimate, err := h.auditService.Estimate(c.Request.Context(), req.options())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, estimate)
}

func (h *ModelCapabilityAuditHandler) EstimateBehavior(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req modelBehaviorAuditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	estimate, err := h.auditService.EstimateBehavior(c.Request.Context(), req.options())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, estimate)
}

func (h *ModelCapabilityAuditHandler) ProbeBehavior(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req modelBehaviorAuditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var requestedBy *int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		id := subject.UserID
		requestedBy = &id
	}
	run, err := h.auditService.StartBehavior(c.Request.Context(), req.options(), requestedBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ModelCapabilityAuditHandler) BehaviorRuns(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	runs, err := h.auditService.ListBehaviorRuns(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, runs)
}

func (h *ModelCapabilityAuditHandler) BehaviorRun(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid run id")
		return
	}
	detail, err := h.auditService.GetBehaviorRun(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
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
