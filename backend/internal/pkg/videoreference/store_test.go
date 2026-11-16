package videoreference

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var png = []byte("\x89PNG\r\n\x1a\nreference-fixture")

func TestUploadSurvivesRestartAndExpires(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.Put(10, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if e.Bytes != int64(len(png)) || e.MIME != "image/png" || e.UserID != 10 || len(e.ID) != 64 {
		t.Fatalf("bad entry: %#v", e)
	}
	if err := s.Delete(11, e.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("another user deleted reference")
	}
	restarted, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := restarted.Open(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if !bytes.Equal(got, png) {
		t.Fatal("content mismatch")
	}
	restarted.now = func() time.Time { return e.ExpiresAt }
	if _, _, err := restarted.Open(e.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired reference still served")
	}
	restarted.Cleanup()
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatal("expiry did not remove media and metadata")
	}
}

func TestRejectActiveContentAndTraversal(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, content := range []string{"<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "<html><script>alert(1)</script></html>", "garbage", ""} {
		if _, err := s.Put(1, bytes.NewReader([]byte(content))); !errors.Is(err, ErrFormat) {
			t.Fatalf("accepted unsupported content %q", content)
		}
	}
	for _, id := range []string{"../secret", "/etc/passwd", "", "bad"} {
		if _, _, err := s.Open(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("accepted invalid path")
		}
	}
}

func TestFileAndUserQuotas(t *testing.T) {
	s, _ := New(t.TempDir())
	file := make([]byte, MaxFileBytes)
	copy(file, png)
	for i := int64(0); i < MaxUserBytes/MaxFileBytes; i++ {
		if _, err := s.Put(1, bytes.NewReader(file)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Put(1, bytes.NewReader(png)); !errors.Is(err, ErrLimit) {
		t.Fatal("user quota not enforced")
	}
	if _, err := s.Put(2, bytes.NewReader(append(file, 0))); !errors.Is(err, ErrLimit) {
		t.Fatal("file quota not enforced")
	}
	if _, err := s.Put(2, bytes.NewReader(png)); err != nil {
		t.Fatal("quota leaked between users")
	}
	parts, _ := filepath.Glob(filepath.Join(s.dir, "*.part"))
	if len(parts) != 0 {
		t.Fatal("oversized upload left partial")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("disconnected") }

func TestInterruptedUploadLeavesNoFiles(t *testing.T) {
	s, _ := New(t.TempDir())
	r := io.MultiReader(bytes.NewReader(append(png, make([]byte, 1024)...)), failingReader{})
	if _, err := s.Put(1, r); err == nil {
		t.Fatal("accepted incomplete upload")
	}
	if files, _ := os.ReadDir(s.dir); len(files) != 0 {
		t.Fatal("incomplete media was saved")
	}
}

type blockedReader struct {
	started chan struct{}
	release chan struct{}
}

func (r blockedReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.release
	return 0, errors.New("cancelled")
}

func TestSlowUploadDoesNotBlockReadersOrCleanup(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.Put(1, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	r := blockedReader{started: make(chan struct{}), release: make(chan struct{})}
	writeDone := make(chan error, 1)
	defer func() {
		close(r.release)
		select {
		case <-writeDone:
		case <-time.After(2 * time.Second):
			t.Error("cancelled writer did not exit")
		}
	}()
	go func() { _, err := s.Put(2, r); writeDone <- err }()
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("writer not started")
	}
	readDone := make(chan error, 1)
	go func() {
		f, _, err := s.Open(entry.ID)
		if err == nil {
			err = f.Close()
		}
		s.Cleanup()
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow upload holds storage mutex")
	}
}

func TestConcurrentReservationsReleaseAfterCancellation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.limits.UserBytes = MaxFileBytes * MaxConcurrentWrites
	release := make(chan struct{})
	completed := make(chan error, MaxConcurrentWrites)
	for i := 0; i < MaxConcurrentWrites; i++ {
		r := blockedReader{started: make(chan struct{}), release: release}
		go func() { _, err := s.Put(1, r); completed <- err }()
		select {
		case <-r.started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("concurrent reservation blocked")
		}
	}
	if _, err := s.Put(1, bytes.NewReader(png)); !errors.Is(err, ErrLimit) {
		t.Fatal("pending user reservations not counted")
	}
	if _, err := s.Put(2, bytes.NewReader(png)); !errors.Is(err, ErrLimit) {
		t.Fatal("global concurrent write bound not enforced")
	}
	close(release)
	for i := 0; i < MaxConcurrentWrites; i++ {
		select {
		case err := <-completed:
			if err == nil {
				t.Fatal("cancelled upload succeeded")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("reservation did not finish")
		}
	}
	if _, err := s.Put(1, bytes.NewReader(png)); err != nil {
		t.Fatal("cancelled reservations leaked", err)
	}
}

func TestStartupRemovesFreshOrphanBytesAndInvalidMetadata(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("a", 64)
	for _, ext := range []string{".bin", ".part", ".json"} {
		if err := os.WriteFile(filepath.Join(dir, id+ext), []byte("interrupted"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(dir); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("fresh orphan bytes escaped quota on restart")
	}
}

func TestCleanupPreservesActivePartialWrite(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := blockedReader{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	defer func() {
		close(r.release)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("cancelled writer did not exit")
		}
	}()
	head := make([]byte, 512)
	copy(head, png)
	go func() { _, err := s.Put(1, io.MultiReader(bytes.NewReader(head), r)); done <- err }()
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not start")
	}
	parts, _ := filepath.Glob(filepath.Join(s.dir, "*.part"))
	if len(parts) != 1 {
		t.Fatal("expected one active partial")
	}
	old := time.Now().Add(-2 * TTL)
	if err := os.Chtimes(parts[0], old, old); err != nil {
		t.Fatal(err)
	}
	s.Cleanup()
	if _, err := os.Stat(parts[0]); err != nil {
		t.Fatal("cleanup removed active write", err)
	}
}
