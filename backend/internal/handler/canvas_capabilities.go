package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func canvasModelCapabilities(modelID, platform string) any {
	return service.CanvasCapabilities(modelID, platform)
}
