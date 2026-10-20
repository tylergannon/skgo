// Command TinyNative serves the SvelteKit application from Go.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	app "github.com/tylergannon/skgo/plugins/native-shell/example"
	"github.com/tylergannon/skgo/plugins/native-shell/example/web"
)

var origin = "http://127.0.0.1:8080"

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", hostPort(origin), "address to listen on")
	trusted := flag.String("origin", origin, "public browser origin")
	flag.Parse()

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return err
	}
	handler, err := app.NewHandler(dist, *trusted)
	if err != nil {
		return err
	}
	log.Printf("TinyNative: listening on http://%s", *listen)
	srv := &http.Server{Addr: *listen, Handler: handler}
	defer srv.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()
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

func hostPort(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return "127.0.0.1:8080"
	}
	return u.Host
}
