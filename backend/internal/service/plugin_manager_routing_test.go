package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pluginRoutingHTTPUpstream struct {
	doCalls        int
	doWithTLSCalls int
}

func (u *pluginRoutingHTTPUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.doCalls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("legacy")),
	}, nil
}

func (u *pluginRoutingHTTPUpstream) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	u.doWithTLSCalls++
	return u.Do(request, proxyURL, accountID, accountConcurrency)
}

// The retired OpenAI OAuth outbound plugin hop is gone: OAuth and API-key
// requests both reach the HTTP upstream directly.
func TestOpenAIGatewayUpstreamHasNoPluginHop(t *testing.T) {
	upstream := &pluginRoutingHTTPUpstream{}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	for i, account := range []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1},
	} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
		require.NoError(t, err)
		response, err := service.doOpenAIUpstream(request, "", account)
		require.NoError(t, err)
		require.NotNil(t, response)
		_ = response.Body.Close()
		assert.Equal(t, i+1, upstream.doCalls)
	}
}

func TestStablePluginBucketIsDeterministicAndBounded(t *testing.T) {
	for id := int64(1); id <= 1000; id++ {
		first := stablePluginBucket(id)
		assert.Equal(t, first, stablePluginBucket(id))
		assert.Less(t, first, uint64(100))
	}
}

type statusStubRepository struct {
	PluginRepository
}

func (r *statusStubRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	return &PluginInstallation{ID: 7}, nil
}

// Status is the read-only, ungated runtime-status channel: when the plugin is not
// running it must report "not running" with no status blob and never error or start
// a runtime (that side-effecting behaviour belongs to Test, not Status).
func TestPluginManagerStatusReportsNotRunningWithoutRuntime(t *testing.T) {
	manager := &PluginManager{repo: &statusStubRepository{}}
	resp, err := manager.Status(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Healthy)
	assert.Equal(t, "插件未运行", resp.Message)
	assert.Empty(t, resp.StatusJson)
}
