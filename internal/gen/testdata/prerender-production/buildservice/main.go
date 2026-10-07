package main

import (
	"fmt"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
	"log"
	"net/http"
)

func main() {
	if err := skgo.RunPrerenderService(nil, generated.Loads(), generated.Remotes(), skgo.PrerenderServiceOptions{
		BindRequest: func(next http.Handler) http.Handler {
			return generated.RequestBoundary(skgo.HandleConfig{Matchers: generated.Matchers(), ClientAddress: func(*http.Request) (string, error) { return "", fmt.Errorf("unavailable during prerender") }}, next)
		},
		Matchers: generated.Matchers(), Endpoints: generated.Endpoints(), FilterSerializedResponseHeaders: app.SerializedHeader,
	}); err != nil {
		log.Fatal(err)
	}
}
