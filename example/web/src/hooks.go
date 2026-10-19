// Package hooks is the app's universal hooks, the Go half of what
// `src/hooks.ts` declares beside it.
//
// Kit's `transport` is a universal hook because both sides of the wire need it:
// the browser needs `decode` to rebuild a class instance, and the server needs
// `encode` to say what that instance is made of. The two halves are one
// declaration, so they live next to each other.
package hooks

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// Money crosses the wire as a custom type. Without this a price arrives in the
// browser as a plain `{ cents: 2000 }` and `price.format()` throws.
var _ = skgo.Transported[businesslogic.Money]("Money")

// Reroute selects a route from Go while preserving the visitor's alias URL.
var Reroute skgo.Reroute = func(ctx context.Context, event skgo.RerouteEvent) (*string, error) {
	if !strings.HasPrefix(event.URL.Path, "/go-alias/") {
		return nil, nil
	}
	name := strings.TrimPrefix(event.URL.Path, "/go-alias/")
	request, err := http.NewRequestWithContext(ctx, "GET", "/api/reroute-target?name="+name, nil)
	if err != nil {
		return nil, err
	}
	response, err := event.Fetch(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	path := string(body)
	return &path, nil
}
