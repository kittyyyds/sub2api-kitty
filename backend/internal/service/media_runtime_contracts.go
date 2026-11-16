package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

// A durable video owner lookup must survive Redis eviction.
type GrokMediaVideoOwnerAccountRepository interface {
	FindGrokMediaVideoOwnerAccountID(
		ctx context.Context,
		requestID string,
		userID, apiKeyID int64,
		groupID *int64,
	) (int64, error)
}

var grokImageAspectRatios = []string{
	"1:1",
	"16:9",
	"9:16",
	"4:3",
	"3:4",
	"3:2",
	"2:3",
	"2:1",
	"1:2",
	"19.5:9",
	"9:19.5",
	"20:9",
	"9:20",
	"auto",
}

func normalizeGrokImageAspectRatio(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, ratio := range grokImageAspectRatios {
		if ratio == value {
			return value, true
		}
	}
	return value, false
}

type grokContentPolicyError struct{ message string }

func (e *grokContentPolicyError) Error() string { return "grok content policy rejection: " + e.message }
func opsNumber(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	case json.Number:
		var err error
		n, err = x.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func opsFresh(observed, until *time.Time, now time.Time) bool {
	return observed != nil && until != nil && !observed.IsZero() && !observed.After(now) && until.After(*observed) && until.After(now)
}
