//go:build skgo_qualification

package example_test

import (
	"net/http"
	"testing"
)

// The /embedded/[name] route's Go file embeds the text files beside it, in a
// directory Go cannot spell, and its GET handler serves them with net/http.
// The bytes below are the fixture file's, stated again here.
func TestARouteServesTheFileItsGoFileEmbeds(t *testing.T) {
	h := newProdHandler(t)

	rec := get(t, h, "/embedded/greeting.txt")
	const want = "lantern-172: embedded beside [name]\n"
	if rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Fatalf("GET /embedded/greeting.txt = %d %q, want 200 %q", rec.Code, rec.Body.String(), want)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "36" {
		t.Fatalf("Content-Length = %q, want 36", got)
	}

	// Only what the pattern embedded is there: the route's own Go source is
	// beside it on disk, but not in the binary.
	if rec := get(t, h, "/embedded/server.go"); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /embedded/server.go = %d %q, want 404", rec.Code, rec.Body.String())
	}
}
