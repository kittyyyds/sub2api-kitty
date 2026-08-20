package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	aiIntelligenceDataURL       = "https://codexradar.com/data/intelligence-efficiency.json"
	aiIntelligenceComponentsURL = "https://status.openai.com/api/v2/components.json"
	aiIntelligenceIncidentsURL  = "https://status.openai.com/api/v2/incidents.json"
	aiIntelligenceCacheTTL      = 5 * time.Minute
	aiIntelligenceFetchTimeout  = 12 * time.Second
	aiIntelligenceMaxBodyBytes  = 2 << 20
	aiIntelligenceWindowDays    = 15
)

type AIIntelligencePoint struct {
	Model               string  `json:"model"`
	Effort              string  `json:"effort"`
	IQ                  float64 `json:"iq"`
	Passed              float64 `json:"passed"`
	ValidTasks          float64 `json:"valid_tasks"`
	AveragePriceUSD     float64 `json:"average_price_usd"`
	AverageMinutes      float64 `json:"average_minutes"`
	LatestGradedAt      string  `json:"latest_graded_at"`
	RecommendationScore float64 `json:"recommendation_score"`
}

type AIServiceComponent struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

type AIServiceDay struct {
	Date   string `json:"date"`
	Status string `json:"status"`
}

type AIServiceIncident struct {
	Title      string `json:"title"`
	Status     string `json:"status"`
	OccurredAt string `json:"occurred_at"`
}

type AIServiceStatus struct {
	OverallStatus  string               `json:"overall_status"`
	Components     []AIServiceComponent `json:"components"`
	Days           []AIServiceDay       `json:"days"`
	EventFreeDays  int                  `json:"event_free_days"`
	WindowDays     int                  `json:"window_days"`
	RecentIncident *AIServiceIncident   `json:"recent_incident,omitempty"`
}

type AIIntelligencePayload struct {
	Points          []AIIntelligencePoint `json:"points"`
	ServiceStatus   AIServiceStatus       `json:"service_status"`
	FetchedAt       string                `json:"fetched_at"`
	SourceUpdatedAt string                `json:"source_updated_at"`
	Stale           bool                  `json:"stale"`
}

type intelligenceUpstreamResponse struct {
	Schema int                         `json:"schema"`
	Type   string                      `json:"type"`
	Points []intelligenceUpstreamPoint `json:"points"`
}

type intelligenceUpstreamPoint struct {
	Model           string  `json:"model"`
	Effort          string  `json:"effort"`
	IQ              float64 `json:"iq"`
	Passed          float64 `json:"passed"`
	ValidTasks      float64 `json:"valid_tasks"`
	AveragePriceUSD float64 `json:"average_price_usd"`
	AverageMinutes  float64 `json:"average_minutes"`
	LatestGradedAt  string  `json:"latest_graded_at"`
}

type statusComponentsResponse struct {
	Components []statusComponent `json:"components"`
}

type statusComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type statusIncidentsResponse struct {
	Incidents []statusIncident `json:"incidents"`
}

type statusIncident struct {
	Name            string                 `json:"name"`
	Status          string                 `json:"status"`
	Impact          string                 `json:"impact"`
	CreatedAt       string                 `json:"created_at"`
	ResolvedAt      string                 `json:"resolved_at"`
	IncidentUpdates []statusIncidentUpdate `json:"incident_updates"`
}

type statusIncidentUpdate struct {
	Body string `json:"body"`
}

type aiIntelligenceEndpoints struct {
	data       string
	components string
	incidents  string
}

type AIIntelligenceHandler struct {
	client    *http.Client
	endpoints aiIntelligenceEndpoints
	cacheTTL  time.Duration
	now       func() time.Time

	mu        sync.RWMutex
	cached    *AIIntelligencePayload
	expiresAt time.Time
	refresh   singleflight.Group
}

func NewAIIntelligenceHandler() *AIIntelligenceHandler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 60 * time.Second
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 8 * time.Second

	return newAIIntelligenceHandler(
		&http.Client{Transport: transport, Timeout: aiIntelligenceFetchTimeout},
		aiIntelligenceEndpoints{
			data:       aiIntelligenceDataURL,
			components: aiIntelligenceComponentsURL,
			incidents:  aiIntelligenceIncidentsURL,
		},
		aiIntelligenceCacheTTL,
		time.Now,
	)
}

func newAIIntelligenceHandler(client *http.Client, endpoints aiIntelligenceEndpoints, cacheTTL time.Duration, now func() time.Time) *AIIntelligenceHandler {
	return &AIIntelligenceHandler{
		client:    client,
		endpoints: endpoints,
		cacheTTL:  cacheTTL,
		now:       now,
	}
}

// Get serves a compact, same-origin snapshot used by the AI Intelligence page.
func (h *AIIntelligenceHandler) Get(c *gin.Context) {
	payload, err := h.getSnapshot()
	if err != nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable(
			"AI_INTELLIGENCE_UPSTREAM_UNAVAILABLE",
			"AI intelligence data is temporarily unavailable",
		).WithCause(err))
		return
	}

	c.Header("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	response.Success(c, payload)
}

