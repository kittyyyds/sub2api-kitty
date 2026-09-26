package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAIIntelligenceHandlerBuildsFilteredSnapshot(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	server, calls, _ := newAIIntelligenceTestServer(t, now)
	defer server.Close()

	handler := newAIIntelligenceHandler(server.Client(), aiIntelligenceEndpoints{
		data:       server.URL + "/data",
		components: server.URL + "/components",
		incidents:  server.URL + "/incidents",
	}, 5*time.Minute, func() time.Time { return now })

	payload, err := handler.getSnapshot()
	require.NoError(t, err)
	require.False(t, payload.Stale)
	require.Len(t, payload.Points, 2)
	require.Equal(t, "gpt-5.6-sol", payload.Points[0].Model)
	require.Equal(t, "low", payload.Points[0].Effort)
	require.InDelta(t, 82, payload.Points[0].RecommendationScore, 0.0001)
	require.Equal(t, "2026-08-15T10:00:00Z", payload.SourceUpdatedAt)
	require.Equal(t, "degraded_performance", payload.ServiceStatus.OverallStatus)
	require.Equal(t, []AIServiceComponent{
		{Key: "api", Status: "degraded_performance"},
		{Key: "web", Status: "operational"},
		{Key: "desktop", Status: "under_maintenance"},
	}, payload.ServiceStatus.Components)
	require.Equal(t, 14, payload.ServiceStatus.EventFreeDays)
	require.NotNil(t, payload.ServiceStatus.RecentIncident)
	require.Equal(t, "Elevated errors in Codex", payload.ServiceStatus.RecentIncident.Title)
	require.Equal(t, int32(3), calls.Load())

	second, err := handler.getSnapshot()
	require.NoError(t, err)
	require.False(t, second.Stale)
	require.Equal(t, int32(3), calls.Load(), "fresh cache must avoid additional upstream requests")
}

func TestAIIntelligenceHandlerReturnsStaleSnapshotWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	server, _, fail := newAIIntelligenceTestServer(t, now)
	defer server.Close()

	handler := newAIIntelligenceHandler(server.Client(), aiIntelligenceEndpoints{
		data:       server.URL + "/data",
		components: server.URL + "/components",
		incidents:  server.URL + "/incidents",
	}, time.Minute, func() time.Time { return now })

	fresh, err := handler.getSnapshot()
	require.NoError(t, err)
	require.False(t, fresh.Stale)

	now = now.Add(2 * time.Minute)
	fail.Store(true)
	stale, err := handler.getSnapshot()
	require.NoError(t, err)
	require.True(t, stale.Stale)
	require.Equal(t, fresh.FetchedAt, stale.FetchedAt)
	require.Equal(t, fresh.Points, stale.Points)
}

func TestAIIntelligenceHandlerCoalescesConcurrentRefreshes(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	server, calls, _ := newAIIntelligenceTestServer(t, now)
	defer server.Close()

	handler := newAIIntelligenceHandler(server.Client(), aiIntelligenceEndpoints{
		data:       server.URL + "/data",
		components: server.URL + "/components",
		incidents:  server.URL + "/incidents",
	}, time.Minute, func() time.Time { return now })

	var wait sync.WaitGroup
	errors := make(chan error, 12)
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := handler.getSnapshot()
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, int32(3), calls.Load())
}

func TestAIIntelligenceHandlerRejectsUnknownSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/data":
			_, _ = writer.Write([]byte(`{"schema":1,"type":"legacy","points":[]}`))
		case "/components":
			_, _ = writer.Write([]byte(`{"components":[]}`))
		default:
			_, _ = writer.Write([]byte(`{"incidents":[]}`))
		}
	}))
	defer server.Close()

	handler := newAIIntelligenceHandler(server.Client(), aiIntelligenceEndpoints{
		data:       server.URL + "/data",
		components: server.URL + "/components",
		incidents:  server.URL + "/incidents",
	}, time.Minute, time.Now)
	_, err := handler.getSnapshot()
	require.ErrorContains(t, err, "unsupported AI intelligence schema")
}

func TestFindAIComponentStatusPrefersCodexSpecificComponent(t *testing.T) {
	components := []statusComponent{
		{Name: "Responses", Status: "degraded_performance"},
		{Name: "Codex API", Status: "operational"},
	}

	status := findAIComponentStatus(components, []string{"Codex API", "Responses", "API"})
	require.Equal(t, "operational", status)
}

func newAIIntelligenceTestServer(t *testing.T, now time.Time) (*httptest.Server, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if fail.Load() {
			http.Error(writer, "upstream unavailable", http.StatusBadGateway)
			return
		}
		writer.Header().Set("Content-Type", "application/json")

		var payload any
		switch request.URL.Path {
		case "/data":
			payload = map[string]any{
				"schema": 2,
				"type":   "distributed_intelligence_efficiency",
				"points": []map[string]any{
					{"model": "gpt-5.6-sol", "effort": "low", "iq": 100, "passed": 63, "valid_tasks": 112, "average_price_usd": 2.02, "average_minutes": 12, "latest_graded_at": "2026-08-15T10:00:00Z"},
					{"model": "gpt-5.5", "effort": "high", "iq": 97, "passed": 72, "valid_tasks": 112, "average_price_usd": 3.87, "average_minutes": 18, "latest_graded_at": "2026-08-15T09:00:00Z"},
					{"model": "gpt-5.6-luna", "effort": "low", "iq": 120, "passed": 90, "valid_tasks": 112, "average_price_usd": 1, "average_minutes": 8, "latest_graded_at": "2026-08-15T11:00:00Z"},
					{"model": "gpt-5.6-sol", "effort": "ultra", "iq": 130, "passed": 95, "valid_tasks": 112, "average_price_usd": 9, "average_minutes": 30, "latest_graded_at": "2026-08-15T11:00:00Z"},
					{"model": "deepseek-v4", "effort": "high", "iq": 110, "passed": 80, "valid_tasks": 112, "average_price_usd": 1, "average_minutes": 10, "latest_graded_at": "2026-08-15T11:00:00Z"},
				},
			}
		case "/components":
			payload = map[string]any{"components": []map[string]any{
				{"name": "Responses", "status": "degraded_performance"},
				{"name": "Codex Web", "status": "operational"},
				{"name": "Codex in ChatGPT Desktop", "status": "under_maintenance"},
			}}
		case "/incidents":
			incidentDay := now.AddDate(0, 0, -2)
			created := time.Date(incidentDay.Year(), incidentDay.Month(), incidentDay.Day(), 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
			resolved := time.Date(incidentDay.Year(), incidentDay.Month(), incidentDay.Day(), 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
			payload = map[string]any{"incidents": []map[string]any{
				{"name": "Unrelated login issue", "impact": "major", "created_at": now.Add(-time.Hour).Format(time.RFC3339), "resolved_at": now.Format(time.RFC3339), "incident_updates": []map[string]string{{"body": "Login only"}}},
				{"name": "Elevated errors in Codex", "impact": "minor", "created_at": created, "resolved_at": resolved, "incident_updates": []map[string]string{{"body": "Codex recovered"}}},
			}}
		default:
			http.NotFound(writer, request)
			return
		}
		require.NoError(t, json.NewEncoder(writer).Encode(payload))
	}))
	return server, &calls, &fail
}
