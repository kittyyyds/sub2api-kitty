package service

import (
	"context"
	"io"
	"sync"
	"time"
)

// Media retrieval is cancellable independently of generation/settlement.
// The idle timer bounds an upstream that sends headers then stops sending data.
func mediaDownloadContext(parent context.Context, total, idle time.Duration) (context.Context, func(), func()) {
	ctx, cancel := context.WithTimeout(parent, total)
	var mu sync.Mutex
	closed := false
	timer := time.AfterFunc(idle, cancel)
	progress := func() {
		mu.Lock()
		defer mu.Unlock()
		if !closed && ctx.Err() == nil {
			timer.Reset(idle)
		}
	}
	release := func() {
		mu.Lock()
		closed = true
		timer.Stop()
		mu.Unlock()
		cancel()
	}
	return ctx, progress, release
}

type mediaDownloadBody struct {
	io.ReadCloser
	progress func()
}

func (b *mediaDownloadBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.progress()
	}
	return n, err
}
