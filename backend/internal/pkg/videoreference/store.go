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

const MaxFileBytes int64 = 50 << 20
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
	policy         func() (Policy, error)
	limits         Policy
	cleanupAllowed bool
	dir            string
	mu             sync.Mutex
	policyMu       sync.Mutex
	entries        map[string]Entry
	writes         map[string]Entry
	now            func() time.Time
}

type Policy struct {
	Retention             time.Duration
	UserBytes, TotalBytes int64
	Holds                 map[string]time.Time
	ProtectAll            bool // Pre-upgrade tasks may lack a reference ID list.
}

func New(dir string, providers ...func() (Policy, error)) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, entries: make(map[string]Entry), writes: make(map[string]Entry), now: time.Now}
	s.limits = Policy{Retention: TTL, UserBytes: MaxUserBytes, TotalBytes: MaxTotalBytes}
	s.cleanupAllowed = true
	if len(providers) > 0 {
		s.policy = providers[0]
	}
	unlock, err := s.lockDisk()
	if err != nil {
		return nil, err
	}
	defer unlock()
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
	if s.policy == nil {
		s.mu.Lock()
		s.cleanupLocked(true)
		s.mu.Unlock()
	} else {
		s.refreshPolicy()
	}
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
	unlock, lockErr := s.lockDisk()
	if lockErr != nil {
		return Entry{}, lockErr
	}
	defer func() { unlock() }()
	if !s.refreshPolicy() {
		return Entry{}, errors.New("素材保留策略暂时不可用")
	}
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
	limit := min(MaxFileBytes, s.limits.TotalBytes-total, s.limits.UserBytes-used)
	if userID < 1 || limit <= 0 || count >= 50 || len(s.entries)+len(s.writes) >= 20000 || len(s.writes) >= MaxConcurrentWrites {
		s.mu.Unlock()
		return Entry{}, ErrLimit
	}
	s.writes[id] = Entry{ID: id, UserID: userID, Bytes: limit}
	if s.policy != nil {
		raw, _ := json.Marshal(s.writes[id])
		if err := os.WriteFile(filepath.Join(s.dir, id+".reserve"), raw, 0600); err != nil {
			delete(s.writes, id)
			s.mu.Unlock()
			return Entry{}, err
		}
	}
	s.mu.Unlock()
	unlock()
	unlock = func() {}
	defer func() {
		end, err := s.lockDisk()
		if err != nil {
			return
		} // A crashed reservation expires conservatively.
		defer end()
		s.mu.Lock()
		delete(s.writes, id)
		if s.policy != nil {
			_ = os.Remove(filepath.Join(s.dir, id+".reserve"))
		}
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
	finish, err := s.lockDisk()
	if err != nil {
		return Entry{}, err
	}
	defer finish()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := Entry{ID: id, UserID: userID, Bytes: byteCount, MIME: mime, ExpiresAt: s.now().UTC().Add(s.limits.Retention)}
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
	if len(b) >= 12 && string(b[4:8]) == "ftyp" {
		return "video/mp4"
	}
	return ""
}

