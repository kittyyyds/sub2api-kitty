// Package videoreference provides bounded, expiring files for video suppliers
// which require a public URL. It does not store canvas projects.
package videoreference

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const MaxFileBytes int64 = 25 << 20
const MaxUserBytes int64 = 100 << 20
const MaxTotalBytes int64 = 2 << 30
const TTL = 24 * time.Hour
const MaxConcurrentWrites = 4

var ErrLimit = errors.New("reference storage quota exceeded")
var ErrFormat = errors.New("unsupported reference format")

type Entry struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	Bytes     int64     `json:"bytes"`
	MIME      string    `json:"mime_type"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	dir     string
	mu      sync.Mutex
	entries map[string]Entry
	writes  map[string]Entry
	now     func() time.Time
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, entries: make(map[string]Entry), writes: make(map[string]Entry), now: time.Now}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		if !validID(id) {
			continue
		}
		info, statErr := file.Info()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, file.Name()))
		var entry Entry
		if readErr != nil || len(data) > 4096 || json.Unmarshal(data, &entry) != nil || entry.ID != id || entry.UserID < 1 {
			continue
		}
		info, statErr = os.Lstat(filepath.Join(dir, id+".bin"))
		if statErr == nil && info.Mode().IsRegular() && info.Size() == entry.Bytes && entry.Bytes > 0 && entry.Bytes <= MaxFileBytes {
			s.entries[id] = entry
		}
	}
	// This directory belongs to a single Store/application process. At startup
	// there are no active writes: orphaned bytes must not escape quota accounting.
	s.mu.Lock()
	s.cleanupLocked(true)
	s.mu.Unlock()
	return s, nil
}

func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// Put reserves worst-case storage under the mutex, then releases it before
// reading the request body. A slow upload must not block downloads or cleanup.
// Reservations include bytes and file counts, and are released on every exit.
func (s *Store) Put(userID int64, r io.Reader) (Entry, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return Entry{}, err
	}
	id := hex.EncodeToString(buf)
	s.mu.Lock()
	s.cleanupLocked(false)
	var total, used int64
	count := 0
	for _, e := range s.entries {
		total += e.Bytes
		if e.UserID == userID {
			used += e.Bytes
			count++
		}
	}
	for _, e := range s.writes {
		total += e.Bytes
		if e.UserID == userID {
			used += e.Bytes
			count++
		}
	}
	limit := min(MaxFileBytes, MaxTotalBytes-total, MaxUserBytes-used)
	if userID < 1 || limit <= 0 || count >= 50 || len(s.entries)+len(s.writes) >= 20000 || len(s.writes) >= MaxConcurrentWrites {
		s.mu.Unlock()
		return Entry{}, ErrLimit
	}
	s.writes[id] = Entry{ID: id, UserID: userID, Bytes: limit}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.writes, id)
		s.mu.Unlock()
	}()
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return Entry{}, err
	}
	head = head[:n]
	mime := detectMIME(head)
	if mime == "" {
		return Entry{}, ErrFormat
	}
	f, err := os.OpenFile(filepath.Join(s.dir, id+".part"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Entry{}, err
	}
	defer os.Remove(f.Name())
	byteCount, copyErr := io.Copy(f, io.LimitReader(io.MultiReader(bytes.NewReader(head), r), limit+1))
	closeErr := f.Close()
	if copyErr != nil {
		return Entry{}, copyErr
	}
	if closeErr != nil {
		return Entry{}, closeErr
	}
	if byteCount > limit {
		return Entry{}, ErrLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := Entry{ID: id, UserID: userID, Bytes: byteCount, MIME: mime, ExpiresAt: s.now().UTC().Add(TTL)}
	dest := filepath.Join(s.dir, id+".bin")
	if err := os.Rename(f.Name(), dest); err != nil {
		return Entry{}, err
	}
	data, _ := json.Marshal(entry)
	if err := os.WriteFile(filepath.Join(s.dir, id+".json"), data, 0600); err != nil {
		_ = os.Remove(dest)
		return Entry{}, err
	}
	s.entries[id] = entry
	return entry, nil
}

func detectMIME(b []byte) string {
	mime := http.DetectContentType(b)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "audio/wave", "audio/x-wav", "audio/mpeg", "application/ogg":
		return mime
	}
	if len(b) >= 4 && string(b[:4]) == "fLaC" {
		return "audio/flac"
	}
	if len(b) >= 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "M4A " || string(b[8:12]) == "M4B ") {
		return "audio/mp4"
	}
	return ""
}

func (s *Store) Open(id string) (*os.File, Entry, error) {
	if !validID(id) {
		return nil, Entry{}, os.ErrNotExist
	}
	s.mu.Lock()
	e, ok := s.entries[id]
	s.mu.Unlock()
	if !ok || !s.now().Before(e.ExpiresAt) {
		return nil, Entry{}, os.ErrNotExist
	}
	f, err := os.Open(filepath.Join(s.dir, id+".bin"))
	return f, e, err
}

func (s *Store) Delete(userID int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !validID(id) || !ok || e.UserID != userID {
		return os.ErrNotExist
	}
	return s.removeLocked(id)
}

func (s *Store) removeLocked(id string) error {
	if err := os.Remove(filepath.Join(s.dir, id+".bin")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, id+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	delete(s.entries, id)
	return nil
}

func (s *Store) Cleanup() { s.mu.Lock(); defer s.mu.Unlock(); s.cleanupLocked(false) }
func (s *Store) cleanupLocked(startup bool) {
	for id, e := range s.entries {
		if !s.now().Before(e.ExpiresAt) {
			_ = s.removeLocked(id)
		}
	}
	// Interrupted writes and orphaned media/metadata expire too. An active
	// reservation owns its .part file even while a concurrent cleanup runs.
	files, _ := os.ReadDir(s.dir)
	for _, file := range files {
		name := file.Name()
		ext := filepath.Ext(name)
		if ext != ".part" && ext != ".bin" && ext != ".json" {
			continue
		}
		id := strings.TrimSuffix(name, ext)
		if !validID(id) {
			continue
		}
		if _, exists := s.entries[id]; exists {
			continue
		}
		if _, exists := s.writes[id]; exists {
			continue
		}
		info, err := file.Info()
		if err == nil && (startup || s.now().Sub(info.ModTime()) >= TTL) {
			_ = os.Remove(filepath.Join(s.dir, name))
		}
	}
}
