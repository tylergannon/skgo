package reroutedendpoint

import (
	"fmt"
	"github.com/tylergannon/skgo"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	event := skgo.EventFrom(r.Context())
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "endpoint|route=%s|param=%s|url=%s", event.RouteID(), event.Param("number"), event.URL().RequestURI())
}

var _ = skgo.GET(get)
