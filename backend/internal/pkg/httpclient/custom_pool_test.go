package httpclient

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCustomClientCacheEvictionClosesIdleButPreservesActiveResponse(t *testing.T) {
	finish := make(chan struct{})
	closed := make(chan struct{}, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			_, _ = io.WriteString(w, "first")
			w.(http.Flusher).Flush()
			<-finish
			_, _ = io.WriteString(w, "last")
			return
		}
		_, _ = io.WriteString(w, "ready")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	released := false
	defer func() {
		if !released {
			close(finish)
		}
	}()
	cache := newCustomClientCache(2)
	opts := Options{Timeout: 5 * time.Second, TransportConfigKey: "test", ConfigureTransport: func(_ *http.Transport) error { return nil }}
	a, err := cache.get("a", opts)
	require.NoError(t, err)
	stream, err := a.Get(server.URL + "/stream")
	require.NoError(t, err)
	defer stream.Body.Close()
	first := make([]byte, 5)
	_, err = io.ReadFull(stream.Body, first)
	require.NoError(t, err)
	ready, err := a.Get(server.URL + "/ready")
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, ready.Body)
	require.NoError(t, err)
	require.NoError(t, ready.Body.Close())
	_, err = cache.get("b", opts)
	require.NoError(t, err)
	_, err = cache.get("c", opts)
	require.NoError(t, err)
	require.Len(t, cache.entries, 2)
	_, exists := cache.entries["a"]
	require.False(t, exists)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("eviction did not close the idle connection")
	}
	close(finish)
	released = true
	rest, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, "last", string(rest), "the evicted client's active response must continue")
	again, err := cache.get("a", opts)
	require.NoError(t, err)
	require.NotSame(t, a, again)
}

func TestCustomClientCacheReusesAndRefreshesLRU(t *testing.T) {
	cache := newCustomClientCache(2)
	opts := Options{TransportConfigKey: "test", ConfigureTransport: func(_ *http.Transport) error { return nil }}
	a, err := cache.get("a", opts)
	require.NoError(t, err)
	_, err = cache.get("b", opts)
	require.NoError(t, err)
	reused, err := cache.get("a", opts)
	require.NoError(t, err)
	require.Same(t, a, reused)
	_, err = cache.get("c", opts)
	require.NoError(t, err)
	_, exists := cache.entries["b"]
	require.False(t, exists)
	_, exists = cache.entries["a"]
	require.True(t, exists)
}
