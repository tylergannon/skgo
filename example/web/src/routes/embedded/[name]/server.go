// Package embedded serves the text files that sit beside it. A route's Go file
// embeds its neighbours with Go's own embed, even from a directory such as
// [name] that Go cannot spell, and serves them with net/http; skgo adds no API
// for reading files.
package embedded

import (
	"embed"
	"net/http"

	"github.com/tylergannon/skgo"
)

//go:embed *.txt
var texts embed.FS

func get(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, texts, skgo.EventFrom(r.Context()).Param("name"))
}

var _ = skgo.GET(get)
