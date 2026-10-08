// Package main exposes the real example application to the iOS host through
// Go's c-archive boundary. The JavaScript renderer remains the pooled Go engine.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/web"
)

var host struct {
	sync.Mutex
	server *http.Server
	origin string
}

// SKGoHostStart returns a caller-owned C string, released with free. Nil means
// startup failed; the error is written to the application's log. Calls serialize.
//
//export SKGoHostStart
func SKGoHostStart() *C.char {
	origin := start()
	if origin == "" {
		return nil
	}
	return C.CString(origin)
}

func start() string {
	host.Lock()
	defer host.Unlock()
	if host.server != nil {
		return host.origin
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Print(err)
		return ""
	}
	origin := "http://" + listener.Addr().String()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		listener.Close()
		log.Print(err)
		return ""
	}
	// More than one runtime is retained: reentrant rendering cannot use a
	// single JavaScript runtime. This is the same production composition.
	handler, _, err := example.NewHandlerSized(dist, "", origin, 2)
	if err != nil {
		listener.Close()
		log.Print(err)
		return ""
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	host.server, host.origin = server, origin
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	return origin
}

//export SKGoHostStop
func SKGoHostStop() {
	host.Lock()
	defer host.Unlock()
	if host.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := host.server.Shutdown(ctx); err != nil {
		host.server.Close()
	}
	host.server, host.origin = nil, ""
}
func main() {}
