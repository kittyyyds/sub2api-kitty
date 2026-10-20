package service

import (
	"context"
	"slices"
)

// Candidates are supplied by the authenticated media service's configured
// catalogue, not by a browser. Policy matching stays in SUB.
type MediaModelCandidate struct {
	Model string `json:"model"`
	Kind  string `json:"kind"`
}

type MediaPermissionGroup struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

func (s *APIKeyService) MediaPermissions(ctx context.Context, userID int64, candidates []MediaModelCandidate) ([]MediaPermissionGroup, error) {
	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	denied, err := s.GetUserGroupDeniedModels(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]MediaPermissionGroup, 0, len(groups))
	for _, group := range groups {
		item := MediaPermissionGroup{ID: group.ID, Name: group.Name, Models: []string{}}
		for _, candidate := range candidates {
			if candidate.Kind == "text" && !mediaTextModelAllowed(group, candidate.Model) {
				continue
			}
			if candidate.Kind == "image" && (group.Platform == PlatformGemini || group.Platform == PlatformAntigravity) && !group.AllowImageGeneration {
				continue
			}
			if group.ModelAllowlist.Allows(candidate.Model) && !modelMatchesGroupModelPatterns(denied[group.ID], candidate.Model) {
				item.Models = append(item.Models, candidate.Model)
			}
		}
		if len(item.Models) > 0 {
			result = append(result, item)
		}
	}
	return result, nil
}

// A disabled allowlist removes a policy restriction, not the platform's routing
// contract. Resolve model families with the same detector as the gateway; a
// group name, pricing entry or image-generation flag is not a capability grant.
func mediaTextModelAllowed(group Group, model string) bool {
	capability := CanvasCapabilities(model, group.Platform)
	if !capability.Supported || capability.MediaType != "text" {
		return false
	}
	platform, known := DetectModelPlatform(model)
	if !known {
		return false
	}
	if group.Platform != PlatformAntigravity {
		return group.Platform == platform
	}
	// Antigravity routes Claude and Gemini models. Empty scopes retain the
	// existing unrestricted setting; image-only scopes cannot grant text.
	var scope string
	switch platform {
	case PlatformAnthropic:
		scope = "claude"
	case PlatformGemini:
		scope = "gemini_text"
	default:
		return false
	}
	return len(group.SupportedModelScopes) == 0 || slices.Contains(group.SupportedModelScopes, scope)
}
