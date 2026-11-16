package vmbridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Seed ready bindings directly so the concurrency tests also run on Windows;
// persistence permissions and managed provisioning are covered separately.
func readyForwardManager(t *testing.T, mode string, p Provider) *Manager {
	t.Helper()
	m := newTestManager(t, p)
	m.accounts[7] = newEntry(persistentAccount{
		AccountID: 7, Mode: mode, State: stateReady, ChatGPTAccountID: "chatgpt-7",
		Resources: Resources{VMID: "vm-7", KeyID: "key-7", APIKey: "test-slot-key"},
	})
	t.Cleanup(m.Close)
	return m
}

func forwardReply() *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}
}

func awaitForward(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forward did not finish")
	}
}

func TestManagedForwardAcquiresHeadersConcurrentlyAndKeepsLeases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{}, 6)
	finish := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	p := &testProvider{forward: func(ctx context.Context, r Resources, _ *http.Request) (*http.Response, error) {
		if r.VMID != "vm-7" || r.APIKey != "test-slot-key" {
			return nil, errors.New("wrong account resources")
		}
		entered <- struct{}{}
		select {
		case <-finish:
			return forwardReply(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := readyForwardManager(t, ModeVMManaged, p)
	results := make(chan *http.Response, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		go func() {
			r, err := m.Forward(ctx, 7, &http.Request{}, nil)
			results <- r
			errs <- err
		}()
	}
	for i := 0; i < 6; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("same-account requests serialized their upstream header waits")
		}
	}
	status, _ := m.Status(7)
	if status.ActiveLeases != 6 {
		t.Fatalf("pending requests were not leased: %+v", status)
	}
	once.Do(func() { close(finish) })
	for i := 0; i < 6; i++ {
		awaitForward(t, errs)
	}
	status, _ = m.Status(7)
	if status.ActiveLeases != 6 {
		t.Fatal("leases ended before their streamed bodies closed")
	}
	for i := 0; i < 6; i++ {
		r := <-results
		_ = r.Body.Close()
		_ = r.Body.Close()
	}
	status, _ = m.Status(7)
	if status.ActiveLeases != 0 {
		t.Fatal("stream closure leaked a lease")
	}
}

func TestSubTokenForwardStillSerializesIdentityAndHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	finish := make(chan struct{}, 2)
	p := &testProvider{forward: func(ctx context.Context, _ Resources, _ *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		select {
		case <-finish:
			return forwardReply(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := readyForwardManager(t, ModeSubToken, p)
	done := make(chan error, 2)
	forward := func() {
		r, err := m.Forward(ctx, 7, &http.Request{}, &Identity{AccessToken: "test-token", ChatGPTAccountID: "chatgpt-7"})
		if r != nil {
			_ = r.Body.Close()
		}
		done <- err
	}
	go forward()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first request did not enter")
	}
	go forward()
	select {
	case <-entered:
		t.Fatal("sub-token requests overlapped identity synchronization")
	case <-time.After(30 * time.Millisecond):
	}
	finish <- struct{}{}
	awaitForward(t, done)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("second request remained blocked after headers")
	}
	finish <- struct{}{}
	awaitForward(t, done)
}

func TestHeaderReadersPreserveAdministrativeExclusionAndCancellation(t *testing.T) {
	e := newEntry(persistentAccount{})
	if err := e.lockHeaders(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.lock(ctx) }()
	deadline := time.Now().Add(time.Second)
	for len(e.op) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(e.op) != 1 {
		e.unlockHeaders()
		t.Fatal("administrator did not stop new reader admission")
	}
	blocked, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err := e.lockHeaders(blocked); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("a new inference bypassed an administrator waiting for existing headers")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("administrative wait ignored cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled administration did not finish")
	}
	if err := e.lockHeaders(context.Background()); err != nil {
		t.Fatal("canceled administrator leaked the operation lock")
	}
	e.unlockHeaders()
	e.unlockHeaders()
	if err := e.lock(context.Background()); err != nil {
		t.Fatal("header readers did not drain")
	}
	e.unlock()
}

func TestManagedForwardCancellationReleasesHeadersAndLease(t *testing.T) {
	entered := make(chan struct{})
	p := &testProvider{forward: func(ctx context.Context, _ Resources, _ *http.Request) (*http.Response, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	m := readyForwardManager(t, ModeVMManaged, p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.Forward(ctx, 7, &http.Request{}, nil); done <- err }()
	waitSignal(t, entered)
	cancel()
	select {
	case err := <-done:
		requireCode(t, err, "upstream_unavailable")
	case <-time.After(time.Second):
		t.Fatal("canceled inference did not finish")
	}
	status, _ := m.Status(7)
	if status.ActiveLeases != 0 {
		t.Fatal("failed inference leaked a stream lease")
	}
	check, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := m.accounts[7].lock(check); err != nil {
		t.Fatal("failed inference leaked its header phase")
	}
	m.accounts[7].unlock()
}

func TestManagedForwardRejectsCredentialsWhileUsingSharedHeaders(t *testing.T) {
	m := readyForwardManager(t, ModeVMManaged, &testProvider{forward: func(context.Context, Resources, *http.Request) (*http.Response, error) {
		t.Fatal("managed credential override reached the provider")
		return nil, nil
	}})
	_, err := m.Forward(context.Background(), 7, &http.Request{}, &Identity{AccessToken: "test-token", ChatGPTAccountID: "chatgpt-7"})
	requireCode(t, err, "managed_credential")
	status, _ := m.Status(7)
	if status.ActiveLeases != 0 {
		t.Fatal("rejected credential override leaked a lease")
	}
}

func TestManagedHeaderDrainStillProtectsDeletion(t *testing.T) {
	// This also exercises Unix state persistence and is run in Linux rehearsal.
	if testing.Short() {
		t.Skip("persistent lifecycle covered by full Linux suite")
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	p := &testProvider{forward: func(ctx context.Context, _ Resources, _ *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-finish:
			return forwardReply(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	m := newTestManager(t, p)
	t.Cleanup(m.Close)
	provisionAccount(t, m, 7)
	m.accounts[7].data.Mode = ModeVMManaged
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan *http.Response, 1)
	done := make(chan error, 1)
	go func() { r, err := m.Forward(ctx, 7, &http.Request{}, nil); result <- r; done <- err }()
	waitSignal(t, entered)
	err := m.Delete(context.Background(), 7)
	requireCode(t, err, "account_busy")
	_, err = m.Forward(context.Background(), 7, &http.Request{}, nil)
	requireCode(t, err, "account_not_ready")
	once.Do(func() { close(finish) })
	awaitForward(t, done)
	r := <-result
	err = m.Delete(context.Background(), 7)
	requireCode(t, err, "account_busy")
	_ = r.Body.Close()
	if err = m.Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
}
