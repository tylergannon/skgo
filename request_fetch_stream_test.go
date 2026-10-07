package skgo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests hold a Go producer open — committed, first chunk flushed, then
// blocked on a release the test controls or on its own request context — and
// drive Event.Fetch from a real Go application body behind the composed
// handler. A fetch that waits for the producer to finish cannot pass any of
// them: the producer never finishes until the test says so. The producer's
// bytes, headers and statuses are literals in this file.

const streamWait = 3 * time.Second

type fetchedHead struct {
	status        int
	header        http.Header
	contentLength int64
	err           error
}

type readChunk struct {
	text string
	err  error
}

// streamApp is FetchConfig over a middleware over a router: /consume is the
// application body that fetches, everything else is the producer under test.
type streamApp struct {
	handler http.Handler
}

func streamMiddleware() Middleware {
	return func(ctx context.Context, e *Event, resolve Resolve) (*http.Response, error) {
		if !e.IsSubRequest() {
			*RequestLocals[mwUser](ctx) = "outer"
		}
		resp, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		resp.Header.Set("X-Mw-Sub", strconv.FormatBool(e.IsSubRequest()))
		return resp, nil
	}
}

func newStreamApp(t *testing.T, mw Middleware, producers, consume http.HandlerFunc) *streamApp {
	t.Helper()
	cfg := Manifest{AppDir: "_app"}.HandleConfig()
	cfg.Origin = mwOrigin
	cfg.OnPanic = func(string, any, []byte) {}
	var stack http.Handler
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mw == nil {
			// No hook layer: every request gets a bare request event.
			r = r.WithContext(withEvent(r.Context(), &Event{req: r, jar: newCookieJar(r, false)}))
		}
		if r.URL.Path == "/consume" {
			consume(w, r)
			return
		}
		producers(w, r)
	})
	stack = mw.Intercept(cfg, router)
	if mw != nil {
		stack = RequestMiddleware[struct{}, mwUser](nil).Intercept(cfg, emptyHookParams, stack)
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { stack.ServeHTTP(w, r) })
	app := &streamApp{handler: FetchConfig{Origin: mwOrigin, Handler: inner}.Intercept(stack)}
	return app
}

// start serves /consume in its own goroutine and returns when the outer
// response is finished, so a test can drive the producer meanwhile.
func (a *streamApp) start(ctx context.Context, target string, header http.Header) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	req := httptest.NewRequest("GET", target, nil).WithContext(ctx)
	req.Host = "127.0.0.1:8080"
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	go func() {
		rec := httptest.NewRecorder()
		a.handler.ServeHTTP(rec, req)
		done <- rec
	}()
	return done
}

func within[T any](t *testing.T, c <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(streamWait):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func signalled(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(streamWait):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func subGet(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func quietLog(t *testing.T) {
	t.Helper()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

func middlewares() map[string]Middleware {
	return map[string]Middleware{"no middleware": nil, "wrapping middleware": streamMiddleware()}
}

func TestInternalFetchDeliversHeadersAndFirstBytesWhileTheProducerIsStillRunning(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			producerDone := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			producers := func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/echo-cookie":
					_, _ = w.Write([]byte("cookie=" + r.Header.Get("Cookie")))
					return
				}
				defer close(producerDone)
				user := mwLocal(r.Context())
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Producer-User", "["+string(user)+"]")
				w.Header().Add("Set-Cookie", "sub=1; Path=/")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("first;"))
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = w.Write([]byte("second;"))
				case <-r.Context().Done():
				}
			}
			heads := make(chan fetchedHead, 1)
			reads := make(chan readChunk, 3)
			consume := func(w http.ResponseWriter, r *http.Request) {
				e := EventFrom(r.Context())
				resp, err := e.Fetch(r.Context(), subGet(t, "/produce"))
				if err != nil {
					heads <- fetchedHead{err: err}
					return
				}
				heads <- fetchedHead{status: resp.StatusCode, header: resp.Header.Clone(), contentLength: resp.ContentLength}
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				rest, err := io.ReadAll(resp.Body)
				reads <- readChunk{string(rest), err}
				_ = resp.Body.Close()
				next, err := e.Fetch(r.Context(), subGet(t, "/echo-cookie"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				body, _ := io.ReadAll(next.Body)
				_ = next.Body.Close()
				reads <- readChunk{string(body), nil}
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", http.Header{"Cookie": {"page=1"}})

			head := within(t, heads, "Fetch to return while the producer is still blocked")
			if head.err != nil {
				t.Fatalf("Fetch: %v", head.err)
			}
			if head.status != http.StatusAccepted || head.header.Get("Content-Type") != "text/plain; charset=utf-8" {
				t.Errorf("status/content-type = %d %q, want 202 text/plain; charset=utf-8", head.status, head.header.Get("Content-Type"))
			}
			if head.header.Get("X-Producer-User") != "[]" {
				t.Errorf("the producer saw locals %q; a subrequest starts with none", head.header.Get("X-Producer-User"))
			}
			if mw != nil && head.header.Get("X-Mw-Sub") != "true" {
				t.Errorf("the middleware did not see the subrequest as one (X-Mw-Sub = %q)", head.header.Get("X-Mw-Sub"))
			}
			first := within(t, reads, "the first chunk before the producer is released")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v; want first;", first.text, first.err)
			}
			select {
			case <-producerDone:
				t.Fatal("the producer finished before it was released")
			default:
			}
			close(release)
			rest := within(t, reads, "the rest of the body after release")
			if rest.err != nil || rest.text != "second;" {
				t.Errorf("rest = %q, %v; want second; then EOF", rest.text, rest.err)
			}
			signalled(t, producerDone, "the producer to finish")
			echoed := within(t, reads, "the follow-up fetch")
			if echoed.text != "cookie=page=1; sub=1" {
				t.Errorf("follow-up fetch sent %q, want the page's cookie and the one the stream's headers set", echoed.text)
			}
			rec := within(t, outer, "the outer response")
			if got := rec.Result().Header.Values("Set-Cookie"); mw != nil && (len(got) != 1 || !strings.HasPrefix(got[0], "sub=1")) {
				t.Errorf("the visitor's response carries %q, want the cookie the subrequest set", got)
			}
		})
	}
}

