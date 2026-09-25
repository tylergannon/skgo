package defaultaction

import (
	"encoding/json"
	"net/http"

	"github.com/tylergannon/skgo"
)

// The sibling endpoint deliberately has no POST; Kit falls back to the page action.
func endpointGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"answer": "Default sibling GET answered by Go"})
}

var _ = skgo.GET(endpointGet)
