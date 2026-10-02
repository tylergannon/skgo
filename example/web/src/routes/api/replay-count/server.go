// Package replaycount answers how many requests a run has made, so a scenario
// can state a number of invocations that nothing the page renders produced.
package replaycount

import (
	"encoding/json"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

func counts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(businesslogic.Replays.Counts(r.URL.Query().Get("run")))
}

var _ = skgo.GET(counts)