func TestInternalFetchCancelledBeforeCommitmentReleasesTheProducer(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			sawCancel := make(chan struct{})
			cancelNow := make(chan struct{})
			producers := func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-r.Context().Done()
				close(sawCancel)
			}
			heads := make(chan fetchedHead, 1)
			consume := func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				go func() {
					select {
					case <-cancelNow:
						cancel()
					case <-ctx.Done():
					}
				}()
				_, err := EventFrom(r.Context()).Fetch(ctx, subGet(t, "/hang"))
				heads <- fetchedHead{err: err}
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			signalled(t, entered, "the producer to start")
			close(cancelNow)
			head := within(t, heads, "Fetch to return after cancellation")
			if !errors.Is(head.err, context.Canceled) {
				t.Errorf("Fetch error = %v, want context canceled", head.err)
			}
			signalled(t, sawCancel, "the producer to see its context cancelled")
			within(t, outer, "the outer response")
		})
	}
}

func TestInternalFetchCancelledAfterTheFirstChunkFailsTheBlockedReaderAndReleasesTheProducer(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			sawCancel := make(chan struct{})
			cancelNow := make(chan struct{})
			producers := func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("first;"))
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(sawCancel)
			}
			reads := make(chan readChunk, 3)
			blocked := make(chan struct{})
			consume := func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				resp, err := EventFrom(r.Context()).Fetch(ctx, subGet(t, "/stream"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				defer resp.Body.Close()
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				go func() { <-cancelNow; cancel() }()
				close(blocked)
				rest, err := io.ReadAll(resp.Body)
				reads <- readChunk{string(rest), err}
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			first := within(t, reads, "the first chunk")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v", first.text, first.err)
			}
			signalled(t, blocked, "the reader to block")
			close(cancelNow)
			rest := within(t, reads, "the blocked reader to be released")
			if !errors.Is(rest.err, context.Canceled) {
				t.Errorf("read after cancellation = %q, %v; want context canceled, never a clean EOF", rest.text, rest.err)
			}
			signalled(t, sawCancel, "the producer to see its context cancelled")
			within(t, outer, "the outer response")
		})
	}
}

func TestClosingAnInternalFetchBodyStopsABlockedProducerWrite(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			closeNow := make(chan struct{})
			writeFailed := make(chan error, 1)
			producers := func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("first;"))
				w.(http.Flusher).Flush()
				for {
					if _, err := w.Write([]byte("more;")); err != nil {
						writeFailed <- err
						return
					}
				}
			}
			reads := make(chan readChunk, 1)
			closed := make(chan error, 1)
			consume := func(w http.ResponseWriter, r *http.Request) {
				resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/stream"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				<-closeNow
				closed <- resp.Body.Close()
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			first := within(t, reads, "the first chunk")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v", first.text, first.err)
			}
			close(closeNow)
			if err := within(t, closed, "Close to return"); err != nil {
				t.Errorf("Close = %v", err)
			}
			if err := within(t, writeFailed, "the producer's blocked write to fail"); err == nil {
				t.Error("the producer's write succeeded after the body was closed")
			}
			within(t, outer, "the outer response")
		})
	}
}

