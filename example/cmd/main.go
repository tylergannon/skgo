// Command skgo-example serves the SvelteKit app in example/web from Go.
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
	origin := flag.String("origin", "", "app origin used for the remote-function CSRF check; empty derives it from --listen")
	flag.Parse()

	if *origin == "" {
		*origin = "http://" + *listen
	}

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
	handler, err := example.NewHandler(dist, *origin)
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}

	log.Printf("skgo-example: pid %d listening on http://%s in production mode", os.Getpid(), *listen)
	if err := http.ListenAndServe(*listen, withMode(handler)); err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
}

// withMode stamps every response so a test can tell which server answered it.
func withMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", "prod")
		next.ServeHTTP(w, r)
	})
}
