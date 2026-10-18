// Package missing answers every path under /missing with Kit's not-found page.
package missing

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

type PageData struct {
	Path string `json:"path"`
}

// docs: not-found
func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, skgo.Errorf(http.StatusNotFound, "Not Found")
}

var _ = skgo.Load(pageLoad)

// docs: end not-found