func TestAbandoningAnInternalFetchBodyByCancellationStopsABlockedProducerWrite(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			cancelNow := make(chan struct{})
			writeFailed := make(chan error, 1)
			producers := func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("first;"))
				w.(http.Flusher).Flush()
				for {
					if _, err := w.Write([]byte("more;")); err != nil {
						writeFailed <- err
						return
					}
				}
			}
			reads := make(chan readChunk, 1)
			consume := func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				resp, err := EventFrom(r.Context()).Fetch(ctx, subGet(t, "/stream"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				<-cancelNow
				cancel() // abandoned: the body is neither read further nor closed
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			first := within(t, reads, "the first chunk")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v", first.text, first.err)
			}
			close(cancelNow)
			if err := within(t, writeFailed, "the abandoned producer's blocked write to fail"); err == nil {
				t.Error("the producer's write succeeded after the fetch was abandoned")
			}
			within(t, outer, "the outer response")
		})
	}
}

func TestInternalFetchOfAPanickingProducerHasADefinedOutcome(t *testing.T) {
	quietLog(t)

	t.Run("before commitment the fetch fails and does not hang", func(t *testing.T) {
		producers := func(w http.ResponseWriter, r *http.Request) { panic("early") }
		heads := make(chan fetchedHead, 1)
		consume := func(w http.ResponseWriter, r *http.Request) {
			_, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/boom"))
			heads <- fetchedHead{err: err}
		}
		app := newStreamApp(t, nil, producers, consume)
		outer := app.start(context.Background(), "/consume", nil)
		head := within(t, heads, "Fetch to return")
		if head.err == nil || !strings.Contains(head.err.Error(), "panicked") {
			t.Errorf("Fetch error = %v, want one that says the handler panicked", head.err)
		}
		within(t, outer, "the outer response")
	})

	t.Run("a middleware turns it into kit's opaque 500", func(t *testing.T) {
		producers := func(w http.ResponseWriter, r *http.Request) { panic("early") }
		heads := make(chan fetchedHead, 1)
		consume := func(w http.ResponseWriter, r *http.Request) {
			resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/boom"))
			if err != nil {
				heads <- fetchedHead{err: err}
				return
			}
			_ = resp.Body.Close()
			heads <- fetchedHead{status: resp.StatusCode}
		}
		app := newStreamApp(t, streamMiddleware(), producers, consume)
		outer := app.start(context.Background(), "/consume", nil)
		if head := within(t, heads, "Fetch to return"); head.err != nil || head.status != http.StatusInternalServerError {
			t.Errorf("Fetch = %d, %v; want a 500 response", head.status, head.err)
		}
		within(t, outer, "the outer response")
	})

	for name, mw := range middlewares() {
		t.Run("after a committed chunk the body fails, never ends cleanly: "+name, func(t *testing.T) {
			producers := func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("first;"))
				w.(http.Flusher).Flush()
				panic(http.ErrAbortHandler)
			}
			reads := make(chan readChunk, 2)
			consume := func(w http.ResponseWriter, r *http.Request) {
				resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/late"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				defer resp.Body.Close()
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				rest, err := io.ReadAll(resp.Body)
				reads <- readChunk{string(rest), err}
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			first := within(t, reads, "the first chunk")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v", first.text, first.err)
			}
			if rest := within(t, reads, "the body to fail"); rest.err == nil {
				t.Errorf("a body whose producer panicked ended cleanly with %q", rest.text)
			}
			within(t, outer, "the outer response")
		})
	}
}

func TestInternalFetchKeepsTheContractOfFiniteAndBodylessAnswers(t *testing.T) {
	producerDone := make(chan string, 4)
	producers := func(w http.ResponseWriter, r *http.Request) {
		defer func() { producerDone <- r.Method + " " + r.URL.Path }()
		switch r.URL.Path {
		case "/finite":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "11")
			_, _ = w.Write([]byte("hello world"))
		case "/nothing":
			w.WriteHeader(http.StatusNoContent)
		case "/silent":
		case "/withbody":
			w.Header().Set("X-Head", "yes")
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("must not be delivered to a HEAD"))
		}
	}
	type result struct {
		status int
		cl     int64
		body   string
		header string
		err    error
	}
	results := make(chan result, 8)
	consume := func(w http.ResponseWriter, r *http.Request) {
		for _, c := range []struct{ method, path string }{{"GET", "/finite"}, {"GET", "/nothing"}, {"GET", "/silent"}, {"HEAD", "/withbody"}} {
			req, _ := http.NewRequest(c.method, c.path, nil)
			resp, err := EventFrom(r.Context()).Fetch(r.Context(), req)
			if err != nil {
				results <- result{err: err}
				continue
			}
			b, rerr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			results <- result{resp.StatusCode, resp.ContentLength, string(b), resp.Header.Get("X-Head"), rerr}
		}
	}
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			want := []result{
				{status: 200, cl: 11, body: "hello world"},
				{status: 204, cl: -1},
				{status: 200, cl: -1},
				{status: 200, cl: -1, header: "yes"},
			}
			for i, w := range want {
				got := within(t, results, fmt.Sprintf("result %d", i))
				if got.err != nil || got.status != w.status || got.body != w.body || got.header != w.header {
					t.Errorf("fetch %d = %+v, want %+v", i, got, w)
				}
				if mw == nil && got.cl != w.cl {
					t.Errorf("fetch %d ContentLength = %d, want %d", i, got.cl, w.cl)
				}
			}
			for i := 0; i < 4; i++ {
				within(t, producerDone, "a producer to run to completion")
			}
			within(t, outer, "the outer response")
		})
	}
}

