// Package example composes the contract application's production handler.
package example

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/tylergannon/skgo"
	appstate "github.com/tylergannon/skgo/example/internal/app"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

const origin = "http://127.0.0.1:8080"

// NewHandler keeps artifacts and middleware local to this handler. Tests and
// the application's binary use the same generated registries and dispatch.
func NewHandler(dist fs.FS, middleware skgo.Middleware) (http.Handler, error) {
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, err
	}
	remoteCfg := manifest.RemoteConfig(origin)
	loadCfg := manifest.LoadConfig(origin)
	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, err
	}
	endpoints, err := skgo.NewEndpoints(manifest.EndpointConfig(origin), generated.Endpoints()...)
	if err != nil {
		return nil, err
	}
	actions, err := skgo.NewActions(generated.Actions()...)
	if err != nil {
		return nil, err
	}
	var app http.Handler
	internal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.ServeHTTP(w, r) })
	ssr, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{
		Runtimes:                        2,
		FilterSerializedResponseHeaders: appstate.SerializedHeader,
		Fetch:                           internal,
		Actions:                         actions,
	})
	if err != nil {
		return nil, err
	}
	pages, err := skgo.NewStaticHandler(dist, skgo.WithSSR(ssr))
	if err != nil {
		return nil, err
	}
	handleCfg := manifest.HandleConfig()
	handleCfg.Origin = origin
	handleCfg.Loads = loads
	handleCfg.Static = skgo.ServedAsFile(pages)
	handleCfg.ErrorTemplate = ssr.ErrorTemplate()
	endpoints.SetErrorTemplate(ssr.ErrorTemplate())
	app = skgo.Sequence(skgo.Handle(func(context.Context) error { return nil }).Middleware(), middleware).Intercept(
		handleCfg, loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
	return skgo.FetchConfig{
		Origin: origin, Base: manifest.Base, Handler: internal, Prerendered: manifest.Prerendered,
	}.Intercept(app), nil
}