func (h *AIIntelligenceHandler) getSnapshot() (*AIIntelligencePayload, error) {
	now := h.now().UTC()
	if cached := h.readFresh(now); cached != nil {
		return cached, nil
	}

	value, err, _ := h.refresh.Do("snapshot", func() (any, error) {
		now = h.now().UTC()
		if cached := h.readFresh(now); cached != nil {
			return cached, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), aiIntelligenceFetchTimeout)
		defer cancel()

		payload, refreshErr := h.fetchSnapshot(ctx, now)
		if refreshErr != nil {
			if stale := h.readCached(); stale != nil {
				stale.Stale = true
				return stale, nil
			}
			return nil, refreshErr
		}

		h.mu.Lock()
		h.cached = cloneAIIntelligencePayload(payload)
		h.expiresAt = now.Add(h.cacheTTL)
		h.mu.Unlock()
		return cloneAIIntelligencePayload(payload), nil
	})
	if err != nil {
		return nil, err
	}
	payload, ok := value.(*AIIntelligencePayload)
	if !ok || payload == nil {
		return nil, errors.New("AI intelligence cache returned an invalid payload")
	}
	return payload, nil
}

func (h *AIIntelligenceHandler) readFresh(now time.Time) *AIIntelligencePayload {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cached == nil || !now.Before(h.expiresAt) {
		return nil
	}
	return cloneAIIntelligencePayload(h.cached)
}

func (h *AIIntelligenceHandler) readCached() *AIIntelligencePayload {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneAIIntelligencePayload(h.cached)
}

func (h *AIIntelligenceHandler) fetchSnapshot(ctx context.Context, now time.Time) (*AIIntelligencePayload, error) {
	var intelligence intelligenceUpstreamResponse
	var components statusComponentsResponse
	var incidents statusIncidentsResponse

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return fetchAIJSON(groupCtx, h.client, h.endpoints.data, &intelligence) })
	group.Go(func() error { return fetchAIJSON(groupCtx, h.client, h.endpoints.components, &components) })
	group.Go(func() error { return fetchAIJSON(groupCtx, h.client, h.endpoints.incidents, &incidents) })
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if intelligence.Schema != 2 || intelligence.Type != "distributed_intelligence_efficiency" {
		return nil, fmt.Errorf("unsupported AI intelligence schema: schema=%d type=%q", intelligence.Schema, intelligence.Type)
	}

	points, sourceUpdatedAt := normalizeAIIntelligencePoints(intelligence.Points)
	if len(points) == 0 {
		return nil, errors.New("AI intelligence source did not contain supported GPT points")
	}

	return &AIIntelligencePayload{
		Points:          points,
		ServiceStatus:   buildAIServiceStatus(components.Components, incidents.Incidents, now),
		FetchedAt:       now.Format(time.RFC3339),
		SourceUpdatedAt: sourceUpdatedAt,
		Stale:           false,
	}, nil
}

func fetchAIJSON(ctx context.Context, client *http.Client, target string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "KittyAPI-AI-Intelligence/1.0")

	result, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", target, err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: upstream returned HTTP %d", target, result.StatusCode)
	}

	limited := io.LimitReader(result.Body, aiIntelligenceMaxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	if len(data) > aiIntelligenceMaxBodyBytes {
		return fmt.Errorf("fetch %s: response exceeded %d bytes", target, aiIntelligenceMaxBodyBytes)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode %s: %w", target, err)
	}
	return nil
}

func normalizeAIIntelligencePoints(source []intelligenceUpstreamPoint) ([]AIIntelligencePoint, string) {
	points := make([]AIIntelligencePoint, 0, len(source))
	var latest time.Time
	var latestText string
	for _, point := range source {
		model := strings.TrimSpace(point.Model)
		effort := strings.ToLower(strings.TrimSpace(point.Effort))
		if !strings.HasPrefix(strings.ToLower(model), "gpt-") || strings.EqualFold(model, "gpt-5.6-luna") || effort == "ultra" {
			continue
		}
		if model == "" || effort == "" || point.IQ <= 0 || point.AverageMinutes <= 0 || point.ValidTasks <= 0 {
			continue
		}

		points = append(points, AIIntelligencePoint{
			Model:               model,
			Effort:              effort,
			IQ:                  point.IQ,
			Passed:              point.Passed,
			ValidTasks:          point.ValidTasks,
			AveragePriceUSD:     point.AveragePriceUSD,
			AverageMinutes:      point.AverageMinutes,
			LatestGradedAt:      point.LatestGradedAt,
			RecommendationScore: point.IQ - 1.5*point.AverageMinutes,
		})

		if parsed, err := time.Parse(time.RFC3339, point.LatestGradedAt); err == nil && parsed.After(latest) {
			latest = parsed
			latestText = point.LatestGradedAt
		}
	}

	sort.SliceStable(points, func(i, j int) bool {
		return points[i].RecommendationScore > points[j].RecommendationScore
	})
	return points, latestText
}

