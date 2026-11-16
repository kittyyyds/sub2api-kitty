package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	UsageAuditVersion = 2

	UsageAuditProtocolAnthropicMessages = "anthropic_messages"
	UsageAuditProtocolOpenAIResponses   = "openai_responses"
	UsageAuditProtocolOpenAIChat        = "openai_chat_completions"
	UsageAuditProtocolUnknown           = "unknown"
)

type UsageAuditStatus string

const (
	UsageAuditStatusPass          UsageAuditStatus = "pass"
	UsageAuditStatusWarning       UsageAuditStatus = "warning"
	UsageAuditStatusAnomaly       UsageAuditStatus = "anomaly"
	UsageAuditStatusUnverifiable  UsageAuditStatus = "unverifiable"
	UsageAuditStatusNotApplicable UsageAuditStatus = "not_applicable"
	UsageAuditFilterMissing                        = "missing"
)

type UsageAuditConfidence string

const (
	UsageAuditConfidenceHigh   UsageAuditConfidence = "high"
	UsageAuditConfidenceMedium UsageAuditConfidence = "medium"
	UsageAuditConfidenceLow    UsageAuditConfidence = "low"
)

type UsageAuditEvidenceSource string

const (
	UsageAuditEvidenceUpstreamRaw UsageAuditEvidenceSource = "upstream_raw"
	UsageAuditEvidenceGateway     UsageAuditEvidenceSource = "gateway_response"
	UsageAuditEvidenceNormalized  UsageAuditEvidenceSource = "normalized_only"
)

type UsageAuditTokenSnapshot struct {
	InputTokens         *int `json:"input_tokens,omitempty"`
	TotalInputTokens    *int `json:"total_input_tokens,omitempty"`
	OutputTokens        *int `json:"output_tokens,omitempty"`
	CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     *int `json:"cache_read_tokens,omitempty"`
}

type UsageAuditTokenDelta struct {
	InputTokens  *int `json:"input_tokens,omitempty"`
	OutputTokens *int `json:"output_tokens,omitempty"`
}

type UsageAuditDeviation struct {
	InputTokens  *float64 `json:"input_tokens,omitempty"`
	OutputTokens *float64 `json:"output_tokens,omitempty"`
}

// UsageAudit contains only token evidence and derived metadata. Request and
// response content are intentionally excluded from the persisted structure.
type UsageAudit struct {
	ImageOutputEvidence *ImageOutputEvidence     `json:"image_output_evidence,omitempty"`
	Version             int                      `json:"version"`
	Status              UsageAuditStatus         `json:"status"`
	Confidence          UsageAuditConfidence     `json:"confidence"`
	EvidenceSource      UsageAuditEvidenceSource `json:"evidence_source"`
	Protocol            string                   `json:"protocol"`
	Estimator           string                   `json:"estimator,omitempty"`
	Upstream            UsageAuditTokenSnapshot  `json:"upstream"`
	Local               UsageAuditTokenSnapshot  `json:"local"`
	Delta               UsageAuditTokenDelta     `json:"delta"`
	DeviationPct        UsageAuditDeviation      `json:"deviation_pct"`
	CacheHitRate        *float64                 `json:"cache_hit_rate,omitempty"`
	Reasons             []string                 `json:"reasons"`
	UpstreamRawUsage    json.RawMessage          `json:"upstream_raw_usage,omitempty"`
	CaptureTruncated    bool                     `json:"capture_truncated"`
}

// UsageAuditBuildInput retains its previous input shape for compatibility.
// Model, RequestBody, ResponseBody and CaptureTruncated are deliberately ignored:
// ordinary generation audits must never reparse or tokenize request/response bodies.
type UsageAuditBuildInput struct {
	Protocol         string
	Model            string
	RequestBody      []byte
	ResponseBody     []byte
	CaptureTruncated bool
	Upstream         UsageAuditTokenSnapshot
}

type UsageAuditStatusCounts struct {
	Pass          int64 `json:"pass"`
	Warning       int64 `json:"warning"`
	Anomaly       int64 `json:"anomaly"`
	Unverifiable  int64 `json:"unverifiable"`
	NotApplicable int64 `json:"not_applicable"`
	Unknown       int64 `json:"unknown"`
	Missing       int64 `json:"missing"`
}

type UsageAuditDimensionStats struct {
	Key                       string                 `json:"key"`
	Name                      string                 `json:"name"`
	Total                     int64                  `json:"total"`
	Counts                    UsageAuditStatusCounts `json:"counts"`
	AverageInputDeviationPct  *float64               `json:"average_input_deviation_pct,omitempty"`
	MaximumInputDeviationPct  *float64               `json:"maximum_input_deviation_pct,omitempty"`
	AverageOutputDeviationPct *float64               `json:"average_output_deviation_pct,omitempty"`
	MaximumOutputDeviationPct *float64               `json:"maximum_output_deviation_pct,omitempty"`
}

