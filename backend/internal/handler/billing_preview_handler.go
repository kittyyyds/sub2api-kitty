package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BillingPreviewHandler struct {
	service *service.BillingPreviewService
}

func NewBillingPreviewHandler(svc *service.BillingPreviewService) *BillingPreviewHandler {
	return &BillingPreviewHandler{service: svc}
}

func (h *BillingPreviewHandler) Catalog(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	catalog, err := h.service.Catalog(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"groups": catalog})
}

func (h *BillingPreviewHandler) Preview(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var input service.BillingPreviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid billing preview request")
		return
	}
	result, err := h.service.Preview(c.Request.Context(), subject.UserID, input)
	if err != nil {
		if errors.Is(err, service.ErrBillingPreviewInvalidInput) ||
			errors.Is(err, service.ErrBillingPreviewSelectionDenied) ||
			errors.Is(err, service.ErrBillingPreviewPricingUnavailable) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
