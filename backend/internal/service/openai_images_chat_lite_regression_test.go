//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIUsageRegressionChatLiteHeaderRequiresMatchingBody(t *testing.T) {
	for _, lite := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "lite_header"}[lite], func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"synthetic hello"}],"reasoning_effort":"high","stream":false}`)
			upstream := &httpUpstreamRecorder{resp: newOpenAIRejectedFieldTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")}
			upstream.resp.Header.Set("Content-Type", "text/event-stream")
			svc := newOpenAIRejectedFieldTestService(upstream)
			c := newOpenAIRejectedFieldTestContext(body)
			c.Request.URL.Path = "/v1/chat/completions"
			if lite {
				c.Request.Header.Set(responsesLiteHeader, "true")
			}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, newOpenAIOAuthNamespaceTestAccount(), body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.bodies, 1)
			if isOpenAIResponsesLiteHeader(upstream.lastReq.Header.Get(responsesLiteHeader)) {
				require.Equal(t, "all_turns", gjson.GetBytes(upstream.lastBody, "reasoning.context").String(), "outbound Lite header and body must agree")
			}
		})
	}
}

func TestOpenAIUsageRegressionChatLitePreservesNamespaceAndHistory(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"synthetic history"}],"reasoning":{"effort":"high","context":"current_turn"},"tools":[{"type":"namespace","name":"audit_tools","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}]}],"tool_choice":{"type":"namespace","name":"audit_tools"},"parallel_tool_calls":true,"stream":false}`)
	original := append([]byte(nil), body...)
	upstream := &httpUpstreamRecorder{resp: newOpenAIRejectedFieldTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_synthetic\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")}
	upstream.resp.Header.Set("Content-Type", "text/event-stream")
	svc := newOpenAIRejectedFieldTestService(upstream)
	c := newOpenAIRejectedFieldTestContext(body)
	c.Request.URL.Path = "/v1/chat/completions"
	c.Request.Header.Set(responsesLiteHeader, "true")
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, newOpenAIOAuthNamespaceTestAccount(), body, "", "")
	require.NoError(t, err)
	require.Equal(t, original, body, "compatibility must not mutate the ingress body reused by failover")
	require.Equal(t, "true", c.GetHeader(responsesLiteHeader))
	require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
	require.Equal(t, "all_turns", gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())
	require.Equal(t, "audit_tools", gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools").tools.0.name`).String())
	require.Equal(t, "lookup", gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools").tools.0.tools.0.name`).String())
	require.Equal(t, "namespace", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
	require.Equal(t, "audit_tools", gjson.GetBytes(upstream.lastBody, "tool_choice.name").String())
	require.Contains(t, string(upstream.lastBody), "synthetic history")
}

func TestOpenAIUsageRegressionChatLiteRejectsUnsupportedToolWithoutDroppingIntent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"synthetic history","tools":[{"type":"image_generation"}],"stream":false}`)
	original := append([]byte(nil), body...)
	upstream := &httpUpstreamRecorder{}
	svc := newOpenAIRejectedFieldTestService(upstream)
	c := newOpenAIRejectedFieldTestContext(body)
	c.Request.URL.Path = "/v1/chat/completions"
	c.Request.Header.Set(responsesLiteHeader, "true")
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, newOpenAIOAuthNamespaceTestAccount(), body, "", "")
	require.ErrorContains(t, err, `does not support tool type "image_generation"`)
	var liteErr *openAIResponsesLiteValidationError
	require.ErrorAs(t, err, &liteErr)
	require.Equal(t, "tools[0].type", liteErr.param)
	require.Equal(t, http.StatusBadRequest, c.Writer.Status())
	require.Empty(t, upstream.requests)
	require.Equal(t, original, body)
}
