package skgo

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
)

// errDownstreamPanic is what a body read reports when the application
// panicked after it had already begun its response.
var errDownstreamPanic = errors.New("skgo: the application panicked while writing its response")

// panicWithStack carries a downstream panic, and the stack it happened on,
// across the goroutine boundary to the middleware that called resolve.
type panicWithStack struct {
	value any
	stack []byte
}

type frame struct {
	data  []byte
	flush bool
}

// downstream is the application's real response while a middleware still holds
// it. The application runs in its own goroutine and hands its output over a
// channel, a chunk at a time, so after-logic sees the status and headers as
// soon as the application commits them and never has to wait for — or hold —
// a streamed body.
type downstream struct {
	frames chan frame
	done   chan struct{}
	ended  chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	stop   func() bool

	// detached is a fetch's response rather than a middleware's. Closing its
	// body lets go of the application without waiting for it to return, so a
	// caller is not held by a handler that ignores its context; and its reads
	// and writes end with its context, so a handler blocked writing to a reader
	// that was abandoned is released by the cancellation alone. A
	// middleware's downstream does neither: its application may still answer a
	// request whose context has been cancelled.
	detached bool

	closeOnce sync.Once

	committed  chan struct{}
	commitOnce sync.Once
	status     int
	header     http.Header

	mu        sync.Mutex
	panicked  *panicWithStack
	preCommit bool
	// cancelled is the context's error as the application returned: a
	// response that ended because the request was cancelled, which a reader
	// must not mistake for one that was complete.
	cancelled error
}

// startDownstream runs next against r (with ctx's values, and cancelled when
// either ctx or the returned downstream is) and returns once next has
// committed its status and headers.
//
// A panic before that point is raised here, in the caller, where the
// middleware's guard turns it into kit's opaque 500; a panic after it is
// carried through the body.
func startDownstream(rctx, base context.Context, next http.Handler, r *http.Request) (*downstream, *http.Response) {
	d := launchDownstream(rctx, base, next, r, false)
	<-d.committed

	d.mu.Lock()
	p, pre := d.panicked, d.preCommit
	d.mu.Unlock()
	if p != nil && pre {
		d.close()
		panic(p)
	}
	return d, d.response(r)
}

// launchDownstream starts the application and returns at once, before it has
// committed anything. d.committed closes when it has.
func launchDownstream(rctx, base context.Context, next http.Handler, r *http.Request, detached bool) *downstream {
	dctx, cancel := context.WithCancel(base)
	d := &downstream{
		frames:    make(chan frame),
		done:      make(chan struct{}),
		ended:     make(chan struct{}),
		committed: make(chan struct{}),
		ctx:       dctx,
		detached:  detached,
		cancel:    cancel,
	}
	if rctx != base {
		d.stop = context.AfterFunc(rctx, cancel)
	}
	dr := r.WithContext(dctx)
	w := &downstreamWriter{d: d, header: http.Header{}}

	go func() {
		defer close(d.ended)
		defer close(d.frames)
		defer func() {
			if value := recover(); value != nil {
				p := &panicWithStack{value: value, stack: debug.Stack()}
				pre := !w.wasCommitted()
				d.mu.Lock()
				d.panicked, d.preCommit = p, pre
				d.mu.Unlock()
				if !pre && value != http.ErrAbortHandler {
					log.Printf("skgo: handler panicked while writing its response: %v\n%s", value, p.stack)
				}
			}
			d.mu.Lock()
			if detached {
				d.cancelled = dctx.Err()
			}
			d.mu.Unlock()
			w.commit(http.StatusOK, nil)
		}()
		next.ServeHTTP(w, dr)
	}()
	return d
}

// response is what the application committed, as an *http.Response whose body
// is the rest of it.
func (d *downstream) response(r *http.Request) *http.Response {
	return &http.Response{
		Status:        http.StatusText(d.status),
		StatusCode:    d.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        d.header,
		Body:          &downstreamBody{d: d},
		ContentLength: -1,
		Request:       r,
	}
}

