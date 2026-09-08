package ssrcontrol

import (
	"encoding/json"
	"net/http"
	"path"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

func status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(businesslogic.RenderProbe(path.Base(r.URL.Path)).Status())
}

func release(w http.ResponseWriter, r *http.Request) {
	businesslogic.RenderProbe(path.Base(r.URL.Path)).Release(r.URL.Query().Get("phase"))
	w.WriteHeader(http.StatusNoContent)
}

var _ = skgo.GET(status)
var _ = skgo.POST(release)
