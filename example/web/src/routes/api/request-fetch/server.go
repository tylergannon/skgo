// Package requestfetchapi is the service the /request-fetch page's Go load
// calls with Event.Fetch. It is an ordinary endpoint: it answers a browser or
// curl exactly as it answers the load's in-process subrequest.
package requestfetchapi

import (
	"encoding/json"
	"net/http"

	app "github.com/tylergannon/skgo/example/internal/app"

	"github.com/tylergannon/skgo"
)

// Fact is the one value the page shows. Nothing else in the app produces it.
const Fact = "harbour-lamp-4096"

func fact(w http.ResponseWriter, r *http.Request) {
	session := app.LocalsFrom(r.Context()).Session
	visitor := "guest"
	if session.User != "" {
		visitor = session.User
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"fact": Fact, "visitor": visitor})
}

var _ = skgo.GET(fact)
