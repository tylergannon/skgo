package actions

import (
	"encoding/json"
	"net/http"

	"github.com/tylergannon/skgo"
)

// This sibling +server route answers a POST only when Kit selects the endpoint.
func endpointPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"answer": "Endpoint POST answered by Go"})
}

var _ = skgo.POST(endpointPost)
