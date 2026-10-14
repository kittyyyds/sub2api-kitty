package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

// Dedicated video groups expose video pricing rather than an image toggle.
// Their normal group/key status, endpoint, model and price guards still apply.
func grokMediaGenerationAllowed(group *service.Group, endpoint service.GrokMediaEndpoint) bool {
	if group != nil && group.Platform == service.PlatformVideo {
		return endpoint == service.GrokMediaEndpointVideosGenerations
	}
	return service.GroupAllowsImageGeneration(group)
}
