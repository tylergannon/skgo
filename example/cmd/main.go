// Command skgo-example serves the SvelteKit app in example/web from Go.
//
// With --proxy it forwards everything to a running `vp dev` server; without
// it, it serves the build embedded at compile time and no Node process is
// involved at all. Either way remote-function calls and server loads are
// answered by Go: the registries sit in front of both the proxy and the static
// handler, because in dev kit's own server would otherwise run the throwing
// client stub.
//
// The stack itself lives in package example, so that the tests exercise this
// binary's composition rather than a copy of it.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/tylergannon/skgo/example"
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

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
	handler, mode, err := example.NewHandler(dist, *proxy, *origin)
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}

	log.Printf("skgo-example: pid %d listening on http://%s in %s mode", os.Getpid(), *listen, mode)
	if err := http.ListenAndServe(*listen, withMode(mode, handler)); err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
}

// withMode stamps every response so a test can tell which server answered it.
func withMode(mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", mode)
		next.ServeHTTP(w, r)
	})
}