// close abandons the application's response: its context is cancelled, its
// writes start failing, and close returns only when its goroutine has, since
// the request it holds must not outlive this one.
func (d *downstream) close() {
	d.release()
	<-d.ended
}

// release abandons the application's response without waiting for it to
// return.
func (d *downstream) release() {
	d.closeOnce.Do(func() {
		if d.stop != nil {
			d.stop()
		}
		close(d.done)
		d.cancel()
	})
}

func (d *downstream) ctxDone() <-chan struct{} {
	if d.detached {
		return d.ctx.Done()
	}
	return nil
}

// discard lets the application run to its end with nobody reading what it
// writes, as a server does for a HEAD request.
func (d *downstream) discard() {
	go func() {
		for range d.frames {
		}
	}()
}

type downstreamWriter struct {
	d      *downstream
	header http.Header

	mu        sync.Mutex
	committed bool
}

func (w *downstreamWriter) Header() http.Header { return w.header }

func (w *downstreamWriter) wasCommitted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.committed
}

// commit snapshots the status and headers the first time anything is written,
// flushed or returned, as net/http does: later edits to the header map are not
// part of the response.
func (w *downstreamWriter) commit(status int, first []byte) {
	w.mu.Lock()
	if w.committed {
		w.mu.Unlock()
		return
	}
	w.committed = true
	w.mu.Unlock()

	header := w.header.Clone()
	if _, ok := header["Content-Type"]; !ok && len(first) > 0 && status != http.StatusNoContent && status != http.StatusNotModified {
		if header.Get("Transfer-Encoding") == "" {
			header.Set("Content-Type", http.DetectContentType(first))
		}
	}
	w.d.commitOnce.Do(func() {
		w.d.status, w.d.header = status, header
		close(w.d.committed)
	})
}

func (w *downstreamWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		return
	}
	w.commit(status, nil)
}

func (w *downstreamWriter) send(f frame) error {
	select {
	case w.d.frames <- f:
		return nil
	case <-w.d.done:
		return io.ErrClosedPipe
	case <-w.d.ctxDone():
		return w.d.ctx.Err()
	}
}

func (w *downstreamWriter) Write(p []byte) (int, error) {
	w.commit(http.StatusOK, p)
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.send(frame{data: append([]byte(nil), p...)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *downstreamWriter) Flush() {
	w.commit(http.StatusOK, nil)
	_ = w.send(frame{flush: true})
}

// downstreamBody is the application's body as the middleware receives it. It
// reads a chunk at a time — whatever the application last wrote — so a
// streamed response is visible as it is produced.
type downstreamBody struct {
	d   *downstream
	cur []byte
	eof bool
}

// readFlush is Read that also reports a flush the application made, so the
// connection can be flushed exactly where the application flushed.
func (b *downstreamBody) readFlush(p []byte) (int, bool, error) {
	if len(b.cur) > 0 {
		n := copy(p, b.cur)
		b.cur = b.cur[n:]
		return n, false, nil
	}
	if b.eof {
		return 0, false, io.EOF
	}
	select {
	case f, ok := <-b.d.frames:
		if !ok {
			b.eof = true
			b.d.mu.Lock()
			midBody := b.d.panicked != nil && !b.d.preCommit
			cancelled := b.d.cancelled
			b.d.mu.Unlock()
			if midBody {
				return 0, false, errDownstreamPanic
			}
			if cancelled != nil {
				return 0, false, cancelled
			}
			return 0, false, io.EOF
		}
		if f.flush {
			return 0, true, nil
		}
		n := copy(p, f.data)
		b.cur = f.data[n:]
		return n, false, nil
	case <-b.d.done:
		return 0, false, io.ErrClosedPipe
	case <-b.d.ctxDone():
		return 0, false, b.d.ctx.Err()
	}
}

func (b *downstreamBody) Read(p []byte) (int, error) {
	for {
		if n, _, err := b.readFlush(p); n > 0 || err != nil {
			return n, err
		}
	}
}

func (b *downstreamBody) Close() error {
	if b.d.detached {
		b.d.release()
	} else {
		b.d.close()
	}
	return nil
}
