// Command TaskList serves the SvelteKit application from Go.
package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	app "github.com/tylergannon/skgo/plugins/mcp/example"
	"github.com/tylergannon/skgo/plugins/mcp/example/web"
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
	apiKey := flag.String("api-key", "", "Optional API key; browser access uses HTTP Basic with the key as password")
	flag.Parse()
	if *apiKey == "" {
		*apiKey = os.Getenv("SKGO_API_KEY")
	}

	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return err
	}
	handler, err := app.NewHandler(dist, *trusted)
	if err != nil {
		return err
	}
	if *apiKey != "" {
		next := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if _, password, ok := r.BasicAuth(); ok && key == "" {
				key = password
			}
			if subtle.ConstantTimeCompare([]byte(key), []byte(*apiKey)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="TaskList"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	// Bind before announcing: a port another process holds fails here, before
	// any line claims this server is up.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	log.Printf("TaskList: listening on http://%s", *listen)
	srv := &http.Server{Handler: handler}
	defer srv.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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

func hostPort(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return "127.0.0.1:8080"
	}
	return u.Host
}