func (s *Store) Open(id string) (*os.File, Entry, error) {
	unlock, err := s.lockDisk()
	if err != nil {
		return nil, Entry{}, err
	}
	defer unlock()
	if !s.refreshPolicy() {
		return nil, Entry{}, errors.New("素材保留策略暂时不可用")
	}
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
	unlock, err := s.lockDisk()
	if err != nil {
		return err
	}
	defer unlock()
	if !s.refreshPolicy() {
		return errors.New("素材引用状态暂时不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !validID(id) || !ok || e.UserID != userID {
		return os.ErrNotExist
	}
	if _, held := s.limits.Holds[id]; held || s.limits.ProtectAll {
		return errors.New("素材仍受任务保留期保护")
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

func (s *Store) Cleanup() {
	unlock, err := s.lockDisk()
	if err != nil {
		return
	}
	defer unlock()
	if !s.refreshPolicy() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(false)
}
func (s *Store) cleanupLocked(startup bool) {
	if !s.cleanupAllowed || s.limits.ProtectAll {
		return
	}
	for id, e := range s.entries {
		if done, held := s.limits.Holds[id]; held && (done.IsZero() || s.now().Before(done.Add(s.limits.Retention))) {
			continue
		}
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

func (s *Store) refreshPolicy() bool {
	if s.policy == nil {
		return true
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	policy, err := s.policy()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil || policy.Retention <= 0 || policy.UserBytes <= 0 || policy.TotalBytes < policy.UserBytes {
		s.cleanupAllowed = false
		return false
	}
	s.cleanupAllowed = true
	s.limits = policy
	// A managed handoff may leave an older process serving downloads. Reload
	// durable metadata so both processes can find newly uploaded references.
	files, readErr := os.ReadDir(s.dir)
	if readErr != nil {
		s.cleanupAllowed = false
		return false
	}
	s.entries = make(map[string]Entry)
	s.writes = make(map[string]Entry)
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".reserve") {
			id := strings.TrimSuffix(file.Name(), ".reserve")
			info, e := file.Info()
			if e != nil || !validID(id) || !info.Mode().IsRegular() || info.Size() > 4096 {
				continue
			}
			if s.now().Sub(info.ModTime()) > 15*time.Minute {
				_ = os.Remove(filepath.Join(s.dir, file.Name()))
				continue
			}
			raw, e := os.ReadFile(filepath.Join(s.dir, file.Name()))
			var reservation Entry
			if e != nil || json.Unmarshal(raw, &reservation) != nil || reservation.ID != id || reservation.Bytes <= 0 || reservation.Bytes > MaxFileBytes {
				s.cleanupAllowed = false
				return false
			}
			s.writes[id] = reservation
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || !validID(id) {
			continue
		}
		info, e := file.Info()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(s.dir, file.Name()))
		var entry Entry
		if e != nil || json.Unmarshal(raw, &entry) != nil || entry.ID != id || entry.UserID < 1 {
			continue
		}
		bin, e := os.Lstat(filepath.Join(s.dir, id+".bin"))
		if e == nil && bin.Mode().IsRegular() && bin.Size() == entry.Bytes && entry.Bytes > 0 && entry.Bytes <= MaxFileBytes {
			old, exists := s.entries[id]
			if !exists || entry.ExpiresAt.After(old.ExpiresAt) {
				s.entries[id] = entry
			}
		}
	}
	for id, done := range policy.Holds {
		e, ok := s.entries[id]
		if !ok {
			continue
		}
		until := done.Add(policy.Retention)
		if done.IsZero() {
			until = s.now().Add(policy.Retention)
		}
		if e.ExpiresAt.Before(until) {
			e.ExpiresAt = until.UTC()
			raw, _ := json.Marshal(e)
			if s.writeMetadata(id, raw) == nil {
				s.entries[id] = e
			} else {
				s.cleanupAllowed = false
				return false
			}
		}
	}
	if policy.ProtectAll {
		for id, e := range s.entries {
			until := s.now().Add(policy.Retention)
			if e.ExpiresAt.Before(until) {
				e.ExpiresAt = until
				raw, _ := json.Marshal(e)
				if s.writeMetadata(id, raw) != nil {
					s.cleanupAllowed = false
					return false
				}
				s.entries[id] = e
			}
		}
	}
	return true
}

func (s *Store) RetainForSubmission(userID int64, id string) error {
	unlock, err := s.lockDisk()
	if err != nil {
		return err
	}
	defer unlock()
	if !s.refreshPolicy() {
		return errors.New("素材保留策略暂时不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok || e.UserID != userID || !s.now().Before(e.ExpiresAt) {
		return os.ErrNotExist
	}
	until := s.now().UTC().Add(s.limits.Retention)
	if until.After(e.ExpiresAt) {
		e.ExpiresAt = until
	}
	raw, _ := json.Marshal(e)
	if err := s.writeMetadata(id, raw); err != nil {
		return err
	}
	s.entries[id] = e
	return nil
}

// Shared volumes survive managed handoffs. Kernel locks are released on crash;
// durable short-lived reservations account for uploads across both processes.
func (s *Store) lockDisk() (func(), error) {
	if s.policy == nil {
		return func() {}, nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, ".store.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(6 * time.Second)
	for {
		if tryFileLock(f) == nil {
			return func() { unlockFile(f); f.Close() }, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("素材存储繁忙，请稍后重试")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Store) writeMetadata(id string, raw []byte) error {
	f, err := os.CreateTemp(s.dir, "metadata-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.dir, id+".json"))
}
