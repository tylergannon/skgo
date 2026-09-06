// Package site serves the remote functions of the route tree's own root
// directory. That directory holds the generated module boundary, so it is the
// one package the link tree addresses file by file rather than as a whole.
package site

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Site describes the running application.
type Site struct {
	// Name is the application's name.
	Name string `json:"name"`
	// Colocated says which directory the Go that answered lives in.
	Colocated string `json:"colocated"`
}

func getSite(ctx context.Context, _ skgo.None) (Site, error) {
	return Site{Name: "skgo", Colocated: "src/routes/site.remote.go"}, nil
}

var _ = skgo.Query(getSite)
