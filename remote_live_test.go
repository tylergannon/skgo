package skgo

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingWriter records how many SSE frames the handler wrote, so a test can
// prove that nothing was written after teardown.
type countingWriter struct {
	http.ResponseWriter
	frames *atomic.Int32
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), "data: ") {
		c.frames.Add(1)
	}
	return c.ResponseWriter.Write(p)
}

func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// liveServer runs a registry behind a real HTTP server. A live query must
// never be tested through httptest.ResponseRecorder: it is not safe to read
// while the handler is still writing.
func liveServer(t *testing.T, fn *Remote) (*httptest.Server, *atomic.Int32, <-chan struct{}) {
	t.Helper()
	rs := testRemotes(t, RemoteConfig{}, fn)

	frames := &atomic.Int32{}
	handlerDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		rs.ServeHTTP(&countingWriter{ResponseWriter: w, frames: frames}, r)
	}))
	t.Cleanup(srv.Close)

	return srv, frames, handlerDone
}

func openLive(t *testing.T, srv *httptest.Server, fn *Remote) (*bufio.Reader, context.CancelFunc) {
	t.Helper()
	rs := testRemotes(t, RemoteConfig{}, fn)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+rs.Prefix()+fn.ID(), nil)
	if err != nil {
		cancel()
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("opening the stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("cache-control = %q", got)
	}

	return bufio.NewReader(resp.Body), cancel
}

// readFrame reads one SSE frame, terminator included.
func readFrame(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	var sb strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading a frame (got %q so far): %v", sb.String(), err)
		}
		sb.WriteString(line)
		if line == "\n" {
			return sb.String()
		}
	}
}

func TestLiveFirstFrameIsGolden(t *testing.T) {
	fn := NewLiveQueryNoArg(testModule, "watchCount", func(ctx context.Context, yield func(string) error) error {
		if err := yield("initial"); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})
	srv, _, _ := liveServer(t, fn)
	br, cancel := openLive(t, srv, fn)
	defer cancel()

	want := "data: {\"type\":\"result\",\"result\":\"[\\\"initial\\\"]\"}\n\n"
	if got := readFrame(t, br); got != want {
		t.Errorf("first frame = %q, want %q", got, want)
	}
}

func TestLiveDedupesUnchangedFrames(t *testing.T) {
	fn := NewLiveQueryNoArg(testModule, "watchCount", func(ctx context.Context, yield func(string) error) error {
		for _, v := range []string{"a", "a", "b"} {
			if err := yield(v); err != nil {
				return err
			}
		}
		<-ctx.Done()
		return nil
	})
	srv, _, _ := liveServer(t, fn)
	br, cancel := openLive(t, srv, fn)
	defer cancel()

	first := readFrame(t, br)
	if !strings.Contains(first, `\"a\"`) {
		t.Fatalf("first frame = %q", first)
	}
	// The repeated "a" is dropped, so the next frame is "b".
	second := readFrame(t, br)
	if !strings.Contains(second, `\"b\"`) {
		t.Errorf("second frame = %q, want the \"b\" value (the repeat should be deduped)", second)
	}
}

func TestLiveSendsKeepAlive(t *testing.T) {
	previous := liveKeepAlive
	liveKeepAlive = 20 * time.Millisecond
	t.Cleanup(func() { liveKeepAlive = previous })

	fn := NewLiveQueryNoArg(testModule, "watchCount", func(ctx context.Context, yield func(string) error) error {
		if err := yield("initial"); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})
	srv, _, _ := liveServer(t, fn)
	br, cancel := openLive(t, srv, fn)
	defer cancel()

	if got := readFrame(t, br); !strings.HasPrefix(got, "data: ") {
		t.Fatalf("first frame = %q", got)
	}
	if got := readFrame(t, br); got != ": keep-alive\n\n" {
		t.Errorf("second frame = %q, want a keep-alive comment", got)
	}
}

func TestLiveCancellationTearsDownOnceAndDropsLateValues(t *testing.T) {
	var cleanups atomic.Int32
	lateYield := make(chan error, 1)

	fn := NewLiveQueryNoArg(testModule, "watchCount", func(ctx context.Context, yield func(string) error) error {
		defer cleanups.Add(1)
		if err := yield("initial"); err != nil {
			return err
		}
		<-ctx.Done()
		// A value produced after cancellation must be refused, and the error
		// this function then returns must never reach the client.
		lateYield <- yield("too late")
		return errors.New("boom")
	})

	srv, frames, handlerDone := liveServer(t, fn)
	br, cancel := openLive(t, srv, fn)

	readFrame(t, br)
	cancel()

	select {
	case err := <-lateYield:
		if err == nil {
			t.Error("yield after cancellation returned nil, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the producer never observed cancellation")
	}

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler did not return after cancellation")
	}

	if got := cleanups.Load(); got != 1 {
		t.Errorf("cleanup ran %d times, want exactly 1", got)
	}
	if got := frames.Load(); got != 1 {
		t.Errorf("%d frames written, want only the initial value (no error frame after teardown)", got)
	}
}

func TestLiveRejectsNonGet(t *testing.T) {
	fn := NewLiveQueryNoArg(testModule, "watchCount", func(ctx context.Context, yield func(string) error) error {
		return nil
	})
	rs := testRemotes(t, RemoteConfig{}, fn)

	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, rs.Prefix()+fn.ID(), strings.NewReader("{}")))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	kind, _, httpErr := envelope(t, rec.Body.Bytes())
	if kind != "error" || httpErr["status"] != float64(405) {
		t.Errorf("envelope = %q %#v", kind, httpErr)
	}
}
