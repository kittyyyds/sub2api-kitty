package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestMonitorGroupProbeForwardsConfiguredReasoningToUpstream(t *testing.T) {
	for _, responses := range []bool{false, true} {
		name := "chat"
		body := `{"choices":[{"message":{"content":"21"},"finish_reason":"stop"}]}`
		if responses {
			name = "responses"
			body = `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"21"}]}]}`
		}
		t.Run(name, func(t *testing.T) {
			a := &Account{
				ID: 77, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test", "base_url": "https://upstream.example.com"},
				Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: responses},
			}
			var payload map[string]any
			upstream := &modelIdentityHTTPUpstream{body: body, beforeReturn: func(req *http.Request, _ int) {
				require.NotNil(t, req.GetBody)
				reader, err := req.GetBody()
				require.NoError(t, err)
				defer reader.Close()
				require.NoError(t, json.NewDecoder(reader).Decode(&payload))
			}}
			router := &groupTestRouterFake{steps: []routeStep{{account: a}}}
			groups, _ := newGroupTestService(newGroupTestRepoFake(), router, nil)
			groups.accounts = &AccountTestService{accountRepo: &modelIdentityAccountRepo{account: a}, httpUpstream: upstream, cfg: &config.Config{}}
			plan := &MonitorProbePlan{GroupID: 4, ModelID: "gpt-test", Config: &MonitorProbeConfig{QuestionKind: "candy", Prompt: CandyPrompt, ReasoningEffort: "high"}}
			result := groups.runSample(context.Background(), plan, &Group{ID: 4, Platform: PlatformOpenAI, Status: StatusActive})
			require.Equal(t, "success", result.Status, result.ErrorMessage)
			require.Equal(t, "21", result.ResponseText)
			if responses {
				require.Equal(t, map[string]any{"effort": "high"}, payload["reasoning"])
			} else {
				require.Equal(t, "high", payload["reasoning_effort"])
			}
			require.Equal(t, []int64{77}, router.released)
			require.Nil(t, result.CostUSD, "an internal probe must not invent a billed cost")
		})
	}
}
