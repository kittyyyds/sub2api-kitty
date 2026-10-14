//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorRequestPathValidation(t *testing.T) {
	for _, path := range []string{"", "  ", "/v1/chat/completions", " /prefix/v1/chat/completions ", "/v1beta/models/gemini:generateContent"} {
		require.NoError(t, validateMonitorRequestPath(path), path)
	}
	for _, path := range []string{"https://other.example/v1", "//other.example/v1", "v1/chat", "/v1?token=x", "/v1#fragment", "/v1\\chat", "/v1/../admin", "/v1/./chat", "/v1//chat", "/v1/%2f%2e", "/v1/%252f", "/v1/\nchat", "/v1/ chat", "/" + strings.Repeat("x", 500)} {
		require.ErrorIs(t, validateMonitorRequestPath(path), ErrChannelMonitorInvalidRequestPath, path)
	}
}

func TestMonitorRequestPathAdapterAndChallenge(t *testing.T) {
	swapMonitorHTTPClient(t)
	for _, tc := range []struct{ provider, override, basePath, expected string }{
		{MonitorProviderZhipu, "", "", "/api/paas/v4/chat/completions"},
		{MonitorProviderZhipu, "/v1/chat/completions", "", "/v1/chat/completions"},
		{MonitorProviderZhipu, "/v1/chat/completions", "/proxy/v1", "/proxy/v1/chat/completions"},
		{MonitorProviderDeepseek, "", "", "/v1/chat/completions"},
	} {
		t.Run(tc.provider+tc.override+tc.basePath, func(t *testing.T) {
			h := &openAICaptureHandler{}
			srv := httptest.NewServer(h)
			defer srv.Close()
			result := runCheckForModel(context.Background(), tc.provider, srv.URL+tc.basePath, "test-key", "test-model", &CheckOptions{RequestPath: tc.override})
			require.Contains(t, []string{"operational", "degraded"}, result.Status)
			require.Equal(t, tc.expected, h.lastPath)
			require.Equal(t, "test-model", h.lastBody["model"])
			require.Equal(t, "Bearer test-key", h.lastHeaders.Get("Authorization"))
			h.rawResponse = `{"choices":[{"message":{"content":"not the answer"}}]}`
			result = runCheckForModel(context.Background(), tc.provider, srv.URL+tc.basePath, "test-key", "test-model", &CheckOptions{RequestPath: tc.override})
			require.Equal(t, "failed", result.Status, "path override must not bypass the challenge")
		})
	}
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	_, _, _, err := callProvider(context.Background(), MonitorProviderZhipu, srv.URL, "test-key", "model", "prompt", &CheckOptions{RequestPath: "//other.example/v1"})
	require.ErrorIs(t, err, ErrChannelMonitorInvalidRequestPath)
	require.False(t, called)
}

func TestMonitorRequestPathPartialUpdateAndClear(t *testing.T) {
	m := &ChannelMonitor{Provider: MonitorProviderZhipu, Endpoint: "https://example.com", APIKey: "encrypted", PrimaryModel: "glm", RequestPath: "/v1/chat/completions"}
	require.NoError(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{}))
	require.Equal(t, "/v1/chat/completions", m.RequestPath)
	bad := "//other.example/v1"
	require.ErrorIs(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{RequestPath: &bad}), ErrChannelMonitorInvalidRequestPath)
	require.Equal(t, "/v1/chat/completions", m.RequestPath)
	empty := ""
	require.NoError(t, applyMonitorUpdate(m, ChannelMonitorUpdateParams{RequestPath: &empty}))
	require.Empty(t, m.RequestPath)
	require.Equal(t, MonitorProviderZhipu, m.Provider)
	require.Equal(t, "encrypted", m.APIKey)
}

func TestMonitorRequestPathCreatePersistsAndValidates(t *testing.T) {
	repo := &duplicateChannelMonitorRepoStub{}
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	p := ChannelMonitorCreateParams{Name: "GLM", Provider: MonitorProviderZhipu, Endpoint: "https://8.8.8.8", APIKey: "test-key", PrimaryModel: "glm", IntervalSeconds: 60, RequestPath: " /v1/chat/completions "}
	m, err := svc.Create(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", m.RequestPath)
	require.Equal(t, m.RequestPath, repo.created[0].RequestPath)
	p.RequestPath = "/v1?key=secret"
	_, err = svc.Create(context.Background(), p)
	require.ErrorIs(t, err, ErrChannelMonitorInvalidRequestPath)
	require.Len(t, repo.created, 1)
}
