package godevendpoint

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

// get is the endpoint the development proof edits while the server runs: its
// status, body and header are literals the proof writes.
func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Go-Revision", "one")
	_, _ = w.Write([]byte("Go endpoint revision one"))
}

var _ = skgo.GET(get)
