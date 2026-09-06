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
// embedded build. Either way the loads registry is outermost and the remote
// registry sits in front of whatever answers pages: kit runs `handle` before it
// dispatches to a page, a data request or a remote function, and `__data.json`
// must never reach the static handler, which would answer it with the boot
// document.
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

	mode := "prod"
	var pages http.Handler
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
		mode, pages = "dev", skgo.NewDevProxy(target, log.Printf)
	} else {
		static, err := skgo.NewStaticHandler(dist)
		if err != nil {
			return nil, "", err
		}
		pages = static
	}

	remotes, err := skgo.NewRemotes(remoteCfg, generated.Remotes()...)
	if err != nil {
		return nil, "", err
	}
	loads, err := skgo.NewLoads(loadCfg, generated.Loads()...)
	if err != nil {
		return nil, "", err
	}
	return loads.Intercept(remotes.Intercept(pages)), mode, nil
}
