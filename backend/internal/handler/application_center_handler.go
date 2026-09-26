package handler

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ApplicationCenterHandler struct {
	service *service.ApplicationCenterService
}

func NewApplicationCenterHandler(applicationService *service.ApplicationCenterService) *ApplicationCenterHandler {
	return &ApplicationCenterHandler{service: applicationService}
}

type startApplicationRunRequest struct {
	PlannerAPIKeyID *int64          `json:"planner_api_key_id" binding:"omitempty,min=1"`
	MediaAPIKeyID   int64           `json:"media_api_key_id" binding:"required,min=1"`
	Input           json.RawMessage `json:"input" binding:"required"`
}

type completeApplicationStepRequest struct {
	Output           json.RawMessage `json:"output" binding:"required"`
	GatewayRequestID string          `json:"gateway_request_id"`
}

type failApplicationStepRequest struct {
	Error            string          `json:"error" binding:"required"`
	Output           json.RawMessage `json:"output"`
	GatewayRequestID string          `json:"gateway_request_id"`
}

type checkpointApplicationStepRequest struct {
	Output           json.RawMessage `json:"output"`
	GatewayRequestID string          `json:"gateway_request_id" binding:"required"`
}

func (h *ApplicationCenterHandler) ListApplications(c *gin.Context) {
	response.Success(c, gin.H{"items": h.service.ListApplications()})
}

func (h *ApplicationCenterHandler) ListRuns(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	runs, result, err := h.service.ListRuns(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc",
	}, strings.TrimSpace(c.Query("app_slug")))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, runs, result.Total, result.Page, result.PageSize)
}

func (h *ApplicationCenterHandler) GetRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	run, err := h.service.GetRun(c.Request.Context(), subject.UserID, c.Param("run_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ApplicationCenterHandler) StartRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req startApplicationRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	run, err := h.service.StartRun(c.Request.Context(), subject.UserID, c.Param("slug"), service.StartApplicationRunRequest{
		PlannerAPIKeyID: req.PlannerAPIKeyID,
		MediaAPIKeyID:   req.MediaAPIKeyID,
		Input:           req.Input,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, run)
}

func (h *ApplicationCenterHandler) CompleteStep(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req completeApplicationStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	run, err := h.service.CompleteStep(
		c.Request.Context(),
		subject.UserID,
		c.Param("run_id"),
		c.Param("step_key"),
		service.CompleteApplicationStepRequest{Output: req.Output, GatewayRequestID: req.GatewayRequestID},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ApplicationCenterHandler) FailStep(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req failApplicationStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	run, err := h.service.FailStep(
		c.Request.Context(),
		subject.UserID,
		c.Param("run_id"),
		c.Param("step_key"),
		service.FailApplicationStepRequest{
			Error: req.Error, Output: req.Output, GatewayRequestID: req.GatewayRequestID,
		},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ApplicationCenterHandler) CheckpointStep(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req checkpointApplicationStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	run, err := h.service.CheckpointStep(
		c.Request.Context(),
		subject.UserID,
		c.Param("run_id"),
		c.Param("step_key"),
		service.CheckpointApplicationStepRequest{
			Output: req.Output, GatewayRequestID: req.GatewayRequestID,
		},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ApplicationCenterHandler) CancelRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	run, err := h.service.CancelRun(c.Request.Context(), subject.UserID, c.Param("run_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *ApplicationCenterHandler) RetryRun(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	run, err := h.service.RetryRun(c.Request.Context(), subject.UserID, c.Param("run_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}