func buildAIServiceStatus(components []statusComponent, incidents []statusIncident, now time.Time) AIServiceStatus {
	componentSpecs := []struct {
		key        string
		candidates []string
	}{
		{key: "api", candidates: []string{"Codex API", "Responses", "API"}},
		{key: "web", candidates: []string{"Codex Web"}},
		{key: "desktop", candidates: []string{"Codex in ChatGPT Desktop"}},
	}

	serviceComponents := make([]AIServiceComponent, 0, len(componentSpecs))
	overall := "operational"
	for _, spec := range componentSpecs {
		status := findAIComponentStatus(components, spec.candidates)
		serviceComponents = append(serviceComponents, AIServiceComponent{Key: spec.key, Status: status})
		overall = worseAIStatus(overall, status)
	}

	relevant := make([]statusIncident, 0, len(incidents))
	for _, incident := range incidents {
		if isCodexIncident(incident) {
			relevant = append(relevant, incident)
		}
	}
	sort.SliceStable(relevant, func(i, j int) bool {
		return relevant[i].CreatedAt > relevant[j].CreatedAt
	})

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -(aiIntelligenceWindowDays - 1))
	days := make([]AIServiceDay, aiIntelligenceWindowDays)
	for index := range days {
		day := start.AddDate(0, 0, index)
		days[index] = AIServiceDay{Date: day.Format("2006-01-02"), Status: "operational"}
	}

	for _, incident := range relevant {
		created, err := time.Parse(time.RFC3339, incident.CreatedAt)
		if err != nil {
			continue
		}
		ended := now
		if incident.ResolvedAt != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, incident.ResolvedAt); parseErr == nil {
				ended = parsed
			}
		}
		severity := aiIncidentSeverity(incident.Impact)
		for index := range days {
			dayStart := start.AddDate(0, 0, index)
			dayEnd := dayStart.Add(24 * time.Hour)
			if created.Before(dayEnd) && !ended.Before(dayStart) {
				days[index].Status = worseAIStatus(days[index].Status, severity)
			}
		}
		if incident.ResolvedAt == "" {
			overall = worseAIStatus(overall, severity)
		}
	}

	eventFreeDays := 0
	for _, day := range days {
		if day.Status == "operational" {
			eventFreeDays++
		}
	}

	var recent *AIServiceIncident
	if len(relevant) > 0 {
		recent = &AIServiceIncident{
			Title:      relevant[0].Name,
			Status:     aiIncidentSeverity(relevant[0].Impact),
			OccurredAt: relevant[0].CreatedAt,
		}
	}

	return AIServiceStatus{
		OverallStatus:  overall,
		Components:     serviceComponents,
		Days:           days,
		EventFreeDays:  eventFreeDays,
		WindowDays:     aiIntelligenceWindowDays,
		RecentIncident: recent,
	}
}

func findAIComponentStatus(components []statusComponent, candidates []string) string {
	for _, candidate := range candidates {
		for _, component := range components {
			if strings.EqualFold(strings.TrimSpace(component.Name), candidate) {
				return normalizeAIStatus(component.Status)
			}
		}
	}
	return "unknown"
}

func isCodexIncident(incident statusIncident) bool {
	if strings.Contains(strings.ToLower(incident.Name), "codex") {
		return true
	}
	for _, update := range incident.IncidentUpdates {
		if strings.Contains(strings.ToLower(update.Body), "codex") {
			return true
		}
	}
	return false
}

func aiIncidentSeverity(impact string) string {
	switch strings.ToLower(strings.TrimSpace(impact)) {
	case "critical":
		return "major_outage"
	case "major":
		return "partial_outage"
	case "maintenance":
		return "under_maintenance"
	default:
		return "degraded_performance"
	}
}

func normalizeAIStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "operational", "under_maintenance", "degraded_performance", "partial_outage", "major_outage":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "unknown"
	}
}

func worseAIStatus(left, right string) string {
	priority := map[string]int{
		"operational":          0,
		"unknown":              1,
		"under_maintenance":    2,
		"degraded_performance": 3,
		"partial_outage":       4,
		"major_outage":         5,
	}
	if priority[right] > priority[left] {
		return right
	}
	return left
}

func cloneAIIntelligencePayload(source *AIIntelligencePayload) *AIIntelligencePayload {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Points = append([]AIIntelligencePoint(nil), source.Points...)
	clone.ServiceStatus.Components = append([]AIServiceComponent(nil), source.ServiceStatus.Components...)
	clone.ServiceStatus.Days = append([]AIServiceDay(nil), source.ServiceStatus.Days...)
	if source.ServiceStatus.RecentIncident != nil {
		incident := *source.ServiceStatus.RecentIncident
		clone.ServiceStatus.RecentIncident = &incident
	}
	return &clone
}
