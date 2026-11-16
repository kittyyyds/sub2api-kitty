package videoreference

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

func TestPolicyProtectsRunningReferencesAndFailsClosed(t *testing.T) {
	now := time.Now()
	holds := map[string]time.Time{}
	fail := false
	s, err := New(t.TempDir(), func() (Policy, error) {
		if fail {
			return Policy{}, errors.New("db unavailable")
		}
		return Policy{Retention: TTL, UserBytes: MaxUserBytes, TotalBytes: MaxTotalBytes, Holds: holds}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	entry, err := s.Put(1, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	holds[entry.ID] = time.Time{}
	now = now.Add(48 * time.Hour)
	s.Cleanup()
	f, _, err := s.Open(entry.ID)
	if err != nil {
		t.Fatal("running task lost reference", err)
	}
	f.Close()
	if err = s.Delete(1, entry.ID); err == nil {
		t.Fatal("active reference deleted")
	}
	holds[entry.ID] = now
	now = now.Add(25 * time.Hour)
	fail = true
	s.Cleanup()
	if _, err = os.Stat(s.dir + "/" + entry.ID + ".bin"); err != nil {
		t.Fatal("failed policy lookup removed file")
	}
	fail = false
	s.Cleanup()
	if _, _, err = s.Open(entry.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("finished task never expired", err)
	}
}

func TestPolicySharesReservationsAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	provider := func() (Policy, error) {
		return Policy{Retention: TTL, UserBytes: MaxFileBytes, TotalBytes: MaxFileBytes}, nil
	}
	a, err := New(dir, provider)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(dir, provider)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() { _, err := a.Put(1, reader); done <- err }()
	defer func() { writer.Close(); reader.Close(); <-done }()
	// The first write reserves all capacity before waiting for the request body.
	until := time.Now().Add(time.Second)
	for {
		files, _ := os.ReadDir(dir)
		found := false
		for _, f := range files {
			if len(f.Name()) > 8 && f.Name()[len(f.Name())-8:] == ".reserve" {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(until) {
			t.Fatal("missing shared reservation")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = b.Put(2, bytes.NewReader(png)); !errors.Is(err, ErrLimit) {
		t.Fatalf("second process bypassed quota: %v", err)
	}
	b.Cleanup()
}
func TestPolicyReloadSeesHandoffUploads(t *testing.T) {
	dir := t.TempDir()
	provider := func() (Policy, error) {
		return Policy{Retention: TTL, UserBytes: MaxUserBytes, TotalBytes: MaxTotalBytes}, nil
	}
	old, _ := New(dir, provider)
	next, _ := New(dir, provider)
	entry, err := next.Put(1, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := old.Open(entry.ID)
	if err != nil {
		t.Fatal("handoff process cannot serve uploaded reference", err)
	}
	f.Close()
}
