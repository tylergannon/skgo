package skgo_test

import (
	"io/fs"
	"net/http"

	"github.com/tylergannon/skgo"
)

// skgo dev builds the application from NewDevelopmentHandler, and the
// production binary from NewHandler. Both call one newHandler: a nil
// *skgo.NativeDevelopment builds the goja renderer and the static handler,
// and the value skgo dev passes replaces both with calls into Kit, so every
// load, remote function and hook is the same Go code in both modes.
func ExampleNativeDevelopment_developmentHandler() {
	newHandler := func(dist fs.FS, origin string, development *skgo.NativeDevelopment) (http.Handler, error) {
		manifest, err := skgo.ReadManifest(dist)
		if err != nil {
			return nil, err
		}
		loads, err := skgo.NewLoads(manifest.LoadConfig(origin))
		if err != nil {
			return nil, err
		}
		remotes, err := skgo.NewRemotes(manifest.RemoteConfig(origin))
		if err != nil {
			return nil, err
		}
		ssr, err := development.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{})
		if err != nil {
			return nil, err
		}
		pages, err := development.NewStaticHandler(dist, skgo.WithSSR(ssr))
		if err != nil {
			return nil, err
		}
		return loads.Intercept(remotes.Intercept(pages)), nil
	}

	NewHandler := func(dist fs.FS, origin string) (http.Handler, error) {
		return newHandler(dist, origin, nil)
	}
	NewDevelopmentHandler := func(dist fs.FS, origin string, development *skgo.NativeDevelopment) (http.Handler, error) {
		return newHandler(dist, origin, development)
	}
	_, _ = NewHandler, NewDevelopmentHandler
}
