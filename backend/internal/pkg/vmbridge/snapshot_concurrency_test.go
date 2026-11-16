package vmbridge

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type snapshotTestProvider struct {
	*managedTestProvider
	snapshot func(context.Context, Resources) (*ManagedSnapshot, error)
}

func (p *snapshotTestProvider) Snapshot(ctx context.Context, r Resources) (*ManagedSnapshot, error) {
	return p.snapshot(ctx, r)
}

func TestManagedSnapshotDoesNotQueueInference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	headers := make(chan struct{}, 2)
	finish := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	p := &managedTestProvider{snap: loggedIn("chatgpt-7")}
	p.forward = func(ctx context.Context, _ Resources, _ *http.Request) (*http.Response, error) {
		headers <- struct{}{}
		select {
		case <-finish:
			return forwardReply(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	m := readyForwardManager(t, ModeVMManaged, p)
	done := make(chan error, 2)
	forward := func() {
		r, err := m.Forward(ctx, 7, &http.Request{}, nil)
		if r != nil {
			_ = r.Body.Close()
		}
		done <- err
	}
	go forward()
	waitSignal(t, headers)
	observed := make(chan error, 1)
	go func() {
		status, err := m.ManagedSnapshot(ctx, 7)
		if err == nil && !status.Ready {
			err = errors.New("healthy observation closed the binding")
		}
		observed <- err
	}()
	// Status polling must finish even while an existing request still waits for
	// headers; otherwise the poll gates every later inference behind that wait.
	awaitForward(t, observed)
	go forward()
	waitSignal(t, headers)
	once.Do(func() { close(finish) })
	awaitForward(t, done)
	awaitForward(t, done)
}

func TestSlowManagedSnapshotAllowsInferenceAndExcludesMutations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{})
	finish := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	p := &snapshotTestProvider{managedTestProvider: &managedTestProvider{}}
	p.snapshot = func(ctx context.Context, _ Resources) (*ManagedSnapshot, error) {
		close(entered)
		select {
		case <-finish:
			snap := loggedIn("chatgpt-7")
			return &snap, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	m := readyForwardManager(t, ModeVMManaged, p)
	observed := make(chan error, 1)
	go func() { _, err := m.ManagedSnapshot(ctx, 7); observed <- err }()
	waitSignal(t, entered)
	requestCtx, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	r, err := m.Forward(requestCtx, 7, &http.Request{}, nil)
	stop()
	if err != nil {
		t.Fatalf("a slow status read blocked inference: %v", err)
	}
	_ = r.Body.Close()
	mutationCtx, stopMutation := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stopMutation()
	if err := m.accounts[7].lock(mutationCtx); !errors.Is(err, context.DeadlineExceeded) {
		if err == nil {
			m.accounts[7].unlock()
		}
		t.Fatal("an exclusive mutation bypassed an in-progress snapshot")
	}
	once.Do(func() { close(finish) })
	awaitForward(t, observed)
	if err := m.accounts[7].lock(ctx); err != nil {
		t.Fatal("snapshot or canceled mutation leaked admission")
	}
	m.accounts[7].unlock()
}

func TestManagedSnapshotCancellationReleasesAdmission(t *testing.T) {
	entered := make(chan struct{})
	p := &snapshotTestProvider{managedTestProvider: &managedTestProvider{}}
	p.snapshot = func(ctx context.Context, _ Resources) (*ManagedSnapshot, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	m := readyForwardManager(t, ModeVMManaged, p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.ManagedSnapshot(ctx, 7); done <- err }()
	waitSignal(t, entered)
	cancel()
	awaitForward(t, done)
	check, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := m.accounts[7].lock(check); err != nil {
		t.Fatal("canceled snapshot leaked admission")
	}
	m.accounts[7].unlock()
}

func TestManagedSnapshotRechecksBeforeChangingLifecycle(t *testing.T) {
	p := &snapshotTestProvider{managedTestProvider: &managedTestProvider{}}
	var calls int
	p.snapshot = func(context.Context, Resources) (*ManagedSnapshot, error) {
		calls++
		snap := loggedIn("chatgpt-7")
		if calls == 1 {
			snap.RefreshError = "invalid_grant"
		}
		return &snap, nil
	}
	m := readyForwardManager(t, ModeVMManaged, p)
	got, err := m.ManagedSnapshot(context.Background(), 7)
	if err != nil || !got.Ready || calls != 2 || got.Snapshot.RefreshError != "" {
		t.Fatalf("a stale observation changed lifecycle without a protected reread: ready=%v calls=%d err=%v", got.Ready, calls, err)
	}
}
