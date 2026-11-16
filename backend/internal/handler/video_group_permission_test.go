package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDedicatedVideoGroupDoesNotDependOnHiddenImageSwitch(t *testing.T) {
	for _, imageSwitch := range []bool{false, true} {
		video := &service.Group{Platform: service.PlatformVideo, AllowImageGeneration: imageSwitch}
		require.True(t, grokMediaGenerationAllowed(video, service.GrokMediaEndpointVideosGenerations))
		require.False(t, grokMediaGenerationAllowed(video, service.GrokMediaEndpointImagesGenerations))
		require.False(t, grokMediaGenerationAllowed(video, service.GrokMediaEndpointImagesEdits))
		require.False(t, grokMediaGenerationAllowed(video, service.GrokMediaEndpointVideosExtensions))
		grok := &service.Group{Platform: service.PlatformGrok, AllowImageGeneration: imageSwitch}
		require.Equal(t, imageSwitch, grokMediaGenerationAllowed(grok, service.GrokMediaEndpointVideosGenerations))
		require.Equal(t, imageSwitch, grokMediaGenerationAllowed(grok, service.GrokMediaEndpointImagesGenerations))
	}
}
