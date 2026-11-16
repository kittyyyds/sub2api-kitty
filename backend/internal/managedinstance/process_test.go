package managedinstance_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// This helper runs only as a dedicated child test executable. It uses the real
// production Redis lease adapter and HTTP wrapper, with a local mock paid job.
func TestManagedProcessFixture(t *testing.T) {
	if os.Getenv("KITTY_MANAGED_TEST_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	require.NoError(t, managedinstance.ConfigureFromEnv())
	c := managedinstance.Current()
	r := redis.NewClient(&redis.Options{Addr: os.Getenv("KITTY_MANAGED_TEST_REDIS")})
	defer r.Close()
	_ = repository.NewConcurrencyCache(r, 1, 60)
	dir := filepath.Dir(os.Getenv("KITTY_INSTANCE_ROLE_FILE"))
	id := os.Getenv("KITTY_INSTANCE_ID")
	stop := make(chan struct{})
	var stopOnce sync.Once
	var wg sync.WaitGroup
	managedinstance.Register("mock-paid-worker", func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(40 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					ctx, done, ok := managedinstance.Begin(context.Background())
					if !ok {
						continue
					}
					_ = os.WriteFile(filepath.Join(dir, "job-entered"), []byte("1"), 0600)
					for {
						if _, err := os.Stat(filepath.Join(dir, "hold-job")); err != nil {
							break
						}
						if ctx.Err() != nil {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					if ctx.Err() == nil {
						_ = r.Incr(ctx, "managed-smoke:jobs:"+id).Err()
					}
					done()
				}
			}
		}()
	}, func() { stopOnce.Do(func() { close(stop) }); wg.Wait() })
	srv := &http.Server{Handler: c.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s-start\n\n", id)
			w.(http.Flusher).Flush()
			for {
				if _, err := os.Stat(filepath.Join(dir, "finish-stream")); err == nil {
					break
				}
				select {
				case <-req.Context().Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			_, _ = fmt.Fprintf(w, "data: %s-finish\n\n", id)
			return
		}
		_, _ = io.WriteString(w, id)
	}))}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	c.MarkReady()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	go srv.Serve(listener)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "url"), []byte("http://"+listener.Addr().String()), 0600))
	for {
		if c.Snapshot().Role == "draining" && c.Snapshot().RetireReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, srv.Shutdown(context.Background()))
}

func TestManagedTwoRealProcessesSwitchWithoutInterruptingOldSSE(t *testing.T) {
	m := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r.Close()
	exe, err := os.Executable()
	require.NoError(t, err)
	type instance struct {
		dir, url string
		cmd      *exec.Cmd
		done     chan error
	}
	start := func(id string) *instance {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "role"), []byte("standby"), 0600))
		cmd := exec.Command(exe, "-test.run=^TestManagedProcessFixture$", "-test.timeout=40s")
		cmd.Env = append(os.Environ(), "KITTY_MANAGED_TEST_CHILD=1", "KITTY_MANAGED_TEST_REDIS="+m.Addr(), "KITTY_MANAGED_INSTANCE=true", "KITTY_INSTANCE_ID="+id, "KITTY_INSTANCE_ROLE_FILE="+filepath.Join(dir, "role"))
		require.NoError(t, cmd.Start())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		var endpoint []byte
		require.Eventually(t, func() bool { endpoint, err = os.ReadFile(filepath.Join(dir, "url")); return err == nil }, 5*time.Second, 20*time.Millisecond)
		return &instance{dir, string(endpoint), cmd, done}
	}
	status := func(i *instance) managedinstance.Status {
		b, _ := os.ReadFile(filepath.Join(i.dir, "status.json"))
		var s managedinstance.Status
		_ = json.Unmarshal(b, &s)
		return s
	}
	role := func(i *instance, role string) {
		require.NoError(t, os.WriteFile(filepath.Join(i.dir, "role.tmp"), []byte(role), 0600))
		require.NoError(t, os.Rename(filepath.Join(i.dir, "role.tmp"), filepath.Join(i.dir, "role")))
	}
	a, b := start("old"), start("new")
	require.False(t, status(a).LeadershipOwned)
	require.False(t, status(b).LeadershipOwned)
	require.NoError(t, os.WriteFile(filepath.Join(a.dir, "hold-job"), []byte("1"), 0600))
	role(a, "active")
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(a.dir, "job-entered")); return err == nil }, 5*time.Second, 20*time.Millisecond)
	var destination atomic.Value
	destination.Store(a.url)
	proxy := &httputil.ReverseProxy{Director: func(req *http.Request) {
		u, _ := url.Parse(destination.Load().(string))
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.Host = u.Host
	}, FlushInterval: -1}
	ingress := httptest.NewServer(proxy)
	defer ingress.Close()
	resp, err := http.Get(ingress.URL + "/stream")
	require.NoError(t, err)
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, first, "old-start")
	role(a, "draining")
	role(b, "active")
	require.Eventually(t, func() bool { return status(a).Role == "draining" }, 4*time.Second, 20*time.Millisecond)
	require.False(t, status(a).LeadershipReleased)
	require.False(t, status(b).LeadershipOwned)
	require.NoError(t, os.Remove(filepath.Join(a.dir, "hold-job")))
	require.Eventually(t, func() bool { return status(a).LeadershipReleased && status(b).LeadershipOwned }, 5*time.Second, 20*time.Millisecond)
	destination.Store(b.url)
	newResp, err := http.Get(ingress.URL + "/id")
	require.NoError(t, err)
	body, err := io.ReadAll(newResp.Body)
	newResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "new", string(body))
	require.False(t, status(a).RetireReady, "old stream is still live after new requests switched")
	require.NoError(t, os.WriteFile(filepath.Join(a.dir, "finish-stream"), []byte("1"), 0600))
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(rest), "old-finish"))
	select {
	case err := <-a.done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("old process did not retire naturally")
	}
	oldJobs, err := r.Get(context.Background(), "managed-smoke:jobs:old").Int64()
	require.NoError(t, err)
	require.Equal(t, int64(1), oldJobs)
	role(b, "draining")
	select {
	case err := <-b.done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("new process did not drain")
	}
}
