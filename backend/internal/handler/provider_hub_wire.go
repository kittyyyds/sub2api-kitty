package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func ProvideProviderHubChannelHandler(channels *service.ChannelService, billing *service.BillingService, pricing *service.PricingService, adminService service.AdminService) *admin.ChannelHandler {
	h := admin.NewChannelHandler(channels, billing, pricing)
	h.SetProviderHubAdmin(adminService)
	return h
}
