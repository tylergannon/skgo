// Command prerender is a hand-written prerender build service: the generated
// one plus the response-header filter the handle hook uses. Point the adapter's
// prerenderPackage option at it to use it.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/tylergannon/skgo"

	"github.com/tylergannon/skgo/example/internal/app"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

func main() {
	err := skgo.RunPrerenderService(generated.Transport(), generated.Loads(), generated.Remotes(), skgo.PrerenderServiceOptions{
		BindRequest: func(next http.Handler) http.Handler {
			return generated.RequestBoundary(skgo.HandleConfig{
				Matchers: generated.Matchers(),
				ClientAddress: func(*http.Request) (string, error) {
					return "", fmt.Errorf("unavailable during prerender")
				},
			}, next)
		},
		Endpoints: generated.Endpoints(),
		Matchers:  generated.Matchers(),
		// The same filter the handle hook passes to resolve.
		FilterSerializedResponseHeaders: app.SerializedHeader,
	})
	if err != nil {
		log.Fatal(err)
	}
}
