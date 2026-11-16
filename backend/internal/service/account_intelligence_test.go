//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type manualIQRepo struct {
	AccountRepository
	mu    sync.Mutex
	a     *Account
	items map[string]*AccountIntelligenceTest
	done  chan *AccountIntelligenceTest
	gets  []int64
}

func (r *manualIQRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets = append(r.gets, id)
	if id != r.a.ID {
		return nil, errors.New("wrong account")
	}
	return r.a, nil
}
func (r *manualIQRepo) IntelligenceFind(_ context.Context, id int64, req string) (*AccountIntelligenceTest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.items[req]
	if v == nil {
		return nil, nil
	}
	copy := *v
	return &copy, nil
}
func (r *manualIQRepo) IntelligenceClaim(_ context.Context, v *AccountIntelligenceTest) (*AccountIntelligenceTest, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.items[v.RequestID]; old != nil {
		return old, false, nil
	}
	for _, old := range r.items {
		if old.Status == "running" {
			return nil, false, ErrAccountIntelligenceBusy
		}
	}
	cp := *v
	r.items[v.RequestID] = &cp
	return &cp, true, nil
}
func (r *manualIQRepo) IntelligenceFinish(_ context.Context, v *AccountIntelligenceTest) error {
	r.mu.Lock()
	cp := *v
	r.items[v.RequestID] = &cp
	r.mu.Unlock()
	r.done <- &cp
	return nil
}
func (r *manualIQRepo) IntelligenceHistory(context.Context, []int64, int, int) ([]AccountIntelligenceTest, error) {
	return nil, nil
}
func TestAccountIntelligencePersistsFixedAccountAndIdempotency(t *testing.T) {
	repo := &manualIQRepo{a: &Account{ID: 27, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"synthetic_ui_test": true}}, items: map[string]*AccountIntelligenceTest{}, done: make(chan *AccountIntelligenceTest, 1)}
	svc := &AccountTestService{accountRepo: repo}
	input := AccountIntelligenceInput{RequestID: uuid.NewString(), Kind: "custom", Model: "gpt-6-astra", Prompt: "synthetic unchanged question"}
	job, err := svc.StartAccountIntelligence(context.Background(), 27, input)
	require.NoError(t, err)
	require.Equal(t, int64(27), job.AccountID)
	select {
	case result := <-repo.done:
		require.Equal(t, "completed", result.Status)
		require.Equal(t, input.Prompt, result.Prompt)
		require.NotEmpty(t, result.Response)
	case <-time.After(5 * time.Second):
		t.Fatal("test did not complete")
	}
	same, err := svc.StartAccountIntelligence(context.Background(), 27, input)
	require.NoError(t, err)
	require.Equal(t, job.ID, same.ID)
	require.Len(t, repo.items, 1)
	for _, id := range repo.gets {
		require.EqualValues(t, 27, id)
	}
}
func TestAccountIntelligenceScoringKeepsFailuresSeparate(t *testing.T) {
	for _, tc := range []struct {
		answer, status, want string
		err                  error
	}{{"21", "success", "correct", nil}, {"29", "success", "incorrect", nil}, {"21", "failed", "error", nil}, {"", "success", "error", nil}, {"21", "success", "error", errors.New("network")}} {
		item := &AccountIntelligenceTest{Kind: "candy", Prompt: "snapshot", ExpectedAnswer: "21"}
		finishAccountIntelligenceResult(item, &ScheduledTestResult{Status: tc.status, ResponseText: tc.answer}, tc.err)
		require.Equal(t, tc.want, item.Status)
		require.Equal(t, "snapshot", item.Prompt)
	}
	input := AccountIntelligenceInput{RequestID: uuid.NewString(), Kind: "candy", Model: "gpt-6-astra", Prompt: "client cannot replace fixed question"}
	require.NoError(t, validateAccountIntelligence(&input))
	require.Contains(t, input.Prompt, CandyPrompt)
}
func TestAccountIntelligenceObservesWithoutChangingAccountHealth(t *testing.T) {
	repo := &openAIAccountTestRepo{}
	svc := &AccountTestService{accountRepo: repo}
	ctx := context.WithValue(context.Background(), accountIntelligenceObservationKey{}, true)
	require.NoError(t, svc.setAccountTestError(ctx, 1, "bad credentials"))
	svc.reconcileOpenAI429State(ctx, &Account{ID: 1, Status: StatusError}, http.Header{"Retry-After": {"30"}}, []byte(`{"error":{"type":"usage_limit_reached","resets_at":2000000000}}`))
	require.Zero(t, repo.setErrorID)
	require.Zero(t, repo.rateLimitedID)
	require.Zero(t, repo.clearedErrorID)
	require.Nil(t, repo.updatedExtra)
}
func TestAccountIntelligenceReusesSelectedProxyAndModelMapping(t *testing.T) {
	response := newJSONResponse(200, "")
	response.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"))
	proxy := &Proxy{ID: 9, Protocol: "http", Host: "proxy.example.test", Port: 8080}
	repo := &openAIAccountTestRepo{}
	upstream := &iqHTTPRecorder{queuedHTTPUpstream: queuedHTTPUpstream{responses: []*http.Response{response}}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{ID: 27, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, ProxyID: &proxy.ID, Proxy: proxy, Credentials: map[string]any{"access_token": "synthetic-access", "model_mapping": map[string]any{"public-model": "gpt-6-astra"}}}
	c, _ := newTestContext()
	c.Request = c.Request.WithContext(withAccountTestMaxOutputTokens(context.WithValue(c.Request.Context(), accountIntelligenceObservationKey{}, true), 8192))
	require.NoError(t, svc.testOpenAIAccountConnection(c, account, "public-model", "synthetic question", ""))
	require.Equal(t, proxy.URL(), upstream.proxy)
	require.EqualValues(t, 27, upstream.account)
	data, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	require.Equal(t, "gpt-6-astra", body["model"])
	require.NotContains(t, body, "max_output_tokens", "manual Codex OAuth checks must omit the unsupported public API cap")
}

type iqHTTPRecorder struct {
	queuedHTTPUpstream
	proxy   string
	account int64
}

func (u *iqHTTPRecorder) DoWithTLS(req *http.Request, proxy string, id int64, c int, p *tlsfingerprint.Profile) (*http.Response, error) {
	u.proxy = proxy
	u.account = id
	return u.queuedHTTPUpstream.DoWithTLS(req, proxy, id, c, p)
}
