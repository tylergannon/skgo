// Package nativehost runs an application's Go handler on a retained loopback
// origin for an embedded native shell.
package nativehost

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Host struct {
	mu       sync.Mutex
	create   func(origin string) (http.Handler, error)
	server   *http.Server
	listener net.Listener
	origin   string
}

func New(create func(string) (http.Handler, error)) *Host { return &Host{create: create} }

func (h *Host) Start() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server != nil {
		return h.origin, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	origin := "http://" + listener.Addr().String()
	handler, err := h.create(origin)
	if err != nil {
		listener.Close()
		return "", err
	}
	h.origin = origin
	h.serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/__skgo_health" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	return origin, nil
}
func (h *Host) serve(listener net.Listener, handler http.Handler) {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	h.server, h.listener = server, listener
	go func() { _ = server.Serve(listener) }()
}

// Recover rebinds the same origin without recreating the application's state.
// A failed rebind retains the handler and origin for another recovery attempt.
func (h *Host) Recover() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server == nil {
		return errors.New("native host is not started")
	}
	handler := h.server.Handler
	h.server.Close()
	if h.listener != nil {
		h.listener.Close()
		h.listener = nil
	}
	listener, err := net.Listen("tcp", strings.TrimPrefix(h.origin, "http://"))
	if err != nil {
		return err
	}
	h.serve(listener, handler)
	return nil
}
func (h *Host) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := h.server.Shutdown(ctx)
	if err != nil {
		h.server.Close()
	}
	if h.listener != nil {
		h.listener.Close()
	}
	h.server, h.listener, h.origin = nil, nil, ""
	return err
}
