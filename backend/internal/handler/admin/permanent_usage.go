package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *ChannelHandler) PermanentUsage(c *gin.Context) {
	out, err := h.channelService.PermanentUsage(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Permanent usage unavailable")
		return
	}
	response.Success(c, out)
}
func (h *ChannelHandler) BackfillPermanentUsage(c *gin.Context) {
	n, err := h.channelService.BackfillPermanentUsage(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Permanent usage backfill failed; retry the same batch safely")
		return
	}
	response.Success(c, gin.H{"processed": n})
}
