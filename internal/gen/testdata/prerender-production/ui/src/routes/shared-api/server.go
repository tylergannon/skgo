package sharedapi

import (
	"github.com/tylergannon/skgo"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Default", "default-literal")
	w.Header().Set("X-Denied", "literal-private-header-275")
	if r.Header.Get("X-Fixture-Go") == "" {
		w.Header().Set("Set-Cookie", "shared=literal-cookie-278")
	}
	w.Write([]byte("literal-api-278"))
}

var _ = skgo.GET(get)
