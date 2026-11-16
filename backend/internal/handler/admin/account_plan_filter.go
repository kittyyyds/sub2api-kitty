package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) ApplyPlanTypeFilter(c *gin.Context) {
	ctx, err := service.WithAccountPlanFilter(c.Request.Context(), c.Query("plan_type"))
	if err != nil {
		response.ErrorFrom(c, err)
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}
