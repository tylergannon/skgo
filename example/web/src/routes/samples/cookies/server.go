// Package cookies is a server route that writes cookies on its own response.
// A +server route's event refuses SetCookie, so it uses net/http instead.
package cookies

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

// get sets two cookies. http.Cookie has no defaults, so these name only Path:
// neither cookie is HttpOnly or Secure, and neither has a SameSite attribute.
func get(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "lamp", Value: "glow", Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "other", Value: "dim", Path: "/"})
	w.WriteHeader(http.StatusNoContent)
}

var _ = skgo.GET(get)
