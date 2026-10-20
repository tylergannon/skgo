// Command skgo-example serves the SvelteKit app in example/web from Go.
//
// The stack itself lives in package example, so that the tests exercise this
// binary's composition rather than a copy of it.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/web"
)

func main() {
	if err := run(); err != nil {
		log.Printf("skgo-example: %v", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	origin := flag.String("origin", "", "app origin used for the remote-function CSRF check; empty derives it from --listen")
	pprofAddr := flag.String("pprof", "", "address for the net/http/pprof server; empty leaves profiling off")
	flag.Parse()

	if *origin == "" {
		*origin = "http://" + *listen
	}

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return err
	}
	handler, err := example.NewHandler(dist, *origin)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", withMode(handler))

	if *pprofAddr != "" {
		go func() { log.Println(http.ListenAndServe(*pprofAddr, nil)) }()
	}

	// Bind before announcing: a port another process holds fails here, before
	// any line claims this server is up.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	log.Printf("skgo-example: pid %d listening on http://%s in production mode", os.Getpid(), *listen)
	srv := &http.Server{Handler: mux}
	defer srv.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; err != http.ErrServerClosed {
		return err
	}
	return nil
}

// withMode stamps every response so a test can tell which server answered it.
func withMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Skgo-Mode", "prod")
		next.ServeHTTP(w, r)
	})
}
