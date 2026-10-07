package optionsapi

import (
	"github.com/tylergannon/skgo"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Public", "public-literal")
	w.Header().Set("X-Late", "late-literal")
	w.Header().Set("X-Denied", "denied-literal")
	w.Header().Set("X-Default", "default-literal")
	w.Write([]byte("options fetched body"))
}

var _ = skgo.GET(get)
