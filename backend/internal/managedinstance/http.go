package managedinstance

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

func WrapHTTP(next http.Handler) http.Handler {
	if c := Current(); c != nil {
		return c.WrapHTTP(next)
	}
	return next
}
func (c *Controller) WrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.http++
		c.mu.Unlock()
		t := &trackedWriter{ResponseWriter: w, c: c, ctx: r.Context()}
		defer func() {
			if t.stopNotify != nil {
				t.stopNotify()
			}
			c.mu.Lock()
			if !t.hijacked {
				c.http--
			}
			if t.sse {
				c.sse--
			}
			c.mu.Unlock()
		}()
		next.ServeHTTP(t, r)
	})
}

type trackedWriter struct {
	http.ResponseWriter
	c                      *Controller
	hijacked, sse, written bool
	ctx                    context.Context
	notifyOnce             sync.Once
	notify                 chan bool
	stopNotify             func() bool
}

func (w *trackedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *trackedWriter) CloseNotify() <-chan bool {
	if notifier, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	w.notifyOnce.Do(func() {
		w.notify = make(chan bool, 1)
		w.stopNotify = context.AfterFunc(w.ctx, func() { w.notify <- true })
	})
	return w.notify
}
func (w *trackedWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
func (w *trackedWriter) mark() {
	if w.written {
		return
	}
	w.written = true
	if strings.HasPrefix(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		w.c.mu.Lock()
		w.c.sse++
		w.sse = true
		w.c.mu.Unlock()
	}
}
func (w *trackedWriter) WriteHeader(code int) {
	if code >= 200 {
		w.mark()
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *trackedWriter) Write(b []byte) (int, error) { w.mark(); return w.ResponseWriter.Write(b) }
func (w *trackedWriter) Flush()                      { w.mark(); _ = http.NewResponseController(w.ResponseWriter).Flush() }
func (w *trackedWriter) FlushError() error {
	w.mark()
	return http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *trackedWriter) ReadFrom(r io.Reader) (int64, error) {
	w.mark()
	return io.Copy(struct{ io.Writer }{w.ResponseWriter}, r)
}
func (w *trackedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.hijacked {
		return nil, nil, errors.New("connection already hijacked")
	}
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.c.mu.Lock()
	w.hijacked = true
	w.c.http--
	w.c.ws++
	w.c.mu.Unlock()
	return &trackedConn{Conn: conn, c: w.c}, rw, nil
}

type trackedConn struct {
	net.Conn
	c    *Controller
	once sync.Once
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.c.mu.Lock(); c.c.ws--; c.c.mu.Unlock() })
	return err
}
