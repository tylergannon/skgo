// Package shadowrest is a dynamic server route whose pattern also matches the
// path of a prerendered page. The prerendered file has to win over it.
package shadowrest

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("dynamic-rest:" + skgo.EventFrom(r.Context()).Param("rest")))
}

var _ = skgo.GET(get)