type UsageAuditStats struct {
	TotalRecords              int64                      `json:"total_records"`
	AuditedRecords            int64                      `json:"audited_records"`
	CoveragePct               float64                    `json:"coverage_pct"`
	Counts                    UsageAuditStatusCounts     `json:"counts"`
	AverageInputDeviationPct  *float64                   `json:"average_input_deviation_pct,omitempty"`
	MaximumInputDeviationPct  *float64                   `json:"maximum_input_deviation_pct,omitempty"`
	AverageOutputDeviationPct *float64                   `json:"average_output_deviation_pct,omitempty"`
	MaximumOutputDeviationPct *float64                   `json:"maximum_output_deviation_pct,omitempty"`
	ByPlatform                []UsageAuditDimensionStats `json:"by_platform"`
	ByAccount                 []UsageAuditDimensionStats `json:"by_account"`
	ByModel                   []UsageAuditDimensionStats `json:"by_model"`
}

// UsageAuditRepository is deliberately narrow so existing billing test doubles
// do not need to implement audit-only reporting methods.
type UsageAuditRepository interface {
	GetUsageAuditStats(ctx context.Context, filters usagestats.UsageLogFilters) (*UsageAuditStats, error)
}

func BuildClaudeUsageAudit(endpoint string, usage ClaudeUsage) *UsageAudit {
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	cacheCreationTokens := usage.CacheCreationInputTokens
	cacheReadTokens := usage.CacheReadInputTokens
	totalInputTokens := inputTokens + cacheCreationTokens + cacheReadTokens
	return BuildUsageAudit(UsageAuditBuildInput{
		Protocol: UsageAuditProtocolForEndpoint(endpoint),
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:         usageAuditIntPtr(inputTokens),
			TotalInputTokens:    usageAuditIntPtr(totalInputTokens),
			OutputTokens:        usageAuditIntPtr(outputTokens),
			CacheCreationTokens: usageAuditIntPtr(cacheCreationTokens),
			CacheReadTokens:     usageAuditIntPtr(cacheReadTokens),
		},
	})
}

func BuildOpenAIUsageAudit(endpoint string, usage OpenAIUsage) *UsageAudit {
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	cacheCreationTokens := usage.CacheCreationInputTokens
	cacheReadTokens := usage.CacheReadInputTokens
	return BuildUsageAudit(UsageAuditBuildInput{
		Protocol: UsageAuditProtocolForEndpoint(endpoint),
		Upstream: UsageAuditTokenSnapshot{
			InputTokens:         usageAuditIntPtr(inputTokens),
			TotalInputTokens:    usageAuditIntPtr(inputTokens),
			OutputTokens:        usageAuditIntPtr(outputTokens),
			CacheCreationTokens: usageAuditIntPtr(cacheCreationTokens),
			CacheReadTokens:     usageAuditIntPtr(cacheReadTokens),
		},
	})
}

func NewUnverifiableUsageAudit(protocol string, upstream UsageAuditTokenSnapshot, reason string) *UsageAudit {
	if strings.TrimSpace(protocol) == "" {
		protocol = UsageAuditProtocolUnknown
	}
	reasons := []string{reason}
	if strings.TrimSpace(reason) == "" {
		reasons = []string{"capture_unavailable"}
	}
	return &UsageAudit{
		Version:        UsageAuditVersion,
		Status:         UsageAuditStatusUnverifiable,
		Confidence:     UsageAuditConfidenceLow,
		EvidenceSource: UsageAuditEvidenceNormalized,
		Protocol:       protocol,
		Upstream:       upstream,
		Reasons:        reasons,
	}
}

func NewNotApplicableUsageAudit(reason string) *UsageAudit {
	if strings.TrimSpace(reason) == "" {
		reason = "non_token_billing"
	}
	return &UsageAudit{
		Version:        UsageAuditVersion,
		Status:         UsageAuditStatusNotApplicable,
		Confidence:     UsageAuditConfidenceHigh,
		EvidenceSource: UsageAuditEvidenceNormalized,
		Protocol:       UsageAuditProtocolUnknown,
		Reasons:        []string{reason},
	}
}

func UsageAuditProtocolForEndpoint(endpoint string) string {
	normalized := strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.Contains(normalized, "chat/completions"):
		return UsageAuditProtocolOpenAIChat
	case strings.Contains(normalized, "responses"):
		return UsageAuditProtocolOpenAIResponses
	case strings.Contains(normalized, "messages"):
		return UsageAuditProtocolAnthropicMessages
	default:
		return UsageAuditProtocolUnknown
	}
}

// BuildUsageAudit records the upstream counters already parsed by forwarding. It
// does not independently verify them: local token estimation and audit-only
// response capture are disabled to avoid duplicating large generation payloads.
func BuildUsageAudit(input UsageAuditBuildInput) *UsageAudit {
	audit := NewUnverifiableUsageAudit(input.Protocol, input.Upstream, "local_estimate_disabled")
	if input.Upstream.TotalInputTokens != nil && *input.Upstream.TotalInputTokens > 0 && input.Upstream.CacheReadTokens != nil {
		hitRate := float64(*input.Upstream.CacheReadTokens) / float64(*input.Upstream.TotalInputTokens) * 100
		audit.CacheHitRate = roundedFloatPtr(hitRate)
	}
	return audit
}

func roundedFloatPtr(value float64) *float64 {
	rounded := math.Round(value*100) / 100
	return &rounded
}

func usageAuditIntPtr(value int) *int {
	return &value
}
