package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestGatewayPromptPassthroughTransformsFollowupOnceBeforeMapping(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_one","model":"upstream-model","usage":{"input_tokens":1,"output_tokens":1}}}`)
	var calls atomic.Int32
	models := make(chan string, 3)
	hooks := &OpenAIWSIngressHooks{
		TransformPayload: func(raw []byte, model string) ([]byte, error) {
			calls.Add(1)
			models <- model
			return sjson.SetBytes(raw, "instructions", "PROMPT\n\n"+gjson.GetBytes(raw, "instructions").String())
		},
		MapRequestModel: func(_ int, model string) (string, error) { return "upstream-model", nil },
	}
	server, done := startPassthroughHookRecordingServer(t, ctx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount(), hooks)
	defer server.Close()
	conn := dialPassthroughLifecycleClient(t, server)
	defer conn.CloseNow()
	requirePassthroughUpstreamWrite(t, upstream, time.Second)
	_, err := readPassthroughLifecycleFrame(t, conn, 3*time.Second)
	require.NoError(t, err)
	require.Zero(t, calls.Load(), "first frame was already transformed by HTTP handler")
	wctx, stop := context.WithTimeout(ctx, time.Second)
	err = conn.Write(wctx, coderws.MessageText, []byte(`{"type":"response.create","instructions":"OLD","input":"next"}`))
	stop()
	require.NoError(t, err)
	frame := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, "gpt-5.1", <-models)
	require.Equal(t, "PROMPT\n\nOLD", gjson.GetBytes(frame, "instructions").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_two","model":"upstream-model","usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, err = readPassthroughLifecycleFrame(t, conn, 3*time.Second)
	require.NoError(t, err)
	conn.CloseNow()
	cancel(context.Canceled)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket did not stop")
	}
}
