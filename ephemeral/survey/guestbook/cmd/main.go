// Command guestbook serves the ported junkyard guestbook from Go.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/survey/generated"
	"github.com/tylergannon/skgo/survey/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "address to listen on")
	proxy := flag.String("proxy", "", "URL of a `vp dev` server to proxy to; empty serves the embedded build")
	origin := flag.String("origin", "", "app origin used for the remote-function CSRF check")
	flag.Parse()

	if *origin == "" {
		*origin = "http://" + *listen
	}

	handler, mode, err := build(*proxy, *origin)
	if err != nil {
		log.Fatalf("guestbook: %v", err)
	}

	log.Printf("guestbook: pid %d listening on http://%s in %s mode", os.Getpid(), *listen, mode)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		log.Fatalf("guestbook: %v", err)
	}
}

func build(proxy, origin string) (http.Handler, string, error) {
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, "", err
	}
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		return nil, "", err
	}

	if proxy != "" {
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, "", err
		}
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
