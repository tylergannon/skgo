package skgo

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// liveKeepAlive is how long a quiet `query.live` connection waits before
// sending a `: keep-alive` comment, so proxies with an idle timeout do not
// close it. It is a variable only so tests can shorten it.
var liveKeepAlive = 30 * time.Second

type liveResultFrame struct {
	Type   string `json:"type"`
	Result string `json:"result"`
}

type liveRedirectFrame struct {
	Type     string `json:"type"`
	Location string `json:"location"`
}

type liveErrorFrame struct {
	Type  string     `json:"type"`
	Error *HTTPError `json:"error"`
}

// serveLive answers a `query.live` call with an SSE stream. The producer runs
// on its own goroutine and hands values back over a channel, so every write to
// the ResponseWriter happens here — a ResponseWriter is not safe to share.
func (rs *Remotes) serveLive(w http.ResponseWriter, r *http.Request, fn *Remote) {
	if r.Method != http.MethodGet {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  405,
			Message: "`query.live` functions must be invoked via GET request, not " + r.Method,
		}, http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		rs.writeError(w, &HTTPError{Status: 500, Message: "Internal Error"})
		return
	}

	arg, present, err := remotearg.ParsePayload(rawPayload(r.URL))
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	// A live query is a query: it may read cookies but never write them, so
	// nothing is ever added to the response headers here.
	ev := rs.newEvent(r, false)

	h := rs.header(w)
	h.Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Cancelling this context is the teardown signal: it stops the producer,
	// and the deferred cancel makes that happen exactly once however the
	// handler returns.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// The producer sees a context that ends when this handler does, so a
	// `query.live` blocked on ctx.Done() unwinds when the client disconnects.
	liveEvent := *ev
	liveEvent.req = r.WithContext(ctx)
	liveCtx := withEvent(ctx, &liveEvent)

	values := make(chan any)
	done := make(chan error, 1)

	go func() {
		done <- rs.callLive(liveCtx, fn, arg, present, func(v any) error {
			select {
			case values <- v:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	keepAlive := time.NewTimer(liveKeepAlive)
	defer keepAlive.Stop()

	var last string
	var hasLast bool

	for {
		select {
		case <-ctx.Done():
			// The client went away. Nothing more may be written, and a value
			// or error the producer emits from here on is ignored.
			return

		case <-keepAlive.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
			keepAlive.Reset(liveKeepAlive)

		case v := <-values:
			serialized, err := devalue.Stringify(v)
			if err != nil {
				rs.sendFrame(w, flusher, liveErrorFrame{Type: "error", Error: &HTTPError{Status: 500, Message: "Internal Error"}})
				return
			}
			// Consecutive identical frames are dropped, so a producer that
			// recomputes an unchanged value costs the client nothing.
			if hasLast && serialized == last {
				continue
			}
			last, hasLast = serialized, true
			if !rs.sendFrame(w, flusher, liveResultFrame{Type: "result", Result: serialized}) {
				return
			}
			if !keepAlive.Stop() {
				select {
				case <-keepAlive.C:
				default:
				}
			}
			keepAlive.Reset(liveKeepAlive)

		case err := <-done:
			if err == nil || ctx.Err() != nil {
				return
			}
			if redirect := asRedirect(err); redirect != nil {
				rs.sendFrame(w, flusher, liveRedirectFrame{Type: "redirect", Location: redirect.Location})
				return
			}
			rs.sendFrame(w, flusher, liveErrorFrame{Type: "error", Error: asHTTPError(err)})
			return
		}
	}
}

func (rs *Remotes) sendFrame(w http.ResponseWriter, flusher http.Flusher, frame any) bool {
	raw, err := jsonBytes(frame)
	if err != nil {
		return false
	}
	if _, err := w.Write(append(append([]byte("data: "), raw...), '\n', '\n')); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
