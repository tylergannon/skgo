// Command skgo-example serves the SvelteKit app in example/web from Go.
//
// The stack itself lives in package example, so that the tests exercise this
// binary's composition rather than a copy of it.
package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/web"
)

// docs: main
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	origin := flag.String("origin", "", "app origin used for the remote-function CSRF check; empty derives it from --listen")
	pprofAddr := flag.String("pprof", "", "address for the net/http/pprof server; empty leaves profiling off")
	flag.Parse()

	if *origin == "" {
		*origin = "http://" + *listen
	}

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}
	// docs: mux
	handler, err := example.NewHandler(dist, *origin)
	if err != nil {
		log.Fatalf("skgo-example: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", withMode(handler))
	// docs: end mux

	// docs: pprof
	if *pprofAddr != "" {
		go func() { log.Println(http.ListenAndServe(*pprofAddr, nil)) }()
	}
	// docs: end pprof

	log.Printf("skgo-example: pid %d listening on http://%s in production mode", os.Getpid(), *listen)
	// docs: shutdown
	srv := &http.Server{Addr: *listen, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("skgo-example: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Print(err)
	}
	// docs: end shutdown
}

// docs: end main

// withMode stamps every response so a test can tell which server answered it.
func withMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", "prod")
		next.ServeHTTP(w, r)
	})
}
