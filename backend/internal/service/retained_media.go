package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RetainedMediaMaxBytes int64 = 96 << 20

type RetainedMediaService struct {
	repo    RetainedMediaRepository
	dir     string
	cleanup bool
	uploads chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

func NewRetainedMediaService(repo RetainedMediaRepository) (*RetainedMediaService, error) {
	s := &RetainedMediaService{repo: repo, dir: os.Getenv("KITTY_MEDIA_STORAGE_DIR"), cleanup: os.Getenv("KITTY_MEDIA_STORAGE_CLEANUP") == "1", uploads: make(chan struct{}, 4), stop: make(chan struct{}), done: make(chan struct{})}
	if s.dir != "" {
		if !filepath.IsAbs(s.dir) {
			return nil, errors.New("KITTY_MEDIA_STORAGE_DIR must be absolute")
		}
		if e := os.MkdirAll(s.dir, 0700); e != nil {
			return nil, e
		}
		f, e := os.CreateTemp(s.dir, ".health-")
		if e != nil {
			return nil, e
		}
		f.Close()
		os.Remove(f.Name())
	}
	go s.cleanupLoop()
	return s, nil
}
func (s *RetainedMediaService) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}
func (s *RetainedMediaService) Enabled() bool { return s.dir != "" }
func (s *RetainedMediaService) Config() map[string]any {
	return map[string]any{"enabled": s.Enabled(), "free_hours": 24, "image_day_usd": "0.01", "video_day_usd": "0.10", "max_days": 365, "max_file_bytes": RetainedMediaMaxBytes, "cleanup_enabled": s.cleanup, "server_time": time.Now().UTC()}
}
func (s *RetainedMediaService) path(id string) (string, error) {
	u, e := uuid.Parse(id)
	if e != nil || u.String() != id {
		return "", ErrStorageInvalid
	}
	return filepath.Join(s.dir, id+".blob"), nil
}
func retainedKind(mime string) string {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif":
		return "image"
	case "video/mp4", "video/webm", "video/quicktime":
		return "video"
	}
	return ""
}
func (s *RetainedMediaService) Upload(ctx context.Context, user int64, name string, input io.Reader) (*RetainedMedia, error) {
	if !s.Enabled() {
		return nil, ErrStorageUnavailable
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errors.New("保存任务较多，请稍后重试")
	}
	f, e := os.CreateTemp(s.dir, ".upload-")
	if e != nil {
		return nil, e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	size, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(input, RetainedMediaMaxBytes+1))
	if e != nil {
		return nil, e
	}
	if size == 0 || size > RetainedMediaMaxBytes {
		return nil, ErrStorageInvalid
	}
	if _, e = f.Seek(0, 0); e != nil {
		return nil, e
	}
	head := make([]byte, 512)
	n, e := f.Read(head)
	if e != nil && e != io.EOF {
		return nil, e
	}
	mime := http.DetectContentType(head[:n])
	if n >= 12 && string(head[4:8]) == "ftyp" {
		if string(head[8:12]) == "avif" || string(head[8:12]) == "avis" {
			mime = "image/avif"
		}
		if string(head[8:12]) == "qt  " {
			mime = "video/quicktime"
		}
	}
	kind := retainedKind(mime)
	if kind == "" {
		return nil, fmt.Errorf("%w：仅支持 PNG/JPEG/GIF/WebP/AVIF 图片及 MP4/WebM/MOV 视频", ErrStorageInvalid)
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	existing, e := s.repo.BySource(ctx, user, digest)
	if e == nil {
		if existing.Status != "active" || !existing.ExpiresAt.After(time.Now()) {
			return nil, ErrStorageExpired
		}
		p, e := s.path(existing.ID)
		if e != nil {
			return nil, e
		}
		if info, e := os.Stat(p); e != nil || info.Size() != existing.Bytes {
			return nil, ErrStorageUnavailable
		}
		return existing, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if e = f.Sync(); e != nil {
		return nil, e
	}
	if e = f.Close(); e != nil {
		return nil, e
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if len([]rune(name)) > 180 {
		name = string([]rune(name)[:180])
	}
	if name == "" || name == "." {
		name = kind
	}
	a := &RetainedMedia{ID: uuid.NewString(), UserID: user, SourceKey: digest, SHA256: digest, Kind: kind, Name: name, MimeType: mime, Bytes: size}
	dest, _ := s.path(a.ID)
	if e = os.Rename(f.Name(), dest); e != nil {
		return nil, e
	}
	directory, err := os.Open(s.dir)
	if err != nil {
		return nil, err
	}
	err = directory.Sync()
	directory.Close()
	if err != nil {
		return nil, err
	}
	// Do not unlink on an ambiguous DB commit error. An orphan is safer than lost paid content.
	stored, e := s.repo.Create(ctx, a)
	if e != nil {
		return nil, e
	}
	if stored.ID != a.ID {
		os.Remove(dest)
	}
	if stored.Status != "active" || !stored.ExpiresAt.After(time.Now()) {
		return nil, ErrStorageExpired
	}
	return stored, nil
}
func (s *RetainedMediaService) Open(ctx context.Context, user int64, id string) (*RetainedMedia, *os.File, error) {
	if !s.Enabled() {
		return nil, nil, ErrStorageUnavailable
	}
	if _, e := s.path(id); e != nil {
		return nil, nil, e
	}
	a, e := s.repo.Get(ctx, user, id)
	if e != nil {
		return nil, nil, e
	}
	if a.Status != "active" || !a.ExpiresAt.After(time.Now()) {
		return nil, nil, ErrStorageExpired
	}
	p, _ := s.path(id)
	f, e := os.Open(p)
	if e != nil {
		return nil, nil, ErrStorageUnavailable
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != a.Bytes {
		f.Close()
		return nil, nil, ErrStorageUnavailable
	}
	return a, f, nil
}
func (s *RetainedMediaService) Extend(ctx context.Context, user int64, id string, cmd StorageExtension) (*StorageReceipt, error) {
	if !s.Enabled() {
		return nil, ErrStorageUnavailable
	}
	// Ownership is checked by the repository; file existence is checked without exposing it to caller.
	if _, e := s.path(id); e != nil {
		return nil, e
	}
	a, e := s.repo.Get(ctx, user, id)
	if e != nil {
		return nil, e
	}
	p, _ := s.path(id)
	info, e := os.Stat(p)
	// Allow an idempotent receipt lookup even after cleanup. Repository cannot charge an expired asset.
	if e != nil || info.Size() != a.Bytes {
		if a.Status == "active" && a.ExpiresAt.After(time.Now()) {
			return nil, ErrStorageUnavailable
		}
	}
	return s.repo.Extend(ctx, user, id, cmd)
}
func (s *RetainedMediaService) List(ctx context.Context, user int64, offset int) ([]RetainedMedia, error) {
	return s.repo.List(ctx, user, offset)
}
func (s *RetainedMediaService) Orders(ctx context.Context, user int64, offset int) ([]StorageReceipt, error) {
	return s.repo.Orders(ctx, user, offset)
}
func (s *RetainedMediaService) cleanupLoop() {
	defer close(s.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			if !s.Enabled() || !s.cleanup {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, e := s.repo.Cleanup(ctx, func(id string) error {
				p, e := s.path(id)
				if e != nil {
					return e
				}
				e = os.Remove(p)
				if errors.Is(e, os.ErrNotExist) {
					return nil
				}
				return e
			})
			cancel()
			if e != nil {
				log.Printf("managed media cleanup failed: %v", e)
			}
		}
	}
}
