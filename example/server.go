// Package example assembles the demo app's server. It exists so that there is
// exactly one production stack: the binary in cmd and the tests beside this
// file both call NewHandler, and neither can be green over a composition the
// other does not use.
//
// The composition is load-bearing and was got wrong once: a test that built
// only `remotes.Intercept(static)` kept asserting that every `__data.json` is
// refused 404, which is what the static handler does on its own and the
// opposite of what the app does.
package example

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"net/url"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/generated"
)

// SessionCookie is the cookie the session id travels in. This is the one place
// the app spells it: everything downstream reads the session, not the cookie.
const SessionCookie = "skgo_session"

// Handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with skgo.LocalOf.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func Handle(ctx context.Context) error {
	id, _ := skgo.EventFrom(ctx).Cookie(SessionCookie)
	return skgo.SetLocal(ctx, businesslogic.Default.Session(id))
}

// NewHandler builds the app's server over the build in dist. With a non-empty
// proxy it forwards pages to a running `vp dev` server; otherwise it serves the
// embedded build.
//
// The order is kit's own dispatch order, turned inside out into middleware. The
// loads registry is outermost because kit runs `handle` before it dispatches to
// anything, and because `__data.json` must never reach the static handler,
// which would answer it with the boot document. Then remote functions, which
// live under the app directory and are not routes at all. Then the server
// routes, which own every path kit compiled a `+server.ts` for — in dev too,
// where kit's own server would otherwise run the generated stub and throw.
// Pages are last, because in kit they are what answers when nothing else did.
func NewHandler(dist fs.FS, proxy, origin string) (http.Handler, string, error) {
	// The manifest is read in both modes: it is where appDir and base come
	// from, and those decide the URL prefix remote calls arrive on.
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, "", err
	}

	remoteCfg := manifest.RemoteConfig(origin)
	loadCfg := manifest.LoadConfig(origin)
	loadCfg.Handle = Handle
	endpointCfg := manifest.EndpointConfig(origin)

	mode := "prod"
	var pages http.Handler
	var static func(*skgo.Loads, *skgo.Remotes) (http.Handler, error)
	if proxy != "" {
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, "", err
		}
		// In dev the client is served by vite, not by this build, so its
		// baked version differs from the manifest's — sending
		// x-sveltekit-version would make it reload in a loop. Kit skips the
		// remote CSRF check in dev too.
		remoteCfg = manifest.RemoteConfig("")
		remoteCfg.Version = ""
		remoteCfg.Dev = true
		remoteCfg.CookieOrigin = origin
		loadCfg.Version = ""
		loadCfg.Dev = true
		endpointCfg.Dev = true
		mode, pages = "dev", skgo.NewDevProxy(target, log.Printf)
	} else {
		// The renderer needs the loads and the remote functions, and they need
		// the manifest, so the page handler is built last — after both of the
		// registries it renders with exist.
		static = func(loads *skgo.Loads, remotes *skgo.Remotes) (http.Handler, error) {
			ssr, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{})
			if err != nil {
				return nil, err
			}
			return skgo.NewStaticHandler(dist, skgo.WithSSR(ssr))
		}
	}

	// The `transport` hook, declared in web/src/hooks.go beside the
	// web/src/hooks.ts that holds its browser half. Both registries get it: a
	// custom type reaches the browser through a remote function and through a
	// server load alike.
	//
	// After the branch above, not before it: the dev arm replaces remoteCfg
	// wholesale with a fresh manifest.RemoteConfig, so anything set earlier is
	// dropped. Setting it above cost a run — the suite was green in prod and
	// the pricing page threw `price.format is not a function` in dev.
	remoteCfg.Transport = generated.Transport()
	loadCfg.Transport = generated.Transport()

	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, "", err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, "", err
	}
	endpoints, err := skgo.NewEndpoints(endpointCfg, generated.Endpoints()...)
	if err != nil {
		return nil, "", err
	}
	if static != nil {
		if pages, err = static(loads, remotes); err != nil {
			return nil, "", err
		}
	}
	return loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))), mode, nil
}
