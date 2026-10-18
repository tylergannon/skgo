package reroutetarget

import (
	"github.com/tylergannon/skgo"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	target := "/rerouted/73"
	if r.URL.Query().Get("name") == "endpoint" {
		target = "/api/rerouted/91"
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(target))
}

var _ = skgo.GET(get)
