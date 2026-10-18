package example_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/web"
)

// testOrigin is the origin the handler under test is built with. Nothing
// listens on it: every request is served in-process.
const testOrigin = "http://127.0.0.1:8080"

// newTestHandler builds the production stack over the embedded build: the same
// NewHandler that cmd/main.go serves.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newHandlerFor(t, testOrigin)
}

// newHandlerFor builds the same stack for another origin, such as a test
// server's own address.
func newHandlerFor(t *testing.T, origin string) http.Handler {
	t.Helper()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	h, err := example.NewHandler(dist, origin)
	if err != nil {
		t.Fatalf("building the handler: %v", err)
	}
	return h
}

// TestAPageRendersThroughTheProductionHandler renders a real page, Go load
// and embedded renderer included, without opening a socket.
func TestAPageRendersThroughTheProductionHandler(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /items/42 = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Widget 42") {
		t.Fatalf("the rendered page does not name Widget 42:\n%s", rec.Body.String())
	}
}
