package migration

import (
	"io/fs"
	"net/http"

	"example.com/migration/internal/generated"
	"github.com/tylergannon/skgo"
)

func NewHandler(dist fs.FS) (http.Handler, error) {
	const origin = "http://127.0.0.1:8080"
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, err
	}
	remotes, err := skgo.NewRemotes(manifest.RemoteConfig(origin), generated.Remotes()...)
	if err != nil {
		return nil, err
	}
	loads, err := skgo.NewLoads(manifest.LoadConfig(origin), generated.Loads()...)
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
	ssr, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{Runtimes: 2, Actions: actions})
	if err != nil {
		return nil, err
	}
	pages, err := skgo.NewStaticHandler(dist, skgo.WithSSR(ssr))
	if err != nil {
		return nil, err
	}
	return loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))), nil
}
