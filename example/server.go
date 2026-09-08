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

// supportID is the fixture value HandleError adds to every failure. It is a
// literal on purpose, the same way businesslogic.Default's fixtures are:
// something the Gherkin suite and the Go tests can both assert against
// without asking the app what it just rendered.
const supportID = "case-1121"

// HandleError is the app's `handleError` hook — kit's own contract, mirrored:
// it runs for every error a page render raises, expected or not
// (`exports/hooks/public.d.ts`: "runs for every error thrown ... except
// redirects"), and whatever it returns is merged over what the error already
// carries.
//
// Every failure gets a support id, which is the one thing a real error page
// almost always adds and kit's own default has no room for. An error nobody
// meant to happen also loses its own message here: this app makes no promise
// about what an arbitrary Go error's text might contain, so the visitor is
// told something a person wrote instead of it. caught.Err still carries the
// real one, for a hook that wants to log it before replacing it.
func HandleError(ctx context.Context, caught skgo.CaughtError) map[string]any {
	extra := map[string]any{"supportId": supportID}
	if caught.Kind == "unknown" {
		extra["message"] = "Something went wrong on our end."
	}
	return extra
}

// NewHandler builds the app's server over the build in dist. With a non-empty
// proxy it forwards pages to a running `vp dev` server; otherwise it serves the
// embedded build.
//
// The order is kit's own dispatch order, turned inside out into middleware.
// Handle is outermost because kit runs `handle` before it dispatches to
// anything, for every kind of request — that is not the loads registry's
// business even though this app also has one. Under it, the loads registry:
// `__data.json` must never reach the static handler, which would answer it
// with the boot document. Then remote functions, which live under the app
// directory and are not routes at all. Then the server routes, which own
// every path kit compiled a `+server.ts` for — in dev too, where kit's own
// server would otherwise run the generated stub and throw. Pages are last,
// because in kit they are what answers when nothing else did.
func NewHandler(dist fs.FS, proxy, origin string) (http.Handler, string, error) {
	// The manifest is read in both modes: it is where appDir and base come
	// from, and those decide the URL prefix remote calls arrive on.
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, "", err
	}

	remoteCfg := manifest.RemoteConfig(origin)
	loadCfg := manifest.LoadConfig(origin)
	endpointCfg := manifest.EndpointConfig(origin)
	handleCfg := manifest.HandleConfig()

	mode := "prod"
	var pages http.Handler
	var static func(*skgo.Loads, *skgo.Remotes) (http.Handler, error)
	// Declared here, ahead of the closure below that reaches into it: the
	// closure runs after NewEndpoints has filled this in, but a closure
	// captures the variable itself and Go resolves the name when the literal
	// is written, not when it is called.
	var endpoints *skgo.Endpoints
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
		handleCfg.Version = ""
		mode, pages = "dev", skgo.NewDevProxy(target, log.Printf)
	} else {
		// The renderer needs the loads and the remote functions, and they need
		// the manifest, so the page handler is built last — after both of the
		// registries it renders with exist.
		static = func(loads *skgo.Loads, remotes *skgo.Remotes) (http.Handler, error) {
			// A render-time `event.fetch` of the app's own routes is answered
			// by the same server-route registry a real request to that path
			// would reach — `endpoints`, filled in below before this closure
			// ever runs — with nothing beneath it: a fetch that matches no
			// `+server.ts` refuses rather than recursing back into the page
			// renderer whose own render is what asked for this fetch.
			ssr, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{
				Fetch: endpoints.Intercept(http.NotFoundHandler()),
			})
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
	loadCfg.HandleError = HandleError

	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, "", err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, "", err
	}
	endpoints, err = skgo.NewEndpoints(endpointCfg, generated.Endpoints()...)
	if err != nil {
		return nil, "", err
	}
	if static != nil {
		if pages, err = static(loads, remotes); err != nil {
			return nil, "", err
		}
	}
	// Handle mounts outermost: kit runs `handle` before it dispatches to
	// anything, and that is true of every registry below, not just the loads
	// one that happens to also answer `__data.json`.
	return skgo.Handle(Handle).Intercept(handleCfg,
		loads.Intercept(remotes.Intercept(endpoints.Intercept(pages)))), mode, nil
}
