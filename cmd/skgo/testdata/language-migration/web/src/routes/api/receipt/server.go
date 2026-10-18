package receipt

import (
	"io"
	"net/http"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, "Go endpoint: migration")
}

var _ = skgo.GET(get)