func TestConcurrentInternalFetchesStreamIndependently(t *testing.T) {
	const n = 4
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			producers := func(w http.ResponseWriter, r *http.Request) {
				id := r.URL.Query().Get("id")
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("first-" + id + ";"))
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = w.Write([]byte("second-" + id + ";"))
				case <-r.Context().Done():
				}
			}
			type got struct {
				id, first, rest string
				err             error
			}
			firsts := make(chan got, n)
			rests := make(chan got, n)
			consume := func(w http.ResponseWriter, r *http.Request) {
				id := r.URL.Query().Get("id")
				resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/produce?id="+id))
				if err != nil {
					firsts <- got{id: id, err: err}
					return
				}
				defer resp.Body.Close()
				first := make([]byte, len("first-0;"))
				_, err = io.ReadFull(resp.Body, first)
				firsts <- got{id: id, first: string(first), err: err}
				rest, err := io.ReadAll(resp.Body)
				rests <- got{id: id, rest: string(rest), err: err}
			}
			app := newStreamApp(t, mw, producers, consume)
			var outers []<-chan *httptest.ResponseRecorder
			for i := 0; i < n; i++ {
				outers = append(outers, app.start(context.Background(), "/consume?id="+strconv.Itoa(i), nil))
			}
			for i := 0; i < n; i++ {
				g := within(t, firsts, "every request's first chunk while all producers are blocked")
				if g.err != nil || g.first != "first-"+g.id+";" {
					t.Errorf("request %s read %q, %v", g.id, g.first, g.err)
				}
			}
			close(release)
			for i := 0; i < n; i++ {
				g := within(t, rests, "every request's remainder")
				if g.err != nil || g.rest != "second-"+g.id+";" {
					t.Errorf("request %s remainder %q, %v", g.id, g.rest, g.err)
				}
			}
			for _, o := range outers {
				within(t, o, "an outer response")
			}
		})
	}
}

func TestNestedInternalFetchStreamsThroughEachLevelAtDepth(t *testing.T) {
	for name, mw := range middlewares() {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			var depths sync.Map
			producers := func(w http.ResponseWriter, r *http.Request) {
				depths.Store(r.URL.Path, fetchDepthOf(r.Context()))
				switch r.URL.Path {
				case "/inner":
					w.Header().Set("Content-Type", "text/plain")
					_, _ = w.Write([]byte("first;"))
					w.(http.Flusher).Flush()
					select {
					case <-release:
						_, _ = w.Write([]byte("second;"))
					case <-r.Context().Done():
					}
				case "/middle":
					resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/inner"))
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadGateway)
						return
					}
					defer resp.Body.Close()
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(resp.StatusCode)
					buf := make([]byte, 64)
					for {
						n, err := resp.Body.Read(buf)
						if n > 0 {
							_, _ = w.Write(buf[:n])
							w.(http.Flusher).Flush()
						}
						if err != nil {
							return
						}
					}
				}
			}
			reads := make(chan readChunk, 3)
			consume := func(w http.ResponseWriter, r *http.Request) {
				resp, err := EventFrom(r.Context()).Fetch(r.Context(), subGet(t, "/middle"))
				if err != nil {
					reads <- readChunk{err: err}
					return
				}
				defer resp.Body.Close()
				first := make([]byte, len("first;"))
				_, err = io.ReadFull(resp.Body, first)
				reads <- readChunk{string(first), err}
				rest, err := io.ReadAll(resp.Body)
				reads <- readChunk{string(rest), err}
			}
			app := newStreamApp(t, mw, producers, consume)
			outer := app.start(context.Background(), "/consume", nil)
			first := within(t, reads, "the first chunk through two levels of fetch")
			if first.err != nil || first.text != "first;" {
				t.Fatalf("first read = %q, %v", first.text, first.err)
			}
			close(release)
			rest := within(t, reads, "the rest")
			if rest.err != nil || rest.text != "second;" {
				t.Errorf("rest = %q, %v", rest.text, rest.err)
			}
			within(t, outer, "the outer response")
			for path, want := range map[string]int{"/middle": 1, "/inner": 2} {
				if v, _ := depths.Load(path); v != want {
					t.Errorf("%s ran at fetch depth %v, want %d", path, v, want)
				}
			}
		})
	}
}
