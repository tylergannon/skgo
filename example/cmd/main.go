// Command skgo-example serves the SvelteKit app in example/web from Go.
//
// With --proxy it forwards everything to a running `vp dev` server; without
// it, it serves the build embedded at compile time and no Node process is
// involved at all. Either way remote-function calls are answered by Go: the
// registry sits in front of both the proxy and the static handler, because in
// dev kit's own server would otherwise run the throwing client stub.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/generated"
	"github.com/tylergannon/skgo/example/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	proxy := flag.String("proxy", "", "URL of a `vp dev` server to proxy to; empty serves the embedded build")
	origin := flag.String("origin", "", "app origin used for the remote-function CSRF check; empty derives it from --listen")
	flag.Parse()

	if *origin == "" {
		*origin = "http://" + *listen
	}

	handler, mode, err := build(*proxy, *origin)
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}

	log.Printf("skgo-example: pid %d listening on http://%s in %s mode", os.Getpid(), *listen, mode)
	if err := http.ListenAndServe(*listen, withMode(mode, handler)); err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
}

func build(proxy, origin string) (http.Handler, string, error) {
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, "", err
	}

	// The manifest is read in both modes: it is where appDir and base come
	// from, and those decide the URL prefix remote calls arrive on.
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, "", err
	}

	if proxy != "" {
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, "", err
		}
		// In dev the client is served by vite, not by this build, so its
		// baked version differs from the manifest's — sending
		// x-sveltekit-version would make it reload in a loop. Kit skips the
		// remote CSRF check in dev too.
		cfg := manifest.RemoteConfig("")
		cfg.Version = ""
		cfg.Dev = true
		cfg.CookieOrigin = origin
		remotes, err := skgo.NewRemotes(cfg, generated.Remotes()...)
		if err != nil {
			return nil, "", err
		}
		return remotes.Intercept(skgo.NewDevProxy(target, log.Printf)), "dev", nil
	}

	remotes, err := skgo.NewRemotes(manifest.RemoteConfig(origin), generated.Remotes()...)
	if err != nil {
		return nil, "", err
	}
	static, err := skgo.NewStaticHandler(dist)
	if err != nil {
		return nil, "", err
	}
	return remotes.Intercept(static), "prod", nil
}

// withMode stamps every response so a test can tell which server answered it.
func withMode(mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", mode)
		next.ServeHTTP(w, r)
	})
}
