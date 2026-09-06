// Command skgo-example serves the SvelteKit app in example/web from Go.
//
// With --proxy it forwards everything to a running `vp dev` server; without
// it, it serves the build embedded at compile time and no Node process is
// involved at all.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	proxy := flag.String("proxy", "", "URL of a `vp dev` server to proxy to; empty serves the embedded build")
	flag.Parse()

	handler, mode, err := build(*proxy)
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}

	log.Printf("skgo-example: pid %d listening on http://%s in %s mode", os.Getpid(), *listen, mode)
	if err := http.ListenAndServe(*listen, withMode(mode, handler)); err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
}

func build(proxy string) (http.Handler, string, error) {
	if proxy != "" {
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, "", err
		}
		return skgo.NewDevProxy(target, log.Printf), "dev", nil
	}

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, "", err
	}
	handler, err := skgo.NewStaticHandler(dist)
	if err != nil {
		return nil, "", err
	}
	return handler, "prod", nil
}

// withMode stamps every response so a test can tell which server answered it.
func withMode(mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", mode)
		next.ServeHTTP(w, r)
	})
}
