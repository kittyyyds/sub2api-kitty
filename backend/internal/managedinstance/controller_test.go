package managedinstance

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testLease struct {
	mu    sync.Mutex
	owner string
	lost  bool
}

func (s *testLease) Acquire(_ context.Context, o string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner != "" {
		return false, nil
	}
	s.owner = o
	return true, nil
}
func (s *testLease) Renew(_ context.Context, o string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.lost && s.owner == o, nil
}
func (s *testLease) Release(_ context.Context, o string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner == o {
		s.owner = ""
	}
	return nil
}
func testController(t *testing.T, id string, lease *testLease) *Controller {
	t.Helper()
	p := filepath.Join(t.TempDir(), "role")
	if err := os.WriteFile(p, []byte("standby\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New(id, p)
	c.store = lease
	c.MarkReady()
	return c
}
func retire(t *testing.T, c *Controller) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.Step(context.Background())
		if c.Snapshot().LeadershipReleased {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("not released: %+v", c.Snapshot())
}

func TestManagedSingleLeaderDeferredStartAndNaturalDrain(t *testing.T) {
	lease := &testLease{}
	a := testController(t, "a", lease)
	b := testController(t, "b", lease)
	var starts, stops atomic.Int32
	a.workers = []worker{{"paid", func() { starts.Add(1) }, func() { stops.Add(1) }}}
	b.workers = []worker{{"paid", func() { starts.Add(1) }, func() { stops.Add(1) }}}
	a.Step(context.Background())
	if starts.Load() != 0 {
		t.Fatal("standby started paid worker")
	}
	if _, _, ok := a.Begin(context.Background()); ok {
		t.Fatal("standby admitted paid work")
	}
	_ = a.SetRole("active")
	a.Step(context.Background())
	a.Step(context.Background())
	ctx, done, ok := a.Begin(context.Background())
	if !ok || starts.Load() != 1 {
		t.Fatal("active worker not started exactly once")
	}
	_ = b.SetRole("active")
	b.Step(context.Background())
	if starts.Load() != 1 {
		t.Fatal("two background leaders")
	}
	a.Drain()
	a.Step(context.Background())
	if ctx.Err() != nil || stops.Load() != 0 {
		t.Fatal("drain cancelled an admitted job")
	}
	if _, _, ok := a.Begin(context.Background()); ok {
		t.Fatal("drain admitted another job")
	}
	b.Step(context.Background())
	if b.Snapshot().LeadershipOwned {
		t.Fatal("leadership handed off before old job completed")
	}
	done()
	retire(t, a)
	b.Step(context.Background())
	if starts.Load() != 2 || stops.Load() != 1 {
		t.Fatalf("handoff did not stop/start exactly once: %d %d", starts.Load(), stops.Load())
	}
	if err := a.SetRole("active"); err == nil {
		t.Fatal("drained instance reactivated")
	}
	b.Drain()
	retire(t, b)
}

func TestManagedLeaseLossStopsAdmissionAndCancelsBackground(t *testing.T) {
	l := &testLease{}
	c := testController(t, "one", l)
	_ = c.SetRole("active")
	c.Step(context.Background())
	ctx, done, ok := c.Begin(context.Background())
	if !ok {
		t.Fatal("not admitted")
	}
	defer done()
	l.mu.Lock()
	l.lost = true
	l.mu.Unlock()
	c.Step(context.Background())
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lease loss did not cancel background")
	}
	if _, _, ok := c.Begin(context.Background()); ok {
		t.Fatal("lease loss admitted another job")
	}
}

func TestManagedLeaseLossDuringStartupStopsOnlyStartedWorkers(t *testing.T) {
	l := &testLease{}
	c := testController(t, "partial-start", l)
	var stopped atomic.Int32
	c.workers = []worker{
		{"started", func() { l.mu.Lock(); l.lost = true; l.mu.Unlock() }, func() { stopped.Add(1) }},
		{"not-started", func() { t.Error("started without lease") }, func() { t.Error("stopped unstarted worker") }},
	}
	_ = c.SetRole("active")
	c.Step(context.Background())
	retire(t, c)
	if stopped.Load() != 1 {
		t.Fatal("started worker was not stopped exactly once")
	}
}

func TestManagedSSEContinuesDuringDrainAndAccountingBlocksRetirement(t *testing.T) {
	c := testController(t, "sse", &testLease{})
	_ = c.SetRole("active")
	c.Step(context.Background())
	finish := make(chan struct{})
	started := make(chan struct{})
	srv := httptest.NewServer(c.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stream" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-finish
		_, _ = io.WriteString(w, "data: done\n\n")
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-started
	s := c.Snapshot()
	if s.ActiveHTTP != 1 || s.ActiveSSE != 1 || s.ActiveRequests != 1 {
		t.Fatalf("bad SSE counts: %+v", s)
	}
	c.Drain()
	retire(t, c)
	if c.Snapshot().RetireReady {
		t.Fatal("live SSE marked retired")
	}
	r, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal("drain rejected HTTP before route handoff")
	}
	close(finish)
	body, err := io.ReadAll(resp.Body)
	if err != nil || !strings.Contains(string(body), "data: done") {
		t.Fatal("stream interrupted")
	}
	c.accounting = []func() bool{func() bool { return false }}
	if c.Snapshot().RetireReady {
		t.Fatal("accounting backlog ignored")
	}
	c.accounting = nil
	if !c.Snapshot().RetireReady {
		t.Fatalf("finished stream not retireable: %+v", c.Snapshot())
	}
}

func TestManagedHijackedConnectionOutlivesHTTPHandler(t *testing.T) {
	c := testController(t, "ws", &testLease{})
	var hijacked net.Conn
	done := make(chan struct{})
	srv := httptest.NewServer(c.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		hijacked = conn
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = rw.Flush()
		close(done)
	})))
	defer srv.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	_, _ = bufio.NewReader(conn).ReadString('\n')
	<-done
	s := c.Snapshot()
	if s.ActiveHTTP != 0 || s.ActiveWS != 1 || s.ActiveRequests != 1 {
		t.Fatalf("hijacked connection was forgotten: %+v", s)
	}
	c.Drain()
	retire(t, c)
	if c.Snapshot().RetireReady {
		t.Fatal("active websocket retired")
	}
	_ = hijacked.Close()
	_ = hijacked.Close()
	if !c.Snapshot().RetireReady || c.Snapshot().ActiveWS != 0 {
		t.Fatalf("close not counted exactly once: %+v", c.Snapshot())
	}
}

func TestManagedRoleFileValidationAndPrivateStatus(t *testing.T) {
	c := testController(t, "private", &testLease{})
	if err := c.ReadRole(); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(c.roleFile), "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Status
	if json.Unmarshal(b, &s) != nil || s.InstanceID != "private" || s.Role != "standby" {
		t.Fatalf("invalid status: %s", b)
	}
	_ = os.WriteFile(c.roleFile, []byte("active\n"), 0600)
	if err := c.ReadRole(); err != nil {
		t.Fatal(err)
	}
	c.Step(context.Background())
	_ = os.WriteFile(c.roleFile, []byte("invalid"), 0600)
	if err := c.ReadRole(); err == nil || c.Snapshot().Role != "draining" {
		t.Fatal("bad role did not fail closed")
	}
}
