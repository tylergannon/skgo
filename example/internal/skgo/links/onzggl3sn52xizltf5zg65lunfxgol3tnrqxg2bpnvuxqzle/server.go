package routing

import (
	"github.com/tylergannon/skgo"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("mixed-endpoint|" + r.URL.Path))
}

var _ = skgo.GET(get)
