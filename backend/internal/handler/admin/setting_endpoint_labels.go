package admin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
)

// A missing field is an older client, while an explicit empty value clears it.
func resolveAPIBaseURLDescription(previous string, requested *string) (string, error) {
	if requested == nil {
		return previous, nil
	}
	description := strings.TrimSpace(*requested)
	if utf8.RuneCountInString(description) > 200 {
		return "", fmt.Errorf("API endpoint description is too long (max 200 characters)")
	}
	return description, nil
}

// Match by URL rather than index so reordering does not move recommendations.
// Older settings clients omit this optional flag; preserve it for the same URL.
// An explicit false still allows the administrator to remove a recommendation.
func preserveEndpointRecommendations(previous, requested []dto.CustomEndpoint) []dto.CustomEndpoint {
	result := append([]dto.CustomEndpoint{}, requested...)
	for i := range result {
		if result[i].Recommended != nil {
			continue
		}
		for _, old := range previous {
			if strings.TrimSpace(old.Endpoint) == strings.TrimSpace(result[i].Endpoint) {
				result[i].Recommended = old.Recommended
				break
			}
		}
	}
	return result
}
